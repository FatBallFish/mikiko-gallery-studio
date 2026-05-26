package openai_test

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/provider/openai"
)

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
		if _, ok := body["n"]; ok {
			t.Fatalf("generation request must not send unsupported n field: %#v", body["n"])
		}
		if got := body["response_format"]; got != "b64_json" {
			t.Fatalf("unexpected response_format %#v", got)
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
		if got := r.FormValue("n"); got != "" {
			t.Fatalf("edit request must not send unsupported n field, got %q", got)
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
