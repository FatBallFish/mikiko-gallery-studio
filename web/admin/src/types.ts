import type { AdminPermission, AdminRole, AdminSession } from '../../shared/api-types'

export type AdminRouteId = 'login' | 'overview' | 'readiness' | 'config' | 'routing' | 'pricing' | 'reviews' | 'users' | 'user-groups' | 'redeem' | 'cashier' | 'call-records' | 'provider-models' | 'audit' | 'health'
export type ProtectedAdminRouteId = Exclude<AdminRouteId, 'login'>

export type AdminNavGroup = {
  label: string
  items: Array<{ id: ProtectedAdminRouteId; label: string; hint: string }>
}

export const ROLE_PERMISSION_MAP: Partial<Record<AdminRole, AdminPermission[]>> = {
  super_admin: ['read:all', 'manage:admins', 'manage:users', 'manage:billing', 'manage:cashier', 'manage:models', 'manage:reviews', 'manage:config', 'manage:dangerous_config', 'view:audit'],
  admin: ['read:all', 'manage:users', 'manage:billing', 'manage:cashier', 'manage:models', 'manage:reviews', 'manage:config', 'view:audit'],
}

export const ADMIN_ROUTE_PERMISSION_MAP: Record<ProtectedAdminRouteId, AdminPermission> = {
  overview: 'read:all',
  readiness: 'read:all',
  health: 'read:all',
  users: 'manage:users',
  'user-groups': 'manage:users',
  redeem: 'manage:billing',
  reviews: 'manage:reviews',
  'call-records': 'read:all',
  cashier: 'manage:cashier',
  routing: 'manage:models',
  'provider-models': 'manage:models',
  pricing: 'manage:models',
  audit: 'view:audit',
  config: 'manage:config',
}

export const ADMIN_ROUTE_ORDER: ProtectedAdminRouteId[] = [
  'overview',
  'readiness',
  'health',
  'users',
  'user-groups',
  'redeem',
  'reviews',
  'call-records',
  'cashier',
  'routing',
  'provider-models',
  'pricing',
  'audit',
  'config',
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
  return ADMIN_ROUTE_ORDER.find((route) => canAccessAdminRoute(session, route)) ?? 'overview'
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
