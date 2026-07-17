// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'

const systemUsersPageSource = readFileSync(new URL('./SystemUsersPage.tsx', import.meta.url), 'utf8')

for (const primitive of ['PageHeader', 'MetricStrip', 'FilterToolbar', 'ListPage', 'DataTable', 'Badge', 'ActionMenu', 'InlineFeedback', 'Modal', 'Pager']) {
  if (!systemUsersPageSource.includes(`<${primitive}`)) {
    throw new Error(`system account list must use the shared ${primitive} primitive`)
  }
}

for (const operation of ['systemAdmins.list', 'systemAdmins.create', 'systemAdmins.update', 'systemAdmins.resetPassword', 'systemAdmins.delete']) {
  if (!systemUsersPageSource.includes(operation)) {
    throw new Error(`system account list must preserve ${operation}`)
  }
}

for (const stateContract of ['mutationError', 'resultSummary=', 'confirmEmail', 'isSelf']) {
  if (!systemUsersPageSource.includes(stateContract)) {
    throw new Error(`system account list must expose ${stateContract}`)
  }
}

for (const legacyPattern of ['<FilterBar', '<StatusStrip', 'rounded-xl', 'tracking-tight', 'tracking-[', 'uppercase', 'rgba(184,135,64']) {
  if (systemUsersPageSource.includes(legacyPattern)) {
    throw new Error(`system account list must remove legacy page-local pattern ${legacyPattern}`)
  }
}
