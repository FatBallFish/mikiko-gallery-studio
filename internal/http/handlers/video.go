package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	videotaskservice "github.com/fatballfish/pic-gallery/internal/service/videotask"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

const videoTaskPathPrefix = "/api/agent/video/v1/tasks/"

var videoCreateRequestLocks videoRequestLockSet

type videoRequestLockSet struct {
	mu      sync.Mutex
	entries map[string]*videoRequestLock
}

type videoRequestLock struct {
	mu   sync.Mutex
	refs int
}

func (locks *videoRequestLockSet) Lock(key string) func() {
	locks.mu.Lock()
	if locks.entries == nil {
		locks.entries = make(map[string]*videoRequestLock)
	}
	entry := locks.entries[key]
	if entry == nil {
		entry = &videoRequestLock{}
		locks.entries[key] = entry
	}
	entry.refs++
	locks.mu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		locks.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(locks.entries, key)
		}
		locks.mu.Unlock()
	}
}

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
	code := strings.TrimSpace(r.URL.Query().Get("route_model_code"))
	if code == "" {
		response, err := a.videoRouting.ListCapabilities(r.Context())
		if err != nil {
			httpx.WriteError(w, r, normalizeVideoTaskError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, response)
		return
	}
	response, err := a.videoRouting.Capabilities(r.Context(), code)
	if err != nil {
		httpx.WriteError(w, r, normalizeVideoTaskError(err))
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
	if appErr := a.requireFeature(r.Context(), "video_creation"); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.videoQuotes == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeInternal, "video estimates are unavailable"))
		return
	}
	if a.videoTasks != nil {
		var request videotaskservice.CreateRequest
		if err := decodeStrictJSON(r, &request); err != nil {
			httpx.WriteError(w, r, videoFieldInvalid("invalid json body"))
			return
		}
		request.UserID = user.ID
		estimate, err := a.videoTasks.Estimate(r.Context(), request)
		if err != nil {
			httpx.WriteError(w, r, normalizeVideoTaskError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, estimate)
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
		httpx.WriteError(w, r, videoFieldInvalid("invalid json body"))
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
		httpx.WriteError(w, r, normalizeVideoTaskError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, estimate)
}

func (a *API) HandleVideoTasks(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.videoTasks == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeInternal, "video tasks are unavailable"))
		return
	}
	switch r.Method {
	case http.MethodPost:
		if appErr := a.requireFeature(r.Context(), "video_creation"); appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		var request videotaskservice.CreateRequest
		if err := decodeStrictJSON(r, &request); err != nil {
			httpx.WriteError(w, r, videoFieldInvalid("invalid json body"))
			return
		}
		request.UserID = user.ID
		request.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if request.IdempotencyKey == "" {
			httpx.WriteError(w, r, videoFieldInvalid("Idempotency-Key is required"))
			return
		}
		if len(request.IdempotencyKey) > 128 {
			httpx.WriteError(w, r, videoFieldInvalid("Idempotency-Key is too long"))
			return
		}
		unlock := videoCreateRequestLocks.Lock(strconv.FormatInt(user.ID, 10) + "\x00" + request.IdempotencyKey)
		defer unlock()
		task, replayed, err := a.videoTasks.Create(r.Context(), request)
		if err != nil {
			httpx.WriteError(w, r, normalizeVideoTaskError(err))
			return
		}
		status := http.StatusAccepted
		if replayed {
			status = http.StatusOK
		}
		httpx.WriteSuccess(w, r, status, task)
	case http.MethodGet:
		status, appErr := videoTaskQueryStatus(r.URL.Query().Get("status"))
		if appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		limit, appErr := videoTaskQueryLimit(r.URL.Query().Get("limit"))
		if appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		request := videotaskservice.ListRequest{
			UserID: user.ID,
			Status: status,
			Cursor: strings.TrimSpace(r.URL.Query().Get("cursor")),
			Limit:  limit,
		}
		if value := strings.TrimSpace(r.URL.Query().Get("project_id")); value != "" {
			projectID, err := uuid.Parse(value)
			if err != nil {
				httpx.WriteError(w, r, videoFieldInvalid("invalid project_id"))
				return
			}
			request.ProjectID = &projectID
		}
		page, err := a.videoTasks.List(r.Context(), request)
		if err != nil {
			httpx.WriteError(w, r, normalizeVideoTaskError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, page)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleVideoTaskDetail(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUserWithQueryToken(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.videoTasks == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeInternal, "video tasks are unavailable"))
		return
	}
	remainder := strings.TrimPrefix(r.URL.Path, videoTaskPathPrefix)
	if strings.HasSuffix(remainder, ":cancel") {
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, r)
			return
		}
		taskID, ok := parseVideoTaskID(strings.TrimSuffix(remainder, ":cancel"))
		if !ok {
			httpx.WriteError(w, r, videoTaskNotFound())
			return
		}
		idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if idempotencyKey == "" {
			httpx.WriteError(w, r, videoFieldInvalid("Idempotency-Key is required"))
			return
		}
		if len(idempotencyKey) > 128 {
			httpx.WriteError(w, r, videoFieldInvalid("Idempotency-Key is too long"))
			return
		}
		task, err := a.videoTasks.Cancel(r.Context(), user.ID, taskID, idempotencyKey)
		if err != nil {
			httpx.WriteError(w, r, normalizeVideoTaskError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusAccepted, task)
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	taskID, ok := parseVideoTaskID(remainder)
	if !ok {
		httpx.WriteError(w, r, videoTaskNotFound())
		return
	}
	task, err := a.videoTasks.Get(r.Context(), user.ID, taskID)
	if err != nil {
		httpx.WriteError(w, r, normalizeVideoTaskError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, task)
}

func (a *API) HandleVideoTaskEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUserWithQueryToken(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.videoTasks == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeInternal, "video tasks are unavailable"))
		return
	}
	listRequest := videotaskservice.ListRequest{UserID: user.ID, Limit: 20}
	if value := strings.TrimSpace(r.URL.Query().Get("project_id")); value != "" {
		projectID, err := uuid.Parse(value)
		if err != nil {
			httpx.WriteError(w, r, videoFieldInvalid("invalid project_id"))
			return
		}
		listRequest.ProjectID = &projectID
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.WriteError(w, r, errs.Internal("streaming is not supported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	resumeAfter := strings.TrimSpace(r.URL.Query().Get("after"))
	if resumeAfter == "" {
		resumeAfter = strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	}
	seen := map[uuid.UUID]int64{}
	send := func() {
		page, err := a.videoTasks.List(r.Context(), listRequest)
		if err != nil {
			writeSSE(w, "error", normalizeVideoTaskError(err))
		} else {
			tasks := append([]videotaskservice.Task(nil), page.Items...)
			sort.SliceStable(tasks, func(i, j int) bool {
				if tasks[i].UpdatedAt.Equal(tasks[j].UpdatedAt) {
					return tasks[i].ID.String() < tasks[j].ID.String()
				}
				return tasks[i].UpdatedAt.Before(tasks[j].UpdatedAt)
			})
			start := 0
			if resumeAfter != "" {
				for index, task := range tasks {
					if videoTaskEventID(task) == resumeAfter {
						start = index + 1
						break
					}
				}
				resumeAfter = ""
			}
			for _, task := range tasks[:start] {
				seen[task.ID] = task.Version
			}
			for _, task := range tasks[start:] {
				if seen[task.ID] == task.Version {
					continue
				}
				seen[task.ID] = task.Version
				writeVideoTaskSSE(w, task)
			}
		}
		flusher.Flush()
	}
	send()
	if strings.EqualFold(r.URL.Query().Get("once"), "true") {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			send()
		}
	}
}

type videoTaskEventProjection struct {
	ID          uuid.UUID              `json:"id"`
	Version     int64                  `json:"version"`
	Status      domainvideo.TaskStatus `json:"status"`
	Stage       string                 `json:"stage"`
	UpdatedAt   time.Time              `json:"updated_at"`
	ResultReady bool                   `json:"result_ready"`
}

func writeVideoTaskSSE(w http.ResponseWriter, task videotaskservice.Task) {
	payload := videoTaskEventProjection{
		ID: task.ID, Version: task.Version, Status: task.Status, Stage: task.ProgressStage,
		UpdatedAt: task.UpdatedAt, ResultReady: videoTaskResultReady(task),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		writeSSE(w, "error", errs.Internal("failed to encode video task event"))
		return
	}
	_, _ = fmt.Fprintf(w, "id: %s\nevent: task\ndata: %s\n\n", videoTaskEventID(task), data)
}

func videoTaskEventID(task videotaskservice.Task) string {
	return task.ID.String() + ":" + strconv.FormatInt(task.Version, 10)
}

func videoTaskResultReady(task videotaskservice.Task) bool {
	for _, item := range task.Items {
		if item.ResultAssetID != nil {
			return true
		}
	}
	return false
}

func parseVideoTaskID(value string) (uuid.UUID, bool) {
	if value == "" || strings.Contains(value, "/") {
		return uuid.Nil, false
	}
	parsed, err := uuid.Parse(value)
	return parsed, err == nil
}

func videoTaskQueryLimit(value string) (int, *errs.Error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 20, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > 100 {
		return 0, videoFieldInvalid("limit must be an integer from 1 to 100")
	}
	return limit, nil
}

func videoTaskQueryStatus(value string) (string, *errs.Error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	switch domainvideo.TaskStatus(value) {
	case domainvideo.TaskStatusQueued,
		domainvideo.TaskStatusRunning,
		domainvideo.TaskStatusSaving,
		domainvideo.TaskStatusSucceeded,
		domainvideo.TaskStatusPartial,
		domainvideo.TaskStatusFailed,
		domainvideo.TaskStatusCancelled:
		return value, nil
	default:
		return "", videoFieldInvalid("invalid video task status")
	}
}

func normalizeVideoTaskError(err error) *errs.Error {
	if errors.Is(err, videotaskservice.ErrIdempotencyConflict) {
		return errs.New(http.StatusConflict, errs.CodeIdempotencyKeyReused, "idempotency key was used for another video task")
	}
	var appErr *errs.Error
	if errors.As(err, &appErr) && (appErr.Code == errs.CodeBadRequest || appErr.Code == errs.CodeValidationFailed) {
		message := strings.ToLower(appErr.Message)
		if strings.Contains(message, "input asset") || strings.Contains(message, "video input") {
			return errs.WithDetails(videoInputInvalid(appErr.Message), appErr.Details)
		}
		return errs.WithDetails(videoFieldInvalid(appErr.Message), appErr.Details)
	}
	return normalizeAppError(err)
}

func videoFieldInvalid(message string) *errs.Error {
	return errs.New(http.StatusBadRequest, errs.CodeVideoFieldInvalid, message)
}

func videoInputInvalid(message string) *errs.Error {
	return errs.New(http.StatusBadRequest, errs.CodeVideoInputInvalid, message)
}

func videoTaskNotFound() *errs.Error {
	return errs.New(http.StatusNotFound, errs.CodeNotFound, "video task not found")
}
