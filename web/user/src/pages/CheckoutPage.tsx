import { FormEvent, useEffect, useMemo, useState } from 'react'
import type { CashierOptions, CashierOrder, CashierPlan, PublicPaymentVisibleMethod } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { formatPoints, pointsExchangeRules } from '../../../shared/pointsDisplay'
import { userApi } from '../../../shared/user-api'
import { Button, EmptyState, ErrorState, LoadingState, useApp } from '../components'
import { isMobileDeviceUA } from '../device'
import { QrCode } from '../ui/icons'
import { errorMessage } from '../useApiResource'
import { checkoutPaymentMethodEmptyState, checkoutPlanEmptyState, checkoutUnavailableEmptyState, type CheckoutUnavailableEmptyState } from './checkoutEmptyState'
import { checkoutPaymentDisplayModel } from './checkoutPaymentDisplay'
import { checkoutPaymentErrorMessage } from './checkoutPaymentError'
import { assignCurrentWindowToPaymentURL, closePaymentWindow, dispatchPaymentWindow, paymentMethodNeedsReservedWindow, redirectPaymentFallbackAction, reservePaymentWindow } from './checkoutPaymentWindow'
import { checkoutPublicPaymentMethod, type CheckoutPaymentBrand } from './checkoutPaymentMethods'
import { checkoutCancelResultState, checkoutMoney, checkoutOrderActionState, checkoutPoints, checkoutRecentOrderRows } from './checkoutOrderState'
import { checkoutPlanValidityLabel, checkoutPurchasablePlans } from './checkoutPlans'
import { cnyPerPointLabel, customAmountPoints, normalizeCustomAmount } from './checkoutCustomAmount'
import { RedeemCodeForm } from './RedeemCodeForm'
import { PaymentMonitorModal } from './PaymentMonitorModal'
import { PaymentOrderDetailModal } from './PaymentOrderDetailModal'
import alipayIcon from '../assets/payment/alipay.svg'
import wechatPayIcon from '../assets/payment/wechat-pay.svg'
import stripeIcon from '../assets/payment/stripe.svg'
import './account-redesign.css'

// Restore the local motion preference as soon as the account bundle loads so
// it survives reloads without touching the application entry point.
try {
  document.documentElement.dataset.userMotion = window.localStorage.getItem('pic-gallery-user-motion') === 'reduced' ? 'reduced' : 'on'
} catch {
  // Storage may be unavailable; the default motion state still applies.
}

function checkoutOrderTone(status?: string) {
  const normalized = (status ?? '').trim().toLowerCase()
  if (normalized === 'paid' || normalized === 'completed') return 'u-status--ok'
  if (normalized === 'pending' || normalized === 'manual_review') return 'u-status--warn'
  if (normalized === 'failed' || normalized === 'refunded' || normalized === 'partially_refunded') return 'u-status--bad'
  return ''
}

export function CheckoutPage() {
  const app = useApp()
  const [options, setOptions] = useState<CashierOptions | null>(null)
  const [selectedPlan, setSelectedPlan] = useState('')
  const [selectedMethod, setSelectedMethod] = useState('')
  const [customAmount, setCustomAmount] = useState('25.00')
  const [purchaseType, setPurchaseType] = useState<'plan' | 'custom_amount'>('plan')
  const [monitorOrder, setMonitorOrder] = useState<CashierOrder | null>(null)
  const [redirectScanFallback, setRedirectScanFallback] = useState(false)
  const [detailOrder, setDetailOrder] = useState<CashierOrder | null>(null)
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
      const purchasablePlans = checkoutPurchasablePlans(next.plans)
      setSelectedPlan((current) => current || purchasablePlans[0]?.plan_code || '')
      setSelectedMethod((current) => current || next.visible_methods.find((item) => item.enabled)?.method || '')
      setPurchaseType((current) => purchasablePlans.length === 0 && next.custom_amount.enabled ? 'custom_amount' : current)
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
  const normalizedCustomAmount = normalizeCustomAmount(customAmount)
  const customPoints = normalizedCustomAmount.valid
    ? customAmountPoints(normalizedCustomAmount.amount, options?.custom_amount?.cny_per_point)
    : '0.00'
  const customUnitPrice = cnyPerPointLabel(options?.custom_amount?.cny_per_point)
  const selectedPurchase = purchaseType === 'custom_amount'
    ? {
      name: '自定义金额',
      points: normalizedCustomAmount.valid ? customPoints : '0.00',
      amountLabel: normalizedCustomAmount.valid ? checkoutMoney(normalizedCustomAmount.value) : normalizedCustomAmount.error ?? '金额无效',
    }
    : {
      name: currentPlan?.plan_name ?? '未选择积分包',
      points: currentPlan ? checkoutPoints(String(Number(currentPlan.points || 0) + Number(currentPlan.bonus_points || 0))) : '0.00',
      amountLabel: currentPlan ? checkoutMoney(currentPlan.price_cny) : '-',
    }
  const recentRows = useMemo(() => checkoutRecentOrderRows(recentOrders), [recentOrders])
  const balanceText = app.balance ? formatPoints(app.balance.available_points).text : '—'

  async function createOrder(event: FormEvent) {
    event.preventDefault()
    if (!selectedMethod) {
      app.notify('warn', '请选择支付方式')
      return
    }
    if (purchaseType === 'plan' && !selectedPlan) {
      app.notify('warn', '请选择积分包')
      return
    }
    if (purchaseType === 'custom_amount' && !normalizedCustomAmount.valid) {
      app.notify('warn', normalizedCustomAmount.error ?? '请输入有效金额')
      return
    }
    const selectedPaymentMethod = methods.find((method) => method.method === selectedMethod)
    const paymentWindow = paymentMethodNeedsReservedWindow(selectedPaymentMethod) ? reservePaymentWindow() : null
    setBusy(true)
    try {
      const nextOrder = await userApi.createCashierOrder({
        purchase_type: purchaseType,
        plan_code: purchaseType === 'plan' ? selectedPlan : undefined,
        amount_cny: purchaseType === 'custom_amount' ? normalizedCustomAmount.value : undefined,
        visible_method: selectedMethod,
        client_return_url: `${window.location.origin}${window.location.pathname}#/checkout`,
      }, orderIdempotencyKey)
      const display = checkoutPaymentDisplayModel(nextOrder)
      const dispatched = dispatchPaymentWindow(paymentWindow, checkoutPaymentDisplayModel(nextOrder))
      const redirectFallback = redirectPaymentFallbackAction(paymentWindow, display, dispatched, isMobileDeviceUA())
      if (redirectFallback.kind === 'assign') assignCurrentWindowToPaymentURL(redirectFallback.href)
      setRedirectScanFallback(redirectFallback.kind === 'qr')
      setDetailOrder(null)
      setMonitorOrder(nextOrder)
      void loadRecentOrders()
      setOrderIdempotencyKey(newCheckoutOrderIdempotencyKey())
      app.notify('success', '订单已创建，请继续完成支付')
    } catch (caught) {
      closePaymentWindow(paymentWindow)
      void loadRecentOrders()
      app.notify('error', checkoutPaymentErrorMessage(caught))
    } finally {
      setBusy(false)
    }
  }

  function openPaymentMonitor(nextOrder: CashierOrder) {
    setDetailOrder(null)
    setMonitorOrder(nextOrder)
  }

  function openOrderDetail(nextOrder: CashierOrder) {
    setMonitorOrder(null)
    setDetailOrder(nextOrder)
  }

  async function mockPay(target: CashierOrder) {
    setBusy(true)
    try {
      const next = await userApi.mockPayCashierOrder(target.id)
      setMonitorOrder(next)
    } catch (caught) {
      app.notify('error', errorMessage(caught))
    } finally {
      setBusy(false)
    }
  }

  async function cancelOrder(target: CashierOrder) {
    setBusy(true)
    try {
      const next = await userApi.cancelCashierOrder(target.id)
      const cancelResult = checkoutCancelResultState(next.status)
      const monitoredPayment = monitorOrder?.id === next.id
      setMonitorOrder((current) => current?.id === next.id ? next : current)
      setDetailOrder((current) => current?.id === next.id ? next : current)
      if (cancelResult === 'paid') {
        if (!monitoredPayment) {
          app.notify('success', '支付成功，积分余额已刷新')
          void app.refreshAccount()
          void loadRecentOrders()
        }
        return
      }
      void loadRecentOrders()
      if (cancelResult === 'canceled') {
        app.notify('success', '订单已取消，可重新创建支付订单')
      } else {
        app.notify('info', '订单状态未改变，请刷新支付结果后重试')
      }
    } catch (caught) {
      app.notify('error', errorMessage(caught))
    } finally {
      setBusy(false)
    }
  }

  if (loading) return <div className="acct-page"><LoadingState label="读取收银台配置..." /></div>
  if (error) return <div className="acct-page"><ErrorState message={error} onRetry={load} /></div>
  if (!options) {
    const empty = checkoutUnavailableEmptyState()
    return (
      <div className="acct-page">
        <EmptyState
          title={empty.title}
          detail={empty.detail}
          action={<CheckoutEmptyActions empty={empty} onRefresh={load} onBalance={() => app.navigate('profile')} />}
        />
      </div>
    )
  }

  return (
    <div className="acct-page">
      <div className="u-page-head">
        <div className="u-page-head__copy">
          <span className="u-eyebrow">Add Points</span>
          <h1 className="u-h1">积分充值</h1>
          <p className="u-sub">套餐购买的积分有效期以套餐说明为准；自定义金额即时到账。余额与消耗按积分等级自动换算展示。</p>
          <div className="u-points-rules" style={{ marginTop: 12 }} aria-label="积分等级与汇率">
            <strong>积分等级与汇率</strong>
            {pointsExchangeRules.map((rule) => <span key={rule}>{rule}</span>)}
            <span>展示口径：余额与消耗按等级自动换算（如 300 积分 = 3 银币积分）；本页积分包与订单记录展示原始积分值。</span>
          </div>
        </div>
        <div className="u-page-head__actions">
          <span className="u-chip u-chip--mono acct-balance-chip">可用积分 <span className="u-points">{balanceText}</span></span>
          <Button tone="ghost" onClick={() => void load()}>刷新配置</Button>
        </div>
      </div>

      <div className="ck-layout">
        <div className="acct-checkout-pane">
          <form id="checkout-order-form" onSubmit={createOrder} className="acct-checkout-form">
            <section aria-labelledby="checkout-plans-title">
              <div className="u-section-head">
                <div style={{ display: 'grid', gap: 5, minWidth: 0 }}>
                  <h2 id="checkout-plans-title" className="u-h3">选择套餐</h2>
                  <span className="u-meta">按积分从少到多</span>
                </div>
                <div className="u-segment" role="group" aria-label="充值方式">
                  <button type="button" className={purchaseType === 'plan' ? 'is-active' : undefined} disabled={!activePlans.length} onClick={() => setPurchaseType('plan')}>积分包</button>
                  <button type="button" className={purchaseType === 'custom_amount' ? 'is-active' : undefined} disabled={!options.custom_amount.enabled} onClick={() => setPurchaseType('custom_amount')}>自定义金额</button>
                </div>
              </div>
              <div className="ck-plans">
                {activePlans.map((plan) => (
                  <PlanButton
                    key={plan.plan_code}
                    plan={plan}
                    active={purchaseType === 'plan' && selectedPlan === plan.plan_code}
                    onSelect={() => {
                      setPurchaseType('plan')
                      setSelectedPlan(plan.plan_code)
                    }}
                  />
                ))}
                {!activePlans.length ? (
                  <div style={{ gridColumn: '1 / -1', minWidth: 0 }}>
                    <CheckoutInlineEmptyState empty={checkoutPlanEmptyState()} onRefresh={load} onBalance={() => app.navigate('profile')} />
                  </div>
                ) : null}
              </div>
            </section>

            {options.custom_amount.enabled ? (
              <section className={cn('ck-custom', purchaseType === 'custom_amount' && 'is-active')} aria-labelledby="checkout-custom-title">
                <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 10 }}>
                  <div style={{ display: 'grid', gap: 2, minWidth: 0 }}>
                    <strong id="checkout-custom-title" style={{ fontSize: 13 }}>自定义金额</strong>
                    <span className="u-meta">范围 {checkoutMoney(options.custom_amount.min_amount_cny)} - {checkoutMoney(options.custom_amount.max_amount_cny)}</span>
                  </div>
                  <label className="u-switch">
                    <input
                      type="checkbox"
                      checked={purchaseType === 'custom_amount'}
                      onChange={(event) => setPurchaseType(event.target.checked ? 'custom_amount' : 'plan')}
                    />
                    <span className="u-switch__track" aria-hidden="true" />
                    使用自定义金额
                  </label>
                </div>
                <div className="ck-custom__row">
                  <label className="u-field" style={{ flex: '0 1 180px', minWidth: 120 }}>
                    <span className="u-label">充值金额</span>
                    <input
                      className="u-input"
                      inputMode="decimal"
                      min="1"
                      max="10000"
                      value={customAmount}
                      onFocus={() => setPurchaseType('custom_amount')}
                      onChange={(event) => {
                        setPurchaseType('custom_amount')
                        setCustomAmount(event.target.value.replace(/[^\d.]/g, ''))
                      }}
                      placeholder="25.00"
                      aria-invalid={!normalizedCustomAmount.valid}
                    />
                  </label>
                  <span className="u-meta">元</span>
                  <div style={{ flex: 1 }} />
                  <div className="acct-custom-result">
                    <span className="u-meta">实时换算</span>
                    <strong className="u-num">{customPoints} 积分</strong>
                    <span className="u-meta">{customUnitPrice}</span>
                  </div>
                </div>
                {!normalizedCustomAmount.valid && normalizedCustomAmount.error ? <span className="u-field__error" role="alert">{normalizedCustomAmount.error}</span> : null}
              </section>
            ) : null}
          </form>

          <section aria-labelledby="checkout-recent-title">
            <div className="u-section-head">
              <h2 id="checkout-recent-title" className="u-h3">最近订单</h2>
              <span className="u-meta">最近 10 条</span>
            </div>
            <div className="u-plate" style={{ padding: '0 18px' }}>
              {recentError ? <ErrorState message={recentError} onRetry={loadRecentOrders} /> : null}
              {!recentError && recentLoading ? <LoadingState label="读取最近订单..." /> : null}
              {!recentError && !recentLoading && !recentRows.length ? <EmptyState title="暂无充值订单" detail="创建充值订单后，最近 10 条记录会显示在这里。" /> : null}
              {!recentError && !recentLoading && recentRows.length ? (
                <div className="u-list">
                  {recentRows.map((row) => {
                    const actions = checkoutOrderActionState(row.order)
                    const selected = monitorOrder?.id === row.id || detailOrder?.id === row.id
                    return (
                      <div className={cn('ck-order', selected && 'is-active')} key={row.id}>
                        <div className="ck-order__main">
                          <strong>{row.title}</strong>
                          <span className="u-meta">{row.orderNo} · {row.method} · {row.createdAtLabel}</span>
                          <span className="u-meta">
                            {row.order.purchase_type === 'custom_amount'
                              ? `到账 ${row.points} 积分`
                              : `套餐 ${row.basePoints} · 赠送 ${row.bonusPoints}`}
                            {' · '}{row.creditValidity}
                          </span>
                        </div>
                        <div className="ck-order__side">
                          <span className={cn('u-status', checkoutOrderTone(row.order.status))}><span className="u-dot" aria-hidden="true" />{row.status}</span>
                          <span className="u-num" style={{ fontSize: 13 }}>{row.amount}</span>
                          {actions.canContinuePay ? (
                            <>
                              <button className="u-btn u-btn--sm" type="button" onClick={() => openPaymentMonitor(row.order)}>继续支付</button>
                              <button className="u-btn u-btn--sm" type="button" disabled={busy} onClick={() => void cancelOrder(row.order)}>取消支付</button>
                            </>
                          ) : (
                            <button className="u-btn u-btn--sm" type="button" onClick={() => openOrderDetail(row.order)}>查看订单</button>
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
              ) : null}
            </div>
          </section>

          <section aria-labelledby="checkout-redeem-title">
            <div className="u-section-head">
              <h2 id="checkout-redeem-title" className="u-h3">兑换积分</h2>
            </div>
            <div className="u-plate u-plate--pad ck-redeem">
              <RedeemCodeForm onRedeemed={() => Promise.all([app.refreshAccount(), loadRecentOrders()]).then(() => undefined)} />
            </div>
          </section>
        </div>

        <aside className="ck-summary" aria-labelledby="checkout-summary-title">
          <div className="u-panel-title" style={{ margin: 0 }}>
            <h3 id="checkout-summary-title">订单概览</h3>
            <span>Step 1 / 2</span>
          </div>
          <div className="u-estimate">
            <div className="u-estimate__row"><span>购买项目</span><b>{selectedPurchase.name}</b></div>
            <div className="u-estimate__row"><span>到账积分</span><b>{selectedPurchase.points}</b></div>
            <div className="u-estimate__row"><span>支付金额</span><b>{selectedPurchase.amountLabel}</b></div>
            <div className="u-estimate__row u-estimate__row--total"><span>应付总额</span><b>{selectedPurchase.amountLabel}</b></div>
          </div>
          <div style={{ display: 'grid', gap: 8 }}>
            <span className="u-label">支付方式</span>
            <div className="ck-methods grid-cols-2">
              {methods.map((method) => (
                <MethodButton key={method.method} method={method} active={selectedMethod === method.method} onSelect={() => setSelectedMethod(method.method)} />
              ))}
            </div>
            {!methods.length ? (
              <CheckoutInlineEmptyState empty={checkoutPaymentMethodEmptyState()} onRefresh={load} onBalance={() => app.navigate('profile')} />
            ) : null}
          </div>
          <button
            className="u-btn u-btn--primary u-btn--lg u-btn--block"
            type="submit"
            form="checkout-order-form"
            disabled={busy || !methods.length || (purchaseType === 'plan' && !activePlans.length) || (purchaseType === 'custom_amount' && !normalizedCustomAmount.valid)}
          >
            {busy ? '创建中...' : `立即支付 ${selectedPurchase.amountLabel}`}
          </button>
          <p className="u-meta" style={{ lineHeight: 1.6 }}>支付成功后积分即时到账。订单与回调可追溯；未到账时可在“最近订单”中查看状态。</p>
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
            <span className="u-chip u-chip--mono">安全支付</span>
            <span className="u-chip u-chip--mono">订单可追溯</span>
            <span className="u-chip u-chip--mono">原路退款</span>
          </div>
        </aside>
      </div>

      {monitorOrder ? (
        <PaymentMonitorModal
          order={monitorOrder}
          busy={busy}
          redirectScanFallback={redirectScanFallback}
          onOrderChange={setMonitorOrder}
          onSuccess={(next) => {
            setMonitorOrder(next)
            app.notify('success', '支付成功，积分余额已刷新')
            void app.refreshAccount()
            void loadRecentOrders()
          }}
          onClose={() => {
            setMonitorOrder(null)
            setRedirectScanFallback(false)
          }}
          onCancel={(target) => void cancelOrder(target)}
          onMockPay={(target) => void mockPay(target)}
        />
      ) : null}
      {detailOrder ? <PaymentOrderDetailModal order={detailOrder} busy={busy} onCancel={(target) => void cancelOrder(target)} onClose={() => setDetailOrder(null)} /> : null}
    </div>
  )
}

function newCheckoutOrderIdempotencyKey() {
  const random = typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `checkout-order-${random}`
}

function CheckoutEmptyActions({ empty, onRefresh, onBalance }: { empty: CheckoutUnavailableEmptyState; onRefresh: () => void | Promise<void>; onBalance: () => void }) {
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', justifyContent: 'center', gap: 12 }}>
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
  const bonus = Number(plan.bonus_points ?? '0')
  return (
    <button type="button" className={cn('ck-plan', active && 'is-active')} aria-pressed={active} onClick={onSelect}>
      {active ? <span className="u-chip u-chip--accent ck-plan__selected">已选择</span> : null}
      <span className="ck-plan__name">{plan.plan_name}</span>
      <span className="ck-plan__points">{checkoutPoints(plan.points)} <small>积分</small></span>
      <span className="ck-plan__price">
        <span>{bonus > 0 ? `含赠送 ${checkoutPoints(plan.bonus_points)}` : '基础到账'}</span>
        <b>{checkoutMoney(plan.price_cny)}</b>
      </span>
      <span className="u-meta">{checkoutPlanValidityLabel(plan)}</span>
    </button>
  )
}

function MethodButton({ method, active, onSelect }: { method: PublicPaymentVisibleMethod; active: boolean; onSelect: () => void }) {
  const display = checkoutPublicPaymentMethod(method)
  return (
    <button type="button" className={cn('ck-method', active && 'is-active')} aria-pressed={active} title={display.detail} onClick={onSelect}>
      <span className="ck-method__logo"><PaymentMethodBrandIcon brand={display.icon} /></span>
      <span className="acct-method-label">{display.label}</span>
    </button>
  )
}

function PaymentMethodBrandIcon({ brand }: { brand: CheckoutPaymentBrand }) {
  const source = brand === 'alipay' ? alipayIcon : brand === 'wechat-pay' ? wechatPayIcon : brand === 'stripe' ? stripeIcon : ''
  return source ? <img className="size-full object-cover" src={source} alt="" aria-hidden="true" /> : <QrCode size={19} strokeWidth={1.6} aria-hidden="true" />
}