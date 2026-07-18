import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { AdminMetric, AdminSession, ProviderHealth } from '../../shared/api-types'
import { adminApi } from '../../shared/admin-api'
import { EmptyBlock, ToastRail, useHashRoute, useToasts } from './components'
import { AdminLayout, normalizeRoute, protectedRoutes, routeHref } from './layout/AdminLayout'
import { AuditPage, CallRecordsPage, CashierPage, LoginPage, MonitoringPage, OrdersPage, OverviewPage, PackagesPage, PricingPage, ProviderModelsPage, RedeemPage, ReviewPage, RoutingPage, SystemSettingsPage, SystemUsersPage, UserGroupsPage, UsersPage } from './pages/index'
import { canAccessAdminRoute, firstAccessibleAdminRoute } from './types'
import type { AdminRouteId, ProtectedAdminRouteId } from './types'
import { useAdminPageMotion } from './ui/adminMotion'

const sessionKey = 'pic_gallery_admin_session'
const returnKey = 'pic_gallery_admin_return'

function readStoredSession(): AdminSession | null {
  try {
    const raw = window.sessionStorage.getItem(sessionKey)
    return raw ? JSON.parse(raw) as AdminSession : null
  } catch {
    return null
  }
}

export default function App() {
  const [route, setRoute] = useHashRoute()
  const [session, setSession] = useState<AdminSession | null>(() => readStoredSession())
  const [sessionResolved, setSessionResolved] = useState(() => session !== null)
  const sessionRef = useRef<AdminSession | null>(session)
  const sessionRefreshRef = useRef<Promise<AdminSession> | null>(null)
  const authEpochRef = useRef(0)
  const logoutPendingRef = useRef(false)
  const [logoutPending, setLogoutPending] = useState(false)
  const [shellMetrics, setShellMetrics] = useState<AdminMetric[]>([])
  const [shellProviders, setShellProviders] = useState<ProviderHealth[]>([])
  const [reviewCount, setReviewCount] = useState(0)
  const [configDrafts, setConfigDrafts] = useState(0)
  const { toasts, pushToast, dismissToast } = useToasts()
  const isAuthed = Boolean(session)

  const expireSession = useCallback(() => {
    const hadSession = Boolean(sessionRef.current)
    authEpochRef.current++
    window.sessionStorage.removeItem(sessionKey)
    sessionRef.current = null
    setSession(null)
    setRoute('login')
    if (hadSession) {
      pushToast({ tone: 'danger', title: '登录已过期', detail: '请重新登录后继续操作。' })
    }
  }, [pushToast, setRoute])

  useLayoutEffect(() => {
    adminApi.configureAuth({
      getToken: () => sessionRef.current?.token,
      getSessionVersion: () => authEpochRef.current,
      onError: (error) => {
        if (error.status === 401) {
          expireSession()
          return
        }
        pushToast({ tone: 'danger', title: '接口调用失败', detail: error.message })
      },
      onUnauthorized: async () => {
        const refreshEpoch = authEpochRef.current
        const expectedSession = sessionRef.current
        try {
          const refreshed = await adminApi.refreshSession()
          if (!expectedSession || refreshEpoch !== authEpochRef.current || sessionRef.current !== expectedSession) {
            return undefined
          }
          sessionRef.current = refreshed
          setSession(refreshed)
          window.sessionStorage.setItem(sessionKey, JSON.stringify(refreshed))
          return refreshed.token
        } catch {
          if (refreshEpoch === authEpochRef.current && sessionRef.current === expectedSession) {
            expireSession()
          }
          return undefined
        }
      },
    })
  }, [expireSession, pushToast])

  useEffect(() => {
    if (sessionResolved) return
    if (!sessionRefreshRef.current) {
      sessionRefreshRef.current = adminApi.refreshSession()
    }
    const refreshEpoch = authEpochRef.current
    let active = true
    void sessionRefreshRef.current.then((refreshed) => {
      if (!active || refreshEpoch !== authEpochRef.current) return
      authEpochRef.current++
      sessionRef.current = refreshed
      setSession(refreshed)
      window.sessionStorage.setItem(sessionKey, JSON.stringify(refreshed))
    }).catch(() => undefined).finally(() => {
      if (active) setSessionResolved(true)
    })
    return () => {
      active = false
    }
  }, [sessionResolved])

  const refreshShell = async () => {
    if (!session) return
    const [dashboard, config] = await Promise.all([adminApi.dashboard(), adminApi.listConfig()])
    setShellMetrics(dashboard.metrics)
    setShellProviders(dashboard.providers)
    setReviewCount(Number(dashboard.queue.find((item: { item: string; count: string }) => item.item.includes('审核'))?.count ?? 0))
    setConfigDrafts(config.filter((item) => item.state !== 'active').length)
  }

  useEffect(() => {
    if (!isAuthed && route !== 'login') {
      window.sessionStorage.setItem(returnKey, route)
      setRoute('login')
      return
    }
    if (isAuthed && route === 'login') {
      setRoute('dashboard')
    }
  }, [isAuthed, route, setRoute])

  useEffect(() => {
    if (!session || route === 'login' || canAccessAdminRoute(session, route)) return
    const fallbackRoute = firstAccessibleAdminRoute(session)
    if (fallbackRoute !== route && canAccessAdminRoute(session, fallbackRoute)) {
      setRoute(fallbackRoute)
      pushToast({ tone: 'warning', title: '暂无该页面权限', detail: '已为你切换到可访问的后台页面。' })
    }
  }, [session, route, setRoute, pushToast])

  useEffect(() => {
    void refreshShell().catch(() => {
      setShellProviders([])
      setShellMetrics([])
    })
  }, [session?.admin_id])

  const feedback = (title: string, detail?: string) => {
    pushToast({ tone: 'success', title, detail })
    void refreshShell()
  }

  const handleLogin = (nextSession: AdminSession) => {
    authEpochRef.current++
    sessionRef.current = nextSession
    setSession(nextSession)
    setSessionResolved(true)
    window.sessionStorage.setItem(sessionKey, JSON.stringify(nextSession))
    const returnRoute = normalizeRoute(`#/${window.sessionStorage.getItem(returnKey) ?? 'dashboard'}`)
    window.sessionStorage.removeItem(returnKey)
    setRoute(returnRoute === 'login' ? 'dashboard' : returnRoute)
    pushToast({ tone: 'success', title: '管理员已登录', detail: nextSession.admin_name })
  }

  const handleLogout = () => {
    if (logoutPendingRef.current) return
    const logoutSession = sessionRef.current
    authEpochRef.current++
    logoutPendingRef.current = true
    setLogoutPending(true)
    void adminApi.logout().then(() => {
      if (sessionRef.current !== logoutSession) return
      sessionRef.current = null
      window.sessionStorage.removeItem(sessionKey)
      setSession(null)
      setRoute('login')
      pushToast({ tone: 'neutral', title: '已退出后台' })
    }).catch((error) => {
      if (sessionRef.current !== logoutSession) return
      pushToast({
        tone: 'danger',
        title: '退出未完成',
        detail: error instanceof Error && error.name !== 'AbortError' ? error.message : '服务未确认退出，请检查网络后重试。',
      })
    }).finally(() => {
      logoutPendingRef.current = false
      setLogoutPending(false)
    })
  }

  const page = useMemo(() => {
    if (session && route !== 'login' && !canAccessAdminRoute(session, route)) {
      return <EmptyBlock title="暂无后台权限" detail="当前管理员角色没有可访问的后台页面，请联系超级管理员调整权限。" />
    }
    switch (route) {
      case 'dashboard':
        return <OverviewPage />
      case 'monitoring':
        return <MonitoringPage />
      case 'routing':
        return <RoutingPage onFeedback={feedback} />
      case 'pricing':
        return <PricingPage onFeedback={feedback} />
      case 'reviews':
        return <ReviewPage accessToken={session?.token} onFeedback={feedback} />
      case 'users':
        return <UsersPage onFeedback={feedback} />
      case 'user-groups':
        return <UserGroupsPage onFeedback={feedback} />
      case 'redeem':
        return <RedeemPage onFeedback={feedback} />
      case 'orders':
        return <OrdersPage onFeedback={feedback} />
      case 'packages':
        return <PackagesPage onFeedback={feedback} />
      case 'cashier-config':
        return <CashierPage onFeedback={feedback} initialTab="overview" allowedTabs={['overview', 'methods', 'instances']} pageTitle="收银台配置" />
      case 'call-records':
        return <CallRecordsPage />
      case 'access-accounts':
        return <ProviderModelsPage accessToken={session?.token} />
      case 'audit':
        return <AuditPage onFeedback={feedback} />
      case 'system-users':
        return session ? <SystemUsersPage session={session} onFeedback={feedback} /> : null
      case 'system-settings':
        return session ? <SystemSettingsPage session={session} onFeedback={feedback} /> : null
      default:
        return <OverviewPage />
    }
  }, [route, session])

  if (!sessionResolved || logoutPending) {
    return <div className="grid min-h-screen place-items-center bg-[var(--bg)] text-sm text-[var(--muted-strong)]" role="status">{logoutPending ? '正在安全退出...' : '正在恢复登录状态...'}</div>
  }

  if (route === 'login' || !session) {
    return (
      <>
        <LoginPage onLogin={handleLogin} />
        <ToastRail toasts={toasts} onDismiss={dismissToast} />
      </>
    )
  }

  const protectedRoute = protectedRoutes.includes(route as ProtectedAdminRouteId) ? route as ProtectedAdminRouteId : 'dashboard'

  return (
    <>
      <AdminLayout
        route={protectedRoute}
        session={session}
        metrics={shellMetrics}
        providers={shellProviders}
        reviewCount={reviewCount}
        configDrafts={configDrafts}
        onNavigate={(nextRoute) => {
          window.location.href = routeHref(nextRoute)
        }}
        onLogout={handleLogout}
      >
        <AdminRouteContent routeKey={protectedRoute}>{page}</AdminRouteContent>
      </AdminLayout>
      <ToastRail toasts={toasts} onDismiss={dismissToast} />
    </>
  )
}

function AdminRouteContent({ routeKey, children }: { routeKey: string; children: React.ReactNode }) {
  const motionRef = useRef<HTMLDivElement | null>(null)
  useAdminPageMotion(motionRef, routeKey)
  return <div ref={motionRef} className="min-h-full" data-admin-motion-root><div data-admin-motion-item>{children}</div></div>
}
