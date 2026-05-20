package openrouter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/provider"
)

var dataURLPattern = regexp.MustCompile(`^data:(?P<mime>[-\w.]+/[-+\w.]+);base64,(?P<data>.+)$`)

type Config struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(cfg Config) *Client {
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Client{baseURL: strings.TrimRight(cfg.BaseURL, "/"), apiKey: cfg.APIKey, httpClient: client}
}

func (c *Client) Generate(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
	payload := c.buildPayload(req)
	return c.doJSON(ctx, payload)
}

func (c *Client) Edit(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
	payload := c.buildPayload(req)
	return c.doJSON(ctx, payload)
}

func (c *Client) buildPayload(req provider.ImageRequest) map[string]any {
	content := any(req.Prompt)
	if len(req.ReferenceImages) > 0 {
		parts := make([]any, 0, 1+len(req.ReferenceImages))
		parts = append(parts, map[string]any{"type": "text", "text": req.Prompt})
		for _, image := range req.ReferenceImages {
			parts = append(parts, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": toDataURL(image)},
			})
		}
		content = parts
	}

	payload := map[string]any{
		"model":      req.Model,
		"messages":   []map[string]any{{"role": "user", "content": content}},
		"modalities": []string{"image", "text"},
	}
	if req.Size != "" {
		payload["size"] = req.Size
	}
	if req.OutputImageCount > 0 {
		payload["n"] = req.OutputImageCount
	}
	if req.Quality != "" {
		payload["quality"] = req.Quality
	}
	return payload
}

func (c *Client) doJSON(ctx context.Context, payload any) (provider.ImageResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return provider.ImageResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return provider.ImageResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return provider.ImageResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return provider.ImageResponse{}, decodeError(resp)
	}

	var payloadResp struct {
		Choices []struct {
			Message struct {
				Images []struct {
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"images"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payloadResp); err != nil {
		return provider.ImageResponse{}, err
	}

	result := provider.ImageResponse{Created: time.Now().Unix(), ProviderRequestID: resp.Header.Get("x-request-id")}
	for _, choice := range payloadResp.Choices {
		for _, image := range choice.Message.Images {
			item := provider.ImageResult{}
			format, b64, ok := parseDataURL(image.ImageURL.URL)
			if ok {
				item.B64JSON = b64
				item.Format = format
			} else {
				item.URL = image.ImageURL.URL
			}
			result.Data = append(result.Data, item)
		}
	}
	return result, nil
}

func (c *Client) endpoint(requestPath string) string {
	if c.baseURL == "" {
		return requestPath
	}
	return c.baseURL + path.Clean("/"+requestPath)
}

func decodeError(resp *http.Response) error {
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	upstream := &provider.UpstreamError{
		Provider:   provider.ProviderTypeOpenRouter,
		HTTPStatus: resp.StatusCode,
		Code:       payload.Error.Code,
		Type:       payload.Error.Type,
		Message:    payload.Error.Message,
		RequestID:  resp.Header.Get("x-request-id"),
	}
	provider.ClassifyUpstreamError(upstream)
	return upstream
}

func toDataURL(input provider.ImageInput) string {
	mimeType := input.MIMEType
	if mimeType == "" {
		mimeType = "image/png"
	}
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(input.Data))
}

func parseDataURL(value string) (string, string, bool) {
	matches := dataURLPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(matches) != 3 {
		return "", "", false
	}
	mimeType := strings.ToLower(matches[1])
	format := ""
	if parts := strings.SplitN(mimeType, "/", 2); len(parts) == 2 {
		format = parts[1]
	}
	return format, matches[2], true
}
