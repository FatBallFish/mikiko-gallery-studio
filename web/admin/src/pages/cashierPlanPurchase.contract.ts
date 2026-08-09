import type { CashierPlan } from '../../../shared/api-types'
import { cashierPlanActions, cashierPlanEmptyState, cashierPlanFilterOptions, cashierPlanPurchaseBadge, cashierPlanSavePayload, cashierPlanSectionCopy } from './cashierPlanPurchase'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'

const cashierPageSource = readFileSync(new URL('./CashierPage.tsx', import.meta.url), 'utf8')

const subscriptionPayload = cashierPlanSavePayload({
  plan_code: 'sub-monthly',
  plan_name: '订阅占位',
  plan_type: 'subscription',
  purchase_enabled: true,
  status: 'active',
  price_cny: '59.90000',
  points: '500.00000',
  bonus_points: '0.00000',
  credit_expiry_enabled: true,
  duration_days: '30',
  sort_order: '10',
  description: '保留订阅定义但不开放购买',
})

if (subscriptionPayload.purchase_enabled !== false) {
  throw new Error(`subscription placeholders must be saved hidden even when toggled on, got ${JSON.stringify(subscriptionPayload)}`)
}

const pointsPayload = cashierPlanSavePayload({
  plan_code: 'points-100',
  plan_name: '100 积分包',
  plan_type: 'points_package',
  purchase_enabled: true,
  status: 'active',
  price_cny: '19.90000',
  points: '100.00000',
  bonus_points: '0.00000',
  credit_expiry_enabled: false,
  duration_days: '30',
  sort_order: '5',
  description: '可购买积分包',
})

if (pointsPayload.purchase_enabled !== true || pointsPayload.sort_order !== 5 || pointsPayload.credit_expiry_enabled !== false || pointsPayload.duration_days !== null || 'currency' in pointsPayload) {
  throw new Error(`points package should preserve purchasable payload fields, got ${JSON.stringify(pointsPayload)}`)
}

const subscriptionPlan = {
  id: 1,
  plan_code: 'sub-monthly',
  plan_name: '订阅占位',
  plan_type: 'subscription',
  purchase_enabled: true,
  status: 'active',
  price_cny: '59.90000',
  points: '500.00000',
  bonus_points: '0.00000',
  duration_days: 30,
  currency: 'CNY',
  created_at: '2026-06-05T00:00:00Z',
  updated_at: '2026-06-05T00:00:00Z',
} satisfies CashierPlan

const hiddenBadge = cashierPlanPurchaseBadge(subscriptionPlan)

if (hiddenBadge.label !== '未开放' || hiddenBadge.tone !== 'warning') {
  throw new Error(`subscription placeholder badge should be hidden regardless of raw purchase flag, got ${JSON.stringify(hiddenBadge)}`)
}

const enabledBadge = cashierPlanPurchaseBadge(pointsPayload as CashierPlan)
if (enabledBadge.label !== '开放购买' || enabledBadge.tone !== 'success') {
  throw new Error(`points package purchase badge should be localized as open, got ${JSON.stringify(enabledBadge)}`)
}

if (cashierPlanEmptyState.title !== '暂无积分包' || !cashierPlanEmptyState.detail.includes('新增套餐') || !cashierPlanEmptyState.detail.includes('固定积分包')) {
  throw new Error(`cashier plan empty state should give the current create action, got ${JSON.stringify(cashierPlanEmptyState)}`)
}

if (/后续|暂未|即将|版本|points_package|subscription/.test(`${cashierPlanEmptyState.title}${cashierPlanEmptyState.detail}`)) {
  throw new Error(`cashier plan empty state should not use weak roadmap wording, got ${JSON.stringify(cashierPlanEmptyState)}`)
}

const visiblePlanCopy = [
  cashierPlanSectionCopy.toolbarDetail,
  cashierPlanSectionCopy.dialogDetail,
  cashierPlanSectionCopy.subscriptionOptionLabel,
].join(' ')

if (!visiblePlanCopy.includes('订阅套餐') || !visiblePlanCopy.includes('不在用户端开放购买')) {
  throw new Error(`cashier plan copy should explain subscription plans are retained but hidden from users, got ${visiblePlanCopy}`)
}

if (/占位|placeholder/i.test(visiblePlanCopy)) {
  throw new Error(`cashier plan admin copy should avoid internal placeholder wording, got ${visiblePlanCopy}`)
}

if (cashierPlanFilterOptions.map((option) => option.value).join(',') !== 'active,disabled,archived') {
  throw new Error(`plan filters must expose active, disabled, and archived states, got ${JSON.stringify(cashierPlanFilterOptions)}`)
}

const activeActions = cashierPlanActions({ ...subscriptionPlan, plan_type: 'points_package', purchase_enabled: true, status: 'active' })
if (activeActions.map((action) => action.action).join(',') !== 'disable,archive') {
  throw new Error(`active plan actions must disable or archive, got ${JSON.stringify(activeActions)}`)
}
const disabledActions = cashierPlanActions({ ...subscriptionPlan, plan_type: 'points_package', purchase_enabled: false, status: 'disabled' })
if (disabledActions.map((action) => action.action).join(',') !== 'enable,archive') {
  throw new Error(`disabled plan actions must enable or archive, got ${JSON.stringify(disabledActions)}`)
}
const archivedActions = cashierPlanActions({ ...subscriptionPlan, plan_type: 'points_package', purchase_enabled: false, status: 'archived' })
if (archivedActions.map((action) => action.action).join(',') !== 'restore') {
  throw new Error(`archived plan actions must only restore, got ${JSON.stringify(archivedActions)}`)
}

for (const lifecycleContract of ['cashierPlanActions(plan).map', '<TooltipIconButton', 'setPlanTransition({ plan, action })', 'planTransition ? <Modal', 'disabled={savingPlan}', "savingPlan ? '处理中...' : '确认'"]) {
  if (!cashierPageSource.includes(lifecycleContract)) {
    throw new Error(`plan lifecycle must retain visible controls, confirmation and loading state with ${lifecycleContract}`)
  }
}
