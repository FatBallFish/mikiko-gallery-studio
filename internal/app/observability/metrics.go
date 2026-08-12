package observability

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Metrics struct {
	httpRequestsTotal                  atomic.Uint64
	publicGalleryListViewsTotal        atomic.Uint64
	publicGalleryDetailLoginBlockTotal atomic.Uint64
	processStartUnix                   int64
	runtime                            *RuntimeMetrics
	multimedia                         multimediaMetrics
}

type multimediaMetrics struct {
	mu                sync.Mutex
	videoStages       map[string]uint64
	artifactBytes     map[string]uint64
	settlements       map[string]uint64
	uploadBytes       map[string]uint64
	derivativeBytes   map[string]uint64
	canvasSaves       map[string]uint64
	objectBytes       map[string]uint64
	tempDiskPercent   uint64
	tempDiskFreeBytes uint64
}

var defaultMetrics = NewMetrics()

func NewMetrics() *Metrics {
	return &Metrics{
		processStartUnix: time.Now().Unix(),
		runtime:          NewRuntimeMetrics(RuntimeMetricsOptions{}),
		multimedia: multimediaMetrics{
			videoStages: map[string]uint64{}, artifactBytes: map[string]uint64{}, settlements: map[string]uint64{},
			uploadBytes: map[string]uint64{}, derivativeBytes: map[string]uint64{}, canvasSaves: map[string]uint64{}, objectBytes: map[string]uint64{},
		},
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

func (m *Metrics) RecordVideoStage(stage, result string) {
	m.addMultimedia(m.multimedia.videoStages, boundedLabel(stage, videoStages), boundedLabel(result, operationResults), 1)
}

func (m *Metrics) RecordArtifactTransfer(mediaType, result string, bytes int64) {
	m.addMultimedia(m.multimedia.artifactBytes, boundedLabel(mediaType, mediaTypes), boundedLabel(result, operationResults), positiveBytes(bytes))
}

func (m *Metrics) RecordSettlement(kind, result string) {
	m.addMultimedia(m.multimedia.settlements, boundedLabel(kind, settlementKinds), boundedLabel(result, operationResults), 1)
}

func (m *Metrics) RecordUpload(stage, result string, bytes int64) {
	m.addMultimedia(m.multimedia.uploadBytes, boundedLabel(stage, uploadStages), boundedLabel(result, operationResults), positiveBytes(bytes))
}

func (m *Metrics) RecordDerivative(kind, result string, bytes int64) {
	m.addMultimedia(m.multimedia.derivativeBytes, boundedLabel(kind, derivativeKinds), boundedLabel(result, operationResults), positiveBytes(bytes))
}

func (m *Metrics) RecordCanvasSave(result string) {
	m.multimedia.mu.Lock()
	m.multimedia.canvasSaves[boundedLabel(result, operationResults)]++
	m.multimedia.mu.Unlock()
}

func (m *Metrics) SetTemporaryDisk(usedPercent int, freeBytes int64) {
	if usedPercent < 0 {
		usedPercent = 0
	}
	if usedPercent > 100 {
		usedPercent = 100
	}
	m.multimedia.mu.Lock()
	m.multimedia.tempDiskPercent = uint64(usedPercent)
	m.multimedia.tempDiskFreeBytes = positiveBytes(freeBytes)
	m.multimedia.mu.Unlock()
}

func (m *Metrics) AddObjectBytes(operation string, bytes int64) {
	m.multimedia.mu.Lock()
	m.multimedia.objectBytes[boundedLabel(operation, objectOperations)] += positiveBytes(bytes)
	m.multimedia.mu.Unlock()
}

func (m *Metrics) addMultimedia(target map[string]uint64, first, second string, value uint64) {
	m.multimedia.mu.Lock()
	target[first+"\x00"+second] += value
	m.multimedia.mu.Unlock()
}

func positiveBytes(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}

var (
	videoStages      = labelSet("queued", "submitting", "reconciling", "provider_queued", "provider_running", "artifact_pending", "recovery_required", "settling", "succeeded", "failed", "cancelled")
	operationResults = labelSet("success", "retry", "failed", "paused", "cancelled")
	mediaTypes       = labelSet("image", "video", "audio")
	settlementKinds  = labelSet("image", "video", "payment")
	uploadStages     = labelSet("initialize", "part", "complete", "abort", "expire")
	derivativeKinds  = labelSet("thumbnail_320", "thumbnail_640", "preview_1280", "poster", "hover_preview", "proxy", "waveform")
	objectOperations = labelSet("read", "written", "deleted")
)

func labelSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func boundedLabel(value string, allowed map[string]struct{}) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if _, ok := allowed[value]; ok {
		return value
	}
	return "unknown"
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
		m.writeMultimedia(w)
	})
}

func (m *Metrics) writeMultimedia(w http.ResponseWriter) {
	m.multimedia.mu.Lock()
	defer m.multimedia.mu.Unlock()
	writeTwoLabelMetric(w, "pic_gallery_video_stage_total", "stage", "result", m.multimedia.videoStages)
	writeTwoLabelMetric(w, "pic_gallery_artifact_transfer_bytes_total", "media_type", "result", m.multimedia.artifactBytes)
	writeTwoLabelMetric(w, "pic_gallery_settlement_total", "kind", "result", m.multimedia.settlements)
	writeTwoLabelMetric(w, "pic_gallery_upload_bytes_total", "stage", "result", m.multimedia.uploadBytes)
	writeTwoLabelMetric(w, "pic_gallery_media_derivative_bytes_total", "kind", "result", m.multimedia.derivativeBytes)
	writeOneLabelMetric(w, "pic_gallery_canvas_save_total", "result", m.multimedia.canvasSaves)
	writeOneLabelMetric(w, "pic_gallery_object_bytes_total", "operation", m.multimedia.objectBytes)
	_, _ = fmt.Fprintf(w, "pic_gallery_worker_temporary_disk_used_percent %d\n", m.multimedia.tempDiskPercent)
	_, _ = fmt.Fprintf(w, "pic_gallery_worker_temporary_disk_free_bytes %d\n", m.multimedia.tempDiskFreeBytes)
}

func writeTwoLabelMetric(w http.ResponseWriter, name, firstName, secondName string, values map[string]uint64) {
	keys := sortedMetricKeys(values)
	for _, key := range keys {
		parts := strings.SplitN(key, "\x00", 2)
		_, _ = fmt.Fprintf(w, "%s{%s=%q,%s=%q} %d\n", name, firstName, parts[0], secondName, parts[1], values[key])
	}
}

func writeOneLabelMetric(w http.ResponseWriter, name, label string, values map[string]uint64) {
	keys := sortedMetricKeys(values)
	for _, key := range keys {
		_, _ = fmt.Fprintf(w, "%s{%s=%q} %d\n", name, label, key, values[key])
	}
}

func sortedMetricKeys(values map[string]uint64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
