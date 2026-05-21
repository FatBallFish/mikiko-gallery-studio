import { useCallback, useEffect, useMemo, useState } from 'react'
import { mockApi } from '../../shared/mock-api'
import type { Balance, UserProfile } from '../../shared/api-types'
import { AppContext, protectedRoutes, Shell, ToastViewport } from './components'
import type { RouteId, SessionState, Toast, ToastTone } from './types'
import { LandingPage } from './pages/LandingPage'
import { LoginPage } from './pages/LoginPage'
import { HomePage } from './pages/HomePage'
import { WorkspacePage } from './pages/WorkspacePage'
import { GalleryPage } from './pages/GalleryPage'
import { ApiKeysPage } from './pages/ApiKeysPage'
import { ProfilePage } from './pages/ProfilePage'
import { DocsPage } from './pages/DocsPage'

const sessionKey = 'pic-gallery-user-session'
const routeSet = new Set<RouteId>(['landing', 'login', 'home', 'genpic', 'gallery', 'api-keys', 'profile', 'docs'])

function parseHash() {
  const raw = window.location.hash.replace(/^#\/?/, '')
  const [path = 'landing', query = ''] = raw.split('?')
  const route = routeSet.has(path as RouteId) ? path as RouteId : 'landing'
  const params = new URLSearchParams(query)
  const returnTo = params.get('returnTo')
  return { route, returnTo: returnTo && routeSet.has(returnTo as RouteId) ? returnTo as RouteId : undefined }
}

function writeHash(route: RouteId, returnTo?: RouteId) {
  const suffix = returnTo ? `?returnTo=${returnTo}` : ''
  window.location.hash = `/${route}${suffix}`
}

function readStoredSession(): SessionState | null {
  try {
    const raw = window.localStorage.getItem(sessionKey)
    return raw ? JSON.parse(raw) as SessionState : null
  } catch {
    return null
  }
}

export default function App() {
  const initial = parseHash()
  const [route, setRoute] = useState<RouteId>(initial.route)
  const [returnTo, setReturnTo] = useState<RouteId | undefined>(initial.returnTo)
  const [session, setSession] = useState<SessionState | null>(() => readStoredSession())
  const [profile, setProfile] = useState<UserProfile | null>(() => readStoredSession()?.profile ?? null)
  const [balance, setBalance] = useState<Balance | null>(null)
  const [toasts, setToasts] = useState<Toast[]>([])

  const notify = useCallback((tone: ToastTone, message: string) => {
    setToasts((items) => [...items, { id: Date.now() + Math.random(), tone, message }])
  }, [])

  const refreshAccount = useCallback(async () => {
    if (!session?.token) return
    const [nextProfile, nextBalance] = await Promise.all([mockApi.getProfile(), mockApi.getBalance()])
    setProfile(nextProfile)
    setBalance(nextBalance)
    const nextSession = { token: session.token, profile: nextProfile }
    setSession(nextSession)
    window.localStorage.setItem(sessionKey, JSON.stringify(nextSession))
  }, [session?.token])

  useEffect(() => {
    const updateRoute = () => {
      const parsed = parseHash()
      setRoute(parsed.route)
      setReturnTo(parsed.returnTo)
    }
    window.addEventListener('hashchange', updateRoute)
    if (!window.location.hash) writeHash(session ? 'home' : 'landing')
    return () => window.removeEventListener('hashchange', updateRoute)
  }, [session])

  useEffect(() => {
    if (session) void refreshAccount()
  }, [session?.token, refreshAccount])

  useEffect(() => {
    if (!session && protectedRoutes.includes(route)) {
      writeHash('login', route)
    }
    if (session && route === 'login') {
      writeHash(returnTo ?? 'home')
    }
  }, [route, returnTo, session])

  const navigate = useCallback((next: RouteId, options?: { returnTo?: RouteId }) => {
    writeHash(next, options?.returnTo)
  }, [])

  const login = useCallback(async (nextSession: SessionState, target?: RouteId) => {
    setSession(nextSession)
    setProfile(nextSession.profile)
    window.localStorage.setItem(sessionKey, JSON.stringify(nextSession))
    notify('success', '登录成功，欢迎回到 Vault')
    writeHash(target ?? returnTo ?? 'home')
  }, [notify, returnTo])

  const logout = useCallback(async () => {
    await mockApi.logout()
    window.localStorage.removeItem(sessionKey)
    setSession(null)
    setProfile(null)
    setBalance(null)
    notify('info', '已退出登录')
    writeHash('landing')
  }, [notify])

  const appValue = useMemo(() => ({
    route,
    isAuthenticated: Boolean(session),
    session,
    profile,
    balance,
    refreshAccount,
    navigate,
    login,
    logout,
    notify,
  }), [route, session, profile, balance, refreshAccount, navigate, login, logout, notify])

  const page = useMemo(() => {
    switch (route) {
      case 'login':
        return <LoginPage returnTo={returnTo} />
      case 'home':
        return <Shell><HomePage /></Shell>
      case 'genpic':
        return <Shell><WorkspacePage /></Shell>
      case 'gallery':
        return <Shell><GalleryPage /></Shell>
      case 'api-keys':
        return <Shell><ApiKeysPage /></Shell>
      case 'profile':
        return <Shell><ProfilePage /></Shell>
      case 'docs':
        return <Shell><DocsPage /></Shell>
      case 'landing':
      default:
        return <LandingPage />
    }
  }, [route, returnTo])

  return (
    <AppContext.Provider value={appValue}>
      {page}
      <ToastViewport toasts={toasts} onExpire={(id) => setToasts((items) => items.filter((toast) => toast.id !== id))} />
    </AppContext.Provider>
  )
}
