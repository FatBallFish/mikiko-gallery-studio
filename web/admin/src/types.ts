import type { AdminPermission, AdminRole, AdminSession } from '../../shared/api-types'

export type AdminRouteId =
  | 'login'
  | 'dashboard'
  | 'monitoring'
  | 'cluster'
  | 'users'
  | 'user-groups'
  | 'call-records'
  | 'redeem'
  | 'reviews'
  | 'orders'
  | 'packages'
  | 'cashier-config'
  | 'routing'
  | 'access-accounts'
  | 'pricing'
  | 'audit'
  | 'system-users'
  | 'system-settings'
export type ProtectedAdminRouteId = Exclude<AdminRouteId, 'login'>

export type AdminNavGroup = {
  label: string
  items: Array<{ id: ProtectedAdminRouteId; label: string; hint?: string; badgeKey?: 'review_count' | 'failed_webhook_count' | 'config_drafts' }>
}

export const ROLE_PERMISSION_MAP: Partial<Record<AdminRole, AdminPermission[]>> = {
  super_admin: ['read:all', 'manage:admins', 'manage:users', 'manage:billing', 'manage:cashier', 'manage:models', 'manage:reviews', 'manage:config', 'manage:dangerous_config', 'view:audit'],
  admin: ['read:all', 'manage:users', 'manage:billing', 'manage:cashier', 'manage:models', 'manage:reviews', 'manage:config', 'view:audit'],
}

export const ADMIN_ROUTE_PERMISSION_MAP: Record<ProtectedAdminRouteId, AdminPermission> = {
  dashboard: 'read:all',
  monitoring: 'read:all',
  cluster: 'read:all',
  users: 'manage:users',
  'user-groups': 'manage:users',
  'call-records': 'read:all',
  redeem: 'manage:billing',
  reviews: 'manage:reviews',
  orders: 'manage:cashier',
  packages: 'manage:cashier',
  'cashier-config': 'manage:cashier',
  routing: 'manage:models',
  'access-accounts': 'manage:models',
  pricing: 'manage:models',
  audit: 'view:audit',
  'system-users': 'manage:admins',
  'system-settings': 'manage:config',
}

export const ADMIN_ROUTE_ORDER: ProtectedAdminRouteId[] = [
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
  'audit',
  'system-users',
  'system-settings',
]

export function resolveAdminPermissions(session: AdminSession | null): AdminPermission[] {
  if (!session) return []
  if (session.permissions) return session.permissions
  return ROLE_PERMISSION_MAP[session.role] ?? []
}

export function canAdmin(session: AdminSession | null, permission: AdminPermission) {
  const permissions = resolveAdminPermissions(session)
  return permissions.includes(permission)
}

export function canAccessAdminRoute(session: AdminSession | null, route: AdminRouteId): boolean {
  if (route === 'login') return !session
  return canAdmin(session, ADMIN_ROUTE_PERMISSION_MAP[route])
}

export function firstAccessibleAdminRoute(session: AdminSession | null): ProtectedAdminRouteId {
  return ADMIN_ROUTE_ORDER.find((route) => canAccessAdminRoute(session, route)) ?? 'dashboard'
}

export function filterAdminNavGroups(groups: AdminNavGroup[], session: AdminSession): AdminNavGroup[] {
  return groups
    .map((group) => ({
      ...group,
      items: group.items.filter((item) => canAccessAdminRoute(session, item.id)),
    }))
    .filter((group) => group.items.length > 0)
}

export type ToastTone = 'success' | 'warning' | 'danger' | 'neutral'

export type ToastMessage = {
  id: string
  tone: ToastTone
  title: string
  detail?: string
}

export type AdminAuthState = {
  session: AdminSession | null
  isAuthenticated: boolean
}

export type PageStatus<T> = {
  loading: boolean
  error: string | null
  data: T | null
}
