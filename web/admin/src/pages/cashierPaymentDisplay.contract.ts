import type { PaymentOrder, PaymentWebhookEvent } from '../../../shared/api-types'
import { cashierAdminDateTime, cashierManualCompletionProviderOptions, cashierOrderPaymentLabel, cashierOrderPurchaseTypeLabel, cashierProviderConfigStatusLabel, cashierProviderSupportedMethodsLabel, cashierWebhookEventTypeLabel, cashierWebhookProviderLabel } from './cashierPaymentDisplay'

const baseOrder: PaymentOrder = {
  id: 1,
  order_no: 'ord_1',
  plan_id: 1,
  plan_code: 'points-100',
  plan_name: '100 积分包',
  provider: 'mock',
  provider_type: 'mock',
  visible_method: 'mock',
  purchase_type: 'plan',
  status: 'pending',
  currency: 'CNY',
  amount_cny: '19.90000',
  points: '100.00000',
  bonus_points: '0.00000',
  expires_at: '2026-06-05T12:00:00Z',
  created_at: '2026-06-05T10:00:00Z',
  updated_at: '2026-06-05T10:00:00Z',
}

const orderCases: Array<[Partial<PaymentOrder>, string]> = [
  [{ visible_method: 'mock', provider_type: 'mock', provider: 'mock' }, 'Mock 测试'],
  [{ visible_method: 'alipay', provider_type: 'alipay_direct', provider: '' }, '支付宝'],
  [{ visible_method: 'wxpay', provider_type: 'wxpay_direct', provider: '' }, '微信支付'],
  [{ visible_method: '', provider_type: 'easypay_alipay', provider: 'easypay_alipay' }, '易支付支付宝'],
  [{ visible_method: '', provider_type: 'easypay_wxpay', provider: 'easypay_wxpay' }, '易支付微信'],
  [{ visible_method: '', provider_type: 'jeepay_alipay', provider: 'jeepay_alipay' }, 'JeePay 支付宝'],
  [{ visible_method: '', provider_type: 'jeepay_wxpay', provider: 'jeepay_wxpay' }, 'JeePay 微信'],
  [{ visible_method: '', provider_type: '', provider: 'manual_alipay' }, '人工确认 · 支付宝'],
  [{ visible_method: '', provider_type: '', provider: 'manual_wxpay' }, '人工确认 · 微信支付'],
  [{ visible_method: '', provider_type: '', provider: 'manual_bank' }, '人工确认 · 银行转账'],
  [{ visible_method: '', provider_type: '', provider: '' }, '-'],
]

for (const [fields, expected] of orderCases) {
  const label = cashierOrderPaymentLabel({ ...baseOrder, ...fields })
  if (label !== expected) {
    throw new Error(`cashier order payment ${JSON.stringify(fields)} should display as ${expected}, got ${label}`)
  }
}

const webhookCases: Array<[Partial<PaymentWebhookEvent>, string]> = [
  [{ provider_type: 'mock' }, 'Mock 测试'],
  [{ provider_type: 'alipay_direct' }, '支付宝直连'],
  [{ provider_type: 'wxpay_direct' }, '微信支付直连'],
  [{ provider_type: 'easypay_alipay' }, '易支付支付宝'],
  [{ provider_type: 'jeepay_wxpay' }, 'JeePay 微信'],
  [{ provider_type: 'custom_gateway' }, 'custom_gateway'],
  [{ provider_type: '' }, '-'],
]

for (const [fields, expected] of webhookCases) {
  const label = cashierWebhookProviderLabel(fields as PaymentWebhookEvent)
  if (label !== expected) {
    throw new Error(`cashier webhook provider ${JSON.stringify(fields)} should display as ${expected}, got ${label}`)
  }
}

const visibleKnown = [
  cashierOrderPaymentLabel({ ...baseOrder, visible_method: 'mock', provider_type: 'mock', provider: 'mock' }),
  cashierOrderPaymentLabel({ ...baseOrder, visible_method: '', provider_type: 'jeepay_alipay', provider: 'jeepay_alipay' }),
  cashierWebhookProviderLabel({ provider_type: 'wxpay_direct' } as PaymentWebhookEvent),
].join(' ')

if (/\b(mock|alipay_direct|wxpay_direct|jeepay_alipay)\b/.test(visibleKnown)) {
  throw new Error(`cashier known payment labels should not expose raw enum values, got ${visibleKnown}`)
}

const manualProviderOptions = cashierManualCompletionProviderOptions.map((option) => `${option.value}:${option.label}`).join(',')
for (const expected of ['manual_alipay:人工确认 · 支付宝', 'manual_wxpay:人工确认 · 微信支付', 'manual_bank:人工确认 · 银行转账']) {
  if (!manualProviderOptions.includes(expected)) {
    throw new Error(`manual completion provider options should include ${expected}, got ${manualProviderOptions}`)
  }
}

if (cashierManualCompletionProviderOptions.some((option) => /manual_|placeholder|占位/i.test(option.label))) {
  throw new Error(`manual completion provider labels should not expose raw provider values, got ${manualProviderOptions}`)
}

if (cashierAdminDateTime(baseOrder.created_at) !== '2026/06/05 10:00') {
  throw new Error(`cashier admin date-time should be stable and timezone-free, got ${cashierAdminDateTime(baseOrder.created_at)}`)
}

if (cashierAdminDateTime(null) !== '-' || cashierAdminDateTime(undefined) !== '-') {
  throw new Error('cashier admin date-time should show dash for empty values')
}

if (cashierAdminDateTime('bad-date') !== 'bad-date') {
  throw new Error(`cashier admin date-time should preserve invalid values, got ${cashierAdminDateTime('bad-date')}`)
}

const purchaseTypeCases: Array<[Partial<PaymentOrder>, string]> = [
  [{ purchase_type: 'plan', plan_name: '100 积分包' }, '积分包购买'],
  [{ purchase_type: 'custom_amount', plan_name: '' }, '自定义金额充值'],
  [{ purchase_type: 'subscription', plan_name: '月度套餐' }, '订阅套餐'],
  [{ purchase_type: 'manual_adjustment', plan_name: '' }, 'manual_adjustment'],
  [{ purchase_type: '', plan_name: '' }, '-'],
]

for (const [fields, expected] of purchaseTypeCases) {
  const label = cashierOrderPurchaseTypeLabel({ ...baseOrder, ...fields })
  if (label !== expected) {
    throw new Error(`cashier purchase type ${JSON.stringify(fields)} should display as ${expected}, got ${label}`)
  }
}

const eventTypeCases: Array<[Partial<PaymentWebhookEvent>, string]> = [
  [{ event_type: 'payment.completed' }, '支付到账回调'],
  [{ event_type: 'payment.retryable_failed' }, '支付回调失败'],
  [{ event_type: 'refund.completed' }, '退款回调'],
  [{ event_type: 'refund.local_finalize_failed' }, '退款本地落账失败'],
  [{ event_type: 'cashier.order.manual_complete' }, '人工确认到账'],
  [{ event_type: 'custom.gateway.notice' }, 'custom.gateway.notice'],
  [{ event_type: '' }, '-'],
]

for (const [fields, expected] of eventTypeCases) {
  const label = cashierWebhookEventTypeLabel(fields as PaymentWebhookEvent)
  if (label !== expected) {
    throw new Error(`cashier webhook event type ${JSON.stringify(fields)} should display as ${expected}, got ${label}`)
  }
}

const typeVisibleKnown = [
  cashierOrderPurchaseTypeLabel({ ...baseOrder, purchase_type: 'custom_amount' }),
  cashierWebhookEventTypeLabel({ event_type: 'payment.retryable_failed' } as PaymentWebhookEvent),
  cashierWebhookEventTypeLabel({ event_type: 'refund.local_finalize_failed' } as PaymentWebhookEvent),
].join(' ')

if (/custom_amount|payment\.retryable_failed|refund\.local_finalize_failed/.test(typeVisibleKnown)) {
  throw new Error(`cashier known type labels should not expose raw enum values, got ${typeVisibleKnown}`)
}

const supportedMethodCases: Array<[string[], string]> = [
  [['alipay'], '支付宝'],
  [['wxpay'], '微信支付'],
  [['mock'], 'Mock 测试'],
  [['alipay', 'wxpay', 'mock'], '支付宝、微信支付、Mock 测试'],
  [['bank_transfer'], 'bank_transfer'],
  [[], '-'],
]

for (const [methods, expected] of supportedMethodCases) {
  const label = cashierProviderSupportedMethodsLabel(methods)
  if (label !== expected) {
    throw new Error(`cashier supported methods ${JSON.stringify(methods)} should display as ${expected}, got ${label}`)
  }
}

const methodVisibleKnown = cashierProviderSupportedMethodsLabel(['alipay', 'wxpay', 'mock'])
if (/\b(alipay|wxpay|mock)\b/.test(methodVisibleKnown)) {
  throw new Error(`cashier supported method labels should not expose raw method values, got ${methodVisibleKnown}`)
}

const configStatusCases: Array<[string | undefined | null, string]> = [
  ['configured', '配置已完成'],
  ['missing', '缺少配置'],
  ['invalid', '配置异常'],
  ['rotating', 'rotating'],
  ['', '-'],
  [undefined, '-'],
  [null, '-'],
]

for (const [status, expected] of configStatusCases) {
  const label = cashierProviderConfigStatusLabel(status)
  if (label !== expected) {
    throw new Error(`cashier provider config status ${status ?? '<empty>'} should display as ${expected}, got ${label}`)
  }
}

const configVisibleKnown = ['configured', 'missing', 'invalid'].map((status) => cashierProviderConfigStatusLabel(status)).join(' ')
if (/\b(configured|missing|invalid)\b/.test(configVisibleKnown)) {
  throw new Error(`cashier known config statuses should not expose raw status values, got ${configVisibleKnown}`)
}
