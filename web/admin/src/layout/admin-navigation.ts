import type { AdminNavGroup, AdminRouteId, ProtectedAdminRouteId } from '../types'

export const protectedRoutes: ProtectedAdminRouteId[] = [
  'dashboard',
  'monitoring',
  'cluster',
  'users',
  'user-groups',
  'call-records',
  'redeem',
  'reviews',
  'orders',
  'packages',
  'cashier-config',
  'routing',
  'access-accounts',
  'pricing',
  'video-tasks',
  'media-policy',
  'audit',
  'system-users',
  'system-settings',
]

export const navGroups: AdminNavGroup[] = [
  {
    label: '概览',
    items: [
      { id: 'dashboard', label: '运营总览' },
      { id: 'monitoring', label: '系统健康' },
      { id: 'cluster', label: '集群节点' },
    ],
  },
  {
    label: '用户与内容',
    items: [
      { id: 'users', label: '用户' },
      { id: 'user-groups', label: '用户分组' },
      { id: 'reviews', label: '审核队列', badgeKey: 'review_count' },
      { id: 'redeem', label: '兑换码' },
    ],
  },
  {
    label: '交易',
    items: [
      { id: 'orders', label: '订单', badgeKey: 'failed_webhook_count' },
      { id: 'packages', label: '套餐' },
      { id: 'cashier-config', label: '支付配置' },
    ],
  },
  {
    label: '模型与生成',
    items: [
      { id: 'access-accounts', label: '接入账号' },
      { id: 'routing', label: '路由模型' },
      { id: 'pricing', label: '价格策略' },
      { id: 'video-tasks', label: '视频任务' },
      { id: 'call-records', label: '图片任务' },
    ],
  },
  {
    label: '系统',
    items: [
      { id: 'system-users', label: '管理员' },
      { id: 'audit', label: '审计日志' },
      { id: 'system-settings', label: '系统设置', badgeKey: 'config_drafts' },
    ],
  },
]

const routeAliases: Partial<Record<string, ProtectedAdminRouteId>> = {
  overview: 'dashboard',
  readiness: 'monitoring',
  health: 'monitoring',
  cashier: 'cashier-config',
  config: 'system-settings',
  'general-config': 'system-settings',
  'general-settings': 'system-settings',
  'security-config': 'system-settings',
  'security-settings': 'system-settings',
  'storage-config': 'system-settings',
  'storage-settings': 'system-settings',
  'provider-models': 'access-accounts',
}

export const routeTitles: Record<ProtectedAdminRouteId, string> = {
  dashboard: '运营总览',
  monitoring: '系统健康',
  cluster: '集群节点',
  users: '用户管理',
  'user-groups': '用户分组',
  'call-records': '图片任务',
  redeem: '兑换码',
  reviews: '审核队列',
  orders: '订单管理',
  packages: '套餐管理',
  'cashier-config': '收银台配置',
  routing: '路由模型',
  'access-accounts': '接入账号',
  pricing: '价格策略',
  'video-tasks': '视频任务',
  'media-policy': '媒体策略',
  audit: '审计日志',
  'system-users': '管理员',
  'system-settings': '系统设置',
}

export function normalizeRoute(hash: string): AdminRouteId {
  const path = hash.replace(/^#\/?/, '').split('?')[0].replace(/^\/+|\/+$/g, '')
  if (path === 'login') return 'login'
  const aliased = routeAliases[path]
  if (aliased) return aliased
  return protectedRoutes.includes(path as ProtectedAdminRouteId) ? (path as AdminRouteId) : 'dashboard'
}

export function routeHref(route: AdminRouteId) {
  return `#/${route}`
}
