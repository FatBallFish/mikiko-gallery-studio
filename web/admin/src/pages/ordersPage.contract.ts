// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'

const ordersSource = readFileSync(new URL('./OrdersPage.tsx', import.meta.url), 'utf8')
const packagesSource = readFileSync(new URL('./PackagesPage.tsx', import.meta.url), 'utf8')

for (const [name, source] of [['OrdersPage', ordersSource], ['PackagesPage', packagesSource]] as const) {
  if (!source.includes(`export function ${name}`)) {
    throw new Error(`${name} should be exported as an independent admin page component`)
  }
  for (const primitive of ['PageHeader', 'FilterToolbar', 'DataTable', 'Badge']) {
    if (!source.includes(`<${primitive}`)) {
      throw new Error(`${name} must use the shared ${primitive} primitive`)
    }
  }
}

for (const apiContract of ['adminApi.listPaymentOrders', 'page_size: nextPageize', 'query: query.trim()', 'status: status || undefined', '<Pager']) {
  if (!ordersSource.includes(apiContract)) {
    throw new Error(`orders redesign must preserve ${apiContract}`)
  }
}

for (const orderProjectionContract of ['user_email', 'user_nickname', 'bonus_points', 'total_points', "value: 'expired'"]) {
  if (!ordersSource.includes(orderProjectionContract)) {
    throw new Error(`orders page must render admin projection ${orderProjectionContract}`)
  }
}

for (const packageContract of ['adminApi.listCashierPlans', 'adminApi.createCashierPlan', 'adminApi.updateCashierPlan', '<CashierPlanEditorDialog', 'cashierPlanStatusBadge', 'cashierPlanPurchaseBadge']) {
  if (!packagesSource.includes(packageContract)) {
    throw new Error(`packages redesign must preserve ${packageContract}`)
  }
}

for (const packageManagementContract of [
  'query: query.trim() || undefined',
  'plan_type: planType || undefined',
  'sort_by: sortBy',
  'sort_order: sortOrder',
  'cashierPlanActions(plan).map',
  'adminApi.transitionCashierPlan',
  '<TooltipIconButton',
  '<Pager',
]) {
  if (!packagesSource.includes(packageManagementContract)) {
    throw new Error(`packages management must expose ${packageManagementContract}`)
  }
}

if (packagesSource.includes("onFeedback('套餐编辑入口'")) {
  throw new Error('PackagesPage must open the real editor instead of showing placeholder feedback')
}

for (const drift of ['adminSurface.card', 'rounded-2xl', 'rounded-3xl', 'uppercase', 'tracking-[', 'text-emerald-', 'text-green-']) {
  if (ordersSource.includes(drift) || packagesSource.includes(drift)) {
    throw new Error(`orders and packages pages must remove visual drift: ${drift}`)
  }
}
