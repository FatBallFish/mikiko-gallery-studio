import type { AdminMetric, AdminUser } from '../../../shared/api-types'
import { adminUserStatusBadge, type AdminUserStatusTone } from './userRows'

export type OverviewRecentUserRow = {
  id: string
  displayName: string
  email: string
  balance: string
  createdAt: string
  statusLabel: string
  statusTone: AdminUserStatusTone
  actionHref: string
  actionLabel: string
  permission: string
  raw: AdminUser
}

export type OverviewMetricRow = AdminMetric & {
  rawKey: string
}

const metricMeta: Record<string, { label: string; detail: string }> = {
  today_order_count: { label: '今日完成订单', detail: '收银台今日已完成订单数量。' },
  payment_success_rate: { label: '支付成功率', detail: '收银台订单支付成功率，用于观察支付渠道健康度。' },
  failed_webhook_count: { label: '失败回调', detail: '支付渠道回调失败数量，可前往收银台回调事件处理。' },
  refund_compensation_failed_count: { label: '退款补偿失败', detail: '真实渠道退款已返回但本地落账失败的数量，请在收银台回调事件中重试或排查。' },
  signup_trial_users: { label: '体验额度用户', detail: '通过注册获得体验额度的用户数量。' },
  trial_expiring_users: { label: '体验额度临期', detail: '体验额度即将过期的用户数量。' },
  preflight_failures: { label: '生成前置失败', detail: '生成前参数、余额、路由或价格校验失败数量。' },
  public_gallery_views: { label: '公开广场访问', detail: '公开广场列表访问次数。' },
  mock_payment: { label: 'Mock 支付', detail: '测试支付渠道状态，生产环境应关闭。' },
}

export function overviewRecentUserRows(users: AdminUser[]): OverviewRecentUserRow[] {
  return users
    .slice()
    .sort((left, right) => right.created_at.localeCompare(left.created_at))
    .slice(0, 5)
    .map((user) => {
      const status = adminUserStatusBadge(user.status)
      return {
        id: String(user.id),
        displayName: user.display_name?.trim() || user.email,
        email: user.email,
        balance: user.balance,
        createdAt: user.created_at,
        statusLabel: status.label,
        statusTone: status.tone,
        actionHref: '#/users',
        actionLabel: '管理',
        permission: 'users.manage',
        raw: user,
      }
    })
}

export function overviewMetricRows(metrics: AdminMetric[]): OverviewMetricRow[] {
  return metrics.map((metric) => {
    const rawKey = metric.key ?? metric.label
    const meta = metricMeta[rawKey]
    return {
      ...metric,
      rawKey,
      label: meta?.label ?? metric.label,
      detail: metric.detail || meta?.detail,
    }
  })
}
