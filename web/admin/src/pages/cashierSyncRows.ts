import type { PaymentOrderSyncResult } from '../../../shared/api-types'

export type CashierSyncTone = 'success' | 'warning' | 'danger' | 'neutral'

export type CashierSyncRow = {
  statusLabel: string
  categoryLabel: string
  actionHint: string
  tone: CashierSyncTone
}

const statusLabels: Record<string, string> = {
  pending: '等待支付',
  paid: '已支付',
  closed: '已关闭',
  failed: '渠道异常',
  refunded: '已退款',
}

export function cashierSyncRow(sync: PaymentOrderSyncResult): CashierSyncRow {
  const category = sync.risk_category || inferSyncCategory(sync)
  return {
    statusLabel: statusLabels[sync.query_status] ?? (sync.query_status || '-'),
    categoryLabel: syncCategoryLabel(category),
    actionHint: sync.action_hint || syncCategoryActionHint(category, sync),
    tone: syncCategoryTone(category, sync.query_status),
  }
}

function inferSyncCategory(sync: PaymentOrderSyncResult) {
  const raw = sync.raw ?? {}
  const rawText = [
    sync.message,
    raw.status,
    raw.trade_status,
    raw.trade_state,
    raw.error_code,
    raw.err_code,
    raw.sub_code,
  ].map((value) => String(value ?? '').toLowerCase()).join(' ')
  if (/risk|fraud|intercept|limit|security|风控|风险/.test(rawText)) return 'risk_control'
  if (sync.query_status === 'paid') return 'paid'
  if (sync.query_status === 'closed') return 'closed'
  if (sync.query_status === 'refunded') return 'refunded'
  if (sync.query_status === 'failed') return 'channel_error'
  return 'pending'
}

function syncCategoryLabel(category: string) {
  if (category === 'risk_control') return '风控拦截'
  if (category === 'paid') return '渠道已支付'
  if (category === 'closed') return '渠道已关闭'
  if (category === 'refunded') return '渠道已退款'
  if (category === 'channel_error') return '渠道处理异常'
  if (category === 'pending') return '等待用户支付'
  return category || '-'
}

function syncCategoryActionHint(category: string, sync: PaymentOrderSyncResult) {
  if (category === 'risk_control') return '渠道侧风控或安全策略拦截，建议让用户更换支付渠道或重新创建订单后再支付。'
  if (category === 'paid') return sync.completed ? '本次查单已完成本地到账；可刷新订单和用户余额确认。' : '渠道已确认支付，本地已存在到账记录，本次不会重复入账。'
  if (category === 'closed') return '渠道订单已关闭，建议取消当前订单并让用户重新创建订单。'
  if (category === 'refunded') return '渠道显示已退款，请核对本地退款流水和用户充值余额是否一致。'
  if (category === 'channel_error') return '渠道返回异常状态，请结合原始响应、商户后台和回调事件继续排查。'
  return sync.message || '渠道仍未确认支付，稍后可再次查单。'
}

function syncCategoryTone(category: string, status: string): CashierSyncTone {
  if (category === 'paid' || status === 'paid') return 'success'
  if (category === 'risk_control' || category === 'channel_error' || status === 'failed') return 'danger'
  if (category === 'closed' || category === 'refunded' || status === 'closed' || status === 'refunded') return 'warning'
  return 'neutral'
}
