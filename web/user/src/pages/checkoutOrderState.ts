import type { CashierOrder, CashierPurchaseType, PaymentProviderType, PaymentVisibleMethod } from '../../../shared/api-types'

export function checkoutMoney(value?: string) {
  return value ? `¥${Number(value).toFixed(2)}` : '¥0.00'
}

export function checkoutPoints(value?: string) {
  return Number(value ?? '0').toFixed(2)
}

export function checkoutDateTime(value?: string | null) {
  const raw = (value ?? '').trim()
  if (!raw) return '-'
  const match = raw.match(/^(\d{4})-(\d{2})-(\d{2})[T\s](\d{2}):(\d{2})/)
  if (!match) return raw
  return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
}

export type CheckoutRecentOrderRow = {
  id: number
  orderNo: string
  title: string
  status: string
  amount: string
  points: string
  method: string
  createdAt: string
  createdAtLabel: string
  order: CashierOrder
}

export type CheckoutPaymentMethodOptionModel = {
  rawMethod: string
  label: string
  detail: string
}

const CHECKOUT_ORDER_STATUS_LABELS: Record<string, string> = {
  pending: '待支付',
  paid: '已到账',
  completed: '已到账',
  canceled: '已取消',
  cancelled: '已取消',
  closed: '已关闭',
  failed: '支付失败',
  expired: '已过期',
  refunded: '已退款',
  partially_refunded: '部分退款',
}

const CHECKOUT_PAYMENT_METHOD_LABELS: Record<string, string> = {
  mock: 'Mock 测试',
  alipay: '支付宝',
  alipay_direct: '支付宝',
  wxpay: '微信支付',
  wxpay_direct: '微信支付',
  easypay_alipay: '易支付支付宝',
  easypay_wxpay: '易支付微信',
  jeepay_alipay: 'JeePay 支付宝',
  jeepay_wxpay: 'JeePay 微信',
  manual_alipay: '人工确认 · 支付宝',
  manual_wxpay: '人工确认 · 微信支付',
  manual_bank: '人工确认 · 银行转账',
}

export function checkoutOrderStatusLabel(status?: string) {
  const normalized = (status ?? '').trim().toLowerCase()
  if (!normalized) return '-'
  return CHECKOUT_ORDER_STATUS_LABELS[normalized] ?? status ?? '-'
}

type CheckoutRecentOrder = {
  id: number
  order_no: string
  status: string
  amount_cny: string
  points: string
  created_at: string
  visible_method?: string
  provider?: string
  provider_type?: PaymentProviderType
  plan_name?: string
  plan_code?: string
  purchase_type?: CashierPurchaseType
  currency?: string
  bonus_points?: string
  expires_at?: string
  updated_at?: string
}

export function checkoutPaymentMethodLabel(order: { visible_method?: string; provider?: string; provider_type?: PaymentProviderType }) {
  const candidates = [order.visible_method, order.provider_type, order.provider]
    .map((value) => (value ?? '').trim())
    .filter(Boolean)
  const raw = candidates[0]
  if (!raw) return '-'
  const normalized = raw.toLowerCase()
  return CHECKOUT_PAYMENT_METHOD_LABELS[normalized] ?? raw
}

export function checkoutPaymentMethodOptionModel(method: PaymentVisibleMethod): CheckoutPaymentMethodOptionModel {
  const rawMethod = method.method.trim()
  const rawProvider = (method.source_provider_type ?? '').trim()
  const label = method.label.trim() || checkoutPaymentMethodLabel({
    visible_method: rawMethod,
    provider_type: rawProvider,
    provider: '',
  })
  const providerLabel = checkoutPaymentMethodLabel({
    visible_method: '',
    provider_type: rawProvider,
    provider: '',
  })
  const detail = method.description?.trim()
    || (rawMethod === 'mock' || rawProvider === 'mock'
      ? '测试环境模拟支付'
      : providerLabel !== '-' ? (providerLabel === rawProvider ? providerLabel : `${providerLabel} 渠道`) : rawMethod || '-')
  return {
    rawMethod,
    label,
    detail,
  }
}

export function checkoutRecentOrderRows(orders: CheckoutRecentOrder[], limit = 10): CheckoutRecentOrderRow[] {
  return [...orders]
    .sort((left, right) => Date.parse(right.created_at || '') - Date.parse(left.created_at || ''))
    .slice(0, limit)
    .map((order) => ({
      id: order.id,
      orderNo: order.order_no,
      title: order.plan_name || (order.purchase_type === 'custom_amount' ? '自定义金额充值' : order.plan_code || '积分充值'),
      status: checkoutOrderStatusLabel(order.status),
      amount: checkoutMoney(order.amount_cny),
      points: checkoutPoints(order.points),
      method: checkoutPaymentMethodLabel(order),
      createdAt: order.created_at,
      createdAtLabel: checkoutDateTime(order.created_at),
      order: toCashierOrder(order),
    }))
}

function toCashierOrder(order: CheckoutRecentOrder): CashierOrder {
  return {
    ...order,
    currency: order.currency ?? 'CNY',
    bonus_points: order.bonus_points ?? '0.00000',
    expires_at: order.expires_at ?? '',
    updated_at: order.updated_at ?? order.created_at,
    purchase_type: order.purchase_type ?? 'plan',
    visible_method: order.visible_method ?? order.provider ?? '',
  }
}

export type CheckoutStep = 'select' | 'confirm' | 'paying' | 'success' | 'failed' | 'expired'

export type CheckoutOrderRuntimeState = {
  step: CheckoutStep
  shouldPoll: boolean
  label: string
  detail: string
}

export type CheckoutOrderActionState = {
  canCancel: boolean
  canMockPay: boolean
  cancelLabel: string
  terminalLabel?: string
}

export function checkoutOrderRuntimeState(order: CashierOrder | null, nowMs = Date.now()): CheckoutOrderRuntimeState {
  if (!order) {
    return { step: 'select', shouldPoll: false, label: '选择充值内容', detail: '请选择固定积分包或自定义金额后创建订单。' }
  }
  const status = order.status.toLowerCase()
  if (status === 'completed' || status === 'paid') {
    return { step: 'success', shouldPoll: false, label: '支付成功', detail: '充值积分已进入充值余额桶，可立即用于生成图片。' }
  }
  if (status === 'expired') {
    return { step: 'expired', shouldPoll: false, label: '订单已过期', detail: '该订单已超过支付有效期，请重新创建订单。' }
  }
  if (status === 'failed' || status === 'closed' || status === 'canceled' || status === 'cancelled' || status === 'refunded') {
    return { step: 'failed', shouldPoll: false, label: '订单未完成', detail: '订单已关闭或支付失败，可重新创建订单。' }
  }
  const expiresAt = Date.parse(order.expires_at)
  if (Number.isFinite(expiresAt) && expiresAt <= nowMs) {
    return { step: 'expired', shouldPoll: false, label: '订单已过期', detail: '该订单已超过支付有效期，请重新创建订单。' }
  }
  return { step: 'paying', shouldPoll: true, label: '等待支付', detail: '完成支付后页面会自动刷新订单状态和账户余额。' }
}

export function checkoutOrderActionState(order: CashierOrder | null, nowMs = Date.now()): CheckoutOrderActionState {
  if (!order) return { canCancel: false, canMockPay: false, cancelLabel: '取消订单' }
  const status = order.status.toLowerCase()
  if (status === 'canceled' || status === 'cancelled') {
    return { canCancel: false, canMockPay: false, cancelLabel: '取消订单', terminalLabel: '订单已取消' }
  }
  if (status !== 'pending') return { canCancel: false, canMockPay: false, cancelLabel: '取消订单' }
  const expiresAt = Date.parse(order.expires_at)
  if (Number.isFinite(expiresAt) && expiresAt <= nowMs) return { canCancel: false, canMockPay: false, cancelLabel: '取消订单' }
  return {
    canCancel: true,
    canMockPay: order.provider_type === 'mock' || order.visible_method === 'mock' || order.provider === 'mock',
    cancelLabel: '取消订单',
  }
}
