import React, { useEffect, useMemo, useState } from 'react'
import type { AdminMetric, AdminSession, ProviderHealth } from '../../shared/api-types'
import type { AdminRouteId, ToastMessage, ToastTone } from './types'

export const protectedRoutes: Exclude<AdminRouteId, 'login'>[] = [
  'overview',
  'config',
  'routing',
  'pricing',
  'reviews',
  'users',
  'redeem',
  'call-records',
  'provider-models',
  'audit',
  'health',
]

export const navGroups: Array<{ label: string; items: Array<{ id: Exclude<AdminRouteId, 'login'>; label: string; hint: string }> }> = [
  {
    label: '概览',
    items: [
      { id: 'overview', label: '控制面板', hint: 'Overview' },
      { id: 'health', label: '系统状态', hint: 'Health' },
    ],
  },
  {
    label: '业务管理',
    items: [
      { id: 'users', label: '用户管理', hint: 'Users' },
      { id: 'redeem', label: '兑换码', hint: 'Redeem' },
      { id: 'reviews', label: '审核队列', hint: 'Review' },
      { id: 'call-records', label: '调用记录', hint: 'Calls' },
    ],
  },
  {
    label: '模型与路由',
    items: [
      { id: 'routing', label: '路由策略', hint: 'Routes' },
      { id: 'provider-models', label: '模型接入', hint: 'Models' },
      { id: 'pricing', label: '价格配置', hint: 'Pricing' },
    ],
  },
  {
    label: '系统',
    items: [
      { id: 'audit', label: '审计日志', hint: 'Trail' },
      { id: 'config', label: '系统设置', hint: 'Config' },
    ],
  },
]

const fallbackProviders: ProviderHealth[] = [
  { provider: 'OpenAI', status: 'healthy', latency_ms: 0, error_rate: '0%', note: '健康' },
  { provider: 'OpenRouter', status: 'healthy', latency_ms: 0, error_rate: '0%', note: '健康' },
  { provider: '任务队列', status: 'degraded', latency_ms: 0, error_rate: '0%', note: '拥堵 (12)' },
]

export function normalizeRoute(hash: string): AdminRouteId {
  const path = hash.replace(/^#\/?/, '').split('?')[0]
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
  route: Exclude<AdminRouteId, 'login'>
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

  return (
    <main className="admin-shell">
      <aside className="sidebar admin-sidebar" aria-label="Pic Gallery Admin Navigation">
        <a className="admin-brand" href={routeHref('overview')} onClick={() => onNavigate('overview')}>
          <span>Pic Gallery Admin</span>
          <strong>Admin Console</strong>
        </a>

        <nav className="admin-nav" aria-label="后台主导航">
          {navGroups.map((group) => (
            <section key={group.label} className="nav-group">
              <p className="nav-label">{group.label}</p>
              {group.items.map((item) => (
                <a
                  key={item.id}
                  href={routeHref(item.id)}
                  className={`nav-item${route === item.id ? ' active' : ''}`}
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

        <div className="admin-side-note">
          <span>Soft Grid Ops</span>
          <strong>反馈沿主操作路径呈现，配置均可回滚。</strong>
        </div>
      </aside>

      <section className="admin-main">
        <header className="topbar admin-global-topbar">
          <div className="status-group console-alert-row" aria-label="Provider 状态">
            {statusProviders.map((provider) => (
              <StatusItem
                key={provider.provider}
                label={provider.provider}
                value={provider.status === 'healthy' ? '健康' : provider.note}
                warn={provider.status !== 'healthy'}
              />
            ))}
          </div>

          <div className="console-meta-row">
            <div className="console-provider-pill">
              <span className="pill-dot" />
              <em>{generationMetric ? `${generationMetric.label} ${generationMetric.value}` : 'Real API online'}</em>
            </div>
            <div className="avatar-widget admin-avatar-widget">
              <span className="avatar-orb">{session.admin_name.slice(0, 2).toUpperCase()}</span>
              <div>
                <strong>{session.admin_name}</strong>
                <span>{session.role}</span>
              </div>
            </div>
            <button className="ghost compact" type="button" onClick={onLogout}>退出</button>
          </div>
        </header>

        <section className="ops-status-strip" aria-label="运营状态条">
          <StatusCell label="环境" value="Production" />
          <StatusCell label="主 Provider" value={providers[0]?.provider ?? 'OpenAI'} />
          <StatusCell label="任务队列" value={providers.find((item) => item.provider === 'Task Worker')?.note ?? 'worker healthy'} />
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
    <div className={`status-item console-chip ${warn ? 'warning' : 'success'}`}>
      <span className={`status-indicator chip-dot${warn ? ' warn' : ''}`} />
      <em>{label}: {value}</em>
    </div>
  )
}

export function StatusChip({ tone, label, value }: { tone: ToastTone | 'primary' | 'success'; label: string; value: string }) {
  return (
    <button type="button" className={`console-chip ${tone}`}>
      <span className="chip-dot" />
      <em>{label}</em>
      <strong>{value}</strong>
    </button>
  )
}

export function StatusCell({ label, value }: { label: string; value: string }) {
  return (
    <div className="status-cell">
      <label>{label}</label>
      <strong>{value}</strong>
    </div>
  )
}

export function ToastRail({ toasts, onDismiss }: { toasts: ToastMessage[]; onDismiss: (id: string) => void }) {
  return (
    <aside className="toast-rail" aria-live="polite" aria-label="操作反馈">
      {toasts.map((toast) => (
        <button key={toast.id} type="button" className={`toast ${toast.tone}`} onClick={() => onDismiss(toast.id)}>
          <strong>{toast.title}</strong>
          {toast.detail ? <span>{toast.detail}</span> : null}
        </button>
      ))}
    </aside>
  )
}

export function PageHeader({ eyebrow, title, detail, actions }: { eyebrow: string; title: string; detail?: string; actions?: React.ReactNode }) {
  return (
    <section className="page-header">
      <div>
        <label>{eyebrow}</label>
        <strong>{title}</strong>
        {detail ? <p>{detail}</p> : null}
      </div>
      {actions ? <div className="page-actions">{actions}</div> : null}
    </section>
  )
}

export function LoadingBlock({ label = '载入运营数据中' }: { label?: string }) {
  return (
    <section className="state-block loading">
      <span className="loader" />
      <strong>{label}</strong>
      <p>正在请求真实后台 API。</p>
    </section>
  )
}

export function ErrorBlock({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <section className="state-block error">
      <strong>加载失败</strong>
      <p>{message}</p>
      <button type="button" className="btn primary" onClick={onRetry}>重试</button>
    </section>
  )
}

export function EmptyBlock({ title, detail }: { title: string; detail: string }) {
  return (
    <section className="state-block empty">
      <strong>{title}</strong>
      <p>{detail}</p>
    </section>
  )
}

export function Badge({ tone = 'neutral', children }: { tone?: ToastTone | 'success' | 'primary'; children: React.ReactNode }) {
  return <span className={`badge ${tone}`}>{children}</span>
}

export function InlineFeedback({ tone, message }: { tone: ToastTone; message: string }) {
  return <div className={`inline-feedback ${tone}`}>{message}</div>
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
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <section className="modal-panel" role="dialog" aria-modal="true" aria-label={title} onMouseDown={(event) => event.stopPropagation()}>
        <header className="modal-head">
          <div>
            <strong>{title}</strong>
            {detail ? <p>{detail}</p> : null}
          </div>
          <button type="button" className="ghost small" onClick={onClose}>关闭</button>
        </header>
        <div className="modal-body">{children}</div>
        <footer className="modal-actions">{footer}</footer>
      </section>
    </div>
  )
}

export function Field({ label, children, error }: { label: string; children: React.ReactNode; error?: string | null }) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
      {error ? <em>{error}</em> : null}
    </label>
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
    <aside className="reason-drawer" role="dialog" aria-modal="false" aria-label={title}>
      <div>
        <label>审核原因</label>
        <strong>{title}</strong>
        <p>{detail}</p>
      </div>
      <textarea value={value} onChange={(event) => onChange(event.target.value)} rows={4} placeholder="写明通过、驳回或下架理由" />
      <div className="drawer-actions">
        <button type="button" className="ghost" onClick={onCancel} disabled={busy}>取消</button>
        <button type="button" className={`btn ${tone === 'danger' ? 'danger' : 'primary'}`} onClick={onConfirm} disabled={busy || value.trim().length < 3}>
          {busy ? '提交中...' : decisionLabel}
        </button>
      </div>
    </aside>
  )
}

export function MetricGrid({ metrics }: { metrics: AdminMetric[] }) {
  return (
    <section className="grid-stats metric-grid">
      {metrics.map((metric) => (
        <div key={metric.label} className={`stat-card metric-card ${metric.tone}`}>
          <div className="stat-label">
            <label>{metric.label}</label>
          </div>
          <div className="stat-value">
            <strong>{metric.value}</strong>
            <span className={`stat-trend ${metric.tone === 'good' ? 'trend-up' : ''}`}>{metric.trend}</span>
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
