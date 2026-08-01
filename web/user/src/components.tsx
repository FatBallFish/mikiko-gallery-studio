import React, { createContext, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { GalleryImage, ImageResult, ImageTaskStatus, ImageTaskType, PublishStatus } from '../../shared/api-types'
import { cn } from '../../shared/classnames'
import { avatarMenuItems, type AvatarMenuIcon } from './avatarMenu'
import { BrandMark, siteBrand } from './brand'
import { openDocsEntry } from './docsUrl'
import { publicEngagementStats } from './publicEngagementModel'
import type { AppContextValue, RouteId, Toast } from './types'
import { userShell, userButton, userForm, userState, userPill, userCard, userText } from './ui/classes'
import { focusableElements, focusTrapTargetIndex } from './ui/focusTrap'
import { overlayLayers, rdShell } from './ui/redesign-classes'
import { Home, Sparkles, LayoutGrid, User, KeyRound, CreditCard, Settings, FileText, Sun, Moon, LogOut, ChevronDown, Eye, Heart, Star, Download, Copy, Edit, Globe, FolderPlus, Trash2, X } from './ui/icons'
import { OverlayPortal } from './ui/overlayPortal'
import { imageMediaTransition, initialImageMediaState } from './ui/imageMediaModel'
import { shouldStartZoomDrag } from './ui/zoomPointer'
import { resetShellScroll, shellActiveNavIndex, shellChromeClasses, shellLayoutClasses, type ShellScrollMode } from './shellLayout'
import { workspaceCreationDraftFromSnapshot, type WorkspaceCreationDraft } from './pages/workspaceCreationDraft'
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
  creationDraft?: WorkspaceCreationDraft
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
  backdrop: `fixed inset-0 ${overlayLayers.lightbox} flex cursor-zoom-out items-start justify-center bg-[var(--lightbox-backdrop)] p-4 pt-10 backdrop-blur-xl animate-in fade-in duration-300 motion-reduce:animate-none sm:p-10`,
  close: 'absolute right-4 top-4 z-10 grid size-8 cursor-pointer place-items-center rounded-full border border-[var(--lightbox-close-border)] bg-[var(--lightbox-close-bg)] text-sm leading-none text-[var(--lightbox-close-text)] shadow-lg transition hover:scale-105',
  stage: 'relative flex max-h-[92vh] w-full max-w-6xl cursor-default flex-col overflow-hidden rounded-3xl border border-[var(--border)] bg-[var(--bg)] shadow-2xl md:flex-row',
  imageWrap: 'flex flex-1 items-start justify-center overflow-auto bg-[var(--lightbox-stage-bg)] p-6 pt-8',
  imageFrame: 'relative grid min-h-80 w-full place-items-center',
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
  zoomBackdrop: `fixed inset-0 ${overlayLayers.zoom} bg-[var(--lightbox-backdrop)]/95 backdrop-blur-xl animate-in fade-in duration-200 motion-reduce:animate-none`,
  zoomClose: `absolute right-4 top-4 ${overlayLayers.zoomControls} grid size-10 place-items-center rounded-full border border-[var(--lightbox-close-border)] bg-[var(--lightbox-close-bg)] text-base text-[var(--lightbox-close-text)] shadow-lg transition hover:scale-105`,
  zoomViewport: 'absolute inset-0 overflow-hidden cursor-grab active:cursor-grabbing',
  zoomStage: 'absolute left-1/2 top-1/2 will-change-transform',
  zoomImage: 'block max-w-none select-none shadow-2xl',
  zoomToolbar: `absolute left-1/2 top-4 ${overlayLayers.zoomControls} flex -translate-x-1/2 items-center gap-2 rounded-full border border-[var(--border)] bg-[var(--surface)]/92 px-3 py-2 text-sm text-[var(--fg)] shadow-xl backdrop-blur`,
  zoomToolButton: 'grid size-8 place-items-center rounded-full border border-[var(--border)] bg-[var(--bg)] text-[var(--fg)] transition hover:border-[var(--accent)] hover:text-[var(--accent)]',
  zoomToolValue: 'min-w-12 text-center font-vault-mono text-xs',
  mediaLoading: 'absolute inset-0 grid place-items-center text-sm text-[var(--muted)]',
  mediaFallback: 'grid min-h-64 w-full max-w-md place-items-center content-center gap-3 rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-8 text-center text-[var(--muted)]',
  mediaFallbackTitle: 'm-0 text-base font-bold text-[var(--fg)]',
  mediaRetry: 'min-h-10 rounded-xl border border-[var(--border)] bg-[var(--bg)] px-4 text-sm font-bold text-[var(--fg)] transition-colors hover:border-[var(--accent)] hover:text-[var(--accent)] motion-reduce:transition-none',
  zoomFallback: 'absolute inset-0 grid place-items-center p-6',
}

function useImageMediaState(url: string) {
  const [state, setState] = useState(() => initialImageMediaState(url))
  useEffect(() => setState((current) => imageMediaTransition(current, { type: 'reset', url })), [url])
  return {
    ...state,
    imageKey: `${state.url}:${state.attempt}`,
    markLoaded: () => setState((current) => imageMediaTransition(current, { type: 'loaded', url })),
    markError: () => setState((current) => imageMediaTransition(current, { type: 'error', url })),
    retry: () => setState((current) => imageMediaTransition(current, { type: 'retry' })),
  }
}

export function ImageMediaFallback({ onRetry }: { onRetry: () => void }) {
  return (
    <div className={lightboxClasses.mediaFallback} role="alert">
      <p className={lightboxClasses.mediaFallbackTitle}>图片暂时无法显示</p>
      <span>资源可能已失效，或当前网络无法完成加载。</span>
      <button type="button" className={lightboxClasses.mediaRetry} onClick={onRetry}>重试加载</button>
    </div>
  )
}

export function ImageLightbox({ image, onClose, onReuseConfiguration }: {
  image: ImageLightboxPayload | null
  onClose: () => void
  onReuseConfiguration?: (draft: WorkspaceCreationDraft) => void
}) {
  const [zoomOpen, setZoomOpen] = useState(false)
  const media = useImageMediaState(image?.url ?? '')
  const dialogRef = useRef<HTMLElement | null>(null)
  useDismissableLayer(Boolean(image), onClose, dialogRef)

  useEffect(() => {
    if (!image) setZoomOpen(false)
  }, [image])

  if (!image) return null
  const pixels = imagePixelsLabel(image.width, image.height)
  const ratio = imageRatioLabel(image.width, image.height, image.ratio)
  const reuseConfiguration = () => {
    if (image.creationDraft) onReuseConfiguration?.(image.creationDraft)
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
    <OverlayPortal>
      <div className={lightboxClasses.backdrop} data-focus-layer role="presentation" onMouseDown={onClose}>
        <section ref={dialogRef} tabIndex={-1} className={lightboxClasses.stage} role="dialog" aria-modal="true" aria-label="图片预览" onMouseDown={(event) => event.stopPropagation()}>
          <button type="button" className={lightboxClasses.close} aria-label="关闭预览" onClick={onClose}><X size={16} strokeWidth={1.7} aria-hidden="true" /></button>
          <div className={lightboxClasses.imageWrap}>
            <div className={lightboxClasses.imageFrame}>
              {media.status === 'loading' ? <span className={lightboxClasses.mediaLoading} role="status">正在加载图片</span> : null}
              {media.status === 'error' ? <ImageMediaFallback onRetry={media.retry} /> : (
                <button type="button" className={lightboxClasses.imageButton} disabled={media.status !== 'loaded'} onClick={() => setZoomOpen(true)} aria-label="放大查看图片">
                  <img key={media.imageKey} className={cn(lightboxClasses.image, media.status !== 'loaded' && 'opacity-0')} src={image.url} alt={image.alt} onLoad={media.markLoaded} onError={media.markError} />
                </button>
              )}
            </div>
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
              {image.creationDraft && onReuseConfiguration ? <button type="button" className={lightboxClasses.primaryAction} onClick={reuseConfiguration}>复用配置</button> : null}
              <button type="button" className={lightboxClasses.secondaryAction} onClick={downloadImage}>下载图片</button>
            </div>
          </aside>
        </section>
        {zoomOpen ? <ImageZoomViewer image={image} onClose={() => setZoomOpen(false)} /> : null}
      </div>
    </OverlayPortal>
  )
}

function ImageZoomViewer({ image, onClose }: { image: ImageLightboxPayload; onClose: () => void }) {
  const [scale, setScale] = useState(1)
  const [position, setPosition] = useState({ x: 0, y: 0 })
  const dragRef = useRef<{ x: number; y: number; active: boolean }>({ x: 0, y: 0, active: false })
  const dialogRef = useRef<HTMLDivElement | null>(null)
  const media = useImageMediaState(image.url)
  useDismissableLayer(true, onClose, dialogRef)

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
    if (!shouldStartZoomDrag({ button: event.button, target: event.target as Element })) return
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
    <OverlayPortal>
      <div
      ref={dialogRef}
      tabIndex={-1}
      className={lightboxClasses.zoomBackdrop}
      data-focus-layer
      role="dialog"
      aria-modal="true"
      aria-label="图片缩放预览"
      onMouseDown={(event) => {
        event.stopPropagation()
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <button type="button" className={lightboxClasses.zoomClose} aria-label="关闭放大预览" onClick={onClose}><X size={18} strokeWidth={1.7} aria-hidden="true" /></button>
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
        {media.status === 'loading' ? <span className={lightboxClasses.mediaLoading} role="status">正在加载大图</span> : null}
        {media.status === 'error' ? <div className={lightboxClasses.zoomFallback}><ImageMediaFallback onRetry={media.retry} /></div> : (
          <div className={lightboxClasses.zoomStage} style={{ transform: `translate(calc(-50% + ${position.x}px), calc(-50% + ${position.y}px)) scale(${scale})` }}>
            <img key={media.imageKey} className={cn(lightboxClasses.zoomImage, media.status !== 'loaded' && 'opacity-0')} src={image.url} alt={image.alt} draggable={false} onLoad={media.markLoaded} onError={media.markError} />
          </div>
        )}
      </div>
      </div>
    </OverlayPortal>
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
            <DetailImageMedia
              src={imageUrl}
              alt={image.prompt || image.id}
              onOpen={onPreviewImage ? () => onPreviewImage({
                url: imageUrl,
                downloadUrl: imageUrl,
                alt: image.prompt || image.id,
                prompt: image.prompt,
                width: image.width,
                height: image.height,
                ratio: image.aspect_ratio,
                model: image.route_model_code || image.abstract_model,
                source: previewSourceLabel,
                creationDraft: workspaceCreationDraftFromSnapshot(image),
              }) : undefined}
            />
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
          <span className={publicDetailClasses.metaItem}>基础分辨率 <b className={publicDetailClasses.metaValue}>{image.base_resolution || '-'}</b></span>
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

function DetailImageMedia({ src, alt, onOpen }: { src: string; alt: string; onOpen?: () => void }) {
  const [failed, setFailed] = useState(false)
  useEffect(() => setFailed(false), [src])
  if (failed) return <div className={publicDetailClasses.placeholder} role="status">图片暂时无法预览</div>
  return (
    <button type="button" className={publicDetailClasses.imageButton} onClick={onOpen} disabled={!onOpen}>
      <img src={src} alt={alt} className={publicDetailClasses.image} loading="lazy" decoding="async" onError={() => setFailed(true)} />
    </button>
  )
}

export const protectedRoutes: RouteId[] = ['home', 'genpic', 'gallery', 'checkout', 'api-keys', 'profile', 'settings']

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
  const mainScrollRef = useRef<HTMLElement | null>(null)
  const accountMenuItems = avatarMenuItems()
  const isDark = app.themePreference.mode === 'dark'
  const layout = shellLayoutClasses(scrollMode)
  const activeNavIndex = shellActiveNavIndex(app.route, navItems)

  useLayoutEffect(() => {
    resetShellScroll(scrollMode, mainScrollRef.current, window)
  }, [app.route, scrollMode])

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
    <div className={cn(shellChromeClasses.wrapper, 'redesign-demo-scope')}>
      <div className={layout.shell}>
      <aside className={shellChromeClasses.sidebar} aria-label={`${siteBrand.name} 用户导航`}>
        <button className={shellChromeClasses.brand} type="button" onClick={() => app.navigate('home')} aria-label={`${siteBrand.name} 首页`}>
          <BrandMark />
        </button>
        <nav className={shellChromeClasses.nav}>
          {activeNavIndex >= 0 ? <span className={shellChromeClasses.navIndicator} style={{ transform: `translateY(${activeNavIndex * 64}px)` }} aria-hidden="true" /> : null}
          {navItems.map((item) => (
            <a
              key={item.route}
              href={`#/${item.route}`}
              className={cn(shellChromeClasses.navLink, app.route === item.route && shellChromeClasses.navLinkActive)}
              aria-current={app.route === item.route ? 'page' : undefined}
              onClick={(event) => {
                event.preventDefault()
                app.navigate(item.route)
              }}
            >
              {item.icon}
              <span className={shellChromeClasses.navLabel}>{item.label}</span>
            </a>
          ))}
        </nav>
      </aside>
      <main ref={mainScrollRef} className={layout.main}>
        <header className={shellChromeClasses.topbar}>
          <div className={shellChromeClasses.topbarInner}>
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
                        if (item.external) openDocsEntry('account-menu')
                        else app.navigate(item.route)
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
        <div className={shellChromeClasses.contentConstrain}>
          <div key={app.route} className={cn('flex min-h-0 flex-1 flex-col', layout.content)}>{children}</div>
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
                <button className={cn(rdShell.footerLink, 'border-0 bg-transparent p-0 text-inherit')} type="button" onClick={() => openDocsEntry('footer')}>API 文档</button>
              </div>
            </div>
          </footer>
        </div>
        <nav className={shellChromeClasses.mobileNav} aria-label={`${siteBrand.name} 移动导航`}>
          {navItems.map((item) => (
            <button key={item.route} type="button" aria-current={app.route === item.route ? 'page' : undefined} className={cn(shellChromeClasses.mobileNavLink, app.route === item.route && shellChromeClasses.mobileNavLinkActive)} onClick={() => app.navigate(item.route)}>
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

export const componentPrimitiveNames = [
  'Button',
  'IconButton',
  'SegmentedControl',
  'Field',
  'Toolbar',
  'StatusRail',
  'Surface',
  'EmptyState',
  'Modal',
  'Drawer',
  'ImageFrame',
  'LocalFeedback',
] as const

export function Field({ label, hint, error, children }: { label: string; hint?: string; error?: string; children: React.ReactNode }) {
  return (
    <label className={userForm.field}>
      <span className={userForm.fieldLabel}>{label}</span>
      {children}
      {error ? <span className="text-xs not-italic text-[var(--accent-coral)]" role="alert">{error}</span> : hint ? <span className="text-xs text-[var(--dim)]">{hint}</span> : null}
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

export function IconButton({ label, tone = 'default', size = 'md', children, ...props }: Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'aria-label'> & {
  label: string
  tone?: 'default' | 'danger'
  size?: 'sm' | 'md'
}) {
  return (
    <button
      {...props}
      type={props.type ?? 'button'}
      aria-label={label}
      title={props.title ?? label}
      className={cn(
        size === 'sm' ? userButton.iconSm : userButton.icon,
        tone === 'danger' && 'text-[var(--accent-coral)] hover:border-[var(--accent-coral)] hover:text-[var(--accent-coral)]',
        props.className,
      )}
    >
      {children}
    </button>
  )
}

export type SegmentedOption<T extends string> = {
  value: T
  label: string
  icon?: React.ReactNode
  disabled?: boolean
}

export function SegmentedControl<T extends string>({ label, value, options, onChange, disabled = false, className = '' }: {
  label: string
  value: T
  options: Array<SegmentedOption<T>>
  onChange: (value: T) => void
  disabled?: boolean
  className?: string
}) {
  return (
    <div className={cn('inline-grid min-w-0 grid-flow-col auto-cols-fr rounded-xl border border-[var(--border)] bg-[var(--bg)] p-1', className)} role="group" aria-label={label}>
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          className={cn(
            'inline-flex min-h-9 min-w-0 items-center justify-center gap-1.5 rounded-lg border-0 bg-transparent px-3 text-sm font-semibold text-[var(--muted)] transition-colors duration-[var(--motion-fast)] hover:text-[var(--fg)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-[var(--focus-ring)]',
            value === option.value && 'bg-[var(--surface)] text-[var(--fg)] shadow-[var(--pg-shadow-sm)]',
          )}
          aria-pressed={value === option.value}
          disabled={disabled || option.disabled}
          onClick={() => onChange(option.value)}
        >
          {option.icon}
          <span className="truncate">{option.label}</span>
        </button>
      ))}
    </div>
  )
}

export function Toolbar({ label = '页面工具栏', children, className = '' }: { label?: string; children: React.ReactNode; className?: string }) {
  return <div className={cn('flex min-h-14 flex-wrap items-center justify-between gap-3 border-y border-[var(--border)] bg-[color-mix(in_oklch,var(--surface)_64%,transparent)] px-4 py-2', className)} role="toolbar" aria-label={label}>{children}</div>
}

export function GalleryFilterToolbar({ label, query, queryPlaceholder, onQueryChange, filters, meta, action, className = '' }: {
  label: string
  query: string
  queryPlaceholder: string
  onQueryChange: (value: string) => void
  filters?: React.ReactNode
  meta?: React.ReactNode
  action?: React.ReactNode
  className?: string
}) {
  return (
    <Toolbar label={label} className={cn('gap-4 px-0 py-3', className)}>
      <label className="relative min-w-[min(100%,17rem)] flex-1 max-[620px]:basis-full max-[620px]:max-w-none sm:max-w-[22rem]">
        <span className="sr-only">{queryPlaceholder}</span>
        <input
          type="search"
          className={cn(userForm.input, 'h-11 w-full rounded-xl pl-4')}
          value={query}
          placeholder={queryPlaceholder}
          onChange={(event) => onQueryChange(event.target.value)}
        />
      </label>
      {filters ? <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2 max-[620px]:basis-full">{filters}</div> : null}
      {meta ? <div className="ml-auto shrink-0 text-xs text-[var(--muted)]" aria-live="polite">{meta}</div> : null}
      {action ? <div className="shrink-0">{action}</div> : null}
    </Toolbar>
  )
}

export type StatusRailItem = {
  key: string
  label: string
  value: React.ReactNode
  tone?: 'neutral' | 'success' | 'warning' | 'error'
}

export function StatusRail({ items, label = '当前状态', className = '' }: { items: StatusRailItem[]; label?: string; className?: string }) {
  const toneClass = {
    neutral: 'bg-[var(--dim)]',
    success: 'bg-[var(--accent-emerald)]',
    warning: 'bg-[var(--accent)]',
    error: 'bg-[var(--accent-coral)]',
  }
  return (
    <dl className={cn('flex min-w-0 flex-wrap items-center gap-x-6 gap-y-2 border-b border-[var(--border)] px-4 py-3', className)} aria-label={label}>
      {items.map((item) => (
        <div key={item.key} className="flex min-w-0 items-center gap-2 text-xs">
          <span className={cn('size-1.5 shrink-0 rounded-full', toneClass[item.tone ?? 'neutral'])} aria-hidden="true" />
          <dt className="text-[var(--muted)]">{item.label}</dt>
          <dd className="m-0 truncate font-semibold text-[var(--fg)]">{item.value}</dd>
        </div>
      ))}
    </dl>
  )
}

export function Surface({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return <section className={cn(userCard.base, className)}>{children}</section>
}

export const imageFrameActionsClass = 'pg-image-frame-actions'

export function ImageFrame({ src, alt, actions, children, className = '', imageClassName = '', onClick }: {
  src?: string
  alt: string
  actions?: React.ReactNode
  children?: React.ReactNode
  className?: string
  imageClassName?: string
  onClick?: () => void
}) {
  const media = src ? <img className={cn('size-full object-cover transition-transform duration-700 ease-out group-hover:scale-105 motion-reduce:transition-none motion-reduce:transform-none', imageClassName)} src={src} alt={alt} /> : children
  return (
    <figure className={cn('group relative m-0 min-h-40 overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--canvas-bg)]', className)}>
      {onClick ? <button type="button" className="block size-full border-0 bg-transparent p-0 text-left" onClick={onClick} aria-label={`查看${alt}`}>{media}</button> : media}
      {actions ? <div className={cn(imageFrameActionsClass, 'absolute inset-x-3 bottom-3 flex translate-y-2 justify-end gap-2 opacity-0 transition-all duration-[var(--motion-fast)] group-hover:translate-y-0 group-hover:opacity-100 group-focus-within:translate-y-0 group-focus-within:opacity-100 motion-reduce:transition-none')}>{actions}</div> : null}
    </figure>
  )
}

export function GalleryImageFrame({ src, alt, width, height, aspectRatio = '4 / 3', actions, topAction, onOpen, selected = false, className = '', imageClassName = '' }: {
  src?: string
  alt: string
  width?: number
  height?: number
  aspectRatio?: string
  actions?: React.ReactNode
  topAction?: React.ReactNode
  onOpen?: () => void
  selected?: boolean
  className?: string
  imageClassName?: string
}) {
  const [imageState, setImageState] = useState<'loading' | 'ready' | 'error'>(src ? 'loading' : 'error')
  const [retryKey, setRetryKey] = useState(0)
  const imageRef = useRef<HTMLImageElement>(null)

  useLayoutEffect(() => {
    const image = imageRef.current
    if (!src) {
      setImageState('error')
    } else if (image && image.complete && image.naturalWidth > 0) {
      setImageState('ready')
    } else {
      setImageState('loading')
    }
  }, [src, retryKey])

  const media = src ? (
    <img
      ref={imageRef}
      key={`${src}:${retryKey}`}
      src={src}
      alt={alt}
      width={width}
      height={height}
      loading="lazy"
      decoding="async"
      className={cn('size-full object-cover transition duration-700 ease-out group-hover:scale-[1.025] motion-reduce:transition-none motion-reduce:transform-none', imageState !== 'ready' && 'opacity-0', imageClassName)}
      onLoad={() => setImageState('ready')}
      onError={() => setImageState('error')}
    />
  ) : null

  return (
    <figure
      className={cn(
        'group relative m-0 isolate overflow-hidden rounded-2xl border bg-[var(--canvas-bg)] transition-colors duration-[var(--motion-fast)] focus-within:border-[var(--focus-ring)] motion-reduce:transition-none',
        selected ? 'border-[var(--accent)] ring-1 ring-[var(--accent)]' : 'border-[var(--border)]',
        className,
      )}
      style={{ aspectRatio }}
    >
      {imageState === 'loading' ? <div className="pg-skeleton absolute inset-0" role="status" aria-label={`正在加载${alt}`} /> : null}
      {onOpen && imageState !== 'error' ? (
        <button type="button" className="absolute inset-0 z-[1] block size-full cursor-zoom-in border-0 bg-transparent p-0 text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[-3px] focus-visible:outline-[var(--focus-ring)]" onClick={onOpen} aria-label={`查看${alt}`}>
          {media}
        </button>
      ) : media}
      {imageState === 'error' ? (
        <div className="absolute inset-0 z-[2] grid place-items-center gap-2 bg-[var(--canvas-bg)] p-5 text-center text-sm text-[var(--muted)]" role="status">
          <span>图片暂时无法显示</span>
          {src ? <button type="button" className="min-h-10 rounded-xl border border-[var(--border)] px-3 text-[var(--fg)] hover:border-[var(--accent)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-[var(--focus-ring)]" onClick={() => { setImageState('loading'); setRetryKey((value) => value + 1) }}>重试</button> : null}
        </div>
      ) : null}
      {topAction ? <div className="absolute left-3 top-3 z-[4]">{topAction}</div> : null}
      {actions ? <div className={cn(imageFrameActionsClass, 'absolute inset-x-3 bottom-3 z-[4] flex translate-y-2 flex-wrap justify-end gap-2 opacity-0 transition-all duration-[var(--motion-fast)] group-hover:translate-y-0 group-hover:opacity-100 group-focus-within:translate-y-0 group-focus-within:opacity-100 motion-reduce:transition-none')}>{actions}</div> : null}
    </figure>
  )
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

function useDismissableLayer(open: boolean, onClose: () => void, focusRef: React.RefObject<HTMLElement | null>) {
  const onCloseRef = useRef(onClose)
  useLayoutEffect(() => {
    onCloseRef.current = onClose
  }, [onClose])

  useLayoutEffect(() => {
    if (!open) return undefined
    const dialog = focusRef.current
    if (!dialog) return undefined
    const layer = dialog.closest<HTMLElement>('[data-focus-layer]')
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const previousOverflow = document.body.style.overflow
    const background = layer?.parentElement
      ? Array.from(layer.parentElement.children).filter((element): element is HTMLElement => element instanceof HTMLElement && element !== layer)
      : []
    const backgroundState = background.map((element) => ({
      element,
      inert: element.inert,
      ariaHidden: element.getAttribute('aria-hidden'),
    }))
    document.body.style.overflow = 'hidden'
    background.forEach((element) => {
      element.inert = true
      element.setAttribute('aria-hidden', 'true')
    })
    const autoFocus = dialog.querySelector<HTMLElement>('[autofocus]')
    ;(autoFocus ?? focusableElements(dialog)[0] ?? dialog).focus()

    const handleKeyDown = (event: KeyboardEvent) => {
      const focusLayers = Array.from(document.querySelectorAll<HTMLElement>('[data-focus-layer]'))
      if (layer && focusLayers[focusLayers.length - 1] !== layer) return
      if (event.key === 'Escape') {
        event.preventDefault()
        event.stopPropagation()
        onCloseRef.current()
        return
      }
      if (event.key !== 'Tab') return
      const focusable = focusableElements(dialog)
      const currentIndex = focusable.indexOf(document.activeElement as HTMLElement)
      const targetIndex = focusTrapTargetIndex(currentIndex, focusable.length, event.shiftKey)
      if (targetIndex === null) {
        if (!focusable.length) {
          event.preventDefault()
          dialog.focus()
        }
        return
      }
      event.preventDefault()
      focusable[targetIndex]?.focus()
    }
    window.addEventListener('keydown', handleKeyDown, true)
    return () => {
      window.removeEventListener('keydown', handleKeyDown, true)
      document.body.style.overflow = previousOverflow
      backgroundState.forEach(({ element, inert, ariaHidden }) => {
        element.inert = inert
        if (ariaHidden === null) element.removeAttribute('aria-hidden')
        else element.setAttribute('aria-hidden', ariaHidden)
      })
      if (previousFocus?.isConnected) previousFocus.focus()
    }
  }, [focusRef, open])
}

export function Modal({ title, children, onClose, className = '' }: { title: string; children: React.ReactNode; onClose: () => void; className?: string }) {
  const dialogRef = useRef<HTMLElement | null>(null)
  useDismissableLayer(true, onClose, dialogRef)
  return (
    <OverlayPortal>
      <div className={userState.modalBackdrop} role="presentation" data-focus-layer onMouseDown={onClose}>
        <section ref={dialogRef} tabIndex={-1} className={cn(userState.modalCard, className)} role="dialog" aria-modal="true" aria-label={title} onMouseDown={(event) => event.stopPropagation()}>
          <div className="flex items-center justify-between gap-5">
            <h2 className="m-0 text-xl">{title}</h2>
            <IconButton label="关闭" onClick={onClose}><X size={18} strokeWidth={1.5} aria-hidden="true" /></IconButton>
          </div>
          {children}
        </section>
      </div>
    </OverlayPortal>
  )
}

export function Drawer({ open, title, children, onClose, side = 'right', className = '' }: {
  open: boolean
  title: string
  children: React.ReactNode
  onClose: () => void
  side?: 'left' | 'right' | 'bottom'
  className?: string
}) {
  const drawerRef = useRef<HTMLElement | null>(null)
  useDismissableLayer(open, onClose, drawerRef)
  if (!open) return null
  const position = side === 'left'
    ? 'inset-y-0 left-0 w-[min(420px,92vw)] border-r'
    : side === 'bottom'
      ? 'inset-x-0 bottom-0 max-h-[88vh] rounded-t-2xl border-t'
      : 'inset-y-0 right-0 w-[min(420px,92vw)] border-l'
  return (
    <OverlayPortal>
      <div className="fixed inset-0 z-[80] bg-black/60 backdrop-blur-sm" role="presentation" data-focus-layer onMouseDown={onClose}>
        <section
          ref={drawerRef}
          tabIndex={-1}
          role="dialog"
          aria-modal="true"
          aria-label={title}
          className={cn('absolute flex max-h-full flex-col border-[var(--border)] bg-[var(--surface)] shadow-[var(--pg-shadow-lg)]', position, className)}
          onMouseDown={(event) => event.stopPropagation()}
        >
          <header className="flex min-h-16 items-center justify-between gap-4 border-b border-[var(--border)] px-5">
            <h2 className="m-0 text-lg font-semibold">{title}</h2>
            <IconButton label="关闭" onClick={onClose}><X size={18} strokeWidth={1.5} aria-hidden="true" /></IconButton>
          </header>
          <div className="min-h-0 flex-1 overflow-y-auto p-5">{children}</div>
        </section>
      </div>
    </OverlayPortal>
  )
}

export function LocalFeedback({ tone = 'info', title, detail, action, className = '' }: {
  tone?: 'success' | 'error' | 'info' | 'warning'
  title: string
  detail?: string
  action?: React.ReactNode
  className?: string
}) {
  const toneClass = {
    success: 'bg-[var(--accent-emerald)]',
    error: 'bg-[var(--accent-coral)]',
    info: 'bg-[var(--accent-purple)]',
    warning: 'bg-[var(--accent)]',
  }
  return (
    <div className={cn('grid grid-cols-[auto_minmax(0,1fr)_auto] items-start gap-3 border-y border-[var(--border)] bg-[color-mix(in_oklch,var(--surface)_68%,transparent)] px-4 py-3', className)} role={tone === 'error' ? 'alert' : 'status'}>
      <span className={cn('mt-1.5 size-2 rounded-full', toneClass[tone])} aria-hidden="true" />
      <div className="grid min-w-0 gap-1">
        <strong className="text-sm text-[var(--fg)]">{title}</strong>
        {detail ? <span className="text-xs leading-5 text-[var(--muted)]">{detail}</span> : null}
      </div>
      {action}
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
