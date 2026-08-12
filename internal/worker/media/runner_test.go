package media

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type fakeStore struct {
	item        WorkItem
	claimed     bool
	claimErr    error
	completed   []CompleteRequest
	failed      []FailRequest
	released    []LeaseRef
	completeErr error
}

func (store *fakeStore) ClaimDue(context.Context, ClaimRequest) (WorkItem, bool, error) {
	return store.item, store.claimed, store.claimErr
}
func (store *fakeStore) Complete(_ context.Context, request CompleteRequest) (bool, error) {
	store.completed = append(store.completed, request)
	return store.completeErr == nil, store.completeErr
}

type cleanupProcessor struct {
	fakeProcessor
	cleaned []ProcessResult
}

func (processor *cleanupProcessor) Cleanup(_ context.Context, result ProcessResult) error {
	processor.cleaned = append(processor.cleaned, result)
	return nil
}

func TestRunnerCleansUploadedDerivativesWhenCompleteFails(t *testing.T) {
	result := ProcessResult{Derivatives: []Derivative{{ObjectKey: "media/derivatives/asset/proxy.mp4"}}}
	store := &fakeStore{claimed: true, item: WorkItem{JobID: "job-complete-fail"}, completeErr: errors.New("database unavailable")}
	processor := &cleanupProcessor{fakeProcessor: fakeProcessor{result: result}}
	runner := NewRunner(store, processor, Options{Owner: "media-a"})

	processed, err := runner.RunOnce(t.Context())
	if !processed || err == nil || len(processor.cleaned) != 1 || processor.cleaned[0].Derivatives[0].ObjectKey != result.Derivatives[0].ObjectKey {
		t.Fatalf("complete failure processed=%v err=%v cleaned=%#v", processed, err, processor.cleaned)
	}
}
func (store *fakeStore) Fail(_ context.Context, request FailRequest) error {
	store.failed = append(store.failed, request)
	return nil
}
func (store *fakeStore) ReleaseLease(_ context.Context, ref LeaseRef) error {
	store.released = append(store.released, ref)
	return nil
}

type fakeProcessor struct {
	result ProcessResult
	err    error
}

func (processor fakeProcessor) Process(context.Context, WorkItem) (ProcessResult, error) {
	return processor.result, processor.err
}

func TestRunnerCompletesClaimedMediaJob(t *testing.T) {
	now := time.Date(2026, 8, 12, 16, 0, 0, 0, time.UTC)
	store := &fakeStore{claimed: true, item: WorkItem{JobID: "job-1", AssetID: "asset-1", AttemptCount: 1, MaxAttempts: 5}}
	want := ProcessResult{Probe: ProbeMetadata{Container: "mp4", VideoCodec: "h264"}, Derivatives: []Derivative{{Kind: "proxy", ObjectKey: "media/derivatives/asset-1/proxy.mp4"}}}
	observer := &mediaObserverSpy{}
	runner := NewRunner(store, fakeProcessor{result: want}, Options{Owner: "media-a", LeaseTTL: time.Minute, Now: func() time.Time { return now }, Observer: observer})

	processed, err := runner.RunOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("RunOnce processed=%v err=%v", processed, err)
	}
	if len(store.completed) != 1 || store.completed[0].JobID != "job-1" || store.completed[0].Result.Probe.Container != "mp4" {
		t.Fatalf("complete requests = %#v", store.completed)
	}
	if len(store.failed) != 0 || len(store.released) != 1 {
		t.Fatalf("failed=%#v released=%#v", store.failed, store.released)
	}
	if len(observer.derivatives) != 1 || observer.derivatives[0] != "proxy:success:0" {
		t.Fatalf("derivative observations = %#v", observer.derivatives)
	}
}

type mediaObserverSpy struct{ derivatives []string }

func (spy *mediaObserverSpy) RecordDerivative(kind, result string, bytes int64) {
	spy.derivatives = append(spy.derivatives, kind+":"+result+":"+fmt.Sprint(bytes))
}

type mediaFailureReporterSpy struct {
	item     WorkItem
	terminal bool
	err      error
}

func (spy *mediaFailureReporterSpy) ReportMediaProcessingFailure(_ context.Context, item WorkItem, terminal bool, err error) {
	spy.item, spy.terminal, spy.err = item, terminal, err
}

func TestRunnerPersistsRetryWithoutKillingRoleLoop(t *testing.T) {
	now := time.Date(2026, 8, 12, 16, 30, 0, 0, time.UTC)
	store := &fakeStore{claimed: true, item: WorkItem{JobID: "job-retry", AssetID: "asset-retry", AttemptCount: 2, MaxAttempts: 5}}
	reporter := &mediaFailureReporterSpy{}
	runner := NewRunner(store, fakeProcessor{err: errors.New("ffmpeg failed with sensitive output")}, Options{Owner: "media-a", Now: func() time.Time { return now }, Reporter: reporter})

	processed, err := runner.RunOnce(t.Context())
	if err != nil || !processed {
		t.Fatalf("RunOnce processing failure = processed %v err %v", processed, err)
	}
	if len(store.failed) != 1 || store.failed[0].Terminal || !store.failed[0].RetryAt.After(now) || store.failed[0].ErrorCode != "media_processing_failed" {
		t.Fatalf("retry request = %#v", store.failed)
	}
	if store.failed[0].ErrorMessage != "media processing failed" {
		t.Fatalf("failure message leaked implementation detail: %q", store.failed[0].ErrorMessage)
	}
	if reporter.item.JobID != "job-retry" || reporter.terminal || reporter.err == nil || reporter.err.Error() != "ffmpeg failed with sensitive output" {
		t.Fatalf("failure report = %#v", reporter)
	}
}

func TestRunnerMarksFinalAttemptTerminal(t *testing.T) {
	store := &fakeStore{claimed: true, item: WorkItem{JobID: "job-final", AttemptCount: 5, MaxAttempts: 5}}
	runner := NewRunner(store, fakeProcessor{err: errors.New("bad media")}, Options{Owner: "media-a"})

	processed, err := runner.RunOnce(t.Context())
	if err != nil || !processed || len(store.failed) != 1 || !store.failed[0].Terminal {
		t.Fatalf("final failure processed=%v err=%v requests=%#v", processed, err, store.failed)
	}
}

func TestRunnerPausesBeforeClaimWhenResourceGateIsClosed(t *testing.T) {
	store := &fakeStore{claimed: true, item: WorkItem{JobID: "must-not-claim"}}
	runner := NewRunner(store, fakeProcessor{}, Options{Owner: "media-a", ClaimAllowed: func(context.Context) (bool, error) { return false, nil }})
	processed, err := runner.RunOnce(t.Context())
	if err != nil || processed || len(store.released) != 0 {
		t.Fatalf("closed gate processed=%v err=%v released=%#v", processed, err, store.released)
	}
}
