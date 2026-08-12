package seedance

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	videoprovider "github.com/fatballfish/pic-gallery/internal/provider/video"
)

const (
	providerCode             = "seedance"
	defaultCallbackTolerance = 5 * time.Minute
)

type Config struct {
	BaseURL, APIKey, ModelCode, CallbackURL, CallbackSecret string
	HTTPClient                                              *http.Client
	Timeout                                                 time.Duration
	Verified                                                bool
	Now                                                     func() time.Time
	CallbackTolerance                                       time.Duration
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
		return nil, fmt.Errorf("seedance video configuration must pass a real-account verification before use")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.ModelCode) == "" {
		return nil, fmt.Errorf("seedance base URL, API key and model code are required")
	}
	if _, err := url.ParseRequestURI(cfg.BaseURL); err != nil {
		return nil, fmt.Errorf("parse seedance base URL: %w", err)
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
		baseURL: strings.TrimRight(cfg.BaseURL, "/"), apiKey: cfg.APIKey, modelCode: cfg.ModelCode,
		httpClient: client, timeout: timeout, callbackURL: cfg.CallbackURL, callbackSecret: cfg.CallbackSecret,
		now: now, callbackTolerance: callbackTolerance,
	}, nil
}

func (c *Client) Submit(ctx context.Context, req videoprovider.Request) (videoprovider.Job, error) {
	if err := validateRequest(req); err != nil {
		return videoprovider.Job{}, err
	}
	content := []map[string]any{{"type": "text", "text": req.Prompt}}
	for _, input := range req.Inputs {
		content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": input.URL}, "role": input.Role})
	}
	payload := map[string]any{"model": c.modelCode, "content": content, "duration": req.DurationSeconds, "resolution": strings.ToLower(req.Resolution), "ratio": req.AspectRatio, "generate_audio": req.GenerateAudio, "output_format": req.OutputFormat}
	if c.callbackURL != "" {
		payload["callback_url"] = c.callbackURL
	}
	if value, ok := req.ProviderOptions["watermark"]; ok {
		payload["watermark"] = value
	}
	var response seedanceTask
	requestID, err := c.doJSON(ctx, http.MethodPost, "/api/v3/contents/generations/tasks", payload, &response, true)
	if err != nil {
		return videoprovider.Job{}, err
	}
	if response.ID == "" {
		return videoprovider.Job{}, invalidResponse("missing task id", nil)
	}
	state := videoprovider.StateQueued
	if response.Status != "" {
		state, err = mapState(response.Status)
		if err != nil {
			return videoprovider.Job{}, err
		}
	}
	return videoprovider.Job{ID: response.ID, State: state, RequestID: requestID}, nil
}
func (c *Client) Get(ctx context.Context, ref videoprovider.JobRef) (videoprovider.Status, error) {
	if ref.ID == "" {
		return videoprovider.Status{}, invalidRequest("job id is required")
	}
	var response seedanceTask
	if _, err := c.doJSON(ctx, http.MethodGet, "/api/v3/contents/generations/tasks/"+url.PathEscape(ref.ID), nil, &response, false); err != nil {
		return videoprovider.Status{}, err
	}
	return mapTask(response)
}
func (c *Client) Cancel(ctx context.Context, ref videoprovider.JobRef) (videoprovider.CancelResult, error) {
	if ref.ID == "" {
		return videoprovider.CancelResult{}, invalidRequest("job id is required")
	}
	var response seedanceTask
	if _, err := c.doJSON(ctx, http.MethodDelete, "/api/v3/contents/generations/tasks/"+url.PathEscape(ref.ID), nil, &response, false); err != nil {
		return videoprovider.CancelResult{}, err
	}
	state, err := mapState(response.Status)
	if err != nil {
		return videoprovider.CancelResult{}, err
	}
	return videoprovider.CancelResult{Accepted: state == videoprovider.StateCancelled, State: state}, nil
}
func (c *Client) VerifyCallback(_ context.Context, headers http.Header, body []byte) (videoprovider.CallbackEvent, error) {
	if c.callbackSecret == "" {
		return videoprovider.CallbackEvent{}, invalidRequest("callback verification is not configured")
	}
	timestamp, err := validCallbackTimestamp(headers.Get("X-Ark-Timestamp"), c.now(), c.callbackTolerance)
	if err != nil {
		return videoprovider.CallbackEvent{}, err
	}
	if !validSignature(c.callbackSecret, headers.Get("X-Ark-Signature"), timestamp, body) {
		return videoprovider.CallbackEvent{}, invalidRequest("invalid callback signature")
	}
	var response seedanceTask
	if err := json.Unmarshal(body, &response); err != nil {
		return videoprovider.CallbackEvent{}, invalidResponse("decode callback", err)
	}
	status, err := mapTask(response)
	if err != nil {
		return videoprovider.CallbackEvent{}, err
	}
	return videoprovider.CallbackEvent{EventID: response.EventID, JobID: status.JobID, Status: status}, nil
}
func (c *Client) NormalizeUsage(status videoprovider.Status) (videoprovider.Usage, error) {
	return videoprovider.Usage{OutputSeconds: decimal3(status.Usage["output_seconds"]), InputVideoSeconds: decimal3(status.Usage["input_seconds"]), ReferenceImageCount: intValue(status.Usage["input_image_count"]), ProviderTokens: integerString(firstValue(status.Usage, "total_tokens", "completion_tokens")), Raw: clone(status.Usage)}, nil
}

type seedanceTask struct {
	ID       string `json:"id"`
	EventID  string `json:"event_id"`
	Status   string `json:"status"`
	Duration int    `json:"duration"`
	Content  struct {
		VideoURL string `json:"video_url"`
		URL      string `json:"url"`
	} `json:"content"`
	Usage map[string]any `json:"usage"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func mapTask(task seedanceTask) (videoprovider.Status, error) {
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
	artifactURL := task.Content.VideoURL
	if artifactURL == "" {
		artifactURL = task.Content.URL
	}
	if artifactURL != "" {
		status.Artifacts = []videoprovider.Artifact{{URL: artifactURL, MIMEType: "video/mp4"}}
	}
	return status, nil
}
func mapState(value string) (videoprovider.State, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "queued", "pending":
		return videoprovider.StateQueued, nil
	case "running", "processing":
		return videoprovider.StateRunning, nil
	case "succeeded", "success":
		return videoprovider.StateSucceeded, nil
	case "failed", "error":
		return videoprovider.StateFailed, nil
	case "cancelled", "canceled":
		return videoprovider.StateCancelled, nil
	default:
		return "", invalidResponse("unknown provider status "+value, nil)
	}
}
func validateRequest(req videoprovider.Request) error {
	if req.Prompt == "" || req.DurationSeconds <= 0 || req.Resolution == "" || req.AspectRatio == "" || req.OutputFormat == "" {
		return invalidRequest("prompt, duration, resolution, aspect ratio and output format are required")
	}
	for key := range req.ProviderOptions {
		if key != "watermark" {
			return invalidRequest("unknown provider option " + key)
		}
	}
	return nil
}
func (c *Client) doJSON(parent context.Context, method, path string, payload, target any, submit bool) (string, error) {
	ctx, cancel := context.WithTimeout(parent, c.timeout)
	defer cancel()
	var raw []byte
	var err error
	if payload != nil {
		raw, err = json.Marshal(payload)
		if err != nil {
			return "", err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
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
			Code    string `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&p)
	code := p.Error.Code
	if code == "" {
		code = p.Error.Type
	}
	return videoprovider.ClassifyHTTP(providerCode, resp.StatusCode, code, p.Error.Message, resp.Header.Get("x-request-id"))
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
func firstValue(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return nil
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
