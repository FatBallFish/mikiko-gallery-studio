import type { AdminUser } from '../../../shared/api-types'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
import {
  adminUserRowActions,
  adminUserRowView,
  adminUserStatusBadge,
  adminUserStatusFilterOptions,
  adminUserStatusOptions,
  adminUserSummary,
} from './userRows'

const usersPageSource = readFileSync(new URL('./UsersPage.tsx', import.meta.url), 'utf8')

for (const requiredPrimitive of ['MetricStrip', 'FilterToolbar', 'Drawer']) {
  if (!usersPageSource.includes(requiredPrimitive)) {
    throw new Error(`user management should use the shared ${requiredPrimitive} primitive`)
  }
}

if (/detailTarget\s*\?\s*\(\s*<Modal/.test(usersPageSource)) {
  throw new Error('user detail should use a Drawer instead of a Modal')
}

if (!usersPageSource.includes('data-admin-user-section={dataSection}')) {
  throw new Error('user detail sections should expose stable rendered section markers')
}

for (const section of ['profile', 'ledger', 'resources', 'limits', 'danger']) {
  if (!usersPageSource.includes(`dataSection="${section}"`)) {
    throw new Error(`user detail drawer should expose the ${section} section`)
  }
}

if (!usersPageSource.includes('resultSummary=')) {
  throw new Error('user filter toolbar should keep the result count in the same operational surface')
}

if (!usersPageSource.includes('const hasResources =')) {
  throw new Error('user resource detail should render an intentional empty state when every resource collection is empty')
}

if (!usersPageSource.includes('message={actionError}')) {
  throw new Error('user mutations should keep failure feedback local to the active operation')
}

if (!usersPageSource.includes('detailTarget && !action ? (')) {
  throw new Error('user detail Drawer must unmount while a nested action Modal owns focus')
}

for (const unsafeConfirmation of ['confirmEmail: rawRow.email', 'confirmEmail: user.email']) {
  if (usersPageSource.includes(unsafeConfirmation)) {
    throw new Error('destructive user actions must require the operator to type the confirmation email')
  }
}

if (!usersPageSource.includes('操作已完成，但详情刷新失败')) {
  throw new Error('detail refresh failures must distinguish a successful mutation from a failed refresh')
}

const active = adminUserStatusBadge('active')
const pending = adminUserStatusBadge('pending')
const disabled = adminUserStatusBadge('disabled')
const closed = adminUserStatusBadge('closed')
if (active.label !== '正常' || active.tone !== 'success' || pending.label !== '待验证' || pending.tone !== 'warning' || disabled.label !== '禁用' || disabled.tone !== 'danger' || closed.label !== '已关闭') {
  throw new Error(`admin user statuses should be localized, got ${JSON.stringify({ active, pending, disabled, closed })}`)
}

const unknown = adminUserStatusBadge('suspended')
if (unknown.label !== 'suspended' || unknown.tone !== 'neutral') {
  throw new Error(`unknown user status should preserve raw value for troubleshooting, got ${JSON.stringify(unknown)}`)
}

if (adminUserStatusBadge('').label !== '未知状态' || adminUserStatusBadge(null).label !== '未知状态') {
  throw new Error('empty user statuses should show a clear fallback')
}

for (const rawValue of ['active', 'pending', 'disabled']) {
  if (!adminUserStatusOptions.some((option) => option.value === rawValue)) {
    throw new Error(`user status options must preserve raw value ${rawValue}`)
  }
}

for (const option of adminUserStatusOptions) {
  if (String(option.label) === String(option.value)) {
    throw new Error(`user status option ${option.value} should expose operator-facing label`)
  }
}

const filterLabels = adminUserStatusFilterOptions.map((option) => option.label).join(',')
if (filterLabels !== '全部状态,正常,待验证,禁用,已关闭') {
  throw new Error(`user status filter options should be localized, got ${filterLabels}`)
}

const rows: AdminUser[] = [
  user({ id: '1', status: 'active' }),
  user({ id: '2', status: 'pending' }),
  user({ id: '3', status: 'disabled' }),
  user({ id: '4', status: 'closed' }),
  user({ id: '5', status: 'active' }),
  user({ id: '6', status: 'suspended' }),
]
const summary = adminUserSummary(rows)
if (summary.total !== 6 || summary.active !== 2 || summary.pending !== 1 || summary.disabled !== 1 || summary.closed !== 1) {
  throw new Error(`user summary should count known statuses and ignore unknown buckets, got ${JSON.stringify(summary)}`)
}

const row = adminUserRowView(user({
  id: '42',
  display_name: '',
  email: 'operator@example.com',
  status: 'active',
  group: 'basic,vip',
  balance: '12.34567',
  created_at: '2026-06-05T13:45:30Z',
  updated_at: '2026-06-06T08:00:00Z',
  last_seen_at: '',
}))

if (row.name !== 'operator@example.com' || row.subtitle !== 'operator@example.com · 42') {
  throw new Error(`admin user row should fall back empty display name to email and keep subtitle, got ${JSON.stringify(row)}`)
}

if (row.statusLabel !== '正常' || row.statusTone !== 'success' || row.groupLabel !== 'basic, vip' || row.balanceLabel !== '12.34567') {
  throw new Error(`admin user row should expose localized status/group/balance labels, got ${JSON.stringify(row)}`)
}

if (row.lastActiveAtLabel !== '2026/06/06 08:00' || row.createdAtLabel !== '2026/06/05 13:45') {
  throw new Error(`admin user row should format list dates, got ${JSON.stringify(row)}`)
}

if (/T|Z$/.test(`${row.lastActiveAtLabel}${row.createdAtLabel}`)) {
  throw new Error(`admin user row dates should not expose ISO separators, got ${JSON.stringify(row)}`)
}

const invalidDateRow = adminUserRowView(user({ created_at: 'not-a-date', last_seen_at: 'also-not-a-date', updated_at: '' }))
if (invalidDateRow.createdAtLabel !== 'not-a-date' || invalidDateRow.lastActiveAtLabel !== 'also-not-a-date') {
  throw new Error(`admin user row should preserve invalid dates for troubleshooting, got ${JSON.stringify(invalidDateRow)}`)
}

const missingDateRow = adminUserRowView(user({ created_at: '', updated_at: '', last_seen_at: '' }))
if (missingDateRow.createdAtLabel !== '-' || missingDateRow.lastActiveAtLabel !== '-') {
  throw new Error(`admin user row should use - for missing dates, got ${JSON.stringify(missingDateRow)}`)
}

const actions = adminUserRowActions(user({ status: 'active', email: 'danger@example.com' }))
if (actions.primary.label !== '详情') {
  throw new Error(`user row primary action should be detail, got ${JSON.stringify(actions.primary)}`)
}

if (actions.secondary) {
  throw new Error(`user row should not expose a second permanent action, got ${JSON.stringify(actions.secondary)}`)
}

const actionLabels = actions.overflow.map((action) => action.label)
for (const label of ['禁用', '调整分组', '调整积分', '设置限额', '重置密码', '删除']) {
  if (!actionLabels.includes(label)) {
    throw new Error(`user row overflow actions should include ${label}, got ${actionLabels.join(',')}`)
  }
}

for (const label of ['禁用', '删除']) {
  const action = actions.overflow.find((item) => item.label === label)
  if (!action?.confirm || action.confirm.expectedValue !== 'danger@example.com') {
    throw new Error(`${label} should require email confirmation, got ${JSON.stringify(action)}`)
  }
}

function user(patch: Partial<AdminUser>): AdminUser {
  return {
    id: patch.id ?? '1',
    email: patch.email ?? 'user@example.com',
    display_name: patch.display_name ?? '测试用户',
    status: patch.status ?? 'active',
    group: patch.group ?? 'basic',
    balance: patch.balance ?? '0.00000',
    created_at: patch.created_at ?? '2026-06-05T00:00:00Z',
    updated_at: patch.updated_at ?? '2026-06-05T00:00:00Z',
    last_seen_at: patch.last_seen_at ?? '2026-06-05T00:00:00Z',
  }
}
