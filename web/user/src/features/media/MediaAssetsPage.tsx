import { continueMediaCreation, mediaDownloadFilename } from './mediaCreationNavigation'
import { useCallback, useEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'
import {
  ExternalLink,
  FolderInput,
  Image as ImageIcon,
  MoreHorizontal,
  Pencil,
  RefreshCw,
  Trash2,
  Upload,
  X,
} from 'lucide-react'
import type { ImageResult, MediaAsset, MediaBatchAction, MediaType, Project, PublishStatus } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { cn } from '../../../../shared/classnames'
import { useProjects } from '../../ProjectContext'
import { EmptyState, ErrorState, GalleryFilterToolbar, ImageDetailModal, copyText, imageRatioLabel, useApp, type ImageDetailAction } from '../../components'
import { userHashForRoute } from '../../routeState'
import { MediaAssetCard, formatBytes } from './MediaAssetCard'
import { MediaGroupSelectionDialog } from './MediaGroupSelectionDialog'
import { VideoPublicationControls } from './VideoPublicationControls'
import { ImagePublicationControls, PublicationControls } from './PublicationControls'
import { videoPublicationApi } from './videoPublicationApi'
import { MediaPreviewDialog } from './MediaPreviewDialog'
import {
  buildMediaAssetQuery,
  mediaCreationActions,
  mediaMarqueeSelection,
  mediaSelectionDragDistance,
  mediaSelectionRectangle,
  reconcileBatchSelection,
  shouldSuppressMediaCardAction,
  type MediaSelectionPoint,
  type MediaSelectionRect,
} from './mediaExperience'
import { MEDIA_ASSETS_CHANGED_EVENT } from './UploadTray'
import { pollGalleryExportJob } from '../../pages/galleryBatchActions'
import { OverlayPortal } from '../../ui/overlayPortal'
import './media-redesign.css'

type FilterState = {
  mediaType: '' | MediaType
  sourceType: string
  groupName: string
  status: string
  visibilityStatus: NonNullable<import('../../../../shared/api-types').MediaAssetFilters['visibility_status']>
  keyword: string
  sort: string
}

const initialFilters: FilterState = { mediaType: '', sourceType: '', groupName: '', status: '', visibilityStatus: 'all', keyword: '', sort: 'created_at:desc' }

function publishStatusForAsset(value: string): PublishStatus {
  if (value === 'public' || value === 'approved') return 'public'
  if (value === 'pending' || value === 'pending_review' || value === 'reviewing') return 'pending_review'
  if (value === 'offline') return 'unpublished'
  if (value === 'rejected' || value === 'unpublished') return value
  return 'private'
}

function mediaAssetAsImageResult(asset: MediaAsset, imageURL: string, project?: Project | null): ImageResult {
  return {
    id: asset.id,
    name: asset.name,
    project_id: asset.project_id,
    project: project ? { id: project.id, name: project.name, is_default: project.is_default } : undefined,
    url: imageURL,
    download_url: imageURL,
    mime_type: asset.mime_type,
    file_size_bytes: asset.file_size_bytes,
    width: asset.width ?? 0,
    height: asset.height ?? 0,
    publish_status: publishStatusForAsset(asset.visibility_status),
    visibility_status: publishStatusForAsset(asset.visibility_status),
    aspect_ratio: asset.width && asset.height ? imageRatioLabel(asset.width, asset.height) : undefined,
    output_format: asset.container || asset.mime_type.split('/')[1]?.toUpperCase(),
    created_at: asset.created_at,
    prompt: '',
  }
}

export function MediaAssetsPage() {
  const app = useApp()
  const projects = useProjects()
  const [filters, setFilters] = useState(initialFilters)
  const [items, setItems] = useState<MediaAsset[]>([])
  const [nextCursor, setNextCursor] = useState('')
  const [selected, setSelected] = useState(new Set<string>())
  const [preview, setPreview] = useState<MediaAsset | null>(null)
  const [mediaPreview, setMediaPreview] = useState<MediaAsset | null>(null)
  const [detailURL, setDetailURL] = useState('')
  const [detailBusy, setDetailBusy] = useState(false)
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [refreshKey, setRefreshKey] = useState(0)
  const [groupTarget, setGroupTarget] = useState<MediaAsset | 'batch' | null>(null)
  const [targetProjectID, setTargetProjectID] = useState('')
  const requestVersion = useRef(0)
  const selectionSurfaceRef = useRef<HTMLDivElement | null>(null)
  const suppressCardActionUntilRef = useRef(0)
  const marqueeFrameRef = useRef<number | null>(null)
  const marqueeDragRef = useRef<{ pointerID: number; start: MediaSelectionPoint; current: MediaSelectionPoint; additive: boolean; dragged: boolean; captureTarget: HTMLElement } | null>(null)
  const [marquee, setMarquee] = useState<MediaSelectionRect | null>(null)

  useEffect(() => () => {
    if (marqueeFrameRef.current !== null) window.cancelAnimationFrame(marqueeFrameRef.current)
  }, [])

  useEffect(() => {
    requestVersion.current += 1
    setItems([])
    setNextCursor('')
    setSelected(new Set())
    setPreview(null)
    setMediaPreview(null)
    setGroupTarget(null)
    setTargetProjectID('')
  }, [projects.selectedProjectID])

  const load = useCallback(async (append = false) => {
    if (!projects.selectedProjectID) return
    const version = ++requestVersion.current
    setLoading(true)
    setError('')
    try {
      const request = buildMediaAssetQuery(projects.selectedProjectID, filters, append ? nextCursor : undefined)
      const page = await userApi.listMediaAssets(request)
      if (version !== requestVersion.current) return
      const scopedItems = page.items.filter((item) => item.project_id === projects.selectedProjectID)
      setItems((current) => append ? [...current, ...scopedItems.filter((item) => !current.some((existing) => existing.id === item.id))] : scopedItems)
      setNextCursor(page.next_cursor ?? '')
      if (!append) setSelected(new Set())
    } catch (caught) {
      if (version !== requestVersion.current) return
      setError(caught instanceof Error ? caught.message : '资产加载失败')
    } finally {
      if (version === requestVersion.current) setLoading(false)
    }
  }, [filters, nextCursor, projects.selectedProjectID])

  useEffect(() => { void load(false) }, [projects.selectedProjectID, filters.mediaType, filters.sourceType, filters.groupName, filters.status, filters.visibilityStatus, filters.keyword, filters.sort, refreshKey])
  useEffect(() => {
    const refresh = (event: Event) => {
      const detail = (event as CustomEvent<{ projectID?: string }>).detail
      if (!detail?.projectID || detail.projectID === projects.selectedProjectID) setRefreshKey((value) => value + 1)
    }
    window.addEventListener(MEDIA_ASSETS_CHANGED_EVENT, refresh)
    return () => window.removeEventListener(MEDIA_ASSETS_CHANGED_EVENT, refresh)
  }, [projects.selectedProjectID])

  useEffect(() => {
    if (!preview || preview.media_type !== 'image') {
      setDetailURL('')
      return undefined
    }
    let alive = true
    setDetailURL('')
    const assetID = preview.id
    const loadPreview = async () => {
      try {
        const access = await userApi.getMediaAssetAccess(assetID, 'preview')
        if (alive) setDetailURL(userApi.mediaAssetUrl(access.url, app.session?.token))
      } catch (caught) {
        try {
          const access = await userApi.getMediaAssetAccess(assetID, 'thumbnail')
          if (alive) setDetailURL(userApi.mediaAssetUrl(access.url, app.session?.token))
        } catch {
          if (alive) {
            setDetailURL('')
            app.notify('error', caught instanceof Error ? caught.message : '媒体预览加载失败')
          }
        }
      }
    }
    void loadPreview()
    return () => { alive = false }
  }, [preview?.id, preview?.version, preview?.media_type, app.session?.token])

  const groups = useMemo(() => Array.from(new Set(items.map((item) => item.group_name).filter(Boolean))).sort(), [items])
  const selectedItems = useMemo(() => items.filter((item) => selected.has(item.id)), [items, selected])
  const allSelected = items.length > 0 && items.every((item) => selected.has(item.id))
  const stats = useMemo(() => {
    const images = items.filter((item) => item.media_type === 'image').length
    const videos = items.filter((item) => item.media_type === 'video').length
    const storage = items.reduce((sum, item) => sum + (Number.isFinite(item.file_size_bytes) ? item.file_size_bytes : 0), 0)
    return { total: items.length, images, videos, storage }
  }, [items])
  const detailImage = useMemo(() => preview ? mediaAssetAsImageResult(preview, detailURL, projects.selectedProject) : null, [preview, detailURL, projects.selectedProject])

  const toggle = (asset: MediaAsset) => setSelected((current) => {
    const next = new Set(current)
    if (next.has(asset.id)) next.delete(asset.id)
    else next.add(asset.id)
    return next
  })
  const selectAll = () => setSelected(new Set(items.map((item) => item.id)))
  const invert = () => setSelected(new Set(items.filter((item) => !selected.has(item.id)).map((item) => item.id)))

  const consumeSuppressedCardAction = () => {
    return shouldSuppressMediaCardAction(suppressCardActionUntilRef.current, performance.now())
  }
  const selectCard = (asset: MediaAsset) => {
    if (!consumeSuppressedCardAction()) toggle(asset)
  }
  const openCard = (asset: MediaAsset) => {
    if (consumeSuppressedCardAction()) return
    if (asset.media_type === 'image') setPreview(asset)
    else setMediaPreview(asset)
  }

  const beginMarqueeSelection = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.pointerType !== 'mouse' || event.button !== 0) return
    const target = event.target as HTMLElement
    if (target.closest('[data-media-selection-control]')) return
    const point = { x: event.clientX, y: event.clientY }
    marqueeDragRef.current = {
      pointerID: event.pointerId,
      start: point,
      current: point,
      additive: event.metaKey || event.ctrlKey || event.shiftKey,
      dragged: false,
      captureTarget: target,
    }
    target.setPointerCapture(event.pointerId)
  }

  const moveMarqueeSelection = (event: ReactPointerEvent<HTMLDivElement>) => {
    const drag = marqueeDragRef.current
    if (!drag || drag.pointerID !== event.pointerId) return
    drag.current = { x: event.clientX, y: event.clientY }
    if (!drag.dragged) {
      if (mediaSelectionDragDistance(drag.start, drag.current) < 6) return
      drag.dragged = true
    }
    event.preventDefault()
    if (marqueeFrameRef.current !== null) return
    marqueeFrameRef.current = window.requestAnimationFrame(() => {
      marqueeFrameRef.current = null
      const current = marqueeDragRef.current
      if (!current?.dragged) return
      setMarquee(mediaSelectionRectangle(current.start, current.current))
      if (current.current.y < 56) window.scrollBy({ top: -12 })
      else if (current.current.y > window.innerHeight - 56) window.scrollBy({ top: 12 })
    })
  }

  const finishMarqueeSelection = (event: ReactPointerEvent<HTMLDivElement>) => {
    const drag = marqueeDragRef.current
    if (!drag || drag.pointerID !== event.pointerId) return
    if (drag.dragged) {
      const rectangle = mediaSelectionRectangle(drag.start, drag.current)
      const candidates = Array.from(selectionSurfaceRef.current?.querySelectorAll<HTMLElement>('[data-media-asset-id]') ?? []).map((element) => {
        const bounds = element.getBoundingClientRect()
        return { id: element.dataset.mediaAssetId ?? '', rect: { left: bounds.left, top: bounds.top, right: bounds.right, bottom: bounds.bottom } }
      }).filter((item) => item.id)
      setSelected((current) => mediaMarqueeSelection(current, candidates, rectangle, drag.additive))
      suppressCardActionUntilRef.current = performance.now() + 250
    }
    if (drag.captureTarget.hasPointerCapture(event.pointerId)) drag.captureTarget.releasePointerCapture(event.pointerId)
    marqueeDragRef.current = null
    setMarquee(null)
  }

  const cancelMarqueeSelection = (event: ReactPointerEvent<HTMLDivElement>) => {
    const drag = marqueeDragRef.current
    if (!drag || drag.pointerID !== event.pointerId) return
    if (drag.captureTarget.hasPointerCapture(event.pointerId)) drag.captureTarget.releasePointerCapture(event.pointerId)
    marqueeDragRef.current = null
    setMarquee(null)
  }

  const loseMarqueeCapture = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (marqueeDragRef.current?.pointerID !== event.pointerId) return
    marqueeDragRef.current = null
    setMarquee(null)
  }

  const batch = async (action: MediaBatchAction, options?: { group_name?: string; target_project_id?: string }) => {
    if (!selectedItems.length) return
    if (action === 'delete' && !window.confirm(`删除选中的 ${selectedItems.length} 项资产？`)) return
    setBusy(true)
    try {
      if (action === 'download') {
        const created = await userApi.createMediaExport(projects.selectedProjectID, selectedItems)
        app.notify('info', '正在后台打包所选资产')
        const ready = await pollGalleryExportJob(created, userApi.getMediaExportJob)
        const archive = await userApi.downloadMediaExport(ready.job.id)
        const url = URL.createObjectURL(archive)
        const link = document.createElement('a')
        link.href = url
        link.download = 'media-assets.zip'
        link.click()
        window.setTimeout(() => URL.revokeObjectURL(url), 0)
        setSelected(new Set())
        app.notify('success', `已打包 ${selectedItems.length} 项资产`)
        return
      }
      const chunks = Array.from({ length: Math.ceil(selectedItems.length / 100) }, (_, index) => selectedItems.slice(index * 100, (index + 1) * 100))
      const responses = []
      for (const chunk of chunks) responses.push(await userApi.batchMediaAssets(action, chunk, options))
      const result = { items: responses.flatMap((response) => response.items) }
      setSelected((current) => reconcileBatchSelection(current, result.items))
      if (action === 'delete') {
        const succeeded = new Set(result.items.filter((item) => item.status === 'succeeded').map((item) => item.id))
        setItems((current) => current.filter((item) => !succeeded.has(item.id)))
      } else if (action === 'group' || action === 'transfer-project') {
        const updates = new Map(result.items.flatMap((item) => item.asset ? [[item.id, item.asset] as const] : []))
        setItems((current) => current.flatMap((item) => {
          const updated = updates.get(item.id)
          if (action === 'transfer-project' && updated) return []
          return [updated ?? item]
        }))
      }
      const failed = result.items.filter((item) => item.status !== 'succeeded').length
      app.notify(failed ? failed < result.items.length ? 'warn' : 'error' : 'success', failed ? `${result.items.length - failed} 项成功，${failed} 项失败` : `已处理 ${result.items.length} 项资产`)
    } catch (caught) {
      app.notify('error', caught instanceof Error ? caught.message : '批量操作失败')
    } finally { setBusy(false) }
  }

  const groupSelected = () => setGroupTarget('batch')
  const publishSelected = async () => {
    const images = selectedItems.filter((item) => item.media_type === 'image' && item.legacy_image_id && item.status === 'ready')
    const videos = selectedItems.filter((item) => item.media_type === 'video' && item.status === 'ready')
    if (!images.length && !videos.length) { app.notify('warn', '请选择关联历史图片记录的就绪图片或就绪视频'); return }
    setBusy(true)
    try {
      const succeededMediaIDs = new Set<string>()
      let failed = 0
      if (images.length) {
        const result = await userApi.batchPublishGalleryImages(images.map((item) => item.legacy_image_id!), projects.selectedProjectID, true)
        images.filter((item) => result.succeeded.some((entry) => entry.id === item.legacy_image_id)).forEach((item) => succeededMediaIDs.add(item.id))
        failed += result.failed.length
      }
      for (const video of videos) {
        try { await videoPublicationApi.submit(video.id); succeededMediaIDs.add(video.id) } catch { failed += 1 }
      }
      setSelected((current) => new Set(Array.from(current).filter((id) => !succeededMediaIDs.has(id))))
      setRefreshKey((value) => value + 1)
      const skipped = selectedItems.length - images.length - videos.length
      const unsuccessful = failed + skipped
      app.notify(unsuccessful ? succeededMediaIDs.size ? 'warn' : 'error' : 'success', unsuccessful ? `${succeededMediaIDs.size} 项已提交，${failed} 项失败，${skipped} 项不支持公开；未成功项保留选择` : '已提交公开审核')
    } catch (caught) { app.notify('error', caught instanceof Error ? caught.message : '公开失败') } finally { setBusy(false) }
  }
  const continueCreation = (options: Parameters<typeof userHashForRoute>[1]) => {
    void continueMediaCreation(options ?? {}).catch((error) => app.notify('error', error instanceof Error ? error.message : '无法继续创作'))
  }

  const removeAssetLocally = (asset: MediaAsset) => {
    setItems((current) => current.filter((item) => item.id !== asset.id))
    setSelected((current) => {
      if (!current.has(asset.id)) return current
      const next = new Set(current)
      next.delete(asset.id)
      return next
    })
  }
  const applyAssetPatch = (asset: MediaAsset) => {
    if (asset.project_id !== projects.selectedProjectID) {
      removeAssetLocally(asset)
      return false
    }
    setItems((current) => current.map((item) => item.id === asset.id ? asset : item))
    return true
  }
  const patchAsset = (asset: MediaAsset) => {
    if (applyAssetPatch(asset)) setPreview(asset)
    else { setPreview(null); setMediaPreview(null) }
  }
  const patchMediaPreviewAsset = (asset: MediaAsset) => {
    if (applyAssetPatch(asset)) setMediaPreview(asset)
    else { setPreview(null); setMediaPreview(null) }
  }
  const openFullPreview = (asset: MediaAsset) => {
    setPreview(null)
    setMediaPreview(asset)
  }
  const renameAsset = async (asset: MediaAsset) => {
    const name = window.prompt('资产名称', asset.name)?.trim()
    if (!name || name === asset.name) return
    setDetailBusy(true)
    try { patchAsset(await userApi.updateMediaAsset(asset, { name })) }
    catch (caught) { app.notify('error', caught instanceof Error ? caught.message : '重命名失败') }
    finally { setDetailBusy(false) }
  }
  const groupAsset = (asset: MediaAsset) => setGroupTarget(asset)
  const removeAsset = async (asset: MediaAsset) => {
    if (!window.confirm(`删除“${asset.name}”？对象文件将由后台异步清理。`)) return
    setDetailBusy(true)
    try {
      await userApi.deleteMediaAsset(asset)
      removeAssetLocally(asset)
      setPreview(null)
      setMediaPreview(null)
    } catch (caught) { app.notify('error', caught instanceof Error ? caught.message : '删除失败') }
    finally { setDetailBusy(false) }
  }
  const downloadAsset = async (asset: MediaAsset) => {
    try {
      const projection = await userApi.getMediaAssetAccess(asset.id, 'download')
      const link = document.createElement('a')
      link.href = userApi.mediaAssetUrl(projection.url, app.session?.token)
      link.download = mediaDownloadFilename(asset)
      link.rel = 'noopener'
      link.click()
    } catch (caught) { app.notify('error', caught instanceof Error ? caught.message : '下载失败') }
  }

  const detailActions = useMemo<ImageDetailAction[]>(() => {
    if (!preview) return []
    const actions: ImageDetailAction[] = mediaCreationActions(preview).map((action) => ({
      key: action.label,
      label: action.label,
      icon: <ExternalLink size={14} strokeWidth={1.5} aria-hidden="true" />,
      onClick: () => { setPreview(null); continueCreation(action.options) },
      disabled: detailBusy,
    }))
    actions.push(
      { key: 'rename', label: '重命名', icon: <Pencil size={14} strokeWidth={1.5} aria-hidden="true" />, onClick: () => void renameAsset(preview), disabled: detailBusy },
      { key: 'group', label: '设置分组', icon: <FolderInput size={14} strokeWidth={1.5} aria-hidden="true" />, onClick: () => void groupAsset(preview), disabled: detailBusy },
      { key: 'more', label: '更多操作', icon: <MoreHorizontal size={14} strokeWidth={1.5} aria-hidden="true" />, onClick: () => openFullPreview(preview), disabled: detailBusy },
      { key: 'delete', label: '删除资产', icon: <Trash2 size={14} strokeWidth={1.5} aria-hidden="true" />, onClick: () => void removeAsset(preview), disabled: detailBusy },
    )
    return actions
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [preview, detailBusy])

  return (
    <main className="media-library">
      <header className="u-page-head">
        <div className="u-page-head__copy">
          <span className="u-eyebrow">Media Library</span>
          <h1 className="u-h1">资产</h1>
          <p className="u-sub">图片、视频、音频与文档在同一个库里。筛选条件保持在同一行，批量操作只在真正选中后出现。</p>
        </div>
        <div className="u-page-head__actions">
          {app.featureFlags.media_upload ? (
            <button className="u-btn u-btn--sm" type="button" onClick={() => window.dispatchEvent(new Event('mgs:open-media-upload'))}>
              <Upload size={16} strokeWidth={1.6} aria-hidden="true" />上传文件
            </button>
          ) : null}
          <button className="u-btn u-btn--sm" type="button" aria-label="刷新资产" onClick={() => setRefreshKey((value) => value + 1)}>
            <RefreshCw size={16} strokeWidth={1.6} className={cn(loading && 'animate-spin')} aria-hidden="true" />刷新
          </button>
        </div>
      </header>

      <section className="u-stat-row media-library__stats" aria-label="资产统计">
        <div className="u-stat">
          <span className="u-stat__label">全部资产</span>
          <span className="u-stat__value">{loading && !items.length ? '—' : stats.total}</span>
          <span className="u-stat__foot">{projects.selectedProject?.name ?? '当前项目'} · 已加载</span>
        </div>
        <div className="u-stat">
          <span className="u-stat__label">图片</span>
          <span className="u-stat__value">{loading && !items.length ? '—' : stats.images}</span>
          <span className="u-stat__foot">当前筛选 · 已加载</span>
        </div>
        <div className="u-stat">
          <span className="u-stat__label">视频</span>
          <span className="u-stat__value">{loading && !items.length ? '—' : stats.videos}</span>
          <span className="u-stat__foot">当前筛选 · 已加载</span>
        </div>
        <div className="u-stat">
          <span className="u-stat__label">存储占用</span>
          <span className="u-stat__value">{loading && !items.length ? '—' : formatBytes(stats.storage)}</span>
          <span className="u-stat__foot">已加载文件合计</span>
        </div>
      </section>

      <GalleryFilterToolbar
        label="资产筛选"
        query={filters.keyword}
        queryPlaceholder="搜索名称、提示词或任务 ID"
        onQueryChange={(keyword) => setFilters((current) => ({ ...current, keyword }))}
        filters={(
          <>
            <select className="u-filter" aria-label="媒体类型" value={filters.mediaType} onChange={(event) => setFilters((current) => ({ ...current, mediaType: event.target.value as FilterState['mediaType'] }))}>
              <option value="">全部类型</option><option value="image">图片</option><option value="video">视频</option><option value="audio">音频</option>
            </select>
            <select className="u-filter" aria-label="来源" value={filters.sourceType} onChange={(event) => setFilters((current) => ({ ...current, sourceType: event.target.value }))}>
              <option value="">全部来源</option><option value="generated">平台生成</option><option value="local_upload">本地上传</option>
            </select>
            <select className="u-filter" aria-label="分组" value={filters.groupName} onChange={(event) => setFilters((current) => ({ ...current, groupName: event.target.value }))}>
              <option value="">全部分组</option>{groups.map((group) => <option key={group}>{group}</option>)}
            </select>
            <select className="u-filter" aria-label="状态" value={filters.status} onChange={(event) => setFilters((current) => ({ ...current, status: event.target.value }))}>
              <option value="">全部状态</option><option value="ready">就绪</option><option value="processing">处理中</option><option value="failed">失败</option>
            </select>
            <select className="u-filter" aria-label="公开状态" value={filters.visibilityStatus} onChange={(event) => setFilters((current) => ({ ...current, visibilityStatus: event.target.value as FilterState['visibilityStatus'] }))}>
              <option value="all">全部公开状态</option><option value="private">私有</option><option value="pending">待审核</option><option value="public">已公开</option><option value="rejected">未通过</option><option value="offline">已下架</option>
            </select>
            <select className="u-filter" aria-label="排序" value={filters.sort} onChange={(event) => setFilters((current) => ({ ...current, sort: event.target.value }))}>
              <option value="created_at:desc">最新优先</option>
              <option value="created_at:asc">最早优先</option>
              <option value="name:asc">名称 A-Z</option>
              <option value="file_size_bytes:desc">文件由大到小</option>
              <option value="duration_ms:desc">时长由长到短</option>
            </select>
            <button className="u-btn u-btn--sm u-btn--quiet" type="button" onClick={() => setFilters(initialFilters)}>重置</button>
          </>
        )}
        meta={loading && items.length ? '正在加载' : items.length ? `已加载 ${items.length} 项` : undefined}
      />

      {error && !items.length ? <ErrorState message={error} onRetry={() => void load(false)} /> : null}
      {error && items.length ? (
        <div className="u-banner u-banner--danger" role="alert">
          <span>{error}</span>
          <button className="u-btn u-btn--sm u-btn--quiet" type="button" onClick={() => void load(false)}>重试</button>
        </div>
      ) : null}

      {loading && !items.length ? (
        <div className="u-grid u-grid--gallery" aria-hidden="true">
          {Array.from({ length: 8 }).map((_, index) => (
            <div className="u-card media-skeleton-card" key={index}>
              <div className="u-skeleton media-skeleton-media" />
              <div className="u-card__body">
                <div className="u-skeleton media-skeleton-line" />
                <div className="u-skeleton media-skeleton-line is-short" />
              </div>
            </div>
          ))}
        </div>
      ) : null}

      {!loading && !error && !items.length ? (
        <EmptyState
          icon={<ImageIcon size={24} strokeWidth={1.5} aria-hidden="true" />}
          title="当前项目暂无资产"
          detail="上传文件或从创作任务生成资产后，会在这里统一管理图片、视频、音频与文档。"
          action={app.featureFlags.media_upload ? <button className="u-btn" type="button" onClick={() => window.dispatchEvent(new Event('mgs:open-media-upload'))}>上传文件</button> : undefined}
        />
      ) : null}

      <div
        ref={selectionSurfaceRef}
        className="media-assets-selection-surface"
        onPointerDown={beginMarqueeSelection}
        onPointerMove={moveMarqueeSelection}
        onPointerUp={finishMarqueeSelection}
        onPointerCancel={cancelMarqueeSelection}
        onLostPointerCapture={loseMarqueeCapture}
      >
        <section className="u-grid u-grid--gallery" aria-busy={loading} aria-label="资产列表">
          {items.map((asset) => (
            <MediaAssetCard
              key={asset.id}
              asset={asset}
              selected={selected.has(asset.id)}
              selectionMode={selected.size > 0}
              onSelect={selectCard}
              onOpen={openCard}
              onRetry={(item) => void userApi.retryMediaAssetProcessing(item.id).then(patchAsset).catch((caught) => app.notify('error', caught instanceof Error ? caught.message : '重试处理失败'))}
              footer={asset.media_type === 'image' ? <ImagePublicationControls asset={asset} onChanged={() => setRefreshKey((value) => value + 1)} /> : asset.media_type === 'video' && asset.status === 'ready' ? <VideoPublicationControls key={`${asset.id}:${asset.visibility_status}`} assetID={asset.id} onChanged={() => setRefreshKey((value) => value + 1)} /> : asset.media_type === 'video' ? <PublicationControls mediaType="video" initialStatus={asset.visibility_status} restriction="视频处理完成后才可申请公开。" change={async () => ({ status: asset.visibility_status })} onChanged={() => undefined} /> : undefined}
            />
          ))}
        </section>
      </div>

      {nextCursor && !loading ? (
        <div className="media-library__more">
          <button className="u-btn u-btn--sm" type="button" onClick={() => void load(true)}>加载更多</button>
        </div>
      ) : null}
      {loading && items.length ? <p className="u-meta" role="status">正在加载更多资产</p> : null}

      {selected.size ? (
        <OverlayPortal>
          <aside className="gl-bulk is-open" role="region" aria-label="批量操作">
            <span className="gl-bulk__count">已选 <b>{selected.size}</b> 项</span>
            <button className="u-btn u-btn--sm u-btn--quiet" type="button" onClick={allSelected ? () => setSelected(new Set()) : selectAll}>{allSelected ? '取消全选' : '全选'}</button>
            <button className="u-btn u-btn--sm u-btn--quiet" type="button" onClick={invert}>反选</button>
            <button className="u-btn u-btn--sm" type="button" disabled={busy} onClick={() => void batch('download')}>下载</button>
            <button className="u-btn u-btn--sm" type="button" disabled={busy} onClick={groupSelected}>分组</button>
            <select className="u-filter gl-bulk__select" aria-label="目标项目" value={targetProjectID} onChange={(event) => setTargetProjectID(event.target.value)}>
              <option value="">转移到项目</option>
              {projects.projects.filter((project) => project.id !== projects.selectedProjectID).map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}
            </select>
            <button className="u-btn u-btn--sm" type="button" disabled={busy || !targetProjectID} onClick={() => void batch('transfer-project', { target_project_id: targetProjectID })}>转移</button>
            <button className="u-btn u-btn--sm" type="button" disabled={busy || !selectedItems.some((item) => item.media_type === 'image' || item.media_type === 'video' && item.status === 'ready')} onClick={() => void publishSelected()}>公开</button>
            <button className="u-btn u-btn--sm u-btn--danger" type="button" disabled={busy} onClick={() => void batch('delete')}>删除</button>
            <button className="u-icon-btn u-icon-btn--plain" type="button" aria-label="取消选择" onClick={() => setSelected(new Set())}><X size={16} strokeWidth={1.6} aria-hidden="true" /></button>
          </aside>
        </OverlayPortal>
      ) : null}

      {marquee ? <OverlayPortal><div className="media-selection-marquee" style={{ left: marquee.left, top: marquee.top, width: marquee.right - marquee.left, height: marquee.bottom - marquee.top }} aria-hidden="true" /></OverlayPortal> : null}

      {preview && detailImage ? (
        <ImageDetailModal
          title={preview.name}
          image={detailImage}
          imageUrl={detailURL || undefined}
          showPublicStats={false}
          onDownload={() => void downloadAsset(preview)}
          onCopyPrompt={(prompt) => { if (prompt) void copyText(prompt) }}
          actions={detailActions}
          previewSourceLabel="资产库"
          onMediaRefresh={preview ? async () => {
            const access = await userApi.getMediaAssetAccess(preview.id, 'preview')
            const authenticatedURL = userApi.mediaAssetUrl(access.url, app.session?.token)
            setDetailURL(authenticatedURL)
            return authenticatedURL
          } : undefined}
          onClose={() => setPreview(null)}
        />
      ) : null}

      {groupTarget ? <MediaGroupSelectionDialog existingGroups={groups} projectId={projects.selectedProjectID} value={groupTarget === 'batch' ? '' : groupTarget.group_name} onClose={() => setGroupTarget(null)} onSelect={async (groupName) => {
        if (groupTarget === 'batch') await batch('group', { group_name: groupName })
        else patchAsset(await userApi.updateMediaAsset(groupTarget, { group_name: groupName }))
      }} /> : null}

      {mediaPreview ? (
        <MediaPreviewDialog
          asset={mediaPreview}
          projects={projects.projects}
          creationActions={mediaCreationActions(mediaPreview)}
          onClose={() => setMediaPreview(null)}
          onChanged={patchMediaPreviewAsset}
          onDeleted={(asset) => { removeAssetLocally(asset); setMediaPreview(null) }}
          onContinue={(options) => { setMediaPreview(null); continueCreation(options) }}
        />
      ) : null}
    </main>
  )
}