package videotask

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
)

var ErrIdempotencyConflict = errors.New("video idempotency key was already used for another request")

type Item struct {
	ID                  uuid.UUID             `json:"id"`
	Ordinal             int                   `json:"ordinal"`
	Status              domainvideo.ItemState `json:"status"`
	Stage               string                `json:"stage"`
	ResultAssetID       *uuid.UUID            `json:"result_asset_id,omitempty"`
	ActualOutputSeconds string                `json:"actual_output_seconds"`
	ActualPoints        string                `json:"actual_points"`
	ErrorCode           string                `json:"error_code,omitempty"`
	ErrorMessage        string                `json:"error_message,omitempty"`
	NextActionAt        *time.Time            `json:"next_action_at,omitempty"`
	Version             int64                 `json:"version"`
}

type Input struct {
	ID            uuid.UUID      `json:"id"`
	AssetID       uuid.UUID      `json:"asset_id"`
	Role          string         `json:"role"`
	Ordinal       int            `json:"ordinal"`
	AssetSnapshot map[string]any `json:"asset_snapshot"`
}

type Task struct {
	ID                    uuid.UUID               `json:"id"`
	UserID                int64                   `json:"-"`
	ProjectID             uuid.UUID               `json:"project_id"`
	SourceChannel         string                  `json:"source_channel"`
	SourceCanvasID        *uuid.UUID              `json:"source_canvas_id,omitempty"`
	SourceCanvasNodeID    string                  `json:"source_canvas_node_id,omitempty"`
	TaskType              domainvideo.TaskType    `json:"task_type"`
	Status                domainvideo.TaskStatus  `json:"status"`
	ProgressStage         string                  `json:"progress_stage"`
	ProgressMessage       string                  `json:"progress_message,omitempty"`
	PromptTemplate        string                  `json:"prompt_template"`
	PromptBindingSnapshot map[string]any          `json:"prompt_binding_snapshot"`
	ExecutionPrompt       string                  `json:"execution_prompt"`
	RouteModelID          int64                   `json:"route_model_id"`
	RouteModelCode        string                  `json:"route_model_code"`
	DurationSeconds       int                     `json:"duration_seconds"`
	Resolution            domainvideo.Resolution  `json:"resolution"`
	AspectRatio           domainvideo.AspectRatio `json:"aspect_ratio"`
	GenerateAudio         bool                    `json:"generate_audio"`
	RequestedOutputCount  int                     `json:"requested_output_count"`
	SuccessOutputCount    int                     `json:"success_output_count"`
	EstimatedPoints       string                  `json:"estimated_points"`
	ReservedPoints        string                  `json:"reserved_points"`
	ActualPoints          string                  `json:"actual_points"`
	PricingSnapshot       map[string]any          `json:"pricing_snapshot"`
	RoutingSnapshot       map[string]any          `json:"routing_snapshot"`
	SettlementStatus      string                  `json:"settlement_status"`
	IdempotencyKey        string                  `json:"-"`
	RequestFingerprint    string                  `json:"-"`
	Items                 []Item                  `json:"items"`
	Inputs                []Input                 `json:"inputs"`
	Version               int64                   `json:"version"`
	CreatedAt             time.Time               `json:"created_at"`
	UpdatedAt             time.Time               `json:"updated_at"`
	StartedAt             *time.Time              `json:"started_at,omitempty"`
	FinishedAt            *time.Time              `json:"finished_at,omitempty"`
}

type CreateInputRecord struct {
	ID            uuid.UUID
	AssetID       uuid.UUID
	Role          string
	Ordinal       int
	AssetSnapshot map[string]any
}

type CreateRecord struct {
	Task          Task
	Inputs        []CreateInputRecord
	ReservePoints string
	ReserveReason string
}

type ListRequest struct {
	UserID    int64
	ProjectID *uuid.UUID
	Status    string
	Cursor    string
	Limit     int
}

type Page struct {
	Items      []Task `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type Store interface {
	FindByIdempotency(context.Context, int64, string) (Task, bool, error)
	Create(context.Context, CreateRecord) (Task, bool, error)
	List(context.Context, ListRequest) (Page, error)
	Get(context.Context, int64, uuid.UUID) (Task, error)
	RequestCancel(context.Context, int64, uuid.UUID, string) (Task, error)
}

type AssetReader interface {
	GetAsset(context.Context, int64, uuid.UUID) (mediaassetservice.Asset, error)
}

type QuoteVerifier interface {
	Verify(context.Context, int64, EstimateRequest, string) (Estimate, error)
}
