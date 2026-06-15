import { API_PATHS } from '../../../shared/api-types'
import { protectedRoutes } from '../layout/admin-navigation'
import { ADMIN_ROUTE_PERMISSION_MAP } from '../types'
import { SecurityConfigPage } from './SecurityConfigPage'

if (API_PATHS.ops.securitySMTP !== '/api/ops/admin/v1/security/smtp') {
  throw new Error(`security smtp API path should be stable, got ${API_PATHS.ops.securitySMTP}`)
}

if (API_PATHS.ops.securitySMTPTest !== '/api/ops/admin/v1/security/smtp/test') {
  throw new Error(`security smtp test API path should be stable, got ${API_PATHS.ops.securitySMTPTest}`)
}

if (!protectedRoutes.includes('system-settings')) {
  throw new Error('system settings route should be protected and navigable')
}

if (ADMIN_ROUTE_PERMISSION_MAP['system-settings'] !== 'manage:config') {
  throw new Error(`system settings route should require manage:config, got ${ADMIN_ROUTE_PERMISSION_MAP['system-settings']}`)
}

if (typeof SecurityConfigPage !== 'function') {
  throw new Error('SecurityConfigPage should be exported as a React page component')
}
