import type { AdminMetric, AdminUser } from '../../../shared/api-types'
import { overviewMetricRows, overviewRecentUserRows } from './overviewRows'

const users = overviewRecentUserRows([
  user({ id: '1', display_name: '较早用户', email: 'old@example.com', status: 'active', balance: '1.00000', created_at: '2026-06-01T00:00:00Z' }),
  user({ id: '2', display_name: '最新用户', email: 'new@example.com', status: 'closed', balance: '2.00000', created_at: '2026-06-06T00:00:00Z' }),
  user({ id: '3', display_name: '', email: 'pending@example.com', status: 'pending', balance: '3.00000', created_at: '2026-06-05T00:00:00Z' }),
  user({ id: '4', display_name: '停用用户', email: 'disabled@example.com', status: 'disabled', balance: '4.00000', created_at: '2026-06-04T00:00:00Z' }),
  user({ id: '5', display_name: '未知状态', email: 'unknown@example.com', status: 'suspended', balance: '5.00000', created_at: '2026-06-03T00:00:00Z' }),
  user({ id: '6', display_name: '第六位', email: 'six@example.com', status: 'active', balance: '6.00000', created_at: '2026-06-02T00:00:00Z' }),
])

if (users.length !== 5 || users[0]?.id !== '2' || users[4]?.id !== '6') {
  throw new Error(`overview recent users should sort by latest created_at and cap to five, got ${JSON.stringify(users)}`)
}

if (users[0]?.statusLabel !== '已关闭' || users[0]?.statusTone !== 'neutral') {
  throw new Error(`overview recent users should reuse localized closed status, got ${JSON.stringify(users[0])}`)
}

if (users[1]?.displayName !== 'pending@example.com' || users[1]?.statusLabel !== '待验证') {
  throw new Error(`overview recent users should fallback display name to email and localize pending status, got ${JSON.stringify(users[1])}`)
}

if (users[2]?.statusLabel !== '禁用' || users[2]?.statusTone !== 'danger') {
  throw new Error(`overview recent users should align disabled copy with user management page, got ${JSON.stringify(users[2])}`)
}

if (users[3]?.statusLabel !== 'suspended' || users[3]?.statusTone !== 'neutral') {
  throw new Error(`overview recent users should preserve unknown statuses for troubleshooting, got ${JSON.stringify(users[3])}`)
}

for (const row of users) {
  if (row.actionHref !== '#/users' || row.actionLabel !== '管理' || row.permission !== 'users.manage') {
    throw new Error(`overview recent user rows should reserve RBAC action anchor, got ${JSON.stringify(row)}`)
  }
}

const metrics = overviewMetricRows([
  metric({ key: 'payment_success_rate', label: 'payment_success_rate', value: '97%', trend: '+2%', tone: 'good' }),
  metric({ key: 'refund_compensation_failed_count', label: 'refund_compensation_failed_count', value: '1', trend: '需处理', tone: 'danger' }),
  metric({ key: 'custom_future_metric', label: 'custom_future_metric', value: '9', trend: '-', tone: 'neutral' }),
])

const paymentMetric = metrics[0]
const refundMetric = metrics[1]
const customMetric = metrics[2]

if (!paymentMetric || paymentMetric.label !== '支付成功率' || !paymentMetric.detail?.includes('收银台')) {
  throw new Error(`known overview metrics should be operator-facing, got ${JSON.stringify(paymentMetric)}`)
}

if (!refundMetric || refundMetric.label !== '退款补偿失败' || !refundMetric.detail?.includes('回调事件')) {
  throw new Error(`refund compensation metric should guide current remediation path, got ${JSON.stringify(refundMetric)}`)
}

if (!customMetric || customMetric.label !== 'custom_future_metric' || customMetric.rawKey !== 'custom_future_metric') {
  throw new Error(`unknown overview metrics should preserve raw key for troubleshooting, got ${JSON.stringify(customMetric)}`)
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

function metric(patch: Partial<AdminMetric>): AdminMetric {
  return {
    key: patch.key,
    label: patch.label ?? '指标',
    value: patch.value ?? '0',
    trend: patch.trend ?? '-',
    tone: patch.tone ?? 'neutral',
    detail: patch.detail,
  }
}
