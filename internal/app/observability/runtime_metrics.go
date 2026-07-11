package observability

import (
	"errors"
	"math"
	runtimelib "runtime"
	runtimemetrics "runtime/metrics"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	sampleInterval     = 5 * time.Second
	maxRuntimeBuckets  = 720
	runtimeRingSize    = maxRuntimeBuckets + 1
	maxSnapshotRoutes  = 10
	latencyBucketCount = 11

	RuntimeStateCollecting = "collecting"
	RuntimeStateHealthy    = "healthy"
	RuntimeStatePressured  = "pressured"
	RuntimeStateCritical   = "critical"

	StateReasonServerErrors = "server_error_rate"
	StateReasonLatency      = "p95_latency"
	StateReasonCPU          = "cpu_pressure"
)

var (
	errInvalidMonitoringWindow = errors.New("invalid monitoring window")
	latencyUpperBoundsMS       = [...]int64{10, 25, 50, 100, 250, 500, 1000, 2000, 5000, 10000}
)

type Window string

const (
	Window5m  Window = "5m"
	Window15m Window = "15m"
	Window30m Window = "30m"
	Window60m Window = "60m"
)

type RuntimeMetricsOptions struct {
	Now     func() time.Time
	Sampler func() RuntimeSample
}

type RuntimeSample struct {
	CPUPercent *float64
	HeapBytes  uint64
	SysBytes   uint64
	Goroutines int
	GCPauseMS  float64
}

type RuntimeCurrent struct {
	Inflight        int64    `json:"inflight"`
	PeakInflight    int64    `json:"peak_inflight"`
	QPS             float64  `json:"qps"`
	P50MS           int64    `json:"p50_ms"`
	P95MS           int64    `json:"p95_ms"`
	P99MS           int64    `json:"p99_ms"`
	ServerErrorRate float64  `json:"server_error_rate"`
	CPUPercent      *float64 `json:"cpu_percent"`
	HeapBytes       uint64   `json:"heap_bytes"`
	SysBytes        uint64   `json:"sys_bytes"`
	Goroutines      int      `json:"goroutines"`
	GCPauseMS       float64  `json:"gc_pause_ms"`
}

type RuntimePoint struct {
	At              time.Time `json:"at"`
	QPS             float64   `json:"qps"`
	PeakInflight    int64     `json:"peak_inflight"`
	P50MS           int64     `json:"p50_ms"`
	P95MS           int64     `json:"p95_ms"`
	P99MS           int64     `json:"p99_ms"`
	ServerErrorRate float64   `json:"server_error_rate"`
	CPUPercent      *float64  `json:"cpu_percent"`
	HeapBytes       uint64    `json:"heap_bytes"`
	SysBytes        uint64    `json:"sys_bytes"`
	Goroutines      int       `json:"goroutines"`
}

type RuntimeStatuses struct {
	Total       uint64 `json:"total"`
	Success     uint64 `json:"success"`
	Redirect    uint64 `json:"redirect"`
	ClientError uint64 `json:"client_error"`
	ServerError uint64 `json:"server_error"`
}

type RuntimeRoute struct {
	Route           string  `json:"route"`
	Requests        uint64  `json:"requests"`
	QPS             float64 `json:"qps"`
	P95MS           int64   `json:"p95_ms"`
	ClientErrorRate float64 `json:"client_error_rate"`
	ServerErrorRate float64 `json:"server_error_rate"`
}

type RuntimeSnapshot struct {
	GeneratedAt           time.Time       `json:"generated_at"`
	Window                Window          `json:"window"`
	SampleIntervalSeconds int             `json:"sample_interval_seconds"`
	Collecting            bool            `json:"collecting"`
	UptimeSeconds         int64           `json:"uptime_seconds"`
	State                 string          `json:"state"`
	StateReasons          []string        `json:"state_reasons"`
	Current               RuntimeCurrent  `json:"current"`
	Series                []RuntimePoint  `json:"series"`
	Statuses              RuntimeStatuses `json:"statuses"`
	Routes                []RuntimeRoute  `json:"routes"`
}

type RuntimeMetrics struct {
	mu               sync.Mutex
	now              func() time.Time
	sampler          func() RuntimeSample
	startedAt        time.Time
	buckets          [runtimeRingSize]runtimeBucket
	inflight         atomic.Int64
	lastSampleBucket time.Time
}

type runtimeBucket struct {
	at           time.Time
	statuses     RuntimeStatuses
	latency      [latencyBucketCount]uint64
	peakInflight int64
	routes       map[string]*runtimeRouteBucket
	runtime      RuntimeSample
	hasRuntime   bool
}

type runtimeRouteBucket struct {
	requests uint64
	statuses RuntimeStatuses
	latency  [latencyBucketCount]uint64
}

func NewRuntimeMetrics(options RuntimeMetricsOptions) *RuntimeMetrics {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	sampler := options.Sampler
	if sampler == nil {
		sampler = sampleGoRuntime
	}
	return &RuntimeMetrics{
		now:       now,
		sampler:   sampler,
		startedAt: now(),
	}
}

func (m *RuntimeMetrics) BeginRequest(method string) func(pattern string, status int, duration time.Duration) {
	now := m.now()
	m.sampleIfDue(now)
	current := m.inflight.Add(1)

	m.mu.Lock()
	bucket := m.bucketLocked(now)
	if current > bucket.peakInflight {
		bucket.peakInflight = current
	}
	m.mu.Unlock()

	var once sync.Once
	return func(pattern string, status int, duration time.Duration) {
		once.Do(func() {
			m.inflight.Add(-1)
			m.observeRequest(method, pattern, status, duration)
		})
	}
}

func (m *RuntimeMetrics) RecordRuntimeSample(sample RuntimeSample) {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	bucket := m.bucketLocked(now)
	bucket.runtime = cloneRuntimeSample(sample)
	bucket.hasRuntime = true
	m.lastSampleBucket = now.Truncate(sampleInterval)
}

func (m *RuntimeMetrics) Snapshot(window Window) (RuntimeSnapshot, error) {
	duration, ok := windowDuration(window)
	if !ok {
		return RuntimeSnapshot{}, errInvalidMonitoringWindow
	}
	now := m.now()
	m.sampleIfDue(now)
	currentBucketAt := now.Truncate(sampleInterval)
	cutoff := currentBucketAt.Add(-duration)

	m.mu.Lock()
	defer m.mu.Unlock()

	buckets := make([]runtimeBucket, 0, int(duration/sampleInterval))
	var latestSample RuntimeSample
	var latestSampleAt time.Time
	for index := range m.buckets {
		bucket := m.buckets[index]
		if bucket.at.IsZero() {
			continue
		}
		if bucket.hasRuntime && !bucket.at.After(currentBucketAt) && bucket.at.After(latestSampleAt) {
			latestSample = cloneRuntimeSample(bucket.runtime)
			latestSampleAt = bucket.at
		}
		if bucket.at.Before(cutoff) || !bucket.at.Before(currentBucketAt) {
			continue
		}
		buckets = append(buckets, cloneRuntimeBucket(bucket))
	}
	sort.Slice(buckets, func(left, right int) bool {
		return buckets[left].at.Before(buckets[right].at)
	})

	snapshot := aggregateRuntimeSnapshot(now, m.startedAt, window, buckets, latestSample)
	snapshot.Current.Inflight = m.inflight.Load()
	if snapshot.Current.Inflight > snapshot.Current.PeakInflight {
		snapshot.Current.PeakInflight = snapshot.Current.Inflight
	}
	return snapshot, nil
}

func (m *RuntimeMetrics) observeRequest(method, pattern string, status int, duration time.Duration) {
	now := m.now()
	route := normalizedRoute(method, pattern)
	m.mu.Lock()
	defer m.mu.Unlock()
	bucket := m.bucketLocked(now)
	addStatus(&bucket.statuses, status)
	addLatency(&bucket.latency, duration)
	if bucket.routes == nil {
		bucket.routes = make(map[string]*runtimeRouteBucket)
	}
	routeBucket := bucket.routes[route]
	if routeBucket == nil {
		routeBucket = &runtimeRouteBucket{}
		bucket.routes[route] = routeBucket
	}
	routeBucket.requests++
	addStatus(&routeBucket.statuses, status)
	addLatency(&routeBucket.latency, duration)
}

func (m *RuntimeMetrics) sampleIfDue(now time.Time) {
	bucketAt := now.Truncate(sampleInterval)
	m.mu.Lock()
	if m.lastSampleBucket.Equal(bucketAt) {
		m.mu.Unlock()
		return
	}
	m.lastSampleBucket = bucketAt
	m.mu.Unlock()

	sample := m.sampler()
	m.mu.Lock()
	bucket := m.bucketLocked(now)
	bucket.runtime = cloneRuntimeSample(sample)
	bucket.hasRuntime = true
	m.mu.Unlock()
}

func (m *RuntimeMetrics) bucketLocked(at time.Time) *runtimeBucket {
	at = at.Truncate(sampleInterval)
	index := int((at.Unix() / int64(sampleInterval/time.Second)) % runtimeRingSize)
	if index < 0 {
		index += runtimeRingSize
	}
	bucket := &m.buckets[index]
	if !bucket.at.Equal(at) {
		*bucket = runtimeBucket{at: at, routes: make(map[string]*runtimeRouteBucket)}
	}
	return bucket
}

func aggregateRuntimeSnapshot(now, startedAt time.Time, window Window, buckets []runtimeBucket, latest RuntimeSample) RuntimeSnapshot {
	snapshot := RuntimeSnapshot{
		GeneratedAt:           now.UTC(),
		Window:                window,
		SampleIntervalSeconds: int(sampleInterval / time.Second),
		Collecting:            len(buckets) == 0,
		UptimeSeconds:         maxInt64(0, int64(now.Sub(startedAt)/time.Second)),
		StateReasons:          []string{},
		Series:                make([]RuntimePoint, 0, len(buckets)),
		Routes:                []RuntimeRoute{},
	}
	aggregateLatency := [latencyBucketCount]uint64{}
	routeTotals := make(map[string]*runtimeRouteBucket)
	for _, bucket := range buckets {
		mergeStatuses(&snapshot.Statuses, bucket.statuses)
		mergeLatency(&aggregateLatency, bucket.latency)
		if bucket.peakInflight > snapshot.Current.PeakInflight {
			snapshot.Current.PeakInflight = bucket.peakInflight
		}
		for route, values := range bucket.routes {
			total := routeTotals[route]
			if total == nil {
				total = &runtimeRouteBucket{}
				routeTotals[route] = total
			}
			total.requests += values.requests
			mergeStatuses(&total.statuses, values.statuses)
			mergeLatency(&total.latency, values.latency)
		}
		snapshot.Series = append(snapshot.Series, runtimePointFromBucket(bucket))
	}

	elapsedSeconds := float64(len(buckets)) * sampleInterval.Seconds()
	if elapsedSeconds > 0 {
		snapshot.Current.QPS = float64(snapshot.Statuses.Total) / elapsedSeconds
	}
	snapshot.Current.P50MS = percentileLatency(aggregateLatency, 0.50)
	snapshot.Current.P95MS = percentileLatency(aggregateLatency, 0.95)
	snapshot.Current.P99MS = percentileLatency(aggregateLatency, 0.99)
	snapshot.Current.ServerErrorRate = percentage(snapshot.Statuses.ServerError, snapshot.Statuses.Total)
	snapshot.Current.CPUPercent = cloneFloat64Pointer(latest.CPUPercent)
	snapshot.Current.HeapBytes = latest.HeapBytes
	snapshot.Current.SysBytes = latest.SysBytes
	snapshot.Current.Goroutines = latest.Goroutines
	snapshot.Current.GCPauseMS = latest.GCPauseMS
	snapshot.Routes = aggregateRoutes(routeTotals, elapsedSeconds)
	snapshot.State, snapshot.StateReasons = runtimeState(snapshot.Collecting, snapshot.Current)
	return snapshot
}

func runtimePointFromBucket(bucket runtimeBucket) RuntimePoint {
	return RuntimePoint{
		At:              bucket.at.UTC(),
		QPS:             float64(bucket.statuses.Total) / sampleInterval.Seconds(),
		PeakInflight:    bucket.peakInflight,
		P50MS:           percentileLatency(bucket.latency, 0.50),
		P95MS:           percentileLatency(bucket.latency, 0.95),
		P99MS:           percentileLatency(bucket.latency, 0.99),
		ServerErrorRate: percentage(bucket.statuses.ServerError, bucket.statuses.Total),
		CPUPercent:      cloneFloat64Pointer(bucket.runtime.CPUPercent),
		HeapBytes:       bucket.runtime.HeapBytes,
		SysBytes:        bucket.runtime.SysBytes,
		Goroutines:      bucket.runtime.Goroutines,
	}
}

func aggregateRoutes(totals map[string]*runtimeRouteBucket, elapsedSeconds float64) []RuntimeRoute {
	routes := make([]RuntimeRoute, 0, len(totals))
	for route, values := range totals {
		qps := 0.0
		if elapsedSeconds > 0 {
			qps = float64(values.requests) / elapsedSeconds
		}
		routes = append(routes, RuntimeRoute{
			Route:           route,
			Requests:        values.requests,
			QPS:             qps,
			P95MS:           percentileLatency(values.latency, 0.95),
			ClientErrorRate: percentage(values.statuses.ClientError, values.statuses.Total),
			ServerErrorRate: percentage(values.statuses.ServerError, values.statuses.Total),
		})
	}
	sort.Slice(routes, func(left, right int) bool {
		if routes[left].Requests == routes[right].Requests {
			return routes[left].Route < routes[right].Route
		}
		return routes[left].Requests > routes[right].Requests
	})
	if len(routes) > maxSnapshotRoutes {
		routes = routes[:maxSnapshotRoutes]
	}
	return routes
}

func runtimeState(collecting bool, current RuntimeCurrent) (string, []string) {
	if collecting {
		return RuntimeStateCollecting, []string{}
	}
	reasons := make([]string, 0, 3)
	if current.ServerErrorRate >= 1 {
		reasons = append(reasons, StateReasonServerErrors)
	}
	if current.P95MS >= 1000 {
		reasons = append(reasons, StateReasonLatency)
	}
	if current.CPUPercent != nil && *current.CPUPercent >= 75 {
		reasons = append(reasons, StateReasonCPU)
	}
	critical := current.ServerErrorRate > 5 || current.P95MS > 2000 ||
		(current.CPUPercent != nil && *current.CPUPercent > 90)
	if critical {
		return RuntimeStateCritical, reasons
	}
	if len(reasons) > 0 {
		return RuntimeStatePressured, reasons
	}
	return RuntimeStateHealthy, reasons
}

func windowDuration(window Window) (time.Duration, bool) {
	switch window {
	case Window5m:
		return 5 * time.Minute, true
	case Window15m:
		return 15 * time.Minute, true
	case Window30m:
		return 30 * time.Minute, true
	case Window60m:
		return 60 * time.Minute, true
	default:
		return 0, false
	}
}

func normalizedRoute(method, pattern string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "UNKNOWN"
	}
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		pattern = "unknown"
	}
	if fields := strings.Fields(pattern); len(fields) > 1 && strings.EqualFold(fields[0], method) {
		return method + " " + strings.Join(fields[1:], " ")
	}
	return method + " " + pattern
}

func addStatus(statuses *RuntimeStatuses, status int) {
	statuses.Total++
	switch {
	case status >= 500:
		statuses.ServerError++
	case status >= 400:
		statuses.ClientError++
	case status >= 300:
		statuses.Redirect++
	default:
		statuses.Success++
	}
}

func mergeStatuses(target *RuntimeStatuses, source RuntimeStatuses) {
	target.Total += source.Total
	target.Success += source.Success
	target.Redirect += source.Redirect
	target.ClientError += source.ClientError
	target.ServerError += source.ServerError
}

func addLatency(histogram *[latencyBucketCount]uint64, duration time.Duration) {
	milliseconds := duration.Milliseconds()
	index := len(latencyUpperBoundsMS)
	for candidate, upper := range latencyUpperBoundsMS {
		if milliseconds <= upper {
			index = candidate
			break
		}
	}
	histogram[index]++
}

func mergeLatency(target *[latencyBucketCount]uint64, source [latencyBucketCount]uint64) {
	for index, count := range source {
		target[index] += count
	}
}

func percentileLatency(histogram [latencyBucketCount]uint64, percentile float64) int64 {
	var total uint64
	for _, count := range histogram {
		total += count
	}
	if total == 0 {
		return 0
	}
	rank := uint64(math.Ceil(float64(total) * percentile))
	var seen uint64
	for index, count := range histogram {
		seen += count
		if seen < rank {
			continue
		}
		if index < len(latencyUpperBoundsMS) {
			return latencyUpperBoundsMS[index]
		}
		return latencyUpperBoundsMS[len(latencyUpperBoundsMS)-1]
	}
	return 0
}

func percentage(part, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) * 100 / float64(total)
}

func cloneRuntimeBucket(source runtimeBucket) runtimeBucket {
	cloned := source
	cloned.runtime = cloneRuntimeSample(source.runtime)
	cloned.routes = make(map[string]*runtimeRouteBucket, len(source.routes))
	for key, value := range source.routes {
		copyValue := *value
		cloned.routes[key] = &copyValue
	}
	return cloned
}

func cloneRuntimeSample(source RuntimeSample) RuntimeSample {
	source.CPUPercent = cloneFloat64Pointer(source.CPUPercent)
	return source
}

func cloneFloat64Pointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

var goCPUSamples = []runtimemetrics.Sample{
	{Name: "/cpu/classes/total:cpu-seconds"},
	{Name: "/cpu/classes/idle:cpu-seconds"},
}

var goCPUSampleState struct {
	sync.Mutex
	total float64
	idle  float64
	ready bool
}

func sampleGoRuntime() RuntimeSample {
	var memory runtimelib.MemStats
	runtimelib.ReadMemStats(&memory)
	runtimemetrics.Read(goCPUSamples)

	total := goCPUSamples[0].Value.Float64()
	idle := goCPUSamples[1].Value.Float64()
	goCPUSampleState.Lock()
	var cpu *float64
	if goCPUSampleState.ready {
		totalDelta := total - goCPUSampleState.total
		idleDelta := idle - goCPUSampleState.idle
		if totalDelta > 0 {
			value := math.Max(0, math.Min(100, (totalDelta-idleDelta)*100/totalDelta))
			cpu = &value
		}
	}
	goCPUSampleState.total = total
	goCPUSampleState.idle = idle
	goCPUSampleState.ready = true
	goCPUSampleState.Unlock()

	return RuntimeSample{
		CPUPercent: cpu,
		HeapBytes:  memory.HeapAlloc,
		SysBytes:   memory.Sys,
		Goroutines: runtimelib.NumGoroutine(),
		GCPauseMS:  float64(memory.PauseTotalNs) / float64(time.Millisecond),
	}
}
