import { FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import type { CashierOptions, CashierOrder, CashierPlan, PaymentVisibleMethod } from '../../../shared/api-types'
import { userApi } from '../../../shared/user-api'
import { Button, EmptyState, ErrorState, Field, LoadingState, PageIntro, useApp } from '../components'
import { errorMessage } from '../useApiResource'
import { checkoutPaymentMethodEmptyState, checkoutPlanEmptyState, checkoutUnavailableEmptyState, type CheckoutUnavailableEmptyState } from './checkoutEmptyState'
import { checkoutPaymentDisplayModel } from './checkoutPaymentDisplay'
import type { CheckoutPaymentDisplayModel } from './checkoutPaymentDisplay'
import { checkoutDateTime, checkoutMoney, checkoutOrderActionState, checkoutOrderRuntimeState, checkoutPaymentMethodOptionModel, checkoutPoints, checkoutRecentOrderRows } from './checkoutOrderState'
import { checkoutPurchasablePlans } from './checkoutPlans'

export function CheckoutPage() {
  const app = useApp()
  const balanceRefreshedOrderID = useRef<number | null>(null)
  const [options, setOptions] = useState<CashierOptions | null>(null)
  const [selectedPlan, setSelectedPlan] = useState('')
  const [selectedMethod, setSelectedMethod] = useState('')
  const [customAmount, setCustomAmount] = useState('25.00')
  const [purchaseType, setPurchaseType] = useState<'plan' | 'custom_amount'>('plan')
  const [order, setOrder] = useState<CashierOrder | null>(null)
  const [recentOrders, setRecentOrders] = useState<CashierOrder[]>([])
  const [recentLoading, setRecentLoading] = useState(true)
  const [recentError, setRecentError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [orderIdempotencyKey, setOrderIdempotencyKey] = useState(() => newCheckoutOrderIdempotencyKey())

  async function load() {
    setLoading(true)
    setError(null)
    try {
      const next = await userApi.getCashierOptions()
      setOptions(next)
      setSelectedPlan((current) => current || checkoutPurchasablePlans(next.plans)[0]?.plan_code || '')
      setSelectedMethod((current) => current || next.visible_methods.find((item) => item.enabled)?.method || '')
    } catch (caught) {
      setError(errorMessage(caught))
    } finally {
      setLoading(false)
    }
  }

  async function loadRecentOrders() {
    setRecentLoading(true)
    setRecentError(null)
    try {
      const result = await userApi.listCashierOrders(1, 10)
      setRecentOrders(result.items)
    } catch (caught) {
      setRecentError(errorMessage(caught))
    } finally {
      setRecentLoading(false)
    }
  }

  useEffect(() => {
    void load()
    void loadRecentOrders()
  }, [])

  const activePlans = useMemo(() => checkoutPurchasablePlans(options?.plans ?? []), [options])
  const methods = useMemo(() => (options?.visible_methods ?? []).filter((item) => item.enabled), [options])
  const currentPlan = activePlans.find((item) => item.plan_code === selectedPlan)
  const customPoints = options?.custom_amount?.cny_per_point
    ? (Number(customAmount || '0') / Number(options.custom_amount.cny_per_point)).toFixed(2)
    : '0.00'
  const orderRuntime = useMemo(() => checkoutOrderRuntimeState(order), [order])
  const orderActions = useMemo(() => checkoutOrderActionState(order), [order])
  const recentRows = useMemo(() => checkoutRecentOrderRows(recentOrders), [recentOrders])

  useEffect(() => {
    if (!order) return
    if (orderRuntime.step === 'success' && balanceRefreshedOrderID.current !== order.id) {
      balanceRefreshedOrderID.current = order.id
      void app.refreshAccount()
      return
    }
    if (!orderRuntime.shouldPoll) return
    const timer = window.setTimeout(() => {
      void userApi.getCashierOrder(order.id)
        .then((next) => {
          setOrder(next)
          const nextRuntime = checkoutOrderRuntimeState(next)
          if (nextRuntime.step === 'success') {
            app.notify('success', '支付成功，充值余额已刷新')
            void loadRecentOrders()
          }
          if (nextRuntime.step === 'expired') app.notify('error', '订单已过期，请重新创建')
        })
        .catch((caught) => {
          app.notify('error', errorMessage(caught))
        })
    }, 2000)
    return () => window.clearTimeout(timer)
  }, [app, order, orderRuntime])

  async function createOrder(event: FormEvent) {
    event.preventDefault()
    if (!selectedMethod) {
      app.notify('error', '请选择支付方式')
      return
    }
    if (purchaseType === 'plan' && !selectedPlan) {
      app.notify('error', '请选择积分包')
      return
    }
    setBusy(true)
    try {
      const nextOrder = await userApi.createCashierOrder({
        purchase_type: purchaseType,
        plan_code: purchaseType === 'plan' ? selectedPlan : undefined,
        amount_cny: purchaseType === 'custom_amount' ? customAmount : undefined,
        visible_method: selectedMethod,
        client_return_url: `${window.location.origin}${window.location.pathname}#/checkout`,
      }, orderIdempotencyKey)
      setOrder(nextOrder)
      void loadRecentOrders()
      balanceRefreshedOrderID.current = null
      setOrderIdempotencyKey(newCheckoutOrderIdempotencyKey())
      app.notify('success', '订单已创建，请继续完成支付')
    } catch (caught) {
      app.notify('error', errorMessage(caught))
    } finally {
      setBusy(false)
    }
  }

  async function refreshOrder() {
    if (!order) return
    setBusy(true)
    try {
      const next = await userApi.getCashierOrder(order.id)
      setOrder(next)
      void loadRecentOrders()
      if (checkoutOrderRuntimeState(next).step === 'success') {
        balanceRefreshedOrderID.current = next.id
        await app.refreshAccount()
      }
    } catch (caught) {
      app.notify('error', errorMessage(caught))
    } finally {
      setBusy(false)
    }
  }

  async function mockPay() {
    if (!order) return
    setBusy(true)
    try {
      const next = await userApi.mockPayCashierOrder(order.id)
      setOrder(next)
      void loadRecentOrders()
      balanceRefreshedOrderID.current = next.id
      await app.refreshAccount()
      app.notify('success', '模拟支付成功，充值余额已刷新')
    } catch (caught) {
      app.notify('error', errorMessage(caught))
    } finally {
      setBusy(false)
    }
  }

  async function cancelOrder() {
    if (!order) return
    setBusy(true)
    try {
      const next = await userApi.cancelCashierOrder(order.id)
      setOrder(next)
      void loadRecentOrders()
      app.notify('success', '订单已取消，可重新创建支付订单')
    } catch (caught) {
      app.notify('error', errorMessage(caught))
    } finally {
      setBusy(false)
    }
  }

  if (loading) return <LoadingState label="读取收银台配置..." />
  if (error) return <ErrorState message={error} onRetry={load} />
  if (!options) {
    const empty = checkoutUnavailableEmptyState()
    return (
      <EmptyState
        title={empty.title}
        detail={empty.detail}
        action={<CheckoutEmptyActions empty={empty} onRefresh={load} onBalance={() => app.navigate('profile')} />}
      />
    )
  }

  return (
    <div className="content checkout-page">
      <PageIntro
        eyebrow="CHECKOUT"
        title="积分充值"
        detail="固定积分包和自定义金额统一通过收银台创建订单，支付成功后进入充值余额桶且不过期。"
        action={<Button tone="ghost" onClick={() => void load()}>刷新配置</Button>}
      />

      <div className="checkout-layout">
        <form className="checkout-panel card" onSubmit={createOrder}>
          <div className="checkout-tabs" role="tablist" aria-label="充值类型">
            <button type="button" className={purchaseType === 'plan' ? 'active' : ''} onClick={() => setPurchaseType('plan')}>固定积分包</button>
            <button type="button" className={purchaseType === 'custom_amount' ? 'active' : ''} onClick={() => setPurchaseType('custom_amount')} disabled={!options.custom_amount.enabled}>自定义金额</button>
          </div>

          {purchaseType === 'plan' ? (
            <section className="checkout-plan-grid" aria-label="固定积分包">
              {activePlans.map((plan) => (
                <PlanButton key={plan.plan_code} plan={plan} active={selectedPlan === plan.plan_code} onSelect={() => setSelectedPlan(plan.plan_code)} />
              ))}
              {!activePlans.length ? (
                <CheckoutInlineEmptyState empty={checkoutPlanEmptyState()} onRefresh={load} onBalance={() => app.navigate('profile')} />
              ) : null}
            </section>
          ) : (
            <section className="checkout-custom card">
              <Field label="充值金额" hint={`范围 ${checkoutMoney(options.custom_amount.min_amount_cny)} - ${checkoutMoney(options.custom_amount.max_amount_cny)}`}>
                <input className="input" inputMode="decimal" value={customAmount} onChange={(event) => setCustomAmount(event.target.value)} />
              </Field>
              <div className="checkout-custom-result">
                <span>预计到账积分</span>
                <strong>{customPoints}</strong>
              </div>
            </section>
          )}

          <section className="checkout-methods" aria-label="支付方式">
            <div className="card-title">支付方式</div>
            <div className="checkout-method-grid">
              {methods.map((method) => (
                <MethodButton key={method.method} method={method} active={selectedMethod === method.method} onSelect={() => setSelectedMethod(method.method)} />
              ))}
            </div>
            {!methods.length ? (
              <CheckoutInlineEmptyState empty={checkoutPaymentMethodEmptyState()} onRefresh={load} onBalance={() => app.navigate('profile')} />
            ) : null}
          </section>

          <Button type="submit" busy={busy} disabled={!methods.length || (purchaseType === 'plan' && !activePlans.length)}>
            创建支付订单
          </Button>
        </form>

        <aside className="checkout-panel card">
          <div className="card-title">订单状态</div>
          {order ? (
            <div className="checkout-order">
              <div><span>订单号</span><strong>{order.order_no}</strong></div>
              <div><span>状态</span><strong>{orderRuntime.label}</strong><em>{orderRuntime.detail}</em></div>
              {orderActions.terminalLabel ? <div><span>结果</span><strong>{orderActions.terminalLabel}</strong></div> : null}
              <div><span>金额</span><strong>{checkoutMoney(order.amount_cny)}</strong></div>
              <div><span>到账积分</span><strong>{checkoutPoints(order.points)}</strong></div>
              <div><span>过期时间</span><strong>{checkoutDateTime(order.expires_at)}</strong></div>
              <PaymentDisplayPanel order={order} />
              <div className="action-row">
                <Button tone="ghost" busy={busy} onClick={() => void refreshOrder()}>刷新订单</Button>
                {orderActions.canCancel ? <Button tone="ghost" busy={busy} onClick={() => void cancelOrder()}>{orderActions.cancelLabel}</Button> : null}
                {orderActions.canMockPay ? <Button tone="ghost" busy={busy} onClick={() => void mockPay()}>模拟支付成功</Button> : null}
              </div>
            </div>
          ) : (
            <EmptyState title="尚未创建订单" detail="选择积分包或自定义金额后创建订单，支付二维码和到账状态会显示在这里。" />
          )}
        </aside>
      </div>

      <section className="checkout-recent card" aria-label="最近充值订单">
        <div className="card-title">
          <span>最近订单</span>
          <Button tone="ghost" busy={recentLoading} onClick={() => void loadRecentOrders()}>刷新</Button>
        </div>
        {recentError ? <ErrorState message={recentError} onRetry={loadRecentOrders} /> : null}
        {!recentError && recentLoading ? <LoadingState label="读取最近订单..." /> : null}
        {!recentError && !recentLoading && !recentRows.length ? <EmptyState title="暂无充值订单" detail="创建充值订单后，最近 10 条记录会显示在这里。" /> : null}
        {!recentError && !recentLoading && recentRows.length ? (
          <div className="checkout-recent-list">
            {recentRows.map((row) => (
              <button
                key={row.id}
                type="button"
                className={`checkout-recent-row${order?.id === row.id ? ' active' : ''}`}
                onClick={() => {
                  setOrder(row.order)
                  balanceRefreshedOrderID.current = checkoutOrderRuntimeState(row.order).step === 'success' ? row.id : null
                }}
              >
                <span>
                  <strong>{row.title}</strong>
                  <em>{row.orderNo}</em>
                </span>
                <span>
                  <strong>{row.amount}</strong>
                  <em>{row.points} 积分</em>
                </span>
                <span>
                  <strong>{row.status}</strong>
                  <em>{row.method}</em>
                </span>
                <time dateTime={row.createdAt}>{row.createdAtLabel}</time>
              </button>
            ))}
          </div>
        ) : null}
      </section>
    </div>
  )
}

function PaymentDisplayPanel({ order }: { order: CashierOrder }) {
  const display = checkoutPaymentDisplayModel(order)
  const openForm = () => {
    if (!display.formHtml) return
    const popup = window.open('', '_blank')
    if (!popup) return
    popup.opener = null
    popup.document.write(display.formHtml)
    popup.document.close()
  }
  return (
    <section className={`checkout-payment-display ${display.kind}`}>
      <span>{display.label}</span>
      <p>{display.detail}</p>
      {display.href ? (
        <>
          {display.kind === 'qr_code' ? <code>{display.href}</code> : null}
          <a className="btn btn-ghost" href={display.href} target="_blank" rel="noreferrer">{display.kind === 'qr_code' ? '打开二维码链接' : '打开支付页'}</a>
        </>
      ) : null}
      {display.kind === 'form' ? <Button tone="ghost" onClick={openForm}>打开支付表单</Button> : null}
    </section>
  )
}

function newCheckoutOrderIdempotencyKey() {
  const random = typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `checkout-order-${random}`
}

function CheckoutEmptyActions({ empty, onRefresh, onBalance }: { empty: CheckoutUnavailableEmptyState; onRefresh: () => void | Promise<void>; onBalance: () => void }) {
  return (
    <div className="action-row">
      <Button tone="primary" onClick={() => void onRefresh()}>{empty.primaryAction}</Button>
      <Button tone="ghost" onClick={onBalance}>{empty.secondaryAction}</Button>
    </div>
  )
}

function CheckoutInlineEmptyState({ empty, onRefresh, onBalance }: { empty: CheckoutUnavailableEmptyState; onRefresh: () => void | Promise<void>; onBalance: () => void }) {
  return (
    <EmptyState
      title={empty.title}
      detail={empty.detail}
      action={<CheckoutEmptyActions empty={empty} onRefresh={onRefresh} onBalance={onBalance} />}
    />
  )
}

function PlanButton({ plan, active, onSelect }: { plan: CashierPlan; active: boolean; onSelect: () => void }) {
  return (
    <button type="button" className={`checkout-plan ${active ? 'active' : ''}`} onClick={onSelect}>
      <span>{plan.plan_name}</span>
      <strong>{checkoutPoints(plan.points)} 积分</strong>
      <em>{checkoutMoney(plan.price_cny)}{Number(plan.bonus_points ?? '0') > 0 ? ` / 赠 ${checkoutPoints(plan.bonus_points)}` : ''}</em>
    </button>
  )
}

function MethodButton({ method, active, onSelect }: { method: PaymentVisibleMethod; active: boolean; onSelect: () => void }) {
  const display = checkoutPaymentMethodOptionModel(method)
  return (
    <button type="button" className={`checkout-method ${active ? 'active' : ''}`} onClick={onSelect}>
      <strong>{display.label}</strong>
      <span>{display.detail}</span>
    </button>
  )
}
