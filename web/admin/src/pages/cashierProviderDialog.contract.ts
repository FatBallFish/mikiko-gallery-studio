// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { fileURLToPath } from 'node:url'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { dirname, join } from 'node:path'
const pageDirectory = dirname(fileURLToPath(import.meta.url))
const componentsSource = readFileSync(join(pageDirectory, '..', 'components.tsx'), 'utf8')

if (!componentsSource.includes('export function Drawer')) {
  throw new Error('Drawer should be exported for complex admin configuration flows')
}

const source = readFileSync(join(pageDirectory, 'CashierPage.tsx'), 'utf8')

for (const primitive of ['<PageHeader', '<AdminTabs', '<InlineFeedback', '<Drawer', '<Modal', '<LoadingBlock', '<ErrorBlock', '<EmptyBlock']) {
  if (!source.includes(primitive)) {
    throw new Error(`cashier page should use the shared ${primitive.slice(1)} primitive`)
  }
}

for (const tab of ["id: 'overview'", "id: 'methods'", "id: 'instances'", "id: 'plans'", "id: 'orders'", "id: 'events'"]) {
  if (!source.includes(tab)) {
    throw new Error(`cashier page should preserve the ${tab} view`)
  }
}

for (const drift of ['rounded-2xl', 'rounded-3xl', 'tracking-[', 'tracking-tighter', 'tracking-wider', 'tracking-widest', 'text-[10px]', 'text-emerald-', 'bg-blue-500', 'bg-emerald-500', 'bg-purple-500', 'bg-amber-500', 'var(--yellow)', 'duration-180', 'rgba(']) {
  if (source.includes(drift)) {
    throw new Error(`cashier page should remove visual drift token ${drift}`)
  }
}

if (/\buppercase\b/.test(source)) {
  throw new Error('cashier page should not use decorative uppercase typography')
}

if (!source.includes('if (error && !data) return <ErrorBlock')) {
  throw new Error('cashier page should reserve the blocking error state for initial loading failures')
}

for (const planStateContract of ['cashierPlanStatusBadge(plan.status)', 'cashierPlanPurchaseBadge(plan)']) {
  if (!source.includes(planStateContract)) {
    throw new Error(`cashier plan cards must preserve operational and purchase state via ${planStateContract}`)
  }
}

const providerDialogStart = source.indexOf('{instanceDialog ? (')
const providerDialogEnd = source.indexOf('{orderDetail ? (')
const providerDialogSource = source.slice(providerDialogStart, providerDialogEnd)

if (!providerDialogSource.includes('<Drawer')) {
  throw new Error('payment provider instance flow should use Drawer instead of a long Modal')
}

if (providerDialogSource.includes('<Modal')) {
  throw new Error('payment provider instance flow must not render Modal')
}
