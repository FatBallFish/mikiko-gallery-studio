package entstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/videotask"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
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
