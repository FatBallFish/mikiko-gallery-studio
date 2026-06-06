import { useEffect, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import type { CashierCustomAmountConfig, CashierOverview, CashierPlan, PageResult, PaymentOrder, PaymentProviderInstance, PaymentProviderInstanceWriteRequest, PaymentProviderType, PaymentSchedulerStrategy, PaymentVisibleMethod, PaymentWebhookEvent } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, Field, LoadingBlock, Modal, PageHeader } from '../components'
import { applyJeePayWayCodeTemplate, jeepayTemplatesForProvider } from './cashierJeePayWayCodeTemplates'
import { cashierAdminDateTime, cashierManualCompletionProviderOptions, cashierOrderPaymentLabel, cashierOrderPurchaseTypeLabel, cashierProviderConfigStatusLabel, cashierProviderSupportedMethodsLabel, cashierWebhookEventTypeLabel, cashierWebhookProviderLabel } from './cashierPaymentDisplay'
import { cashierPlanEmptyState, cashierPlanPurchaseBadge, cashierPlanSavePayload, cashierPlanSectionCopy } from './cashierPlanPurchase'
import { cashierProviderConfigGuide, cashierProviderInstanceFieldHints, cashierProviderLabel, cashierProviderSupportedMethodOptions, cashierProviderTypes, cashierProviderTypesForMethod, cashierToggleSupportedMethod } from './cashierProviderOptions'
import type { CashierStatusBadge } from './cashierStatusRows'
import { cashierVisibleMethodRow } from './cashierVisibleMethodRows'
import {
  cashierBooleanVisibilityLabel,
  cashierEnabledBadge,
  cashierOrderStatusBadge,
  cashierPlanStatusBadge,
  cashierPlanStatusOptions,
  cashierPlanTypeLabel,
  cashierSyncStatusLabel,
  cashierVisibleFlagLabel,
  cashierWebhookStatusBadge,
} from './cashierStatusRows'
import { cashierWebhookEventAction } from './cashierWebhookEventActions'

type CashierData = {
  overview: CashierOverview
  plans: PageResult<CashierPlan>
  customAmount: CashierCustomAmountConfig
  methods: PaymentVisibleMethod[]
  instances: PageResult<PaymentProviderInstance>
  orders: PageResult<PaymentOrder>
  events: PageResult<PaymentWebhookEvent>
}

type PlanDraft = {
  row?: CashierPlan
  plan_code: string
  plan_name: string
  plan_type: string
  purchase_enabled: boolean
  status: string
  price_cny: string
  points: string
  bonus_points: string
  duration_days: string
  currency: string
  sort_order: string
  description: string
}

type InstanceDraft = {
  row?: PaymentProviderInstance
  provider_type: PaymentProviderType
  name: string
  enabled: boolean
  supported_methods: string
  sort_order: string
  scheduler_weight: string
  min_amount_cny: string
  max_amount_cny: string
  daily_amount_limit_cny: string
  config_text: string
}

type CompleteOrderDraft = {
  order: PaymentOrder
  provider: string
  trade_no: string
  reason: string
}

type RefundOrderDraft = {
  order: PaymentOrder
  refund_trade_no: string
  refund_amount_cny: string
  reason: string
}

type ChargebackOrderDraft = {
  order: PaymentOrder
  charge_points: string
  reason: string
  idempotency_key: string
}

const schedulerOptions: Array<{ value: PaymentSchedulerStrategy; label: string }> = [
  { value: 'round_robin', label: '轮询调度' },
  { value: 'random', label: '随机调度' },
]

function methodsForProviderType(providerType: PaymentProviderType) {
  if (providerType === 'wxpay_direct' || providerType === 'easypay_wxpay' || providerType === 'jeepay_wxpay') return 'wxpay'
  if (providerType === 'mock') return 'mock'
  return 'alipay'
}

export function CashierPage({ onFeedback }: { onFeedback?: (title: string, detail?: string) => void }) {
  const [data, setData] = useState<CashierData | null>(null)
  const [customDraft, setCustomDraft] = useState<CashierCustomAmountConfig | null>(null)
  const [methodsDraft, setMethodsDraft] = useState<PaymentVisibleMethod[]>([])
  const [planDialog, setPlanDialog] = useState<PlanDraft | null>(null)
  const [instanceDialog, setInstanceDialog] = useState<InstanceDraft | null>(null)
  const [orderDetail, setOrderDetail] = useState<PaymentOrder | null>(null)
  const [completeDialog, setCompleteDialog] = useState<CompleteOrderDraft | null>(null)
  const [refundDialog, setRefundDialog] = useState<RefundOrderDraft | null>(null)
  const [chargebackDialog, setChargebackDialog] = useState<ChargebackOrderDraft | null>(null)
  const [loadingOrderID, setLoadingOrderID] = useState<number | string | null>(null)
  const [retryingEventID, setRetryingEventID] = useState<number | string | null>(null)
  const [loading, setLoading] = useState(true)
  const [savingCustomAmount, setSavingCustomAmount] = useState(false)
  const [savingMethods, setSavingMethods] = useState(false)
  const [savingPlan, setSavingPlan] = useState(false)
  const [savingInstance, setSavingInstance] = useState(false)
  const [completingOrder, setCompletingOrder] = useState(false)
  const [refundingOrder, setRefundingOrder] = useState(false)
  const [chargingBackOrder, setChargingBackOrder] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function load() {
    setLoading(true)
    setError(null)
    try {
      const [overview, plans, customAmount, methods, instances, orders, events] = await Promise.all([
        adminApi.getCashierOverview(),
        adminApi.listCashierPlans(),
        adminApi.getCashierCustomAmountConfig(),
        adminApi.listPaymentVisibleMethods(),
        adminApi.listPaymentProviderInstances(),
        adminApi.listPaymentOrders({ page: 1, page_size: 10 }),
        adminApi.listPaymentWebhookEvents({ page: 1, page_size: 10 }),
      ])
      setData({ overview, plans, customAmount, methods, instances, orders, events })
      setCustomDraft(customAmount)
      setMethodsDraft(methods)
      onFeedback?.('收银台数据已刷新')
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '收银台数据载入失败')
    } finally {
      setLoading(false)
    }
  }

  async function saveCustomAmount(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!customDraft) return
    setSavingCustomAmount(true)
    setError(null)
    try {
      const updated = await adminApi.updateCashierCustomAmountConfig(customDraft)
      setCustomDraft(updated)
      setData((current) => current ? { ...current, customAmount: updated } : current)
      onFeedback?.('自定义金额配置已保存')
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '自定义金额配置保存失败')
    } finally {
      setSavingCustomAmount(false)
    }
  }

  async function saveVisibleMethods(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSavingMethods(true)
    setError(null)
    try {
      const response = await adminApi.updatePaymentVisibleMethods(methodsDraft)
      const updated = response.items ?? []
      setMethodsDraft(updated)
      setData((current) => current ? { ...current, methods: updated } : current)
      onFeedback?.('支付方式配置已保存')
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '支付方式配置保存失败')
    } finally {
      setSavingMethods(false)
    }
  }

  function updateMethodDraft(index: number, patch: Partial<PaymentVisibleMethod>) {
    setMethodsDraft((current) => current.map((method, itemIndex) => itemIndex === index ? { ...method, ...patch } : method))
  }

  async function reloadPlans() {
    const plans = await adminApi.listCashierPlans()
    setData((current) => current ? { ...current, plans } : current)
  }

  async function reloadInstances() {
    const instances = await adminApi.listPaymentProviderInstances()
    setData((current) => current ? { ...current, instances } : current)
  }

  async function savePlan() {
    if (!planDialog) return
    setSavingPlan(true)
    setError(null)
    try {
      const payload = cashierPlanSavePayload(planDialog)
      if (planDialog.row) await adminApi.updateCashierPlan(planDialog.row.id, payload)
      else await adminApi.createCashierPlan(payload)
      setPlanDialog(null)
      await reloadPlans()
      onFeedback?.('充值套餐已保存', planDialog.plan_name)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '充值套餐保存失败')
    } finally {
      setSavingPlan(false)
    }
  }

  async function saveInstance() {
    if (!instanceDialog) return
    setSavingInstance(true)
    setError(null)
    try {
      const config = parseConfigText(instanceDialog.config_text)
      const payload: PaymentProviderInstanceWriteRequest = {
        provider_type: instanceDialog.provider_type,
        name: instanceDialog.name,
        enabled: instanceDialog.enabled,
        supported_methods: instanceDialog.supported_methods.split(',').map((item) => item.trim()).filter(Boolean),
        sort_order: Number(instanceDialog.sort_order) || 0,
        scheduler_weight: Number(instanceDialog.scheduler_weight) || 100,
        limits: {
          min_amount_cny: instanceDialog.min_amount_cny,
          max_amount_cny: instanceDialog.max_amount_cny,
          daily_amount_limit_cny: instanceDialog.daily_amount_limit_cny || undefined,
        },
        config,
      }
      if (instanceDialog.row) await adminApi.updatePaymentProviderInstance(instanceDialog.row.id, payload)
      else await adminApi.createPaymentProviderInstance(payload)
      setInstanceDialog(null)
      await reloadInstances()
      onFeedback?.('支付渠道实例已保存', instanceDialog.name)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '支付渠道实例保存失败')
    } finally {
      setSavingInstance(false)
    }
  }

  async function openOrderDetail(order: PaymentOrder) {
    setLoadingOrderID(order.id)
    setError(null)
    try {
      const detail = await adminApi.getPaymentOrder(order.id)
      setOrderDetail(detail)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '订单详情读取失败')
    } finally {
      setLoadingOrderID(null)
    }
  }

  async function retryWebhookEvent(event: PaymentWebhookEvent) {
    setRetryingEventID(event.id)
    setError(null)
    try {
      const updated = await adminApi.retryPaymentWebhookEvent(event.id)
      setData((current) => {
        if (!current) return current
        return {
          ...current,
          events: {
            ...current.events,
            items: current.events.items.map((item) => item.id === updated.id ? updated : item),
          },
        }
      })
      onFeedback?.('回调事件已重试', updated.order_no ?? String(updated.id))
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '回调事件重试失败')
    } finally {
      setRetryingEventID(null)
    }
  }

  async function completeOrderManually() {
    if (!completeDialog) return
    setCompletingOrder(true)
    setError(null)
    try {
      const updated = await adminApi.completePaymentOrder(completeDialog.order.id, {
        provider: completeDialog.provider,
        trade_no: completeDialog.trade_no,
        reason: completeDialog.reason,
      })
      setData((current) => {
        if (!current) return current
        return {
          ...current,
          orders: {
            ...current.orders,
            items: current.orders.items.map((item) => item.id === updated.id ? updated : item),
          },
        }
      })
      setOrderDetail((current) => current?.id === updated.id ? updated : current)
      setCompleteDialog(null)
      onFeedback?.('订单已人工补单完成', updated.order_no)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '人工补单失败')
    } finally {
      setCompletingOrder(false)
    }
  }

  async function refundPaymentOrder() {
    if (!refundDialog) return
    setRefundingOrder(true)
    setError(null)
    try {
      const updated = await adminApi.refundPaymentOrder(refundDialog.order.id, {
        refund_trade_no: refundDialog.refund_trade_no,
        refund_amount_cny: refundDialog.refund_amount_cny.trim() || undefined,
        reason: refundDialog.reason,
      })
      setData((current) => {
        if (!current) return current
        return {
          ...current,
          orders: {
            ...current.orders,
            items: current.orders.items.map((item) => item.id === updated.id ? updated : item),
          },
        }
      })
      setOrderDetail((current) => current?.id === updated.id ? updated : current)
      setRefundDialog(null)
      onFeedback?.('订单已退款', updated.order_no)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '订单退款失败')
    } finally {
      setRefundingOrder(false)
    }
  }

  async function chargebackPaymentOrder() {
    if (!chargebackDialog) return
    setChargingBackOrder(true)
    setError(null)
    try {
      const result = await adminApi.chargebackPaymentOrder(chargebackDialog.order.id, {
        charge_points: chargebackDialog.charge_points,
        reason: chargebackDialog.reason,
      }, chargebackDialog.idempotency_key)
      setData((current) => {
        if (!current) return current
        return {
          ...current,
          orders: {
            ...current.orders,
            items: current.orders.items.map((item) => item.id === result.order.id ? result.order : item),
          },
        }
      })
      setOrderDetail((current) => current?.id === result.order.id ? result.order : current)
      setChargebackDialog(null)
      onFeedback?.('订单已人工追扣', `当前可用余额 ${Number(result.balance.available_points ?? '0').toFixed(2)}`)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '订单追扣失败')
    } finally {
      setChargingBackOrder(false)
    }
  }

  async function syncPaymentOrder(order: PaymentOrder) {
    setLoadingOrderID(order.id)
    setError(null)
    try {
      const result = await adminApi.syncPaymentOrder(order.id)
      setData((current) => {
        if (!current) return current
        return {
          ...current,
          orders: {
            ...current.orders,
            items: current.orders.items.map((item) => item.id === result.order.id ? result.order : item),
          },
        }
      })
      setOrderDetail((current) => current?.id === result.order.id ? result.order : current)
      onFeedback?.(result.sync.completed ? '查单已确认到账' : '查单完成', result.sync.message ?? cashierSyncStatusLabel(result.sync.query_status))
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '订单查单失败')
    } finally {
      setLoadingOrderID(null)
    }
  }

  useEffect(() => { void load() }, [])

  if (loading) return <LoadingBlock label="读取收银台配置" />
  if (error) return <ErrorBlock message={error} onRetry={load} />
  if (!data) return <EmptyBlock title="暂无收银台数据" detail="后台尚未返回收银台配置。" />

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="Cashier"
        title="收银台"
        detail="统一管理充值积分包、自定义金额、可见支付方式、渠道实例、订单和回调事件。"
        actions={<button type="button" className="btn" onClick={() => void load()}>刷新</button>}
      />
      <section className="ops-status-strip compact-strip">
        <div className="status-cell"><label>今日成交</label><strong>¥{Number(data.overview.today_amount_cny).toFixed(2)}</strong></div>
        <div className="status-cell"><label>成功率</label><strong>{data.overview.success_rate}</strong></div>
        <div className="status-cell"><label>待支付订单</label><strong>{data.overview.pending_count}</strong></div>
        <div className="status-cell"><label>Mock 渠道</label><strong>{cashierBooleanVisibilityLabel(data.overview.mock_enabled)}</strong></div>
      </section>

      <section className="pg-admin-card ops-surface full-main cashier-surface">
        <section className="main-lane table-lane no-divider">
          <CashierSection title="自定义金额">
            <form className="cashier-config-form" onSubmit={(event) => void saveCustomAmount(event)}>
              <label className="check-option cashier-toggle">
                <input
                  type="checkbox"
                  checked={Boolean(customDraft?.enabled)}
                  onChange={(event) => setCustomDraft((current) => current ? { ...current, enabled: event.target.checked } : current)}
                />
                <span>允许用户输入金额</span>
              </label>
              <div className="form-grid cashier-amount-grid">
                <Field label="最小金额">
                  <input
                    value={customDraft?.min_amount_cny ?? ''}
                    onChange={(event) => setCustomDraft((current) => current ? { ...current, min_amount_cny: event.target.value } : current)}
                    inputMode="decimal"
                    placeholder="1.00000"
                  />
                </Field>
                <Field label="最大金额">
                  <input
                    value={customDraft?.max_amount_cny ?? ''}
                    onChange={(event) => setCustomDraft((current) => current ? { ...current, max_amount_cny: event.target.value } : current)}
                    inputMode="decimal"
                    placeholder="999.00000"
                  />
                </Field>
                <Field label="CNY / 积分">
                  <input
                    value={customDraft?.cny_per_point ?? ''}
                    onChange={(event) => setCustomDraft((current) => current ? { ...current, cny_per_point: event.target.value } : current)}
                    inputMode="decimal"
                    placeholder="0.31250"
                  />
                </Field>
                <div className="cashier-config-actions">
                  <StatusBadge badge={cashierEnabledBadge(Boolean(data.customAmount.enabled))} />
                  <button type="submit" className="btn" disabled={savingCustomAmount}>{savingCustomAmount ? '保存中' : '保存'}</button>
                </div>
              </div>
            </form>
          </CashierSection>

          <CashierSection title="固定积分包">
            <div className="cashier-method-toolbar">
              <p>{cashierPlanSectionCopy.toolbarDetail}</p>
              <button type="button" className="btn" onClick={() => setPlanDialog(newPlanDraft())}>新增套餐</button>
            </div>
            <div className="admin-data-grid cashier-plan-grid">
              <div className="table-head"><span>套餐</span><span>类型</span><span>价格</span><span>积分</span><span>排序</span><span>状态</span><span>购买</span><span>操作</span></div>
              {data.plans.items.map((plan) => {
                const purchaseBadge = cashierPlanPurchaseBadge(plan)
                return (
                  <div key={plan.id} className="table-row">
                    <div><strong>{plan.plan_name}</strong><p>{plan.plan_code}</p></div>
                    <span>{cashierPlanTypeLabel(plan.plan_type)}</span>
                    <code>¥{Number(plan.price_cny).toFixed(2)}</code>
                    <code>{Number(plan.points).toFixed(2)}</code>
                    <code>{plan.sort_order ?? 0}</code>
                    <StatusBadge badge={cashierPlanStatusBadge(plan.status)} />
                    <Badge tone={purchaseBadge.tone}>{purchaseBadge.label}</Badge>
                    <button type="button" className="ghost small" onClick={() => setPlanDialog(editPlanDraft(plan))}>编辑</button>
                  </div>
                )
              })}
            </div>
            {!data.plans.items.length ? <EmptyBlock title={cashierPlanEmptyState.title} detail={cashierPlanEmptyState.detail} /> : null}
          </CashierSection>

          <CashierSection title="可见支付方式">
            <form className="cashier-config-form" onSubmit={(event) => void saveVisibleMethods(event)}>
              <div className="cashier-method-toolbar">
                <p>控制用户收银台可选择的支付入口；生产环境 Mock 仍由后端隐藏。</p>
                <div className="row-actions buttons">
                  <button type="button" className="ghost small" onClick={() => setMethodsDraft(data.methods)} disabled={savingMethods}>重置</button>
                  <button type="submit" className="btn" disabled={savingMethods}>{savingMethods ? '保存中' : '保存支付方式'}</button>
                </div>
              </div>
              <div className="admin-data-grid cashier-method-grid editable-method-grid">
                <div className="table-head"><span>方式</span><span>渠道类型</span><span>调度</span><span>排序</span><span>状态</span></div>
                {methodsDraft.map((method, index) => {
                  const row = cashierVisibleMethodRow(method)
                  return (
                    <div key={method.method} className="table-row editable-row">
                      <div className="cashier-method-name">
                        <input
                          value={method.label}
                          onChange={(event) => updateMethodDraft(index, { label: event.target.value })}
                          aria-label={`${row.title} 展示名称`}
                        />
                        <p>{row.title} · {row.detail}</p>
                      </div>
                      <select
                        value={method.source_provider_type ?? ''}
                        onChange={(event) => updateMethodDraft(index, { source_provider_type: event.target.value as PaymentProviderType })}
                        aria-label={`${row.title} 渠道类型`}
                      >
                        {cashierProviderTypesForMethod(method.method).map((provider) => (
                          <option key={provider} value={provider}>{cashierProviderLabel(provider)}</option>
                        ))}
                      </select>
                      <select
                        value={method.scheduler_strategy ?? 'round_robin'}
                        onChange={(event) => updateMethodDraft(index, { scheduler_strategy: event.target.value as PaymentSchedulerStrategy })}
                        aria-label={`${row.title} 调度策略`}
                      >
                        {schedulerOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                      </select>
                      <input
                        value={method.display_order}
                        type="number"
                        min="0"
                        step="1"
                        onChange={(event) => updateMethodDraft(index, { display_order: Number(event.target.value) || 0 })}
                        aria-label={`${row.title} 展示排序`}
                      />
                      <label className="check-option cashier-method-enabled">
                        <input
                          type="checkbox"
                          checked={method.enabled}
                          onChange={(event) => updateMethodDraft(index, { enabled: event.target.checked })}
                        />
                        <span>{cashierVisibleFlagLabel(method.enabled)}</span>
                      </label>
                    </div>
                  )
                })}
              </div>
            </form>
          </CashierSection>

          <CashierSection title="支付渠道实例">
            <div className="cashier-method-toolbar">
              <p>配置真实支付账号或测试 Mock 账号；密钥保存后仅显示配置状态和指纹。</p>
              <button type="button" className="btn" onClick={() => setInstanceDialog(newInstanceDraft())}>新增实例</button>
            </div>
            <div className="admin-data-grid cashier-instance-grid">
              <div className="table-head"><span>实例</span><span>类型</span><span>方式</span><span>权重</span><span>状态</span><span>操作</span></div>
              {data.instances.items.map((instance) => (
                <div key={instance.id} className="table-row">
                  <div><strong>{instance.name}</strong><p>{cashierProviderConfigStatusLabel(instance.config_status)}</p></div>
                  <span>{cashierProviderLabel(instance.provider_type)}</span>
                  <span>{cashierProviderSupportedMethodsLabel(instance.supported_methods)}</span>
                  <code>{instance.scheduler_weight}</code>
                  <StatusBadge badge={cashierEnabledBadge(instance.enabled)} />
                  <button type="button" className="ghost small" onClick={() => setInstanceDialog(editInstanceDraft(instance))}>编辑</button>
                </div>
              ))}
            </div>
          </CashierSection>

          <CashierSection title="最近订单">
            <div className="admin-data-grid cashier-order-grid">
              <div className="table-head"><span>订单</span><span>金额</span><span>积分</span><span>方式</span><span>状态</span><span>操作</span></div>
              {data.orders.items.map((order) => (
                <div key={order.id ?? order.order_no} className="table-row">
                  <div><strong>{order.order_no}</strong><p>{cashierOrderPurchaseTypeLabel(order)}</p></div>
                  <code>¥{Number(order.amount_cny ?? '0').toFixed(2)}</code>
                  <code>{Number(order.points ?? '0').toFixed(2)}</code>
                  <code>{cashierOrderPaymentLabel(order)}</code>
                  <StatusBadge badge={cashierOrderStatusBadge(order.status)} />
                  <div className="row-actions">
                    <button type="button" className="ghost small" disabled={loadingOrderID === order.id} onClick={() => void openOrderDetail(order)}>
                      {loadingOrderID === order.id ? '读取中' : '详情'}
                    </button>
                    {order.status === 'pending' ? (
                      <>
                        <button type="button" className="ghost small" disabled={loadingOrderID === order.id} onClick={() => void syncPaymentOrder(order)}>
                          {loadingOrderID === order.id ? '查单中' : '查单'}
                        </button>
                        <button type="button" className="ghost small" onClick={() => setCompleteDialog(newCompleteOrderDraft(order))}>补单</button>
                      </>
                    ) : null}
                    {order.status === 'completed' || order.status === 'partially_refunded' ? (
                      <button type="button" className="ghost small" onClick={() => setRefundDialog(newRefundOrderDraft(order))}>退款</button>
                    ) : null}
                    {canChargebackOrder(order) ? (
                      <button type="button" className="ghost small" onClick={() => setChargebackDialog(newChargebackOrderDraft(order))}>追扣</button>
                    ) : null}
                  </div>
                </div>
              ))}
            </div>
          </CashierSection>

          <CashierSection title="回调事件">
            <div className="admin-data-grid cashier-event-grid">
              <div className="table-head"><span>事件</span><span>订单</span><span>渠道</span><span>状态</span><span>操作</span></div>
              {data.events.items.map((event) => (
                <div key={event.id} className="table-row">
                  <div><strong>{cashierWebhookEventTypeLabel(event)}</strong><p>{event.failure_reason ?? '-'}</p></div>
                  <code>{event.order_no ?? event.order_id ?? '-'}</code>
                  <code>{cashierWebhookProviderLabel(event)}</code>
                  <StatusBadge badge={cashierWebhookStatusBadge(event.status)} />
                  <WebhookEventAction event={event} retrying={retryingEventID === event.id} onRetry={retryWebhookEvent} />
                </div>
              ))}
            </div>
          </CashierSection>
        </section>
      </section>
      {planDialog ? (
        <Modal
          title={planDialog.row ? '编辑充值套餐' : '新增充值套餐'}
          detail={cashierPlanSectionCopy.dialogDetail}
          onClose={() => setPlanDialog(null)}
          footer={<><button className="ghost" type="button" disabled={savingPlan} onClick={() => setPlanDialog(null)}>取消</button><button className="btn primary" type="button" disabled={savingPlan || !planDialog.plan_code || !planDialog.plan_name || !planDialog.price_cny || !planDialog.points} onClick={() => void savePlan()}>{savingPlan ? '保存中...' : '保存'}</button></>}
        >
          <div className="form-grid">
            <Field label="套餐代码"><input value={planDialog.plan_code} disabled={Boolean(planDialog.row)} onChange={(event) => setPlanDialog({ ...planDialog, plan_code: event.target.value })} placeholder="points-100" /></Field>
            <Field label="套餐名称"><input value={planDialog.plan_name} onChange={(event) => setPlanDialog({ ...planDialog, plan_name: event.target.value })} placeholder="100 积分包" /></Field>
            <Field label="套餐类型"><select value={planDialog.plan_type} onChange={(event) => setPlanDialog({ ...planDialog, plan_type: event.target.value, purchase_enabled: event.target.value === 'subscription' ? false : planDialog.purchase_enabled })}><option value="points_package">积分包</option><option value="subscription">{cashierPlanSectionCopy.subscriptionOptionLabel}</option></select></Field>
            <Field label="状态">
              <select value={planDialog.status} onChange={(event) => setPlanDialog({ ...planDialog, status: event.target.value })}>
                {cashierPlanStatusOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
            <Field label="售价 CNY"><input value={planDialog.price_cny} onChange={(event) => setPlanDialog({ ...planDialog, price_cny: event.target.value })} inputMode="decimal" placeholder="19.90000" /></Field>
            <Field label="基础积分"><input value={planDialog.points} onChange={(event) => setPlanDialog({ ...planDialog, points: event.target.value })} inputMode="decimal" placeholder="100.00000" /></Field>
            <Field label="赠送积分"><input value={planDialog.bonus_points} onChange={(event) => setPlanDialog({ ...planDialog, bonus_points: event.target.value })} inputMode="decimal" placeholder="0.00000" /></Field>
            <Field label="有效天数"><input value={planDialog.duration_days} onChange={(event) => setPlanDialog({ ...planDialog, duration_days: event.target.value })} type="number" min="1" /></Field>
            <Field label="币种"><input value={planDialog.currency} onChange={(event) => setPlanDialog({ ...planDialog, currency: event.target.value })} /></Field>
            <Field label="排序"><input value={planDialog.sort_order} onChange={(event) => setPlanDialog({ ...planDialog, sort_order: event.target.value })} type="number" /></Field>
            <label className="check-option cashier-toggle">
              <input type="checkbox" checked={planDialog.plan_type !== 'subscription' && planDialog.purchase_enabled} disabled={planDialog.plan_type === 'subscription'} onChange={(event) => setPlanDialog({ ...planDialog, purchase_enabled: event.target.checked })} />
              <span>允许用户购买</span>
            </label>
            <Field label="描述"><input value={planDialog.description} onChange={(event) => setPlanDialog({ ...planDialog, description: event.target.value })} placeholder="适合轻量体验" /></Field>
          </div>
        </Modal>
      ) : null}
      {instanceDialog ? (
        <Modal
          title={instanceDialog.row ? '编辑支付渠道实例' : '新增支付渠道实例'}
          detail="配置 JSON 中可填写商户号、密钥、网关地址等渠道参数；保存后不会回显密钥明文。"
          onClose={() => setInstanceDialog(null)}
          footer={<><button className="ghost" type="button" disabled={savingInstance} onClick={() => setInstanceDialog(null)}>取消</button><button className="btn primary" type="button" disabled={savingInstance || !instanceDialog.name || !instanceDialog.provider_type} onClick={() => void saveInstance()}>{savingInstance ? '保存中...' : '保存'}</button></>}
        >
          <div className="form-grid">
            <ProviderConfigGuide providerType={instanceDialog.provider_type} />
            <Field label="实例名称"><input value={instanceDialog.name} onChange={(event) => setInstanceDialog({ ...instanceDialog, name: event.target.value })} placeholder="支付宝沙箱主账号" /></Field>
            <Field label="渠道类型">
              <select
                value={instanceDialog.provider_type}
                onChange={(event) => {
                  const providerType = event.target.value as PaymentProviderType
                  setInstanceDialog({ ...instanceDialog, provider_type: providerType, supported_methods: methodsForProviderType(providerType) })
                }}
              >
                {cashierProviderTypes.map((providerType) => <option key={providerType} value={providerType}>{cashierProviderLabel(providerType)}</option>)}
              </select>
            </Field>
            <Field label="支持方式">
              <div className="cashier-supported-methods">
                {cashierProviderSupportedMethodOptions(instanceDialog.provider_type, instanceDialog.supported_methods).map((option) => (
                  <label key={option.value} className="check-option">
                    <input
                      type="checkbox"
                      checked={option.checked}
                      onChange={(event) => setInstanceDialog({
                        ...instanceDialog,
                        supported_methods: cashierToggleSupportedMethod(instanceDialog.supported_methods, option.value, event.target.checked),
                      })}
                    />
                    <span>{option.label}</span>
                  </label>
                ))}
              </div>
            </Field>
            <Field label="排序" hint={cashierProviderInstanceFieldHints.sortOrder}><input value={instanceDialog.sort_order} onChange={(event) => setInstanceDialog({ ...instanceDialog, sort_order: event.target.value })} type="number" min="0" /></Field>
            <Field label="调度权重" hint={cashierProviderInstanceFieldHints.schedulerWeight}><input value={instanceDialog.scheduler_weight} onChange={(event) => setInstanceDialog({ ...instanceDialog, scheduler_weight: event.target.value })} type="number" min="1" /></Field>
            <Field label="最小金额" hint={cashierProviderInstanceFieldHints.minAmount}><input value={instanceDialog.min_amount_cny} onChange={(event) => setInstanceDialog({ ...instanceDialog, min_amount_cny: event.target.value })} inputMode="decimal" placeholder="1.00000" /></Field>
            <Field label="最大金额" hint={cashierProviderInstanceFieldHints.maxAmount}><input value={instanceDialog.max_amount_cny} onChange={(event) => setInstanceDialog({ ...instanceDialog, max_amount_cny: event.target.value })} inputMode="decimal" placeholder="500.00000" /></Field>
            <Field label="日限额" hint={cashierProviderInstanceFieldHints.dailyLimit}><input value={instanceDialog.daily_amount_limit_cny} onChange={(event) => setInstanceDialog({ ...instanceDialog, daily_amount_limit_cny: event.target.value })} inputMode="decimal" placeholder="5000.00000" /></Field>
            <label className="check-option cashier-toggle">
              <input type="checkbox" checked={instanceDialog.enabled} onChange={(event) => setInstanceDialog({ ...instanceDialog, enabled: event.target.checked })} />
              <span>启用实例</span>
            </label>
            {jeepayTemplatesForProvider(instanceDialog.provider_type).length ? (
              <div className="cashier-jeepay-template span-2">
                <div>
                  <strong>JeePay wayCode 模板</strong>
                  <p>套用模板会保留已有商户号、密钥和网关地址，只补齐支付模式、wayCode 和 channelExtra 示例。</p>
                </div>
                <div className="template-button-row">
                  {jeepayTemplatesForProvider(instanceDialog.provider_type).map((template) => (
                    <button
                      key={template.way_code}
                      type="button"
                      className="ghost small"
                      title={template.description}
                      onClick={() => {
                        try {
                          setInstanceDialog({ ...instanceDialog, config_text: applyJeePayWayCodeTemplate(instanceDialog.config_text, template.way_code) })
                        } catch (caught) {
                          setError(caught instanceof Error ? caught.message : 'JeePay 模板套用失败')
                        }
                      }}
                    >
                      {template.way_code}
                    </button>
                  ))}
                </div>
              </div>
            ) : null}
            <Field label="渠道配置 JSON" hint={cashierProviderInstanceFieldHints.configJSON}>
              <textarea className="cashier-config-textarea" value={instanceDialog.config_text} onChange={(event) => setInstanceDialog({ ...instanceDialog, config_text: event.target.value })} rows={8} spellCheck={false} />
            </Field>
          </div>
        </Modal>
      ) : null}
      {orderDetail ? (
        <Modal
          title="订单详情"
          detail={orderDetail.order_no}
          onClose={() => setOrderDetail(null)}
          footer={<button className="btn" type="button" onClick={() => setOrderDetail(null)}>关闭</button>}
        >
          <div className="cashier-detail-grid">
            <DetailItem label="订单状态" value={<StatusBadge badge={cashierOrderStatusBadge(orderDetail.status)} />} />
            <DetailItem label="用户 ID" value={orderDetail.user_id ?? '-'} />
            <DetailItem label="套餐" value={orderDetail.plan_name || orderDetail.plan_code || '-'} />
            <DetailItem label="支付渠道" value={cashierOrderPaymentLabel(orderDetail)} />
            <DetailItem label="金额" value={`¥${Number(orderDetail.amount_cny ?? '0').toFixed(2)}`} />
            <DetailItem label="到账积分" value={Number(orderDetail.points ?? '0').toFixed(2)} />
            <DetailItem label="赠送积分" value={Number(orderDetail.bonus_points ?? '0').toFixed(2)} />
            <DetailItem label="交易号" value={orderDetail.trade_no || '-'} />
            <DetailItem label="退款交易号" value={orderDetail.refund_trade_no || '-'} />
            <DetailItem label="累计退款金额" value={orderDetail.refunded_amount_cny ? `¥${Number(orderDetail.refunded_amount_cny).toFixed(2)}` : '-'} />
            <DetailItem label="累计退款积分" value={orderDetail.refunded_points ? Number(orderDetail.refunded_points).toFixed(2) : '-'} />
            <DetailItem label="创建时间" value={cashierAdminDateTime(orderDetail.created_at)} />
            <DetailItem label="支付时间" value={cashierAdminDateTime(orderDetail.paid_at)} />
            <DetailItem label="退款时间" value={cashierAdminDateTime(orderDetail.refunded_at)} />
            <DetailItem label="关闭时间" value={cashierAdminDateTime(orderDetail.closed_at)} />
            <DetailItem label="失败原因" value={orderDetail.failure_reason || '-'} />
          </div>
        </Modal>
      ) : null}
      {completeDialog ? (
        <Modal
          title="人工补单完成"
          detail={completeDialog.order.order_no}
          onClose={() => setCompleteDialog(null)}
          footer={<><button className="ghost" type="button" disabled={completingOrder} onClick={() => setCompleteDialog(null)}>取消</button><button className="btn primary" type="button" disabled={completingOrder || !completeDialog.trade_no.trim()} onClick={() => void completeOrderManually()}>{completingOrder ? '处理中...' : '确认到账'}</button></>}
        >
          <div className="form-grid">
            <Field label="支付渠道">
              <select value={completeDialog.provider} onChange={(event) => setCompleteDialog({ ...completeDialog, provider: event.target.value })}>
                {cashierManualCompletionProviderOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
            <Field label="渠道交易号">
              <input value={completeDialog.trade_no} onChange={(event) => setCompleteDialog({ ...completeDialog, trade_no: event.target.value })} placeholder="MANUAL-TRADE-001" />
            </Field>
            <Field label="补单原因">
              <input value={completeDialog.reason} onChange={(event) => setCompleteDialog({ ...completeDialog, reason: event.target.value })} placeholder="已在渠道后台确认支付成功" />
            </Field>
          </div>
        </Modal>
      ) : null}
      {refundDialog ? (
        <Modal
          title="订单退款"
          detail={refundDialog.order.order_no}
          onClose={() => setRefundDialog(null)}
          footer={<><button className="ghost" type="button" disabled={refundingOrder} onClick={() => setRefundDialog(null)}>取消</button><button className="btn primary" type="button" disabled={refundingOrder || !refundDialog.refund_trade_no.trim()} onClick={() => void refundPaymentOrder()}>{refundingOrder ? '处理中...' : '确认退款'}</button></>}
        >
          <div className="form-grid">
            <Field label="退款交易号">
              <input value={refundDialog.refund_trade_no} onChange={(event) => setRefundDialog({ ...refundDialog, refund_trade_no: event.target.value })} placeholder="REFUND-TRADE-001" />
            </Field>
            <Field label="退款金额">
              <input value={refundDialog.refund_amount_cny} onChange={(event) => setRefundDialog({ ...refundDialog, refund_amount_cny: event.target.value })} inputMode="decimal" placeholder={remainingRefundAmountCNY(refundDialog.order)} />
            </Field>
            <Field label="退款原因">
              <input value={refundDialog.reason} onChange={(event) => setRefundDialog({ ...refundDialog, reason: event.target.value })} placeholder="用户申请退款，余额未消费" />
            </Field>
          </div>
        </Modal>
      ) : null}
      {chargebackDialog ? (
        <Modal
          title="订单追扣"
          detail={chargebackDialog.order.order_no}
          onClose={() => setChargebackDialog(null)}
          footer={<><button className="ghost" type="button" disabled={chargingBackOrder} onClick={() => setChargebackDialog(null)}>取消</button><button className="btn primary" type="button" disabled={chargingBackOrder || !chargebackDialog.charge_points.trim() || !chargebackDialog.reason.trim() || !chargebackDialog.idempotency_key.trim()} onClick={() => void chargebackPaymentOrder()}>{chargingBackOrder ? '处理中...' : '确认追扣'}</button></>}
        >
          <div className="form-grid">
            <Field label="追扣积分">
              <input value={chargebackDialog.charge_points} onChange={(event) => setChargebackDialog({ ...chargebackDialog, charge_points: event.target.value })} inputMode="decimal" placeholder="5.00000" />
            </Field>
            <Field label="追扣原因">
              <input value={chargebackDialog.reason} onChange={(event) => setChargebackDialog({ ...chargebackDialog, reason: event.target.value })} placeholder="渠道拒付已确认" />
            </Field>
            <Field label="Idempotency-Key">
              <div className="inline-control">
                <input value={chargebackDialog.idempotency_key} onChange={(event) => setChargebackDialog({ ...chargebackDialog, idempotency_key: event.target.value })} placeholder="必填，用于防止重复追扣" />
                <button className="ghost small" type="button" onClick={() => setChargebackDialog({ ...chargebackDialog, idempotency_key: newCashierOrderChargebackKey(chargebackDialog.order.id) })}>生成</button>
              </div>
            </Field>
          </div>
        </Modal>
      ) : null}
    </section>
  )
}

function ProviderConfigGuide({ providerType }: { providerType: PaymentProviderType }) {
  const guide = cashierProviderConfigGuide(providerType)
  return (
    <div className="cashier-provider-guide span-2">
      <div>
        <strong>{guide.title}</strong>
        <p>{guide.detail}</p>
        <p>{guide.secretHint}</p>
      </div>
      <div className="cashier-provider-guide-fields">
        <span>必填：{guide.requiredFields.length ? guide.requiredFields.join(' / ') : '按渠道文档填写'}</span>
        {guide.optionalFields.length ? <span>可选：{guide.optionalFields.join(' / ')}</span> : null}
      </div>
    </div>
  )
}

function CashierSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="cashier-section">
      <div className="section-head compact">
        <strong>{title}</strong>
      </div>
      {children}
    </section>
  )
}

function DetailItem({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="cashier-detail-item">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  )
}

function StatusBadge({ badge }: { badge: CashierStatusBadge }) {
  return <Badge tone={badge.tone}>{badge.label}</Badge>
}

function WebhookEventAction({ event, retrying, onRetry }: { event: PaymentWebhookEvent; retrying: boolean; onRetry: (event: PaymentWebhookEvent) => void }) {
  const action = cashierWebhookEventAction(event)
  if (!action.canRetry) {
    return <span className="muted-action" title={action.title}>{action.label}</span>
  }
  return (
    <button type="button" className="ghost small" disabled={retrying} title={action.title} onClick={() => void onRetry(event)}>
      {retrying ? '重试中' : action.label}
    </button>
  )
}

function parseConfigText(raw: string): Record<string, unknown> {
  const trimmed = raw.trim()
  if (!trimmed) return {}
  const parsed = JSON.parse(trimmed)
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('渠道配置必须是 JSON 对象')
  }
  return parsed as Record<string, unknown>
}

function mergeConfigObjects(base: Record<string, unknown>, patch: Record<string, unknown>): Record<string, unknown> {
  const merged: Record<string, unknown> = { ...base }
  for (const [key, patchValue] of Object.entries(patch)) {
    const baseValue = merged[key]
    if (isPlainRecord(baseValue) && isPlainRecord(patchValue)) {
      merged[key] = mergeConfigObjects(baseValue, patchValue)
    } else if (baseValue === undefined || baseValue === null || baseValue === '') {
      merged[key] = patchValue
    } else if (key === 'payment_mode' || key === 'way_code') {
      merged[key] = patchValue
    }
  }
  return merged
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function newPlanDraft(): PlanDraft {
  return {
    plan_code: '',
    plan_name: '',
    plan_type: 'points_package',
    purchase_enabled: true,
    status: 'active',
    price_cny: '19.90000',
    points: '100.00000',
    bonus_points: '0.00000',
    duration_days: '30',
    currency: 'CNY',
    sort_order: '10',
    description: '',
  }
}

function editPlanDraft(row: CashierPlan): PlanDraft {
  return {
    row,
    plan_code: row.plan_code,
    plan_name: row.plan_name,
    plan_type: row.plan_type ?? 'points_package',
    purchase_enabled: Boolean(row.purchase_enabled),
    status: row.status,
    price_cny: row.price_cny,
    points: row.points,
    bonus_points: row.bonus_points,
    duration_days: String(row.duration_days),
    currency: row.currency,
    sort_order: String(row.sort_order ?? 0),
    description: row.description ?? '',
  }
}

function newInstanceDraft(): InstanceDraft {
  return {
    provider_type: 'mock',
    name: '',
    enabled: true,
    supported_methods: 'mock',
    sort_order: '10',
    scheduler_weight: '100',
    min_amount_cny: '1.00000',
    max_amount_cny: '999.00000',
    daily_amount_limit_cny: '',
    config_text: '{\n  "mock": true\n}',
  }
}

function editInstanceDraft(row: PaymentProviderInstance): InstanceDraft {
  const limits = row.limits ?? {}
  return {
    row,
    provider_type: row.provider_type,
    name: row.name,
    enabled: Boolean(row.enabled),
    supported_methods: row.supported_methods.join(', '),
    sort_order: String(row.sort_order ?? 0),
    scheduler_weight: String(row.scheduler_weight ?? 100),
    min_amount_cny: limits.min_amount_cny ?? '',
    max_amount_cny: limits.max_amount_cny ?? '',
    daily_amount_limit_cny: limits.daily_amount_limit_cny ?? '',
    config_text: JSON.stringify(row.config ?? {}, null, 2),
  }
}

function newCompleteOrderDraft(order: PaymentOrder): CompleteOrderDraft {
  return {
    order,
    provider: order.provider_type || order.provider || order.visible_method || 'manual',
    trade_no: order.trade_no ?? '',
    reason: '',
  }
}

function newRefundOrderDraft(order: PaymentOrder): RefundOrderDraft {
  return {
    order,
    refund_trade_no: order.refund_trade_no ?? '',
    refund_amount_cny: '',
    reason: '',
  }
}

function newChargebackOrderDraft(order: PaymentOrder): ChargebackOrderDraft {
  const remainingPoints = Math.max(0, Number(order.points ?? '0') + Number(order.bonus_points ?? '0') - Number(order.refunded_points ?? '0'))
  return {
    order,
    charge_points: remainingPoints > 0 ? remainingPoints.toFixed(5) : '',
    reason: '',
    idempotency_key: newCashierOrderChargebackKey(order.id),
  }
}

function canChargebackOrder(order: PaymentOrder) {
  return order.status === 'completed' || order.status === 'partially_refunded' || order.status === 'refunded'
}

function newCashierOrderChargebackKey(orderID: string | number) {
  const random = typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `cashier-order-${orderID}-chargeback-${random}`
}

function remainingRefundAmountCNY(order: PaymentOrder) {
  const total = Number(order.amount_cny ?? '0')
  const refunded = Number(order.refunded_amount_cny ?? '0')
  const remaining = Math.max(0, total - refunded)
  return remaining > 0 ? remaining.toFixed(5) : '不填则退剩余金额'
}
