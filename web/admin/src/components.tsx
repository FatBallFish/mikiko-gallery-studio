import React, { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import type { AdminMetric, AdminSession, ProviderHealth, UserGroup } from '../../shared/api-types'
import { cn } from '../../shared/classnames'
import { providerHealthValue, providerHealthWarn, taskQueuePressure } from './healthRows'
import { filterAdminNavGroups } from './types'
import type { AdminNavGroup, AdminRouteId, ProtectedAdminRouteId, ToastMessage, ToastTone } from './types'
import { adminButton, adminShell } from './ui/classes'

export const protectedRoutes: ProtectedAdminRouteId[] = [
  'overview',
  'readiness',
  'config',
  'security-config',
  'routing',
  'pricing',
  'reviews',
  'users',
  'user-groups',
  'redeem',
  'cashier',
  'call-records',
  'provider-models',
  'audit',
  'health',
]

export const navGroups: AdminNavGroup[] = [
  {
    label: '概览',
    items: [
      { id: 'overview', label: '控制面板', hint: 'Overview' },
      { id: 'readiness', label: '上线检查', hint: 'Ready' },
      { id: 'health', label: '系统状态', hint: 'Health' },
    ],
  },
  {
    label: '业务管理',
    items: [
      { id: 'users', label: '用户管理', hint: 'Users' },
      { id: 'user-groups', label: '分组管理', hint: 'Groups' },
      { id: 'redeem', label: '兑换码', hint: 'Redeem' },
      { id: 'reviews', label: '审核队列', hint: 'Review' },
      { id: 'call-records', label: '调用记录', hint: 'Calls' },
    ],
  },
  {
    label: '商业化',
    items: [
      { id: 'cashier', label: '收银台', hint: 'Cashier' },
    ],
  },
  {
    label: '模型与路由',
    items: [
      { id: 'routing', label: '路由模型', hint: 'Routes' },
      { id: 'provider-models', label: '接入账号', hint: 'Accounts' },
      { id: 'pricing', label: '价格配置', hint: 'Pricing' },
    ],
  },
  {
    label: '系统',
    items: [
      { id: 'audit', label: '审计日志', hint: 'Trail' },
      { id: 'config', label: '系统设置', hint: 'Config' },
      { id: 'security-config', label: '安全配置', hint: 'Secrets' },
    ],
  },
]

const fallbackProviders: ProviderHealth[] = [
  { provider: 'OpenAI', status: 'healthy', latency_ms: 0, error_rate: '0%', note: '健康' },
  { provider: 'OpenRouter', status: 'healthy', latency_ms: 0, error_rate: '0%', note: '健康' },
  { provider: '任务队列', status: 'degraded', latency_ms: 0, error_rate: '0%', note: '拥堵 (12)' },
]

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

const stateBlockBase = 'grid min-h-[260px] place-items-center content-center gap-2.5 rounded-[var(--pg-radius-sm)] border border-dashed border-[var(--line-strong)] bg-white/55 p-7 text-center'
const fieldLabelClass = 'flex items-center justify-between gap-2 text-[10px] font-extrabold uppercase tracking-[.14em] text-[var(--soft)]'
const checkGridClass = 'grid max-h-[220px] gap-2 overflow-auto rounded-[10px] border border-[var(--line)] bg-white/60 p-2'
const checkGridEmptyClass = 'grid-cols-1 text-sm font-bold text-[var(--soft)]'
const checkOptionClass = 'grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 rounded-lg border border-[var(--line)] bg-white/70 p-2 text-sm has-[:checked]:border-[var(--blue)] has-[:checked]:bg-[rgba(87,117,185,.08)]'
const checkOptionNameClass = 'min-w-0 overflow-hidden text-ellipsis whitespace-nowrap'
const checkOptionMetaClass = 'text-xs not-italic font-extrabold text-[var(--soft)]'
const metricToneClass: Record<string, string> = {
  good: '[&_span]:text-[var(--green)]',
  warn: '[&_span]:text-[var(--amber)]',
  bad: '[&_span]:text-[var(--red)]',
  danger: '[&_span]:text-[var(--red)]',
}

export function normalizeRoute(hash: string): AdminRouteId {
  const path = hash.replace(/^#\/?/, '').split('?')[0].replace(/^\/+|\/+$/g, '')
  if (path === 'login') return 'login'
  return protectedRoutes.includes(path as Exclude<AdminRouteId, 'login'>) ? (path as AdminRouteId) : 'overview'
}

export function routeHref(route: AdminRouteId) {
  return `#/${route}`
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
  const generationMetric = metrics.find((item) => item.label.includes('生成'))
  const statusProviders = providers.length ? providers : fallbackProviders
  const visibleNavGroups = filterAdminNavGroups(navGroups, session)

  return (
    <main className={adminShell.root}>
      <aside className={adminShell.sidebar} aria-label="Pic Gallery Admin Navigation">
        <a className={adminShell.brand} href={routeHref('overview')} onClick={() => onNavigate('overview')}>
          <span>Pic Gallery Admin</span>
          <strong>Admin Console</strong>
        </a>

        <nav className={adminShell.nav} aria-label="后台主导航">
          {visibleNavGroups.map((group) => (
            <section key={group.label} className={adminShell.navGroup}>
              <p className={adminShell.navLabel}>{group.label}</p>
              {group.items.map((item) => (
                <a
                  key={item.id}
                  href={routeHref(item.id)}
                  className={cn(adminShell.navLink, route === item.id && adminShell.navLinkActive)}
                  onClick={() => onNavigate(item.id)}
                  aria-current={route === item.id ? 'page' : undefined}
                >
                  <span>{item.label}</span>
                  <em>{item.hint}</em>
                </a>
              ))}
            </section>
          ))}
        </nav>

        <div className={adminShell.sideNote}>
          <span>Soft Grid Ops</span>
          <strong>反馈沿主操作路径呈现，配置均可回滚。</strong>
        </div>
      </aside>

      <section className={adminShell.main}>
        <header className={adminShell.topbar}>
          <div className={adminShell.flexRow} aria-label="Provider 状态">
            {statusProviders.map((provider) => (
              <StatusItem
                key={provider.provider}
                label={provider.provider}
                value={providerHealthValue(provider)}
                warn={providerHealthWarn(provider)}
              />
            ))}
          </div>

          <div className={adminShell.metaRow}>
            <div className={adminShell.providerPill}>
              <span className={cn('inline-block size-2 rounded-full', dotToneClass.success)} />
              <em>{generationMetric ? `${generationMetric.label} ${generationMetric.value}` : 'Real API online'}</em>
            </div>
            <div className={adminShell.avatarWidget}>
              <span className={adminShell.avatarOrb}>{session.admin_name.slice(0, 2).toUpperCase()}</span>
              <div>
                <strong>{session.admin_name}</strong>
                <span>{session.role}</span>
              </div>
            </div>
            <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={onLogout}>退出</button>
          </div>
        </header>

        <section className={adminShell.statusStrip} aria-label="运营状态条">
          <StatusCell label="环境" value="Production" />
          <StatusCell label="主 Provider" value={providers[0]?.provider ?? 'OpenAI'} />
          <StatusCell label="任务队列" value={taskQueuePressure(providers)} />
          <StatusCell label="策略状态" value={configDrafts ? `${configDrafts} 项待发布` : '全量已生效'} />
          <StatusCell label="待审核" value={`${String(reviewCount).padStart(2, '0')} 项`} />
        </section>

        {children}
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
    <div className={adminShell.statusCell}>
      <label className={adminShell.statusLabel}>{label}</label>
      <strong className={adminShell.statusValue}>{value}</strong>
    </div>
  )
}

export function StatusStrip({ children, columns = 5 }: { children: React.ReactNode; columns?: 4 | 5 }) {
  const columnClass = columns === 4
    ? 'grid-cols-4 max-[920px]:grid-cols-2 max-[620px]:grid-cols-1'
    : 'grid-cols-5 max-[1260px]:grid-cols-3 max-[920px]:grid-cols-2 max-[620px]:grid-cols-1'

  return (
    <section className={cn('grid overflow-hidden rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-[var(--surface-frost)] shadow-[var(--pg-shadow-sm)] backdrop-blur-[14px]', columnClass)} aria-label="运营状态条">
      {children}
    </section>
  )
}

export function ToastRail({ toasts, onDismiss }: { toasts: ToastMessage[]; onDismiss: (id: string) => void }) {
  return (
    <aside className="fixed right-5 top-5 z-[120] grid w-[min(380px,calc(100vw-40px))] gap-2" aria-live="polite" aria-label="操作反馈">
      {toasts.map((toast) => (
        <button key={toast.id} type="button" className={cn('grid rounded-xl border border-[var(--line)] bg-white p-3 text-left shadow-[var(--pg-shadow-sm)]', toastToneClass[toast.tone])} onClick={() => onDismiss(toast.id)}>
          <strong>{toast.title}</strong>
          {toast.detail ? <span>{toast.detail}</span> : null}
        </button>
      ))}
    </aside>
  )
}

export function PageHeader({ eyebrow, title, detail, actions }: { eyebrow: string; title: string; detail?: string; actions?: React.ReactNode }) {
  return (
    <section className="flex items-center justify-between gap-4 rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-[var(--surface-frost)] p-5 shadow-[var(--pg-shadow-sm)] backdrop-blur-[14px] max-[920px]:flex-col max-[920px]:items-start">
      <div>
        <label className="m-0 text-[10px] font-extrabold uppercase tracking-[.14em] text-[var(--soft)]">{eyebrow}</label>
        <strong className="mt-0.5 block text-xl font-medium text-[var(--text)]">{title}</strong>
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
    <div className="fixed inset-0 z-[90] grid place-items-center bg-[rgba(26,37,50,0.24)] p-6 backdrop-blur-sm" role="presentation" onMouseDown={onClose}>
      <section className="grid max-h-[92vh] w-[min(760px,calc(100vw-48px))] gap-5 overflow-auto rounded-[18px] border border-[var(--line)] bg-white p-5 shadow-[0_24px_80px_rgba(26,37,50,.18)]" role="dialog" aria-modal="true" aria-label={title} onMouseDown={(event) => event.stopPropagation()}>
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
    <aside className="grid min-h-full content-start gap-3 border-l border-[var(--line)] bg-[rgba(248,250,251,.82)] p-4" role="dialog" aria-modal="false" aria-label={title}>
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
    <section className="grid grid-cols-[repeat(auto-fit,minmax(170px,1fr))] gap-3 max-[1260px]:grid-cols-2 max-[620px]:grid-cols-1">
      {metrics.map((metric) => (
        <div key={metric.label} className={cn('grid min-h-[104px] gap-2 rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-white p-4', metricToneClass[metric.tone])}>
          <div>
            <label>{metric.label}</label>
          </div>
          <div>
            <strong className="my-1 block text-[1.8rem] font-medium">{metric.value}</strong>
            <span className="text-xs font-extrabold text-[var(--soft)]">{metric.trend}</span>
          </div>
        </div>
      ))}
    </section>
  )
}

export function useFilteredTabs<T extends { tab?: string }>(rows: T[]) {
  const tabs = useMemo(() => Array.from(new Set(rows.map((row) => row.tab).filter(Boolean))) as string[], [rows])
  const [activeTab, setActiveTab] = useState<string>('全部')
  const visibleRows = activeTab === '全部' ? rows : rows.filter((row) => row.tab === activeTab)
  return { tabs: ['全部', ...tabs], activeTab, setActiveTab, visibleRows }
}
