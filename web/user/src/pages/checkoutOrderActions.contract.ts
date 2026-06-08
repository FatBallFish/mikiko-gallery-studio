import type { CashierOrder } from '../../../shared/api-types'
import { checkoutOrderActionState } from './checkoutOrderState'

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
if (!pendingActions.canCancel || !pendingActions.canMockPay || pendingActions.cancelLabel !== '取消订单') {
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
