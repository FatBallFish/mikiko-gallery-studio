package bailian_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	videoprovider "github.com/fatballfish/pic-gallery/internal/provider/video"
	"github.com/fatballfish/pic-gallery/internal/provider/video/bailian"
)

func videoRequest() videoprovider.Request {
	return videoprovider.Request{
		TaskID: "task-1", ItemID: "item-1", AttemptID: "attempt-1", IdempotencyKey: "idem-1",
		TaskType: "text_to_video", Prompt: "一只橘猫在草地上慢跑", DurationSeconds: 5,
		Resolution: "480p", AspectRatio: "16:9",
	}
}

func newClient(t *testing.T, server *httptest.Server) *bailian.Client {
	t.Helper()
	client, err := bailian.NewClient(bailian.Config{
		BaseURL: server.URL + "/api/v1", APIKey: "sk-bailian", ModelCode: "wan3.0-video",
		HTTPClient: server.Client(), Verified: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestClientSubmitTextToVideoPayload(t *testing.T) {
	var body map[string]any
	var asyncHeader, authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/services/aigc/video-generation/video-synthesis" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		asyncHeader = r.Header.Get("X-DashScope-Async")
		authHeader = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"output":{"task_id":"task-abc"},"request_id":"req-1"}`)
	}))
	defer server.Close()

	job, err := newClient(t, server).Submit(t.Context(), videoRequest())
	if err != nil {
		t.Fatal(err)
	}
	if job.ID != "task-abc" || job.State != videoprovider.StateQueued {
		t.Fatalf("job = %#v", job)
	}
	if asyncHeader != "enable" || authHeader != "Bearer sk-bailian" {
		t.Fatalf("headers async=%q auth=%q", asyncHeader, authHeader)
	}
	if body["model"] != "wan3.0-video" {
		t.Fatalf("model = %#v", body["model"])
	}
	input := body["input"].(map[string]any)
	if input["prompt"] != "一只橘猫在草地上慢跑" {
		t.Fatalf("prompt = %#v", input["prompt"])
	}
	if _, hasMedia := input["media"]; hasMedia {
		t.Fatalf("t2v must not send media")
	}
	params := body["parameters"].(map[string]any)
	if params["resolution"] != "480P" || params["ratio"] != "16:9" || params["duration"] != float64(5) || params["prompt_extend"] != false {
		t.Fatalf("parameters = %#v", params)
	}
}

func TestClientSubmitFrameInputs(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"output":{"task_id":"task-frames"}}`)
	}))
	defer server.Close()

	req := videoRequest()
	req.TaskType = "first_last_frame_to_video"
	req.Inputs = []videoprovider.Input{
		{AssetID: "first", Role: "first_frame", URL: "https://93.184.216.34/first.png", MIMEType: "image/png"},
		{AssetID: "last", Role: "last_frame", URL: "https://93.184.216.34/last.png", MIMEType: "image/png"},
	}
	if _, err := newClient(t, server).Submit(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	media := body["input"].(map[string]any)["media"].([]any)
	if len(media) != 2 {
		t.Fatalf("media = %#v", media)
	}
	first := media[0].(map[string]any)
	last := media[1].(map[string]any)
	if first["type"] != "first_frame" || last["type"] != "last_frame" {
		t.Fatalf("media types = %#v %#v", first, last)
	}
}

func TestClientSubmitReferenceVideoMediaConstruction(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"output":{"task_id":"task-r2v"}}`)
	}))
	defer server.Close()

	req := videoRequest()
	req.TaskType = "video_edit"
	req.Prompt = "编辑视频：把背景替换成雪原"
	req.Inputs = []videoprovider.Input{
		{AssetID: "clip", Role: "reference_video", URL: "https://93.184.216.34/clip.mp4", MIMEType: "video/mp4"},
	}
	if _, err := newClient(t, server).Submit(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	media := body["input"].(map[string]any)["media"].([]any)
	entry := media[0].(map[string]any)
	if entry["type"] != "reference_video" || entry["url"] != "https://93.184.216.34/clip.mp4" {
		t.Fatalf("media entry = %#v", entry)
	}
}

func TestClientRejectsPrivateInputAssetsFast(t *testing.T) {
	submits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		submits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"output":{"task_id":"task-x"}}`)
	}))
	defer server.Close()

	req := videoRequest()
	req.TaskType = "video_edit"
	req.Inputs = []videoprovider.Input{
		{AssetID: "clip", Role: "reference_video", URL: "http://127.0.0.1:9000/bucket/clip.mp4", MIMEType: "video/mp4"},
	}
	_, err := newClient(t, server).Submit(t.Context(), req)
	providerErr, ok := videoprovider.AsError(err)
	if !ok || !strings.Contains(providerErr.Message, "publicly reachable") {
		t.Fatalf("private asset should fail fast with an actionable error, got %v", err)
	}
	if submits != 0 {
		t.Fatalf("private asset must be rejected before any submit, submits=%d", submits)
	}
}

func TestClientGetMapsStatusesAndArtifact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/tasks/task-run":
			_, _ = io.WriteString(w, `{"output":{"task_id":"task-run","task_status":"RUNNING"},"usage":{"video_duration":5}}`)
		case "/api/v1/tasks/task-done":
			_, _ = io.WriteString(w, `{"output":{"task_id":"task-done","task_status":"SUCCEEDED","video_url":"https://dashscope-result.oss.aliyuncs.com/video.mp4"},"usage":{"video_duration":5}}`)
		case "/api/v1/tasks/task-bad":
			_, _ = io.WriteString(w, `{"output":{"task_id":"task-bad","task_status":"FAILED","code":"InvalidParameter","message":"bad input"}}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := newClient(t, server)

	running, err := client.Get(t.Context(), videoprovider.JobRef{ID: "task-run"})
	if err != nil || running.State != videoprovider.StateRunning {
		t.Fatalf("running = %#v, %v", running, err)
	}
	done, err := client.Get(t.Context(), videoprovider.JobRef{ID: "task-done"})
	if err != nil || done.State != videoprovider.StateSucceeded || len(done.Artifacts) != 1 {
		t.Fatalf("done = %#v, %v", done, err)
	}
	usage, err := client.NormalizeUsage(done)
	if err != nil || usage.OutputSeconds != "5.000" {
		t.Fatalf("usage = %#v, %v", usage, err)
	}
	failed, err := client.Get(t.Context(), videoprovider.JobRef{ID: "task-bad"})
	if err != nil || failed.State != videoprovider.StateFailed || failed.ErrorCode != "InvalidParameter" {
		t.Fatalf("failed = %#v, %v", failed, err)
	}
}

func TestClientCancelTask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/tasks/task-1/cancel" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"output":{"task_id":"task-1","task_status":"CANCELED"}}`)
	}))
	defer server.Close()
	result, err := newClient(t, server).Cancel(t.Context(), videoprovider.JobRef{ID: "task-1"})
	if err != nil || !result.Accepted || result.State != videoprovider.StateCancelled {
		t.Fatalf("cancel = %#v, %v", result, err)
	}
}

func TestClientRejectsUnknownRoleAndUnverifiedConfig(t *testing.T) {
	if _, err := bailian.NewClient(bailian.Config{BaseURL: "https://dashscope.aliyuncs.com", APIKey: "k", ModelCode: "m"}); err == nil {
		t.Fatal("expected unverified configuration to be rejected")
	}
	client, err := bailian.NewClient(bailian.Config{BaseURL: "https://dashscope.aliyuncs.com", APIKey: "k", ModelCode: "m", Verified: true})
	if err != nil {
		t.Fatal(err)
	}
	req := videoRequest()
	req.Inputs = []videoprovider.Input{{AssetID: "x", Role: "unknown_role", URL: "https://93.184.216.34/x.png"}}
	if _, err := client.Submit(t.Context(), req); err == nil {
		t.Fatal("expected unknown role to be rejected")
	}
	req2 := videoRequest()
	req2.ProviderOptions = map[string]any{"nope": true}
	if _, err := client.Submit(t.Context(), req2); err == nil {
		t.Fatal("expected unknown provider option to be rejected")
	}
}
