import { readFileSync } from 'node:fs'
import type { CashierOrder } from '../../../shared/api-types'
import { checkoutCancelResultState, checkoutOrderActionState } from './checkoutOrderState'

const baseOrder: CashierOrder = {
  id: 7,
  order_no: 'pay_7',
  purchase_type: 'plan',
  visible_method: 'mock',
  status: 'pending',
  currency: 'CNY',
  amount_cny: '19.90000',
  points: '100.00000',
  bonus_points: '0.00000',
  expires_at: '2026-06-05T10:10:00Z',
  created_at: '2026-06-05T10:00:00Z',
  updated_at: '2026-06-05T10:00:00Z',
}

const pendingActions = checkoutOrderActionState(baseOrder, Date.parse('2026-06-05T10:05:00Z'))
if (!pendingActions.canContinuePay || !pendingActions.canCancel || !pendingActions.canMockPay || pendingActions.cancelLabel !== '取消订单') {
  throw new Error(`pending cashier orders should expose cancel action, got ${JSON.stringify(pendingActions)}`)
}

const expiredActions = checkoutOrderActionState(baseOrder, Date.parse('2026-06-05T10:10:01Z'))
if (expiredActions.canCancel || expiredActions.canMockPay) {
  throw new Error(`expired cashier orders should not expose cancel action, got ${JSON.stringify(expiredActions)}`)
}

const completedActions = checkoutOrderActionState({ ...baseOrder, status: 'completed' }, Date.parse('2026-06-05T10:05:00Z'))
if (completedActions.canCancel || completedActions.canMockPay) {
  throw new Error(`completed cashier orders should not expose cancel action, got ${JSON.stringify(completedActions)}`)
}

const canceledActions = checkoutOrderActionState({ ...baseOrder, status: 'canceled' }, Date.parse('2026-06-05T10:05:00Z'))
if (canceledActions.canCancel || canceledActions.canMockPay || canceledActions.terminalLabel !== '订单已取消') {
  throw new Error(`canceled cashier orders should show terminal canceled label, got ${JSON.stringify(canceledActions)}`)
}

for (const status of ['canceled', 'cancelled', 'closed']) {
  if (checkoutCancelResultState(status) !== 'canceled') {
    throw new Error(`safe cancellation result ${status} must show canceled feedback`)
  }
}
for (const status of ['paid', 'completed']) {
  if (checkoutCancelResultState(status) !== 'paid') {
    throw new Error(`safe cancellation result ${status} must show payment success feedback`)
  }
}
for (const status of ['pending', 'failed', 'expired', 'refunded']) {
  if (checkoutCancelResultState(status) !== 'unchanged') {
    throw new Error(`safe cancellation result ${status} must not claim the order was canceled`)
  }
}

const checkoutSource = readFileSync(new URL('./CheckoutPage.tsx', import.meta.url), 'utf8')
for (const staleCopy of ['固定积分包和自定义金额统一通过收银台创建订单，支付成功后进入充值余额桶且不过期。', '支付成功，充值余额已刷新']) {
  if (checkoutSource.includes(staleCopy)) {
    throw new Error(`checkout must not claim fixed packages always enter permanent recharge balance: ${staleCopy}`)
  }
}
const cancelOrderBody = checkoutSource.match(/async function cancelOrder[\s\S]*?\n  }\n\n  if \(loading\)/)?.[0] ?? ''
for (const required of ['checkoutCancelResultState(next.status)', 'monitorOrder?.id === next.id', 'app.refreshAccount()', 'loadRecentOrders()']) {
  if (!cancelOrderBody.includes(required)) {
    throw new Error(`safe cancellation UI must handle ${required}`)
  }
}
if (!cancelOrderBody.includes("cancelResult === 'paid'") || !cancelOrderBody.includes("cancelResult === 'canceled'")) {
  throw new Error('safe cancellation UI must separate paid and canceled provider outcomes')
}
