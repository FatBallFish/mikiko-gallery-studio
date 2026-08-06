import fs from 'node:fs'
import type { CashierOrder } from '../../../shared/api-types'
import { paymentMonitorAutoCloseDelay, shouldAutoClosePaymentMonitor } from './paymentMonitorState'

const pendingOrder: CashierOrder = {
  id: 9,
  order_no: 'PGO-MONITOR-9',
  purchase_type: 'plan',
  visible_method: 'alipay',
  status: 'pending',
  currency: 'CNY',
  amount_cny: '99.00000',
  points: '300.00000',
  bonus_points: '0.00000',
  expires_at: '2026-08-06T14:00:00Z',
  created_at: '2026-08-06T13:00:00Z',
  updated_at: '2026-08-06T13:00:00Z',
}

if (paymentMonitorAutoCloseDelay !== 3000) {
  throw new Error(`payment monitor success delay should be 3000ms, got ${paymentMonitorAutoCloseDelay}`)
}

for (const status of ['pending', 'canceled', 'closed', 'failed', 'expired', 'refunded']) {
  if (shouldAutoClosePaymentMonitor(pendingOrder, { ...pendingOrder, status })) {
    throw new Error(`payment monitor must remain visible for ${status}`)
  }
}
for (const status of ['paid', 'completed']) {
  if (!shouldAutoClosePaymentMonitor(pendingOrder, { ...pendingOrder, status })) {
    throw new Error(`payment monitor should auto-close after confirmed ${status}`)
  }
}
if (shouldAutoClosePaymentMonitor({ ...pendingOrder, status: 'completed' }, { ...pendingOrder, status: 'completed' })) {
  throw new Error('an already-completed order must not start a new close timer merely because a monitor mounted')
}

const monitorPath = new URL('./PaymentMonitorModal.tsx', import.meta.url)
const detailPath = new URL('./PaymentOrderDetailModal.tsx', import.meta.url)
if (!fs.existsSync(monitorPath) || !fs.existsSync(detailPath)) {
  throw new Error('payment monitoring and historical order details must use separate modal components')
}

const monitorSource = fs.readFileSync(monitorPath, 'utf8')
const detailSource = fs.readFileSync(detailPath, 'utf8')
const checkoutSource = fs.readFileSync(new URL('./CheckoutPage.tsx', import.meta.url), 'utf8')

if (!monitorSource.includes('syncCashierOrder') || !monitorSource.includes("addEventListener('focus'")) {
  throw new Error('payment monitor must actively sync and sync again when the window regains focus')
}
if (!monitorSource.includes('mountedRef.current')) {
  throw new Error('payment monitor must ignore an in-flight sync result after it unmounts')
}
if (detailSource.includes('syncCashierOrder') || detailSource.includes('setTimeout') || detailSource.includes('setInterval')) {
  throw new Error('historical order detail must not poll, sync, or auto-close')
}
if (!checkoutSource.includes('monitorOrder') || !checkoutSource.includes('detailOrder')) {
  throw new Error('checkout must keep monitor and historical detail state independent')
}
const mockPayBody = checkoutSource.match(/async function mockPay[\s\S]*?\n  }\n\n  async function cancelOrder/)?.[0] ?? ''
if (mockPayBody.includes('refreshAccount') || mockPayBody.includes("notify('success'")) {
  throw new Error('mock payment success side effects must flow through the payment monitor exactly once')
}
const monitorCloseBody = checkoutSource.match(/onClose=\{\(\) => \{[\s\S]*?\}\}/)?.[0] ?? ''
if (monitorCloseBody.includes('refreshAccount') || monitorCloseBody.includes('loadRecentOrders')) {
  throw new Error('closing a successful monitor must not repeat account and recent-order refreshes')
}
if (!monitorSource.includes('aria-live="polite"') || !monitorSource.includes('role="status"')) {
  throw new Error('payment status transitions must be announced through a polite live region')
}
