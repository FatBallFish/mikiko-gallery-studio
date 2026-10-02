package minimax

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	videoprovider "github.com/fatballfish/pic-gallery/internal/provider/video"
)

const (
	providerCode             = "minimax"
	defaultCallbackTolerance = 5 * time.Minute
	maxChallengeRunes        = 256
)

type Config struct {
	BaseURL           string
	APIKey            string
	ModelCode         string
	HTTPClient        *http.Client
	Timeout           time.Duration
	Verified          bool
	CallbackURL       string
	CallbackSecret    string
	Now               func() time.Time
	CallbackTolerance time.Duration
}

type Client struct {
	baseURL, apiKey, modelCode, callbackURL, callbackSecret string
	httpClient                                              *http.Client
	timeout                                                 time.Duration
	now                                                     func() time.Time
	callbackTolerance                                       time.Duration
}

func NewClient(cfg Config) (*Client, error) {
	if !cfg.Verified {
		return nil, fmt.Errorf("minimax video configuration must pass a real-account verification before use")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.ModelCode) == "" {
		return nil, fmt.Errorf("minimax base URL, API key and model code are required")
	}
	// The v2 paths below already carry the /v2 prefix; accounts may store either
	// https://api.minimax.cn or https://api.minimax.cn/v2.
	normalizedBase := strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"), "/v2")
	if _, err := url.ParseRequestURI(normalizedBase); err != nil {
		return nil, fmt.Errorf("parse minimax base URL: %w", err)
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	callbackTolerance := cfg.CallbackTolerance
	if callbackTolerance <= 0 {
		callbackTolerance = defaultCallbackTolerance
	}
	return &Client{
		baseURL: normalizedBase, apiKey: cfg.APIKey, modelCode: cfg.ModelCode,
		httpClient: client, timeout: timeout, callbackURL: cfg.CallbackURL, callbackSecret: cfg.CallbackSecret,
		now: now, callbackTolerance: callbackTolerance,
	}, nil
}

func (c *Client) Submit(ctx context.Context, req videoprovider.Request) (videoprovider.Job, error) {
	if err := validateRequest(req, map[string]bool{"aigc_watermark": true}); err != nil {
		return videoprovider.Job{}, err
	}
	content := []map[string]any{{"type": "text", "text": req.Prompt}}
	for _, input := range req.Inputs {
		mediaURL := input.URL
		if inline, err := c.inlinePrivateMedia(ctx, input); err != nil {
			return videoprovider.Job{}, err
		} else if inline != "" {
			mediaURL = inline
		}
		entry, err := mediaEntry(input, mediaURL)
		if err != nil {
			return videoprovider.Job{}, err
		}
		content = append(content, entry)
	}
	payload := map[string]any{"model": c.modelCode, "content": content, "duration": req.DurationSeconds, "resolution": mapResolution(req.Resolution)}
	// v2: text-to-video requires a concrete ratio (adaptive is rejected),
	// frame inputs always output adaptive, and multimodal reference tasks
	// accept a concrete ratio while adaptive stays the documented default.
	switch {
	case hasFrameInput(req):
		payload["ratio"] = "adaptive"
	case hasReferenceInput(req):
		if !strings.EqualFold(strings.TrimSpace(req.AspectRatio), "adaptive") {
			payload["ratio"] = req.AspectRatio
		}
	default:
		if strings.EqualFold(strings.TrimSpace(req.AspectRatio), "adaptive") {
			return videoprovider.Job{}, invalidRequest("aspect ratio must be a concrete value for text-to-video")
		}
		payload["ratio"] = req.AspectRatio
	}
	if c.callbackURL != "" {
		payload["callback_url"] = c.callbackURL
	}
	if value, ok := req.ProviderOptions["aigc_watermark"]; ok {
		payload["aigc_watermark"] = value
	}
	var response struct {
		TaskID string `json:"task_id"`
	}
	requestID, err := c.doJSON(ctx, http.MethodPost, "/v2/video_generation", payload, &response, true, req.IdempotencyKey)
	if err != nil {
		return videoprovider.Job{}, err
	}
	if strings.TrimSpace(response.TaskID) == "" {
		return videoprovider.Job{}, invalidResponse("missing task_id", nil)
	}
	return videoprovider.Job{ID: response.TaskID, State: videoprovider.StateQueued, RequestID: requestID}, nil
}

// mediaEntry maps one input to the v2 content entry declared by the official
// API: image_url / video_url / audio_url, with roles first_frame, last_frame,
// reference_image, reference_video and reference_audio. Frame images and
// reference inputs are mutually exclusive upstream; role selection is
// enforced by the platform capability check before submission.
func mediaEntry(input videoprovider.Input, mediaURL string) (map[string]any, error) {
	role := strings.TrimSpace(input.Role)
	switch mediaKind(input) {
	case "video":
		if role == "" {
			role = "reference_video"
		}
		return map[string]any{"type": "video_url", "video_url": map[string]any{"url": mediaURL}, "role": role}, nil
	case "audio":
		if role == "" {
			role = "reference_audio"
		}
		return map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": mediaURL}, "role": role}, nil
	case "image":
		entry := map[string]any{"type": "image_url", "image_url": map[string]any{"url": mediaURL}}
		if role != "" {
			entry["role"] = role
		}
		return entry, nil
	default:
		return nil, invalidRequest("unsupported input media type " + mediaKind(input))
	}
}

// mediaKind prefers the storage-declared media type and falls back to the
// MIME prefix so legacy tasks without MediaType keep their image behaviour.
func mediaKind(input videoprovider.Input) string {
	kind := strings.ToLower(strings.TrimSpace(input.MediaType))
	switch kind {
	case "image", "video", "audio":
		return kind
	}
	mimeType := strings.ToLower(strings.TrimSpace(input.MIMEType))
	switch {
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	case strings.HasPrefix(mimeType, "audio/"):
		return "audio"
	default:
		return "image"
	}
}

func (c *Client) Reconcile(ctx context.Context, req videoprovider.Request) (videoprovider.Job, bool, error) {
	job, err := c.Submit(ctx, req)
	if err != nil {
		return videoprovider.Job{}, false, err
	}
	return job, true, nil
}

func (c *Client) Get(ctx context.Context, ref videoprovider.JobRef) (videoprovider.Status, error) {
	if strings.TrimSpace(ref.ID) == "" {
		return videoprovider.Status{}, invalidRequest("job id is required")
	}
	var response struct {
		Task minimaxTask `json:"task"`
	}
	if _, err := c.doJSON(ctx, http.MethodGet, "/v2/query/video_generation/"+url.PathEscape(ref.ID), nil, &response, false); err != nil {
		return videoprovider.Status{}, err
	}
	return mapTask(response.Task)
}

func (c *Client) Cancel(ctx context.Context, ref videoprovider.JobRef) (videoprovider.CancelResult, error) {
	if strings.TrimSpace(ref.ID) == "" {
		return videoprovider.CancelResult{}, invalidRequest("job id is required")
	}
	var response struct {
		Status string `json:"status"`
		Action string `json:"action"`
	}
	if _, err := c.doJSON(ctx, http.MethodDelete, "/v2/video_generation/"+url.PathEscape(ref.ID), nil, &response, false); err != nil {
		return videoprovider.CancelResult{}, err
	}
	state, err := mapState(response.Status)
	if err != nil {
		return videoprovider.CancelResult{}, err
	}
	return videoprovider.CancelResult{Accepted: response.Action == "cancelled" || state == videoprovider.StateCancelled, State: state}, nil
}

func (c *Client) VerifyCallback(_ context.Context, headers http.Header, body []byte) (videoprovider.CallbackEvent, error) {
	var challenge struct {
		Challenge *string `json:"challenge"`
	}
	if err := json.Unmarshal(body, &challenge); err != nil {
		return videoprovider.CallbackEvent{}, invalidResponse("decode callback", err)
	}
	if challenge.Challenge != nil {
		if strings.TrimSpace(*challenge.Challenge) == "" || utf8.RuneCountInString(*challenge.Challenge) > maxChallengeRunes {
			return videoprovider.CallbackEvent{}, invalidRequest("invalid callback challenge")
		}
		return videoprovider.CallbackEvent{Challenge: *challenge.Challenge}, nil
	}
	if c.callbackSecret == "" {
		return videoprovider.CallbackEvent{}, invalidRequest("callback verification is not configured")
	}
	timestamp, err := validCallbackTimestamp(headers.Get("X-Minimax-Timestamp"), c.now(), c.callbackTolerance)
	if err != nil {
		return videoprovider.CallbackEvent{}, err
	}
	if !validSignature(c.callbackSecret, headers.Get("X-Minimax-Signature"), timestamp, body) {
		return videoprovider.CallbackEvent{}, invalidRequest("invalid callback signature")
	}
	var payload struct {
		EventID string      `json:"event_id"`
		Task    minimaxTask `json:"task"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return videoprovider.CallbackEvent{}, invalidResponse("decode callback", err)
	}
	status, err := mapTask(payload.Task)
	if err != nil {
		return videoprovider.CallbackEvent{}, err
	}
	return videoprovider.CallbackEvent{EventID: payload.EventID, JobID: status.JobID, Status: status}, nil
}

func (c *Client) NormalizeUsage(status videoprovider.Status) (videoprovider.Usage, error) {
	return videoprovider.Usage{OutputSeconds: decimal3(status.Usage["output_seconds"]), InputVideoSeconds: decimal3(status.Usage["input_seconds"]), ReferenceImageCount: intValue(status.Usage["input_image_count"]), ProviderTokens: integerString(status.Usage["total_tokens"]), Raw: clone(status.Usage)}, nil
}

type minimaxTask struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Duration int    `json:"duration"`
	Content  struct {
		URL string `json:"url"`
	} `json:"content"`
	Usage map[string]any `json:"usage"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func mapTask(task minimaxTask) (videoprovider.Status, error) {
	state, err := mapState(task.Status)
	if err != nil {
		return videoprovider.Status{}, err
	}
	status := videoprovider.Status{JobID: task.ID, State: state, Usage: clone(task.Usage), ErrorCode: task.Error.Code, ErrorMessage: task.Error.Message}
	if status.Usage == nil {
		status.Usage = map[string]any{}
	}
	if _, ok := status.Usage["output_seconds"]; !ok && task.Duration > 0 {
		status.Usage["output_seconds"] = task.Duration
	}
	if task.Content.URL != "" {
		status.Artifacts = []videoprovider.Artifact{{URL: task.Content.URL, MIMEType: "video/mp4"}}
	}
	return status, nil
}

func mapState(value string) (videoprovider.State, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "queued":
		return videoprovider.StateQueued, nil
	case "running":
		return videoprovider.StateRunning, nil
	case "succeeded":
		return videoprovider.StateSucceeded, nil
	case "failed":
		return videoprovider.StateFailed, nil
	case "cancelled", "deleted":
		return videoprovider.StateCancelled, nil
	default:
		return "", invalidResponse("unknown provider status "+value, nil)
	}
}

func mapResolution(value string) string {
	if strings.EqualFold(value, "2k") {
		return "2K"
	}
	return strings.ToUpper(value)
}

func hasFrameInput(req videoprovider.Request) bool {
	for _, input := range req.Inputs {
		role := strings.ToLower(strings.TrimSpace(input.Role))
		if role == "first_frame" || role == "last_frame" {
			return true
		}
	}
	return false
}

func hasReferenceInput(req videoprovider.Request) bool {
	for _, input := range req.Inputs {
		role := strings.ToLower(strings.TrimSpace(input.Role))
		if role == "reference_image" || role == "reference_video" || role == "reference_audio" {
			return true
		}
	}
	return false
}

const (
	// Inline caps mirror the official r2va limits while keeping the base64
	// request body under MiniMax's 64MB ceiling: reference videos are capped
	// at 50MB upstream but base64 inflates by ~4/3, so 40MB keeps the total
	// body feasible; audio caps at the 15MB upstream limit.
	maxInlineImageBytes = 10 << 20
	maxInlineVideoBytes = 40 << 20
	maxInlineAudioBytes = 15 << 20
)

func inlineLimitFor(kind string) int64 {
	switch kind {
	case "video":
		return maxInlineVideoBytes
	case "audio":
		return maxInlineAudioBytes
	default:
		return maxInlineImageBytes
	}
}

// inlinePrivateMedia downloads loopback/private-network input URLs and returns
// them as base64 data URLs. MiniMax fetches content itself, so a presigned
// URL that only resolves inside the deployment network (e.g. MinIO on 127.0.0.1
// or a docker bridge) is unreachable upstream.
func (c *Client) inlinePrivateMedia(ctx context.Context, input videoprovider.Input) (string, error) {
	if input.URL == "" || publiclyRoutableImageURL(input.URL) {
		return "", nil
	}
	kind := mediaKind(input)
	limit := inlineLimitFor(kind)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
	if err != nil {
		return "", invalidRequest("parse private media url: " + err.Error())
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", invalidRequest("download private media: " + err.Error())
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", invalidRequest(fmt.Sprintf("download private media: status %d", resp.StatusCode))
	}
	limited := io.LimitReader(resp.Body, limit+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return "", invalidRequest("read private media: " + err.Error())
	}
	if int64(len(raw)) > limit {
		return "", invalidRequest(fmt.Sprintf("private %s exceeds %dMB inline limit; store it on publicly reachable storage", kind, limit>>20))
	}
	mimeType := strings.TrimSpace(input.MIMEType)
	if mimeType == "" {
		mimeType = http.DetectContentType(raw)
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(raw), nil
}

func publiclyRoutableImageURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if host == "" {
		return false
	}
	ips := []net.IP{}
	if ip := net.ParseIP(host); ip != nil {
		ips = append(ips, ip)
	} else if resolved, err := net.LookupIP(host); err == nil {
		ips = resolved
	} else {
		// Unresolvable for us means unreachable for MiniMax too.
		return false
	}
	for _, ip := range ips {
		if ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
			return true
		}
	}
	return false
}

func (c *Client) doJSON(parent context.Context, method, path string, payload any, target any, submit bool, idempotencyKey ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, c.timeout)
	defer cancel()
	var body *bytes.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return "", err
		}
		body = bytes.NewReader(raw)
	} else {
		body = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if len(idempotencyKey) > 0 && strings.TrimSpace(idempotencyKey[0]) != "" {
		req.Header.Set("Idempotency-Key", strings.TrimSpace(idempotencyKey[0]))
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", transportError(err, submit)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", decodeHTTPError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return "", invalidResponse("decode response", err)
	}
	return resp.Header.Get("x-request-id"), nil
}

func validateRequest(req videoprovider.Request, allowed map[string]bool) error {
	if strings.TrimSpace(req.IdempotencyKey) == "" || strings.TrimSpace(req.Prompt) == "" || req.DurationSeconds <= 0 || req.Resolution == "" || req.AspectRatio == "" {
		return invalidRequest("idempotency key, prompt, duration, resolution and aspect ratio are required")
	}
	for key := range req.ProviderOptions {
		if !allowed[key] {
			return invalidRequest("unknown provider option " + key)
		}
	}
	return nil
}
func validCallbackTimestamp(raw string, now time.Time, tolerance time.Duration) (string, error) {
	timestamp := strings.TrimSpace(raw)
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return "", invalidRequest("invalid callback timestamp")
	}
	delta := now.Sub(time.Unix(seconds, 0))
	if delta < -tolerance || delta > tolerance {
		return "", invalidRequest("callback timestamp is outside the allowed window")
	}
	return timestamp, nil
}
func validSignature(secret, signature, timestamp string, body []byte) bool {
	provided, err := hex.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}
func decodeHTTPError(resp *http.Response) error {
	var p struct {
		Error struct {
			Type     string `json:"type"`
			Message  string `json:"message"`
			HTTPCode string `json:"http_code"`
		} `json:"error"`
		RequestID string `json:"request_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&p)
	return videoprovider.ClassifyHTTP(providerCode, resp.StatusCode, p.Error.Type, p.Error.Message, p.RequestID)
}
func transportError(err error, submit bool) error {
	return &videoprovider.Error{Provider: providerCode, Category: videoprovider.ErrorUnavailable, Code: "transport_error", Message: "provider request failed", Retryable: true, SubmissionUnknown: submit, Cause: err}
}
func invalidRequest(message string) error {
	return &videoprovider.Error{Provider: providerCode, Category: videoprovider.ErrorInvalidRequest, Code: "invalid_request", Message: message}
}
func invalidResponse(message string, cause error) error {
	return &videoprovider.Error{Provider: providerCode, Category: videoprovider.ErrorInvalidResponse, Code: "invalid_response", Message: message, Retryable: true, Cause: cause}
}
func clone(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for k, v := range value {
		out[k] = v
	}
	return out
}
func intValue(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := strconv.Atoi(v.String())
		return n
	}
	return 0
}
func integerString(value any) string {
	switch v := value.(type) {
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int:
		return strconv.Itoa(v)
	case json.Number:
		return v.String()
	case string:
		return v
	}
	return "0"
}
func decimal3(value any) string {
	switch v := value.(type) {
	case float64:
		return fmt.Sprintf("%.3f", v)
	case int:
		return fmt.Sprintf("%d.000", v)
	case json.Number:
		f, _ := v.Float64()
		return fmt.Sprintf("%.3f", f)
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return fmt.Sprintf("%.3f", f)
		}
	}
	return "0.000"
}
