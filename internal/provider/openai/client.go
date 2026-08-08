package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"mime/multipart"
	"net/http"
	"path"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/provider"
	_ "golang.org/x/image/webp"
)

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
	payload := map[string]any{
		"model":   req.Model,
		"prompt":  req.Prompt,
		"quality": req.Quality,
	}
	if req.Size != "" {
		payload["size"] = req.Size
	}
	if req.Background != "" {
		payload["background"] = req.Background
	}
	if req.ResponseFormat != "" && !isGPTImageModel(req.Model) {
		payload["response_format"] = string(req.ResponseFormat)
	}
	if req.OutputImageCount > 0 {
		payload["n"] = req.OutputImageCount
	}
	if req.OutputFormat != "" {
		payload["output_format"] = req.OutputFormat
	}
	if req.OutputCompression > 0 && (strings.EqualFold(req.OutputFormat, "jpeg") || strings.EqualFold(req.OutputFormat, "webp")) {
		payload["output_compression"] = req.OutputCompression
	}
	if req.Moderation != "" {
		payload["moderation"] = req.Moderation
	}
	if req.User != "" {
		payload["user"] = req.User
	}
	return c.doJSON(ctx, http.MethodPost, "/v1/images/generations", payload)
}

func isGPTImageModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "gpt-image-")
}

func (c *Client) Edit(ctx context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writeField := func(name, value string) error {
		if value == "" {
			return nil
		}
		return writer.WriteField(name, value)
	}

	if err := writeField("model", req.Model); err != nil {
		return provider.ImageResponse{}, err
	}
	if err := writeField("prompt", req.Prompt); err != nil {
		return provider.ImageResponse{}, err
	}
	if err := writeField("size", req.Size); err != nil {
		return provider.ImageResponse{}, err
	}
	if err := writeField("quality", req.Quality); err != nil {
		return provider.ImageResponse{}, err
	}
	if err := writeField("response_format", string(req.ResponseFormat)); err != nil {
		return provider.ImageResponse{}, err
	}
	if err := writeField("output_format", req.OutputFormat); err != nil {
		return provider.ImageResponse{}, err
	}
	if req.OutputCompression > 0 && (strings.EqualFold(req.OutputFormat, "jpeg") || strings.EqualFold(req.OutputFormat, "webp")) {
		if err := writeField("output_compression", fmt.Sprintf("%d", req.OutputCompression)); err != nil {
			return provider.ImageResponse{}, err
		}
	}
	if err := writeField("moderation", req.Moderation); err != nil {
		return provider.ImageResponse{}, err
	}
	if req.OutputImageCount > 0 {
		if err := writeField("n", fmt.Sprintf("%d", req.OutputImageCount)); err != nil {
			return provider.ImageResponse{}, err
		}
	}

	for _, image := range req.ReferenceImages {
		if err := writeFilePart(writer, "image", image); err != nil {
			return provider.ImageResponse{}, err
		}
	}
	if req.Mask != nil {
		if err := writeFilePart(writer, "mask", *req.Mask); err != nil {
			return provider.ImageResponse{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return provider.ImageResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/v1/images/edits"), body)
	if err != nil {
		return provider.ImageResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("Accept", "application/json")
	return c.do(httpReq)
}

func (c *Client) doJSON(ctx context.Context, method string, requestPath string, payload any) (provider.ImageResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return provider.ImageResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, c.endpoint(requestPath), bytes.NewReader(body))
	if err != nil {
		return provider.ImageResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	return c.do(httpReq)
}

func (c *Client) do(httpReq *http.Request) (provider.ImageResponse, error) {
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return provider.ImageResponse{}, provider.NewTransportError(provider.ProviderTypeOpenAI, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return provider.ImageResponse{}, decodeError(resp, provider.ProviderTypeOpenAI)
	}

	var result provider.ImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		if responseBodyReadTimedOut(httpReq, err) {
			return provider.ImageResponse{}, newResponseReadTimeoutError(err)
		}
		return provider.ImageResponse{}, provider.NewInvalidResponseError(provider.ProviderTypeOpenAI, err)
	}
	result.ProviderRequestID = resp.Header.Get("x-request-id")
	for i := range result.Data {
		if result.Data[i].Width > 0 && result.Data[i].Height > 0 || result.Data[i].B64JSON == "" {
			continue
		}
		decoded, decodeErr := base64.StdEncoding.DecodeString(result.Data[i].B64JSON)
		if decodeErr != nil {
			continue
		}
		config, _, decodeErr := image.DecodeConfig(bytes.NewReader(decoded))
		if decodeErr == nil {
			result.Data[i].Width, result.Data[i].Height = config.Width, config.Height
		}
	}
	return result, nil
}

func responseBodyReadTimedOut(req *http.Request, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if req != nil && req.Context() != nil && errors.Is(req.Context().Err(), context.DeadlineExceeded) {
		return true
	}
	return false
}

func newResponseReadTimeoutError(err error) *provider.UpstreamError {
	upstream := provider.NewTransportError(provider.ProviderTypeOpenAI, err)
	upstream.Code = "timeout"
	upstream.Type = "timeout"
	upstream.Message = err.Error()
	return upstream
}

func (c *Client) endpoint(requestPath string) string {
	if c.baseURL == "" {
		return requestPath
	}
	return c.baseURL + path.Clean("/"+requestPath)
}

func writeFilePart(writer *multipart.Writer, fieldName string, input provider.ImageInput) error {
	filename := input.Filename
	if filename == "" {
		filename = fieldName
	}
	headers := textprotoMIMEHeader(fieldName, filename, input.MIMEType)
	part, err := writer.CreatePart(headers)
	if err != nil {
		return err
	}
	_, err = part.Write(input.Data)
	return err
}

func textprotoMIMEHeader(fieldName string, filename string, mimeType string) map[string][]string {
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return map[string][]string{
		"Content-Disposition": {fmt.Sprintf(`form-data; name=%q; filename=%q`, fieldName, filename)},
		"Content-Type":        {mimeType},
	}
}

func decodeError(resp *http.Response, providerType provider.ProviderType) error {
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	upstream := &provider.UpstreamError{
		Provider:   providerType,
		HTTPStatus: resp.StatusCode,
		Code:       payload.Error.Code,
		Type:       payload.Error.Type,
		Message:    payload.Error.Message,
		RequestID:  resp.Header.Get("x-request-id"),
	}
	provider.ClassifyUpstreamError(upstream)
	return upstream
}

func normalizeCount(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}
