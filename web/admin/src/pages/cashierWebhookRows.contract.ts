import type { PaymentWebhookEvent } from '../../../shared/api-types'
import { cashierWebhookRow, cashierWebhookSignatureLabel } from './cashierWebhookRows'

const failedEvent = {
  id: 1,
  order_id: 10,
  order_no: 'PG-ORDER-1',
  provider_type: 'mock',
  status: 'failed',
  event_type: 'refund.local_finalize_failed',
  failure_reason: 'ledger insert failed',
  signature_status: 'failed',
  result_summary: '处理失败，等待人工或自动重试',
  payload_preview: '{"refund_trade_no":"RF-1","order_no":"PG-ORDER-1"}',
  received_at: '2026-06-05T10:00:00Z',
  processed_at: null,
} satisfies PaymentWebhookEvent

const failedRow = cashierWebhookRow(failedEvent)
if (failedRow.title !== '退款本地落账失败' || failedRow.orderLabel !== 'PG-ORDER-1' || failedRow.signatureLabel !== '验签/处理失败') {
  throw new Error(`failed webhook rows should localize event, order and signature status, got ${JSON.stringify(failedRow)}`)
}

if (!failedRow.resultSummary.includes('自动重试') || !failedRow.payloadPreview.includes('refund_trade_no')) {
  throw new Error(`failed webhook rows should expose result summary and payload preview, got ${JSON.stringify(failedRow)}`)
}

const processedRow = cashierWebhookRow({
  id: 2,
  provider_type: 'alipay_direct',
  status: 'processed',
  event_type: 'payment.completed',
  signature_status: 'verified',
  result_summary: '已完成本地处理',
  payload_preview: '',
  received_at: '2026-06-05T10:00:00Z',
  processed_at: '2026-06-05T10:01:00Z',
} satisfies PaymentWebhookEvent)

if (processedRow.signatureLabel !== '验签通过' || processedRow.processedAtLabel !== '2026/06/05 10:01' || processedRow.payloadPreview !== '无 payload 预览') {
  throw new Error(`processed webhook rows should show signature/result/times safely, got ${JSON.stringify(processedRow)}`)
}

const signatureCases: Array<[string | undefined | null, string]> = [
  ['verified', '验签通过'],
  ['recorded', '已记录签名'],
  ['not_recorded', '未记录签名'],
  ['failed', '验签/处理失败'],
  ['provider_custom', 'provider_custom'],
  ['', '未返回'],
  [undefined, '未返回'],
  [null, '未返回'],
]

for (const [status, expected] of signatureCases) {
  const label = cashierWebhookSignatureLabel(status)
  if (label !== expected) {
    throw new Error(`signature status ${status ?? '<empty>'} should display as ${expected}, got ${label}`)
  }
}

const visibleCopy = [
  failedRow.title,
  failedRow.signatureLabel,
  failedRow.resultSummary,
  processedRow.signatureLabel,
  processedRow.resultSummary,
].join(' ')

if (/refund\.local_finalize_failed|payment\.completed|verified|not_recorded|processed|failed/.test(visibleCopy)) {
  throw new Error(`webhook row copy should not expose raw enums, got ${visibleCopy}`)
}
