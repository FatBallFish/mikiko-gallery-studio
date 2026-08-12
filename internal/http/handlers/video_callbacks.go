package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

const (
	videoCallbackPathPrefix = "/api/open/video/v1/provider-callbacks/"
	videoCallbackMaxBody    = int64(1 << 20)
)

func (a *API) HandleVideoProviderCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	if a.videoCallbacks == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeInternal, "video callbacks are unavailable"))
		return
	}
	remainder := strings.TrimPrefix(r.URL.Path, videoCallbackPathPrefix)
	segments := strings.Split(remainder, "/")
	if len(segments) != 2 || strings.TrimSpace(segments[0]) == "" {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "video callback endpoint not found"))
		return
	}
	accountPublicID, err := uuid.Parse(segments[1])
	if err != nil {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "video callback endpoint not found"))
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, videoCallbackMaxBody+1))
	if err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid callback body"))
		return
	}
	if int64(len(body)) > videoCallbackMaxBody {
		httpx.WriteError(w, r, errs.New(http.StatusRequestEntityTooLarge, errs.CodeBadRequest, "callback body is too large"))
		return
	}
	result, err := a.videoCallbacks.Receive(r.Context(), segments[0], accountPublicID, r.Header.Clone(), body)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	if result.Challenge != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"challenge": result.Challenge})
		return
	}
	httpx.WriteSuccess(w, r, http.StatusAccepted, result)
}
