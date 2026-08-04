import { useEffect, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import { Archive, Pause, Pencil, Play, RotateCcw } from 'lucide-react'
import type { CashierCustomAmountConfig, CashierOverview, CashierPlan, CashierPlanStatus, CashierPlanTransitionAction, PageResult, PaymentOrder, PaymentProviderInstance, PaymentProviderType, PaymentSchedulerStrategy, PaymentVisibleMethod, PaymentWebhookEvent } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { AdminTabs, Badge, Drawer, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, Modal, PageHeader, TooltipIconButton } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { adminDataGrid, adminGridCols } from '../ui/dataGrid'
import { FilterBar, ListPage, Pager } from '../ui/dataTable'
import { applyJeePayWayCodeTemplate, jeepayTemplatesForProvider } from './cashierJeePayWayCodeTemplates'
import { cashierAdminDateTime, cashierManualCompletionProviderOptions, cashierOrderPaymentLabel, cashierOrderPurchaseTypeLabel, cashierProviderConfigStatusLabel, cashierProviderSupportedMethodsLabel, cashierWebhookEventTypeLabel, cashierWebhookProviderLabel } from './cashierPaymentDisplay'
import { CashierPlanEditorDialog } from './CashierPlanEditorDialog'
import { cashierPlanDraftFromRow, cashierPlanEmptyDraft, cashierPlanPayloadFromDraft, type CashierPlanDraft } from './cashierPlanDraft'
import { cashierPlanActions, cashierPlanEmptyState, cashierPlanFilterOptions, cashierPlanPurchaseBadge, cashierPlanSectionCopy, type CashierPlanAction } from './cashierPlanPurchase'
import { cashierProviderConfigFields, cashierProviderConfigGuide, cashierProviderInstanceFieldHints, cashierProviderLabel, cashierProviderSupportedMethodOptions, cashierProviderTypes, cashierProviderTypesForMethod, cashierSupportedMethodLabel, cashierToggleSupportedMethod } from './cashierProviderOptions'
import type { CashierProviderConfigField } from './cashierProviderOptions'
import { newProviderDraft, providerDraftForTypeChange, providerDraftFromInstance, providerPayloadFromDraft, type CashierProviderFormDraft } from './cashierProviderForm'
import { cashierOrderRiskRows, cashierWebhookRiskRow } from './cashierRiskRows'
import type { CashierRiskRow } from './cashierRiskRows'
import { cashierSyncRow } from './cashierSyncRows'
import { cashierTrialConfigDraft, cashierTrialConfigDraftDetail, cashierTrialConfigPayload, cashierTrialConfigSummary, type CashierTrialConfigDraft, type CashierTrialConfigSummary } from './cashierTrialConfig'
import type { CashierStatusBadge } from './cashierStatusRows'
import { cashierVisibleMethodRow } from './cashierVisibleMethodRows'
import { cashierWebhookRow } from './cashierWebhookRows'
import {
  cashierBooleanVisibilityLabel,
  cashierEnabledBadge,
  cashierOrderStatusBadge,
  cashierPlanStatusBadge,
  cashierPlanTypeLabel,
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
  trial: CashierTrialConfigSummary
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

type OrderFilters = {
  order_no: string
  user_id: string
  status: string
  visible_method: string
  purchase_type: string
}

type CashierTabId = 'overview' | 'plans' | 'methods' | 'instances' | 'orders' | 'events'

type PlanTransitionDraft = {
  plan: CashierPlan
  action: CashierPlanAction
}

const cashierTabs: Array<{ id: CashierTabId; label: string; detail: string }> = [
  { id: 'overview', label: '概览', detail: '指标与体验额度' },
  { id: 'plans', label: '充值套餐', detail: '固定积分包与自定义金额' },
  { id: 'methods', label: '支付方式', detail: '用户可见入口' },
  { id: 'instances', label: '渠道实例', detail: '多账号与限额' },
  { id: 'orders', label: '订单', detail: '补单查单退款' },
  { id: 'events', label: '回调事件', detail: '验签与重试' },
]

const cashierAdminPageSize = 10
const emptyOrderFilters: OrderFilters = { order_no: '', user_id: '', status: '', visible_method: '', purchase_type: '' }
const cashierClasses = {
  page: 'grid gap-6',
  overviewGrid: 'grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4',
  overviewCard: 'relative grid min-h-[108px] gap-1.5 overflow-hidden rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-4 transition-[border-color,background-color,transform] duration-[var(--admin-motion-base)] hover:-translate-y-0.5 hover:border-[var(--border-strong)] hover:bg-[var(--elevated)]',
  overviewLabel: 'text-xs font-semibold text-[var(--muted-strong)]',
  overviewValue: 'text-2xl font-bold text-[var(--text)]',
  overviewTrend: 'flex items-center gap-1 text-xs font-semibold text-[var(--green)]',
  chartContainer: 'rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-5',
  sectionTitle: 'mb-4 text-base font-semibold text-[var(--text)]',
  revenueBars: 'flex h-[220px] w-full items-end gap-1 px-2',
  revenueBar: 'group relative flex-1 rounded-t-sm bg-[var(--accent)]/20 transition-all hover:bg-[var(--accent)]',
  chartAxis: 'mt-3 flex justify-between px-2 text-xs font-medium text-[var(--muted-strong)]',
  splitCharts: 'grid grid-cols-1 gap-4 xl:grid-cols-2',
  distributionRow: 'grid gap-2',
  distributionMeta: 'flex justify-between gap-3 text-xs font-bold',
  distributionTrack: 'h-1.5 w-full overflow-hidden rounded-full bg-[var(--canvas)]',
  spenderRow: 'flex items-center justify-between gap-3 rounded-lg p-3 transition-all hover:bg-[var(--canvas)]',
  spenderAvatar: 'grid size-8 place-items-center rounded-lg bg-[var(--canvas)] text-xs font-bold text-[var(--muted-strong)]',
  configForm: 'grid gap-4',
  configPanelGrid: 'grid grid-cols-2 gap-4 max-[1100px]:grid-cols-1',
  configPanel: 'rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-5',
  configPanelHead: 'mb-4 flex flex-wrap items-center justify-between gap-3 border-b border-[var(--border)] pb-3',
  configPanelTitle: 'text-base font-semibold text-[var(--text)]',
  providerList: 'divide-y divide-[var(--border)]',
  providerItem: 'group flex items-center justify-between gap-4 px-2 py-3 transition-colors duration-[var(--admin-motion-base)] hover:bg-[var(--elevated)]',
  providerDot: 'size-2 rounded-full bg-[var(--muted)]',
  providerName: 'font-semibold text-[var(--text)]',
  providerType: 'mt-1 text-xs text-[var(--muted-strong)]',
  toggleSetting: 'flex items-center justify-between gap-4 px-2 py-3 transition-colors duration-[var(--admin-motion-base)] hover:bg-[var(--elevated)]',
  toggleSwitch: 'relative h-5 w-10 rounded-full transition-all',
  toggleKnob: 'absolute top-1 size-3 rounded-full bg-[var(--surface-solid)] transition-all',
  riskPanel: 'rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-5',
  riskMetricGrid: 'grid grid-cols-3 divide-x divide-[var(--border)] max-[760px]:grid-cols-1 max-[760px]:divide-x-0 max-[760px]:divide-y',
  riskMetric: 'px-4 py-2 first:pl-0 last:pr-0 max-[760px]:px-0 max-[760px]:py-3',
  riskLabel: 'mb-1 text-xs font-semibold text-[var(--muted-strong)]',
  riskValue: 'text-lg font-bold text-[var(--text)]',
  toggle: 'grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 rounded-xl border border-[var(--border)] bg-[var(--canvas)] p-2 text-sm has-[:checked]:border-[var(--accent)]/40 has-[:checked]:bg-[var(--accent)]/10',
  amountGrid: 'grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3',
  toolbar: 'flex flex-wrap items-start justify-between gap-3',
  actions: 'flex flex-wrap items-center justify-end gap-2',
  orderFilter: 'flex flex-wrap items-center justify-between gap-4',
  orderFilterFields: 'grid flex-1 grid-cols-[minmax(180px,1fr)_minmax(120px,.5fr)_minmax(120px,.5fr)_minmax(120px,.5fr)_minmax(120px,.5fr)] gap-3 max-[1100px]:grid-cols-2 max-[620px]:grid-cols-1',
  webhookInspector: 'grid gap-2 rounded-lg border border-[var(--border)] bg-[var(--canvas)] p-4 text-[var(--fg)]',
  webhookPre: 'max-h-[220px] overflow-auto whitespace-pre-wrap rounded-lg bg-[var(--canvas)] p-3 font-mono text-xs',
  jeepayTemplate: 'grid gap-3 rounded-lg border border-[var(--line)] bg-[var(--canvas)] p-4',
  templateButtonRow: 'grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-2',
  templateButton: 'grid gap-1 rounded-lg border border-[var(--line)] bg-[var(--surface-solid)] p-3 text-left text-sm transition-colors duration-[var(--admin-motion-base)] hover:border-[var(--accent)]/30 hover:bg-[var(--accent)]/10',
  textarea: 'min-h-[160px] w-full resize-y rounded-lg border border-[var(--line)] bg-[var(--surface-solid)] px-3 py-2 font-mono text-xs outline-none focus:border-[var(--accent)]/50 focus:ring-1 focus:ring-[var(--accent)]/50',
  detailGrid: 'grid grid-cols-[repeat(auto-fit,minmax(190px,1fr))] gap-3',
  detailItem: 'grid gap-1 rounded-lg border border-[var(--line)] bg-[var(--canvas)] p-3',
  riskGrid: 'grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3',
  riskItem: 'grid gap-1 rounded-lg border p-3',
  riskTone: {
    neutral: 'border-[var(--line)] bg-[var(--canvas)]',
    success: 'border-[var(--green)]/30 bg-[var(--green)]/10',
    warning: 'border-[var(--amber)]/30 bg-[var(--amber)]/10',
    danger: 'border-[var(--red)]/30 bg-[var(--red)]/10',
  },
  section: 'grid gap-4 rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-5',
  sectionHead: 'flex flex-wrap items-center justify-between gap-2',
  pager: 'flex flex-wrap items-center justify-between gap-3 pt-3',
  providerGuide: 'col-span-full grid gap-3 rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-4',
  providerGuideFields: 'flex flex-wrap gap-2 text-xs text-[var(--soft)]',
  structuredConfig: 'col-span-full grid gap-3 rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-4',
  secretConfig: 'col-span-full grid gap-3 rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-4',
  secretConfigGrid: 'grid grid-cols-[minmax(0,1fr)_minmax(220px,.7fr)] gap-3 max-[760px]:grid-cols-1',
  supportedMethods: 'grid grid-cols-[repeat(auto-fit,minmax(150px,1fr))] gap-2',
  checkOption: 'grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 rounded-xl border border-[var(--border)] bg-[var(--canvas)] p-2 text-sm has-[:checked]:border-[var(--accent)]/40 has-[:checked]:bg-[var(--accent)]/10',
  inlineControl: 'flex items-center gap-2',
  methodName: 'grid gap-1',
  methodCode: 'grid gap-1',
}

const schedulerOptions: Array<{ value: PaymentSchedulerStrategy; label: string }> = [
  { value: 'round_robin', label: '轮询调度' },
  { value: 'random', label: '随机调度' },
]

const commonVisibleMethodOptions = ['mock', 'alipay', 'wxpay', 'stripe']

export function CashierPage({
  onFeedback,
  initialTab = 'overview',
  allowedTabs,
  pageTitle = '收银台',
}: {
  onFeedback?: (title: string, detail?: string) => void
  initialTab?: CashierTabId
  allowedTabs?: CashierTabId[]
  pageTitle?: string
}) {
  const [data, setData] = useState<CashierData | null>(null)
  const [customDraft, setCustomDraft] = useState<CashierCustomAmountConfig | null>(null)
  const [trialDraft, setTrialDraft] = useState<CashierTrialConfigDraft | null>(null)
  const [methodsDraft, setMethodsDraft] = useState<PaymentVisibleMethod[]>([])
  const [planDialog, setPlanDialog] = useState<CashierPlanDraft | null>(null)
  const [planTransition, setPlanTransition] = useState<PlanTransitionDraft | null>(null)
  const [planStatusFilter, setPlanStatusFilter] = useState<CashierPlanStatus>('active')
  const [instanceDialog, setInstanceDialog] = useState<CashierProviderFormDraft | null>(null)
  const [orderDetail, setOrderDetail] = useState<PaymentOrder | null>(null)
  const [completeDialog, setCompleteDialog] = useState<CompleteOrderDraft | null>(null)
  const [refundDialog, setRefundDialog] = useState<RefundOrderDraft | null>(null)
  const [chargebackDialog, setChargebackDialog] = useState<ChargebackOrderDraft | null>(null)
  const [loadingOrderID, setLoadingOrderID] = useState<number | string | null>(null)
  const [closingOrderID, setClosingOrderID] = useState<number | string | null>(null)
  const [retryingEventID, setRetryingEventID] = useState<number | string | null>(null)
  const [loading, setLoading] = useState(true)
  const visibleTabs = cashierTabs.filter((tab) => !allowedTabs || allowedTabs.includes(tab.id))
  const safeInitialTab = visibleTabs.some((tab) => tab.id === initialTab) ? initialTab : visibleTabs[0]?.id ?? 'overview'
  const [activeTab, setActiveTab] = useState<CashierTabId>(safeInitialTab)
  const [customAmountOpen, setCustomAmountOpen] = useState(false)
  const [ordersPage, setOrdersPage] = useState(1)
  const [ordersPageSize, setOrdersPageSize] = useState(cashierAdminPageSize)
  const [orderFilters, setOrderFilters] = useState<OrderFilters>(emptyOrderFilters)
  const [eventsPage, setEventsPage] = useState(1)
  const [eventsPageSize, setEventsPageSize] = useState(cashierAdminPageSize)
  const [savingTrial, setSavingTrial] = useState(false)
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
      const [overview, plans, customAmount, methods, instances, orders, events, configTabs] = await Promise.all([
        adminApi.getCashierOverview(),
        adminApi.listCashierPlans({ status: planStatusFilter }),
        adminApi.getCashierCustomAmountConfig(),
        adminApi.listPaymentVisibleMethods(),
        adminApi.listPaymentProviderInstances(),
        adminApi.listPaymentOrders(cashierOrderQuery(ordersPage, orderFilters)),
        adminApi.listPaymentWebhookEvents({ page: eventsPage, page_size: eventsPageSize }),
        adminApi.listConfigTabs(),
      ])
      const trial = cashierTrialConfigSummary(configTabs)
      setData({ overview, plans, customAmount, methods, instances, orders, events, trial })
      setCustomDraft(customAmount)
      setTrialDraft(cashierTrialConfigDraft(trial))
      setMethodsDraft(methods)
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

  async function saveTrialConfig(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!data || !trialDraft) return
    setSavingTrial(true)
    setError(null)
    try {
      await adminApi.updateConfigTab(data.trial.tabKey, cashierTrialConfigPayload(data.trial, trialDraft))
      const configTabs = await adminApi.listConfigTabs()
      const trial = cashierTrialConfigSummary(configTabs)
      setData((current) => current ? { ...current, trial } : current)
      setTrialDraft(cashierTrialConfigDraft(trial))
      onFeedback?.('注册送体验额度已保存', trial.detail)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '注册送体验额度保存失败')
    } finally {
      setSavingTrial(false)
    }
  }

  async function saveVisibleMethods(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError(null)
    const validationError = validatePaymentVisibleMethods(methodsDraft)
    if (validationError) {
      setError(validationError)
      return
    }
    setSavingMethods(true)
    try {
      const response = await adminApi.updatePaymentVisibleMethods(normalizePaymentVisibleMethods(methodsDraft))
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

  function updateMethodCodeDraft(index: number, methodCode: string) {
    const method = methodCode.trim().toLowerCase()
    setMethodsDraft((current) => current.map((item, itemIndex) => {
      if (itemIndex !== index) return item
      const compatibleProviders = cashierProviderTypesForMethod(method)
      const sourceProviderType = compatibleProviders.includes(item.source_provider_type ?? '') ? item.source_provider_type : compatibleProviders[0]
      return {
        ...item,
        method,
        label: item.label.trim() ? item.label : cashierSupportedMethodLabel(method),
        source_provider_type: sourceProviderType,
      }
    }))
  }

  function addVisibleMethodDraft() {
    setMethodsDraft((current) => [...current, newVisibleMethodDraft(current)])
  }

  function deleteVisibleMethodDraft(index: number) {
    setMethodsDraft((current) => current.filter((_, itemIndex) => itemIndex !== index))
  }

  async function reloadPlans(status: CashierPlanStatus = planStatusFilter) {
    const plans = await adminApi.listCashierPlans({ status })
    setData((current) => current ? { ...current, plans } : current)
  }

  async function changePlanFilter(status: CashierPlanStatus) {
    setPlanStatusFilter(status)
    setSavingPlan(true)
    setError(null)
    try {
      await reloadPlans(status)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '套餐列表载入失败')
    } finally {
      setSavingPlan(false)
    }
  }

  async function reloadInstances() {
    const instances = await adminApi.listPaymentProviderInstances()
    setData((current) => current ? { ...current, instances } : current)
  }

  async function reloadOrders(page = ordersPage, filters = orderFilters) {
    const orders = await adminApi.listPaymentOrders(cashierOrderQuery(page, filters, ordersPageSize))
    setOrdersPage(page)
    setData((current) => current ? { ...current, orders } : current)
  }

  async function applyOrderFilters(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    await reloadOrders(1, orderFilters)
  }

  async function resetOrderFilters() {
    setOrderFilters(emptyOrderFilters)
    await reloadOrders(1, emptyOrderFilters)
  }

  async function reloadEvents(page = eventsPage) {
    const events = await adminApi.listPaymentWebhookEvents({ page, page_size: eventsPageSize })
    setEventsPage(page)
    setData((current) => current ? { ...current, events } : current)
  }

  async function savePlan() {
    if (!planDialog) return
    setSavingPlan(true)
    setError(null)
    try {
      const payload = cashierPlanPayloadFromDraft(planDialog)
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

  async function transitionPlan() {
    if (!planTransition) return
    setSavingPlan(true)
    setError(null)
    try {
      const updated = await adminApi.transitionCashierPlan(planTransition.plan.id, planTransition.action.action)
      setPlanTransition(null)
      await reloadPlans()
      onFeedback?.(planTransition.action.label, updated.plan_name)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '套餐状态更新失败')
    } finally {
      setSavingPlan(false)
    }
  }

  async function saveInstance() {
    if (!instanceDialog) return
    setSavingInstance(true)
    setError(null)
    try {
      const payload = providerPayloadFromDraft(instanceDialog)
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

  async function deleteInstance(instance: PaymentProviderInstance) {
    if (!window.confirm(`确定删除支付渠道实例「${instance.name}」吗？已创建订单的渠道快照会保留。`)) return
    setSavingInstance(true)
    setError(null)
    try {
      const deleted = await adminApi.deletePaymentProviderInstance(instance.id)
      await reloadInstances()
      onFeedback?.('支付渠道实例已删除', deleted.name)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '支付渠道实例删除失败')
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
      await reloadOrders()
      onFeedback?.('订单已人工补单完成', updated.order_no)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '人工补单失败')
    } finally {
      setCompletingOrder(false)
    }
  }

  async function closePaymentOrder(order: PaymentOrder) {
    if (!window.confirm(`确认关闭待支付订单 ${order.order_no}？关闭后用户需要重新创建订单。`)) return
    setClosingOrderID(order.id)
    setError(null)
    try {
      const updated = await adminApi.closePaymentOrder(order.id, { reason: '运营关闭待支付订单' })
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
      await reloadOrders()
      onFeedback?.('订单已关闭', updated.order_no)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '订单关闭失败')
    } finally {
      setClosingOrderID(null)
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
      await reloadOrders()
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
      await reloadOrders()
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
      const syncRow = cashierSyncRow(result.sync)
      await reloadOrders()
      onFeedback?.(result.sync.completed ? '查单已确认到账' : syncRow.categoryLabel, syncRow.actionHint)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '订单查单失败')
    } finally {
      setLoadingOrderID(null)
    }
  }

  useEffect(() => { void load() }, [])
  useEffect(() => {
    setActiveTab(safeInitialTab)
  }, [safeInitialTab])

  if (loading) return <LoadingBlock label="读取收银台配置" />
  if (error && !data) return <ErrorBlock message={error} onRetry={load} />
  if (!data) return <EmptyBlock title="暂无收银台数据" detail="后台尚未返回收银台配置。" />

  const isOrdersPage = visibleTabs.some((tab) => tab.id === 'orders')
  const isConfigPage = visibleTabs.some((tab) => tab.id === 'methods' || tab.id === 'instances')
  const isPackagesPage = visibleTabs.length === 1 && visibleTabs[0]?.id === 'plans'
  const showSoloTabs = visibleTabs.length > 1
  const tabs = visibleTabs.map((tab) => ({
    ...tab,
    label: tab.id === 'overview' && isOrdersPage ? '订单概览' : tab.id === 'orders' ? '订单记录' : tab.label,
  }))

  return (
    <section className={cashierClasses.page}>
      <PageHeader
        title={pageTitle}
        description={isConfigPage ? '管理支付方式、渠道实例和体验额度，长表单使用抽屉分步配置。' : '管理收银台相关套餐、订单和支付配置。'}
      />
      {showSoloTabs ? (
        <AdminTabs
          ariaLabel="收银台管理分区"
          items={tabs.map((tab) => ({ id: tab.id, label: tab.label, description: tab.detail }))}
          value={activeTab}
          onChange={setActiveTab}
        />
      ) : null}

      {error ? <InlineFeedback tone="danger" message={error} /> : null}

      {activeTab === 'overview' && isOrdersPage ? <OrderOverviewPanel data={data} /> : null}

      {activeTab === 'overview' && !isOrdersPage ? (
        <>
          {isConfigPage ? <CashierConfigOverview data={data} onAddInstance={() => setInstanceDialog(newProviderDraft('mock'))} /> : <CashierOverviewCards data={data} />}
          {isConfigPage ? <CashierSection title="注册送体验额度">
            <form className={cashierClasses.configForm} onSubmit={(event) => void saveTrialConfig(event)}>
              <div className={cashierClasses.toolbar}>
                <p>{trialDraft ? cashierTrialConfigDraftDetail(trialDraft) : data.trial.detail}</p>
                <div className={cashierClasses.actions}>
                  <StatusBadge badge={cashierEnabledBadge(Boolean(trialDraft?.enabled))} />
                  <button type="submit" className={adminButton.base} disabled={savingTrial || !trialDraft}>{savingTrial ? '保存中' : '保存体验额度'}</button>
                </div>
              </div>
              <label className={cashierClasses.toggle}>
                <input
                  type="checkbox"
                  checked={Boolean(trialDraft?.enabled)}
                  onChange={(event) => setTrialDraft((current) => current ? { ...current, enabled: event.target.checked } : current)}
                />
                <span>启用注册送体验额度</span>
              </label>
              <div className={cashierClasses.amountGrid}>
                <Field label="赠送积分">
                  <input
                    value={trialDraft?.points ?? ''}
                    onChange={(event) => setTrialDraft((current) => current ? { ...current, points: event.target.value } : current)}
                    inputMode="decimal"
                    placeholder="20.00000"
                  />
                </Field>
                <Field label="有效天数">
                  <input
                    value={trialDraft?.valid_days ?? ''}
                    onChange={(event) => setTrialDraft((current) => current ? { ...current, valid_days: event.target.value } : current)}
                    type="number"
                    min="1"
                    step="1"
                  />
                </Field>
                <Field label="提醒阈值">
                  <input
                    value={trialDraft?.expiry_reminder_days ?? ''}
                    onChange={(event) => setTrialDraft((current) => current ? { ...current, expiry_reminder_days: event.target.value } : current)}
                    type="number"
                    min="0"
                    step="1"
                  />
                </Field>
                <label className={cashierClasses.toggle}>
                  <input
                    type="checkbox"
                    checked={Boolean(trialDraft?.grant_once_per_user)}
                    onChange={(event) => setTrialDraft((current) => current ? { ...current, grant_once_per_user: event.target.checked } : current)}
                  />
                  <span>每个用户仅领取一次</span>
                </label>
              </div>
            </form>
          </CashierSection> : null}
        </>
      ) : null}

          {activeTab === 'plans' ? <CashierSection
            title="套餐配置"
            plain={isPackagesPage}
            actions={<>
              {isPackagesPage ? (
                <button type="button" className={cn(adminButton.base, adminButton.ghost)} aria-expanded={customAmountOpen} onClick={() => setCustomAmountOpen((value) => !value)}>自定义金额</button>
              ) : null}
              <button type="button" className={cn(adminButton.base, adminButton.primary)} onClick={() => setPlanDialog(cashierPlanEmptyDraft())}>新增套餐</button>
            </>}
          >
            <div className={cashierClasses.toolbar}>
              {!isPackagesPage ? <p>{cashierPlanSectionCopy.toolbarDetail}</p> : <span />}
              <label className="flex items-center gap-2 text-sm font-semibold text-[var(--text)]">
                <span>状态</span>
                <select value={planStatusFilter} disabled={savingPlan} onChange={(event) => void changePlanFilter(event.target.value as CashierPlanStatus)}>
                  {cashierPlanFilterOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                </select>
              </label>
            </div>
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
              {data.plans.items.map((plan) => {
                const active = plan.status === 'active' && Boolean(plan.purchase_enabled)
                const planStatus = cashierPlanStatusBadge(plan.status)
                const purchaseStatus = cashierPlanPurchaseBadge(plan)
                return (
                  <div key={plan.id} className={cn('group rounded-lg border p-5 transition-[border-color,background-color,transform] duration-[var(--admin-motion-base)] hover:-translate-y-0.5', active ? 'border-[var(--border-strong)] bg-[var(--elevated)]' : 'border-[var(--border)] bg-[var(--surface)] opacity-60')}>
                    <div className="mb-5 flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <h4 className="truncate text-base font-semibold text-[var(--text)] transition-colors group-hover:text-[var(--accent)]">{plan.plan_name}</h4>
                        <p className="mt-1 font-mono text-xs text-[var(--muted-strong)]">{plan.plan_code} · {cashierPlanTypeLabel(plan.plan_type)}</p>
                      </div>
                      <span className="flex flex-wrap justify-end gap-1.5"><StatusBadge badge={planStatus} /><StatusBadge badge={purchaseStatus} /></span>
                    </div>
                    <div className="mb-5 grid gap-1.5">
                      <div className="text-2xl font-bold text-[var(--text)]">
                        {Number(plan.points).toFixed(0)}
                        <span className="ml-1 text-xs font-normal text-[var(--muted-strong)]">积分</span>
                      </div>
                      <div className="font-mono text-base font-semibold text-[var(--accent)]">¥ {Number(plan.price_cny).toFixed(2)}</div>
                    </div>
                    <div className="flex items-center justify-end gap-2">
                      {plan.status !== 'archived' ? <TooltipIconButton label={`编辑 ${plan.plan_name}`} disabled={savingPlan} onClick={() => setPlanDialog(cashierPlanDraftFromRow(plan))}><Pencil /></TooltipIconButton> : null}
                      {cashierPlanActions(plan).map((action) => (
                        <TooltipIconButton
                          key={action.action}
                          label={`${action.label} ${plan.plan_name}`}
                          disabled={savingPlan}
                          className={action.tone === 'danger' ? 'text-[var(--red)]' : undefined}
                          onClick={() => setPlanTransition({ plan, action })}
                        >
                          <PlanActionIcon action={action.action} />
                        </TooltipIconButton>
                      ))}
                    </div>
                  </div>
                )
              })}
            </div>
            {!data.plans.items.length ? <EmptyBlock title={cashierPlanEmptyState.title} detail={cashierPlanEmptyState.detail} /> : null}
          </CashierSection> : null}

          {planTransition ? <Modal
            title={planTransition.action.label}
            detail={planTransition.action.detail}
            onClose={() => setPlanTransition(null)}
            footer={<>
              <button type="button" className={cn(adminButton.base, adminButton.ghost)} disabled={savingPlan} onClick={() => setPlanTransition(null)}>取消</button>
              <button type="button" className={cn(adminButton.base, planTransition.action.tone === 'danger' ? adminButton.danger : adminButton.primary)} disabled={savingPlan} onClick={() => void transitionPlan()}>{savingPlan ? '处理中...' : '确认'}</button>
            </>}
          ><p className="text-sm leading-6 text-[var(--muted-strong)]">状态变更只影响后续展示和购买，不会修改历史订单、积分或账本记录。</p></Modal> : null}

          {activeTab === 'plans' && (!isPackagesPage || customAmountOpen) ? <CashierSection title="自定义金额">
            <form className={cashierClasses.configForm} onSubmit={(event) => void saveCustomAmount(event)}>
              <label className={cashierClasses.toggle}>
                <input
                  type="checkbox"
                  checked={Boolean(customDraft?.enabled)}
                  onChange={(event) => setCustomDraft((current) => current ? { ...current, enabled: event.target.checked } : current)}
                />
                <span>允许用户输入金额</span>
              </label>
              <div className={cashierClasses.amountGrid}>
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
                <div className={cashierClasses.actions}>
                  <StatusBadge badge={cashierEnabledBadge(Boolean(data.customAmount.enabled))} />
                  <button type="submit" className={adminButton.base} disabled={savingCustomAmount}>{savingCustomAmount ? '保存中' : '保存'}</button>
                </div>
              </div>
            </form>
          </CashierSection> : null}

          {activeTab === 'methods' ? <CashierSection title="可见支付方式">
            <form className={cashierClasses.configForm} onSubmit={(event) => void saveVisibleMethods(event)}>
              <div className={cashierClasses.toolbar}>
                <p>控制用户收银台可选择的支付入口；生产环境 Mock 仍由后端隐藏。</p>
                <div className={cashierClasses.actions}>
                  <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={addVisibleMethodDraft} disabled={savingMethods}>新增支付方式</button>
                  <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={() => setMethodsDraft(data.methods)} disabled={savingMethods}>重置</button>
                  <button type="submit" className={adminButton.base} disabled={savingMethods}>{savingMethods ? '保存中' : '保存支付方式'}</button>
                </div>
              </div>
              <div className={cn(adminDataGrid.root, adminGridCols.editableMethods)}>
                <div className={cn(adminDataGrid.head, adminGridCols.editableMethods)}><span>方式</span><span>展示名称</span><span>渠道类型</span><span>调度</span><span>排序</span><span>状态</span><span>操作</span></div>
                {methodsDraft.map((method, index) => {
                  const row = cashierVisibleMethodRow(method)
                  return (
                    <div key={`${method.method}-${index}`} className={cn(adminDataGrid.row, adminGridCols.editableMethods, '[&_input]:min-h-[34px] [&_input]:px-2.5 [&_input]:py-2 [&_input]:text-sm [&_select]:min-h-[34px] [&_select]:px-2.5 [&_select]:py-2 [&_select]:text-sm')}>
                      <div className={cashierClasses.methodCode}>
                        <input
                          list="cashier-visible-method-codes"
                          value={method.method}
                          onChange={(event) => updateMethodCodeDraft(index, event.target.value)}
                          aria-label={`${row.title} 支付方式代码`}
                          placeholder="alipay"
                        />
                        <p>{row.detail}</p>
                      </div>
                      <div className={cashierClasses.methodName}>
                        <input
                          value={method.label}
                          onChange={(event) => updateMethodDraft(index, { label: event.target.value })}
                          aria-label={`${row.title} 展示名称`}
                        />
                        <p>{row.title}</p>
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
                      <label className={cashierClasses.toggle}>
                        <input
                          type="checkbox"
                          checked={method.enabled}
                          onChange={(event) => updateMethodDraft(index, { enabled: event.target.checked })}
                        />
                        <span>{cashierVisibleFlagLabel(method.enabled)}</span>
                      </label>
                      <div className={adminDataGrid.actions}>
                        <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small, adminButton.danger)} disabled={savingMethods} onClick={() => deleteVisibleMethodDraft(index)}>删除</button>
                      </div>
                    </div>
                  )
                })}
              </div>
              <datalist id="cashier-visible-method-codes">
                {commonVisibleMethodOptions.map((method) => <option key={method} value={method}>{cashierSupportedMethodLabel(method)}</option>)}
              </datalist>
            </form>
          </CashierSection> : null}

          {activeTab === 'instances' ? <CashierSection title="支付渠道实例">
            <div className={cashierClasses.toolbar}>
              <p>配置真实支付账号或测试 Mock 账号；密钥保存后仅显示配置状态和指纹。</p>
              <button type="button" className={adminButton.base} onClick={() => setInstanceDialog(newProviderDraft('mock'))}>新增实例</button>
            </div>
            <div className={cn(adminDataGrid.root, adminGridCols.cashierInstances)}>
              <div className={cn(adminDataGrid.head, adminGridCols.cashierInstances)}><span>实例</span><span>类型</span><span>方式</span><span>权重</span><span>状态</span><span>操作</span></div>
              {data.instances.items.map((instance) => (
                <div key={instance.id} className={cn(adminDataGrid.row, adminGridCols.cashierInstances)}>
                  <div className={adminDataGrid.stackCell}><strong>{instance.name}</strong><p className={adminDataGrid.detail}>{cashierProviderConfigStatusLabel(instance.config_status)}</p></div>
                  <span>{cashierProviderLabel(instance.provider_type)}</span>
                  <span>{cashierProviderSupportedMethodsLabel(instance.supported_methods)}</span>
                  <code className={adminDataGrid.code}>{instance.scheduler_weight}</code>
                  <StatusBadge badge={cashierEnabledBadge(instance.enabled)} />
                  <div className={adminDataGrid.actions}>
                    <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={() => setInstanceDialog(providerDraftFromInstance(instance))}>编辑</button>
                    <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small, adminButton.danger)} disabled={savingInstance} onClick={() => void deleteInstance(instance)}>删除</button>
                  </div>
                </div>
              ))}
            </div>
          </CashierSection> : null}

          {activeTab === 'orders' ? <CashierSection title="订单记录">
            <form onSubmit={(event) => void applyOrderFilters(event)}>
              <FilterBar
                fields={[
                  { key: 'order_no', label: '订单号', primary: true, control: <input value={orderFilters.order_no} onChange={(event) => setOrderFilters({ ...orderFilters, order_no: event.target.value })} placeholder="订单号 / 用户 ID..." /> },
                  { key: 'user_id', label: '用户 ID', primary: true, control: <input value={orderFilters.user_id} onChange={(event) => setOrderFilters({ ...orderFilters, user_id: event.target.value })} inputMode="numeric" placeholder="用户 ID" /> },
                  { key: 'status', label: '状态', primary: true, control: <select value={orderFilters.status} onChange={(event) => setOrderFilters({ ...orderFilters, status: event.target.value })}><option value="">全部状态</option><option value="pending">待支付</option><option value="completed">已到账</option><option value="canceled">已关闭</option><option value="failed">支付失败</option><option value="partially_refunded">部分退款</option><option value="refunded">已退款</option></select> },
                  { key: 'visible_method', label: '支付方式', control: <select value={orderFilters.visible_method} onChange={(event) => setOrderFilters({ ...orderFilters, visible_method: event.target.value })}><option value="">全部方式</option><option value="mock">Mock</option><option value="alipay">支付宝</option><option value="wxpay">微信支付</option></select> },
                  { key: 'purchase_type', label: '购买类型', control: <select value={orderFilters.purchase_type} onChange={(event) => setOrderFilters({ ...orderFilters, purchase_type: event.target.value })}><option value="">全部类型</option><option value="plan">固定积分包</option><option value="custom_amount">自定义金额</option></select> },
                ]}
                actions={(
                  <>
                    <button type="submit" className={cn(adminButton.base, adminButton.primary, adminButton.small)}>查询</button>
                    <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={() => void resetOrderFilters()}>重置</button>
                  </>
                )}
              />
            </form>
            <Pager
              page={ordersPage}
              pageSize={cashierAdminPageSize}
              total={data.orders.total}
              onChange={(next) => void reloadOrders(next)}
              onPageSizeChange={(size) => { setOrdersPageSize(size); void reloadOrders(1) }}
            />
            <div className={cn(adminDataGrid.root, adminGridCols.cashierOrders)}>
              <div className={cn(adminDataGrid.head, adminGridCols.cashierOrders)}><span>订单</span><span>金额</span><span>积分</span><span>方式</span><span>状态</span><span>操作</span></div>
              {data.orders.items.map((order) => (
                <div key={order.id ?? order.order_no} className={cn(adminDataGrid.row, adminGridCols.cashierOrders)}>
                  <div className={adminDataGrid.stackCell}><strong>{order.order_no}</strong><p className={adminDataGrid.detail}>{cashierOrderPurchaseTypeLabel(order)}</p></div>
                  <code className={adminDataGrid.code}>¥{Number(order.amount_cny ?? '0').toFixed(2)}</code>
                  <code className={adminDataGrid.code}>{Number(order.points ?? '0').toFixed(2)}</code>
                  <code className={adminDataGrid.code}>{cashierOrderPaymentLabel(order)}</code>
                  <StatusBadge badge={cashierOrderStatusBadge(order.status)} />
                  <div className={adminDataGrid.actions}>
                    <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} disabled={loadingOrderID === order.id} onClick={() => void openOrderDetail(order)}>
                      {loadingOrderID === order.id ? '读取中' : '详情'}
                    </button>
                    {order.status === 'pending' ? (
                      <>
                        <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} disabled={loadingOrderID === order.id} onClick={() => void syncPaymentOrder(order)}>
                          {loadingOrderID === order.id ? '查单中' : '查单'}
                        </button>
                        <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={() => setCompleteDialog(newCompleteOrderDraft(order))}>补单</button>
                        <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small, 'text-[var(--red)]')} disabled={closingOrderID === order.id} onClick={() => void closePaymentOrder(order)}>
                          {closingOrderID === order.id ? '关闭中' : '关闭'}
                        </button>
                      </>
                    ) : null}
                    {order.status === 'completed' || order.status === 'partially_refunded' ? (
                      <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={() => setRefundDialog(newRefundOrderDraft(order))}>退款</button>
                    ) : null}
                    {canChargebackOrder(order) ? (
                      <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={() => setChargebackDialog(newChargebackOrderDraft(order))}>追扣</button>
                    ) : null}
                  </div>
                </div>
              ))}
            </div>
          </CashierSection> : null}

          {activeTab === 'events' ? <CashierSection title="回调事件">
            <Pager
              page={eventsPage}
              pageSize={cashierAdminPageSize}
              total={data.events.total}
              onChange={(next) => void reloadEvents(next)}
              onPageSizeChange={(size) => { setEventsPageSize(size); void reloadEvents(1) }}
            />
            <div className={cn(adminDataGrid.root, adminGridCols.cashierEvents)}>
              <div className={cn(adminDataGrid.head, adminGridCols.cashierEvents)}><span>事件</span><span>订单</span><span>渠道</span><span>状态</span><span>操作</span></div>
              {data.events.items.map((event) => {
                const row = cashierWebhookRow(event)
                return (
                  <div key={event.id} className={cn(adminDataGrid.row, adminGridCols.cashierEvents)}>
                    <div className={adminDataGrid.stackCell}>
                      <strong>{row.title}</strong>
                      <p className={adminDataGrid.detail}>{cashierWebhookRiskRow(event).detail}</p>
                    </div>
                    <code className={adminDataGrid.code}>{row.orderLabel}</code>
                    <code className={adminDataGrid.code}>{row.providerLabel}</code>
                    <StatusBadge badge={row.statusBadge} />
                    <WebhookEventAction event={event} retrying={retryingEventID === event.id} onRetry={retryWebhookEvent} />
                    <div className={cashierClasses.webhookInspector}>
                      <span>验签：{row.signatureLabel}</span>
                      <span>处理：{row.resultSummary}</span>
                      <span>接收：{row.receivedAtLabel}</span>
                      <span>处理时间：{row.processedAtLabel}</span>
                      <pre className={cashierClasses.webhookPre}>{row.payloadPreview}</pre>
                    </div>
                  </div>
                )
              })}
            </div>
          </CashierSection> : null}
      {planDialog ? (
        <CashierPlanEditorDialog draft={planDialog} saving={savingPlan} error={error} onChange={setPlanDialog} onClose={() => setPlanDialog(null)} onSave={() => void savePlan()} />
      ) : null}
      {instanceDialog ? (
        <Drawer
          title={instanceDialog.row ? '编辑支付渠道实例' : '新增支付渠道实例'}
          description="按基础信息、金额限制和渠道字段完成配置；密钥保存后不会回显明文。"
          onClose={() => setInstanceDialog(null)}
          footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={savingInstance} onClick={() => setInstanceDialog(null)}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={savingInstance || !instanceDialog.name || !instanceDialog.provider_type} onClick={() => void saveInstance()}>{savingInstance ? '保存中...' : '保存'}</button></>}
        >
          {error ? <InlineFeedback tone="danger" message={error} /> : null}
          <div className="mb-4 grid grid-cols-3 gap-2 text-xs font-bold text-[var(--muted)] max-[720px]:grid-cols-1">
            {['基础信息', '金额限制', '渠道字段'].map((step, index) => (
              <span key={step} className="rounded-lg border border-[var(--border)] bg-[var(--surface)] px-3 py-2">{index + 1}. {step}</span>
            ))}
          </div>
          <div className={adminPage.formGrid}>
            <ProviderConfigGuide providerType={instanceDialog.provider_type} />
            <Field label="实例名称"><input value={instanceDialog.name} onChange={(event) => setInstanceDialog({ ...instanceDialog, name: event.target.value })} placeholder="支付宝沙箱主账号" /></Field>
            <Field label="渠道类型">
              <select
                value={instanceDialog.provider_type}
                onChange={(event) => {
                  const providerType = event.target.value as PaymentProviderType
                  setInstanceDialog(providerDraftForTypeChange(instanceDialog, providerType))
                }}
              >
                {cashierProviderTypes.map((providerType) => <option key={providerType} value={providerType}>{cashierProviderLabel(providerType)}</option>)}
              </select>
            </Field>
            <Field label="支持方式">
              <div className={cashierClasses.supportedMethods}>
                {cashierProviderSupportedMethodOptions(instanceDialog.provider_type, instanceDialog.supported_methods).map((option) => (
                  <label key={option.value} className={cashierClasses.checkOption}>
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
            <label className={cashierClasses.toggle}>
              <input type="checkbox" checked={instanceDialog.enabled} onChange={(event) => setInstanceDialog({ ...instanceDialog, enabled: event.target.checked })} />
              <span>启用实例</span>
            </label>
            {jeepayTemplatesForProvider(instanceDialog.provider_type).length ? (
              <div className={cashierClasses.jeepayTemplate}>
                <div>
                  <strong>JeePay 场景模板</strong>
                  <p>按基础支付、网页支付、服务商、分账和行业参数套用模板；会保留已有商户号、密钥和网关地址，只补齐支付模式、wayCode 和 channelExtra 示例。</p>
                </div>
                <div className={cashierClasses.templateButtonRow}>
                  {jeepayTemplatesForProvider(instanceDialog.provider_type).map((template) => (
                    <button
                      key={template.way_code}
                      type="button"
                      className={cn(adminButton.base, adminButton.ghost, adminButton.small, cashierClasses.templateButton)}
                      title={`${template.category} · ${template.description}`}
                      onClick={() => {
                        try {
                          const config = parseConfigText(applyJeePayWayCodeTemplate(JSON.stringify(instanceDialog.config), template.way_code))
                          setInstanceDialog({ ...instanceDialog, config })
                        } catch (caught) {
                          setError(caught instanceof Error ? caught.message : 'JeePay 模板套用失败')
                        }
                      }}
                    >
                      <span>{template.category}</span>
                      <strong>{template.label}</strong>
                      <em>{template.way_code}</em>
                    </button>
                  ))}
                </div>
              </div>
            ) : null}
            <ProviderStructuredConfigFields
              draft={instanceDialog}
              onChange={setInstanceDialog}
            />
          </div>
        </Drawer>
      ) : null}
      {orderDetail ? (
        <Modal
          title="订单详情"
          detail={orderDetail.order_no}
          onClose={() => setOrderDetail(null)}
          footer={<button className={adminButton.base} type="button" onClick={() => setOrderDetail(null)}>关闭</button>}
        >
          <CashierRiskPanel rows={cashierOrderRiskRows(orderDetail)} />
          <div className={cashierClasses.detailGrid}>
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
          footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={completingOrder} onClick={() => setCompleteDialog(null)}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={completingOrder || !completeDialog.trade_no.trim()} onClick={() => void completeOrderManually()}>{completingOrder ? '处理中...' : '确认到账'}</button></>}
        >
          {error ? <InlineFeedback tone="danger" message={error} /> : null}
          <div className={adminPage.formGrid}>
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
          footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={refundingOrder} onClick={() => setRefundDialog(null)}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={refundingOrder || !refundDialog.refund_trade_no.trim()} onClick={() => void refundPaymentOrder()}>{refundingOrder ? '处理中...' : '确认退款'}</button></>}
        >
          {error ? <InlineFeedback tone="danger" message={error} /> : null}
          <div className={adminPage.formGrid}>
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
          footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={chargingBackOrder} onClick={() => setChargebackDialog(null)}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={chargingBackOrder || !chargebackDialog.charge_points.trim() || !chargebackDialog.reason.trim() || !chargebackDialog.idempotency_key.trim()} onClick={() => void chargebackPaymentOrder()}>{chargingBackOrder ? '处理中...' : '确认追扣'}</button></>}
        >
          {error ? <InlineFeedback tone="danger" message={error} /> : null}
          <div className={adminPage.formGrid}>
            <Field label="追扣积分">
              <input value={chargebackDialog.charge_points} onChange={(event) => setChargebackDialog({ ...chargebackDialog, charge_points: event.target.value })} inputMode="decimal" placeholder="5.00000" />
            </Field>
            <Field label="追扣原因">
              <input value={chargebackDialog.reason} onChange={(event) => setChargebackDialog({ ...chargebackDialog, reason: event.target.value })} placeholder="渠道拒付已确认" />
            </Field>
            <Field label="Idempotency-Key">
              <div className={cashierClasses.inlineControl}>
                <input value={chargebackDialog.idempotency_key} onChange={(event) => setChargebackDialog({ ...chargebackDialog, idempotency_key: event.target.value })} placeholder="必填，用于防止重复追扣" />
                <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => setChargebackDialog({ ...chargebackDialog, idempotency_key: newCashierOrderChargebackKey(chargebackDialog.order.id) })}>生成</button>
              </div>
            </Field>
          </div>
        </Modal>
      ) : null}
    </section>
  )
}

function OrderOverviewPanel({ data }: { data: CashierData }) {
  const todayAmount = Number(data.overview.today_amount_cny ?? '0')
  const completedOrders = data.orders.items.filter((order) => order.status === 'completed' || order.status === 'partially_refunded' || order.status === 'refunded')
  const totalRevenue = completedOrders.reduce((sum, order) => sum + Number(order.amount_cny ?? '0'), 0)
  const averageAmount = data.overview.today_completed_count > 0 ? todayAmount / data.overview.today_completed_count : 0
  const revenueBars = revenueBarHeights(data.orders.items)
  const paymentRows = paymentDistributionRows(data.orders.items, data.overview.enabled_methods)
  const spenderRows = topSpenderRows(data.orders.items)

  return (
    <div className="grid gap-6">
      <div className={cashierClasses.overviewGrid}>
        <FinancialStatCard label="今日收入" value={`¥ ${todayAmount.toFixed(2)}`} trend={`支付成功率 ${data.overview.success_rate}`} />
        <FinancialStatCard label="今日订单数" value={String(data.overview.today_order_count)} trend={`已完成 ${data.overview.today_completed_count} 笔`} />
        <FinancialStatCard label="平均订单金额" value={`¥ ${averageAmount.toFixed(2)}`} trend={`待支付 ${data.overview.pending_count} 笔`} />
        <FinancialStatCard label="当前页营收" value={`¥ ${totalRevenue.toFixed(2)}`} trend={`共 ${data.orders.total ?? data.orders.items.length} 条记录`} />
      </div>

      <div className={cashierClasses.chartContainer}>
        <h3 className={cashierClasses.sectionTitle}>近 30 天营收趋势</h3>
        <div className={cashierClasses.revenueBars} aria-label="近 30 天营收趋势">
          {revenueBars.map((height, index) => (
            <div key={`${index}-${height}`} className={cashierClasses.revenueBar} style={{ height: `${height}%` }} />
          ))}
        </div>
        <div className={cashierClasses.chartAxis}>
          <span>30 天前</span>
          <span>今天</span>
        </div>
      </div>

      <div className={cashierClasses.splitCharts}>
        <div className={cashierClasses.chartContainer}>
          <h3 className={cashierClasses.sectionTitle}>支付方式分布</h3>
          <div className="grid gap-4">
            {paymentRows.map((row) => (
              <DistributionRow key={row.label} label={row.label} value={row.value} percentage={row.percentage} tone={row.tone} />
            ))}
          </div>
        </div>
        <div className={cashierClasses.chartContainer}>
          <h3 className={cashierClasses.sectionTitle}>用户消费排行</h3>
          <div className="grid gap-1">
            {spenderRows.map((row) => (
              <UserSpendingRow key={row.user} user={row.user} amount={row.amount} orders={row.orders} />
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function CashierConfigOverview({ data, onAddInstance }: { data: CashierData; onAddInstance: () => void }) {
  const providers = data.instances.items.slice(0, 4)
  const methodRows = data.methods.slice(0, 4)
  const riskMetrics = [
    { label: '订单同步频率', value: `${data.events.total ? 'Every 5m' : 'Manual'}` },
    { label: '单日支付上限', value: highestDailyLimit(data.instances.items) },
    { label: '异常订单阻断', value: data.overview.failed_webhook_count > 0 ? `${data.overview.failed_webhook_count} 待处理` : 'Enabled', positive: data.overview.failed_webhook_count === 0 },
  ]

  return (
    <div className="grid gap-6">
      <div className={cashierClasses.configPanelGrid}>
        <section className={cashierClasses.configPanel}>
          <div className={cashierClasses.configPanelHead}>
            <h3 className={cashierClasses.configPanelTitle}>支付通道管理</h3>
            <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={onAddInstance}>添加通道</button>
          </div>
          <div className={cashierClasses.providerList}>
            {providers.length ? providers.map((provider) => <ProviderItem key={provider.id} provider={provider} />) : <EmptyBlock title="暂无支付通道" detail="新增支付渠道实例后可在用户收银台调度。" />}
          </div>
        </section>
        <section className={cashierClasses.configPanel}>
          <div className={cashierClasses.configPanelHead}>
            <h3 className={cashierClasses.configPanelTitle}>收银台展示设置</h3>
          </div>
          <div className={cashierClasses.providerList}>
            <ToggleSetting title="允许自定义充值金额" detail="开启后用户可输入任意金额按比例兑换积分" enabled={Boolean(data.customAmount.enabled)} />
            {methodRows.map((method) => (
              <ToggleSetting key={method.method} title={method.label || cashierSupportedMethodLabel(method.method)} detail={`${method.method} · ${cashierProviderLabel(method.source_provider_type || cashierProviderTypesForMethod(method.method)[0])}`} enabled={method.enabled} />
            ))}
          </div>
        </section>
      </div>
      <section className={cashierClasses.riskPanel}>
        <div className={cashierClasses.configPanelHead}>
          <h3 className={cashierClasses.configPanelTitle}>风控与对账</h3>
        </div>
        <div className={cashierClasses.riskMetricGrid}>
          {riskMetrics.map((metric) => <RiskMetric key={metric.label} {...metric} />)}
        </div>
      </section>
    </div>
  )
}

function ProviderItem({ provider }: { provider: PaymentProviderInstance }) {
  const warning = provider.enabled && provider.config_status !== 'configured'
  return (
    <div className={cashierClasses.providerItem}>
      <div className="flex min-w-0 items-center gap-4">
        <div className={cn(cashierClasses.providerDot, warning ? 'bg-[var(--amber)]' : provider.enabled ? 'bg-[var(--green)]' : 'bg-[var(--muted)]')} />
        <div className="min-w-0">
          <div className={cashierClasses.providerName}>{provider.name}</div>
          <div className={cashierClasses.providerType}>{cashierProviderLabel(provider.provider_type)} · {cashierProviderSupportedMethodsLabel(provider.supported_methods)}</div>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        {warning ? <Badge tone="warning">需配置</Badge> : <StatusBadge badge={cashierEnabledBadge(provider.enabled)} />}
      </div>
    </div>
  )
}

function ToggleSetting({ title, detail, enabled }: { title: string; detail: string; enabled: boolean }) {
  return (
    <div className={cashierClasses.toggleSetting}>
      <div className="min-w-0">
        <div className={cashierClasses.providerName}>{title}</div>
        <div className={cashierClasses.providerType}>{detail}</div>
      </div>
      <div className={cn(cashierClasses.toggleSwitch, enabled ? 'bg-[var(--accent)]' : 'bg-[var(--border-strong)]')} aria-hidden="true">
        <div className={cn(cashierClasses.toggleKnob, enabled ? 'right-1' : 'left-1')} />
      </div>
    </div>
  )
}

function RiskMetric({ label, value, positive }: { label: string; value: string; positive?: boolean }) {
  return (
    <div className={cashierClasses.riskMetric}>
      <div className={cashierClasses.riskLabel}>{label}</div>
      <div className={cn(cashierClasses.riskValue, positive && 'text-[var(--green)]')}>{value}</div>
    </div>
  )
}

function CashierOverviewCards({ data }: { data: CashierData }) {
  return (
    <div className={cashierClasses.overviewGrid}>
      <FinancialStatCard label="今日订单" value={String(data.overview.today_order_count)} trend={`完成 ${data.overview.today_completed_count} 单`} />
      <FinancialStatCard label="到账金额" value={`¥ ${Number(data.overview.today_amount_cny).toFixed(2)}`} trend={`待支付 ${data.overview.pending_count} 单`} />
      <FinancialStatCard label="失败回调" value={String(data.overview.failed_webhook_count)} trend={`Mock ${cashierBooleanVisibilityLabel(data.overview.mock_enabled)}`} />
      <FinancialStatCard label="启用实例" value={String(data.overview.enabled_provider_instances)} trend={data.overview.enabled_methods?.length ? data.overview.enabled_methods.join(' / ') : '暂无启用支付方式'} />
    </div>
  )
}

function FinancialStatCard({ label, value, trend }: { label: string; value: string; trend: string }) {
  return (
    <div className={cashierClasses.overviewCard}>
      <span className={cashierClasses.overviewLabel}>{label}</span>
      <strong className={cashierClasses.overviewValue}>{value}</strong>
      <span className={cashierClasses.overviewTrend}>{trend}</span>
    </div>
  )
}

function DistributionRow({ label, value, percentage, tone }: { label: string; value: string; percentage: number; tone: string }) {
  return (
    <div className={cashierClasses.distributionRow}>
      <div className={cashierClasses.distributionMeta}>
        <span className="text-[var(--soft)]">{label}</span>
        <span className="text-[var(--text)]">{value} ({percentage}%)</span>
      </div>
      <div className={cashierClasses.distributionTrack}>
        <div className={cn('h-full', tone)} style={{ width: `${Math.max(4, percentage)}%` }} />
      </div>
    </div>
  )
}

function UserSpendingRow({ user, amount, orders }: { user: string; amount: string; orders: number }) {
  return (
    <div className={cashierClasses.spenderRow}>
      <div className="flex min-w-0 items-center gap-3">
        <div className={cashierClasses.spenderAvatar}>{user.slice(0, 2).toUpperCase()}</div>
        <div className="min-w-0">
          <strong className="block truncate text-sm">{user}</strong>
          <span className="text-xs text-[var(--muted-strong)]">{orders} 笔订单</span>
        </div>
      </div>
      <strong className="text-sm font-semibold text-[var(--green)]">{amount}</strong>
    </div>
  )
}

function revenueBarHeights(orders: PaymentOrder[]) {
  const buckets = Array.from({ length: 30 }, (_, index) => 12 + ((index * 17) % 68))
  orders.slice(0, 30).forEach((order, index) => {
    buckets[29 - index] = Math.min(94, Math.max(12, Number(order.amount_cny ?? '0') * 3))
  })
  return buckets
}

function paymentDistributionRows(orders: PaymentOrder[], enabledMethods: string[]) {
  const totals = new Map<string, number>()
  orders.forEach((order) => {
    const key = order.visible_method || order.provider || 'unknown'
    totals.set(key, (totals.get(key) ?? 0) + Number(order.amount_cny ?? '0'))
  })
  enabledMethods.forEach((method) => {
    if (!totals.has(method)) totals.set(method, 0)
  })
  const total = Array.from(totals.values()).reduce((sum, item) => sum + item, 0)
  const tones = ['bg-[var(--accent)]', 'bg-[var(--green)]', 'bg-[var(--amber)]', 'bg-[var(--muted)]']
  const entries = Array.from(totals.entries()).slice(0, 4)
  if (!entries.length) entries.push(['暂无支付方式', 0])
  return entries.map(([method, amount], index) => ({
    label: cashierSupportedMethodLabel(method),
    value: `¥ ${amount.toFixed(2)}`,
    percentage: total > 0 ? Math.round((amount / total) * 100) : 0,
    tone: tones[index % tones.length],
  }))
}

function topSpenderRows(orders: PaymentOrder[]) {
  const totals = new Map<string, { amount: number; orders: number }>()
  orders.forEach((order) => {
    const key = order.user_id ? `User #${order.user_id}` : 'Unknown User'
    const current = totals.get(key) ?? { amount: 0, orders: 0 }
    totals.set(key, { amount: current.amount + Number(order.amount_cny ?? '0'), orders: current.orders + 1 })
  })
  const rows = Array.from(totals.entries())
    .sort((left, right) => right[1].amount - left[1].amount)
    .slice(0, 4)
    .map(([user, row]) => ({ user, amount: `¥ ${row.amount.toFixed(2)}`, orders: row.orders }))
  return rows.length ? rows : [{ user: 'No Orders', amount: '¥ 0.00', orders: 0 }]
}

function highestDailyLimit(instances: PaymentProviderInstance[]) {
  const limits = instances
    .map((instance) => Number(instance.limits?.daily_amount_limit_cny ?? 0))
    .filter((value) => Number.isFinite(value) && value > 0)
  if (!limits.length) return '未设置'
  return `¥ ${Math.max(...limits).toFixed(2)}`
}

function ProviderConfigGuide({ providerType }: { providerType: PaymentProviderType }) {
  const guide = cashierProviderConfigGuide(providerType)
  return (
    <div className={cashierClasses.providerGuide}>
      <div>
        <strong>{guide.title}</strong>
        <p>{guide.detail}</p>
        <p>{guide.secretHint}</p>
      </div>
      <div className={cashierClasses.providerGuideFields}>
        <span>必填：{guide.requiredFields.length ? guide.requiredFields.join(' / ') : '按渠道文档填写'}</span>
        {guide.optionalFields.length ? <span>可选：{guide.optionalFields.join(' / ')}</span> : null}
      </div>
    </div>
  )
}

function ProviderStructuredConfigFields({ draft, onChange }: {
  draft: CashierProviderFormDraft
  onChange: (draft: CashierProviderFormDraft) => void
}) {
  const fields = cashierProviderConfigFields(draft.provider_type)
  if (!fields.length) return null
  const updateField = (field: CashierProviderConfigField, value: string) => {
    if (field.kind === 'callback-base') {
      onChange({ ...draft, callback_bases: { ...draft.callback_bases, [field.key]: value } })
      return
    }
    if (field.storage === 'secret') {
      onChange({ ...draft, secrets: { ...draft.secrets, [field.key]: value } })
      return
    }
    onChange({ ...draft, config: updateProviderConfigField(draft.config, field, value) })
  }
  const fieldValue = (field: CashierProviderConfigField) => {
    if (field.kind === 'callback-base') return draft.callback_bases[field.key as 'notify_url' | 'return_url'] ?? ''
    return stringFromRecord(field.storage === 'secret' ? draft.secrets : draft.config, field.key)
  }
  return (
    <section className={cashierClasses.structuredConfig}>
      <div>
        <strong>渠道字段配置</strong>
        <p>标记 * 的字段为必填。密钥只用于新增或轮换，编辑时留空会保留已保存的密钥。</p>
        {draft.row?.credentials_status ? <Badge tone={draft.row.credentials_status.has_secret ? 'success' : 'warning'}>{draft.row.credentials_status.has_secret ? '已保存密钥' : '未保存密钥'}</Badge> : null}
      </div>
      <div className={adminPage.formGrid}>
        {fields.map((field) => (
          <Field key={field.key} label={`${field.label} ${field.required ? '*' : '（选填）'}`} hint={field.hint}>
            {field.kind === 'select' ? (
              <select value={fieldValue(field)} required={field.required} aria-required={field.required} onChange={(event) => updateField(field, event.target.value)}>
                {field.options?.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            ) : field.kind === 'textarea' ? (
              <textarea
                className={cashierClasses.textarea}
                value={fieldValue(field)}
                onChange={(event) => updateField(field, event.target.value)}
                rows={4}
                placeholder={field.placeholder}
                spellCheck={false}
                required={field.required}
                aria-required={field.required}
              />
            ) : (
              <input
                type={field.kind === 'password' ? 'password' : 'text'}
                value={fieldValue(field)}
                onChange={(event) => updateField(field, event.target.value)}
                placeholder={field.placeholder}
                required={field.required}
                aria-required={field.required}
                autoComplete={field.kind === 'password' ? 'new-password' : undefined}
              />
            )}
          </Field>
        ))}
      </div>
    </section>
  )
}

function updateProviderConfigField(config: Record<string, unknown>, field: CashierProviderConfigField, rawValue: string): Record<string, unknown> {
  const next = { ...config }
  const value = rawValue.trim()
  if (!value) {
    delete next[field.key]
  } else if (field.key === 'channel_extra') {
    next[field.key] = rawValue
  } else if (value === 'true' || value === 'false') {
    next[field.key] = value === 'true'
  } else {
    next[field.key] = rawValue
  }
  return next
}

function CashierSection({ title, children, plain = false, actions }: { title: string; children: ReactNode; plain?: boolean; actions?: ReactNode }) {
  return (
    <section className={plain ? 'grid gap-6' : cashierClasses.section}>
      <div className={plain ? 'flex flex-wrap items-center justify-between gap-3' : cashierClasses.sectionHead}>
        <strong className={plain ? cashierClasses.sectionTitle : undefined}>{title}</strong>
        {actions ? <div className={cashierClasses.actions}>{actions}</div> : null}
      </div>
      {children}
    </section>
  )
}

function CashierPager({ page, pageSize, total, onPrev, onNext, onRefresh }: {
  page: number
  pageSize: number
  total?: number
  onPrev: () => void
  onNext: () => void
  onRefresh: () => void
}) {
  const safeTotal = Math.max(0, Number(total ?? 0))
  const start = safeTotal === 0 ? 0 : (page - 1) * pageSize + 1
  const end = safeTotal === 0 ? 0 : Math.min(safeTotal, page * pageSize)
  const canPrev = page > 1
  const canNext = safeTotal === 0 ? false : page * pageSize < safeTotal
  return (
    <div className={cashierClasses.pager}>
      <span>第 {page} 页 · {start}-{end} / {safeTotal}</span>
      <div className={cashierClasses.actions}>
        <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={onRefresh}>刷新</button>
        <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} disabled={!canPrev} onClick={onPrev}>上一页</button>
        <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} disabled={!canNext} onClick={onNext}>下一页</button>
      </div>
    </div>
  )
}

function DetailItem({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className={cashierClasses.detailItem}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  )
}

function StatusBadge({ badge }: { badge: CashierStatusBadge }) {
  return <Badge tone={badge.tone}>{badge.label}</Badge>
}

function CashierRiskPanel({ rows }: { rows: CashierRiskRow[] }) {
  return (
    <div className={cashierClasses.riskGrid}>
      {rows.map((row) => (
        <div key={row.key} className={cn(cashierClasses.riskItem, cashierClasses.riskTone[row.tone])}>
          <span>{row.label}</span>
          <strong>{row.value}</strong>
          <p>{row.detail}</p>
        </div>
      ))}
    </div>
  )
}

function WebhookEventAction({ event, retrying, onRetry }: { event: PaymentWebhookEvent; retrying: boolean; onRetry: (event: PaymentWebhookEvent) => void }) {
  const action = cashierWebhookEventAction(event)
  if (!action.canRetry) {
    return <span className={adminPage.mutedAction} title={action.title}>{action.label}</span>
  }
  return (
    <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} disabled={retrying} title={action.title} onClick={() => void onRetry(event)}>
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

function stringFromRecord(record: Record<string, unknown>, key: string) {
  const value = record[key]
  if (value === undefined || value === null) return ''
  if (isPlainRecord(value) || Array.isArray(value)) return JSON.stringify(value, null, 2)
  return String(value)
}

function normalizePaymentVisibleMethods(items: PaymentVisibleMethod[]): PaymentVisibleMethod[] {
  return items.map((item, index) => {
    const method = item.method.trim().toLowerCase()
    const compatibleProviders = cashierProviderTypesForMethod(method)
    const sourceProviderType = compatibleProviders.includes(item.source_provider_type ?? '') ? item.source_provider_type : compatibleProviders[0]
    return {
      ...item,
      method,
      label: item.label.trim() || cashierSupportedMethodLabel(method),
      source_provider_type: sourceProviderType,
      scheduler_strategy: item.scheduler_strategy || 'round_robin',
      display_order: Number(item.display_order) > 0 ? Number(item.display_order) : (index + 1) * 10,
    }
  })
}

function validatePaymentVisibleMethods(items: PaymentVisibleMethod[]) {
  const seen = new Set<string>()
  for (const item of items) {
    const method = item.method.trim().toLowerCase()
    if (!method) return '支付方式代码不能为空'
    if (seen.has(method)) return `支付方式代码重复：${method}`
    seen.add(method)
    if (!item.label.trim()) return `支付方式 ${method} 的展示名称不能为空`
    const scheduler = item.scheduler_strategy || 'round_robin'
    if (scheduler !== 'round_robin' && scheduler !== 'random') return `支付方式 ${method} 的调度策略不支持`
    const providerType = item.source_provider_type || cashierProviderTypesForMethod(method)[0]
    if (!cashierProviderTypesForMethod(method).includes(providerType)) {
      return `支付方式 ${method} 不支持渠道类型 ${cashierProviderLabel(providerType)}`
    }
  }
  return ''
}

function newVisibleMethodDraft(current: PaymentVisibleMethod[]): PaymentVisibleMethod {
  const method = commonVisibleMethodOptions.find((candidate) => !current.some((item) => item.method === candidate)) ?? `custom_${current.length + 1}`
  return {
    method,
    label: cashierSupportedMethodLabel(method),
    enabled: true,
    source_provider_type: cashierProviderTypesForMethod(method)[0],
    scheduler_strategy: 'round_robin',
    display_order: (current.length + 1) * 10,
    description: '',
  }
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
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

function PlanActionIcon({ action }: { action: CashierPlanTransitionAction }) {
  if (action === 'enable') return <Play />
  if (action === 'disable') return <Pause />
  if (action === 'restore') return <RotateCcw />
  return <Archive />
}

function cashierOrderQuery(page: number, filters: OrderFilters, pageSize: number = cashierAdminPageSize) {
  return {
    page,
    page_size: pageSize,
    order_no: filters.order_no.trim() || undefined,
    user_id: filters.user_id.trim() || undefined,
    status: filters.status || undefined,
    visible_method: filters.visible_method || undefined,
    purchase_type: filters.purchase_type || undefined,
  }
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
