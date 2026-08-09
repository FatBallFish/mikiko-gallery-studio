package imagetask

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const (
	artifactRecoveryPending    = "pending"
	artifactRecoveryPersisting = "persisting"
)

const maxGeneratedArtifactBytes = int64(64 << 20)

type artifactPersistenceFailure struct {
	diagnostic domainimagetask.ArtifactDiagnostic
	cause      error
}

func (e *artifactPersistenceFailure) Error() string {
	if e == nil {
		return "artifact persistence failed"
	}
	return e.diagnostic.Code
}

func (e *artifactPersistenceFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (s *Service) checkpointProviderSuccess(
	ctx context.Context,
	task *domainimagetask.Task,
	owner string,
	candidate modelhub.ProviderCandidate,
	response provider.ImageResponse,
	outboundSize string,
	attempts []domainimagetask.Attempt,
	startedAt time.Time,
	finishedAt time.Time,
) error {
	if task == nil {
		return fmt.Errorf("checkpoint provider success: task is nil")
	}
	payload, err := s.encryptArtifactResults(response.Data)
	if err != nil {
		return fmt.Errorf("encrypt artifact recovery payload: %w", err)
	}
	decorated := s.decorateTaskProvider(*task, candidate)
	decorated.Status = domainimagetask.StatusRunning
	setTaskProgress(&decorated, domainimagetask.ProgressStagePersisting, "图片生成完成，正在保存结果")
	decorated.ProviderRequestID = strings.TrimSpace(response.ProviderRequestID)
	completedAt := finishedAt.UTC()
	decorated.UpstreamSucceededAt = &completedAt
	if len(attempts) == 0 {
		attempt := buildProviderAttempt(candidate, domainimagetask.StatusSucceeded, nil, startedAt, finishedAt)
		attempt.SourceSizeMode = task.SizeMode
		attempt.OutboundSize = outboundSize
		attempt.ProviderRequestID = strings.TrimSpace(response.ProviderRequestID)
		attempt.RequestedImageCount = normalizedCount(task.OutputImageCount)
		attempt.ReturnedImageCount = len(response.Data)
		if len(response.Data) > 0 {
			attempt.ReturnedWidth, attempt.ReturnedHeight = response.Data[0].Width, response.Data[0].Height
		}
		attempt.SizeDiagnostic = classifyImageSize(outboundSize, attempt.ReturnedWidth, attempt.ReturnedHeight)
		attempts = []domainimagetask.Attempt{attempt}
	}
	decorated.Attempts = append(decorated.Attempts, attempts...)
	decorated.ProviderCost = calculateProviderCost(candidate, len(response.Data))
	decorated.ArtifactRecovery = domainimagetask.ArtifactRecovery{
		Status:           artifactRecoveryPersisting,
		EncryptedPayload: payload,
	}
	if err := s.saveOwnedTask(ctx, decorated, owner); err != nil {
		// A paid upstream result must survive an expired lease. SaveTerminalState
		// preserves the current owner's lease columns while durably recording the
		// recovery envelope, allowing the current or reclaiming worker to resume it.
		if snapshotErr := s.saveTerminalState(ctx, decorated, owner); snapshotErr != nil {
			return fmt.Errorf("persist provider success checkpoint: %w", err)
		}
	}
	*task = decorated
	if err := s.pinArtifactWriter(ctx, task, owner); err != nil {
		return fmt.Errorf("resolve artifact storage writer: %w", err)
	}
	return nil
}

func (s *Service) executeArtifactRecovery(ctx context.Context, task domainimagetask.Task, owner string) (domainimagetask.ExecuteResult, error) {
	task.Status = domainimagetask.StatusRunning
	setTaskProgress(&task, domainimagetask.ProgressStagePersisting, "正在恢复并保存生成结果")
	results, err := s.decryptArtifactResults(task.ArtifactRecovery.EncryptedPayload)
	if err != nil {
		failure := newArtifactFailure(s, errs.CodeArtifactRecoveryPayloadInvalid, "decode", false, err)
		return s.handleArtifactPersistenceFailure(ctx, task, owner, failure)
	}
	task.ArtifactRecovery.Status = artifactRecoveryPersisting
	task.ArtifactRecovery.NextRetryAt = nil
	if err := s.pinArtifactWriter(ctx, &task, owner); err != nil {
		var failure *artifactPersistenceFailure
		if errors.As(err, &failure) {
			return s.handleArtifactPersistenceFailure(ctx, task, owner, failure)
		}
		return domainimagetask.ExecuteResult{}, err
	}
	persisted, err := s.persistImageResults(ctx, task, results)
	if err != nil {
		return s.handleArtifactPersistenceFailure(ctx, task, owner, err)
	}
	reconcileAttemptDimensions(&task, persisted)
	task.Results = append([]provider.ImageResult(nil), persisted...)
	if err := s.applyActualPoints(&task, len(persisted)); err != nil {
		return s.failOwnedTask(ctx, task, owner, err)
	}
	task.GrossMargin = calculateGrossMargin(task.ActualPoints, task.ProviderCost)
	task.ArtifactRecovery = completedArtifactRecovery(task.ArtifactRecovery)
	setTaskProgress(&task, domainimagetask.ProgressStageSettling, "结果已恢复，正在结算积分")
	if err := s.saveOwnedTask(ctx, task, owner); err != nil {
		return domainimagetask.ExecuteResult{}, err
	}
	if err := s.settleTaskBilling(ctx, task, "task succeeded after artifact recovery"); err != nil {
		return domainimagetask.ExecuteResult{}, err
	}
	task.Status = domainimagetask.StatusSucceeded
	if len(persisted) < normalizedCount(task.OutputImageCount) {
		task.Status = domainimagetask.StatusPartialFailed
	}
	setCompletedTaskProgress(&task)
	task.LeaseOwner = ""
	task.LeaseExpiresAt = nil
	if err := s.saveOwnedTask(ctx, task, owner); err != nil {
		return domainimagetask.ExecuteResult{}, err
	}
	return domainimagetask.ExecuteResult{Task: task, Response: provider.ImageResponse{ProviderRequestID: task.ProviderRequestID, Data: persisted}}, nil
}

func (s *Service) handleProviderSuccessCheckpointFailure(ctx context.Context, task domainimagetask.Task, owner string, err error) (domainimagetask.ExecuteResult, error) {
	var failure *artifactPersistenceFailure
	if errors.As(err, &failure) {
		return s.handleArtifactPersistenceFailure(ctx, task, owner, failure)
	}
	return domainimagetask.ExecuteResult{}, err
}

func (s *Service) pinArtifactWriter(ctx context.Context, task *domainimagetask.Task, owner string) error {
	if task == nil {
		return fmt.Errorf("pin artifact storage writer: task is nil")
	}
	if strings.TrimSpace(task.ArtifactRecovery.StorageDriver) != "" && len(task.ArtifactRecovery.ObjectKeys) > 0 {
		return nil
	}
	var (
		writer storage.BackendRef
		err    error
	)
	if strings.TrimSpace(task.ArtifactRecovery.StorageConfigID) != "" {
		writer, err = s.router.BackendFor(ctx, task.ArtifactRecovery.StorageConfigID, task.ArtifactRecovery.StorageDriver)
	} else {
		writer, err = s.router.DefaultWriter(ctx)
	}
	if err != nil {
		return newArtifactFailure(s, errs.CodeArtifactStorageUnavailable, "resolve_storage", true, err)
	}
	results, err := s.decryptArtifactResults(task.ArtifactRecovery.EncryptedPayload)
	if err != nil {
		return newArtifactFailure(s, errs.CodeArtifactRecoveryPayloadInvalid, "decode", false, err)
	}
	task.ArtifactRecovery.StorageConfigID = writer.ConfigID
	task.ArtifactRecovery.StorageDriver = writer.Driver
	task.ArtifactRecovery.StorageBucket = writer.Bucket
	if strings.TrimSpace(writer.ConfigID) != "" {
		task.ArtifactRecovery.StorageBucket = ""
	}
	task.ArtifactRecovery.ObjectKeys = artifactRecoveryObjectKeys(*task, results)
	task.ArtifactRecovery.StorageVersion = writer.Version
	if err := s.saveOwnedTask(ctx, *task, owner); err != nil {
		if snapshotErr := s.saveTerminalState(ctx, *task, owner); snapshotErr != nil {
			return fmt.Errorf("persist artifact storage writer checkpoint: %w", err)
		}
	}
	return nil
}

func artifactRecoveryObjectKeys(task domainimagetask.Task, results []provider.ImageResult) []string {
	keys := make([]string, 0, len(results))
	for index, result := range results {
		resultID := strings.TrimSpace(result.ID)
		if resultID == "" || strings.TrimSpace(result.B64JSON) != "" || isDataURL(result.URL) {
			resultID = deterministicImageResultID(task.ID, index)
		}
		format := defaultString(result.Format, task.OutputFormat)
		mimeType := defaultString(result.MimeType, "image/"+strings.ToLower(strings.TrimSpace(task.OutputFormat)))
		keys = append(keys, generatedImageObjectKey(task.UserID, task.ID, index, resultID, imageExtension(format, mimeType)))
	}
	return keys
}

func recoveryObjectKey(task domainimagetask.Task, index int, fallback string) string {
	if index >= 0 && index < len(task.ArtifactRecovery.ObjectKeys) {
		if key := strings.TrimSpace(task.ArtifactRecovery.ObjectKeys[index]); key != "" {
			return key
		}
	}
	return fallback
}

func (s *Service) handleArtifactPersistenceFailure(ctx context.Context, task domainimagetask.Task, owner string, failure error) (domainimagetask.ExecuteResult, error) {
	diagnostic, retryable := artifactDiagnostic(failure)
	task.ArtifactRecovery.AttemptCount++
	diagnostic.Attempt = task.ArtifactRecovery.AttemptCount
	task.ArtifactRecovery.LastDiagnostic = diagnostic
	task.ArtifactRecovery.Diagnostics = append(task.ArtifactRecovery.Diagnostics, diagnostic)
	if !retryable || task.ArtifactRecovery.AttemptCount >= 4 {
		task.ArtifactRecovery.Status = "failed"
		task.ArtifactRecovery.NextRetryAt = nil
		task.ArtifactRecovery.EncryptedPayload = ""
		terminal := errs.New(500, errs.CodeImageStorageFailed, diagnostic.Code)
		return s.failOwnedTask(ctx, task, owner, terminal)
	}
	nextRetryAt := s.nowUTC().Add(artifactRetryDelay(task.ArtifactRecovery.AttemptCount))
	task.ArtifactRecovery.Status = artifactRecoveryPending
	task.ArtifactRecovery.NextRetryAt = &nextRetryAt
	task.Status = domainimagetask.StatusQueued
	task.LeaseOwner = ""
	task.LeaseExpiresAt = nil
	task.ErrorCode = ""
	task.ErrorMessage = ""
	if err := s.saveOwnedTask(ctx, task, owner); err != nil {
		return domainimagetask.ExecuteResult{}, err
	}
	return domainimagetask.ExecuteResult{Task: task}, nil
}

func completedArtifactRecovery(recovery domainimagetask.ArtifactRecovery) domainimagetask.ArtifactRecovery {
	recovery.Status = "completed"
	recovery.EncryptedPayload = ""
	recovery.NextRetryAt = nil
	return recovery
}

func artifactRetryDelay(attemptCount int) time.Duration {
	switch attemptCount {
	case 1:
		return time.Second
	case 2:
		return 3 * time.Second
	default:
		return 10 * time.Second
	}
}

func artifactDiagnostic(err error) (domainimagetask.ArtifactDiagnostic, bool) {
	var failure *artifactPersistenceFailure
	if errors.As(err, &failure) {
		return failure.diagnostic, failure.diagnostic.Retryable
	}
	now := time.Now().UTC()
	return domainimagetask.ArtifactDiagnostic{Code: errs.CodeArtifactStorageWriteFailed, Stage: "store", Retryable: true, Cause: sanitizeArtifactCause(err), StartedAt: now, FinishedAt: now}, true
}

func newArtifactFailure(s *Service, code, stage string, retryable bool, cause error) *artifactPersistenceFailure {
	now := s.nowUTC()
	return &artifactPersistenceFailure{diagnostic: domainimagetask.ArtifactDiagnostic{Code: code, Stage: stage, Retryable: retryable, Cause: sanitizeArtifactCause(cause), StartedAt: now, FinishedAt: now}, cause: cause}
}

func sanitizeArtifactCause(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if parsed, parseErr := url.Parse(message); parseErr == nil && parsed.Host != "" {
		parsed.RawQuery, parsed.Fragment = "", ""
		return parsed.String()
	}
	if index := strings.Index(message, "?"); index >= 0 {
		message = message[:index]
	}
	return message
}

func classifyFetchError(s *Service, err error) *artifactPersistenceFailure {
	code := errs.CodeArtifactFetchConnectionFailed
	if errors.Is(err, context.DeadlineExceeded) {
		code = errs.CodeArtifactFetchTimeout
	} else if networkErr, ok := err.(net.Error); ok && networkErr.Timeout() {
		code = errs.CodeArtifactFetchTimeout
	}
	return newArtifactFailure(s, code, "fetch", true, err)
}

func decorateArtifactHTTPDiagnostic(s *Service, diagnostic *domainimagetask.ArtifactDiagnostic, req *http.Request, resp *http.Response, bytesRead int64, startedAt time.Time) {
	if diagnostic == nil {
		return
	}
	if req != nil && req.URL != nil {
		diagnostic.URLHost = req.URL.Host
		diagnostic.URLPath = req.URL.EscapedPath()
	}
	if resp != nil {
		diagnostic.HTTPStatus = resp.StatusCode
		diagnostic.ContentType = resp.Header.Get("Content-Type")
		diagnostic.ContentLength = resp.ContentLength
	}
	finishedAt := s.nowUTC()
	diagnostic.BytesRead = bytesRead
	diagnostic.StartedAt = startedAt
	diagnostic.FinishedAt = finishedAt
	diagnostic.DurationMS = finishedAt.Sub(startedAt).Milliseconds()
}

func verifyStoredArtifact(ctx context.Context, backend storage.Backend, key string, expected []byte) error {
	stored, err := backend.Get(ctx, key)
	if err != nil {
		return err
	}
	if !bytes.Equal(stored, expected) {
		return fmt.Errorf("stored artifact content mismatch")
	}
	return nil
}

func (s *Service) encryptArtifactResults(results []provider.ImageResult) (string, error) {
	if s == nil || s.recoveryCodec == nil {
		return "", fmt.Errorf("artifact recovery codec is unavailable")
	}
	envelope, err := s.recoveryCodec.EncryptJSON(map[string]any{"results": results})
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (s *Service) decryptArtifactResults(payload string) ([]provider.ImageResult, error) {
	var envelope map[string]any
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return nil, err
	}
	decoded, err := s.recoveryCodec.DecryptJSON(envelope)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(decoded["results"])
	if err != nil {
		return nil, err
	}
	var results []provider.ImageResult
	if err := json.Unmarshal(raw, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *Service) artifactWriter(ctx context.Context, task domainimagetask.Task) (storage.BackendRef, error) {
	if strings.TrimSpace(task.ArtifactRecovery.StorageConfigID) != "" {
		return s.router.BackendFor(ctx, task.ArtifactRecovery.StorageConfigID, task.ArtifactRecovery.StorageDriver)
	}
	return s.router.DefaultWriter(ctx)
}
