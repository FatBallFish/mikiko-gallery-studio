package openai_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/provider/openai"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGenerateOmitsGPTImageOptionalFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"size", "response_format", "background", "stream", "partial_images"} {
			if _, ok := body[key]; ok {
				t.Fatalf("payload must omit %q: %#v", key, body)
			}
		}
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer server.Close()
	client := openai.NewClient(openai.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	_, err := client.Generate(context.Background(), provider.ImageRequest{Model: "gpt-image-2", Prompt: "safe test", Quality: "auto", OutputFormat: "png", Moderation: "auto", OutputImageCount: 1})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGenerateRetainsResponseFormatForLegacyImageModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if got := body["response_format"]; got != string(provider.ResponseFormatB64JSON) {
			t.Fatalf("legacy image payload response_format = %#v, want b64_json: %#v", got, body)
		}
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer server.Close()

	client := openai.NewClient(openai.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	_, err := client.Generate(context.Background(), provider.ImageRequest{
		Model:          "dall-e-3",
		Prompt:         "safe test",
		ResponseFormat: provider.ResponseFormatB64JSON,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGenerateIncludesValidatedBackground(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["background"] != "transparent" || body["size"] != "1280x720" {
			t.Fatalf("payload = %#v", body)
		}
		if _, ok := body["response_format"]; ok {
			t.Fatalf("GPT Image payload includes response_format: %#v", body)
		}
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer server.Close()
	client := openai.NewClient(openai.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	_, err := client.Generate(context.Background(), provider.ImageRequest{Model: "gpt-image-2", Prompt: "safe test", Size: "1280x720", Background: "transparent", OutputFormat: "png", OutputImageCount: 1})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGenerateRejectsTransparentJPEGBeforeHTTP(t *testing.T) {
	called := false
	client := openai.NewClient(openai.Config{
		BaseURL: "https://api.example.test",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			called = true
			return nil, errors.New("must not be called")
		})},
	})
	_, err := client.Generate(context.Background(), provider.ImageRequest{
		Model: "gpt-image-2", Prompt: "safe test", Background: "transparent", OutputFormat: "jpeg",
	})
	if err == nil {
		t.Fatal("expected transparent JPEG validation error")
	}
	if called {
		t.Fatal("transparent JPEG must be rejected before HTTP")
	}
}

func TestGenerateDecodesReturnedImageDimensions(t *testing.T) {
	const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lqR5DQAAAABJRU5ErkJggg=="
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"`+onePixelPNG+`"}]}`)
	}))
	defer server.Close()
	client := openai.NewClient(openai.Config{BaseURL: server.URL, HTTPClient: server.Client()})

	resp, err := client.Generate(context.Background(), provider.ImageRequest{Model: "gpt-image-2", Prompt: "dimension test", OutputFormat: "png", OutputImageCount: 1})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Width != 1 || resp.Data[0].Height != 1 {
		t.Fatalf("decoded dimensions = %#v", resp.Data)
	}
}

func TestGenerateUsesImagesEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected auth header %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode json: %v", err)
		}
		if got := body["model"]; got != "gpt-image-2" {
			t.Fatalf("unexpected model %#v", got)
		}
		if got := body["n"]; got != float64(3) {
			t.Fatalf("unexpected n %#v", got)
		}
		if _, ok := body["response_format"]; ok {
			t.Fatalf("GPT Image must omit response_format: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-request-id", "openai-req-1")
		_, _ = io.WriteString(w, `{"created":1770000000,"data":[{"b64_json":"ZmFrZQ==","revised_prompt":"better prompt"}]}`)
	}))
	defer server.Close()

	client := openai.NewClient(openai.Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
	resp, err := client.Generate(context.Background(), provider.ImageRequest{
		Model:            "gpt-image-2",
		TaskType:         provider.TaskTypeTextToImage,
		Prompt:           "A scenic mountain lake",
		Size:             "1536x1024",
		Quality:          "high",
		OutputImageCount: 3,
		ResponseFormat:   provider.ResponseFormatB64JSON,
		User:             "user-123",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.ProviderRequestID != "openai-req-1" {
		t.Fatalf("unexpected provider request id %q", resp.ProviderRequestID)
	}
	if len(resp.Data) != 1 || resp.Data[0].B64JSON != "ZmFrZQ==" {
		t.Fatalf("unexpected response %#v", resp)
	}
}

func TestGenerateParsesOpenAICompatibleB64JSONWithUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"created": 1783531969,
			"model": "gpt-image-2",
			"data": [
				{"b64_json": "ZmFrZS1pbWFnZQ=="}
			],
			"usage": {
				"prompt_tokens": 16,
				"completion_tokens": 1756,
				"total_tokens": 1772
			}
		}`)
	}))
	defer server.Close()

	client := openai.NewClient(openai.Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
	resp, err := client.Generate(context.Background(), provider.ImageRequest{
		Model:          "gpt-image-2",
		Prompt:         "A small product photo of a ceramic coffee cup on a clean desk",
		Size:           "1024x1024",
		Quality:        "auto",
		ResponseFormat: provider.ResponseFormatB64JSON,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Created != 1783531969 || len(resp.Data) != 1 || resp.Data[0].B64JSON != "ZmFrZS1pbWFnZQ==" {
		t.Fatalf("unexpected response %#v", resp)
	}
}

func TestGenerateMapsTransportEOFToUpstreamUnavailable(t *testing.T) {
	client := openai.NewClient(openai.Config{
		BaseURL: "https://api.example.test",
		APIKey:  "test-key",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, io.ErrUnexpectedEOF
		})},
	})

	_, err := client.Generate(context.Background(), provider.ImageRequest{
		Model:          "gpt-image-2",
		Prompt:         "test",
		Size:           "1024x1024",
		Quality:        "auto",
		ResponseFormat: provider.ResponseFormatB64JSON,
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	upstream, ok := provider.AsUpstreamError(err)
	if !ok {
		t.Fatalf("expected upstream error, got %T %[1]v", err)
	}
	if upstream.Family != provider.UpstreamErrorFamilyUnavailable || upstream.Code != "transport_error" {
		t.Fatalf("unexpected upstream classification %#v", upstream)
	}
}

func TestGenerateMapsResponseBodyReadTimeoutToTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(50 * time.Millisecond)
		_, _ = io.WriteString(w, `{"created":1770000000,"data":[{"b64_json":"ZmFrZQ=="}]}`)
	}))
	defer server.Close()

	client := openai.NewClient(openai.Config{
		BaseURL:    server.URL,
		APIKey:     "test-key",
		HTTPClient: &http.Client{Timeout: 10 * time.Millisecond},
	})
	_, err := client.Generate(context.Background(), provider.ImageRequest{
		Model:          "gpt-image-2",
		Prompt:         "test",
		Size:           "1024x1024",
		Quality:        "auto",
		ResponseFormat: provider.ResponseFormatB64JSON,
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	upstream, ok := provider.AsUpstreamError(err)
	if !ok {
		t.Fatalf("expected upstream error, got %T %[1]v", err)
	}
	if upstream.Code != "timeout" || upstream.Type != "timeout" || upstream.Family != provider.UpstreamErrorFamilyUnavailable {
		t.Fatalf("unexpected upstream classification %#v", upstream)
	}
}

func TestEditUsesMultipartImagesEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("parse media type: %v", err)
		}
		if mediaType != "multipart/form-data" {
			t.Fatalf("unexpected media type %q", mediaType)
		}
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		if got := r.FormValue("model"); got != "gpt-image-2" {
			t.Fatalf("unexpected model %q", got)
		}
		if got := r.FormValue("n"); got != "2" {
			t.Fatalf("unexpected n %q", got)
		}
		images := r.MultipartForm.File["image"]
		if len(images) != 2 {
			t.Fatalf("expected 2 image parts, got %d", len(images))
		}
		masks := r.MultipartForm.File["mask"]
		if len(masks) != 1 {
			t.Fatalf("expected 1 mask part, got %d", len(masks))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"created":1770000001,"data":[{"url":"https://cdn.example.com/result.png"}]}`)
	}))
	defer server.Close()

	client := openai.NewClient(openai.Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
	resp, err := client.Edit(context.Background(), provider.ImageRequest{
		Model:            "gpt-image-2",
		TaskType:         provider.TaskTypeImageEdit,
		Prompt:           "Replace the sky with sunset colors",
		Size:             "1024x1024",
		Quality:          "medium",
		OutputImageCount: 2,
		ResponseFormat:   provider.ResponseFormatURL,
		ReferenceImages: []provider.ImageInput{
			{Filename: "source-1.png", MIMEType: "image/png", Data: []byte("img-1")},
			{Filename: "source-2.png", MIMEType: "image/png", Data: []byte("img-2")},
		},
		Mask: &provider.ImageInput{Filename: "mask.png", MIMEType: "image/png", Data: []byte("mask")},
	})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if len(resp.Data) != 1 || !strings.Contains(resp.Data[0].URL, "result.png") {
		t.Fatalf("unexpected response %#v", resp)
	}
}
