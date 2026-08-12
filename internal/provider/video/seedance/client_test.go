package seedance_test

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
	"testing"
	"time"

	videoprovider "github.com/fatballfish/pic-gallery/internal/provider/video"
	"github.com/fatballfish/pic-gallery/internal/provider/video/seedance"
)

func TestClientSubmitGetCancelAndNormalizeUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ark-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/contents/generations/tasks":
			if r.Header.Get("Idempotency-Key") != "idem-1" {
				t.Fatalf("idempotency header = %q", r.Header.Get("Idempotency-Key"))
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["model"] != "doubao-seedance-2-5-260628" || body["resolution"] != "720p" || body["duration"] != float64(5) || body["ratio"] != "16:9" {
				t.Fatalf("submit body = %#v", body)
			}
			if body["generate_audio"] != true || body["output_format"] != "mp4" {
				t.Fatalf("submit options = %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("x-request-id", "ark-submit-1")
			_, _ = io.WriteString(w, `{"id":"ark-job-1","status":"queued"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/contents/generations/tasks/ark-job-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"ark-job-1","status":"succeeded","content":{"video_url":"https://video.example.com/seedance.mp4"},"duration":5,"usage":{"completion_tokens":120,"total_tokens":120}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v3/contents/generations/tasks/ark-job-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"ark-job-1","status":"cancelled"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := seedance.NewClient(seedance.Config{BaseURL: server.URL, APIKey: "ark-key", ModelCode: "doubao-seedance-2-5-260628", HTTPClient: server.Client(), Verified: true, CallbackSecret: "callback-secret"})
	if err != nil {
		t.Fatal(err)
	}
	req := videoprovider.Request{
		TaskID: "task-1", ItemID: "item-1", AttemptID: "attempt-1", IdempotencyKey: "idem-1",
		TaskType: "text_to_video", Prompt: "A cinematic sunrise", DurationSeconds: 5,
		Resolution: "720p", AspectRatio: "16:9", GenerateAudio: true, OutputFormat: "mp4",
		ProviderOptions: map[string]any{"watermark": false},
	}
	job, err := client.Submit(t.Context(), req)
	if err != nil || job.ID != "ark-job-1" {
		t.Fatalf("Submit() = %#v, %v", job, err)
	}
	status, err := client.Get(t.Context(), videoprovider.JobRef{ID: job.ID})
	if err != nil || status.State != videoprovider.StateSucceeded || len(status.Artifacts) != 1 {
		t.Fatalf("Get() = %#v, %v", status, err)
	}
	usage, err := client.NormalizeUsage(status)
	if err != nil || usage.OutputSeconds != "5.000" || usage.ProviderTokens != "120" {
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
		_, _ = io.WriteString(w, `{"id":"ark-job-stable","status":"queued"}`)
	}))
	defer server.Close()
	client, err := seedance.NewClient(seedance.Config{BaseURL: server.URL, APIKey: "key", ModelCode: "doubao-seedance-2-5-260628", Verified: true, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	req := videoprovider.Request{TaskID: "task-1", ItemID: "item-1", AttemptID: "attempt-1", IdempotencyKey: "idem-1", TaskType: "text_to_video", Prompt: "move", DurationSeconds: 5, Resolution: "720p", AspectRatio: "16:9", OutputFormat: "mp4"}
	first, err := client.Submit(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	recovered, found, err := client.Reconcile(t.Context(), req)
	if err != nil || !found || recovered.ID != first.ID || requests != 2 {
		t.Fatalf("reconcile = %#v found=%v requests=%d err=%v", recovered, found, requests, err)
	}
}

func TestClientRejectsUnknownOptionsAndHonorsTimeout(t *testing.T) {
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
	client, err := seedance.NewClient(seedance.Config{BaseURL: server.URL, APIKey: "key", ModelCode: "doubao-seedance-2-0-260128", Verified: true, HTTPClient: server.Client(), Timeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	req := videoprovider.Request{TaskID: "t", ItemID: "i", AttemptID: "a", IdempotencyKey: "k", TaskType: "text_to_video", Prompt: "p", DurationSeconds: 5, Resolution: "720p", AspectRatio: "16:9", OutputFormat: "mp4"}
	req.ProviderOptions = map[string]any{"unknown": true}
	if _, err := client.Submit(t.Context(), req); err == nil {
		t.Fatal("expected unknown provider option error")
	}
	req.ProviderOptions = nil
	_, err = client.Submit(context.Background(), req)
	providerErr, ok := videoprovider.AsError(err)
	if !ok || providerErr.Category != videoprovider.ErrorUnavailable || !providerErr.SubmissionUnknown {
		t.Fatalf("timeout = %#v, %v", providerErr, err)
	}
}

func TestClientVerifiesSignedCallback(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	client, err := seedance.NewClient(seedance.Config{
		BaseURL: "https://api.example.com", APIKey: "key", ModelCode: "doubao-seedance-2-5-260628",
		Verified: true, CallbackSecret: "callback-secret", Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"id":"ark-job-1","status":"running"}`)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	headers := seedanceCallbackHeaders("callback-secret", timestamp, body)
	event, err := client.VerifyCallback(t.Context(), headers, body)
	if err != nil || event.JobID != "ark-job-1" || event.Status.State != videoprovider.StateRunning {
		t.Fatalf("callback = %#v, %v", event, err)
	}
}

func TestClientRejectsCallbackOutsideTimestampWindow(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	body := []byte(`{"id":"ark-job-1","status":"running"}`)
	tests := []struct {
		name      string
		timestamp string
		headers   http.Header
	}{
		{name: "missing timestamp", headers: http.Header{"X-Ark-Signature": []string{"00"}}},
		{name: "invalid timestamp", timestamp: "not-a-timestamp"},
		{name: "older than default tolerance", timestamp: strconv.FormatInt(now.Add(-5*time.Minute-time.Second).Unix(), 10)},
		{name: "future beyond default tolerance", timestamp: strconv.FormatInt(now.Add(5*time.Minute+time.Second).Unix(), 10)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := seedance.NewClient(seedance.Config{
				BaseURL: "https://api.example.com", APIKey: "key", ModelCode: "doubao-seedance-2-5-260628",
				Verified: true, CallbackSecret: "callback-secret", Now: func() time.Time { return now },
			})
			if err != nil {
				t.Fatal(err)
			}
			headers := test.headers
			if headers == nil {
				headers = seedanceCallbackHeaders("callback-secret", test.timestamp, body)
			}
			if _, err := client.VerifyCallback(t.Context(), headers, body); err == nil {
				t.Fatal("expected callback timestamp to be rejected")
			}
		})
	}
}

func TestClientUsesConfiguredCallbackTolerance(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	body := []byte(`{"id":"ark-job-1","status":"running"}`)
	client, err := seedance.NewClient(seedance.Config{
		BaseURL: "https://api.example.com", APIKey: "key", ModelCode: "doubao-seedance-2-5-260628",
		Verified: true, CallbackSecret: "callback-secret", Now: func() time.Time { return now }, CallbackTolerance: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	timestamp := strconv.FormatInt(now.Add(-time.Minute-time.Second).Unix(), 10)
	if _, err := client.VerifyCallback(t.Context(), seedanceCallbackHeaders("callback-secret", timestamp, body), body); err == nil {
		t.Fatal("expected callback outside configured tolerance to be rejected")
	}
}

func seedanceCallbackHeaders(secret, timestamp string, body []byte) http.Header {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	return http.Header{
		"X-Ark-Timestamp": []string{timestamp},
		"X-Ark-Signature": []string{hex.EncodeToString(mac.Sum(nil))},
	}
}
