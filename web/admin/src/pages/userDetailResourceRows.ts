import type { ApiKey, BalanceBucket, ImageTask, PaymentOrder } from '../../../shared/api-types'
import { adminTaskTypeLabel } from './adminTaskTypes'

export type UserDetailStatusTone = 'success' | 'warning' | 'danger' | 'neutral'

export type UserDetailBucketRow = {
  key: string
  label: string
  availablePoints: string
  expiresAtLabel: string
  expiryTone: UserDetailStatusTone
}

export type UserDetailOrderRow = {
  id: PaymentOrder['id']
  orderNo: string
  statusLabel: string
  statusTone: UserDetailStatusTone
  amountCny: string
  points: string
  createdAtLabel: string
}

export type UserDetailTaskRow = {
  id: ImageTask['id']
  shortId: string
  statusLabel: string
  statusTone: UserDetailStatusTone
  typeLabel: string
  modelLabel: string
  pointsLabel: string
}

export type UserDetailApiKeyRow = {
  id: ApiKey['id']
  name: string
  statusLabel: string
  statusTone: UserDetailStatusTone
  groupCode: string
  accessKey: string
  lastUsedAtLabel: string
}

const paymentOrderStatusLabels: Record<string, string> = {
  pending: '待支付',
  paid: '已到账',
  completed: '已到账',
  canceled: '已关闭',
  cancelled: '已关闭',
  closed: '已关闭',
  failed: '支付失败',
  refunded: '已退款',
  partially_refunded: '部分退款',
}

const imageTaskStatusLabels: Record<string, string> = {
  queued: '排队中',
  running: '生成中',
  succeeded: '成功',
  partial_failed: '部分成功',
  failed: '失败',
  rejected: '已拒绝',
  deleted: '已删除',
  canceled: '已取消',
  cancelled: '已取消',
}

const apiKeyStatusLabels: Record<string, string> = {
  active: '启用',
  disabled: '禁用',
  revoked: '已撤销',
  expired: '已过期',
  deleted: '已删除',
}

export function paymentOrderStatusLabel(status: string) {
  return paymentOrderStatusLabels[normalizeStatus(status)] ?? statusLabelFallback(status)
}

export function paymentOrderStatusTone(status: string): UserDetailStatusTone {
  const normalized = normalizeStatus(status)
  if (normalized === 'paid' || normalized === 'completed') return 'success'
  if (normalized === 'pending' || normalized === 'partially_refunded') return 'warning'
  if (normalized === 'failed') return 'danger'
  return 'neutral'
}

export function imageTaskStatusLabel(status: string) {
  return imageTaskStatusLabels[normalizeStatus(status)] ?? statusLabelFallback(status)
}

export function imageTaskStatusTone(status: string): UserDetailStatusTone {
  const normalized = normalizeStatus(status)
  if (normalized === 'succeeded') return 'success'
  if (normalized === 'failed' || normalized === 'rejected') return 'danger'
  if (normalized === 'queued' || normalized === 'running' || normalized === 'partial_failed') return 'warning'
  return 'neutral'
}

export function apiKeyStatusLabel(status: string) {
  return apiKeyStatusLabels[normalizeStatus(status)] ?? statusLabelFallback(status)
}

export function apiKeyStatusTone(status: string): UserDetailStatusTone {
  const normalized = normalizeStatus(status)
  if (normalized === 'active') return 'success'
  if (normalized === 'disabled' || normalized === 'expired') return 'warning'
  if (normalized === 'revoked' || normalized === 'deleted') return 'danger'
  return 'neutral'
}

export function imageTaskTypeLabel(type: string) {
  return adminTaskTypeLabel(type)
}

export function userDetailBucketRow(bucket: BalanceBucket, index: number): UserDetailBucketRow {
  const expiresAt = bucket.expires_at ?? bucket.next_expiring_at
  return {
    key: `${bucket.bucket}-${index}`,
    label: bucket.label || balanceBucketLabel(bucket.bucket),
    availablePoints: bucket.available_points,
    expiresAtLabel: expiresAt ? formatDateTime(expiresAt) : '长期有效',
    expiryTone: bucket.expire_warning ? 'warning' : 'neutral',
  }
}

export function userDetailOrderRow(order: PaymentOrder): UserDetailOrderRow {
  return {
    id: order.id,
    orderNo: order.order_no || `#${order.id}`,
    statusLabel: paymentOrderStatusLabel(order.status),
    statusTone: paymentOrderStatusTone(order.status),
    amountCny: order.amount_cny || '-',
    points: order.points || '-',
    createdAtLabel: formatDateTime(order.created_at),
  }
}

export function userDetailTaskRow(task: ImageTask): UserDetailTaskRow {
  return {
    id: task.id,
    shortId: shortID(task.id),
    statusLabel: imageTaskStatusLabel(task.status),
    statusTone: imageTaskStatusTone(task.status),
    typeLabel: imageTaskTypeLabel(task.task_type),
    modelLabel: firstNonEmpty(task.route_model_name, task.route_model_code, task.abstract_model, task.model_group),
    pointsLabel: firstNonEmpty(task.actual_points, task.estimated_points, task.estimate_points),
  }
}

export function userDetailApiKeyRow(apiKey: ApiKey): UserDetailApiKeyRow {
  return {
    id: apiKey.id,
    name: apiKey.name.trim() || '未命名密钥',
    statusLabel: apiKeyStatusLabel(apiKey.status),
    statusTone: apiKeyStatusTone(apiKey.status),
    groupCode: apiKey.group_code || '-',
    accessKey: apiKey.access_key || '-',
    lastUsedAtLabel: apiKey.last_used_at ? formatDateTime(apiKey.last_used_at) : '未调用',
  }
}

function normalizeStatus(status: string) {
  return status.trim().toLowerCase()
}

function statusLabelFallback(status: string) {
  const normalized = status.trim()
  return normalized || '未知状态'
}

function balanceBucketLabel(bucket: string) {
  if (bucket === 'trial') return '体验额度'
  if (bucket === 'subscription') return '订阅额度'
  if (bucket === 'recharge') return '充值额度'
  if (bucket === 'gift') return '赠送额度'
  return bucket || '未知额度'
}

function firstNonEmpty(...values: Array<string | null | undefined>) {
  for (const value of values) {
    const normalized = value?.trim()
    if (normalized) return normalized
  }
  return '-'
}

function shortID(id: string) {
  if (!id) return '-'
  return id.length > 8 ? id.slice(0, 8) : id
}

function formatDateTime(value?: string | null) {
  const input = value ?? ''
  const match = input.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!match) return input || '-'
  return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
}
