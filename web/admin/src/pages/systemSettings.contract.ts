import { normalizeRoute, protectedRoutes } from '../layout/admin-navigation'
import { systemSettingsTabFromHash } from './SystemSettingsPage'

if (!protectedRoutes.includes('system-settings')) {
  throw new Error('system-settings must be the single protected settings route')
}

for (const route of ['general-settings', 'security-settings', 'storage-settings'] as const) {
  if ((protectedRoutes as readonly string[]).includes(route)) {
    throw new Error(`${route} must stay as a compatibility alias, not a first-level route`)
  }
}

const routeCases = ['#/general-settings', '#/security-settings', '#/storage-settings', '#/system-settings?tab=storage']
for (const hash of routeCases) {
  if (normalizeRoute(hash) !== 'system-settings') {
    throw new Error(`${hash} should normalize to system-settings`)
  }
}

const tabCases = new Map([
  ['#/system-settings', 'general'],
  ['#/general-settings', 'general'],
  ['#/security-settings', 'security'],
  ['#/storage-settings', 'storage'],
  ['#/system-settings?tab=security', 'security'],
  ['#/system-settings?tab=storage', 'storage'],
])

for (const [hash, expected] of tabCases) {
  const actual = systemSettingsTabFromHash(hash)
  if (actual !== expected) throw new Error(`${hash} should select ${expected}, got ${actual}`)
}
