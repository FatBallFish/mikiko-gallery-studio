package openrouter_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/provider/openrouter"
)

func TestGenerateUsesChatCompletions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer or-key" {
			t.Fatalf("unexpected auth header %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode json: %v", err)
		}
		if got := body["model"]; got != "openrouter/imagen" {
			t.Fatalf("unexpected model %#v", got)
		}
		modalities, ok := body["modalities"].([]any)
		if !ok || len(modalities) != 2 {
			t.Fatalf("unexpected modalities %#v", body["modalities"])
		}
		messages, ok := body["messages"].([]any)
		if !ok || len(messages) != 1 {
			t.Fatalf("unexpected messages %#v", body["messages"])
		}
		message, ok := messages[0].(map[string]any)
		if !ok || message["content"] != "Generate a logo" {
			t.Fatalf("unexpected message %#v", messages[0])
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-request-id", "or-req-1")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"images":[{"image_url":{"url":"https://cdn.example.com/openrouter.png"}}]}}]}`)
	}))
	defer server.Close()

	client := openrouter.NewClient(openrouter.Config{BaseURL: server.URL, APIKey: "or-key", HTTPClient: server.Client()})
	resp, err := client.Generate(context.Background(), provider.ImageRequest{
		Model:            "openrouter/imagen",
		TaskType:         provider.TaskTypeTextToImage,
		Prompt:           "Generate a logo",
		Size:             "1024x1024",
		Quality:          "high",
		OutputImageCount: 1,
		ResponseFormat:   provider.ResponseFormatURL,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.ProviderRequestID != "or-req-1" {
		t.Fatalf("unexpected request id %q", resp.ProviderRequestID)
	}
	if len(resp.Data) != 1 || resp.Data[0].URL != "https://cdn.example.com/openrouter.png" {
		t.Fatalf("unexpected response %#v", resp)
	}
}

func TestEditNormalizesDataURLResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode json: %v", err)
		}
		messages, ok := body["messages"].([]any)
		if !ok || len(messages) != 1 {
			t.Fatalf("unexpected messages %#v", body["messages"])
		}
		message, ok := messages[0].(map[string]any)
		if !ok {
			t.Fatalf("unexpected message %#v", messages[0])
		}
		content, ok := message["content"].([]any)
		if !ok || len(content) != 3 {
			t.Fatalf("unexpected content %#v", message["content"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"images":[{"image_url":{"url":"data:image/png;base64,ZmFrZS1pbWFnZQ=="}}]}}]}`)
	}))
	defer server.Close()

	client := openrouter.NewClient(openrouter.Config{BaseURL: server.URL, APIKey: "or-key", HTTPClient: server.Client()})
	resp, err := client.Edit(context.Background(), provider.ImageRequest{
		Model:            "openrouter/imagen",
		TaskType:         provider.TaskTypeImageEdit,
		Prompt:           "Change the background to orange",
		Size:             "1536x1024",
		Quality:          "medium",
		OutputImageCount: 2,
		ResponseFormat:   provider.ResponseFormatB64JSON,
		ReferenceImages: []provider.ImageInput{
			{Filename: "source-1.png", MIMEType: "image/png", Data: []byte("image-1")},
			{Filename: "source-2.png", MIMEType: "image/png", Data: []byte("image-2")},
		},
	})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].B64JSON != "ZmFrZS1pbWFnZQ==" {
		t.Fatalf("unexpected response %#v", resp)
	}
}
