import type { CashierPlan, CashierPlanStatus, CashierPlanTransitionAction } from '../../../shared/api-types'

export type CashierPlanDraftInput = {
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

export type CashierPlanPurchaseBadge = {
  label: '开放购买' | '未开放'
  tone: 'success' | 'warning'
}

export const cashierPlanEmptyState = {
  title: '暂无积分包',
  detail: '点击“新增套餐”创建固定积分包，启用购买后会出现在用户收银台。',
} as const

export const cashierPlanSectionCopy = {
  toolbarDetail: '订阅套餐定义会保留在后台；只有启用状态的积分包且开放购买时，才会出现在用户收银台。',
  dialogDetail: '积分包可直接开放购买；订阅套餐仅在后台保留定义，不在用户端开放购买。',
  subscriptionOptionLabel: '订阅套餐',
} as const

export const cashierPlanFilterOptions: ReadonlyArray<{ value: CashierPlanStatus; label: string }> = [
  { value: 'active', label: '启用' },
  { value: 'disabled', label: '停用' },
  { value: 'archived', label: '已删除' },
]

export type CashierPlanAction = {
  action: CashierPlanTransitionAction
  label: string
  detail: string
  tone: 'neutral' | 'warning' | 'danger'
}

export function cashierPlanActions(plan: CashierPlan): CashierPlanAction[] {
  if (plan.status === 'archived') {
    return [{ action: 'restore', label: '恢复套餐', detail: `恢复「${plan.plan_name}」后将处于停用状态，需要再次启用才会开放购买。`, tone: 'neutral' }]
  }
  const availability: CashierPlanAction = plan.status === 'active'
    ? { action: 'disable', label: '停用套餐', detail: `停用「${plan.plan_name}」后，用户将无法创建新订单。`, tone: 'warning' }
    : { action: 'enable', label: '启用套餐', detail: `启用「${plan.plan_name}」后，积分包会重新开放购买。`, tone: 'neutral' }
  return [availability, { action: 'archive', label: '删除套餐', detail: `删除「${plan.plan_name}」后将默认隐藏，历史订单和积分发放不受影响。`, tone: 'danger' }]
}

export function cashierPlanPurchaseEnabled(planType: string | undefined, purchaseEnabled: boolean): boolean {
  return (planType ?? 'points_package') === 'points_package' && purchaseEnabled
}

export function cashierPlanSavePayload(draft: CashierPlanDraftInput): Partial<CashierPlan> {
  const planType = draft.plan_type || 'points_package'
  return {
    plan_code: draft.plan_code,
    plan_name: draft.plan_name,
    plan_type: planType,
    purchase_enabled: cashierPlanPurchaseEnabled(planType, draft.purchase_enabled),
    status: draft.status,
    price_cny: draft.price_cny,
    points: draft.points,
    bonus_points: draft.bonus_points,
    duration_days: Number(draft.duration_days) || 30,
    currency: draft.currency,
    sort_order: Number(draft.sort_order) || 0,
    description: draft.description,
  }
}

export function cashierPlanPurchaseBadge(plan: Pick<CashierPlan, 'plan_type' | 'purchase_enabled' | 'status'>): CashierPlanPurchaseBadge {
  const enabled = (plan.plan_type ?? 'points_package') === 'points_package' && plan.purchase_enabled === true && plan.status === 'active'
  return enabled ? { label: '开放购买', tone: 'success' } : { label: '未开放', tone: 'warning' }
}
