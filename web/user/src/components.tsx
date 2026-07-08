import React, { createContext, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { GalleryImage, ImageResult, ImageTaskStatus, ImageTaskType, PublishStatus } from '../../shared/api-types'
import { cn } from '../../shared/classnames'
import { avatarMenuItems, type AvatarMenuIcon } from './avatarMenu'
import { BrandMark, siteBrand } from './brand'
import { publicEngagementStats } from './publicEngagementModel'
import type { AppContextValue, RouteId, Toast } from './types'
import { userShell, userButton, userForm, userState, userPill, userCard, userText } from './ui/classes'
import { rdShell } from './ui/redesign-classes'
import { Home, Sparkles, LayoutGrid, User, KeyRound, CreditCard, Settings, FileText, Sun, Moon, LogOut, ChevronDown, Eye, Heart, Star, Download, Copy, Edit, Globe, FolderPlus, Trash2 } from './ui/icons'
import { shellLayoutClasses, type ShellScrollMode } from './shellLayout'
export { userShell, userButton, userForm, userState, userPill, userCard, userText }

export const AppContext = createContext<AppContextValue | null>(null)

export function useApp() {
  const value = useContext(AppContext)
  if (!value) throw new Error('useApp must be used within AppContext.Provider')
  return value
}

export type ImageLightboxPayload = {
  url: string
  downloadUrl?: string
  alt: string
  prompt?: string
  width?: number
  height?: number
  ratio?: string
  model?: string
  source?: string
}

export function imagePixelsLabel(width?: number, height?: number) {
  return width && height ? `${width} x ${height}` : '未知'
}

export function imageRatioLabel(width?: number, height?: number, fallback?: string) {
  if (fallback) return fallback
  if (!width || !height) return '未知'
  const divisor = gcd(width, height)
  return `${width / divisor}:${height / divisor}`
}

function gcd(a: number, b: number): number {
  return b ? gcd(b, a % b) : Math.abs(a)
}

const lightboxClasses = {
  backdrop: 'fixed inset-0 z-[100] flex cursor-zoom-out items-start justify-center bg-[var(--lightbox-backdrop)] p-4 pt-10 backdrop-blur-xl animate-in fade-in duration-300 sm:p-10',
  close: 'absolute right-4 top-4 z-10 grid size-8 cursor-pointer place-items-center rounded-full border border-[var(--lightbox-close-border)] bg-[var(--lightbox-close-bg)] text-sm leading-none text-[var(--lightbox-close-text)] shadow-lg transition hover:scale-105',
  stage: 'relative flex max-h-[92vh] w-full max-w-6xl cursor-default flex-col overflow-hidden rounded-3xl border border-[var(--border)] bg-[var(--bg)] shadow-2xl md:flex-row',
  imageWrap: 'flex flex-1 items-start justify-center overflow-auto bg-[var(--lightbox-stage-bg)] p-6 pt-8',
  imageButton: 'max-h-[80vh] w-auto max-w-full cursor-zoom-in border-0 bg-transparent p-0',
  image: 'max-h-[80vh] w-auto max-w-full rounded-xl object-contain shadow-xl',
  panel: 'flex max-h-[92vh] w-full flex-col justify-between gap-6 overflow-y-auto border-t border-[var(--border)] bg-[var(--surface)] p-8 md:w-96 md:border-l md:border-t-0',
  title: 'text-[10px] font-bold uppercase tracking-wider text-[var(--accent)]',
  meta: 'grid grid-cols-2 gap-2.5',
  metaItem: 'rounded-2xl border border-[var(--border)] bg-[var(--bg)]/70 p-3',
  metaLabel: 'mb-1 block text-[10px] font-vault-mono uppercase tracking-widest text-[var(--muted)]',
  metaValue: 'text-sm font-black text-[var(--fg)]',
  prompt: 'max-h-44 overflow-y-auto rounded-2xl border border-[var(--border)] bg-[var(--bg)] p-4 text-sm leading-relaxed text-[var(--fg)] [overflow-wrap:anywhere]',
  actions: 'flex flex-col gap-3',
  primaryAction: 'w-full rounded-xl bg-[var(--accent)] py-3 text-sm font-bold text-white transition-transform hover:scale-[1.02]',
  secondaryAction: 'w-full rounded-xl border border-[var(--border)] bg-[var(--bg)] py-3 text-sm font-bold text-[var(--fg)] transition-colors hover:border-[var(--accent)]',
  zoomBackdrop: 'fixed inset-0 z-[110] bg-[var(--lightbox-backdrop)]/95 backdrop-blur-xl animate-in fade-in duration-200',
  zoomClose: 'absolute right-4 top-4 z-[111] grid size-10 place-items-center rounded-full border border-[var(--lightbox-close-border)] bg-[var(--lightbox-close-bg)] text-base text-[var(--lightbox-close-text)] shadow-lg transition hover:scale-105',
  zoomViewport: 'absolute inset-0 overflow-hidden cursor-grab active:cursor-grabbing',
  zoomStage: 'absolute left-1/2 top-1/2 will-change-transform',
  zoomImage: 'block max-w-none select-none shadow-2xl',
  zoomToolbar: 'absolute left-1/2 top-4 z-[111] flex -translate-x-1/2 items-center gap-2 rounded-full border border-[var(--border)] bg-[var(--surface)]/92 px-3 py-2 text-sm text-[var(--fg)] shadow-xl backdrop-blur',
  zoomToolButton: 'grid size-8 place-items-center rounded-full border border-[var(--border)] bg-[var(--bg)] text-[var(--fg)] transition hover:border-[var(--accent)] hover:text-[var(--accent)]',
  zoomToolValue: 'min-w-12 text-center font-vault-mono text-xs',
}

export function ImageLightbox({ image, onClose }: {
  image: ImageLightboxPayload | null
  onClose: () => void
}) {
  const [zoomOpen, setZoomOpen] = useState(false)
  useEffect(() => {
    if (!image) return undefined
    const close = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        if (zoomOpen) setZoomOpen(false)
        else onClose()
      }
    }
    window.addEventListener('keydown', close)
    return () => window.removeEventListener('keydown', close)
  }, [image, onClose, zoomOpen])

  useEffect(() => {
    if (!image) setZoomOpen(false)
  }, [image])

  if (!image) return null
  const pixels = imagePixelsLabel(image.width, image.height)
  const ratio = imageRatioLabel(image.width, image.height, image.ratio)
  const copyConfig = async () => {
    await copyText(JSON.stringify({
      prompt: image.prompt || image.alt || '',
      model: image.model || '',
      ratio,
      pixels,
      source: image.source || '',
    }, null, 2))
  }
  const downloadImage = () => {
    const link = document.createElement('a')
    link.href = image.downloadUrl || image.url
    link.download = `${image.alt || 'mikiko-image'}.png`
    link.rel = 'noopener noreferrer'
    document.body.appendChild(link)
    link.click()
    link.remove()
  }
  return (
    <div className={lightboxClasses.backdrop} role="dialog" aria-modal="true" aria-label="图片预览" onMouseDown={onClose}>
      <section className={lightboxClasses.stage} onMouseDown={(event) => event.stopPropagation()}>
        <button type="button" className={lightboxClasses.close} aria-label="关闭预览" onClick={onClose}>✕</button>
        <div className={lightboxClasses.imageWrap}>
          <button type="button" className={lightboxClasses.imageButton} onClick={() => setZoomOpen(true)} aria-label="放大查看图片">
            <img className={lightboxClasses.image} src={image.url} alt={image.alt} />
          </button>
        </div>
        <aside className={lightboxClasses.panel}>
          <div>
            <div className="flex flex-col gap-5">
              <span className={lightboxClasses.title}>画卷配置详情</span>
              <div className={lightboxClasses.meta}>
                <LightboxInfo label="像素" value={pixels} />
                <LightboxInfo label="比例" value={ratio} />
                <LightboxInfo label="模型" value={image.model || '未知'} />
                <LightboxInfo label="来源" value={image.source || 'Mikiko Studio'} />
              </div>
              <div>
                <span className="mb-2 block text-xs text-[var(--muted)]">提示词</span>
                <p className={lightboxClasses.prompt}>"{image.prompt || image.alt || '暂无提示词'}"</p>
              </div>
            </div>
          </div>
          <div className={lightboxClasses.actions}>
            <button type="button" className={lightboxClasses.primaryAction} onClick={() => void copyConfig()}>复制配置</button>
            <button type="button" className={lightboxClasses.secondaryAction} onClick={downloadImage}>下载图片</button>
          </div>
        </aside>
      </section>
      {zoomOpen ? <ImageZoomViewer image={image} onClose={() => setZoomOpen(false)} /> : null}
    </div>
  )
}

function ImageZoomViewer({ image, onClose }: { image: ImageLightboxPayload; onClose: () => void }) {
  const [scale, setScale] = useState(1)
  const [position, setPosition] = useState({ x: 0, y: 0 })
  const dragRef = useRef<{ x: number; y: number; active: boolean }>({ x: 0, y: 0, active: false })

  const scaleLabel = useMemo(() => `${Math.round(scale * 100)}%`, [scale])

  useEffect(() => {
    setScale(1)
    setPosition({ x: 0, y: 0 })
  }, [image.url])

  function clampScale(next: number) {
    return Math.min(4, Math.max(0.6, Number(next.toFixed(2))))
  }

  function handleWheel(event: React.WheelEvent<HTMLDivElement>) {
    event.preventDefault()
    const delta = event.deltaY > 0 ? -0.1 : 0.1
    setScale((current) => clampScale(current + delta))
  }

  function handlePointerDown(event: React.PointerEvent<HTMLDivElement>) {
    dragRef.current = { x: event.clientX - position.x, y: event.clientY - position.y, active: true }
    event.currentTarget.setPointerCapture(event.pointerId)
  }

  function handlePointerMove(event: React.PointerEvent<HTMLDivElement>) {
    if (!dragRef.current.active) return
    setPosition({ x: event.clientX - dragRef.current.x, y: event.clientY - dragRef.current.y })
  }

  function handlePointerUp(event: React.PointerEvent<HTMLDivElement>) {
    dragRef.current.active = false
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
  }

  return (
    <div className={lightboxClasses.zoomBackdrop} role="dialog" aria-modal="true" aria-label="图片缩放预览" onMouseDown={onClose}>
      <button type="button" className={lightboxClasses.zoomClose} aria-label="关闭放大预览" onClick={onClose}>✕</button>
      <div className={lightboxClasses.zoomToolbar} onMouseDown={(event) => event.stopPropagation()}>
        <button type="button" className={lightboxClasses.zoomToolButton} aria-label="缩小" onClick={() => setScale((current) => clampScale(current - 0.1))}>-</button>
        <span className={lightboxClasses.zoomToolValue}>{scaleLabel}</span>
        <button type="button" className={lightboxClasses.zoomToolButton} aria-label="放大" onClick={() => setScale((current) => clampScale(current + 0.1))}>+</button>
        <button type="button" className={lightboxClasses.zoomToolButton} aria-label="重置缩放" onClick={() => { setScale(1); setPosition({ x: 0, y: 0 }) }}>↺</button>
      </div>
      <div
        className={lightboxClasses.zoomViewport}
        onMouseDown={(event) => event.stopPropagation()}
        onWheel={handleWheel}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerCancel={handlePointerUp}
      >
        <div className={lightboxClasses.zoomStage} style={{ transform: `translate(calc(-50% + ${position.x}px), calc(-50% + ${position.y}px)) scale(${scale})` }}>
          <img className={lightboxClasses.zoomImage} src={image.url} alt={image.alt} draggable={false} />
        </div>
      </div>
    </div>
  )
}

function LightboxInfo({ label, value }: { label: string; value: string }) {
  return (
    <div className={lightboxClasses.metaItem}>
      <span className={lightboxClasses.metaLabel}>{label}</span>
      <span className={lightboxClasses.metaValue}>{value}</span>
    </div>
  )
}

export function PublicDetailIcon({ name, active }: { name: 'eye' | 'heart' | 'star' | 'download' | 'copy' | 'edit' | 'public' | 'group' | 'delete'; active?: boolean }) {
  const props = { size: 18, strokeWidth: 1.5, fill: active && (name === 'heart' || name === 'star') ? 'currentColor' : 'none' }
  if (name === 'eye') return <Eye {...props} />
  if (name === 'heart') return <Heart {...props} />
  if (name === 'star') return <Star {...props} />
  if (name === 'download') return <Download {...props} />
  if (name === 'edit') return <Edit {...props} />
  if (name === 'public') return <Globe {...props} />
  if (name === 'group') return <FolderPlus {...props} />
  if (name === 'delete') return <Trash2 {...props} />
  return <Copy {...props} />
}

const publicDetailClasses = {
  root: 'grid grid-cols-[minmax(0,1.15fr)_minmax(300px,.85fr)] items-start gap-6 max-[760px]:grid-cols-1',
  media: 'grid min-w-0 gap-3',
  references: 'flex items-center gap-2.5 overflow-x-auto rounded-xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] p-2.5',
  referenceLabel: 'shrink-0 text-xs text-[var(--muted)]',
  referenceButton: 'size-[72px] shrink-0 cursor-zoom-in overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--bg)] p-0 disabled:cursor-default disabled:opacity-80',
  referenceImage: 'block size-full object-cover',
  imageFrame: 'grid min-h-80 place-items-center overflow-hidden rounded-2xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--bg)_82%,black_10%)]',
  imageButton: 'min-h-80 size-full cursor-zoom-in border-0 bg-transparent p-0 disabled:cursor-default',
  image: 'block size-full max-h-[66vh] object-contain',
  placeholder: 'grid min-h-80 place-items-center text-[var(--muted)]',
  side: 'grid min-w-0 gap-[18px]',
  prompt: 'grid grid-cols-[minmax(0,1fr)_auto] items-start gap-3 border-b border-[var(--border)] pb-[18px]',
  promptLabel: 'text-[11px] uppercase tracking-[.08em] text-[var(--muted)]',
  promptText: 'm-0 mt-2 leading-[1.7] text-[var(--muted)] [overflow-wrap:anywhere]',
  meta: 'grid grid-cols-2 gap-2.5 rounded-xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] p-3.5 max-[760px]:grid-cols-1',
  stats: 'grid grid-cols-3 gap-2.5 max-[760px]:grid-cols-1',
  metaItem: 'grid min-w-0 gap-1 text-xs text-[var(--muted)]',
  metaValue: 'overflow-hidden text-ellipsis whitespace-nowrap text-sm text-[var(--fg)]',
  statItem: 'grid min-w-0 gap-1 rounded-xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] p-3 text-center text-xs text-[var(--muted)]',
  statValue: 'overflow-hidden text-ellipsis whitespace-nowrap font-vault-mono text-lg text-[var(--fg)]',
  actions: 'flex justify-end gap-2.5 pt-2',
  iconButton: cn(userButton.icon, 'pg-public-detail-action size-10 min-h-10 cursor-pointer rounded-xl p-0 hover:border-[var(--accent)] hover:bg-[color-mix(in_oklch,var(--accent)_12%,transparent)] hover:text-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-45'),
  iconDanger: 'hover:border-[var(--accent-coral)] hover:bg-[color-mix(in_oklch,var(--accent-coral)_12%,transparent)] hover:text-[var(--accent-coral)]',
  iconLiked: 'border-[color-mix(in_oklch,var(--accent-coral)_72%,var(--border))] bg-[color-mix(in_oklch,var(--accent-coral)_18%,transparent)] text-[var(--accent-coral)] shadow-[0_0_18px_color-mix(in_oklch,var(--accent-coral)_18%,transparent)]',
  iconFavorited: 'border-[color-mix(in_oklch,var(--accent)_72%,var(--border))] bg-[color-mix(in_oklch,var(--accent)_18%,transparent)] text-[var(--accent)] shadow-[0_0_18px_color-mix(in_oklch,var(--accent)_18%,transparent)]',
}

export function publicDetailButton(label: string, icon: React.ReactNode, onClick: () => void, className = '', disabled?: boolean) {
  const toneClass = className === 'liked'
    ? publicDetailClasses.iconLiked
    : className === 'favorited'
      ? publicDetailClasses.iconFavorited
      : className === 'danger'
        ? publicDetailClasses.iconDanger
        : className
  return (
    <button type="button" className={cn(publicDetailClasses.iconButton, toneClass)} title={label} aria-label={label} disabled={disabled} onClick={onClick}>
      {icon}
    </button>
  )
}

export type ImageDetailAction = {
  key: string
  label: string
  icon: React.ReactNode
  onClick: () => void
  tone?: string
  disabled?: boolean
}

export function ImageDetailModal({ title, image, imageUrl, referenceImages = [], showPublicStats = true, onPreviewImage, onLike, onFavorite, onDownload, onCopyPrompt, actions = [], previewSourceLabel = '历史资产', onClose }: {
  title: string
  image: ImageResult | GalleryImage | null
  imageUrl?: string
  referenceImages?: Array<{ id: string; url: string; alt: string; onPreview?: () => void }>
  showPublicStats?: boolean
  onPreviewImage?: (payload: ImageLightboxPayload) => void
  onLike?: (image: ImageResult | GalleryImage) => void
  onFavorite?: (image: ImageResult | GalleryImage) => void
  onDownload?: (image: ImageResult | GalleryImage) => void
  onCopyPrompt: (prompt: string) => void
  actions?: ImageDetailAction[]
  previewSourceLabel?: string
  onClose: () => void
}) {
  if (!image) return null
  return (
    <Modal title={title} onClose={onClose}>
      <PublicImageDetail
        image={image}
        imageUrl={imageUrl}
        referenceImages={referenceImages}
        showPublicStats={showPublicStats}
        onPreviewImage={onPreviewImage}
        onLike={onLike}
        onFavorite={onFavorite}
        onDownload={onDownload}
        onCopyPrompt={onCopyPrompt}
        actions={actions}
        previewSourceLabel={previewSourceLabel}
      />
    </Modal>
  )
}

export function PublicImageDetail({ image, imageUrl, referenceImages = [], showPublicStats = true, onPreviewImage, onLike, onFavorite, onDownload, onCopyPrompt, actions = [], previewSourceLabel = '历史资产' }: {
  image: ImageResult | GalleryImage
  imageUrl?: string
  referenceImages?: Array<{ id: string; url: string; alt: string; onPreview?: () => void }>
  showPublicStats?: boolean
  onPreviewImage?: (payload: ImageLightboxPayload) => void
  onLike?: (image: ImageResult | GalleryImage) => void
  onFavorite?: (image: ImageResult | GalleryImage) => void
  onDownload?: (image: ImageResult | GalleryImage) => void
  onCopyPrompt: (prompt: string) => void
  actions?: ImageDetailAction[]
  previewSourceLabel?: string
}) {
  const authorName = image.author_name || '匿名用户'
  const prompt = image.prompt || '-'
  return (
    <div className={publicDetailClasses.root}>
      <div className={publicDetailClasses.media}>
        {referenceImages.length ? (
          <div className={publicDetailClasses.references}>
            <span className={publicDetailClasses.referenceLabel}>引用图片</span>
            {referenceImages.map((item) => (
              <button key={item.id || item.url} type="button" className={publicDetailClasses.referenceButton} onClick={item.onPreview} disabled={!item.onPreview}>
                <img src={item.url} alt={item.alt} className={publicDetailClasses.referenceImage} />
              </button>
            ))}
          </div>
        ) : null}
        <div className={publicDetailClasses.imageFrame}>
          {imageUrl ? (
            <button
              type="button"
              className={publicDetailClasses.imageButton}
              onClick={() => onPreviewImage?.({
                url: imageUrl,
                downloadUrl: imageUrl,
                alt: image.prompt || image.id,
                prompt: image.prompt,
                width: image.width,
                height: image.height,
                ratio: image.aspect_ratio,
                model: image.route_model_code || image.abstract_model,
                source: previewSourceLabel,
              })}
              disabled={!onPreviewImage}
            >
              <img src={imageUrl} alt={image.prompt || image.id} className={publicDetailClasses.image} />
            </button>
          ) : <div className={publicDetailClasses.placeholder}>图片不可预览</div>}
        </div>
      </div>
      <div className={publicDetailClasses.side}>
        <div className={publicDetailClasses.prompt}>
          <div>
            <small className={publicDetailClasses.promptLabel}>Prompt</small>
            <p className={publicDetailClasses.promptText}>{prompt}</p>
          </div>
          {publicDetailButton('复制 Prompt', <PublicDetailIcon name="copy" />, () => onCopyPrompt(prompt), '', !image.prompt)}
        </div>
        <div className={publicDetailClasses.meta}>
          <span className={publicDetailClasses.metaItem}>作者 <b className={publicDetailClasses.metaValue}>{authorName}</b></span>
          <span className={publicDetailClasses.metaItem}>模型 <b className={publicDetailClasses.metaValue}>{image.route_model_code || image.abstract_model || '-'}</b></span>
          <span className={publicDetailClasses.metaItem}>清晰度 <b className={publicDetailClasses.metaValue}>{image.quality || '-'}</b></span>
          <span className={publicDetailClasses.metaItem}>比例 <b className={publicDetailClasses.metaValue}>{image.aspect_ratio || '-'}</b></span>
        </div>
        {showPublicStats ? (
          <div className={publicDetailClasses.stats}>
            {publicEngagementStats(image).map((item) => (
              <span key={item.key} className={publicDetailClasses.statItem}>{item.label} <b className={publicDetailClasses.statValue}>{item.value}</b></span>
            ))}
          </div>
        ) : null}
        <div className={publicDetailClasses.actions}>
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

export const protectedRoutes: RouteId[] = ['home', 'genpic', 'gallery', 'checkout', 'api-keys', 'profile', 'docs', 'settings']

function HomeIcon() { return <Home size={22} strokeWidth={1.5} /> }
function SparklesIcon() { return <Sparkles size={22} strokeWidth={1.5} /> }
function GridIcon() { return <LayoutGrid size={22} strokeWidth={1.5} /> }
function UserIcon() { return <User size={22} strokeWidth={1.5} /> }
function KeyIcon() { return <KeyRound size={18} strokeWidth={1.5} /> }
function CreditCardIcon() { return <CreditCard size={22} strokeWidth={1.5} /> }
function SettingsIcon() { return <Settings size={22} strokeWidth={1.5} /> }
function DocsIcon() { return <FileText size={18} strokeWidth={1.5} /> }
function SunIcon() { return <Sun size={20} strokeWidth={1.5} /> }
function MoonIcon() { return <Moon size={20} strokeWidth={1.5} /> }
function LogoutIcon() { return <LogOut size={18} strokeWidth={1.5} /> }
function ChevronIcon({ className = '' }: { className?: string }) { return <ChevronDown size={16} strokeWidth={1.5} className={className} /> }

function avatarMenuIcon(icon: AvatarMenuIcon) {
  if (icon === 'profile') return <User size={18} strokeWidth={1.5} />
  if (icon === 'billing') return <CreditCard size={18} strokeWidth={1.5} />
  if (icon === 'key') return <KeyRound size={18} strokeWidth={1.5} />
  if (icon === 'docs') return <FileText size={18} strokeWidth={1.5} />
  return <User size={18} strokeWidth={1.5} />
}

export const navItems: Array<{ route: RouteId; label: string; icon: React.ReactNode }> = [
  { route: 'home', label: '首页', icon: <HomeIcon /> },
  { route: 'genpic', label: '创作', icon: <SparklesIcon /> },
  { route: 'gallery', label: '资产', icon: <GridIcon /> },
  { route: 'checkout', label: '积分', icon: <CreditCardIcon /> },
  { route: 'api-keys', label: '密钥', icon: <KeyIcon /> },
  { route: 'settings', label: '设置', icon: <SettingsIcon /> },
]

export function Shell({ children, scrollMode = 'app' }: { children: React.ReactNode; scrollMode?: ShellScrollMode }) {
  const app = useApp()
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement | null>(null)
  const accountMenuItems = avatarMenuItems()
  const isDark = app.themePreference.mode === 'dark'
  const layout = shellLayoutClasses(scrollMode)

  useEffect(() => {
    if (!menuOpen) return undefined
    const close = (event: MouseEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setMenuOpen(false)
    }
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMenuOpen(false)
    }
    window.addEventListener('mousedown', close)
    window.addEventListener('keydown', closeOnEscape)
    return () => {
      window.removeEventListener('mousedown', close)
      window.removeEventListener('keydown', closeOnEscape)
    }
  }, [menuOpen])

  return (
    <div className={cn(rdShell.shellWrapper, 'redesign-demo-scope')}>
      <div className={layout.shell}>
      <aside className={rdShell.sidebar} aria-label={`${siteBrand.name} 用户导航`}>
        <button className={rdShell.brand} type="button" onClick={() => app.navigate('home')} aria-label={`${siteBrand.name} 首页`}>
          <BrandMark />
        </button>
        <nav className={rdShell.nav}>
          {navItems.map((item) => (
            <a
              key={item.route}
              href={`#/${item.route}`}
              className={cn(rdShell.navLink, app.route === item.route && rdShell.navLinkActive)}
              onClick={(event) => {
                event.preventDefault()
                app.navigate(item.route)
              }}
            >
              <span className={cn(rdShell.navLinkIndicator, app.route === item.route && rdShell.navLinkIndicatorActive)} />
              {item.icon}
              <span className={rdShell.navLabel}>{item.label}</span>
            </a>
          ))}
        </nav>
      </aside>
      <main className={layout.main}>
        <header className={rdShell.topbar}>
          <div className={rdShell.topbarInner}>
          <div className={rdShell.userTools}>
            <button
              type="button"
              className="grid size-11 place-items-center rounded-2xl border border-[var(--border)] bg-[var(--surface)] text-[var(--fg)] transition-all hover:border-[var(--accent)] hover:text-[var(--accent)]"
              aria-label={isDark ? '切换浅色主题' : '切换深色主题'}
              title={isDark ? '切换浅色主题' : '切换深色主题'}
              onClick={() => void app.setThemePreference({ mode: isDark ? 'light' : 'dark' })}
            >
              {isDark ? <MoonIcon /> : <SunIcon />}
            </button>
            <div className={rdShell.balancePill}>
              <span className={rdShell.balanceText}>◈</span>
              <b className={rdShell.balanceValue}>{app.balance?.available_points ?? '...'}</b>
              <button type="button" className={rdShell.rechargeBtn} onClick={() => app.navigate('checkout')} aria-label="充值积分">+</button>
            </div>
            <div className="relative" ref={menuRef}>
              <button
                className={rdShell.avatarBtn}
                type="button"
                aria-haspopup="menu"
                aria-expanded={menuOpen}
                onClick={() => setMenuOpen((open) => !open)}
              >
                <span className={rdShell.avatarImg}>{app.profile?.avatar_initials ?? 'PG'}</span>
                <b className={rdShell.userName}>{app.profile?.display_name ?? 'Guest'}</b>
                <ChevronIcon className={cn(rdShell.userChevron, menuOpen && 'rotate-180')} />
              </button>
              {menuOpen ? (
                <div className="absolute right-0 top-[calc(100%+12px)] z-50 w-56 rounded-2xl border border-[var(--border)] bg-[var(--surface)]/95 p-2 shadow-2xl shadow-black/30 backdrop-blur-2xl" role="menu">
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
                      className="flex min-h-10 w-full cursor-pointer items-center gap-3 rounded-xl border-0 bg-transparent px-3 py-2 text-left text-sm font-bold text-[var(--fg)] transition-colors hover:bg-[var(--accent)]/10 hover:text-[var(--accent)]"
                    >
                      {avatarMenuIcon(item.icon)}
                      {item.label}
                    </button>
                  ))}
                  <hr className="my-2 h-px border-0 bg-[var(--border)]" />
                  <button type="button" role="menuitem" className="flex min-h-10 w-full cursor-pointer items-center gap-3 rounded-xl border-0 bg-transparent px-3 py-2 text-left text-sm font-bold text-[var(--accent-coral)] transition-colors hover:bg-[color-mix(in_oklch,var(--accent-coral)_14%,transparent)]" onClick={() => { setMenuOpen(false); void app.logout() }}><LogoutIcon />退出登录</button>
                </div>
              ) : null}
            </div>
          </div>
          </div>
        </header>
        <div className={rdShell.contentConstrain}>
          <div className="flex min-h-0 flex-1 flex-col">{children}</div>
          <footer className={rdShell.footer}>
            <div className={rdShell.footerContent}>
              <div className="flex flex-col items-center gap-2 md:flex-row">
                <span>© 2026 {siteBrand.name}. All rights reserved.</span>
                <span className="hidden text-[var(--border)] md:inline">|</span>
                <span>京ICP备20261024号-1</span>
              </div>
              <div className={rdShell.footerLinks}>
                <span className={rdShell.footerLink}>服务协议</span>
                <span className={rdShell.footerLink}>隐私条款</span>
                <a className={rdShell.footerLink} href="#/docs">API 文档</a>
              </div>
            </div>
          </footer>
        </div>
        <nav className={rdShell.mobileNav} aria-label={`${siteBrand.name} 移动导航`}>
          {navItems.slice(0, 5).map((item) => (
            <button key={item.route} type="button" className={cn(rdShell.mobileNavLink, app.route === item.route && rdShell.mobileNavLinkActive)} onClick={() => app.navigate(item.route)}>
              {item.icon}
              <span className="text-[10px] font-bold">{item.label}</span>
            </button>
          ))}
        </nav>
      </main>
      </div>
    </div>
  )
}

export function ToastViewport({ toasts, onExpire }: { toasts: Toast[]; onExpire: (id: number) => void }) {
  return (
    <div className={userState.toastStack} aria-live="polite" aria-atomic="true">
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
    <div className={userState.toast} style={{ '--toast-duration': `${toast.durationMs ?? 4200}ms`, '--toast-ring': toast.tone === 'success' ? 'var(--accent-emerald)' : toast.tone === 'error' ? 'var(--accent-coral)' : 'var(--accent)' } as React.CSSProperties}>
      <span className="grid size-7 place-items-center rounded-full bg-[color-mix(in_oklch,var(--toast-ring)_18%,transparent)] font-black text-[var(--toast-ring)]">{toast.tone === 'success' ? '✓' : toast.tone === 'error' ? '!' : 'i'}</span>
      <p>{toast.message}</p>
    </div>
  )
}

export function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <label className={userForm.field}>
      <span className={userForm.fieldLabel}>{label}</span>
      {children}
      {hint ? <em>{hint}</em> : null}
    </label>
  )
}

export function Button({ children, tone = 'primary', busy = false, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement> & { tone?: 'primary' | 'ghost' | 'danger'; busy?: boolean }) {
  const toneClass = tone === 'primary' ? userButton.primary : tone === 'danger' ? userButton.danger : userButton.ghost
  return (
    <button {...props} className={cn(userButton.base, toneClass, props.className)} disabled={props.disabled || busy}>
      {busy ? <span className={userState.spinner} aria-hidden="true" /> : null}
      {children}
    </button>
  )
}

export function Surface({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return <section className={cn(userCard.base, className)}>{children}</section>
}

export function PageIntro({ eyebrow, title, detail, action }: { eyebrow?: string; title: string; detail?: string; action?: React.ReactNode }) {
  return (
    <div className="mb-6 flex flex-wrap items-end justify-between gap-5">
      <div>
        {eyebrow ? <p className={userText.eyebrow}>{eyebrow}</p> : null}
        <h1 className="m-0 font-vault-display text-[clamp(2.4rem,5vw,4.5rem)] font-medium leading-none">{title}</h1>
        {detail ? <span className="mt-3 block max-w-[68ch] text-[var(--muted)]">{detail}</span> : null}
      </div>
      {action ? <div className="flex flex-wrap items-center gap-3">{action}</div> : null}
    </div>
  )
}

export function LoadingState({ label = '正在读取实时数据...' }: { label?: string }) {
  return <div className={userState.stateLine}><span className={userState.spinner} />{label}</div>
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className={cn(userState.stateLine, 'error-state')}>
      <b>读取失败</b>
      <span>{message}</span>
      {onRetry ? <Button tone="ghost" onClick={onRetry}>重试</Button> : null}
    </div>
  )
}

export function EmptyState({ title, detail, action, icon }: { title: string; detail: string; action?: React.ReactNode; icon?: React.ReactNode }) {
  return (
    <div className={userState.empty}>
      {icon ? <div className={userState.emptyIcon}>{icon}</div> : null}
      <strong className={userState.emptyTitle}>{title}</strong>
      <span className={userState.emptyDetail}>{detail}</span>
      {action}
    </div>
  )
}

export function Modal({ title, children, onClose }: { title: string; children: React.ReactNode; onClose: () => void }) {
  return (
    <div className={userState.modalBackdrop} role="presentation" onMouseDown={onClose}>
      <section className={userState.modalCard} role="dialog" aria-modal="true" aria-label={title} onMouseDown={(event) => event.stopPropagation()}>
        <div className="flex items-center justify-between gap-5">
          <h2 className="m-0 text-xl">{title}</h2>
          <button
            type="button"
            className="grid size-[38px] place-items-center rounded-full border border-[var(--border)] bg-[var(--bg)] text-base leading-none text-[var(--fg)] shadow-lg transition hover:scale-105"
            onClick={onClose}
            aria-label="关闭"
          >
            ×
          </button>
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
  const pillTone = tone as keyof typeof userPill
  return <span className={cn(userPill.base, userPill[pillTone] ?? userPill.neutral)}>{label[status] ?? status}</span>
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
