import React, { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import type { AdminMetric, AdminSession, ProviderHealth, UserGroup } from '../../shared/api-types'
import { cn } from '../../shared/classnames'
import { navGroups, normalizeRoute, protectedRoutes, routeHref, routeTitles } from './layout/admin-navigation'
import { useAdminTheme } from './layout/useAdminTheme'
import { filterAdminNavGroups } from './types'
import type { AdminRouteId, ProtectedAdminRouteId, ToastMessage, ToastTone } from './types'
import { adminButton, adminFeedback, adminMetric, adminPill, adminShell, adminState, adminType } from './ui/classes'
import { useAdminLayerMotion } from './ui/adminMotion'
import {
  AccessAccountsIcon,
  AlertIcon,
  AuditIcon,
  BellIcon,
  CallRecordsIcon,
  CashierIcon,
  ChevronRightIcon,
  DashboardIcon,
  EmptyIcon,
  ImageEmptyIcon,
  LoaderIcon,
  LogOutIcon,
  MenuIcon,
  MonitoringIcon,
  MoonIcon,
  OrdersIcon,
  PackageIcon,
  PricingIcon,
  RedeemIcon,
  ReviewIcon,
  RoutingIcon,
  SunIcon,
  SystemSettingsIcon,
  SystemUsersIcon,
  UserGroupsIcon,
  UserMenuIcon,
  UsersIcon,
  XIcon,
} from './ui/icons'

export { navGroups, normalizeRoute, protectedRoutes, routeHref } from './layout/admin-navigation'

const badgeToneClass: Record<ToastTone | 'success' | 'primary' | 'neutral', string> = {
  success: 'border-[color-mix(in_oklch,var(--green)_22%,transparent)] bg-[color-mix(in_oklch,var(--green)_10%,transparent)] text-[var(--green)]',
  warning: 'border-[color-mix(in_oklch,var(--amber)_24%,transparent)] bg-[color-mix(in_oklch,var(--amber)_10%,transparent)] text-[var(--amber)]',
  danger: 'border-[color-mix(in_oklch,var(--red)_24%,transparent)] bg-[color-mix(in_oklch,var(--red)_10%,transparent)] text-[var(--red)]',
  primary: 'border-[color-mix(in_oklch,var(--accent)_24%,transparent)] bg-[color-mix(in_oklch,var(--accent)_10%,transparent)] text-[var(--accent)]',
  neutral: 'border-[var(--border)] bg-[var(--surface)] text-[var(--muted)]',
}

const feedbackToneClass: Record<ToastTone, string> = {
  success: badgeToneClass.success,
  warning: badgeToneClass.warning,
  danger: badgeToneClass.danger,
  neutral: badgeToneClass.primary,
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
const stateBlockBase = adminState.block
const stateBlockIcon = adminState.iconWrap
const fieldLabelClass = 'flex items-center justify-between gap-2 text-[11px] font-semibold text-[var(--soft)]'
const checkGridClass = 'grid max-h-[220px] gap-2 overflow-auto rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-2'
const checkGridEmptyClass = 'grid-cols-1 text-sm font-bold text-[var(--soft)]'
const checkOptionClass = 'grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 rounded-xl border border-[var(--border)] bg-[var(--surface-solid)] p-2 text-sm has-[:checked]:border-[var(--accent)]/40 has-[:checked]:bg-[var(--accent)]/10'
const checkOptionNameClass = 'min-w-0 overflow-hidden text-ellipsis whitespace-nowrap'
const checkOptionMetaClass = 'text-xs not-italic font-extrabold text-[var(--soft)]'
const metricToneClass: Record<string, string> = {
  good: '[&_span]:text-[var(--green)]',
  warn: '[&_span]:text-[var(--amber)]',
  bad: '[&_span]:text-[var(--red)]',
  danger: '[&_span]:text-[var(--red)]',
}

const focusableSelector = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

function focusableElements(root: HTMLElement) {
  return Array.from(root.querySelectorAll<HTMLElement>(focusableSelector)).filter((element) => !element.hidden && element.getAttribute('aria-hidden') !== 'true')
}

function useDialogFocus(onClose: () => void, dialogRef: React.RefObject<HTMLElement | null>) {
  const onCloseRef = useRef(onClose)
  onCloseRef.current = onClose
  useEffect(() => {
    const dialog = dialogRef.current
    if (!dialog) return undefined
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    ;(focusableElements(dialog)[0] ?? dialog).focus()

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        onCloseRef.current()
        return
      }
      if (event.key !== 'Tab') return
      const focusable = focusableElements(dialog)
      if (!focusable.length) {
        event.preventDefault()
        dialog.focus()
        return
      }
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', onKeyDown, true)
    return () => {
      document.removeEventListener('keydown', onKeyDown, true)
      document.body.style.overflow = previousOverflow
      previousFocus?.focus()
    }
  }, [dialogRef])
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
  const [navOpen, setNavOpen] = useState(false)
  const [accountOpen, setAccountOpen] = useState(false)
  const navTriggerRef = useRef<HTMLButtonElement | null>(null)
  const mobileDrawerRef = useRef<HTMLElement | null>(null)
  const accountButtonRef = useRef<HTMLButtonElement | null>(null)
  const accountMenuRef = useRef<HTMLDivElement | null>(null)
  const navBadges = {
    review_count: reviewCount > 0 ? String(reviewCount) : '',
    failed_webhook_count: '',
    config_drafts: configDrafts > 0 ? String(configDrafts) : '',
  }
  const currentTitle = routeTitles[route]

  useEffect(() => {
    if (!navOpen) return undefined
    const drawer = mobileDrawerRef.current
    if (!drawer) return undefined
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    ;(focusableElements(drawer)[0] ?? drawer).focus()

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        setNavOpen(false)
        return
      }
      if (event.key === 'Tab') {
        const focusable = focusableElements(drawer)
        if (!focusable.length) {
          event.preventDefault()
          drawer.focus()
          return
        }
        const first = focusable[0]
        const last = focusable[focusable.length - 1]
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault()
          last.focus()
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault()
          first.focus()
        }
      }
    }
    document.addEventListener('keydown', onKeyDown, true)
    return () => {
      document.removeEventListener('keydown', onKeyDown, true)
      document.body.style.overflow = previousOverflow
      navTriggerRef.current?.focus()
    }
  }, [navOpen])

  useEffect(() => {
    if (!accountOpen) return undefined
    const menu = accountMenuRef.current
    window.requestAnimationFrame(() => menu?.querySelector<HTMLElement>('[role="menuitem"]')?.focus())
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node
      if (accountButtonRef.current?.contains(target) || menu?.contains(target)) return
      setAccountOpen(false)
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        setAccountOpen(false)
        accountButtonRef.current?.focus()
      }
    }
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown, true)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown, true)
    }
  }, [accountOpen])

  const renderNav = (onItemClick?: (route: ProtectedAdminRouteId) => void) => (
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
                onClick={() => {
                  onNavigate(item.id)
                  onItemClick?.(item.id)
                }}
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
  )

  return (
    <main className={adminShell.root} data-theme={theme}>
      <header className={adminShell.mobileTopbar}>
        <button
          ref={navTriggerRef}
          className={adminShell.iconButton}
          type="button"
          aria-label="打开导航"
          title="打开导航"
          onClick={() => setNavOpen(true)}
        >
          <MenuIcon className="size-5" />
        </button>
        <strong className={adminShell.mobileTitle}>{currentTitle}</strong>
        <button
          className={adminShell.iconButton}
          type="button"
          aria-label={theme === 'dark' ? '切换到亮色模式' : '切换到暗色模式'}
          title={theme === 'dark' ? '切换到亮色模式' : '切换到暗色模式'}
          onClick={() => setTheme((current) => current === 'dark' ? 'light' : 'dark')}
        >
          {theme === 'dark' ? <SunIcon /> : <MoonIcon />}
        </button>
      </header>

      {navOpen ? (
        <>
          <button className={adminShell.mobileDrawerBackdrop} type="button" aria-label="关闭导航" onClick={() => setNavOpen(false)} />
          <aside ref={mobileDrawerRef} tabIndex={-1} className={adminShell.mobileDrawer} aria-label="移动端后台导航">
            <header className={adminShell.mobileDrawerHead}>
              <span className={adminShell.brandOrb}>M</span>
              <strong className={adminShell.brandText}>Mikiko Admin</strong>
              <button className={adminShell.iconButton} type="button" aria-label="关闭导航" title="关闭导航" onClick={() => setNavOpen(false)}><XIcon className="size-5" /></button>
            </header>
            {renderNav(() => setNavOpen(false))}
            <div className={adminShell.sideNote}>
              <button className={cn(adminButton.base, adminButton.ghost, adminButton.small, 'w-full')} type="button" onClick={onLogout}><LogOutIcon className="size-4" />退出</button>
            </div>
          </aside>
        </>
      ) : null}

      <aside className={adminShell.sidebar} aria-label="Pic Gallery Admin Navigation">
        <a className={adminShell.brand} href={routeHref('dashboard')} onClick={() => onNavigate('dashboard')}>
          <span className={adminShell.brandOrb}>M</span>
          <strong className={adminShell.brandText}>Mikiko Admin</strong>
        </a>

        {renderNav()}

        <div className={adminShell.sideNote}>
          <div className={adminShell.sideNoteIdentity}>
            <span className={adminShell.avatarOrb}>{session.admin_name.slice(0, 2).toUpperCase()}</span>
            <div className="grid min-w-0 flex-1 gap-0.5">
              <strong className="truncate text-sm text-[var(--text)]">{session.admin_name}</strong>
              <span className="truncate text-xs text-[var(--muted-strong)]">{session.role === 'super_admin' ? '超级管理员' : '运营管理员'}</span>
            </div>
          </div>
          <button className={cn(adminButton.base, adminButton.ghost, adminButton.small, 'w-full')} type="button" onClick={onLogout}><LogOutIcon className="size-4" />退出</button>
        </div>
      </aside>

      <section className={adminShell.main}>
        <header className={adminShell.topbar}>
          <div className={adminShell.topbarMeta}>
            <div className={adminShell.providerPill} title="Provider 状态">
              <span className={cn('inline-block size-2 rounded-full', providers.some((item) => item.status !== 'healthy') ? dotToneClass.warning : dotToneClass.success)} />
              <span>上游状态</span>
              <strong>{providers.length}</strong>
            </div>
            <StatusChip tone={reviewCount > 0 ? 'warning' : 'success'} label="待审" value={String(reviewCount)} />
            <StatusChip tone={configDrafts > 0 ? 'primary' : 'neutral'} label="草稿" value={String(configDrafts)} />
          </div>

          <div className={adminShell.metaRow}>
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
            <div className={adminShell.avatarWidget}>
              <button
                ref={accountButtonRef}
                type="button"
                className="flex min-h-10 items-center gap-2 rounded-lg border border-transparent px-2 text-left transition-colors hover:border-[var(--border)] hover:bg-[var(--surface)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]/25"
                aria-haspopup="menu"
                aria-expanded={accountOpen}
                onClick={() => setAccountOpen((current) => !current)}
              >
                <span className={adminShell.avatarOrb}>{session.admin_name.slice(0, 1).toUpperCase()}</span>
                <span className="grid text-right">
                  <strong className="text-sm text-[var(--fg)]">{session.admin_name}</strong>
                  <span className="text-xs text-[var(--dim)]">{session.role === 'super_admin' ? '超级管理员' : '运营管理员'}</span>
                </span>
                <UserMenuIcon className="size-4 text-[var(--dim)]" />
              </button>
              {accountOpen ? (
                <div ref={accountMenuRef} className="absolute right-0 top-[calc(100%+8px)] z-[70] grid min-w-52 rounded-xl border border-[var(--border)] bg-[var(--surface-solid)] p-1.5 shadow-[var(--pg-shadow-lg)]" role="menu" aria-label="管理员账户菜单">
                  <div className="grid gap-0.5 border-b border-[var(--border)] px-3 py-2">
                    <strong className="text-sm text-[var(--fg)]">{session.admin_name}</strong>
                    <span className="text-xs text-[var(--dim)]">{session.email}</span>
                  </div>
                  <button className="flex min-h-10 items-center gap-2 rounded-lg px-3 text-left text-sm font-semibold text-[var(--muted)] hover:bg-[var(--surface)] hover:text-[var(--fg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]/25" type="button" role="menuitem" onClick={onLogout}>
                    <LogOutIcon className="size-4" />退出后台
                  </button>
                </div>
              ) : null}
            </div>
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
    <span className={cn(adminShell.chip, badgeToneClass[tone])}>
      <span className={cn('inline-block size-2 rounded-full', dotToneClass[tone === 'neutral' ? 'primary' : tone])} />
      <em>{label}</em>
      <strong>{value}</strong>
    </span>
  )
}

export function StatusCell({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="relative grid min-h-[112px] content-center gap-2 overflow-hidden rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-5 transition-colors hover:border-[var(--border-strong)] hover:bg-[var(--elevated)]">
      <label className="m-0 text-[11px] font-semibold text-[var(--dim)]">{label}</label>
      <strong className="block truncate text-[1.75rem] font-black text-[var(--fg)]">{value}</strong>
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

export function PageHeader({
  title,
  description,
  detail,
  meta,
  primaryAction,
  secondaryActions,
  actions,
}: {
  eyebrow?: string
  title: string
  description?: string
  detail?: string
  meta?: React.ReactNode
  primaryAction?: React.ReactNode
  secondaryActions?: React.ReactNode
  actions?: React.ReactNode
}) {
  const resolvedDescription = description ?? detail
  const resolvedActions = actions ?? (
    primaryAction || secondaryActions ? (
      <>
        {secondaryActions}
        {primaryAction}
      </>
    ) : null
  )

  return (
    <header className="flex min-h-16 w-full max-w-none items-end justify-between gap-4 max-[920px]:min-h-0 max-[920px]:flex-col max-[920px]:items-start">
      <div className="min-w-0">
        <h1 className={cn('m-0', adminType.pageTitle)}>{title}</h1>
        {resolvedDescription ? <p className={cn('mt-1 max-w-[76ch]', adminType.pageDescription)}>{resolvedDescription}</p> : null}
        {meta ? <div className="mt-2">{meta}</div> : null}
      </div>
      {resolvedActions ? <div className="flex flex-wrap items-center gap-2">{resolvedActions}</div> : null}
    </header>
  )
}

export function PageToolbar({ children, actions }: { children: React.ReactNode; actions?: React.ReactNode }) {
  return (
    <section className="flex flex-wrap items-end justify-between gap-3 rounded-lg bg-[var(--surface-solid)] px-3 py-3">
      <div className="flex min-w-0 flex-1 flex-wrap items-end gap-2">{children}</div>
      {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
    </section>
  )
}

export type AdminTabItem<T extends string> = {
  id: T
  label: string
  description?: string
  disabled?: boolean
  badge?: React.ReactNode
}

export function AdminTabs<T extends string>({
  items,
  value,
  onChange,
  orientation = 'horizontal',
  ariaLabel,
  className,
}: {
  items: AdminTabItem<T>[]
  value: T
  onChange: (value: T) => void
  orientation?: 'horizontal' | 'vertical'
  ariaLabel: string
  className?: string
}) {
  const vertical = orientation === 'vertical'
  const tabRefs = useRef<Array<HTMLButtonElement | null>>([])

  const moveFocus = (currentIndex: number, key: string) => {
    const enabledIndexes = items.flatMap((item, index) => item.disabled ? [] : [index])
    if (!enabledIndexes.length) return false
    const enabledPosition = enabledIndexes.indexOf(currentIndex)
    let nextIndex: number | undefined
    if (key === 'Home') nextIndex = enabledIndexes[0]
    if (key === 'End') nextIndex = enabledIndexes[enabledIndexes.length - 1]
    if (key === 'ArrowRight' || key === 'ArrowDown') nextIndex = enabledIndexes[(enabledPosition + 1) % enabledIndexes.length]
    if (key === 'ArrowLeft' || key === 'ArrowUp') nextIndex = enabledIndexes[(enabledPosition - 1 + enabledIndexes.length) % enabledIndexes.length]
    if (nextIndex === undefined) return false
    tabRefs.current[nextIndex]?.focus()
    onChange(items[nextIndex].id)
    return true
  }

  return (
    <div
      role="tablist"
      aria-label={ariaLabel}
      aria-orientation={orientation}
      className={cn(
        'rounded-lg bg-[var(--surface-solid)] p-1',
        vertical ? 'grid gap-1' : 'flex flex-wrap items-center gap-1',
        className,
      )}
    >
      {items.map((item, index) => {
        const active = value === item.id
        return (
          <button
            ref={(node) => { tabRefs.current[index] = node }}
            key={item.id}
            type="button"
            role="tab"
            aria-selected={active}
            aria-disabled={item.disabled || undefined}
            tabIndex={active ? 0 : -1}
            disabled={item.disabled}
            className={cn(
              'min-h-9 rounded-md px-3 py-2 text-sm font-bold text-[var(--muted)] transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color-mix(in_oklch,var(--accent)_28%,transparent)] disabled:cursor-not-allowed disabled:opacity-45',
              vertical ? 'grid w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-2 text-left' : 'inline-flex items-center justify-center gap-2',
              !item.disabled && 'hover:bg-[var(--elevated)] hover:text-[var(--fg)]',
              active && 'bg-[var(--elevated)] text-[var(--fg)] shadow-[var(--pg-shadow-sm)]',
            )}
            onClick={() => onChange(item.id)}
            onKeyDown={(event) => {
              if (moveFocus(index, event.key)) event.preventDefault()
            }}
          >
            <span className="min-w-0 truncate">{item.label}</span>
            {item.badge ? <span className="shrink-0">{item.badge}</span> : null}
            {vertical && item.description ? <span className="col-span-2 text-xs font-medium text-[var(--soft)]">{item.description}</span> : null}
          </button>
        )
      })}
    </div>
  )
}

/** @deprecated 使用 `ui/dataTable.tsx` 中的 `FilterBar` 替代。第二阶段迁移完成后将移除。 */
export type FilterField = {
  key: string
  label?: string
  primary?: boolean
  control: React.ReactNode
}

/** @deprecated 使用 `ui/dataTable.tsx` 中的 `FilterBar` 替代。第二阶段迁移完成后将移除。 */
export function ListFilterBar({
  fields,
  actions,
  advancedOpen,
  onAdvancedOpenChange,
}: {
  fields: FilterField[]
  actions?: React.ReactNode
  advancedOpen?: boolean
  onAdvancedOpenChange?: (open: boolean) => void
}) {
  const [internalOpen, setInternalOpen] = useState(false)
  const open = advancedOpen ?? internalOpen
  const advancedFields = fields.filter((field) => !field.primary)
  const setOpen = (next: boolean) => {
    setInternalOpen(next)
    onAdvancedOpenChange?.(next)
  }
  const visibleFields = fields.filter((field) => field.primary || open)

  return (
    <section className="flex flex-wrap items-end justify-between gap-3">
      <div className="flex min-w-0 flex-1 flex-wrap items-end gap-3">
        {visibleFields.map((field) => (
          <label key={field.key} className="grid min-w-[180px] max-w-full flex-[1_1_180px] gap-1 text-xs font-bold text-[var(--muted)]">
            {field.label ? <span>{field.label}</span> : null}
            {field.control}
          </label>
        ))}
      </div>
      <div className="flex flex-wrap items-center justify-end gap-2">
        {advancedFields.length ? (
          <button
            type="button"
            className={cn(adminButton.base, adminButton.ghost, adminButton.small)}
            aria-expanded={open}
            onClick={() => setOpen(!open)}
          >
            {open ? '收起筛选' : `更多筛选 ${advancedFields.length}`}
          </button>
        ) : null}
        {actions}
      </div>
    </section>
  )
}

/** @deprecated 使用 `ui/dataTable.tsx` 中的 `ListPage` 替代。第二阶段迁移完成后将移除。 */
export function AdminListPage({
  filters,
  actions,
  children,
  pagination,
  resultSummary,
  className,
}: {
  filters?: React.ReactNode
  actions?: React.ReactNode
  children: React.ReactNode
  pagination?: React.ReactNode
  defaultFiltersOpen?: boolean
  collapsibleFilters?: boolean
  resultSummary?: React.ReactNode
  className?: string
}) {
  return (
    <section className={cn('grid min-h-0 gap-3 rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-4 shadow-[var(--pg-shadow-sm)]', className)}>
      {(filters || actions || resultSummary) ? (
        <header className="flex flex-wrap items-end justify-between gap-3">
          <div className="min-w-0 flex-1">{filters}</div>
          {(actions || resultSummary) ? <div className="flex shrink-0 flex-wrap items-center justify-end gap-2 text-xs font-semibold text-[var(--muted)]">{resultSummary}{actions}</div> : null}
        </header>
      ) : null}
      <div className="min-w-0 overflow-hidden">
        {children}
      </div>
      {pagination ? <footer className="flex flex-wrap items-center justify-between gap-3 pt-3 text-xs text-[var(--muted)]">{pagination}</footer> : null}
    </section>
  )
}

export function PageSection({
  title,
  description,
  children,
  variant = 'plain',
}: {
  title?: string
  description?: string
  children: React.ReactNode
  variant?: 'plain' | 'panel'
}) {
  return (
    <section className={variant === 'panel' ? 'rounded-lg border border-[var(--border)] bg-[var(--surface)] p-4' : 'grid gap-3'}>
      {title || description ? (
        <header className="grid gap-1">
          {title ? <h2 className="m-0 text-base font-semibold text-[var(--fg)]">{title}</h2> : null}
          {description ? <p className="text-sm text-[var(--soft)]">{description}</p> : null}
        </header>
      ) : null}
      {children}
    </section>
  )
}

type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger' | 'text'
  size?: 'sm' | 'md'
  icon?: React.ReactNode
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(function Button({
  variant = 'secondary',
  size = 'md',
  icon,
  children,
  className,
  ...props
}, ref) {
  const variantClass = {
    primary: adminButton.primary,
    secondary: adminButton.secondary,
    ghost: adminButton.ghost,
    danger: adminButton.danger,
    text: adminButton.text,
  }[variant]
  return (
    <button ref={ref} className={cn(adminButton.base, variantClass, size === 'sm' && adminButton.small, className)} type="button" {...props}>
      {icon}
      {children}
    </button>
  )
})

export function IconButton({
  label,
  title,
  children,
  className,
  ...props
}: Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'aria-label'> & {
  label: string
}) {
  return (
    <button className={cn(adminShell.iconButton, className)} type="button" aria-label={label} title={title ?? label} {...props}>
      {children ?? '·'}
    </button>
  )
}

export function SegmentedControl<T extends string>({
  value,
  options,
  onChange,
}: {
  value: T
  options: Array<{ value: T; label: string; disabled?: boolean }>
  onChange: (value: T) => void
}) {
  return (
    <div role="tablist" className="inline-flex flex-wrap items-center gap-1 rounded-lg border border-[var(--border)] bg-[var(--surface)] p-1">
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          role="tab"
          aria-selected={value === option.value}
          disabled={option.disabled}
          className={cn(
            'min-h-8 rounded-md px-3 text-sm font-bold text-[var(--muted)] disabled:cursor-not-allowed disabled:opacity-45',
            value === option.value && 'bg-[var(--elevated)] text-[var(--fg)]',
          )}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}

export function StatusBadge({ tone = 'neutral', children }: { tone?: ToastTone | 'success' | 'primary'; children: React.ReactNode }) {
  return <Badge tone={tone}>{children}</Badge>
}

export type ActionMenuItem = {
  id: string
  label: string
  tone?: 'neutral' | 'danger'
  confirm?: { title: string; expectedValue?: string }
  run: () => Promise<void> | void
}

export function ActionMenu({ actions }: { actions: ActionMenuItem[] }) {
  const [open, setOpen] = useState(false)
  const buttonRef = useRef<HTMLButtonElement | null>(null)
  const menuRef = useRef<HTMLSpanElement | null>(null)
  const [menuStyle, setMenuStyle] = useState<React.CSSProperties | null>(null)
  const [theme, setTheme] = useState<string | undefined>(undefined)

  useEffect(() => {
    if (!open) return
    const updatePosition = () => {
      const button = buttonRef.current
      if (!button) return
      const rect = button.getBoundingClientRect()
      const menuWidth = Math.max(menuRef.current?.offsetWidth ?? 144, 144)
      const viewportPadding = 12
      const left = Math.min(
        Math.max(viewportPadding, rect.right - menuWidth),
        window.innerWidth - menuWidth - viewportPadding,
      )
      const preferredTop = rect.bottom + 6
      const menuHeight = menuRef.current?.offsetHeight ?? 0
      const top = preferredTop + menuHeight > window.innerHeight - viewportPadding
        ? Math.max(viewportPadding, rect.top - menuHeight - 6)
        : preferredTop
      setMenuStyle({ left, top })
      setTheme(button.closest('[data-theme]')?.getAttribute('data-theme') ?? undefined)
    }
    updatePosition()
    window.requestAnimationFrame(() => menuRef.current?.querySelector<HTMLButtonElement>('[role="menuitem"]')?.focus())
    window.addEventListener('resize', updatePosition)
    window.addEventListener('scroll', updatePosition, true)
    return () => {
      window.removeEventListener('resize', updatePosition)
      window.removeEventListener('scroll', updatePosition, true)
    }
  }, [open])

  useEffect(() => {
    if (!open) return
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node
      if (buttonRef.current?.contains(target) || menuRef.current?.contains(target)) return
      setOpen(false)
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setOpen(false)
        buttonRef.current?.focus()
      }
    }
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  const runAction = async (action: ActionMenuItem) => {
    if (action.confirm) {
      const ok = action.confirm.expectedValue
        ? window.prompt(action.confirm.title) === action.confirm.expectedValue
        : window.confirm(action.confirm.title)
      if (!ok) return
    }
    setOpen(false)
    buttonRef.current?.focus()
    await action.run()
  }

  const handleMenuKeyDown = (event: React.KeyboardEvent<HTMLSpanElement>) => {
    const items = Array.from(menuRef.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? [])
    const currentIndex = items.indexOf(document.activeElement as HTMLButtonElement)
    let nextIndex: number | undefined
    if (event.key === 'ArrowDown') nextIndex = (currentIndex + 1) % items.length
    if (event.key === 'ArrowUp') nextIndex = (currentIndex - 1 + items.length) % items.length
    if (event.key === 'Home') nextIndex = 0
    if (event.key === 'End') nextIndex = items.length - 1
    if (nextIndex === undefined || !items.length) return
    event.preventDefault()
    items[nextIndex]?.focus()
  }

  return (
    <span className="relative inline-flex">
      <Button
        ref={buttonRef}
        variant="ghost"
        size="sm"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
        onKeyDown={(event) => {
          if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
            event.preventDefault()
            setOpen(true)
          }
        }}
      >更多</Button>
      {open ? createPortal(
        <span
          ref={menuRef}
          className="fixed z-[120] grid min-w-36 rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-1 shadow-[var(--pg-shadow-sm)]"
          data-theme={theme}
          role="menu"
          style={menuStyle ?? undefined}
          onKeyDown={handleMenuKeyDown}
        >
          {actions.map((action) => (
            <button
              key={action.id}
              type="button"
              role="menuitem"
              className={cn('rounded-md px-3 py-2 text-left text-sm font-bold text-[var(--muted)] hover:bg-[var(--surface)] hover:text-[var(--fg)]', action.tone === 'danger' && 'text-[var(--red)]')}
              onClick={() => void runAction(action)}
            >
              {action.label}
            </button>
          ))}
        </span>,
        document.body,
      ) : null}
    </span>
  )
}

export function LoadingBlock({ label = '载入运营数据中' }: { label?: string }) {
  return (
    <section className={stateBlockBase}>
      <span className={stateBlockIcon}><LoaderIcon className="size-6 animate-spin" /></span>
      <strong className={adminState.title}>{label}</strong>
      <p className={adminState.detail}>正在请求真实后台 API，并同步构建管理端骨架数据。</p>
      <div className="grid w-full max-w-[360px] gap-2">
        <div className="pg-skeleton h-4 rounded-md" />
        <div className="pg-skeleton h-4 w-4/5 rounded-md" />
        <div className="pg-skeleton h-12 rounded-xl" />
      </div>
    </section>
  )
}

export function ErrorBlock({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <section className={stateBlockBase}>
      <span className={cn(stateBlockIcon, 'bg-[var(--red)]/10 text-[var(--red)]')}><AlertIcon className="size-6" /></span>
      <strong className={adminState.title}>加载失败</strong>
      <p className={adminState.detail}>{message}</p>
      <button type="button" className={cn(adminButton.base, adminButton.primary)} onClick={onRetry}>重试</button>
    </section>
  )
}

export function EmptyBlock({ title, detail, action, icon = <EmptyIcon className="size-6" /> }: { title: string; detail: string; action?: React.ReactNode; icon?: React.ReactNode }) {
  return (
    <section className={stateBlockBase}>
      <span className={stateBlockIcon}>{icon}</span>
      <strong className={adminState.title}>{title}</strong>
      <p className={adminState.detail}>{detail}</p>
      {action}
    </section>
  )
}

export function Badge({ tone = 'neutral', children }: { tone?: ToastTone | 'success' | 'primary'; children: React.ReactNode }) {
  return <span className={cn(adminPill.base, 'border', badgeToneClass[tone])}>{children}</span>
}

export function InlineFeedback({ tone, message }: { tone: ToastTone; message: string }) {
  return <div className={cn(adminFeedback.inline, feedbackToneClass[tone])} role={tone === 'danger' ? 'alert' : 'status'}>{message}</div>
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
  const dialogRef = useRef<HTMLElement | null>(null)
  const layerRef = useRef<HTMLDivElement | null>(null)
  useDialogFocus(onClose, dialogRef)
  useAdminLayerMotion(layerRef)

  return (
    <div ref={layerRef} className="fixed inset-0 z-[90] grid place-items-center bg-black/60 p-6 backdrop-blur-md" role="presentation" onMouseDown={onClose}>
      <section ref={dialogRef} tabIndex={-1} data-admin-motion-panel className="grid max-h-[92vh] w-[min(760px,calc(100vw-48px))] gap-5 overflow-auto rounded-xl border border-[var(--border)] bg-[var(--surface-solid)] p-5 shadow-[0_24px_80px_rgba(0,0,0,.28)]" role="dialog" aria-modal="true" aria-label={title} onMouseDown={(event) => event.stopPropagation()}>
        <header className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <strong className="font-[family-name:var(--font-admin-display)] text-lg font-semibold">{title}</strong>
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

export function Drawer({
  title,
  description,
  children,
  footer,
  onClose,
}: {
  title: string
  description?: string
  children: React.ReactNode
  footer?: React.ReactNode
  onClose: () => void
}) {
  const dialogRef = useRef<HTMLElement | null>(null)
  const layerRef = useRef<HTMLDivElement | null>(null)
  useDialogFocus(onClose, dialogRef)
  useAdminLayerMotion(layerRef)

  return (
    <div ref={layerRef} className="fixed inset-0 z-[90] bg-black/55 backdrop-blur-sm" role="presentation" onMouseDown={onClose}>
      <aside
        ref={dialogRef}
        tabIndex={-1}
        data-admin-motion-panel
        className="ml-auto grid h-full w-[min(760px,100vw)] grid-rows-[auto_minmax(0,1fr)_auto] border-l border-[var(--border)] bg-[var(--surface-solid)] shadow-[0_24px_90px_rgba(0,0,0,.34)]"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <header className="flex flex-wrap items-start justify-between gap-3 border-b border-[var(--border)] p-5">
          <div className="grid gap-1">
            <strong className="text-lg font-semibold">{title}</strong>
            {description ? <p className="max-w-[62ch] text-sm text-[var(--soft)]">{description}</p> : null}
          </div>
          <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={onClose}>关闭</button>
        </header>
        <section className="min-h-0 overflow-y-auto p-5">{children}</section>
        {footer ? <footer className="flex flex-wrap items-center justify-end gap-3 border-t border-[var(--border)] p-4">{footer}</footer> : null}
      </aside>
    </div>
  )
}

function routeIcon(route: ProtectedAdminRouteId) {
  const icons: Record<ProtectedAdminRouteId, React.ReactNode> = {
    dashboard: <DashboardIcon className={navIconClass} />,
    monitoring: <MonitoringIcon className={navIconClass} />,
    users: <UsersIcon />,
    'user-groups': <UserGroupsIcon className={navIconClass} />,
    'call-records': <CallRecordsIcon className={navIconClass} />,
    redeem: <RedeemIcon className={navIconClass} />,
    reviews: <ReviewIcon className={navIconClass} />,
    orders: <OrdersIcon className={navIconClass} />,
    packages: <PackageIcon className={navIconClass} />,
    'cashier-config': <CashierIcon className={navIconClass} />,
    routing: <RoutingIcon className={navIconClass} />,
    'access-accounts': <AccessAccountsIcon className={navIconClass} />,
    pricing: <PricingIcon className={navIconClass} />,
    audit: <AuditIcon className={navIconClass} />,
    'system-users': <SystemUsersIcon className={navIconClass} />,
    'system-settings': <SystemSettingsIcon className={navIconClass} />,
  }
  return icons[route]
}

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
        className="grid size-[18px] place-items-center rounded-full border border-[var(--line)] text-[11px] text-[var(--blue)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--blue)]"
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
    <aside className="grid min-h-full content-start gap-3 border-l border-[var(--border)] bg-[var(--surface)] p-4" role="dialog" aria-modal="false" aria-label={title}>
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

export function MetricStrip({ metrics }: { metrics: AdminMetric[] }) {
  return (
    <section className={adminMetric.strip} aria-label="关键运营指标">
      {metrics.map((metric) => (
        <div key={metric.label} className={cn(adminMetric.item, 'transition-colors duration-[var(--admin-motion-base)] hover:border-[var(--border-strong)] hover:bg-[var(--elevated)]', metricToneClass[metric.tone])}>
          <span className={adminType.label}>{metric.label}</span>
          <strong className={adminMetric.value}>{metric.value}</strong>
          <span className={adminType.support}>{metric.trend}</span>
        </div>
      ))}
    </section>
  )
}

export function MetricGrid({ metrics }: { metrics: AdminMetric[] }) {
  return <MetricStrip metrics={metrics} />
}

export function useFilteredTabs<T extends { tab?: string }>(rows: T[]) {
  const tabs = useMemo(() => Array.from(new Set(rows.map((row) => row.tab).filter(Boolean))) as string[], [rows])
  const [activeTab, setActiveTab] = useState<string>('全部')
  const visibleRows = activeTab === '全部' ? rows : rows.filter((row) => row.tab === activeTab)
  return { tabs: ['全部', ...tabs], activeTab, setActiveTab, visibleRows }
}
