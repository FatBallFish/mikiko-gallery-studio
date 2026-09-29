package gasic_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	videoprovider "github.com/fatballfish/pic-gallery/internal/provider/video"
	"github.com/fatballfish/pic-gallery/internal/provider/video/gasic"
)

func videoRequest() videoprovider.Request {
	return videoprovider.Request{
		TaskID: "task-1", ItemID: "item-1", AttemptID: "attempt-1", IdempotencyKey: "idem-1",
		TaskType: "text_to_video", Prompt: "一只橘猫在草地上慢跑", DurationSeconds: 5,
		Resolution: "720p", AspectRatio: "16:9",
	}
}

func newClient(t *testing.T, server *httptest.Server) *gasic.Client {
	t.Helper()
	client, err := gasic.NewClient(gasic.Config{
		BaseURL: server.URL + "/v1", APIKey: "sk-gasic", ModelCode: "doubao-seedance-2-5-260628",
		HTTPClient: server.Client(), Verified: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestClientSubmitUsesSecondsAndMetadata(t *testing.T) {
	var body map[string]any
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/videos" {
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
		authHeader = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"task_abc","object":"video","status":"queued","progress":0}`)
	}))
	defer server.Close()

	job, err := newClient(t, server).Submit(t.Context(), videoRequest())
	if err != nil {
		t.Fatal(err)
	}
	if job.ID != "task_abc" || job.State != videoprovider.StateQueued {
		t.Fatalf("job = %#v", job)
	}
	if authHeader != "Bearer sk-gasic" {
		t.Fatalf("authorization = %q", authHeader)
	}
	if body["model"] != "doubao-seedance-2-5-260628" || body["seconds"] != float64(5) || body["duration"] != float64(5) {
		t.Fatalf("submit body = %#v", body)
	}
	metadata, ok := body["metadata"].(map[string]any)
	if !ok || metadata["resolution"] != "720p" || metadata["ratio"] != "16:9" || metadata["duration"] != float64(5) {
		t.Fatalf("metadata = %#v", body["metadata"])
	}
	if _, hasImages := body["images"]; hasImages {
		t.Fatalf("text-to-video must not send images: %#v", body)
	}
}

func TestClientGetPollsAndFetchesArtifacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/videos/task_abc":
			_, _ = io.WriteString(w, `{"id":"task_abc","status":"completed","progress":100,"seconds":5,"created_at":100,"completed_at":200}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tasks/task_abc/artifacts":
			_, _ = io.WriteString(w, `{"task_id":"task_abc","artifacts":[{"key":"video","type":"video","mime_type":"video/mp4","content_url":"https://g-aisc.xyz/v1/tasks/task_abc/artifacts/video/content?access=tok"}]}`)
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	status, err := newClient(t, server).Get(t.Context(), videoprovider.JobRef{ID: "task_abc"})
	if err != nil {
		t.Fatal(err)
	}
	if status.State != videoprovider.StateSucceeded || len(status.Artifacts) != 1 {
		t.Fatalf("status = %#v", status)
	}
	if status.Artifacts[0].URL != "https://g-aisc.xyz/v1/tasks/task_abc/artifacts/video/content?access=tok" {
		t.Fatalf("artifact url = %q", status.Artifacts[0].URL)
	}
	usage, err := newClient(t, server).NormalizeUsage(status)
	if err != nil || usage.OutputSeconds != "5.000" {
		t.Fatalf("usage = %#v, %v", usage, err)
	}
}

func TestClientGetMapsInProgressAndFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/videos/task_run" {
			_, _ = io.WriteString(w, `{"id":"task_run","status":"in_progress","progress":50}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"task_bad","status":"failed","error":{"code":"moderation","message":"内容审核未通过"}}`)
	}))
	defer server.Close()
	client := newClient(t, server)

	running, err := client.Get(t.Context(), videoprovider.JobRef{ID: "task_run"})
	if err != nil || running.State != videoprovider.StateRunning {
		t.Fatalf("running = %#v, %v", running, err)
	}
	failed, err := client.Get(t.Context(), videoprovider.JobRef{ID: "task_bad"})
	if err != nil || failed.State != videoprovider.StateFailed || failed.ErrorCode != "moderation" {
		t.Fatalf("failed = %#v, %v", failed, err)
	}
}

func TestClientCancelAcceptsLocallyAndCallbackUnsupported(t *testing.T) {
	client, err := gasic.NewClient(gasic.Config{BaseURL: "https://g-aisc.xyz", APIKey: "k", ModelCode: "m", Verified: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Cancel(t.Context(), videoprovider.JobRef{ID: "task_1"})
	if err != nil || !result.Accepted || result.State != videoprovider.StateCancelled {
		t.Fatalf("cancel = %#v, %v", result, err)
	}
	if _, err := client.VerifyCallback(t.Context(), http.Header{}, []byte(`{}`)); err == nil {
		t.Fatal("callbacks should be rejected")
	}
}

func TestClientSubmitsReferenceImagesAsDataURI(t *testing.T) {
	var body map[string]any
	image := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	defer image.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"task_img","status":"queued"}`)
	}))
	defer server.Close()

	req := videoRequest()
	req.TaskType = "image_to_video"
	req.Inputs = []videoprovider.Input{{AssetID: "frame", Role: "first_frame", URL: image.URL + "/frame.png", MIMEType: "image/png"}}
	if _, err := newClient(t, server).Submit(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	images, ok := body["images"].([]any)
	if !ok || len(images) != 1 {
		t.Fatalf("images = %#v", body["images"])
	}
	if !strings.HasPrefix(images[0].(string), "data:image/png;base64,") {
		t.Fatalf("private image should be inlined, got %v", images[0])
	}
	if body["image"] != images[0] {
		t.Fatalf("first image mirror missing: %#v", body["image"])
	}
}

func TestClientNormalizesBaseURLWithVersionSuffix(t *testing.T) {
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"task_x","status":"queued"}`)
	}))
	defer server.Close()
	client, err := gasic.NewClient(gasic.Config{BaseURL: server.URL + "/v1/", APIKey: "k", ModelCode: "m", Verified: true, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Submit(t.Context(), videoRequest()); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "/v1/videos" {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestClientRejectsUnverifiedConfigAndInvalidRequests(t *testing.T) {
	if _, err := gasic.NewClient(gasic.Config{BaseURL: "https://g-aisc.xyz", APIKey: "k", ModelCode: "m"}); err == nil {
		t.Fatal("expected unverified configuration to be rejected")
	}
	client, err := gasic.NewClient(gasic.Config{BaseURL: "https://g-aisc.xyz", APIKey: "k", ModelCode: "m", Verified: true})
	if err != nil {
		t.Fatal(err)
	}
	bad := videoRequest()
	bad.ProviderOptions = map[string]any{"unknown": true}
	if _, err := client.Submit(t.Context(), bad); err == nil {
		t.Fatal("expected unknown provider option to be rejected")
	}
	empty := videoRequest()
	empty.Prompt = " "
	if _, err := client.Submit(t.Context(), empty); err == nil {
		t.Fatal("expected empty prompt to be rejected")
	}
}

func TestClientSurfacesHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"message":"分组无权","code":"group_forbidden"}}`)
	}))
	defer server.Close()
	_, err := newClient(t, server).Submit(t.Context(), videoRequest())
	providerErr, ok := videoprovider.AsError(err)
	if !ok || providerErr.Category == videoprovider.ErrorUnavailable {
		t.Fatalf("expected classified http error, got %#v", err)
	}
	_ = time.Second
}
