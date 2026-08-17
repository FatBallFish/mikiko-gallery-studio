package entstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	domaincanvas "github.com/fatballfish/pic-gallery/internal/domain/canvas"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/canvasgenerationrun"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/creativecanvas"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaassetreference"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/project"
	canvasservice "github.com/fatballfish/pic-gallery/internal/service/canvas"
)

type CanvasStore struct{ client *repoent.Client }

func NewCanvasStore(client *repoent.Client) *CanvasStore { return &CanvasStore{client: client} }

func (s *CanvasStore) List(ctx context.Context, req canvasservice.ListRequest) ([]canvasservice.Canvas, error) {
	query := s.client.CreativeCanvas.Query().Where(creativecanvas.UserIDEQ(req.UserID), creativecanvas.StatusEQ(string(canvasservice.CanvasStatusActive)), creativecanvas.DeletedAtIsNil())
	if req.ProjectID != nil {
		query.Where(creativecanvas.ProjectIDEQ(*req.ProjectID))
	}
	if req.Search != "" {
		query.Where(creativecanvas.NameContainsFold(req.Search))
	}
	entities, err := query.Order(repoent.Desc(creativecanvas.FieldUpdatedAt), repoent.Desc(creativecanvas.FieldID)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list canvases: %w", err)
	}
	items := make([]canvasservice.Canvas, 0, len(entities))
	for _, entity := range entities {
		item, mapErr := mapCanvasEntity(ctx, s.client, entity)
		if mapErr != nil {
			return nil, mapErr
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *CanvasStore) Get(ctx context.Context, userID int64, id uuid.UUID) (canvasservice.Canvas, error) {
	entity, err := ownedCanvasQuery(s.client, userID, id).Only(ctx)
	if repoent.IsNotFound(err) {
		return canvasservice.Canvas{}, canvasservice.ErrNotFound
	}
	if err != nil {
		return canvasservice.Canvas{}, fmt.Errorf("get canvas: %w", err)
	}
	return mapCanvasEntity(ctx, s.client, entity)
}

func (s *CanvasStore) Create(ctx context.Context, item canvasservice.Canvas) (canvasservice.Canvas, error) {
	document, err := documentMap(item.Document)
	if err != nil {
		return canvasservice.Canvas{}, err
	}
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (canvasservice.Canvas, error) {
		owned, err := tx.Project.Query().Where(project.IDEQ(item.ProjectID), project.UserIDEQ(item.UserID), project.StatusEQ("active"), project.DeletedAtIsNil()).Exist(ctx)
		if err != nil {
			return canvasservice.Canvas{}, fmt.Errorf("validate canvas project: %w", err)
		}
		if !owned {
			return canvasservice.Canvas{}, canvasservice.ErrNotFound
		}
		entity, err := tx.CreativeCanvas.Create().SetID(item.ID).SetUserID(item.UserID).SetProjectID(item.ProjectID).SetName(item.Name).SetNameKey(strings.ToLower(item.Name)).SetSchemaVersion(item.SchemaVersion).SetRevision(item.Revision).SetMetadataVersion(item.MetadataVersion).SetDocumentJSON(document).SetDocumentBytes(item.DocumentBytes).SetNodeCount(item.NodeCount).SetEdgeCount(item.EdgeCount).SetRunningTaskCount(item.RunningTaskCount).SetFailedTaskCount(item.FailedTaskCount).SetStatus(string(item.Status)).SetLastSavedAt(item.LastSavedAt).SetCreatedAt(item.CreatedAt).SetUpdatedAt(item.UpdatedAt).Save(ctx)
		if err != nil {
			return canvasservice.Canvas{}, fmt.Errorf("create canvas: %w", err)
		}
		if err := s.rebuildReferences(ctx, tx.Client(), entity.ID, item.UserID, item.AssetReferences); err != nil {
			return canvasservice.Canvas{}, err
		}
		return mapCanvasEntity(ctx, tx.Client(), entity)
	})
}

func (s *CanvasStore) Rename(ctx context.Context, req canvasservice.RenameRequest) (canvasservice.Canvas, error) {
	affected, err := s.client.CreativeCanvas.Update().Where(creativecanvas.IDEQ(req.CanvasID), creativecanvas.UserIDEQ(req.UserID), creativecanvas.MetadataVersionEQ(req.ExpectedMetadataVersion), creativecanvas.StatusEQ("active"), creativecanvas.DeletedAtIsNil()).SetName(req.Name).SetNameKey(strings.ToLower(req.Name)).AddMetadataVersion(1).Save(ctx)
	if err != nil {
		return canvasservice.Canvas{}, fmt.Errorf("rename canvas: %w", err)
	}
	if affected == 0 {
		return canvasservice.Canvas{}, s.metadataError(ctx, req.UserID, req.CanvasID)
	}
	return s.Get(ctx, req.UserID, req.CanvasID)
}

func (s *CanvasStore) SaveDocument(ctx context.Context, req canvasservice.SaveDocumentRecord) (canvasservice.Canvas, error) {
	currentBefore, err := ownedCanvasQuery(s.client, req.UserID, req.CanvasID).Only(ctx)
	if repoent.IsNotFound(err) {
		return canvasservice.Canvas{}, canvasservice.ErrNotFound
	}
	if err != nil {
		return canvasservice.Canvas{}, fmt.Errorf("load canvas before save: %w", err)
	}
	if currentBefore.Revision != req.ExpectedRevision {
		item, mapErr := mapCanvasEntity(ctx, s.client, currentBefore)
		if mapErr != nil {
			return canvasservice.Canvas{}, mapErr
		}
		return item, canvasservice.ErrRevisionChanged
	}
	type result struct{ item canvasservice.Canvas }
	value, err := withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (result, error) {
		current, err := ownedCanvasQuery(tx.Client(), req.UserID, req.CanvasID).Only(ctx)
		if repoent.IsNotFound(err) {
			return result{}, canvasservice.ErrNotFound
		}
		if err != nil {
			return result{}, fmt.Errorf("load canvas for save: %w", err)
		}
		if current.Revision != req.ExpectedRevision {
			return result{}, canvasservice.ErrRevisionChanged
		}
		document, err := documentMap(req.Document)
		if err != nil {
			return result{}, err
		}
		nextRevision := current.Revision + 1
		if _, err := tx.CreativeCanvas.Update().Where(creativecanvas.IDEQ(req.CanvasID), creativecanvas.UserIDEQ(req.UserID), creativecanvas.RevisionEQ(req.ExpectedRevision), creativecanvas.StatusEQ("active"), creativecanvas.DeletedAtIsNil()).SetSchemaVersion(req.Document.SchemaVersion).SetDocumentJSON(document).SetDocumentBytes(len(req.CanonicalJSON)).SetNodeCount(len(req.Document.Nodes)).SetEdgeCount(len(req.Document.Edges)).SetRevision(nextRevision).SetLastSavedAt(time.Now().UTC()).Save(ctx); err != nil {
			return result{}, fmt.Errorf("save canvas document: %w", err)
		}
		if _, err := tx.CreativeCanvasRevision.Create().SetCanvasID(req.CanvasID).SetRevision(nextRevision).SetSchemaVersion(req.Document.SchemaVersion).SetDocumentJSON(document).SetReason(canvasDefaultString(req.Reason, "periodic")).SetCreatedBy("user").SetDocumentBytes(len(req.CanonicalJSON)).Save(ctx); err != nil {
			return result{}, fmt.Errorf("snapshot canvas revision: %w", err)
		}
		if err := s.rebuildReferences(ctx, tx.Client(), req.CanvasID, req.UserID, req.AssetReferences); err != nil {
			return result{}, err
		}
		updated, err := ownedCanvasQuery(tx.Client(), req.UserID, req.CanvasID).Only(ctx)
		if err != nil {
			return result{}, err
		}
		item, err := mapCanvasEntity(ctx, tx.Client(), updated)
		return result{item: item}, err
	})
	if errors.Is(err, canvasservice.ErrRevisionChanged) {
		latest, getErr := s.Get(ctx, req.UserID, req.CanvasID)
		if getErr != nil {
			return canvasservice.Canvas{}, getErr
		}
		return latest, canvasservice.ErrRevisionChanged
	}
	return value.item, err
}

func (s *CanvasStore) Delete(ctx context.Context, req canvasservice.DeleteRequest) error {
	_, err := withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (struct{}, error) {
		entity, err := ownedCanvasQuery(tx.Client(), req.UserID, req.CanvasID).Only(ctx)
		if repoent.IsNotFound(err) {
			return struct{}{}, canvasservice.ErrNotFound
		}
		if err != nil {
			return struct{}{}, err
		}
		if entity.MetadataVersion != req.ExpectedMetadataVersion {
			return struct{}{}, canvasservice.ErrMetadataChanged
		}
		active, err := activeRunQuery(tx.Client(), req.CanvasID).Exist(ctx)
		if err != nil {
			return struct{}{}, err
		}
		if entity.RunningTaskCount > 0 || active {
			return struct{}{}, canvasservice.ErrCanvasBusy
		}
		now := time.Now().UTC()
		if _, err := tx.CreativeCanvas.UpdateOneID(req.CanvasID).SetStatus("deleted").SetDeletedAt(now).AddMetadataVersion(1).Save(ctx); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

func (s *CanvasStore) TransferProject(ctx context.Context, req canvasservice.TransferProjectRequest) (canvasservice.Canvas, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (canvasservice.Canvas, error) {
		entity, err := ownedCanvasQuery(tx.Client(), req.UserID, req.CanvasID).Only(ctx)
		if repoent.IsNotFound(err) {
			return canvasservice.Canvas{}, canvasservice.ErrNotFound
		}
		if err != nil {
			return canvasservice.Canvas{}, err
		}
		if entity.MetadataVersion != req.ExpectedMetadataVersion {
			return canvasservice.Canvas{}, canvasservice.ErrMetadataChanged
		}
		owned, err := tx.Project.Query().Where(project.IDEQ(req.TargetProjectID), project.UserIDEQ(req.UserID), project.StatusEQ("active"), project.DeletedAtIsNil()).Exist(ctx)
		if err != nil {
			return canvasservice.Canvas{}, err
		}
		if !owned {
			return canvasservice.Canvas{}, canvasservice.ErrNotFound
		}
		active, err := activeRunQuery(tx.Client(), req.CanvasID).Exist(ctx)
		if err != nil {
			return canvasservice.Canvas{}, err
		}
		if entity.RunningTaskCount > 0 || active {
			return canvasservice.Canvas{}, canvasservice.ErrCanvasBusy
		}
		updated, err := tx.CreativeCanvas.UpdateOneID(req.CanvasID).SetProjectID(req.TargetProjectID).AddMetadataVersion(1).SetLastTransferredAt(time.Now().UTC()).Save(ctx)
		if err != nil {
			return canvasservice.Canvas{}, err
		}
		return mapCanvasEntity(ctx, tx.Client(), updated)
	})
}

func (s *CanvasStore) CreateRun(ctx context.Context, run canvasservice.Run) (canvasservice.Run, bool, error) {
	type result struct {
		run    canvasservice.Run
		replay bool
	}
	value, err := withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (result, error) {
		existing, queryErr := tx.CanvasGenerationRun.Query().Where(canvasgenerationrun.CanvasIDEQ(run.CanvasID), canvasgenerationrun.NodeIDEQ(run.NodeID), canvasgenerationrun.IdempotencyKeyEQ(run.IdempotencyKey)).Only(ctx)
		if queryErr == nil {
			return result{run: mapRunEntity(existing), replay: true}, nil
		}
		if !repoent.IsNotFound(queryErr) {
			return result{}, queryErr
		}
		owned, err := ownedCanvasQuery(tx.Client(), run.UserID, run.CanvasID).Exist(ctx)
		if err != nil {
			return result{}, err
		}
		if !owned {
			return result{}, canvasservice.ErrNotFound
		}
		snapshot := map[string]any{}
		if err := json.Unmarshal(run.NodeSnapshot, &snapshot); err != nil {
			return result{}, fmt.Errorf("decode run snapshot: %w", err)
		}
		entity, err := tx.CanvasGenerationRun.Create().SetID(run.ID).SetCanvasID(run.CanvasID).SetUserID(run.UserID).SetNodeID(run.NodeID).SetSubmittedRevision(run.SubmittedRevision).SetTaskKind(string(run.TaskKind)).SetTaskID(run.TaskID).SetNodeSnapshot(snapshot).SetStatus(string(run.Status)).SetResultAssetIds(run.ResultAssetIDs).SetIdempotencyKey(run.IdempotencyKey).SetCreatedAt(run.CreatedAt).SetUpdatedAt(run.UpdatedAt).Save(ctx)
		if err != nil {
			return result{}, fmt.Errorf("create canvas run: %w", err)
		}
		if run.Status.Active() {
			if _, err := tx.CreativeCanvas.UpdateOneID(run.CanvasID).AddRunningTaskCount(1).Save(ctx); err != nil {
				return result{}, err
			}
		}
		return result{run: mapRunEntity(entity)}, nil
	})
	return value.run, value.replay, err
}

func (s *CanvasStore) GetRun(ctx context.Context, userID int64, canvasID, runID uuid.UUID) (canvasservice.Run, error) {
	entity, err := s.client.CanvasGenerationRun.Query().Where(canvasgenerationrun.IDEQ(runID), canvasgenerationrun.CanvasIDEQ(canvasID), canvasgenerationrun.UserIDEQ(userID)).Only(ctx)
	if repoent.IsNotFound(err) {
		return canvasservice.Run{}, canvasservice.ErrNotFound
	}
	if err != nil {
		return canvasservice.Run{}, err
	}
	return mapRunEntity(entity), nil
}
func (s *CanvasStore) ListRuns(ctx context.Context, userID int64, canvasID uuid.UUID) ([]canvasservice.Run, error) {
	owned, err := ownedCanvasQuery(s.client, userID, canvasID).Exist(ctx)
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, canvasservice.ErrNotFound
	}
	entities, err := s.client.CanvasGenerationRun.Query().Where(canvasgenerationrun.CanvasIDEQ(canvasID), canvasgenerationrun.UserIDEQ(userID)).Order(repoent.Desc(canvasgenerationrun.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]canvasservice.Run, 0, len(entities))
	for _, entity := range entities {
		items = append(items, mapRunEntity(entity))
	}
	return items, nil
}

func (s *CanvasStore) UpdateRun(ctx context.Context, req canvasservice.RunUpdate) (canvasservice.Run, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (canvasservice.Run, error) {
		entity, err := tx.CanvasGenerationRun.Query().Where(canvasgenerationrun.IDEQ(req.RunID), canvasgenerationrun.CanvasIDEQ(req.CanvasID), canvasgenerationrun.UserIDEQ(req.UserID)).Only(ctx)
		if repoent.IsNotFound(err) {
			return canvasservice.Run{}, canvasservice.ErrNotFound
		}
		if err != nil {
			return canvasservice.Run{}, err
		}
		builder := tx.CanvasGenerationRun.UpdateOneID(req.RunID).SetStatus(string(req.Status)).SetResultAssetIds(req.ResultAssetIDs)
		if req.TaskID != nil {
			builder.SetTaskID(*req.TaskID)
		}
		if req.ErrorCode != "" {
			builder.SetErrorCode(req.ErrorCode)
		}
		if req.ErrorMessage != "" {
			builder.SetErrorMessage(req.ErrorMessage)
		}
		updated, err := builder.Save(ctx)
		if err != nil {
			return canvasservice.Run{}, err
		}
		if canvasservice.RunStatus(entity.Status).Active() && !req.Status.Active() {
			canvas, err := tx.CreativeCanvas.Query().Where(creativecanvas.IDEQ(req.CanvasID)).Only(ctx)
			if err != nil {
				return canvasservice.Run{}, err
			}
			change := tx.CreativeCanvas.UpdateOneID(req.CanvasID)
			if canvas.RunningTaskCount > 0 {
				change.AddRunningTaskCount(-1)
			}
			if req.Status == canvasservice.RunStatusFailed {
				change.AddFailedTaskCount(1)
			}
			if _, err := change.Save(ctx); err != nil {
				return canvasservice.Run{}, err
			}
		}
		return mapRunEntity(updated), nil
	})
}

func (s *CanvasStore) AttachResults(ctx context.Context, req canvasservice.AttachRecord) (canvasservice.Run, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (canvasservice.Run, error) {
		run, err := tx.CanvasGenerationRun.Query().Where(canvasgenerationrun.IDEQ(req.RunID), canvasgenerationrun.CanvasIDEQ(req.CanvasID), canvasgenerationrun.UserIDEQ(req.UserID)).Only(ctx)
		if repoent.IsNotFound(err) {
			return canvasservice.Run{}, canvasservice.ErrNotFound
		}
		if err != nil {
			return canvasservice.Run{}, err
		}
		if run.Status == string(canvasservice.RunStatusAttached) || (run.Status == string(canvasservice.RunStatusUnplaced) && !req.RecoverUnplaced) {
			return mapRunEntity(run), nil
		}
		canvas, err := ownedCanvasQuery(tx.Client(), req.UserID, req.CanvasID).Only(ctx)
		if err != nil {
			return canvasservice.Run{}, err
		}
		document, err := decodeDocument(canvas.DocumentJSON)
		if err != nil {
			return canvasservice.Run{}, err
		}
		if !req.RecoverUnplaced {
			found := false
			for _, node := range document.Nodes {
				if node.ID == run.NodeID {
					found = true
					break
				}
			}
			if !found {
				updated, err := tx.CanvasGenerationRun.UpdateOneID(run.ID).SetStatus(string(canvasservice.RunStatusUnplaced)).Save(ctx)
				if err != nil {
					return canvasservice.Run{}, err
				}
				return mapRunEntity(updated), nil
			}
		}
		if req.RecoverUnplaced && run.Status != string(canvasservice.RunStatusUnplaced) {
			return canvasservice.Run{}, fmt.Errorf("canvas run is not unplaced")
		}
		if !req.RecoverUnplaced && req.ExpectedRevision > 0 && canvas.Revision != req.ExpectedRevision {
			updated, err := tx.CanvasGenerationRun.UpdateOneID(run.ID).SetStatus(string(canvasservice.RunStatusUnplaced)).Save(ctx)
			if err != nil {
				return canvasservice.Run{}, err
			}
			return mapRunEntity(updated), nil
		}
		nodeIndexes := make(map[string]int, len(document.Nodes))
		for index, node := range document.Nodes {
			nodeIndexes[node.ID] = index
		}
		for _, node := range req.UpdatedNodes {
			if index, ok := nodeIndexes[node.ID]; ok {
				document.Nodes[index] = node
			}
		}
		nodes := map[string]struct{}{}
		for _, node := range document.Nodes {
			nodes[node.ID] = struct{}{}
		}
		for _, node := range req.Nodes {
			if _, ok := nodes[node.ID]; !ok {
				document.Nodes = append(document.Nodes, node)
			}
		}
		edges := map[string]struct{}{}
		for _, edge := range document.Edges {
			edges[edge.ID] = struct{}{}
		}
		for _, edge := range req.Edges {
			if _, ok := edges[edge.ID]; !ok {
				document.Edges = append(document.Edges, edge)
			}
		}
		raw, _ := json.Marshal(document)
		documentJSON, _ := documentMap(document)
		next := canvas.Revision + 1
		update := tx.CreativeCanvas.Update().Where(creativecanvas.IDEQ(canvas.ID), creativecanvas.RevisionEQ(canvas.Revision), creativecanvas.StatusEQ("active"), creativecanvas.DeletedAtIsNil()).SetDocumentJSON(documentJSON).SetDocumentBytes(len(raw)).SetNodeCount(len(document.Nodes)).SetEdgeCount(len(document.Edges)).SetRevision(next).SetLastSavedAt(time.Now().UTC())
		affected, err := update.Save(ctx)
		if err != nil {
			return canvasservice.Run{}, err
		}
		if affected == 0 {
			updated, err := tx.CanvasGenerationRun.UpdateOneID(run.ID).SetStatus(string(canvasservice.RunStatusUnplaced)).Save(ctx)
			if err != nil {
				return canvasservice.Run{}, err
			}
			return mapRunEntity(updated), nil
		}
		if _, err := tx.CreativeCanvasRevision.Create().SetCanvasID(canvas.ID).SetRevision(next).SetSchemaVersion(document.SchemaVersion).SetDocumentJSON(documentJSON).SetReason("attach").SetCreatedBy("system").SetDocumentBytes(len(raw)).Save(ctx); err != nil {
			return canvasservice.Run{}, err
		}
		refs := make([]uuid.UUID, 0)
		for _, value := range domaincanvas.ExtractAssetReferences(document) {
			if id, parseErr := uuid.Parse(value); parseErr == nil {
				refs = append(refs, id)
			}
		}
		if err := s.rebuildReferences(ctx, tx.Client(), canvas.ID, req.UserID, refs); err != nil {
			return canvasservice.Run{}, err
		}
		updated, err := tx.CanvasGenerationRun.UpdateOneID(run.ID).SetStatus(string(canvasservice.RunStatusAttached)).SetAttachedRevision(next).Save(ctx)
		if err != nil {
			return canvasservice.Run{}, err
		}
		return mapRunEntity(updated), nil
	})
}

func (s *CanvasStore) rebuildReferences(ctx context.Context, client *repoent.Client, canvasID uuid.UUID, userID int64, refs []uuid.UUID) error {
	now := time.Now().UTC()
	if _, err := client.MediaAssetReference.Update().Where(mediaassetreference.RefTypeEQ("canvas_node"), mediaassetreference.RefIDEQ(canvasID), mediaassetreference.DeletedAtIsNil()).SetDeletedAt(now).Save(ctx); err != nil {
		return fmt.Errorf("clear canvas references: %w", err)
	}
	for _, assetID := range refs {
		owned, err := client.MediaAsset.Query().Where(mediaasset.IDEQ(assetID), mediaasset.UserIDEQ(userID)).Exist(ctx)
		if err != nil {
			return err
		}
		if !owned {
			return canvasservice.ErrNotFound
		}
		if _, err := client.MediaAssetReference.Create().SetAssetID(assetID).SetRefType("canvas_node").SetRefID(canvasID).SetRefKey(assetID.String()).SetUserID(userID).Save(ctx); err != nil {
			return fmt.Errorf("create canvas reference: %w", err)
		}
	}
	return nil
}
func (s *CanvasStore) metadataError(ctx context.Context, userID int64, id uuid.UUID) error {
	exists, err := ownedCanvasQuery(s.client, userID, id).Exist(ctx)
	if err != nil {
		return err
	}
	if !exists {
		return canvasservice.ErrNotFound
	}
	return canvasservice.ErrMetadataChanged
}
func ownedCanvasQuery(client *repoent.Client, userID int64, id uuid.UUID) *repoent.CreativeCanvasQuery {
	return client.CreativeCanvas.Query().Where(creativecanvas.IDEQ(id), creativecanvas.UserIDEQ(userID), creativecanvas.StatusEQ("active"), creativecanvas.DeletedAtIsNil())
}
func activeRunQuery(client *repoent.Client, canvasID uuid.UUID) *repoent.CanvasGenerationRunQuery {
	return client.CanvasGenerationRun.Query().Where(canvasgenerationrun.CanvasIDEQ(canvasID), canvasgenerationrun.StatusIn(string(canvasservice.RunStatusSubmitting), string(canvasservice.RunStatusQueued), string(canvasservice.RunStatusRunning), string(canvasservice.RunStatusSaving)))
}
func mapCanvasEntity(ctx context.Context, client *repoent.Client, entity *repoent.CreativeCanvas) (canvasservice.Canvas, error) {
	document, err := decodeDocument(entity.DocumentJSON)
	if err != nil {
		return canvasservice.Canvas{}, err
	}
	refs, err := client.MediaAssetReference.Query().Where(mediaassetreference.RefTypeEQ("canvas_node"), mediaassetreference.RefIDEQ(entity.ID), mediaassetreference.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return canvasservice.Canvas{}, err
	}
	assetRefs := make([]uuid.UUID, 0, len(refs))
	for _, ref := range refs {
		assetRefs = append(assetRefs, ref.AssetID)
	}
	item := canvasservice.Canvas{ID: entity.ID, UserID: entity.UserID, ProjectID: entity.ProjectID, Name: entity.Name, SchemaVersion: entity.SchemaVersion, Revision: entity.Revision, MetadataVersion: entity.MetadataVersion, Document: document, DocumentBytes: entity.DocumentBytes, NodeCount: entity.NodeCount, EdgeCount: entity.EdgeCount, AssetReferences: assetRefs, RunningTaskCount: entity.RunningTaskCount, FailedTaskCount: entity.FailedTaskCount, Status: canvasservice.CanvasStatus(entity.Status), LastSavedAt: entity.LastSavedAt, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt}
	if entity.LastTransferredAt != nil {
		item.LastTransferredAt = *entity.LastTransferredAt
	}
	return item, nil
}
func mapRunEntity(entity *repoent.CanvasGenerationRun) canvasservice.Run {
	snapshot, _ := json.Marshal(entity.NodeSnapshot)
	run := canvasservice.Run{ID: entity.ID, CanvasID: entity.CanvasID, UserID: entity.UserID, NodeID: entity.NodeID, SubmittedRevision: entity.SubmittedRevision, TaskKind: canvasservice.TaskKind(entity.TaskKind), TaskID: entity.TaskID, NodeSnapshot: snapshot, Status: canvasservice.RunStatus(entity.Status), ResultAssetIDs: append([]uuid.UUID(nil), entity.ResultAssetIds...), AttachedRevision: entity.AttachedRevision, IdempotencyKey: entity.IdempotencyKey, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt}
	if entity.ErrorCode != nil {
		run.ErrorCode = *entity.ErrorCode
	}
	if entity.ErrorMessage != nil {
		run.ErrorMessage = *entity.ErrorMessage
	}
	return run
}
func documentMap(document domaincanvas.DocumentV1) (map[string]any, error) {
	raw, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode canvas document: %w", err)
	}
	result := map[string]any{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("map canvas document: %w", err)
	}
	return result, nil
}
func decodeDocument(value map[string]any) (domaincanvas.DocumentV1, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return domaincanvas.DocumentV1{}, err
	}
	var result domaincanvas.DocumentV1
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	return domaincanvas.NormalizeCollections(result), nil
}
func canvasDefaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
