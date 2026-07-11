export type MotionPreference = Pick<MediaQueryList, 'matches'>

export function prefersReducedMotion(preference?: MotionPreference | null): boolean {
  if (preference) return preference.matches
  return globalThis.window?.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
}

export function routeTransitionClass(reducedMotion = prefersReducedMotion()): string {
  return reducedMotion ? '' : 'pg-route-enter'
}
