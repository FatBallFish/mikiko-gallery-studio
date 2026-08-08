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
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/errgroup"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
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
	cfg             config.Config
	resolver        *modelhub.Resolver
	providers       map[string]provider.ImageProvider
	store           Store
	assets          AssetLoader
	billing         BillingManager
	apiKeys         APIKeyUsageManager
	router          storage.Router
	recoveryCodec   *secretcodec.Codec
	httpClient      *http.Client
	now             func() time.Time
	concurrencyGate ConcurrencyGate
}

type ConcurrencyGate interface {
	Acquire(ctx context.Context, resources []ConcurrencyResource, leaseTTL time.Duration) (func(), error)
}

type ConcurrencyResource struct {
	Key   string
	Limit int
}

type localConcurrencyGate struct {
	mu      sync.Mutex
	entries map[string]*modelAccountConcurrencyState
	changed chan struct{}
}

type modelAccountConcurrencyState struct {
	active int
	limit  int
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
	Attempts []domainimagetask.Attempt
}

type AssetLoader interface {
	LoadInput(userID int64, assetID string) (provider.ImageInput, error)
}

type assetMediaURLLoader interface {
	GetWithContext(ctx context.Context, userID int64, assetID string) (domainassets.ReferenceAsset, error)
}

type BillingManager interface {
	Estimate(req domainbilling.EstimateRequest) (domainbilling.EstimateResult, error)
	ActualPoints(snapshot domainbilling.PricingSnapshot, successOutputImageCount int) (string, error)
	ReserveTask(ctx context.Context, req domainbilling.ReserveRequest) (domainbilling.BalanceSummary, error)
	FinalizeTask(ctx context.Context, req domainbilling.FinalizeRequest) (domainbilling.BalanceSummary, error)
}

type routeTaskPreflightManager interface {
	ResolveAndEstimateRouteTask(ctx context.Context, req domainbilling.EstimateRequest, limits config.GenerationLimitsConfig) (modelhub.ResolvedRequest, domainbilling.EstimateResult, error)
}

type APIKeyUsageManager interface {
	CheckTaskAllowed(ctx context.Context, apiKeyID, userID int64, estimatedPoints string, now time.Time) (domainbilling.APIKeyQuota, error)
}

type UserConcurrencyLimitSource interface {
	UserConcurrencyLimit(ctx context.Context, userID int64) (int, error)
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
		cfg:             cfg,
		resolver:        modelhub.NewResolver(cfg),
		providers:       providers,
		store:           store,
		assets:          assets,
		billing:         billing,
		router:          router,
		recoveryCodec:   secretcodec.New(cfg.Security.SecureConfigEncryptionKey),
		httpClient:      &http.Client{Timeout: 30 * time.Second},
		now:             time.Now,
		concurrencyGate: NewLocalConcurrencyGate(),
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

func (s *Service) SetConcurrencyGate(gate ConcurrencyGate) {
	if gate != nil {
		s.concurrencyGate = gate
	}
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
	if !provider.IsSupportedTaskType(req.TaskType) {
		return domainimagetask.Task{}, errs.BadRequest("unsupported task_type")
	}
	normalizedReq, err := normalizeCreateRequest(req)
	if err != nil {
		_ = s.persistPreflightFailedRequest(ctx, req, modelhub.ResolvedRequest{}, err)
		return domainimagetask.Task{}, err
	}
	req = normalizedReq
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

	var prefetchedEstimate *domainbilling.EstimateResult
	var resolved modelhub.ResolvedRequest
	if manager, ok := s.billing.(routeTaskPreflightManager); ok && strings.TrimSpace(req.RouteModelCode) != "" {
		resolvedEstimate, estimate, resolveErr := manager.ResolveAndEstimateRouteTask(ctx, taskEstimateRequest(req, normalizedCount(req.OutputImageCount)), s.cfg.GenerationLimits)
		resolved, err = resolvedEstimate, resolveErr
		if resolveErr == nil {
			prefetchedEstimate = &estimate
		}
	} else {
		resolved, err = s.resolveTask(ctx, req.TaskID, req.AbstractModel, req.RouteModelCode, req.UserGroupCodes, req.TaskType, req.SizeMode, req.AspectRatio, req.BaseResolution, req.Quality, req.OutputFormat, req.Background, req.OutputCompression, req.Moderation, req.RequestedSize, req.OutputImageCount, req.ReferenceImageCount, req.MaskPresent, req.CapabilityVersion)
	}
	if err != nil {
		_ = s.persistPreflightFailedRequest(ctx, req, resolved, err)
		return domainimagetask.Task{}, err
	}
	if strings.TrimSpace(req.TaskID) == "" {
		req.TaskID = uuid.NewString()
	}

	task := buildTask(req, resolved, domainimagetask.StatusQueued)
	if prefetchedEstimate != nil {
		err = s.applyTaskEstimateResult(ctx, &task, req, *prefetchedEstimate)
	} else {
		err = s.applyTaskEstimate(ctx, &task, req)
	}
	if err != nil {
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
	setTaskProgress(&task, domainimagetask.ProgressStageFailed, defaultString(task.ErrorMessage, "任务生成失败"))
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
	normalizedReq, err := normalizeExecuteRequest(req)
	if err != nil {
		return domainimagetask.ExecuteResult{}, err
	}
	req = normalizedReq
	resolved, err := s.resolveTask(ctx, req.TaskID, req.AbstractModel, req.RouteModelCode, req.UserGroupCodes, req.TaskType, req.SizeMode, req.AspectRatio, req.BaseResolution, req.Quality, req.OutputFormat, req.Background, req.OutputCompression, req.Moderation, req.RequestedSize, req.OutputImageCount, len(req.ReferenceImages), req.Mask != nil, "")
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
		SizeMode:            req.SizeMode,
		AspectRatio:         req.AspectRatio,
		RequestedSize:       req.RequestedSize,
		BaseResolution:      req.BaseResolution,
		Quality:             req.Quality,
		OutputFormat:        req.OutputFormat,
		Background:          req.Background,
		OutputCompression:   req.OutputCompression,
		Moderation:          req.Moderation,
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
		RouteModelCode:      req.RouteModelCode,
		TaskType:            req.TaskType,
		Prompt:              req.Prompt,
		SizeMode:            req.SizeMode,
		AspectRatio:         req.AspectRatio,
		RequestedSize:       req.RequestedSize,
		BaseResolution:      req.BaseResolution,
		Quality:             req.Quality,
		OutputFormat:        req.OutputFormat,
		Background:          req.Background,
		OutputCompression:   req.OutputCompression,
		Moderation:          req.Moderation,
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

	requestedBaseResolution, requestedAspectRatio, requestedSize, snapshotErr := generationResolveFieldsFromTask(task)
	if snapshotErr != nil {
		return s.failOwnedTask(ctx, task, owner, snapshotErr)
	}
	trustedResolvedSize, snapshotErr := validateImmutableGenerationSnapshot(task)
	if snapshotErr != nil {
		return s.failOwnedTask(ctx, task, owner, snapshotErr)
	}
	resolved, err := s.resolver.ResolveContext(ctx, modelhub.ResolveRequest{
		AbstractModel: task.AbstractModel, RouteModelCode: task.RouteModelCode, TaskType: task.TaskType,
		SizeMode: task.SizeMode, AspectRatio: requestedAspectRatio, BaseResolution: requestedBaseResolution,
		Quality: task.Quality, OutputFormat: task.OutputFormat, Background: task.Background,
		OutputCompression: task.OutputCompression, Moderation: task.Moderation, RequestedSize: requestedSize,
		RequestedOutputImageCount: task.OutputImageCount, ReferenceImageCount: len(referenceImages),
		RouteKey: task.ID, TrustedResolvedSize: trustedResolvedSize,
	})
	if err != nil {
		return s.failOwnedTask(ctx, task, owner, err)
	}
	if snapshotErr := validateResolvedSizeSnapshot(task, resolved); snapshotErr != nil {
		return s.failOwnedTask(ctx, task, owner, snapshotErr)
	}

	task.LeaseOwner = owner
	return s.executeResolvedTask(ctx, task, owner, resolved, executionOptions{
		prompt:             task.Prompt,
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
	sizeMode := testModelSizeMode(req.SizeMode, candidate.SizeModes)
	requestedBaseResolution := strings.TrimSpace(req.BaseResolution)
	requestedAspectRatio := strings.TrimSpace(req.AspectRatio)
	requestedSize := strings.TrimSpace(req.RequestedSize)
	switch sizeMode {
	case modelhub.SizeModeRatio:
		requestedBaseResolution = defaultString(requestedBaseResolution, firstConfiguredValue(candidate.SupportedBaseResolution, "1k"))
		requestedAspectRatio = defaultString(requestedAspectRatio, firstConfiguredValue(candidate.SupportedAspectRatios, "1:1"))
	case modelhub.SizeModePixel:
		requestedSize = defaultString(requestedSize, firstConfiguredValue(candidate.SupportedPixelSizes, "1024x1024"))
	}
	quality := defaultString(req.Quality, firstConfiguredValue(candidate.Quality, "auto"))
	outputFormatExplicit := strings.TrimSpace(req.OutputFormat) != ""
	outputFormat := defaultString(req.OutputFormat, firstConfiguredValue(candidate.OutputFormat, "png"))
	if !outputFormatExplicit && strings.EqualFold(strings.TrimSpace(req.Background), "transparent") && !strings.EqualFold(outputFormat, "png") && !strings.EqualFold(outputFormat, "webp") {
		outputFormat = firstConfiguredValueMatching(candidate.OutputFormat, []string{"png", "webp"}, outputFormat)
	}
	moderation := defaultString(req.Moderation, firstConfiguredValue(candidate.Moderation, "auto"))
	normalized, normalizeErr := modelhub.NormalizeResolveRequest(modelhub.ResolveRequest{
		TaskType:                  string(provider.TaskTypeTextToImage),
		SizeMode:                  sizeMode,
		AspectRatio:               requestedAspectRatio,
		BaseResolution:            requestedBaseResolution,
		Quality:                   quality,
		OutputFormat:              outputFormat,
		Background:                req.Background,
		OutputCompression:         req.OutputCompression,
		Moderation:                moderation,
		RequestedSize:             requestedSize,
		RequestedOutputImageCount: 1,
	})
	if normalizeErr != nil {
		return domainimagetask.TestModelAccountResult{}, normalizeErr
	}
	resolvedBaseResolution := strings.ToLower(strings.TrimSpace(normalized.BaseResolution))
	if modelhub.PublicSizeMode(normalized.SizeMode) == modelhub.SizeModePixel {
		baseResolution, baseResolutionErr := modelhub.BaseResolutionByPixelSize(normalized.RequestedSize)
		if baseResolutionErr != nil {
			return domainimagetask.TestModelAccountResult{}, baseResolutionErr
		}
		resolvedBaseResolution = baseResolution
	} else if modelhub.PublicSizeMode(normalized.SizeMode) != modelhub.SizeModeAuto && (resolvedBaseResolution == "" || resolvedBaseResolution == "auto") {
		resolvedBaseResolution = "1k"
	}
	if !modelhub.CandidateSupportsRequest(candidate, normalized, resolvedBaseResolution) {
		return domainimagetask.TestModelAccountResult{}, errs.New(409, errs.CodeImageCapabilityMismatch, "当前配置暂不支持生成，请更换类似配置。")
	}
	normalizedGeneration, generationErr := modelhub.NormalizeCandidateGenerationRequest(candidate, normalized)
	if generationErr != nil {
		return domainimagetask.TestModelAccountResult{}, generationErr
	}
	taskID := uuid.NewString()
	task := domainimagetask.Task{
		UserID:            0,
		ID:                taskID,
		Status:            domainimagetask.StatusRunning,
		SourceChannel:     "admin_test",
		AbstractModel:     candidate.ModelCode,
		TaskType:          string(provider.TaskTypeTextToImage),
		Prompt:            prompt,
		SizeMode:          modelhub.PublicSizeMode(normalized.SizeMode),
		RequestedSize:     normalizedGeneration.OutboundSize,
		ResolvedWidth:     normalizedGeneration.Width,
		ResolvedHeight:    normalizedGeneration.Height,
		BaseResolution:    resolvedBaseResolution,
		Quality:           normalized.Quality,
		OutputFormat:      normalized.OutputFormat,
		Background:        normalized.Background,
		OutputCompression: defaultPositive(req.OutputCompression, 100),
		Moderation:        normalized.Moderation,
		AspectRatio:       normalized.AspectRatio,
		ResponseMode:      "sync",
		SavePolicy:        "private",
		OutputImageCount:  1,
		AccountModelID:    candidate.AccountModelID,
		ModelAccountID:    candidate.ModelAccountID,
		UpstreamModelCode: candidate.ModelCode,
		EstimatedPoints:   s.zeroPoints(),
		ChargedPoints:     s.zeroPoints(),
		ActualPoints:      s.zeroPoints(),
	}
	resolved := modelhub.ResolvedRequest{
		BaseResolution: resolvedBaseResolution,
		ResolvedSize:   normalizedGeneration.OutboundSize,
		Providers:      []modelhub.ProviderCandidate{candidate},
	}
	providerReq := provider.ImageRequest{
		Model:             candidate.ModelCode,
		TaskType:          provider.TaskTypeTextToImage,
		Prompt:            prompt,
		Size:              task.RequestedSize,
		Quality:           task.Quality,
		OutputFormat:      defaultString(task.OutputFormat, "png"),
		Background:        task.Background,
		OutputCompression: defaultPositive(task.OutputCompression, 100),
		Moderation:        defaultString(task.Moderation, "auto"),
		OutputImageCount:  1,
		ResponseFormat:    provider.ResponseFormatB64JSON,
	}
	applyOutputCompressionCapability(&providerReq, candidate)
	if sizeErr := applyProviderImageSize(&providerReq, task, resolved); sizeErr != nil {
		return domainimagetask.TestModelAccountResult{}, sizeErr
	}
	if compatErr := applyProviderRequestCompatibility(&providerReq, task, candidate, resolved); compatErr != nil {
		return domainimagetask.TestModelAccountResult{}, compatErr
	}

	startedAt := s.nowUTC()
	resp, err := s.executeProviderRequest(ctx, providerClient, candidate, task, providerReq)
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
	projectedTask := cloneTask(task)
	for index, image := range projectedTask.Results {
		projected, projectErr := s.projectImageResultMedia(ctx, image, "/api/ops/admin/v1/image-reviews/"+url.PathEscape(image.ID)+"/image")
		if projectErr != nil {
			return domainimagetask.TestModelAccountResult{}, projectErr
		}
		projectedTask.Results[index] = projected
	}
	result := domainimagetask.TestModelAccountResult{
		Status:            domainimagetask.StatusSucceeded,
		ProviderRequestID: resp.ProviderRequestID,
		ActualParams: map[string]string{
			"model":              providerReq.Model,
			"size":               providerReq.Size,
			"quality":            providerReq.Quality,
			"output_format":      providerReq.OutputFormat,
			"output_compression": strconv.Itoa(providerReq.OutputCompression),
			"moderation":         providerReq.Moderation,
		},
		ElapsedMS: finishedAt.Sub(startedAt).Milliseconds(),
		Task:      projectedTask,
	}
	if len(projectedTask.Results) > 0 {
		image := projectedTask.Results[0]
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
	attemptedCandidates := 0
	for _, candidate := range orderedProviders {
		fallbackCount := attemptedCandidates
		attemptedCandidates++
		providerClient, ok := s.providerClientForCandidate(candidate)
		if !ok {
			continue
		}
		providerReq := provider.ImageRequest{
			Model:             s.providerModelName(task.AbstractModel, candidate.Provider, candidate.ModelCode),
			TaskType:          provider.TaskType(task.TaskType),
			Prompt:            opts.prompt,
			Quality:           defaultString(task.Quality, "auto"),
			OutputFormat:      defaultString(task.OutputFormat, "png"),
			Background:        task.Background,
			OutputCompression: defaultPositive(task.OutputCompression, 100),
			Moderation:        defaultString(task.Moderation, "auto"),
			OutputImageCount:  normalizedCount(task.OutputImageCount),
			ResponseFormat:    normalizeResponseFormat(opts.responseFormat),
			ReferenceImages:   append([]provider.ImageInput(nil), opts.referenceImages...),
			Mask:              opts.mask,
			User:              opts.user,
		}
		applyOutputCompressionCapability(&providerReq, candidate)
		if sizeErr := applyProviderImageSize(&providerReq, task, resolved); sizeErr != nil {
			return s.failOwnedTask(ctx, task, owner, sizeErr)
		}
		if compatErr := applyProviderRequestCompatibility(&providerReq, task, candidate, resolved); compatErr != nil {
			return s.failOwnedTask(ctx, task, owner, compatErr)
		}

		attemptStarted := s.nowUTC()
		openAIFormat := strings.EqualFold(candidate.Provider, string(provider.ProviderTypeOpenAI)) || strings.EqualFold(candidate.AdapterType, "openai_compatible")
		if openAIFormat && normalizedCount(task.OutputImageCount) > 1 {
			progress, progressErr := s.executeOpenAIFanout(ctx, providerClient, candidate, task, providerReq)
			attemptFinished := s.nowUTC()
			if progressErr != nil {
				task = s.decorateTaskProvider(task, candidate)
				if len(progress.Attempts) > 0 {
					task.Attempts = append(task.Attempts, progress.Attempts...)
				} else {
					task.Attempts = append(task.Attempts, buildProviderAttempt(candidate, domainimagetask.StatusFailed, progressErr, attemptStarted, attemptFinished))
				}
				task.FallbackCount = fallbackCount
				lastErr = progressErr
				shouldRetry := false
				if upstream, ok := provider.AsUpstreamError(progressErr); ok && upstream.Action == provider.UpstreamErrorActionRetry {
					shouldRetry = true
				}
				if saveErr := s.saveOwnedTask(ctx, task, owner); saveErr != nil {
					if shouldRetry {
						return domainimagetask.ExecuteResult{}, saveErr
					}
					return s.failOwnedTask(ctx, task, owner, progressErr)
				}
				if shouldRetry {
					continue
				}
				break
			}
			resp := progress.Response
			resp.Data = append([]provider.ImageResult(nil), progress.Results...)
			task = s.decorateTaskProvider(task, candidate)
			if len(progress.Failures) > 0 {
				task.ErrorCode = errorCode(progress.Failures[0])
				task.ErrorMessage = errorMessage(progress.Failures[0])
			}
			if checkpointErr := s.checkpointProviderSuccess(ctx, &task, owner, candidate, resp, providerReq.Size, progress.Attempts, attemptStarted, attemptFinished); checkpointErr != nil {
				return s.handleProviderSuccessCheckpointFailure(ctx, task, owner, checkpointErr)
			}
			persistedResults, persistErr := s.persistImageResults(ctx, task, resp.Data)
			if persistErr != nil {
				return s.handleArtifactPersistenceFailure(ctx, task, owner, persistErr)
			}
			reconcileAttemptDimensions(&task, persistedResults)
			finalStatus := domainimagetask.StatusSucceeded
			if len(persistedResults) < normalizedCount(task.OutputImageCount) {
				finalStatus = domainimagetask.StatusPartialFailed
				if task.ErrorCode == "" {
					task.ErrorCode = errs.CodeUpstreamUnavailable
					task.ErrorMessage = "部分上游批次未返回有效图片"
				}
			}
			task.Status = domainimagetask.StatusRunning
			task.FallbackCount = fallbackCount
			task.Results = persistedResults
			task.ArtifactRecovery = completedArtifactRecovery(task.ArtifactRecovery)
			if billingErr := s.applyActualPoints(&task, len(persistedResults)); billingErr != nil {
				return s.failOwnedTask(ctx, task, owner, billingErr)
			}
			task.ProviderCost = calculateProviderCost(candidate, len(persistedResults))
			task.GrossMargin = calculateGrossMargin(task.ActualPoints, task.ProviderCost)
			setTaskProgress(&task, domainimagetask.ProgressStageSettling, "结果已保存，正在结算积分")
			if saveErr := s.saveOwnedTask(ctx, task, owner); saveErr != nil {
				recoveredTask, recovered, recoverErr := s.recoverTerminalLeaseConflict(ctx, task, owner, finalStatus, "task completed after lease conflict")
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
			setCompletedTaskProgress(&task)
			task.LeaseOwner = ""
			task.LeaseExpiresAt = nil
			if saveErr := s.saveOwnedTask(ctx, task, owner); saveErr != nil {
				return domainimagetask.ExecuteResult{}, saveErr
			}
			return domainimagetask.ExecuteResult{Task: task, Response: resp}, nil
		}

		resp, err := s.executeProviderRequest(ctx, providerClient, candidate, task, providerReq)
		attemptFinished := s.nowUTC()
		if err == nil {
			if checkpointErr := s.checkpointProviderSuccess(ctx, &task, owner, candidate, resp, providerReq.Size, nil, attemptStarted, attemptFinished); checkpointErr != nil {
				return s.handleProviderSuccessCheckpointFailure(ctx, task, owner, checkpointErr)
			}
			persistedResults, persistErr := s.persistImageResults(ctx, task, resp.Data)
			if persistErr != nil {
				return s.handleArtifactPersistenceFailure(ctx, task, owner, persistErr)
			}
			reconcileAttemptDimensions(&task, persistedResults)
			finalStatus := domainimagetask.StatusSucceeded
			if len(persistedResults) < normalizedCount(task.OutputImageCount) {
				finalStatus = domainimagetask.StatusPartialFailed
				task.ErrorCode = errs.CodeUpstreamUnavailable
				task.ErrorMessage = "部分上游批次未返回有效图片"
			}
			task.Status = domainimagetask.StatusRunning
			task.FallbackCount = fallbackCount
			task.Results = append([]provider.ImageResult(nil), persistedResults...)
			task.ArtifactRecovery = completedArtifactRecovery(task.ArtifactRecovery)
			if billingErr := s.applyActualPoints(&task, len(persistedResults)); billingErr != nil {
				return s.failOwnedTask(ctx, task, owner, billingErr)
			}
			task.ProviderCost = calculateProviderCost(candidate, len(persistedResults))
			task.GrossMargin = calculateGrossMargin(task.ActualPoints, task.ProviderCost)
			setTaskProgress(&task, domainimagetask.ProgressStageSettling, "结果已保存，正在结算积分")
			if saveErr := s.saveOwnedTask(ctx, task, owner); saveErr != nil {
				recoveredTask, recovered, recoverErr := s.recoverTerminalLeaseConflict(ctx, task, owner, finalStatus, "task completed after lease conflict")
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
			setCompletedTaskProgress(&task)
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
		task.FallbackCount = fallbackCount
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

func (s *Service) executeOpenAIFanout(ctx context.Context, client provider.ImageProvider, candidate modelhub.ProviderCandidate, task domainimagetask.Task, req provider.ImageRequest) (openAIFanoutProgress, error) {
	userConcurrencyLimit, err := s.userConcurrencyLimit(ctx, task.UserID)
	if err != nil {
		return openAIFanoutProgress{}, err
	}
	call := func(ctx context.Context, singleReq provider.ImageRequest) (provider.ImageResponse, error) {
		release, err := s.acquireProviderConcurrency(ctx, task.UserID, userConcurrencyLimit, candidate)
		if err != nil {
			return provider.ImageResponse{}, err
		}
		defer release()
		if task.TaskType == string(provider.TaskTypeImageEdit) {
			return client.Edit(ctx, singleReq)
		}
		return client.Generate(ctx, singleReq)
	}

	count := normalizedCount(req.OutputImageCount)
	chunks := splitOutputImageCount(count, candidate.MaxImageCount)
	progress := openAIFanoutProgress{
		Results:  make([]provider.ImageResult, 0, count),
		Failures: make([]error, 0),
		Attempts: make([]domainimagetask.Attempt, 0, len(chunks)),
	}
	var mu sync.Mutex
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(fanoutConcurrencyLimit(candidate, userConcurrencyLimit, len(chunks)))
	for i, chunkCount := range chunks {
		fanoutIndex := i + 1
		chunkCount := chunkCount
		group.Go(func() error {
			singleReq := req
			singleReq.OutputImageCount = chunkCount
			attrs := fanoutLogAttrs("openai_progress", task, candidate, singleReq, len(chunks), fanoutIndex)
			slog.Info("image fanout request started", attrs...)
			startedAt := s.nowUTC()
			resp, err := call(groupCtx, singleReq)
			finishedAt := s.nowUTC()
			durationMs := finishedAt.Sub(startedAt).Milliseconds()
			if err != nil {
				slog.Warn("image fanout request failed", append(attrs,
					"duration_ms", durationMs,
					"error", err.Error(),
					"error_code", errorCode(err),
					"context_error", contextErrorString(groupCtx),
				)...)
			} else {
				slog.Info("image fanout request finished", append(attrs,
					"duration_ms", durationMs,
					"image_count", len(resp.Data),
					"provider_request_id", resp.ProviderRequestID,
					"context_error", contextErrorString(groupCtx),
				)...)
			}
			mu.Lock()
			defer mu.Unlock()
			attemptErr := err
			if attemptErr == nil && len(resp.Data) == 0 {
				attemptErr = errs.New(502, errs.CodeUpstreamUnavailable, "provider returned no images")
			}
			status := domainimagetask.StatusSucceeded
			if attemptErr != nil {
				status = domainimagetask.StatusFailed
			}
			attempt := buildProviderAttempt(candidate, status, attemptErr, startedAt, finishedAt)
			attempt.SourceSizeMode = task.SizeMode
			attempt.OutboundSize = singleReq.Size
			if requestID := strings.TrimSpace(resp.ProviderRequestID); requestID != "" {
				attempt.ProviderRequestID = requestID
			}
			attempt.RequestedImageCount = chunkCount
			attempt.ReturnedImageCount = len(resp.Data)
			if len(resp.Data) > 0 {
				attempt.ReturnedWidth, attempt.ReturnedHeight = resp.Data[0].Width, resp.Data[0].Height
			}
			attempt.SizeDiagnostic = classifyImageSize(singleReq.Size, attempt.ReturnedWidth, attempt.ReturnedHeight)
			progress.Attempts = append(progress.Attempts, attempt)
			if progress.Response.Created == 0 {
				progress.Response.Created = resp.Created
			}
			if progress.Response.ProviderRequestID == "" {
				progress.Response.ProviderRequestID = resp.ProviderRequestID
			}
			if attemptErr != nil {
				progress.Failures = append(progress.Failures, attemptErr)
				return nil
			}
			if len(resp.Data) == 0 {
				slog.Warn("image fanout request returned no images", append(attrs,
					"duration_ms", durationMs,
					"context_error", contextErrorString(groupCtx),
				)...)
				return nil
			}
			progress.Results = append(progress.Results, resp.Data...)
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return openAIFanoutProgress{}, err
	}
	if len(progress.Results) == 0 {
		if len(progress.Failures) > 0 {
			return progress, progress.Failures[0]
		}
		return progress, errs.New(502, errs.CodeUpstreamUnavailable, "provider returned no images")
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
		req.Quality = defaultString(task.Quality, req.Quality)
		return nil
	}

	req.Quality = "auto"
	req.ResponseFormat = provider.ResponseFormatB64JSON
	if modelhub.PublicSizeMode(task.SizeMode) == modelhub.SizeModeAuto {
		req.Size = ""
		return nil
	}
	if width, height, ok := modelhub.ParseImageSize(req.Size); ok && width > 0 && height > 0 {
		return nil
	}
	size, err := modelhub.CalculateImageSize(defaultString(resolved.BaseResolution, task.BaseResolution), defaultString(task.AspectRatio, "1:1"))
	if err != nil {
		return errs.New(400, errs.CodeImageCapabilityMismatch, "unsupported gpt-image-2 size parameters")
	}
	req.Size = size
	return nil
}

func applyOutputCompressionCapability(req *provider.ImageRequest, candidate modelhub.ProviderCandidate) {
	if req == nil {
		return
	}
	compressibleFormat := strings.EqualFold(req.OutputFormat, "jpeg") || strings.EqualFold(req.OutputFormat, "webp")
	if !candidate.SupportsOutputCompression || !compressibleFormat {
		req.OutputCompression = 0
	}
}

func testModelSizeMode(requested string, configured []string) string {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested != "" {
		return requested
	}
	for _, mode := range configured {
		normalized := strings.ToLower(strings.TrimSpace(mode))
		if normalized == modelhub.SizeModeAuto || normalized == modelhub.SizeModeRatio || normalized == modelhub.SizeModePixel {
			return normalized
		}
	}
	return modelhub.SizeModeRatio
}

func firstConfiguredValue(values []string, fallback string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return fallback
}

func firstConfiguredValueMatching(values, allowed []string, fallback string) string {
	for _, value := range values {
		for _, candidate := range allowed {
			if strings.EqualFold(strings.TrimSpace(value), candidate) {
				return value
			}
		}
	}
	return fallback
}

func applyProviderImageSize(req *provider.ImageRequest, task domainimagetask.Task, resolved modelhub.ResolvedRequest) error {
	if req == nil {
		return nil
	}
	switch modelhub.PublicSizeMode(task.SizeMode) {
	case modelhub.SizeModeAuto:
		req.Size = ""
		return nil
	case modelhub.SizeModePixel:
		size := modelhub.NormalizePixelSize(task.RequestedSize)
		if size == "" {
			return errs.New(400, errs.CodeImageAutoUnsupported, "unsupported image size")
		}
		req.Size = size
		return nil
	default:
		if size := modelhub.NormalizePixelSize(task.RequestedSize); size != "" {
			req.Size = size
			return nil
		}
		size, err := modelhub.CalculateImageSize(defaultString(resolved.BaseResolution, task.BaseResolution), defaultString(task.AspectRatio, "1:1"))
		if err != nil {
			return errs.New(400, errs.CodeImageCapabilityMismatch, "unsupported image size")
		}
		req.Size = size
		return nil
	}
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
		attempt.ProviderRequestID = strings.TrimSpace(upstream.RequestID)
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

func classifyImageSize(outbound string, actualWidth, actualHeight int) string {
	if actualWidth <= 0 || actualHeight <= 0 {
		return "decode_failed"
	}
	if strings.TrimSpace(outbound) == "" {
		return "missing_outbound_size"
	}
	width, height, ok := modelhub.ParseImageSize(outbound)
	if !ok {
		return "local_contract_violation"
	}
	if width == actualWidth && height == actualHeight {
		return "match"
	}
	return "upstream_rewritten"
}

func reconcileAttemptDimensions(task *domainimagetask.Task, results []provider.ImageResult) {
	if task == nil || len(results) == 0 {
		return
	}
	resultIndex := 0
	for index := range task.Attempts {
		attempt := &task.Attempts[index]
		if attempt.Status != domainimagetask.StatusSucceeded || attempt.ReturnedImageCount <= 0 {
			continue
		}
		if resultIndex >= len(results) {
			return
		}
		result := results[resultIndex]
		attempt.ReturnedWidth = result.Width
		attempt.ReturnedHeight = result.Height
		attempt.SizeDiagnostic = classifyImageSize(attempt.OutboundSize, result.Width, result.Height)
		resultIndex += attempt.ReturnedImageCount
	}
}

func (s *Service) executeProviderRequest(ctx context.Context, client provider.ImageProvider, candidate modelhub.ProviderCandidate, task domainimagetask.Task, req provider.ImageRequest) (provider.ImageResponse, error) {
	userConcurrencyLimit, err := s.userConcurrencyLimit(ctx, task.UserID)
	if err != nil {
		return provider.ImageResponse{}, err
	}
	call := func(ctx context.Context, singleReq provider.ImageRequest) (provider.ImageResponse, error) {
		release, err := s.acquireProviderConcurrency(ctx, task.UserID, userConcurrencyLimit, candidate)
		if err != nil {
			return provider.ImageResponse{}, err
		}
		defer release()
		if task.TaskType == string(provider.TaskTypeImageEdit) {
			return client.Edit(ctx, singleReq)
		}
		return client.Generate(ctx, singleReq)
	}

	count := normalizedCount(req.OutputImageCount)
	chunks := splitOutputImageCount(count, candidate.MaxImageCount)
	if len(chunks) == 1 {
		return call(ctx, req)
	}

	results := make([]provider.ImageResult, 0, count)
	var (
		mu        sync.Mutex
		firstResp provider.ImageResponse
		firstErr  error
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(fanoutConcurrencyLimit(candidate, userConcurrencyLimit, len(chunks)))
	for i, chunkCount := range chunks {
		fanoutIndex := i + 1
		chunkCount := chunkCount
		group.Go(func() error {
			singleReq := req
			singleReq.OutputImageCount = chunkCount
			attrs := fanoutLogAttrs("provider_aggregate", task, candidate, singleReq, len(chunks), fanoutIndex)
			slog.Info("image fanout request started", attrs...)
			startedAt := time.Now()
			resp, err := call(groupCtx, singleReq)
			durationMs := time.Since(startedAt).Milliseconds()
			if err != nil {
				slog.Warn("image fanout request failed", append(attrs,
					"duration_ms", durationMs,
					"error", err.Error(),
					"error_code", errorCode(err),
					"context_error", contextErrorString(groupCtx),
				)...)
			} else {
				slog.Info("image fanout request finished", append(attrs,
					"duration_ms", durationMs,
					"image_count", len(resp.Data),
					"provider_request_id", resp.ProviderRequestID,
					"context_error", contextErrorString(groupCtx),
				)...)
			}
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
			return openaiprovider.NewClient(openaiprovider.Config{BaseURL: candidate.BaseURL, APIKey: apiKey, HTTPClient: s.providerHTTPClient(candidate)}), true
		case "openrouter":
			return openrouterprovider.NewClient(openrouterprovider.Config{BaseURL: candidate.BaseURL, APIKey: apiKey, HTTPClient: s.providerHTTPClient(candidate)}), true
		default:
			return nil, false
		}
	}
	providerClient, ok := s.providers[candidate.Provider]
	return providerClient, ok
}

func (s *Service) providerHTTPClient(candidate modelhub.ProviderCandidate) *http.Client {
	if candidate.TimeoutMS <= 0 {
		return s.httpClient
	}
	base := s.httpClient
	if base == nil {
		base = http.DefaultClient
	}
	clone := *base
	clone.Timeout = time.Duration(candidate.TimeoutMS) * time.Millisecond
	return &clone
}

func (s *Service) failOwnedTask(ctx context.Context, task domainimagetask.Task, owner string, failure error) (domainimagetask.ExecuteResult, error) {
	task.Status = domainimagetask.StatusRunning
	task.ActualPoints = s.zeroPoints()
	task.ErrorCode = errorCode(failure)
	task.ErrorMessage = errorMessage(failure)
	setTaskProgress(&task, domainimagetask.ProgressStageSettling, "生成失败，正在释放预留积分")
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
	setTaskProgress(&task, domainimagetask.ProgressStageFailed, defaultString(task.ErrorMessage, "任务生成失败"))
	task.LeaseOwner = ""
	task.LeaseExpiresAt = nil
	if saveErr := s.saveOwnedTask(ctx, task, owner); saveErr != nil {
		return domainimagetask.ExecuteResult{}, saveErr
	}
	return domainimagetask.ExecuteResult{Task: task}, failure
}

func (s *Service) resumeTerminalization(ctx context.Context, task domainimagetask.Task, owner string) (domainimagetask.ExecuteResult, bool, error) {
	switch task.Status {
	case domainimagetask.StatusSucceeded, domainimagetask.StatusPartialFailed:
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
		setTaskProgress(&task, domainimagetask.ProgressStageSettling, "结果已保存，正在结算积分")
		if settleErr := s.settleTaskBilling(ctx, task, "resume settled image task"); settleErr != nil {
			return domainimagetask.ExecuteResult{}, true, settleErr
		}
		task.Status = completedStatusForResults(task)
		setCompletedTaskProgress(&task)
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

	setTaskProgress(&task, domainimagetask.ProgressStageSettling, "生成失败，正在释放预留积分")
	if settleErr := s.settleTaskBilling(ctx, task, "resume failed image task"); settleErr != nil {
		return domainimagetask.ExecuteResult{}, true, settleErr
	}
	task.Status = domainimagetask.StatusFailed
	setTaskProgress(&task, domainimagetask.ProgressStageFailed, defaultString(task.ErrorMessage, "任务生成失败"))
	task.LeaseOwner = ""
	task.LeaseExpiresAt = nil
	if saveErr := s.saveOwnedTask(ctx, task, owner); saveErr != nil {
		return domainimagetask.ExecuteResult{}, true, saveErr
	}
	return domainimagetask.ExecuteResult{Task: task}, true, errs.New(500, defaultString(task.ErrorCode, errs.CodeInternal), defaultString(task.ErrorMessage, "image task failed"))
}

func (s *Service) resolveTask(ctx context.Context, routeKey, abstractModel, routeModelCode string, userGroupCodes []string, taskType, sizeMode, aspectRatio, baseResolution, quality, outputFormat, background string, outputCompression int, moderation, requestedSize string, outputImageCount, referenceImageCount int, maskPresent bool, capabilityVersion string) (modelhub.ResolvedRequest, error) {
	return s.resolver.ResolveContext(ctx, modelhub.ResolveRequest{
		AbstractModel:             abstractModel,
		RouteModelCode:            routeModelCode,
		TaskType:                  taskType,
		SizeMode:                  sizeMode,
		AspectRatio:               aspectRatio,
		BaseResolution:            baseResolution,
		Quality:                   quality,
		OutputFormat:              outputFormat,
		Background:                background,
		OutputCompression:         outputCompression,
		Moderation:                moderation,
		RequestedSize:             requestedSize,
		RequestedOutputImageCount: outputImageCount,
		ReferenceImageCount:       referenceImageCount,
		MaskPresent:               maskPresent,
		RouteKey:                  routeKey,
		UserGroupCodes:            append([]string(nil), userGroupCodes...),
		ExpectedCapabilityVersion: capabilityVersion,
	})
}

func (s *Service) GetByID(ctx context.Context, userID int64, taskID string) (domainimagetask.Task, error) {
	task, err := s.loadOwnedTask(ctx, userID, taskID)
	if err != nil {
		return domainimagetask.Task{}, err
	}
	projected, projectErr := s.projectTaskMedia(ctx, cloneTask(task), "/api/agent/image/v1/images/")
	if projectErr != nil {
		return domainimagetask.Task{}, projectErr
	}
	return projected, nil
}

func (s *Service) loadOwnedTask(ctx context.Context, userID int64, taskID string) (domainimagetask.Task, error) {
	task, err := s.store.GetByID(ctx, userID, taskID)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return domainimagetask.Task{}, errs.New(404, errs.CodeNotFound, "image task not found")
		}
		return domainimagetask.Task{}, errs.Internal("failed to load image task")
	}
	return task, nil
}

type ImageResultDelivery struct {
	Result       provider.ImageResult
	Content      []byte
	TemporaryURL string
}

func (s *Service) GetOwnedImageResult(ctx context.Context, userID int64, imageID string) (provider.ImageResult, error) {
	result, err := s.store.GetImageResultByID(ctx, userID, imageID)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return provider.ImageResult{}, errs.New(404, errs.CodeNotFound, "image not found")
		}
		return provider.ImageResult{}, errs.Internal("failed to load image result")
	}
	return result, nil
}

func (s *Service) DeliverImageResult(ctx context.Context, userID int64, imageID string) (ImageResultDelivery, error) {
	result, err := s.store.GetImageResultByID(ctx, userID, imageID)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return ImageResultDelivery{}, errs.New(404, errs.CodeNotFound, "image not found")
		}
		return ImageResultDelivery{}, errs.Internal("failed to load image result")
	}
	if result.StorageDriver == "remote" || strings.TrimSpace(result.ObjectKey) == "" {
		return ImageResultDelivery{}, errs.New(404, errs.CodeNotFound, "image not found")
	}
	backend, routeErr := s.router.BackendFor(ctx, result.StorageConfigID, result.StorageDriver)
	if routeErr != nil {
		return ImageResultDelivery{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "storage config is unavailable")
	}
	if urls, supported, signErr := storage.ProjectTemporaryMediaURLs(ctx, backend.Backend, result.ObjectKey, result.MimeType, imageResultDeliveryFilename(result)); supported {
		if signErr != nil {
			return ImageResultDelivery{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "storage config is unavailable")
		}
		return ImageResultDelivery{Result: result, TemporaryURL: urls.DownloadURL}, nil
	}
	content, readErr := backend.Backend.Get(ctx, result.ObjectKey)
	if readErr != nil {
		return ImageResultDelivery{}, errs.New(404, errs.CodeNotFound, "image not found")
	}
	return ImageResultDelivery{Result: result, Content: content}, nil
}

func imageResultDeliveryFilename(result provider.ImageResult) string {
	name := strings.TrimSpace(result.ID)
	if name == "" {
		name = "image"
	}
	if filepath.Ext(name) != "" {
		return name
	}
	extension := filepath.Ext(strings.TrimSpace(result.ObjectKey))
	if extension == "" {
		switch strings.ToLower(strings.TrimSpace(result.MimeType)) {
		case "image/jpeg", "image/jpg":
			extension = ".jpg"
		case "image/webp":
			extension = ".webp"
		case "image/gif":
			extension = ".gif"
		default:
			extension = ".png"
		}
	}
	return name + extension
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
	result, err := s.GetImageResultForAdmin(ctx, imageID)
	if err != nil {
		return provider.ImageResult{}, nil, err
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

func (s *Service) GetImageResultForAdmin(ctx context.Context, imageID string) (provider.ImageResult, error) {
	result, err := s.store.GetImageResultForAdmin(ctx, imageID)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return provider.ImageResult{}, errs.New(404, errs.CodeNotFound, "image not found")
		}
		return provider.ImageResult{}, errs.Internal("failed to load image result")
	}
	return result, nil
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
		StorageConfigID:  image.StorageConfigID,
		ObjectKey:        image.ObjectKey,
		StorageDriver:    image.StorageDriver,
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
	projected := make([]domainimagetask.Task, 0, len(list))
	for _, task := range list {
		item, projectErr := s.projectTaskMedia(ctx, cloneTask(task), "/api/agent/image/v1/images/")
		if projectErr != nil {
			return nil, projectErr
		}
		projected = append(projected, item)
	}
	return projected, nil
}

func (s *Service) projectTaskMedia(ctx context.Context, task domainimagetask.Task, fallbackPrefix string) (domainimagetask.Task, error) {
	for index, result := range task.Results {
		fallbackURL := strings.TrimRight(fallbackPrefix, "/") + "/" + url.PathEscape(strings.TrimSpace(result.ID))
		projected, err := s.projectImageResultMedia(ctx, result, fallbackURL)
		if err != nil {
			return domainimagetask.Task{}, err
		}
		task.Results[index] = projected
	}
	return task, nil
}

func (s *Service) projectImageResultMedia(ctx context.Context, result provider.ImageResult, fallbackURL string) (provider.ImageResult, error) {
	if strings.EqualFold(strings.TrimSpace(result.StorageDriver), "remote") || strings.TrimSpace(result.ObjectKey) == "" {
		if remoteURL := absoluteHTTPMediaURL(defaultString(result.URL, result.ObjectKey)); remoteURL != "" {
			result.URL, result.DownloadURL = remoteURL, remoteURL
			return result, nil
		}
		result.URL, result.DownloadURL = fallbackURL, fallbackURL
		return result, nil
	}
	backend, err := s.router.BackendFor(ctx, result.StorageConfigID, result.StorageDriver)
	if err != nil {
		return provider.ImageResult{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "storage config is unavailable")
	}
	urls, supported, err := storage.ProjectTemporaryMediaURLs(ctx, backend.Backend, result.ObjectKey, result.MimeType, imageResultDeliveryFilename(result))
	if err != nil {
		return provider.ImageResult{}, errs.New(500, "STORAGE_CONFIG_UNAVAILABLE", "storage config is unavailable")
	}
	if supported {
		result.URL, result.DownloadURL = urls.PreviewURL, urls.DownloadURL
		result.PreviewExpiresAt = imageMediaExpiryPointer(urls.PreviewExpiresAt)
		result.DownloadExpiresAt = imageMediaExpiryPointer(urls.DownloadExpiresAt)
		return result, nil
	}
	result.URL, result.DownloadURL = fallbackURL, fallbackURL
	return result, nil
}

func (s *Service) projectGalleryImageMedia(ctx context.Context, image domainimagetask.GalleryImage, fallbackURL string) (domainimagetask.GalleryImage, error) {
	result, err := s.projectImageResultMedia(ctx, provider.ImageResult{
		ID: image.ID, URL: image.URL, DownloadURL: image.DownloadURL, MimeType: image.MimeType,
		StorageConfigID: image.StorageConfigID, ObjectKey: image.ObjectKey, StorageDriver: image.StorageDriver,
	}, fallbackURL)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	image.URL, image.DownloadURL = result.URL, result.DownloadURL
	image.PreviewExpiresAt, image.DownloadExpiresAt = result.PreviewExpiresAt, result.DownloadExpiresAt
	assetLoader, ok := s.assets.(assetMediaURLLoader)
	if !ok {
		return image, nil
	}
	for index, reference := range image.ReferenceAssets {
		asset, loadErr := assetLoader.GetWithContext(ctx, image.UserID, reference.ID)
		if loadErr != nil {
			var appErr *errs.Error
			if errors.As(loadErr, &appErr) && appErr.StatusCode == http.StatusNotFound {
				continue
			}
			return domainimagetask.GalleryImage{}, loadErr
		}
		if strings.TrimSpace(asset.PreviewURL) != "" {
			image.ReferenceAssets[index].PreviewURL = asset.PreviewURL
			image.ReferenceAssets[index].PreviewExpiresAt = asset.PreviewExpiresAt
		}
	}
	return image, nil
}

func imageMediaExpiryPointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	value = value.UTC()
	return &value
}

func (s *Service) projectGalleryPageMedia(ctx context.Context, page domainimagetask.GalleryPage, fallback func(string) string) (domainimagetask.GalleryPage, error) {
	for index, image := range page.Items {
		projected, err := s.projectGalleryImageMedia(ctx, image, fallback(image.ID))
		if err != nil {
			return domainimagetask.GalleryPage{}, err
		}
		page.Items[index] = projected
	}
	return page, nil
}

func absoluteHTTPMediaURL(value string) string {
	target, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" || target.User != nil {
		return ""
	}
	return target.String()
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
	original, err := s.loadOwnedTask(ctx, userID, taskID)
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
		SizeMode:            original.SizeMode,
		RequestedSize:       original.RequestedSize,
		BaseResolution:      original.BaseResolution,
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
	return s.projectGalleryImageMedia(ctx, image, "/api/agent/image/v1/images/"+url.PathEscape(image.ID))
}

func (s *Service) CancelPublish(ctx context.Context, userID int64, imageID string) (domainimagetask.GalleryImage, error) {
	image, err := s.store.CancelPublish(ctx, userID, imageID)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return domainimagetask.GalleryImage{}, errs.New(404, errs.CodeNotFound, "image not found")
		}
		return domainimagetask.GalleryImage{}, errs.Internal("failed to cancel image publish")
	}
	return s.projectGalleryImageMedia(ctx, image, "/api/agent/image/v1/images/"+url.PathEscape(image.ID))
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
	return s.projectGalleryImageMedia(ctx, image, "/api/agent/image/v1/images/"+url.PathEscape(image.ID))
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

func (s *Service) ProjectGalleryImageForAdmin(ctx context.Context, image domainimagetask.GalleryImage) (domainimagetask.GalleryImage, error) {
	return s.projectGalleryImageMedia(ctx, image, "/api/ops/admin/v1/image-reviews/"+url.PathEscape(image.ID)+"/image")
}

func (s *Service) ListGallery(ctx context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	req.Page, req.PageSize = normalizeListPage(req.Page, req.PageSize)
	req.Status = strings.TrimSpace(req.Status)
	req.UserQuery = strings.TrimSpace(req.UserQuery)
	req.PromptQuery = strings.TrimSpace(req.PromptQuery)
	req.ModelQuery = strings.TrimSpace(req.ModelQuery)
	req.TaskType = strings.TrimSpace(req.TaskType)
	req.BaseResolution = strings.TrimSpace(req.BaseResolution)
	req.RequestedSize = strings.TrimSpace(req.RequestedSize)
	req.AspectRatio = strings.TrimSpace(req.AspectRatio)
	page, err := s.store.ListGallery(ctx, req)
	if err != nil {
		return domainimagetask.GalleryPage{}, errs.Internal("failed to list gallery images")
	}
	return s.projectGalleryPageMedia(ctx, page, func(imageID string) string {
		return "/api/ops/admin/v1/image-reviews/" + url.PathEscape(imageID) + "/image"
	})
}

func (s *Service) ListGalleryByUser(ctx context.Context, userID int64, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	req.Page, req.PageSize = normalizeListPage(req.Page, req.PageSize)
	req.Status = strings.TrimSpace(req.Status)
	page, err := s.store.ListGalleryByUser(ctx, userID, req)
	if err != nil {
		return domainimagetask.GalleryPage{}, errs.Internal("failed to list user gallery images")
	}
	return s.projectGalleryPageMedia(ctx, page, func(imageID string) string {
		return "/api/agent/image/v1/images/" + url.PathEscape(imageID)
	})
}

func (s *Service) ListPublicGallery(ctx context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	req.Page, req.PageSize = normalizeListPage(req.Page, req.PageSize)
	req.Sort = strings.TrimSpace(req.Sort)
	page, err := s.store.ListPublicGallery(ctx, req)
	if err != nil {
		return domainimagetask.GalleryPage{}, errs.Internal("failed to list public gallery images")
	}
	return s.projectGalleryPageMedia(ctx, page, func(imageID string) string {
		return "/api/open/image/v1/gallery/images/" + url.PathEscape(imageID) + "/image"
	})
}

func (s *Service) GetPublicImage(ctx context.Context, imageID string, viewerUserID int64) (domainimagetask.GalleryImage, error) {
	image, err := s.store.GetPublicImage(ctx, imageID, viewerUserID)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return domainimagetask.GalleryImage{}, errs.New(404, errs.CodeNotFound, "gallery image not found")
		}
		return domainimagetask.GalleryImage{}, errs.Internal("failed to load public gallery image")
	}
	return s.projectGalleryImageMedia(ctx, image, "/api/open/image/v1/gallery/images/"+url.PathEscape(image.ID)+"/image")
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
	return s.projectGalleryImageMedia(ctx, image, "/api/open/image/v1/gallery/images/"+url.PathEscape(image.ID)+"/image")
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

func (s *Service) updateOwnedProgress(ctx context.Context, task domainimagetask.Task, owner string) error {
	if err := s.store.UpdateProgressIfOwned(ctx, task.ID, owner, task.ProgressStage, task.ProgressMessage, time.Now().UTC()); err != nil {
		switch {
		case errors.Is(err, repoerr.ErrNotFound):
			return errs.New(404, errs.CodeNotFound, "image task not found")
		case errors.Is(err, repoerr.ErrConflict):
			return errs.New(409, errs.CodeConflict, "image task lease conflict")
		default:
			return errs.Internal("failed to persist image task progress")
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
	if snapshotErr != nil {
		if isTerminalRecoveryRace(snapshotErr) {
			return domainimagetask.Task{}, false, nil
		}
		return domainimagetask.Task{}, false, snapshotErr
	}
	if settleErr := s.settleTaskBilling(ctx, task, settleReason); settleErr != nil {
		return domainimagetask.Task{}, false, settleErr
	}

	recoveredTask := cloneTask(task)
	recoveredTask.Status = terminalStatus
	if terminalStatus == domainimagetask.StatusFailed {
		setTaskProgress(&recoveredTask, domainimagetask.ProgressStageFailed, defaultString(recoveredTask.ErrorMessage, "任务生成失败"))
	} else {
		setCompletedTaskProgress(&recoveredTask)
	}
	recoveredTask.LeaseOwner = ""
	recoveredTask.LeaseExpiresAt = nil
	if persistErr := s.saveTerminalState(ctx, recoveredTask, owner); persistErr != nil {
		if isTerminalRecoveryRace(persistErr) {
			return domainimagetask.Task{}, false, nil
		}
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

func isTaskLeaseConflict(err error) bool {
	appErr, ok := err.(*errs.Error)
	return ok && appErr.Code == errs.CodeConflict
}

func terminalTaskResult(task domainimagetask.Task) (domainimagetask.ExecuteResult, bool, error) {
	switch task.Status {
	case domainimagetask.StatusSucceeded, domainimagetask.StatusPartialFailed:
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

func completedStatusForResults(task domainimagetask.Task) string {
	if len(task.Results) > 0 && len(task.Results) < normalizedCount(task.OutputImageCount) {
		return domainimagetask.StatusPartialFailed
	}
	return domainimagetask.StatusSucceeded
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

	estimate, err := s.billing.Estimate(taskEstimateRequest(req, task.OutputImageCount))
	if err != nil {
		return err
	}
	return s.applyTaskEstimateResult(ctx, task, req, estimate)
}

func taskEstimateRequest(req domainimagetask.CreateRequest, outputImageCount int) domainbilling.EstimateRequest {
	return domainbilling.EstimateRequest{
		RouteKey:                  req.TaskID,
		TaskType:                  req.TaskType,
		AbstractModel:             req.AbstractModel,
		RouteModelCode:            req.RouteModelCode,
		SizeMode:                  req.SizeMode,
		AspectRatio:               req.AspectRatio,
		BaseResolution:            req.BaseResolution,
		Quality:                   req.Quality,
		OutputFormat:              req.OutputFormat,
		Background:                req.Background,
		OutputCompression:         req.OutputCompression,
		Moderation:                req.Moderation,
		RequestedSize:             req.RequestedSize,
		RequestedOutputImageCount: outputImageCount,
		ReferenceImageCount:       req.ReferenceImageCount,
		MaskPresent:               req.MaskPresent,
		UserGroupCode:             req.UserGroupCode,
		UserGroupCodes:            append([]string(nil), req.UserGroupCodes...),
		UserGroupMultiplier:       req.UserGroupMultiplier,
		CapabilityVersion:         req.CapabilityVersion,
	}
}

func (s *Service) applyTaskEstimateResult(ctx context.Context, task *domainimagetask.Task, req domainimagetask.CreateRequest, estimate domainbilling.EstimateResult) error {
	task.BaseResolution = estimate.BaseResolution
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
	task := domainimagetask.Task{
		UserID:               req.UserID,
		APIKeyID:             req.APIKeyID,
		SourceChannel:        defaultString(req.SourceChannel, "web"),
		ID:                   taskID,
		Status:               status,
		AbstractModel:        defaultString(req.AbstractModel, req.RouteModelCode),
		RouteModelCode:       defaultString(resolved.RouteModelCode, req.RouteModelCode),
		RouteModelID:         resolved.RouteModelID,
		RouteSnapshotVersion: resolved.RouteSnapshotVersion,
		TaskType:             req.TaskType,
		Prompt:               req.Prompt,
		NegativePrompt:       req.NegativePrompt,
		SizeMode:             modelhub.PublicSizeMode(req.SizeMode),
		RequestedSize:        req.RequestedSize,
		BaseResolution:       resolved.BaseResolution,
		Quality:              defaultString(modelhub.NormalizeQuality(req.Quality), "auto"),
		OutputFormat:         defaultString(modelhub.NormalizeOutputFormat(req.OutputFormat), "png"),
		Background:           strings.ToLower(strings.TrimSpace(req.Background)),
		OutputCompression:    defaultPositive(req.OutputCompression, 100),
		Moderation:           defaultString(modelhub.NormalizeModeration(req.Moderation), "auto"),
		AspectRatio:          req.AspectRatio,
		ResponseMode:         defaultString(req.ResponseMode, "async"),
		SavePolicy:           defaultString(req.SavePolicy, "private"),
		OutputImageCount:     normalizedCount(req.OutputImageCount),
		ReferenceImageCount:  req.ReferenceImageCount,
		ReferenceAssetIDs:    append([]string(nil), req.ReferenceAssetIDs...),
		ReferenceStrength:    req.ReferenceStrength,
		Seed:                 req.Seed,
	}
	switch task.SizeMode {
	case modelhub.SizeModeAuto:
		task.RequestedSize, task.AspectRatio = "", ""
	case modelhub.SizeModeRatio:
		size := strings.TrimSpace(resolved.ResolvedSize)
		if size == "" {
			size, _ = modelhub.CalculateImageSize(resolved.BaseResolution, req.AspectRatio)
		}
		if size != "" {
			task.RequestedSize = size
			task.ResolvedWidth, task.ResolvedHeight, _ = modelhub.ParseImageSize(size)
		}
	case modelhub.SizeModePixel:
		task.AspectRatio = ""
		task.ResolvedWidth, task.ResolvedHeight, _ = modelhub.ParseImageSize(task.RequestedSize)
	}
	if strings.TrimSpace(resolved.CapabilityVersion) != "" {
		task.GenerationSnapshot = domainimagetask.GenerationSnapshot{
			CapabilityVersion: resolved.CapabilityVersion,
			SizeMode:          task.SizeMode,
			BaseResolution:    task.BaseResolution,
			AspectRatio:       task.AspectRatio,
			ResolvedSize:      task.RequestedSize,
			ResolvedWidth:     task.ResolvedWidth,
			ResolvedHeight:    task.ResolvedHeight,
		}
	}
	setInitialTaskProgress(&task)
	return task
}

func generationResolveFieldsFromTask(task domainimagetask.Task) (baseResolution, aspectRatio, requestedSize string, err error) {
	if strings.TrimSpace(task.SizeMode) == "" {
		return task.BaseResolution, task.AspectRatio, task.RequestedSize, nil
	}
	if strings.EqualFold(strings.TrimSpace(task.Background), "transparent") && !strings.EqualFold(task.OutputFormat, "png") && !strings.EqualFold(task.OutputFormat, "webp") {
		return "", "", "", errs.New(400, modelhub.CodeTransparentFormatConflict, "transparent background requires png or webp")
	}
	switch modelhub.PublicSizeMode(task.SizeMode) {
	case modelhub.SizeModeAuto:
		if strings.TrimSpace(task.AspectRatio) != "" || strings.TrimSpace(task.RequestedSize) != "" || task.ResolvedWidth != 0 || task.ResolvedHeight != 0 {
			return "", "", "", errs.New(400, modelhub.CodeInvalidSizeMode, "auto task snapshot contains size fields")
		}
		return "", "", "", nil
	case modelhub.SizeModeRatio:
		expected := modelhub.NormalizePixelSize(task.RequestedSize)
		width, height, ok := modelhub.ParseImageSize(expected)
		if ok && (task.ResolvedWidth != 0 && task.ResolvedWidth != width || task.ResolvedHeight != 0 && task.ResolvedHeight != height) {
			return "", "", "", errs.New(400, modelhub.CodeInvalidSizeMode, "ratio task snapshot does not match its resolved size")
		}
		if !modelhub.IsLegalResolvedRatioSize(task.BaseResolution, task.AspectRatio, expected) {
			return "", "", "", errs.New(400, modelhub.CodeInvalidAspectRatio, "ratio task snapshot is invalid")
		}
		return task.BaseResolution, task.AspectRatio, "", nil
	case modelhub.SizeModePixel:
		if strings.TrimSpace(task.AspectRatio) != "" {
			return "", "", "", errs.New(400, modelhub.CodeInvalidSizeMode, "pixel task snapshot contains ratio fields")
		}
		size := modelhub.NormalizePixelSize(task.RequestedSize)
		width, height, ok := modelhub.ParseImageSize(size)
		if !ok || !modelhub.IsLegalCustomImageSize(width, height) || task.ResolvedWidth != 0 && task.ResolvedWidth != width || task.ResolvedHeight != 0 && task.ResolvedHeight != height {
			return "", "", "", errs.New(400, modelhub.CodeInvalidExplicitDimensions, "pixel task snapshot is invalid")
		}
		bucket, bucketErr := modelhub.BaseResolutionByPixelSize(size)
		if bucketErr != nil || !strings.EqualFold(strings.TrimSpace(task.BaseResolution), bucket) {
			return "", "", "", errs.New(400, modelhub.CodeInvalidExplicitDimensions, "pixel task pricing bucket does not match its dimensions")
		}
		return "", "", size, nil
	default:
		return "", "", "", errs.New(400, modelhub.CodeInvalidSizeMode, "task size_mode is unsupported")
	}
}

func validateResolvedSizeSnapshot(task domainimagetask.Task, resolved modelhub.ResolvedRequest) error {
	if strings.TrimSpace(task.GenerationSnapshot.CapabilityVersion) != "" {
		_, err := validateImmutableGenerationSnapshot(task)
		return err
	}
	mode := modelhub.PublicSizeMode(task.SizeMode)
	currentSize := modelhub.NormalizePixelSize(task.RequestedSize)
	if strings.TrimSpace(resolved.ResolvedSize) == "" {
		if mode != modelhub.SizeModeRatio {
			return nil
		}
		expectedSize, err := modelhub.CalculateImageSize(task.BaseResolution, task.AspectRatio)
		if err != nil {
			return errs.New(400, modelhub.CodeInvalidAspectRatio, "ratio task snapshot cannot be resolved")
		}
		width, height, _ := modelhub.ParseImageSize(expectedSize)
		if currentSize != expectedSize || task.ResolvedWidth != width || task.ResolvedHeight != height {
			return errs.New(400, modelhub.CodeInvalidSizeMode, "ratio task snapshot does not match the nominal size")
		}
		return nil
	}
	if strings.TrimSpace(task.PricingSnapshot.SizeMode) != "" && currentSize != modelhub.NormalizePixelSize(task.PricingSnapshot.RequestedSize) {
		return errs.New(400, modelhub.CodeInvalidSizeMode, "task size snapshot does not match its pricing snapshot")
	}
	switch mode {
	case modelhub.SizeModeAuto:
		if currentSize != "" || strings.TrimSpace(resolved.ResolvedSize) != "" {
			return errs.New(400, modelhub.CodeInvalidSizeMode, "auto task snapshot contains a resolved size")
		}
	case modelhub.SizeModeRatio, modelhub.SizeModePixel:
		expectedSize := modelhub.NormalizePixelSize(resolved.ResolvedSize)
		if expectedSize == "" || currentSize != expectedSize {
			return errs.New(400, modelhub.CodeInvalidSizeMode, "task size snapshot does not match the resolved size")
		}
		width, height, _ := modelhub.ParseImageSize(expectedSize)
		if task.ResolvedWidth != 0 && task.ResolvedWidth != width || task.ResolvedHeight != 0 && task.ResolvedHeight != height {
			return errs.New(400, modelhub.CodeInvalidSizeMode, "task dimensions do not match the resolved size")
		}
	}
	return nil
}

func validateImmutableGenerationSnapshot(task domainimagetask.Task) (string, error) {
	snapshot := task.GenerationSnapshot
	if strings.TrimSpace(snapshot.CapabilityVersion) == "" {
		return "", nil
	}
	mode := modelhub.PublicSizeMode(task.SizeMode)
	if mode != modelhub.PublicSizeMode(snapshot.SizeMode) ||
		!strings.EqualFold(strings.TrimSpace(task.BaseResolution), strings.TrimSpace(snapshot.BaseResolution)) ||
		modelhub.NormalizeRatio(task.AspectRatio) != modelhub.NormalizeRatio(snapshot.AspectRatio) ||
		modelhub.NormalizePixelSize(task.RequestedSize) != modelhub.NormalizePixelSize(snapshot.ResolvedSize) ||
		task.ResolvedWidth != snapshot.ResolvedWidth || task.ResolvedHeight != snapshot.ResolvedHeight {
		return "", errs.New(400, modelhub.CodeInvalidSizeMode, "task size fields do not match the immutable generation snapshot")
	}
	if strings.TrimSpace(task.PricingSnapshot.SizeMode) != "" &&
		(mode != modelhub.PublicSizeMode(task.PricingSnapshot.SizeMode) ||
			modelhub.NormalizePixelSize(task.RequestedSize) != modelhub.NormalizePixelSize(task.PricingSnapshot.RequestedSize)) {
		return "", errs.New(400, modelhub.CodeInvalidSizeMode, "task size snapshot does not match its pricing snapshot")
	}
	return modelhub.NormalizePixelSize(snapshot.ResolvedSize), nil
}

func setInitialTaskProgress(task *domainimagetask.Task) {
	if task == nil {
		return
	}
	switch task.Status {
	case domainimagetask.StatusQueued:
		setTaskProgress(task, domainimagetask.ProgressStageQueued, "任务已进入生成队列")
	case domainimagetask.StatusRunning:
		setTaskProgress(task, domainimagetask.ProgressStageProvider, "正在调用模型生成图片")
	case domainimagetask.StatusFailed, domainimagetask.StatusRejected:
		setTaskProgress(task, domainimagetask.ProgressStageFailed, defaultString(task.ErrorMessage, "任务生成失败"))
	case domainimagetask.StatusSucceeded, domainimagetask.StatusPartialFailed:
		setCompletedTaskProgress(task)
	}
}

func setCompletedTaskProgress(task *domainimagetask.Task) {
	message := "生成完成，结果已同步到资产"
	if task != nil && task.Status == domainimagetask.StatusPartialFailed {
		message = "部分图片生成完成，其余图片生成失败"
	}
	setTaskProgress(task, domainimagetask.ProgressStageCompleted, message)
}

func setTaskProgress(task *domainimagetask.Task, stage, message string) {
	if task == nil {
		return
	}
	task.ProgressStage = stage
	task.ProgressMessage = message
}

func normalizeCreateRequest(req domainimagetask.CreateRequest) (domainimagetask.CreateRequest, error) {
	if !provider.IsSupportedTaskType(req.TaskType) {
		return req, errs.BadRequest("unsupported task_type")
	}
	normalized, err := modelhub.NormalizeResolveRequest(modelhub.ResolveRequest{
		SizeMode:          req.SizeMode,
		AspectRatio:       req.AspectRatio,
		BaseResolution:    req.BaseResolution,
		Quality:           req.Quality,
		OutputFormat:      req.OutputFormat,
		Background:        req.Background,
		OutputCompression: req.OutputCompression,
		Moderation:        req.Moderation,
		RequestedSize:     req.RequestedSize,
	})
	if err != nil {
		return req, err
	}
	req.SizeMode = normalized.SizeMode
	req.AspectRatio = normalized.AspectRatio
	req.BaseResolution = normalized.BaseResolution
	req.Quality = normalized.Quality
	req.OutputFormat = normalized.OutputFormat
	req.Background = normalized.Background
	req.OutputCompression = normalized.OutputCompression
	req.Moderation = normalized.Moderation
	req.RequestedSize = normalized.RequestedSize
	return req, nil
}

func normalizeExecuteRequest(req domainimagetask.ExecuteRequest) (domainimagetask.ExecuteRequest, error) {
	if !provider.IsSupportedTaskType(req.TaskType) {
		return req, errs.BadRequest("unsupported task_type")
	}
	normalized, err := modelhub.NormalizeResolveRequest(modelhub.ResolveRequest{
		SizeMode:          req.SizeMode,
		AspectRatio:       req.AspectRatio,
		BaseResolution:    req.BaseResolution,
		Quality:           req.Quality,
		OutputFormat:      req.OutputFormat,
		Background:        req.Background,
		OutputCompression: req.OutputCompression,
		Moderation:        req.Moderation,
		RequestedSize:     req.RequestedSize,
	})
	if err != nil {
		return req, err
	}
	req.SizeMode = normalized.SizeMode
	req.AspectRatio = normalized.AspectRatio
	req.BaseResolution = normalized.BaseResolution
	req.Quality = normalized.Quality
	req.OutputFormat = normalized.OutputFormat
	req.Background = normalized.Background
	req.OutputCompression = normalized.OutputCompression
	req.Moderation = normalized.Moderation
	req.RequestedSize = normalized.RequestedSize
	return req, nil
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

func splitOutputImageCount(total, maxPerRequest int) []int {
	total = normalizedCount(total)
	if maxPerRequest <= 0 {
		maxPerRequest = 1
	}
	if maxPerRequest > 10 {
		maxPerRequest = 10
	}
	chunks := make([]int, 0, (total-1)/maxPerRequest+1)
	remaining := total
	for remaining > 0 {
		count := maxPerRequest
		if remaining < count {
			count = remaining
		}
		chunks = append(chunks, count)
		remaining -= count
	}
	return chunks
}

func fanoutConcurrencyLimit(candidate modelhub.ProviderCandidate, userConcurrencyLimit, batchCount int) int {
	if batchCount <= 1 {
		return 1
	}
	limit := batchCount
	if candidate.ConcurrencyLimit > 0 && candidate.ConcurrencyLimit < limit {
		limit = candidate.ConcurrencyLimit
	}
	if userConcurrencyLimit > 0 && userConcurrencyLimit < limit {
		limit = userConcurrencyLimit
	}
	return limit
}

func (s *Service) userConcurrencyLimit(ctx context.Context, userID int64) (int, error) {
	if userID <= 0 {
		return 0, nil
	}
	source, ok := s.store.(UserConcurrencyLimitSource)
	if !ok {
		return 0, nil
	}
	limit, err := source.UserConcurrencyLimit(ctx, userID)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("load user concurrency limit: %w", err)
	}
	return limit, nil
}

func (s *Service) acquireProviderConcurrency(ctx context.Context, userID int64, userConcurrencyLimit int, candidate modelhub.ProviderCandidate) (func(), error) {
	resources := []ConcurrencyResource{
		{Key: userConcurrencyKey(userID), Limit: userConcurrencyLimit},
		{Key: modelAccountConcurrencyKey(candidate), Limit: candidate.ConcurrencyLimit},
	}
	return s.concurrencyGate.Acquire(ctx, resources, providerConcurrencyLeaseTTL(candidate))
}

func NewLocalConcurrencyGate() ConcurrencyGate {
	return &localConcurrencyGate{
		entries: make(map[string]*modelAccountConcurrencyState),
		changed: make(chan struct{}),
	}
}

func (g *localConcurrencyGate) Acquire(ctx context.Context, resources []ConcurrencyResource, _ time.Duration) (func(), error) {
	resources = normalizeConcurrencyResources(resources)
	if len(resources) == 0 {
		return func() {}, nil
	}
	for {
		g.mu.Lock()
		available := true
		for _, resource := range resources {
			state := g.entries[resource.Key]
			if state == nil {
				state = &modelAccountConcurrencyState{limit: resource.Limit}
				g.entries[resource.Key] = state
			} else if state.limit != resource.Limit {
				state.limit = resource.Limit
				g.signalChangedLocked()
			}
			if state.active >= state.limit {
				available = false
			}
		}
		if available {
			for _, resource := range resources {
				g.entries[resource.Key].active++
			}
			g.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					g.mu.Lock()
					for _, resource := range resources {
						state := g.entries[resource.Key]
						if state != nil && state.active > 0 {
							state.active--
						}
					}
					g.signalChangedLocked()
					g.mu.Unlock()
				})
			}, nil
		}
		changed := g.changed
		g.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

func (g *localConcurrencyGate) signalChangedLocked() {
	close(g.changed)
	g.changed = make(chan struct{})
}

func normalizeConcurrencyResources(resources []ConcurrencyResource) []ConcurrencyResource {
	normalized := make([]ConcurrencyResource, 0, len(resources))
	indexes := make(map[string]int, len(resources))
	for _, resource := range resources {
		resource.Key = strings.TrimSpace(resource.Key)
		if resource.Key == "" || resource.Limit <= 0 {
			continue
		}
		if index, ok := indexes[resource.Key]; ok {
			if resource.Limit < normalized[index].Limit {
				normalized[index].Limit = resource.Limit
			}
			continue
		}
		indexes[resource.Key] = len(normalized)
		normalized = append(normalized, resource)
	}
	return normalized
}

func providerConcurrencyLeaseTTL(candidate modelhub.ProviderCandidate) time.Duration {
	if candidate.TimeoutMS > 0 {
		return time.Duration(candidate.TimeoutMS)*time.Millisecond + 30*time.Second
	}
	return 2 * time.Minute
}

func modelAccountConcurrencyKey(candidate modelhub.ProviderCandidate) string {
	if candidate.ConcurrencyLimit <= 0 {
		return ""
	}
	if candidate.ModelAccountID > 0 {
		return fmt.Sprintf("model-account:%d", candidate.ModelAccountID)
	}
	if candidate.AccountModelID > 0 {
		return fmt.Sprintf("account-model:%d", candidate.AccountModelID)
	}
	return "provider:" + strings.ToLower(strings.TrimSpace(candidate.Provider)) + "|" + strings.TrimSpace(candidate.BaseURL)
}

func userConcurrencyKey(userID int64) string {
	if userID <= 0 {
		return ""
	}
	return fmt.Sprintf("user:%d", userID)
}

func defaultPositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func fanoutLogAttrs(operation string, task domainimagetask.Task, candidate modelhub.ProviderCandidate, req provider.ImageRequest, totalCount, fanoutIndex int) []any {
	return []any{
		"component", "imagetask",
		"operation", operation,
		"task_id", task.ID,
		"user_id", task.UserID,
		"task_type", task.TaskType,
		"size_mode", task.SizeMode,
		"aspect_ratio", task.AspectRatio,
		"requested_size", task.RequestedSize,
		"base_resolution", task.BaseResolution,
		"route_model_id", candidate.RouteModelID,
		"route_model_code", candidate.RouteModelCode,
		"account_model_id", candidate.AccountModelID,
		"model_account_id", candidate.ModelAccountID,
		"provider", candidate.Provider,
		"adapter_type", candidate.AdapterType,
		"model_code", candidate.ModelCode,
		"timeout_ms", candidate.TimeoutMS,
		"fanout_index", fanoutIndex,
		"fanout_total", totalCount,
		"output_count", req.OutputImageCount,
		"request_size", req.Size,
		"quality", req.Quality,
		"request_output_format", req.OutputFormat,
		"request_output_compression", req.OutputCompression,
		"request_moderation", req.Moderation,
		"request_response_format", string(req.ResponseFormat),
	}
}

func contextErrorString(ctx context.Context) string {
	if ctx == nil || ctx.Err() == nil {
		return ""
	}
	return ctx.Err().Error()
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
