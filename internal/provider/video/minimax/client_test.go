package minimax_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	videoprovider "github.com/fatballfish/pic-gallery/internal/provider/video"
	"github.com/fatballfish/pic-gallery/internal/provider/video/minimax"
)

func TestClientSubmitGetCancelAndNormalizeUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mm-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/video_generation":
			if r.Header.Get("Idempotency-Key") != "idem-1" {
				t.Fatalf("idempotency header = %q", r.Header.Get("Idempotency-Key"))
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["model"] != "MiniMax-H3" || body["resolution"] != "2K" || body["ratio"] != "adaptive" || body["duration"] != float64(5) {
				t.Fatalf("submit body = %#v", body)
			}
			content, ok := body["content"].([]any)
			if !ok || len(content) != 3 {
				t.Fatalf("content = %#v", body["content"])
			}
			first := content[1].(map[string]any)
			last := content[2].(map[string]any)
			if first["role"] != "first_frame" || last["role"] != "last_frame" {
				t.Fatalf("frame roles = %#v %#v", first, last)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("x-request-id", "mm-submit-1")
			_, _ = io.WriteString(w, `{"task_id":"mm-job-1"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/query/video_generation/mm-job-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"task":{"id":"mm-job-1","model":"MiniMax-H3","status":"succeeded","content":{"url":"https://video.example.com/result.mp4"},"duration":5,"resolution":"2K","ratio":"16:9","usage":{"total_seconds":5,"input_seconds":0,"output_seconds":5,"input_image_count":2}}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v2/video_generation/mm-job-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"task_id":"mm-job-1","action":"cancelled","status":"cancelled"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := minimax.NewClient(minimax.Config{
		BaseURL: server.URL, APIKey: "mm-key", ModelCode: "MiniMax-H3",
		HTTPClient: server.Client(), Verified: true, CallbackSecret: "callback-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	job, err := client.Submit(t.Context(), videoRequest())
	if err != nil || job.ID != "mm-job-1" || job.RequestID != "mm-submit-1" {
		t.Fatalf("Submit() = %#v, %v", job, err)
	}
	status, err := client.Get(t.Context(), videoprovider.JobRef{ID: job.ID})
	if err != nil || status.State != videoprovider.StateSucceeded || len(status.Artifacts) != 1 {
		t.Fatalf("Get() = %#v, %v", status, err)
	}
	usage, err := client.NormalizeUsage(status)
	if err != nil || usage.OutputSeconds != "5.000" || usage.ReferenceImageCount != 2 {
		t.Fatalf("NormalizeUsage() = %#v, %v", usage, err)
	}
	cancelled, err := client.Cancel(t.Context(), videoprovider.JobRef{ID: job.ID})
	if err != nil || !cancelled.Accepted || cancelled.State != videoprovider.StateCancelled {
		t.Fatalf("Cancel() = %#v, %v", cancelled, err)
	}
}

func TestClientReconcilesSubmissionByReplayingTheSameIdempotencyKey(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Idempotency-Key") != "idem-1" {
			t.Fatalf("idempotency header = %q", r.Header.Get("Idempotency-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"task_id":"mm-job-stable"}`)
	}))
	defer server.Close()
	client, err := minimax.NewClient(minimax.Config{BaseURL: server.URL, APIKey: "key", ModelCode: "MiniMax-H3", Verified: true, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.Submit(t.Context(), videoRequest())
	if err != nil {
		t.Fatal(err)
	}
	recovered, found, err := client.Reconcile(t.Context(), videoRequest())
	if err != nil || !found || recovered.ID != first.ID || requests != 2 {
		t.Fatalf("reconcile = %#v found=%v requests=%d err=%v", recovered, found, requests, err)
	}
}

func TestClientRejectsUnknownOptionsAndUnverifiedConfig(t *testing.T) {
	if _, err := minimax.NewClient(minimax.Config{BaseURL: "https://api.example.com", APIKey: "key", ModelCode: "MiniMax-H3"}); err == nil {
		t.Fatal("expected unverified configuration to be rejected")
	}
	client, err := minimax.NewClient(minimax.Config{BaseURL: "https://api.example.com", APIKey: "key", ModelCode: "MiniMax-H3", Verified: true})
	if err != nil {
		t.Fatal(err)
	}
	req := videoRequest()
	req.ProviderOptions = map[string]any{"unknown": true}
	if _, err := client.Submit(t.Context(), req); err == nil {
		t.Fatal("expected unknown provider option to be rejected")
	}
}

func TestClientHonorsContextDeadline(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-release:
			return
		}
	}))
	t.Cleanup(func() {
		close(release)
		server.CloseClientConnections()
		server.Close()
	})
	client, err := minimax.NewClient(minimax.Config{BaseURL: server.URL, APIKey: "key", ModelCode: "MiniMax-H3", Verified: true, HTTPClient: server.Client(), Timeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Submit(context.Background(), videoRequest())
	providerErr, ok := videoprovider.AsError(err)
	if !ok || providerErr.Category != videoprovider.ErrorUnavailable || !providerErr.Retryable || !providerErr.SubmissionUnknown {
		t.Fatalf("Submit timeout error = %#v, %v", providerErr, err)
	}
}

func TestClientVerifiesChallengeAndSignedCallback(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	client, err := minimax.NewClient(minimax.Config{
		BaseURL: "https://api.example.com", APIKey: "key", ModelCode: "MiniMax-H3",
		Verified: true, CallbackSecret: "callback-secret", Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := client.VerifyCallback(t.Context(), http.Header{}, []byte(`{"challenge":"verify-me"}`))
	// Challenges have no event identity; the callback service must not persist them.
	if err != nil || challenge.Challenge != "verify-me" || challenge.EventID != "" || challenge.JobID != "" {
		t.Fatalf("challenge = %#v, %v", challenge, err)
	}
	body := []byte(`{"task":{"id":"mm-job-1","status":"running"}}`)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	headers := minimaxCallbackHeaders("callback-secret", timestamp, body)
	event, err := client.VerifyCallback(t.Context(), headers, body)
	if err != nil || event.JobID != "mm-job-1" || event.Status.State != videoprovider.StateRunning {
		t.Fatalf("callback = %#v, %v", event, err)
	}
}

func TestClientRejectsInvalidChallenge(t *testing.T) {
	client, err := minimax.NewClient(minimax.Config{BaseURL: "https://api.example.com", APIKey: "key", ModelCode: "MiniMax-H3", Verified: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range [][]byte{
		[]byte(`{"challenge":"   "}`),
		[]byte(`{"challenge":"` + strings.Repeat("x", 257) + `"}`),
	} {
		if _, err := client.VerifyCallback(t.Context(), http.Header{}, body); err == nil {
			t.Fatalf("expected invalid challenge to be rejected: %s", body)
		}
	}
}

func TestClientRejectsCallbackOutsideTimestampWindow(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	body := []byte(`{"task":{"id":"mm-job-1","status":"running"}}`)
	tests := []struct {
		name      string
		timestamp string
		headers   http.Header
	}{
		{name: "missing timestamp", headers: http.Header{"X-Minimax-Signature": []string{"00"}}},
		{name: "invalid timestamp", timestamp: "not-a-timestamp"},
		{name: "older than default tolerance", timestamp: strconv.FormatInt(now.Add(-5*time.Minute-time.Second).Unix(), 10)},
		{name: "future beyond default tolerance", timestamp: strconv.FormatInt(now.Add(5*time.Minute+time.Second).Unix(), 10)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := minimax.NewClient(minimax.Config{
				BaseURL: "https://api.example.com", APIKey: "key", ModelCode: "MiniMax-H3",
				Verified: true, CallbackSecret: "callback-secret", Now: func() time.Time { return now },
			})
			if err != nil {
				t.Fatal(err)
			}
			headers := test.headers
			if headers == nil {
				headers = minimaxCallbackHeaders("callback-secret", test.timestamp, body)
			}
			if _, err := client.VerifyCallback(t.Context(), headers, body); err == nil {
				t.Fatal("expected callback timestamp to be rejected")
			}
		})
	}
}

func TestClientUsesConfiguredCallbackTolerance(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	body := []byte(`{"task":{"id":"mm-job-1","status":"running"}}`)
	client, err := minimax.NewClient(minimax.Config{
		BaseURL: "https://api.example.com", APIKey: "key", ModelCode: "MiniMax-H3",
		Verified: true, CallbackSecret: "callback-secret", Now: func() time.Time { return now }, CallbackTolerance: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	timestamp := strconv.FormatInt(now.Add(-time.Minute-time.Second).Unix(), 10)
	if _, err := client.VerifyCallback(t.Context(), minimaxCallbackHeaders("callback-secret", timestamp, body), body); err == nil {
		t.Fatal("expected callback outside configured tolerance to be rejected")
	}
}

func minimaxCallbackHeaders(secret, timestamp string, body []byte) http.Header {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	return http.Header{
		"X-Minimax-Timestamp": []string{timestamp},
		"X-Minimax-Signature": []string{hex.EncodeToString(mac.Sum(nil))},
	}
}

func videoRequest() videoprovider.Request {
	return videoprovider.Request{
		TaskID: "task-1", ItemID: "item-1", AttemptID: "attempt-1", IdempotencyKey: "idem-1",
		TaskType: "first_last_frame_to_video", Prompt: "A child grows up", DurationSeconds: 5,
		Resolution: "2k", AspectRatio: "adaptive", OutputFormat: "mp4",
		Inputs: []videoprovider.Input{
			{AssetID: "first", Role: "first_frame", URL: "https://assets.example.com/first.png"},
			{AssetID: "last", Role: "last_frame", URL: "https://assets.example.com/last.png"},
		},
	}
}
