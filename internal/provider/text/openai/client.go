package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"

	textprovider "github.com/fatballfish/pic-gallery/internal/provider/text"
)

var safeErrorCode = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

type Config struct {
	BaseURL    string
	APIKey     string
	APIStyle   string
	HTTPClient *http.Client
}

type Client struct {
	baseURL    *url.URL
	apiKey     string
	apiStyle   string
	httpClient *http.Client
}

func NewClient(cfg Config) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.User != nil {
		return nil, fmt.Errorf("invalid text provider base URL")
	}
	apiStyle := strings.TrimSpace(cfg.APIStyle)
	if apiStyle != textprovider.APIStyleChatCompletions && apiStyle != textprovider.APIStyleResponses {
		return nil, fmt.Errorf("unsupported text provider API style")
	}
	baseClient := cfg.HTTPClient
	if baseClient == nil {
		baseClient = http.DefaultClient
	}
	clientCopy := *baseClient
	existingRedirect := baseClient.CheckRedirect
	clientCopy.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !strings.EqualFold(req.URL.Scheme, baseURL.Scheme) || !strings.EqualFold(req.URL.Host, baseURL.Host) {
			return fmt.Errorf("text provider redirect target is not allowed")
		}
		if len(via) >= 3 {
			return fmt.Errorf("text provider redirect limit exceeded")
		}
		if existingRedirect != nil {
			return existingRedirect(req, via)
		}
		return nil
	}
	return &Client{baseURL: baseURL, apiKey: strings.TrimSpace(cfg.APIKey), apiStyle: apiStyle, httpClient: &clientCopy}, nil
}

func (c *Client) Optimize(ctx context.Context, req textprovider.OptimizeRequest) (textprovider.OptimizeResponse, error) {
	requestPath := "/v1/chat/completions"
	payload := map[string]any{
		"model": req.Model,
		"messages": []map[string]string{
			{"role": "system", "content": req.SystemPrompt},
			{"role": "user", "content": req.Prompt},
		},
	}
	if c.apiStyle == textprovider.APIStyleResponses {
		requestPath = "/v1/responses"
		payload = map[string]any{"model": req.Model, "instructions": req.SystemPrompt, "input": req.Prompt}
	}
	if req.MaxOutputTokens > 0 {
		payload["max_output_tokens"] = req.MaxOutputTokens
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return textprovider.OptimizeResponse{}, fmt.Errorf("marshal text provider request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(requestPath), bytes.NewReader(body))
	if err != nil {
		return textprovider.OptimizeResponse{}, fmt.Errorf("create text provider request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return textprovider.OptimizeResponse{}, &textprovider.Error{Code: "transport_error", Message: "text provider is unavailable"}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return textprovider.OptimizeResponse{}, decodeProviderError(resp)
	}
	result, err := decodeResponse(resp, c.apiStyle)
	if err != nil {
		return textprovider.OptimizeResponse{}, err
	}
	result.RequestID = resp.Header.Get("x-request-id")
	return result, nil
}

func (c *Client) endpoint(requestPath string) string {
	copyURL := *c.baseURL
	basePath := strings.TrimRight(copyURL.Path, "/")
	if strings.HasSuffix(strings.ToLower(basePath), "/v1") {
		requestPath = strings.TrimPrefix(requestPath, "/v1")
	}
	copyURL.Path = path.Join(basePath, requestPath)
	copyURL.RawPath = ""
	return copyURL.String()
}

func decodeResponse(resp *http.Response, apiStyle string) (textprovider.OptimizeResponse, error) {
	var payload struct {
		OutputText string `json:"output_text"`
		Choices    []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			InputTokens      int `json:"input_tokens"`
			OutputTokens     int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return textprovider.OptimizeResponse{}, &textprovider.Error{StatusCode: resp.StatusCode, Code: "invalid_response", Message: "text provider returned invalid JSON"}
	}
	text := strings.TrimSpace(payload.OutputText)
	if apiStyle == textprovider.APIStyleChatCompletions && len(payload.Choices) > 0 {
		text = strings.TrimSpace(payload.Choices[0].Message.Content)
	}
	if text == "" {
		for _, output := range payload.Output {
			for _, content := range output.Content {
				if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
					text = strings.TrimSpace(content.Text)
					break
				}
			}
			if text != "" {
				break
			}
		}
	}
	if text == "" {
		return textprovider.OptimizeResponse{}, &textprovider.Error{StatusCode: resp.StatusCode, Code: "empty_response", Message: "text provider returned no optimized prompt"}
	}
	inputTokens := payload.Usage.InputTokens
	if inputTokens == 0 {
		inputTokens = payload.Usage.PromptTokens
	}
	outputTokens := payload.Usage.OutputTokens
	if outputTokens == 0 {
		outputTokens = payload.Usage.CompletionTokens
	}
	return textprovider.OptimizeResponse{Text: text, InputTokens: inputTokens, OutputTokens: outputTokens}, nil
}

func decodeProviderError(resp *http.Response) error {
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	code := strings.TrimSpace(payload.Error.Code)
	if !safeErrorCode.MatchString(code) {
		code = http.StatusText(resp.StatusCode)
		code = strings.ToLower(strings.ReplaceAll(code, " ", "_"))
	}
	return &textprovider.Error{StatusCode: resp.StatusCode, Code: code, Message: "text provider request failed"}
}
