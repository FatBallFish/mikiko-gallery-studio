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

for (const packageContract of ['adminApi.listCashierPlans', "onFeedback('套餐编辑入口'", 'cashierPlanStatusBadge', 'cashierPlanPurchaseBadge']) {
  if (!packagesSource.includes(packageContract)) {
    throw new Error(`packages redesign must preserve ${packageContract}`)
  }
}

for (const drift of ['adminSurface.card', 'rounded-2xl', 'rounded-3xl', 'uppercase', 'tracking-[', 'text-emerald-', 'text-green-']) {
  if (ordersSource.includes(drift) || packagesSource.includes(drift)) {
    throw new Error(`orders and packages pages must remove visual drift: ${drift}`)
  }
}
