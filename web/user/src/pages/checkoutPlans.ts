import type { CashierPlan } from '../../../shared/api-types'

export function checkoutPurchasablePlans(plans: CashierPlan[]): CashierPlan[] {
  return plans.filter((plan) => {
    const planType = plan.plan_type ?? 'points_package'
    const status = plan.status ?? 'active'
    return planType === 'points_package' && plan.purchase_enabled !== false && status !== 'disabled' && status !== 'archived'
  })
}

export function checkoutPlanValidityLabel(plan: CashierPlan): string {
  if (plan.credit_expiry_enabled === false) return '永久有效'
  const configuredDays = Number(plan.duration_days)
  const days = Number.isInteger(configuredDays) && configuredDays > 0 ? configuredDays : 30
  return `${days} 天有效`
}
