import type { CallRecord } from '../../../shared/api-types'
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
    label: 'Provider',
    placeholder: 'Provider 或账号',
  },
  userId: {
    label: '用户 ID',
    placeholder: '用户 ID',
  },
  taskId: {
    label: '任务 ID',
    placeholder: '任务 ID',
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
    routeLabel: record.abstract_model || '-',
    routeDetail: `${record.base_resolution || '-'} · ${record.quality || '-'} · ${channelLabel(record.source_channel)}`,
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
  }))
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
