package observability

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

type Metrics struct {
	httpRequestsTotal atomic.Uint64
	processStartUnix  int64
}

var defaultMetrics = NewMetrics()

func NewMetrics() *Metrics {
	return &Metrics{processStartUnix: time.Now().Unix()}
}

func DefaultMetrics() *Metrics {
	return defaultMetrics
}

func (m *Metrics) IncHTTPRequest() {
	m.httpRequestsTotal.Add(1)
}

func (m *Metrics) HTTPRequestsTotal() uint64 {
	return m.httpRequestsTotal.Load()
}

func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = fmt.Fprintf(w, "# HELP pic_gallery_http_requests_total Total HTTP requests processed by the API.\n")
		_, _ = fmt.Fprintf(w, "# TYPE pic_gallery_http_requests_total counter\n")
		_, _ = fmt.Fprintf(w, "pic_gallery_http_requests_total %d\n", m.HTTPRequestsTotal())
		_, _ = fmt.Fprintf(w, "# HELP pic_gallery_process_start_time_seconds Process start time.\n")
		_, _ = fmt.Fprintf(w, "# TYPE pic_gallery_process_start_time_seconds gauge\n")
		_, _ = fmt.Fprintf(w, "pic_gallery_process_start_time_seconds %d\n", m.processStartUnix)
	})
}
