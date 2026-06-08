import { API_PATHS } from '../../../shared/api-types'
import { protectedRoutes } from '../components'
import { ADMIN_ROUTE_PERMISSION_MAP } from '../types'
import { SecurityConfigPage } from './SecurityConfigPage'

if (API_PATHS.ops.securitySMTP !== '/api/ops/admin/v1/security/smtp') {
  throw new Error(`security smtp API path should be stable, got ${API_PATHS.ops.securitySMTP}`)
}

if (API_PATHS.ops.securitySMTPTest !== '/api/ops/admin/v1/security/smtp/test') {
  throw new Error(`security smtp test API path should be stable, got ${API_PATHS.ops.securitySMTPTest}`)
}

if (!protectedRoutes.includes('security-config')) {
  throw new Error('security config route should be protected and navigable')
}

if (ADMIN_ROUTE_PERMISSION_MAP['security-config'] !== 'manage:dangerous_config') {
  throw new Error(`security config route should require manage:dangerous_config, got ${ADMIN_ROUTE_PERMISSION_MAP['security-config']}`)
}

if (typeof SecurityConfigPage !== 'function') {
  throw new Error('SecurityConfigPage should be exported as a React page component')
}
