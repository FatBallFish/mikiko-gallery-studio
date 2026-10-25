package videotask

import (
	"context"
	"encoding/json"
	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
)

type historyStore interface {
	HideHistory(context.Context, int64, uuid.UUID) error
	ClearFailedHistory(context.Context, int64, string) (int, error)
}

func (s *Service) HideHistory(ctx context.Context, userID int64, id uuid.UUID) error {
	store, ok := s.store.(historyStore)
	if !ok {
		return errs.Internal("video history is unavailable")
	}
	return mapOwnershipError(store.HideHistory(ctx, userID, id), "video task not found")
}
func (s *Service) ClearFailedHistory(ctx context.Context, userID int64, project string) (int, error) {
	store, ok := s.store.(historyStore)
	if !ok {
		return 0, errs.Internal("video history is unavailable")
	}
	return store.ClearFailedHistory(ctx, userID, project)
}
func (s *Service) Retry(ctx context.Context, userID int64, id uuid.UUID, groups []string) (Task, error) {
	original, err := s.Get(ctx, userID, id)
	if err != nil {
		return Task{}, err
	}
	if original.Status != domainvideo.TaskStatusFailed && original.Status != domainvideo.TaskStatusPartial {
		return Task{}, errs.New(409, errs.CodeConflict, "only failed video tasks can be retried")
	}
	req, err := videoRetryRequest(original)
	if err != nil {
		return Task{}, err
	}
	req.UserID = userID
	req.UserGroupCodes = append([]string(nil), groups...)
	req.IdempotencyKey = uuid.NewString()
	estimate, err := s.Estimate(ctx, req)
	if err != nil {
		return Task{}, err
	}
	req.QuoteToken = estimate.QuoteToken
	task, _, err := s.Create(ctx, req)
	return task, err
}
func videoRetryRequest(task Task) (CreateRequest, error) {
	req := CreateRequest{UserID: task.UserID, ProjectID: task.ProjectID, RouteModelCode: task.RouteModelCode, TaskType: task.TaskType, PromptTemplate: task.PromptTemplate, DurationSeconds: task.DurationSeconds, Resolution: task.Resolution, AspectRatio: task.AspectRatio, AudioMode: domainvideo.AudioModeSilent, OutputCount: task.RequestedOutputCount, SourceChannel: task.SourceChannel, SourceCanvasID: task.SourceCanvasID, SourceCanvasNodeID: task.SourceCanvasNodeID}
	if task.GenerateAudio {
		req.AudioMode = domainvideo.AudioModeGenerated
	}
	raw, err := json.Marshal(task.PromptBindingSnapshot)
	if err != nil {
		return CreateRequest{}, err
	}
	var bindings struct {
		Variables  []VariableBinding  `json:"variables"`
		References []ReferenceBinding `json:"references"`
	}
	if err := json.Unmarshal(raw, &bindings); err != nil {
		return CreateRequest{}, errs.BadRequest("historical video bindings are unavailable; restore inputs before retrying")
	}
	req.PromptVariables = bindings.Variables
	req.ReferenceBindings = bindings.References
	for _, input := range task.Inputs {
		req.Inputs = append(req.Inputs, InputRequest{AssetID: input.AssetID, Role: domainvideo.InputRole(input.Role), Ordinal: input.Ordinal})
	}
	return req, nil
}
