package observability

import (
	"net/http"
	"testing"
	"time"
)

func TestRuntimeMetricsTracksInflightStatusAndLatency(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	metrics := NewRuntimeMetrics(RuntimeMetricsOptions{Now: clock})

	finishOK := metrics.BeginRequest("GET")
	finishFailure := metrics.BeginRequest("POST")
	before := mustSnapshot(t, metrics, Window5m)
	if before.Current.Inflight != 2 {
		t.Fatalf("inflight before completion = %d, want 2", before.Current.Inflight)
	}

	finishOK("/api/users/{user_id}", http.StatusOK, 120*time.Millisecond)
	finishFailure("/api/tasks", http.StatusInternalServerError, 1200*time.Millisecond)
	now = now.Add(sampleInterval)
	metrics.RecordRuntimeSample(RuntimeSample{
		CPUPercent: float64Pointer(42.5),
		HeapBytes:  32 << 20,
		SysBytes:   64 << 20,
		Goroutines: 24,
	})

	snapshot := mustSnapshot(t, metrics, Window5m)
	if snapshot.Collecting {
		t.Fatal("snapshot should be ready after one complete bucket")
	}
	if snapshot.Current.Inflight != 0 || snapshot.Current.PeakInflight != 2 {
		t.Fatalf("concurrency = current %d peak %d, want 0/2", snapshot.Current.Inflight, snapshot.Current.PeakInflight)
	}
	if snapshot.Statuses.Success != 1 || snapshot.Statuses.ServerError != 1 {
		t.Fatalf("statuses = %+v, want one success and one server error", snapshot.Statuses)
	}
	if snapshot.Current.QPS != 0.4 {
		t.Fatalf("qps = %v, want 0.4", snapshot.Current.QPS)
	}
	if snapshot.Current.P50MS != 250 || snapshot.Current.P95MS != 2000 || snapshot.Current.P99MS != 2000 {
		t.Fatalf("latency percentiles = %d/%d/%d, want 250/2000/2000", snapshot.Current.P50MS, snapshot.Current.P95MS, snapshot.Current.P99MS)
	}
	if snapshot.Current.CPUPercent == nil || *snapshot.Current.CPUPercent != 42.5 {
		t.Fatalf("cpu = %v, want 42.5", snapshot.Current.CPUPercent)
	}
}

func TestRuntimeMetricsReturnsRequestedWindow(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	metrics := NewRuntimeMetrics(RuntimeMetricsOptions{Now: func() time.Time { return now }})

	for index := 0; index < 130; index++ {
		finish := metrics.BeginRequest("GET")
		finish("/api/tasks", http.StatusOK, 25*time.Millisecond)
		now = now.Add(sampleInterval)
		metrics.RecordRuntimeSample(RuntimeSample{HeapBytes: uint64(index + 1)})
	}

	fiveMinutes := mustSnapshot(t, metrics, Window5m)
	if got, want := len(fiveMinutes.Series), 60; got != want {
		t.Fatalf("5m points = %d, want %d", got, want)
	}
	fifteenMinutes := mustSnapshot(t, metrics, Window15m)
	if got, want := len(fifteenMinutes.Series), 130; got != want {
		t.Fatalf("15m points = %d, want %d available points", got, want)
	}
	if _, err := metrics.Snapshot(Window("10m")); err == nil {
		t.Fatal("invalid window should fail")
	}
}

func TestRuntimeMetricsRollsOverAfterSixtyMinutes(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	metrics := NewRuntimeMetrics(RuntimeMetricsOptions{Now: func() time.Time { return now }})

	for index := 0; index < maxRuntimeBuckets+20; index++ {
		finish := metrics.BeginRequest("GET")
		finish("/api/health", http.StatusOK, 10*time.Millisecond)
		now = now.Add(sampleInterval)
		metrics.RecordRuntimeSample(RuntimeSample{HeapBytes: uint64(index + 1)})
	}

	snapshot := mustSnapshot(t, metrics, Window60m)
	if got := len(snapshot.Series); got != maxRuntimeBuckets {
		t.Fatalf("60m points = %d, want bounded %d", got, maxRuntimeBuckets)
	}
	if snapshot.Series[0].At.Before(now.Add(-60 * time.Minute)) {
		t.Fatalf("oldest point %s escaped 60m retention at %s", snapshot.Series[0].At, now)
	}
}

func TestRuntimeMetricsAggregatesNormalizedRoutes(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	metrics := NewRuntimeMetrics(RuntimeMetricsOptions{Now: func() time.Time { return now }})

	for index := 0; index < 3; index++ {
		finish := metrics.BeginRequest("GET")
		finish("/api/users/{user_id}", http.StatusOK, 40*time.Millisecond)
	}
	finish := metrics.BeginRequest("POST")
	finish("/api/tasks", http.StatusBadRequest, 400*time.Millisecond)
	now = now.Add(sampleInterval)
	metrics.RecordRuntimeSample(RuntimeSample{})

	snapshot := mustSnapshot(t, metrics, Window5m)
	if len(snapshot.Routes) != 2 {
		t.Fatalf("routes = %+v, want two normalized routes", snapshot.Routes)
	}
	if snapshot.Routes[0].Route != "GET /api/users/{user_id}" || snapshot.Routes[0].Requests != 3 {
		t.Fatalf("top route = %+v, want aggregated users route", snapshot.Routes[0])
	}
	if snapshot.Routes[1].Route != "POST /api/tasks" || snapshot.Routes[1].ClientErrorRate != 100 {
		t.Fatalf("second route = %+v, want task client error", snapshot.Routes[1])
	}
}

func TestRuntimeMetricsCollectingState(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	metrics := NewRuntimeMetrics(RuntimeMetricsOptions{Now: func() time.Time { return now }})

	snapshot := mustSnapshot(t, metrics, Window5m)
	if !snapshot.Collecting || snapshot.State != RuntimeStateCollecting {
		t.Fatalf("initial snapshot = collecting %v state %q", snapshot.Collecting, snapshot.State)
	}
	if snapshot.Current.CPUPercent != nil {
		t.Fatalf("unavailable CPU should remain nil, got %v", snapshot.Current.CPUPercent)
	}
}

func TestRuntimeMetricsClassifiesPressureWithReasons(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	metrics := NewRuntimeMetrics(RuntimeMetricsOptions{Now: func() time.Time { return now }})

	for index := 0; index < 20; index++ {
		finish := metrics.BeginRequest("GET")
		status := http.StatusOK
		if index == 0 {
			status = http.StatusInternalServerError
		}
		finish("/api/tasks", status, 2300*time.Millisecond)
	}
	now = now.Add(sampleInterval)
	metrics.RecordRuntimeSample(RuntimeSample{CPUPercent: float64Pointer(92)})

	snapshot := mustSnapshot(t, metrics, Window5m)
	if snapshot.State != RuntimeStateCritical {
		t.Fatalf("state = %q, want critical; reasons=%v", snapshot.State, snapshot.StateReasons)
	}
	for _, expected := range []string{StateReasonServerErrors, StateReasonLatency, StateReasonCPU} {
		if !contains(snapshot.StateReasons, expected) {
			t.Fatalf("missing state reason %q in %v", expected, snapshot.StateReasons)
		}
	}
}

func mustSnapshot(t *testing.T, metrics *RuntimeMetrics, window Window) RuntimeSnapshot {
	t.Helper()
	snapshot, err := metrics.Snapshot(window)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return snapshot
}

func float64Pointer(value float64) *float64 {
	return &value
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
