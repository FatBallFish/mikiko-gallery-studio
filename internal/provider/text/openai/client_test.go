package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	textprovider "github.com/fatballfish/pic-gallery/internal/provider/text"
	textopenai "github.com/fatballfish/pic-gallery/internal/provider/text/openai"
)

func TestClientOptimizesWithChatCompletions(t *testing.T) {
	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		if got := r.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Fatalf("unexpected authorization header %q", got)
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "gpt-test" || len(body.Messages) != 2 || body.Messages[1].Content != "short prompt" {
			t.Fatalf("unexpected request body %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-request-id", "req-chat")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"A detailed optimized prompt"}}],"usage":{"prompt_tokens":12,"completion_tokens":7}}`))
	}))
	defer server.Close()

	client, err := textopenai.NewClient(textopenai.Config{BaseURL: server.URL + "/v1", APIKey: "test-secret", APIStyle: textprovider.APIStyleChatCompletions})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	result, err := client.Optimize(context.Background(), textprovider.OptimizeRequest{Model: "gpt-test", SystemPrompt: "Improve image prompts", Prompt: "short prompt"})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if requestPath != "/v1/chat/completions" {
		t.Fatalf("unexpected chat path %q", requestPath)
	}
	if result.Text != "A detailed optimized prompt" || result.InputTokens != 12 || result.OutputTokens != 7 || result.RequestID != "req-chat" {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestClientOptimizesWithResponsesStructuredOutput(t *testing.T) {
	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"content":[{"type":"output_text","text":"Optimized through Responses"}]}],"usage":{"input_tokens":9,"output_tokens":5}}`))
	}))
	defer server.Close()

	client, err := textopenai.NewClient(textopenai.Config{BaseURL: server.URL, APIKey: "test-secret", APIStyle: textprovider.APIStyleResponses})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	result, err := client.Optimize(context.Background(), textprovider.OptimizeRequest{Model: "gpt-test", SystemPrompt: "Improve", Prompt: "draft"})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if requestPath != "/v1/responses" {
		t.Fatalf("unexpected responses path %q", requestPath)
	}
	if result.Text != "Optimized through Responses" || result.InputTokens != 9 || result.OutputTokens != 5 {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestClientSanitizesProviderErrors(t *testing.T) {
	const secret = "secret-must-not-leak"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_api_key","message":"upstream rejected ` + secret + `"}}`))
	}))
	defer server.Close()

	client, err := textopenai.NewClient(textopenai.Config{BaseURL: server.URL, APIKey: secret, APIStyle: textprovider.APIStyleChatCompletions})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.Optimize(context.Background(), textprovider.OptimizeRequest{Model: "gpt-test", Prompt: "draft"})
	if err == nil {
		t.Fatal("expected provider error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("provider error leaked credential: %v", err)
	}
	providerErr, ok := err.(*textprovider.Error)
	if !ok || providerErr.StatusCode != http.StatusUnauthorized || providerErr.Code != "invalid_api_key" {
		t.Fatalf("unexpected provider error %#v", err)
	}
}
