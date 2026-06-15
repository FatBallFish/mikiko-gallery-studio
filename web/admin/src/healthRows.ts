import type { ProviderHealth, ReadinessCheck } from '../../shared/api-types'
import { adminActionHref } from './adminRouteLinks'

export type HealthTone = 'success' | 'warning' | 'danger' | 'neutral'

export type HealthProviderRow = {
  key: string
  name: string
  statusLabel: string
  statusTone: HealthTone
  latencyLabel: string
  errorRate: string
  note: string
}

export type HealthReadinessRow = {
  key: string
  name: string
  statusLabel: string
  statusTone: HealthTone
  scope: string
  probeLabel: string
  detail: string
  actionHref: string
  actionLabel: string
}

const healthStatusLabels: Record<string, string> = {
  healthy: '健康',
  degraded: '降级',
  down: '不可用',
  pass: '通过',
  warn: '警告',
  fail: '失败',
}

export function healthStatusLabel(status?: string | null) {
  const normalized = normalizeStatus(status)
  return healthStatusLabels[normalized] ?? statusFallback(status)
}

export function healthStatusTone(status?: string | null): HealthTone {
  const normalized = normalizeStatus(status)
  if (normalized === 'healthy' || normalized === 'pass') return 'success'
  if (normalized === 'degraded' || normalized === 'warn') return 'warning'
  if (normalized === 'down' || normalized === 'fail') return 'danger'
  return 'neutral'
}

export function providerHealthValue(provider: ProviderHealth) {
  if (normalizeStatus(provider.status) === 'healthy') return healthStatusLabel(provider.status)
  return provider.note || healthStatusLabel(provider.status)
}

export function providerHealthWarn(provider: ProviderHealth) {
  return healthStatusTone(provider.status) !== 'success'
}

export function taskQueuePressure(providers: ProviderHealth[]) {
  return providers.find((item) => item.provider === 'Task Worker')?.note ?? '正常'
}

export function healthProviderRows(providers: ProviderHealth[]): HealthProviderRow[] {
  return providers.map((provider) => ({
    key: provider.provider_code ?? provider.provider,
    name: provider.provider,
    statusLabel: healthStatusLabel(provider.status),
    statusTone: healthStatusTone(provider.status),
    latencyLabel: `${provider.latency_ms}ms`,
    errorRate: provider.error_rate,
    note: provider.note || '-',
  }))
}

export function healthReadinessRows(checks: ReadinessCheck[]): HealthReadinessRow[] {
  return checks.map((check) => {
    return {
      key: check.key,
      name: check.label,
      statusLabel: healthStatusLabel(check.status),
      statusTone: healthStatusTone(check.status),
      scope: check.key,
      probeLabel: check.blocking ? '阻塞上线' : '上线检查',
      detail: check.detail || check.summary || '-',
      actionHref: adminActionHref(check.fix_route ?? check.action_route),
      actionLabel: check.fix_action ?? check.action_label ?? '查看',
    }
  })
}

export function healthRefreshTimeLabel(value?: string | null) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const year = date.getUTCFullYear()
  const month = String(date.getUTCMonth() + 1).padStart(2, '0')
  const day = String(date.getUTCDate()).padStart(2, '0')
  const hour = String(date.getUTCHours()).padStart(2, '0')
  const minute = String(date.getUTCMinutes()).padStart(2, '0')
  return `${year}/${month}/${day} ${hour}:${minute}`
}

export function refreshPolicyLabel(value: string) {
  if (value === '30s interval') return '每 30 秒巡检'
  return value.trim() || '按需刷新'
}

function normalizeStatus(status?: string | null) {
  return (status ?? '').trim().toLowerCase()
}

function statusFallback(status?: string | null) {
  const normalized = (status ?? '').trim()
  return normalized || '未知状态'
}
