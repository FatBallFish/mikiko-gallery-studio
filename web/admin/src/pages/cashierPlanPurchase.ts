import type { CashierPlan } from '../../../shared/api-types'
import { normalizeAdminCurrency } from '../ui/currency'

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
    currency: normalizeAdminCurrency(draft.currency),
    sort_order: Number(draft.sort_order) || 0,
    description: draft.description,
  }
}

export function cashierPlanPurchaseBadge(plan: Pick<CashierPlan, 'plan_type' | 'purchase_enabled' | 'status'>): CashierPlanPurchaseBadge {
  const enabled = (plan.plan_type ?? 'points_package') === 'points_package' && plan.purchase_enabled === true && plan.status === 'active'
  return enabled ? { label: '开放购买', tone: 'success' } : { label: '未开放', tone: 'warning' }
}
