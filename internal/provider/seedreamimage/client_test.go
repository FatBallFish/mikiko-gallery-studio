package seedreamimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/internal/provider"
)

const pro = "doubao-seedream-5-0-pro-260628"
const layers = `{"created":123,"usage":{"input_images":1,"generated_images":2,"output_tokens":9,"future":1},"data":[{"url":"https://base","size":"1024x1024","output_format":"jpeg","z_index":0},{"url":"https://layer","size":"512x512","output_format":"png","z_index":1,"name":"tree","description":"green","bounding_box":{"absolute":[100,200,400,600],"normalized":[98,195,391,586]}}]}`

func boolptr(v bool) *bool { return &v }
func TestLayerPayloadMetadataCheckpoint(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/images/generations" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("request %v", r)
		}
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		want := map[string]any{"model": pro, "prompt": "split 图1 tree<point>520 460</point>", "image": "data:image/png;base64," + base64.StdEncoding.EncodeToString(fixturePNG()), "layer_decomposition": true, "size": "auto", "output_format": "jpeg", "response_format": "url", "watermark": false, "optimize_prompt_options": map[string]any{"mode": "fast"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("payload %#v want %#v", got, want)
		}
		w.Header().Set("x-request-id", "request")
		_, _ = w.Write([]byte(layers))
	}))
	defer s.Close()
	r := provider.ImageRequest{Model: pro, Operation: nativeimage.OperationLayerDecomposition, Prompt: "split", Size: "auto", OutputImageCount: 1, OutputFormat: "jpeg", ResponseFormat: provider.ResponseFormatURL, ReferenceImages: []provider.ImageInput{{MIMEType: "image/png", Data: fixturePNG()}}, NativeOptions: &nativeimage.Options{Watermark: boolptr(false), Seedream: &nativeimage.SeedreamOptions{OptimizePromptMode: "fast"}, Annotations: []nativeimage.Annotation{{ReferenceIndex: 1, Label: "tree", Point: &[2]int{520, 460}}}}}
	out, err := New(Config{BaseURL: s.URL, APIKey: "secret"}).Edit(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Data) != 2 || !out.Data[0].Layer.IsBase || out.Data[1].Layer.Description != "green" || out.Data[1].Layer.BoundingBox.Absolute[3] != 600 {
		t.Fatalf("metadata %+v", out)
	}
	type checkpoint struct {
		Response provider.ImageResponse `json:"response"`
		Metadata json.RawMessage        `json:"metadata"`
	}
	snapshot := checkpoint{out, out.ProviderMetadata}
	b, _ := json.Marshal(snapshot)
	var restoredCheckpoint checkpoint
	var restored provider.ImageResponse
	_ = json.Unmarshal(b, &restoredCheckpoint)
	restored = restoredCheckpoint.Response
	restored.ProviderMetadata = restoredCheckpoint.Metadata
	if public, _ := json.Marshal(out); bytes.Contains(public, []byte("provider_metadata")) {
		t.Fatal("public metadata leak")
	}
	if !reflect.DeepEqual(out, restored) {
		t.Fatalf("checkpoint lost fields %s", b)
	}
}
func TestOrdinaryAndEndpointID(t *testing.T) {
	p, _ := nativeimage.ProfileForModel(pro)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		if _, ok := got["n"]; ok {
			t.Error("sent n")
		}
		if _, ok := got["stream"]; ok {
			t.Error("sent stream")
		}
		if _, ok := got["sequential_image_generation"]; ok {
			t.Error("sent batch")
		}
		if got["model"] != "ep-real" {
			t.Error(got)
		}
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"eA==","size":"1024x1024","output_format":"png"}]}`))
	}))
	defer s.Close()
	out, err := New(Config{BaseURL: s.URL, Profile: &p}).Generate(context.Background(), provider.ImageRequest{Model: "ep-real", Prompt: "x", ResponseFormat: provider.ResponseFormatB64JSON})
	if err != nil || out.Data[0].B64JSON != "eA==" {
		t.Fatalf("%+v %v", out, err)
	}
}
func TestRejectedFields(t *testing.T) {
	seed := int64(1)
	cases := []provider.ImageRequest{{OutputImageCount: 17}, {OutputImageCount: -1}, {Mask: &provider.ImageInput{}}, {Quality: "high"}, {NativeOptions: &nativeimage.Options{Seed: &seed}}, {NativeOptions: &nativeimage.Options{NegativePrompt: "no"}}, {NativeOptions: &nativeimage.Options{Qwen: &nativeimage.QwenOptions{}}}, {Size: "512x512"}, {Size: "10000x1000"}, {Model: "doubao-seedream-5-0-flash-260915", NativeOptions: &nativeimage.Options{Seedream: &nativeimage.SeedreamOptions{OptimizePromptMode: "fast"}}}}
	for _, r := range cases {
		if r.Model == "" {
			r.Model = pro
		}
		r.Prompt = "x"
		if _, err := New(Config{BaseURL: "http://127.0.0.1:1"}).Generate(context.Background(), r); err == nil {
			t.Errorf("accepted %+v", r)
		}
	}
}
func TestLayerResponseValidationAndErrors(t *testing.T) {
	cases := []struct {
		body   string
		status int
		op     nativeimage.Operation
	}{{`{"error":{"code":"bad","message":"invalid"}}`, 400, nativeimage.OperationEdit}, {`{"error":{"code":"bad","message":"invalid"}}`, 200, nativeimage.OperationEdit}, {`invalid`, 200, nativeimage.OperationEdit}, {`{"data":[]}`, 200, nativeimage.OperationEdit}, {layers, 200, nativeimage.OperationEdit}, {strings.ReplaceAll(layers, `"z_index":0`, `"z_index":1`), 200, nativeimage.OperationLayerDecomposition}, {strings.ReplaceAll(layers, `"normalized":[98,195,391,586]`, `"normalized":[98,195,1001,586]`), 200, nativeimage.OperationLayerDecomposition}, {strings.ReplaceAll(layers, `"absolute":[100,200,400,600]`, `"absolute":[100,200,400,1600]`), 200, nativeimage.OperationLayerDecomposition}, {strings.ReplaceAll(layers, `"output_format":"png"`, `"output_format":"jpeg"`), 200, nativeimage.OperationLayerDecomposition}}
	for _, tc := range cases {
		t.Run(tc.body[:min(20, len(tc.body))], func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer s.Close()
			_, err := New(Config{BaseURL: s.URL}).Edit(context.Background(), provider.ImageRequest{Model: pro, Prompt: "x", Operation: tc.op, ReferenceImages: []provider.ImageInput{{MIMEType: "image/png", Data: fixturePNG()}}})
			if _, ok := provider.AsUpstreamError(err); !ok {
				t.Fatalf("expected upstream error: %v", err)
			}
		})
	}
}
func TestTransparency(t *testing.T) {
	for _, transparent := range []bool{false, true} {
		im := image.NewNRGBA(image.Rect(0, 0, 16, 16))
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				im.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
			}
		}
		if transparent {
			im.SetNRGBA(0, 0, color.NRGBA{})
		}
		var b bytes.Buffer
		_ = png.Encode(&b, im)
		err := validateTransparency(context.Background(), b.Bytes())
		if (err == nil) != transparent {
			t.Fatalf("alpha=%v err=%v", transparent, err)
		}
	}
	if validateTransparency(context.Background(), []byte("invalid")) == nil {
		t.Fatal("invalid bytes accepted")
	}
}

func fixturePNG() []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 512, 512)))
	return b.Bytes()
}

func TestPartialPreservesPaidArtifactsAndSorts(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprint(partial), func(t *testing.T) {
			var wire map[string]any
			_ = json.Unmarshal([]byte(layers), &wire)
			items := wire["data"].([]any)
			items[0], items[1] = items[1], items[0]
			if partial {
				items = append(items, map[string]any{"error": map[string]any{"code": "failed", "message": "private https://signed"}})
			}
			wire["data"] = items
			body, _ := json.Marshal(wire)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
			defer server.Close()
			out, err := New(Config{BaseURL: server.URL}).Edit(context.Background(), provider.ImageRequest{Model: pro, Operation: nativeimage.OperationLayerDecomposition, ReferenceImages: []provider.ImageInput{{MIMEType: "image/png", Data: fixturePNG()}}})
			if len(out.Data) != 2 || out.Data[0].Layer.ZIndex != 0 || out.Data[1].Layer.ZIndex != 1 {
				t.Fatalf("lost/scrambled results %+v %v", out, err)
			}
			if partial {
				up, ok := provider.AsUpstreamError(err)
				if !ok || up.Action != provider.UpstreamErrorActionPass || up.Code != "partial_artifact_error" {
					t.Fatal(err)
				}
				if strings.Contains(err.Error(), "signed") {
					t.Fatal("leaked private diagnostics")
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestGenerationEndpointPaths(t *testing.T) {
	cases := []struct {
		basePath string
		wantPath string
	}{
		{"", "/api/v3/images/generations"},
		{"/", "/api/v3/images/generations"},
		{"/api/v3", "/api/v3/images/generations"},
		{"/api/v3/", "/api/v3/images/generations"},
		{"/api/v3///", "/api/v3/images/generations"},
		{"/api/v3/images/generations", "/api/v3/images/generations"},
		{"/api/v3/images/generations/", "/api/v3/images/generations"},
		{"/images/generations", "/images/generations"},
		{"/proxy/ark", "/proxy/ark/images/generations"},
		{"/proxy/ark/", "/proxy/ark/images/generations"},
		{"/proxy/api/v3", "/proxy/api/v3/images/generations"},
		{"/proxy/api/v3/images/generations/", "/proxy/api/v3/images/generations"},
	}
	for _, tc := range cases {
		t.Run(tc.basePath, func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPost || r.URL.RequestURI() != tc.wantPath {
					t.Errorf("request %s %s, want POST %s", r.Method, r.URL.RequestURI(), tc.wantPath)
				}
				_, _ = w.Write([]byte(`{"data":[{"b64_json":"eA==","size":"1024x1024","output_format":"png"}]}`))
			}))
			defer s.Close()
			_, err := New(Config{BaseURL: s.URL + tc.basePath}).Generate(context.Background(), provider.ImageRequest{Model: pro, Prompt: "x"})
			if err != nil || calls != 1 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
	for _, base := range []string{"https://ark.cn-beijing.volces.com", "https://ark.cn-beijing.volces.com/", DefaultBaseURL, DefaultBaseURL + "/"} {
		got, err := generationEndpoint(base)
		if err != nil || got != "https://ark.cn-beijing.volces.com/api/v3/images/generations" {
			t.Fatalf("official endpoint = %q, %v", got, err)
		}
	}
}

func TestGenerationEndpointRejectsInvalidBeforeTransport(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		t.Error("invalid endpoint reached transport")
	}))
	defer s.Close()
	bases := []string{
		"/api/v3", "ark.cn-beijing.volces.com", "ftp://example.com", "https://", "https://%zz",
		"https://user:private-secret@example.com", s.URL + "?token=private-secret", s.URL + "?", s.URL + "#private-secret", s.URL + "#",
		s.URL + "/proxy/../api/v3", s.URL + "/proxy/./api/v3", s.URL + "/proxy//api/v3",
		s.URL + "/proxy/%2e%2e/api/v3", s.URL + "/proxy%2fapi/v3", s.URL + "/proxy\\api/v3", s.URL + "/proxy path",
	}
	for i, base := range bases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			_, err := New(Config{BaseURL: base, APIKey: "private-secret"}).Generate(context.Background(), provider.ImageRequest{Model: pro, Prompt: "x"})
			if err == nil || !strings.Contains(err.Error(), "invalid Seedream BaseURL") || strings.Contains(err.Error(), "private-secret") || strings.Contains(err.Error(), base) {
				t.Fatalf("expected sanitized configuration error, got %v", err)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("invalid endpoints made %d requests", calls)
	}
}

func TestCanceledContextCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New(Config{}).Generate(ctx, provider.ImageRequest{Model: pro, Prompt: "x"})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
