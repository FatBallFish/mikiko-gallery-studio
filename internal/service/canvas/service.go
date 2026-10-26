package canvas

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	domaincanvas "github.com/fatballfish/pic-gallery/internal/domain/canvas"
	mediaerrors "github.com/fatballfish/pic-gallery/internal/service/mediaerrors"
	"github.com/google/uuid"
)

type CanvasStatus string

const (
	CanvasStatusActive  CanvasStatus = "active"
	CanvasStatusDeleted CanvasStatus = "deleted"
)

type Template string

const (
	TemplateBlank            Template = "blank"
	TemplateImageExploration Template = "image_exploration"
	TemplateImageToVideo     Template = "image_to_video"
)

type TaskKind string

const (
	TaskKindImage TaskKind = "image"
	TaskKindVideo TaskKind = "video"
)

type RunStatus string

const (
	RunStatusSubmitting RunStatus = "submitting"
	RunStatusQueued     RunStatus = "queued"
	RunStatusRunning    RunStatus = "running"
	RunStatusSaving     RunStatus = "saving"
	RunStatusSucceeded  RunStatus = "succeeded"
	RunStatusFailed     RunStatus = "failed"
	RunStatusCanceled   RunStatus = "canceled"
	RunStatusAttached   RunStatus = "attached"
	RunStatusUnplaced   RunStatus = "unplaced"
)

func (s RunStatus) Active() bool {
	return s == RunStatusSubmitting || s == RunStatusQueued || s == RunStatusRunning || s == RunStatusSaving
}

type Canvas struct {
	ID                uuid.UUID               `json:"id"`
	UserID            int64                   `json:"-"`
	ProjectID         uuid.UUID               `json:"project_id"`
	Name              string                  `json:"name"`
	SchemaVersion     int                     `json:"schema_version"`
	Revision          int64                   `json:"revision"`
	MetadataVersion   int64                   `json:"metadata_version"`
	Document          domaincanvas.DocumentV1 `json:"document"`
	DocumentBytes     int                     `json:"document_bytes"`
	NodeCount         int                     `json:"node_count"`
	EdgeCount         int                     `json:"edge_count"`
	AssetReferences   []uuid.UUID             `json:"asset_references"`
	RunningTaskCount  int                     `json:"running_task_count"`
	FailedTaskCount   int                     `json:"failed_task_count"`
	Status            CanvasStatus            `json:"status"`
	LastTransferredAt time.Time               `json:"last_transferred_at,omitempty"`
	LastSavedAt       time.Time               `json:"last_saved_at"`
	CreatedAt         time.Time               `json:"created_at"`
	UpdatedAt         time.Time               `json:"updated_at"`
}
type Run struct {
	ProgressStage     string          `json:"progress_stage,omitempty"`
	ProgressMessage   string          `json:"progress_message,omitempty"`
	ID                uuid.UUID       `json:"id"`
	CanvasID          uuid.UUID       `json:"canvas_id"`
	UserID            int64           `json:"-"`
	NodeID            string          `json:"node_id"`
	SubmittedRevision int64           `json:"submitted_revision"`
	TaskKind          TaskKind        `json:"task_kind"`
	TaskID            uuid.UUID       `json:"task_id"`
	NodeSnapshot      json.RawMessage `json:"node_snapshot"`
	Status            RunStatus       `json:"status"`
	ResultAssetIDs    []uuid.UUID     `json:"result_asset_ids"`
	AttachedRevision  *int64          `json:"attached_revision,omitempty"`
	IdempotencyKey    string          `json:"-"`
	ErrorCode         string          `json:"error_code,omitempty"`
	ErrorMessage      string          `json:"error_message,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}
type Summary struct {
	NodeCount        int `json:"node_count"`
	EdgeCount        int `json:"edge_count"`
	RunningTaskCount int `json:"running_task_count"`
}
type RevisionConflictError struct {
	RemoteRevision  int64     `json:"remote_revision"`
	RemoteUpdatedAt time.Time `json:"remote_updated_at"`
	Summary         Summary   `json:"summary"`
}

func (e *RevisionConflictError) Error() string { return "canvas document revision changed" }

type ListRequest struct {
	UserID    int64
	ProjectID *uuid.UUID
	Search    string
}
type CreateRequest struct {
	UserID    int64                    `json:"-"`
	ProjectID uuid.UUID                `json:"project_id"`
	Name      string                   `json:"name"`
	Template  Template                 `json:"template"`
	Document  *domaincanvas.DocumentV1 `json:"document,omitempty"`
}
type RenameRequest struct {
	UserID                  int64     `json:"-"`
	CanvasID                uuid.UUID `json:"-"`
	Name                    string    `json:"name"`
	ExpectedMetadataVersion int64     `json:"expected_metadata_version"`
}
type DuplicateRequest struct {
	UserID    int64      `json:"-"`
	CanvasID  uuid.UUID  `json:"-"`
	Name      string     `json:"name"`
	ProjectID *uuid.UUID `json:"project_id,omitempty"`
}
type SaveDocumentRequest struct {
	UserID           int64                   `json:"-"`
	CanvasID         uuid.UUID               `json:"-"`
	ExpectedRevision int64                   `json:"expected_revision"`
	Document         domaincanvas.DocumentV1 `json:"document"`
}
type DeleteRequest struct {
	UserID                  int64     `json:"-"`
	CanvasID                uuid.UUID `json:"-"`
	ExpectedMetadataVersion int64     `json:"expected_metadata_version"`
}
type TransferProjectRequest struct {
	UserID                  int64     `json:"-"`
	CanvasID                uuid.UUID `json:"-"`
	TargetProjectID         uuid.UUID `json:"target_project_id"`
	ExpectedMetadataVersion int64     `json:"expected_metadata_version"`
}
type GenerateRequest struct {
	UserID              int64     `json:"-"`
	UserGroupCode       string    `json:"-"`
	UserGroupCodes      []string  `json:"-"`
	UserGroupMultiplier string    `json:"-"`
	CanvasID            uuid.UUID `json:"-"`
	NodeID              string    `json:"-"`
	IdempotencyKey      string    `json:"-"`
}
type AttachResultsRequest struct {
	UserID           int64               `json:"-"`
	CanvasID         uuid.UUID           `json:"-"`
	RunID            uuid.UUID           `json:"-"`
	RecoveryPosition *domaincanvas.Point `json:"recovery_position,omitempty"`
}
type Estimate struct {
	Points string         `json:"points"`
	Detail map[string]any `json:"detail,omitempty"`
}
type GenerationSubmission struct {
	UserID              int64
	UserGroupCode       string
	UserGroupCodes      []string
	UserGroupMultiplier string
	ProjectID           uuid.UUID
	CanvasID            uuid.UUID
	NodeID              string
	Kind                TaskKind
	Revision            int64
	IdempotencyKey      string
	Node                domaincanvas.Node
	Inputs              []GenerationInput
}
type GenerationInput struct {
	Node    domaincanvas.Node
	Role    domaincanvas.InputRole
	Ordinal int
}
type GenerationTask struct {
	TaskID uuid.UUID
	Kind   TaskKind
	Status RunStatus
}
type TaskStatus struct {
	ProgressStage   string
	ProgressMessage string
	Status          RunStatus
	ResultAssetIDs  []uuid.UUID
	ErrorCode       string
	ErrorMessage    string
}
type Generator interface {
	Estimate(context.Context, GenerationSubmission) (Estimate, error)
	Generate(context.Context, GenerationSubmission) (GenerationTask, error)
	Status(context.Context, int64, TaskKind, uuid.UUID) (TaskStatus, error)
	Cancel(context.Context, int64, TaskKind, uuid.UUID) error
}
type ProjectResolver interface {
	ResolveOwned(context.Context, int64, uuid.UUID) error
}

type Observer interface {
	RecordCanvasSave(result string)
}

type Service struct {
	store     Store
	generator Generator
	projects  ProjectResolver
	limits    domaincanvas.Limits
	now       func() time.Time
	observer  Observer
}

func NewService(store Store, generator Generator, projects ProjectResolver) *Service {
	return &Service{store: store, generator: generator, projects: projects, limits: domaincanvas.DefaultLimits(), now: time.Now}
}

func (s *Service) SetObserver(observer Observer) {
	if s != nil {
		s.observer = observer
	}
}

func (s *Service) List(ctx context.Context, req ListRequest) ([]Canvas, error) {
	if req.UserID <= 0 {
		return nil, ErrNotFound
	}
	req.Search = strings.TrimSpace(req.Search)
	items, err := s.store.List(ctx, req)
	for index := range items {
		items[index] = normalizeCanvasProjection(items[index])
	}
	return items, err
}
func (s *Service) Get(ctx context.Context, userID int64, id uuid.UUID) (Canvas, error) {
	item, err := s.store.Get(ctx, userID, id)
	return normalizeCanvasProjection(item), err
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Canvas, error) {
	if req.UserID <= 0 || req.ProjectID == uuid.Nil {
		return Canvas{}, fmt.Errorf("invalid canvas owner or project")
	}
	if s.projects != nil {
		if err := s.projects.ResolveOwned(ctx, req.UserID, req.ProjectID); err != nil {
			return Canvas{}, ErrNotFound
		}
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "未命名画布"
	}
	if len([]rune(name)) > 128 {
		return Canvas{}, fmt.Errorf("canvas name is too long")
	}
	doc := templateDocument(req.Template)
	if req.Document != nil {
		doc = *req.Document
	}
	normalized, raw, refs, err := normalizeDocument(doc, s.limits)
	if err != nil {
		return Canvas{}, err
	}
	if validation := domaincanvas.ValidateMutation(domaincanvas.DocumentV1{}, normalized); validation != nil {
		return Canvas{}, validation
	}
	now := s.now().UTC()
	item := Canvas{ID: uuid.New(), UserID: req.UserID, ProjectID: req.ProjectID, Name: name, SchemaVersion: 1, Revision: 1, MetadataVersion: 1, Document: normalized, DocumentBytes: len(raw), NodeCount: len(normalized.Nodes), EdgeCount: len(normalized.Edges), AssetReferences: refs, Status: CanvasStatusActive, LastSavedAt: now, CreatedAt: now, UpdatedAt: now}
	return s.store.Create(ctx, item)
}
func (s *Service) Rename(ctx context.Context, req RenameRequest) (Canvas, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 128 {
		return Canvas{}, fmt.Errorf("invalid canvas name")
	}
	return s.store.Rename(ctx, req)
}
func (s *Service) Duplicate(ctx context.Context, req DuplicateRequest) (Canvas, error) {
	source, err := s.store.Get(ctx, req.UserID, req.CanvasID)
	if err != nil {
		return Canvas{}, err
	}
	projectID := source.ProjectID
	if req.ProjectID != nil {
		projectID = *req.ProjectID
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = source.Name + " 副本"
	}
	return s.Create(ctx, CreateRequest{UserID: req.UserID, ProjectID: projectID, Name: name, Document: &source.Document})
}
func (s *Service) Delete(ctx context.Context, req DeleteRequest) error {
	return s.store.Delete(ctx, req)
}
func (s *Service) TransferProject(ctx context.Context, req TransferProjectRequest) (Canvas, error) {
	if req.TargetProjectID == uuid.Nil {
		return Canvas{}, fmt.Errorf("target project is required")
	}
	if s.projects != nil {
		if err := s.projects.ResolveOwned(ctx, req.UserID, req.TargetProjectID); err != nil {
			return Canvas{}, ErrNotFound
		}
	}
	return s.store.TransferProject(ctx, req)
}

func (s *Service) SaveDocument(ctx context.Context, req SaveDocumentRequest) (Canvas, error) {
	normalized, raw, refs, err := normalizeDocument(req.Document, s.limits)
	if err != nil {
		if s.observer != nil {
			s.observer.RecordCanvasSave("failed")
		}
		return Canvas{}, err
	}
	item, err := s.store.SaveDocument(ctx, SaveDocumentRecord{UserID: req.UserID, CanvasID: req.CanvasID, ExpectedRevision: req.ExpectedRevision, Document: normalized, CanonicalJSON: raw, AssetReferences: refs, Reason: "periodic"})
	if errors.Is(err, ErrRevisionChanged) {
		if s.observer != nil {
			s.observer.RecordCanvasSave("failed")
		}
		return Canvas{}, &RevisionConflictError{RemoteRevision: item.Revision, RemoteUpdatedAt: item.UpdatedAt, Summary: Summary{NodeCount: item.NodeCount, EdgeCount: item.EdgeCount, RunningTaskCount: item.RunningTaskCount}}
	}
	if s.observer != nil {
		if err == nil {
			s.observer.RecordCanvasSave("success")
		} else {
			s.observer.RecordCanvasSave("failed")
		}
	}
	return item, err
}

func (s *Service) Estimate(ctx context.Context, req GenerateRequest) (Estimate, error) {
	submission, err := s.submission(ctx, req)
	if err != nil {
		return Estimate{}, err
	}
	if s.generator == nil {
		return Estimate{}, errors.New("canvas generation is unavailable")
	}
	return s.generator.Estimate(ctx, submission)
}
func (s *Service) Generate(ctx context.Context, req GenerateRequest) (Run, error) {
	if strings.TrimSpace(req.IdempotencyKey) == "" || len(req.IdempotencyKey) > 128 {
		return Run{}, fmt.Errorf("valid idempotency key is required")
	}
	existing, err := s.store.ListRuns(ctx, req.UserID, req.CanvasID)
	if err != nil {
		return Run{}, err
	}
	for _, run := range existing {
		if run.NodeID == req.NodeID && run.IdempotencyKey == req.IdempotencyKey {
			return run, nil
		}
	}
	submission, err := s.submission(ctx, req)
	if err != nil {
		return Run{}, err
	}
	if s.generator == nil {
		return Run{}, errors.New("canvas generation is unavailable")
	}
	snapshot, _ := json.Marshal(map[string]any{"node": submission.Node, "inputs": submission.Inputs})
	now := s.now().UTC()
	run := Run{ID: uuid.New(), CanvasID: req.CanvasID, UserID: req.UserID, NodeID: req.NodeID, SubmittedRevision: submission.Revision, TaskKind: submission.Kind, TaskID: uuid.New(), NodeSnapshot: snapshot, Status: RunStatusSubmitting, IdempotencyKey: req.IdempotencyKey, CreatedAt: now, UpdatedAt: now}
	created, replayed, err := s.store.CreateRun(ctx, run)
	if err != nil || replayed {
		return created, err
	}
	task, err := s.generator.Generate(ctx, submission)
	if err != nil {
		_, _ = s.store.UpdateRun(ctx, RunUpdate{UserID: req.UserID, CanvasID: req.CanvasID, RunID: created.ID, Status: RunStatusFailed, ErrorCode: "SUBMISSION_FAILED", ErrorMessage: err.Error()})
		return Run{}, err
	}
	return s.store.UpdateRun(ctx, RunUpdate{UserID: req.UserID, CanvasID: req.CanvasID, RunID: created.ID, TaskID: &task.TaskID, Status: task.Status})
}
func (s *Service) ListRuns(ctx context.Context, userID int64, canvasID uuid.UUID, refresh bool) ([]Run, error) {
	runs, err := s.store.ListRuns(ctx, userID, canvasID)
	if err != nil {
		return nil, err
	}
	if refresh {
		for i := range runs {
			if runs[i].Status.Active() || runs[i].Status == RunStatusSucceeded {
				runs[i], err = s.RefreshRun(ctx, userID, canvasID, runs[i].ID)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	for i := range runs {
		// Historical runs may carry raw vendor text; serve the unified code
		// and curated copy instead. Runs without any error stay untouched —
		// resolving empty fields would inject the generic failure copy.
		if runs[i].ErrorCode == "" && runs[i].ErrorMessage == "" {
			continue
		}
		resolution := mediaerrors.ResolveImageTask(runs[i].ErrorCode, runs[i].ErrorMessage)
		if runs[i].TaskKind == TaskKindVideo {
			resolution = mediaerrors.ResolveVideoItem(runs[i].ErrorCode, runs[i].ErrorMessage)
		}
		if resolution.Code != "" {
			runs[i].ErrorCode = resolution.Code
		}
		runs[i].ErrorMessage = resolution.Message
	}
	return runs, nil
}
func (s *Service) RefreshRun(ctx context.Context, userID int64, canvasID, runID uuid.UUID) (Run, error) {
	run, err := s.store.GetRun(ctx, userID, canvasID, runID)
	if err != nil {
		return Run{}, err
	}
	if run.Status == RunStatusSucceeded {
		return s.attachSucceededResults(ctx, run)
	}
	if !run.Status.Active() || run.Status == RunStatusSubmitting || s.generator == nil {
		return run, nil
	}
	status, err := s.generator.Status(ctx, userID, run.TaskKind, run.TaskID)
	if err != nil {
		return Run{}, err
	}
	updated, err := s.store.UpdateRun(ctx, RunUpdate{UserID: userID, CanvasID: canvasID, RunID: runID, Status: status.Status, ResultAssetIDs: status.ResultAssetIDs, ErrorCode: status.ErrorCode, ErrorMessage: status.ErrorMessage})
	if err == nil && updated.Status == RunStatusSucceeded {
		updated, err = s.attachSucceededResults(ctx, updated)
	}
	updated.ProgressStage, updated.ProgressMessage = status.ProgressStage, status.ProgressMessage
	return updated, err
}
func (s *Service) CancelRun(ctx context.Context, userID int64, canvasID, runID uuid.UUID) (Run, error) {
	run, err := s.store.GetRun(ctx, userID, canvasID, runID)
	if err != nil {
		return Run{}, err
	}
	if !run.Status.Active() {
		return run, nil
	}
	if s.generator == nil {
		return Run{}, errors.New("canvas generation is unavailable")
	}
	if err := s.generator.Cancel(ctx, userID, run.TaskKind, run.TaskID); err != nil {
		return Run{}, err
	}
	return s.store.UpdateRun(ctx, RunUpdate{UserID: userID, CanvasID: canvasID, RunID: runID, Status: RunStatusCanceled})
}
func (s *Service) AttachResults(ctx context.Context, req AttachResultsRequest) (Run, error) {
	run, err := s.RefreshRun(ctx, req.UserID, req.CanvasID, req.RunID)
	if err != nil {
		return Run{}, err
	}
	if run.Status == RunStatusAttached {
		return run, nil
	}
	if run.Status == RunStatusUnplaced {
		if req.RecoveryPosition == nil {
			return run, nil
		}
		if !validRecoveryPosition(*req.RecoveryPosition) {
			return Run{}, fmt.Errorf("invalid recovery position")
		}
		layers, err := s.runLayers(ctx, run)
		if err != nil {
			return Run{}, err
		}
		nodes := recoveredResultPlacement(run, *req.RecoveryPosition)
		if len(layers) > 0 {
			const width = 320.0
			height := width * float64(layers[0].result.Height) / float64(layers[0].result.Width)
			nodes = layerPlacement(run, layers, domaincanvas.Point{X: req.RecoveryPosition.X - width/2, Y: req.RecoveryPosition.Y - height/2}, width)
		}
		return s.store.AttachResults(ctx, AttachRecord{UserID: run.UserID, CanvasID: run.CanvasID, RunID: run.ID, Nodes: nodes, RecoverUnplaced: true})
	}
	if run.Status != RunStatusSucceeded {
		return Run{}, fmt.Errorf("run is not ready to attach")
	}
	return s.attachSucceededResults(ctx, run)
}

func validRecoveryPosition(position domaincanvas.Point) bool {
	const maxCoordinate = 1_000_000
	return !math.IsNaN(position.X) && !math.IsInf(position.X, 0) && math.Abs(position.X) <= maxCoordinate &&
		!math.IsNaN(position.Y) && !math.IsInf(position.Y, 0) && math.Abs(position.Y) <= maxCoordinate
}

func (s *Service) attachSucceededResults(ctx context.Context, run Run) (Run, error) {
	canvas, err := s.store.Get(ctx, run.UserID, run.CanvasID)
	if err != nil {
		return Run{}, err
	}
	source, ok := findNode(canvas.Document, run.NodeID)
	if !ok || !CanAttachToSource(run, canvas.Document) {
		return s.store.AttachResults(ctx, AttachRecord{UserID: run.UserID, CanvasID: run.CanvasID, RunID: run.ID})
	}
	layers, err := s.runLayers(ctx, run)
	if err != nil {
		return Run{}, err
	}
	if len(layers) > 0 {
		width := source.Size.Width
		if width <= 0 {
			width = 320
		}
		nodes := layerPlacement(run, layers, domaincanvas.Point{X: source.Position.X + source.Size.Width + 80, Y: source.Position.Y}, width)
		return s.store.AttachResults(ctx, AttachRecord{UserID: run.UserID, CanvasID: run.CanvasID, RunID: run.ID, ExpectedRevision: canvas.Revision, Nodes: nodes})
	}
	runs, err := s.store.ListRuns(ctx, run.UserID, run.CanvasID)
	if err != nil {
		return Run{}, err
	}
	batchIndex := 0
	for _, existing := range runs {
		if existing.ID != run.ID && existing.NodeID == run.NodeID && existing.Status == RunStatusAttached && existing.AttachedRevision != nil {
			batchIndex++
		}
	}
	updatedNodes, nodes, edges := resultPlacement(run, source, canvas.Document, batchIndex == 0, batchIndex)
	return s.store.AttachResults(ctx, AttachRecord{UserID: run.UserID, CanvasID: run.CanvasID, RunID: run.ID, ExpectedRevision: canvas.Revision, UpdatedNodes: updatedNodes, Nodes: nodes, Edges: edges})
}

func (s *Service) submission(ctx context.Context, req GenerateRequest) (GenerationSubmission, error) {
	canvas, err := s.store.Get(ctx, req.UserID, req.CanvasID)
	if err != nil {
		return GenerationSubmission{}, err
	}
	node, ok := findNode(canvas.Document, req.NodeID)
	if !ok {
		return GenerationSubmission{}, ErrNotFound
	}
	kind := TaskKindImage
	if node.Type == domaincanvas.NodeTypeVideoV2 {
		kind = TaskKindVideo
	} else if node.Type != domaincanvas.NodeTypeImageV2 {
		return GenerationSubmission{}, fmt.Errorf("node is not a generation node")
	}
	if strings.TrimSpace(node.AssetID) != "" {
		return GenerationSubmission{}, fmt.Errorf("filled media nodes cannot generate")
	}
	if validation := domaincanvas.ValidateDocument(canvas.Document, s.limits); validation != nil {
		return GenerationSubmission{}, validation
	}
	inputs := make([]GenerationInput, 0)
	for _, edge := range canvas.Document.Edges {
		if edge.Target == node.ID && edge.InputRole != domaincanvas.InputRoleResult {
			if input, found := findNode(canvas.Document, edge.Source); found {
				inputs = append(inputs, GenerationInput{Node: input, Role: edge.InputRole, Ordinal: edge.Ordinal})
			}
		}
	}
	sort.SliceStable(inputs, func(i, j int) bool {
		if inputs[i].Ordinal != inputs[j].Ordinal {
			return inputs[i].Ordinal < inputs[j].Ordinal
		}
		return inputs[i].Node.ID < inputs[j].Node.ID
	})
	return GenerationSubmission{UserID: req.UserID, UserGroupCode: req.UserGroupCode, UserGroupCodes: append([]string(nil), req.UserGroupCodes...), UserGroupMultiplier: req.UserGroupMultiplier, ProjectID: canvas.ProjectID, CanvasID: canvas.ID, NodeID: node.ID, Kind: kind, Revision: canvas.Revision, IdempotencyKey: req.IdempotencyKey, Node: node, Inputs: inputs}, nil
}

func normalizeDocument(doc domaincanvas.DocumentV1, limits domaincanvas.Limits) (domaincanvas.DocumentV1, []byte, []uuid.UUID, error) {
	doc = domaincanvas.NormalizeCollections(doc)
	raw, err := json.Marshal(doc)
	if err != nil {
		return doc, nil, nil, err
	}
	var normalized domaincanvas.DocumentV1
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&normalized); err != nil {
		return doc, nil, nil, err
	}
	normalized = domaincanvas.NormalizeCollections(normalized)
	if validation := domaincanvas.ValidateDocument(normalized, limits); validation != nil {
		return doc, nil, nil, validation
	}
	if err := rejectUnsafeDocument(raw); err != nil {
		return doc, nil, nil, err
	}
	raw, _ = json.Marshal(normalized)
	refs := parseAssetReferences(normalized)
	for _, node := range normalized.Nodes {
		if domaincanvas.IsIntegrated(node.Type) || node.Type == domaincanvas.NodeTypeImage || node.Type == domaincanvas.NodeTypeVideo || node.Type == domaincanvas.NodeTypeAudio {
			if (domaincanvas.IsIntegrated(node.Type) || node.Type == domaincanvas.NodeTypeImage) && strings.TrimSpace(node.AssetID) == "" {
				continue
			}
			if _, err := uuid.Parse(strings.TrimSpace(node.AssetID)); err != nil {
				return doc, nil, nil, fmt.Errorf("invalid asset id on node %s", node.ID)
			}
		}
	}
	return normalized, raw, refs, nil
}

func normalizeCanvasProjection(item Canvas) Canvas {
	item.Document = domaincanvas.NormalizeCollections(item.Document)
	if item.AssetReferences == nil {
		item.AssetReferences = make([]uuid.UUID, 0)
	}
	return item
}
func parseAssetReferences(doc domaincanvas.DocumentV1) []uuid.UUID {
	values := domaincanvas.ExtractAssetReferences(doc)
	result := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		if id, err := uuid.Parse(strings.TrimSpace(value)); err == nil {
			result = append(result, id)
		}
	}
	return result
}
func rejectUnsafeDocument(raw []byte) error {
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		return fmt.Errorf("decode canvas document safety projection: %w", err)
	}
	return inspectPersistedValue("", document)
}

func inspectPersistedValue(key string, value any) error {
	normalizedKey := strings.ToLower(strings.TrimSpace(key))
	if strings.Contains(normalizedKey, "url") || normalizedKey == "provider_api_key" || normalizedKey == "provider_response" || normalizedKey == "secret_key" {
		return fmt.Errorf("canvas document contains forbidden persisted field %q", key)
	}
	switch typed := value.(type) {
	case map[string]any:
		for childKey, child := range typed {
			if err := inspectPersistedValue(childKey, child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := inspectPersistedValue(key, child); err != nil {
				return err
			}
		}
	case string:
		lower := strings.ToLower(strings.TrimSpace(typed))
		if strings.HasPrefix(lower, "data:") && strings.Contains(lower, ";base64,") {
			return fmt.Errorf("canvas document contains embedded binary data")
		}
	}
	return nil
}
func findNode(doc domaincanvas.DocumentV1, id string) (domaincanvas.Node, bool) {
	for _, node := range doc.Nodes {
		if node.ID == id {
			return node, true
		}
	}
	return domaincanvas.Node{}, false
}
func documentHasNode(doc domaincanvas.DocumentV1, id string) bool {
	_, ok := findNode(doc, id)
	return ok
}
func cloneDocument(doc domaincanvas.DocumentV1) domaincanvas.DocumentV1 {
	raw, _ := json.Marshal(doc)
	var result domaincanvas.DocumentV1
	_ = json.Unmarshal(raw, &result)
	return result
}
func resultPlacement(run Run, source domaincanvas.Node, document domaincanvas.DocumentV1, consumeEmptySlots bool, batchIndex int) ([]domaincanvas.Node, []domaincanvas.Node, []domaincanvas.Edge) {
	if domaincanvas.IsIntegrated(source.Type) {
		updated := make([]domaincanvas.Node, 0, 1)
		siblings := make([]domaincanvas.Node, 0)
		for i, assetID := range run.ResultAssetIDs {
			if i == 0 {
				source.AssetID = assetID.String()
				updated = append(updated, source)
				continue
			}
			id := domaincanvas.StableResultNodeID(run.ID.String(), assetID.String())
			payload, _ := json.Marshal(map[string]any{"name": id, "title": resultNodeName(source.Type, 0, i)})
			siblings = append(siblings, domaincanvas.Node{ID: id, Type: source.Type, AssetID: assetID.String(), Position: domaincanvas.Point{X: source.Position.X + float64(i)*(source.Size.Width+48), Y: source.Position.Y}, Size: source.Size, Payload: payload})
		}
		return updated, siblings, nil
	}

	updatedNodes := make([]domaincanvas.Node, 0)
	nodes := make([]domaincanvas.Node, 0, len(run.ResultAssetIDs))
	edges := make([]domaincanvas.Edge, 0, len(run.ResultAssetIDs))
	mediaType := domaincanvas.NodeTypeImage
	if run.TaskKind == TaskKindVideo {
		mediaType = domaincanvas.NodeTypeVideo
	}
	width := source.Size.Width
	if width <= 0 {
		width = 320
	}
	height := source.Size.Height
	if height <= 0 {
		height = 240
	}
	type outputSlot struct {
		node      domaincanvas.Node
		ordinal   int
		edgeIndex int
	}
	slots := make([]outputSlot, 0)
	nodeByID := make(map[string]domaincanvas.Node, len(document.Nodes))
	for _, node := range document.Nodes {
		nodeByID[node.ID] = node
	}
	nextOrdinal := 0
	for index, edge := range document.Edges {
		if edge.Source != source.ID || edge.InputRole != domaincanvas.InputRoleResult {
			continue
		}
		if edge.Ordinal >= nextOrdinal {
			nextOrdinal = edge.Ordinal + 1
		}
		target, ok := nodeByID[edge.Target]
		if consumeEmptySlots && ok && target.Type == domaincanvas.NodeTypeImage && strings.TrimSpace(target.AssetID) == "" {
			slots = append(slots, outputSlot{node: target, ordinal: edge.Ordinal, edgeIndex: index})
		}
	}
	sort.SliceStable(slots, func(i, j int) bool {
		if slots[i].ordinal == slots[j].ordinal {
			return slots[i].edgeIndex < slots[j].edgeIndex
		}
		return slots[i].ordinal < slots[j].ordinal
	})
	resultIndex := 0
	for resultIndex < len(run.ResultAssetIDs) && resultIndex < len(slots) {
		slot := slots[resultIndex].node
		slot.AssetID = run.ResultAssetIDs[resultIndex].String()
		slot.Payload = ensureResultNodeName(slot.Payload, resultNodeName(mediaType, batchIndex, resultIndex))
		updatedNodes = append(updatedNodes, slot)
		resultIndex++
	}
	for index := resultIndex; index < len(run.ResultAssetIDs); index++ {
		assetID := run.ResultAssetIDs[index]
		id := domaincanvas.StableResultNodeID(run.ID.String(), assetID.String())
		nodes = append(nodes, domaincanvas.Node{ID: id, Type: mediaType, AssetID: assetID.String(), Position: domaincanvas.Point{X: source.Position.X + source.Size.Width + 80 + float64(batchIndex)*(width+48), Y: source.Position.Y + float64(index)*(height+32)}, Size: domaincanvas.Size{Width: width, Height: height}, Payload: ensureResultNodeName(nil, resultNodeName(mediaType, batchIndex, index))})
		edges = append(edges, domaincanvas.Edge{ID: "edge-" + id, Source: source.ID, Target: id, InputRole: domaincanvas.InputRoleResult, Ordinal: nextOrdinal})
		nextOrdinal++
	}
	return updatedNodes, nodes, edges
}

func recoveredResultPlacement(run Run, center domaincanvas.Point) []domaincanvas.Node {
	const width, height, gap = 320.0, 240.0, 32.0
	mediaType := domaincanvas.NodeTypeImage
	if run.TaskKind == TaskKindVideo {
		mediaType = domaincanvas.NodeTypeVideo
	}
	var snapshot struct {
		Node domaincanvas.Node `json:"node"`
	}
	_ = json.Unmarshal(run.NodeSnapshot, &snapshot)
	if domaincanvas.IsIntegrated(snapshot.Node.Type) {
		mediaType = snapshot.Node.Type
	}
	nodes := make([]domaincanvas.Node, 0, len(run.ResultAssetIDs))
	for index, assetID := range run.ResultAssetIDs {
		nodes = append(nodes, domaincanvas.Node{
			ID:      domaincanvas.StableResultNodeID(run.ID.String(), assetID.String()),
			Type:    mediaType,
			AssetID: assetID.String(),
			Position: domaincanvas.Point{
				X: center.X - width/2,
				Y: center.Y - height/2 + float64(index)*(height+gap),
			},
			Size:    domaincanvas.Size{Width: width, Height: height},
			Payload: ensureResultNodeName(nil, resultNodeName(mediaType, 0, index)),
		})
	}
	if domaincanvas.IsIntegrated(mediaType) {
		for i := range nodes {
			nodes[i].Payload, _ = json.Marshal(map[string]any{"name": nodes[i].ID, "title": resultNodeName(mediaType, 0, i)})
		}
	}
	return nodes
}

func resultNodeName(mediaType domaincanvas.NodeType, batchIndex, resultIndex int) string {
	kind := "图片"
	if mediaType == domaincanvas.NodeTypeVideo || mediaType == domaincanvas.NodeTypeVideoV2 {
		kind = "视频"
	}
	return fmt.Sprintf("生成%s %d-%d", kind, batchIndex+1, resultIndex+1)
}

func ensureResultNodeName(raw json.RawMessage, fallback string) json.RawMessage {
	payload := make(map[string]any)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &payload)
	}
	if name, ok := payload["name"].(string); !ok || strings.TrimSpace(name) == "" {
		payload["name"] = fallback
	}
	encoded, _ := json.Marshal(payload)
	return encoded
}
func templateDocument(template Template) domaincanvas.DocumentV1 {
	doc := domaincanvas.DocumentV1{SchemaVersion: 1, Viewport: domaincanvas.Viewport{Zoom: 1}, Nodes: make([]domaincanvas.Node, 0), Edges: make([]domaincanvas.Edge, 0)}
	switch template {
	case TemplateImageExploration:
		doc.Nodes = []domaincanvas.Node{{ID: "image-generation", Type: domaincanvas.NodeTypeImageV2, Position: domaincanvas.Point{X: 40, Y: 80}, Size: domaincanvas.Size{Width: 320, Height: 240}, Payload: json.RawMessage(`{"name":"image_1","title":"图片","draft":{"prompt_template":"","variables":{}}}`)}}
	case TemplateImageToVideo:
		doc.Nodes = []domaincanvas.Node{{ID: "video-generation", Type: domaincanvas.NodeTypeVideoV2, Position: domaincanvas.Point{X: 40, Y: 80}, Size: domaincanvas.Size{Width: 320, Height: 240}, Payload: json.RawMessage(`{"name":"video_1","title":"视频","draft":{"prompt_template":"","variables":{}}}`)}}
	}
	if template == TemplateImageExploration {
		first := doc.Nodes[0]
		for i := 1; i < 4; i++ {
			node := first
			node.ID = fmt.Sprintf("image-%d", i+1)
			node.Position.X += float64(i) * 368
			node.Payload = json.RawMessage(fmt.Sprintf(`{"name":"image_%d","title":"图片 %d","draft":{"prompt_template":"","variables":{}}}`, i+1, i+1))
			doc.Nodes = append(doc.Nodes, node)
		}
	}
	if template == TemplateImageToVideo {
		doc.Nodes = append(doc.Nodes, domaincanvas.Node{ID: "image-reference", Type: domaincanvas.NodeTypeImageV2, Position: domaincanvas.Point{X: 408, Y: 80}, Size: domaincanvas.Size{Width: 320, Height: 240}, Payload: json.RawMessage(`{"name":"image_1","title":"图片参考","draft":{"prompt_template":"","variables":{}}}`)})
	}
	return doc
}

// CanAttachToSource prevents an old task from replacing a newer draft or media selection.
// Historical runs retain their original recovery semantics.
func CanAttachToSource(run Run, document domaincanvas.DocumentV1) bool {
	var snapshot struct {
		Node   domaincanvas.Node `json:"node"`
		Inputs []GenerationInput `json:"inputs"`
	}
	if json.Unmarshal(run.NodeSnapshot, &snapshot) != nil {
		return false
	}
	if !domaincanvas.IsIntegrated(snapshot.Node.Type) {
		return documentHasNode(document, run.NodeID)
	}
	current, ok := findNode(document, run.NodeID)
	if !ok || current.Type != snapshot.Node.Type || strings.TrimSpace(current.AssetID) != "" {
		return false
	}
	// Position, size and display title can change while a task runs.
	signature := func(node domaincanvas.Node) string {
		var payload map[string]any
		_ = json.Unmarshal(node.Payload, &payload)
		delete(payload, "title")
		raw, _ := json.Marshal(payload)
		return node.AssetID + "\x00" + string(raw)
	}
	if signature(current) != signature(snapshot.Node) {
		return false
	}
	inputs := make(map[string]GenerationInput)
	for _, input := range snapshot.Inputs {
		inputs[input.Node.ID+"/"+string(input.Role)+fmt.Sprint(input.Ordinal)] = input
	}
	count := 0
	for _, edge := range document.Edges {
		if edge.Target != current.ID || edge.InputRole == domaincanvas.InputRoleResult {
			continue
		}
		input, exists := inputs[edge.Source+"/"+string(edge.InputRole)+fmt.Sprint(edge.Ordinal)]
		node, found := findNode(document, edge.Source)
		if !exists || !found || node.Type != input.Node.Type || signature(node) != signature(input.Node) {
			return false
		}
		count++
	}
	return count == len(snapshot.Inputs)
}
