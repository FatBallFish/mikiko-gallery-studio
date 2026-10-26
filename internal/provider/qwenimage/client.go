package qwenimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/internal/provider"
)

const Adapter = nativeimage.AdapterQwen
const DefaultBaseURL = "https://dashscope.aliyuncs.com"

type Config struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Profile    *nativeimage.CapabilityProfile
}
type Client struct{ config Config }

func New(c Config) *Client {
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 600 * time.Second}
	}
	return &Client{c}
}
func (c *Client) Generate(ctx context.Context, r provider.ImageRequest) (provider.ImageResponse, error) {
	if r.Operation == "" {
		r.Operation = nativeimage.OperationGenerate
	}
	if r.Operation != nativeimage.OperationGenerate {
		return provider.ImageResponse{}, fmt.Errorf("Generate requires generate operation")
	}
	return c.run(ctx, r)
}
func (c *Client) Edit(ctx context.Context, r provider.ImageRequest) (provider.ImageResponse, error) {
	if r.Operation == "" {
		r.Operation = nativeimage.OperationEdit
	}
	if r.Operation != nativeimage.OperationEdit {
		return provider.ImageResponse{}, fmt.Errorf("Qwen supports edit only")
	}
	return c.run(ctx, r)
}
func (c *Client) run(ctx context.Context, r provider.ImageRequest) (provider.ImageResponse, error) {
	p, ok := nativeimage.ProfileForModel(r.Model)
	if c.config.Profile != nil {
		if err := nativeimage.ValidateProfile(*c.config.Profile); err != nil {
			return provider.ImageResponse{}, err
		}
		if ok && c.config.Profile.Model != r.Model {
			return provider.ImageResponse{}, fmt.Errorf("profile model conflicts with request model")
		}
		p = *c.config.Profile
		ok = true
	}
	if !ok || p.Adapter != Adapter {
		return provider.ImageResponse{}, fmt.Errorf("unknown Qwen model profile")
	}
	if r.Mask != nil || r.Quality != "" || r.OutputCompression != 0 || r.Moderation != "" || r.User != "" || r.SendResponseFormat || (r.ResponseFormat != "" && r.ResponseFormat != provider.ResponseFormatURL) {
		return provider.ImageResponse{}, fmt.Errorf("unsupported Qwen request field")
	}
	if err := nativeimage.ValidateRequest(p, nativeimage.ValidationRequest{Operation: r.Operation, Prompt: r.Prompt, Size: r.Size, OutputFormat: r.OutputFormat, Background: r.Background, Count: r.OutputImageCount, ReferenceCount: len(r.ReferenceImages), Options: r.NativeOptions}); err != nil {
		return provider.ImageResponse{}, err
	}
	content := []map[string]string{}
	for _, im := range r.ReferenceImages {
		allowedFormat := false
		for _, format := range p.InputFormats {
			if im.MIMEType == "image/"+format {
				allowedFormat = true
			}
		}
		if !allowedFormat {
			return provider.ImageResponse{}, fmt.Errorf("reference format excluded by model profile")
		}
		if len(im.Data) == 0 || !validMIME(im.MIMEType) {
			return provider.ImageResponse{}, fmt.Errorf("unsupported or empty Qwen reference image")
		}
		if err := nativeimage.ValidateReference(ctx, Adapter, r.Operation, im.MIMEType, im.Data); err != nil {
			return provider.ImageResponse{}, err
		}
		content = append(content, map[string]string{"image": "data:" + im.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(im.Data)})
	}
	content = append(content, map[string]string{"text": r.Prompt})
	params := map[string]any{}
	if r.OutputImageCount > 0 {
		params["n"] = r.OutputImageCount
	}
	if r.Size != "" && r.Size != "auto" {
		params["size"] = strings.ReplaceAll(r.Size, "x", "*")
	}
	if o := r.NativeOptions; o != nil {
		if o.Seed != nil {
			params["seed"] = *o.Seed
		}
		if o.NegativePrompt != "" {
			params["negative_prompt"] = o.NegativePrompt
		}
		if o.Watermark != nil {
			params["watermark"] = *o.Watermark
		}
		if q := o.Qwen; q != nil {
			if q.PromptExtend != nil {
				params["prompt_extend"] = *q.PromptExtend
			}
			if q.EnableThinking != nil {
				params["enable_thinking"] = *q.EnableThinking
			}
			if q.PromptExtendMode != "" {
				params["prompt_extend_mode"] = q.PromptExtendMode
			}
		}
	}
	body, _ := json.Marshal(map[string]any{"model": r.Model, "input": map[string]any{"messages": []any{map[string]any{"role": "user", "content": content}}}, "parameters": params})
	endpoint := strings.TrimRight(c.config.BaseURL, "/")
	if !strings.HasSuffix(endpoint, "/multimodal-generation/generation") {
		if strings.HasSuffix(endpoint, "/api/v1") {
			endpoint += "/services/aigc/multimodal-generation/generation"
		} else {
			endpoint += "/api/v1/services/aigc/multimodal-generation/generation"
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return provider.ImageResponse{}, fmt.Errorf("build Qwen request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return provider.ImageResponse{}, fmt.Errorf("image request canceled: %w", ctx.Err())
		}
		return provider.ImageResponse{}, fmt.Errorf("image transport: %w", &transportError{upstream: provider.NewTransportError(provider.ProviderType(Adapter), err), cause: err})
	}
	defer resp.Body.Close()
	raw, err := nativeimage.ReadResponse(ctx, resp.Body, 32<<20)
	if err != nil {
		if ctx.Err() != nil {
			return provider.ImageResponse{}, fmt.Errorf("image response canceled: %w", ctx.Err())
		}
		return provider.ImageResponse{}, provider.NewInvalidResponseError(provider.ProviderType(Adapter), err)
	}
	var wire struct {
		RequestID string          `json:"request_id"`
		Code      string          `json:"code"`
		Message   string          `json:"message"`
		Usage     json.RawMessage `json:"usage"`
		Output    struct {
			Choices []struct {
				Message struct {
					Content []struct {
						Image string `json:"image"`
						Text  string `json:"text"`
					} `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		} `json:"output"`
	}
	if err = json.Unmarshal(raw, &wire); err != nil {
		if ctx.Err() != nil {
			return provider.ImageResponse{}, fmt.Errorf("image response canceled: %w", ctx.Err())
		}
		return provider.ImageResponse{}, provider.NewInvalidResponseError(provider.ProviderType(Adapter), err)
	}
	if wire.RequestID == "" {
		wire.RequestID = resp.Header.Get("x-request-id")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || wire.Code != "" {
		e := &provider.UpstreamError{Provider: provider.ProviderType(Adapter), HTTPStatus: resp.StatusCode, Code: wire.Code, Message: wire.Message, RequestID: wire.RequestID}
		provider.ClassifyUpstreamError(e)
		return provider.ImageResponse{}, e
	}
	out := provider.ImageResponse{Operation: r.Operation, ProviderRequestID: wire.RequestID, Usage: nativeimage.MeteringUsage(wire.Usage), ProviderMetadata: raw}
	var usage struct {
		Width        int `json:"width"`
		Height       int `json:"height"`
		OutputWidth  int `json:"output_width"`
		OutputHeight int `json:"output_height"`
	}
	_ = json.Unmarshal(wire.Usage, &usage)
	if usage.OutputWidth > 0 {
		usage.Width = usage.OutputWidth
		usage.Height = usage.OutputHeight
	}
	for _, ch := range wire.Output.Choices {
		for _, im := range ch.Message.Content {
			if im.Image != "" {
				out.Data = append(out.Data, provider.ImageResult{URL: im.Image, Format: "png", MimeType: "image/png", Width: usage.Width, Height: usage.Height})
			}
		}
	}
	if len(out.Data) == 0 || len(out.Data) > p.MaxOutputImages {
		return provider.ImageResponse{}, provider.NewInvalidResponseError(provider.ProviderType(Adapter), fmt.Errorf("invalid image count"))
	}
	return out, nil
}
func validMIME(s string) bool {
	switch s {
	case "image/jpeg", "image/png", "image/bmp", "image/tiff", "image/webp", "image/gif":
		return true
	}
	return false
}

// Preserve both provider classification and the original transport cause.
type transportError struct {
	upstream *provider.UpstreamError
	cause    error
}

func (e *transportError) Error() string   { return e.upstream.Error() }
func (e *transportError) Unwrap() []error { return []error{e.upstream, e.cause} }
