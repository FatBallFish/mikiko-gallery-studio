import { API_PATHS } from '../../shared/api-types'
import { userHashForRoute } from './routeState'
import type { RouteId } from './types'

// Backend authorize endpoint (same origin as the SPA behind nginx/vite proxy).
export function oidcAuthorizeHref(returnToURL: string): string {
  return `${API_PATHS.agent.oidcAuthorize}?return_to=${encodeURIComponent(returnToURL)}`
}

// The same-origin URL (hash included) the SPA should restore after login.
// Falls back to home for direct visits on the login route.
export function targetURLAfterLogin(route: RouteId | undefined, imageId?: string, taskId?: string): string {
  const target = route && route !== 'login' ? route : 'home'
  return `${window.location.origin}${window.location.pathname}#${userHashForRoute(target, { imageId, taskId })}`
}

// Defense in depth: the backend already sanitizes return_to; never follow a
// cross-origin or malformed target on the client.
export function sanitizeOIDCReturnTo(returnTo?: string): string {
  const fallback = `${window.location.origin}/#/home`
  if (!returnTo) return fallback
  try {
    const url = new URL(returnTo, window.location.origin)
    if (url.origin !== window.location.origin) return fallback
    return url.href
  } catch {
    return fallback
  }
}

// Strips the portal silent-login marker and returns the cleaned address.
export function stripSSOSilent(): { active: boolean; cleanPath: string } {
  const params = new URLSearchParams(window.location.search)
  if (!params.get('sso')) return { active: false, cleanPath: window.location.pathname + window.location.search + window.location.hash }
  params.delete('sso')
  const search = params.toString()
  const cleanPath = window.location.pathname + (search ? `?${search}` : '') + window.location.hash
  window.history.replaceState(null, '', cleanPath)
  return { active: true, cleanPath }
}

// Detects the provider's full-page redirect back to /auth/callback.
export function readOIDCCallback(): { code: string; state: string } | { error: string; description: string } | null {
  if (window.location.pathname.replace(/\/+$/, '') !== '/auth/callback') return null
  const params = new URLSearchParams(window.location.search)
  const error = params.get('error')
  if (error) return { error, description: params.get('error_description') ?? '' }
  const code = params.get('code')
  const state = params.get('state')
  if (!code || !state) return null
  return { code, state }
}
