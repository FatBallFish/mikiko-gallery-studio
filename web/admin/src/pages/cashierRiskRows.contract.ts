import type { PaymentOrder, PaymentWebhookEvent } from '../../../shared/api-types'
import { cashierOrderRiskRows, cashierWebhookRiskRow } from './cashierRiskRows'

const completedOrder = {
  id: 1,
  order_no: 'PG-ORDER-1',
  user_id: 7,
  plan_id: 1,
  plan_code: 'points-100',
  plan_name: '100 积分包',
  provider: 'mock',
  provider_type: 'mock',
  visible_method: 'mock',
  purchase_type: 'custom_amount',
  status: 'completed',
  currency: 'CNY',
  amount_cny: '10.00000',
  points: '20.00000',
  bonus_points: '0.00000',
  trade_no: 'MOCK-TRADE-1',
  expires_at: '2026-06-05T10:00:00Z',
  created_at: '2026-06-05T09:00:00Z',
  updated_at: '2026-06-05T09:00:00Z',
} satisfies PaymentOrder

const completedRows = cashierOrderRiskRows(completedOrder)
const refundable = completedRows.find((row) => row.key === 'refund-available')
if (!refundable || refundable.value !== '¥10.00 / 20.00 积分' || refundable.tone !== 'success') {
  throw new Error(`completed orders should show refundable balance, got ${JSON.stringify(completedRows)}`)
}

const partialRows = cashierOrderRiskRows({
  ...completedOrder,
  status: 'partially_refunded',
  refund_trade_no: 'RF-1',
  refunded_amount_cny: '5.00000',
  refunded_points: '10.00000',
})
const partial = partialRows.find((row) => row.key === 'refund-partial')
if (!partial || partial.value !== '已退 ¥5.00 / 剩余可退 ¥5.00' || !partial.detail.includes('仍可继续退 10.00 积分')) {
  throw new Error(`partial refunds should explain refunded and remaining amount, got ${JSON.stringify(partialRows)}`)
}

const failedRows = cashierOrderRiskRows({
  ...completedOrder,
  status: 'failed',
  failure_reason: 'amount mismatch',
})
const failed = failedRows.find((row) => row.key === 'payment-failed')
if (!failed || failed.tone !== 'danger' || failed.detail !== 'amount mismatch') {
  throw new Error(`failed orders should expose actionable failure reason, got ${JSON.stringify(failedRows)}`)
}

const finalizeFailed = cashierWebhookRiskRow({
  id: 1,
  order_id: 1,
  order_no: 'PG-ORDER-1',
  provider_type: 'mock',
  status: 'failed',
  event_type: 'refund.local_finalize_failed',
  failure_reason: 'ledger insert failed',
  received_at: '2026-06-05T10:00:00Z',
  processed_at: null,
} satisfies PaymentWebhookEvent)

if (finalizeFailed.label !== '退款补偿' || finalizeFailed.value !== '本地落账失败' || finalizeFailed.tone !== 'danger' || !finalizeFailed.detail.includes('自动补偿')) {
  throw new Error(`refund finalize failures should be classified as compensation risk, got ${JSON.stringify(finalizeFailed)}`)
}

const retryablePayment = cashierWebhookRiskRow({
  id: 2,
  provider_type: 'alipay_direct',
  status: 'failed',
  event_type: 'payment.retryable_failed',
  failure_reason: 'signature mismatch',
  received_at: '2026-06-05T10:00:00Z',
  processed_at: null,
} satisfies PaymentWebhookEvent)

if (retryablePayment.label !== '支付回调' || retryablePayment.value !== '待重试' || !retryablePayment.detail.includes('渠道交易号和金额一致')) {
  throw new Error(`retryable payment failures should guide channel reconciliation, got ${JSON.stringify(retryablePayment)}`)
}

const visibleCopy = [
  ...partialRows.map((row) => `${row.label}${row.value}${row.detail}`),
  `${finalizeFailed.label}${finalizeFailed.value}${finalizeFailed.detail}`,
  `${retryablePayment.label}${retryablePayment.value}${retryablePayment.detail}`,
].join(' ')

if (/partially_refunded|refund\.local_finalize_failed|payment\.retryable_failed|TODO|后续|暂不/.test(visibleCopy)) {
  throw new Error(`cashier risk copy should not expose raw enums or roadmap wording, got ${visibleCopy}`)
}
