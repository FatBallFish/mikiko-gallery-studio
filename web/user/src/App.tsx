import { lazy, Suspense, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { Balance, UserProfile, UserThemePreference } from '../../shared/api-types'
import { useBootstrapGuard } from './bootstrapGuard'
import { userApi } from '../../shared/user-api'
import { AppContext, protectedRoutes, Shell, ToastViewport } from './components'
import type { RouteId, SessionState, Toast, ToastTone } from './types'
import { LoginPage } from './pages/LoginPage'
import { HomePage } from './pages/HomePage'
import { WorkspacePage } from './pages/WorkspacePage'
import { GalleryPage } from './pages/GalleryPage'
import { PublicGalleryPage } from './pages/PublicGalleryPage'
import { CheckoutPage } from './pages/CheckoutPage'
import { ApiKeysPage } from './pages/ApiKeysPage'
import { ProfilePage } from './pages/ProfilePage'
import { SettingsPage } from './pages/SettingsPage'
import { ProjectsPage } from './pages/ProjectsPage'
import { ProjectProvider } from './ProjectContext'
import { parseUserHashState, userHashForRoute, type UserRouteOptions } from './routeState'
import { applyThemePreference, readLocalThemePreference, serializeThemePreference, themePreferenceFromProfile, writeLocalThemePreference } from './themePreferences'

const sessionKey = 'pic-gallery-user-session'
const LandingPage = lazy(() => import('./pages/LandingPage'))

function parseHash() {
  return parseUserHashState(window.location.hash)
}

function writeHash(route: RouteId, options?: UserRouteOptions) {
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
  const bootstrap = useBootstrapGuard()

  if (bootstrap.phase === 'ready') return <UserApplication />
  if (bootstrap.phase === 'broken') {
    return <BootstrapFailure message={`服务初始化配置异常${bootstrap.diagnostic_code ? ` (${bootstrap.diagnostic_code})` : ''}，请联系运维人员处理。`} onRetry={bootstrap.retry} />
  }
  if (bootstrap.phase === 'error') {
    return <BootstrapFailure message="暂时无法确认服务初始化状态，请检查 API 地址或网络连接后重试。" onRetry={bootstrap.retry} />
  }
  return <div className="grid min-h-screen place-items-center bg-[var(--bg)] text-sm text-[var(--muted)]" role="status">正在检查服务状态...</div>
}

function BootstrapFailure({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <main className="grid min-h-screen place-items-center bg-[var(--bg)] p-6">
      <section className="w-full max-w-lg border border-[var(--border)] bg-[var(--surface)] p-6 text-[var(--fg)]" role="alert">
        <h1 className="text-lg font-semibold">服务暂不可用</h1>
        <p className="mt-2 text-sm leading-6 text-[var(--muted)]">{message}</p>
        <button className="mt-5 min-h-10 border border-[var(--border)] px-4 text-sm hover:border-[var(--accent)]" type="button" onClick={onRetry}>重试</button>
      </section>
    </main>
  )
}

function UserApplication() {
  const initial = parseHash()
  const [route, setRoute] = useState<RouteId>(initial.route)
  const [returnTo, setReturnTo] = useState<RouteId | undefined>(initial.returnTo)
  const [routeImageId, setRouteImageId] = useState<string | undefined>(initial.imageId)
  const [routeTaskId, setRouteTaskId] = useState<string | undefined>(initial.taskId)
  const [session, setSession] = useState<SessionState | null>(() => readStoredSession())
  const sessionRef = useRef<SessionState | null>(session)
  const sessionVersionRef = useRef(0)
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
    sessionVersionRef.current += 1
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
      writeHash('login', {
        returnTo: currentRoute,
        imageId: currentRoute === 'public-gallery' ? routeImageId : undefined,
        taskId: currentRoute === 'genpic' ? routeTaskId : undefined,
      })
    }
  }, [notify, routeImageId, routeTaskId])

  useLayoutEffect(() => {
    userApi.configureAuth({
      getToken: () => sessionRef.current?.token,
      getSessionVersion: () => sessionVersionRef.current,
      onUnauthorized: async () => {
        const refreshSessionVersion = sessionVersionRef.current
        try {
          const refreshed = await userApi.refreshSession()
          if (sessionVersionRef.current !== refreshSessionVersion) return null
          const currentProfile = sessionRef.current?.profile ?? profile ?? refreshed.profile ?? {
            id: String(refreshed.user_id ?? ''),
            email: '',
            has_password: false,
            display_name: 'Mikiko User',
            avatar_initials: 'PG',
            tier: 'FREE' as const,
            group: 'DEFAULT',
            signature: '',
            preferences: { model_group: 'plus-image', base_resolution: 'auto', quality: 'auto', aspect_ratio: '16:9', image_count: 1 },
          }
          const nextSession = { token: refreshed.access_token, profile: currentProfile }
          sessionRef.current = nextSession
          setSession(nextSession)
          window.localStorage.setItem(sessionKey, JSON.stringify(nextSession))
          expiredNoticeRef.current = false
          return refreshed.access_token
        } catch {
          if (sessionVersionRef.current === refreshSessionVersion) expireSession()
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
    const currentSession = sessionRef.current
    if (!currentSession?.token) return
    const refreshAccountVersion = sessionVersionRef.current
    try {
      const [nextProfile, nextBalance] = await Promise.all([userApi.getProfile(), userApi.getBalance()])
      if (sessionVersionRef.current !== refreshAccountVersion) return
      const latestToken = sessionRef.current?.token
      if (!latestToken) return
      setProfile(nextProfile)
      setBalance(nextBalance)
      installSession({ token: latestToken, profile: nextProfile })
    } catch (caught) {
      if (sessionVersionRef.current !== refreshAccountVersion) return
      if (caught && typeof caught === 'object' && 'status' in caught && caught.status === 401) {
        if (sessionVersionRef.current === refreshAccountVersion) expireSession()
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
      setRouteTaskId(parsed.taskId)
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
          has_password: false,
          display_name: 'Mikiko User',
          avatar_initials: 'PG',
          tier: 'FREE',
          group: 'DEFAULT',
          signature: '',
          preferences: { model_group: 'plus-image', base_resolution: 'auto', quality: 'auto', aspect_ratio: '16:9', image_count: 1 },
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
      writeHash('login', {
        returnTo: route,
        imageId: route === 'public-gallery' ? routeImageId : undefined,
        taskId: route === 'genpic' ? routeTaskId : undefined,
      })
    }
    if (session && route === 'login') {
      writeHash(returnTo ?? 'home', {
        imageId: returnTo === 'public-gallery' ? routeImageId : undefined,
        taskId: returnTo === 'genpic' ? routeTaskId : undefined,
      })
    }
  }, [route, returnTo, routeImageId, routeTaskId, session])

  const navigate = useCallback((next: RouteId, options?: UserRouteOptions) => {
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

  const login = useCallback(async (nextSession: SessionState, target?: RouteId, options?: { imageId?: string | null; taskId?: string | null }) => {
    sessionVersionRef.current += 1
    expiredNoticeRef.current = false
    installSession(nextSession, { applyProfileTheme: true })
    notify('success', '登录成功，欢迎回到 Mikiko Studio')
    const destination = target ?? returnTo ?? 'home'
    writeHash(destination, {
      imageId: destination === 'public-gallery' ? options?.imageId ?? routeImageId : undefined,
      taskId: destination === 'genpic' ? options?.taskId ?? routeTaskId : undefined,
    })
  }, [installSession, notify, returnTo, routeImageId, routeTaskId])

  const logout = useCallback(async () => {
    const logoutRequest = userApi.logout().catch(() => undefined)
    sessionVersionRef.current += 1
    sessionRef.current = null
    window.localStorage.removeItem(sessionKey)
    setSession(null)
    setProfile(null)
    setBalance(null)
    notify('info', '已退出登录')
    writeHash('login')
    await logoutRequest
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
      return <LoginPage returnTo={route} imageId={route === 'public-gallery' ? routeImageId : undefined} taskId={route === 'genpic' ? routeTaskId : undefined} />
    }
    switch (route) {
      case 'login':
        return <LoginPage returnTo={returnTo} imageId={returnTo === 'public-gallery' ? routeImageId : undefined} taskId={returnTo === 'genpic' ? routeTaskId : undefined} />
      case 'home':
        return <Shell><HomePage /></Shell>
      case 'genpic':
        return <Shell><WorkspacePage initialTaskId={routeTaskId} /></Shell>
      case 'gallery':
        return <Shell><GalleryPage /></Shell>
      case 'projects':
        return <Shell scrollMode="document"><ProjectsPage /></Shell>
      case 'public-gallery':
        return <Shell><PublicGalleryPage imageId={routeImageId} /></Shell>
      case 'checkout':
        return <Shell><CheckoutPage /></Shell>
      case 'api-keys':
        return <Shell><ApiKeysPage /></Shell>
      case 'profile':
        return <Shell scrollMode="document"><ProfilePage /></Shell>
      case 'settings':
        return <Shell><SettingsPage /></Shell>
      case 'landing':
      default:
        return (
          <Suspense fallback={<main className="min-h-screen bg-[var(--bg)]" aria-label="正在载入首页" />}>
            <LandingPage />
          </Suspense>
        )
    }
  }, [route, returnTo, routeImageId, routeTaskId, session])

  const projectUserID = String(profile?.id || session?.profile.id || '')

  return (
    <AppContext.Provider value={appValue}>
      {session && projectUserID ? (
        <ProjectProvider key={projectUserID} userID={projectUserID}>{page}</ProjectProvider>
      ) : page}
      <ToastViewport toasts={toasts} onExpire={(id) => setToasts((items) => items.filter((toast) => toast.id !== id))} />
    </AppContext.Provider>
  )
}
