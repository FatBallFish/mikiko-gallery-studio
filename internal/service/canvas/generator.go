package canvas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	domaincanvas "github.com/fatballfish/pic-gallery/internal/domain/canvas"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/prompttemplate"
	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	videotaskservice "github.com/fatballfish/pic-gallery/internal/service/videotask"
)

type ImageTaskPort interface {
	CreateTask(context.Context, domainimagetask.CreateRequest) (domainimagetask.Task, error)
	GetByID(context.Context, int64, string) (domainimagetask.Task, error)
}

type ImageEstimator interface {
	EstimateImage(context.Context, domainimagetask.CreateRequest) (Estimate, error)
}

type BillingEstimator interface {
	EstimateContext(context.Context, domainbilling.EstimateRequest) (domainbilling.EstimateResult, error)
}

type ImageBillingEstimator struct{ billing BillingEstimator }

func NewImageBillingEstimator(billing BillingEstimator) *ImageBillingEstimator {
	return &ImageBillingEstimator{billing: billing}
}

func (e *ImageBillingEstimator) EstimateImage(ctx context.Context, req domainimagetask.CreateRequest) (Estimate, error) {
	if e == nil || e.billing == nil {
		return Estimate{}, errors.New("canvas image estimates are unavailable")
	}
	count := req.OutputImageCount
	if count <= 0 {
		count = 1
	}
	result, err := e.billing.EstimateContext(ctx, domainbilling.EstimateRequest{
		RouteKey: req.TaskID, TaskType: req.TaskType, AbstractModel: req.AbstractModel, RouteModelCode: req.RouteModelCode,
		SizeMode: req.SizeMode, AspectRatio: req.AspectRatio, BaseResolution: req.BaseResolution, Quality: req.Quality,
		OutputFormat: req.OutputFormat, Background: req.Background, OutputCompression: req.OutputCompression, Moderation: req.Moderation,
		RequestedSize: req.RequestedSize, RequestedOutputImageCount: count, ReferenceImageCount: req.ReferenceImageCount,
		MaskPresent: req.MaskPresent, UserGroupCode: req.UserGroupCode, UserGroupCodes: append([]string(nil), req.UserGroupCodes...),
		UserGroupMultiplier: req.UserGroupMultiplier, CapabilityVersion: req.CapabilityVersion,
	})
	if err != nil {
		return Estimate{}, err
	}
	return Estimate{Points: result.EstimatedPoints, Detail: map[string]any{"charged_points": result.ChargedPoints, "base_resolution": result.BaseResolution, "resolved_size": result.ResolvedSize, "capability_version": result.CapabilityVersion}}, nil
}

type VideoTaskPort interface {
	Estimate(context.Context, videotaskservice.CreateRequest) (videotaskservice.Estimate, error)
	Create(context.Context, videotaskservice.CreateRequest) (videotaskservice.Task, bool, error)
	Get(context.Context, int64, uuid.UUID) (videotaskservice.Task, error)
	Cancel(context.Context, int64, uuid.UUID, string) (videotaskservice.Task, error)
}

type TaskGenerator struct {
	images         ImageTaskPort
	imageEstimator ImageEstimator
	videos         VideoTaskPort
}

func NewTaskGenerator(images ImageTaskPort, imageEstimator ImageEstimator, videos VideoTaskPort) *TaskGenerator {
	return &TaskGenerator{images: images, imageEstimator: imageEstimator, videos: videos}
}

func (g *TaskGenerator) Estimate(ctx context.Context, submission GenerationSubmission) (Estimate, error) {
	switch submission.Kind {
	case TaskKindImage:
		if g == nil || g.imageEstimator == nil {
			return Estimate{}, errors.New("canvas image estimates are unavailable")
		}
		request, err := imageRequest(submission)
		if err != nil {
			return Estimate{}, err
		}
		return g.imageEstimator.EstimateImage(ctx, request)
	case TaskKindVideo:
		if g == nil || g.videos == nil {
			return Estimate{}, errors.New("canvas video estimates are unavailable")
		}
		request, err := videoRequest(submission)
		if err != nil {
			return Estimate{}, err
		}
		estimate, err := g.videos.Estimate(ctx, request)
		if err != nil {
			return Estimate{}, err
		}
		return Estimate{Points: estimate.EstimatedPoints, Detail: map[string]any{"quote_token": estimate.QuoteToken, "max_reserved_points": estimate.MaxReservedPoints, "expires_at": estimate.ExpiresAt}}, nil
	default:
		return Estimate{}, fmt.Errorf("unsupported canvas task kind %q", submission.Kind)
	}
}

func (g *TaskGenerator) Generate(ctx context.Context, submission GenerationSubmission) (GenerationTask, error) {
	switch submission.Kind {
	case TaskKindImage:
		if g == nil || g.images == nil {
			return GenerationTask{}, errors.New("canvas image generation is unavailable")
		}
		request, err := imageRequest(submission)
		if err != nil {
			return GenerationTask{}, err
		}
		task, err := g.images.CreateTask(ctx, request)
		if err != nil {
			return GenerationTask{}, err
		}
		id, err := uuid.Parse(task.ID)
		if err != nil {
			return GenerationTask{}, fmt.Errorf("image task returned invalid id: %w", err)
		}
		return GenerationTask{TaskID: id, Kind: TaskKindImage, Status: imageRunStatus(task.Status)}, nil
	case TaskKindVideo:
		if g == nil || g.videos == nil {
			return GenerationTask{}, errors.New("canvas video generation is unavailable")
		}
		request, err := videoRequest(submission)
		if err != nil {
			return GenerationTask{}, err
		}
		task, _, err := g.videos.Create(ctx, request)
		if err != nil {
			return GenerationTask{}, err
		}
		return GenerationTask{TaskID: task.ID, Kind: TaskKindVideo, Status: videoRunStatus(task.Status)}, nil
	default:
		return GenerationTask{}, fmt.Errorf("unsupported canvas task kind %q", submission.Kind)
	}
}

func (g *TaskGenerator) Status(ctx context.Context, userID int64, kind TaskKind, taskID uuid.UUID) (TaskStatus, error) {
	switch kind {
	case TaskKindImage:
		if g == nil || g.images == nil {
			return TaskStatus{}, errors.New("canvas image generation is unavailable")
		}
		task, err := g.images.GetByID(ctx, userID, taskID.String())
		if err != nil {
			return TaskStatus{}, err
		}
		assets := make([]uuid.UUID, 0, len(task.Results))
		for _, result := range task.Results {
			if id, parseErr := uuid.Parse(result.ID); parseErr == nil {
				assets = append(assets, id)
			}
		}
		return TaskStatus{Status: imageRunStatus(task.Status), ResultAssetIDs: assets, ErrorCode: task.ErrorCode, ErrorMessage: task.ErrorMessage}, nil
	case TaskKindVideo:
		if g == nil || g.videos == nil {
			return TaskStatus{}, errors.New("canvas video generation is unavailable")
		}
		task, err := g.videos.Get(ctx, userID, taskID)
		if err != nil {
			return TaskStatus{}, err
		}
		assets := make([]uuid.UUID, 0, len(task.Items))
		for _, item := range task.Items {
			if item.ResultAssetID != nil {
				assets = append(assets, *item.ResultAssetID)
			}
		}
		return TaskStatus{Status: videoRunStatus(task.Status), ResultAssetIDs: assets}, nil
	default:
		return TaskStatus{}, fmt.Errorf("unsupported canvas task kind %q", kind)
	}
}

func (g *TaskGenerator) Cancel(ctx context.Context, userID int64, kind TaskKind, taskID uuid.UUID) error {
	if kind == TaskKindImage {
		return errors.New("image task cancellation is not supported")
	}
	if kind != TaskKindVideo || g == nil || g.videos == nil {
		return errors.New("canvas video generation is unavailable")
	}
	_, err := g.videos.Cancel(ctx, userID, taskID, "canvas-"+taskID.String())
	return err
}

func imageRequest(submission GenerationSubmission) (domainimagetask.CreateRequest, error) {
	type imageDraft struct {
		AbstractModel     string `json:"abstract_model"`
		RouteModelCode    string `json:"route_model_code"`
		TaskType          string `json:"task_type"`
		Prompt            string `json:"prompt"`
		PromptTemplate    string `json:"prompt_template"`
		NegativePrompt    string `json:"negative_prompt"`
		SizeMode          string `json:"size_mode"`
		RequestedSize     string `json:"requested_size"`
		BaseResolution    string `json:"base_resolution"`
		Quality           string `json:"quality"`
		OutputFormat      string `json:"output_format"`
		Background        string `json:"background"`
		OutputCompression int    `json:"output_compression"`
		Moderation        string `json:"moderation"`
		AspectRatio       string `json:"aspect_ratio"`
		OutputImageCount  int    `json:"output_image_count"`
		ReferenceStrength int    `json:"reference_strength"`
		Seed              *int64 `json:"seed"`
		ResponseMode      string `json:"response_mode"`
		SavePolicy        string `json:"save_policy"`
		CapabilityVersion string `json:"capability_version"`
	}
	var draft imageDraft
	if err := json.Unmarshal(generationDraft(submission.Node.Payload), &draft); err != nil {
		return domainimagetask.CreateRequest{}, fmt.Errorf("decode image generation node: %w", err)
	}
	request := domainimagetask.CreateRequest{AbstractModel: draft.AbstractModel, RouteModelCode: draft.RouteModelCode, TaskType: draft.TaskType, Prompt: draft.Prompt, PromptTemplate: draft.PromptTemplate, NegativePrompt: draft.NegativePrompt, SizeMode: draft.SizeMode, RequestedSize: draft.RequestedSize, BaseResolution: draft.BaseResolution, Quality: draft.Quality, OutputFormat: draft.OutputFormat, Background: draft.Background, OutputCompression: draft.OutputCompression, Moderation: draft.Moderation, AspectRatio: draft.AspectRatio, OutputImageCount: draft.OutputImageCount, ReferenceStrength: draft.ReferenceStrength, Seed: draft.Seed, ResponseMode: draft.ResponseMode, SavePolicy: draft.SavePolicy, CapabilityVersion: draft.CapabilityVersion}
	switch request.SizeMode {
	case "auto":
		request.RequestedSize, request.BaseResolution, request.AspectRatio = "", "", ""
	case "pixel":
		request.BaseResolution, request.AspectRatio = "", ""
	case "ratio":
		request.RequestedSize = ""
	}
	prompt, err := promptFromInputs(submission)
	if err != nil {
		return request, err
	}
	if prompt.Template != "" {
		request.Prompt = prompt.Template
		request.PromptTemplate = prompt.Template
		request.PromptVariables = make([]domainimagetask.PromptVariableInput, 0, len(prompt.VariableNames))
		for _, name := range prompt.VariableNames {
			if strings.TrimSpace(prompt.Variables[name]) == "" {
				return request, fmt.Errorf("prompt variable %q is not filled", name)
			}
			request.PromptVariables = append(request.PromptVariables, domainimagetask.PromptVariableInput{Name: name, Value: prompt.Variables[name]})
		}
	}
	if request.TaskType == "" {
		request.TaskType = "text_to_image"
	}
	if request.OutputImageCount <= 0 {
		request.OutputImageCount = 1
	}
	if request.AbstractModel == "" && request.RouteModelCode == "" {
		return request, errors.New("image generation node must select a model")
	}
	if request.Prompt == "" && request.PromptTemplate == "" {
		return request, errors.New("image generation node must connect or define a prompt")
	}
	request.TaskID = uuid.NewString()
	request.UserID = submission.UserID
	request.UserGroupCode = submission.UserGroupCode
	request.UserGroupCodes = append([]string(nil), submission.UserGroupCodes...)
	request.UserGroupMultiplier = submission.UserGroupMultiplier
	request.ProjectID = submission.ProjectID.String()
	request.SourceChannel = "canvas"
	request.ReferenceAssetIDs = request.ReferenceAssetIDs[:0]
	for _, input := range submission.Inputs {
		if input.Role == "reference" && input.Node.AssetID != "" {
			request.ReferenceAssetIDs = append(request.ReferenceAssetIDs, input.Node.AssetID)
		}
	}
	request.ReferenceImageCount = len(request.ReferenceAssetIDs)
	bindings, err := promptReferenceBindings(prompt.ReferenceNames, submission.Inputs)
	if err != nil {
		return request, err
	}
	request.ReferenceBindings = make([]domainimagetask.PromptReferenceInput, 0, len(bindings))
	for _, binding := range bindings {
		request.ReferenceBindings = append(request.ReferenceBindings, domainimagetask.PromptReferenceInput{Name: binding.Name, AssetID: binding.AssetID})
	}
	return request, nil
}

func videoRequest(submission GenerationSubmission) (videotaskservice.CreateRequest, error) {
	var request videotaskservice.CreateRequest
	if err := json.Unmarshal(generationDraft(submission.Node.Payload), &request); err != nil {
		return request, fmt.Errorf("decode video generation node: %w", err)
	}
	request.UserID = submission.UserID
	request.UserGroupCodes = append([]string(nil), submission.UserGroupCodes...)
	request.ProjectID = submission.ProjectID
	request.IdempotencyKey = submission.IdempotencyKey
	request.SourceChannel = "canvas"
	request.SourceCanvasID = &submission.CanvasID
	request.SourceCanvasNodeID = submission.NodeID
	prompt, err := promptFromInputs(submission)
	if err != nil {
		return request, err
	}
	if prompt.Template != "" {
		request.PromptTemplate = prompt.Template
		request.PromptVariables = make([]videotaskservice.VariableBinding, 0, len(prompt.VariableNames))
		for _, name := range prompt.VariableNames {
			if strings.TrimSpace(prompt.Variables[name]) == "" {
				return request, fmt.Errorf("prompt variable %q is not filled", name)
			}
			request.PromptVariables = append(request.PromptVariables, videotaskservice.VariableBinding{Name: name, Value: prompt.Variables[name]})
		}
	}
	request.Inputs = request.Inputs[:0]
	for _, input := range submission.Inputs {
		if input.Node.AssetID == "" {
			continue
		}
		assetID, err := uuid.Parse(input.Node.AssetID)
		if err != nil {
			return request, fmt.Errorf("invalid video input asset id: %w", err)
		}
		role := domainvideo.InputRole(input.Role)
		if role != domainvideo.InputRoleFirstFrame && role != domainvideo.InputRoleLastFrame {
			continue
		}
		request.Inputs = append(request.Inputs, videotaskservice.InputRequest{AssetID: assetID, Role: role, Ordinal: input.Ordinal})
	}
	bindings, err := promptReferenceBindings(prompt.ReferenceNames, submission.Inputs)
	if err != nil {
		return request, err
	}
	request.ReferenceBindings = make([]videotaskservice.ReferenceBinding, 0, len(bindings))
	for _, binding := range bindings {
		assetID, parseErr := uuid.Parse(binding.AssetID)
		if parseErr != nil {
			return request, fmt.Errorf("invalid prompt reference asset id: %w", parseErr)
		}
		request.ReferenceBindings = append(request.ReferenceBindings, videotaskservice.ReferenceBinding{Name: binding.Name, AssetID: assetID})
	}
	return request, nil
}

func generationDraft(payload json.RawMessage) json.RawMessage {
	var envelope struct {
		Draft json.RawMessage `json:"draft"`
	}
	if json.Unmarshal(payload, &envelope) == nil && len(envelope.Draft) > 0 && string(envelope.Draft) != "null" {
		return envelope.Draft
	}
	return payload
}

type selectedPrompt struct {
	Template       string
	Variables      map[string]string
	VariableNames  []string
	ReferenceNames []string
}

func promptFromInputs(submission GenerationSubmission) (selectedPrompt, error) {
	var envelope struct {
		ActivePromptNodeID string `json:"active_prompt_node_id"`
	}
	if err := json.Unmarshal(submission.Node.Payload, &envelope); err != nil {
		return selectedPrompt{}, fmt.Errorf("decode generation node prompt selection: %w", err)
	}
	prompts := make([]GenerationInput, 0, 1)
	for _, input := range submission.Inputs {
		if input.Role == domaincanvas.InputRolePrompt {
			prompts = append(prompts, input)
		}
	}
	if len(prompts) == 0 {
		return selectedPrompt{}, nil
	}
	if len(prompts) > 1 && envelope.ActivePromptNodeID == "" {
		return selectedPrompt{}, errors.New("image generation node must select one connected prompt")
	}
	selected := prompts[0]
	if envelope.ActivePromptNodeID != "" {
		found := false
		for _, input := range prompts {
			if input.Node.ID == envelope.ActivePromptNodeID {
				selected, found = input, true
				break
			}
		}
		if !found {
			return selectedPrompt{}, errors.New("selected prompt is not connected to the generation node")
		}
	}
	var payload struct {
		Text      string            `json:"text"`
		Prompt    string            `json:"prompt"`
		Template  string            `json:"template"`
		Variables map[string]string `json:"variables"`
	}
	if err := json.Unmarshal(selected.Node.Payload, &payload); err != nil {
		return selectedPrompt{}, fmt.Errorf("decode selected prompt node: %w", err)
	}
	result := selectedPrompt{Variables: payload.Variables}
	for _, value := range []string{payload.Template, payload.Prompt, payload.Text} {
		if value != "" {
			result.Template = value
			break
		}
	}
	if result.Template == "" {
		return result, nil
	}
	document, err := prompttemplate.Parse(result.Template, prompttemplate.DefaultLimits())
	if err != nil {
		return selectedPrompt{}, fmt.Errorf("parse selected prompt template: %w", err)
	}
	result.Template = document.Canonical
	result.VariableNames = document.VariableNames
	result.ReferenceNames = document.ReferenceNames
	return result, nil
}

func promptReferenceBindings(names []string, inputs []GenerationInput) ([]prompttemplate.ReferenceBinding, error) {
	if len(names) == 0 {
		return nil, nil
	}
	assetsByName := make(map[string]string)
	for _, input := range inputs {
		if input.Role != domaincanvas.InputRoleReference && input.Role != domaincanvas.InputRoleFirstFrame && input.Role != domaincanvas.InputRoleLastFrame {
			continue
		}
		var payload struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(input.Node.Payload, &payload) != nil || payload.Name == "" || input.Node.AssetID == "" {
			continue
		}
		name, err := prompttemplate.NormalizeName(payload.Name, prompttemplate.DefaultLimits().MaxNameRunes)
		if err != nil {
			continue
		}
		if existing, exists := assetsByName[name]; exists && existing != input.Node.AssetID {
			return nil, fmt.Errorf("multiple connected assets are named %q", name)
		}
		assetsByName[name] = input.Node.AssetID
	}
	bindings := make([]prompttemplate.ReferenceBinding, 0, len(names))
	for _, name := range names {
		assetID := assetsByName[name]
		if assetID == "" {
			return nil, fmt.Errorf("prompt reference %q has no connected asset with the same name", name)
		}
		bindings = append(bindings, prompttemplate.ReferenceBinding{Name: name, AssetID: assetID})
	}
	return bindings, nil
}

func imageRunStatus(status string) RunStatus {
	switch status {
	case domainimagetask.StatusQueued:
		return RunStatusQueued
	case domainimagetask.StatusRunning:
		return RunStatusRunning
	case domainimagetask.StatusSucceeded, domainimagetask.StatusPartialFailed:
		return RunStatusSucceeded
	case domainimagetask.StatusFailed, domainimagetask.StatusRejected, domainimagetask.StatusDeleted:
		return RunStatusFailed
	default:
		return RunStatusRunning
	}
}

func videoRunStatus(status domainvideo.TaskStatus) RunStatus {
	switch status {
	case domainvideo.TaskStatusQueued:
		return RunStatusQueued
	case domainvideo.TaskStatusRunning:
		return RunStatusRunning
	case domainvideo.TaskStatusSaving:
		return RunStatusSaving
	case domainvideo.TaskStatusSucceeded, domainvideo.TaskStatusPartial:
		return RunStatusSucceeded
	case domainvideo.TaskStatusCancelled:
		return RunStatusCanceled
	case domainvideo.TaskStatusFailed:
		return RunStatusFailed
	default:
		return RunStatusRunning
	}
}

var _ ImageTaskPort = (*imagetaskservice.Service)(nil)
var _ VideoTaskPort = (*videotaskservice.Service)(nil)
