package handlers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	compatservice "github.com/fatballfish/pic-gallery/internal/service/compat"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
)

func TestCompatLifecycleSSEFinalShape(t *testing.T) {
	a := &API{compat: compatservice.NewService(config.Config{})}
	task := domainimagetask.Task{ID: "saved-task", Status: domainimagetask.StatusSucceeded, Results: []provider.ImageResult{{URL: "https://storage.example/saved.png"}}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/images/generations", nil)
	a.finishCompat(w, r, openplatform.Principal{}, compatInput{Stream: true}, task)
	body := w.Body.String()
	for _, want := range []string{"event: task", "saved-task", "event: result", "data: [DONE]"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
	if strings.Contains(body, "partial_image") {
		t.Fatal(body)
	}
	if w.Header().Get("X-Stream-Protocol") != "platform-image-lifecycle-v1" {
		t.Fatal(w.Header())
	}
}
func TestCompatLifecycleSyncOpenAIShape(t *testing.T) {
	a := &API{compat: compatservice.NewService(config.Config{})}
	task := domainimagetask.Task{ID: "saved-task", Status: domainimagetask.StatusSucceeded, Results: []provider.ImageResult{{URL: "https://storage.example/saved.png", StorageConfigID: "private", ObjectKey: "private-key"}}}
	w := httptest.NewRecorder()
	a.finishCompat(w, httptest.NewRequest("POST", "/v1/images/generations", nil), openplatform.Principal{}, compatInput{}, task)
	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(body) != 2 || body["data"] == nil || strings.Contains(w.Body.String(), "private") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
func TestCompatLifecycleDisconnectDoesNotCancelTask(t *testing.T) {
	a := &API{compat: compatservice.NewService(config.Config{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequest("POST", "/v1/images/generations", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	a.finishCompat(w, r, openplatform.Principal{}, compatInput{}, domainimagetask.Task{ID: "saved", Status: domainimagetask.StatusRunning})
	if w.Body.Len() != 0 {
		t.Fatal(w.Body.String())
	}
}
func TestCompatMutationIDStableAcrossPayloadMutations(t *testing.T) {
	p := openplatform.Principal{UserID: 1, APIKeyID: "2"}
	first := compatMutationID(p, "same")
	if first != compatMutationID(p, "same") {
		t.Fatal("unstable key")
	}
	p.APIKeyID = "3"
	if first == compatMutationID(p, "same") {
		t.Fatal("key identities collided")
	}
}
