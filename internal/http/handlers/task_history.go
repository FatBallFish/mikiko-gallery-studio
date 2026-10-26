package handlers

import (
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
	"github.com/google/uuid"
	"net/http"
	"strings"
)

func (a *API) HandleImageHistoryClearFailed(w http.ResponseWriter, r *http.Request) {
	a.handleHistoryClearFailed(w, r, false)
}
func (a *API) HandleVideoHistoryClearFailed(w http.ResponseWriter, r *http.Request) {
	a.handleHistoryClearFailed(w, r, true)
}
func (a *API) handleHistoryClearFailed(w http.ResponseWriter, r *http.Request, video bool) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	var body struct {
		ProjectID string `json:"project_id,omitempty"`
	}
	if err := decodeStrictJSON(r, &body); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	var count int
	var err error
	if video {
		if a.videoTasks == nil {
			httpx.WriteError(w, r, errs.Internal("video tasks unavailable"))
			return
		}
		count, err = a.videoTasks.ClearFailedHistory(r.Context(), user.ID, body.ProjectID)
	} else {
		count, err = a.tasks.ClearFailedHistory(r.Context(), user.ID, body.ProjectID)
	}
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]int{"hidden_count": count})
}
func (a *API) HandleVideoHistoryTaskDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/agent/video/v1/history/tasks/")
	retry := strings.HasSuffix(path, "/retry")
	if retry {
		path = strings.TrimSuffix(path, "/retry")
	}
	if (retry && r.Method != http.MethodPost) || (!retry && r.Method != http.MethodDelete) {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.videoTasks == nil {
		httpx.WriteError(w, r, errs.Internal("video tasks unavailable"))
		return
	}
	id, err := uuid.Parse(path)
	if err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid task id"))
		return
	}
	if retry {
		if appErr := a.requireFeature(r.Context(), "video_creation"); appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		task, err := a.videoTasks.Retry(r.Context(), user.ID, id, userGroupCodes(user))
		if err != nil {
			httpx.WriteError(w, r, normalizeVideoTaskError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusAccepted, task)
		return
	}
	if err := a.videoTasks.HideHistory(r.Context(), user.ID, id); err != nil {
		httpx.WriteError(w, r, normalizeVideoTaskError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
