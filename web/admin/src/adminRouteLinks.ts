const readinessRouteAliases: Record<string, string> = {
  overview: 'dashboard',
  readiness: 'monitoring',
  health: 'monitoring',
  cashier: 'cashier-config',
  config: 'system-settings',
  'security-config': 'system-settings',
  'provider-models': 'access-accounts',
}

export function adminActionHref(route?: string | null, fallback = 'monitoring') {
  const normalized = (route ?? '').trim() || fallback
  return `#/${readinessRouteAliases[normalized] ?? normalized}`
}
