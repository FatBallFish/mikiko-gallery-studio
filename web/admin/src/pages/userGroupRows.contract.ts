import type { UserGroup } from '../../../shared/api-types'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
import { userGroupRows, userGroupStatusLabel, userGroupStatusTone, userGroupSummary } from './userGroupRows'

const userGroupsPageSource = readFileSync(new URL('./UserGroupsPage.tsx', import.meta.url), 'utf8')

for (const primitive of ['PageHeader', 'MetricStrip', 'FilterToolbar', 'ListPage', 'DataTable', 'Badge', 'ActionMenu', 'InlineFeedback', 'Modal']) {
  if (!userGroupsPageSource.includes(`<${primitive}`)) {
    throw new Error(`user group list must use the shared ${primitive} primitive`)
  }
}

for (const operation of ['adminApi.listUserGroups', 'adminApi.createUserGroup', 'adminApi.updateUserGroup', 'adminApi.deleteUserGroup']) {
  if (!userGroupsPageSource.includes(operation)) {
    throw new Error(`user group list must preserve ${operation}`)
  }
}

for (const stateContract of ['mutationError', 'resultSummary=', 'userGroupStatusTone', 'userGroupSummary']) {
  if (!userGroupsPageSource.includes(stateContract)) {
    throw new Error(`user group list must expose ${stateContract}`)
  }
}

for (const legacyPattern of ['<details', 'rounded-xl', 'tracking-[', 'uppercase', 'groupClasses.summary']) {
  if (userGroupsPageSource.includes(legacyPattern)) {
    throw new Error(`user group list must remove legacy page-local pattern ${legacyPattern}`)
  }
}

const group = (override: Partial<UserGroup>): UserGroup => ({
  id: override.code ?? 'basic',
  code: 'basic',
  name: '基础分组',
  group_code: 'basic',
  group_name: '基础分组',
  multiplier: '1.00000',
  status: 'enabled',
  sort_order: 10,
  is_default: false,
  description: null,
  created_at: '2026-06-05T00:00:00Z',
  updated_at: '2026-06-05T00:00:00Z',
  ...override,
})

const groups = [
  group({ code: 'basic', name: '基础分组', status: 'enabled', is_default: true, multiplier: '1.00000' }),
  group({ code: 'legacy', name: '兼容分组', status: 'active', multiplier: '1.25000' }),
  group({ code: 'disabled', name: '停用分组', status: 'disabled', multiplier: '0.90000', description: '历史活动' }),
  group({ code: 'future', name: '未来分组', status: 'paused', multiplier: '1.50000' }),
]

const rows = userGroupRows(groups)

if (rows[0]?.status !== 'enabled' || rows[0]?.statusLabel !== '启用' || rows[0]?.statusTone !== 'success') {
  throw new Error(`enabled user group should preserve raw status and show localized label, got ${JSON.stringify(rows[0])}`)
}

if (rows[1]?.status !== 'active' || rows[1]?.statusLabel !== '启用' || rows[1]?.statusTone !== 'success') {
  throw new Error(`legacy active user group should be treated as enabled, got ${JSON.stringify(rows[1])}`)
}

if (rows[2]?.statusLabel !== '停用' || rows[2]?.statusTone !== 'warning' || rows[2]?.description !== '历史活动') {
  throw new Error(`disabled user group should show stopped status and preserve description, got ${JSON.stringify(rows[2])}`)
}

if (rows[3]?.statusLabel !== 'paused' || rows[3]?.statusTone !== 'neutral') {
  throw new Error(`unknown user group status should preserve raw value with neutral tone, got ${JSON.stringify(rows[3])}`)
}

if (rows[0]?.defaultLabel !== '默认' || rows[0]?.defaultTone !== 'primary' || rows[1]?.defaultLabel !== '普通') {
  throw new Error(`user group default labels should be localized, got ${JSON.stringify(rows.slice(0, 2))}`)
}

const summary = userGroupSummary(groups)
if (summary.total !== 4 || summary.enabled !== 2 || summary.defaultName !== '基础分组' || summary.highestMultiplier !== '1.50000') {
  throw new Error(`user group summary should count enabled aliases and highest multiplier, got ${JSON.stringify(summary)}`)
}

if (userGroupStatusLabel(' ENABLED ') !== '启用' || userGroupStatusTone('ACTIVE') !== 'success') {
  throw new Error('user group status helpers should normalize case and whitespace')
}

if (userGroupStatusLabel('') !== '未知状态' || userGroupStatusTone('unknown') !== 'neutral') {
  throw new Error('empty or unknown user group status should have safe fallbacks')
}
