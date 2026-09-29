// Package gasic adapts the GASIC video relay (https://g-aisc.xyz/v1), an
// OpenAI-Sora-compatible gateway that fronts Seedance models. Generation is
// asynchronous: POST /v1/videos, poll GET /v1/videos/{id} (status uses
// "in_progress"), and fetch the result URL from GET /v1/tasks/{id}/artifacts
// because the poll response never carries one. Generated files are kept for
// one hour, so the runner must download and re-host promptly.
package gasic

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	videoprovider "github.com/fatballfish/pic-gallery/internal/provider/video"
)

const providerCode = "gasic"

const maxInlineImageBytes = 10 << 20 // parity with the other video adapters; inputs are capability-capped at 10MB.

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
	now                        func() time.Time
}

func NewClient(cfg Config) (*Client, error) {
	if !cfg.Verified {
		return nil, fmt.Errorf("gasic video configuration must pass a real-account verification before use")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.ModelCode) == "" {
		return nil, fmt.Errorf("gasic base URL, API key and model code are required")
	}
	// Paths below already carry the /v1 prefix; accounts may store either
	// https://g-aisc.xyz or https://g-aisc.xyz/v1.
	normalizedBase := strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"), "/v1")
	if _, err := url.ParseRequestURI(normalizedBase); err != nil {
		return nil, fmt.Errorf("parse gasic base URL: %w", err)
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
	return &Client{baseURL: normalizedBase, apiKey: cfg.APIKey, modelCode: cfg.ModelCode, httpClient: client, timeout: timeout, now: now}, nil
}

func (c *Client) Submit(ctx context.Context, req videoprovider.Request) (videoprovider.Job, error) {
	if err := validateRequest(req); err != nil {
		return videoprovider.Job{}, err
	}
	images := make([]string, 0, len(req.Inputs))
	for _, input := range req.Inputs {
		imageURL := input.URL
		if inline, err := c.inlinePrivateImage(ctx, input); err != nil {
			return videoprovider.Job{}, err
		} else if inline != "" {
			imageURL = inline
		}
		images = append(images, imageURL)
	}
	payload := map[string]any{
		// The gateway only honours "seconds"; "duration" is a harmless
		// fallback recommended by the relay documentation.
		"model": c.modelCode, "prompt": req.Prompt,
		"seconds": req.DurationSeconds, "duration": req.DurationSeconds,
		"metadata": map[string]any{"resolution": strings.ToLower(req.Resolution), "ratio": req.AspectRatio, "duration": req.DurationSeconds},
	}
	if len(images) > 0 {
		payload["images"] = images
		payload["image"] = images[0]
	}
	var response struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	requestID, err := c.doJSON(ctx, http.MethodPost, "/v1/videos", payload, &response, true, req.IdempotencyKey)
	if err != nil {
		return videoprovider.Job{}, err
	}
	if strings.TrimSpace(response.ID) == "" {
		return videoprovider.Job{}, invalidResponse("missing task id", nil)
	}
	return videoprovider.Job{ID: response.ID, State: videoprovider.StateQueued, RequestID: requestID}, nil
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
		ID          string         `json:"id"`
		Status      string         `json:"status"`
		Progress    int            `json:"progress"`
		Seconds     any            `json:"seconds"`
		Error       gasicError     `json:"error"`
		CreatedAt   int64          `json:"created_at"`
		CompletedAt int64          `json:"completed_at"`
		Usage       map[string]any `json:"usage"`
	}
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/videos/"+url.PathEscape(ref.ID), nil, &response, false); err != nil {
		return videoprovider.Status{}, err
	}
	state, err := mapState(response.Status)
	if err != nil {
		return videoprovider.Status{}, err
	}
	status := videoprovider.Status{JobID: response.ID, State: state, ErrorCode: response.Error.Code, ErrorMessage: response.Error.Message, Usage: clone(response.Usage)}
	if status.Usage == nil {
		status.Usage = map[string]any{}
	}
	if _, ok := status.Usage["output_seconds"]; !ok {
		if seconds := anySeconds(response.Seconds); seconds > 0 {
			status.Usage["output_seconds"] = seconds
		} else if response.CompletedAt > 0 && response.CreatedAt > 0 && response.CompletedAt > response.CreatedAt {
			// The poll payload has no duration field; wall-clock seconds are a
			// weak fallback that keeps settlement from showing zero.
			status.Usage["output_seconds"] = response.CompletedAt - response.CreatedAt
		}
	}
	if state == videoprovider.StateSucceeded {
		artifactURL, err := c.artifactURL(ctx, ref.ID)
		if err != nil {
			return videoprovider.Status{}, err
		}
		if artifactURL != "" {
			status.Artifacts = []videoprovider.Artifact{{URL: artifactURL, MIMEType: "video/mp4"}}
		}
	}
	return status, nil
}

// artifactURL fetches the shareable content URL; the poll response never
// carries one. An empty artifacts list (task not downloadable yet) yields no
// artifact so the runner retries instead of failing hard.
func (c *Client) artifactURL(ctx context.Context, jobID string) (string, error) {
	var response struct {
		Artifacts []struct {
			Key        string `json:"key"`
			Type       string `json:"type"`
			MIMEType   string `json:"mime_type"`
			ContentURL string `json:"content_url"`
		} `json:"artifacts"`
	}
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/tasks/"+url.PathEscape(jobID)+"/artifacts", nil, &response, false); err != nil {
		return "", err
	}
	for _, artifact := range response.Artifacts {
		if strings.TrimSpace(artifact.ContentURL) != "" {
			return artifact.ContentURL, nil
		}
	}
	return "", nil
}

// Cancel marks the attempt cancelled locally. The relay exposes no cancel
// endpoint, so the remote task keeps running until it finishes on its own;
// returning an error here would wedge the item in an endless cancel-retry
// loop because the runner reschedules on provider cancel failures.
func (c *Client) Cancel(_ context.Context, _ videoprovider.JobRef) (videoprovider.CancelResult, error) {
	return videoprovider.CancelResult{Accepted: true, State: videoprovider.StateCancelled}, nil
}

// VerifyCallback is unused: this provider runs in poll mode only.
func (c *Client) VerifyCallback(_ context.Context, _ http.Header, _ []byte) (videoprovider.CallbackEvent, error) {
	return videoprovider.CallbackEvent{}, invalidRequest("callbacks are not supported by the gasic provider")
}

func (c *Client) NormalizeUsage(status videoprovider.Status) (videoprovider.Usage, error) {
	return videoprovider.Usage{OutputSeconds: decimal3(status.Usage["output_seconds"]), Raw: clone(status.Usage)}, nil
}

type gasicError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func mapState(value string) (videoprovider.State, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "queued":
		return videoprovider.StateQueued, nil
	case "in_progress", "running", "processing":
		return videoprovider.StateRunning, nil
	case "completed", "succeeded":
		return videoprovider.StateSucceeded, nil
	case "failed":
		return videoprovider.StateFailed, nil
	case "cancelled", "canceled":
		return videoprovider.StateCancelled, nil
	default:
		return "", invalidResponse("unknown provider status "+value, nil)
	}
}

func validateRequest(req videoprovider.Request) error {
	if strings.TrimSpace(req.IdempotencyKey) == "" || strings.TrimSpace(req.Prompt) == "" || req.DurationSeconds <= 0 || req.Resolution == "" || req.AspectRatio == "" {
		return invalidRequest("idempotency key, prompt, duration, resolution and aspect ratio are required")
	}
	if len(req.ProviderOptions) > 0 {
		for key := range req.ProviderOptions {
			return invalidRequest("unknown provider option " + key)
		}
	}
	return nil
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
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	message := payload.Error.Message
	if message == "" {
		message = payload.Message
	}
	return videoprovider.ClassifyHTTP(providerCode, resp.StatusCode, payload.Error.Code, message, "")
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

// inlinePrivateImage downloads loopback/private-network image URLs and
// returns them as base64 data URLs; the relay fetches image URLs itself, so
// presigned URLs that only resolve inside the deployment network are
// unreachable upstream. The relay accepts data URIs for reference images.
func (c *Client) inlinePrivateImage(ctx context.Context, input videoprovider.Input) (string, error) {
	if input.URL == "" || publiclyRoutableImageURL(input.URL) {
		return "", nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
	if err != nil {
		return "", invalidRequest("parse private image url: " + err.Error())
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", invalidRequest("download private image: " + err.Error())
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", invalidRequest(fmt.Sprintf("download private image: status %d", resp.StatusCode))
	}
	limited := io.LimitReader(resp.Body, maxInlineImageBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return "", invalidRequest("read private image: " + err.Error())
	}
	if len(raw) > maxInlineImageBytes {
		return "", invalidRequest("private image exceeds 10MB inline limit")
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
		// Unresolvable for us means unreachable for the relay too.
		return false
	}
	for _, ip := range ips {
		if ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
			return true
		}
	}
	return false
}
