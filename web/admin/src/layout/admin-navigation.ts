import type { AdminNavGroup, AdminRouteId, ProtectedAdminRouteId } from '../types'

export const protectedRoutes: ProtectedAdminRouteId[] = [
  'dashboard',
  'monitoring',
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
  'audit',
  'system-users',
  'system-settings',
]

export const navGroups: AdminNavGroup[] = [
  {
    label: '概览 / Overview',
    items: [
      { id: 'dashboard', label: '运营大盘', hint: 'Dashboard' },
      { id: 'monitoring', label: '运维监控', hint: 'Monitor' },
    ],
  },
  {
    label: '业务管理 / Business',
    items: [
      { id: 'users', label: '用户管理', hint: 'Users' },
      { id: 'user-groups', label: '用户分组', hint: 'Groups' },
      { id: 'call-records', label: '调用记录', hint: 'Calls' },
      { id: 'redeem', label: '兑换码', hint: 'Coupons' },
      { id: 'reviews', label: '审核队列', hint: 'Review', badgeKey: 'review_count' },
    ],
  },
  {
    label: '商业化 / Commercial',
    items: [
      { id: 'orders', label: '订单管理', hint: 'Orders', badgeKey: 'failed_webhook_count' },
      { id: 'packages', label: '套餐管理', hint: 'Plans' },
      { id: 'cashier-config', label: '收银台配置', hint: 'Cashier' },
    ],
  },
  {
    label: '路由与模型 / Models',
    items: [
      { id: 'routing', label: '路由模型', hint: 'Routes' },
      { id: 'access-accounts', label: '接入账号', hint: 'Accounts' },
      { id: 'pricing', label: '价格配置', hint: 'Pricing' },
    ],
  },
  {
    label: '系统 / System',
    items: [
      { id: 'audit', label: '审计日志', hint: 'Trail' },
      { id: 'system-users', label: '系统账户', hint: 'Admins' },
      { id: 'system-settings', label: '系统设置', hint: 'Settings', badgeKey: 'config_drafts' },
    ],
  },
]

const routeAliases: Partial<Record<string, ProtectedAdminRouteId>> = {
  overview: 'dashboard',
  readiness: 'monitoring',
  health: 'monitoring',
  cashier: 'cashier-config',
  config: 'system-settings',
  'security-config': 'system-settings',
  'provider-models': 'access-accounts',
}

export const routeTitles: Record<ProtectedAdminRouteId, string> = {
  dashboard: '运营大盘',
  monitoring: '运维监控',
  users: '用户管理',
  'user-groups': '用户分组',
  'call-records': '调用记录',
  redeem: '兑换码',
  reviews: '审核队列',
  orders: '订单管理',
  packages: '套餐管理',
  'cashier-config': '收银台配置',
  routing: '路由模型',
  'access-accounts': '接入账号',
  pricing: '价格配置',
  audit: '审计日志',
  'system-users': '系统账户',
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
