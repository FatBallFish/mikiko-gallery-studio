import type { CashierPlan } from '../../../shared/api-types'

export function checkoutPurchasablePlans(plans: CashierPlan[]): CashierPlan[] {
  return plans.filter((plan) => {
    const planType = plan.plan_type ?? 'points_package'
    const status = plan.status ?? 'active'
    return planType === 'points_package' && plan.purchase_enabled !== false && status !== 'disabled' && status !== 'archived'
  })
}
