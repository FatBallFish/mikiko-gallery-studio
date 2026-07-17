package observability

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

type Metrics struct {
	httpRequestsTotal                  atomic.Uint64
	publicGalleryListViewsTotal        atomic.Uint64
	publicGalleryDetailLoginBlockTotal atomic.Uint64
	processStartUnix                   int64
	runtime                            *RuntimeMetrics
}

var defaultMetrics = NewMetrics()

func NewMetrics() *Metrics {
	return &Metrics{
		processStartUnix: time.Now().Unix(),
		runtime:          NewRuntimeMetrics(RuntimeMetricsOptions{}),
	}
}

func DefaultMetrics() *Metrics {
	return defaultMetrics
}

func (m *Metrics) IncHTTPRequest() {
	m.httpRequestsTotal.Add(1)
}

func (m *Metrics) IncPublicGalleryListView() {
	m.publicGalleryListViewsTotal.Add(1)
}

func (m *Metrics) IncPublicGalleryDetailLoginBlock() {
	m.publicGalleryDetailLoginBlockTotal.Add(1)
}

func (m *Metrics) HTTPRequestsTotal() uint64 {
	return m.httpRequestsTotal.Load()
}

func (m *Metrics) PublicGalleryListViewsTotal() uint64 {
	return m.publicGalleryListViewsTotal.Load()
}

func (m *Metrics) PublicGalleryDetailLoginBlockTotal() uint64 {
	return m.publicGalleryDetailLoginBlockTotal.Load()
}

func (m *Metrics) Runtime() *RuntimeMetrics {
	return m.runtime
}

func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = fmt.Fprintf(w, "# HELP pic_gallery_http_requests_total Total HTTP requests processed by the API.\n")
		_, _ = fmt.Fprintf(w, "# TYPE pic_gallery_http_requests_total counter\n")
		_, _ = fmt.Fprintf(w, "pic_gallery_http_requests_total %d\n", m.HTTPRequestsTotal())
		_, _ = fmt.Fprintf(w, "# HELP pic_gallery_public_gallery_list_views_total Total public gallery list views served by the API.\n")
		_, _ = fmt.Fprintf(w, "# TYPE pic_gallery_public_gallery_list_views_total counter\n")
		_, _ = fmt.Fprintf(w, "pic_gallery_public_gallery_list_views_total %d\n", m.PublicGalleryListViewsTotal())
		_, _ = fmt.Fprintf(w, "# HELP pic_gallery_public_gallery_detail_login_blocks_total Total public gallery detail requests blocked by login requirement.\n")
		_, _ = fmt.Fprintf(w, "# TYPE pic_gallery_public_gallery_detail_login_blocks_total counter\n")
		_, _ = fmt.Fprintf(w, "pic_gallery_public_gallery_detail_login_blocks_total %d\n", m.PublicGalleryDetailLoginBlockTotal())
		_, _ = fmt.Fprintf(w, "# HELP pic_gallery_process_start_time_seconds Process start time.\n")
		_, _ = fmt.Fprintf(w, "# TYPE pic_gallery_process_start_time_seconds gauge\n")
		_, _ = fmt.Fprintf(w, "pic_gallery_process_start_time_seconds %d\n", m.processStartUnix)
	})
}
