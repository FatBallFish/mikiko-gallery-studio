package video

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	providervideo "github.com/fatballfish/pic-gallery/internal/provider/video"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/shopspring/decimal"
)

var ErrStepConflict = errors.New("video worker step conflict")

type Attempt struct {
	ID               string
	No               int
	RouteCandidateID int64
	AccountModelID   int64
	ModelAccountID   int64
	ProviderCode     string
	ModelCode        string
	IdempotencyKey   string
	JobID            string
	Status           string
	PlatformAbsorbed bool
}

type WorkItem struct {
	ID                  string
	TaskID              string
	UserID              int64
	ProjectID           string
	ProviderCode        string
	State               domainvideo.ItemState
	Version             int64
	PricePoints         string
	ActualPoints        string
	Request             providervideo.Request
	Attempt             Attempt
	Artifact            providervideo.Artifact
	ArtifactAttempts    int
	MaxArtifactAttempts int
	ResultAssetID       string
	ErrorCode           string
	ErrorMessage        string
	NeedsSettlement     bool
	NextActionAt        *time.Time
	LeaseOwner          string
	LeaseExpiresAt      time.Time
}

type ClaimRequest struct {
	Owner    string
	Now      time.Time
	LeaseTTL time.Duration
}

type PrepareAttemptRequest struct {
	ItemID                 string
	Owner                  string
	ExpectedVersion        int64
	AttemptID              string
	ProviderIdempotencyKey string
}

type ApplyStepRequest struct {
	ItemID                    string
	Owner                     string
	ExpectedVersion           int64
	Target                    domainvideo.ItemState
	ProviderJobID             string
	AttemptStatus             string
	Artifact                  providervideo.Artifact
	ProviderStatusSnapshot    map[string]any
	UsageRaw                  map[string]any
	UsageNormalized           providervideo.Usage
	UsageNormalizationError   string
	PlatformAbsorbed          bool
	ErrorCode                 string
	ErrorMessage              string
	NextActionAt              *time.Time
	IncrementArtifactAttempts bool
	ArtifactExhausted         bool
}

type ArtifactCommitRequest struct {
	ItemID          string
	Owner           string
	ExpectedVersion int64
	AssetID         string
	UserID          int64
	ProjectID       string
	Status          string
	StorageConfigID string
	StorageDriver   string
	Bucket          string
	ObjectKey       string
	MIMEType        string
	SizeBytes       int64
	SHA256          string
}

type SettlementItem struct {
	State        domainvideo.ItemState
	PricePoints  string
	UsagePending bool
}

type SettlementSnapshot struct {
	TaskID         string
	ReservedPoints string
	Items          []SettlementItem
}

type FinalizeRequest struct {
	TaskID             string
	Status             domainvideo.TaskStatus
	SuccessOutputCount int
	ActualPoints       string
	ReservedPoints     string
}

type LeaseRef struct {
	ItemID string
	Owner  string
}

type Store interface {
	ClaimDue(context.Context, ClaimRequest) (WorkItem, bool, error)
	PrepareAttempt(context.Context, PrepareAttemptRequest) (WorkItem, error)
	ApplyStep(context.Context, ApplyStepRequest) (bool, error)
	CommitArtifact(context.Context, ArtifactCommitRequest) (bool, error)
	LoadSettlement(context.Context, string) (SettlementSnapshot, error)
	FinalizeTask(context.Context, FinalizeRequest) (bool, error)
	ReleaseLease(context.Context, LeaseRef) error
}

type ProviderRef struct {
	RouteCandidateID int64
	AccountModelID   int64
	ModelAccountID   int64
	ProviderCode     string
	ModelCode        string
}

type ProviderResolver interface {
	Resolve(context.Context, ProviderRef) (ResolvedExecution, error)
}

type ResolvedExecution struct {
	Provider             providervideo.Provider
	ArtifactAllowedHosts []string
}

// Reconciler is an optional provider capability for looking up an uncertain
// submission by the platform idempotency key without submitting it again.
type Reconciler interface {
	Reconcile(context.Context, providervideo.Request) (providervideo.Job, bool, error)
}

type Observer interface {
	RecordVideoStage(stage, result string)
	RecordArtifactTransfer(mediaType, result string, bytes int64)
	RecordSettlement(kind, result string)
}

type Options struct {
	Owner                      string
	LeaseTTL                   time.Duration
	PollIntervals              []time.Duration
	ArtifactMaxBytes           int64
	ArtifactTransferTimeout    time.Duration
	ArtifactAllowedHosts       []string
	AllowLoopbackArtifactHosts bool
	HTTPClient                 *http.Client
	Now                        func() time.Time
	AttemptID                  func() string
	AssetID                    func() string
	ResolveHostIPs             func(context.Context, string) ([]net.IP, error)
	ClaimAllowed               func(context.Context) (bool, error)
	Observer                   Observer
}

type Runner struct {
	store      Store
	providers  ProviderResolver
	storage    storage.Router
	httpClient *http.Client
	options    Options
}

func NewRunner(store Store, providers ProviderResolver, storageRouter storage.Router, options Options) *Runner {
	if strings.TrimSpace(options.Owner) == "" {
		options.Owner = "video-worker"
	}
	if options.LeaseTTL <= 0 {
		options.LeaseTTL = 30 * time.Second
	}
	if len(options.PollIntervals) == 0 {
		options.PollIntervals = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 20 * time.Second, 30 * time.Second}
	}
	if options.ArtifactMaxBytes <= 0 {
		options.ArtifactMaxBytes = 1 << 30
	}
	if options.ArtifactTransferTimeout <= 0 {
		options.ArtifactTransferTimeout = 10 * time.Minute
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.AttemptID == nil {
		options.AttemptID = uuid.NewString
	}
	if options.AssetID == nil {
		options.AssetID = uuid.NewString
	}
	if options.ResolveHostIPs == nil {
		resolver := net.DefaultResolver
		options.ResolveHostIPs = func(ctx context.Context, host string) ([]net.IP, error) { return resolver.LookupIP(ctx, "ip", host) }
	}
	if storageRouter == nil {
		storageRouter = storage.NewStaticRouter(nil)
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Runner{store: store, providers: providers, storage: storageRouter, httpClient: httpClient, options: options}
}

func (r *Runner) RunOnce(ctx context.Context) (bool, error) {
	now := r.options.Now().UTC()
	if r.options.ClaimAllowed != nil {
		allowed, err := r.options.ClaimAllowed(ctx)
		if err != nil || !allowed {
			return false, err
		}
	}
	item, claimed, err := r.store.ClaimDue(ctx, ClaimRequest{Owner: r.options.Owner, Now: now, LeaseTTL: r.options.LeaseTTL})
	if err != nil || !claimed {
		return claimed, err
	}
	lease := LeaseRef{ItemID: item.ID, Owner: r.options.Owner}
	defer func() { _ = r.store.ReleaseLease(context.WithoutCancel(ctx), lease) }()

	if item.NeedsSettlement {
		done, settleErr := r.settle(ctx, item)
		if settleErr != nil || done {
			return true, settleErr
		}
		return true, nil
	}

	var stepErr error
	switch item.State {
	case domainvideo.ItemStateQueued:
		stepErr = r.submit(ctx, item)
	case domainvideo.ItemStateSubmitting, domainvideo.ItemStateReconciling:
		stepErr = r.reconcile(ctx, item)
	case domainvideo.ItemStateProviderQueued, domainvideo.ItemStateProviderRunning:
		stepErr = r.poll(ctx, item)
	case domainvideo.ItemStateCancelRequested:
		stepErr = r.cancel(ctx, item)
	case domainvideo.ItemStateArtifactPending, domainvideo.ItemStateRecoveryRequired:
		stepErr = r.persistArtifact(ctx, item)
	default:
		stepErr = nil
	}
	return true, stepErr
}

func (r *Runner) submit(ctx context.Context, item WorkItem) error {
	attemptID := r.options.AttemptID()
	idempotencyKey := item.TaskID + ":" + item.ID + ":" + attemptID
	prepared, err := r.store.PrepareAttempt(ctx, PrepareAttemptRequest{
		ItemID: item.ID, Owner: r.options.Owner, ExpectedVersion: item.Version,
		AttemptID: attemptID, ProviderIdempotencyKey: idempotencyKey,
	})
	if err != nil {
		if errors.Is(err, ErrStepConflict) {
			return nil
		}
		return err
	}
	provider, err := r.resolveProvider(ctx, prepared)
	if err != nil {
		return r.applyFailure(ctx, prepared, "provider_unavailable", err.Error(), false)
	}
	request := providerRequest(prepared)
	request, err = r.signInputs(ctx, request)
	if err != nil {
		return r.applyFailure(ctx, prepared, "provider_input_unavailable", err.Error(), false)
	}
	job, err := provider.Submit(ctx, request)
	if err != nil {
		if providerErr, ok := providervideo.AsError(err); ok && providerErr.SubmissionUnknown {
			return r.apply(ctx, prepared, domainvideo.ItemStateReconciling, "reconciling", "", providervideo.Artifact{}, nextAt(r.options.Now(), r.pollInterval(prepared.Attempt.No)), "", "", false)
		}
		return r.applyFailure(ctx, prepared, "provider_submit_failed", err.Error(), false)
	}
	return r.applyJob(ctx, prepared, job)
}

func (r *Runner) reconcile(ctx context.Context, item WorkItem) error {
	provider, err := r.resolveProvider(ctx, item)
	if err != nil {
		return r.schedule(ctx, item, item.State, "reconciling", "provider_unavailable", err.Error())
	}
	reconciler, ok := provider.(Reconciler)
	if !ok {
		return r.applyFailure(ctx, item, "reconcile_unsupported", "provider cannot reconcile an uncertain submission", true)
	}
	job, found, err := reconciler.Reconcile(ctx, providerRequest(item))
	if err != nil {
		return r.schedule(ctx, item, item.State, "reconciling", "reconcile_failed", err.Error())
	}
	if !found {
		return r.schedule(ctx, item, item.State, "reconciling", "", "")
	}
	return r.applyJob(ctx, item, job)
}

func (r *Runner) poll(ctx context.Context, item WorkItem) error {
	provider, err := r.resolveProvider(ctx, item)
	if err != nil {
		return r.schedule(ctx, item, item.State, item.Attempt.Status, "provider_unavailable", err.Error())
	}
	status, err := provider.Get(ctx, providervideo.JobRef{ID: item.Attempt.JobID})
	if err != nil {
		return r.schedule(ctx, item, item.State, item.Attempt.Status, "provider_poll_failed", err.Error())
	}
	return r.applyStatus(ctx, item, status)
}

func (r *Runner) cancel(ctx context.Context, item WorkItem) error {
	provider, err := r.resolveProvider(ctx, item)
	if err != nil {
		return r.schedule(ctx, item, item.State, "cancel_requested", "provider_unavailable", err.Error())
	}
	result, err := provider.Cancel(ctx, providervideo.JobRef{ID: item.Attempt.JobID})
	if err != nil {
		return r.schedule(ctx, item, item.State, "cancel_requested", "provider_cancel_failed", err.Error())
	}
	if result.Accepted || result.State == providervideo.StateCancelled {
		return r.apply(ctx, item, domainvideo.ItemStateCancelled, "cancelled", "", providervideo.Artifact{}, nil, "", "", false)
	}
	if result.State == providervideo.StateSucceeded {
		status, getErr := provider.Get(ctx, providervideo.JobRef{ID: item.Attempt.JobID})
		if getErr != nil {
			return r.schedule(ctx, item, item.State, "cancel_requested", "provider_poll_failed", getErr.Error())
		}
		return r.applyStatus(ctx, item, status)
	}
	return r.schedule(ctx, item, item.State, "cancel_requested", "", "")
}

func (r *Runner) applyJob(ctx context.Context, item WorkItem, job providervideo.Job) error {
	if job.State == providervideo.StateFailed || job.State == providervideo.StateCancelled {
		return r.applyFailure(ctx, item, "provider_submit_failed", "provider rejected the video job", false)
	}
	return r.apply(ctx, item, domainvideo.ItemStateProviderQueued, "provider_queued", job.ID, providervideo.Artifact{}, nextAt(r.options.Now(), r.pollInterval(item.Attempt.No)), "", "", false)
}

func (r *Runner) applyStatus(ctx context.Context, item WorkItem, status providervideo.Status) error {
	usage, normalizeErr := r.normalizeUsage(ctx, item, status)
	switch status.State {
	case providervideo.StateQueued:
		if item.State == domainvideo.ItemStateProviderRunning {
			return r.schedule(ctx, item, item.State, "provider_running", "", "")
		}
		return r.applyProviderStatus(ctx, item, domainvideo.ItemStateProviderQueued, "provider_queued", status, providervideo.Artifact{}, nextAt(r.options.Now(), r.pollInterval(item.Attempt.No)), "", "", false, usage, normalizeErr)
	case providervideo.StateRunning:
		return r.applyProviderStatus(ctx, item, domainvideo.ItemStateProviderRunning, "provider_running", status, providervideo.Artifact{}, nextAt(r.options.Now(), r.pollInterval(item.Attempt.No)), "", "", false, usage, normalizeErr)
	case providervideo.StateSucceeded:
		if len(status.Artifacts) == 0 {
			return r.applyFailure(ctx, item, "provider_artifact_missing", "provider succeeded without an artifact", true)
		}
		return r.applyProviderStatus(ctx, item, domainvideo.ItemStateArtifactPending, "artifact_pending", status, status.Artifacts[0], nextAt(r.options.Now(), 0), "", "", false, usage, normalizeErr)
	case providervideo.StateCancelled:
		return r.apply(ctx, item, domainvideo.ItemStateCancelled, "cancelled", status.JobID, providervideo.Artifact{}, nil, "", "", false)
	case providervideo.StateFailed:
		return r.applyFailure(ctx, item, status.ErrorCode, status.ErrorMessage, true)
	default:
		return r.schedule(ctx, item, item.State, item.Attempt.Status, "provider_status_unknown", string(status.State))
	}
}

func (r *Runner) normalizeUsage(ctx context.Context, item WorkItem, status providervideo.Status) (providervideo.Usage, error) {
	provider, err := r.resolveProvider(ctx, item)
	if err != nil {
		return providervideo.Usage{}, err
	}
	return provider.NormalizeUsage(status)
}

func (r *Runner) applyProviderStatus(ctx context.Context, item WorkItem, target domainvideo.ItemState, attemptStatus string, status providervideo.Status, artifact providervideo.Artifact, next *time.Time, code, message string, absorbed bool, usage providervideo.Usage, normalizeErr error) error {
	snapshot := status.Raw
	if snapshot == nil {
		snapshot = map[string]any{"job_id": status.JobID, "state": string(status.State), "error_code": status.ErrorCode, "error_message": status.ErrorMessage}
	}
	req := ApplyStepRequest{
		ItemID: item.ID, Owner: r.options.Owner, ExpectedVersion: item.Version, Target: target,
		ProviderJobID: status.JobID, AttemptStatus: attemptStatus, Artifact: artifact,
		ProviderStatusSnapshot: snapshot, UsageRaw: status.Usage, UsageNormalized: usage,
		PlatformAbsorbed: absorbed, ErrorCode: code, ErrorMessage: message, NextActionAt: next,
	}
	if normalizeErr != nil {
		req.UsageNormalizationError = normalizeErr.Error()
	}
	applied, err := r.store.ApplyStep(ctx, req)
	if r.options.Observer != nil {
		switch {
		case err != nil:
			r.options.Observer.RecordVideoStage(string(target), "failed")
		case applied:
			r.options.Observer.RecordVideoStage(string(target), "success")
		}
	}
	return err
}

func (r *Runner) persistArtifact(ctx context.Context, item WorkItem) error {
	execution, err := r.resolveExecution(ctx, item)
	if err != nil {
		return r.handleArtifactFailure(ctx, item, fmt.Errorf("resolve artifact provider account: %w", err))
	}
	file, size, sum, mimeType, err := r.downloadArtifact(ctx, item.Artifact, execution.ArtifactAllowedHosts)
	if err != nil {
		return r.handleArtifactFailure(ctx, item, err)
	}
	defer func() {
		name := file.Name()
		_ = file.Close()
		_ = os.Remove(name)
	}()
	writer, err := r.storage.DefaultWriter(ctx)
	if err != nil {
		return r.handleArtifactFailure(ctx, item, fmt.Errorf("resolve artifact storage: %w", err))
	}
	streaming, ok := writer.Backend.(storage.StreamingBackend)
	if !ok {
		return r.handleArtifactFailure(ctx, item, errors.New("artifact storage does not support streaming writes"))
	}
	assetID := r.options.AssetID()
	extension := extensionForMIME(mimeType)
	objectKey := filepath.ToSlash(filepath.Join("media", "original", fmt.Sprint(item.UserID), item.ProjectID, assetID+extension))
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return r.handleArtifactFailure(ctx, item, fmt.Errorf("rewind artifact: %w", err))
	}
	if err := streaming.PutReader(ctx, objectKey, mimeType, file, size); err != nil {
		_ = writer.Backend.Delete(context.WithoutCancel(ctx), objectKey)
		return r.handleArtifactFailure(ctx, item, fmt.Errorf("store artifact: %w", err))
	}
	committed, err := r.store.CommitArtifact(ctx, ArtifactCommitRequest{
		ItemID: item.ID, Owner: r.options.Owner, ExpectedVersion: item.Version,
		AssetID: assetID, UserID: item.UserID, ProjectID: item.ProjectID, Status: "ready_original",
		StorageConfigID: writer.ConfigID, StorageDriver: writer.Driver, Bucket: writer.Bucket,
		ObjectKey: objectKey, MIMEType: mimeType, SizeBytes: size, SHA256: sum,
	})
	if err != nil || !committed {
		_ = writer.Backend.Delete(context.WithoutCancel(ctx), objectKey)
		if err != nil && r.options.Observer != nil {
			r.options.Observer.RecordArtifactTransfer("video", "failed", 0)
		}
	} else if r.options.Observer != nil {
		r.options.Observer.RecordArtifactTransfer("video", "success", size)
	}
	return err
}

func (r *Runner) downloadArtifact(ctx context.Context, artifact providervideo.Artifact, accountHosts []string) (*os.File, int64, string, string, error) {
	transferCtx, cancel := context.WithTimeout(ctx, r.options.ArtifactTransferTimeout)
	defer cancel()
	parsed, err := url.Parse(strings.TrimSpace(artifact.URL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, 0, "", "", errors.New("artifact URL must use HTTPS and an allowlisted host")
	}
	client := *r.httpClient
	transport, err := r.artifactTransport(accountHosts)
	if err != nil {
		return nil, 0, "", "", err
	}
	client.Transport = transport
	priorRedirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errors.New("artifact redirect host is not allowlisted")
		}
		if priorRedirect != nil {
			return priorRedirect(req, via)
		}
		if len(via) >= 10 {
			return errors.New("too many artifact redirects")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(transferCtx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, 0, "", "", err
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, 0, "", "", fmt.Errorf("download artifact: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, 0, "", "", fmt.Errorf("download artifact: HTTP %d", response.StatusCode)
	}
	if response.ContentLength > r.options.ArtifactMaxBytes {
		return nil, 0, "", "", errors.New("artifact exceeds maximum size")
	}
	file, err := os.CreateTemp("", "video-artifact-*")
	if err != nil {
		return nil, 0, "", "", err
	}
	cleanup := func(downloadErr error) (*os.File, int64, string, string, error) {
		name := file.Name()
		_ = file.Close()
		_ = os.Remove(name)
		return nil, 0, "", "", downloadErr
	}
	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(response.Body, r.options.ArtifactMaxBytes+1))
	if err != nil {
		return cleanup(fmt.Errorf("read artifact: %w", err))
	}
	if size > r.options.ArtifactMaxBytes {
		return cleanup(errors.New("artifact exceeds maximum size"))
	}
	if response.ContentLength >= 0 && response.ContentLength != size {
		return cleanup(errors.New("artifact Content-Length does not match actual size"))
	}
	if artifact.SizeBytes > 0 && artifact.SizeBytes != size {
		return cleanup(errors.New("provider artifact size does not match actual size"))
	}
	sum := hex.EncodeToString(hasher.Sum(nil))
	if artifact.SHA256 != "" && !strings.EqualFold(strings.TrimSpace(artifact.SHA256), sum) {
		return cleanup(errors.New("provider artifact checksum does not match"))
	}
	mimeType := strings.TrimSpace(artifact.MIMEType)
	if mimeType == "" {
		mimeType = strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return file, size, sum, mimeType, nil
}

func (r *Runner) handleArtifactFailure(ctx context.Context, item WorkItem, cause error) error {
	maxAttempts := item.MaxArtifactAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	if item.ArtifactAttempts+1 < maxAttempts {
		_, err := r.store.ApplyStep(ctx, ApplyStepRequest{
			ItemID: item.ID, Owner: r.options.Owner, ExpectedVersion: item.Version,
			Target: domainvideo.ItemStateRecoveryRequired, AttemptStatus: "recovery_required",
			ErrorCode: "artifact_persist_failed", ErrorMessage: cause.Error(),
			NextActionAt: nextAt(r.options.Now(), r.pollInterval(item.ArtifactAttempts)), IncrementArtifactAttempts: true,
		})
		return err
	}
	_, err := r.store.ApplyStep(ctx, ApplyStepRequest{
		ItemID: item.ID, Owner: r.options.Owner, ExpectedVersion: item.Version,
		Target: domainvideo.ItemStateFailed, AttemptStatus: "failed", PlatformAbsorbed: true,
		ErrorCode: "artifact_persist_failed", ErrorMessage: cause.Error(),
		IncrementArtifactAttempts: true, ArtifactExhausted: true,
	})
	return err
}

func (r *Runner) settle(ctx context.Context, item WorkItem) (bool, error) {
	snapshot, err := r.store.LoadSettlement(ctx, item.TaskID)
	if err != nil {
		return false, err
	}
	states := make([]domainvideo.ItemState, 0, len(snapshot.Items))
	actual := decimal.Zero
	successCount := 0
	for _, settlementItem := range snapshot.Items {
		states = append(states, settlementItem.State)
		if !isTerminal(settlementItem.State) {
			return false, nil
		}
		if settlementItem.UsagePending {
			return false, nil
		}
		if settlementItem.State == domainvideo.ItemStateSucceeded {
			points, parseErr := decimal.NewFromString(settlementItem.PricePoints)
			if parseErr != nil {
				return false, fmt.Errorf("parse settlement points: %w", parseErr)
			}
			actual = actual.Add(points)
			successCount++
		}
	}
	status := domainvideo.AggregateTaskStatus(states)
	finalized, err := r.store.FinalizeTask(ctx, FinalizeRequest{
		TaskID: snapshot.TaskID, Status: status, SuccessOutputCount: successCount,
		ActualPoints: actual.StringFixed(5), ReservedPoints: snapshot.ReservedPoints,
	})
	if r.options.Observer != nil {
		switch {
		case err != nil:
			r.options.Observer.RecordSettlement("video", "failed")
		case finalized:
			r.options.Observer.RecordSettlement("video", "success")
		}
	}
	return finalized && err == nil, err
}

func (r *Runner) resolveProvider(ctx context.Context, item WorkItem) (providervideo.Provider, error) {
	execution, err := r.resolveExecution(ctx, item)
	if err != nil {
		return nil, err
	}
	if execution.Provider == nil {
		return nil, errors.New("video provider resolver returned no provider")
	}
	return execution.Provider, nil
}

func (r *Runner) resolveExecution(ctx context.Context, item WorkItem) (ResolvedExecution, error) {
	code := item.Attempt.ProviderCode
	if code == "" {
		code = item.ProviderCode
	}
	return r.providers.Resolve(ctx, ProviderRef{
		RouteCandidateID: item.Attempt.RouteCandidateID, AccountModelID: item.Attempt.AccountModelID, ModelAccountID: item.Attempt.ModelAccountID,
		ProviderCode: code, ModelCode: item.Attempt.ModelCode,
	})
}

func (r *Runner) applyFailure(ctx context.Context, item WorkItem, code, message string, absorbed bool) error {
	if code == "" {
		code = "provider_failed"
	}
	return r.apply(ctx, item, domainvideo.ItemStateFailed, "failed", "", providervideo.Artifact{}, nil, code, message, absorbed)
}

func (r *Runner) schedule(ctx context.Context, item WorkItem, target domainvideo.ItemState, status, code, message string) error {
	return r.apply(ctx, item, target, status, "", providervideo.Artifact{}, nextAt(r.options.Now(), r.pollInterval(item.Attempt.No)), code, message, false)
}

func (r *Runner) apply(ctx context.Context, item WorkItem, target domainvideo.ItemState, attemptStatus, jobID string, artifact providervideo.Artifact, next *time.Time, code, message string, absorbed bool) error {
	applied, err := r.store.ApplyStep(ctx, ApplyStepRequest{
		ItemID: item.ID, Owner: r.options.Owner, ExpectedVersion: item.Version, Target: target,
		ProviderJobID: jobID, AttemptStatus: attemptStatus, Artifact: artifact,
		PlatformAbsorbed: absorbed, ErrorCode: code, ErrorMessage: message, NextActionAt: next,
	})
	if r.options.Observer != nil {
		switch {
		case err != nil:
			r.options.Observer.RecordVideoStage(string(target), "failed")
		case applied:
			r.options.Observer.RecordVideoStage(string(target), "success")
		}
	}
	return err
}

func (r *Runner) pollInterval(attempt int) time.Duration {
	index := attempt
	if index < 0 {
		index = 0
	}
	if index >= len(r.options.PollIntervals) {
		index = len(r.options.PollIntervals) - 1
	}
	return r.options.PollIntervals[index]
}

func (r *Runner) allowedArtifactHost(ctx context.Context, host string, accountHosts []string) bool {
	_, err := r.resolveArtifactHost(ctx, host, accountHosts)
	return err == nil
}

func (r *Runner) resolveArtifactHost(ctx context.Context, host string, accountHosts []string) (net.IP, error) {
	hostname := strings.TrimSpace(host)
	if parsedHost, _, err := net.SplitHostPort(hostname); err == nil {
		hostname = parsedHost
	}
	allowedHosts := append(append([]string(nil), accountHosts...), r.options.ArtifactAllowedHosts...)
	matched := false
	for _, allowed := range allowedHosts {
		if strings.EqualFold(strings.TrimSpace(allowed), host) {
			matched = true
			break
		}
	}
	if !matched {
		return nil, errors.New("artifact host is not allowlisted")
	}
	addresses, err := r.options.ResolveHostIPs(ctx, hostname)
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("artifact host did not resolve")
	}
	for _, address := range addresses {
		if !isAllowedArtifactIP(address, r.options.AllowLoopbackArtifactHosts) {
			return nil, errors.New("artifact host resolved to a non-public address")
		}
	}
	return append(net.IP(nil), addresses[0]...), nil
}

func (r *Runner) artifactTransport(accountHosts []string) (http.RoundTripper, error) {
	base := r.httpClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	transport, ok := base.(*http.Transport)
	if !ok {
		return nil, errors.New("artifact HTTP transport cannot enforce address pinning")
	}
	return artifactRoundTripper{runner: r, base: transport, accountHosts: append([]string(nil), accountHosts...)}, nil
}

type artifactRoundTripper struct {
	runner       *Runner
	base         *http.Transport
	accountHosts []string
}

func (transport artifactRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil || request.URL.Scheme != "https" {
		return nil, errors.New("artifact request must use HTTPS")
	}
	address, err := transport.runner.resolveArtifactHost(request.Context(), request.URL.Host, transport.accountHosts)
	if err != nil {
		return nil, err
	}
	port := request.URL.Port()
	if port == "" {
		port = "443"
	}
	pinnedAddress := net.JoinHostPort(address.String(), port)
	clone := transport.base.Clone()
	clone.Proxy = nil
	clone.DialTLS = nil
	clone.DialTLSContext = nil
	clone.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, pinnedAddress)
	}
	return clone.RoundTrip(request)
}

var forbiddenArtifactPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"), netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("100::/64"), netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"), netip.MustParsePrefix("ff00::/8"),
}

func isAllowedArtifactIP(ip net.IP, allowLoopback bool) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return allowLoopback
	}
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() {
		return false
	}
	for _, prefix := range forbiddenArtifactPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func providerRequest(item WorkItem) providervideo.Request {
	request := item.Request
	request.TaskID = item.TaskID
	request.ItemID = item.ID
	request.AttemptID = item.Attempt.ID
	request.IdempotencyKey = item.Attempt.IdempotencyKey
	return request
}

func (r *Runner) signInputs(ctx context.Context, request providervideo.Request) (providervideo.Request, error) {
	for index := range request.Inputs {
		input := &request.Inputs[index]
		ref, err := r.storage.BackendFor(ctx, input.StorageConfigID, input.StorageDriver)
		if err != nil {
			return providervideo.Request{}, fmt.Errorf("resolve input asset storage: %w", err)
		}
		access, supported, err := storage.ProjectTemporaryMediaAccess(ctx, ref.Backend, input.ObjectKey, input.MIMEType, "", storage.TemporaryMediaPurposePreview)
		if err != nil {
			return providervideo.Request{}, fmt.Errorf("sign input asset: %w", err)
		}
		if !supported || !strings.HasPrefix(access.URL, "http") {
			return providervideo.Request{}, errors.New("input asset storage cannot issue an externally reachable temporary URL")
		}
		input.URL = access.URL
	}
	return request, nil
}

func nextAt(now time.Time, delay time.Duration) *time.Time {
	value := now.UTC().Add(delay)
	return &value
}

func isTerminal(state domainvideo.ItemState) bool {
	return state == domainvideo.ItemStateSucceeded || state == domainvideo.ItemStateFailed || state == domainvideo.ItemStateCancelled
}

func extensionForMIME(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "video/webm":
		return ".webm"
	default:
		return ".bin"
	}
}
