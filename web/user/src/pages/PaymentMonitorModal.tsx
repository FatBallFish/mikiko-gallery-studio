import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { toDataURL } from 'qrcode'
import { CheckCircle2, CircleAlert, Clock3, ExternalLink, RefreshCw } from 'lucide-react'
import type { CashierOrder } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { userApi } from '../../../shared/user-api'
import { Button, LoadingState, Modal, copyText } from '../components'
import { errorMessage } from '../useApiResource'
import { checkoutPaymentDisplayModel } from './checkoutPaymentDisplay'
import { dispatchPaymentWindow, navigatePaymentWindow, reservePaymentWindow } from './checkoutPaymentWindow'
import { checkoutMoney, checkoutOrderActionState, checkoutOrderRuntimeState, checkoutPaymentMethodLabel } from './checkoutOrderState'
import { paymentMonitorAutoCloseDelay, shouldAutoClosePaymentMonitor } from './paymentMonitorState'

const StripePaymentPanel = lazy(async () => ({ default: (await import('./StripePaymentPanel')).StripePaymentPanel }))

export function PaymentMonitorModal({ order, busy, onOrderChange, onSuccess, onCancel, onMockPay, onClose }: {
  order: CashierOrder
  busy: boolean
  onOrderChange: (order: CashierOrder) => void
  onSuccess: (order: CashierOrder) => void
  onCancel: (order: CashierOrder) => void
  onMockPay: (order: CashierOrder) => void
  onClose: () => void
}) {
  const [nowMs, setNowMs] = useState(() => Date.now())
  const [syncing, setSyncing] = useState(false)
  const [syncError, setSyncError] = useState<string | null>(null)
  const orderRef = useRef(order)
  const callbacksRef = useRef({ onOrderChange, onSuccess, onClose })
  const syncInFlightRef = useRef(false)
  const mountedRef = useRef(true)
  const successHandledRef = useRef<number | null>(null)
  const previousOrderRef = useRef(order)
  orderRef.current = order
  callbacksRef.current = { onOrderChange, onSuccess, onClose }

  const runtime = useMemo(() => checkoutOrderRuntimeState(order, nowMs), [nowMs, order])
  const actions = useMemo(() => checkoutOrderActionState(order, nowMs), [nowMs, order])

  const syncCashierOrder = useCallback(async () => {
    if (!mountedRef.current || syncInFlightRef.current) return
    syncInFlightRef.current = true
    setSyncing(true)
    try {
      const result = await userApi.syncCashierOrder(orderRef.current.id)
      if (!mountedRef.current) return
      setSyncError(null)
      callbacksRef.current.onOrderChange(result.order)
    } catch (caught) {
      if (mountedRef.current) setSyncError(errorMessage(caught))
    } finally {
      syncInFlightRef.current = false
      if (mountedRef.current) setSyncing(false)
    }
  }, [])

  useEffect(() => {
    mountedRef.current = true
    return () => { mountedRef.current = false }
  }, [])

  useEffect(() => {
    const timer = window.setInterval(() => setNowMs(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])

  useEffect(() => {
    if (!runtime.shouldPoll) return undefined
    void syncCashierOrder()
    const timer = window.setInterval(() => void syncCashierOrder(), 2000)
    return () => window.clearInterval(timer)
  }, [runtime.shouldPoll, syncCashierOrder])

  useEffect(() => {
    const handleFocus = () => void syncCashierOrder()
    window.addEventListener('focus', handleFocus)
    return () => window.removeEventListener('focus', handleFocus)
  }, [syncCashierOrder])

  useEffect(() => {
    const previousOrder = previousOrderRef.current
    previousOrderRef.current = order
    if (!shouldAutoClosePaymentMonitor(previousOrder, order) || successHandledRef.current === order.id) return undefined
    successHandledRef.current = order.id
    callbacksRef.current.onSuccess(order)
    const timer = window.setTimeout(() => callbacksRef.current.onClose(), paymentMonitorAutoCloseDelay)
    return () => window.clearTimeout(timer)
  }, [order.id, order.status])

  const statusTone = runtime.step === 'success' ? 'success' : runtime.step === 'paying' ? 'waiting' : 'error'
  const StatusIcon = statusTone === 'success' ? CheckCircle2 : statusTone === 'waiting' ? Clock3 : CircleAlert
  return (
    <Modal title="等待支付结果" onClose={onClose}>
      <div className="grid gap-5 pt-1">
        <section className="grid grid-cols-[auto_minmax(0,1fr)] items-center gap-4 border-b border-[var(--border)] pb-5" role="status" aria-live="polite">
          <span className={cn('grid size-11 place-items-center rounded-full', statusTone === 'success' ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400' : statusTone === 'waiting' ? 'bg-[color-mix(in_oklch,var(--accent)_14%,transparent)] text-[var(--accent)]' : 'bg-[color-mix(in_oklch,var(--accent-coral)_14%,transparent)] text-[var(--accent-coral)]')}>
            <StatusIcon size={22} aria-hidden="true" />
          </span>
          <span className="grid min-w-0 gap-1">
            <strong className="text-lg font-black text-[var(--fg)]">{runtime.label}</strong>
            <span className="text-sm leading-relaxed text-[var(--muted)]">{runtime.detail}</span>
          </span>
        </section>

        <div className="grid grid-cols-2 gap-x-6 gap-y-3 text-sm sm:grid-cols-3">
          <MonitorFact label="购买内容" value={order.plan_name || (order.purchase_type === 'custom_amount' ? '自定义金额充值' : order.plan_code || '积分充值')} />
          <MonitorFact label="支付金额" value={checkoutMoney(order.amount_cny)} />
          <MonitorFact label="支付方式" value={checkoutPaymentMethodLabel(order)} />
        </div>

        {runtime.step === 'paying' ? <PaymentDisplayPanel order={order} busy={busy || syncing} onMockPay={() => onMockPay(order)} onConfirmed={syncCashierOrder} /> : null}
        {syncError ? <p className="m-0 rounded-lg border border-[color-mix(in_oklch,var(--accent-coral)_40%,var(--border))] px-3 py-2 text-sm text-[var(--accent-coral)]" role="alert">{syncError}</p> : null}
        {runtime.step === 'success' ? <p className="m-0 text-sm text-[var(--muted)]">支付已确认，窗口将在 3 秒后关闭。</p> : null}

        <div className="flex flex-wrap justify-end gap-3 border-t border-[var(--border)] pt-4 max-[420px]:flex-col-reverse">
          <Button tone="ghost" busy={syncing} onClick={() => void syncCashierOrder()}><RefreshCw size={16} aria-hidden="true" />刷新结果</Button>
          {actions.canCancel ? <Button tone="danger" busy={busy} onClick={() => onCancel(order)}>{actions.cancelLabel}</Button> : null}
          <Button tone={runtime.step === 'success' ? 'primary' : 'ghost'} onClick={onClose}>{runtime.step === 'paying' ? '稍后支付' : '关闭'}</Button>
        </div>
      </div>
    </Modal>
  )
}

function MonitorFact({ label, value }: { label: string; value: string }) {
  return <span className="grid min-w-0 gap-1"><small className="text-[var(--muted)]">{label}</small><strong className="[overflow-wrap:anywhere]">{value}</strong></span>
}

function PaymentDisplayPanel({ order, busy, onMockPay, onConfirmed }: { order: CashierOrder; busy: boolean; onMockPay: () => void; onConfirmed: () => void }) {
  const display = checkoutPaymentDisplayModel(order)
  const paymentHref = display.href ?? ''
  const openPayment = () => {
    const paymentWindow = reservePaymentWindow()
    if (display.kind === 'form' || display.kind === 'redirect') {
      dispatchPaymentWindow(paymentWindow, display)
      return
    }
    if (display.href && paymentWindow) {
      navigatePaymentWindow(paymentWindow, display.href, display.kind === 'qr_code')
      return
    }
    paymentWindow?.close()
  }
  return (
    <section className="grid gap-3 border-y border-[var(--border)] py-4">
      <span className="grid gap-1"><strong>{display.label}</strong><small className="text-sm leading-relaxed text-[var(--muted)]">{display.detail}</small></span>
      {display.kind === 'qr_code' && paymentHref ? <div className="grid justify-items-center gap-3"><PaymentQRCode value={paymentHref} /><Button tone="ghost" onClick={openPayment}><ExternalLink size={16} aria-hidden="true" />打开支付页</Button><Button tone="ghost" onClick={() => void copyText(paymentHref)}>复制支付链接</Button></div> : null}
      {display.kind === 'redirect' || display.kind === 'form' ? <Button tone="primary" onClick={openPayment}><ExternalLink size={16} aria-hidden="true" />打开支付页</Button> : null}
      {display.kind === 'mock' ? <Button tone="ghost" busy={busy} onClick={onMockPay}>模拟支付成功</Button> : null}
      {display.kind === 'stripe' && display.publishableKey && display.clientSecret ? (
        <Suspense fallback={<LoadingState label="加载 Stripe 安全支付..." />}>
          <StripePaymentPanel publishableKey={display.publishableKey} clientSecret={display.clientSecret} disabled={busy} onConfirmed={onConfirmed} />
        </Suspense>
      ) : null}
    </section>
  )
}

function PaymentQRCode({ value }: { value: string }) {
  const [dataUrl, setDataUrl] = useState('')
  useEffect(() => {
    let active = true
    void toDataURL(value, { margin: 1, width: 208 }).then((url) => active && setDataUrl(url)).catch(() => active && setDataUrl(''))
    return () => { active = false }
  }, [value])
  return dataUrl ? <img className="size-52 rounded-lg bg-white p-3" src={dataUrl} alt="支付二维码" /> : <div className="size-52 rounded-lg bg-white" />
}
