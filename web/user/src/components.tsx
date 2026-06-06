import React, { createContext, useContext, useEffect, useRef, useState } from 'react'
import type { GalleryImage, ImageResult, ImageTaskStatus, ImageTaskType, PublishStatus } from '../../shared/api-types'
import { avatarMenuItems, type AvatarMenuIcon } from './avatarMenu'
import { publicEngagementStats } from './publicEngagementModel'
import { topbarStatusChips } from './topbarStatus'
import type { AppContextValue, RouteId, Toast } from './types'

export const AppContext = createContext<AppContextValue | null>(null)

export function useApp() {
  const value = useContext(AppContext)
  if (!value) throw new Error('useApp must be used within AppContext.Provider')
  return value
}

export function ImageLightbox({ image, onClose }: { image: { url: string; alt: string } | null; onClose: () => void }) {
  useEffect(() => {
    if (!image) return undefined
    const close = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', close)
    return () => window.removeEventListener('keydown', close)
  }, [image, onClose])

  if (!image) return null
  return (
    <div className="image-lightbox" role="dialog" aria-modal="true" aria-label="图片预览" onMouseDown={onClose}>
      <button type="button" className="image-lightbox-close" aria-label="关闭预览" onClick={onClose}>×</button>
      <img src={image.url} alt={image.alt} onMouseDown={(event) => event.stopPropagation()} />
    </div>
  )
}

export function PublicDetailIcon({ name, active }: { name: 'eye' | 'heart' | 'star' | 'download' | 'copy' | 'edit' | 'public' | 'group' | 'delete'; active?: boolean }) {
  const common = { width: 18, height: 18, viewBox: '0 0 24 24', fill: active && (name === 'heart' || name === 'star') ? 'currentColor' : 'none', stroke: 'currentColor', strokeWidth: 2, strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const }
  if (name === 'eye') return <svg {...common}><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z" /><circle cx="12" cy="12" r="3" /></svg>
  if (name === 'heart') return <svg {...common}><path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.6l-1-1a5.5 5.5 0 0 0-7.8 7.8l1 1L12 21l7.8-7.6 1-1a5.5 5.5 0 0 0 0-7.8Z" /></svg>
  if (name === 'star') return <svg {...common}><path d="m12 2 3.1 6.3 6.9 1-5 4.9 1.2 6.8-6.2-3.3L5.8 21 7 14.2 2 9.3l6.9-1Z" /></svg>
  if (name === 'download') return <svg {...common}><path d="M12 3v12" /><path d="m7 10 5 5 5-5" /><path d="M5 21h14" /></svg>
  if (name === 'edit') return <svg {...common}><path d="M12 20h9" /><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z" /></svg>
  if (name === 'public') return <svg {...common}><circle cx="12" cy="12" r="10" /><path d="M2 12h20" /><path d="M12 2a15 15 0 0 1 0 20" /><path d="M12 2a15 15 0 0 0 0 20" /></svg>
  if (name === 'group') return <svg {...common}><path d="M20 12V7a2 2 0 0 0-2-2h-6.2a2 2 0 0 1-1.4-.6L9.6 3.6A2 2 0 0 0 8.2 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-3" /><path d="M16 11h6" /><path d="M19 8v6" /></svg>
  if (name === 'delete') return <svg {...common}><path d="M3 6h18" /><path d="M8 6V4h8v2" /><path d="M19 6l-1 14H6L5 6" /></svg>
  return <svg {...common}><rect x="9" y="9" width="13" height="13" rx="2" /><rect x="2" y="2" width="13" height="13" rx="2" /></svg>
}

export function publicDetailButton(label: string, icon: React.ReactNode, onClick: () => void, className = '', disabled?: boolean) {
  return (
    <button type="button" className={`public-detail-icon ${className}`} title={label} aria-label={label} disabled={disabled} onClick={onClick}>
      {icon}
    </button>
  )
}

type ImageDetailAction = {
  key: string
  label: string
  icon: React.ReactNode
  onClick: () => void
  tone?: string
  disabled?: boolean
}

export function PublicImageDetail({ image, imageUrl, referenceImages = [], showPublicStats = true, onPreviewImage, onLike, onFavorite, onDownload, onCopyPrompt, actions = [] }: {
  image: ImageResult | GalleryImage
  imageUrl?: string
  referenceImages?: Array<{ id: string; url: string; alt: string; onPreview?: () => void }>
  showPublicStats?: boolean
  onPreviewImage?: (url: string, alt: string) => void
  onLike?: (image: ImageResult | GalleryImage) => void
  onFavorite?: (image: ImageResult | GalleryImage) => void
  onDownload?: (image: ImageResult | GalleryImage) => void
  onCopyPrompt: (prompt: string) => void
  actions?: ImageDetailAction[]
}) {
  const authorName = image.author_name || '匿名用户'
  const prompt = image.prompt || '-'
  return (
    <div className="public-detail">
      <div className="public-detail-media">
        {referenceImages.length ? (
          <div className="public-detail-references">
            <span>引用图片</span>
            {referenceImages.map((item) => (
              <button key={item.id || item.url} type="button" onClick={item.onPreview} disabled={!item.onPreview}>
                <img src={item.url} alt={item.alt} />
              </button>
            ))}
          </div>
        ) : null}
        <div className="public-detail-image">
          {imageUrl ? (
            <button type="button" className="public-detail-image-button" onClick={() => onPreviewImage?.(imageUrl, image.prompt || image.id)} disabled={!onPreviewImage}>
              <img src={imageUrl} alt={image.prompt || image.id} />
            </button>
          ) : <div className="public-detail-placeholder">图片不可预览</div>}
        </div>
      </div>
      <div className="public-detail-side">
        <div className="public-detail-prompt">
          <div>
            <small>Prompt</small>
            <p>{prompt}</p>
          </div>
          {publicDetailButton('复制 Prompt', <PublicDetailIcon name="copy" />, () => onCopyPrompt(prompt), '', !image.prompt)}
        </div>
        <div className="public-detail-meta">
          <span>作者 <b>{authorName}</b></span>
          <span>模型 <b>{image.route_model_code || image.abstract_model || '-'}</b></span>
          <span>清晰度 <b>{image.quality || '-'}</b></span>
          <span>比例 <b>{image.aspect_ratio || '-'}</b></span>
        </div>
        {showPublicStats ? (
          <div className="public-detail-stats">
            {publicEngagementStats(image).map((item) => (
              <span key={item.key}>{item.label} <b>{item.value}</b></span>
            ))}
          </div>
        ) : null}
        <div className="public-detail-actions">
          {onLike ? publicDetailButton(`点赞 ${image.like_count ?? 0}`, <PublicDetailIcon name="heart" active={image.liked_by_viewer} />, () => onLike(image), image.liked_by_viewer ? 'liked' : '') : null}
          {onFavorite ? publicDetailButton(`收藏 ${image.favorite_count ?? 0}`, <PublicDetailIcon name="star" active={image.favorited_by_viewer} />, () => onFavorite(image), image.favorited_by_viewer ? 'favorited' : '') : null}
          {onDownload ? publicDetailButton('下载图片', <PublicDetailIcon name="download" />, () => onDownload(image), '', !image.url && !image.download_url) : null}
          {actions.map((action) => (
            <React.Fragment key={action.key}>
              {publicDetailButton(action.label, action.icon, action.onClick, action.tone ?? '', action.disabled)}
            </React.Fragment>
          ))}
        </div>
      </div>
    </div>
  )
}

export const protectedRoutes: RouteId[] = ['home', 'genpic', 'gallery', 'checkout', 'api-keys', 'profile', 'docs']

function HomeIcon() {
  return <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M3 9l9-7 9 7v11a2 2 0 01-2 2H5a2 2 0 01-2-2z" /></svg>
}
function SparklesIcon() {
  return <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M12 2v20M2 12h20M5 5l14 14M5 19L19 5" /></svg>
}
function GridIcon() {
  return <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M9 3v18M15 3v18M3 9h18M3 15h18" /></svg>
}
function UserIcon() {
  return <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M20 21v-2a4 4 0 00-4-4H8a4 4 0 00-4 4v2" /><circle cx="12" cy="7" r="4" /></svg>
}
function KeyIcon() {
  return <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="7.5" cy="15.5" r="5.5" /><path d="M12 12l8-8M17 7l3 3M14 10l3 3" /></svg>
}
function LogoutIcon() {
  return <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M9 21H5a2 2 0 01-2-2V5a2 2 0 012-2h4" /><path d="M16 17l5-5-5-5M21 12H9" /></svg>
}
function DotIcon() {
  return <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="4" /></svg>
}

function avatarMenuIcon(icon: AvatarMenuIcon) {
  if (icon === 'profile') return <UserIcon />
  if (icon === 'key') return <KeyIcon />
  return <DotIcon />
}

export const navItems: Array<{ route: RouteId; short: string; label: string; icon: React.ReactNode }> = [
  { route: 'home', short: 'HOME', label: '首页', icon: <HomeIcon /> },
  { route: 'genpic', short: 'GEN', label: '生图', icon: <SparklesIcon /> },
  { route: 'gallery', short: 'IMG', label: '图库', icon: <GridIcon /> },
  { route: 'public-gallery', short: 'PUB', label: '公开广场', icon: <GridIcon /> },
  { route: 'checkout', short: 'PAY', label: '充值', icon: <DotIcon /> },
]

export function Shell({ children }: { children: React.ReactNode }) {
  const app = useApp()
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement | null>(null)
  const statusChips = topbarStatusChips(app.balance)
  const accountMenuItems = avatarMenuItems()

  useEffect(() => {
    if (!menuOpen) return undefined
    const close = (event: MouseEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setMenuOpen(false)
    }
    window.addEventListener('mousedown', close)
    return () => window.removeEventListener('mousedown', close)
  }, [menuOpen])

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
        <nav className="vault-nav vault-nav-bottom" aria-label="User navigation">
          <a
            href="#/profile"
            className={app.route === 'profile' ? 'active' : ''}
            onClick={(event) => {
              event.preventDefault()
              app.navigate('profile')
            }}
          >
            <UserIcon />
            <small>ME</small>
            <span>我的</span>
          </a>
        </nav>
      </aside>
      <main className="vault-main">
        <header className="vault-topbar glass-panel">
          <div className="quick-links" aria-label="Quick entries">
            <button type="button" onClick={() => app.navigate('genpic')}>灵感模板</button>
            <button type="button" onClick={() => app.navigate('public-gallery')}>公开广场</button>
            <button type="button" onClick={() => app.navigate('docs')}>开发文档</button>
          </div>
          <div className="topbar-tools">
            {statusChips.map((chip) => (
              <span key={chip.label} className="top-chip" title={chip.detail}>
                {chip.label} <b>{chip.value}</b>
              </span>
            ))}
            <button className="balance-chip" type="button" onClick={() => app.navigate('checkout')}>
              <span>◈</span>
              <b>{app.balance?.available_points ?? '...'}</b>
            </button>
            <div className="avatar-menu-wrap" ref={menuRef}>
              <button
                className="avatar-chip"
                type="button"
                aria-haspopup="menu"
                aria-expanded={menuOpen}
                onClick={() => setMenuOpen((open) => !open)}
              >
                <span>{app.profile?.avatar_initials ?? 'PG'}</span>
                <b>{app.profile?.display_name ?? 'Guest'}</b>
              </button>
              {menuOpen ? (
                <div className="avatar-menu glass-panel" role="menu">
                  {accountMenuItems.map((item) => (
                    <button
                      key={item.key}
                      type="button"
                      role="menuitem"
                      data-permission={item.permission}
                      onClick={() => {
                        setMenuOpen(false)
                        app.navigate(item.route)
                      }}
                    >
                      {avatarMenuIcon(item.icon)}
                      {item.label}
                    </button>
                  ))}
                  <hr />
                  <button type="button" role="menuitem" className="danger" onClick={() => { setMenuOpen(false); void app.logout() }}><LogoutIcon />退出登录</button>
                </div>
              ) : null}
            </div>
          </div>
        </header>
        <div className="route-surface">{children}</div>
      </main>
    </div>
  )
}

export function ToastViewport({ toasts, onExpire }: { toasts: Toast[]; onExpire: (id: number) => void }) {
  return (
    <div className="toast-stack" aria-live="polite" aria-atomic="true">
      {toasts.map((toast) => (
        <ToastItem key={toast.id} toast={toast} onExpire={onExpire} />
      ))}
    </div>
  )
}

function ToastItem({ toast, onExpire }: { toast: Toast; onExpire: (id: number) => void }) {
  useEffect(() => {
    const timer = window.setTimeout(() => onExpire(toast.id), toast.durationMs ?? 4200)
    return () => window.clearTimeout(timer)
  }, [onExpire, toast.durationMs, toast.id])

  return (
    <div className={`toast toast-${toast.tone}`} style={{ '--toast-duration': `${toast.durationMs ?? 4200}ms` } as React.CSSProperties}>
      <span>{toast.tone === 'success' ? '✓' : toast.tone === 'error' ? '!' : 'i'}</span>
      <p>{toast.message}</p>
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
  if (!date) return '-'
  const parsed = new Date(date)
  if (Number.isNaN(parsed.getTime())) return date
  const year = parsed.getUTCFullYear()
  const month = String(parsed.getUTCMonth() + 1).padStart(2, '0')
  const day = String(parsed.getUTCDate()).padStart(2, '0')
  const hour = String(parsed.getUTCHours()).padStart(2, '0')
  const minute = String(parsed.getUTCMinutes()).padStart(2, '0')
  return `${year}/${month}/${day} ${hour}:${minute}`
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
