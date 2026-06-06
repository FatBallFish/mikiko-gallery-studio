import type { PaymentOrder, PaymentWebhookEvent } from '../../../shared/api-types'

const orderPaymentLabels: Record<string, string> = {
  mock: 'Mock 测试',
  alipay: '支付宝',
  wxpay: '微信支付',
  alipay_direct: '支付宝',
  wxpay_direct: '微信支付',
  easypay_alipay: '易支付支付宝',
  easypay_wxpay: '易支付微信',
  jeepay_alipay: 'JeePay 支付宝',
  jeepay_wxpay: 'JeePay 微信',
  manual_alipay: '人工确认 · 支付宝',
  manual_wxpay: '人工确认 · 微信支付',
  manual_bank: '人工确认 · 银行转账',
}

export const cashierManualCompletionProviderOptions = [
  { value: 'manual_alipay', label: '人工确认 · 支付宝' },
  { value: 'manual_wxpay', label: '人工确认 · 微信支付' },
  { value: 'manual_bank', label: '人工确认 · 银行转账' },
] as const

const visibleMethodLabels: Record<string, string> = {
  alipay: '支付宝',
  wxpay: '微信支付',
  mock: 'Mock 测试',
}

const webhookProviderLabels: Record<string, string> = {
  mock: 'Mock 测试',
  alipay_direct: '支付宝直连',
  wxpay_direct: '微信支付直连',
  easypay_alipay: '易支付支付宝',
  easypay_wxpay: '易支付微信',
  jeepay_alipay: 'JeePay 支付宝',
  jeepay_wxpay: 'JeePay 微信',
}

const purchaseTypeLabels: Record<string, string> = {
  plan: '积分包购买',
  custom_amount: '自定义金额充值',
  subscription: '订阅套餐',
}

const webhookEventTypeLabels: Record<string, string> = {
  'payment.completed': '支付到账回调',
  'payment.succeeded': '支付成功回调',
  'payment.retryable_failed': '支付回调失败',
  'refund.completed': '退款回调',
  'refund.local_finalize_failed': '退款本地落账失败',
  'cashier.order.manual_complete': '人工确认到账',
}

const providerConfigStatusLabels: Record<string, string> = {
  configured: '配置已完成',
  missing: '缺少配置',
  invalid: '配置异常',
}

export function cashierOrderPaymentLabel(order: Pick<PaymentOrder, 'visible_method' | 'provider_type' | 'provider'>) {
  return firstKnownLabel([order.visible_method, order.provider_type, order.provider], orderPaymentLabels)
}

export function cashierOrderPurchaseTypeLabel(order: Pick<PaymentOrder, 'purchase_type'>) {
  return firstKnownLabel([order.purchase_type], purchaseTypeLabels)
}

export function cashierProviderSupportedMethodsLabel(methods: string[] | undefined | null) {
  const labels = (methods ?? [])
    .map((method) => firstKnownLabel([method], visibleMethodLabels))
    .filter((label) => label !== '-')
  return labels.length ? labels.join('、') : '-'
}

export function cashierProviderConfigStatusLabel(status: string | undefined | null) {
  return firstKnownLabel([status], providerConfigStatusLabels)
}

export function cashierWebhookProviderLabel(event: Pick<PaymentWebhookEvent, 'provider_type'>) {
  return firstKnownLabel([event.provider_type], webhookProviderLabels)
}

export function cashierWebhookEventTypeLabel(event: Pick<PaymentWebhookEvent, 'event_type'>) {
  return firstKnownLabel([event.event_type], webhookEventTypeLabels)
}

export function cashierAdminDateTime(value?: string | null) {
  const input = value ?? ''
  const match = input.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!match) return input || '-'
  return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
}

function firstKnownLabel(values: Array<string | undefined | null>, labels: Record<string, string>) {
  for (const value of values) {
    const normalized = (value ?? '').trim()
    if (!normalized) continue
    return labels[normalized] ?? normalized
  }
  return '-'
}
