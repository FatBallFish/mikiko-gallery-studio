import type { CashierPlan } from '../../../shared/api-types'
import { checkoutPlanValidityLabel, checkoutPurchasablePlans } from './checkoutPlans'

function plan(input: Partial<CashierPlan> & Pick<CashierPlan, 'id' | 'plan_code' | 'plan_name'>): CashierPlan {
  return {
    status: 'active',
    price_cny: '19.90000',
    points: '100.00000',
    bonus_points: '0.00000',
    duration_days: 30,
    currency: 'CNY',
    created_at: '2026-06-05T00:00:00Z',
    updated_at: '2026-06-05T00:00:00Z',
    plan_type: 'points_package',
    purchase_enabled: true,
    sort_order: 10,
    ...input,
  }
}

const plans = checkoutPurchasablePlans([
  plan({ id: 1, plan_code: 'points-100', plan_name: '100 积分包', sort_order: 20 }),
  plan({ id: 2, plan_code: 'subscription-monthly', plan_name: '月度订阅', plan_type: 'subscription' }),
  plan({ id: 3, plan_code: 'points-disabled', plan_name: '停用积分包', status: 'disabled' }),
  plan({ id: 4, plan_code: 'points-hidden', plan_name: '隐藏积分包', purchase_enabled: false }),
])

if (plans.length !== 1 || plans[0]?.plan_code !== 'points-100') {
  throw new Error(`checkout should expose only purchasable points packages, got ${JSON.stringify(plans)}`)
}

const permanent = checkoutPlanValidityLabel(plan({
  id: 5,
  plan_code: 'points-permanent',
  plan_name: '永久积分包',
  credit_expiry_enabled: false,
  duration_days: undefined,
}))
if (permanent !== '永久有效') {
  throw new Error(`permanent package validity should be explicit, got ${permanent}`)
}

const expiring = checkoutPlanValidityLabel(plan({
  id: 6,
  plan_code: 'points-expiring',
  plan_name: '限时积分包',
  credit_expiry_enabled: true,
  duration_days: 45,
}))
if (expiring !== '45 天有效') {
  throw new Error(`expiring package validity should be explicit, got ${expiring}`)
}
