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
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/errgroup"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/provider"
	openaiprovider "github.com/fatballfish/pic-gallery/internal/provider/openai"
	openrouterprovider "github.com/fatballfish/pic-gallery/internal/provider/openrouter"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/internal/service/secretcodec"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type Service struct {
	cfg           config.Config
	resolver      *modelhub.Resolver
	providers     map[string]provider.ImageProvider
	store         Store
	assets        AssetLoader
	billing       BillingManager
	apiKeys       APIKeyUsageManager
	router        storage.Router
	recoveryCodec *secretcodec.Codec
	httpClient    *http.Client
	now           func() time.Time
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

type openAIFanoutProgress struct {
	Response provider.ImageResponse
	Results  []provider.ImageResult
	Failures []error
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
	backend, err := storage.NewBackend(cfg.Storage)
	if err != nil {
		backend = storage.NewLocalBackend(cfg.Storage.LocalRoot)
	}
	return NewServiceWithProvidersStoreAssetsBillingAndBackend(cfg, providers, store, assets, billing, backend)
}

func NewServiceWithProvidersStoreAssetsBillingAndBackend(cfg config.Config, providers map[string]provider.ImageProvider, store Store, assets AssetLoader, billing BillingManager, backend storage.Backend) *Service {
	if backend == nil {
		backend = storage.NewLocalBackend(cfg.Storage.LocalRoot)
	}
	return NewServiceWithProvidersStoreAssetsBillingAndRouter(cfg, providers, store, assets, billing, storage.NewStaticRouter(backend))
}

func NewServiceWithProvidersStoreAssetsBillingAndRouter(cfg config.Config, providers map[string]provider.ImageProvider, store Store, assets AssetLoader, billing BillingManager, router storage.Router) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	if providers == nil {
		providers = defaultProviders(cfg)
	}
	if router == nil {
		router = storage.NewStaticRouter(storage.NewLocalBackend(cfg.Storage.LocalRoot))
	}
	return &Service{
		cfg:           cfg,
		resolver:      modelhub.NewResolver(cfg),
		providers:     providers,
		store:         store,
		assets:        assets,
		billing:       billing,
		router:        router,
		recoveryCodec: secretcodec.New(cfg.Security.SecureConfigEncryptionKey),
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		now:           time.Now,
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

func (s *Service) SetHTTPClient(client *http.Client) {
	if client == nil {
		return
	}
	s.httpClient = client
}

func (s *Service) nowUTC() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
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

	resolved, err := s.resolveTask(ctx, req.TaskID, req.AbstractModel, req.RouteModelCode, req.UserGroupCodes, req.TaskType, req.RequestedQuality, req.RequestedSize, req.OutputImageCount, req.ReferenceImageCount, req.MaskPresent)
	if err != nil {
		_ = s.persistPreflightFailedRequest(ctx, req, resolved, err)
		return domainimagetask.Task{}, err
	}
	if strings.TrimSpace(req.TaskID) == "" {
		req.TaskID = uuid.NewString()
	}

	task := buildTask(req, resolved, domainimagetask.StatusQueued)
	if err := s.applyTaskEstimate(ctx, &task, req); err != nil {
		_ = s.persistPreflightFailedTask(ctx, task, err)
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

func (s *Service) persistPreflightFailedRequest(ctx context.Context, req domainimagetask.CreateRequest, resolved modelhub.ResolvedRequest, failure error) error {
	if strings.TrimSpace(req.TaskID) == "" {
		req.TaskID = uuid.NewString()
	}
	task := buildTask(req, resolved, domainimagetask.StatusFailed)
	return s.persistPreflightFailedTask(ctx, task, failure)
}

func (s *Service) persistPreflightFailedTask(ctx context.Context, task domainimagetask.Task, failure error) error {
	task.Status = domainimagetask.StatusFailed
	task.ErrorCode = errorCode(failure)
	task.ErrorMessage = errorMessage(failure)
	task.ActualPoints = s.zeroPoints()
	if strings.TrimSpace(task.EstimatedPoints) == "" {
		task.EstimatedPoints = s.zeroPoints()
	}
	if strings.TrimSpace(task.ChargedPoints) == "" {
		task.ChargedPoints = s.zeroPoints()
	}
	return s.store.Save(ctx, task)
}

func (s *Service) AcquireNextTask(ctx context.Context, owner string, leaseTTL time.Duration) (domainimagetask.Task, bool, error) {
	task, err := s.store.AcquireNextQueuedTask(ctx, owner, s.nowUTC(), leaseTTL)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return domainimagetask.Task{}, false, nil
		}
		if repoerr.IsTransientContention(err) {
			return domainimagetask.Task{}, false, repoerr.TransientContention(err)
		}
		return domainimagetask.Task{}, false, errs.Internal("failed to acquire queued image task")
	}
	return cloneTask(task), true, nil
}

func (s *Service) HeartbeatTask(ctx context.Context, taskID, owner string, leaseTTL time.Duration) (domainimagetask.Task, error) {
	task, err := s.store.RenewTaskLease(ctx, taskID, owner, s.nowUTC(), leaseTTL)
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
	resolved, err := s.resolveTask(ctx, req.TaskID, req.AbstractModel, req.RouteModelCode, req.UserGroupCodes, req.TaskType, req.RequestedQuality, req.RequestedSize, req.OutputImageCount, len(req.ReferenceImages), req.Mask != nil)
	if err != nil {
		return domainimagetask.ExecuteResult{}, err
	}

	task := buildTask(domainimagetask.CreateRequest{
		TaskID:              req.TaskID,
		UserID:              req.UserID,
		APIKeyID:            req.APIKeyID,
		SourceChannel:       req.SourceChannel,
		UserGroupCode:       req.UserGroupCode,
		UserGroupCodes:      append([]string(nil), req.UserGroupCodes...),
		UserGroupMultiplier: req.UserGroupMultiplier,
		AbstractModel:       req.AbstractModel,
		RouteModelCode:      req.RouteModelCode,
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
	leaseExpiresAt := s.nowUTC().Add(2 * time.Minute)
	task.LeaseOwner = leaseOwner
	task.LeaseExpiresAt = &leaseExpiresAt
	if err := s.applyTaskEstimate(ctx, &task, domainimagetask.CreateRequest{
		UserID:              req.UserID,
		APIKeyID:            req.APIKeyID,
		SourceChannel:       req.SourceChannel,
		UserGroupCode:       req.UserGroupCode,
		UserGroupCodes:      append([]string(nil), req.UserGroupCodes...),
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
	if !leaseOwnedBy(task, owner, s.nowUTC()) {
		return domainimagetask.ExecuteResult{}, errs.New(409, errs.CodeConflict, "image task lease conflict")
	}
	if task.ArtifactRecovery.EncryptedPayload != "" {
		return s.executeArtifactRecovery(ctx, task, owner)
	}

	if recovered, ok, err := s.resumeTerminalization(ctx, task, owner); err != nil || ok {
		return recovered, err
	}

	referenceImages, err := s.loadReferenceImages(task)
	if err != nil {
		return s.failOwnedTask(ctx, task, owner, err)
	}

	resolved, err := s.resolveTask(ctx, task.ID, task.AbstractModel, task.RouteModelCode, nil, task.TaskType, task.RequestedQuality, task.RequestedSize, task.OutputImageCount, len(referenceImages), false)
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

func (s *Service) TestModelAccount(ctx context.Context, req domainimagetask.TestModelAccountRequest, candidate modelhub.ProviderCandidate) (domainimagetask.TestModelAccountResult, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = "A small product photo of a ceramic coffee cup on a clean desk"
	}
	if strings.TrimSpace(req.SourceMode) != "" {
		if candidate.AccountExtra == nil {
			candidate.AccountExtra = map[string]any{}
		}
		candidate.AccountExtra["source_mode"] = strings.TrimSpace(req.SourceMode)
	}
	providerClient, ok := s.providerClientForCandidate(candidate)
	if !ok {
		return domainimagetask.TestModelAccountResult{}, errs.New(409, errs.CodeUpstreamUnavailable, "model account provider is not configured")
	}
	taskID := uuid.NewString()
	task := domainimagetask.Task{
		UserID:                0,
		ID:                    taskID,
		Status:                domainimagetask.StatusRunning,
		SourceChannel:         "admin_test",
		AbstractModel:         candidate.ModelCode,
		TaskType:              string(provider.TaskTypeTextToImage),
		Prompt:                prompt,
		RequestedSize:         "1024x1024",
		RequestedQuality:      "1K",
		AspectRatio:           "1:1",
		ResolvedQualityBucket: "1k",
		ResponseMode:          "sync",
		SavePolicy:            "private",
		OutputImageCount:      1,
		AccountModelID:        candidate.AccountModelID,
		ModelAccountID:        candidate.ModelAccountID,
		UpstreamModelCode:     candidate.ModelCode,
		EstimatedPoints:       s.zeroPoints(),
		ChargedPoints:         s.zeroPoints(),
		ActualPoints:          s.zeroPoints(),
	}
	resolved := modelhub.ResolvedRequest{
		ResolvedQualityBucket: "1k",
		Providers:             []modelhub.ProviderCandidate{candidate},
	}
	providerReq := provider.ImageRequest{
		Model:            candidate.ModelCode,
		TaskType:         provider.TaskTypeTextToImage,
		Prompt:           prompt,
		Size:             task.RequestedSize,
		Quality:          task.ResolvedQualityBucket,
		OutputImageCount: 1,
		ResponseFormat:   provider.ResponseFormatB64JSON,
	}
	if compatErr := applyProviderRequestCompatibility(&providerReq, task, candidate, resolved); compatErr != nil {
		return domainimagetask.TestModelAccountResult{}, compatErr
	}

	startedAt := s.nowUTC()
	resp, err := s.executeProviderRequest(ctx, providerClient, candidate, task.TaskType, providerReq)
	finishedAt := s.nowUTC()
	task.Provider = candidate.Provider
	task.ProviderModelID = candidate.ProviderModelID
	task.RouteModelID = candidate.RouteModelID
	task.RouteModelCode = candidate.RouteModelCode
	task.AccountModelID = candidate.AccountModelID
	task.ModelAccountID = candidate.ModelAccountID
	task.UpstreamModelCode = candidate.ModelCode
	task.RouteSnapshotVersion = candidate.RouteSnapshotVersion
	task.FallbackCount = 0
	if err != nil {
		task.Status = domainimagetask.StatusFailed
		task.ErrorCode = errorCode(err)
		task.ErrorMessage = errorMessage(err)
		task.Attempts = append(task.Attempts, buildProviderAttempt(candidate, domainimagetask.StatusFailed, err, startedAt, finishedAt))
		_ = s.store.Save(ctx, task)
		return domainimagetask.TestModelAccountResult{}, err
	}
	persisted, persistErr := s.persistImageResults(ctx, task, resp.Data)
	if persistErr != nil {
		task.Status = domainimagetask.StatusFailed
		task.ErrorCode = errorCode(persistErr)
		task.ErrorMessage = errorMessage(persistErr)
		task.Attempts = append(task.Attempts, buildProviderAttempt(candidate, domainimagetask.StatusFailed, persistErr, startedAt, finishedAt))
		_ = s.store.Save(ctx, task)
		return domainimagetask.TestModelAccountResult{}, persistErr
	}
	task.Status = domainimagetask.StatusSucceeded
	task.Attempts = append(task.Attempts, buildProviderAttempt(candidate, domainimagetask.StatusSucceeded, nil, startedAt, finishedAt))
	task.Results = append([]provider.ImageResult(nil), persisted...)
	if err := s.store.Save(ctx, task); err != nil {
		return domainimagetask.TestModelAccountResult{}, errs.Internal("failed to persist model account test image")
	}
	result := domainimagetask.TestModelAccountResult{
		Status:            domainimagetask.StatusSucceeded,
		ProviderRequestID: resp.ProviderRequestID,
		ActualParams: map[string]string{
			"model":   providerReq.Model,
			"size":    providerReq.Size,
			"quality": providerReq.Quality,
		},
		ElapsedMS: finishedAt.Sub(startedAt).Milliseconds(),
		Task:      task,
	}
	if len(persisted) > 0 {
		image := persisted[0]
		result.Image = image
		result.ImageURL = image.URL
		result.Width = image.Width
		result.Height = image.Height
	}
	return result, nil
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
		providerClient, ok := s.providerClientForCandidate(candidate)
		if !ok {
			continue
		}
		providerReq := provider.ImageRequest{
			Model:            s.providerModelName(task.AbstractModel, candidate.Provider, candidate.ModelCode),
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
		if compatErr := applyProviderRequestCompatibility(&providerReq, task, candidate, resolved); compatErr != nil {
			return s.failOwnedTask(ctx, task, owner, compatErr)
		}

		attemptStarted := s.nowUTC()
		openAIFormat := strings.EqualFold(candidate.Provider, string(provider.ProviderTypeOpenAI)) || strings.EqualFold(candidate.AdapterType, "openai_compatible")
		if openAIFormat && normalizedCount(task.OutputImageCount) > 1 {
			progress, progressErr := s.executeOpenAIFanoutWithProgress(ctx, providerClient, candidate, task, owner, providerReq)
			attemptFinished := s.nowUTC()
			if progressErr != nil {
				return s.failOwnedTask(ctx, task, owner, progressErr)
			}
			resp := progress.Response
			resp.Data = append([]provider.ImageResult(nil), progress.Results...)
			persistedResults := append([]provider.ImageResult(nil), progress.Results...)
			finalStatus := domainimagetask.StatusSucceeded
			if len(progress.Failures) > 0 && len(persistedResults) > 0 {
				finalStatus = domainimagetask.StatusPartialFailed
			}
			task = s.decorateTaskProvider(task, candidate)
			task.Status = domainimagetask.StatusRunning
			task.FallbackCount = len(task.Attempts)
			task.Attempts = append(task.Attempts, buildProviderAttempt(candidate, domainimagetask.StatusSucceeded, nil, attemptStarted, attemptFinished))
			task.Results = persistedResults
			if len(progress.Failures) > 0 {
				task.ErrorCode = errorCode(progress.Failures[0])
				task.ErrorMessage = errorMessage(progress.Failures[0])
			}
			if billingErr := s.applyActualPoints(&task, len(persistedResults)); billingErr != nil {
				return s.failOwnedTask(ctx, task, owner, billingErr)
			}
			task.ProviderCost = calculateProviderCost(candidate, len(persistedResults))
			task.GrossMargin = calculateGrossMargin(task.ActualPoints, task.ProviderCost)
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
			task.Status = finalStatus
			task.LeaseOwner = ""
			task.LeaseExpiresAt = nil
			if saveErr := s.saveOwnedTask(ctx, task, owner); saveErr != nil {
				return domainimagetask.ExecuteResult{}, saveErr
			}
			return domainimagetask.ExecuteResult{Task: task, Response: resp}, nil
		}

		resp, err := s.executeProviderRequest(ctx, providerClient, candidate, task.TaskType, providerReq)
		attemptFinished := s.nowUTC()
		if err == nil {
			if checkpointErr := s.checkpointProviderSuccess(ctx, &task, owner, candidate, resp, attemptStarted, attemptFinished); checkpointErr != nil {
				return domainimagetask.ExecuteResult{}, checkpointErr
			}
			persistedResults, persistErr := s.persistImageResults(ctx, task, resp.Data)
			if persistErr != nil {
				return s.handleArtifactPersistenceFailure(ctx, task, owner, persistErr)
			}
			finalStatus := domainimagetask.StatusSucceeded
			if openAIFormat && len(persistedResults) < normalizedCount(task.OutputImageCount) {
				finalStatus = domainimagetask.StatusPartialFailed
			}
			task.Status = domainimagetask.StatusRunning
			task.FallbackCount = len(task.Attempts)
			task.Results = append([]provider.ImageResult(nil), persistedResults...)
			task.ArtifactRecovery = completedArtifactRecovery(task.ArtifactRecovery)
			if billingErr := s.applyActualPoints(&task, len(persistedResults)); billingErr != nil {
				return s.failOwnedTask(ctx, task, owner, billingErr)
			}
			task.ProviderCost = calculateProviderCost(candidate, len(persistedResults))
			task.GrossMargin = calculateGrossMargin(task.ActualPoints, task.ProviderCost)
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
			task.Status = finalStatus
			task.LeaseOwner = ""
			task.LeaseExpiresAt = nil
			if saveErr := s.saveOwnedTask(ctx, task, owner); saveErr != nil {
				return domainimagetask.ExecuteResult{}, saveErr
			}
			return domainimagetask.ExecuteResult{Task: task, Response: resp}, nil
		}

		lastErr = err
		task.Provider = candidate.Provider
		task.ProviderModelID = candidate.ProviderModelID
		task.RouteModelID = candidate.RouteModelID
		task.RouteModelCode = candidate.RouteModelCode
		task.AccountModelID = candidate.AccountModelID
		task.ModelAccountID = candidate.ModelAccountID
		task.UpstreamModelCode = candidate.ModelCode
		task.RouteSnapshotVersion = candidate.RouteSnapshotVersion
		task.Attempts = append(task.Attempts, buildProviderAttempt(candidate, domainimagetask.StatusFailed, err, attemptStarted, attemptFinished))
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

func (s *Service) decorateTaskProvider(task domainimagetask.Task, candidate modelhub.ProviderCandidate) domainimagetask.Task {
	task.Provider = candidate.Provider
	task.ProviderModelID = candidate.ProviderModelID
	task.RouteModelID = candidate.RouteModelID
	task.RouteModelCode = candidate.RouteModelCode
	task.AccountModelID = candidate.AccountModelID
	task.ModelAccountID = candidate.ModelAccountID
	task.UpstreamModelCode = candidate.ModelCode
	task.RouteSnapshotVersion = candidate.RouteSnapshotVersion
	return task
}

func (s *Service) executeOpenAIFanoutWithProgress(ctx context.Context, client provider.ImageProvider, candidate modelhub.ProviderCandidate, task domainimagetask.Task, owner string, req provider.ImageRequest) (openAIFanoutProgress, error) {
	call := func(ctx context.Context, singleReq provider.ImageRequest) (provider.ImageResponse, error) {
		if task.TaskType == string(provider.TaskTypeImageEdit) {
			return client.Edit(ctx, singleReq)
		}
		return client.Generate(ctx, singleReq)
	}

	count := normalizedCount(req.OutputImageCount)
	progress := openAIFanoutProgress{
		Results:  make([]provider.ImageResult, 0, count),
		Failures: make([]error, 0),
	}
	var mu sync.Mutex
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(count)
	for i := 0; i < count; i++ {
		group.Go(func() error {
			singleReq := req
			singleReq.OutputImageCount = 1
			resp, err := call(groupCtx, singleReq)
			mu.Lock()
			defer mu.Unlock()
			if progress.Response.Created == 0 {
				progress.Response.Created = resp.Created
			}
			if progress.Response.ProviderRequestID == "" {
				progress.Response.ProviderRequestID = resp.ProviderRequestID
			}
			if err != nil {
				progress.Failures = append(progress.Failures, err)
				return nil
			}
			if len(resp.Data) == 0 {
				progress.Failures = append(progress.Failures, errs.New(502, errs.CodeUpstreamUnavailable, "provider returned no images"))
				return nil
			}
			persisted, persistErr := s.persistImageResults(ctx, task, resp.Data)
			if persistErr != nil {
				progress.Failures = append(progress.Failures, persistErr)
				return nil
			}
			progress.Results = append(progress.Results, persisted...)
			snapshot := s.decorateTaskProvider(task, candidate)
			snapshot.Status = domainimagetask.StatusRunning
			snapshot.Results = append([]provider.ImageResult(nil), progress.Results...)
			if len(progress.Failures) > 0 {
				snapshot.ErrorCode = errorCode(progress.Failures[0])
				snapshot.ErrorMessage = errorMessage(progress.Failures[0])
			}
			if billingErr := s.applyActualPoints(&snapshot, len(snapshot.Results)); billingErr != nil {
				progress.Failures = append(progress.Failures, billingErr)
				return nil
			}
			snapshot.ProviderCost = calculateProviderCost(candidate, len(snapshot.Results))
			snapshot.GrossMargin = calculateGrossMargin(snapshot.ActualPoints, snapshot.ProviderCost)
			if saveErr := s.saveOwnedTask(ctx, snapshot, owner); saveErr != nil {
				return saveErr
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return openAIFanoutProgress{}, err
	}
	if len(progress.Results) == 0 {
		if len(progress.Failures) > 0 {
			return openAIFanoutProgress{}, progress.Failures[0]
		}
		return openAIFanoutProgress{}, errs.New(502, errs.CodeUpstreamUnavailable, "provider returned no images")
	}
	return progress, nil
}

func applyProviderRequestCompatibility(req *provider.ImageRequest, task domainimagetask.Task, candidate modelhub.ProviderCandidate, resolved modelhub.ResolvedRequest) error {
	if req == nil {
		return nil
	}
	if !isGPTImage2Model(req.Model, candidate) || !isOpenAIImagesCompatibleCandidate(candidate) {
		return nil
	}

	if !isGPTImage2CodexSource(req.Model, candidate) {
		req.Quality = mapGPTImage2UpstreamQuality(defaultString(resolved.ResolvedQualityBucket, task.ResolvedQualityBucket), req.Quality)
		return nil
	}

	req.Quality = "auto"
	if width, height, ok := modelhub.ParseImageSize(req.Size); ok && width > 0 && height > 0 {
		return nil
	}
	size, err := modelhub.CalculateImageSize(defaultString(resolved.ResolvedQualityBucket, task.ResolvedQualityBucket), defaultString(task.AspectRatio, "1:1"))
	if err != nil {
		return errs.New(400, errs.CodeImageCapabilityMismatch, "unsupported gpt-image-2 size parameters")
	}
	req.Size = size
	return nil
}

func isGPTImage2Model(model string, candidate modelhub.ProviderCandidate) bool {
	return strings.EqualFold(strings.TrimSpace(model), "gpt-image-2") || strings.EqualFold(strings.TrimSpace(candidate.ModelCode), "gpt-image-2")
}

func isGPTImage2CodexSource(model string, candidate modelhub.ProviderCandidate) bool {
	if !isGPTImage2Model(model, candidate) {
		return false
	}
	return truthyExtra(candidate.AccountExtra, "gpt_image_2_codex_source") ||
		truthyExtra(candidate.ModelExtra, "gpt_image_2_codex_source") ||
		strings.EqualFold(stringExtra(candidate.AccountExtra, "source_mode"), "codex_responses") ||
		strings.EqualFold(stringExtra(candidate.ModelExtra, "source_mode"), "codex_responses")
}

func isOpenAIImagesCompatibleCandidate(candidate modelhub.ProviderCandidate) bool {
	return strings.EqualFold(candidate.Provider, string(provider.ProviderTypeOpenAI)) ||
		strings.EqualFold(candidate.AdapterType, "openai_compatible")
}

func mapGPTImage2UpstreamQuality(resolvedQuality string, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(defaultString(resolvedQuality, fallback))) {
	case "1k", "low":
		return "low"
	case "2k", "medium":
		return "medium"
	case "4k", "high":
		return "high"
	default:
		return "auto"
	}
}

func truthyExtra(extra map[string]any, key string) bool {
	switch value := extra[key].(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true") || strings.EqualFold(strings.TrimSpace(value), "yes") || strings.EqualFold(strings.TrimSpace(value), "1")
	case float64:
		return value != 0
	case int:
		return value != 0
	default:
		return false
	}
}

func stringExtra(extra map[string]any, key string) string {
	if extra == nil {
		return ""
	}
	switch value := extra[key].(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func buildProviderAttempt(candidate modelhub.ProviderCandidate, status string, err error, startedAt, finishedAt time.Time) domainimagetask.Attempt {
	attempt := domainimagetask.Attempt{
		Provider:       candidate.Provider,
		AdapterType:    candidate.AdapterType,
		AccountModelID: candidate.AccountModelID,
		ModelAccountID: candidate.ModelAccountID,
		ModelCode:      candidate.ModelCode,
		Status:         status,
		StartedAt:      &startedAt,
		FinishedAt:     &finishedAt,
	}
	if err == nil {
		return attempt
	}
	attempt.Error = err.Error()
	attempt.ErrorMessage = err.Error()
	if upstream, ok := provider.AsUpstreamError(err); ok {
		attempt.ErrorCode = upstream.Code
		attempt.ErrorMessage = defaultString(upstream.Message, err.Error())
		attempt.ErrorDetail = map[string]any{
			"provider":    string(upstream.Provider),
			"http_status": upstream.HTTPStatus,
			"code":        upstream.Code,
			"type":        upstream.Type,
			"message":     upstream.Message,
			"request_id":  upstream.RequestID,
			"action":      string(upstream.Action),
			"family":      string(upstream.Family),
		}
	}
	return attempt
}

func (s *Service) executeProviderRequest(ctx context.Context, client provider.ImageProvider, candidate modelhub.ProviderCandidate, taskType string, req provider.ImageRequest) (provider.ImageResponse, error) {
	call := func(ctx context.Context, singleReq provider.ImageRequest) (provider.ImageResponse, error) {
		if taskType == string(provider.TaskTypeImageEdit) {
			return client.Edit(ctx, singleReq)
		}
		return client.Generate(ctx, singleReq)
	}

	count := normalizedCount(req.OutputImageCount)
	openAIFormat := strings.EqualFold(candidate.Provider, string(provider.ProviderTypeOpenAI)) || strings.EqualFold(candidate.AdapterType, "openai_compatible")
	if !openAIFormat || count <= 1 {
		return call(ctx, req)
	}

	results := make([]provider.ImageResult, 0, count)
	var (
		mu        sync.Mutex
		firstResp provider.ImageResponse
		firstErr  error
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(count)
	for i := 0; i < count; i++ {
		group.Go(func() error {
			singleReq := req
			singleReq.OutputImageCount = 1
			resp, err := call(groupCtx, singleReq)
			mu.Lock()
			defer mu.Unlock()
			if firstResp.Created == 0 {
				firstResp.Created = resp.Created
			}
			if firstResp.ProviderRequestID == "" {
				firstResp.ProviderRequestID = resp.ProviderRequestID
			}
			if len(resp.Data) > 0 {
				results = append(results, resp.Data...)
			}
			if err != nil && firstErr == nil {
				firstErr = err
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return provider.ImageResponse{}, err
	}
	if len(results) == 0 {
		if firstErr != nil {
			return provider.ImageResponse{}, firstErr
		}
		return provider.ImageResponse{}, errs.New(502, errs.CodeUpstreamUnavailable, "provider returned no images")
	}
	firstResp.Data = results
	return firstResp, nil
}

func (s *Service) providerClientForCandidate(candidate modelhub.ProviderCandidate) (provider.ImageProvider, bool) {
	if candidate.AdapterType != "" {
		if candidate.AuthType != "" && candidate.AuthType != "api_key" {
			return nil, false
		}
		apiKey := strings.TrimSpace(candidate.Credentials["api_key"])
		if apiKey == "" || strings.TrimSpace(candidate.BaseURL) == "" {
			return nil, false
		}
		switch candidate.AdapterType {
		case "openai_compatible":
			return openaiprovider.NewClient(openaiprovider.Config{BaseURL: candidate.BaseURL, APIKey: apiKey}), true
		case "openrouter":
			return openrouterprovider.NewClient(openrouterprovider.Config{BaseURL: candidate.BaseURL, APIKey: apiKey}), true
		default:
			return nil, false
		}
	}
	providerClient, ok := s.providers[candidate.Provider]
	return providerClient, ok
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

func (s *Service) resolveTask(ctx context.Context, routeKey, abstractModel, routeModelCode string, userGroupCodes []string, taskType, requestedQuality, requestedSize string, outputImageCount, referenceImageCount int, maskPresent bool) (modelhub.ResolvedRequest, error) {
	return s.resolver.ResolveContext(ctx, modelhub.ResolveRequest{
		AbstractModel:             abstractModel,
		RouteModelCode:            routeModelCode,
		TaskType:                  taskType,
		RequestedQuality:          requestedQuality,
		RequestedSize:             requestedSize,
		RequestedOutputImageCount: outputImageCount,
		ReferenceImageCount:       referenceImageCount,
		MaskPresent:               maskPresent,
		RouteKey:                  routeKey,
		UserGroupCodes:            append([]string(nil), userGroupCodes...),
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
	if result.StorageDriver == "remote" || strings.TrimSpace(result.ObjectKey) == "" {
		return provider.ImageResult{}, nil, errs.New(404, errs.CodeNotFound, "image not found")
	}
	backend, routeErr := s.router.BackendFor(ctx, result.StorageConfigID, result.StorageDriver)
	if routeErr != nil {
		return provider.ImageResult{}, nil, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "storage config is unavailable")
	}
	content, readErr := backend.Backend.Get(ctx, result.ObjectKey)
	if readErr != nil {
		return provider.ImageResult{}, nil, errs.New(404, errs.CodeNotFound, "image not found")
	}
	return result, content, nil
}

func (s *Service) DownloadImageResultForAdmin(ctx context.Context, imageID string) (provider.ImageResult, []byte, error) {
	result, err := s.store.GetImageResultForAdmin(ctx, imageID)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return provider.ImageResult{}, nil, errs.New(404, errs.CodeNotFound, "image not found")
		}
		return provider.ImageResult{}, nil, errs.Internal("failed to load image result")
	}
	if result.StorageDriver == "remote" || strings.TrimSpace(result.ObjectKey) == "" {
		return provider.ImageResult{}, nil, errs.New(404, errs.CodeNotFound, "image not found")
	}
	backend, routeErr := s.router.BackendFor(ctx, result.StorageConfigID, result.StorageDriver)
	if routeErr != nil {
		return provider.ImageResult{}, nil, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "storage config is unavailable")
	}
	content, readErr := backend.Backend.Get(ctx, result.ObjectKey)
	if readErr != nil {
		return provider.ImageResult{}, nil, errs.New(404, errs.CodeNotFound, "image not found")
	}
	return result, content, nil
}

func (s *Service) DownloadPublicImageResult(ctx context.Context, imageID string) (provider.ImageResult, []byte, error) {
	image, err := s.store.GetPublicImage(ctx, imageID, 0)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return provider.ImageResult{}, nil, errs.New(404, errs.CodeNotFound, "image not found")
		}
		return provider.ImageResult{}, nil, errs.Internal("failed to load public image result")
	}
	result := provider.ImageResult{
		ID:               image.ID,
		MimeType:         image.MimeType,
		FileSizeBytes:    image.FileSizeBytes,
		Width:            image.Width,
		Height:           image.Height,
		SHA256:           image.SHA256,
		ObjectKey:        image.ObjectKey,
		StorageDriver:    image.StorageDriver,
		StorageConfigID:  image.StorageConfigID,
		ImageGroup:       image.ImageGroup,
		VisibilityStatus: image.VisibilityStatus,
		ReviewReason:     image.ReviewReason,
		PublishedAt:      image.PublishedAt,
	}
	if result.StorageDriver == "remote" || strings.TrimSpace(result.ObjectKey) == "" {
		return provider.ImageResult{}, nil, errs.New(404, errs.CodeNotFound, "image not found")
	}
	backend, routeErr := s.router.BackendFor(ctx, result.StorageConfigID, result.StorageDriver)
	if routeErr != nil {
		return provider.ImageResult{}, nil, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "storage config is unavailable")
	}
	content, readErr := backend.Backend.Get(ctx, result.ObjectKey)
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

func (s *Service) RetryTask(ctx context.Context, userID int64, taskID string, req domainimagetask.RetryRequest) (domainimagetask.Task, error) {
	original, err := s.GetByID(ctx, userID, taskID)
	if err != nil {
		return domainimagetask.Task{}, err
	}
	switch original.Status {
	case domainimagetask.StatusFailed, domainimagetask.StatusPartialFailed, domainimagetask.StatusRejected:
	default:
		return domainimagetask.Task{}, errs.New(409, errs.CodeConflict, "only failed image tasks can be retried")
	}
	retryReq := domainimagetask.CreateRequest{
		UserID:              userID,
		APIKeyID:            original.APIKeyID,
		SourceChannel:       defaultString(original.SourceChannel, "web"),
		AbstractModel:       original.AbstractModel,
		RouteModelCode:      original.RouteModelCode,
		TaskType:            original.TaskType,
		Prompt:              original.Prompt,
		NegativePrompt:      original.NegativePrompt,
		RequestedSize:       original.RequestedSize,
		RequestedQuality:    original.RequestedQuality,
		AspectRatio:         original.AspectRatio,
		OutputImageCount:    original.OutputImageCount,
		ReferenceImageCount: original.ReferenceImageCount,
		ReferenceAssetIDs:   append([]string(nil), original.ReferenceAssetIDs...),
		ReferenceStrength:   original.ReferenceStrength,
		Seed:                original.Seed,
		UserGroupCode:       req.UserGroupCode,
		UserGroupCodes:      append([]string(nil), req.UserGroupCodes...),
		UserGroupMultiplier: req.UserGroupMultiplier,
		ResponseMode:        defaultString(original.ResponseMode, "async"),
		SavePolicy:          defaultString(original.SavePolicy, "private"),
	}
	return s.CreateTask(ctx, retryReq)
}

func (s *Service) DeleteImageResult(ctx context.Context, userID int64, imageID string) error {
	result, err := s.store.GetImageResultByID(ctx, userID, imageID)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return errs.New(404, errs.CodeNotFound, "image not found")
		}
		return errs.Internal("failed to load image result")
	}
	if result.StorageDriver != "remote" && strings.TrimSpace(result.ObjectKey) != "" {
		backend, routeErr := s.router.BackendFor(ctx, result.StorageConfigID, result.StorageDriver)
		if routeErr != nil {
			return errs.Internal("failed to resolve image storage")
		}
		if err := backend.Backend.Delete(ctx, result.ObjectKey); err != nil {
			return errs.Internal("failed to delete image file")
		}
	}
	if _, err := s.store.DeleteImageResult(ctx, userID, imageID); err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return errs.New(404, errs.CodeNotFound, "image not found")
		}
		return errs.Internal("failed to delete image result")
	}
	return nil
}

func (s *Service) RequestPublish(ctx context.Context, userID int64, imageID string) (domainimagetask.GalleryImage, error) {
	image, err := s.store.RequestPublish(ctx, userID, imageID)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return domainimagetask.GalleryImage{}, errs.New(404, errs.CodeNotFound, "image not found")
		}
		return domainimagetask.GalleryImage{}, errs.Internal("failed to request image publish")
	}
	return image, nil
}

func (s *Service) SetImageGroup(ctx context.Context, userID int64, imageID, imageGroup string) (domainimagetask.GalleryImage, error) {
	imageGroup = strings.TrimSpace(imageGroup)
	if len([]rune(imageGroup)) > 64 {
		return domainimagetask.GalleryImage{}, errs.BadRequest("image group is too long")
	}
	image, err := s.store.SetImageGroup(ctx, userID, imageID, imageGroup)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return domainimagetask.GalleryImage{}, errs.New(404, errs.CodeNotFound, "image not found")
		}
		return domainimagetask.GalleryImage{}, errs.Internal("failed to update image group")
	}
	return image, nil
}

func (s *Service) ReviewImage(ctx context.Context, imageID, nextStatus, reviewReason string, publishedAt *time.Time) (domainimagetask.GalleryImage, error) {
	switch nextStatus {
	case domainimagetask.VisibilityApproved, domainimagetask.VisibilityRejected, domainimagetask.VisibilityUnpublished:
	default:
		return domainimagetask.GalleryImage{}, errs.BadRequest("invalid review status")
	}
	image, err := s.store.ReviewImage(ctx, imageID, nextStatus, strings.TrimSpace(reviewReason), publishedAt)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return domainimagetask.GalleryImage{}, errs.New(404, errs.CodeNotFound, "image not found")
		}
		return domainimagetask.GalleryImage{}, errs.Internal("failed to update image review")
	}
	return image, nil
}

func (s *Service) ListGallery(ctx context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	req.Page, req.PageSize = normalizeListPage(req.Page, req.PageSize)
	req.Status = strings.TrimSpace(req.Status)
	page, err := s.store.ListGallery(ctx, req)
	if err != nil {
		return domainimagetask.GalleryPage{}, errs.Internal("failed to list gallery images")
	}
	return page, nil
}

func (s *Service) ListGalleryByUser(ctx context.Context, userID int64, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	req.Page, req.PageSize = normalizeListPage(req.Page, req.PageSize)
	req.Status = strings.TrimSpace(req.Status)
	page, err := s.store.ListGalleryByUser(ctx, userID, req)
	if err != nil {
		return domainimagetask.GalleryPage{}, errs.Internal("failed to list user gallery images")
	}
	return page, nil
}

func (s *Service) ListPublicGallery(ctx context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	req.Page, req.PageSize = normalizeListPage(req.Page, req.PageSize)
	req.Sort = strings.TrimSpace(req.Sort)
	page, err := s.store.ListPublicGallery(ctx, req)
	if err != nil {
		return domainimagetask.GalleryPage{}, errs.Internal("failed to list public gallery images")
	}
	return page, nil
}

func (s *Service) GetPublicImage(ctx context.Context, imageID string, viewerUserID int64) (domainimagetask.GalleryImage, error) {
	image, err := s.store.GetPublicImage(ctx, imageID, viewerUserID)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return domainimagetask.GalleryImage{}, errs.New(404, errs.CodeNotFound, "gallery image not found")
		}
		return domainimagetask.GalleryImage{}, errs.Internal("failed to load public gallery image")
	}
	return image, nil
}

func (s *Service) SetPublicImageInteraction(ctx context.Context, userID int64, imageID, kind string, active bool) (domainimagetask.GalleryImage, error) {
	kind = strings.TrimSpace(kind)
	if kind != "like" && kind != "favorite" {
		return domainimagetask.GalleryImage{}, errs.BadRequest("invalid interaction type")
	}
	image, err := s.store.SetPublicImageInteraction(ctx, userID, imageID, kind, active)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return domainimagetask.GalleryImage{}, errs.New(404, errs.CodeNotFound, "gallery image not found")
		}
		return domainimagetask.GalleryImage{}, errs.Internal("failed to update public image interaction")
	}
	return image, nil
}

func (s *Service) saveOwnedTask(ctx context.Context, task domainimagetask.Task, owner string) error {
	if err := s.store.SaveIfOwned(ctx, task, owner, s.nowUTC()); err != nil {
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
	if err := s.store.SaveTerminalState(ctx, task, owner, s.nowUTC()); err != nil {
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

func (s *Service) persistImageResults(ctx context.Context, task domainimagetask.Task, results []provider.ImageResult) ([]provider.ImageResult, error) {
	persisted := make([]provider.ImageResult, 0, len(results))
	for index, result := range results {
		item := result
		item.VisibilityStatus = defaultString(item.VisibilityStatus, "private")
		if isDataURL(item.URL) {
			item.B64JSON = item.URL
			item.URL = ""
		}
		if strings.TrimSpace(item.URL) != "" {
			mirrored, err := s.persistRemoteImageResult(ctx, task, index, item)
			if err != nil {
				return nil, err
			}
			persisted = append(persisted, mirrored)
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

func isDataURL(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "data:")
}

func (s *Service) persistRemoteImageResult(ctx context.Context, task domainimagetask.Task, index int, result provider.ImageResult) (provider.ImageResult, error) {
	startedAt := s.nowUTC()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, result.URL, nil)
	if err != nil {
		return provider.ImageResult{}, newArtifactFailure(s, errs.CodeArtifactSourceURLInvalid, "fetch", false, err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		failure := classifyFetchError(s, err)
		decorateArtifactHTTPDiagnostic(s, &failure.diagnostic, req, nil, 0, startedAt)
		return provider.ImageResult{}, failure
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		failure := newArtifactFailure(s, errs.CodeArtifactFetchHTTPStatus, "fetch", resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500, fmt.Errorf("upstream artifact returned status %d", resp.StatusCode))
		decorateArtifactHTTPDiagnostic(s, &failure.diagnostic, req, resp, 0, startedAt)
		return provider.ImageResult{}, failure
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxGeneratedArtifactBytes+1))
	if err != nil {
		failure := newArtifactFailure(s, errs.CodeArtifactFetchReadFailed, "read", true, err)
		decorateArtifactHTTPDiagnostic(s, &failure.diagnostic, req, resp, int64(len(content)), startedAt)
		return provider.ImageResult{}, failure
	}
	if int64(len(content)) > maxGeneratedArtifactBytes {
		failure := newArtifactFailure(s, errs.CodeArtifactSizeLimitExceeded, "read", false, fmt.Errorf("generated artifact exceeds %d bytes", maxGeneratedArtifactBytes))
		decorateArtifactHTTPDiagnostic(s, &failure.diagnostic, req, resp, int64(len(content)), startedAt)
		return provider.ImageResult{}, failure
	}
	if len(content) == 0 {
		failure := newArtifactFailure(s, errs.CodeArtifactEmptyBody, "read", true, errors.New("generated artifact body is empty"))
		decorateArtifactHTTPDiagnostic(s, &failure.diagnostic, req, resp, 0, startedAt)
		return provider.ImageResult{}, failure
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		failure := newArtifactFailure(s, errs.CodeArtifactFormatUnsupported, "validate", false, err)
		decorateArtifactHTTPDiagnostic(s, &failure.diagnostic, req, resp, int64(len(content)), startedAt)
		return provider.ImageResult{}, failure
	}
	mimeType := imageMimeType(format, provider.ImageResult{MimeType: defaultString(resp.Header.Get("Content-Type"), result.MimeType), Format: result.Format})
	hash := sha256.Sum256(content)
	sha := hex.EncodeToString(hash[:])
	resultID := strings.TrimSpace(result.ID)
	if resultID == "" {
		resultID = deterministicImageResultID(task.ID, index)
	}
	objectKey := generatedImageObjectKey(task.UserID, task.ID, index, resultID, imageExtension(format, mimeType))
	writer, err := s.artifactWriter(ctx, task)
	if err != nil {
		return provider.ImageResult{}, newArtifactFailure(s, errs.CodeArtifactStorageUnavailable, "resolve_storage", true, err)
	}
	if err := writer.Backend.Put(ctx, objectKey, mimeType, content); err != nil {
		failure := newArtifactFailure(s, errs.CodeArtifactStorageWriteFailed, "store", true, err)
		failure.diagnostic.StorageConfigID, failure.diagnostic.StorageVersion = writer.ConfigID, writer.Version
		failure.diagnostic.BytesRead = int64(len(content))
		return provider.ImageResult{}, failure
	}
	if err := verifyStoredArtifact(ctx, writer.Backend, objectKey, content); err != nil {
		failure := newArtifactFailure(s, errs.CodeArtifactStorageVerifyFailed, "verify", true, err)
		failure.diagnostic.StorageConfigID, failure.diagnostic.StorageVersion = writer.ConfigID, writer.Version
		return provider.ImageResult{}, failure
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
	result.StorageConfigID = writer.ConfigID
	result.StorageDriver = writer.Driver
	result.VisibilityStatus = defaultString(result.VisibilityStatus, "private")
	result.DownloadURL = "/api/agent/image/v1/images/" + resultID
	result.URL = result.DownloadURL
	return result, nil
}

func (s *Service) persistBase64ImageResult(task domainimagetask.Task, index int, result provider.ImageResult) (provider.ImageResult, error) {
	content, err := decodeBase64ImageResult(result.B64JSON)
	if err != nil {
		return provider.ImageResult{}, newArtifactFailure(s, errs.CodeArtifactRecoveryPayloadInvalid, "decode", false, err)
	}
	if int64(len(content)) > maxGeneratedArtifactBytes {
		return provider.ImageResult{}, newArtifactFailure(s, errs.CodeArtifactSizeLimitExceeded, "decode", false, fmt.Errorf("generated artifact exceeds %d bytes", maxGeneratedArtifactBytes))
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return provider.ImageResult{}, newArtifactFailure(s, errs.CodeArtifactFormatUnsupported, "validate", false, err)
	}
	mimeType := imageMimeType(format, result)
	hash := sha256.Sum256(content)
	sha := hex.EncodeToString(hash[:])
	resultID := deterministicImageResultID(task.ID, index)
	objectKey := generatedImageObjectKey(task.UserID, task.ID, index, resultID, imageExtension(format, mimeType))
	writer, err := s.artifactWriter(context.Background(), task)
	if err != nil {
		return provider.ImageResult{}, newArtifactFailure(s, errs.CodeArtifactStorageUnavailable, "resolve_storage", true, err)
	}
	if err := writer.Backend.Put(context.Background(), objectKey, mimeType, content); err != nil {
		failure := newArtifactFailure(s, errs.CodeArtifactStorageWriteFailed, "store", true, err)
		failure.diagnostic.StorageConfigID, failure.diagnostic.StorageVersion = writer.ConfigID, writer.Version
		failure.diagnostic.BytesRead = int64(len(content))
		return provider.ImageResult{}, failure
	}
	if err := verifyStoredArtifact(context.Background(), writer.Backend, objectKey, content); err != nil {
		failure := newArtifactFailure(s, errs.CodeArtifactStorageVerifyFailed, "verify", true, err)
		failure.diagnostic.StorageConfigID, failure.diagnostic.StorageVersion = writer.ConfigID, writer.Version
		return provider.ImageResult{}, failure
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
	result.StorageConfigID = writer.ConfigID
	result.StorageDriver = writer.Driver
	result.VisibilityStatus = defaultString(result.VisibilityStatus, "private")
	result.DownloadURL = "/api/agent/image/v1/images/" + resultID
	result.URL = result.DownloadURL
	return result, nil
}

func deterministicImageResultID(taskID string, index int) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%s:%d", taskID, index))).String()
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

func normalizeListPage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
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

func generatedImageObjectKey(userID int64, taskID string, index int, resultID string, ext string) string {
	return filepath.ToSlash(filepath.Join("generated-images", fmt.Sprintf("%d", userID), taskID, fmt.Sprintf("%d-%s%s", index, resultID, ext)))
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

func (s *Service) providerModelName(abstractModel, providerName, resolvedModelCode string) string {
	if value := strings.TrimSpace(resolvedModelCode); value != "" {
		return value
	}
	if value := strings.TrimSpace(s.cfg.Routing.ProviderModelMap[strings.ToLower(abstractModel)][strings.ToLower(providerName)]); value != "" {
		return value
	}
	return abstractModel
}

func calculateProviderCost(candidate modelhub.ProviderCandidate, outputImageCount int) string {
	unitCost, err := decimal.NewFromString(strings.TrimSpace(candidate.OutputCost))
	if err != nil {
		return "0.00000"
	}
	count := decimal.NewFromInt(int64(normalizedCount(outputImageCount)))
	return unitCost.Mul(count).Round(5).StringFixed(5)
}

func calculateGrossMargin(actualPoints, providerCost string) string {
	actualValue, actualErr := decimal.NewFromString(strings.TrimSpace(actualPoints))
	costValue, costErr := decimal.NewFromString(strings.TrimSpace(providerCost))
	if actualErr != nil || costErr != nil {
		return "0.00000"
	}
	return actualValue.Sub(costValue).Round(5).StringFixed(5)
}

func (s *Service) applyTaskEstimate(ctx context.Context, task *domainimagetask.Task, req domainimagetask.CreateRequest) error {
	if s.billing == nil {
		return nil
	}

	estimate, err := s.billing.Estimate(domainbilling.EstimateRequest{
		TaskType:                  req.TaskType,
		AbstractModel:             req.AbstractModel,
		RouteModelCode:            req.RouteModelCode,
		RequestedQuality:          req.RequestedQuality,
		RequestedSize:             req.RequestedSize,
		RequestedOutputImageCount: task.OutputImageCount,
		ReferenceImageCount:       req.ReferenceImageCount,
		UserGroupCode:             req.UserGroupCode,
		UserGroupCodes:            append([]string(nil), req.UserGroupCodes...),
		UserGroupMultiplier:       req.UserGroupMultiplier,
	})
	if err != nil {
		return err
	}

	task.ResolvedQualityBucket = estimate.ResolvedQualityBucket
	task.EstimatedPoints = estimate.EstimatedPoints
	task.ChargedPoints = defaultString(estimate.ChargedPoints, estimate.EstimatedPoints)
	task.EffectiveMultiplier = estimate.UserGroupMultiplier
	task.ActualPoints = s.zeroPoints()
	task.PricingSnapshot = estimate.PricingSnapshot

	var apiKeyQuota domainbilling.APIKeyQuota
	if s.apiKeys != nil && req.APIKeyID > 0 {
		quota, err := s.apiKeys.CheckTaskAllowed(ctx, req.APIKeyID, req.UserID, estimate.EstimatedPoints, s.nowUTC())
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
		AbstractModel:         defaultString(req.AbstractModel, req.RouteModelCode),
		RouteModelCode:        defaultString(resolved.RouteModelCode, req.RouteModelCode),
		RouteModelID:          resolved.RouteModelID,
		RouteSnapshotVersion:  resolved.RouteSnapshotVersion,
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
	task.ArtifactRecovery.Diagnostics = append([]domainimagetask.ArtifactDiagnostic(nil), task.ArtifactRecovery.Diagnostics...)
	task.ReferenceAssetIDs = append([]string(nil), task.ReferenceAssetIDs...)
	if task.Seed != nil {
		seed := *task.Seed
		task.Seed = &seed
	}
	if task.LeaseExpiresAt != nil {
		expiresAt := *task.LeaseExpiresAt
		task.LeaseExpiresAt = &expiresAt
	}
	if task.UpstreamSucceededAt != nil {
		succeededAt := *task.UpstreamSucceededAt
		task.UpstreamSucceededAt = &succeededAt
	}
	if task.ArtifactRecovery.NextRetryAt != nil {
		nextRetryAt := *task.ArtifactRecovery.NextRetryAt
		task.ArtifactRecovery.NextRetryAt = &nextRetryAt
	}
	return task
}
