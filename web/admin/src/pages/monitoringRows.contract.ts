import type { AdminMonitoringSnapshot, ReadinessCheck } from '../../../shared/api-types'
import {
  formatMonitoringBytes,
  formatMonitoringDuration,
  formatMonitoringPercent,
  formatMonitoringQPS,
  formatMonitoringUptime,
  monitoringDiagnostics,
  monitoringMetricRows,
  monitoringRouteRows,
  monitoringStateView,
  monitoringWindows,
} from './monitoringRows'

const snapshot: AdminMonitoringSnapshot = {
  generated_at: '2026-07-12T00:10:00Z',
  window: '5m',
  sample_interval_seconds: 5,
  collecting: false,
  uptime_seconds: 3723,
  state: 'pressured',
  state_reasons: ['p95_latency'],
  current: {
    inflight: 3,
    peak_inflight: 8,
    qps: 12.345,
    p50_ms: 100,
    p95_ms: 1200,
    p99_ms: 2000,
    server_error_rate: 1.25,
    cpu_percent: 76.4,
    heap_bytes: 32 * 1024 * 1024,
    sys_bytes: 64 * 1024 * 1024,
    goroutines: 42,
    gc_pause_ms: 14.2,
  },
  series: [
    { at: '2026-07-12T00:09:55Z', qps: 10, peak_inflight: 5, p50_ms: 80, p95_ms: 800, p99_ms: 1000, server_error_rate: 0, cpu_percent: 50, heap_bytes: 30, sys_bytes: 60, goroutines: 40 },
    { at: '2026-07-12T00:10:00Z', qps: 12, peak_inflight: 8, p50_ms: 100, p95_ms: 1200, p99_ms: 2000, server_error_rate: 1.25, cpu_percent: 76.4, heap_bytes: 32, sys_bytes: 64, goroutines: 42 },
  ],
  statuses: { total: 100, success: 90, redirect: 1, client_error: 7, server_error: 2 },
  routes: [
    { route: 'GET /api/slow', requests: 5, qps: 0.2, p95_ms: 2000, client_error_rate: 0, server_error_rate: 20 },
    { route: 'GET /api/popular', requests: 25, qps: 1, p95_ms: 250, client_error_rate: 4, server_error_rate: 0 },
    { route: 'POST /api/tasks', requests: 25, qps: 1, p95_ms: 1000, client_error_rate: 0, server_error_rate: 2 },
  ],
  providers: [
    { provider_code: 'openai-main', provider_type: 'openai', status: 'healthy', enabled: true },
    { provider_code: 'backup', provider_type: 'openrouter', status: 'down', enabled: true },
  ],
}

assertEqual(monitoringWindows.join(','), '5m,15m,30m,60m', 'window order')
assertEqual(monitoringStateView('healthy').label, '运行正常', 'healthy label')
assertEqual(monitoringStateView('pressured').tone, 'warning', 'pressured tone')
assertEqual(monitoringStateView('critical').tone, 'danger', 'critical tone')
assertEqual(monitoringStateView('collecting').label, '采集中', 'collecting label')
assertEqual(formatMonitoringBytes(32 * 1024 * 1024), '32.0 MB', 'byte format')
assertEqual(formatMonitoringBytes(null), '-', 'unavailable byte format')
assertEqual(formatMonitoringDuration(1200), '1.20s', 'duration seconds')
assertEqual(formatMonitoringDuration(250), '250ms', 'duration milliseconds')
assertEqual(formatMonitoringPercent(null), '-', 'unavailable percent')
assertEqual(formatMonitoringPercent(1.25), '1.25%', 'percent format')
assertEqual(formatMonitoringQPS(12.345), '12.35', 'QPS format')
assertEqual(formatMonitoringUptime(3723), '1h 02m', 'uptime format')

const metrics = monitoringMetricRows(snapshot)
assertEqual(metrics.length, 8, 'metric count')
assertEqual(metrics[1]?.value ?? '', '12.35', 'QPS metric')
assertEqual(metrics[2]?.value ?? '', '3', 'inflight metric')
assertEqual(metrics[3]?.value ?? '', '1.20s', 'P95 metric')
assertEqual(metrics[5]?.value ?? '', '76.4%', 'CPU metric')

const routes = monitoringRouteRows(snapshot.routes)
assertEqual(routes.map((row) => row.route).join(','), 'GET /api/popular,POST /api/tasks,GET /api/slow', 'route sort')
assertEqual(routes[0]?.requestsLabel ?? '', '25', 'route request format')
assertEqual(routes[2]?.statusTone ?? '', 'danger', 'route critical tone')

const readiness: ReadinessCheck[] = [
  { key: 'storage', label: '对象存储', status: 'pass', detail: 'ok', blocking: false },
  { key: 'cashier', label: '支付配置', status: 'warn', detail: '未启用', blocking: false, fix_route: 'cashier', fix_action: '去配置' },
]
const diagnostics = monitoringDiagnostics(snapshot.providers, readiness)
assertEqual(diagnostics.length, 2, 'exception-only diagnostics')
assertEqual(diagnostics[0]?.key ?? '', 'provider-backup', 'provider diagnostic first')
assertEqual(diagnostics[0]?.actionHref ?? '', '#/access-accounts', 'provider action')
assertEqual(diagnostics[1]?.actionHref ?? '', '#/cashier-config', 'readiness action')

const timestamps = snapshot.series.map((point) => point.at)
assertEqual(timestamps.join(','), '2026-07-12T00:09:55Z,2026-07-12T00:10:00Z', 'real chronological series')

function assertEqual(actual: unknown, expected: unknown, name: string) {
  if (actual !== expected) {
    throw new Error(`${name}: expected ${String(expected)}, got ${String(actual)}`)
  }
}
