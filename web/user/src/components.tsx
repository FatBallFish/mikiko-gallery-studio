import React, { createContext, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Maximize2 as FullscreenIcon, CircleCheck, TriangleAlert, CircleX, Info } from 'lucide-react'
import { useToastLifetime } from './ui/useToastLifetime'
import type { GalleryImage, ImageResult, ImageTaskStatus, ImageTaskType, PublishStatus, MediaAsset, Project } from '../../shared/api-types'
import { cn } from '../../shared/classnames'
import { userApi } from '../../shared/user-api'
import { continueMediaCreation, mediaDisplayName, mediaDownloadFilename } from './features/media/mediaCreationNavigation'
import { MediaGroupSelectionDialog } from './features/media/MediaGroupSelectionDialog'
import { mediaCreationActions } from './features/media/mediaExperience'
import { pointsExchangeRules, pointsText } from '../../shared/pointsDisplay'
import { RefreshableMediaImage, useMediaRefreshOnce, type MediaRefreshHandler } from './ui/mediaRefresh'
import { avatarMenuItems, type AvatarMenuIcon } from './avatarMenu'
import { siteBrand } from './brand'
import { useProjects } from './ProjectContext'
import { openDocsEntry } from './docsUrl'
import { publicEngagementStats } from './publicEngagementModel'
import type { AppContextValue, RouteId, Toast } from './types'
import { userShell, userButton, userForm, userState, userPill, userCard, userText } from './ui/classes'
import { focusableElements, focusTrapTargetIndex } from './ui/focusTrap'
import { overlayLayers, rdShell } from './ui/redesign-classes'
import { Home, Sparkles, LayoutGrid, User, KeyRound, CreditCard, Settings, FileText, Sun, Moon, LogOut, ChevronDown, Eye, Heart, Star, Download, Copy, Edit, Globe, FolderPlus, FolderKanban, Trash2, X, Bell, Film, Plus, Search } from './ui/icons'
import { OverlayPortal } from './ui/overlayPortal'
import { imageMediaTransition, initialImageMediaState } from './ui/imageMediaModel'
import { shouldStartZoomDrag } from './ui/zoomPointer'
import { resetShellScroll, shellActiveNavIndex, shellChromeClasses, shellLayoutClasses, type ShellScrollMode } from './shellLayout'
import { workspaceCreationDraftFromSnapshot, type WorkspaceCreationDraft } from './pages/workspaceCreationDraft'
import type { MediaResource } from './mediaAccess'
export { userShell, userButton, userForm, userState, userPill, userCard, userText }

export const AppContext = createContext<AppContextValue | null>(null)

export function useApp() {
  const value = useContext(AppContext)
  if (!value) throw new Error('useApp must be used within AppContext.Provider')
  return value
}

export type ImagePreviewPayload = {
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
  detailImage?: ImageResult
  mediaResource?: MediaResource
  mediaExpiresAt?: string
  onMediaRefresh?: MediaRefreshHandler
  referenceImages?: Array<{ id: string; url: string; alt: string; mediaExpiresAt?: string; onMediaRefresh?: MediaRefreshHandler }>
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

function ImageZoomViewer({ image, onClose }: { image: ImagePreviewPayload; onClose: () => void }) {
  const [scale, setScale] = useState(1)
  const [position, setPosition] = useState({ x: 0, y: 0 })
  const dragRef = useRef<{ x: number; y: number; active: boolean }>({ x: 0, y: 0, active: false })
  const dialogRef = useRef<HTMLDivElement | null>(null)
  const media = useImageMediaState(image.url)
  const { currentSrc, mediaRetryKey, markMediaLoaded, refreshFailedMedia, resetMediaRefresh } = useMediaRefreshOnce(image.url, image.onMediaRefresh, image.mediaExpiresAt, true)
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
        {media.status === 'error' ? <div className={lightboxClasses.zoomFallback}><ImageMediaFallback onRetry={() => { resetMediaRefresh(); media.retry() }} /></div> : (
          <div className={lightboxClasses.zoomStage} style={{ transform: `translate(calc(-50% + ${position.x}px), calc(-50% + ${position.y}px)) scale(${scale})` }}>
            <img
              key={`${media.imageKey}:${currentSrc}:${mediaRetryKey}`}
              className={cn(lightboxClasses.zoomImage, media.status !== 'loaded' && 'opacity-0')}
              src={currentSrc}
              alt={image.alt}
              draggable={false}
              onLoad={() => { markMediaLoaded(); media.markLoaded() }}
              onError={() => {
                void refreshFailedMedia().then((refreshed) => {
                  if (!refreshed) media.markError()
                })
              }}
            />
          </div>
        )}
      </div>
      </div>
    </OverlayPortal>
  )
}

/** Shared full-screen image viewer for media-library and legacy image details. */
export function ImageZoomPreview({ url, alt, onClose, onMediaRefresh, mediaExpiresAt }: {
  url: string
  alt: string
  onClose: () => void
  onMediaRefresh?: MediaRefreshHandler
  mediaExpiresAt?: string
}) {
  return <ImageZoomViewer image={{ url, alt, onMediaRefresh, mediaExpiresAt }} onClose={onClose} />
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

export function publicDetailButton(label: string, icon: React.ReactNode, onClick: () => void, tone = '', disabled?: boolean) {
  const toneClass = tone === 'liked' || tone === 'favorited' ? 'is-on' : tone === 'danger' ? 'u-action--danger' : ''
  return (
    <button type="button" className={cn('u-action', toneClass)} title={label} aria-label={label} disabled={disabled} onClick={onClick}>
      {icon}
      <span>{label}</span>
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

/**
 * One media-detail surface for images and videos. The drawer keeps a fixed
 * stage height and a 392px information column; "详情" expands the same
 * component to a full-width detail view and returns to the source.
 */
export function ImageDetailModal({ context = 'owned', title, image, imageUrl, referenceImages = [], showPublicStats = true, onLike, onFavorite, onDownload, onCopyPrompt, actions = [], previewSourceLabel = '历史资产', onMediaRefresh, onClose }: {
  title: string
  context?: 'public' | 'owned'
  image: ImageResult | GalleryImage | null
  imageUrl?: string
  referenceImages?: Array<{ id: string; url: string; alt: string; mediaExpiresAt?: string; onMediaRefresh?: () => string | undefined | void | Promise<string | undefined | void> }>
  showPublicStats?: boolean
  onLike?: (image: ImageResult | GalleryImage) => void
  onFavorite?: (image: ImageResult | GalleryImage) => void
  onDownload?: (image: ImageResult | GalleryImage) => void
  onCopyPrompt: (prompt: string) => void
  actions?: ImageDetailAction[]
  previewSourceLabel?: string
  onMediaRefresh?: () => string | undefined | void | Promise<string | undefined | void>
  onClose: () => void
}) {
  const app = useContext(AppContext)
  const [asset, setAsset] = useState<MediaAsset | null>(null)
  const [ownedProjects, setOwnedProjects] = useState<Project[]>([])
  const [sourceImage, setSourceImage] = useState<ImageResult | null>(null)
  const [assetBusy, setAssetBusy] = useState(false)
  const [groupOpen, setGroupOpen] = useState(false)
  const [renameValue, setRenameValue] = useState<string | null>(null)
  useEffect(() => {
    let alive = true
    setAsset(null)
    setSourceImage(null)
    if (context === 'owned' && image && app?.session) {
      void userApi.getMediaAsset(image.id).then(async (next) => {
        if (!alive) return
        setAsset(next)
        if (next.source_task_kind === 'image' && next.source_task_id) {
          const task = await userApi.getTask(next.source_task_id)
          if (alive) setSourceImage({ ...task, ...task.results.find((item) => item.id === image.id), id: image.id, url: image.url || '', width: image.width, height: image.height, publish_status: task.results.find((item) => item.id === image.id)?.publish_status ?? ('publish_status' in image ? image.publish_status : image.visibility_status) })
        }
      }).catch(() => undefined)
      void userApi.listProjects().then((next) => { if (alive) setOwnedProjects(next) }).catch(() => undefined)
    }
    return () => { alive = false }
  }, [image?.id, app?.session?.token, context])
  const mutateAsset = async (patch: { name?: string; group_name?: string; project_id?: string }) => {
    if (context !== 'owned' || !asset) return
    setAssetBusy(true)
    try { setAsset(await userApi.updateMediaAsset(asset, patch)); window.dispatchEvent(new Event('mgs:media-assets-changed')); return true }
    catch (error) { app?.notify('error', error instanceof Error ? error.message : '更新资产失败'); return false }
    finally { setAssetBusy(false) }
  }
  const ownedAsset = context === 'owned' ? asset : null
  const ownedActions: ImageDetailAction[] = ownedAsset && asset ? [
    ...mediaCreationActions(asset).map((action) => ({ key: action.options.taskId ? 'reuse' : action.options.media === 'video' ? 'first-frame' : 'edit-image', label: action.label, icon: <Edit size={14} />, onClick: () => { void continueMediaCreation(action.options).then(onClose).catch((error) => app?.notify('error', error instanceof Error ? error.message : '无法继续创作')) } })),
    { key: 'rename', label: '重命名', icon: <Edit size={14} />, onClick: () => setRenameValue(asset!.name), disabled: assetBusy },
    { key: 'group', label: '设置分组', icon: <FolderPlus size={14} />, onClick: () => setGroupOpen(true), disabled: assetBusy },
    { key: 'delete', label: '删除', icon: <Trash2 size={14} />, tone: 'danger', onClick: () => { if (!window.confirm(`删除“${asset.name}”？`)) return; setAssetBusy(true); void userApi.deleteMediaAsset(asset).then(() => { window.dispatchEvent(new Event('mgs:media-assets-changed')); onClose() }).catch((error) => app?.notify('error', error instanceof Error ? error.message : '删除失败')).finally(() => setAssetBusy(false)) }, disabled: assetBusy },
  ] : []
  const resolvedActions = context === 'public' ? [] : asset ? [...actions.filter((action) => !ownedActions.some((owned) => owned.key === action.key || owned.label === action.label) && !['more', 'delete'].includes(action.key)), ...ownedActions] : actions
  const detailImage = image ? { ...image, ...(context === 'owned' ? sourceImage : null), name: ownedAsset?.name ?? image.name, project: ownedAsset ? ownedProjects.find((project) => project.id === ownedAsset.project_id) ?? image.project : image.project } : null
  const [zoomImage, setZoomImage] = useState<ImagePreviewPayload | null>(null)
  const [expanded, setExpanded] = useState(false)
  const drawerRef = useRef<HTMLElement | null>(null)
  useDismissableLayer(Boolean(image), onClose, drawerRef)
  useEffect(() => setZoomImage(null), [image?.id])
  useEffect(() => { setExpanded(false); setGroupOpen(false); setRenameValue(null) }, [image?.id, context])
  if (!image) return null
  const zoomableReferences = referenceImages.map((item) => ({
    ...item,
    onPreview: () => setZoomImage({
      url: item.url,
      downloadUrl: item.url,
      alt: item.alt,
      source: '原图引用',
      mediaExpiresAt: item.mediaExpiresAt,
      onMediaRefresh: item.onMediaRefresh ?? onMediaRefresh,
    }),
  }))

  return (
    <>
      <OverlayPortal>
        <div className="u-backdrop is-open" role="presentation" data-focus-layer onMouseDown={onClose}>
          <aside
            ref={drawerRef}
            tabIndex={-1}
            role="dialog"
            aria-modal="true"
            aria-label={title}
            className={cn('u-drawer is-open', expanded && 'u-drawer--page')}
            onMouseDown={(event) => event.stopPropagation()}
          >
            <header className="u-drawer__head">
              <div className="u-drawer__title">
                <strong>{expanded ? '媒体详情' : title}</strong>
                <span>{previewSourceLabel}{image.created_at ? ` · ${formatDate(image.created_at || '')}` : ''}</span>
              </div>
              <div className="u-drawer__spacer" />
              {expanded ? (
                <button className="u-btn u-btn--sm" type="button" onClick={() => setExpanded(false)}>返回来源</button>
              ) : (
                <button className="u-btn u-btn--sm" type="button" onClick={() => setExpanded(true)}>详情</button>
              )}
              <button className="u-icon-btn" type="button" aria-label="关闭" onClick={onClose}><X size={18} strokeWidth={1.6} aria-hidden="true" /></button>
            </header>
            <div className="u-drawer__body">
              <PublicImageDetail
                context={context}
                image={detailImage!}
                imageUrl={imageUrl}
                referenceImages={zoomableReferences}
                showPublicStats={showPublicStats}
                onPreviewImage={setZoomImage}
                onLike={onLike}
                onFavorite={onFavorite}
                onDownload={ownedAsset && asset ? () => {
                  void userApi.getMediaAssetAccess(asset.id, 'download').then((access) => {
                    const link = document.createElement('a')
                    link.href = userApi.mediaAssetUrl(access.url, app?.session?.token)
                    link.download = mediaDownloadFilename(asset)
                    link.rel = 'noopener'
                    link.click()
                  }).catch((error) => app?.notify('error', error instanceof Error ? error.message : '下载失败'))
                } : onDownload}
                onCopyPrompt={onCopyPrompt}
                actions={resolvedActions}
                organization={ownedAsset && asset ? <label className="u-field"><span className="u-label">转移项目</span><select className="u-select" value={asset.project_id} disabled={assetBusy} onChange={(event) => void mutateAsset({ project_id: event.target.value })}>{ownedProjects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select></label> : undefined}
                previewSourceLabel={previewSourceLabel}
                onMediaRefresh={onMediaRefresh}
              />
            </div>
          </aside>
        </div>
      </OverlayPortal>
      {groupOpen && ownedAsset ? <MediaGroupSelectionDialog value={ownedAsset.group_name} projectId={ownedAsset.project_id} onClose={() => setGroupOpen(false)} onSelect={async (group_name) => {
        const updated = await userApi.updateMediaAsset(ownedAsset, { group_name })
        setAsset(updated)
        window.dispatchEvent(new Event('mgs:media-assets-changed'))
      }} /> : null}
      {renameValue !== null && ownedAsset ? <Modal title="重命名资产" onClose={() => setRenameValue(null)}><form onSubmit={(event) => { event.preventDefault(); if (renameValue.trim()) void mutateAsset({ name: renameValue.trim() }).then((saved) => { if (saved) setRenameValue(null) }) }}><label className="u-field"><span className="u-label">资产名称</span><input className="u-input" autoFocus value={renameValue} onChange={(event) => setRenameValue(event.target.value)} /></label><button type="submit" className="u-btn u-btn--primary" disabled={assetBusy || !renameValue.trim()}>保存</button></form></Modal> : null}
      {zoomImage ? <ImageZoomViewer image={zoomImage} onClose={() => setZoomImage(null)} /> : null}
    </>
  )
}

export function PublicImageDetail({ context = 'owned', image, imageUrl, referenceImages = [], showPublicStats = true, onPreviewImage, onLike, onFavorite, onDownload, onCopyPrompt, actions = [], organization, previewSourceLabel = '历史资产', onMediaRefresh }: {
  context?: 'public' | 'owned'
  image: ImageResult | GalleryImage
  imageUrl?: string
  referenceImages?: Array<{ id: string; url: string; alt: string; mediaExpiresAt?: string; onPreview?: () => void; onMediaRefresh?: () => string | undefined | void | Promise<string | undefined | void> }>
  showPublicStats?: boolean
  onPreviewImage?: (payload: ImagePreviewPayload) => void
  onLike?: (image: ImageResult | GalleryImage) => void
  onFavorite?: (image: ImageResult | GalleryImage) => void
  onDownload?: (image: ImageResult | GalleryImage) => void
  onCopyPrompt: (prompt: string) => void
  actions?: ImageDetailAction[]
  organization?: React.ReactNode
  previewSourceLabel?: string
  onMediaRefresh?: () => string | undefined | void | Promise<string | undefined | void>
}) {
  const authorName = image.author_name || '匿名用户'
  const prompt = image.prompt || ''
  const actualResolution = image.width > 0 && image.height > 0 ? imagePixelsLabel(image.width, image.height) : '-'
  const taskType = image.task_type === 'image_edit' ? '图片编辑' : image.task_type === 'text_to_image' ? '文生图' : image.task_type || '-'
  const sizeMode = image.size_mode === 'pixel' ? '像素尺寸' : image.size_mode === 'ratio' ? '按比例' : image.size_mode || '-'
  const outputCompression = image.output_compression == null ? '-' : `${image.output_compression}%`
  const outputCount = image.requested_output_image_count ?? image.image_count ?? '-'
  const visibility = image.visibility_status || ('publish_status' in image ? image.publish_status : undefined)
  const statusValue = ('task_status' in image && image.task_status) || visibility || 'succeeded'
  const statusLabel = visibility === 'public' || visibility === 'approved' ? '已公开' : visibility === 'pending_review' || visibility === 'reviewing' ? '审核中' : visibility === 'rejected' ? '已驳回' : visibility === 'private' ? '私有' : '未公开'
  const publicContext = context === 'public'
  const title = mediaDisplayName(image)
  const facts: Array<{ label: string; value: React.ReactNode }> = [
    { label: '实际分辨率', value: actualResolution },
    { label: '基础分辨率', value: image.base_resolution || '-' },
    { label: '比例', value: image.aspect_ratio || '-' },
    { label: '尺寸模式', value: sizeMode },
    { label: '请求尺寸', value: image.requested_size || '-' },
    { label: '质量', value: image.quality || '-' },
    { label: '任务类型', value: taskType },
    { label: '输出格式', value: image.output_format?.toUpperCase() || '-' },
    { label: '压缩质量', value: outputCompression },
    { label: '审核等级', value: image.moderation || '-' },
    ...(!publicContext ? [{ label: '输出数量', value: outputCount }] : []),
    { label: '作者', value: authorName },
    ...(!publicContext ? [{ label: '归属', value: image.project?.name || '默认项目' }] : []),
  ]

  return (
    <div className="u-detail">
      <section className="u-detail__stage-col">
        {referenceImages.length ? (
          <div className="u-detail__refs">
            <span className="u-detail__refs-label">参考图 {referenceImages.length}</span>
            {referenceImages.map((item, index) => (
              <button key={item.id || item.url} className="u-detail__ref" type="button" onClick={item.onPreview} disabled={!item.onPreview} aria-label={`查看参考图 ${index + 1}`}>
                <RefreshableMediaImage src={item.url} mediaExpiresAt={item.mediaExpiresAt} alt={item.alt} onMediaRefresh={item.onMediaRefresh ?? onMediaRefresh} />
              </button>
            ))}
          </div>
        ) : null}
        <div className="u-detail__stage">
          <span className="u-detail__stage-label">
            <StatusPill status={statusValue} />
            <span className="u-chip u-chip--mono">{actualResolution}</span>
          </span>
          {onPreviewImage && imageUrl ? (
            <button className="u-icon-btn u-detail__zoom" type="button" aria-label="全屏查看" onClick={() => onPreviewImage({
              url: imageUrl,
              downloadUrl: imageUrl,
              alt: title,
              prompt: image.prompt,
              width: image.width,
              height: image.height,
              ratio: image.aspect_ratio,
              model: publicContext ? undefined : image.route_model_code || image.abstract_model,
              source: previewSourceLabel,
              creationDraft: publicContext ? undefined : workspaceCreationDraftFromSnapshot(image),
              mediaExpiresAt: image.preview_expires_at,
              onMediaRefresh,
            })}><FullscreenIcon /></button>
          ) : null}
          {imageUrl ? (
            <DetailImageMedia
              src={imageUrl}
              mediaExpiresAt={image.preview_expires_at}
              alt={title}
              onMediaRefresh={onMediaRefresh}
            />
          ) : <div className="u-detail__placeholder">图片不可预览</div>}
        </div>
        <div className="u-actions" style={{ justifyContent: 'space-between' }}>
          <div className="u-actions">
            <span className="u-chip u-chip--accent">{statusLabel}</span>
            {!publicContext ? <span className="u-chip u-chip--mono">{image.route_model_code || image.abstract_model || '-'}</span> : null}
          </div>
          <div className="u-actions">
            {onLike ? publicDetailButton(`点赞 ${image.like_count ?? 0}`, <PublicDetailIcon name="heart" active={image.liked_by_viewer} />, () => onLike(image), image.liked_by_viewer ? 'liked' : '') : null}
            {onFavorite ? publicDetailButton(`收藏 ${image.favorite_count ?? 0}`, <PublicDetailIcon name="star" active={image.favorited_by_viewer} />, () => onFavorite(image), image.favorited_by_viewer ? 'favorited' : '') : null}
          </div>
        </div>
      </section>

      <section className="u-detail__info">
        <div className="u-detail__info-scroll">
          <div style={{ display: 'grid', gap: 7 }}>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
              <span className="u-chip u-chip--accent">图片</span>
              {!publicContext ? <span className="u-chip u-chip--mono">{image.route_model_code || image.abstract_model || '未知模型'}</span> : null}
              <span className="u-chip u-chip--mono">{statusLabel}</span>
            </div>
            <h2 className="u-detail__title">{title}</h2>
            <div className="u-meta">{formatDate(image.created_at || '')}{!publicContext ? ` · ${image.project?.name || '默认项目'}` : ''}</div>
          </div>

          <div className="u-detail__section">
            <h4>Prompt</h4>
            <div className="u-prompt" tabIndex={0} aria-label="Prompt">
              {prompt || '未记录 Prompt'}
              {prompt ? (
                <button className="u-icon-btn u-icon-btn--sm u-prompt__copy" type="button" aria-label="复制 Prompt" onClick={() => onCopyPrompt(prompt)}><Copy size={14} strokeWidth={1.6} aria-hidden="true" /></button>
              ) : null}
            </div>
          </div>

          <div className="u-detail__section">
            <h4>生成参数</h4>
            <dl className="u-facts">
              {facts.map((fact) => (
                <div className="u-fact" key={fact.label}><dt>{fact.label}</dt><dd title={String(fact.value)}>{fact.value}</dd></div>
              ))}
            </dl>
          </div>

          {showPublicStats ? (
            <div className="u-detail__section">
              <h4>互动与任务</h4>
              <div className="u-facts">
                {publicEngagementStats(image).map((item) => (
                  <div className="u-fact" key={item.key}><dt>{item.label}</dt><dd>{item.value}</dd></div>
                ))}
                {!publicContext ? <div className="u-fact"><dt>任务 ID</dt><dd>{('task_id' in image ? image.task_id : undefined) || image.id}</dd></div> : null}
                <div className="u-fact"><dt>来源</dt><dd>{previewSourceLabel}</dd></div>
              </div>
            </div>
          ) : null}

          {!publicContext && organization ? <div className="u-detail__section"><h4>组织与归属</h4>{organization}</div> : null}
          {!publicContext && actions.some((action) => action.key !== 'delete') ? (
            <div className="u-detail__section">
              <h4>更多操作</h4>
              <div className="u-actions">
                {actions.filter((action) => action.key !== 'delete').map((action) => (
                  <button key={action.key} type="button" className="u-action" onClick={action.onClick} disabled={action.disabled}>
                    {action.icon}
                    <span>{action.label}</span>
                  </button>
                ))}
              </div>
            </div>
          ) : null}
        </div>
        <div className="u-detail__foot">
          {onDownload ? <button className="u-btn u-btn--primary u-btn--sm" type="button" onClick={() => onDownload(image)} disabled={!image.url && !image.download_url}>下载原图</button> : null}
          {(publicContext ? [] : actions).filter((action) => action.key === 'delete').map((action) => <button key={action.key} type="button" className="u-btn u-btn--sm u-btn--danger" disabled={action.disabled} onClick={action.onClick}>{action.icon}{action.label}</button>)}
        </div>
      </section>
    </div>
  )
}

function DetailImageMedia({ src, mediaExpiresAt, alt, onMediaRefresh }: { src: string; mediaExpiresAt?: string; alt: string; onMediaRefresh?: () => string | undefined | void | Promise<string | undefined | void> }) {
  const [failed, setFailed] = useState(false)
  const { currentSrc, mediaRetryKey, markMediaLoaded, refreshFailedMedia, resetMediaRefresh } = useMediaRefreshOnce(src, onMediaRefresh, mediaExpiresAt, true)
  useEffect(() => setFailed(false), [src])
  if (failed) return <div className="u-detail__placeholder"><ImageMediaFallback onRetry={() => { resetMediaRefresh(); setFailed(false) }} /></div>
  return (
    <img
      key={mediaRetryKey}
      src={currentSrc}
      alt={alt}
      loading="lazy"
      decoding="async"
      onLoad={() => { markMediaLoaded(); setFailed(false) }}
      onError={() => {
        void refreshFailedMedia().then((refreshed) => {
          if (!refreshed) setFailed(true)
        })
      }}
    />
  )
}

export const protectedRoutes: RouteId[] = ['home', 'genpic', 'creative-canvas', 'gallery', 'projects', 'checkout', 'api-keys', 'profile', 'settings', 'oauth-consent']

function HomeIcon() { return <Home size={22} strokeWidth={1.5} /> }
function SparklesIcon() { return <Sparkles size={22} strokeWidth={1.5} /> }
function GridIcon() { return <LayoutGrid size={22} strokeWidth={1.5} /> }
function ProjectsIcon() { return <FolderKanban size={22} strokeWidth={1.5} /> }
function CanvasIcon() { return <FolderPlus size={22} strokeWidth={1.5} /> }
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

function PublicGalleryIcon() { return <Film size={22} strokeWidth={1.5} /> }

export const navItems: Array<{ route: RouteId; label: string; icon: React.ReactNode }> = [
  { route: 'home', label: '首页', icon: <HomeIcon /> },
  { route: 'genpic', label: '创作', icon: <SparklesIcon /> },
  { route: 'creative-canvas', label: '画布', icon: <CanvasIcon /> },
  { route: 'gallery', label: '资产', icon: <GridIcon /> },
  { route: 'projects', label: '项目', icon: <ProjectsIcon /> },
  { route: 'public-gallery', label: '广场', icon: <PublicGalleryIcon /> },
  { route: 'checkout', label: '积分', icon: <CreditCardIcon /> },
  { route: 'api-keys', label: '密钥', icon: <KeyIcon /> },
  { route: 'settings', label: '设置', icon: <SettingsIcon /> },
]

export const mobileNavItems: Array<{ route: RouteId; label: string; icon: React.ReactNode }> = [
  { route: 'home', label: '首页', icon: <HomeIcon /> },
  { route: 'genpic', label: '创作', icon: <SparklesIcon /> },
  { route: 'creative-canvas', label: '画布', icon: <CanvasIcon /> },
  { route: 'gallery', label: '资产', icon: <GridIcon /> },
  { route: 'profile', label: '我的', icon: <UserIcon /> },
]

export function mobileNavActiveRoute(route: RouteId): RouteId {
  return route === 'projects' || route === 'checkout' || route === 'api-keys' || route === 'settings' ? 'profile' : route
}

export function Shell({ children, scrollMode = 'app' }: { children: React.ReactNode; scrollMode?: ShellScrollMode }) {
  const app = useApp()
  const projects = useProjects()
  const [menuOpen, setMenuOpen] = useState(false)
  const [menuPosition, setMenuPosition] = useState({ top: 0, right: 8 })
  const [projectOpen, setProjectOpen] = useState(false)
  const [projectPosition, setProjectPosition] = useState({ top: 0, left: 8 })
  const menuRef = useRef<HTMLDivElement | null>(null)
  const menuButtonRef = useRef<HTMLButtonElement | null>(null)
  const projectMenuRef = useRef<HTMLDivElement | null>(null)
  const projectButtonRef = useRef<HTMLButtonElement | null>(null)
  const mainScrollRef = useRef<HTMLElement | null>(null)
  const accountMenuItems = avatarMenuItems()
  const isDark = app.themePreference.mode === 'dark'
  const visibleNavItems = app.featureFlags.creative_canvas ? navItems : navItems.filter((item) => item.route !== 'creative-canvas')
  const visibleMobileNavItems = app.featureFlags.creative_canvas
    ? mobileNavItems
    : mobileNavItems.map((item) => item.route === 'creative-canvas' ? { route: 'projects' as RouteId, label: '项目', icon: <ProjectsIcon /> } : item)
  const mobileActiveRoute = mobileNavActiveRoute(app.route)
  const activeNavIndex = shellActiveNavIndex(app.route, visibleNavItems)
  const activeProjectName = projects.selectedProject?.name ?? '默认项目'
  const pointsLabel = app.balance ? pointsText(app.balance.available_points) : '...'

  useLayoutEffect(() => {
    resetShellScroll(scrollMode, mainScrollRef.current, window)
  }, [app.route, scrollMode])

  useEffect(() => {
    if (!menuOpen && !projectOpen) return undefined
    const close = (event: MouseEvent) => {
      const target = event.target as Node
      if (menuOpen && !menuRef.current?.contains(target) && !menuButtonRef.current?.contains(target)) setMenuOpen(false)
      if (projectOpen && !projectMenuRef.current?.contains(target) && !projectButtonRef.current?.contains(target)) setProjectOpen(false)
    }
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      if (projectOpen) {
        setProjectOpen(false)
        projectButtonRef.current?.focus()
      } else if (menuOpen) {
        setMenuOpen(false)
        menuButtonRef.current?.focus()
      }
    }
    const closeOnScroll = () => { setMenuOpen(false); setProjectOpen(false) }
    window.addEventListener('mousedown', close)
    window.addEventListener('keydown', closeOnEscape)
    window.addEventListener('resize', closeOnScroll)
    window.addEventListener('scroll', closeOnScroll, true)
    return () => {
      window.removeEventListener('mousedown', close)
      window.removeEventListener('keydown', closeOnEscape)
      window.removeEventListener('resize', closeOnScroll)
      window.removeEventListener('scroll', closeOnScroll, true)
    }
  }, [menuOpen, projectOpen])

  const toggleAccountMenu = () => {
    if (menuOpen) {
      setMenuOpen(false)
      return
    }
    const rect = menuButtonRef.current?.getBoundingClientRect()
    if (rect) setMenuPosition({ top: Math.round(rect.bottom + 8), right: Math.max(8, Math.round(window.innerWidth - rect.right)) })
    setProjectOpen(false)
    setMenuOpen(true)
  }

  const toggleProjectMenu = () => {
    if (projectOpen) {
      setProjectOpen(false)
      return
    }
    const rect = projectButtonRef.current?.getBoundingClientRect()
    if (rect) setProjectPosition({ top: Math.round(rect.bottom + 8), left: Math.max(8, Math.round(rect.left)) })
    setMenuOpen(false)
    setProjectOpen(true)
  }

  return (
    <>
      <div className="u-ambient" aria-hidden="true" />
      <div className="u-grain" aria-hidden="true" />
      <div className="u-shell">
        <aside className="u-rail" aria-label={`${siteBrand.name} 用户导航`}>
          <button className="u-rail__brand" type="button" onClick={() => app.navigate('home')} aria-label={`${siteBrand.name} 首页`}>
            <img src={siteBrand.logoUrl} alt="" aria-hidden="true" />
          </button>
          <nav className="u-rail__nav" aria-label="主导航">
            {activeNavIndex >= 0 ? <span className="u-rail__indicator" style={{ transform: `translateY(${activeNavIndex * 55}px)` }} aria-hidden="true" /> : null}
            {visibleNavItems.map((item) => (
              <a
                key={item.route}
                href={`#/${item.route}`}
                className={cn('u-rail__link', app.route === item.route && 'is-active')}
                aria-current={app.route === item.route ? 'page' : undefined}
                onClick={(event) => {
                  event.preventDefault()
                  app.navigate(item.route)
                }}
              >
                {item.icon}
                <span>{item.label}</span>
              </a>
            ))}
          </nav>
          <div className="u-rail__spacer" />
          <div className="u-rail__foot">
            <button
              type="button"
              className="u-icon-btn u-icon-btn--plain"
              aria-label={isDark ? '切换浅色主题' : '切换深色主题'}
              title={isDark ? '切换浅色主题' : '切换深色主题'}
              onClick={() => void app.setThemePreference({ mode: isDark ? 'light' : 'dark' })}
            >
              {isDark ? <MoonIcon /> : <SunIcon />}
            </button>
            <button type="button" className="u-icon-btn u-icon-btn--plain" aria-label="退出登录" title="退出登录" onClick={() => void app.logout()}>
              <LogoutIcon />
            </button>
          </div>
        </aside>

        <main ref={mainScrollRef} className="u-main">
          <header className="u-topbar">
            <div className="u-topbar__context">
              <button
                ref={projectButtonRef}
                className="u-topbar__project"
                type="button"
                aria-haspopup="menu"
                aria-expanded={projectOpen}
                onClick={toggleProjectMenu}
              >
                <FolderKanban size={15} strokeWidth={1.6} aria-hidden="true" />
                <span>{activeProjectName}</span>
                <ChevronDown size={14} strokeWidth={1.6} aria-hidden="true" />
              </button>
            </div>
            <div className="u-topbar__spacer" />
            <div className="u-topbar__actions">
              <button className="u-icon-btn" type="button" aria-label="通知" title="通知" onClick={() => app.notify('info', '暂无新通知')}>
                <Bell size={17} strokeWidth={1.6} aria-hidden="true" />
              </button>
              <button
                className="u-balance"
                type="button"
                onClick={() => app.navigate('checkout')}
                title={pointsExchangeRules.join('\n')}
                aria-label={`可用余额 ${pointsLabel}。${pointsExchangeRules.join('；')}`}
              >
                <span className="u-topbar__dot" aria-hidden="true" />
                <span className="u-balance__label">可用积分</span>
                <b>{pointsLabel}</b>
                <span className="u-balance__plus" aria-hidden="true"><Plus size={13} strokeWidth={2} /></span>
              </button>
              <button
                ref={menuButtonRef}
                className="u-avatar"
                type="button"
                aria-label="账户菜单"
                aria-haspopup="menu"
                aria-expanded={menuOpen}
                onClick={toggleAccountMenu}
              >
                {app.profile?.avatar_initials ?? 'PG'}
              </button>
            </div>
          </header>

          <div className="u-content">
            <div key={app.route} className={cn('flex min-h-0 flex-1 flex-col', shellLayoutClasses(scrollMode).content)}>{children}</div>
          </div>
        </main>
      </div>

      <nav className="u-bottom-nav" aria-label={`${siteBrand.name} 移动导航`}>
        {visibleMobileNavItems.map((item) => (
          <button
            key={item.route}
            type="button"
            className={cn(mobileActiveRoute === item.route && 'is-active')}
            aria-current={mobileActiveRoute === item.route ? 'page' : undefined}
            onClick={() => app.navigate(item.route)}
          >
            {item.icon}
            <span>{item.label}</span>
          </button>
        ))}
      </nav>

      {projectOpen ? (
        <div
          ref={projectMenuRef}
          className="u-account-menu is-open"
          style={{ top: projectPosition.top, left: projectPosition.left, right: 'auto', width: 280 }}
          role="menu"
          aria-label="切换项目"
        >
          <div className="u-account-menu__head" style={{ paddingBottom: 8 }}>
            <div className="u-account-menu__identity">
              <strong>切换项目</strong>
              <span>{projects.projects.length} 个项目</span>
            </div>
          </div>
          {projects.projects.length ? projects.projects.map((project) => (
            <button
              key={project.id}
              type="button"
              role="menuitemradio"
              aria-checked={project.id === projects.selectedProjectID}
              className="u-account-menu__item"
              onClick={() => {
                projects.selectProject(project.id)
                setProjectOpen(false)
              }}
            >
              <FolderKanban size={15} strokeWidth={1.6} aria-hidden="true" />
              <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{project.name}</span>
              {project.id === projects.selectedProjectID ? <span className="u-dot" style={{ color: 'var(--accent)' }} aria-hidden="true" /> : null}
            </button>
          )) : <div className="u-account-menu__item" style={{ cursor: 'default' }}>{projects.loading ? '正在载入项目…' : '暂无项目'}</div>}
          <div className="u-account-menu__sep" />
          <button type="button" role="menuitem" className="u-account-menu__item" onClick={() => { setProjectOpen(false); app.navigate('projects') }}>
            <Settings size={15} strokeWidth={1.6} aria-hidden="true" />管理项目
          </button>
        </div>
      ) : null}

      {menuOpen ? (
        <div
          ref={menuRef}
          className="u-account-menu is-open"
          style={{ top: menuPosition.top, right: menuPosition.right }}
          role="menu"
          aria-label="账户菜单"
        >
          <div className="u-account-menu__head">
            <span className="u-avatar">{app.profile?.avatar_initials ?? 'PG'}</span>
            <div className="u-account-menu__identity">
              <strong>{app.profile?.display_name ?? 'Guest'}</strong>
              <span>{app.profile?.email || '未绑定邮箱'}</span>
            </div>
          </div>
          <div className="u-account-menu__balance">
            <span>可用积分</span>
            <b>{app.balance ? pointsText(app.balance.available_points) : '...'}</b>
          </div>
          {accountMenuItems.map((item) => (
            <button
              key={item.key}
              type="button"
              role="menuitem"
              data-permission={item.permission}
              className="u-account-menu__item"
              style={{ width: '100%', border: 0, background: 'transparent', textAlign: 'left' }}
              onClick={() => {
                setMenuOpen(false)
                if (item.external) openDocsEntry('account-menu')
                else app.navigate(item.route)
              }}
            >
              {avatarMenuIcon(item.icon)}
              {item.label}
            </button>
          ))}
          <div className="u-account-menu__sep" />
          <button
            type="button"
            role="menuitem"
            className="u-account-menu__item u-account-menu__item--danger"
            style={{ width: '100%', border: 0, background: 'transparent', textAlign: 'left' }}
            onClick={() => { setMenuOpen(false); void app.logout() }}
          >
            <LogoutIcon />退出登录
          </button>
        </div>
      ) : null}
    </>
  )
}

export function ToastViewport({ toasts, onExpire }: { toasts: Toast[]; onExpire: (id: number) => void }) {
  return (
    <div className={`${userState.toastStack} u-toast-viewport`} aria-label="操作反馈">
      {toasts.map((toast) => (
        <ToastItem key={toast.id} toast={toast} onExpire={onExpire} />
      ))}
    </div>
  )
}

const toastPresentation = {
  success: { label: '成功', Icon: CircleCheck },
  warn: { label: '警告', Icon: TriangleAlert },
  error: { label: '错误', Icon: CircleX },
  info: { label: '提示', Icon: Info },
} as const

function ToastItem({ toast, onExpire }: { toast: Toast; onExpire: (id: number) => void }) {
  const lifetime = useToastLifetime(toast.durationMs ?? 4200, () => onExpire(toast.id))
  const { label, Icon } = toastPresentation[toast.tone]
  return (
    <div {...lifetime} className={`u-toast-message u-toast-message--${toast.tone}`}>
      <Icon size={20} aria-hidden="true" className="u-toast-message__icon" />
      <div role={toast.tone === 'error' ? 'alert' : 'status'} aria-atomic="true">
        <strong>{label}</strong><p>{toast.message}</p>
      </div>
      <button type="button" className="u-toast-message__close" aria-label={`关闭${label}通知`} onClick={() => onExpire(toast.id)}><X size={16} aria-hidden="true" /></button>
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
    <label className="u-field">
      <span>{label}{hint ? <em className="u-label__hint">{hint}</em> : null}</span>
      {children}
      {error ? <span className="u-field__error" role="alert">{error}</span> : null}
    </label>
  )
}

export function Button({ children, tone = 'primary', busy = false, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement> & { tone?: 'primary' | 'ghost' | 'danger'; busy?: boolean }) {
  const toneClass = tone === 'primary' ? 'u-btn--primary' : tone === 'danger' ? 'u-btn--danger' : 'u-btn--quiet'
  return (
    <button {...props} className={cn('u-btn', toneClass, props.className)} disabled={props.disabled || busy}>
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
      className={cn('u-icon-btn', size === 'sm' && 'u-icon-btn--sm', tone === 'danger' && 'u-icon-btn--danger', props.className)}
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
    <div className={cn('u-segment', className)} role="group" aria-label={label}>
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          className={cn(value === option.value && 'is-active')}
          aria-pressed={value === option.value}
          disabled={disabled || option.disabled}
          onClick={() => onChange(option.value)}
        >
          {option.icon}
          <span>{option.label}</span>
        </button>
      ))}
    </div>
  )
}

export function Toolbar({ label = '页面工具栏', children, className = '' }: { label?: string; children: React.ReactNode; className?: string }) {
  return <div className={cn('u-toolbar', className)} role="toolbar" aria-label={label}>{children}</div>
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
    <Toolbar label={label} className={className}>
      <label className="u-search max-[620px]:basis-full">
        <span className="sr-only">{queryPlaceholder}</span>
        <Search size={15} strokeWidth={1.6} aria-hidden="true" />
        <input
          type="search"
          value={query}
          placeholder={queryPlaceholder}
          onChange={(event) => onQueryChange(event.target.value)}
        />
      </label>
      {filters ? <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2 max-[620px]:basis-full">{filters}</div> : null}
      {meta ? <span className="u-meta" aria-live="polite">{meta}</span> : null}
      {action}
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
    <dl className={cn('u-stat-row', className)} aria-label={label}>
      {items.map((item) => (
        <div key={item.key} className="u-stat">
          <dt className="u-stat__label">{item.label}</dt>
          <dd className={cn('u-stat__value', item.tone === 'error' && 'text-[var(--danger)]')}>{item.value}</dd>
          <span className={cn('u-stat__foot', toneClass[item.tone ?? 'neutral'])}>{item.tone === 'warning' ? '需要关注' : item.tone === 'error' ? '异常' : item.tone === 'success' ? '正常' : '就绪'}</span>
        </div>
      ))}
    </dl>
  )
}

export function Surface({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return <section className={cn('u-plate', className)}>{children}</section>
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

export function GalleryImageFrame({ src, mediaExpiresAt, alt, width, height, aspectRatio = '4 / 3', actions, topAction, onOpen, onMediaRefresh, selected = false, className = '', imageClassName = '' }: {
  src?: string
  mediaExpiresAt?: string
  alt: string
  width?: number
  height?: number
  aspectRatio?: string
  actions?: React.ReactNode
  topAction?: React.ReactNode
  onOpen?: () => void
  onMediaRefresh?: () => string | undefined | void | Promise<string | undefined | void>
  selected?: boolean
  className?: string
  imageClassName?: string
}) {
  const [imageState, setImageState] = useState<'loading' | 'ready' | 'error'>(src ? 'loading' : 'error')
  const [retryKey, setRetryKey] = useState(0)
  const imageRef = useRef<HTMLImageElement>(null)
  const { currentSrc, mediaRetryKey, markMediaLoaded, refreshFailedMedia, resetMediaRefresh } = useMediaRefreshOnce(src, onMediaRefresh, mediaExpiresAt)

  useLayoutEffect(() => {
    const image = imageRef.current
    if (!src) {
      setImageState('error')
    } else if (image && image.complete && image.naturalWidth > 0) {
      setImageState('ready')
    } else {
      setImageState('loading')
    }
  }, [currentSrc, retryKey, src])

  const media = src ? (
    <img
      ref={imageRef}
      key={`${currentSrc}:${retryKey}:${mediaRetryKey}`}
      src={currentSrc}
      alt={alt}
      width={width}
      height={height}
      loading="lazy"
      decoding="async"
      className={cn('size-full object-cover transition duration-700 ease-out group-hover:scale-[1.025] motion-reduce:transition-none motion-reduce:transform-none', imageState !== 'ready' && 'opacity-0', imageClassName)}
      onLoad={() => { markMediaLoaded(); setImageState('ready') }}
      onError={() => {
        void refreshFailedMedia().then((refreshed) => {
          if (!refreshed) setImageState('error')
        })
      }}
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
        <button type="button" data-gallery-card-open className="absolute inset-0 z-[1] block size-full cursor-zoom-in border-0 bg-transparent p-0 text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[-3px] focus-visible:outline-[var(--focus-ring)]" onClick={onOpen} aria-label={`查看${alt}`}>
          {media}
        </button>
      ) : media}
      {imageState === 'error' ? (
        <div className="absolute inset-0 z-[5] grid place-items-center gap-2 bg-[var(--canvas-bg)] p-5 text-center text-sm text-[var(--muted)]" role="status">
          <span>图片暂时无法显示</span>
          {src ? <button type="button" className="min-h-10 rounded-xl border border-[var(--border)] px-3 text-[var(--fg)] hover:border-[var(--accent)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-[var(--focus-ring)]" onClick={() => { resetMediaRefresh(); setImageState('loading'); setRetryKey((value) => value + 1) }}>重试</button> : null}
        </div>
      ) : null}
      {topAction ? <div data-gallery-selection-control className="absolute left-3 top-3 z-[4]">{topAction}</div> : null}
      {actions ? <div data-gallery-card-actions className={cn(imageFrameActionsClass, 'absolute inset-x-3 bottom-3 z-[4] flex translate-y-2 flex-wrap justify-end gap-2 opacity-0 transition-all duration-[var(--motion-fast)] group-hover:translate-y-0 group-hover:opacity-100 group-focus-within:translate-y-0 group-focus-within:opacity-100 motion-reduce:transition-none')}>{actions}</div> : null}
    </figure>
  )
}

export function PageIntro({ eyebrow, title, detail, action }: { eyebrow?: string; title: string; detail?: string; action?: React.ReactNode }) {
  return (
    <div className="u-page-head">
      <div className="u-page-head__copy">
        {eyebrow ? <span className="u-eyebrow">{eyebrow}</span> : null}
        <h1 className="u-h1">{title}</h1>
        {detail ? <p className="u-sub">{detail}</p> : null}
      </div>
      {action ? <div className="u-page-head__actions">{action}</div> : null}
    </div>
  )
}

export function LoadingState({ label = '正在读取实时数据...' }: { label?: string }) {
  return (
    <div className="u-empty" role="status">
      <span className="u-empty__icon"><span className={userState.spinner} aria-hidden="true" /></span>
      <h3>{label}</h3>
    </div>
  )
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="u-empty error-state" role="alert">
      <span className="u-empty__icon" aria-hidden="true">!</span>
      <h3>读取失败</h3>
      <p>{message}</p>
      {onRetry ? <Button tone="ghost" onClick={onRetry}>重试</Button> : null}
    </div>
  )
}

export function EmptyState({ title, detail, action, icon }: { title: string; detail: string; action?: React.ReactNode; icon?: React.ReactNode }) {
  return (
    <div className="u-empty">
      {icon ? <div className="u-empty__icon">{icon}</div> : null}
      <h3>{title}</h3>
      <p>{detail}</p>
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
      <div className="u-backdrop is-open" role="presentation" data-focus-layer onMouseDown={onClose}>
        <section ref={dialogRef} tabIndex={-1} className={cn('u-modal is-open', className)} role="dialog" aria-modal="true" aria-label={title} onMouseDown={(event) => event.stopPropagation()}>
          <div className="u-modal__head">
            <h2>{title}</h2>
            <IconButton label="关闭" onClick={onClose}><X size={18} strokeWidth={1.5} aria-hidden="true" /></IconButton>
          </div>
          {children}
        </section>
      </div>
    </OverlayPortal>
  )
}

export function Drawer({ open, title, children, onClose, side = 'right', wide = false, className = '' }: {
  open: boolean
  title: string
  children: React.ReactNode
  onClose: () => void
  side?: 'left' | 'right' | 'bottom'
  wide?: boolean
  className?: string
}) {
  const drawerRef = useRef<HTMLElement | null>(null)
  useDismissableLayer(open, onClose, drawerRef)
  if (!open) return null
  const width = wide ? 'min(1080px, 96vw)' : 'min(720px, 100vw)'
  const position = side === 'left'
    ? { inset: '0 auto 0 0', width, borderLeft: 0, borderRight: '1px solid var(--line-2)', transform: 'none' }
    : side === 'bottom'
      ? { inset: 'auto 0 0 0', width: '100vw', maxHeight: '88vh', borderTop: '1px solid var(--line-2)', transform: 'none' }
      : { width }
  return (
    <OverlayPortal>
      <div className="u-backdrop is-open" role="presentation" data-focus-layer onMouseDown={onClose}>
        <section
          ref={drawerRef}
          tabIndex={-1}
          role="dialog"
          aria-modal="true"
          aria-label={title}
          className={cn('u-drawer is-open', className)}
          style={position}
          onMouseDown={(event) => event.stopPropagation()}
        >
          <header className="u-drawer__head">
            <div className="u-drawer__title"><strong>{title}</strong></div>
            <div className="u-drawer__spacer" />
            <IconButton label="关闭" onClick={onClose}><X size={18} strokeWidth={1.5} aria-hidden="true" /></IconButton>
          </header>
          <div className="u-drawer__body">{children}</div>
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
	const tone = status === 'succeeded' || status === 'public' || status === 'approved' || status === 'active'
    ? 'good'
    : status === 'failed' || status === 'rejected' || status === 'disabled'
      ? 'bad'
		: status === 'running' || status === 'queued' || status === 'reviewing' || status === 'pending_review'
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
		pending_review: '审核中',
		public: '已公开',
		approved: '已公开',
		rejected: '已拒绝',
		unpublished: '已下架',
    active: '启用中',
    disabled: '已禁用',
  }
  const statusTone = tone === 'good' ? 'u-status--ok' : tone === 'bad' ? 'u-status--bad' : tone === 'warn' ? 'u-status--warn' : ''
  return <span className="u-chip"><span className={cn('u-status', statusTone)}><span className="u-dot" aria-hidden="true" />{label[status] ?? status}</span></span>
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

// Compact icon-only copy affordance for meta rows (task ids, tracking ids).
export function InlineCopyButton({ text, label }: { text: string; label?: string }) {
  const app = useApp()
  return (
    <button
      type="button"
      className="inline-flex shrink-0 items-center rounded p-0.5 align-middle text-[var(--muted)] transition-colors hover:text-[var(--fg)]"
      title={label ?? '复制'}
      aria-label={label ?? '复制'}
      onClick={async (event) => {
        event.stopPropagation()
        await copyText(text)
        app.notify('success', '已复制到剪贴板')
      }}
    >{label ?? <Copy size={13} aria-hidden="true" />}</button>
  )
}
