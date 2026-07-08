import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { AdminMetric, AdminSession, ProviderHealth } from '../../shared/api-types'
import { adminApi } from '../../shared/admin-api'
import { EmptyBlock, ToastRail, useHashRoute, useToasts } from './components'
import { AdminLayout, normalizeRoute, protectedRoutes, routeHref } from './layout/AdminLayout'
import { AuditPage, CallRecordsPage, CashierPage, LoginPage, MonitoringPage, OrdersPage, OverviewPage, PackagesPage, PricingPage, ProviderModelsPage, RedeemPage, ReviewPage, RoutingPage, SystemSettingsPage, SystemUsersPage, UserGroupsPage, UsersPage } from './pages/index'
import { canAccessAdminRoute, firstAccessibleAdminRoute } from './types'
import type { AdminRouteId, ProtectedAdminRouteId } from './types'

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
  const sessionRef = useRef<AdminSession | null>(session)
  const [shellMetrics, setShellMetrics] = useState<AdminMetric[]>([])
  const [shellProviders, setShellProviders] = useState<ProviderHealth[]>([])
  const [reviewCount, setReviewCount] = useState(0)
  const [configDrafts, setConfigDrafts] = useState(0)
  const { toasts, pushToast, dismissToast } = useToasts()
  const isAuthed = Boolean(session)

  useLayoutEffect(() => {
    sessionRef.current = session
    adminApi.configureAuth({
      getToken: () => sessionRef.current?.token,
      onError: (error) => {
        pushToast({ tone: 'danger', title: '接口调用失败', detail: error.message })
      },
      onUnauthorized: () => {
        if (sessionRef.current) {
          window.sessionStorage.removeItem(sessionKey)
          sessionRef.current = null
          setSession(null)
          setRoute('login')
          pushToast({ tone: 'danger', title: '登录已过期', detail: '请重新登录后继续操作。' })
        }
        return undefined
      },
    })
  }, [session, pushToast, setRoute])

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
  }, [session])

  const feedback = (title: string, detail?: string) => {
    pushToast({ tone: 'success', title, detail })
    void refreshShell()
  }

  const handleLogin = (nextSession: AdminSession) => {
    sessionRef.current = nextSession
    setSession(nextSession)
    window.sessionStorage.setItem(sessionKey, JSON.stringify(nextSession))
    const returnRoute = normalizeRoute(`#/${window.sessionStorage.getItem(returnKey) ?? 'dashboard'}`)
    window.sessionStorage.removeItem(returnKey)
    setRoute(returnRoute === 'login' ? 'dashboard' : returnRoute)
    pushToast({ tone: 'success', title: '管理员已登录', detail: nextSession.admin_name })
  }

  const handleLogout = () => {
    void adminApi.logout().catch(() => undefined)
    window.sessionStorage.removeItem(sessionKey)
    setSession(null)
    setRoute('login')
    pushToast({ tone: 'neutral', title: '已退出后台' })
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
        {page}
      </AdminLayout>
      <ToastRail toasts={toasts} onDismiss={dismissToast} />
    </>
  )
}
