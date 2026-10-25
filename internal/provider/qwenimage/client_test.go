package qwenimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/internal/provider"
)

func ptr[T any](v T) *T { return &v }
func TestNativePayloadAndCheckpoint(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/services/aigc/multimodal-generation/generation" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("unexpected request %s", r.URL)
		}
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"model": "qwen-image-3.0", "input": map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"image": "data:image/png;base64," + base64.StdEncoding.EncodeToString(fixturePNG())}, map[string]any{"text": "edit"}}}}}, "parameters": map[string]any{"n": float64(2), "size": "2048*1024", "seed": float64(0), "negative_prompt": "no text", "watermark": false, "prompt_extend": true, "enable_thinking": false, "prompt_extend_mode": "direct"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("payload=%#v want=%#v", got, want)
		}
		_, _ = w.Write([]byte(`{"request_id":"req","output":{"choices":[{"message":{"content":[{"image":"https://image/1"},{"image":"https://image/2"}]}}]},"usage":{"output_width":2048,"output_height":1024,"output_image_count":2},"extra":"preserved"}`))
	}))
	defer s.Close()
	out, err := New(Config{BaseURL: s.URL, APIKey: "secret"}).Edit(context.Background(), provider.ImageRequest{Model: "qwen-image-3.0", Prompt: "edit", Size: "2048x1024", OutputImageCount: 2, ReferenceImages: []provider.ImageInput{{MIMEType: "image/png", Data: fixturePNG()}}, NativeOptions: &nativeimage.Options{Seed: ptr(int64(0)), NegativePrompt: "no text", Watermark: ptr(false), Qwen: &nativeimage.QwenOptions{PromptExtend: ptr(true), EnableThinking: ptr(false), PromptExtendMode: "direct"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Data) != 2 || out.Data[0].Width != 2048 || out.Operation != nativeimage.OperationEdit {
		t.Fatalf("bad response %+v", out)
	}
	type checkpoint struct {
		Response provider.ImageResponse `json:"response"`
		Metadata json.RawMessage        `json:"metadata"`
	}
	snapshot := checkpoint{out, out.ProviderMetadata}
	b, _ := json.Marshal(snapshot)
	var restoredCheckpoint checkpoint
	var restored provider.ImageResponse
	if err = json.Unmarshal(b, &restoredCheckpoint); err != nil {
		t.Fatal(err)
	}
	restored = restoredCheckpoint.Response
	restored.ProviderMetadata = restoredCheckpoint.Metadata
	if public, _ := json.Marshal(out); bytes.Contains(public, []byte("provider_metadata")) {
		t.Fatal("public metadata leak")
	}
	if !reflect.DeepEqual(out, restored) {
		t.Fatalf("checkpoint lost fields: %s", b)
	}
}
func TestRejectUnsupportedBeforeHTTP(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer s.Close()
	cases := []provider.ImageRequest{{Mask: &provider.ImageInput{}}, {Quality: "high"}, {ResponseFormat: provider.ResponseFormatB64JSON}, {Background: "transparent"}, {NativeOptions: &nativeimage.Options{Seedream: &nativeimage.SeedreamOptions{}}}, {NativeOptions: &nativeimage.Options{Seed: ptr(int64(-1))}}, {OutputImageCount: 7}, {Size: "3000x3000"}, {NativeOptions: &nativeimage.Options{NegativePrompt: "no"}, Model: "qwen-image-2.1-pro"}, {NativeOptions: &nativeimage.Options{Qwen: &nativeimage.QwenOptions{PromptExtend: ptr(false), EnableThinking: ptr(true)}}}}
	for _, r := range cases {
		if r.Model == "" {
			r.Model = "qwen-image-3.0"
		}
		r.Prompt = "x"
		if _, err := New(Config{BaseURL: s.URL}).Generate(context.Background(), r); err == nil {
			t.Errorf("accepted %+v", r)
		}
	}
	if calls != 0 {
		t.Fatalf("sent %d invalid calls", calls)
	}
}
func TestErrors(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
		code   string
	}{{`{"code":"InvalidApiKey","message":"bad","request_id":"id"}`, 401, "InvalidApiKey"}, {`{"code":"Bad","message":"bad"}`, 200, "Bad"}, {`broken`, 200, "invalid_response"}, {`{"output":{}}`, 200, "invalid_response"}, {`{"code":"limit"}`, 429, "limit"}} {
		t.Run(tc.code, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer s.Close()
			_, err := New(Config{BaseURL: s.URL}).Generate(context.Background(), provider.ImageRequest{Model: "qwen-image-3.0", Prompt: "x"})
			e, ok := provider.AsUpstreamError(err)
			if !ok || e.Code != tc.code {
				t.Fatalf("%v", err)
			}
		})
	}
}
func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New(Config{}).Generate(ctx, provider.ImageRequest{Model: "qwen-image-3.0", Prompt: "x"})
	if err == nil {
		t.Fatal("expected cancellation")
	}
}

func fixturePNG() []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 512, 512)))
	return b.Bytes()
}
