package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	compatservice "github.com/fatballfish/pic-gallery/internal/service/compat"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
	"github.com/google/uuid"
)

func (a *API) SetCompatOperations(operations compatservice.Operations) {
	a.compat.SetOperations(operations)
}

type compatInput struct {
	Model             string `json:"model"`
	Prompt            string `json:"prompt"`
	Size              string `json:"size"`
	N                 int    `json:"n"`
	Quality           string `json:"quality"`
	OutputFormat      string `json:"output_format"`
	Background        string `json:"background"`
	OutputCompression int    `json:"output_compression"`
	Moderation        string `json:"moderation"`
	ResponseFormat    string `json:"response_format"`
	User              string `json:"user"`
	Stream            bool   `json:"stream"`
	PartialImages     *int   `json:"partial_images,omitempty"`
}

func (a *API) HandleOpenAIImageGeneration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeCompatError(w, methodNotAllowedError())
		return
	}
	p, appErr := a.GetOpenPrincipal(r, "images:write", "images:read")
	if appErr != nil {
		a.writeCompatError(w, appErr)
		return
	}
	var input compatInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		a.writeCompatError(w, errs.BadRequest("invalid or unsupported image generation parameters"))
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		a.writeCompatError(w, errs.BadRequest("expected one JSON object"))
		return
	}
	a.startCompat(w, r, p, input, nil, nil)
}

func (a *API) HandleOpenAIImageEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeCompatError(w, methodNotAllowedError())
		return
	}
	p, appErr := a.GetOpenPrincipal(r, "images:write", "images:read")
	if appErr != nil {
		a.writeCompatError(w, appErr)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		a.writeCompatError(w, errs.BadRequest("invalid multipart form"))
		return
	}
	defer r.MultipartForm.RemoveAll()
	allowed := map[string]bool{"model": true, "prompt": true, "size": true, "n": true, "quality": true, "response_format": true, "user": true, "stream": true, "partial_images": true, "output_format": true, "background": true, "output_compression": true, "moderation": true}
	for name, values := range r.MultipartForm.Value {
		if !allowed[name] || len(values) != 1 {
			a.writeCompatError(w, errs.BadRequest("unsupported or repeated edit parameter: "+name))
			return
		}
	}
	for name := range r.MultipartForm.File {
		if name != "image" && name != "image[]" && name != "mask" {
			a.writeCompatError(w, errs.BadRequest("unsupported edit file: "+name))
			return
		}
	}
	n, err := strconv.Atoi(defaultString(r.FormValue("n"), "1"))
	if err != nil {
		a.writeCompatError(w, errs.BadRequest("invalid n"))
		return
	}
	stream, err := strconv.ParseBool(defaultString(r.FormValue("stream"), "false"))
	if err != nil {
		a.writeCompatError(w, errs.BadRequest("invalid stream"))
		return
	}
	compression, err := strconv.Atoi(defaultString(r.FormValue("output_compression"), "0"))
	if err != nil {
		a.writeCompatError(w, errs.BadRequest("invalid output_compression"))
		return
	}
	input := compatInput{Model: r.FormValue("model"), Prompt: r.FormValue("prompt"), Size: r.FormValue("size"), N: n, Quality: r.FormValue("quality"), ResponseFormat: r.FormValue("response_format"), User: r.FormValue("user"), Stream: stream, OutputFormat: r.FormValue("output_format"), Background: r.FormValue("background"), OutputCompression: compression, Moderation: r.FormValue("moderation")}
	if _, present := r.MultipartForm.Value["partial_images"]; present {
		zero := 0
		input.PartialImages = &zero
	}
	files := append(r.MultipartForm.File["image"], r.MultipartForm.File["image[]"]...)
	images, err := readCompatImages(files)
	if err != nil || len(images) == 0 {
		a.writeCompatError(w, errs.BadRequest("image is required and must be readable"))
		return
	}
	var mask *provider.ImageInput
	if files := r.MultipartForm.File["mask"]; len(files) > 0 {
		if len(files) != 1 {
			a.writeCompatError(w, errs.BadRequest("one mask is allowed"))
			return
		}
		value, err := readCompatImage(files[0])
		if err != nil {
			a.writeCompatError(w, errs.BadRequest("invalid mask"))
			return
		}
		mask = &value
	}
	a.startCompat(w, r, p, input, images, mask)
}

func compatMutationID(p openplatform.Principal, key string) string {
	if strings.TrimSpace(key) == "" {
		return uuid.NewString()
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("compat-image:%d:%s:%s", p.UserID, p.APIKeyID, key)))
	return "compat-" + hex.EncodeToString(digest[:])
}

func (a *API) startCompat(w http.ResponseWriter, r *http.Request, p openplatform.Principal, input compatInput, images []provider.ImageInput, mask *provider.ImageInput) {
	if input.PartialImages != nil {
		a.writeCompatError(w, errs.BadRequest("partial_images is unsupported; stream=true emits platform task lifecycle and final result events, not partial images"))
		return
	}
	if input.ResponseFormat != "" && input.ResponseFormat != "url" && input.ResponseFormat != "b64_json" {
		a.writeCompatError(w, errs.BadRequest("unsupported response_format"))
		return
	}
	if input.N == 0 {
		input.N = 1
	}
	if input.N < 1 {
		a.writeCompatError(w, errs.BadRequest("n must be positive"))
		return
	}
	if strings.TrimSpace(input.Model) == "" || strings.TrimSpace(input.Prompt) == "" {
		a.writeCompatError(w, errs.BadRequest("model and prompt are required"))
		return
	}
	if input.Stream {
		if _, ok := w.(http.Flusher); !ok {
			a.writeCompatError(w, errs.Internal("streaming is unavailable"))
			return
		}
	}
	taskID := compatMutationID(p, r.Header.Get("Idempotency-Key"))
	sizeMode, base, size := compatGenerationSizeFields(input.Size)
	// Direct route model codes are accepted. Resolve configured aliases without
	// deriving the discoverable catalogue from those static aliases.
	selection, err := a.compat.ResolveModel(r.Context(), input.Model)
	if err != nil {
		a.writeCompatError(w, compatservice.MapError(err))
		return
	}
	route := selection.RouteModelCode
	if route == "" && selection.AbstractModel == "" {
		route = input.Model
	}
	native := map[string]any{"idempotency_key": taskID, "task_type": string(provider.TaskTypeTextToImage), "prompt": input.Prompt, "abstract_model": selection.AbstractModel, "route_model_code": route, "size_mode": sizeMode, "base_resolution": base, "requested_size": size, "quality": compatGenerationQuality(input.Quality), "output_format": input.OutputFormat, "background": input.Background, "output_compression": input.OutputCompression, "moderation": input.Moderation, "output_image_count": input.N}
	if len(images) > 0 {
		native["task_type"] = string(provider.TaskTypeImageEdit)
		ids := make([]string, 0, len(images))
		for _, image := range images {
			asset, err := a.assets.UploadWithMetadataContext(r.Context(), p.UserID, image.Filename, image.MIMEType, image.Data, domainassets.UploadMetadata{})
			if err != nil {
				a.writeCompatError(w, compatservice.MapError(err))
				return
			}
			ids = append(ids, asset.ID)
		}
		native["reference_asset_ids"] = ids
	}
	if mask != nil {
		asset, err := a.assets.UploadWithMetadataContext(r.Context(), p.UserID, mask.Filename, mask.MIMEType, mask.Data, domainassets.UploadMetadata{})
		if err != nil {
			a.writeCompatError(w, compatservice.MapError(err))
			return
		}
		native["mask_asset_id"] = asset.ID
	}
	task, err := a.compat.Start(r.Context(), p, native)
	if err != nil {
		a.writeCompatError(w, compatservice.MapError(err))
		return
	}
	w.Header().Set("X-Task-ID", task.ID)
	w.Header().Set("Location", "/api/open/v1/image-tasks/"+task.ID)
	a.finishCompat(w, r, p, input, task)
}

func (a *API) finishCompat(w http.ResponseWriter, r *http.Request, p openplatform.Principal, input compatInput, task domainimagetask.Task) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	var observe func(domainimagetask.Task) error
	send := func(event string, data any) error {
		raw, err := json.Marshal(data)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw)
		if err == nil {
			w.(http.Flusher).Flush()
		}
		return err
	}
	if input.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.Header().Set("X-Stream-Protocol", "platform-image-lifecycle-v1")
		previous := ""
		observe = func(task domainimagetask.Task) error {
			state := task.Status + ":" + task.ProgressStage
			if state == previous {
				return nil
			}
			previous = state
			return send("task", map[string]any{"task_id": task.ID, "status": task.Status, "progress_stage": task.ProgressStage})
		}
	}
	task, err := a.compat.Wait(ctx, p, task, observe)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		if input.Stream {
			_ = send("error", map[string]any{"code": "TASK_WAIT_INTERRUPTED", "message": "recover the durable task using task_id", "task_id": task.ID})
			return
		}
		a.writeCompatError(w, errs.New(504, "TASK_WAIT_INTERRUPTED", "image task is durable; recover using X-Task-ID and Location"))
		return
	}
	response, err := a.compat.Result(ctx, p, task, input.ResponseFormat)
	if err != nil {
		if input.Stream {
			_ = send("error", map[string]any{"code": compatservice.MapError(err).Code, "message": compatservice.MapError(err).Message, "task_id": task.ID})
			return
		}
		a.writeCompatError(w, compatservice.MapError(err))
		return
	}
	if input.Stream {
		_ = send("result", response)
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		w.(http.Flusher).Flush()
		return
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

func (a *API) HandleOpenAIModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeCompatError(w, methodNotAllowedError())
		return
	}
	p, appErr := a.GetOpenPrincipal(r, "models:read")
	if appErr != nil {
		a.writeCompatError(w, appErr)
		return
	}
	models, err := a.compat.ModelsForPrincipal(r.Context(), p)
	if err != nil {
		a.writeCompatError(w, compatservice.MapError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, models)
}
