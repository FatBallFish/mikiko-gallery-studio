package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/provider"
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
		"model":           req.Model,
		"prompt":          req.Prompt,
		"size":            req.Size,
		"n":               normalizeCount(req.OutputImageCount),
		"quality":         req.Quality,
		"response_format": string(req.ResponseFormat),
	}
	if req.User != "" {
		payload["user"] = req.User
	}
	return c.doJSON(ctx, http.MethodPost, "/v1/images/generations", payload)
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
	if err := writeField("n", strconv.Itoa(normalizeCount(req.OutputImageCount))); err != nil {
		return provider.ImageResponse{}, err
	}
	if err := writeField("response_format", string(req.ResponseFormat)); err != nil {
		return provider.ImageResponse{}, err
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
		return provider.ImageResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return provider.ImageResponse{}, decodeError(resp, provider.ProviderTypeOpenAI)
	}

	var result provider.ImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return provider.ImageResponse{}, err
	}
	result.ProviderRequestID = resp.Header.Get("x-request-id")
	return result, nil
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
