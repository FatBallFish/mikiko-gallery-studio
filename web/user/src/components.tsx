import React, { createContext, useContext, useEffect } from 'react'
import type { ImageTaskStatus, ImageTaskType, PublishStatus } from '../../shared/api-types'
import type { AppContextValue, RouteId, Toast } from './types'

export const AppContext = createContext<AppContextValue | null>(null)

export function useApp() {
  const value = useContext(AppContext)
  if (!value) throw new Error('useApp must be used within AppContext.Provider')
  return value
}

export const protectedRoutes: RouteId[] = ['home', 'genpic', 'gallery', 'api-keys', 'profile', 'docs']

function HomeIcon() {
  return <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M3 9l9-7 9 7v11a2 2 0 01-2 2H5a2 2 0 01-2-2z" /></svg>
}
function SparklesIcon() {
  return <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M12 2v20M2 12h20M5 5l14 14M5 19L19 5" /></svg>
}
function GridIcon() {
  return <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M9 3v18M15 3v18M3 9h18M3 15h18" /></svg>
}
function ClawIcon() {
  return <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M16 18l6-6-6-6M8 6l-6 6 6 6" /></svg>
}
function DocsIcon() {
  return <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" /><path d="M14 2v6h6M16 13H8M16 17H8M10 9H8" /></svg>
}
function UserIcon() {
  return <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M20 21v-2a4 4 0 00-4-4H8a4 4 0 00-4 4v2" /><circle cx="12" cy="7" r="4" /></svg>
}

export const navItems: Array<{ route: RouteId; short: string; label: string; icon: React.ReactNode }> = [
  { route: 'home', short: 'HOME', label: '首页', icon: <HomeIcon /> },
  { route: 'genpic', short: 'GEN', label: '生图', icon: <SparklesIcon /> },
  { route: 'gallery', short: 'IMG', label: '图库', icon: <GridIcon /> },
  { route: 'api-keys', short: 'API', label: 'Claw', icon: <ClawIcon /> },
  { route: 'docs', short: 'DOC', label: '文档', icon: <DocsIcon /> },
  { route: 'profile', short: 'ME', label: '账户', icon: <UserIcon /> },
]

export function Shell({ children }: { children: React.ReactNode }) {
  const app = useApp()
  return (
    <div className="vault-shell">
      <aside className="vault-sidebar" aria-label="Pic Gallery user navigation">
        <button className="vault-brand" type="button" onClick={() => app.navigate('landing')} aria-label="Pic Gallery landing">
          <span className="brand-orb">V</span>
          <span>Pic Gallery</span>
        </button>
        <nav className="vault-nav">
          {navItems.map((item) => (
            <a
              key={item.route}
              href={`#/${item.route}`}
              className={app.route === item.route ? 'active' : ''}
              onClick={(event) => {
                event.preventDefault()
                app.navigate(item.route)
              }}
            >
              {item.icon}
              <small>{item.short}</small>
              <span>{item.label}</span>
            </a>
          ))}
        </nav>
      </aside>
      <main className="vault-main">
        <header className="vault-topbar glass-panel">
          <div className="quick-links" aria-label="Quick entries">
            <button type="button" onClick={() => app.navigate('genpic')}>灵感模板</button>
            <button type="button" onClick={() => app.navigate('gallery')}>公开广场</button>
            <button type="button" onClick={() => app.navigate('docs')}>开发文档</button>
          </div>
          <div className="topbar-tools">
            <span className="top-chip">消息 <b>3</b></span>
            <span className="top-chip">活动 <b>2</b></span>
            <button className="balance-chip" type="button" onClick={() => app.navigate('profile')}>
              <span>◈</span>
              <b>{app.balance?.available_points ?? '...'}</b>
            </button>
            <button className="avatar-chip" type="button" onClick={() => app.navigate('profile')}>
              <span>{app.profile?.avatar_initials ?? 'PG'}</span>
              <b>{app.profile?.display_name ?? 'Guest'}</b>
            </button>
          </div>
        </header>
        <div className="route-surface">{children}</div>
      </main>
    </div>
  )
}

export function ToastViewport({ toasts, onExpire }: { toasts: Toast[]; onExpire: (id: number) => void }) {
  useEffect(() => {
    if (!toasts.length) return undefined
    const timers = toasts.map((toast) => window.setTimeout(() => onExpire(toast.id), 3600))
    return () => timers.forEach(window.clearTimeout)
  }, [toasts, onExpire])

  return (
    <div className="toast-stack" aria-live="polite" aria-atomic="true">
      {toasts.map((toast) => (
        <div key={toast.id} className={`toast toast-${toast.tone}`}>
          <span>{toast.tone === 'success' ? '✓' : toast.tone === 'error' ? '!' : 'i'}</span>
          {toast.message}
        </div>
      ))}
    </div>
  )
}

export function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <label className="field-block">
      <span>{label}</span>
      {children}
      {hint ? <em>{hint}</em> : null}
    </label>
  )
}

export function Button({ children, tone = 'primary', busy = false, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement> & { tone?: 'primary' | 'ghost' | 'danger'; busy?: boolean }) {
  return (
    <button {...props} className={`btn btn-${tone} ${props.className ?? ''}`} disabled={props.disabled || busy}>
      {busy ? <span className="spinner" aria-hidden="true" /> : null}
      {children}
    </button>
  )
}

export function Surface({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return <section className={`glass-panel ${className}`}>{children}</section>
}

export function PageIntro({ eyebrow, title, detail, action }: { eyebrow: string; title: string; detail: string; action?: React.ReactNode }) {
  return (
    <div className="page-intro">
      <div>
        <p className="eyebrow">{eyebrow}</p>
        <h1>{title}</h1>
        <span>{detail}</span>
      </div>
      {action ? <div className="intro-action">{action}</div> : null}
    </div>
  )
}

export function LoadingState({ label = '正在读取实时数据...' }: { label?: string }) {
  return <div className="state-line"><span className="spinner" />{label}</div>
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="state-line error-state">
      <b>读取失败</b>
      <span>{message}</span>
      {onRetry ? <Button tone="ghost" onClick={onRetry}>重试</Button> : null}
    </div>
  )
}

export function EmptyState({ title, detail, action }: { title: string; detail: string; action?: React.ReactNode }) {
  return (
    <div className="empty-state">
      <strong>{title}</strong>
      <span>{detail}</span>
      {action}
    </div>
  )
}

export function Modal({ title, children, onClose }: { title: string; children: React.ReactNode; onClose: () => void }) {
  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <section className="modal-card glass-panel" role="dialog" aria-modal="true" aria-label={title} onMouseDown={(event) => event.stopPropagation()}>
        <div className="modal-head">
          <h2>{title}</h2>
          <button type="button" onClick={onClose} aria-label="关闭">×</button>
        </div>
        {children}
      </section>
    </div>
  )
}

export function StatusPill({ status }: { status: ImageTaskStatus | PublishStatus | string }) {
  const tone = status === 'succeeded' || status === 'public' || status === 'active'
    ? 'good'
    : status === 'failed' || status === 'rejected' || status === 'disabled'
      ? 'bad'
      : status === 'running' || status === 'queued' || status === 'reviewing'
        ? 'warn'
        : 'neutral'
  const label: Record<string, string> = {
    queued: '排队中',
    running: '生成中',
    succeeded: '已完成',
    failed: '失败',
    cancelled: '已取消',
    private: '私有',
    reviewing: '审核中',
    public: '已公开',
    rejected: '已拒绝',
    active: '启用中',
    disabled: '已禁用',
  }
  return <span className={`status-pill ${tone}`}>{label[status] ?? status}</span>
}

export function taskTypeLabel(type: ImageTaskType | string) {
  const labels: Record<string, string> = {
    text_to_image: '文生图',
    reference_to_image: '参考生图',
    image_edit: '图片编辑',
  }
  return labels[type] ?? type
}

export function formatDate(date: string) {
  return date.slice(0, 16)
}

export async function copyText(text: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text)
    return
  }
  const input = document.createElement('textarea')
  input.value = text
  input.setAttribute('readonly', 'true')
  input.style.position = 'fixed'
  input.style.opacity = '0'
  document.body.appendChild(input)
  input.select()
  document.execCommand('copy')
  document.body.removeChild(input)
}

export function CopyButton({ text, label = '复制' }: { text: string; label?: string }) {
  const app = useApp()
  return (
    <Button tone="ghost" onClick={async () => {
      await copyText(text)
      app.notify('success', '已复制到剪贴板')
    }}>{label}</Button>
  )
}
