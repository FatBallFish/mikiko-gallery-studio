package entstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videotask"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videotaskinput"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videotaskitem"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	videotaskservice "github.com/fatballfish/pic-gallery/internal/service/videotask"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type VideoTaskCreate struct {
	ID                    uuid.UUID
	UserID                int64
	ProjectID             uuid.UUID
	APIKeyID              *int64
	SourceChannel         string
	SourceCanvasID        *uuid.UUID
	SourceCanvasNodeID    *string
	TaskType              string
	PromptTemplate        string
	PromptBindingSnapshot map[string]any
	ExecutionPrompt       string
	RouteModelID          int64
	RouteModelCode        string
	DurationSeconds       int
	Resolution            string
	AspectRatio           string
	GenerateAudio         bool
	RequestedOutputCount  int
	EstimatedPoints       string
	ReservedPoints        string
	PricingSnapshot       map[string]any
	RoutingSnapshot       map[string]any
	IdempotencyKey        string
	RequestFingerprint    string
}

func (s *VideoTaskStore) FindByIdempotency(ctx context.Context, userID int64, key string) (videotaskservice.Task, bool, error) {
	entity, err := s.queryTask(userID).Where(videotask.IdempotencyKeyEQ(strings.TrimSpace(key))).Only(ctx)
	if repoent.IsNotFound(err) {
		return videotaskservice.Task{}, false, nil
	}
	if err != nil {
		return videotaskservice.Task{}, false, err
	}
	return mapVideoTask(entity), true, nil
}

func (s *VideoTaskStore) Create(ctx context.Context, record videotaskservice.CreateRecord) (videotaskservice.Task, bool, error) {
	task := record.Task
	entity, err := s.CreateWithReservation(ctx, CreateVideoTaskWithReservationRequest{
		Task: VideoTaskCreate{
			ID: task.ID, UserID: task.UserID, ProjectID: task.ProjectID, SourceChannel: task.SourceChannel, SourceCanvasID: task.SourceCanvasID,
			SourceCanvasNodeID: optionalString(task.SourceCanvasNodeID), TaskType: string(task.TaskType), PromptTemplate: task.PromptTemplate,
			PromptBindingSnapshot: cloneMap(task.PromptBindingSnapshot), ExecutionPrompt: task.ExecutionPrompt, RouteModelID: task.RouteModelID,
			RouteModelCode: task.RouteModelCode, DurationSeconds: task.DurationSeconds, Resolution: string(task.Resolution), AspectRatio: string(task.AspectRatio),
			GenerateAudio: task.GenerateAudio, RequestedOutputCount: task.RequestedOutputCount, EstimatedPoints: task.EstimatedPoints, ReservedPoints: task.ReservedPoints,
			PricingSnapshot: cloneMap(task.PricingSnapshot), RoutingSnapshot: cloneMap(task.RoutingSnapshot), IdempotencyKey: task.IdempotencyKey, RequestFingerprint: task.RequestFingerprint,
		},
		Items: mapVideoItemCreates(task.Items), Inputs: mapVideoInputCreates(record.Inputs),
		Reserve: billingservice.ReserveStoreRequest{UserID: task.UserID, TaskID: task.ID.String(), EstimatedPoints: record.ReservePoints, Reason: record.ReserveReason},
	})
	if err != nil {
		if repoent.IsConstraintError(err) {
			if existing, found, findErr := s.FindByIdempotency(ctx, task.UserID, task.IdempotencyKey); findErr == nil && found {
				if existing.RequestFingerprint != task.RequestFingerprint {
					return videotaskservice.Task{}, false, videotaskservice.ErrIdempotencyConflict
				}
				return existing, true, nil
			}
		}
		return videotaskservice.Task{}, false, err
	}
	created, err := s.Get(ctx, task.UserID, entity.ID)
	return created, false, err
}

func (s *VideoTaskStore) List(ctx context.Context, req videotaskservice.ListRequest) (videotaskservice.Page, error) {
	query := s.queryTask(req.UserID)
	if req.ProjectID != nil {
		query.Where(videotask.ProjectIDEQ(*req.ProjectID))
	}
	if strings.TrimSpace(req.Status) != "" {
		query.Where(videotask.StatusEQ(strings.TrimSpace(req.Status)))
	}
	if cursor := strings.TrimSpace(req.Cursor); cursor != "" {
		cursorID, err := uuid.Parse(cursor)
		if err != nil {
			return videotaskservice.Page{}, errs.BadRequest("invalid video task cursor")
		}
		anchor, err := s.client.VideoTask.Query().Where(
			videotask.IDEQ(cursorID), videotask.UserIDEQ(req.UserID), videotask.DeletedAtIsNil(),
		).Only(ctx)
		if repoent.IsNotFound(err) {
			return videotaskservice.Page{}, errs.BadRequest("invalid video task cursor")
		}
		if err != nil {
			return videotaskservice.Page{}, err
		}
		query.Where(videotask.Or(
			videotask.CreatedAtLT(anchor.CreatedAt),
			videotask.And(videotask.CreatedAtEQ(anchor.CreatedAt), videotask.IDLT(anchor.ID)),
		))
	}
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	entities, err := query.Order(videotask.ByCreatedAt(sql.OrderDesc()), videotask.ByID(sql.OrderDesc())).Limit(limit + 1).All(ctx)
	if err != nil {
		return videotaskservice.Page{}, err
	}
	page := videotaskservice.Page{}
	if len(entities) > limit {
		page.NextCursor = entities[limit-1].ID.String()
		entities = entities[:limit]
	}
	for _, entity := range entities {
		page.Items = append(page.Items, mapVideoTask(entity))
	}
	return page, nil
}

func (s *VideoTaskStore) Get(ctx context.Context, userID int64, id uuid.UUID) (videotaskservice.Task, error) {
	entity, err := s.queryTask(userID).Where(videotask.IDEQ(id)).Only(ctx)
	if repoent.IsNotFound(err) {
		return videotaskservice.Task{}, repoerr.ErrNotFound
	}
	if err != nil {
		return videotaskservice.Task{}, err
	}
	return mapVideoTask(entity), nil
}

func (s *VideoTaskStore) RequestCancel(ctx context.Context, userID int64, id uuid.UUID, key string) (videotaskservice.Task, error) {
	_ = key
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (videotaskservice.Task, error) {
		entity, err := tx.VideoTask.Query().Where(videotask.IDEQ(id), videotask.UserIDEQ(userID), videotask.DeletedAtIsNil()).WithItems().WithInputs().Only(ctx)
		if repoent.IsNotFound(err) {
			return videotaskservice.Task{}, repoerr.ErrNotFound
		}
		if err != nil {
			return videotaskservice.Task{}, err
		}
		changed := false
		for _, item := range entity.Edges.Items {
			current := domainvideo.ItemStateSnapshot{State: domainvideo.ItemState(item.Status), Version: item.Version}
			if current.State == domainvideo.ItemStateCancelRequested || isTerminalVideoItem(current.State) {
				continue
			}
			transition, transitionErr := domainvideo.AdvanceItemState(current, domainvideo.ItemTransition{ExpectedVersion: current.Version, Target: domainvideo.ItemStateCancelRequested})
			if transitionErr != nil {
				continue
			}
			if _, err := tx.VideoTaskItem.Update().Where(videotaskitem.IDEQ(item.ID), videotaskitem.VersionEQ(item.Version)).SetStatus(string(transition.Snapshot.State)).SetStage("cancel_requested").SetVersion(transition.Snapshot.Version).Save(ctx); err != nil {
				return videotaskservice.Task{}, err
			}
			changed = changed || transition.Changed
		}
		if changed {
			if _, err := tx.VideoTask.UpdateOne(entity).SetStatus(string(domainvideo.TaskStatusRunning)).SetProgressStage("cancel_requested").Save(ctx); err != nil {
				return videotaskservice.Task{}, err
			}
		}
		refreshed, err := tx.VideoTask.Query().Where(videotask.IDEQ(id)).WithItems().WithInputs().Only(ctx)
		if err != nil {
			return videotaskservice.Task{}, err
		}
		return mapVideoTask(refreshed), nil
	})
}

func (s *VideoTaskStore) queryTask(userID int64) *repoent.VideoTaskQuery {
	return s.client.VideoTask.Query().Where(videotask.UserIDEQ(userID), videotask.DeletedAtIsNil()).WithItems(func(query *repoent.VideoTaskItemQuery) {
		query.Order(videotaskitem.ByOrdinal())
	}).WithInputs(func(query *repoent.VideoTaskInputQuery) {
		query.Order(videotaskinput.ByOrdinal())
	})
}

type VideoTaskItemCreate struct {
	ID      uuid.UUID
	Ordinal int
}

type VideoTaskInputCreate struct {
	ID            uuid.UUID
	AssetID       uuid.UUID
	Role          string
	Ordinal       int
	AssetSnapshot map[string]any
}

type CreateVideoTaskWithReservationRequest struct {
	Task    VideoTaskCreate
	Items   []VideoTaskItemCreate
	Inputs  []VideoTaskInputCreate
	Reserve billingservice.ReserveStoreRequest
}

type FinalizeVideoTaskRequest struct {
	TaskID             uuid.UUID
	UserID             int64
	Status             string
	SuccessOutputCount int
	ActualPoints       string
	UsageSummary       map[string]any
}

type videoTaskBillingTx interface {
	ReserveTaskTx(context.Context, *repoent.Tx, billingservice.ReserveStoreRequest, string) (billingservice.BalanceState, error)
	FinalizeTaskTx(context.Context, *repoent.Tx, billingservice.FinalizeStoreRequest, string, map[string]any) (billingservice.BalanceState, error)
}

type VideoTaskStore struct {
	client  *repoent.Client
	billing videoTaskBillingTx
}

func NewVideoTaskStore(client *repoent.Client, billing videoTaskBillingTx) *VideoTaskStore {
	return &VideoTaskStore{client: client, billing: billing}
}

func (s *VideoTaskStore) CreateWithReservation(ctx context.Context, req CreateVideoTaskWithReservationRequest) (*repoent.VideoTask, error) {
	if err := validateVideoTaskCreateWithReservation(req); err != nil {
		return nil, err
	}
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (*repoent.VideoTask, error) {
		task, err := createVideoTask(ctx, tx, req.Task)
		if err != nil {
			return nil, fmt.Errorf("create video task: %w", err)
		}
		if _, err := s.billing.ReserveTaskTx(ctx, tx, req.Reserve, "video"); err != nil {
			return nil, fmt.Errorf("reserve video task points: %w", err)
		}
		for _, item := range req.Items {
			builder := tx.VideoTaskItem.Create().SetTaskID(task.ID).SetOrdinal(item.Ordinal)
			if item.ID != uuid.Nil {
				builder.SetID(item.ID)
			}
			if _, err := builder.Save(ctx); err != nil {
				return nil, fmt.Errorf("create video task item %d: %w", item.Ordinal, err)
			}
		}
		for _, input := range req.Inputs {
			builder := tx.VideoTaskInput.Create().
				SetTaskID(task.ID).
				SetAssetID(input.AssetID).
				SetRole(strings.TrimSpace(input.Role)).
				SetOrdinal(input.Ordinal).
				SetAssetSnapshot(cloneMap(input.AssetSnapshot))
			if input.ID != uuid.Nil {
				builder.SetID(input.ID)
			}
			if _, err := builder.Save(ctx); err != nil {
				return nil, fmt.Errorf("create video task input %s/%d: %w", input.Role, input.Ordinal, err)
			}
			if _, err := tx.MediaAssetReference.Create().
				SetAssetID(input.AssetID).
				SetRefType("video_task_input").
				SetRefID(task.ID).
				SetRefKey(fmt.Sprintf("%s:%d", strings.TrimSpace(input.Role), input.Ordinal)).
				SetUserID(task.UserID).
				Save(ctx); err != nil {
				return nil, fmt.Errorf("create video task media reference %s/%d: %w", input.Role, input.Ordinal, err)
			}
		}
		return task, nil
	})
}

func (s *VideoTaskStore) FinalizeWithBilling(ctx context.Context, req FinalizeVideoTaskRequest) (*repoent.VideoTask, error) {
	if req.TaskID == uuid.Nil || req.UserID <= 0 {
		return nil, errs.BadRequest("video task id and user id are required")
	}
	if strings.TrimSpace(req.Status) == "" {
		return nil, errs.BadRequest("video task status is required")
	}
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (*repoent.VideoTask, error) {
		task, err := tx.VideoTask.Query().Where(videotask.IDEQ(req.TaskID), videotask.UserIDEQ(req.UserID)).Only(ctx)
		if err != nil {
			if repoent.IsNotFound(err) {
				return nil, errs.New(404, errs.CodeNotFound, "video task not found")
			}
			return nil, err
		}
		if task.SettlementStatus == "finalized" {
			return task, nil
		}
		apiKeyID := int64(0)
		if task.APIKeyID != nil {
			apiKeyID = *task.APIKeyID
		}
		if _, err := s.billing.FinalizeTaskTx(ctx, tx, billingservice.FinalizeStoreRequest{
			UserID: req.UserID, APIKeyID: apiKeyID, TaskID: task.ID.String(), EstimatedPoints: task.ReservedPoints,
			ActualPoints: req.ActualPoints, Reason: "video generation finalize",
		}, "video", req.UsageSummary); err != nil {
			return nil, fmt.Errorf("finalize video task points: %w", err)
		}
		finishedAt := time.Now().UTC()
		updated, err := tx.VideoTask.UpdateOne(task).
			SetStatus(strings.TrimSpace(req.Status)).
			SetProgressStage("completed").
			SetSuccessOutputCount(req.SuccessOutputCount).
			SetActualPoints(req.ActualPoints).
			SetSettlementStatus("finalized").
			SetFinishedAt(finishedAt).
			Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("mark video task finalized: %w", err)
		}
		return updated, nil
	})
}

func validateVideoTaskCreateWithReservation(req CreateVideoTaskWithReservationRequest) error {
	if req.Task.ID == uuid.Nil || req.Task.UserID <= 0 || req.Task.ProjectID == uuid.Nil {
		return errs.BadRequest("video task id, user id and project id are required")
	}
	if req.Reserve.UserID != req.Task.UserID || strings.TrimSpace(req.Reserve.TaskID) != req.Task.ID.String() {
		return errs.BadRequest("video task reservation identity does not match task")
	}
	if strings.TrimSpace(req.Task.TaskType) == "" || strings.TrimSpace(req.Task.RouteModelCode) == "" || req.Task.RouteModelID <= 0 {
		return errs.BadRequest("video task type and route model are required")
	}
	if req.Task.DurationSeconds <= 0 || strings.TrimSpace(req.Task.Resolution) == "" || strings.TrimSpace(req.Task.AspectRatio) == "" {
		return errs.BadRequest("video duration, resolution and aspect ratio are required")
	}
	if req.Task.RequestedOutputCount < 1 || req.Task.RequestedOutputCount > 4 || len(req.Items) != req.Task.RequestedOutputCount {
		return errs.BadRequest("video task items must match requested output count")
	}
	if strings.TrimSpace(req.Task.IdempotencyKey) == "" || strings.TrimSpace(req.Task.RequestFingerprint) == "" {
		return errs.BadRequest("video task idempotency key and fingerprint are required")
	}
	return nil
}

func createVideoTask(ctx context.Context, tx *repoent.Tx, task VideoTaskCreate) (*repoent.VideoTask, error) {
	sourceChannel := strings.TrimSpace(task.SourceChannel)
	if sourceChannel == "" {
		sourceChannel = "web"
	}
	builder := tx.VideoTask.Create().
		SetID(task.ID).
		SetUserID(task.UserID).
		SetProjectID(task.ProjectID).
		SetNillableAPIKeyID(task.APIKeyID).
		SetSourceChannel(sourceChannel).
		SetNillableSourceCanvasID(task.SourceCanvasID).
		SetNillableSourceCanvasNodeID(task.SourceCanvasNodeID).
		SetTaskType(strings.TrimSpace(task.TaskType)).
		SetPromptTemplate(task.PromptTemplate).
		SetPromptBindingSnapshot(cloneMap(task.PromptBindingSnapshot)).
		SetExecutionPrompt(task.ExecutionPrompt).
		SetRouteModelID(task.RouteModelID).
		SetRouteModelCode(strings.TrimSpace(task.RouteModelCode)).
		SetDurationSeconds(task.DurationSeconds).
		SetResolution(strings.TrimSpace(task.Resolution)).
		SetAspectRatio(strings.TrimSpace(task.AspectRatio)).
		SetGenerateAudio(task.GenerateAudio).
		SetRequestedOutputCount(task.RequestedOutputCount).
		SetEstimatedPoints(task.EstimatedPoints).
		SetReservedPoints(task.ReservedPoints).
		SetPricingSnapshot(cloneMap(task.PricingSnapshot)).
		SetRoutingSnapshot(cloneMap(task.RoutingSnapshot)).
		SetIdempotencyKey(strings.TrimSpace(task.IdempotencyKey)).
		SetRequestFingerprint(strings.TrimSpace(task.RequestFingerprint))
	return builder.Save(ctx)
}

func mapVideoItemCreates(items []videotaskservice.Item) []VideoTaskItemCreate {
	result := make([]VideoTaskItemCreate, 0, len(items))
	for _, item := range items {
		result = append(result, VideoTaskItemCreate{ID: item.ID, Ordinal: item.Ordinal})
	}
	return result
}

func mapVideoInputCreates(inputs []videotaskservice.CreateInputRecord) []VideoTaskInputCreate {
	result := make([]VideoTaskInputCreate, 0, len(inputs))
	for _, input := range inputs {
		result = append(result, VideoTaskInputCreate{ID: input.ID, AssetID: input.AssetID, Role: input.Role, Ordinal: input.Ordinal, AssetSnapshot: cloneMap(input.AssetSnapshot)})
	}
	return result
}

func mapVideoTask(entity *repoent.VideoTask) videotaskservice.Task {
	result := videotaskservice.Task{
		ID: entity.ID, UserID: entity.UserID, ProjectID: entity.ProjectID, SourceChannel: entity.SourceChannel, SourceCanvasID: entity.SourceCanvasID,
		TaskType: domainvideo.TaskType(entity.TaskType), Status: domainvideo.TaskStatus(entity.Status), ProgressStage: entity.ProgressStage,
		ProgressMessage: entity.ProgressMessage, PromptTemplate: entity.PromptTemplate, PromptBindingSnapshot: cloneMap(entity.PromptBindingSnapshot),
		ExecutionPrompt: entity.ExecutionPrompt, RouteModelID: entity.RouteModelID, RouteModelCode: entity.RouteModelCode,
		DurationSeconds: entity.DurationSeconds, Resolution: domainvideo.Resolution(entity.Resolution), AspectRatio: domainvideo.AspectRatio(entity.AspectRatio),
		GenerateAudio: entity.GenerateAudio, RequestedOutputCount: entity.RequestedOutputCount, SuccessOutputCount: entity.SuccessOutputCount,
		EstimatedPoints: entity.EstimatedPoints, ReservedPoints: entity.ReservedPoints, ActualPoints: entity.ActualPoints,
		PricingSnapshot: cloneMap(entity.PricingSnapshot), RoutingSnapshot: cloneMap(entity.RoutingSnapshot), SettlementStatus: entity.SettlementStatus,
		IdempotencyKey: entity.IdempotencyKey, RequestFingerprint: entity.RequestFingerprint, Version: entity.UpdatedAt.UnixNano(),
		CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt, StartedAt: entity.StartedAt, FinishedAt: entity.FinishedAt,
	}
	if entity.SourceCanvasNodeID != nil {
		result.SourceCanvasNodeID = *entity.SourceCanvasNodeID
	}
	for _, item := range entity.Edges.Items {
		result.Items = append(result.Items, videotaskservice.Item{ID: item.ID, Ordinal: item.Ordinal, Status: domainvideo.ItemState(item.Status), Stage: item.Stage,
			ResultAssetID: item.ResultAssetID, ActualOutputSeconds: item.ActualOutputSeconds, ActualPoints: item.ActualPoints,
			ErrorCode: optionalStringValue(item.ErrorCode), ErrorMessage: optionalStringValue(item.ErrorMessage), NextActionAt: item.NextActionAt, Version: item.Version})
	}
	for _, input := range entity.Edges.Inputs {
		result.Inputs = append(result.Inputs, videotaskservice.Input{ID: input.ID, AssetID: input.AssetID, Role: input.Role, Ordinal: input.Ordinal, AssetSnapshot: cloneMap(input.AssetSnapshot)})
	}
	return result
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func isTerminalVideoItem(state domainvideo.ItemState) bool {
	switch state {
	case domainvideo.ItemStateSucceeded, domainvideo.ItemStateFailed, domainvideo.ItemStateCancelled:
		return true
	default:
		return false
	}
}
