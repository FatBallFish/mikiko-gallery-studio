import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { Balance, UserProfile, UserThemePreference } from '../../shared/api-types'
import { userApi } from '../../shared/user-api'
import { AppContext, protectedRoutes, Shell, ToastViewport } from './components'
import type { RouteId, SessionState, Toast, ToastTone } from './types'
import { LandingPage } from './pages/LandingPage'
import { LoginPage } from './pages/LoginPage'
import { HomePage } from './pages/HomePage'
import { WorkspacePage } from './pages/WorkspacePage'
import { GalleryPage } from './pages/GalleryPage'
import { PublicGalleryPage } from './pages/PublicGalleryPage'
import { CheckoutPage } from './pages/CheckoutPage'
import { ApiKeysPage } from './pages/ApiKeysPage'
import { ProfilePage } from './pages/ProfilePage'
import { DocsPage } from './pages/DocsPage'
import { SettingsPage } from './pages/SettingsPage'
import { parseUserHashState, userHashForRoute } from './routeState'
import { applyThemePreference, readLocalThemePreference, serializeThemePreference, themePreferenceFromProfile, writeLocalThemePreference } from './themePreferences'

const sessionKey = 'pic-gallery-user-session'

function parseHash() {
  return parseUserHashState(window.location.hash)
}

function writeHash(route: RouteId, options?: { returnTo?: RouteId; imageId?: string | null }) {
  window.location.hash = userHashForRoute(route, options)
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
  const [routeImageId, setRouteImageId] = useState<string | undefined>(initial.imageId)
  const [session, setSession] = useState<SessionState | null>(() => readStoredSession())
  const sessionRef = useRef<SessionState | null>(session)
  const routeRef = useRef<RouteId>(initial.route)
  const expiredNoticeRef = useRef(false)
  const [profile, setProfile] = useState<UserProfile | null>(() => readStoredSession()?.profile ?? null)
  const [balance, setBalance] = useState<Balance | null>(null)
  const [toasts, setToasts] = useState<Toast[]>([])
  const [themePreference, setThemePreferenceState] = useState<UserThemePreference>(() => (
    themePreferenceFromProfile(readStoredSession()?.profile) ?? readLocalThemePreference()
  ))

  const notify = useCallback((tone: ToastTone, message: string) => {
    setToasts((items) => [...items, { id: Date.now() + Math.random(), tone, message, durationMs: 4200 }])
  }, [])

  useEffect(() => {
    sessionRef.current = session
  }, [session])

  useEffect(() => {
    routeRef.current = route
  }, [route])

  useEffect(() => {
    applyThemePreference(themePreference)
    writeLocalThemePreference(themePreference)
  }, [themePreference])

  const expireSession = useCallback(() => {
    sessionRef.current = null
    setSession(null)
    setProfile(null)
    setBalance(null)
    window.localStorage.removeItem(sessionKey)
    if (!expiredNoticeRef.current) {
      expiredNoticeRef.current = true
      notify('error', '登录已过期，需要重新登录')
    }
    const currentRoute = routeRef.current
    if (protectedRoutes.includes(currentRoute)) {
      writeHash('login', { returnTo: currentRoute, imageId: currentRoute === 'public-gallery' ? routeImageId : undefined })
    }
  }, [notify, routeImageId])

  useLayoutEffect(() => {
    userApi.configureAuth({
      getToken: () => sessionRef.current?.token,
      onUnauthorized: async () => {
        try {
          const refreshed = await userApi.refreshSession()
          const currentProfile = sessionRef.current?.profile ?? profile ?? refreshed.profile ?? {
            id: String(refreshed.user_id ?? ''),
            email: '',
            display_name: 'Mikiko User',
            avatar_initials: 'PG',
            tier: 'FREE' as const,
            group: 'DEFAULT',
            signature: '',
            preferences: { model_group: 'plus-image', quality: 'auto', aspect_ratio: '16:9', image_count: 1 },
          }
          const nextSession = { token: refreshed.access_token, profile: currentProfile }
          sessionRef.current = nextSession
          setSession(nextSession)
          window.localStorage.setItem(sessionKey, JSON.stringify(nextSession))
          expiredNoticeRef.current = false
          return refreshed.access_token
        } catch {
          expireSession()
          return null
        }
      },
      onError: (error) => {
        if (error.status === 401) expireSession()
      },
    })
  }, [expireSession, profile])

  const installSession = useCallback((nextSession: SessionState, options?: { applyProfileTheme?: boolean }) => {
    sessionRef.current = nextSession
    setSession(nextSession)
    setProfile(nextSession.profile)
    if (options?.applyProfileTheme) {
      const profileTheme = themePreferenceFromProfile(nextSession.profile)
      if (profileTheme) setThemePreferenceState(profileTheme)
    }
    window.localStorage.setItem(sessionKey, JSON.stringify(nextSession))
  }, [])

  const refreshAccount = useCallback(async () => {
    if (!sessionRef.current?.token) return
    try {
      const [nextProfile, nextBalance] = await Promise.all([userApi.getProfile(), userApi.getBalance()])
      setProfile(nextProfile)
      setBalance(nextBalance)
      installSession({ token: sessionRef.current.token, profile: nextProfile })
    } catch (caught) {
      if (caught && typeof caught === 'object' && 'status' in caught && caught.status === 401) {
        expireSession()
        return
      }
      throw caught
    }
  }, [expireSession, installSession])

  useEffect(() => {
    const updateRoute = () => {
      const parsed = parseHash()
      setRoute(parsed.route)
      setReturnTo(parsed.returnTo)
      setRouteImageId(parsed.imageId)
    }
    window.addEventListener('hashchange', updateRoute)
    if (!window.location.hash) writeHash(session ? 'home' : 'landing')
    return () => window.removeEventListener('hashchange', updateRoute)
  }, [session])

  useEffect(() => {
    if (session) void refreshAccount()
  }, [session?.token, refreshAccount])

  useEffect(() => {
    if (sessionRef.current) return
    if (!window.localStorage.getItem(sessionKey)) return
    let cancelled = false
    async function bootstrap() {
      try {
        const refreshed = await userApi.refreshSession()
        sessionRef.current = { token: refreshed.access_token, profile: profile ?? {
          id: String(refreshed.user_id ?? ''),
          email: '',
          display_name: 'Mikiko User',
          avatar_initials: 'PG',
          tier: 'FREE',
          group: 'DEFAULT',
          signature: '',
          preferences: { model_group: 'plus-image', quality: 'auto', aspect_ratio: '16:9', image_count: 1 },
        } }
        const nextProfile = await userApi.getProfile()
        if (cancelled) return
        installSession({ token: refreshed.access_token, profile: nextProfile }, { applyProfileTheme: true })
        setBalance(await userApi.getBalance())
      } catch {
        window.localStorage.removeItem(sessionKey)
      }
    }
    void bootstrap()
    return () => { cancelled = true }
  }, [installSession, profile])

  useEffect(() => {
    if (!session && protectedRoutes.includes(route)) {
      writeHash('login', { returnTo: route, imageId: route === 'public-gallery' ? routeImageId : undefined })
    }
    if (session && route === 'login') {
      writeHash(returnTo ?? 'home', { imageId: returnTo === 'public-gallery' ? routeImageId : undefined })
    }
  }, [route, returnTo, routeImageId, session])

  const navigate = useCallback((next: RouteId, options?: { returnTo?: RouteId; imageId?: string | null }) => {
    writeHash(next, options)
  }, [])

  const setThemePreference = useCallback(async (patch: Partial<UserThemePreference>) => {
    const next = { ...themePreference, ...patch } as UserThemePreference
    setThemePreferenceState(next)
    writeLocalThemePreference(next)
    applyThemePreference(next)
    if (!sessionRef.current?.token) return
    try {
      const nextProfile = await userApi.updatePreferences({
        theme: serializeThemePreference(next),
        theme_mode: next.mode,
        accent_theme: next.accent,
      })
      setProfile(nextProfile)
      sessionRef.current = { token: sessionRef.current.token, profile: nextProfile }
      setSession(sessionRef.current)
      window.localStorage.setItem(sessionKey, JSON.stringify(sessionRef.current))
    } catch {
      notify('error', '主题偏好已应用到本机，但暂未同步到账户')
    }
  }, [installSession, notify, themePreference])

  const login = useCallback(async (nextSession: SessionState, target?: RouteId, options?: { imageId?: string | null }) => {
    expiredNoticeRef.current = false
    installSession(nextSession, { applyProfileTheme: true })
    notify('success', '登录成功，欢迎回到 Mikiko Studio')
    const destination = target ?? returnTo ?? 'home'
    writeHash(destination, { imageId: destination === 'public-gallery' ? options?.imageId ?? routeImageId : undefined })
  }, [installSession, notify, returnTo, routeImageId])

  const logout = useCallback(async () => {
    await userApi.logout().catch(() => undefined)
    window.localStorage.removeItem(sessionKey)
    setSession(null)
    setProfile(null)
    setBalance(null)
    notify('info', '已退出登录')
    writeHash('login')
  }, [notify])

  const appValue = useMemo(() => ({
    route,
    isAuthenticated: Boolean(session),
    session,
    profile,
    balance,
    themePreference,
    setThemePreference,
    refreshAccount,
    navigate,
    login,
    logout,
    notify,
  }), [route, session, profile, balance, themePreference, setThemePreference, refreshAccount, navigate, login, logout, notify])

  const page = useMemo(() => {
    if (!session && protectedRoutes.includes(route)) {
      return <LoginPage returnTo={route} imageId={route === 'public-gallery' ? routeImageId : undefined} />
    }
    switch (route) {
      case 'login':
        return <LoginPage returnTo={returnTo} imageId={returnTo === 'public-gallery' ? routeImageId : undefined} />
      case 'home':
        return <Shell><HomePage /></Shell>
      case 'genpic':
        return <Shell><WorkspacePage /></Shell>
      case 'gallery':
        return <Shell><GalleryPage /></Shell>
      case 'public-gallery':
        return <Shell><PublicGalleryPage imageId={routeImageId} /></Shell>
      case 'checkout':
        return <Shell><CheckoutPage /></Shell>
      case 'api-keys':
        return <Shell><ApiKeysPage /></Shell>
      case 'profile':
        return <Shell scrollMode="document"><ProfilePage /></Shell>
      case 'docs':
        return <Shell><DocsPage /></Shell>
      case 'settings':
        return <Shell><SettingsPage /></Shell>
      case 'landing':
      default:
        return <LandingPage />
    }
  }, [route, returnTo, routeImageId, session])

  return (
    <AppContext.Provider value={appValue}>
      {page}
      <ToastViewport toasts={toasts} onExpire={(id) => setToasts((items) => items.filter((toast) => toast.id !== id))} />
    </AppContext.Provider>
  )
}
