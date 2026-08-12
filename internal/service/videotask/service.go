package videotask

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	"github.com/fatballfish/pic-gallery/internal/domain/prompttemplate"
	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type VariableBinding struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type ReferenceBinding struct {
	Name    string    `json:"name"`
	AssetID uuid.UUID `json:"asset_id"`
}
type InputRequest struct {
	AssetID uuid.UUID             `json:"asset_id"`
	Role    domainvideo.InputRole `json:"role"`
	Ordinal int                   `json:"ordinal"`
}

type CreateRequest struct {
	UserID             int64                   `json:"-"`
	ProjectID          uuid.UUID               `json:"project_id"`
	IdempotencyKey     string                  `json:"-"`
	QuoteToken         string                  `json:"quote_token"`
	RouteModelCode     string                  `json:"route_model_code"`
	TaskType           domainvideo.TaskType    `json:"task_type"`
	PromptTemplate     string                  `json:"prompt_template"`
	PromptVariables    []VariableBinding       `json:"prompt_variables"`
	ReferenceBindings  []ReferenceBinding      `json:"reference_bindings"`
	Inputs             []InputRequest          `json:"inputs"`
	DurationSeconds    int                     `json:"duration_seconds"`
	Resolution         domainvideo.Resolution  `json:"resolution"`
	AspectRatio        domainvideo.AspectRatio `json:"aspect_ratio"`
	AudioMode          domainvideo.AudioMode   `json:"audio_mode"`
	OutputCount        int                     `json:"output_count"`
	SourceChannel      string                  `json:"-"`
	SourceCanvasID     *uuid.UUID              `json:"-"`
	SourceCanvasNodeID string                  `json:"-"`
}

type ownedProjectResolver interface {
	ResolveOwned(context.Context, int64, string) (domainproject.Project, error)
}

type Service struct {
	store    Store
	quotes   QuoteVerifier
	projects ownedProjectResolver
	assets   AssetReader
	now      func() time.Time
}

type preparedRequest struct {
	video        domainvideo.Request
	resolved     prompttemplate.ResolveResult
	inputRecords []CreateInputRecord
}

func NewService(store Store, quotes QuoteVerifier, projects ownedProjectResolver, assets AssetReader, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, quotes: quotes, projects: projects, assets: assets, now: now}
}

func (s *Service) Estimate(ctx context.Context, req CreateRequest) (Estimate, error) {
	if s == nil || s.quotes == nil || s.projects == nil || s.assets == nil {
		return Estimate{}, errs.Internal("video task service is unavailable")
	}
	if req.UserID <= 0 || req.ProjectID == uuid.Nil {
		return Estimate{}, errs.BadRequest("user and project are required")
	}
	prepared, err := s.prepare(ctx, req)
	if err != nil {
		return Estimate{}, err
	}
	estimator, ok := s.quotes.(interface {
		Estimate(context.Context, int64, EstimateRequest) (Estimate, error)
	})
	if !ok {
		return Estimate{}, errs.Internal("video estimates are unavailable")
	}
	return estimator.Estimate(ctx, req.UserID, EstimateRequest{RouteModelCode: strings.TrimSpace(req.RouteModelCode), Video: prepared.video})
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Task, bool, error) {
	if s == nil || s.store == nil || s.quotes == nil || s.projects == nil || s.assets == nil {
		return Task{}, false, errs.Internal("video task service is unavailable")
	}
	if req.UserID <= 0 || req.ProjectID == uuid.Nil || strings.TrimSpace(req.IdempotencyKey) == "" || strings.TrimSpace(req.QuoteToken) == "" {
		return Task{}, false, errs.BadRequest("user, project, idempotency key and quote are required")
	}
	if len(req.IdempotencyKey) > 128 {
		return Task{}, false, errs.BadRequest("idempotency key is too long")
	}
	fingerprint, err := createFingerprint(req)
	if err != nil {
		return Task{}, false, err
	}
	if existing, found, findErr := s.store.FindByIdempotency(ctx, req.UserID, strings.TrimSpace(req.IdempotencyKey)); findErr != nil {
		return Task{}, false, findErr
	} else if found {
		if existing.RequestFingerprint != fingerprint {
			return Task{}, false, ErrIdempotencyConflict
		}
		return existing, true, nil
	}
	prepared, err := s.prepare(ctx, req)
	if err != nil {
		return Task{}, false, err
	}
	verified, err := s.quotes.Verify(ctx, req.UserID, EstimateRequest{RouteModelCode: strings.TrimSpace(req.RouteModelCode), Video: prepared.video}, req.QuoteToken)
	if err != nil {
		return Task{}, false, err
	}
	now := s.now().UTC()
	task := Task{ID: uuid.New(), UserID: req.UserID, ProjectID: req.ProjectID, SourceChannel: defaultSource(req.SourceChannel), SourceCanvasID: req.SourceCanvasID, SourceCanvasNodeID: req.SourceCanvasNodeID, TaskType: req.TaskType, Status: domainvideo.TaskStatusQueued, ProgressStage: "queued", PromptTemplate: prepared.resolved.CanonicalTemplate, ExecutionPrompt: prepared.resolved.Expanded, RouteModelID: verified.RouteModelID, RouteModelCode: strings.TrimSpace(req.RouteModelCode), DurationSeconds: req.DurationSeconds, Resolution: req.Resolution, AspectRatio: req.AspectRatio, GenerateAudio: req.AudioMode == domainvideo.AudioModeGenerated, RequestedOutputCount: req.OutputCount, EstimatedPoints: verified.EstimatedPoints, ReservedPoints: verified.MaxReservedPoints, ActualPoints: "0.00000", SettlementStatus: "reserved", IdempotencyKey: strings.TrimSpace(req.IdempotencyKey), RequestFingerprint: fingerprint, Version: 1, CreatedAt: now, UpdatedAt: now}
	task.PromptBindingSnapshot = promptSnapshot(prepared.resolved, req.PromptVariables)
	task.PricingSnapshot = map[string]any{"price_version": verified.PriceVersion, "unit_points": verified.UnitPoints, "estimated_points": verified.EstimatedPoints, "max_reserved_points": verified.MaxReservedPoints, "reference_image_count": len(req.Inputs), "sales_rule": verified.SalesRule}
	task.RoutingSnapshot = map[string]any{
		"capability_version": verified.CapabilityVersion, "config_version": verified.ConfigVersion, "route_model_code": verified.RouteModelCode,
		"route_candidate_id": verified.RouteCandidateID, "account_model_id": verified.AccountModelID, "model_account_id": verified.ModelAccountID,
		"provider_code": verified.ProviderCode, "model_code": verified.ModelCode,
	}
	for index := 0; index < req.OutputCount; index++ {
		task.Items = append(task.Items, Item{ID: uuid.New(), Ordinal: index, Status: domainvideo.ItemStateQueued, Stage: "queued", ActualOutputSeconds: "0.000", ActualPoints: "0.00000", Version: 1})
	}
	for _, record := range prepared.inputRecords {
		task.Inputs = append(task.Inputs, Input{ID: record.ID, AssetID: record.AssetID, Role: record.Role, Ordinal: record.Ordinal, AssetSnapshot: cloneMap(record.AssetSnapshot)})
	}
	created, replayed, err := s.store.Create(ctx, CreateRecord{Task: task, Inputs: prepared.inputRecords, ReservePoints: verified.MaxReservedPoints, ReserveReason: "video generation reserve"})
	if err != nil {
		return Task{}, false, err
	}
	return created, replayed, nil
}

func (s *Service) prepare(ctx context.Context, req CreateRequest) (preparedRequest, error) {
	project, err := s.projects.ResolveOwned(ctx, req.UserID, req.ProjectID.String())
	if err != nil {
		return preparedRequest{}, mapOwnershipError(err, "project not found")
	}
	if project.Status != domainproject.StatusActive {
		return preparedRequest{}, errs.New(404, errs.CodeNotFound, "project not found")
	}

	domainInputs := make([]domainvideo.Input, 0, len(req.Inputs))
	selectedIDs := make([]string, 0, len(req.Inputs))
	promptAssets := make([]prompttemplate.Asset, 0, len(req.Inputs))
	inputRecords := make([]CreateInputRecord, 0, len(req.Inputs))
	seen := map[uuid.UUID]struct{}{}
	for _, input := range req.Inputs {
		if input.AssetID == uuid.Nil {
			return preparedRequest{}, errs.BadRequest("input asset id is required")
		}
		asset, assetErr := s.assets.GetAsset(ctx, req.UserID, input.AssetID)
		if assetErr != nil {
			return preparedRequest{}, mapOwnershipError(assetErr, "input asset not found")
		}
		if asset.MediaType != "image" || !(asset.Status == "ready" || asset.Status == "ready_original") {
			return preparedRequest{}, errs.BadRequest("video input asset must be a ready image")
		}
		domainInputs = append(domainInputs, domainvideo.Input{AssetID: asset.ID.String(), Role: input.Role, Ordinal: input.Ordinal, MediaType: string(asset.MediaType), Format: asset.MIMEType, SizeBytes: asset.FileSizeBytes, Width: intValuePtr(asset.Width), Height: intValuePtr(asset.Height)})
		if _, ok := seen[asset.ID]; !ok {
			seen[asset.ID] = struct{}{}
			selectedIDs = append(selectedIDs, asset.ID.String())
			promptAssets = append(promptAssets, prompttemplate.Asset{ID: asset.ID.String(), Name: asset.Name})
		}
		inputRecords = append(inputRecords, CreateInputRecord{ID: uuid.New(), AssetID: asset.ID, Role: string(input.Role), Ordinal: input.Ordinal, AssetSnapshot: assetSnapshot(asset)})
	}
	references := make([]prompttemplate.ReferenceBinding, 0, len(req.ReferenceBindings))
	for _, binding := range req.ReferenceBindings {
		references = append(references, prompttemplate.ReferenceBinding{Name: binding.Name, AssetID: binding.AssetID.String()})
	}
	variables := make([]prompttemplate.VariableBinding, 0, len(req.PromptVariables))
	for _, binding := range req.PromptVariables {
		variables = append(variables, prompttemplate.VariableBinding{Name: binding.Name, Value: binding.Value})
	}
	resolved, err := prompttemplate.Resolve(prompttemplate.ResolveRequest{Template: req.PromptTemplate, ReferenceAssetIDs: selectedIDs, ReferenceBindings: references, VariableBindings: variables, Assets: promptAssets, Limits: prompttemplate.DefaultLimits()})
	if err != nil {
		return preparedRequest{}, mapPromptError(err)
	}
	videoReq := domainvideo.Request{TaskType: req.TaskType, Prompt: resolved.Expanded, DurationSeconds: req.DurationSeconds, Resolution: req.Resolution, AspectRatio: req.AspectRatio, AudioMode: req.AudioMode, OutputCount: req.OutputCount, Inputs: domainInputs}
	return preparedRequest{video: videoReq, resolved: resolved, inputRecords: inputRecords}, nil
}

func (s *Service) List(ctx context.Context, req ListRequest) (Page, error) {
	if s == nil || s.store == nil {
		return Page{}, errs.Internal("video task service is unavailable")
	}
	if req.UserID <= 0 {
		return Page{}, errs.BadRequest("user is required")
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}
	if req.Limit > 100 {
		req.Limit = 100
	}
	return s.store.List(ctx, req)
}
func (s *Service) Get(ctx context.Context, userID int64, id uuid.UUID) (Task, error) {
	if s == nil || s.store == nil {
		return Task{}, errs.Internal("video task service is unavailable")
	}
	task, err := s.store.Get(ctx, userID, id)
	if err != nil {
		return Task{}, mapOwnershipError(err, "video task not found")
	}
	return task, nil
}
func (s *Service) Cancel(ctx context.Context, userID int64, id uuid.UUID, key string) (Task, error) {
	if strings.TrimSpace(key) == "" {
		return Task{}, errs.BadRequest("Idempotency-Key is required")
	}
	task, err := s.store.RequestCancel(ctx, userID, id, strings.TrimSpace(key))
	if err != nil {
		return Task{}, mapOwnershipError(err, "video task not found")
	}
	return task, nil
}

func createFingerprint(req CreateRequest) (string, error) {
	payload := struct {
		ProjectID         uuid.UUID               `json:"project_id"`
		RouteModelCode    string                  `json:"route_model_code"`
		TaskType          domainvideo.TaskType    `json:"task_type"`
		PromptTemplate    string                  `json:"prompt_template"`
		PromptVariables   []VariableBinding       `json:"prompt_variables"`
		ReferenceBindings []ReferenceBinding      `json:"reference_bindings"`
		Inputs            []InputRequest          `json:"inputs"`
		DurationSeconds   int                     `json:"duration_seconds"`
		Resolution        domainvideo.Resolution  `json:"resolution"`
		AspectRatio       domainvideo.AspectRatio `json:"aspect_ratio"`
		AudioMode         domainvideo.AudioMode   `json:"audio_mode"`
		OutputCount       int                     `json:"output_count"`
	}{req.ProjectID, strings.TrimSpace(req.RouteModelCode), req.TaskType, req.PromptTemplate, req.PromptVariables, req.ReferenceBindings, req.Inputs, req.DurationSeconds, req.Resolution, req.AspectRatio, req.AudioMode, req.OutputCount}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func promptSnapshot(resolved prompttemplate.ResolveResult, variables []VariableBinding) map[string]any {
	refs := make([]map[string]any, 0, len(resolved.Snapshot.References))
	for _, ref := range resolved.Snapshot.References {
		refs = append(refs, map[string]any{"name": ref.Name, "asset_id": ref.AssetID, "index": ref.Index})
	}
	vars := make([]map[string]any, 0, len(variables))
	for _, v := range variables {
		vars = append(vars, map[string]any{"name": v.Name, "value": v.Value})
	}
	return map[string]any{"references": refs, "variables": vars}
}
func assetSnapshot(asset mediaassetservice.Asset) map[string]any {
	return map[string]any{"id": asset.ID.String(), "name": asset.Name, "project_id": asset.ProjectID.String(), "media_type": asset.MediaType, "mime_type": asset.MIMEType, "file_size_bytes": asset.FileSizeBytes, "sha256": asset.SHA256, "width": asset.Width, "height": asset.Height}
}
func mapPromptError(err error) error {
	var target *prompttemplate.Error
	if errors.As(err, &target) {
		status := 400
		if target.Code == prompttemplate.CodeStale {
			status = 409
		}
		return errs.WithDetails(errs.New(status, errs.CodeValidationFailed, target.Message), map[string]any{"field": target.Field, "rule": target.Rule, "name": target.Name, "offset": target.Offset})
	}
	return err
}
func mapOwnershipError(err error, message string) error {
	if errors.Is(err, projectservice.ErrNotFound) || errors.Is(err, repoerr.ErrNotFound) {
		return errs.New(404, errs.CodeNotFound, message)
	}
	var appErr *errs.Error
	if errors.As(err, &appErr) {
		return appErr
	}
	return err
}
func defaultSource(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "web"
	}
	return value
}
func intValuePtr(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
func cloneMap(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for k, v := range value {
		out[k] = v
	}
	return out
}
