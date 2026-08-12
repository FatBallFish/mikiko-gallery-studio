package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	domaincanvas "github.com/fatballfish/pic-gallery/internal/domain/canvas"
	canvasservice "github.com/fatballfish/pic-gallery/internal/service/canvas"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

const canvasPathPrefix = "/api/agent/canvas/v1/canvases/"

func (a *API) HandleCanvases(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.canvases == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeInternal, "canvas service is unavailable"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		req := canvasservice.ListRequest{UserID: user.ID, Search: r.URL.Query().Get("search")}
		if raw := strings.TrimSpace(r.URL.Query().Get("project_id")); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				httpx.WriteError(w, r, errs.BadRequest("invalid project_id"))
				return
			}
			req.ProjectID = &id
		}
		items, err := a.canvases.List(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, canvasAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		if appErr := a.requireFeature(r.Context(), "creative_canvas"); appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		var req canvasservice.CreateRequest
		if err := decodeCanvasJSON(r, &req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		req.UserID = user.ID
		created, err := a.canvases.Create(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, canvasAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, created)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleCanvasDetail(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.canvases == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeInternal, "canvas service is unavailable"))
		return
	}
	remainder := strings.TrimPrefix(r.URL.Path, canvasPathPrefix)
	parts := strings.Split(strings.Trim(remainder, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "canvas not found"))
		return
	}
	canvasPart := parts[0]
	action := ""
	if strings.Contains(canvasPart, ":") {
		canvasPart, action, _ = strings.Cut(canvasPart, ":")
	}
	canvasID, err := uuid.Parse(canvasPart)
	if err != nil {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "canvas not found"))
		return
	}
	if action != "" {
		a.handleCanvasAction(w, r, user.ID, canvasID, action)
		return
	}
	if len(parts) == 1 {
		a.handleCanvasRoot(w, r, user.ID, canvasID)
		return
	}
	switch parts[1] {
	case "document":
		a.handleCanvasDocument(w, r, user.ID, canvasID)
	case "runs":
		a.handleCanvasRuns(w, r, user.ID, canvasID, parts)
	case "nodes":
		a.handleCanvasNodes(w, r, user, canvasID, parts)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "canvas route not found"))
	}
}

func (a *API) handleCanvasRoot(w http.ResponseWriter, r *http.Request, userID int64, canvasID uuid.UUID) {
	switch r.Method {
	case http.MethodGet:
		item, err := a.canvases.Get(r.Context(), userID, canvasID)
		if err != nil {
			httpx.WriteError(w, r, canvasAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, item)
	case http.MethodPatch:
		if appErr := a.requireFeature(r.Context(), "creative_canvas"); appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		var req canvasservice.RenameRequest
		if decodeCanvasJSON(r, &req) != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		req.UserID, req.CanvasID = userID, canvasID
		item, err := a.canvases.Rename(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, canvasAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, item)
	case http.MethodDelete:
		if appErr := a.requireFeature(r.Context(), "creative_canvas"); appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		var req canvasservice.DeleteRequest
		if decodeCanvasJSON(r, &req) != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		req.UserID, req.CanvasID = userID, canvasID
		if err := a.canvases.Delete(r.Context(), req); err != nil {
			httpx.WriteError(w, r, canvasAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"id": canvasID, "status": "deleted"})
	default:
		writeMethodNotAllowed(w, r)
	}
}
func (a *API) handleCanvasAction(w http.ResponseWriter, r *http.Request, userID int64, canvasID uuid.UUID, action string) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	if featureWriteBlocked("creative_canvas", r.Method, action) {
		if appErr := a.requireFeature(r.Context(), "creative_canvas"); appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
	}
	switch action {
	case "transfer-project":
		var req canvasservice.TransferProjectRequest
		if decodeCanvasJSON(r, &req) != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		req.UserID, req.CanvasID = userID, canvasID
		item, err := a.canvases.TransferProject(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, canvasAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, item)
	case "duplicate":
		var req canvasservice.DuplicateRequest
		if decodeCanvasJSON(r, &req) != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		req.UserID, req.CanvasID = userID, canvasID
		item, err := a.canvases.Duplicate(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, canvasAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, item)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "canvas action not found"))
	}
}
func (a *API) handleCanvasDocument(w http.ResponseWriter, r *http.Request, userID int64, canvasID uuid.UUID) {
	if r.Method != http.MethodPut {
		writeMethodNotAllowed(w, r)
		return
	}
	if appErr := a.requireFeature(r.Context(), "creative_canvas"); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	var body struct {
		ExpectedRevision int64                   `json:"expected_revision"`
		Document         domaincanvas.DocumentV1 `json:"document"`
	}
	if decodeCanvasJSON(r, &body) != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	item, err := a.canvases.SaveDocument(r.Context(), canvasservice.SaveDocumentRequest{UserID: userID, CanvasID: canvasID, ExpectedRevision: body.ExpectedRevision, Document: body.Document})
	if err != nil {
		httpx.WriteError(w, r, canvasAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, item)
}
func (a *API) handleCanvasNodes(w http.ResponseWriter, r *http.Request, user *domainauth.User, canvasID uuid.UUID, parts []string) {
	if len(parts) != 3 || r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "canvas node action not found"))
		return
	}
	nodeID, action, ok := strings.Cut(parts[2], ":")
	if !ok || nodeID == "" {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "canvas node action not found"))
		return
	}
	if featureWriteBlocked("creative_canvas", r.Method, action) {
		if appErr := a.requireFeature(r.Context(), "creative_canvas"); appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
	}
	req := canvasservice.GenerateRequest{UserID: user.ID, UserGroupCode: user.GroupCode, UserGroupCodes: userGroupCodes(user), UserGroupMultiplier: user.GroupMultiplier, CanvasID: canvasID, NodeID: nodeID, IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key"))}
	switch action {
	case "estimate":
		estimate, err := a.canvases.Estimate(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, canvasAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, estimate)
	case "generate":
		run, err := a.canvases.Generate(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, canvasAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusAccepted, run)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "canvas node action not found"))
	}
}
func (a *API) handleCanvasRuns(w http.ResponseWriter, r *http.Request, userID int64, canvasID uuid.UUID, parts []string) {
	if len(parts) == 2 {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w, r)
			return
		}
		refresh, _ := strconv.ParseBool(r.URL.Query().Get("refresh"))
		runs, err := a.canvases.ListRuns(r.Context(), userID, canvasID, refresh)
		if err != nil {
			httpx.WriteError(w, r, canvasAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": runs})
		return
	}
	if len(parts) != 3 || r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "canvas run action not found"))
		return
	}
	idPart, action, ok := strings.Cut(parts[2], ":")
	runID, err := uuid.Parse(idPart)
	if !ok || err != nil {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "canvas run action not found"))
		return
	}
	if featureWriteBlocked("creative_canvas", r.Method, action) {
		if appErr := a.requireFeature(r.Context(), "creative_canvas"); appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
	}
	var run canvasservice.Run
	switch action {
	case "attach-results":
		var req canvasservice.AttachResultsRequest
		if decodeErr := decodeCanvasJSON(r, &req); decodeErr != nil && !errors.Is(decodeErr, io.EOF) {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		req.UserID, req.CanvasID, req.RunID = userID, canvasID, runID
		run, err = a.canvases.AttachResults(r.Context(), req)
	case "cancel":
		run, err = a.canvases.CancelRun(r.Context(), userID, canvasID, runID)
	case "refresh":
		run, err = a.canvases.RefreshRun(r.Context(), userID, canvasID, runID)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "canvas run action not found"))
		return
	}
	if err != nil {
		httpx.WriteError(w, r, canvasAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, run)
}
func decodeCanvasJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}
func canvasAppError(err error) *errs.Error {
	var conflict *canvasservice.RevisionConflictError
	var validation *domaincanvas.ValidationError
	var appErr *errs.Error
	switch {
	case errors.As(err, &conflict):
		return errs.WithDetails(errs.New(http.StatusConflict, "CANVAS_REVISION_CHANGED", "canvas changed; refresh or copy local version"), map[string]any{"remote_revision": conflict.RemoteRevision, "remote_updated_at": conflict.RemoteUpdatedAt, "summary": conflict.Summary})
	case errors.As(err, &validation):
		return errs.WithDetails(errs.New(http.StatusUnprocessableEntity, errs.CodeValidationFailed, validation.Error()), map[string]any{"validation_code": validation.Code, "node_id": validation.NodeID, "edge_id": validation.EdgeID})
	case errors.Is(err, canvasservice.ErrCanvasBusy):
		return errs.New(http.StatusConflict, "CANVAS_BUSY", "canvas has active generation runs")
	case errors.Is(err, canvasservice.ErrMetadataChanged):
		return errs.New(http.StatusConflict, "CANVAS_METADATA_CHANGED", "canvas metadata changed; refresh and retry")
	case errors.Is(err, canvasservice.ErrNotFound):
		return errs.New(http.StatusNotFound, errs.CodeNotFound, "canvas not found")
	case errors.As(err, &appErr):
		return appErr
	default:
		return errs.BadRequest(err.Error())
	}
}
