// Package bailian adapts Alibaba Bailian (DashScope) Wan3.0 video generation.
// Wan3.0 is an all-in-one model: the task type is inferred from input.media[]
// types (first_frame/last_frame/reference_image/reference_video) plus prompt
// intent, covering text-to-video, first/last-frame i2v, multimodal
// reference-to-video, video editing and video extension.
//
// Protocol (DashScope async tasks):
//   - POST {base}/api/v1/services/aigc/video-generation/video-synthesis
//     with header X-DashScope-Async: enable → {output:{task_id}}
//   - GET  {base}/api/v1/tasks/{task_id} → {output:{task_status, video_url}}
//     with task_status PENDING/RUNNING/SUCCEEDED/FAILED/CANCELED/UNKNOWN
//   - POST {base}/api/v1/tasks/{task_id}/cancel
//
// Private deployment assets (presigned MinIO URLs) are unreachable from
// Aliyun: when an input URL is not publicly routable the client uploads the
// file through the DashScope upload policy flow and passes the file-mgr https URL.
package bailian

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	videoprovider "github.com/fatballfish/pic-gallery/internal/provider/video"
)

const providerCode = "bailian"

const (
	maxUploadImageBytes = 20 << 20 // Wan3.0 reference image limit
	maxUploadVideoBytes = 100 << 20
)

type Config struct {
	BaseURL    string
	APIKey     string
	ModelCode  string
	HTTPClient *http.Client
	Timeout    time.Duration
	Verified   bool
	Now        func() time.Time
}

type Client struct {
	baseURL, apiKey, modelCode string
	httpClient                 *http.Client
	timeout                    time.Duration
}

func NewClient(cfg Config) (*Client, error) {
	if !cfg.Verified {
		return nil, fmt.Errorf("bailian video configuration must pass a real-account verification before use")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.ModelCode) == "" {
		return nil, fmt.Errorf("bailian base URL, API key and model code are required")
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	// Accept both the bare workspace host and a host that already carries /api/v1.
	base = strings.TrimSuffix(base, "/api/v1")
	if _, err := url.ParseRequestURI(base); err != nil {
		return nil, fmt.Errorf("parse bailian base URL: %w", err)
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &Client{baseURL: base, apiKey: cfg.APIKey, modelCode: cfg.ModelCode, httpClient: client, timeout: timeout}, nil
}

type wanMedia struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

func (c *Client) Submit(ctx context.Context, req videoprovider.Request) (videoprovider.Job, error) {
	if err := validateRequest(req); err != nil {
		return videoprovider.Job{}, err
	}
	media := make([]wanMedia, 0, len(req.Inputs))
	for _, input := range req.Inputs {
		mediaType, ok := wanMediaType(input.Role)
		if !ok {
			return videoprovider.Job{}, invalidRequest("unsupported input role " + input.Role)
		}
		if !publiclyRoutableAssetURL(input.URL) {
			// Wan3.0 downloads input media itself and only accepts
			// http/https URLs that are reachable from Aliyun (base64 and
			// oss:// are rejected by the generation workers), so private
			// deployment assets cannot be passed through — fail fast with
			// an actionable error instead of an upstream download failure.
			return videoprovider.Job{}, invalidRequest("bailian input assets must be stored on publicly reachable storage; asset " + input.AssetID + " resolves to a private address")
		}
		media = append(media, wanMedia{Type: mediaType, URL: input.URL})
	}
	input := map[string]any{"prompt": req.Prompt}
	if len(media) > 0 {
		input["media"] = media
	}
	payload := map[string]any{
		"model": c.modelCode,
		"input": input,
		"parameters": map[string]any{
			"resolution":    mapResolution(req.Resolution),
			"ratio":         strings.ToLower(strings.TrimSpace(req.AspectRatio)),
			"duration":      req.DurationSeconds,
			"prompt_extend": false,
		},
	}
	var response struct {
		Output struct {
			TaskID string `json:"task_id"`
		} `json:"output"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if _, err := c.doJSON(ctx, http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", payload, &response, true, req.IdempotencyKey, map[string]string{"X-DashScope-Async": "enable"}); err != nil {
		return videoprovider.Job{}, err
	}
	if strings.TrimSpace(response.Output.TaskID) == "" {
		return videoprovider.Job{}, invalidResponse("missing task_id", nil)
	}
	return videoprovider.Job{ID: response.Output.TaskID, State: videoprovider.StateQueued}, nil
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
		Output struct {
			TaskID     string `json:"task_id"`
			TaskStatus string `json:"task_status"`
			VideoURL   string `json:"video_url"`
			Message    string `json:"message"`
			Code       string `json:"code"`
		} `json:"output"`
		Usage map[string]any `json:"usage"`
	}
	if _, err := c.doJSON(ctx, http.MethodGet, "/api/v1/tasks/"+url.PathEscape(ref.ID), nil, &response, false, "", nil); err != nil {
		return videoprovider.Status{}, err
	}
	state, err := mapState(response.Output.TaskStatus)
	if err != nil {
		return videoprovider.Status{}, err
	}
	usage := clone(response.Usage)
	status := videoprovider.Status{JobID: response.Output.TaskID, State: state, Usage: usage, ErrorCode: response.Output.Code, ErrorMessage: firstNonEmpty(response.Output.Message, response.Output.Code)}
	if seconds := anySeconds(usage["video_duration"]); seconds > 0 {
		if status.Usage == nil {
			status.Usage = map[string]any{}
		}
		status.Usage["output_seconds"] = seconds
	}
	if state == videoprovider.StateSucceeded {
		if artifactURL := strings.TrimSpace(response.Output.VideoURL); artifactURL != "" {
			status.Artifacts = []videoprovider.Artifact{{URL: artifactURL, MIMEType: "video/mp4"}}
		}
	}
	return status, nil
}

// Cancel uses the DashScope task cancel API; a task already finished answers
// an error which surfaces as a retryable schedule like the other adapters.
func (c *Client) Cancel(ctx context.Context, ref videoprovider.JobRef) (videoprovider.CancelResult, error) {
	if strings.TrimSpace(ref.ID) == "" {
		return videoprovider.CancelResult{}, invalidRequest("job id is required")
	}
	var response struct {
		Output struct {
			TaskStatus string `json:"task_status"`
		} `json:"output"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_, err := c.doJSON(ctx, http.MethodPost, "/api/v1/tasks/"+url.PathEscape(ref.ID)+"/cancel", map[string]any{}, &response, false, "", nil)
	if err != nil {
		return videoprovider.CancelResult{}, err
	}
	state, err := mapState(response.Output.TaskStatus)
	if err != nil {
		return videoprovider.CancelResult{}, invalidResponse("unknown cancel status", nil)
	}
	return videoprovider.CancelResult{Accepted: state == videoprovider.StateCancelled, State: state}, nil
}

func (c *Client) VerifyCallback(_ context.Context, _ http.Header, _ []byte) (videoprovider.CallbackEvent, error) {
	return videoprovider.CallbackEvent{}, invalidRequest("callbacks are not supported by the bailian provider")
}

func (c *Client) NormalizeUsage(status videoprovider.Status) (videoprovider.Usage, error) {
	return videoprovider.Usage{OutputSeconds: decimal3(status.Usage["output_seconds"]), Raw: clone(status.Usage)}, nil
}

// resolveAssetURL passes public URLs through and uploads private deployment
// assets (presigned MinIO) to DashScope object storage, returning an https
// URL that Wan3.0 can read.
func (c *Client) resolveAssetURL(ctx context.Context, input videoprovider.Input) (string, error) {
	if input.URL == "" {
		return "", invalidRequest("input url is required")
	}
	if publiclyRoutableAssetURL(input.URL) {
		return input.URL, nil
	}
	maxBytes := int64(maxUploadImageBytes)
	if strings.EqualFold(strings.TrimSpace(input.Role), "reference_video") {
		maxBytes = int64(maxUploadVideoBytes)
	}
	data, mimeType, err := c.downloadAsset(ctx, input, maxBytes)
	if err != nil {
		return "", err
	}
	filename := orDefault(path.Base(strings.TrimSpace(input.ObjectKey)), "asset")
	if !strings.Contains(filename, ".") {
		if ext, ok := extensionByMIME(mimeType); ok {
			filename += ext
		}
	}
	return c.uploadToDashScope(ctx, filename, data, mimeType)
}

func extensionByMIME(mimeType string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return ".png", true
	case "image/jpeg", "image/jpg":
		return ".jpg", true
	case "image/webp":
		return ".webp", true
	case "video/mp4":
		return ".mp4", true
	case "video/quicktime":
		return ".mov", true
	default:
		return "", false
	}
}

func (c *Client) downloadAsset(ctx context.Context, input videoprovider.Input, maxBytes int64) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
	if err != nil {
		return nil, "", invalidRequest("parse private asset url: " + err.Error())
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", invalidRequest("download private asset: " + err.Error())
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", invalidRequest(fmt.Sprintf("download private asset: status %d", resp.StatusCode))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", invalidRequest("read private asset: " + err.Error())
	}
	if int64(len(raw)) > maxBytes {
		return nil, "", invalidRequest(fmt.Sprintf("private asset exceeds %d bytes upload limit", maxBytes))
	}
	mimeType := strings.TrimSpace(input.MIMEType)
	if mimeType == "" {
		mimeType = http.DetectContentType(raw)
	}
	return raw, mimeType, nil
}

// uploadToDashScope exchanges an in-memory asset for an oss:// URL via the
// DashScope upload policy flow.
func (c *Client) uploadToDashScope(ctx context.Context, filename string, data []byte, mimeType string) (string, error) {
	policyURL := fmt.Sprintf("/api/v1/uploads?action=getPolicy&model=%s", url.QueryEscape(c.modelCode))
	var policy struct {
		Data struct {
			Policy              string `json:"policy"`
			Signature           string `json:"signature"`
			UploadDir           string `json:"upload_dir"`
			UploadHost          string `json:"upload_host"`
			OSSAccessKeyID      string `json:"oss_access_key_id"`
			XOSSObjectACL       string `json:"x_oss_object_acl"`
			XOSSForbidOverwrite string `json:"x_oss_forbid_overwrite"`
		} `json:"data"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if _, err := c.doJSON(ctx, http.MethodGet, policyURL, nil, &policy, false, "", nil); err != nil {
		return "", err
	}
	if strings.TrimSpace(policy.Data.UploadHost) == "" {
		return "", invalidResponse("upload policy missing upload_host", nil)
	}
	objectKey := path.Join(policy.Data.UploadDir, filename)
	form := &bytes.Buffer{}
	writer := multipart.NewWriter(form)
	for _, field := range []struct{ key, value string }{
		{"OSSAccessKeyId", policy.Data.OSSAccessKeyID},
		{"policy", policy.Data.Policy},
		{"Signature", policy.Data.Signature},
		{"success_action_status", "200"},
	} {
		if field.value != "" {
			_ = writer.WriteField(field.key, field.value)
		}
	}
	if policy.Data.XOSSObjectACL != "" {
		_ = writer.WriteField("x-oss-object-acl", policy.Data.XOSSObjectACL)
	}
	if policy.Data.XOSSForbidOverwrite != "" {
		_ = writer.WriteField("x-oss-forbid-overwrite", policy.Data.XOSSForbidOverwrite)
	}
	_ = writer.WriteField("Key", objectKey)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", invalidRequest("build upload form: " + err.Error())
	}
	if _, err := part.Write(data); err != nil {
		return "", invalidRequest("write upload form: " + err.Error())
	}
	if err := writer.Close(); err != nil {
		return "", invalidRequest("close upload form: " + err.Error())
	}
	uploadReq, err := http.NewRequestWithContext(ctx, http.MethodPost, policy.Data.UploadHost, form)
	if err != nil {
		return "", invalidRequest("build upload request: " + err.Error())
	}
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := c.httpClient.Do(uploadReq)
	if err != nil {
		return "", transportError(err, false)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return "", invalidRequest(fmt.Sprintf("upload asset to dashscope: status %d", resp.StatusCode))
	}
	// The workspace maas gateway only accepts http/https media URLs (an
	// oss:// scheme is rejected with "media.url scheme must be http/https");
	// generation workers read the file-mgr bucket with service credentials
	// even though the uploaded objects are private.
	return strings.TrimRight(policy.Data.UploadHost, "/") + "/" + objectKey, nil
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" || value == "." || value == "/" {
		return fallback
	}
	return value
}

func wanMediaType(role string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "first_frame":
		return "first_frame", true
	case "last_frame":
		return "last_frame", true
	case "reference_image":
		return "reference_image", true
	case "reference_video":
		return "reference_video", true
	default:
		return "", false
	}
}

func mapResolution(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "480p":
		return "480P"
	case "1080p":
		return "1080P"
	default:
		return "720P"
	}
}

func mapState(value string) (videoprovider.State, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "PENDING":
		return videoprovider.StateQueued, nil
	case "RUNNING":
		return videoprovider.StateRunning, nil
	case "SUCCEEDED":
		return videoprovider.StateSucceeded, nil
	case "FAILED":
		return videoprovider.StateFailed, nil
	case "CANCELED", "CANCELLED":
		return videoprovider.StateCancelled, nil
	default:
		return videoprovider.StateFailed, invalidResponse("unknown provider status "+value, nil)
	}
}

func validateRequest(req videoprovider.Request) error {
	if strings.TrimSpace(req.IdempotencyKey) == "" || strings.TrimSpace(req.Prompt) == "" || req.DurationSeconds <= 0 || req.Resolution == "" || req.AspectRatio == "" {
		return invalidRequest("idempotency key, prompt, duration, resolution and aspect ratio are required")
	}
	for _, input := range req.Inputs {
		if _, ok := wanMediaType(input.Role); !ok {
			return invalidRequest("unsupported input role " + input.Role)
		}
	}
	if len(req.ProviderOptions) > 0 {
		for key := range req.ProviderOptions {
			return invalidRequest("unknown provider option " + key)
		}
	}
	return nil
}

func (c *Client) doJSON(parent context.Context, method, path string, payload any, target any, submit bool, idempotencyKey string, extraHeaders map[string]string) (string, error) {
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
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("X-DashScope-Idempotency-Key", idempotencyKey)
	}
	for key, value := range extraHeaders {
		req.Header.Set(key, value)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", transportError(err, submit)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return "", decodeHTTPError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return "", invalidResponse("decode response", err)
	}
	return resp.Header.Get("x-request-id"), nil
}

func decodeHTTPError(resp *http.Response) error {
	var payload struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	return videoprovider.ClassifyHTTP(providerCode, resp.StatusCode, payload.Code, payload.Message, payload.RequestID)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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

func anySeconds(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}

func decimal3(value any) string {
	switch v := value.(type) {
	case float64:
		return fmt.Sprintf("%.3f", v)
	case int:
		return fmt.Sprintf("%d.000", v)
	case int64:
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

func publiclyRoutableAssetURL(raw string) bool {
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
		return false
	}
	for _, ip := range ips {
		if ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
			return true
		}
	}
	return false
}
