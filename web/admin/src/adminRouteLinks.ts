const readinessRouteAliases: Record<string, string> = {
  overview: 'dashboard',
  readiness: 'monitoring',
  health: 'monitoring',
  cashier: 'cashier-config',
  config: 'system-settings?tab=general',
  'system-settings': 'system-settings',
  'general-settings': 'system-settings?tab=general',
  'security-config': 'system-settings?tab=security',
  'security-settings': 'system-settings?tab=security',
  'storage-config': 'system-settings?tab=storage',
  'storage-settings': 'system-settings?tab=storage',
  'provider-models': 'access-accounts',
}

export function adminActionHref(route?: string | null, fallback = 'monitoring') {
  const normalized = (route ?? '').trim() || fallback
  return `#/${readinessRouteAliases[normalized] ?? normalized}`
}
