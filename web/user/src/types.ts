import type { Balance, UserProfile } from '../../shared/api-types'

export type RouteId = 'landing' | 'login' | 'home' | 'genpic' | 'gallery' | 'public-gallery' | 'checkout' | 'api-keys' | 'profile' | 'docs'

export type ToastTone = 'success' | 'error' | 'info'

export type Toast = {
  id: number
  tone: ToastTone
  message: string
  durationMs?: number
}

export type SessionState = {
  token: string
  profile: UserProfile
}

export type AppContextValue = {
  route: RouteId
  isAuthenticated: boolean
  session: SessionState | null
  profile: UserProfile | null
  balance: Balance | null
  refreshAccount: () => Promise<void>
  navigate: (route: RouteId, options?: { returnTo?: RouteId; imageId?: string | null }) => void
  login: (session: SessionState, returnTo?: RouteId, options?: { imageId?: string | null }) => Promise<void>
  logout: () => Promise<void>
  notify: (tone: ToastTone, message: string) => void
}
