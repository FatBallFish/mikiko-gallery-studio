import type { PaymentOrderSyncStatus } from '../../../shared/api-types'

export type CashierStatusTone = 'success' | 'warning' | 'danger' | 'neutral'

export type CashierStatusBadge = {
  label: string
  tone: CashierStatusTone
}

export const cashierPlanStatusOptions = [
  { value: 'active', label: '启用' },
  { value: 'disabled', label: '停用' },
  { value: 'archived', label: '已归档' },
] as const

export function cashierEnabledBadge(enabled: boolean): CashierStatusBadge {
  return enabled ? { label: '启用', tone: 'success' } : { label: '停用', tone: 'warning' }
}

export function cashierVisibleFlagLabel(enabled: boolean) {
  return enabled ? '启用' : '停用'
}

export function cashierBooleanVisibilityLabel(visible: boolean) {
  return visible ? '已启用' : '已隐藏'
}

export function cashierPlanStatusBadge(status?: string | null): CashierStatusBadge {
  const normalized = normalize(status)
  if (normalized === 'active') return { label: '启用', tone: 'success' }
  if (normalized === 'disabled') return { label: '停用', tone: 'warning' }
  if (normalized === 'archived') return { label: '已归档', tone: 'neutral' }
  return { label: normalized || '未知状态', tone: 'neutral' }
}

export function cashierPlanTypeLabel(type?: string | null) {
  const normalized = normalize(type)
  if (!normalized || normalized === 'points_package') return '积分包'
  if (normalized === 'subscription') return '订阅套餐'
  return normalized
}

export function cashierOrderStatusBadge(status?: string | null): CashierStatusBadge {
  const normalized = normalize(status)
  if (normalized === 'pending') return { label: '待支付', tone: 'warning' }
  if (normalized === 'paid') return { label: '已支付', tone: 'success' }
  if (normalized === 'completed') return { label: '已完成', tone: 'success' }
  if (normalized === 'partially_refunded') return { label: '部分退款', tone: 'warning' }
  if (normalized === 'refunded') return { label: '已退款', tone: 'neutral' }
  if (normalized === 'failed') return { label: '失败', tone: 'danger' }
  if (normalized === 'canceled') return { label: '已取消', tone: 'neutral' }
  if (normalized === 'closed') return { label: '已关闭', tone: 'neutral' }
	if (normalized === 'expired') return { label: '支付超时', tone: 'danger' }
  return { label: normalized || '未知状态', tone: 'neutral' }
}

export function cashierWebhookStatusBadge(status?: string | null): CashierStatusBadge {
  const normalized = normalize(status)
  if (normalized === 'received') return { label: '已接收', tone: 'warning' }
  if (normalized === 'verified') return { label: '已验签', tone: 'warning' }
  if (normalized === 'processed') return { label: '已处理', tone: 'success' }
  if (normalized === 'failed') return { label: '失败', tone: 'danger' }
  return { label: normalized || '未知状态', tone: 'neutral' }
}

export function cashierSyncStatusLabel(status?: PaymentOrderSyncStatus | string | null) {
  const normalized = normalize(status)
  if (normalized === 'pending') return '待支付'
  if (normalized === 'paid') return '已支付'
  if (normalized === 'closed') return '已关闭'
  if (normalized === 'failed') return '查询失败'
  if (normalized === 'refunded') return '已退款'
  return normalized || '未知状态'
}

function normalize(value?: string | null) {
  return (value ?? '').trim().toLowerCase()
}
