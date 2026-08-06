import type { CashierOrder } from '../../../shared/api-types'

export const paymentMonitorAutoCloseDelay = 3000

export function shouldAutoClosePaymentMonitor(previous: CashierOrder, current: CashierOrder) {
  return previous.id === current.id && !isPaymentSuccess(previous.status) && isPaymentSuccess(current.status)
}

function isPaymentSuccess(status: string) {
  const normalized = status.trim().toLowerCase()
  return normalized === 'paid' || normalized === 'completed'
}
