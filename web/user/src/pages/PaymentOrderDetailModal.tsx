import type { CashierOrder } from '../../../shared/api-types'
import { Button, Modal } from '../components'
import { checkoutDateTime, checkoutMoney, checkoutOrderActionState, checkoutOrderStatusLabel, checkoutPaymentMethodLabel, checkoutPoints } from './checkoutOrderState'

export function PaymentOrderDetailModal({ order, busy, onCancel, onClose }: {
  order: CashierOrder
  busy: boolean
  onCancel: (order: CashierOrder) => void
  onClose: () => void
}) {
  const actions = checkoutOrderActionState(order)
  const title = order.plan_name || (order.purchase_type === 'custom_amount' ? '自定义金额充值' : order.plan_code || '积分充值')
  return (
    <Modal title="订单详情" onClose={onClose}>
      <div className="grid gap-5 pt-1">
        <header className="grid gap-1 border-b border-[var(--border)] pb-5">
          <span className="text-xs font-bold text-[var(--muted)]">{checkoutOrderStatusLabel(order.status)}</span>
          <strong className="text-xl font-black text-[var(--fg)]">{title}</strong>
          <span className="text-sm text-[var(--muted)]">{order.order_no}</span>
        </header>
        <dl className="grid gap-x-6 gap-y-4 sm:grid-cols-2">
          <OrderFact label="支付金额" value={checkoutMoney(order.amount_cny)} />
          <OrderFact label="到账积分" value={`${checkoutPoints(order.points)} 积分`} />
          <OrderFact label="支付方式" value={checkoutPaymentMethodLabel(order)} />
          <OrderFact label="创建时间" value={checkoutDateTime(order.created_at)} />
          <OrderFact label="更新时间" value={checkoutDateTime(order.updated_at)} />
          <OrderFact label="过期时间" value={checkoutDateTime(order.expires_at)} />
        </dl>
        <div className="flex flex-wrap justify-end gap-3 border-t border-[var(--border)] pt-4">
          {actions.canCancel ? <Button tone="danger" busy={busy} onClick={() => onCancel(order)}>{actions.cancelLabel}</Button> : null}
          <Button tone="primary" onClick={onClose}>关闭</Button>
        </div>
      </div>
    </Modal>
  )
}

function OrderFact({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid min-w-0 gap-1">
      <dt className="text-xs text-[var(--muted)]">{label}</dt>
      <dd className="m-0 text-sm font-bold text-[var(--fg)] [overflow-wrap:anywhere]">{value}</dd>
    </div>
  )
}
