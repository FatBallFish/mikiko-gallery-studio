package mcpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNativePreviewBudgetAndDecode(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1254, 1254))
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, src); err != nil {
		t.Fatal(err)
	}
	calls := 0
	cfg := Config{Preview: func(context.Context, openplatform.Principal, string) (mediaasset.ContentStream, error) {
		calls++
		return mediaasset.ContentStream{Reader: io.NopCloser(bytes.NewReader(encoded.Bytes()))}, nil
	}}
	result := envelopeResult(map[string]any{"data": map[string]any{"url": "https://studio.example/access"}})
	got := withImagePreview(context.Background(), cfg, openplatform.Principal{}, "get_image_result_access", []byte(`{"purpose":"preview"}`), map[string]any{"url": "https://studio.example/access"}, result)
	if got.IsError || calls != 1 || !fitsResult(got) {
		t.Fatalf("unexpected result calls=%d result=%+v", calls, got)
	}
	if len(got.Content) != 2 {
		t.Fatalf("contents=%d", len(got.Content))
	}
	img, ok := got.Content[1].(*mcp.ImageContent)
	if !ok {
		t.Fatalf("image content missing: %T", got.Content[1])
	}
	if len(img.Data) > 18<<10 || img.MIMEType != "image/jpeg" {
		t.Fatal("image exceeds budget or incorrect MIME")
	}
	decoded, err := jpeg.Decode(bytes.NewReader(img.Data))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() > 256 || decoded.Bounds().Dy() > 256 {
		t.Fatal("thumbnail dimensions exceed cap")
	}
	for _, purpose := range []string{"download", "thumbnail", ""} {
		withImagePreview(context.Background(), cfg, openplatform.Principal{}, "get_image_result_access", []byte(`{"purpose":"`+purpose+`"}`), map[string]any{"url": "https://studio.example/access"}, result)
	}
	if calls != 1 {
		t.Fatalf("non-preview called reader: %d", calls)
	}
}

func TestPreviewRejectsLargeDimensionsAndSource(t *testing.T) {
	var data bytes.Buffer
	png.Encode(&data, image.NewGray(image.Rect(0, 0, 8192, 4097)))
	if _, err := thumbnailJPEG(context.Background(), bytes.NewReader(data.Bytes())); err == nil {
		t.Fatal("oversized dimensions accepted")
	}
	if _, err := thumbnailJPEG(context.Background(), io.LimitReader(zeroReader{}, (16<<20)+1)); err == nil {
		t.Fatal("oversized input accepted")
	}
}

func TestOfficialSDKNativePreview(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var source bytes.Buffer
	src := image.NewRGBA(image.Rect(0, 0, 1254, 1254))
	for i := range src.Pix {
		src.Pix[i] = byte((i*31 + i/113) % 256)
	}
	png.Encode(&source, src)
	expected, err := thumbnailJPEG(ctx, bytes.NewReader(source.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	p := openplatform.Principal{UserID: 7, ClientID: "c", GrantID: "g", Scopes: []string{"images:read"}}
	var fail atomic.Bool
	handler, err := NewHandler(Config{ResourceURL: "https://studio.example/mcp", Validator: ValidateAccessFunc(func(context.Context, string) (openplatform.Principal, error) { return p, nil }), Service: executeFunc(func(context.Context, openplatform.Principal, string, json.RawMessage) (any, error) {
		return mediaasset.AccessResult{URL: "https://studio.example/api/open/v1/asset-access/test/content", ExpiresAt: time.Now().Add(time.Minute), Range: true}, nil
	}), Preview: func(context.Context, openplatform.Principal, string) (mediaasset.ContentStream, error) {
		if fail.Load() {
			return mediaasset.ContentStream{}, errors.New("fixture failure")
		}
		return mediaasset.ContentStream{Reader: io.NopCloser(bytes.NewReader(source.Bytes())), SizeBytes: int64(source.Len())}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "preview-test", Version: "1"}, nil)
	capture := &previewCaptureTransport{next: bearerTransport{token: "fixture", next: http.DefaultTransport}}
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: srv.URL, HTTPClient: &http.Client{Transport: capture}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	call := func() *mcp.CallToolResult {
		r, e := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_image_result_access", Arguments: map[string]any{"task_id": "task", "image_id": "image", "purpose": "preview"}})
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	got := call()
	if got.IsError || len(got.Content) != 2 {
		t.Fatalf("unexpected result %+v", got)
	}
	img, ok := got.Content[1].(*mcp.ImageContent)
	if !ok {
		t.Fatalf("missing image: %T", got.Content[1])
	}
	if sha256.Sum256(img.Data) != sha256.Sum256(expected) {
		t.Fatal("SDK image bytes changed")
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(img.Data))
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": got})
	if len(wire) >= resultBudget {
		t.Fatalf("wire=%d", len(wire))
	}
	t.Logf("source=%d JPEG=%d wire=%d dimensions=%dx%d sha256=%x", source.Len(), len(img.Data), len(wire), cfg.Width, cfg.Height, sha256.Sum256(img.Data))
	actual := capture.body()
	if len(actual) >= resultBudget || bytes.Count(actual, []byte(`"type":"image"`)) != 1 || bytes.Count(actual, []byte(`"mimeType":"image/jpeg"`)) != 1 {
		t.Fatalf("actual HTTP image body invalid: bytes=%d images=%d", len(actual), bytes.Count(actual, []byte(`"type":"image"`)))
	}
	t.Logf("actual HTTP/SSE body=%d JPEG=%d dimensions=%dx%d sha256=%x", len(actual), len(img.Data), cfg.Width, cfg.Height, sha256.Sum256(img.Data))
	fail.Store(true)
	got = call()
	if got.IsError {
		t.Fatal("thumbnail failure must preserve access success")
	}
	text, _ := json.Marshal(got.Content)
	if !strings.Contains(string(text), "thumbnail unavailable") {
		t.Fatal("missing failure note")
	}
}

func TestPreviewCancellationClosesStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	defer writer.Close()
	cfg := Config{Preview: func(context.Context, openplatform.Principal, string) (mediaasset.ContentStream, error) {
		cancel()
		return mediaasset.ContentStream{Reader: reader}, nil
	}}
	done := make(chan *mcp.CallToolResult, 1)
	go func() {
		done <- withImagePreview(ctx, cfg, openplatform.Principal{}, "get_image_result_access", []byte(`{"purpose":"preview"}`), map[string]any{"url": "fixture"}, envelopeResult(map[string]any{"data": map[string]any{"url": "fixture"}}))
	}()
	select {
	case result := <-done:
		if result.IsError {
			t.Fatal("access success lost")
		}
	case <-time.After(time.Second):
		reader.Close()
		t.Fatal("canceled read did not close")
	}
}

func TestPreviewFailureNoteSurvivesBudget(t *testing.T) {
	value := map[string]any{"url": strings.Repeat("a", 19000)}
	result := envelopeResult(map[string]any{"data": value})
	got := withImagePreview(context.Background(), Config{}, openplatform.Principal{}, "get_image_result_access", []byte(`{"purpose":"preview"}`), value, result)
	if !fitsResult(got) || got.IsError || got.StructuredContent == nil {
		t.Fatal("failure fallback lost metadata or budget")
	}
	wire, _ := json.Marshal(got.Content)
	if !strings.Contains(string(wire), "thumbnail unavailable") {
		t.Fatal("note omitted")
	}
}

type previewCaptureTransport struct {
	next http.RoundTripper
	mu   sync.Mutex
	last []byte
}

func (c *previewCaptureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := c.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if req.Method == http.MethodPost && (strings.Contains(res.Header.Get("Content-Type"), "text/event-stream") || strings.Contains(res.Header.Get("Content-Type"), "application/json")) {
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return nil, err
		}
		res.Body = io.NopCloser(bytes.NewReader(body))
		c.mu.Lock()
		c.last = append([]byte(nil), body...)
		c.mu.Unlock()
	}
	return res, nil
}
func (c *previewCaptureTransport) body() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.last...)
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }
