package videomodeltest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	videoprovider "github.com/fatballfish/pic-gallery/internal/provider/video"
	"github.com/fatballfish/pic-gallery/internal/provider/video/bailian"
	"github.com/fatballfish/pic-gallery/internal/provider/video/gasic"
	"github.com/fatballfish/pic-gallery/internal/provider/video/minimax"
	"github.com/fatballfish/pic-gallery/internal/provider/video/seedance"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

// AccountLoader is the narrow read surface needed to build a one-off provider
// client for an administrator-initiated video model test.
type AccountLoader interface {
	GetModelAccount(context.Context, int64) (domainmodeladmin.ModelAccount, error)
	GetModelAccountModel(context.Context, int64) (domainmodeladmin.ModelAccountModel, error)
}

type Input struct {
	Role      string `json:"role"`
	URL       string `json:"url"`
	MediaType string `json:"media_type,omitempty"`
	Ordinal   int    `json:"ordinal,omitempty"`
}

type Request struct {
	AccountID       int64   `json:"-"`
	AccountModelID  int64   `json:"model_id"`
	Prompt          string  `json:"prompt"`
	TaskType        string  `json:"task_type"`
	DurationSeconds int     `json:"duration_seconds"`
	Resolution      string  `json:"resolution"`
	AspectRatio     string  `json:"aspect_ratio"`
	GenerateAudio   bool    `json:"generate_audio"`
	Inputs          []Input `json:"inputs,omitempty"`
}

type Artifact struct {
	URL       string     `json:"url"`
	MIMEType  string     `json:"mime_type,omitempty"`
	SizeBytes int64      `json:"size_bytes,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type Test struct {
	ID             string     `json:"id"`
	AccountID      int64      `json:"account_id"`
	AccountModelID int64      `json:"account_model_id"`
	Provider       string     `json:"provider"`
	ModelCode      string     `json:"model_code"`
	TaskType       string     `json:"task_type"`
	Prompt         string     `json:"prompt"`
	Status         string     `json:"status"`
	ProviderJobID  string     `json:"provider_job_id,omitempty"`
	Error          string     `json:"error,omitempty"`
	Artifacts      []Artifact `json:"artifacts,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Service struct {
	loader       AccountLoader
	mu           sync.Mutex
	tests        map[string]*Test
	newProvider  func(domainmodeladmin.ModelAccount, domainmodeladmin.ModelAccountModel) (videoprovider.Provider, error)
	pollInterval time.Duration
	timeout      time.Duration
	now          func() time.Time
}

func NewService(loader AccountLoader) *Service {
	return &Service{
		loader:       loader,
		tests:        map[string]*Test{},
		newProvider:  buildProvider,
		pollInterval: 2 * time.Second,
		timeout:      10 * time.Minute,
		now:          func() time.Time { return time.Now().UTC() },
	}
}

// Submit creates a test job. Provider submission is synchronous (it only
// creates the upstream job); polling continues in the background so the admin
// UI can show progress without holding a long HTTP request.
func (s *Service) Submit(ctx context.Context, req Request) (Test, error) {
	if s == nil || s.loader == nil {
		return Test{}, errs.Internal("video model test service is unavailable")
	}
	normalized, err := normalizeRequest(req)
	if err != nil {
		return Test{}, err
	}
	account, err := s.loader.GetModelAccount(ctx, normalized.AccountID)
	if err != nil {
		return Test{}, err
	}
	model, err := s.loader.GetModelAccountModel(ctx, normalized.AccountModelID)
	if err != nil {
		return Test{}, err
	}
	if model.AccountID != account.ID {
		return Test{}, errs.BadRequest("model does not belong to account")
	}
	if !model.Enabled {
		return Test{}, errs.New(409, "MODEL_DISABLED", "model is disabled")
	}
	if strings.TrimSpace(account.CredentialsEncrypted["api_key"]) == "" {
		return Test{}, errs.BadRequest("account api_key is required for a video model test")
	}
	provider, err := s.newProvider(account, model)
	if err != nil {
		return Test{}, errs.BadRequest(err.Error())
	}

	now := s.now()
	test := &Test{
		ID: uuid.NewString(), AccountID: account.ID, AccountModelID: model.ID,
		Provider: strings.ToLower(account.AdapterType), ModelCode: model.ModelCode,
		TaskType: normalized.TaskType, Prompt: normalized.Prompt, Status: "queued",
		CreatedAt: now, UpdatedAt: now,
	}
	s.store(test)
	s.cleanup(now)

	job, submitErr := provider.Submit(ctx, toProviderRequest(test.ID, normalized))
	if submitErr != nil {
		s.finish(test.ID, "failed", "", submitErr.Error(), nil)
		return s.Get(test.ID)
	}
	test.ProviderJobID = job.ID
	test.Status = stateLabel(job.State)
	s.store(test)
	go s.poll(test.ID, provider, videoprovider.JobRef{ID: job.ID})
	return s.Get(test.ID)
}

func (s *Service) Get(id string) (Test, error) {
	if s == nil {
		return Test{}, errs.Internal("video model test service is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	test, ok := s.tests[strings.TrimSpace(id)]
	if !ok {
		return Test{}, errs.New(404, "VIDEO_MODEL_TEST_NOT_FOUND", "video model test not found")
	}
	return cloneTest(test), nil
}

func (s *Service) poll(id string, provider videoprovider.Provider, ref videoprovider.JobRef) {
	defer func() {
		if recovered := recover(); recovered != nil {
			s.finish(id, "failed", "", fmt.Sprintf("video model test panicked: %v", recovered), nil)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	for {
		status, err := provider.Get(ctx, ref)
		if err != nil {
			if ctx.Err() != nil {
				s.finish(id, "failed", "", "video model test timed out", nil)
				return
			}
			s.finish(id, "failed", "", err.Error(), nil)
			return
		}
		artifacts := mapArtifacts(status.Artifacts)
		switch status.State {
		case videoprovider.StateSucceeded:
			s.finish(id, "succeeded", status.JobID, "", artifacts)
			return
		case videoprovider.StateFailed:
			s.finish(id, "failed", status.JobID, firstNonEmpty(status.ErrorMessage, status.ErrorCode, "provider reported a failed test"), artifacts)
			return
		case videoprovider.StateCancelled:
			s.finish(id, "cancelled", status.JobID, status.ErrorMessage, artifacts)
			return
		default:
			s.update(id, stateLabel(status.State), status.JobID, artifacts)
		}
		select {
		case <-ctx.Done():
			s.finish(id, "failed", status.JobID, "video model test timed out", artifacts)
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) store(test *Test) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := cloneTest(test)
	s.tests[test.ID] = &cloned
}

func (s *Service) update(id, status, jobID string, artifacts []Artifact) {
	s.mu.Lock()
	defer s.mu.Unlock()
	test, ok := s.tests[id]
	if !ok {
		return
	}
	if status != "" {
		test.Status = status
	}
	if jobID != "" {
		test.ProviderJobID = jobID
	}
	if artifacts != nil {
		test.Artifacts = artifacts
	}
	test.UpdatedAt = s.now()
}

func (s *Service) finish(id, status, jobID, message string, artifacts []Artifact) {
	s.mu.Lock()
	defer s.mu.Unlock()
	test, ok := s.tests[id]
	if !ok {
		return
	}
	test.Status = status
	if jobID != "" {
		test.ProviderJobID = jobID
	}
	test.Error = message
	if artifacts != nil {
		test.Artifacts = artifacts
	}
	test.UpdatedAt = s.now()
}

func (s *Service) cleanup(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, test := range s.tests {
		if now.Sub(test.UpdatedAt) > time.Hour {
			delete(s.tests, id)
		}
	}
}

func normalizeRequest(req Request) (Request, error) {
	if req.AccountID <= 0 || req.AccountModelID <= 0 {
		return Request{}, errs.BadRequest("account_id and model_id are required")
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		return Request{}, errs.BadRequest("prompt is required")
	}
	req.TaskType = strings.TrimSpace(req.TaskType)
	if req.TaskType == "" {
		return Request{}, errs.BadRequest("task_type is required")
	}
	if req.DurationSeconds <= 0 {
		req.DurationSeconds = 5
	}
	if strings.TrimSpace(req.Resolution) == "" {
		req.Resolution = "480p"
	}
	if strings.TrimSpace(req.AspectRatio) == "" {
		req.AspectRatio = "16:9"
	}
	for index := range req.Inputs {
		req.Inputs[index].Role = strings.TrimSpace(req.Inputs[index].Role)
		req.Inputs[index].URL = strings.TrimSpace(req.Inputs[index].URL)
		req.Inputs[index].MediaType = strings.TrimSpace(req.Inputs[index].MediaType)
		if req.Inputs[index].Role == "" || req.Inputs[index].URL == "" {
			return Request{}, errs.BadRequest("video test inputs require role and url")
		}
	}
	return req, nil
}

func toProviderRequest(id string, req Request) videoprovider.Request {
	inputs := make([]videoprovider.Input, 0, len(req.Inputs))
	for _, input := range req.Inputs {
		inputs = append(inputs, videoprovider.Input{
			AssetID: input.URL, Role: input.Role, URL: input.URL, Ordinal: input.Ordinal, MediaType: input.MediaType,
		})
	}
	return videoprovider.Request{
		TaskID: id, ItemID: id, AttemptID: id, IdempotencyKey: "video-model-test-" + id,
		TaskType: req.TaskType, Prompt: req.Prompt, DurationSeconds: req.DurationSeconds,
		Resolution: req.Resolution, AspectRatio: req.AspectRatio, GenerateAudio: req.GenerateAudio,
		Inputs: inputs,
	}
}

func buildProvider(account domainmodeladmin.ModelAccount, model domainmodeladmin.ModelAccountModel) (videoprovider.Provider, error) {
	timeout := time.Duration(account.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	switch strings.ToLower(strings.TrimSpace(account.AdapterType)) {
	case "seedance":
		return seedance.NewClient(seedance.Config{
			BaseURL: account.BaseURL, APIKey: strings.TrimSpace(account.CredentialsEncrypted["api_key"]),
			ModelCode: model.ModelCode, Timeout: timeout, Verified: true,
		})
	case "minimax":
		return minimax.NewClient(minimax.Config{
			BaseURL: account.BaseURL, APIKey: strings.TrimSpace(account.CredentialsEncrypted["api_key"]),
			ModelCode: model.ModelCode, Timeout: timeout, Verified: true,
		})
	case "gasic":
		return gasic.NewClient(gasic.Config{
			BaseURL: account.BaseURL, APIKey: strings.TrimSpace(account.CredentialsEncrypted["api_key"]),
			ModelCode: model.ModelCode, Timeout: timeout, Verified: true,
		})
	case "bailian":
		return bailian.NewClient(bailian.Config{
			BaseURL: account.BaseURL, APIKey: strings.TrimSpace(account.CredentialsEncrypted["api_key"]),
			ModelCode: model.ModelCode, Timeout: timeout, Verified: true,
		})
	default:
		return nil, fmt.Errorf("adapter %q does not support video model tests", account.AdapterType)
	}
}

func stateLabel(state videoprovider.State) string {
	switch state {
	case videoprovider.StateQueued:
		return "queued"
	case videoprovider.StateRunning:
		return "running"
	case videoprovider.StateSucceeded:
		return "succeeded"
	case videoprovider.StateFailed:
		return "failed"
	case videoprovider.StateCancelled:
		return "cancelled"
	default:
		return "running"
	}
}

func mapArtifacts(items []videoprovider.Artifact) []Artifact {
	if len(items) == 0 {
		return nil
	}
	result := make([]Artifact, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.URL) == "" {
			continue
		}
		result = append(result, Artifact{URL: item.URL, MIMEType: item.MIMEType, SizeBytes: item.SizeBytes, ExpiresAt: item.ExpiresAt})
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cloneTest(test *Test) Test {
	cloned := *test
	if test.Artifacts != nil {
		cloned.Artifacts = append([]Artifact(nil), test.Artifacts...)
	}
	return cloned
}
