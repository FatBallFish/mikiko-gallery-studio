import type { AdminSession } from '../../shared/api-types'
import {
  canAdmin,
  canAccessAdminRoute,
  filterAdminNavGroups,
  firstAccessibleAdminRoute,
  resolveAdminPermissions,
} from './types'
import { navGroups } from './layout/admin-navigation'

const opsAdmin: AdminSession = {
  token: 'token',
  admin_name: 'Ops Admin',
  role: 'admin',
}

if (!canAccessAdminRoute(opsAdmin, 'cashier-config') || !canAccessAdminRoute(opsAdmin, 'users')) {
  throw new Error('admin should retain operational access to users and cashier config')
}

if (canAdmin(opsAdmin, 'manage:admins')) {
  throw new Error('admin must not receive administrator account management permission')
}

if (!canAccessAdminRoute(opsAdmin, 'system-settings')) {
  throw new Error('admin should retain access to system settings')
}

if ((navGroups.flatMap((group) => group.items.map((item) => item.id)) as readonly string[]).some((id) => ['general-settings', 'security-settings', 'storage-settings'].includes(id))) {
  throw new Error('split settings routes must not remain in primary navigation')
}

const explicitlyDeniedOpsAdmin: AdminSession = {
  ...opsAdmin,
  permissions: [],
}

if (resolveAdminPermissions(explicitlyDeniedOpsAdmin).length !== 0 || canAccessAdminRoute(explicitlyDeniedOpsAdmin, 'users')) {
  throw new Error('explicit session permissions must override built-in role fallback even when empty')
}

const unknownRole: AdminSession = {
  token: 'token',
  admin_name: 'Custom Role',
  role: 'finance_admin',
}

if (resolveAdminPermissions(unknownRole).length !== 0 || canAccessAdminRoute(unknownRole, 'dashboard')) {
  throw new Error('unknown roles without explicit permissions must default to no access')
}

const customCashierRole: AdminSession = {
  ...unknownRole,
  permissions: ['read:all', 'manage:cashier'],
}

if (!canAccessAdminRoute(customCashierRole, 'cashier-config') || canAccessAdminRoute(customCashierRole, 'users')) {
  throw new Error('custom roles should be governed by explicit session permissions')
}

if (firstAccessibleAdminRoute(customCashierRole) !== 'dashboard') {
  throw new Error('custom role with read permission should land on dashboard')
}

const filtered = filterAdminNavGroups(navGroups, customCashierRole)
const visibleIds = filtered.flatMap((group) => group.items.map((item) => item.id)).join(',')

if (visibleIds.includes('users') || !visibleIds.includes('cashier-config')) {
  throw new Error(`admin navigation should hide unauthorized entries, got ${visibleIds}`)
}

if (visibleIds.includes('security-settings') || visibleIds.includes('storage-settings') || visibleIds.includes('general-settings')) {
  throw new Error(`split settings routes should stay hidden because settings are aggregated, got ${visibleIds}`)
}
