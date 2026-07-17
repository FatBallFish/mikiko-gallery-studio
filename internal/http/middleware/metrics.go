package middleware

import (
	"io"
	"net/http"
	"time"

	"github.com/fatballfish/pic-gallery/internal/app/observability"
)

func Metrics(next http.Handler) http.Handler {
	metrics := observability.DefaultMetrics()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metrics.IncHTTPRequest()
		metricsWithRuntime(metrics.Runtime(), next).ServeHTTP(w, r)
	})
}

func metricsWithRuntime(metrics *observability.RuntimeMetrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !shouldObserveRuntimeRequest(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		startedAt := time.Now()
		finish := metrics.BeginRequest(r.Method)
		tracker := &metricsResponseWriter{ResponseWriter: w}
		wrapped := wrapMetricsCapabilities(tracker, w)
		defer func() {
			status := tracker.status
			if status == 0 {
				status = http.StatusOK
			}
			finish(r.Pattern, status, time.Since(startedAt))
		}()
		next.ServeHTTP(wrapped, r)
	})
}

func shouldObserveRuntimeRequest(path string) bool {
	switch path {
	case "/metrics", "/readyz", "/api/ops/admin/v1/monitoring/snapshot":
		return false
	default:
		return true
	}
}

type metricsResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *metricsResponseWriter) WriteHeader(status int) {
	if status >= 100 && status < 200 && status != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *metricsResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *metricsResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func newMetricsResponseWriter(w http.ResponseWriter) http.ResponseWriter {
	tracker := &metricsResponseWriter{ResponseWriter: w}
	return wrapMetricsCapabilities(tracker, w)
}

type trackedFlusher struct {
	tracker *metricsResponseWriter
	target  http.Flusher
}

func (w trackedFlusher) Flush() {
	if w.tracker.status == 0 {
		w.tracker.WriteHeader(http.StatusOK)
	}
	w.target.Flush()
}

type trackedReaderFrom struct {
	tracker *metricsResponseWriter
}

func (w trackedReaderFrom) ReadFrom(reader io.Reader) (int64, error) {
	return io.Copy(w.tracker, reader)
}

func wrapMetricsCapabilities(tracker *metricsResponseWriter, original http.ResponseWriter) http.ResponseWriter {
	rawFlusher, hasFlusher := original.(http.Flusher)
	hijacker, hasHijacker := original.(http.Hijacker)
	pusher, hasPusher := original.(http.Pusher)
	_, hasReaderFrom := original.(io.ReaderFrom)
	flusher := trackedFlusher{tracker: tracker, target: rawFlusher}
	readerFrom := trackedReaderFrom{tracker: tracker}

	mask := 0
	if hasFlusher {
		mask |= 1
	}
	if hasHijacker {
		mask |= 2
	}
	if hasPusher {
		mask |= 4
	}
	if hasReaderFrom {
		mask |= 8
	}
	switch mask {
	case 1:
		return struct {
			*metricsResponseWriter
			http.Flusher
		}{tracker, flusher}
	case 2:
		return struct {
			*metricsResponseWriter
			http.Hijacker
		}{tracker, hijacker}
	case 3:
		return struct {
			*metricsResponseWriter
			http.Flusher
			http.Hijacker
		}{tracker, flusher, hijacker}
	case 4:
		return struct {
			*metricsResponseWriter
			http.Pusher
		}{tracker, pusher}
	case 5:
		return struct {
			*metricsResponseWriter
			http.Flusher
			http.Pusher
		}{tracker, flusher, pusher}
	case 6:
		return struct {
			*metricsResponseWriter
			http.Hijacker
			http.Pusher
		}{tracker, hijacker, pusher}
	case 7:
		return struct {
			*metricsResponseWriter
			http.Flusher
			http.Hijacker
			http.Pusher
		}{tracker, flusher, hijacker, pusher}
	case 8:
		return struct {
			*metricsResponseWriter
			io.ReaderFrom
		}{tracker, readerFrom}
	case 9:
		return struct {
			*metricsResponseWriter
			http.Flusher
			io.ReaderFrom
		}{tracker, flusher, readerFrom}
	case 10:
		return struct {
			*metricsResponseWriter
			http.Hijacker
			io.ReaderFrom
		}{tracker, hijacker, readerFrom}
	case 11:
		return struct {
			*metricsResponseWriter
			http.Flusher
			http.Hijacker
			io.ReaderFrom
		}{tracker, flusher, hijacker, readerFrom}
	case 12:
		return struct {
			*metricsResponseWriter
			http.Pusher
			io.ReaderFrom
		}{tracker, pusher, readerFrom}
	case 13:
		return struct {
			*metricsResponseWriter
			http.Flusher
			http.Pusher
			io.ReaderFrom
		}{tracker, flusher, pusher, readerFrom}
	case 14:
		return struct {
			*metricsResponseWriter
			http.Hijacker
			http.Pusher
			io.ReaderFrom
		}{tracker, hijacker, pusher, readerFrom}
	case 15:
		return struct {
			*metricsResponseWriter
			http.Flusher
			http.Hijacker
			http.Pusher
			io.ReaderFrom
		}{tracker, flusher, hijacker, pusher, readerFrom}
	default:
		return tracker
	}
}
