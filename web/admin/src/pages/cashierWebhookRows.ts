import type { PaymentWebhookEvent } from '../../../shared/api-types'
import { cashierAdminDateTime, cashierWebhookEventTypeLabel, cashierWebhookProviderLabel } from './cashierPaymentDisplay'
import { cashierWebhookStatusBadge } from './cashierStatusRows'
import type { CashierStatusBadge } from './cashierStatusRows'

export type CashierWebhookRow = {
  id: string
  title: string
  subtitle: string
  orderLabel: string
  providerLabel: string
  statusBadge: CashierStatusBadge
  signatureLabel: string
  resultSummary: string
  payloadPreview: string
  receivedAtLabel: string
  processedAtLabel: string
}

export function cashierWebhookRow(event: PaymentWebhookEvent): CashierWebhookRow {
  const riskReason = event.failure_reason?.trim()
  return {
    id: String(event.id),
    title: cashierWebhookEventTypeLabel(event),
    subtitle: riskReason || event.result_summary || '等待渠道返回处理结果',
    orderLabel: String(event.order_no ?? event.order_id ?? '-'),
    providerLabel: cashierWebhookProviderLabel(event),
    statusBadge: cashierWebhookStatusBadge(event.status),
    signatureLabel: cashierWebhookSignatureLabel(event.signature_status),
    resultSummary: event.result_summary || webhookResultFallback(event),
    payloadPreview: formatPayloadPreview(event.payload_preview),
    receivedAtLabel: cashierAdminDateTime(event.received_at),
    processedAtLabel: cashierAdminDateTime(event.processed_at),
  }
}

export function cashierWebhookSignatureLabel(status?: string | null) {
  const normalized = (status ?? '').trim()
  if (normalized === 'verified') return '验签通过'
  if (normalized === 'recorded') return '已记录签名'
  if (normalized === 'not_recorded') return '未记录签名'
  if (normalized === 'failed') return '验签/处理失败'
  return normalized || '未返回'
}

function webhookResultFallback(event: PaymentWebhookEvent) {
  if (event.status === 'processed') return '已完成本地处理'
  if (event.status === 'failed') return event.failure_reason || '处理失败，等待重试'
  if (event.status === 'verified') return '已验签，等待本地落账'
  if (event.status === 'received') return '已接收，等待验签'
  return event.status || '-'
}

function formatPayloadPreview(payload?: string | null) {
  const value = payload?.trim()
  if (!value) return '无 payload 预览'
  return value
}
