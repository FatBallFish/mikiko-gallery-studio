import React, { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import type { AdminMetric, AdminSession, ProviderHealth, UserGroup } from '../../shared/api-types'
import { cn } from '../../shared/classnames'
import { navGroups, normalizeRoute, protectedRoutes, routeHref, routeTitles } from './layout/admin-navigation'
import { useAdminTheme } from './layout/useAdminTheme'
import { filterAdminNavGroups } from './types'
import type { AdminRouteId, ProtectedAdminRouteId, ToastMessage, ToastTone } from './types'
import { adminButton, adminShell } from './ui/classes'

export { navGroups, normalizeRoute, protectedRoutes, routeHref } from './layout/admin-navigation'

const badgeToneClass: Record<ToastTone | 'success' | 'primary' | 'neutral', string> = {
  success: 'bg-[rgba(90,149,114,.12)] text-[var(--green)]',
  warning: 'bg-[rgba(184,135,64,.13)] text-[var(--amber)]',
  danger: 'bg-[rgba(184,95,84,.13)] text-[var(--red)]',
  primary: 'bg-[rgba(87,117,185,.12)] text-[var(--blue)]',
  neutral: 'bg-[rgba(104,120,139,.12)] text-[var(--soft)]',
}

const feedbackToneClass: Record<ToastTone, string> = {
  success: 'border-[rgba(90,149,114,.28)] bg-[rgba(90,149,114,.08)] text-[var(--green)]',
  warning: 'border-[rgba(184,135,64,.3)] bg-[rgba(184,135,64,.08)] text-[var(--amber)]',
  danger: 'border-[rgba(184,95,84,.3)] bg-[rgba(184,95,84,.08)] text-[var(--red)]',
  neutral: 'border-[rgba(87,117,185,.24)] bg-[rgba(87,117,185,.07)] text-[var(--blue)]',
}

const toastToneClass: Record<ToastTone, string> = {
  success: 'border-l-4 border-l-[var(--green)]',
  warning: 'border-l-4 border-l-[var(--amber)]',
  danger: 'border-l-4 border-l-[var(--red)]',
  neutral: 'border-l-4 border-l-[var(--blue)]',
}

const dotToneClass = {
  success: 'bg-[var(--green)]',
  warning: 'bg-[var(--amber)]',
  danger: 'bg-[var(--red)]',
  primary: 'bg-[var(--blue)]',
}

const navIconClass = 'size-5 opacity-70 transition-opacity group-hover:opacity-100'
const stateBlockBase = 'grid min-h-[260px] place-items-center content-center gap-2.5 rounded-3xl border border-dashed border-white/10 bg-white/[0.02] p-7 text-center text-white/70'
const fieldLabelClass = 'flex items-center justify-between gap-2 text-[10px] font-extrabold uppercase tracking-[.14em] text-[var(--soft)]'
const checkGridClass = 'grid max-h-[220px] gap-2 overflow-auto rounded-2xl border border-[var(--line)] bg-white/[0.02] p-2'
const checkGridEmptyClass = 'grid-cols-1 text-sm font-bold text-[var(--soft)]'
const checkOptionClass = 'grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 rounded-xl border border-[var(--line)] bg-white/5 p-2 text-sm has-[:checked]:border-[var(--accent)]/40 has-[:checked]:bg-[var(--accent)]/10'
const checkOptionNameClass = 'min-w-0 overflow-hidden text-ellipsis whitespace-nowrap'
const checkOptionMetaClass = 'text-xs not-italic font-extrabold text-[var(--soft)]'
const metricToneClass: Record<string, string> = {
  good: '[&_span]:text-[var(--green)]',
  warn: '[&_span]:text-[var(--amber)]',
  bad: '[&_span]:text-[var(--red)]',
  danger: '[&_span]:text-[var(--red)]',
}

export function useHashRoute() {
  const [route, setRouteState] = useState<AdminRouteId>(() => normalizeRoute(window.location.hash))

  useEffect(() => {
    const onHashChange = () => setRouteState(normalizeRoute(window.location.hash))
    window.addEventListener('hashchange', onHashChange)
    return () => window.removeEventListener('hashchange', onHashChange)
  }, [])

  const setRoute = (next: AdminRouteId) => {
    if (normalizeRoute(window.location.hash) === next) {
      setRouteState(next)
      return
    }
    window.location.hash = `/${next}`
  }

  return [route, setRoute] as const
}

export function useToasts() {
  const [toasts, setToasts] = useState<ToastMessage[]>([])

  const pushToast = (toast: Omit<ToastMessage, 'id'>) => {
    const id = `${Date.now()}_${Math.random().toString(36).slice(2, 7)}`
    setToasts((current) => [{ ...toast, id }, ...current].slice(0, 4))
    window.setTimeout(() => {
      setToasts((current) => current.filter((item) => item.id !== id))
    }, 5200)
  }

  const dismissToast = (id: string) => setToasts((current) => current.filter((item) => item.id !== id))

  return { toasts, pushToast, dismissToast }
}

export function AdminLayout({
  route,
  session,
  metrics,
  providers,
  reviewCount,
  configDrafts,
  children,
  onNavigate,
  onLogout,
}: {
  route: ProtectedAdminRouteId
  session: AdminSession
  metrics: AdminMetric[]
  providers: ProviderHealth[]
  reviewCount: number
  configDrafts: number
  children: React.ReactNode
  onNavigate: (route: AdminRouteId) => void
  onLogout: () => void
}) {
  const visibleNavGroups = filterAdminNavGroups(navGroups, session)
  const { theme, setTheme } = useAdminTheme()
  const navBadges = {
    review_count: reviewCount > 0 ? String(reviewCount) : '',
    failed_webhook_count: '',
    config_drafts: configDrafts > 0 ? String(configDrafts) : '',
  }

  return (
    <main className={adminShell.root} data-theme={theme}>
      <aside className={adminShell.sidebar} aria-label="Pic Gallery Admin Navigation">
        <a className={adminShell.brand} href={routeHref('dashboard')} onClick={() => onNavigate('dashboard')}>
          <span className={adminShell.brandOrb}>M</span>
          <strong className={adminShell.brandText}>Mikiko Admin</strong>
        </a>

        <nav className={adminShell.nav} aria-label="后台主导航">
          {visibleNavGroups.map((group) => (
            <section key={group.label} className={adminShell.navGroup}>
              <p className={adminShell.navLabel}>{group.label}</p>
              {group.items.map((item) => {
                const badge = item.badgeKey ? navBadges[item.badgeKey] : ''
                return (
                  <a
                    key={item.id}
                    href={routeHref(item.id)}
                    className={cn(adminShell.navLink, route === item.id && adminShell.navLinkActive)}
                    onClick={() => onNavigate(item.id)}
                    aria-current={route === item.id ? 'page' : undefined}
                  >
                    <span className={adminShell.navIcon}>{routeIcon(item.id)}</span>
                    <span>{item.label}</span>
                    {badge ? <em className={adminShell.navBadge}>{badge}</em> : null}
                  </a>
                )
              })}
            </section>
          ))}
        </nav>

        <div className={adminShell.sideNote}>
          <span className={adminShell.avatarOrb}>AD</span>
          <div className="grid min-w-0 flex-1 gap-0.5">
            <strong className="truncate text-sm text-[var(--text)]">Admin</strong>
            <span className="truncate text-[10px] text-[var(--muted-strong)]">Super Administrator</span>
          </div>
          <button className={cn(adminButton.base, adminButton.ghost, adminButton.small, 'shrink-0')} type="button" onClick={onLogout}>退出</button>
        </div>
      </aside>

      <section className={adminShell.main}>
        <header className={adminShell.topbar}>
          <div className={adminShell.titleBlock}>
            <h1 className={adminShell.pageTitle}>{routeTitles[route]}</h1>
            <div className={adminShell.breadcrumb}>
              <span>Admin</span>
              <ChevronRightIcon className="size-3" />
              <strong>{routeTitles[route]}</strong>
            </div>
          </div>

          <div className={adminShell.metaRow}>
            <div className={adminShell.providerPill} title="系统状态">
              <span className={cn('inline-block size-2 rounded-full', dotToneClass.success)} />
              <em>System Online</em>
            </div>
            <button
              className={adminShell.iconButton}
              type="button"
              aria-label={theme === 'dark' ? '切换到亮色模式' : '切换到暗色模式'}
              title={theme === 'dark' ? '切换到亮色模式' : '切换到暗色模式'}
              onClick={() => setTheme((current) => current === 'dark' ? 'light' : 'dark')}
            >
              {theme === 'dark' ? <SunIcon /> : <MoonIcon />}
            </button>
            <button className={adminShell.iconButton} type="button" aria-label="通知" title="通知">
              <BellIcon />
            </button>
          </div>
        </header>

        <section className={adminShell.content}>
          {children}
        </section>
      </section>
    </main>
  )
}

export function StatusItem({ label, value, warn = false }: { label: string; value: string; warn?: boolean }) {
  return (
    <div className={adminShell.chip}>
      <span className={cn('inline-block size-2 rounded-full', warn ? dotToneClass.warning : dotToneClass.success)} />
      <em>{label}: {value}</em>
    </div>
  )
}

export function StatusChip({ tone, label, value }: { tone: ToastTone | 'primary' | 'success'; label: string; value: string }) {
  return (
    <button type="button" className={cn(adminShell.chip, badgeToneClass[tone])}>
      <span className={cn('inline-block size-2 rounded-full', dotToneClass[tone === 'neutral' ? 'primary' : tone])} />
      <em>{label}</em>
      <strong>{value}</strong>
    </button>
  )
}

export function StatusCell({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="relative grid min-h-[118px] content-center gap-2 overflow-hidden rounded-3xl border border-white/5 bg-white/[0.02] p-6 transition-all hover:border-white/10 hover:bg-white/[0.04]">
      <label className="m-0 text-xs font-medium uppercase tracking-wider text-[var(--muted-strong)]">{label}</label>
      <strong className="block truncate text-3xl font-black tracking-tighter text-[var(--text)]">{value}</strong>
    </div>
  )
}

export function StatusStrip({ children, columns = 5 }: { children: React.ReactNode; columns?: 4 | 5 }) {
  const columnClass = columns === 4
    ? 'grid-cols-1 md:grid-cols-2 xl:grid-cols-4'
    : 'grid-cols-1 md:grid-cols-2 xl:grid-cols-5'

  return (
    <section className={cn('grid gap-6', columnClass)} aria-label="运营状态条">
      {children}
    </section>
  )
}

export function ToastRail({ toasts, onDismiss }: { toasts: ToastMessage[]; onDismiss: (id: string) => void }) {
  return (
    <aside className="fixed right-5 top-5 z-[120] grid w-[min(380px,calc(100vw-40px))] gap-2" aria-live="polite" aria-label="操作反馈">
      {toasts.map((toast) => (
        <button key={toast.id} type="button" className={cn('grid rounded-xl border border-[var(--line)] bg-[var(--surface)] p-3 text-left shadow-[var(--pg-shadow-sm)]', toastToneClass[toast.tone])} onClick={() => onDismiss(toast.id)}>
          <strong>{toast.title}</strong>
          {toast.detail ? <span>{toast.detail}</span> : null}
        </button>
      ))}
    </aside>
  )
}

export function PageHeader({ eyebrow, title, detail, actions }: { eyebrow: string; title: string; detail?: string; actions?: React.ReactNode }) {
  return (
    <section className="flex items-end justify-between gap-4 max-[920px]:flex-col max-[920px]:items-start">
      <div>
        <label className="m-0 text-[10px] font-extrabold uppercase tracking-[.18em] text-[var(--muted-strong)]">{eyebrow}</label>
        <strong className="mt-2 flex items-center gap-3 text-sm font-bold uppercase tracking-[0.15em] text-[var(--muted-strong)] before:h-px before:w-6 before:bg-[var(--accent)]">{title}</strong>
        {detail ? <p className="mt-1 max-w-[76ch] text-sm text-[var(--soft)]">{detail}</p> : null}
      </div>
      {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
    </section>
  )
}

export function LoadingBlock({ label = '载入运营数据中' }: { label?: string }) {
  return (
    <section className={stateBlockBase}>
      <span className="size-4 animate-spin rounded-full border-2 border-[var(--line)] border-t-[var(--blue)]" />
      <strong>{label}</strong>
      <p>正在请求真实后台 API。</p>
    </section>
  )
}

export function ErrorBlock({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <section className={stateBlockBase}>
      <strong>加载失败</strong>
      <p>{message}</p>
      <button type="button" className={cn(adminButton.base, adminButton.primary)} onClick={onRetry}>重试</button>
    </section>
  )
}

export function EmptyBlock({ title, detail }: { title: string; detail: string }) {
  return (
    <section className={stateBlockBase}>
      <strong>{title}</strong>
      <p>{detail}</p>
    </section>
  )
}

export function Badge({ tone = 'neutral', children }: { tone?: ToastTone | 'success' | 'primary'; children: React.ReactNode }) {
  return <span className={cn('inline-flex w-fit items-center rounded-full px-2 py-1 text-[11px] font-extrabold', badgeToneClass[tone])}>{children}</span>
}

export function InlineFeedback({ tone, message }: { tone: ToastTone; message: string }) {
  return <div className={cn('rounded-[10px] border px-3 py-2 text-sm', feedbackToneClass[tone])}>{message}</div>
}

export function GroupOptionGrid({
  selected,
  groups,
  onChange,
  emptyText = '暂无可选分组',
}: {
  selected: string[]
  groups: UserGroup[]
  onChange: (ids: string[]) => void
  emptyText?: string
}) {
  if (!groups.length) return <div className={cn(checkGridClass, checkGridEmptyClass)}><span>{emptyText}</span></div>

  return (
    <div className={checkGridClass}>
      {groups.map((group) => {
        const id = String(group.id ?? group.code)
        const checked = selected.includes(id) || selected.includes(group.code)
        const nextSelected = (isChecked: boolean) => (
          isChecked
            ? Array.from(new Set([...selected.filter((item) => item !== group.code), id]))
            : selected.filter((item) => item !== id && item !== group.code)
        )

        return (
          <label key={id} className={checkOptionClass}>
            <input type="checkbox" checked={checked} onChange={(event) => onChange(nextSelected(event.target.checked))} />
            <span className={checkOptionNameClass}>{group.name}</span>
            <em className={checkOptionMetaClass}>{group.multiplier}x</em>
          </label>
        )
      })}
    </div>
  )
}

export function Modal({
  title,
  detail,
  children,
  footer,
  onClose,
}: {
  title: string
  detail?: string
  children: React.ReactNode
  footer: React.ReactNode
  onClose: () => void
}) {
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose])

  return (
    <div className="fixed inset-0 z-[90] grid place-items-center bg-black/60 p-6 backdrop-blur-md" role="presentation" onMouseDown={onClose}>
      <section className="grid max-h-[92vh] w-[min(760px,calc(100vw-48px))] gap-5 overflow-auto rounded-3xl border border-[var(--line)] bg-[var(--surface)] p-5 shadow-[0_24px_80px_rgba(0,0,0,.45)]" role="dialog" aria-modal="true" aria-label={title} onMouseDown={(event) => event.stopPropagation()}>
        <header className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <strong>{title}</strong>
            {detail ? <p>{detail}</p> : null}
          </div>
          <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={onClose}>关闭</button>
        </header>
        <div>{children}</div>
        <footer className="flex flex-wrap items-center justify-end gap-3">{footer}</footer>
      </section>
    </div>
  )
}

function routeIcon(route: ProtectedAdminRouteId) {
  const icons: Record<ProtectedAdminRouteId, React.ReactNode> = {
    dashboard: <ChartIcon />,
    monitoring: <ActivityIcon />,
    users: <UsersIcon />,
    'user-groups': <GroupIcon />,
    'call-records': <ListIcon />,
    redeem: <TicketIcon />,
    reviews: <ShieldIcon />,
    orders: <CreditCardIcon />,
    packages: <BoxIcon />,
    'cashier-config': <LayoutIcon />,
    routing: <ZapIcon />,
    'access-accounts': <CloudIcon />,
    pricing: <CoinsIcon />,
    audit: <FileTextIcon />,
    'system-users': <SystemUserIcon />,
    'system-settings': <SettingsIcon />,
  }
  return icons[route]
}

const ChartIcon = () => <svg className={navIconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M3 3v18h18" /><path d="m19 9-5 5-4-4-3 3" /></svg>
const ActivityIcon = () => <svg className={navIconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M22 12h-4l-3 9L9 3l-3 9H2" /></svg>
const UsersIcon = () => <svg className={navIconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" /><circle cx="9" cy="7" r="4" /><path d="M22 21v-2a4 4 0 0 0-3-3.87" /><path d="M16 3.13a4 4 0 0 1 0 7.75" /></svg>
const GroupIcon = () => <svg className={navIconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" /><circle cx="9" cy="7" r="4" /><path d="M23 21v-2a4 4 0 0 0-3-3.87" /><path d="M16 3.13a4 4 0 0 1 0 7.75" /></svg>
const ListIcon = () => <svg className={navIconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="8" x2="21" y1="6" y2="6" /><line x1="8" x2="21" y1="12" y2="12" /><line x1="8" x2="21" y1="18" y2="18" /><line x1="3" x2="3.01" y1="6" y2="6" /><line x1="3" x2="3.01" y1="12" y2="12" /><line x1="3" x2="3.01" y1="18" y2="18" /></svg>
const TicketIcon = () => <svg className={navIconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M2 9a3 3 0 0 1 0 6v2a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-2a3 3 0 0 1 0-6V7a2 2 0 0 0-2-2H4a2 2 0 0 0-2 2Z" /><path d="M13 5v2" /><path d="M13 17v2" /><path d="M13 11v2" /></svg>
const ShieldIcon = () => <svg className={navIconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" /></svg>
const CreditCardIcon = () => <svg className={navIconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect width="20" height="14" x="2" y="5" rx="2" /><line x1="2" x2="22" y1="10" y2="10" /></svg>
const BoxIcon = () => <svg className={navIconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z" /><path d="m3.3 7 8.7 5 8.7-5" /><path d="M12 22V12" /></svg>
const LayoutIcon = () => <svg className={navIconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect width="18" height="18" x="3" y="3" rx="2" /><line x1="3" x2="21" y1="9" y2="9" /><line x1="9" x2="9" y1="21" y2="9" /></svg>
const ZapIcon = () => <svg className={navIconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" /></svg>
const CloudIcon = () => <svg className={navIconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M17.5 19c.1 0 .2 0 .3 0A5.5 5.5 0 0 0 16 8.1l-1.3-.1A7.5 7.5 0 0 0 2 12a7.5 7.5 0 0 0 12.3 5.8l1.2 1.2M17.5 19h.3" /><path d="M12 12l2 2 4-4" /></svg>
const CoinsIcon = () => <svg className={navIconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="8" cy="8" r="6" /><path d="M18.09 10.37A6 6 0 1 1 10.34 18" /><path d="M7 6h1v4" /><path d="m16.71 13.88.7.71-2.82 2.82" /></svg>
const FileTextIcon = () => <svg className={navIconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z" /><polyline points="14 2 14 8 20 8" /><line x1="16" x2="8" y1="13" y2="13" /><line x1="16" x2="8" y1="17" y2="17" /></svg>
const SystemUserIcon = () => <svg className={navIconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" /><circle cx="12" cy="11" r="3" /></svg>
const SettingsIcon = () => <svg className={navIconClass} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.1a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" /><circle cx="12" cy="12" r="3" /></svg>
const SunIcon = () => <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="4" /><path d="M12 2v2" /><path d="M12 20v2" /><path d="m4.93 4.93 1.41 1.41" /><path d="m17.66 17.66 1.41 1.41" /><path d="M2 12h2" /><path d="M20 12h2" /><path d="m6.34 17.66-1.41 1.41" /><path d="m19.07 4.93-1.41 1.41" /></svg>
const MoonIcon = () => <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" /></svg>
const BellIcon = () => <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9" /><path d="M13.73 21a2 2 0 0 1-3.46 0" /></svg>

export function Field({ label, children, error, hint }: { label: string; children: React.ReactNode; error?: string | null; hint?: string }) {
  return (
    <label className="grid gap-1.5 text-xs font-extrabold text-[var(--soft)]">
      <span className={fieldLabelClass}>
        <span>{label}</span>
        {hint ? <FieldHint text={hint} /> : null}
      </span>
      {children}
      {error ? <em className="text-xs not-italic text-[var(--red)]">{error}</em> : null}
    </label>
  )
}

function FieldHint({ text }: { text: string }) {
  const anchorRef = useRef<HTMLSpanElement | null>(null)
  const [open, setOpen] = useState(false)
  const [position, setPosition] = useState<{ left: number; top: number; placement: 'top' | 'bottom' }>({ left: 0, top: 0, placement: 'top' })

  useEffect(() => {
    if (!open) return
    const update = () => {
      const rect = anchorRef.current?.getBoundingClientRect()
      if (!rect) return
      const placement = rect.top < 72 ? 'bottom' : 'top'
      const maxWidth = 260
      const halfWidth = maxWidth / 2
      setPosition({
        left: Math.min(window.innerWidth - halfWidth - 12, Math.max(halfWidth + 12, rect.left + rect.width / 2)),
        top: placement === 'bottom' ? rect.bottom + 8 : rect.top - 8,
        placement,
      })
    }
    update()
    window.addEventListener('resize', update)
    window.addEventListener('scroll', update, true)
    return () => {
      window.removeEventListener('resize', update)
      window.removeEventListener('scroll', update, true)
    }
  }, [open])

  return (
    <>
      <span
        ref={anchorRef}
        className="grid size-[18px] place-items-center rounded-full border border-[var(--line)] text-[10px] text-[var(--blue)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--blue)]"
        tabIndex={0}
        aria-label={text}
        onMouseEnter={() => setOpen(true)}
        onMouseLeave={() => setOpen(false)}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
      >
        i
      </span>
      {open ? createPortal(
        <span className="fixed z-[200] max-w-[260px] -translate-x-1/2 rounded-lg border border-[var(--line)] bg-[var(--text)] px-3 py-2 text-xs font-medium normal-case tracking-normal text-white shadow-[var(--pg-shadow-lg)]" role="tooltip" data-placement={position.placement} style={{ left: position.left, top: position.top }}>{text}</span>,
        document.body,
      ) : null}
    </>
  )
}

export function ConfirmDrawer({
  title,
  detail,
  value,
  decisionLabel,
  tone,
  busy,
  onChange,
  onCancel,
  onConfirm,
}: {
  title: string
  detail: string
  value: string
  decisionLabel: string
  tone: ToastTone
  busy: boolean
  onChange: (value: string) => void
  onCancel: () => void
  onConfirm: () => void
}) {
  return (
    <aside className="grid min-h-full content-start gap-3 border-l border-[var(--line)] bg-white/[0.02] p-4" role="dialog" aria-modal="false" aria-label={title}>
      <div>
        <label>审核原因</label>
        <strong>{title}</strong>
        <p>{detail}</p>
      </div>
      <textarea value={value} onChange={(event) => onChange(event.target.value)} rows={4} placeholder="写明通过、驳回或下架理由" />
      <div className="flex flex-wrap items-center justify-end gap-2">
        <button type="button" className={cn(adminButton.base, adminButton.ghost)} onClick={onCancel} disabled={busy}>取消</button>
        <button type="button" className={cn(adminButton.base, tone === 'danger' ? adminButton.danger : adminButton.primary)} onClick={onConfirm} disabled={busy || value.trim().length < 3}>
          {busy ? '提交中...' : decisionLabel}
        </button>
      </div>
    </aside>
  )
}

export function MetricGrid({ metrics }: { metrics: AdminMetric[] }) {
  return (
    <section className="grid grid-cols-1 gap-6 md:grid-cols-2 xl:grid-cols-4">
      {metrics.map((metric) => (
        <div key={metric.label} className={cn('relative grid min-h-[130px] content-center gap-2 overflow-hidden rounded-3xl border border-white/5 bg-white/[0.02] p-6 transition-all hover:border-white/10 hover:bg-white/[0.04]', metricToneClass[metric.tone])}>
          <div>
            <label className="m-0 text-xs font-medium uppercase tracking-wider text-[var(--muted-strong)]">{metric.label}</label>
          </div>
          <div>
            <strong className="my-1 block text-3xl font-black tracking-tighter text-[var(--text)]">{metric.value}</strong>
            <span className="text-xs font-extrabold text-[var(--soft)]">{metric.trend}</span>
          </div>
        </div>
      ))}
    </section>
  )
}

function ChevronRightIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="m9 18 6-6-6-6" />
    </svg>
  )
}

export function useFilteredTabs<T extends { tab?: string }>(rows: T[]) {
  const tabs = useMemo(() => Array.from(new Set(rows.map((row) => row.tab).filter(Boolean))) as string[], [rows])
  const [activeTab, setActiveTab] = useState<string>('全部')
  const visibleRows = activeTab === '全部' ? rows : rows.filter((row) => row.tab === activeTab)
  return { tabs: ['全部', ...tabs], activeTab, setActiveTab, visibleRows }
}
