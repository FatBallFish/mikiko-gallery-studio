import { FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import { toDataURL } from 'qrcode'
import type { CashierOptions, CashierOrder, CashierPlan, PaymentVisibleMethod } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { userApi } from '../../../shared/user-api'
import { Button, EmptyState, ErrorState, LoadingState, Modal, useApp } from '../components'
import { userCard } from '../ui/classes'
import { rdBilling } from '../ui/redesign-classes'
import { errorMessage } from '../useApiResource'
import { checkoutPaymentMethodEmptyState, checkoutPlanEmptyState, checkoutUnavailableEmptyState, type CheckoutUnavailableEmptyState } from './checkoutEmptyState'
import { checkoutPaymentDisplayModel } from './checkoutPaymentDisplay'
import { checkoutDateTime, checkoutMoney, checkoutOrderActionState, checkoutOrderRuntimeState, checkoutOrderStatusLabel, checkoutPaymentMethodLabel, checkoutPaymentMethodOptionModel, checkoutPoints, checkoutRecentOrderRows } from './checkoutOrderState'
import { checkoutPurchasablePlans } from './checkoutPlans'
import { cnyPerPointLabel, customAmountPoints, normalizeCustomAmount } from './checkoutCustomAmount'

const checkoutClasses = {
  page: 'w-full flex-1 p-6 md:p-10',
  header: 'mb-12',
  title: 'mb-4 text-4xl font-black md:text-6xl',
  detail: 'text-lg leading-relaxed text-[var(--muted)]',
  layout: rdBilling.layout,
  panel: cn(rdBilling.card, 'grid min-w-0 gap-8'),
  sectionHeading: 'text-sm font-bold text-[var(--muted)]',
  optionGrid: rdBilling.planGrid,
  optionButton: rdBilling.planItem,
  optionActive: rdBilling.planActive,
  optionLabel: 'text-[13px] text-[var(--muted)]',
  planPoints: rdBilling.planPrice,
  planPrice: 'text-[13px] not-italic text-[var(--fg)]',
  methodGrid: 'grid grid-cols-1 gap-3',
  methodButton: 'group flex min-h-[74px] cursor-pointer items-center gap-3 rounded-[1.4rem] border border-[var(--border)] bg-[var(--bg)]/50 p-4 text-left text-[var(--fg)] transition-all duration-300 hover:border-[var(--accent)]',
  methodIcon: 'grid size-10 shrink-0 place-items-center rounded-xl border border-[var(--border)] bg-[var(--surface)] text-[var(--accent)]',
  custom: 'mt-4 w-full rounded-2xl border border-[var(--border)] bg-[var(--surface)]/80 px-4 py-3',
  customResult: 'grid min-h-[70px] gap-1.5 rounded-lg border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] p-3.5',
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
  payButton: 'relative mt-1 grid h-16 w-full place-items-center overflow-hidden rounded-2xl bg-[var(--accent)] px-5 text-lg font-black text-white shadow-[0_18px_42px_rgba(var(--accent-rgb),0.28)] transition hover:scale-[1.02] disabled:cursor-not-allowed disabled:opacity-55 disabled:hover:scale-100',
  actions: 'flex flex-wrap justify-end gap-3 max-[420px]:flex-col max-[420px]:items-stretch',
  paymentPrompt: 'mx-auto grid w-full max-w-[480px] justify-items-center gap-5 pt-3 text-center',
  paymentPromptBody: 'grid w-full justify-items-center gap-4',
  paymentPromptKicker: 'font-vault-mono text-xs text-[var(--muted)]',
  paymentPromptAmount: 'font-vault-display text-4xl font-black leading-none text-[var(--fg)]',
  paymentPromptDetail: 'm-0 max-w-[34rem] text-sm leading-relaxed text-[var(--muted)]',
  paymentQrImage: 'size-56 rounded-2xl border border-[color-mix(in_oklch,var(--fg)_12%,transparent)] bg-white p-3 shadow-[0_22px_60px_rgba(0,0,0,0.16)]',
  paymentCountdown: 'inline-flex min-h-10 items-center rounded-full border border-[color-mix(in_oklch,var(--accent)_28%,var(--border))] bg-[color-mix(in_oklch,var(--accent)_9%,var(--surface))] px-4 font-vault-mono text-sm font-bold text-[var(--accent)]',
  paymentRedirectPanel: 'grid w-full gap-5 rounded-2xl border border-[color-mix(in_oklch,var(--fg)_10%,transparent)] bg-[color-mix(in_oklch,var(--surface)_72%,var(--bg))] p-5 text-left',
  paymentRedirectAmount: 'flex items-end justify-between gap-4 border-b border-[var(--border)] pb-4',
  paymentRedirectTitle: 'text-lg font-black text-[var(--fg)]',
  paymentRedirectMeta: 'font-vault-mono text-xs text-[var(--muted)] [overflow-wrap:anywhere]',
  orderDetailBody: 'grid gap-6 pt-4',
  orderDetailHero: 'flex flex-col gap-4 border-b border-[var(--border)] pb-5 sm:flex-row sm:items-end sm:justify-between',
  orderDetailStatus: 'text-sm font-bold text-[var(--accent)]',
  orderDetailTitle: 'mt-1 text-2xl font-black text-[var(--fg)]',
  orderDetailAmount: 'font-vault-display text-4xl font-black leading-none text-[var(--fg)]',
  orderDetailList: 'grid gap-0 divide-y divide-[var(--border)]',
  orderDetailRow: 'grid gap-1 py-3 sm:grid-cols-[150px_minmax(0,1fr)] sm:items-start',
  orderDetailLabel: 'text-xs font-bold text-[var(--muted)]',
  orderDetailValue: 'text-sm font-semibold text-[var(--fg)] [overflow-wrap:anywhere]',
  orderDetailNote: 'rounded-xl bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] px-4 py-3 text-sm leading-relaxed text-[var(--muted)]',
  recentActions: 'flex flex-wrap items-center justify-end gap-2 md:col-start-4 md:justify-self-end',
  recent: cn(userCard.padded, 'mt-8 grid gap-4 rounded-[2rem] bg-[color-mix(in_oklch,var(--surface)_92%,var(--bg))] shadow-none'),
  recentTitle: 'flex items-center justify-between gap-3',
  recentList: 'grid overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--surface)]',
  recentRow: 'grid grid-cols-1 items-center gap-3 border-b border-[var(--border)] bg-[color-mix(in_oklch,var(--surface)_94%,var(--bg))] p-4 text-left text-[var(--fg)] last:border-b-0 md:grid-cols-[minmax(180px,1.25fr)_minmax(120px,.72fr)_minmax(120px,.72fr)_minmax(260px,.95fr)]',
  recentRowActive: 'bg-[color-mix(in_oklch,var(--accent)_8%,var(--surface))]',
  recentCell: 'grid min-w-0 gap-1',
  recentStrong: 'min-w-0 [overflow-wrap:anywhere]',
  recentMeta: 'text-xs not-italic text-[var(--muted)] [overflow-wrap:anywhere]',
}

export function CheckoutPage() {
  const app = useApp()
  const balanceRefreshedOrderID = useRef<number | null>(null)
  const [options, setOptions] = useState<CashierOptions | null>(null)
  const [selectedPlan, setSelectedPlan] = useState('')
  const [selectedMethod, setSelectedMethod] = useState('')
  const [customAmount, setCustomAmount] = useState('25.00')
  const [purchaseType, setPurchaseType] = useState<'plan' | 'custom_amount'>('plan')
  const [order, setOrder] = useState<CashierOrder | null>(null)
  const [paymentModalOpen, setPaymentModalOpen] = useState(false)
  const [detailOrder, setDetailOrder] = useState<CashierOrder | null>(null)
  const [nowMs, setNowMs] = useState(() => Date.now())
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
  const orderRuntime = useMemo(() => checkoutOrderRuntimeState(order, nowMs), [nowMs, order])
  const orderActions = useMemo(() => checkoutOrderActionState(order, nowMs), [nowMs, order])
  const pollingRuntime = useMemo(() => checkoutOrderRuntimeState(order), [order])
  const recentRows = useMemo(() => checkoutRecentOrderRows(recentOrders), [recentOrders])

  useEffect(() => {
    if (!paymentModalOpen && !detailOrder) return undefined
    const timer = window.setInterval(() => setNowMs(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [detailOrder, paymentModalOpen])

  useEffect(() => {
    if (!order) return
    if (pollingRuntime.step === 'success' && balanceRefreshedOrderID.current !== order.id) {
      balanceRefreshedOrderID.current = order.id
      void app.refreshAccount()
      return
    }
    if (!pollingRuntime.shouldPoll) return
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
  }, [app, order, pollingRuntime])

  function openPaymentModal(nextOrder: CashierOrder) {
    setOrder(nextOrder)
    setNowMs(Date.now())
    setPaymentModalOpen(true)
  }

  function closePaymentModal() {
    setPaymentModalOpen(false)
    void app.refreshAccount()
    void loadRecentOrders()
  }

  function openOrderDetail(nextOrder: CashierOrder) {
    setDetailOrder(nextOrder)
    setNowMs(Date.now())
  }

  function closeOrderDetail() {
    setDetailOrder(null)
    void loadRecentOrders()
  }

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
    setBusy(true)
    try {
      const nextOrder = await userApi.createCashierOrder({
        purchase_type: purchaseType,
        plan_code: purchaseType === 'plan' ? selectedPlan : undefined,
        amount_cny: purchaseType === 'custom_amount' ? normalizedCustomAmount.value : undefined,
        visible_method: selectedMethod,
        client_return_url: `${window.location.origin}${window.location.pathname}#/checkout`,
      }, orderIdempotencyKey)
      setOrder(nextOrder)
      setPaymentModalOpen(true)
      setNowMs(Date.now())
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

  async function refreshOrder(target: CashierOrder | null = order) {
    if (!target) return null
    setBusy(true)
    try {
      const next = await userApi.getCashierOrder(target.id)
      if (order?.id === next.id) setOrder(next)
      if (detailOrder?.id === next.id) setDetailOrder(next)
      void loadRecentOrders()
      if (checkoutOrderRuntimeState(next).step === 'success') {
        balanceRefreshedOrderID.current = next.id
        await app.refreshAccount()
      }
      return next
    } catch (caught) {
      app.notify('error', errorMessage(caught))
      return null
    } finally {
      setBusy(false)
    }
  }

  async function confirmPaid() {
    if (!order) return
    const next = await refreshOrder(order)
    if (!next) return
    const nextRuntime = checkoutOrderRuntimeState(next)
    if (nextRuntime.step === 'success') {
      app.notify('success', '支付成功，充值余额已刷新')
      setPaymentModalOpen(false)
      void loadRecentOrders()
      return
    }
    app.notify('error', '暂未查询到支付成功，请稍后再试')
  }

  async function selectRecentOrder(nextOrder: CashierOrder, openModal = false) {
    setOrder(nextOrder)
    balanceRefreshedOrderID.current = checkoutOrderRuntimeState(nextOrder).step === 'success' ? nextOrder.id : null
    if (openModal) {
      openPaymentModal(nextOrder)
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

  async function cancelOrder(target: CashierOrder | null = order) {
    if (!target) return
    setBusy(true)
    try {
      const next = await userApi.cancelCashierOrder(target.id)
      if (order?.id === next.id || target.id === order?.id) setOrder(next)
      if (detailOrder?.id === next.id) setDetailOrder(next)
      void loadRecentOrders()
      if (paymentModalOpen && target.id === order?.id) setOrder(next)
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
    <div className={checkoutClasses.page}>
      <div className={checkoutClasses.header}>
        <div className="flex flex-col gap-5 md:flex-row md:items-end md:justify-between">
          <div>
            <h1 className={checkoutClasses.title}>积分充值</h1>
            <p className={checkoutClasses.detail}>固定积分包和自定义金额统一通过收银台创建订单，支付成功后进入充值余额桶且不过期。</p>
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
                className={cn(checkoutClasses.recentRow, order?.id === row.id && checkoutClasses.recentRowActive)}
              >
                <span className={checkoutClasses.recentCell}>
                  <strong className={checkoutClasses.recentStrong}>{row.title}</strong>
                  <em className={checkoutClasses.recentMeta}>{row.orderNo}</em>
                </span>
                <span className={checkoutClasses.recentCell}>
                  <strong className={checkoutClasses.recentStrong}>{row.amount}</strong>
                  <em className={checkoutClasses.recentMeta}>{row.points} 积分</em>
                </span>
                <span className={checkoutClasses.recentCell}>
                  <strong className={checkoutClasses.recentStrong}>{row.status}</strong>
                  <em className={checkoutClasses.recentMeta}>{row.method}</em>
                </span>
                <div className={checkoutClasses.recentActions}>
                  <time className={checkoutClasses.recentMeta} dateTime={row.createdAt}>{row.createdAtLabel}</time>
                  {checkoutOrderActionState(row.order).canContinuePay ? (
                    <>
                      <Button tone="ghost" onClick={() => void selectRecentOrder(row.order, true)}>继续支付</Button>
                      <Button tone="ghost" busy={busy} onClick={() => void cancelOrder(row.order)}>取消支付</Button>
                      <Button tone="ghost" onClick={() => openOrderDetail(row.order)}>查看订单</Button>
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

      {paymentModalOpen && order ? (
        <PaymentOrderModal
          order={order}
          nowMs={nowMs}
          busy={busy}
          runtime={orderRuntime}
          actions={orderActions}
          onClose={closePaymentModal}
          onRefresh={() => void refreshOrder()}
          onConfirmPaid={() => void confirmPaid()}
          onMockPay={() => void mockPay()}
        />
      ) : null}

      {detailOrder ? (
        <OrderDetailModal
          order={detailOrder}
          nowMs={nowMs}
          busy={busy}
          onClose={closeOrderDetail}
          onContinuePay={() => {
            setDetailOrder(null)
            openPaymentModal(detailOrder)
          }}
          onCancel={() => void cancelOrder(detailOrder)}
          onRefresh={() => void refreshOrder(detailOrder)}
        />
      ) : null}
    </div>
  )
}

function PaymentOrderModal({
  order,
  nowMs,
  busy,
  runtime,
  actions,
  onClose,
  onRefresh,
  onConfirmPaid,
  onMockPay,
}: {
  order: CashierOrder
  nowMs: number
  busy: boolean
  runtime: ReturnType<typeof checkoutOrderRuntimeState>
  actions: ReturnType<typeof checkoutOrderActionState>
  onClose: () => void
  onRefresh: () => void
  onConfirmPaid: () => void
  onMockPay: () => void
}) {
  const countdown = checkoutRemainingLabel(order.expires_at, nowMs)
  const display = checkoutPaymentDisplayModel(order)
  const title = display.kind === 'qr_code' ? '扫码支付' : '完成支付'
  return (
    <Modal title={title} onClose={onClose}>
      <div className={checkoutClasses.paymentPrompt}>
        {display.kind === 'qr_code' ? (
          <QRCodePaymentPrompt order={order} countdown={countdown} displayHref={display.href ?? ''} />
        ) : (
          <RedirectPaymentPrompt
            order={order}
            display={display}
            runtime={runtime}
            actions={actions}
            busy={busy}
            onMockPay={onMockPay}
          />
        )}
        {display.kind === 'qr_code' ? null : (
          <div className={checkoutClasses.actions}>
            {display.kind === 'redirect' || display.kind === 'form' || display.kind === 'mock' ? <Button tone="primary" busy={busy} onClick={onConfirmPaid}>我已支付</Button> : null}
            {display.kind === 'unsupported' || display.kind === 'none' ? <Button tone="ghost" busy={busy} onClick={onRefresh}>刷新订单</Button> : null}
            <Button tone="ghost" onClick={onClose}>取消</Button>
          </div>
        )}
      </div>
    </Modal>
  )
}

function QRCodePaymentPrompt({ order, countdown, displayHref }: { order: CashierOrder; countdown: string; displayHref: string }) {
  return (
    <div className={checkoutClasses.paymentPromptBody}>
      <span className={checkoutClasses.paymentCountdown}>剩余支付时间 {countdown}</span>
      <PaymentQRCode value={displayHref} />
      <div>
        <div className={checkoutClasses.paymentPromptKicker}>支付金额</div>
        <div className={checkoutClasses.paymentPromptAmount}>{checkoutMoney(order.amount_cny)}</div>
      </div>
    </div>
  )
}

function RedirectPaymentPrompt({
  order,
  display,
  runtime,
  actions,
  busy,
  onMockPay,
}: {
  order: CashierOrder
  display: ReturnType<typeof checkoutPaymentDisplayModel>
  runtime: ReturnType<typeof checkoutOrderRuntimeState>
  actions: ReturnType<typeof checkoutOrderActionState>
  busy: boolean
  onMockPay: () => void
}) {
  const openForm = () => {
    if (!display.formHtml) return
    const popup = window.open('', '_blank')
    if (!popup) return
    popup.opener = null
    popup.document.write(display.formHtml)
    popup.document.close()
  }
  const terminal = runtime.step !== 'paying' && runtime.step !== 'select'
  return (
    <div className={checkoutClasses.paymentPromptBody}>
      <div className={checkoutClasses.paymentRedirectPanel}>
        <div className={checkoutClasses.paymentRedirectAmount}>
          <span>
            <strong className={checkoutClasses.paymentRedirectTitle}>{terminal ? orderActionsLabel(actions, runtime.step) : '请在新页面完成支付'}</strong>
            <span className="mt-1 block text-sm text-[var(--muted)]">{checkoutPaymentMethodLabel(order)}</span>
          </span>
          <strong className="font-vault-display text-3xl text-[var(--accent)]">{checkoutMoney(order.amount_cny)}</strong>
        </div>
        <p className={checkoutClasses.paymentPromptDetail}>{terminal ? runtime.detail : display.detail}</p>
        <span className={checkoutClasses.paymentRedirectMeta}>{order.order_no}</span>
        {display.kind === 'redirect' && display.href ? (
          <Button tone="primary" onClick={() => window.open(display.href, '_blank', 'noopener,noreferrer')}>打开支付页</Button>
        ) : null}
        {display.kind === 'form' ? <Button tone="primary" onClick={openForm}>打开支付页</Button> : null}
        {display.kind === 'mock' ? <Button tone="primary" busy={busy} onClick={onMockPay}>模拟支付成功</Button> : null}
        {display.kind === 'unsupported' || display.kind === 'none' ? (
          <p className={checkoutClasses.paymentPromptDetail}>{display.label}</p>
        ) : null}
      </div>
    </div>
  )
}

function OrderDetailModal({ order, nowMs, busy, onClose, onContinuePay, onCancel, onRefresh }: {
  order: CashierOrder
  nowMs: number
  busy: boolean
  onClose: () => void
  onContinuePay: () => void
  onCancel: () => void
  onRefresh: () => void
}) {
  const runtime = checkoutOrderRuntimeState(order, nowMs)
  const actions = checkoutOrderActionState(order, nowMs)
  const rows = [
    ['订单号', order.order_no],
    ['充值内容', order.plan_name || (order.purchase_type === 'custom_amount' ? '自定义金额充值' : order.plan_code || '积分充值')],
    ['支付方式', checkoutPaymentMethodLabel(order)],
    ['到账积分', `${checkoutPoints(order.points)} 积分`],
    ['创建时间', checkoutDateTime(order.created_at)],
    ['过期时间', checkoutDateTime(order.expires_at)],
    ['支付时间', checkoutDateTime(order.paid_at)],
    ['完成时间', checkoutDateTime(order.completed_at)],
    ['渠道流水号', order.trade_no || '-'],
    ['失败原因', order.failure_reason || '-'],
  ]
  return (
    <Modal title="订单详情" onClose={onClose}>
      <div className={checkoutClasses.orderDetailBody}>
        <div className={checkoutClasses.orderDetailHero}>
          <div>
            <div className={checkoutClasses.orderDetailStatus}>{runtime.label}</div>
            <h3 className={checkoutClasses.orderDetailTitle}>{checkoutOrderStatusLabel(order.status)}</h3>
          </div>
          <div className="text-left sm:text-right">
            <div className={checkoutClasses.orderDetailAmount}>{checkoutMoney(order.amount_cny)}</div>
            <span className="text-sm text-[var(--muted)]">{checkoutPoints(order.points)} 积分</span>
          </div>
        </div>
        <dl className={checkoutClasses.orderDetailList}>
          {rows.map(([label, value]) => (
            <div className={checkoutClasses.orderDetailRow} key={label}>
              <dt className={checkoutClasses.orderDetailLabel}>{label}</dt>
              <dd className={checkoutClasses.orderDetailValue}>{value}</dd>
            </div>
          ))}
        </dl>
        <p className={checkoutClasses.orderDetailNote}>{runtime.detail}</p>
        <div className={checkoutClasses.actions}>
          <Button tone="ghost" busy={busy} onClick={onRefresh}>刷新订单</Button>
          {actions.canContinuePay ? <Button tone="primary" onClick={onContinuePay}>继续支付</Button> : null}
          {actions.canCancel ? <Button tone="ghost" busy={busy} onClick={onCancel}>取消订单</Button> : null}
          {!actions.canContinuePay ? <Button tone="primary" onClick={onClose}>关闭</Button> : null}
        </div>
      </div>
    </Modal>
  )
}

function PaymentQRCode({ value }: { value: string }) {
  const [dataUrl, setDataUrl] = useState('')

  useEffect(() => {
    let cancelled = false
    void toDataURL(value, {
      margin: 1,
      width: 208,
      color: {
        dark: '#111111',
        light: '#ffffff',
      },
    }).then((url) => {
      if (!cancelled) setDataUrl(url)
    }).catch(() => {
      if (!cancelled) setDataUrl('')
    })
    return () => {
      cancelled = true
    }
  }, [value])

  if (!dataUrl) return <div className={checkoutClasses.paymentQrImage} />
  return <img className={checkoutClasses.paymentQrImage} src={dataUrl} alt="支付二维码" />
}

function newCheckoutOrderIdempotencyKey() {
  const random = typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `checkout-order-${random}`
}

function checkoutRemainingLabel(expiresAt: string, nowMs: number) {
  const expiresMs = Date.parse(expiresAt)
  if (!Number.isFinite(expiresMs)) return '--:--'
  const remaining = Math.max(0, Math.floor((expiresMs - nowMs) / 1000))
  const minutes = Math.floor(remaining / 60)
  const seconds = remaining % 60
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`
}

function orderActionsLabel(actions: ReturnType<typeof checkoutOrderActionState>, step: ReturnType<typeof checkoutOrderRuntimeState>['step']) {
  if (actions.terminalLabel) return actions.terminalLabel
  if (step === 'success') return '支付成功'
  if (step === 'expired') return '订单已过期'
  return '订单未完成'
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

function MethodButton({ method, active, onSelect }: { method: PaymentVisibleMethod; active: boolean; onSelect: () => void }) {
  const display = checkoutPaymentMethodOptionModel(method)
  return (
    <button type="button" className={cn(checkoutClasses.optionButton, checkoutClasses.methodButton, active && checkoutClasses.optionActive)} onClick={onSelect}>
      <span className={checkoutClasses.methodIcon}>{method.method.includes('wx') ? '微' : method.method.includes('ali') ? '支' : '¥'}</span>
      <span className="grid min-w-0 gap-1">
        <strong>{display.label}</strong>
        <span className={checkoutClasses.optionLabel}>{display.detail}</span>
      </span>
    </button>
  )
}
