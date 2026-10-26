package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
	"io"
	"net/http"
	"strings"
)

func parseNativeOptionsQuery(r *http.Request) (*nativeimage.Options, *errs.Error) {
	raw := r.URL.Query().Get("native_options")
	if raw == "" {
		return nil, nil
	}
	if len(raw) > 16384 {
		return nil, errs.BadRequest("native_options exceeds 16384 bytes")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var options nativeimage.Options
	if err := decoder.Decode(&options); err != nil {
		return nil, errs.BadRequest("invalid native_options")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errs.BadRequest("invalid native_options trailing data")
	}
	return &options, nil
}

func layerTaskID(r *http.Request) string {
	path := strings.TrimPrefix(r.URL.Path, "/api/agent/image/v1/tasks/")
	return strings.SplitN(path, "/", 2)[0]
}

func (a *API) ImageTaskLayers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	task, err := a.tasks.GetByID(r.Context(), user.ID, layerTaskID(r))
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	if task.Operation != nativeimage.OperationLayerDecomposition {
		httpx.WriteError(w, r, errs.BadRequest("task is not a layer decomposition"))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"version": 1, "task_id": task.ID, "operation": task.Operation, "layers": task.Results})
}

func (a *API) ImageTaskLayerArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	task, err := a.tasks.GetByID(r.Context(), user.ID, layerTaskID(r))
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	if task.Operation != nativeimage.OperationLayerDecomposition || len(task.Results) == 0 || len(task.Results) > 17 {
		httpx.WriteError(w, r, errs.BadRequest("task has no valid layer set"))
		return
	}
	// Resolve only owned persisted results. No client supplied URLs or filenames.
	var body bytes.Buffer
	archive := zip.NewWriter(&body)
	manifest := make([]map[string]any, 0, len(task.Results))
	var total int
	for i, result := range task.Results {
		stored, content, downloadErr := a.tasks.DownloadImageResult(r.Context(), user.ID, result.ID)
		if downloadErr != nil {
			httpx.WriteError(w, r, normalizeAppError(downloadErr))
			return
		}
		total += len(content)
		if total > 128<<20 {
			httpx.WriteError(w, r, errs.BadRequest("layer archive exceeds 128 MiB"))
			return
		}
		ext := "png"
		if stored.MimeType == "image/jpeg" {
			ext = "jpg"
		} else if stored.MimeType == "image/webp" {
			ext = "webp"
		}
		filename := fmt.Sprintf("layer-%02d.%s", i, ext)
		file, writeErr := archive.Create(filename)
		if writeErr == nil {
			_, writeErr = file.Write(content)
		}
		if writeErr != nil {
			httpx.WriteError(w, r, errs.Internal("failed to create layer archive"))
			return
		}
		manifest = append(manifest, map[string]any{"file": filename, "id": result.ID, "width": stored.Width, "height": stored.Height, "layer": result.Layer})
	}
	raw, _ := json.Marshal(map[string]any{"version": 1, "task_id": task.ID, "layers": manifest})
	file, err := archive.Create("manifest.json")
	if err == nil {
		_, err = file.Write(raw)
	}
	if err == nil {
		err = archive.Close()
	}
	if err != nil {
		httpx.WriteError(w, r, errs.Internal("failed to create layer archive"))
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="layers.zip"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body.Bytes())
}
