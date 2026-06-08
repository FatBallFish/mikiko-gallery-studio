import type { PaymentWebhookEvent } from '../../../shared/api-types'

export type CashierWebhookEventAction = {
  canRetry: boolean
  label: string
  title: string
}

export function cashierWebhookEventAction(event: PaymentWebhookEvent): CashierWebhookEventAction {
  if (event.status === 'failed') {
    return {
      canRetry: true,
      label: '重试',
      title: event.failure_reason || '重新处理失败回调事件',
    }
  }
  if (event.status === 'processed') {
    return {
      canRetry: false,
      label: '已处理',
      title: '该回调事件已处理完成',
    }
  }
  return {
    canRetry: false,
    label: '等待处理',
    title: '该回调事件尚未进入失败状态',
  }
}
