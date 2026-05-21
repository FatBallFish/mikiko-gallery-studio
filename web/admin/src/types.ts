import type { AdminSession } from '../../shared/api-types'

export type AdminRouteId = 'login' | 'overview' | 'config' | 'routing' | 'pricing' | 'reviews' | 'users' | 'audit' | 'health'

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
