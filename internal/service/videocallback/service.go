package videocallback

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	videoprovider "github.com/fatballfish/pic-gallery/internal/provider/video"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type ResolvedProvider struct {
	ModelAccountID int64
	Provider       videoprovider.Provider
}

type EventRecord struct {
	ProviderCode    string
	ModelAccountID  int64
	ProviderEventID string
	ProviderJobID   string
	PayloadSnapshot map[string]any
	ReceivedAt      time.Time
}

type ProviderResolver interface {
	ResolveProvider(context.Context, string, uuid.UUID) (ResolvedProvider, error)
}

type EventStore interface {
	RecordEvent(context.Context, EventRecord) (duplicate bool, err error)
}

type Result struct {
	Challenge string `json:"challenge,omitempty"`
	Accepted  bool   `json:"accepted,omitempty"`
	Duplicate bool   `json:"duplicate,omitempty"`
}

type Service struct {
	resolver ProviderResolver
	store    EventStore
	now      func() time.Time
}

func NewService(resolver ProviderResolver, store EventStore) *Service {
	return &Service{resolver: resolver, store: store, now: time.Now}
}

func (s *Service) Receive(ctx context.Context, providerCode string, accountPublicID uuid.UUID, headers http.Header, body []byte) (Result, error) {
	if s == nil || s.resolver == nil || s.store == nil {
		return Result{}, errs.Internal("video callback service is unavailable")
	}
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	if providerCode == "" || accountPublicID == uuid.Nil {
		return Result{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "video callback endpoint not found")
	}
	resolved, err := s.resolver.ResolveProvider(ctx, providerCode, accountPublicID)
	if err != nil || resolved.Provider == nil {
		var appErr *errs.Error
		if errors.As(err, &appErr) {
			return Result{}, appErr
		}
		return Result{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "video callback endpoint not found")
	}
	event, err := resolved.Provider.VerifyCallback(ctx, headers, body)
	if err != nil {
		return Result{}, errs.BadRequest("invalid video provider callback")
	}
	if event.Challenge != "" {
		return Result{Challenge: event.Challenge}, nil
	}
	normalizedUsage, normalizeErr := resolved.Provider.NormalizeUsage(event.Status)
	if normalizeErr != nil {
		return Result{}, errs.BadRequest("invalid video provider callback usage")
	}
	jobID := strings.TrimSpace(event.JobID)
	if jobID == "" {
		jobID = strings.TrimSpace(event.Status.JobID)
	}
	if jobID == "" {
		return Result{}, errs.BadRequest("video provider callback job id is required")
	}
	eventID := strings.TrimSpace(event.EventID)
	if eventID == "" {
		digest := sha256.Sum256(append([]byte(jobID+"\x00"), body...))
		eventID = "sha256:" + hex.EncodeToString(digest[:])
	}
	duplicate, err := s.store.RecordEvent(ctx, EventRecord{
		ProviderCode: providerCode, ModelAccountID: resolved.ModelAccountID, ProviderEventID: eventID, ProviderJobID: jobID,
		PayloadSnapshot: callbackSnapshot(event.Status, normalizedUsage), ReceivedAt: s.now().UTC(),
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Accepted: true, Duplicate: duplicate}, nil
}

func callbackSnapshot(status videoprovider.Status, usage videoprovider.Usage) map[string]any {
	snapshot := map[string]any{
		"job_id":        status.JobID,
		"state":         string(status.State),
		"error_code":    status.ErrorCode,
		"error_message": status.ErrorMessage,
		"artifacts":     status.Artifacts,
		"usage":         status.Usage,
		"usage_normalized": map[string]any{
			"output_seconds": usage.OutputSeconds, "input_video_seconds": usage.InputVideoSeconds,
			"reference_image_count": usage.ReferenceImageCount, "provider_tokens": usage.ProviderTokens,
		},
		"provider_status": status.Raw,
	}
	return snapshot
}
