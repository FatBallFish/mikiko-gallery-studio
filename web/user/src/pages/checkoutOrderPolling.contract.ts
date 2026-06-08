import type { CashierOrder } from '../../../shared/api-types'
import { checkoutOrderRuntimeState } from './checkoutOrderState'

const pendingOrder: CashierOrder = {
  id: 2,
  order_no: 'pay_2',
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

const pendingState = checkoutOrderRuntimeState(pendingOrder, Date.parse('2026-06-05T10:05:00Z'))
if (pendingState.step !== 'paying' || !pendingState.shouldPoll) {
  throw new Error(`pending unexpired order should keep polling, got ${JSON.stringify(pendingState)}`)
}

const expiredState = checkoutOrderRuntimeState(pendingOrder, Date.parse('2026-06-05T10:10:01Z'))
if (expiredState.step !== 'expired' || expiredState.shouldPoll) {
  throw new Error(`expired order should stop polling, got ${JSON.stringify(expiredState)}`)
}

const completedState = checkoutOrderRuntimeState({ ...pendingOrder, status: 'completed' }, Date.parse('2026-06-05T10:05:00Z'))
if (completedState.step !== 'success' || completedState.shouldPoll) {
  throw new Error(`completed order should be terminal success, got ${JSON.stringify(completedState)}`)
}

const failedState = checkoutOrderRuntimeState({ ...pendingOrder, status: 'failed' }, Date.parse('2026-06-05T10:05:00Z'))
if (failedState.step !== 'failed' || failedState.shouldPoll) {
  throw new Error(`failed order should be terminal failure, got ${JSON.stringify(failedState)}`)
}
