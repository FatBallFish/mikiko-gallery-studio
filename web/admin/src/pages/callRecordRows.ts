import type { CallRecord, CallRecordResultImage, ModelProvider, RouteModel } from '../../../shared/api-types'
import { adminTaskTypeLabel } from './adminTaskTypes'

export type CallRecordRowModel = {
  id: string
  fullTaskId: string
  taskLabel: string
  taskDetail: string
  userLabel: string
  userDetail: string
  routeLabel: string
  routeDetail: string
  routeModelCode: string
  providerLabel: string
  providerDetail: string
  status: string
  statusLabel: string
  statusTone: 'success' | 'warning' | 'danger' | 'neutral'
  failureLabel: string
  failureDetail: string
  amountLabel: string
  costLabel: string
  marginLabel: string
  lifecycleLabel: string
  createdAt: string
  startedAtLabel: string
  finishedAtLabel: string
  updatedAtLabel: string
  upstreamSucceededAtLabel: string
  durationSeconds: number
  durationLabel: string
  previewURL: string | null
  resultImages: CallRecordResultImage[]
  prompt: string
}

export type CallRecordFilterOption = {
  value: string
  label: string
}

export const callRecordFilterCopy = {
  errorCode: {
    label: '错误码',
    placeholder: '选择或输入错误码',
  },
  sourceChannel: {
    label: '入口',
  },
  provider: {
    label: '上游渠道',
    placeholder: '选择上游渠道',
  },
  routeModel: {
    label: '路由模型',
  },
  userId: {
    label: '用户',
    placeholder: '用户 ID / 邮箱',
  },
  taskId: {
    label: '任务 ID',
    placeholder: '任务 ID',
  },
  createdAt: {
    label: '创建时间',
  },
} as const

export const callRecordStatusOptions = [
  { value: '', label: '全部状态' },
  { value: 'queued', label: '排队中' },
  { value: 'running', label: '执行中' },
  { value: 'succeeded', label: '成功' },
  { value: 'failed', label: '失败' },
  { value: 'canceled', label: '已取消' },
] as const

export const callRecordSourceChannelOptions = [
  { value: '', label: '全部入口' },
  { value: 'web', label: '用户 Web' },
  { value: 'open_api', label: 'Open API' },
  { value: 'openai_compatible', label: 'OpenAI 兼容' },
] as const

export const callRecordCommonErrorCodes = [
  '',
  'MODEL_ROUTE_NOT_FOUND',
  'MODEL_ROUTE_NO_CANDIDATE',
  'ROUTE_MODEL_PRICE_MISSING',
  'MODEL_ROUTE_NOT_VISIBLE',
  'BILLING_INSUFFICIENT_POINTS',
  'UPSTREAM_UNAVAILABLE',
] as const

export function callRecordRepair(errorCode?: string | null): { label: string; href: string } | null {
  if (errorCode === 'ROUTE_MODEL_PRICE_MISSING') return { label: '配置价格', href: '#/pricing' }
  if (errorCode === 'MODEL_ROUTE_NO_CANDIDATE' || errorCode === 'MODEL_ROUTE_NOT_FOUND' || errorCode === 'MODEL_ROUTE_NOT_VISIBLE') return { label: '检查路由', href: '#/routing' }
  if (errorCode === 'BILLING_INSUFFICIENT_POINTS') return { label: '查看用户', href: '#/users' }
  if (errorCode === 'UPSTREAM_UNAVAILABLE') return { label: '检查接入账号', href: '#/access-accounts' }
  return null
}

export function callRecordRows(records: CallRecord[]): CallRecordRowModel[] {
  return records.map((record) => ({
    id: String(record.id ?? record.task_id),
    fullTaskId: record.task_id,
    taskLabel: shortID(record.task_id),
    taskDetail: `${adminTaskTypeLabel(record.task_type)} · ${record.requested_output_image_count} 张请求 / ${record.success_output_image_count} 张成功 / ${record.reference_image_count} 张参考`,
    userLabel: `User #${record.user_id}`,
    userDetail: record.api_key_id ? `API Key #${record.api_key_id}` : channelLabel(record.source_channel),
    routeLabel: record.route_model_code || snapshotText(record.pricing_snapshot, 'route_model_code') || record.abstract_model || '-',
    routeDetail: `${record.base_resolution || '-'} · ${record.quality || '-'} · ${channelLabel(record.source_channel)}`,
    routeModelCode: callRecordRouteModelCode(record),
    providerLabel: record.provider || '-',
    providerDetail: providerDetail(record),
    status: record.status,
    statusLabel: callRecordStatusLabel(record.status),
    statusTone: statusTone(record.status),
    failureLabel: record.error_code || '无错误',
    failureDetail: record.error_message || (record.error_code ? errorCodeHint(record.error_code) : '调用成功或尚未返回错误'),
    amountLabel: `${emptyDash(record.estimated_points)} / ${emptyDash(record.actual_points)}`,
    costLabel: emptyDash(record.provider_cost),
    marginLabel: emptyDash(record.gross_margin),
    lifecycleLabel: lifecycleText(record),
    createdAt: formatDateTime(record.created_at),
    startedAtLabel: formatDateTime(record.started_at),
    finishedAtLabel: formatDateTime(record.finished_at),
    updatedAtLabel: formatDateTime(record.updated_at),
    upstreamSucceededAtLabel: formatDateTime(record.upstream_succeeded_at),
    durationSeconds: callRecordDurationSeconds(record),
    durationLabel: callRecordDurationLabel(record),
    previewURL: callRecordPreviewURL(record),
    resultImages: record.result_images ?? [],
    prompt: callRecordPrompt(record),
  }))
}

export function callRecordRouteModelCode(record: CallRecord): string {
  return record.route_model_code || snapshotText(record.pricing_snapshot, 'route_model_code') || ''
}

export function callRecordDurationSeconds(record: CallRecord): number {
  const started = firstValidTime(record.started_at, record.created_at)
  let finished = validTime(record.finished_at)
  if (finished === null && isTerminalStatus(record.status)) {
    finished = validTime(record.upstream_succeeded_at)
    if (finished === null) finished = validTime(record.updated_at)
  }
  if (started === null || finished === null || finished <= started) return 0
  return (finished - started) / 1000
}

export function callRecordDurationLabel(record: CallRecord): string {
  const seconds = callRecordDurationSeconds(record)
  return seconds > 0 ? `${seconds.toFixed(1)}s` : '—'
}

export function callRecordRouteModelOptions(routes: Iterable<RouteModel>, records: CallRecord[] = []): CallRecordFilterOption[] {
  const options = new Map<string, string>()
  for (const route of routes) {
    if (route.code) options.set(route.code, route.name ? `${route.name} · ${route.code}` : route.code)
  }
  for (const record of records) {
    const code = callRecordRouteModelCode(record)
    if (code && !options.has(code)) options.set(code, code)
  }
  return Array.from(options, ([value, label]) => ({ value, label }))
    .sort((left, right) => left.label.localeCompare(right.label, 'zh-Hans-CN'))
}

export function callRecordProviderOptions(providers: ModelProvider[], records: CallRecord[] = []): CallRecordFilterOption[] {
  const options = new Map<string, string>()
  for (const provider of providers) {
    if (provider.provider_code) options.set(provider.provider_code, provider.provider_code)
  }
  for (const record of records) {
    if (record.provider && !options.has(record.provider)) options.set(record.provider, record.provider)
  }
  return Array.from(options, ([value, label]) => ({ value, label }))
    .sort((left, right) => left.label.localeCompare(right.label, 'zh-CN'))
}

type CallRecordPreviewFields = {
  preview_url?: unknown
  image_url?: unknown
  download_url?: unknown
  output_url?: unknown
  result_url?: unknown
  results?: unknown
}

/**
 * The current admin call-record API does not project output artifacts. Keep this
 * resolver narrow so a future backend projection can be rendered without
 * inventing URLs; callers must show an explicit empty state when it returns null.
 */
export function callRecordPreviewURL(record: CallRecord): string | null {
  const extended = record as CallRecord & CallRecordPreviewFields
  for (const value of [extended.preview_url, extended.image_url, extended.download_url, extended.output_url, extended.result_url]) {
    const url = safePreviewURL(value)
    if (url) return url
  }
  if (Array.isArray(extended.results)) {
    for (const item of extended.results) {
      if (!item || typeof item !== 'object') continue
      const result = item as Record<string, unknown>
      for (const value of [result.preview_url, result.url, result.download_url, result.image_url]) {
        const url = safePreviewURL(value)
        if (url) return url
      }
    }
  }
  return null
}

export function callRecordPrompt(record: CallRecord): string {
  const extended = record as CallRecord & { prompt?: unknown; negative_prompt?: unknown }
  if (typeof extended.prompt === 'string' && extended.prompt.trim()) return extended.prompt.trim()
  const snapshotPrompt = record.pricing_snapshot?.prompt
  return typeof snapshotPrompt === 'string' ? snapshotPrompt.trim() : ''
}

function safePreviewURL(value: unknown): string | null {
  if (typeof value !== 'string') return null
  const url = value.trim()
  if (!url) return null
  if (url.startsWith('//') || url.startsWith('/\\')) return null
  if (url.startsWith('/') || url.startsWith('https://') || url.startsWith('http://')) return url
  return null
}

function validTime(value?: string | null): number | null {
  if (!value) return null
  const parsed = new Date(value).getTime()
  return Number.isFinite(parsed) ? parsed : null
}

function firstValidTime(...values: Array<string | null | undefined>): number | null {
  for (const value of values) {
    const parsed = validTime(value)
    if (parsed !== null) return parsed
  }
  return null
}

function isTerminalStatus(status: string) {
  return status === 'succeeded' || status === 'failed' || status === 'canceled' || status === 'rejected'
}

export function callRecordStatusLabel(status: string) {
  if (status === 'queued') return '排队中'
  if (status === 'running') return '执行中'
  if (status === 'succeeded') return '成功'
  if (status === 'failed') return '失败'
  if (status === 'canceled') return '已取消'
  if (status === 'rejected') return '已拒绝'
  if (status === 'pending') return '待处理'
  return status || '未知状态'
}

function statusTone(status: string): CallRecordRowModel['statusTone'] {
  if (status === 'succeeded') return 'success'
  if (status === 'failed' || status === 'rejected' || status === 'canceled') return 'danger'
  if (status === 'queued' || status === 'running' || status === 'pending') return 'warning'
  return 'neutral'
}

function shortID(id: string) {
  if (!id) return '-'
  return id.length > 14 ? `${id.slice(0, 8)}...${id.slice(-4)}` : id
}

function emptyDash(value?: string | null) {
  return value && value !== '0' ? value : value === '0' ? '0' : '-'
}

function formatDateTime(value?: string | null) {
  const input = value ?? ''
  const match = input.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!match) return input || '-'
  return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
}

function lifecycleText(record: CallRecord) {
  const started = formatDateTime(record.started_at)
  const finished = formatDateTime(record.finished_at)
  if (started === '-' && finished === '-') return `创建 ${formatDateTime(record.created_at)}`
  return `开始 ${started} · 结束 ${finished}`
}

function providerDetail(record: CallRecord) {
  const parts = [`${record.attempt_count || 0} 次尝试`]
  if (record.account_model_id) parts.push(`账号模型 #${record.account_model_id}`)
  if (record.model_account_id) parts.push(`模型账号 #${record.model_account_id}`)
  if (record.upstream_model_code) parts.push(record.upstream_model_code)
  return parts.join(' · ')
}

function snapshotText(snapshot: Record<string, unknown> | undefined, key: string) {
  const value = snapshot?.[key]
  return typeof value === 'string' ? value : ''
}

function channelLabel(channel?: string) {
  if (channel === 'web') return '用户 Web'
  if (channel === 'open_api') return 'Open API'
  if (channel === 'openai_compatible') return 'OpenAI 兼容'
  return channel || '未知入口'
}

function errorCodeHint(code: string) {
  if (code === 'MODEL_ROUTE_NOT_FOUND') return '路由模型不存在，检查模型路由配置。'
  if (code === 'MODEL_ROUTE_NO_CANDIDATE') return '路由模型没有可用候选账号。'
  if (code === 'ROUTE_MODEL_PRICE_MISSING') return '缺少该模型/任务类型/质量的价格配置。'
  if (code === 'MODEL_ROUTE_NOT_VISIBLE') return '当前用户分组不可见或模型已隐藏。'
  if (code === 'BILLING_INSUFFICIENT_POINTS') return '用户余额不足，需充值或后台调整额度。'
  if (code === 'UPSTREAM_UNAVAILABLE') return '底层 Provider 不可用或返回失败。'
  return '查看任务详情、路由配置或 Provider 日志继续定位。'
}
