import type { PaymentWebhookEvent } from '../../../shared/api-types'
import { cashierWebhookEventAction } from './cashierWebhookEventActions'

const failedAction = cashierWebhookEventAction({
  id: 1,
  order_id: 10,
  order_no: 'PG-ORDER-1',
  provider_type: 'mock',
  status: 'failed',
  event_type: 'payment.succeeded',
  failure_reason: 'signature mismatch',
  received_at: '2026-06-05T00:00:00Z',
  processed_at: null,
} satisfies PaymentWebhookEvent)

if (!failedAction.canRetry || failedAction.label !== '重试' || failedAction.title !== 'signature mismatch') {
  throw new Error(`failed webhook events should expose retry action with failure reason, got ${JSON.stringify(failedAction)}`)
}

const processedAction = cashierWebhookEventAction({
  id: 2,
  provider_type: 'mock',
  status: 'processed',
  event_type: 'payment.succeeded',
  received_at: '2026-06-05T00:00:00Z',
  processed_at: '2026-06-05T00:00:01Z',
} satisfies PaymentWebhookEvent)

if (processedAction.canRetry || processedAction.label !== '已处理') {
  throw new Error(`processed webhook events should not look retryable, got ${JSON.stringify(processedAction)}`)
}

const receivedAction = cashierWebhookEventAction({
  id: 3,
  provider_type: 'mock',
  status: 'received',
  event_type: 'payment.succeeded',
  received_at: '2026-06-05T00:00:00Z',
  processed_at: null,
} satisfies PaymentWebhookEvent)

if (receivedAction.canRetry || receivedAction.label !== '等待处理') {
  throw new Error(`non-failed webhook events should wait instead of retrying, got ${JSON.stringify(receivedAction)}`)
}
