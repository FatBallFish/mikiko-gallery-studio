package handlers

import (
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
	"github.com/google/uuid"
	"net/http"
	"strings"
)

func (a *API) HandleMediaGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.mediaAssets == nil {
		httpx.WriteError(w, r, errs.Internal("media asset service is unavailable"))
		return
	}
	var projectID *uuid.UUID
	if value := strings.TrimSpace(r.URL.Query().Get("project_id")); value != "" {
		id, err := uuid.Parse(value)
		if err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid project_id"))
			return
		}
		projectID = &id
	}
	groups, err := a.mediaAssets.ListGroups(r.Context(), user.ID, projectID)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, groups)
}
