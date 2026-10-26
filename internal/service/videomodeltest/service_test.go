package videomodeltest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	videoprovider "github.com/fatballfish/pic-gallery/internal/provider/video"
	worker "github.com/fatballfish/pic-gallery/internal/worker/video"
)

type fakeLoader struct {
	account domainmodeladmin.ModelAccount
	model   domainmodeladmin.ModelAccountModel
}

func (f fakeLoader) GetModelAccount(context.Context, int64) (domainmodeladmin.ModelAccount, error) {
	return f.account, nil
}

func (f fakeLoader) GetModelAccountModel(context.Context, int64) (domainmodeladmin.ModelAccountModel, error) {
	return f.model, nil
}

type fakeProvider struct {
	status  videoprovider.Status
	request videoprovider.Request
}

func (f *fakeProvider) Submit(_ context.Context, req videoprovider.Request) (videoprovider.Job, error) {
	f.request = req
	return videoprovider.Job{ID: "job-1", State: videoprovider.StateQueued}, nil
}

func (f *fakeProvider) Get(context.Context, videoprovider.JobRef) (videoprovider.Status, error) {
	return f.status, nil
}

func (f *fakeProvider) Cancel(context.Context, videoprovider.JobRef) (videoprovider.CancelResult, error) {
	return videoprovider.CancelResult{}, errors.New("not implemented")
}

func (f *fakeProvider) VerifyCallback(context.Context, http.Header, []byte) (videoprovider.CallbackEvent, error) {
	return videoprovider.CallbackEvent{}, errors.New("not implemented")
}

func (f *fakeProvider) NormalizeUsage(videoprovider.Status) (videoprovider.Usage, error) {
	return videoprovider.Usage{}, nil
}

func TestSubmitPollsProviderUntilArtifactIsReady(t *testing.T) {
	loader := fakeLoader{
		account: domainmodeladmin.ModelAccount{
			ID: 1, AdapterType: "bailian", BaseURL: "https://example.test", TimeoutMS: 1000,
			CredentialsEncrypted: map[string]string{"api_key": "test-key"},
		},
		model: domainmodeladmin.ModelAccountModel{ID: 2, AccountID: 1, ModelCode: "wan-3.0", Enabled: true},
	}
	service := NewService(loader)
	service.validateArtifact = func(context.Context, videoprovider.Artifact, []string, *bool) error { return nil }
	service.pollInterval = 5 * time.Millisecond
	service.timeout = time.Second
	service.newProvider = func(domainmodeladmin.ModelAccount, domainmodeladmin.ModelAccountModel) (videoprovider.Provider, error) {
		return &fakeProvider{status: videoprovider.Status{
			JobID: "job-1", State: videoprovider.StateSucceeded,
			Artifacts: []videoprovider.Artifact{{URL: "https://cdn.example.test/result.mp4", MIMEType: "video/mp4", SizeBytes: 12}},
		}}, nil
	}

	test, err := service.Submit(context.Background(), Request{
		AccountID: 1, AccountModelID: 2, Prompt: "ocean", TaskType: "text_to_video",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		test, err = service.Get(test.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if test.Status == "succeeded" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if test.Status != "succeeded" {
		t.Fatalf("expected succeeded test, got %#v", test)
	}
	if len(test.Artifacts) != 1 || test.Artifacts[0].URL != "https://cdn.example.test/result.mp4" {
		t.Fatalf("expected artifact URL, got %#v", test.Artifacts)
	}
}

func TestSubmitPreservesProviderRequestAndSetsMP4(t *testing.T) {
	for _, defaults := range []bool{false, true} {
		name := "explicit parameters"
		if defaults {
			name = "default parameters"
		}
		t.Run(name, func(t *testing.T) {
			service := NewService(fakeLoader{
				account: domainmodeladmin.ModelAccount{ID: 1, CredentialsEncrypted: map[string]string{"api_key": "test-key"}},
				model:   domainmodeladmin.ModelAccountModel{ID: 2, AccountID: 1, Enabled: true},
			})
			provider := &fakeProvider{status: videoprovider.Status{JobID: "job-1", State: videoprovider.StateCancelled}}
			service.newProvider = func(domainmodeladmin.ModelAccount, domainmodeladmin.ModelAccountModel) (videoprovider.Provider, error) {
				return provider, nil
			}
			req := Request{
				AccountID: 1, AccountModelID: 2, Prompt: " ocean ", TaskType: " reference_to_video ",
				DurationSeconds: 8, Resolution: "720p", AspectRatio: "9:16", GenerateAudio: true,
				Inputs: []Input{
					{Role: " reference_video ", URL: " https://cdn.example.test/ref.mp4 ", MediaType: " video ", Ordinal: 2},
					{Role: "reference_audio", URL: "https://cdn.example.test/ref.mp3", MediaType: "audio", Ordinal: 3},
				},
			}
			duration, resolution, ratio := 8, "720p", "9:16"
			if defaults {
				req.DurationSeconds, req.Resolution, req.AspectRatio = 0, "", ""
				duration, resolution, ratio = 5, "480p", "16:9"
			}
			result, err := service.Submit(t.Context(), req)
			if err != nil {
				t.Fatalf("Submit: %v", err)
			}
			if result.ID == "" || result.ProviderJobID != "job-1" {
				t.Fatalf("unexpected test: %#v", result)
			}
			want := videoprovider.Request{
				TaskID: result.ID, ItemID: result.ID, AttemptID: result.ID, IdempotencyKey: "video-model-test-" + result.ID,
				TaskType: "reference_to_video", Prompt: "ocean", DurationSeconds: duration,
				Resolution: resolution, AspectRatio: ratio, GenerateAudio: true, OutputFormat: "mp4",
				Inputs: []videoprovider.Input{
					{AssetID: "https://cdn.example.test/ref.mp4", Role: "reference_video", URL: "https://cdn.example.test/ref.mp4", MediaType: "video", Ordinal: 2},
					{AssetID: "https://cdn.example.test/ref.mp3", Role: "reference_audio", URL: "https://cdn.example.test/ref.mp3", MediaType: "audio", Ordinal: 3},
				},
			}
			if !reflect.DeepEqual(provider.request, want) {
				t.Fatalf("provider request = %#v, want %#v", provider.request, want)
			}
		})
	}
}

func TestSubmitSeedanceSendsMP4Payload(t *testing.T) {
	type submission struct {
		body           map[string]any
		idempotencyKey string
		authorization  string
	}
	requests := make(chan submission, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/contents/generations/tasks":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode submission: %v", err)
				http.Error(w, "invalid JSON", http.StatusBadRequest)
				return
			}
			requests <- submission{body: body, idempotencyKey: r.Header.Get("Idempotency-Key"), authorization: r.Header.Get("Authorization")}
			_, _ = w.Write([]byte(`{"id":"ark-admin-test","status":"queued"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/contents/generations/tasks/ark-admin-test":
			_, _ = w.Write([]byte(`{"id":"ark-admin-test","status":"cancelled"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := NewService(fakeLoader{
		account: domainmodeladmin.ModelAccount{
			ID: 1, AdapterType: "seedance", BaseURL: server.URL, TimeoutMS: 1000,
			CredentialsEncrypted: map[string]string{"api_key": "fixture-key"},
		},
		model: domainmodeladmin.ModelAccountModel{ID: 2, AccountID: 1, ModelCode: "doubao-seedance-2-5-260628", Enabled: true},
	})
	service.timeout = time.Second
	result, err := service.Submit(t.Context(), Request{
		AccountID: 1, AccountModelID: 2, Prompt: " ocean ", TaskType: "text_to_video",
		DurationSeconds: 8, Resolution: "720p", AspectRatio: "9:16", GenerateAudio: true,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.ProviderJobID != "ark-admin-test" || result.Error != "" {
		t.Fatalf("expected successful upstream submission, got %#v", result)
	}
	select {
	case got := <-requests:
		want := map[string]any{
			"model": "doubao-seedance-2-5-260628", "duration": float64(8), "resolution": "720p",
			"ratio": "9:16", "generate_audio": true, "output_format": "mp4",
			"content": []any{map[string]any{"type": "text", "text": "ocean"}},
		}
		if !reflect.DeepEqual(got.body, want) {
			t.Errorf("Seedance payload = %#v, want %#v", got.body, want)
		}
		if got.idempotencyKey != "video-model-test-"+result.ID || got.authorization != "Bearer fixture-key" {
			t.Errorf("unexpected headers: idempotency=%q authorization=%q", got.idempotencyKey, got.authorization)
		}
	default:
		t.Fatal("Seedance submission did not reach the HTTP fixture")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		result, err = service.Get(result.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if result.Status == "cancelled" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected fixture polling to finish, got %#v", result)
}

func TestSubmitRejectsModelFromAnotherAccount(t *testing.T) {
	service := NewService(fakeLoader{
		account: domainmodeladmin.ModelAccount{ID: 1, AdapterType: "bailian", BaseURL: "https://example.test", CredentialsEncrypted: map[string]string{"api_key": "test-key"}},
		model:   domainmodeladmin.ModelAccountModel{ID: 2, AccountID: 99, ModelCode: "wan-3.0", Enabled: true},
	})
	_, err := service.Submit(context.Background(), Request{AccountID: 1, AccountModelID: 2, Prompt: "ocean", TaskType: "text_to_video"})
	if err == nil {
		t.Fatal("expected model/account mismatch error")
	}
}

func TestAdminVideoArtifactPolicyPropagationAndFailure(t *testing.T) {
	for _, tc := range []struct {
		name        string
		extra       map[string]any
		wantEnabled bool
		wantNil     bool
		wantHosts   int
	}{
		{"default", nil, false, true, 0},
		{"legacy", map[string]any{"video_artifact_hosts": []any{"cdn.example.com"}}, false, true, 1},
		{"disabled", map[string]any{"video_artifact_hosts_enabled": false, "video_artifact_hosts": []string{"cdn.example.com"}}, false, false, 1},
		{"enabled empty", map[string]any{"video_artifact_hosts_enabled": true}, true, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := NewService(fakeLoader{})
			called := false
			service.validateArtifact = func(_ context.Context, _ videoprovider.Artifact, hosts []string, enabled *bool) error {
				called = true
				if len(hosts) != tc.wantHosts || (enabled == nil) != tc.wantNil || (enabled != nil && *enabled != tc.wantEnabled) {
					t.Errorf("hosts=%v enabled=%v", hosts, enabled)
				}
				return errors.New("blocked artifact")
			}
			now := time.Now()
			service.store(&Test{ID: "test", CreatedAt: now, UpdatedAt: now})
			hosts, enabled := worker.ArtifactHostPolicyFromExtra(tc.extra)
			service.poll("test", &fakeProvider{status: videoprovider.Status{State: videoprovider.StateSucceeded, Artifacts: []videoprovider.Artifact{{URL: "https://cdn.example.com/video.mp4"}}}}, videoprovider.JobRef{}, hosts, enabled)
			result, _ := service.Get("test")
			if !called || result.Status != "failed" || len(result.Artifacts) != 0 || result.Error == "" {
				t.Fatalf("result=%#v called=%v", result, called)
			}
		})
	}
}
