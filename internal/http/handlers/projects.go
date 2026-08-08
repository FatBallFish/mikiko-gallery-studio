package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

const projectPathPrefix = "/api/agent/project/v1/projects/"

func (a *API) HandleProjects(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := a.projects.List(r.Context(), user.ID)
		if err != nil {
			httpx.WriteError(w, r, projectAppError(err))
			return
		}
		defaultID := ""
		for _, item := range items {
			if item.IsDefault {
				defaultID = item.ID
				break
			}
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": items, "default_project_id": defaultID})
	case http.MethodPost:
		var req domainproject.CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		req.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		created, err := a.projects.Create(r.Context(), user.ID, req)
		if err != nil {
			httpx.WriteError(w, r, projectAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, created)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleProjectDetail(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	projectID := strings.Trim(strings.TrimPrefix(r.URL.Path, projectPathPrefix), "/")
	if projectID == "" || strings.Contains(projectID, "/") {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "project not found"))
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req domainproject.RenameRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		updated, err := a.projects.Rename(r.Context(), user.ID, projectID, req)
		if err != nil {
			httpx.WriteError(w, r, projectAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	case http.MethodDelete:
		var req domainproject.DeleteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		deleted, err := a.projects.Delete(r.Context(), user.ID, projectID, req)
		if err != nil {
			httpx.WriteError(w, r, projectAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, deleted)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func projectAppError(err error) *errs.Error {
	var nonEmpty *projectservice.NonEmptyError
	switch {
	case errors.As(err, &nonEmpty):
		return errs.WithDetails(errs.New(http.StatusConflict, errs.CodeProjectNotEmpty, "project contains assets; choose a transfer target"), map[string]any{
			"counts": nonEmpty.Counts,
		})
	case errors.Is(err, projectservice.ErrDefaultImmutable):
		return errs.New(http.StatusForbidden, errs.CodeDefaultProjectImmutable, "default project cannot be changed")
	case errors.Is(err, projectservice.ErrProjectChanged):
		return errs.New(http.StatusConflict, errs.CodeProjectChanged, "project changed; refresh and retry")
	case errors.Is(err, projectservice.ErrNameConflict):
		return errs.New(http.StatusConflict, errs.CodeConflict, "project name already exists")
	case errors.Is(err, projectservice.ErrNotFound):
		return errs.New(http.StatusNotFound, errs.CodeNotFound, "project not found")
	case errors.Is(err, projectservice.ErrInvalid):
		return errs.BadRequest("invalid project request")
	default:
		return errs.Internal("project operation failed")
	}
}
