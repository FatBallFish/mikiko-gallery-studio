package video

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	providervideo "github.com/fatballfish/pic-gallery/internal/provider/video"
	"github.com/fatballfish/pic-gallery/internal/provider/video/minimax"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

func localFrame(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestRunnerLocalFirstFrameReachesMiniMax(t *testing.T) {
	content := localFrame(t)
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/v2/video_generation" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body struct {
			Content []struct {
				Role     string `json:"role"`
				ImageURL struct {
					URL string `json:"url"`
				} `json:"image_url"`
			} `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.Content) != 2 || body.Content[1].Role != "first_frame" || body.Content[1].ImageURL.URL != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(content) {
			t.Errorf("first frame did not reach adapter")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"task_id":"local-frame-job"}`)
	}))
	defer server.Close()
	provider, err := minimax.NewClient(minimax.Config{BaseURL: server.URL, APIKey: "test", ModelCode: "MiniMax-H3", Verified: true})
	if err != nil {
		t.Fatal(err)
	}
	backend := storage.NewLocalBackend(t.TempDir())
	if err := backend.Put(t.Context(), "frame.png", "image/png", content); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	store := newMemoryStore(WorkItem{ID: "local-frame", TaskID: "task-local-frame", State: domainvideo.ItemStateQueued, Version: 1, ProviderCode: "minimax", Request: providervideo.Request{TaskType: "image_to_video", Prompt: "move", DurationSeconds: 5, Resolution: "768p", AspectRatio: "16:9", Inputs: []providervideo.Input{{AssetID: "frame", Role: "first_frame", MediaType: "image", StorageDriver: "local", ObjectKey: "frame.png", MIMEType: "image/png", SizeBytes: int64(len(content)), SHA256: hex.EncodeToString(sum[:])}}}})
	runner := NewRunner(store, staticProviderResolver{provider: provider}, storage.NewStaticRouter(backend), Options{Owner: "local-worker"})
	if processed, err := runner.RunOnce(t.Context()); err != nil || !processed {
		t.Fatalf("RunOnce = %v, %v", processed, err)
	}
	if !called || store.itemSnapshot().State != domainvideo.ItemStateProviderQueued {
		t.Fatalf("called=%v state=%s", called, store.itemSnapshot().State)
	}
}

func TestRunnerLocalUnknownSubmissionRestoresIdenticalInputs(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing=%t", missing), func(t *testing.T) {
			content := localFrame(t)
			var bodies [][]byte
			var keys []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				bodies = append(bodies, body)
				keys = append(keys, r.Header.Get("Idempotency-Key"))
				if len(bodies) == 1 {
					// Provider may have accepted the job before its connection closed.
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = conn.Close()
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"task_id":"recovered-local-job"}`)
			}))
			defer server.Close()
			provider, err := minimax.NewClient(minimax.Config{BaseURL: server.URL, APIKey: "test", ModelCode: "MiniMax-H3", Verified: true})
			if err != nil {
				t.Fatal(err)
			}
			backend := storage.NewLocalBackend(t.TempDir())
			if err := backend.Put(t.Context(), "frame.png", "image/png", content); err != nil {
				t.Fatal(err)
			}
			store := newMemoryStore(WorkItem{ID: "unknown-local", TaskID: "unknown-task", State: domainvideo.ItemStateQueued, Version: 1, ProviderCode: "minimax", Request: providervideo.Request{TaskType: "image_to_video", Prompt: "move", DurationSeconds: 5, Resolution: "768p", AspectRatio: "16:9", Inputs: []providervideo.Input{{AssetID: "frame", Role: "first_frame", MediaType: "image", StorageDriver: "local", ObjectKey: "frame.png", MIMEType: "image/png", SizeBytes: int64(len(content))}}}})
			now := time.Now()
			runner := NewRunner(store, staticProviderResolver{provider: provider}, storage.NewStaticRouter(backend), Options{Owner: "worker-one", Now: func() time.Time { return now }})
			if processed, err := runner.RunOnce(t.Context()); err != nil || !processed {
				t.Fatalf("initial RunOnce=%v,%v", processed, err)
			}
			item := store.itemSnapshot()
			if item.State != domainvideo.ItemStateReconciling || len(bodies) != 1 {
				t.Fatalf("initial state=%s calls=%d", item.State, len(bodies))
			}
			attempt := item.Attempt
			// Reclaim from persisted fields: Data is deliberately not available.
			item.Request.Inputs[0].Data = nil
			item.Request.Inputs[0].URL = ""
			reclaimed := newMemoryStore(item)
			now = now.Add(time.Hour)
			if missing {
				if err := backend.Delete(t.Context(), "frame.png"); err != nil {
					t.Fatal(err)
				}
			}
			runner = NewRunner(reclaimed, staticProviderResolver{provider: provider}, storage.NewStaticRouter(backend), Options{Owner: "worker-two", Now: func() time.Time { return now }})
			if processed, err := runner.RunOnce(t.Context()); err != nil || !processed {
				t.Fatalf("reclaim RunOnce=%v,%v", processed, err)
			}
			got := reclaimed.itemSnapshot()
			if reclaimed.prepareCalls != 0 || got.Attempt.ID != attempt.ID || got.Attempt.IdempotencyKey != attempt.IdempotencyKey {
				t.Fatal("reclaim created a new attempt/key")
			}
			if missing {
				if len(bodies) != 1 || got.State != domainvideo.ItemStateReconciling || got.ErrorCode != "provider_input_unavailable" {
					t.Fatalf("missing input state=%s error=%s calls=%d", got.State, got.ErrorCode, len(bodies))
				}
			} else if len(bodies) != 2 || !bytes.Equal(bodies[0], bodies[1]) || keys[0] == "" || keys[0] != keys[1] || got.State != domainvideo.ItemStateProviderQueued {
				t.Fatalf("replay changed request/key or state: calls=%d keys=%v state=%s", len(bodies), keys, got.State)
			}
		})
	}
}

func TestSignLocalImagesValidatesStorageAndMetadata(t *testing.T) {
	content := localFrame(t)
	backend := storage.NewLocalBackend(t.TempDir())
	if err := backend.Put(t.Context(), "frame.png", "image/png", content); err != nil {
		t.Fatal(err)
	}
	base := providervideo.Input{Role: "first_frame", MediaType: "image", StorageDriver: "local", ObjectKey: "frame.png", MIMEType: "image/png", SizeBytes: int64(len(content))}
	runner := NewRunner(nil, nil, storage.NewStaticRouter(backend), Options{})
	for _, tc := range []struct {
		name, provider string
		mutate         func(*providervideo.Input)
		wantErr        bool
	}{
		{"valid", "minimax", func(*providervideo.Input) {}, false},
		{"wrong MIME", "minimax", func(i *providervideo.Input) { i.MIMEType = "image/jpeg" }, true},
		{"wrong size", "minimax", func(i *providervideo.Input) { i.SizeBytes++ }, true},
		{"wrong checksum", "minimax", func(i *providervideo.Input) { i.SHA256 = strings.Repeat("a", 64) }, true},
		{"over limit", "minimax", func(i *providervideo.Input) { i.SizeBytes = 10<<20 + 1 }, true},
		{"missing", "minimax", func(i *providervideo.Input) { i.ObjectKey = "missing.png" }, true},
		{"audio", "minimax", func(i *providervideo.Input) { i.MediaType = "audio" }, true},
		{"video", "minimax", func(i *providervideo.Input) { i.MediaType = "video" }, true},
		{"URL only provider", "bailian", func(*providervideo.Input) {}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := base
			tc.mutate(&input)
			got, err := runner.signInputs(t.Context(), providervideo.Request{Inputs: []providervideo.Input{input}}, tc.provider)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, tc.wantErr)
			}
			if err == nil {
				if !bytes.Equal(got.Inputs[0].Data, content) || got.Inputs[0].URL != "" {
					t.Fatal("expected private bounded bytes without URL")
				}
				raw, _ := json.Marshal(got)
				logs := fmt.Sprint(videoRequestLogAttrs(WorkItem{}, got))
				if bytes.Contains(raw, []byte(base64.StdEncoding.EncodeToString(content))) || strings.Contains(logs, "data:image/") {
					t.Fatal("inline payload exposed")
				}
			}
		})
	}
	// Three valid 7 MiB images must still respect the 20 MiB request budget.
	padded := append(append([]byte(nil), content...), make([]byte, (7<<20)-len(content))...)
	if err := backend.Put(t.Context(), "padded.png", "image/png", padded); err != nil {
		t.Fatal(err)
	}
	aggregate := base
	aggregate.ObjectKey = "padded.png"
	aggregate.SizeBytes = int64(len(padded))
	if _, err := runner.signInputs(t.Context(), providervideo.Request{Inputs: []providervideo.Input{aggregate, aggregate, aggregate}}, "minimax"); err == nil {
		t.Fatal("aggregate inline image budget was not enforced")
	}
	// Even when metadata is absent the backend must enforce the limit before allocating the full object.
	if err := backend.Put(t.Context(), "large.png", "image/png", make([]byte, 10<<20+1)); err != nil {
		t.Fatal(err)
	}
	input := base
	input.ObjectKey = "large.png"
	input.SizeBytes = 0
	if _, err := runner.signInputs(t.Context(), providervideo.Request{Inputs: []providervideo.Input{input}}, "minimax"); err == nil {
		t.Fatal("untrusted oversized file accepted")
	}
}
