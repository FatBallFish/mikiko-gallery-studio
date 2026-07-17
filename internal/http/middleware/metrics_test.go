package middleware

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/app/observability"
)

func TestMetricsRecordsCompletedRequestsByPattern(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	runtimeMetrics := observability.NewRuntimeMetrics(observability.RuntimeMetricsOptions{
		Now:     func() time.Time { return now },
		Sampler: func() observability.RuntimeSample { return observability.RuntimeSample{} },
	})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/{user_id}", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})
	mux.HandleFunc("POST /api/tasks", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad input", http.StatusBadRequest)
	})
	mux.HandleFunc("GET /api/fail", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	handler := RequestID(metricsWithRuntime(runtimeMetrics, mux))

	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/users/42", nil),
		httptest.NewRequest(http.MethodPost, "/api/tasks", nil),
		httptest.NewRequest(http.MethodGet, "/api/fail", nil),
	} {
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}
	now = now.Add(5 * time.Second)
	runtimeMetrics.RecordRuntimeSample(observability.RuntimeSample{})

	snapshot, err := runtimeMetrics.Snapshot(observability.Window5m)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.Statuses.Success != 1 || snapshot.Statuses.ClientError != 1 || snapshot.Statuses.ServerError != 1 {
		t.Fatalf("statuses = %+v, want one 2xx, 4xx, and 5xx", snapshot.Statuses)
	}
	routes := make(map[string]observability.RuntimeRoute, len(snapshot.Routes))
	for _, route := range snapshot.Routes {
		routes[route.Route] = route
	}
	if routes["GET /api/users/{user_id}"].Requests != 1 {
		t.Fatalf("normalized route missing from %+v", snapshot.Routes)
	}
	if _, leaked := routes["GET /api/users/42"]; leaked {
		t.Fatalf("raw path leaked into routes: %+v", snapshot.Routes)
	}
}

func TestMetricsExcludesOperationalSelfTraffic(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	runtimeMetrics := observability.NewRuntimeMetrics(observability.RuntimeMetricsOptions{
		Now:     func() time.Time { return now },
		Sampler: func() observability.RuntimeSample { return observability.RuntimeSample{} },
	})
	handler := metricsWithRuntime(runtimeMetrics, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, path := range []string{
		"/metrics",
		"/readyz",
		"/api/ops/admin/v1/monitoring/snapshot?window=5m",
	} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	now = now.Add(5 * time.Second)
	runtimeMetrics.RecordRuntimeSample(observability.RuntimeSample{})

	snapshot, err := runtimeMetrics.Snapshot(observability.Window5m)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.Statuses.Total != 0 || len(snapshot.Routes) != 0 {
		t.Fatalf("self traffic should be excluded, got statuses=%+v routes=%+v", snapshot.Statuses, snapshot.Routes)
	}
}

func TestMetricsResponseWriterPreservesSupportedInterfaces(t *testing.T) {
	underlying := &allCapabilityWriter{header: make(http.Header)}
	wrapped := newMetricsResponseWriter(underlying)

	if _, ok := wrapped.(http.Flusher); !ok {
		t.Fatal("wrapped writer lost http.Flusher")
	}
	if _, ok := wrapped.(http.Hijacker); !ok {
		t.Fatal("wrapped writer lost http.Hijacker")
	}
	if _, ok := wrapped.(http.Pusher); !ok {
		t.Fatal("wrapped writer lost http.Pusher")
	}
	if _, ok := wrapped.(io.ReaderFrom); !ok {
		t.Fatal("wrapped writer lost io.ReaderFrom")
	}
	_, _ = io.Copy(wrapped, strings.NewReader("stream"))
	wrapped.(http.Flusher).Flush()
	if !underlying.flushed || underlying.body.String() != "stream" {
		t.Fatalf("capability calls did not reach underlying writer: flushed=%v body=%q", underlying.flushed, underlying.body.String())
	}
}

func TestMetricsResponseWriterDoesNotAdvertiseUnsupportedInterfaces(t *testing.T) {
	wrapped := newMetricsResponseWriter(httptest.NewRecorder())
	if _, ok := wrapped.(http.Hijacker); ok {
		t.Fatal("wrapped recorder must not advertise http.Hijacker")
	}
	if _, ok := wrapped.(http.Pusher); ok {
		t.Fatal("wrapped recorder must not advertise http.Pusher")
	}
}

func TestMetricsResponseWriterTracksFinalStatusAfterEarlyHints(t *testing.T) {
	underlying := &allCapabilityWriter{header: make(http.Header)}
	tracker := &metricsResponseWriter{ResponseWriter: underlying}
	wrapped := wrapMetricsCapabilities(tracker, underlying)

	wrapped.WriteHeader(http.StatusEarlyHints)
	wrapped.WriteHeader(http.StatusServiceUnavailable)

	if tracker.status != http.StatusServiceUnavailable {
		t.Fatalf("tracked status = %d, want final 503", tracker.status)
	}
	if got := underlying.statuses; len(got) != 2 || got[0] != http.StatusEarlyHints || got[1] != http.StatusServiceUnavailable {
		t.Fatalf("underlying statuses = %v, want 103 then 503", got)
	}
}

func TestMetricsResponseWriterFlushCommitsSuccess(t *testing.T) {
	underlying := &allCapabilityWriter{header: make(http.Header)}
	tracker := &metricsResponseWriter{ResponseWriter: underlying}
	wrapped := wrapMetricsCapabilities(tracker, underlying)

	wrapped.(http.Flusher).Flush()
	wrapped.WriteHeader(http.StatusInternalServerError)

	if tracker.status != http.StatusOK {
		t.Fatalf("tracked status after flush = %d, want 200", tracker.status)
	}
	if len(underlying.statuses) != 1 || underlying.statuses[0] != http.StatusOK {
		t.Fatalf("underlying statuses = %v, want only committed 200", underlying.statuses)
	}
}

func TestMetricsResponseWriterReaderFromCommitsSuccess(t *testing.T) {
	underlying := &allCapabilityWriter{header: make(http.Header)}
	tracker := &metricsResponseWriter{ResponseWriter: underlying}
	wrapped := wrapMetricsCapabilities(tracker, underlying)

	_, _ = wrapped.(io.ReaderFrom).ReadFrom(strings.NewReader("stream"))
	if tracker.status != http.StatusOK {
		t.Fatalf("tracked status after ReadFrom = %d, want 200", tracker.status)
	}
}

func TestMetricsResponseWriterReaderFromErrorKeepsStatusWritable(t *testing.T) {
	underlying := &allCapabilityWriter{header: make(http.Header)}
	tracker := &metricsResponseWriter{ResponseWriter: underlying}
	wrapped := wrapMetricsCapabilities(tracker, underlying)

	_, readErr := wrapped.(io.ReaderFrom).ReadFrom(zeroByteErrorReader{})
	if readErr == nil {
		t.Fatal("expected reader error")
	}
	wrapped.WriteHeader(http.StatusInternalServerError)
	if tracker.status != http.StatusInternalServerError {
		t.Fatalf("tracked status after zero-byte error = %d, want 500", tracker.status)
	}
	if len(underlying.statuses) != 1 || underlying.statuses[0] != http.StatusInternalServerError {
		t.Fatalf("underlying statuses = %v, want only 500", underlying.statuses)
	}
}

type allCapabilityWriter struct {
	header   http.Header
	body     strings.Builder
	status   int
	statuses []int
	flushed  bool
}

type zeroByteErrorReader struct{}

func (zeroByteErrorReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func (w *allCapabilityWriter) Header() http.Header {
	return w.header
}

func (w *allCapabilityWriter) Write(body []byte) (int, error) {
	return w.body.Write(body)
}

func (w *allCapabilityWriter) WriteHeader(status int) {
	w.status = status
	w.statuses = append(w.statuses, status)
}

func (w *allCapabilityWriter) Flush() {
	w.flushed = true
}

func (w *allCapabilityWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, http.ErrNotSupported
}

func (w *allCapabilityWriter) Push(string, *http.PushOptions) error {
	return nil
}

func (w *allCapabilityWriter) ReadFrom(reader io.Reader) (int64, error) {
	return io.Copy(&w.body, reader)
}
