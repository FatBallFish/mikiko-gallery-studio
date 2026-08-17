package canvas

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	domaincanvas "github.com/fatballfish/pic-gallery/internal/domain/canvas"
	"github.com/google/uuid"
)

var (
	ErrNotFound            = errors.New("canvas not found")
	ErrRevisionChanged     = errors.New("canvas revision changed")
	ErrMetadataChanged     = errors.New("canvas metadata changed")
	ErrCanvasBusy          = errors.New("canvas has active generation runs")
	ErrIdempotencyConflict = errors.New("canvas generation idempotency conflict")
)

type Store interface {
	List(context.Context, ListRequest) ([]Canvas, error)
	Get(context.Context, int64, uuid.UUID) (Canvas, error)
	Create(context.Context, Canvas) (Canvas, error)
	Rename(context.Context, RenameRequest) (Canvas, error)
	SaveDocument(context.Context, SaveDocumentRecord) (Canvas, error)
	Delete(context.Context, DeleteRequest) error
	TransferProject(context.Context, TransferProjectRequest) (Canvas, error)
	CreateRun(context.Context, Run) (Run, bool, error)
	GetRun(context.Context, int64, uuid.UUID, uuid.UUID) (Run, error)
	ListRuns(context.Context, int64, uuid.UUID) ([]Run, error)
	UpdateRun(context.Context, RunUpdate) (Run, error)
	AttachResults(context.Context, AttachRecord) (Run, error)
}

type SaveDocumentRecord struct {
	UserID           int64
	CanvasID         uuid.UUID
	ExpectedRevision int64
	Document         domaincanvas.DocumentV1
	CanonicalJSON    []byte
	AssetReferences  []uuid.UUID
	Reason           string
}

type RunUpdate struct {
	UserID         int64
	CanvasID       uuid.UUID
	RunID          uuid.UUID
	TaskID         *uuid.UUID
	Status         RunStatus
	ResultAssetIDs []uuid.UUID
	ErrorCode      string
	ErrorMessage   string
}

type AttachRecord struct {
	UserID           int64
	CanvasID         uuid.UUID
	RunID            uuid.UUID
	ExpectedRevision int64
	UpdatedNodes     []domaincanvas.Node
	Nodes            []domaincanvas.Node
	Edges            []domaincanvas.Edge
	RecoverUnplaced  bool
}

type MemoryStore struct {
	mu       sync.Mutex
	canvases map[uuid.UUID]Canvas
	runs     map[uuid.UUID]Run
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{canvases: make(map[uuid.UUID]Canvas), runs: make(map[uuid.UUID]Run)}
}

func (s *MemoryStore) List(_ context.Context, req ListRequest) ([]Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]Canvas, 0)
	for _, item := range s.canvases {
		if item.UserID != req.UserID || item.Status != CanvasStatusActive {
			continue
		}
		if req.ProjectID != nil && item.ProjectID != *req.ProjectID {
			continue
		}
		if req.Search != "" && !strings.Contains(strings.ToLower(item.Name), strings.ToLower(req.Search)) {
			continue
		}
		items = append(items, cloneCanvas(item))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items, nil
}

func (s *MemoryStore) Get(_ context.Context, userID int64, id uuid.UUID) (Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.canvases[id]
	if !ok || item.UserID != userID || item.Status != CanvasStatusActive {
		return Canvas{}, ErrNotFound
	}
	return cloneCanvas(item), nil
}

func (s *MemoryStore) Create(_ context.Context, item Canvas) (Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.canvases[item.ID] = cloneCanvas(item)
	return cloneCanvas(item), nil
}

func (s *MemoryStore) Rename(_ context.Context, req RenameRequest) (Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.canvases[req.CanvasID]
	if !ok || item.UserID != req.UserID || item.Status != CanvasStatusActive {
		return Canvas{}, ErrNotFound
	}
	if item.MetadataVersion != req.ExpectedMetadataVersion {
		return Canvas{}, ErrMetadataChanged
	}
	item.Name = req.Name
	item.MetadataVersion++
	item.UpdatedAt = time.Now().UTC()
	s.canvases[item.ID] = item
	return cloneCanvas(item), nil
}

func (s *MemoryStore) SaveDocument(_ context.Context, req SaveDocumentRecord) (Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.canvases[req.CanvasID]
	if !ok || item.UserID != req.UserID || item.Status != CanvasStatusActive {
		return Canvas{}, ErrNotFound
	}
	if item.Revision != req.ExpectedRevision {
		return cloneCanvas(item), ErrRevisionChanged
	}
	item.Document = cloneDocument(req.Document)
	item.AssetReferences = append([]uuid.UUID(nil), req.AssetReferences...)
	item.DocumentBytes = len(req.CanonicalJSON)
	item.NodeCount = len(req.Document.Nodes)
	item.EdgeCount = len(req.Document.Edges)
	item.Revision++
	item.LastSavedAt = time.Now().UTC()
	item.UpdatedAt = item.LastSavedAt
	s.canvases[item.ID] = item
	return cloneCanvas(item), nil
}

func (s *MemoryStore) Delete(_ context.Context, req DeleteRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.canvases[req.CanvasID]
	if !ok || item.UserID != req.UserID || item.Status != CanvasStatusActive {
		return ErrNotFound
	}
	if item.MetadataVersion != req.ExpectedMetadataVersion {
		return ErrMetadataChanged
	}
	if item.RunningTaskCount > 0 || s.hasActiveRun(item.ID) {
		return ErrCanvasBusy
	}
	item.Status = CanvasStatusDeleted
	item.MetadataVersion++
	item.UpdatedAt = time.Now().UTC()
	s.canvases[item.ID] = item
	return nil
}

func (s *MemoryStore) TransferProject(_ context.Context, req TransferProjectRequest) (Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.canvases[req.CanvasID]
	if !ok || item.UserID != req.UserID || item.Status != CanvasStatusActive {
		return Canvas{}, ErrNotFound
	}
	if item.MetadataVersion != req.ExpectedMetadataVersion {
		return Canvas{}, ErrMetadataChanged
	}
	if item.RunningTaskCount > 0 || s.hasActiveRun(item.ID) {
		return Canvas{}, ErrCanvasBusy
	}
	item.ProjectID = req.TargetProjectID
	item.MetadataVersion++
	item.LastTransferredAt = time.Now().UTC()
	item.UpdatedAt = item.LastTransferredAt
	s.canvases[item.ID] = item
	return cloneCanvas(item), nil
}

func (s *MemoryStore) CreateRun(_ context.Context, run Run) (Run, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.canvases[run.CanvasID]
	if !ok || item.UserID != run.UserID || item.Status != CanvasStatusActive {
		return Run{}, false, ErrNotFound
	}
	for _, existing := range s.runs {
		if existing.CanvasID == run.CanvasID && existing.NodeID == run.NodeID && existing.IdempotencyKey == run.IdempotencyKey {
			return cloneRun(existing), true, nil
		}
	}
	s.runs[run.ID] = cloneRun(run)
	if run.Status.Active() {
		item.RunningTaskCount++
		s.canvases[item.ID] = item
	}
	return cloneRun(run), false, nil
}

func (s *MemoryStore) GetRun(_ context.Context, userID int64, canvasID, runID uuid.UUID) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok || run.UserID != userID || run.CanvasID != canvasID {
		return Run{}, ErrNotFound
	}
	return cloneRun(run), nil
}

func (s *MemoryStore) ListRuns(_ context.Context, userID int64, canvasID uuid.UUID) ([]Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	canvas, ok := s.canvases[canvasID]
	if !ok || canvas.UserID != userID || canvas.Status != CanvasStatusActive {
		return nil, ErrNotFound
	}
	items := make([]Run, 0)
	for _, run := range s.runs {
		if run.UserID == userID && run.CanvasID == canvasID {
			items = append(items, cloneRun(run))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (s *MemoryStore) UpdateRun(_ context.Context, req RunUpdate) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[req.RunID]
	if !ok || run.UserID != req.UserID || run.CanvasID != req.CanvasID {
		return Run{}, ErrNotFound
	}
	wasActive := run.Status.Active()
	run.Status = req.Status
	if req.TaskID != nil {
		run.TaskID = *req.TaskID
	}
	run.ResultAssetIDs = append([]uuid.UUID(nil), req.ResultAssetIDs...)
	run.ErrorCode, run.ErrorMessage = req.ErrorCode, req.ErrorMessage
	run.UpdatedAt = time.Now().UTC()
	s.runs[run.ID] = run
	item := s.canvases[run.CanvasID]
	if wasActive && !run.Status.Active() && item.RunningTaskCount > 0 {
		item.RunningTaskCount--
	}
	if req.Status == RunStatusFailed {
		item.FailedTaskCount++
	}
	s.canvases[item.ID] = item
	return cloneRun(run), nil
}

func (s *MemoryStore) AttachResults(_ context.Context, req AttachRecord) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[req.RunID]
	item, canvasOK := s.canvases[req.CanvasID]
	if !ok || !canvasOK || run.UserID != req.UserID || item.UserID != req.UserID || run.CanvasID != req.CanvasID {
		return Run{}, ErrNotFound
	}
	if run.Status == RunStatusAttached || (run.Status == RunStatusUnplaced && !req.RecoverUnplaced) {
		return cloneRun(run), nil
	}
	if !req.RecoverUnplaced && !documentHasNode(item.Document, run.NodeID) {
		run.Status = RunStatusUnplaced
		run.UpdatedAt = time.Now().UTC()
		s.runs[run.ID] = run
		return cloneRun(run), nil
	}
	if req.RecoverUnplaced && run.Status != RunStatusUnplaced {
		return Run{}, fmt.Errorf("canvas run is not unplaced")
	}
	if !req.RecoverUnplaced && req.ExpectedRevision > 0 && item.Revision != req.ExpectedRevision {
		run.Status = RunStatusUnplaced
		run.UpdatedAt = time.Now().UTC()
		s.runs[run.ID] = run
		return cloneRun(run), nil
	}
	existingNodeIndexes := make(map[string]int, len(item.Document.Nodes))
	for index, node := range item.Document.Nodes {
		existingNodeIndexes[node.ID] = index
	}
	for _, node := range req.UpdatedNodes {
		if index, exists := existingNodeIndexes[node.ID]; exists {
			item.Document.Nodes[index] = node
		}
	}
	existingNodes := make(map[string]struct{}, len(item.Document.Nodes))
	for _, node := range item.Document.Nodes {
		existingNodes[node.ID] = struct{}{}
	}
	for _, node := range req.Nodes {
		if _, exists := existingNodes[node.ID]; !exists {
			item.Document.Nodes = append(item.Document.Nodes, node)
		}
	}
	existingEdges := make(map[string]struct{}, len(item.Document.Edges))
	for _, edge := range item.Document.Edges {
		existingEdges[edge.ID] = struct{}{}
	}
	for _, edge := range req.Edges {
		if _, exists := existingEdges[edge.ID]; !exists {
			item.Document.Edges = append(item.Document.Edges, edge)
		}
	}
	item.AssetReferences = parseAssetReferences(item.Document)
	item.NodeCount, item.EdgeCount = len(item.Document.Nodes), len(item.Document.Edges)
	item.Revision++
	item.LastSavedAt = time.Now().UTC()
	item.UpdatedAt = item.LastSavedAt
	run.Status = RunStatusAttached
	run.AttachedRevision = &item.Revision
	run.UpdatedAt = item.UpdatedAt
	s.canvases[item.ID], s.runs[run.ID] = item, run
	return cloneRun(run), nil
}

func (s *MemoryStore) hasActiveRun(canvasID uuid.UUID) bool {
	for _, run := range s.runs {
		if run.CanvasID == canvasID && run.Status.Active() {
			return true
		}
	}
	return false
}

func cloneCanvas(item Canvas) Canvas {
	item.Document = cloneDocument(item.Document)
	item.AssetReferences = append([]uuid.UUID(nil), item.AssetReferences...)
	return item
}

func cloneRun(run Run) Run {
	run.ResultAssetIDs = append([]uuid.UUID(nil), run.ResultAssetIDs...)
	run.NodeSnapshot = append([]byte(nil), run.NodeSnapshot...)
	return run
}
