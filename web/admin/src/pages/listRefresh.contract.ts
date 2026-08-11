// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
import { createLatestListRequestGuard } from './listRefresh'

const guard = createLatestListRequestGuard()
const first = guard.begin()
const second = guard.begin()
if (guard.isCurrent(first) || !guard.isCurrent(second)) throw new Error('only the latest list request may commit results')
guard.invalidate()
if (guard.isCurrent(second)) throw new Error('unmounted list requests must be invalidated')

const components = readFileSync(new URL('../components.tsx', import.meta.url), 'utf8')
for (const marker of ['export function RefreshIconButton', '<RefreshCw', 'aria-label={label}', 'animate-spin']) {
  if (!components.includes(marker)) throw new Error(`shared refresh control must implement ${marker}`)
}

const pages = [
  'OverviewPage', 'MonitoringPage', 'ClusterPage', 'UsersPage', 'UserGroupsPage', 'ReviewPage',
  'RedeemPage', 'OrdersPage', 'PackagesPage', 'CashierPage', 'ProviderModelsPage', 'RoutingPage',
  'PricingPage', 'CallRecordsPage', 'SystemUsersPage', 'AuditPage', 'SystemSettingsPage',
]
for (const page of pages) {
  const source = readFileSync(new URL(`./${page}.tsx`, import.meta.url), 'utf8')
  if (!source.includes('<RefreshIconButton')) throw new Error(`${page} must expose the shared icon refresh control`)
  if (page !== 'SystemSettingsPage' && ![
    'createLatestListRequestGuard',
    'requestGenerationRef',
    'requestSequence',
  ].some((marker) => source.includes(marker))) {
    throw new Error(`${page} must prevent older list requests from overwriting newer refresh results`)
  }
}

for (const page of ['UsersPage', 'ReviewPage', 'CallRecordsPage', 'SystemUsersPage']) {
  const source = readFileSync(new URL(`./${page}.tsx`, import.meta.url), 'utf8')
  if (!source.includes('reloadKey') || !source.includes('setReloadKey')) {
    throw new Error(`${page} must issue a request when unchanged filters are submitted again`)
  }
}
