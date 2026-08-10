import { FormEvent, useEffect, useMemo, useState } from 'react'
import type { CashierOptions, CashierOrder, CashierPlan, PublicPaymentVisibleMethod } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { userApi } from '../../../shared/user-api'
import { Button, EmptyState, ErrorState, LoadingState, useApp } from '../components'
import { rdBilling } from '../ui/redesign-classes'
import { QrCode } from '../ui/icons'
import { errorMessage } from '../useApiResource'
import { checkoutPaymentMethodEmptyState, checkoutPlanEmptyState, checkoutUnavailableEmptyState, type CheckoutUnavailableEmptyState } from './checkoutEmptyState'
import { checkoutPaymentDisplayModel } from './checkoutPaymentDisplay'
import { checkoutPaymentErrorMessage } from './checkoutPaymentError'
import { closePaymentWindow, dispatchPaymentWindow, paymentMethodNeedsReservedWindow, reservePaymentWindow } from './checkoutPaymentWindow'
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

const checkoutClasses = {
  page: 'w-full flex-1 px-4 py-6 sm:px-6 md:px-10 md:py-8',
  header: 'mb-8 border-b border-[var(--border)] pb-7',
  title: 'mb-3 text-[clamp(2rem,4vw,3.25rem)] font-black leading-none',
  detail: 'max-w-3xl text-sm leading-relaxed text-[var(--muted)] md:text-base',
  layout: rdBilling.layout,
  panel: cn(rdBilling.card, 'grid min-w-0 gap-6'),
  sectionHeading: 'text-sm font-bold text-[var(--muted)]',
  optionGrid: rdBilling.planGrid,
  optionButton: rdBilling.planItem,
  optionActive: rdBilling.planActive,
  optionLabel: 'text-[13px] text-[var(--muted)]',
  planPoints: rdBilling.planPrice,
  planPrice: 'text-[13px] not-italic text-[var(--fg)]',
  optionMeta: 'text-xs font-semibold text-[var(--muted)]',
  methodGrid: 'grid grid-cols-2 gap-2 max-[420px]:grid-cols-1',
  methodButton: 'group flex min-h-[64px] min-w-0 cursor-pointer items-center gap-3 rounded-lg border border-[var(--border)] bg-[var(--bg)]/50 p-3 text-left text-[var(--fg)] transition-colors hover:border-[var(--accent)] motion-reduce:transition-none',
  methodIcon: 'grid size-10 shrink-0 place-items-center overflow-hidden rounded-lg border border-[var(--border)] bg-white text-[var(--accent)]',
  custom: 'mt-4 w-full rounded-2xl border border-[var(--border)] bg-[var(--surface)]/80 px-4 py-3',
  customResult: 'grid min-h-[70px] gap-1.5 rounded-xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] p-3.5',
  customResultLabel: 'text-xs text-[var(--muted)]',
  customResultValue: 'font-mono text-2xl text-[var(--accent)]',
  sectionTitle: 'text-sm font-bold text-[var(--muted)]',
  orderPanel: rdBilling.orderPanel,
  order: 'grid gap-3',
  orderTitle: rdBilling.orderTitle,
  orderField: rdBilling.orderRow,
  orderLabel: 'text-xs text-[var(--muted)]',
  orderValue: 'min-w-0 [overflow-wrap:anywhere]',
  orderHint: 'text-[13px] leading-normal not-italic text-[var(--muted)]',
  orderSectionTitle: 'text-xs font-bold text-[var(--muted)]',
  orderTotalRow: cn(rdBilling.orderRow, 'mt-2 border-0 pt-6'),
  orderTotal: rdBilling.orderTotal,
  payButton: 'relative mt-1 grid h-14 w-full place-items-center overflow-hidden rounded-xl bg-[var(--accent)] px-5 text-base font-black text-[#111218] shadow-[0_14px_34px_rgba(var(--accent-rgb),0.24)] transition-transform hover:scale-[1.01] active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-55 disabled:hover:scale-100 motion-reduce:transform-none motion-reduce:transition-none',
  actions: 'flex flex-wrap justify-end gap-3 max-[420px]:flex-col max-[420px]:items-stretch',
  recentActions: 'flex flex-wrap items-center justify-end gap-2 md:col-start-4 md:justify-self-end',
  recent: 'mt-10 grid gap-4 border-t border-[var(--border)] pt-8',
  recentTitle: 'flex items-center justify-between gap-3',
  recentList: 'grid gap-2.5',
  recentRow: 'grid grid-cols-1 items-center gap-3 rounded-xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--surface)_86%,var(--bg))] p-3.5 text-left text-[var(--fg)] md:grid-cols-[minmax(180px,1.25fr)_minmax(120px,.72fr)_minmax(120px,.72fr)_minmax(220px,.9fr)]',
  recentRowActive: 'border-[var(--accent)] bg-[color-mix(in_oklch,var(--accent)_10%,var(--surface))]',
  recentCell: 'grid min-w-0 gap-1',
  recentStrong: 'min-w-0 [overflow-wrap:anywhere]',
  recentMeta: 'text-xs not-italic text-[var(--muted)] [overflow-wrap:anywhere]',
}

export function CheckoutPage() {
  const app = useApp()
  const [options, setOptions] = useState<CashierOptions | null>(null)
  const [selectedPlan, setSelectedPlan] = useState('')
  const [selectedMethod, setSelectedMethod] = useState('')
  const [customAmount, setCustomAmount] = useState('25.00')
  const [purchaseType, setPurchaseType] = useState<'plan' | 'custom_amount'>('plan')
  const [monitorOrder, setMonitorOrder] = useState<CashierOrder | null>(null)
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
    if (purchaseType === 'custom_amount' && !normalizedCustomAmount.valid) {
      app.notify('error', normalizedCustomAmount.error ?? '请输入有效金额')
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
      dispatchPaymentWindow(paymentWindow, checkoutPaymentDisplayModel(nextOrder))
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
    <div className={checkoutClasses.page}>
      <div className={checkoutClasses.header}>
        <div className="flex flex-col gap-5 md:flex-row md:items-end md:justify-between">
          <div>
            <h1 className={checkoutClasses.title}>积分充值</h1>
            <p className={checkoutClasses.detail}>自定义金额充值积分长期有效。</p>
          </div>
          <Button tone="ghost" onClick={() => void load()}>刷新配置</Button>
        </div>
      </div>

      <div className={checkoutClasses.layout}>
        <div className="flex flex-col gap-8">
          <form id="checkout-order-form" className={checkoutClasses.panel} onSubmit={createOrder}>
            <h3 className={checkoutClasses.sectionHeading}>选择积分包</h3>
            <section className={checkoutClasses.optionGrid} aria-label="积分包与自定义金额">
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
              {options.custom_amount.enabled ? (
                <CustomAmountCard
                  active={purchaseType === 'custom_amount'}
                  customAmount={customAmount}
                  customPoints={customPoints}
                  unitPrice={customUnitPrice}
                  error={normalizedCustomAmount.valid ? undefined : normalizedCustomAmount.error}
                  rangeLabel={`范围 ${checkoutMoney(options.custom_amount.min_amount_cny)} - ${checkoutMoney(options.custom_amount.max_amount_cny)}`}
                  onSelect={() => setPurchaseType('custom_amount')}
                  onChange={(value) => {
                    setPurchaseType('custom_amount')
                    setCustomAmount(value)
                  }}
                />
              ) : null}
              {!activePlans.length && !options.custom_amount.enabled ? (
                <CheckoutInlineEmptyState empty={checkoutPlanEmptyState()} onRefresh={load} onBalance={() => app.navigate('profile')} />
              ) : null}
            </section>
          </form>
        </div>

        <aside className={checkoutClasses.orderPanel}>
          <h3 className={checkoutClasses.orderTitle}>订单概览</h3>
          <div className={checkoutClasses.order}>
            <div className={checkoutClasses.orderField}><span className={checkoutClasses.orderLabel}>购买项目</span><strong className={checkoutClasses.orderValue}>{selectedPurchase.name}</strong></div>
            <div className={checkoutClasses.orderField}><span className={checkoutClasses.orderLabel}>到账积分</span><strong className="text-[var(--accent)]">{selectedPurchase.points} ◈</strong></div>
            <div className={checkoutClasses.orderField}><span className={checkoutClasses.orderLabel}>支付金额</span><strong className={checkoutClasses.orderValue}>{selectedPurchase.amountLabel}</strong></div>
            <div className={checkoutClasses.orderTotalRow}><span className="text-xl font-black">应付总额</span><span className={checkoutClasses.orderTotal}>{selectedPurchase.amountLabel}</span></div>

            <section className="grid gap-3 pt-2" aria-label="支付方式">
              <div className={checkoutClasses.orderSectionTitle}>支付方式</div>
              <div className={checkoutClasses.methodGrid}>
                {methods.map((method) => (
                  <MethodButton key={method.method} method={method} active={selectedMethod === method.method} onSelect={() => setSelectedMethod(method.method)} />
                ))}
              </div>
              {!methods.length ? (
                <CheckoutInlineEmptyState empty={checkoutPaymentMethodEmptyState()} onRefresh={load} onBalance={() => app.navigate('profile')} />
              ) : null}
            </section>

            <button className={checkoutClasses.payButton} type="submit" form="checkout-order-form" disabled={busy || !methods.length || (purchaseType === 'plan' && !activePlans.length) || (purchaseType === 'custom_amount' && !normalizedCustomAmount.valid)}>
              {busy ? '创建中...' : '立即支付'}
            </button>
          </div>
        </aside>
      </div>

      <section className={checkoutClasses.recent} aria-label="最近充值订单">
        <div className={checkoutClasses.recentTitle}>
          <span>最近订单</span>
          <Button tone="ghost" busy={recentLoading} onClick={() => void loadRecentOrders()}>刷新</Button>
        </div>
        {recentError ? <ErrorState message={recentError} onRetry={loadRecentOrders} /> : null}
        {!recentError && recentLoading ? <LoadingState label="读取最近订单..." /> : null}
        {!recentError && !recentLoading && !recentRows.length ? <EmptyState title="暂无充值订单" detail="创建充值订单后，最近 10 条记录会显示在这里。" /> : null}
        {!recentError && !recentLoading && recentRows.length ? (
          <div className={checkoutClasses.recentList}>
            {recentRows.map((row) => (
              <article
                key={row.id}
                className={cn(checkoutClasses.recentRow, (monitorOrder?.id === row.id || detailOrder?.id === row.id) && checkoutClasses.recentRowActive)}
              >
                <span className={checkoutClasses.recentCell}>
                  <strong className={checkoutClasses.recentStrong}>{row.title}</strong>
                  <em className={checkoutClasses.recentMeta}>{row.orderNo}</em>
                </span>
                <span className={checkoutClasses.recentCell}>
                  <strong className={checkoutClasses.recentStrong}>{row.amount}</strong>
                  <em className={checkoutClasses.recentMeta}>{row.order.purchase_type === 'custom_amount' ? `到账 ${row.points} 积分` : `套餐 ${row.basePoints} · 赠送 ${row.bonusPoints}`}</em>
                  <em className={checkoutClasses.recentMeta}>{row.creditValidity}</em>
                </span>
                <span className={checkoutClasses.recentCell}>
                  <strong className={checkoutClasses.recentStrong}>{row.status}</strong>
                  <em className={checkoutClasses.recentMeta}>{row.method}</em>
                </span>
                <div className={checkoutClasses.recentActions}>
                  <time className={checkoutClasses.recentMeta} dateTime={row.createdAt}>{row.createdAtLabel}</time>
                  {checkoutOrderActionState(row.order).canContinuePay ? (
                    <>
                      <Button tone="ghost" onClick={() => openPaymentMonitor(row.order)}>继续支付</Button>
                      <Button tone="ghost" busy={busy} onClick={() => void cancelOrder(row.order)}>取消支付</Button>
                    </>
                  ) : (
                    <Button tone="ghost" onClick={() => openOrderDetail(row.order)}>查看订单</Button>
                  )}
                </div>
              </article>
            ))}
          </div>
        ) : null}
      </section>

      <section className="mt-10 grid max-w-xl gap-4 border-t border-[var(--border)] pt-8" aria-label="兑换积分">
        <div>
          <h2 className="m-0 text-lg font-black text-[var(--fg)]">兑换积分</h2>
          <p className="m-0 mt-1 text-sm text-[var(--muted)]">输入有效兑换码后，积分会直接进入账户余额。</p>
        </div>
        <RedeemCodeForm onRedeemed={() => Promise.all([app.refreshAccount(), loadRecentOrders()]).then(() => undefined)} />
      </section>

      {monitorOrder ? (
        <PaymentMonitorModal
          order={monitorOrder}
          busy={busy}
          onOrderChange={setMonitorOrder}
          onSuccess={(next) => {
            setMonitorOrder(next)
            app.notify('success', '支付成功，积分余额已刷新')
            void app.refreshAccount()
            void loadRecentOrders()
          }}
          onClose={() => {
            setMonitorOrder(null)
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
    <div className={checkoutClasses.actions}>
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
    <button type="button" className={cn(checkoutClasses.optionButton, active && checkoutClasses.optionActive)} onClick={onSelect}>
      <span className={checkoutClasses.optionLabel}>{plan.plan_name}</span>
      <strong className={checkoutClasses.planPoints}>{checkoutPoints(plan.points)} 积分</strong>
      <em className={checkoutClasses.planPrice}>{checkoutMoney(plan.price_cny)}{Number(plan.bonus_points ?? '0') > 0 ? ` / 赠 ${checkoutPoints(plan.bonus_points)}` : ''}</em>
      <span className={checkoutClasses.optionMeta}>{checkoutPlanValidityLabel(plan)}</span>
    </button>
  )
}

function CustomAmountCard({ active, customAmount, customPoints, unitPrice, error, rangeLabel, onSelect, onChange }: {
  active: boolean
  customAmount: string
  customPoints: string
  unitPrice: string
  error?: string
  rangeLabel: string
  onSelect: () => void
  onChange: (value: string) => void
}) {
  return (
    <button type="button" className={cn(checkoutClasses.optionButton, active && checkoutClasses.optionActive)} onClick={onSelect}>
      <span className={checkoutClasses.optionLabel}>自定义金额</span>
      <strong className={checkoutClasses.planPoints}>{customPoints} 积分</strong>
      <em className={checkoutClasses.planPrice}>{error ?? unitPrice}</em>
      <span className={checkoutClasses.custom} onClick={(event) => event.stopPropagation()}>
        <label className="mb-2 block text-[10px] font-vault-mono uppercase tracking-widest text-[var(--muted)]">支付金额 / 1-10000</label>
        <span className="flex items-center gap-2">
          <span className="text-sm font-bold text-[var(--muted)]">¥</span>
          <input
            className="w-full bg-transparent text-xl font-black text-[var(--fg)] outline-none placeholder:text-[var(--muted)]"
            inputMode="decimal"
            min="1"
            max="10000"
            value={customAmount}
            onFocus={onSelect}
            onChange={(event) => onChange(event.target.value.replace(/[^\d.]/g, ''))}
            placeholder="100"
          />
        </span>
        <span className="mt-2 block text-[11px] text-[var(--muted)]">{rangeLabel}</span>
      </span>
    </button>
  )
}

function MethodButton({ method, active, onSelect }: { method: PublicPaymentVisibleMethod; active: boolean; onSelect: () => void }) {
  const display = checkoutPublicPaymentMethod(method)
  return (
    <button type="button" className={cn(checkoutClasses.optionButton, checkoutClasses.methodButton, active && checkoutClasses.optionActive)} onClick={onSelect}>
      <span className={checkoutClasses.methodIcon}><PaymentMethodBrandIcon brand={display.icon} /></span>
      <span className="grid min-w-0 gap-1">
        <strong>{display.label}</strong>
        <span className={checkoutClasses.optionLabel}>{display.detail}</span>
      </span>
    </button>
  )
}

function PaymentMethodBrandIcon({ brand }: { brand: CheckoutPaymentBrand }) {
  const source = brand === 'alipay' ? alipayIcon : brand === 'wechat-pay' ? wechatPayIcon : brand === 'stripe' ? stripeIcon : ''
  return source ? <img className="size-full object-cover" src={source} alt="" aria-hidden="true" /> : <QrCode size={19} strokeWidth={1.6} aria-hidden="true" />
}
