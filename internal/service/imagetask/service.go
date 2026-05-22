package imagetask

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/provider"
	openaiprovider "github.com/fatballfish/pic-gallery/internal/provider/openai"
	openrouterprovider "github.com/fatballfish/pic-gallery/internal/provider/openrouter"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type Service struct {
	cfg       config.Config
	resolver  *modelhub.Resolver
	providers map[string]provider.ImageProvider
	store     Store
	assets    AssetLoader
	billing   BillingManager
	apiKeys   APIKeyUsageManager
}

type executionOptions struct {
	prompt             string
	size               string
	responseFormat     string
	user               string
	referenceImages    []provider.ImageInput
	mask               *provider.ImageInput
	preferredProviders []string
}

type AssetLoader interface {
	LoadInput(userID int64, assetID string) (provider.ImageInput, error)
}

type BillingManager interface {
	Estimate(req domainbilling.EstimateRequest) (domainbilling.EstimateResult, error)
	ActualPoints(snapshot domainbilling.PricingSnapshot, successOutputImageCount int) (string, error)
	ReserveTask(ctx context.Context, req domainbilling.ReserveRequest) (domainbilling.BalanceSummary, error)
	FinalizeTask(ctx context.Context, req domainbilling.FinalizeRequest) (domainbilling.BalanceSummary, error)
}

type APIKeyUsageManager interface {
	CheckTaskAllowed(ctx context.Context, apiKeyID, userID int64, estimatedPoints string, now time.Time) (domainbilling.APIKeyQuota, error)
}

func NewService(cfg config.Config) *Service {
	return NewServiceWithProvidersStoreAssetsAndBilling(cfg, defaultProviders(cfg), NewMemoryStore(), nil, nil)
}

func NewServiceWithProviders(cfg config.Config, providers map[string]provider.ImageProvider) *Service {
	return NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, NewMemoryStore(), nil, nil)
}

func NewServiceWithStore(cfg config.Config, store Store) *Service {
	return NewServiceWithProvidersStoreAssetsAndBilling(cfg, defaultProviders(cfg), store, nil, nil)
}

func NewServiceWithStoreAndAssets(cfg config.Config, store Store, assets AssetLoader) *Service {
	return NewServiceWithProvidersStoreAssetsAndBilling(cfg, defaultProviders(cfg), store, assets, nil)
}

func NewServiceWithProvidersAndStore(cfg config.Config, providers map[string]provider.ImageProvider, store Store) *Service {
	return NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, store, nil, nil)
}

func NewServiceWithProvidersStoreAndAssets(cfg config.Config, providers map[string]provider.ImageProvider, store Store, assets AssetLoader) *Service {
	return NewServiceWithProvidersStoreAssetsAndBilling(cfg, providers, store, assets, nil)
}

func NewServiceWithStoreAssetsAndBilling(cfg config.Config, store Store, assets AssetLoader, billing BillingManager) *Service {
	return NewServiceWithProvidersStoreAssetsAndBilling(cfg, defaultProviders(cfg), store, assets, billing)
}

func NewServiceWithProvidersStoreAssetsAndBilling(cfg config.Config, providers map[string]provider.ImageProvider, store Store, assets AssetLoader, billing BillingManager) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{
		cfg:       cfg,
		resolver:  modelhub.NewResolver(cfg),
		providers: providers,
		store:     store,
		assets:    assets,
		billing:   billing,
	}
}

func (s *Service) BillingManager() BillingManager {
	if s == nil {
		return nil
	}
	return s.billing
}

func (s *Service) SetAPIKeyUsageManager(apiKeys APIKeyUsageManager) {
	s.apiKeys = apiKeys
}

func (s *Service) SetModelRoutingSource(source modelhub.ModelRoutingSource) {
	s.resolver.SetModelRoutingSource(source)
}

func (s *Service) CreateTask(ctx context.Context, req domainimagetask.CreateRequest) (domainimagetask.Task, error) {
	if strings.TrimSpace(req.TaskID) != "" {
		existing, err := s.store.GetByID(ctx, req.UserID, req.TaskID)
		switch {
		case err == nil:
			return cloneTask(existing), nil
		case errors.Is(err, repoerr.ErrNotFound):
		default:
			return domainimagetask.Task{}, errs.Internal("failed to load existing image task")
		}
	}

	resolved, err := s.resolveTask(ctx, req.AbstractModel, req.TaskType, req.RequestedQuality, req.RequestedSize, req.OutputImageCount, req.ReferenceImageCount, req.MaskPresent)
	if err != nil {
		return domainimagetask.Task{}, err
	}
	if strings.TrimSpace(req.TaskID) == "" {
		req.TaskID = uuid.NewString()
	}

	task := buildTask(req, resolved, domainimagetask.StatusQueued)
	if err := s.applyTaskEstimate(ctx, &task, req); err != nil {
		return domainimagetask.Task{}, err
	}
	if err := s.store.Save(ctx, task); err != nil {
		if rollbackErr := s.rollbackTaskReserve(ctx, task); rollbackErr != nil {
			return domainimagetask.Task{}, errs.Internal("failed to persist image task and rollback reserved points")
		}
		return domainimagetask.Task{}, err
	}
	return cloneTask(task), nil
}

func (s *Service) AcquireNextTask(ctx context.Context, owner string, leaseTTL time.Duration) (domainimagetask.Task, bool, error) {
	task, err := s.store.AcquireNextQueuedTask(ctx, owner, time.Now().UTC(), leaseTTL)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return domainimagetask.Task{}, false, nil
		}
		return domainimagetask.Task{}, false, errs.Internal("failed to acquire queued image task")
	}
	return cloneTask(task), true, nil
}

func (s *Service) HeartbeatTask(ctx context.Context, taskID, owner string, leaseTTL time.Duration) (domainimagetask.Task, error) {
	task, err := s.store.RenewTaskLease(ctx, taskID, owner, time.Now().UTC(), leaseTTL)
	if err != nil {
		switch {
		case errors.Is(err, repoerr.ErrNotFound):
			return domainimagetask.Task{}, errs.New(404, errs.CodeNotFound, "image task not found")
		case errors.Is(err, repoerr.ErrConflict):
			return domainimagetask.Task{}, errs.New(409, errs.CodeConflict, "image task lease conflict")
		default:
			return domainimagetask.Task{}, errs.Internal("failed to renew image task lease")
		}
	}
	return cloneTask(task), nil
}

func (s *Service) Execute(ctx context.Context, req domainimagetask.ExecuteRequest) (domainimagetask.ExecuteResult, error) {
	resolved, err := s.resolveTask(ctx, req.AbstractModel, req.TaskType, req.RequestedQuality, req.RequestedSize, req.OutputImageCount, len(req.ReferenceImages), req.Mask != nil)
	if err != nil {
		return domainimagetask.ExecuteResult{}, err
	}

	task := buildTask(domainimagetask.CreateRequest{
		TaskID:              req.TaskID,
		UserID:              req.UserID,
		APIKeyID:            req.APIKeyID,
		SourceChannel:       req.SourceChannel,
		UserGroupCode:       req.UserGroupCode,
		UserGroupMultiplier: req.UserGroupMultiplier,
		AbstractModel:       req.AbstractModel,
		TaskType:            req.TaskType,
		Prompt:              req.Prompt,
		RequestedSize:       req.RequestedSize,
		RequestedQuality:    req.RequestedQuality,
		OutputImageCount:    req.OutputImageCount,
		ReferenceImageCount: len(req.ReferenceImages),
		ReferenceAssetIDs:   nil,
		MaskPresent:         req.Mask != nil,
		ResponseMode:        "sync",
		SavePolicy:          "private",
	}, resolved, domainimagetask.StatusRunning)
	leaseOwner := "inline-executor"
	leaseExpiresAt := time.Now().UTC().Add(2 * time.Minute)
	task.LeaseOwner = leaseOwner
	task.LeaseExpiresAt = &leaseExpiresAt
	if err := s.applyTaskEstimate(ctx, &task, domainimagetask.CreateRequest{
		UserID:              req.UserID,
		APIKeyID:            req.APIKeyID,
		SourceChannel:       req.SourceChannel,
		UserGroupCode:       req.UserGroupCode,
		UserGroupMultiplier: req.UserGroupMultiplier,
		AbstractModel:       req.AbstractModel,
		TaskType:            req.TaskType,
		Prompt:              req.Prompt,
		RequestedSize:       req.RequestedSize,
		RequestedQuality:    req.RequestedQuality,
		OutputImageCount:    req.OutputImageCount,
		ReferenceImageCount: len(req.ReferenceImages),
		MaskPresent:         req.Mask != nil,
		TaskID:              task.ID,
	}); err != nil {
		return domainimagetask.ExecuteResult{}, err
	}
	if err := s.store.Save(ctx, task); err != nil {
		if rollbackErr := s.rollbackTaskReserve(ctx, task); rollbackErr != nil {
			return domainimagetask.ExecuteResult{}, errs.Internal("failed to persist image task and rollback reserved points")
		}
		return domainimagetask.ExecuteResult{}, err
	}

	return s.executeResolvedTask(ctx, task, leaseOwner, resolved, executionOptions{
		prompt:             req.Prompt,
		size:               req.RequestedSize,
		responseFormat:     req.ResponseFormat,
		user:               req.User,
		referenceImages:    append([]provider.ImageInput(nil), req.ReferenceImages...),
		mask:               req.Mask,
		preferredProviders: req.PreferredProviders,
	})
}

func (s *Service) ExecuteLeasedTask(ctx context.Context, task domainimagetask.Task, owner string, preferredProviders []string) (domainimagetask.ExecuteResult, error) {
	latestTask, err := s.store.GetByID(ctx, task.UserID, task.ID)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return domainimagetask.ExecuteResult{}, errs.New(404, errs.CodeNotFound, "image task not found")
		}
		return domainimagetask.ExecuteResult{}, errs.Internal("failed to load leased image task")
	}
	task = latestTask

	if stored, ok, err := terminalTaskResult(task); err != nil || ok {
		return stored, err
	}
	if !leaseOwnedBy(task, owner, time.Now().UTC()) {
		return domainimagetask.ExecuteResult{}, errs.New(409, errs.CodeConflict, "image task lease conflict")
	}

	if recovered, ok, err := s.resumeTerminalization(ctx, task, owner); err != nil || ok {
		return recovered, err
	}

	referenceImages, err := s.loadReferenceImages(task)
	if err != nil {
		return s.failOwnedTask(ctx, task, owner, err)
	}

	resolved, err := s.resolveTask(ctx, task.AbstractModel, task.TaskType, task.RequestedQuality, task.RequestedSize, task.OutputImageCount, len(referenceImages), false)
	if err != nil {
		return s.failOwnedTask(ctx, task, owner, err)
	}

	task.LeaseOwner = owner
	return s.executeResolvedTask(ctx, task, owner, resolved, executionOptions{
		prompt:             task.Prompt,
		size:               task.RequestedSize,
		responseFormat:     string(provider.ResponseFormatURL),
		referenceImages:    referenceImages,
		preferredProviders: preferredProviders,
	})
}

func (s *Service) executeResolvedTask(ctx context.Context, task domainimagetask.Task, owner string, resolved modelhub.ResolvedRequest, opts executionOptions) (domainimagetask.ExecuteResult, error) {
	preferredProviders := opts.preferredProviders
	if resolved.RuntimeRoutingApplied {
		preferredProviders = nil
	}
	orderedProviders := s.orderedProviders(resolved.Providers, preferredProviders)
	if len(orderedProviders) == 0 {
		return s.failOwnedTask(ctx, task, owner, errs.New(503, errs.CodeUpstreamUnavailable, "no provider client configured"))
	}

	var lastErr error
	for _, candidate := range orderedProviders {
		providerClient, ok := s.providers[candidate.Provider]
		if !ok {
			continue
		}
		providerReq := provider.ImageRequest{
			Model:            s.providerModelName(task.AbstractModel, candidate.Provider),
			TaskType:         provider.TaskType(task.TaskType),
			Prompt:           opts.prompt,
			Size:             defaultString(opts.size, "auto"),
			Quality:          resolved.ResolvedQualityBucket,
			OutputImageCount: normalizedCount(task.OutputImageCount),
			ResponseFormat:   normalizeResponseFormat(opts.responseFormat),
			ReferenceImages:  append([]provider.ImageInput(nil), opts.referenceImages...),
			Mask:             opts.mask,
			User:             opts.user,
		}

		var (
			resp provider.ImageResponse
			err  error
		)
		if task.TaskType == string(provider.TaskTypeImageEdit) {
			resp, err = providerClient.Edit(ctx, providerReq)
		} else {
			resp, err = providerClient.Generate(ctx, providerReq)
		}
		if err == nil {
			persistedResults, persistErr := s.persistImageResults(ctx, task, resp.Data)
			if persistErr != nil {
				return s.failOwnedTask(ctx, task, owner, persistErr)
			}
			task.Status = domainimagetask.StatusRunning
			task.Provider = candidate.Provider
			task.Attempts = append(task.Attempts, domainimagetask.Attempt{Provider: candidate.Provider, Status: domainimagetask.StatusSucceeded})
			task.Results = append([]provider.ImageResult(nil), persistedResults...)
			if billingErr := s.applyActualPoints(&task, len(resp.Data)); billingErr != nil {
				return s.failOwnedTask(ctx, task, owner, billingErr)
			}
			if saveErr := s.saveOwnedTask(ctx, task, owner); saveErr != nil {
				recoveredTask, recovered, recoverErr := s.recoverTerminalLeaseConflict(ctx, task, owner, domainimagetask.StatusSucceeded, "task succeeded after lease conflict")
				if recoverErr != nil {
					return domainimagetask.ExecuteResult{}, recoverErr
				}
				if recovered {
					return domainimagetask.ExecuteResult{Task: recoveredTask, Response: resp}, nil
				}
				return domainimagetask.ExecuteResult{}, saveErr
			}
			if settleErr := s.settleTaskBilling(ctx, task, "task succeeded"); settleErr != nil {
				return domainimagetask.ExecuteResult{}, settleErr
			}
			task.Status = domainimagetask.StatusSucceeded
			task.LeaseOwner = ""
			task.LeaseExpiresAt = nil
			if saveErr := s.saveOwnedTask(ctx, task, owner); saveErr != nil {
				return domainimagetask.ExecuteResult{}, saveErr
			}
			return domainimagetask.ExecuteResult{Task: task, Response: resp}, nil
		}

		lastErr = err
		task.Attempts = append(task.Attempts, domainimagetask.Attempt{Provider: candidate.Provider, Status: domainimagetask.StatusFailed, Error: err.Error()})
		shouldRetry := false
		if upstream, ok := provider.AsUpstreamError(err); ok && upstream.Action == provider.UpstreamErrorActionRetry {
			shouldRetry = true
		}
		if saveErr := s.saveOwnedTask(ctx, task, owner); saveErr != nil {
			if shouldRetry {
				return domainimagetask.ExecuteResult{}, saveErr
			}
			return s.failOwnedTask(ctx, task, owner, err)
		}
		if shouldRetry {
			continue
		}
		break
	}

	return s.failOwnedTask(ctx, task, owner, lastErr)
}

func (s *Service) failOwnedTask(ctx context.Context, task domainimagetask.Task, owner string, failure error) (domainimagetask.ExecuteResult, error) {
	task.Status = domainimagetask.StatusRunning
	task.ActualPoints = s.zeroPoints()
	task.ErrorCode = errorCode(failure)
	task.ErrorMessage = errorMessage(failure)
	if saveErr := s.saveOwnedTask(ctx, task, owner); saveErr != nil {
		recoveredTask, recovered, recoverErr := s.recoverTerminalLeaseConflict(ctx, task, owner, domainimagetask.StatusFailed, "task failed after lease conflict")
		if recoverErr != nil {
			return domainimagetask.ExecuteResult{}, recoverErr
		}
		if recovered {
			return domainimagetask.ExecuteResult{Task: recoveredTask}, failure
		}
		return domainimagetask.ExecuteResult{}, saveErr
	}
	if settleErr := s.settleTaskBilling(ctx, task, "task failed"); settleErr != nil {
		return domainimagetask.ExecuteResult{}, settleErr
	}
	task.Status = domainimagetask.StatusFailed
	task.LeaseOwner = ""
	task.LeaseExpiresAt = nil
	if saveErr := s.saveOwnedTask(ctx, task, owner); saveErr != nil {
		return domainimagetask.ExecuteResult{}, saveErr
	}
	return domainimagetask.ExecuteResult{Task: task}, failure
}

func (s *Service) resumeTerminalization(ctx context.Context, task domainimagetask.Task, owner string) (domainimagetask.ExecuteResult, bool, error) {
	switch task.Status {
	case domainimagetask.StatusSucceeded:
		return domainimagetask.ExecuteResult{
			Task: task,
			Response: provider.ImageResponse{
				Data: append([]provider.ImageResult(nil), task.Results...),
			},
		}, true, nil
	case domainimagetask.StatusFailed:
		return domainimagetask.ExecuteResult{Task: task}, true, errs.New(500, defaultString(task.ErrorCode, errs.CodeInternal), defaultString(task.ErrorMessage, "image task failed"))
	}

	if len(task.Results) == 0 && strings.TrimSpace(task.ErrorCode) == "" && strings.TrimSpace(task.ErrorMessage) == "" {
		return domainimagetask.ExecuteResult{}, false, nil
	}

	if len(task.Results) > 0 {
		if settleErr := s.settleTaskBilling(ctx, task, "resume settled image task"); settleErr != nil {
			return domainimagetask.ExecuteResult{}, true, settleErr
		}
		task.Status = domainimagetask.StatusSucceeded
		task.LeaseOwner = ""
		task.LeaseExpiresAt = nil
		if saveErr := s.saveOwnedTask(ctx, task, owner); saveErr != nil {
			return domainimagetask.ExecuteResult{}, true, saveErr
		}
		return domainimagetask.ExecuteResult{
			Task: task,
			Response: provider.ImageResponse{
				Data: append([]provider.ImageResult(nil), task.Results...),
			},
		}, true, nil
	}

	if settleErr := s.settleTaskBilling(ctx, task, "resume failed image task"); settleErr != nil {
		return domainimagetask.ExecuteResult{}, true, settleErr
	}
	task.Status = domainimagetask.StatusFailed
	task.LeaseOwner = ""
	task.LeaseExpiresAt = nil
	if saveErr := s.saveOwnedTask(ctx, task, owner); saveErr != nil {
		return domainimagetask.ExecuteResult{}, true, saveErr
	}
	return domainimagetask.ExecuteResult{Task: task}, true, errs.New(500, defaultString(task.ErrorCode, errs.CodeInternal), defaultString(task.ErrorMessage, "image task failed"))
}

func (s *Service) resolveTask(ctx context.Context, abstractModel, taskType, requestedQuality, requestedSize string, outputImageCount, referenceImageCount int, maskPresent bool) (modelhub.ResolvedRequest, error) {
	return s.resolver.ResolveContext(ctx, modelhub.ResolveRequest{
		AbstractModel:             abstractModel,
		TaskType:                  taskType,
		RequestedQuality:          requestedQuality,
		RequestedSize:             requestedSize,
		RequestedOutputImageCount: outputImageCount,
		ReferenceImageCount:       referenceImageCount,
		MaskPresent:               maskPresent,
	})
}

func (s *Service) GetByID(ctx context.Context, userID int64, taskID string) (domainimagetask.Task, error) {
	task, err := s.store.GetByID(ctx, userID, taskID)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return domainimagetask.Task{}, errs.New(404, errs.CodeNotFound, "image task not found")
		}
		return domainimagetask.Task{}, errs.Internal("failed to load image task")
	}
	return cloneTask(task), nil
}

func (s *Service) DownloadImageResult(ctx context.Context, userID int64, imageID string) (provider.ImageResult, []byte, error) {
	result, err := s.store.GetImageResultByID(ctx, userID, imageID)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return provider.ImageResult{}, nil, errs.New(404, errs.CodeNotFound, "image not found")
		}
		return provider.ImageResult{}, nil, errs.Internal("failed to load image result")
	}
	if result.StorageDriver != "local" || strings.TrimSpace(result.ObjectKey) == "" {
		return provider.ImageResult{}, nil, errs.New(404, errs.CodeNotFound, "image not found")
	}
	fullPath, ok := generatedImagePath(localStorageRoot(s.cfg.Storage.LocalRoot), result.ObjectKey)
	if !ok {
		return provider.ImageResult{}, nil, errs.New(404, errs.CodeNotFound, "image not found")
	}
	content, readErr := os.ReadFile(fullPath)
	if readErr != nil {
		return provider.ImageResult{}, nil, errs.New(404, errs.CodeNotFound, "image not found")
	}
	return result, content, nil
}

func (s *Service) ListByUser(ctx context.Context, userID int64) ([]domainimagetask.Task, error) {
	list, err := s.store.ListByUser(ctx, userID)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return nil, errs.New(404, errs.CodeNotFound, "image task not found")
		}
		return nil, errs.Internal("failed to list image tasks")
	}
	return list, nil
}

func (s *Service) DeleteByID(ctx context.Context, userID int64, taskID string) error {
	if err := s.store.DeleteByID(ctx, userID, taskID); err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return errs.New(404, errs.CodeNotFound, "image task not found")
		}
		return errs.Internal("failed to delete image task")
	}
	return nil
}

func (s *Service) saveOwnedTask(ctx context.Context, task domainimagetask.Task, owner string) error {
	if err := s.store.SaveIfOwned(ctx, task, owner, time.Now().UTC()); err != nil {
		switch {
		case errors.Is(err, repoerr.ErrNotFound):
			return errs.New(404, errs.CodeNotFound, "image task not found")
		case errors.Is(err, repoerr.ErrConflict):
			return errs.New(409, errs.CodeConflict, "image task lease conflict")
		default:
			return errs.Internal("failed to persist leased image task")
		}
	}
	return nil
}

func (s *Service) saveTerminalState(ctx context.Context, task domainimagetask.Task, owner string) error {
	if err := s.store.SaveTerminalState(ctx, task, owner, time.Now().UTC()); err != nil {
		switch {
		case errors.Is(err, repoerr.ErrNotFound):
			return errs.New(404, errs.CodeNotFound, "image task not found")
		case errors.Is(err, repoerr.ErrConflict):
			return errs.New(409, errs.CodeConflict, "image task lease conflict")
		default:
			return errs.Internal("failed to persist recoverable image task state")
		}
	}
	return nil
}

func (s *Service) recoverTerminalLeaseConflict(ctx context.Context, task domainimagetask.Task, owner, terminalStatus, settleReason string) (domainimagetask.Task, bool, error) {
	snapshotErr := s.saveTerminalState(ctx, task, owner)
	if settleErr := s.settleTaskBilling(ctx, task, settleReason); settleErr != nil {
		return domainimagetask.Task{}, false, settleErr
	}
	if snapshotErr != nil {
		if isTerminalRecoveryRace(snapshotErr) {
			return domainimagetask.Task{}, false, nil
		}
		return domainimagetask.Task{}, false, snapshotErr
	}

	recoveredTask := cloneTask(task)
	recoveredTask.Status = terminalStatus
	recoveredTask.LeaseOwner = ""
	recoveredTask.LeaseExpiresAt = nil
	if persistErr := s.saveTerminalState(ctx, recoveredTask, owner); persistErr != nil && !isTerminalRecoveryRace(persistErr) {
		return domainimagetask.Task{}, false, persistErr
	}
	return recoveredTask, true, nil
}

func isTerminalRecoveryRace(err error) bool {
	appErr, ok := err.(*errs.Error)
	if !ok {
		return false
	}
	return appErr.Code == errs.CodeConflict || appErr.Code == errs.CodeNotFound
}

func terminalTaskResult(task domainimagetask.Task) (domainimagetask.ExecuteResult, bool, error) {
	switch task.Status {
	case domainimagetask.StatusSucceeded:
		return domainimagetask.ExecuteResult{
			Task: task,
			Response: provider.ImageResponse{
				Data: append([]provider.ImageResult(nil), task.Results...),
			},
		}, true, nil
	case domainimagetask.StatusFailed:
		return domainimagetask.ExecuteResult{Task: task}, true, errs.New(500, defaultString(task.ErrorCode, errs.CodeInternal), defaultString(task.ErrorMessage, "image task failed"))
	default:
		return domainimagetask.ExecuteResult{}, false, nil
	}
}

func (s *Service) persistImageResults(_ context.Context, task domainimagetask.Task, results []provider.ImageResult) ([]provider.ImageResult, error) {
	persisted := make([]provider.ImageResult, 0, len(results))
	for index, result := range results {
		item := result
		item.VisibilityStatus = defaultString(item.VisibilityStatus, "private")
		if strings.TrimSpace(item.URL) != "" {
			if strings.TrimSpace(item.ID) == "" {
				item.ID = uuid.NewString()
			}
			item.StorageDriver = "remote"
			item.MimeType = defaultString(item.MimeType, defaultString(item.Format, "application/octet-stream"))
			persisted = append(persisted, item)
			continue
		}
		if strings.TrimSpace(item.B64JSON) == "" {
			persisted = append(persisted, item)
			continue
		}
		local, err := s.persistBase64ImageResult(task, index, item)
		if err != nil {
			return nil, err
		}
		persisted = append(persisted, local)
	}
	return persisted, nil
}

func (s *Service) persistBase64ImageResult(task domainimagetask.Task, index int, result provider.ImageResult) (provider.ImageResult, error) {
	content, err := decodeBase64ImageResult(result.B64JSON)
	if err != nil {
		return provider.ImageResult{}, err
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return provider.ImageResult{}, errs.New(500, errs.CodeImageStorageFailed, "generated image has unsupported format")
	}
	mimeType := imageMimeType(format, result)
	hash := sha256.Sum256(content)
	sha := hex.EncodeToString(hash[:])
	resultID := uuid.NewString()
	ext := imageExtension(format, mimeType)
	objectKey := filepath.ToSlash(filepath.Join("generated-images", fmt.Sprintf("%d", task.UserID), task.ID, fmt.Sprintf("%d-%s%s", index, resultID, ext)))
	fullPath := filepath.Join(localStorageRoot(s.cfg.Storage.LocalRoot), filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return provider.ImageResult{}, errs.New(500, errs.CodeImageStorageFailed, "failed to prepare generated image storage")
	}
	if err := os.WriteFile(fullPath, content, 0o644); err != nil {
		return provider.ImageResult{}, errs.New(500, errs.CodeImageStorageFailed, "failed to store generated image")
	}
	result.ID = resultID
	result.B64JSON = ""
	result.Format = defaultString(result.Format, format)
	result.MimeType = mimeType
	result.FileSizeBytes = int64(len(content))
	result.Width = cfg.Width
	result.Height = cfg.Height
	result.SHA256 = sha
	result.ObjectKey = objectKey
	result.StorageDriver = "local"
	result.VisibilityStatus = defaultString(result.VisibilityStatus, "private")
	result.DownloadURL = "/api/agent/image/v1/images/" + resultID
	result.URL = result.DownloadURL
	return result, nil
}

func decodeBase64ImageResult(value string) ([]byte, error) {
	trimmed := strings.TrimSpace(value)
	if comma := strings.Index(trimmed, ","); strings.HasPrefix(trimmed, "data:") && comma >= 0 {
		trimmed = trimmed[comma+1:]
	}
	content, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		content, err = base64.RawStdEncoding.DecodeString(trimmed)
	}
	if err != nil {
		return nil, errs.New(500, errs.CodeImageStorageFailed, "generated image base64 is invalid")
	}
	return content, nil
}

func imageMimeType(format string, result provider.ImageResult) string {
	if strings.TrimSpace(result.MimeType) != "" {
		return result.MimeType
	}
	if strings.Contains(result.Format, "/") {
		return result.Format
	}
	if strings.TrimSpace(format) != "" {
		return "image/" + strings.ToLower(format)
	}
	if strings.TrimSpace(result.Format) != "" {
		return "image/" + strings.ToLower(result.Format)
	}
	return "application/octet-stream"
}

func imageExtension(format, mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "jpeg":
		return ".jpg"
	case "jpg", "png", "gif", "webp":
		return "." + strings.ToLower(format)
	}
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}

func localStorageRoot(root string) string {
	if strings.TrimSpace(root) == "" {
		return filepath.Join(os.TempDir(), "pic-gallery")
	}
	return root
}

func generatedImagePath(root string, objectKey string) (string, bool) {
	cleanKey := filepath.ToSlash(filepath.Clean(filepath.FromSlash(objectKey)))
	if cleanKey == "." || strings.HasPrefix(cleanKey, "../") || strings.HasPrefix(cleanKey, "/") {
		return "", false
	}
	if cleanKey != "generated-images" && !strings.HasPrefix(cleanKey, "generated-images/") {
		return "", false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	fullPath, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(cleanKey)))
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(rootAbs, fullPath)
	if err != nil {
		return "", false
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", false
	}
	return fullPath, true
}

func leaseOwnedBy(task domainimagetask.Task, owner string, now time.Time) bool {
	if task.Status != domainimagetask.StatusRunning {
		return false
	}
	if task.LeaseOwner != owner {
		return false
	}
	return task.LeaseExpiresAt == nil || !task.LeaseExpiresAt.Before(now)
}

func (s *Service) loadReferenceImages(task domainimagetask.Task) ([]provider.ImageInput, error) {
	if len(task.ReferenceAssetIDs) == 0 {
		return nil, nil
	}
	if s.assets == nil {
		return nil, errs.New(500, errs.CodeImageReferenceRequired, "reference asset loader unavailable")
	}
	inputs := make([]provider.ImageInput, 0, len(task.ReferenceAssetIDs))
	for _, assetID := range task.ReferenceAssetIDs {
		input, err := s.assets.LoadInput(task.UserID, assetID)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, input)
	}
	return inputs, nil
}

func defaultProviders(cfg config.Config) map[string]provider.ImageProvider {
	providers := map[string]provider.ImageProvider{}
	if cfg.Providers.OpenAI.Enabled {
		providers["openai"] = openaiprovider.NewClient(openaiprovider.Config{BaseURL: cfg.Providers.OpenAI.BaseURL, APIKey: cfg.Providers.OpenAI.APIKey})
	}
	if cfg.Providers.OpenRouter.Enabled {
		providers["openrouter"] = openrouterprovider.NewClient(openrouterprovider.Config{BaseURL: cfg.Providers.OpenRouter.BaseURL, APIKey: cfg.Providers.OpenRouter.APIKey})
	}
	return providers
}

func (s *Service) orderedProviders(candidates []modelhub.ProviderCandidate, preferred []string) []modelhub.ProviderCandidate {
	if len(preferred) == 0 {
		return candidates
	}
	ordered := make([]modelhub.ProviderCandidate, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, name := range preferred {
		for _, candidate := range candidates {
			if strings.EqualFold(candidate.Provider, name) {
				ordered = append(ordered, candidate)
				seen[strings.ToLower(candidate.Provider)] = struct{}{}
			}
		}
	}
	for _, candidate := range candidates {
		if _, ok := seen[strings.ToLower(candidate.Provider)]; ok {
			continue
		}
		ordered = append(ordered, candidate)
	}
	return ordered
}

func (s *Service) providerModelName(abstractModel, providerName string) string {
	if value := strings.TrimSpace(s.cfg.Routing.ProviderModelMap[strings.ToLower(abstractModel)][strings.ToLower(providerName)]); value != "" {
		return value
	}
	return abstractModel
}

func (s *Service) applyTaskEstimate(ctx context.Context, task *domainimagetask.Task, req domainimagetask.CreateRequest) error {
	if s.billing == nil {
		return nil
	}

	estimate, err := s.billing.Estimate(domainbilling.EstimateRequest{
		TaskType:                  req.TaskType,
		AbstractModel:             req.AbstractModel,
		RequestedQuality:          req.RequestedQuality,
		RequestedSize:             req.RequestedSize,
		RequestedOutputImageCount: task.OutputImageCount,
		ReferenceImageCount:       req.ReferenceImageCount,
		UserGroupCode:             req.UserGroupCode,
		UserGroupMultiplier:       req.UserGroupMultiplier,
	})
	if err != nil {
		return err
	}

	task.ResolvedQualityBucket = estimate.ResolvedQualityBucket
	task.EstimatedPoints = estimate.EstimatedPoints
	task.ActualPoints = s.zeroPoints()
	task.PricingSnapshot = estimate.PricingSnapshot

	var apiKeyQuota domainbilling.APIKeyQuota
	if s.apiKeys != nil && req.APIKeyID > 0 {
		quota, err := s.apiKeys.CheckTaskAllowed(ctx, req.APIKeyID, req.UserID, estimate.EstimatedPoints, time.Now())
		if err != nil {
			return err
		}
		apiKeyQuota = quota
	}
	if _, err := s.billing.ReserveTask(ctx, domainbilling.ReserveRequest{
		UserID:          req.UserID,
		APIKeyID:        req.APIKeyID,
		TaskID:          task.ID,
		EstimatedPoints: estimate.EstimatedPoints,
		Reason:          "reserve image task points",
		APIKeyQuota:     apiKeyQuota,
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) rollbackTaskReserve(ctx context.Context, task domainimagetask.Task) error {
	if s.billing == nil || strings.TrimSpace(task.EstimatedPoints) == "" {
		return nil
	}
	_, err := s.billing.FinalizeTask(ctx, domainbilling.FinalizeRequest{
		UserID:          task.UserID,
		APIKeyID:        task.APIKeyID,
		TaskID:          task.ID,
		EstimatedPoints: task.EstimatedPoints,
		ActualPoints:    s.zeroPoints(),
		Reason:          "rollback reserved image task points",
	})
	return err
}

func (s *Service) applyActualPoints(task *domainimagetask.Task, successOutputImageCount int) error {
	if s.billing == nil || strings.TrimSpace(task.EstimatedPoints) == "" {
		task.ActualPoints = s.zeroPoints()
		return nil
	}

	actual, err := s.billing.ActualPoints(task.PricingSnapshot, successOutputImageCount)
	if err != nil {
		return fmt.Errorf("calculate actual image task points: %w", err)
	}
	task.ActualPoints = actual
	return nil
}

func (s *Service) settleTaskBilling(ctx context.Context, task domainimagetask.Task, reason string) error {
	if s.billing == nil || strings.TrimSpace(task.EstimatedPoints) == "" {
		return nil
	}
	_, err := s.billing.FinalizeTask(ctx, domainbilling.FinalizeRequest{
		UserID:          task.UserID,
		APIKeyID:        task.APIKeyID,
		TaskID:          task.ID,
		EstimatedPoints: task.EstimatedPoints,
		ActualPoints:    defaultString(task.ActualPoints, s.zeroPoints()),
		Reason:          reason,
	})
	return err
}

func (s *Service) zeroPoints() string {
	return fmt.Sprintf("%.*f", 5, 0.0)
}

func buildTask(req domainimagetask.CreateRequest, resolved modelhub.ResolvedRequest, status string) domainimagetask.Task {
	taskID := strings.TrimSpace(req.TaskID)
	if taskID == "" {
		taskID = uuid.NewString()
	}
	return domainimagetask.Task{
		UserID:                req.UserID,
		APIKeyID:              req.APIKeyID,
		SourceChannel:         defaultString(req.SourceChannel, "web"),
		ID:                    taskID,
		Status:                status,
		AbstractModel:         req.AbstractModel,
		TaskType:              req.TaskType,
		Prompt:                req.Prompt,
		NegativePrompt:        req.NegativePrompt,
		RequestedSize:         req.RequestedSize,
		RequestedQuality:      defaultString(req.RequestedQuality, "auto"),
		AspectRatio:           defaultString(req.AspectRatio, "1:1"),
		ResolvedQualityBucket: resolved.ResolvedQualityBucket,
		ResponseMode:          defaultString(req.ResponseMode, "async"),
		SavePolicy:            defaultString(req.SavePolicy, "private"),
		OutputImageCount:      normalizedCount(req.OutputImageCount),
		ReferenceImageCount:   req.ReferenceImageCount,
		ReferenceAssetIDs:     append([]string(nil), req.ReferenceAssetIDs...),
		ReferenceStrength:     req.ReferenceStrength,
		Seed:                  req.Seed,
	}
}

func normalizeResponseFormat(value string) provider.ResponseFormat {
	if strings.EqualFold(strings.TrimSpace(value), string(provider.ResponseFormatB64JSON)) {
		return provider.ResponseFormatB64JSON
	}
	return provider.ResponseFormatURL
}

func normalizedCount(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func errorCode(err error) string {
	if err == nil {
		return ""
	}
	if upstream, ok := provider.AsUpstreamError(err); ok {
		return upstream.Code
	}
	var appErr *errs.Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ""
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func cloneTask(task domainimagetask.Task) domainimagetask.Task {
	task.Attempts = append([]domainimagetask.Attempt(nil), task.Attempts...)
	task.Results = append([]provider.ImageResult(nil), task.Results...)
	task.ReferenceAssetIDs = append([]string(nil), task.ReferenceAssetIDs...)
	if task.Seed != nil {
		seed := *task.Seed
		task.Seed = &seed
	}
	if task.LeaseExpiresAt != nil {
		expiresAt := *task.LeaseExpiresAt
		task.LeaseExpiresAt = &expiresAt
	}
	return task
}
