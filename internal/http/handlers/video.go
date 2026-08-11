package handlers

import (
	"net/http"
	"strings"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	videotaskservice "github.com/fatballfish/pic-gallery/internal/service/videotask"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

func (a *API) HandleVideoCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if _, appErr := a.requireUser(r); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.videoRouting == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeInternal, "video capabilities are unavailable"))
		return
	}
	response, err := a.videoRouting.Capabilities(r.Context(), r.URL.Query().Get("route_model_code"))
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, response)
}

func (a *API) HandleVideoEstimates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.videoQuotes == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeInternal, "video estimates are unavailable"))
		return
	}
	var body struct {
		RouteModelCode string `json:"route_model_code"`
		TaskType       string `json:"task_type"`
		Prompt         string `json:"prompt"`
		Duration       int    `json:"duration_seconds"`
		Resolution     string `json:"resolution"`
		AspectRatio    string `json:"aspect_ratio"`
		AudioMode      string `json:"audio_mode"`
		OutputCount    int    `json:"output_count"`
		Inputs         []struct {
			AssetID   string `json:"asset_id"`
			Role      string `json:"role"`
			Ordinal   int    `json:"ordinal"`
			MediaType string `json:"media_type"`
			Format    string `json:"format"`
			SizeBytes int64  `json:"size_bytes"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"inputs"`
	}
	if err := decodeStrictJSON(r, &body); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	request := videotaskservice.EstimateRequest{
		RouteModelCode: strings.TrimSpace(body.RouteModelCode),
		Video: domainvideo.Request{
			TaskType: domainvideo.TaskType(body.TaskType), Prompt: body.Prompt, DurationSeconds: body.Duration,
			Resolution: domainvideo.Resolution(body.Resolution), AspectRatio: domainvideo.AspectRatio(body.AspectRatio),
			AudioMode: domainvideo.AudioMode(body.AudioMode), OutputCount: body.OutputCount,
		},
	}
	for _, input := range body.Inputs {
		request.Video.Inputs = append(request.Video.Inputs, domainvideo.Input{
			AssetID: input.AssetID, Role: domainvideo.InputRole(input.Role), Ordinal: input.Ordinal, MediaType: input.MediaType,
			Format: input.Format, SizeBytes: input.SizeBytes, Width: input.Width, Height: input.Height,
		})
	}
	estimate, err := a.videoQuotes.Estimate(r.Context(), user.ID, request)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, estimate)
}
