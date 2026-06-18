import type { CashierOrder } from '../../../shared/api-types'
import { checkoutOrderActionState, checkoutOrderRuntimeState } from './checkoutOrderState'
import { checkoutPaymentDisplayModel } from './checkoutPaymentDisplay'

const baseOrder: CashierOrder = {
  id: 11,
  order_no: 'PGO-10001',
  purchase_type: 'custom_amount',
  visible_method: 'alipay',
  status: 'pending',
  currency: 'CNY',
  amount_cny: '25.00000',
  points: '80.00000',
  bonus_points: '0.00000',
  expires_at: '2026-06-05T10:10:00Z',
  created_at: '2026-06-05T10:00:00Z',
  updated_at: '2026-06-05T10:00:00Z',
}

const pendingActions = checkoutOrderActionState(baseOrder, Date.parse('2026-06-05T10:05:00Z'))
if (!pendingActions.canContinuePay || !pendingActions.canCancel) {
  throw new Error(`order detail should expose valid pending-order operations, got ${JSON.stringify(pendingActions)}`)
}

const completedActions = checkoutOrderActionState({ ...baseOrder, status: 'completed' }, Date.parse('2026-06-05T10:05:00Z'))
if (completedActions.canContinuePay || completedActions.canCancel) {
  throw new Error(`completed order detail should not expose payment operations, got ${JSON.stringify(completedActions)}`)
}

const expiredRuntime = checkoutOrderRuntimeState(baseOrder, Date.parse('2026-06-05T10:10:01Z'))
if (expiredRuntime.step !== 'expired' || expiredRuntime.shouldPoll) {
  throw new Error(`expired order detail should be a pure terminal information state, got ${JSON.stringify(expiredRuntime)}`)
}

const qrDisplay = checkoutPaymentDisplayModel({
  ...baseOrder,
  payment_display: { type: 'qr_code', qr_code: 'weixin://pay/session' },
})
if (qrDisplay.kind !== 'qr_code' || !qrDisplay.href) {
  throw new Error(`payment prompt should distinguish qr payment displays, got ${JSON.stringify(qrDisplay)}`)
}

const redirectDisplay = checkoutPaymentDisplayModel({
  ...baseOrder,
  payment_display: { type: 'redirect', payment_url: 'https://pay.example.test/session' },
})
if (redirectDisplay.kind !== 'redirect' || !redirectDisplay.href) {
  throw new Error(`payment prompt should distinguish redirect payment displays, got ${JSON.stringify(redirectDisplay)}`)
}
