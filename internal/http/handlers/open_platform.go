package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
	"github.com/google/uuid"
)

type OpenPlatform struct {
	service      *openplatform.Service
	actor        func(*http.Request) (openplatform.Principal, error)
	uploadAccess http.Handler
}

func NewOpenPlatform(service *openplatform.Service, actor func(*http.Request) (openplatform.Principal, error)) *OpenPlatform {
	return &OpenPlatform{service: service, actor: actor}
}
func (h *OpenPlatform) SetUploadAccess(handler http.Handler) { h.uploadAccess = handler }

func (h *OpenPlatform) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h != nil && h.uploadAccess != nil && strings.HasPrefix(r.URL.Path, "/api/open/v1/upload-access/") {
		h.uploadAccess.ServeHTTP(w, r)
		return
	}
	if h == nil || h.service == nil || h.actor == nil {
		httpx.WriteError(w, r, errs.Internal("open platform unavailable"))
		return
	}
	p, err := h.actor(r)
	if err != nil {
		writeOpenError(w, r, err)
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/open/v1"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 3 && (parts[0] == "image-tasks" || parts[0] == "video-tasks") && parts[2] == "events" && r.Method == http.MethodGet {
		h.events(w, r, p, parts[0], parts[1])
		return
	}
	if len(parts) == 4 && parts[0] == "uploads" && parts[2] == "parts" && r.Method == http.MethodPut {
		id, e1 := uuid.Parse(parts[1])
		number, e2 := strconv.Atoi(parts[3])
		if e1 != nil || e2 != nil || number < 1 {
			writeOpenError(w, r, errs.BadRequest("invalid upload part"))
			return
		}
		part, err := h.service.UploadPart(r.Context(), p, id, number, r.Body, r.ContentLength, r.Header.Get("X-Checksum-SHA256"))
		if err != nil {
			writeOpenError(w, r, err)
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, part)
		return
	}
	op, extra := openRoute(parts, r.Method)
	if op == "" {
		httpx.WriteError(w, r, errs.New(404, "NOT_FOUND", "endpoint not found"))
		return
	}
	input := map[string]any{}
	for key, values := range r.URL.Query() {
		if key == "access_token" || key == "token" {
			writeOpenError(w, r, errs.BadRequest("URL tokens are not accepted"))
			return
		}
		if len(values) == 1 {
			if key == "limit" || key == "expected_version" {
				n, err := strconv.ParseInt(values[0], 10, 64)
				if err != nil {
					writeOpenError(w, r, errs.BadRequest("invalid "+key))
					return
				}
				input[key] = n
			} else {
				input[key] = values[0]
			}
		}
	}
	if r.Method != http.MethodGet && r.Body != nil {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
		if err != nil {
			writeOpenError(w, r, errs.BadRequest("request body too large"))
			return
		}
		if len(body) > 0 {
			if err = json.Unmarshal(body, &input); err != nil || input == nil {
				writeOpenError(w, r, errs.BadRequest("JSON object required"))
				return
			}
		}
	}
	for key, value := range extra {
		input[key] = value
	}
	if key := r.Header.Get("Idempotency-Key"); key != "" {
		input["idempotency_key"] = key
	}
	raw, _ := json.Marshal(input)
	result, err := h.service.Execute(r.Context(), p, op, raw)
	if err != nil {
		writeOpenError(w, r, err)
		return
	}
	status := http.StatusOK
	if strings.HasPrefix(op, "create_") {
		status = http.StatusCreated
	}
	httpx.WriteSuccess(w, r, status, result)
}
func openRoute(parts []string, method string) (string, map[string]any) {
	extra := map[string]any{}
	if len(parts) == 0 {
		return "", extra
	}
	resource := parts[0]
	if method == http.MethodGet {
		switch strings.Join(parts, "/") {
		case "me":
			return "get_current_user", extra
		case "me/balance":
			return "get_balance", extra
		case "image-models":
			return "list_image_models", extra
		case "video-models":
			return "list_video_models", extra
		}
	}
	if resource == "image-tasks" || resource == "video-tasks" {
		kind := strings.TrimSuffix(resource, "-tasks")
		switch {
		case kind == "image" && len(parts) == 3 && parts[2] == "layers" && method == http.MethodGet:
			extra["id"] = parts[1]
			return "get_image_task_layers", extra
		case len(parts) == 5 && parts[2] == "results" && parts[4] == "access" && method == http.MethodPost:
			extra["task_id"] = parts[1]
			if kind == "image" {
				extra["image_id"] = parts[3]
			} else {
				extra["asset_id"] = parts[3]
			}
			return "get_" + kind + "_result_access", extra
		case len(parts) == 1 && method == http.MethodGet:
			return "list_" + kind + "_tasks", extra
		case len(parts) == 1 && method == http.MethodPost:
			return "create_" + kind + "_task", extra
		case len(parts) == 2 && parts[1] == "preflight" && method == http.MethodPost:
			return "preflight_" + kind + "_task", extra
		case len(parts) == 2 && method == http.MethodGet:
			extra["id"] = parts[1]
			return "get_" + kind + "_task", extra
		case kind == "video" && len(parts) == 3 && parts[2] == "cancel" && method == http.MethodPost:
			extra["id"] = parts[1]
			return "cancel_video_task", extra
		}
		return "", extra
	}
	singular := map[string]string{"assets": "asset", "projects": "project", "groups": "group", "uploads": "upload"}[resource]
	if singular == "" {
		return "", extra
	}
	if len(parts) == 1 {
		if method == http.MethodGet && singular != "upload" {
			return "list_" + resource, extra
		}
		if method == http.MethodPost && singular != "asset" {
			return "create_" + singular, extra
		}
	}
	if len(parts) >= 2 {
		extra["id"] = parts[1]
	}
	if len(parts) == 2 {
		switch method {
		case http.MethodGet:
			return "get_" + singular, extra
		case http.MethodPatch:
			if singular != "upload" {
				return "update_" + singular, extra
			}
		case http.MethodDelete:
			if singular != "upload" {
				return "delete_" + singular, extra
			}
		}
	}
	if len(parts) == 3 && method == http.MethodPost {
		if singular == "asset" && parts[2] == "access" {
			return "get_asset_access", extra
		}
		if singular == "upload" && (parts[2] == "complete" || parts[2] == "abort") {
			return parts[2] + "_upload", extra
		}
	}
	if len(parts) == 5 && singular == "upload" && parts[2] == "parts" && parts[4] == "url" && method == http.MethodPost {
		n, err := strconv.Atoi(parts[3])
		if err == nil {
			extra["part_number"] = n
			return "get_upload_part_url", extra
		}
	}
	return "", extra
}
func writeOpenError(w http.ResponseWriter, r *http.Request, err error) {
	var app *errs.Error
	if !errors.As(err, &app) {
		app = errs.Internal("operation failed")
	}
	httpx.WriteError(w, r, app)
}
func (h *OpenPlatform) events(w http.ResponseWriter, r *http.Request, p openplatform.Principal, kind, id string) {
	if r.URL.Query().Has("token") || r.URL.Query().Has("access_token") {
		writeOpenError(w, r, errs.BadRequest("URL tokens are not accepted"))
		return
	}
	op := "get_" + strings.TrimSuffix(kind, "-tasks") + "_task"
	input, _ := json.Marshal(map[string]any{"id": id})
	task, err := h.service.Execute(r.Context(), p, op, input)
	if err != nil {
		writeOpenError(w, r, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeOpenError(w, r, errs.Internal("stream unavailable"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	previous := ""
	for {
		raw, err := json.Marshal(task)
		if err != nil {
			return
		}
		sum := uuid.NewSHA1(uuid.NameSpaceOID, raw).String()
		if previous != sum {
			if _, err = fmt.Fprintf(w, "id: %s\nevent: task.status\ndata: %s\n\n", sum, raw); err != nil {
				return
			}
			flusher.Flush()
			previous = sum
		}
		var state struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(raw, &state)
		switch state.Status {
		case "succeeded", "failed", "partial", "partial_failed", "cancelled", "canceled", "rejected", "deleted":
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
		task, err = h.service.Execute(r.Context(), p, op, input)
		if err != nil {
			body, _ := json.Marshal(map[string]string{"error": "authorization or task access is no longer available"})
			_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", body)
			flusher.Flush()
			return
		}
	}
}
