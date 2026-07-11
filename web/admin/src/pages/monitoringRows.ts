import type {
  AdminMonitoringProvider,
  AdminMonitoringRoute,
  AdminMonitoringSnapshot,
  MonitoringWindow,
  ReadinessCheck,
} from '../../../shared/api-types'
import { readinessRows } from './readinessRows'

type SemanticTone = 'success' | 'warning' | 'danger' | 'neutral'
type MetricTone = 'good' | 'warn' | 'bad' | 'neutral'

export const monitoringWindows: MonitoringWindow[] = ['5m', '15m', '30m', '60m']

export type MonitoringRouteRow = AdminMonitoringRoute & {
  requestsLabel: string
  qpsLabel: string
  p95Label: string
  clientErrorLabel: string
  serverErrorLabel: string
  statusTone: SemanticTone
  statusLabel: string
}

export type MonitoringDiagnosticRow = {
  key: string
  label: string
  detail: string
  statusLabel: string
  statusTone: SemanticTone
  actionHref: string
  actionLabel: string
}

export function monitoringStateView(state: string): { label: string; tone: SemanticTone } {
  switch (state) {
    case 'healthy':
      return { label: '运行正常', tone: 'success' }
    case 'pressured':
      return { label: '存在压力', tone: 'warning' }
    case 'critical':
      return { label: '运行异常', tone: 'danger' }
    case 'collecting':
      return { label: '采集中', tone: 'neutral' }
    default:
      return { label: state.trim() || '状态未知', tone: 'neutral' }
  }
}

export function formatMonitoringBytes(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value) || value < 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let normalized = value
  let unit = 0
  while (normalized >= 1024 && unit < units.length - 1) {
    normalized /= 1024
    unit += 1
  }
  return `${normalized.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`
}

export function formatMonitoringDuration(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value) || value < 0) return '-'
  return value >= 1000 ? `${(value / 1000).toFixed(2)}s` : `${Math.round(value)}ms`
}

export function formatMonitoringPercent(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '-'
  return `${value.toLocaleString('en-US', { minimumFractionDigits: 1, maximumFractionDigits: 2 })}%`
}

export function formatMonitoringQPS(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '-'
  return value.toFixed(2)
}

export function formatMonitoringUptime(seconds: number | null | undefined): string {
  if (seconds == null || !Number.isFinite(seconds) || seconds < 0) return '-'
  const totalMinutes = Math.floor(seconds / 60)
  const days = Math.floor(totalMinutes / 1440)
  const hours = Math.floor((totalMinutes % 1440) / 60)
  const minutes = totalMinutes % 60
  if (days > 0) return `${days}d ${String(hours).padStart(2, '0')}h`
  if (hours > 0) return `${hours}h ${String(minutes).padStart(2, '0')}m`
  return `${minutes}m`
}

export function monitoringMetricRows(snapshot: AdminMonitoringSnapshot) {
  const state = monitoringStateView(snapshot.state)
  const current = snapshot.current
  return [
    {
      label: 'API 状态',
      value: state.label,
      trend: snapshot.collecting ? '等待完整采样周期' : `运行 ${formatMonitoringUptime(snapshot.uptime_seconds)}`,
      tone: stateMetricTone(state.tone),
    },
    { label: '当前 QPS', value: formatMonitoringQPS(current.qps), trend: `${snapshot.window} 请求速率`, tone: 'neutral' as MetricTone },
    { label: '请求并发', value: String(current.inflight), trend: `窗口峰值 ${current.peak_inflight}`, tone: current.inflight > 0 ? 'neutral' as MetricTone : 'good' as MetricTone },
    { label: 'P95 延迟', value: formatMonitoringDuration(current.p95_ms), trend: `P50 ${formatMonitoringDuration(current.p50_ms)} · P99 ${formatMonitoringDuration(current.p99_ms)}`, tone: latencyTone(current.p95_ms) },
    { label: '5xx 错误率', value: formatMonitoringPercent(current.server_error_rate), trend: `${snapshot.statuses.server_error} / ${snapshot.statuses.total} 请求`, tone: errorTone(current.server_error_rate) },
    { label: 'API CPU', value: formatMonitoringPercent(current.cpu_percent), trend: current.cpu_percent == null ? '首个采样周期后可用' : 'Go 调度容量占用', tone: cpuTone(current.cpu_percent) },
    { label: 'Go Heap', value: formatMonitoringBytes(current.heap_bytes), trend: `Runtime ${formatMonitoringBytes(current.sys_bytes)}`, tone: 'neutral' as MetricTone },
    { label: 'Goroutine', value: String(current.goroutines), trend: `GC 累计暂停 ${formatMonitoringDuration(current.gc_pause_ms)}`, tone: 'neutral' as MetricTone },
  ]
}

export function monitoringRouteRows(routes: AdminMonitoringRoute[]): MonitoringRouteRow[] {
  return routes
    .slice()
    .sort((left, right) => right.requests - left.requests || left.route.localeCompare(right.route))
    .map((route) => {
      const status = routeStatus(route)
      return {
        ...route,
        requestsLabel: route.requests.toLocaleString('en-US'),
        qpsLabel: formatMonitoringQPS(route.qps),
        p95Label: formatMonitoringDuration(route.p95_ms),
        clientErrorLabel: formatMonitoringPercent(route.client_error_rate),
        serverErrorLabel: formatMonitoringPercent(route.server_error_rate),
        statusTone: status.tone,
        statusLabel: status.label,
      }
    })
}

export function monitoringDiagnostics(
  providers: AdminMonitoringProvider[],
  checks: ReadinessCheck[],
): MonitoringDiagnosticRow[] {
  const providerRows = providers
    .filter((provider) => provider.enabled && provider.status !== 'healthy')
    .sort((left, right) => providerSeverity(right.status) - providerSeverity(left.status) || left.provider_code.localeCompare(right.provider_code))
    .map((provider): MonitoringDiagnosticRow => {
      const critical = providerSeverity(provider.status) >= 2
      return {
        key: `provider-${provider.provider_code}`,
        label: provider.provider_code,
        detail: `${provider.provider_type} Provider 当前状态为 ${provider.status || 'unknown'}`,
        statusLabel: critical ? '不可用' : '降级',
        statusTone: critical ? 'danger' : 'warning',
        actionHref: '#/access-accounts',
        actionLabel: '检查账号',
      }
    })
  const readiness = readinessRows(checks)
    .filter((row) => row.rawStatus === 'fail' || row.rawStatus === 'warn')
    .map((row): MonitoringDiagnosticRow => ({
      key: `readiness-${row.key}`,
      label: row.label,
      detail: row.detail,
      statusLabel: row.status,
      statusTone: row.statusTone,
      actionHref: row.actionHref,
      actionLabel: row.actionLabel,
    }))
  return [...providerRows, ...readiness]
}

function routeStatus(route: AdminMonitoringRoute): { label: string; tone: SemanticTone } {
  if (route.server_error_rate > 5 || route.p95_ms > 2000) return { label: '异常', tone: 'danger' }
  if (route.server_error_rate >= 1 || route.p95_ms >= 1000 || route.client_error_rate >= 10) return { label: '承压', tone: 'warning' }
  return { label: '正常', tone: 'success' }
}

function providerSeverity(status: string) {
  switch (status) {
    case 'down':
    case 'error':
    case 'unhealthy':
    case 'unavailable':
      return 2
    case 'degraded':
      return 1
    default:
      return 0
  }
}

function stateMetricTone(tone: SemanticTone): MetricTone {
  if (tone === 'success') return 'good'
  if (tone === 'warning') return 'warn'
  if (tone === 'danger') return 'bad'
  return 'neutral'
}

function latencyTone(value: number): MetricTone {
  if (value > 2000) return 'bad'
  if (value >= 1000) return 'warn'
  return 'good'
}

function errorTone(value: number): MetricTone {
  if (value > 5) return 'bad'
  if (value >= 1) return 'warn'
  return 'good'
}

function cpuTone(value: number | null): MetricTone {
  if (value == null) return 'neutral'
  if (value > 90) return 'bad'
  if (value >= 75) return 'warn'
  return 'good'
}
