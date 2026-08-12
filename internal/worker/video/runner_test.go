package video

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	providervideo "github.com/fatballfish/pic-gallery/internal/provider/video"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

func TestNewRunnerDefaultIdentifiersAreUniqueUUIDs(t *testing.T) {
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	runner := NewRunner(nil, nil, nil, Options{Now: func() time.Time { return now }})

	for name, generate := range map[string]func() string{
		"attempt": runner.options.AttemptID,
		"asset":   runner.options.AssetID,
	} {
		first := generate()
		second := generate()
		if _, err := uuid.Parse(first); err != nil {
			t.Errorf("%s id %q is not a UUID: %v", name, first, err)
		}
		if _, err := uuid.Parse(second); err != nil {
			t.Errorf("second %s id %q is not a UUID: %v", name, second, err)
		}
		if first == second {
			t.Errorf("%s ids must be unique, both were %q", name, first)
		}
	}
}

func TestNewRunnerUsesConfiguredArtifactHTTPClient(t *testing.T) {
	client := &http.Client{Timeout: time.Second}
	runner := NewRunner(nil, nil, nil, Options{HTTPClient: client})
	if runner.httpClient != client {
		t.Fatal("runner ignored configured artifact HTTP client")
	}
}

func TestRunnerCreatesOneAttemptAndReconcilesUnknownSubmission(t *testing.T) {
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	store := newMemoryStore(WorkItem{ID: "item-1", TaskID: "task-1", State: domainvideo.ItemStateQueued, Version: 1, PricePoints: "12.50000"})
	provider := &providerStub{submitErr: &providervideo.Error{SubmissionUnknown: true, Retryable: true}}
	runner := newTestRunner(store, provider, now)

	processed, err := runner.RunOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("first step processed=%v err=%v", processed, err)
	}
	item := store.itemSnapshot()
	if store.prepareCalls != 1 || item.State != domainvideo.ItemStateReconciling || item.Attempt.No != 1 || item.Attempt.ID == "" || item.Attempt.IdempotencyKey == "" {
		t.Fatalf("unknown submit item=%#v prepare_calls=%d", item, store.prepareCalls)
	}
	if provider.submitCalls != 1 {
		t.Fatalf("submit calls=%d", provider.submitCalls)
	}

	provider.reconcileJob = providervideo.Job{ID: "provider-job-1", State: providervideo.StateQueued}
	provider.reconcileFound = true
	store.makeDue(now)
	processed, err = runner.RunOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("reconcile step processed=%v err=%v", processed, err)
	}
	item = store.itemSnapshot()
	if store.prepareCalls != 1 || provider.submitCalls != 1 || provider.reconcileCalls != 1 || item.State != domainvideo.ItemStateProviderQueued || item.Attempt.JobID != "provider-job-1" {
		t.Fatalf("reconciled item=%#v submit=%d reconcile=%d prepare=%d", item, provider.submitCalls, provider.reconcileCalls, store.prepareCalls)
	}
	if item.LeaseOwner != "" || item.NextActionAt == nil || !item.NextActionAt.After(now) {
		t.Fatalf("step lease/backoff not released: %#v", item)
	}
}

func TestRunnerReconcilesPersistedSubmittingAttemptWithoutResubmitting(t *testing.T) {
	now := time.Date(2026, 8, 12, 8, 30, 0, 0, time.UTC)
	store := newMemoryStore(WorkItem{
		ID: "item-crash", TaskID: "task-crash", State: domainvideo.ItemStateSubmitting, Version: 2,
		Attempt: Attempt{ID: "attempt-crash", No: 1, ProviderCode: "fake", IdempotencyKey: "task-crash:item-crash:attempt-crash", Status: "submitting"},
	})
	provider := &providerStub{reconcileJob: providervideo.Job{ID: "provider-job-crash", State: providervideo.StateQueued}, reconcileFound: true}
	runner := newTestRunner(store, provider, now)

	processed, err := runner.RunOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("RunOnce() processed=%v err=%v", processed, err)
	}
	item := store.itemSnapshot()
	if store.prepareCalls != 0 || provider.submitCalls != 0 || provider.reconcileCalls != 1 {
		t.Fatalf("crash recovery prepare=%d submit=%d reconcile=%d", store.prepareCalls, provider.submitCalls, provider.reconcileCalls)
	}
	if item.State != domainvideo.ItemStateProviderQueued || item.Attempt.ID != "attempt-crash" || item.Attempt.JobID != "provider-job-crash" {
		t.Fatalf("recovered item = %#v", item)
	}
}

func TestRunnerClaimsOnlyDueItemsAndPollsWithoutHoldingLease(t *testing.T) {
	now := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	future := now.Add(time.Minute)
	store := newMemoryStore(WorkItem{ID: "item-poll", TaskID: "task-poll", State: domainvideo.ItemStateProviderQueued, Version: 4, NextActionAt: &future, Attempt: Attempt{ID: "attempt-1", No: 1, JobID: "job-1", ProviderCode: "fake"}})
	provider := &providerStub{getStatus: providervideo.Status{JobID: "job-1", State: providervideo.StateRunning}}
	runner := newTestRunner(store, provider, now)

	processed, err := runner.RunOnce(t.Context())
	if err != nil || processed || provider.getCalls != 0 {
		t.Fatalf("future item processed=%v get_calls=%d err=%v", processed, provider.getCalls, err)
	}
	store.makeDue(now)
	processed, err = runner.RunOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("due poll processed=%v err=%v", processed, err)
	}
	item := store.itemSnapshot()
	if item.State != domainvideo.ItemStateProviderRunning || item.LeaseOwner != "" || item.NextActionAt == nil || !item.NextActionAt.After(now) {
		t.Fatalf("polled item=%#v", item)
	}
}

func TestRunnerPersistsProviderStatusAndNormalizedUsage(t *testing.T) {
	now := time.Date(2026, 8, 12, 9, 15, 0, 0, time.UTC)
	store := newMemoryStore(WorkItem{ID: "item-usage", TaskID: "task-usage", State: domainvideo.ItemStateProviderRunning, Version: 3, Attempt: Attempt{ID: "attempt-usage", No: 1, JobID: "job-usage", ProviderCode: "fake"}})
	provider := &providerStub{getStatus: providervideo.Status{
		JobID: "job-usage", State: providervideo.StateSucceeded,
		Artifacts: []providervideo.Artifact{{URL: "https://cdn.example.test/video.mp4", MIMEType: "video/mp4"}},
		Usage:     map[string]any{"output_seconds": 5, "total_tokens": 1200}, Raw: map[string]any{"request_id": "provider-request"},
	}}
	runner := newTestRunner(store, provider, now)

	processed, err := runner.RunOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("RunOnce() processed=%v err=%v", processed, err)
	}
	if store.lastApply.ProviderStatusSnapshot["request_id"] != "provider-request" || store.lastApply.UsageRaw["total_tokens"] != 1200 {
		t.Fatalf("provider snapshots = status %#v usage %#v", store.lastApply.ProviderStatusSnapshot, store.lastApply.UsageRaw)
	}
	if store.lastApply.UsageNormalized.OutputSeconds != "5.000" || store.lastApply.UsageNormalized.ProviderTokens != "1200" {
		t.Fatalf("normalized usage = %#v", store.lastApply.UsageNormalized)
	}
}

func TestRunnerReportsCommittedStageArtifactAndSettlementMetrics(t *testing.T) {
	now := time.Date(2026, 8, 12, 9, 30, 0, 0, time.UTC)
	observer := &videoObserverSpy{}
	store := newMemoryStore(WorkItem{ID: "item-stage", TaskID: "task-stage", State: domainvideo.ItemStateProviderQueued, Version: 1, Attempt: Attempt{ID: "attempt-stage", No: 1, JobID: "job-stage", ProviderCode: "fake"}})
	provider := &providerStub{getStatus: providervideo.Status{JobID: "job-stage", State: providervideo.StateRunning}}
	runner := newTestRunner(store, provider, now)
	runner.options.Observer = observer
	if processed, err := runner.RunOnce(t.Context()); err != nil || !processed {
		t.Fatalf("stage run processed=%v err=%v", processed, err)
	}
	if len(observer.stages) != 1 || observer.stages[0] != "provider_running:success" {
		t.Fatalf("stage observations = %#v", observer.stages)
	}

	store = newMemoryStore(WorkItem{ID: "settle-item", TaskID: "settle-task", State: domainvideo.ItemStateSucceeded, Version: 1, NeedsSettlement: true})
	store.settlement = SettlementSnapshot{TaskID: "settle-task", ReservedPoints: "1.00000", Items: []SettlementItem{{State: domainvideo.ItemStateSucceeded, PricePoints: "1.00000"}}}
	runner = newTestRunner(store, provider, now)
	runner.options.Observer = observer
	if processed, err := runner.RunOnce(t.Context()); err != nil || !processed {
		t.Fatalf("settlement run processed=%v err=%v", processed, err)
	}
	if len(observer.settlements) != 1 || observer.settlements[0] != "video:success" {
		t.Fatalf("settlement observations = %#v", observer.settlements)
	}
}

type videoObserverSpy struct {
	stages      []string
	artifacts   []string
	settlements []string
}

func (spy *videoObserverSpy) RecordVideoStage(stage, result string) {
	spy.stages = append(spy.stages, stage+":"+result)
}

func (spy *videoObserverSpy) RecordArtifactTransfer(mediaType, result string, bytes int64) {
	spy.artifacts = append(spy.artifacts, mediaType+":"+result+":"+fmt.Sprint(bytes))
}

func (spy *videoObserverSpy) RecordSettlement(kind, result string) {
	spy.settlements = append(spy.settlements, kind+":"+result)
}

func TestRunnerStopsNewClaimsWhenConcurrencyGateIsUnavailable(t *testing.T) {
	var gateCalls int
	store := newMemoryStore(WorkItem{ID: "must-not-claim", State: domainvideo.ItemStateQueued})
	runner := NewRunner(store, nil, nil, Options{ClaimAllowed: func(context.Context) (bool, error) {
		gateCalls++
		return false, nil
	}})
	processed, err := runner.RunOnce(t.Context())
	if err != nil || processed || gateCalls != 1 || store.itemSnapshot().LeaseOwner != "" {
		t.Fatalf("closed gate processed=%v err=%v calls=%d item=%#v", processed, err, gateCalls, store.itemSnapshot())
	}
}

func TestRunnerIgnoresStalePollAfterCallbackWins(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	store := newMemoryStore(WorkItem{ID: "item-race", TaskID: "task-race", State: domainvideo.ItemStateProviderQueued, Version: 7, Attempt: Attempt{ID: "attempt-race", No: 1, JobID: "job-race", ProviderCode: "fake"}})
	store.beforeApply = func(s *memoryStore) {
		s.item.State = domainvideo.ItemStateArtifactPending
		s.item.Version++
		s.beforeApply = nil
	}
	provider := &providerStub{getStatus: providervideo.Status{JobID: "job-race", State: providervideo.StateRunning}}
	runner := newTestRunner(store, provider, now)

	processed, err := runner.RunOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("race processed=%v err=%v", processed, err)
	}
	item := store.itemSnapshot()
	if item.State != domainvideo.ItemStateArtifactPending || item.Version != 8 {
		t.Fatalf("stale poll regressed callback state: %#v", item)
	}
}

func TestRunnerTreatsProviderSuccessAfterCancelAsArtifactPending(t *testing.T) {
	now := time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC)
	artifact := providervideo.Artifact{URL: "https://media.example.test/result.mp4", MIMEType: "video/mp4", SizeBytes: 10}
	store := newMemoryStore(WorkItem{ID: "item-cancel", TaskID: "task-cancel", State: domainvideo.ItemStateCancelRequested, Version: 3, Attempt: Attempt{ID: "attempt-cancel", No: 1, JobID: "job-cancel", ProviderCode: "fake"}})
	provider := &providerStub{
		cancelResult: providervideo.CancelResult{Accepted: false, State: providervideo.StateSucceeded},
		getStatus:    providervideo.Status{JobID: "job-cancel", State: providervideo.StateSucceeded, Artifacts: []providervideo.Artifact{artifact}},
	}
	runner := newTestRunner(store, provider, now)

	processed, err := runner.RunOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("late cancel processed=%v err=%v", processed, err)
	}
	item := store.itemSnapshot()
	if item.State != domainvideo.ItemStateArtifactPending || item.Artifact.URL != artifact.URL || provider.getCalls != 1 {
		t.Fatalf("late cancel item=%#v get_calls=%d", item, provider.getCalls)
	}
}

func TestRunnerStreamsValidatedArtifactAndCommitsReadyOriginal(t *testing.T) {
	payload := bytes.Repeat([]byte("video"), 1024)
	digest := sha256.Sum256(payload)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "5120")
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	store := newMemoryStore(WorkItem{
		ID: "item-artifact", TaskID: "task-artifact", UserID: 7, ProjectID: "project-1", State: domainvideo.ItemStateArtifactPending, Version: 5,
		PricePoints: "9.25000", Attempt: Attempt{ID: "attempt-artifact", No: 1, ProviderCode: "fake"},
		Artifact: providervideo.Artifact{URL: server.URL + "/result.mp4", MIMEType: "video/mp4", SizeBytes: int64(len(payload)), SHA256: hex.EncodeToString(digest[:])},
	})
	backend := &streamingBackend{objects: map[string][]byte{}}
	runner := newTestRunner(store, &providerStub{}, now)
	runner.storage = storage.NewStaticRouter(backend)
	runner.httpClient = server.Client()
	runner.options.ArtifactAllowedHosts = []string{server.Listener.Addr().String()}
	runner.options.AllowLoopbackArtifactHosts = true
	runner.options.ResolveHostIPs = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}

	processed, err := runner.RunOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("artifact processed=%v err=%v", processed, err)
	}
	item := store.itemSnapshot()
	if item.State != domainvideo.ItemStateSucceeded || item.ResultAssetID == "" || item.ActualPoints != "9.25000" {
		t.Fatalf("artifact item=%#v", item)
	}
	if backend.putReaderCalls != 1 || backend.putCalls != 0 || !bytes.Equal(backend.objects[store.lastArtifact.ObjectKey], payload) {
		t.Fatalf("artifact was not streamed: put_reader=%d put=%d objects=%#v", backend.putReaderCalls, backend.putCalls, backend.objects)
	}
	if store.lastArtifact.Status != "ready_original" || store.lastArtifact.SHA256 != hex.EncodeToString(digest[:]) || store.lastArtifact.SizeBytes != int64(len(payload)) {
		t.Fatalf("artifact commit=%#v", store.lastArtifact)
	}
}

func TestRunnerArtifactDownloadTimesOutAfterResponseHeaders(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-release
	}))
	defer func() { close(release); server.Close() }()
	runner := NewRunner(nil, nil, nil, Options{
		HTTPClient: server.Client(), ArtifactTransferTimeout: 50 * time.Millisecond,
		ArtifactMaxBytes: 1024, ArtifactAllowedHosts: []string{server.Listener.Addr().String()},
		AllowLoopbackArtifactHosts: true,
		ResolveHostIPs: func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		},
	})
	started := time.Now()
	_, _, _, _, err := runner.downloadArtifact(t.Context(), providervideo.Artifact{URL: server.URL + "/result.mp4"}, nil)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("download error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("artifact timeout took too long: %v", elapsed)
	}
}

func TestRunnerRejectsArtifactRedirectHostSizeAndChecksum(t *testing.T) {
	payload := []byte("video-content")
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer other.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) }))
	defer redirect.Close()
	tests := []struct {
		name     string
		artifact providervideo.Artifact
		server   *httptest.Server
	}{
		{name: "redirect host", artifact: providervideo.Artifact{URL: redirect.URL, SizeBytes: int64(len(payload))}, server: redirect},
		{name: "declared size", artifact: providervideo.Artifact{URL: other.URL, SizeBytes: int64(len(payload)) + 1}, server: other},
		{name: "checksum", artifact: providervideo.Artifact{URL: other.URL, SizeBytes: int64(len(payload)), SHA256: hex.EncodeToString(make([]byte, sha256.Size))}, server: other},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newMemoryStore(WorkItem{ID: "item-bad", TaskID: "task-bad", State: domainvideo.ItemStateArtifactPending, Version: 2, MaxArtifactAttempts: 1, Attempt: Attempt{ID: "attempt-bad", No: 1}, Artifact: test.artifact})
			backend := &streamingBackend{objects: map[string][]byte{}}
			runner := newTestRunner(store, &providerStub{}, time.Now().UTC())
			runner.storage = storage.NewStaticRouter(backend)
			runner.httpClient = test.server.Client()
			runner.options.ArtifactAllowedHosts = []string{redirect.Listener.Addr().String()}
			runner.options.AllowLoopbackArtifactHosts = true
			runner.options.ResolveHostIPs = func(context.Context, string) ([]net.IP, error) {
				return []net.IP{net.ParseIP("127.0.0.1")}, nil
			}
			if test.name != "redirect host" {
				runner.options.ArtifactAllowedHosts = []string{other.Listener.Addr().String()}
			}

			processed, err := runner.RunOnce(t.Context())
			if err != nil || !processed {
				t.Fatalf("invalid artifact processed=%v err=%v", processed, err)
			}
			item := store.itemSnapshot()
			if item.State != domainvideo.ItemStateFailed || !item.Attempt.PlatformAbsorbed || item.ResultAssetID != "" {
				t.Fatalf("invalid artifact item=%#v", item)
			}
			if len(backend.objects) != 0 {
				t.Fatalf("invalid artifact left stored object: %#v", backend.objects)
			}
		})
	}
}

func TestRunnerRejectsAllowlistedArtifactHostResolvingToPrivateAddress(t *testing.T) {
	runner := newTestRunner(newMemoryStore(WorkItem{}), &providerStub{}, time.Now().UTC())
	runner.options.ArtifactAllowedHosts = []string{"cdn.provider.example"}
	runner.options.ResolveHostIPs = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("169.254.169.254")}, nil
	}
	if runner.allowedArtifactHost(t.Context(), "cdn.provider.example", nil) {
		t.Fatal("metadata address must be rejected even when host is allowlisted")
	}
	runner.options.ResolveHostIPs = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	}
	if !runner.allowedArtifactHost(t.Context(), "cdn.provider.example", nil) {
		t.Fatal("globally routable allowlisted address should be accepted")
	}
}

func TestRunnerPinsArtifactConnectionToTheValidatedDNSAddress(t *testing.T) {
	payload := []byte("video-content")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port := serverURL.Port()
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	transport.TLSClientConfig.InsecureSkipVerify = true // The test isolates address pinning from certificate hostname validation.
	transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}
	runner := NewRunner(nil, nil, nil, Options{
		HTTPClient: &http.Client{Transport: transport}, ArtifactTransferTimeout: 100 * time.Millisecond,
		ArtifactAllowedHosts: []string{"artifact.example.test:" + port},
		ResolveHostIPs: func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("8.8.8.8")}, nil
		},
	})
	_, _, _, _, err = runner.downloadArtifact(t.Context(), providervideo.Artifact{URL: "https://artifact.example.test:" + port + "/result.mp4"}, nil)
	if err == nil {
		t.Fatal("artifact download followed a second DNS/dial path instead of the validated address")
	}
}

func TestRunnerRejectsNonPublicArtifactAddressRanges(t *testing.T) {
	runner := newTestRunner(newMemoryStore(WorkItem{}), &providerStub{}, time.Now().UTC())
	runner.options.ArtifactAllowedHosts = []string{"cdn.provider.example"}
	for _, raw := range []string{"100.64.0.1", "198.18.0.1", "192.0.2.1", "198.51.100.1", "203.0.113.1", "2001:db8::1"} {
		runner.options.ResolveHostIPs = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP(raw)}, nil }
		if runner.allowedArtifactHost(t.Context(), "cdn.provider.example", nil) {
			t.Fatalf("non-public artifact address %s must be rejected", raw)
		}
	}
}

func TestRunnerPersistsArtifactRetriesBeforeExhaustion(t *testing.T) {
	now := time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC)
	store := newMemoryStore(WorkItem{
		ID: "item-retry", TaskID: "task-retry", State: domainvideo.ItemStateArtifactPending, Version: 2,
		MaxArtifactAttempts: 2, Attempt: Attempt{ID: "attempt-retry", No: 1},
		Artifact: providervideo.Artifact{URL: "http://media.example.test/result.mp4"},
	})
	runner := newTestRunner(store, &providerStub{}, now)

	processed, err := runner.RunOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("first artifact attempt processed=%v err=%v", processed, err)
	}
	item := store.itemSnapshot()
	if item.State != domainvideo.ItemStateRecoveryRequired || item.ArtifactAttempts != 1 || item.Attempt.PlatformAbsorbed {
		t.Fatalf("recoverable artifact failure item=%#v", item)
	}

	store.makeDue(now)
	processed, err = runner.RunOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("exhausted artifact attempt processed=%v err=%v", processed, err)
	}
	item = store.itemSnapshot()
	if item.State != domainvideo.ItemStateFailed || item.ArtifactAttempts != 2 || !item.Attempt.PlatformAbsorbed {
		t.Fatalf("exhausted artifact failure item=%#v", item)
	}
}

func TestRunnerFinalizesZeroAndPartialSuccessExactlyOnce(t *testing.T) {
	tests := []struct {
		name       string
		items      []SettlementItem
		wantStatus domainvideo.TaskStatus
		wantCount  int
		wantPoints string
	}{
		{name: "zero success", items: []SettlementItem{{State: domainvideo.ItemStateFailed, PricePoints: "10.00000"}, {State: domainvideo.ItemStateCancelled, PricePoints: "10.00000"}}, wantStatus: domainvideo.TaskStatusFailed, wantPoints: "0.00000"},
		{name: "partial success", items: []SettlementItem{{State: domainvideo.ItemStateSucceeded, PricePoints: "10.25000"}, {State: domainvideo.ItemStateFailed, PricePoints: "8.00000"}, {State: domainvideo.ItemStateSucceeded, PricePoints: "3.75000"}}, wantStatus: domainvideo.TaskStatusPartial, wantCount: 2, wantPoints: "14.00000"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newMemoryStore(WorkItem{ID: "settle-item", TaskID: "settle-task", State: domainvideo.ItemStateFailed, Version: 3, NeedsSettlement: true})
			store.settlement = SettlementSnapshot{TaskID: "settle-task", ReservedPoints: "25.00000", Items: test.items}
			now := time.Now().UTC()
			runner := newTestRunner(store, &providerStub{}, now)
			for i := 0; i < 2; i++ {
				store.makeDue(now)
				processed, err := runner.RunOnce(t.Context())
				if err != nil || !processed {
					t.Fatalf("settlement pass %d processed=%v err=%v", i, processed, err)
				}
			}
			if store.finalizeCalls != 1 || store.lastFinalize.Status != test.wantStatus || store.lastFinalize.SuccessOutputCount != test.wantCount || store.lastFinalize.ActualPoints != test.wantPoints {
				t.Fatalf("finalize calls=%d request=%#v", store.finalizeCalls, store.lastFinalize)
			}
		})
	}
}

func TestRunnerDoesNotFinalizeWhileArtifactIsPending(t *testing.T) {
	store := newMemoryStore(WorkItem{ID: "settle-pending", TaskID: "task-pending", State: domainvideo.ItemStateArtifactPending, Version: 2, NeedsSettlement: true})
	store.settlement = SettlementSnapshot{TaskID: "task-pending", ReservedPoints: "10.00000", Items: []SettlementItem{{State: domainvideo.ItemStateArtifactPending, PricePoints: "10.00000"}}}
	runner := newTestRunner(store, &providerStub{}, time.Now().UTC())
	processed, err := runner.RunOnce(t.Context())
	if err != nil || !processed || store.finalizeCalls != 0 {
		t.Fatalf("pending settlement processed=%v finalize_calls=%d err=%v", processed, store.finalizeCalls, err)
	}
}

func TestRunnerDoesNotFinalizeMeteredItemWhileUsageIsPending(t *testing.T) {
	store := newMemoryStore(WorkItem{ID: "settle-metered", TaskID: "task-metered", State: domainvideo.ItemStateSucceeded, Version: 2, NeedsSettlement: true})
	store.settlement = SettlementSnapshot{TaskID: "task-metered", ReservedPoints: "10.00000", Items: []SettlementItem{{State: domainvideo.ItemStateSucceeded, UsagePending: true}}}
	runner := newTestRunner(store, &providerStub{}, time.Now().UTC())
	processed, err := runner.RunOnce(t.Context())
	if err != nil || !processed || store.finalizeCalls != 0 {
		t.Fatalf("pending usage processed=%v finalize_calls=%d err=%v", processed, store.finalizeCalls, err)
	}
}

func TestRunnerSignsPrivateInputImmediatelyBeforeSubmit(t *testing.T) {
	now := time.Date(2026, 8, 12, 16, 0, 0, 0, time.UTC)
	item := WorkItem{ID: "input-item", TaskID: "input-task", State: domainvideo.ItemStateQueued, Version: 1, Request: providervideo.Request{
		Inputs: []providervideo.Input{{AssetID: "asset-1", Role: "first_frame", StorageConfigID: "storage-1", StorageDriver: "s3", ObjectKey: "media/original/input.png", MIMEType: "image/png"}},
	}}
	store := newMemoryStore(item)
	provider := &providerStub{submitJob: providervideo.Job{ID: "job-input", State: providervideo.StateQueued}}
	backend := &inputSigningBackend{streamingBackend: streamingBackend{objects: map[string][]byte{}}}
	router := &inputRouter{ref: storage.BackendRef{ConfigID: "storage-1", Driver: "s3", Backend: backend}}
	runner := NewRunner(store, staticProviderResolver{provider: provider}, router, Options{Owner: "video-worker", Now: func() time.Time { return now }, AttemptID: func() string { return "attempt-input" }})

	processed, err := runner.RunOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("RunOnce() processed=%v err=%v", processed, err)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.submitCalls != 1 || len(provider.lastSubmit.Inputs) != 1 || provider.lastSubmit.Inputs[0].URL != "https://media.example.test/media/original/input.png?signature=short" {
		t.Fatalf("provider submit request = %#v calls=%d", provider.lastSubmit, provider.submitCalls)
	}
}

func TestRunnerDoesNotSubmitPrivateInputWhenStorageCannotSign(t *testing.T) {
	now := time.Date(2026, 8, 12, 16, 0, 0, 0, time.UTC)
	item := WorkItem{ID: "local-item", TaskID: "local-task", State: domainvideo.ItemStateQueued, Version: 1, Request: providervideo.Request{
		Inputs: []providervideo.Input{{AssetID: "asset-local", Role: "first_frame", StorageDriver: "local", ObjectKey: "media/original/local.png", MIMEType: "image/png"}},
	}}
	store := newMemoryStore(item)
	provider := &providerStub{}
	runner := NewRunner(store, staticProviderResolver{provider: provider}, storage.NewStaticRouter(&streamingBackend{objects: map[string][]byte{}}), Options{Owner: "video-worker", Now: func() time.Time { return now }, AttemptID: func() string { return "attempt-local" }})

	processed, err := runner.RunOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("RunOnce() processed=%v err=%v", processed, err)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.submitCalls != 0 {
		t.Fatalf("provider submit calls = %d", provider.submitCalls)
	}
	if got := store.itemSnapshot(); got.State != domainvideo.ItemStateFailed || got.ErrorCode != "provider_input_unavailable" {
		t.Fatalf("failed item = %#v", got)
	}
}

func TestRunnerResolvesTheExactAccountSelectedByTheAttempt(t *testing.T) {
	now := time.Date(2026, 8, 12, 16, 30, 0, 0, time.UTC)
	item := WorkItem{ID: "account-item", TaskID: "account-task", State: domainvideo.ItemStateProviderQueued, Version: 2,
		Attempt: Attempt{ID: "attempt-account", RouteCandidateID: 91, AccountModelID: 92, ModelAccountID: 93, ProviderCode: "seedance", ModelCode: "seedance-2-5", JobID: "job-account"}}
	store := newMemoryStore(item)
	provider := &providerStub{getStatus: providervideo.Status{JobID: "job-account", State: providervideo.StateRunning}}
	resolver := &capturingProviderResolver{provider: provider}
	runner := NewRunner(store, resolver, storage.NewStaticRouter(&streamingBackend{objects: map[string][]byte{}}), Options{Owner: "video-worker", Now: func() time.Time { return now }})

	if processed, err := runner.RunOnce(t.Context()); err != nil || !processed {
		t.Fatalf("RunOnce() processed=%v err=%v", processed, err)
	}
	want := ProviderRef{RouteCandidateID: 91, AccountModelID: 92, ModelAccountID: 93, ProviderCode: "seedance", ModelCode: "seedance-2-5"}
	if resolver.last != want {
		t.Fatalf("provider ref = %#v, want %#v", resolver.last, want)
	}
}

func TestRunnerUsesAttemptAccountArtifactHostAllowlist(t *testing.T) {
	provider := &providerStub{}
	resolver := &capturingProviderResolver{provider: provider, artifactHosts: []string{"cdn.seedance.example"}}
	runner := NewRunner(newMemoryStore(WorkItem{}), resolver, storage.NewStaticRouter(nil), Options{})
	item := WorkItem{Attempt: Attempt{RouteCandidateID: 1, AccountModelID: 2, ModelAccountID: 3, ProviderCode: "seedance", ModelCode: "seedance-2-5"}}
	resolved, err := runner.resolveExecution(t.Context(), item)
	if err != nil || len(resolved.ArtifactAllowedHosts) != 1 || resolved.ArtifactAllowedHosts[0] != "cdn.seedance.example" {
		t.Fatalf("resolved execution = %#v err=%v", resolved, err)
	}
}

func TestRunnerLoopbackArtifactHostRequiresExplicitLocalOption(t *testing.T) {
	resolveLoopback := func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	allowedHost := "127.0.0.1:18443"
	strict := NewRunner(newMemoryStore(WorkItem{}), nil, nil, Options{ResolveHostIPs: resolveLoopback})
	if strict.allowedArtifactHost(t.Context(), allowedHost, []string{allowedHost}) {
		t.Fatal("loopback artifact host must be rejected by default")
	}
	local := NewRunner(newMemoryStore(WorkItem{}), nil, nil, Options{ResolveHostIPs: resolveLoopback, AllowLoopbackArtifactHosts: true})
	if !local.allowedArtifactHost(t.Context(), allowedHost, []string{allowedHost}) {
		t.Fatal("explicit local-only option must allow an exactly allowlisted loopback host")
	}
	private := NewRunner(newMemoryStore(WorkItem{}), nil, nil, Options{
		ResolveHostIPs:             func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("10.0.0.7")}, nil },
		AllowLoopbackArtifactHosts: true,
	})
	if private.allowedArtifactHost(t.Context(), "10.0.0.7:18443", []string{"10.0.0.7:18443"}) {
		t.Fatal("local-only option must not allow private non-loopback artifact hosts")
	}
}

func newTestRunner(store Store, provider providervideo.Provider, now time.Time) *Runner {
	return NewRunner(store, staticProviderResolver{provider: provider}, storage.NewStaticRouter(&streamingBackend{objects: map[string][]byte{}}), Options{
		Owner: "video-worker", LeaseTTL: 30 * time.Second, PollIntervals: []time.Duration{2 * time.Second, 5 * time.Second}, ArtifactMaxBytes: 1 << 20,
		Now: func() time.Time { return now }, AttemptID: func() string { return "attempt-generated" }, AssetID: func() string { return "asset-generated" },
		ResolveHostIPs: func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil },
	})
}

type staticProviderResolver struct{ provider providervideo.Provider }

func (r staticProviderResolver) Resolve(context.Context, ProviderRef) (ResolvedExecution, error) {
	if r.provider == nil {
		return ResolvedExecution{}, errors.New("provider unavailable")
	}
	return ResolvedExecution{Provider: r.provider}, nil
}

type capturingProviderResolver struct {
	provider      providervideo.Provider
	last          ProviderRef
	artifactHosts []string
}

func (r *capturingProviderResolver) Resolve(_ context.Context, ref ProviderRef) (ResolvedExecution, error) {
	r.last = ref
	return ResolvedExecution{Provider: r.provider, ArtifactAllowedHosts: append([]string(nil), r.artifactHosts...)}, nil
}

type providerStub struct {
	mu             sync.Mutex
	submitCalls    int
	getCalls       int
	cancelCalls    int
	reconcileCalls int
	submitJob      providervideo.Job
	submitErr      error
	getStatus      providervideo.Status
	getErr         error
	cancelResult   providervideo.CancelResult
	cancelErr      error
	reconcileJob   providervideo.Job
	reconcileFound bool
	reconcileErr   error
	lastSubmit     providervideo.Request
}

func (p *providerStub) Submit(_ context.Context, req providervideo.Request) (providervideo.Job, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.submitCalls++
	p.lastSubmit = req
	return p.submitJob, p.submitErr
}
func (p *providerStub) Get(context.Context, providervideo.JobRef) (providervideo.Status, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.getCalls++
	return p.getStatus, p.getErr
}
func (p *providerStub) Cancel(context.Context, providervideo.JobRef) (providervideo.CancelResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancelCalls++
	return p.cancelResult, p.cancelErr
}
func (*providerStub) VerifyCallback(context.Context, http.Header, []byte) (providervideo.CallbackEvent, error) {
	return providervideo.CallbackEvent{}, nil
}
func (*providerStub) NormalizeUsage(status providervideo.Status) (providervideo.Usage, error) {
	return providervideo.Usage{OutputSeconds: "5.000", ProviderTokens: "1200", Raw: status.Usage}, nil
}
func (p *providerStub) Reconcile(context.Context, providervideo.Request) (providervideo.Job, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reconcileCalls++
	return p.reconcileJob, p.reconcileFound, p.reconcileErr
}

type memoryStore struct {
	mu            sync.Mutex
	item          WorkItem
	prepareCalls  int
	beforeApply   func(*memoryStore)
	settlement    SettlementSnapshot
	finalized     bool
	finalizeCalls int
	lastFinalize  FinalizeRequest
	lastArtifact  ArtifactCommitRequest
	lastApply     ApplyStepRequest
}

func newMemoryStore(item WorkItem) *memoryStore { return &memoryStore{item: item} }

func (s *memoryStore) ClaimDue(_ context.Context, req ClaimRequest) (WorkItem, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.item.ID == "" || s.item.LeaseOwner != "" || (s.item.NextActionAt != nil && s.item.NextActionAt.After(req.Now)) {
		return WorkItem{}, false, nil
	}
	s.item.LeaseOwner = req.Owner
	s.item.LeaseExpiresAt = req.Now.Add(req.LeaseTTL)
	return s.item, true, nil
}

func (s *memoryStore) PrepareAttempt(_ context.Context, req PrepareAttemptRequest) (WorkItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prepareCalls++
	if s.item.Version != req.ExpectedVersion || s.item.State != domainvideo.ItemStateQueued {
		return WorkItem{}, ErrStepConflict
	}
	s.item.State = domainvideo.ItemStateSubmitting
	s.item.Version++
	s.item.Attempt = Attempt{ID: req.AttemptID, No: 1, ProviderCode: "fake", IdempotencyKey: req.ProviderIdempotencyKey, Status: "submitting"}
	return s.item, nil
}

func (s *memoryStore) ApplyStep(_ context.Context, req ApplyStepRequest) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastApply = req
	if s.beforeApply != nil {
		s.beforeApply(s)
	}
	if s.item.Version != req.ExpectedVersion || s.item.LeaseOwner != req.Owner {
		return false, nil
	}
	if req.ArtifactExhausted && s.item.State == domainvideo.ItemStateArtifactPending {
		recovery, err := domainvideo.AdvanceItemState(domainvideo.ItemStateSnapshot{State: s.item.State, Version: s.item.Version}, domainvideo.ItemTransition{ExpectedVersion: req.ExpectedVersion, Target: domainvideo.ItemStateRecoveryRequired})
		if err != nil {
			return false, err
		}
		s.item.State, s.item.Version = recovery.Snapshot.State, recovery.Snapshot.Version
		req.ExpectedVersion = s.item.Version
	}
	transition, err := domainvideo.AdvanceItemState(domainvideo.ItemStateSnapshot{State: s.item.State, Version: s.item.Version}, domainvideo.ItemTransition{ExpectedVersion: req.ExpectedVersion, Target: req.Target})
	if err != nil {
		return false, err
	}
	if transition.Changed {
		s.item.State, s.item.Version = transition.Snapshot.State, transition.Snapshot.Version
	}
	if req.ProviderJobID != "" {
		s.item.Attempt.JobID = req.ProviderJobID
	}
	if req.AttemptStatus != "" {
		s.item.Attempt.Status = req.AttemptStatus
	}
	if req.Artifact.URL != "" {
		s.item.Artifact = req.Artifact
	}
	if req.PlatformAbsorbed {
		s.item.Attempt.PlatformAbsorbed = true
	}
	if req.IncrementArtifactAttempts {
		s.item.ArtifactAttempts++
	}
	s.item.ErrorCode, s.item.ErrorMessage = req.ErrorCode, req.ErrorMessage
	s.item.NextActionAt = req.NextActionAt
	s.item.LeaseOwner = ""
	s.item.LeaseExpiresAt = time.Time{}
	return true, nil
}

func (s *memoryStore) CommitArtifact(_ context.Context, req ArtifactCommitRequest) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.item.Version != req.ExpectedVersion || s.item.LeaseOwner != req.Owner {
		return false, nil
	}
	s.lastArtifact = req
	s.item.State = domainvideo.ItemStateSucceeded
	s.item.Version++
	s.item.ResultAssetID = req.AssetID
	s.item.ActualPoints = s.item.PricePoints
	s.item.LeaseOwner = ""
	s.item.LeaseExpiresAt = time.Time{}
	return true, nil
}

func (s *memoryStore) LoadSettlement(context.Context, string) (SettlementSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settlement, nil
}

func (s *memoryStore) FinalizeTask(_ context.Context, req FinalizeRequest) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finalized {
		return false, nil
	}
	s.finalized = true
	s.finalizeCalls++
	s.lastFinalize = req
	s.item.NeedsSettlement = false
	s.item.LeaseOwner = ""
	s.item.LeaseExpiresAt = time.Time{}
	return true, nil
}

func (s *memoryStore) ReleaseLease(context.Context, LeaseRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.item.LeaseOwner = ""
	s.item.LeaseExpiresAt = time.Time{}
	return nil
}

func (s *memoryStore) itemSnapshot() WorkItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.item
}

func (s *memoryStore) makeDue(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.item.NextActionAt = &now
	s.item.LeaseOwner = ""
	s.item.LeaseExpiresAt = time.Time{}
}

type streamingBackend struct {
	mu             sync.Mutex
	objects        map[string][]byte
	putCalls       int
	putReaderCalls int
}

type inputSigningBackend struct{ streamingBackend }

func (*inputSigningBackend) Driver() string { return "s3" }
func (*inputSigningBackend) TemporaryGetURL(_ context.Context, key string, _ storage.TemporaryGetURLOptions) (string, error) {
	return "https://media.example.test/" + key + "?signature=short", nil
}

type inputRouter struct{ ref storage.BackendRef }

func (r *inputRouter) DefaultWriter(context.Context) (storage.BackendRef, error) { return r.ref, nil }
func (r *inputRouter) BackendFor(_ context.Context, _, _ string) (storage.BackendRef, error) {
	return r.ref, nil
}
func (r *inputRouter) ReadableBackends(context.Context) ([]storage.BackendRef, error) {
	return []storage.BackendRef{r.ref}, nil
}

func (*streamingBackend) Driver() string { return "local" }
func (b *streamingBackend) Put(_ context.Context, key, _ string, content []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.putCalls++
	b.objects[key] = append([]byte(nil), content...)
	return nil
}
func (b *streamingBackend) PutReader(_ context.Context, key, _ string, reader io.Reader, size int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.putReaderCalls++
	content, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if int64(len(content)) != size {
		return storage.ErrSizeMismatch
	}
	b.objects[key] = content
	return nil
}
func (b *streamingBackend) Get(_ context.Context, key string) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	content, ok := b.objects[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return append([]byte(nil), content...), nil
}
func (b *streamingBackend) OpenReader(ctx context.Context, key string, max int64) (io.ReadCloser, int64, error) {
	content, err := b.Get(ctx, key)
	if err != nil {
		return nil, 0, err
	}
	if int64(len(content)) > max {
		return nil, 0, storage.ErrObjectTooLarge
	}
	return io.NopCloser(bytes.NewReader(content)), int64(len(content)), nil
}
func (b *streamingBackend) Delete(_ context.Context, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.objects, key)
	return nil
}
