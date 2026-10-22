import React, { useEffect, useId, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { RefreshCw } from 'lucide-react'
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
  ClusterIcon,
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
const fieldLabelClass = 'flex min-h-[18px] items-center justify-between gap-2 text-[11px] font-semibold leading-[18px] text-[var(--soft)]'
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
    <nav className="a-side__nav" aria-label="后台主导航">
      {visibleNavGroups.map((group) => (
        <section key={group.label} className="a-side__group">
          <div className="a-side__label">{group.label}</div>
          {group.items.map((item) => {
            const badge = item.badgeKey ? navBadges[item.badgeKey] : ''
            return (
              <a
                key={item.id}
                href={routeHref(item.id)}
                className={cn('a-side__link', route === item.id && 'is-active')}
                onClick={() => {
                  onNavigate(item.id)
                  onItemClick?.(item.id)
                }}
                aria-current={route === item.id ? 'page' : undefined}
              >
                {routeIcon(item.id)}
                <span>{item.label}</span>
                {badge ? <span className="a-side__badge">{badge}</span> : null}
              </a>
            )
          })}
        </section>
      ))}
    </nav>
  )

  return (
    <main className="a-shell">
      {navOpen ? <button className="a-scrim is-open a-side-scrim" type="button" aria-label="关闭导航" onClick={() => setNavOpen(false)} /> : null}
      <aside ref={mobileDrawerRef} tabIndex={-1} className={cn('a-side', navOpen && 'is-open')} aria-label="Mikiko Gallery Studio Admin Navigation">
        <div className="a-side__brand">
          <span className="a-side__mark">M</span>
          <span className="a-side__name"><strong>Mikiko Ops</strong><span>Command Deck</span></span>
          <button className="a-icon-btn a-side-close" type="button" aria-label="关闭导航" title="关闭导航" onClick={() => setNavOpen(false)}><XIcon /></button>
        </div>

        {renderNav(() => setNavOpen(false))}

        <div className="a-side__foot">
          <div className="a-side__foot-identity">
            <span className="a-cell-entity__avatar">{session.admin_name.slice(0, 1).toUpperCase()}</span>
            <div className="a-side__identity-copy">
              <strong>{session.admin_name}</strong>
              <span>{session.email || '未绑定邮箱'}</span>
              <span className="a-side__role">{session.role === 'super_admin' ? 'super_admin' : 'operator'}</span>
            </div>
          </div>
          <button className="a-btn a-btn--quiet a-btn--sm" type="button" onClick={onLogout}><LogOutIcon />退出登录</button>
        </div>
      </aside>

      <div className="a-main">
        <header className="a-top">
          <button
            ref={navTriggerRef}
            className="a-icon-btn a-side-toggle"
            type="button"
            aria-label="打开导航"
            title="打开导航"
            onClick={() => setNavOpen(true)}
          >
            <MenuIcon />
          </button>
          <div className="a-top__crumbs">
            <span>控制台</span>
            <ChevronRightIcon />
            <strong>{currentTitle}</strong>
          </div>
          <div className="a-top__spacer" />
          <div className="a-top__actions">
            <div className="a-top__pulse" title="上游状态">
              <span className={cn('a-dot', providers.some((item) => item.status !== 'healthy') ? 'is-warn' : 'is-ok')} />
              <span>上游 {providers.length}</span>
            </div>
            <span className={cn('a-badge', reviewCount > 0 ? 'a-badge--warn' : 'a-badge--ok')}>待审 {reviewCount}</span>
            {configDrafts > 0 ? <span className="a-badge a-badge--info">草稿 {configDrafts}</span> : null}
            <button
              className="a-icon-btn"
              type="button"
              aria-label={theme === 'dark' ? '切换到亮色模式' : '切换到暗色模式'}
              title={theme === 'dark' ? '切换到亮色模式' : '切换到暗色模式'}
              onClick={() => setTheme((current) => current === 'dark' ? 'light' : 'dark')}
            >
              {theme === 'dark' ? <SunIcon /> : <MoonIcon />}
            </button>
            <div className="a-top__account">
              <button
                ref={accountButtonRef}
                type="button"
                className="a-cell-entity a-top__avatar-button"
                aria-haspopup="menu"
                aria-expanded={accountOpen}
                aria-label="管理员账户菜单"
                onClick={() => setAccountOpen((current) => !current)}
              >
                <span className="a-cell-entity__avatar">{session.admin_name.slice(0, 1).toUpperCase()}</span>
              </button>
              {accountOpen ? (
                <div ref={accountMenuRef} className="a-account-menu" role="menu" aria-label="管理员账户菜单">
                  <div className="a-account-menu__head">
                    <span className="a-cell-entity__avatar">{session.admin_name.slice(0, 1).toUpperCase()}</span>
                    <div className="a-cell-entity__copy">
                      <span className="a-cell-entity__name">{session.admin_name}</span>
                      <span className="a-cell-entity__meta">{session.email}</span>
                    </div>
                  </div>
                  <button className="a-account-menu__item a-account-menu__item--danger" type="button" role="menuitem" onClick={onLogout}>
                    <LogOutIcon />退出后台
                  </button>
                </div>
              ) : null}
            </div>
          </div>
        </header>

        <div className="a-content">
          {children}
        </div>
      </div>
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
    <aside className="a-toast-stack" aria-live="polite" aria-label="操作反馈">
      {toasts.map((toast) => (
        <button key={toast.id} type="button" className={cn('a-toast', toast.tone === 'success' ? 'a-toast--ok' : toast.tone === 'danger' ? 'a-toast--bad' : '')} onClick={() => onDismiss(toast.id)}>
          <div>
            <strong>{toast.title}</strong>
            {toast.detail ? <span>{toast.detail}</span> : null}
          </div>
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
  eyebrow,
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
    <header className="a-page-head">
      <div className="a-page-head__copy">
        <span className="a-kicker">{eyebrow ?? 'Mikiko Ops'}</span>
        <h1>{title}</h1>
        {resolvedDescription ? <p>{resolvedDescription}</p> : null}
        {meta ? <div>{meta}</div> : null}
      </div>
      {resolvedActions ? <div className="a-page-head__actions">{resolvedActions}</div> : null}
    </header>
  )
}

export function PageToolbar({ children, actions }: { children: React.ReactNode; actions?: React.ReactNode }) {
  return (
    <section className="a-panel a-panel--pad" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-end', justifyContent: 'space-between', gap: 12 }}>
      <div style={{ display: 'flex', minWidth: 0, flex: '1 1 auto', flexWrap: 'wrap', alignItems: 'flex-end', gap: 8 }}>{children}</div>
      {actions ? <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8 }}>{actions}</div> : null}
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
      className={cn('a-tabs', vertical && 'a-tabs--vertical', className)}
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
            className={cn('a-tab', active && 'is-active')}
            onClick={() => onChange(item.id)}
            onKeyDown={(event) => {
              if (moveFocus(index, event.key)) event.preventDefault()
            }}
          >
            <span>{item.label}</span>
            {item.badge ? <span className="a-tab__count">{item.badge}</span> : null}
            {vertical && item.description ? <span className="a-hint">{item.description}</span> : null}
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
  const primaryFields = fields.filter((field) => field.primary)
  const advancedFields = fields.filter((field) => !field.primary)
  const setOpen = (next: boolean) => {
    setInternalOpen(next)
    onAdvancedOpenChange?.(next)
  }

  return (
    <section className={cn('a-filters', open && 'is-expanded')}>
      <div className="a-filters__row">
        {primaryFields.map((field) => (
          <label key={field.key} className="a-field">
            {field.label ? <span>{field.label}</span> : null}
            {field.control}
          </label>
        ))}
      </div>
      {advancedFields.length ? (
        <div className="a-filters__row a-filters__more">
          {advancedFields.map((field) => (
            <label key={field.key} className="a-field">
              {field.label ? <span>{field.label}</span> : null}
              {field.control}
            </label>
          ))}
        </div>
      ) : null}
      <div className="a-filters__foot">
        <span className="a-filters__result">{primaryFields.length + advancedFields.length} 个筛选条件</span>
        <div className="a-filters__actions">
          {advancedFields.length ? (
            <button
              type="button"
              className="a-btn a-btn--sm a-btn--quiet"
              aria-expanded={open}
              onClick={() => setOpen(!open)}
            >
              {open ? '收起筛选' : `更多筛选 ${advancedFields.length}`}
            </button>
          ) : null}
          {actions}
        </div>
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
    <section className={cn('a-panel a-panel--flush', className)}>
      {(filters || actions || resultSummary) ? (
        <header className="a-panel__head">
          <div style={{ minWidth: 0, flex: '1 1 auto' }}>{filters}</div>
          {(actions || resultSummary) ? <div className="a-row-actions">{resultSummary}{actions}</div> : null}
        </header>
      ) : null}
      <div style={{ minWidth: 0, overflow: 'hidden' }}>
        {children}
      </div>
      {pagination ? <footer className="a-pager">{pagination}</footer> : null}
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
    <section className={variant === 'panel' ? 'a-panel a-panel--pad' : 'a-grid'} style={{ gap: variant === 'panel' ? 0 : 12 }}>
      {title || description ? (
        <header className="a-section-title" style={{ marginBottom: 12 }}>
          <div>
            {title ? <h2 style={{ margin: 0 }}>{title}</h2> : null}
            {description ? <p>{description}</p> : null}
          </div>
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
    primary: 'a-btn--primary',
    secondary: '',
    ghost: 'a-btn--quiet',
    danger: 'a-btn--danger',
    text: 'a-btn--quiet',
  }[variant]
  return (
    <button ref={ref} className={cn('a-btn', variantClass, size === 'sm' && 'a-btn--sm', className)} type="button" {...props}>
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
    <button className={cn('a-icon-btn', className)} type="button" aria-label={label} title={title ?? label} {...props}>
      {children ?? '·'}
    </button>
  )
}

export function TooltipIconButton({
  label,
  disabledReason,
  disabled,
  children,
  className,
  ...props
}: Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'aria-label' | 'title'> & {
  label: string
  disabledReason?: string
}) {
  const anchorRef = useRef<HTMLSpanElement | null>(null)
  const tooltipID = useId()
  const [open, setOpen] = useState(false)
  const [position, setPosition] = useState<{ left: number; top: number; placement: 'top' | 'bottom' }>({ left: 0, top: 0, placement: 'top' })
  const tooltipText = disabled && disabledReason ? disabledReason : label

  useEffect(() => {
    if (!open) return undefined
    const updatePosition = () => {
      const rect = anchorRef.current?.getBoundingClientRect()
      if (!rect) return
      const placement = rect.top < 72 ? 'bottom' : 'top'
      const halfWidth = 130
      setPosition({
        left: Math.min(window.innerWidth - halfWidth - 12, Math.max(halfWidth + 12, rect.left + rect.width / 2)),
        top: placement === 'bottom' ? rect.bottom + 8 : rect.top - 8,
        placement,
      })
    }
    updatePosition()
    window.addEventListener('resize', updatePosition)
    window.addEventListener('scroll', updatePosition, true)
    return () => {
      window.removeEventListener('resize', updatePosition)
      window.removeEventListener('scroll', updatePosition, true)
    }
  }, [open])

  return (
    <span
      ref={anchorRef}
      className="inline-flex"
      tabIndex={disabled ? 0 : -1}
      aria-label={disabled ? tooltipText : undefined}
      aria-describedby={open ? tooltipID : undefined}
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
      onFocus={() => setOpen(true)}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setOpen(false)
      }}
    >
      <button
        className={cn(adminShell.iconButton, 'size-10 [&_svg]:size-5', 'disabled:cursor-not-allowed disabled:opacity-45', className)}
        type="button"
        aria-label={label}
        aria-describedby={open ? tooltipID : undefined}
        disabled={disabled}
        {...props}
      >
        {children}
      </button>
      {open ? createPortal(
        <span
          id={tooltipID}
          className={cn(
            'pointer-events-none fixed z-[200] max-w-[260px] -translate-x-1/2 rounded-md border border-[var(--border)] bg-[var(--fg)] px-2.5 py-1.5 text-center text-xs font-medium text-[var(--surface-solid)] shadow-[var(--pg-shadow-sm)]',
            position.placement === 'top' && '-translate-y-full',
          )}
          role="tooltip"
          style={{ left: position.left, top: position.top }}
        >
          {tooltipText}
        </span>,
        document.body,
      ) : null}
    </span>
  )
}

export function RefreshIconButton({ label = '刷新列表', refreshing = false, disabled = false, onClick }: {
  label?: string
  refreshing?: boolean
  disabled?: boolean
  onClick: () => void
}) {
  return <TooltipIconButton label={label} disabled={disabled || refreshing} onClick={onClick}><RefreshCw className={cn('size-4', refreshing && 'animate-spin')} aria-hidden="true" /></TooltipIconButton>
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
    <div role="tablist" className="a-seg">
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          role="tab"
          aria-selected={value === option.value}
          disabled={option.disabled}
          className={cn('a-seg__item', value === option.value && 'is-active')}
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
    <section className="a-empty">
      <span className="a-empty__icon"><LoaderIcon className="size-6 animate-spin" /></span>
      <h3>{label}</h3>
      <p>正在请求真实后台 API，并同步构建管理端骨架数据。</p>
      <div style={{ display: 'grid', width: '100%', maxWidth: 360, gap: 8 }}>
        <div className="a-skeleton" style={{ height: 14 }} />
        <div className="a-skeleton" style={{ height: 14, width: '80%' }} />
        <div className="a-skeleton" style={{ height: 46 }} />
      </div>
    </section>
  )
}

export function ErrorBlock({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <section className="a-empty" role="alert">
      <span className="a-empty__icon" style={{ color: 'var(--a-danger)' }}><AlertIcon className="size-6" /></span>
      <h3>加载失败</h3>
      <p>{message}</p>
      <button type="button" className="a-btn a-btn--primary" onClick={onRetry}>重试</button>
    </section>
  )
}

export function EmptyBlock({ title, detail, action, icon = <EmptyIcon className="size-6" />, variant = 'framed' }: { title: string; detail: string; action?: React.ReactNode; icon?: React.ReactNode; variant?: 'framed' | 'inline' }) {
  return (
    <section className={cn('a-empty', variant === 'inline' && 'a-empty--inline')}>
      <span className="a-empty__icon">{icon}</span>
      <h3>{title}</h3>
      <p>{detail}</p>
      {action}
    </section>
  )
}

export function Badge({ tone = 'neutral', children }: { tone?: ToastTone | 'success' | 'primary'; children: React.ReactNode }) {
  const toneClass = tone === 'success' ? 'a-badge--ok'
    : tone === 'warning' ? 'a-badge--warn'
      : tone === 'danger' ? 'a-badge--bad'
        : tone === 'primary' ? 'a-badge--accent'
          : ''
  return <span className={cn('a-badge', toneClass)}>{children}</span>
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
    <div ref={layerRef} className="a-layer" role="presentation" onMouseDown={onClose}>
      <div className="a-scrim is-open" aria-hidden="true" />
      <section ref={dialogRef} tabIndex={-1} data-admin-motion-panel className="a-modal is-open" role="dialog" aria-modal="true" aria-label={title} onMouseDown={(event) => event.stopPropagation()}>
        <header className="a-modal__head">
          <div>
            <h2>{title}</h2>
            {detail ? <p className="a-hint">{detail}</p> : null}
          </div>
          <button type="button" className="a-btn a-btn--sm a-btn--quiet" onClick={onClose}>关闭</button>
        </header>
        <div>{children}</div>
        <footer className="a-modal__foot">{footer}</footer>
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
  wide = false,
  stacked = false,
  headerAside,
}: {
  title: string
  description?: string
  children: React.ReactNode
  footer?: React.ReactNode
  onClose: () => void
  wide?: boolean
  stacked?: boolean
  headerAside?: React.ReactNode
}) {
  const dialogRef = useRef<HTMLElement | null>(null)
  const layerRef = useRef<HTMLDivElement | null>(null)
  useDialogFocus(onClose, dialogRef)
  useAdminLayerMotion(layerRef)

  return (
    <div ref={layerRef} className={cn('a-layer', stacked && 'a-layer--stacked')} role="presentation" onMouseDown={onClose}>
      {stacked ? null : <div className="a-scrim is-open" aria-hidden="true" />}
      <aside
        ref={dialogRef}
        tabIndex={-1}
        data-admin-motion-panel
        className={cn('a-drawer is-open', wide && 'a-drawer--wide', stacked && 'is-stacked')}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <header className="a-drawer__head">
          <div>
            <h2>{title}</h2>
            {description ? <p>{description}</p> : null}
          </div>
          {headerAside ? <div className="a-drawer__spacer">{headerAside}</div> : <div className="a-drawer__spacer" />}
          <button type="button" className="a-icon-btn" aria-label="关闭" onClick={onClose}><XIcon /></button>
        </header>
        <section className="a-drawer__body">{children}</section>
        {footer ? <footer className="a-drawer__foot">{footer}</footer> : null}
      </aside>
    </div>
  )
}

function routeIcon(route: ProtectedAdminRouteId) {
  const icons: Record<ProtectedAdminRouteId, React.ReactNode> = {
    dashboard: <DashboardIcon className={navIconClass} />,
    monitoring: <MonitoringIcon className={navIconClass} />,
    cluster: <ClusterIcon className={navIconClass} />,
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
    'video-tasks': <CallRecordsIcon className={navIconClass} />,
    'media-policy': <SystemSettingsIcon className={navIconClass} />,
    audit: <AuditIcon className={navIconClass} />,
    'system-users': <SystemUsersIcon className={navIconClass} />,
    'system-settings': <SystemSettingsIcon className={navIconClass} />,
  }
  return icons[route]
}

export function Field({ label, children, error, hint }: { label: string; children: React.ReactNode; error?: string | null; hint?: string }) {
  return (
    <label className="a-field">
      <span>
        <span>{label}</span>
        {hint ? <FieldHint text={hint} /> : null}
      </span>
      {children}
      {error ? <em className="a-field__error">{error}</em> : null}
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
        className="grid size-[18px] shrink-0 place-items-center rounded-full border border-[var(--line)] text-[11px] text-[var(--blue)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--blue)]"
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
        <span className={cn(
          'pointer-events-none fixed z-[200] max-w-[260px] -translate-x-1/2 rounded-lg border border-[var(--line)] bg-[var(--text)] px-3 py-2 text-xs font-medium normal-case tracking-normal text-white shadow-[var(--pg-shadow-lg)]',
          position.placement === 'top' && '-translate-y-full',
        )} role="tooltip" data-placement={position.placement} style={{ left: position.left, top: position.top }}>{text}</span>,
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
    <section className="a-metrics" aria-label="关键运营指标">
      {metrics.map((metric) => (
        <div key={metric.label} className={cn('a-metric', metricToneClass[metric.tone])}>
          <div className="a-metric__label"><span>{metric.label}</span></div>
          <div className="a-metric__value">{metric.value}</div>
          <div className="a-metric__foot">{metric.trend}</div>
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
