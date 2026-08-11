import { useEffect, useMemo, useRef, useState } from 'react'
import type { MouseEvent, PointerEvent as ReactPointerEvent, ReactNode } from 'react'
import type { GalleryImage, ImageTaskStatus, ImageTaskType, PublishStatus } from '../../../shared/api-types'
import { userApi } from '../../../shared/user-api'
import { cn } from '../../../shared/classnames'
import { Button, EmptyState, ErrorState, GalleryFilterToolbar, GalleryImageFrame, ImageDetailModal, Modal, PublicDetailIcon, StatusPill, copyText, useApp } from '../components'
import { errorMessage } from '../useApiResource'
import { mediaAccess } from '../mediaAccess'
import { userForm, userState } from '../ui/classes'
import { rdGallery } from '../ui/redesign-classes'
import { Check, Copy, Download, Edit, FolderPlus, Globe, RotateCcw, Trash2, X } from '../ui/icons'
import { OverlayPortal } from '../ui/overlayPortal'
import { RefreshCw } from 'lucide-react'
import { stageWorkspaceCreationDraft, workspaceCreationDraftFromSnapshot } from './workspaceCreationDraft'
import { invertLoadedGallerySelection, pollGalleryExportJob, reconcileGalleryBatchSelection } from './galleryBatchActions'
import { areAllVisibleGalleryItemsSelected, galleryImageAspect, galleryMarqueeSelection, gallerySelectionClickAction, gallerySelectionDragDistance, gallerySelectionRectangle, pruneGallerySelection, selectVisibleGalleryImages, selectedVisibleGalleryItems, toggleGalleryImageSelection, type GallerySelectionPoint, type GallerySelectionRect } from './galleryExperience'
import { applyGalleryPage, initialGalleryPageState, patchGalleryPageItems, removeGalleryPageItems } from './galleryPagination'
import { filterGalleryImages, galleryImageCard, galleryPublishActionPresentation, galleryPublishStatus, type GalleryPublishActionPresentation } from './galleryRows'
import { ProjectSelector, useProjects } from '../ProjectContext'

const GALLERY_PAGE_SIZE = 50

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === 'AbortError'
}

const galleryClasses = {
  content: 'w-full flex-1 p-6 md:p-10',
  header: 'mb-8 flex flex-col items-start justify-between gap-6 md:flex-row md:items-end',
  title: 'm-0 text-4xl font-black text-[var(--fg)] md:text-6xl',
  filterGroup: rdGallery.filterGroup,
  filterSelect: rdGallery.filterSelectWrap,
  filterTrigger: rdGallery.filterSelectBtn,
  filterLabel: 'sr-only',
  filterValue: 'overflow-hidden text-ellipsis whitespace-nowrap',
  filterMenu: cn(rdGallery.filterSelectDropdown, 'max-h-64 overflow-auto'),
  filterOption: rdGallery.filterOption,
  filterOptionActive: rdGallery.filterOptionActive,
  filterToolbar: 'mb-8',
  batchBar: cn(rdGallery.batchBar, 'w-max max-w-[calc(100vw-24px)] max-md:bottom-20 overflow-x-auto rounded-md'),
  selectCheck: 'inline-flex items-center gap-2 text-sm text-[var(--fg)]',
  batchSelectAll: 'flex items-center gap-1.5 rounded-xl px-4 py-2 text-xs font-medium text-[var(--fg)] transition-colors hover:bg-[var(--surface)] hover:text-[var(--accent)]',
  batchSelectAllActive: 'bg-[var(--accent)]/12 text-[var(--accent)] ring-1 ring-[var(--accent)]/35',
  batchSpacer: 'min-w-0 flex-1',
  batchBtn: rdGallery.batchBtn,
  grid: rdGallery.masonry,
  card: 'group/asset mb-8 block w-full break-inside-avoid',
  assetSelectHitArea: 'group/select grid size-10 place-items-center rounded-lg border-0 bg-transparent p-0 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]',
  assetSelectVisual: 'grid size-[22px] place-items-center rounded-md border border-[var(--image-action-border)] bg-[var(--image-action-bg)] text-[var(--image-action-text)] opacity-80 shadow-sm backdrop-blur transition-[opacity,transform,background-color,border-color,color] duration-200 group-hover/asset:opacity-100 group-hover/select:opacity-100 group-focus-visible/select:opacity-100 group-active/select:scale-90 motion-reduce:transition-none [&_svg]:size-3.5',
  assetSelectVisualSelected: 'border-[var(--fg)] bg-[var(--fg)] text-[var(--bg)] opacity-100 ring-2 ring-[var(--bg)] shadow-md',
  thumbImage: 'object-cover',
  info: 'grid gap-2 pt-3',
  titleLine: rdGallery.itemTitle,
  metaLine: rdGallery.itemMeta,
  groupLabel: 'inline-flex items-center rounded-full border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_7%,transparent)] px-2.5 py-1 font-vault-mono text-[10px] text-[var(--muted)]',
  iconActions: 'flex flex-wrap items-center justify-end gap-1 rounded-xl border border-[var(--image-action-border)] bg-[var(--image-action-bg)] p-1 backdrop-blur',
  iconButton: 'grid size-10 place-items-center rounded-lg p-1 text-[var(--image-action-text)] transition-colors hover:bg-[var(--image-action-hover-bg)] hover:text-[var(--image-action-hover-text)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-[var(--focus-ring)] active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-45 [&_svg]:size-4',
  iconButtonPositive: 'hover:border-[var(--accent-emerald)] hover:bg-[color-mix(in_oklch,var(--accent-emerald)_12%,transparent)] hover:text-[var(--accent-emerald)]',
  iconButtonWarning: 'hover:border-[var(--pg-accent-amber)] hover:bg-[color-mix(in_oklch,var(--pg-accent-amber)_12%,transparent)] hover:text-[var(--pg-accent-amber)]',
  iconButtonDanger: 'hover:border-[var(--accent-coral)] hover:bg-[color-mix(in_oklch,var(--accent-coral)_12%,transparent)] hover:text-[var(--accent-coral)]',
  publishConfirm: 'grid min-w-0 gap-5',
  publishConfirmBody: 'grid min-w-0 grid-cols-[42px_minmax(0,1fr)] items-start gap-4 max-[420px]:grid-cols-1',
  publishConfirmMark: 'grid size-[42px] place-items-center rounded-xl border border-[color-mix(in_oklch,var(--pg-accent-amber)_42%,var(--border))] bg-[color-mix(in_oklch,var(--pg-accent-amber)_14%,transparent)] text-[var(--pg-accent-amber)]',
  publishConfirmMarkDanger: 'border-[color-mix(in_oklch,var(--accent-coral)_42%,var(--border))] bg-[color-mix(in_oklch,var(--accent-coral)_14%,transparent)] text-[var(--accent-coral)]',
  publishConfirmCopy: 'min-w-0',
  publishConfirmActions: 'flex justify-end gap-2 max-[420px]:flex-col max-[420px]:items-stretch',
  deleteConfirm: 'grid grid-cols-[42px_minmax(0,1fr)] items-start gap-4',
  deleteMark: 'grid size-[42px] place-items-center rounded-xl border border-[color-mix(in_oklch,var(--accent-coral)_42%,var(--border))] bg-[color-mix(in_oklch,var(--accent-coral)_14%,transparent)] text-[var(--accent-coral)]',
  deleteTitle: 'm-0 mb-2 text-xl',
  deleteText: 'm-0 leading-[1.65] text-[var(--muted)]',
  deleteList: 'col-span-full flex flex-wrap gap-2 rounded-xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] p-3',
  deleteListItem: 'max-w-full overflow-hidden text-ellipsis whitespace-nowrap rounded-full bg-[color-mix(in_oklch,var(--fg)_7%,transparent)] px-2 py-1 text-xs text-[var(--muted)]',
  deleteActions: 'col-span-full flex justify-end gap-2 max-[420px]:flex-col max-[420px]:items-stretch',
  groupEditor: 'grid gap-4',
  groupEditorLabel: 'grid gap-2 text-sm text-[var(--muted)]',
  groupText: 'm-0 text-[var(--muted)]',
  groupActions: 'flex justify-end gap-2 max-[420px]:flex-col max-[420px]:items-stretch',
}

const typeFilters: Array<{ value: 'all' | ImageTaskType | 'api'; label: string }> = [
  { value: 'all', label: '全部类型' },
  { value: 'text_to_image', label: '文生图' },
  { value: 'image_edit', label: '图片编辑' },
  { value: 'api', label: 'API 调用' },
]

const statusFilters: Array<{ value: 'all' | ImageTaskStatus; label: string }> = [
  { value: 'all', label: '全部状态' },
  { value: 'succeeded', label: '已完成' },
  { value: 'running', label: '生成中' },
  { value: 'queued', label: '排队中' },
  { value: 'failed', label: '失败' },
]

const publishFilters: Array<{ value: 'all' | PublishStatus; label: string }> = [
  { value: 'all', label: '全部公开状态' },
  { value: 'private', label: '私有' },
  { value: 'pending_review', label: '审核中' },
  { value: 'approved', label: '已公开' },
  { value: 'rejected', label: '已拒绝' },
  { value: 'unpublished', label: '已下架' },
]

type FilterOption<T extends string> = {
  value: T
  label: string
}

function FilterSelect<T extends string>({ label, value, options, onChange }: {
  label: string
  value: T
  options: Array<FilterOption<T>>
  onChange: (value: T) => void
}) {
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement | null>(null)
  const current = options.find((item) => item.value === value) ?? options[0]

  useEffect(() => {
    if (!open) return undefined
    const close = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false)
    }
    window.addEventListener('pointerdown', close)
    return () => window.removeEventListener('pointerdown', close)
  }, [open])

  return (
    <div className={galleryClasses.filterSelect} ref={rootRef}>
      <button type="button" className={galleryClasses.filterTrigger} aria-haspopup="listbox" aria-expanded={open} onClick={() => setOpen((next) => !next)}>
        <span className="grid min-w-0 gap-0.5">
          <span className={galleryClasses.filterLabel}>{label}</span>
          <span className={galleryClasses.filterValue}>{current?.label ?? value}</span>
        </span>
        <span aria-hidden="true">⌄</span>
      </button>
      {open ? (
        <div className={galleryClasses.filterMenu} role="listbox" aria-label={label}>
          {options.map((option) => (
            <button
              key={option.value}
              type="button"
              role="option"
              aria-selected={option.value === value}
              className={galleryClasses.filterOption}
              onClick={() => {
                onChange(option.value)
                setOpen(false)
              }}
            >
              {option.label}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  )
}

function ActionIcon({ name }: { name: 'download' | 'public' | 'delete' | 'edit' | 'copy' | 'group' }) {
  const props = { size: 14, strokeWidth: 1.5 } as const
  if (name === 'download') return <Download {...props} />
  if (name === 'public') return <Globe {...props} />
  if (name === 'delete') return <Trash2 {...props} />
  if (name === 'edit') return <Edit {...props} />
  if (name === 'copy') return <Copy {...props} />
  return <FolderPlus {...props} />
}

function PublishActionIcon({ icon }: { icon: GalleryPublishActionPresentation['icon'] }) {
  const props = { size: 14, strokeWidth: 1.5 } as const
  if (icon === 'withdraw') return <RotateCcw {...props} />
  if (icon === 'unpublish') return <X {...props} />
  return <Globe {...props} />
}

function publishActionPresentation(image: GalleryImage) {
  return galleryPublishActionPresentation(galleryPublishStatus(image), Boolean(image.url || image.download_url))
}

function publishActionDetailTone(tone: GalleryPublishActionPresentation['tone']) {
  if (tone === 'danger') return 'danger'
  if (tone === 'warning') return 'hover:border-[var(--pg-accent-amber)] hover:bg-[color-mix(in_oklch,var(--pg-accent-amber)_12%,transparent)] hover:text-[var(--pg-accent-amber)]'
  return 'hover:border-[var(--accent-emerald)] hover:bg-[color-mix(in_oklch,var(--accent-emerald)_12%,transparent)] hover:text-[var(--accent-emerald)]'
}

function iconButton(label: string, icon: ReactNode, onClick: () => void, disabled?: boolean, busy?: boolean, tone?: GalleryPublishActionPresentation['tone'] | 'danger') {
  return (
    <button
      type="button"
      className={cn(
        galleryClasses.iconButton,
        tone === 'positive' && galleryClasses.iconButtonPositive,
        tone === 'warning' && galleryClasses.iconButtonWarning,
        tone === 'danger' && galleryClasses.iconButtonDanger,
      )}
      title={label}
      aria-label={label}
      disabled={disabled || busy}
      onClick={(event: MouseEvent<HTMLButtonElement>) => {
        event.stopPropagation()
        onClick()
      }}
    >
      {busy ? <span className={userState.spinner} /> : icon}
    </button>
  )
}

export function GalleryPage() {
  const app = useApp()
  const projectContext = useProjects()
  const { selectedProjectID } = projectContext
  const [galleryPage, setGalleryPage] = useState(() => initialGalleryPageState<GalleryImage>())
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [loadError, setLoadError] = useState('')
  const loadGenerationRef = useRef(0)
  const loadMoreRef = useRef<HTMLDivElement | null>(null)
  const [query, setQuery] = useState('')
  const [type, setType] = useState<(typeof typeFilters)[number]['value']>('all')
  const [status, setStatus] = useState<(typeof statusFilters)[number]['value']>('all')
  const [publishStatus, setPublishStatus] = useState<(typeof publishFilters)[number]['value']>('all')
  const [imageGroup, setImageGroup] = useState('all')
  const [selected, setSelected] = useState<GalleryImage | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(() => new Set())
  const [groupDialog, setGroupDialog] = useState<{ ids: string[] } | null>(null)
  const [groupDraft, setGroupDraft] = useState('')
	const [deleteDialog, setDeleteDialog] = useState<{ images: GalleryImage[] } | null>(null)
  const [publishDialog, setPublishDialog] = useState<{ image: GalleryImage; label: string } | null>(null)
		const [transferDialog, setTransferDialog] = useState<{ ids: string[]; targetProjectID: string } | null>(null)
		const exportAbortRef = useRef<AbortController | null>(null)
	const selectionSurfaceRef = useRef<HTMLDivElement | null>(null)
	const suppressGalleryOpenRef = useRef(false)
	const marqueeFrameRef = useRef<number | null>(null)
	const marqueeDragRef = useRef<{ pointerID: number; start: GallerySelectionPoint; current: GallerySelectionPoint; additive: boolean; dragged: boolean; baseSelection: Set<string>; targetImageID: string } | null>(null)
	const [marquee, setMarquee] = useState<GallerySelectionRect | null>(null)

	useEffect(() => () => {
	  const controller = exportAbortRef.current
	  if (controller) controller.abort()
	  if (marqueeFrameRef.current !== null) window.cancelAnimationFrame(marqueeFrameRef.current)
	}, [])

	useEffect(() => {
	  const clearSelection = (event: KeyboardEvent) => {
	    if (event.key !== 'Escape') return
	    const drag = marqueeDragRef.current
	    const surface = selectionSurfaceRef.current
	    if (drag && surface?.hasPointerCapture(drag.pointerID)) surface.releasePointerCapture(drag.pointerID)
	    marqueeDragRef.current = null
	    if (marqueeFrameRef.current !== null) {
	      window.cancelAnimationFrame(marqueeFrameRef.current)
	      marqueeFrameRef.current = null
	    }
	    setMarquee(null)
	    setSelectedIds(new Set())
	  }
	  window.addEventListener('keydown', clearSelection)
	  return () => window.removeEventListener('keydown', clearSelection)
	}, [])

  async function loadPage(pageNumber: number, mode: 'replace' | 'append') {
    const generation = ++loadGenerationRef.current
    if (!selectedProjectID) {
      setGalleryPage(initialGalleryPageState<GalleryImage>())
      setLoading(false)
      setLoadingMore(false)
      return
    }
    if (mode === 'replace') setLoading(true)
    else setLoadingMore(true)
    setLoadError('')
    try {
      const incoming = await userApi.listGalleryImages(pageNumber, GALLERY_PAGE_SIZE, selectedProjectID)
      if (generation !== loadGenerationRef.current) return
      setGalleryPage((current) => applyGalleryPage(current, incoming, {
        page: pageNumber,
        pageSize: GALLERY_PAGE_SIZE,
        mode,
      }))
    } catch (err) {
      if (generation === loadGenerationRef.current) {
        const message = errorMessage(err)
        setLoadError(message)
        app.notify('error', message)
      }
    } finally {
      if (generation === loadGenerationRef.current) {
        setLoading(false)
        setLoadingMore(false)
      }
    }
  }

  useEffect(() => {
    setGalleryPage(initialGalleryPageState<GalleryImage>())
    setSelectedIds(new Set())
    setSelected(null)
    setGroupDialog(null)
    setDeleteDialog(null)
    setPublishDialog(null)
	setTransferDialog(null)
    void loadPage(1, 'replace')
    return () => { loadGenerationRef.current += 1 }
  }, [selectedProjectID])

  useEffect(() => {
    if (!galleryPage.hasMore || loading || loadingMore || loadError) return undefined
    const target = loadMoreRef.current
    if (!target || typeof IntersectionObserver === 'undefined') return undefined
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) void loadPage(galleryPage.page + 1, 'append')
    }, { rootMargin: '320px 0px' })
    observer.observe(target)
    return () => observer.disconnect()
  }, [galleryPage.hasMore, galleryPage.page, loadError, loading, loadingMore])

  useEffect(() => {
    setImageGroup('all')
  }, [type])

  const rows = galleryPage.items
  const typeRows = useMemo(() => filterGalleryImages(rows, { type, status: 'all', publishStatus: 'all', imageGroup: 'all', query: '' }), [rows, type])

  const groupFilters = useMemo(() => {
    const groups = new Set<string>()
    typeRows.forEach((image) => {
      const group = image.image_group?.trim()
      if (group) groups.add(group)
    })
    return Array.from(groups).sort()
  }, [typeRows])

  const allGroupFilters = useMemo(() => {
    const groups = new Set<string>()
    ;galleryPage.items.forEach((image) => {
      const group = image.image_group?.trim()
      if (group) groups.add(group)
    })
    return Array.from(groups).sort()
  }, [galleryPage.items])

	const filtered = useMemo(() => filterGalleryImages(typeRows, { type: 'all', status, publishStatus, imageGroup, query }), [typeRows, query, status, publishStatus, imageGroup])
	const filteredIDs = useMemo(() => filtered.map((image) => image.id), [filtered])

	useEffect(() => {
		setSelectedIds((current) => pruneGallerySelection(current, filteredIDs))
	}, [filteredIDs])

	const selectedImages = useMemo(() => selectedVisibleGalleryItems(filtered, selectedIds), [filtered, selectedIds])
  const allVisibleSelected = useMemo(() => areAllVisibleGalleryItemsSelected(filtered, selectedIds), [filtered, selectedIds])

  function patchImages(updates: GalleryImage[]) {
    if (!updates.length) return
    setGalleryPage((current) => patchGalleryPageItems(current, updates))
    const updatesByID = new Map(updates.map((image) => [image.id, image]))
    setSelected((current) => {
      if (!current) return current
      const update = updatesByID.get(current.id)
      return update ? { ...current, ...update } : current
    })
  }

  async function publishImage(image: GalleryImage) {
	setBusyId(image.id)
	try {
      const updated = await userApi.publishImage(image.id)
      patchImages([updated])
      app.notify('success', '已提交公开审核')
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusyId(null)
    }
  }

  function handlePublishAction(image: GalleryImage) {
    const card = galleryImageCard(image)
    if (card.publishAction === 'cancel') {
      setPublishDialog({ image, label: card.publishActionLabel })
      return
    }
    if (card.publishAction === 'request') void publishImage(image)
  }

  async function confirmCancelPublish() {
    if (!publishDialog) return
    setBusyId(publishDialog.image.id)
    try {
      const updated = await userApi.cancelImagePublish(publishDialog.image.id)
      patchImages([updated])
      setPublishDialog(null)
      app.notify('success', publishDialog.label === '取消公开' ? '已取消公开' : '已取消公开申请')
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusyId(null)
    }
  }

  async function publishImages(images: GalleryImage[], publish = true) {
    if (!images.length) return
    setBusyId('batch')
    try {
	  const candidates = images.filter((image) => publish ? galleryImageCard(image).publishAction === 'request' : galleryImageCard(image).publishAction === 'cancel')
	  if (!candidates.length) {
		app.notify('error', publish ? '所选图片中没有可申请公开的项目' : '所选图片中没有可取消公开的项目')
        return
      }
	  const result = await userApi.batchPublishGalleryImages(candidates.map((image) => image.id), selectedProjectID, publish)
	  patchImages(result.succeeded.map(({ entity }) => entity))
	  reconcileServerBatchSelection(result)
	  reportGalleryBatchResult(publish ? '提交公开审核' : '取消公开', result.succeeded.length, result.failed.length)
	} catch (err) {
	  app.notify('error', errorMessage(err))
    } finally {
      setBusyId(null)
    }
  }

  function requestDeleteImages(images: GalleryImage[]) {
    if (!images.length) return
    setDeleteDialog({ images })
  }

  async function confirmDeleteImages() {
    const images = deleteDialog?.images ?? []
    if (!images.length) return
    setBusyId(images.length === 1 ? images[0].id : 'batch')
    try {
	  const result = await userApi.batchDeleteGalleryImages(images.map((image) => image.id), selectedProjectID)
	  const succeeded = new Set(result.succeeded.map(({ id }) => id))
      setGalleryPage((current) => removeGalleryPageItems(current, succeeded))
	  reconcileServerBatchSelection(result)
      setSelected((current) => current && succeeded.has(current.id) ? null : current)
	  const failedIDs = new Set(result.failed.map(({ id }) => id))
	  setDeleteDialog(result.failed.length ? { images: images.filter((image) => failedIDs.has(image.id)) } : null)
      reportGalleryBatchResult('永久删除', result.succeeded.length, result.failed.length)
	} catch (err) {
	  app.notify('error', errorMessage(err))
    } finally {
      setBusyId(null)
    }
  }

  function reuseConfiguration(image: GalleryImage) {
    stageWorkspaceCreationDraft(workspaceCreationDraftFromSnapshot(image), window.sessionStorage, window.history)
    app.navigate('genpic')
  }

  async function refreshPrivateImage(imageId: string) {
    const projection = await mediaAccess.preview({ kind: 'image', scope: 'private', id: imageId })
    const refreshedURL = userApi.imageAssetUrl(projection.url, app.session?.token)
    setGalleryPage((current) => ({
      ...current,
      items: current.items.map((image) => image.id === imageId ? { ...image, url: refreshedURL, download_url: refreshedURL, preview_expires_at: projection.expires_at } : image),
    }))
    setSelected((current) => current?.id === imageId ? { ...current, url: refreshedURL, download_url: refreshedURL, preview_expires_at: projection.expires_at } : current)
    return refreshedURL
  }

  async function refreshReferenceAsset(assetId: string) {
    const projection = await mediaAccess.preview({ kind: 'reference', scope: 'private', id: assetId })
    const refreshedURL = userApi.imageAssetUrl(projection.url, app.session?.token)
    setSelected((current) => current ? {
      ...current,
      reference_assets: current.reference_assets?.map((asset) => asset.id === assetId ? { ...asset, preview_url: refreshedURL, download_url: refreshedURL, preview_expires_at: projection.expires_at } : asset),
    } : current)
    return refreshedURL
  }

  async function downloadImage(image?: Pick<GalleryImage, 'url' | 'download_url' | 'id'>) {
    if (!image) return
    try {
      const projection = await mediaAccess.download({ kind: 'image', scope: 'private', id: image.id })
      const link = document.createElement('a')
      link.href = userApi.imageAssetUrl(projection.url, app.session?.token)
      link.download = downloadFilename(image)
      link.rel = 'noopener noreferrer'
      document.body.appendChild(link)
      link.click()
      link.remove()
    } catch (error) {
      app.notify('error', errorMessage(error))
    }
  }

  function assetUrl(url: string) {
    return userApi.imageAssetUrl(url, app.session?.token)
  }

  async function downloadImages(images: GalleryImage[]) {
	if (!images.length) return
	const previousController = exportAbortRef.current
	if (previousController) previousController.abort()
	const controller = new AbortController()
	exportAbortRef.current = controller
	setBusyId('batch')
	try {
	  const response = await userApi.batchDownloadGalleryImages(images.map((image) => image.id), selectedProjectID)
	  let archive: Blob
	  if (response instanceof Blob) {
		archive = response
	  } else {
		app.notify('success', '打包任务已创建')
		const status = await pollGalleryExportJob(response, userApi.getGalleryExportJob, { signal: controller.signal })
		controller.signal.throwIfAborted()
		archive = await userApi.downloadGalleryExport(status.job.id, controller.signal)
	  }
	  const url = URL.createObjectURL(archive)
	  const link = document.createElement('a')
	  link.href = url
	  link.download = 'gallery-assets.zip'
	  document.body.appendChild(link)
	  link.click()
	  link.remove()
	  URL.revokeObjectURL(url)
	  app.notify('success', `已打包 ${images.length} 个已加载资产`)
	} catch (err) {
	  if (!isAbortError(err)) app.notify('error', errorMessage(err))
	} finally {
	  if (exportAbortRef.current === controller) {
		exportAbortRef.current = null
		setBusyId(null)
	  }
	}
  }

  function downloadFilename(image: Pick<GalleryImage, 'id' | 'url' | 'download_url'>) {
    const source = image.download_url ?? image.url ?? ''
    const clean = source.split('?')[0]
    const ext = clean.match(/\.(png|jpe?g|webp|gif)$/i)?.[0] ?? '.png'
    return `${image.id || 'image'}${ext}`
  }

  function toggleSelected(imageID: string, checked?: boolean) {
    setSelectedIds((current) => toggleGalleryImageSelection(current, imageID, checked))
  }

	function beginMarqueeSelection(event: ReactPointerEvent<HTMLDivElement>) {
	  if ((event.pointerType !== 'mouse' && event.pointerType !== 'pen') || event.button !== 0) return
	  const target = event.target as HTMLElement
	  if (target.closest('[data-gallery-selection-control],[data-gallery-card-actions]')) return
	  const interactive = target.closest<HTMLElement>('button,a,input,select,textarea,[role="button"]')
	  if (interactive && !interactive.matches('[data-gallery-card-open]')) return
	  event.preventDefault()
	  const point = { x: event.clientX, y: event.clientY }
	  marqueeDragRef.current = {
	    pointerID: event.pointerId,
	    start: point,
	    current: point,
	    additive: event.metaKey || event.ctrlKey || event.shiftKey,
	    dragged: false,
	    baseSelection: new Set(selectedIds),
	    targetImageID: target.closest<HTMLElement>('[data-gallery-image-id]')?.dataset.galleryImageId ?? '',
	  }
	  event.currentTarget.setPointerCapture(event.pointerId)
	}

	function marqueeSelectionItems() {
	  return Array.from(selectionSurfaceRef.current?.querySelectorAll<HTMLElement>('[data-gallery-image-id]') ?? []).map((element) => {
	    const bounds = element.getBoundingClientRect()
	    return { id: element.dataset.galleryImageId ?? '', rect: { left: bounds.left, top: bounds.top, right: bounds.right, bottom: bounds.bottom } }
	  }).filter((item) => item.id)
	}

	function galleryScrollContainer() {
	  let element = selectionSurfaceRef.current?.parentElement ?? null
	  while (element) {
	    const overflowY = window.getComputedStyle(element).overflowY
	    if ((overflowY === 'auto' || overflowY === 'scroll') && element.scrollHeight > element.clientHeight) return element
	    element = element.parentElement
	  }
	  return document.scrollingElement instanceof HTMLElement ? document.scrollingElement : document.documentElement
	}

	function scheduleMarqueeFrame() {
	  if (marqueeFrameRef.current !== null) return
	  marqueeFrameRef.current = window.requestAnimationFrame(() => {
	    marqueeFrameRef.current = null
	    const drag = marqueeDragRef.current
	    if (!drag?.dragged) return
	    const rectangle = gallerySelectionRectangle(drag.start, drag.current)
	    setMarquee(rectangle)
	    setSelectedIds(galleryMarqueeSelection(drag.baseSelection, marqueeSelectionItems(), rectangle, drag.additive))

	    const scrollContainer = galleryScrollContainer()
	    const scrollBounds = scrollContainer.getBoundingClientRect()
	    const documentScroller = scrollContainer === document.documentElement || scrollContainer === document.body
	    const scrollTopEdge = documentScroller ? 0 : scrollBounds.top
	    const scrollBottomEdge = documentScroller ? window.innerHeight : scrollBounds.bottom
	    const maxScrollTop = Math.max(0, scrollContainer.scrollHeight - scrollContainer.clientHeight)
	    let keepScrolling = false
	    if (drag.current.y < scrollTopEdge + 56 && scrollContainer.scrollTop > 0) {
	      scrollContainer.scrollBy({ top: -12 })
	      keepScrolling = true
	    } else if (drag.current.y > scrollBottomEdge - 56 && scrollContainer.scrollTop < maxScrollTop) {
	      scrollContainer.scrollBy({ top: 12 })
	      keepScrolling = true
	    }
	    if (keepScrolling) scheduleMarqueeFrame()
	  })
	}

	function moveMarqueeSelection(event: ReactPointerEvent<HTMLDivElement>) {
	  const drag = marqueeDragRef.current
	  if (!drag || drag.pointerID !== event.pointerId) return
	  drag.current = { x: event.clientX, y: event.clientY }
	  if (!drag.dragged && gallerySelectionDragDistance(drag.start, drag.current) < 6) return
	  if (!drag.dragged) {
	    drag.dragged = true
	  }
	  event.preventDefault()
	  scheduleMarqueeFrame()
	}

	function finishMarqueeSelection(event: ReactPointerEvent<HTMLDivElement>) {
	  const drag = marqueeDragRef.current
	  if (!drag || drag.pointerID !== event.pointerId) return
	  if (marqueeFrameRef.current !== null) {
	    window.cancelAnimationFrame(marqueeFrameRef.current)
	    marqueeFrameRef.current = null
	  }
	  if (drag.dragged) {
	    const rectangle = gallerySelectionRectangle(drag.start, drag.current)
	    setSelectedIds(galleryMarqueeSelection(drag.baseSelection, marqueeSelectionItems(), rectangle, drag.additive))
	    suppressGalleryOpenRef.current = true
	    window.setTimeout(() => { suppressGalleryOpenRef.current = false }, 0)
	  } else if (drag.targetImageID) {
	    const image = filtered.find((item) => item.id === drag.targetImageID)
	    if (image) {
	      suppressGalleryOpenRef.current = true
	      if (gallerySelectionClickAction(selectedIds.size) === 'toggle') toggleSelected(image.id)
	      else setSelected(image)
	      window.setTimeout(() => { suppressGalleryOpenRef.current = false }, 0)
	    }
	  }
	  if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
	  marqueeDragRef.current = null
	  setMarquee(null)
	}

	function cancelMarqueeSelection(event: ReactPointerEvent<HTMLDivElement>) {
	  if (marqueeDragRef.current?.pointerID !== event.pointerId) return
	  if (marqueeFrameRef.current !== null) {
	    window.cancelAnimationFrame(marqueeFrameRef.current)
	    marqueeFrameRef.current = null
	  }
	  marqueeDragRef.current = null
	  setMarquee(null)
	}

  function selectAllVisible(checked: boolean) {
		setSelectedIds((current) => selectVisibleGalleryImages(current, filteredIDs, checked))
  }

  function invertLoadedSelection() {
	setSelectedIds((current) => invertLoadedGallerySelection(current, filteredIDs))
  }

  function openGroupDialog(images: GalleryImage[]) {
    if (!images.length) return
    setGroupDialog({ ids: images.map((image) => image.id) })
    setGroupDraft(images.length === 1 ? images[0].image_group ?? '' : '')
  }

  async function applyGroup() {
    const name = groupDraft.trim()
    if (!groupDialog) return
    setBusyId('group')
    try {
	  const result = await userApi.batchGroupGalleryImages(groupDialog.ids, selectedProjectID, name)
	  patchImages(result.succeeded.map(({ entity }) => entity))
	  reconcileServerBatchSelection(result)
	  setGroupDialog(result.failed.length ? { ids: result.failed.map(({ id }) => id) } : null)
      if (!result.failed.length) setGroupDraft('')
      reportGalleryBatchResult(name ? '设置图片分组' : '清除图片分组', result.succeeded.length, result.failed.length)
	} catch (err) {
	  app.notify('error', errorMessage(err))
    } finally {
      setBusyId(null)
    }
  }

  function clearSucceededSelection(succeeded: ReadonlySet<string>) {
    setSelectedIds((current) => new Set(Array.from(current).filter((id) => !succeeded.has(id))))
  }

  function reconcileServerBatchSelection(result: { succeeded: Array<{ id: string }>; failed: Array<{ id: string }> }) {
	setSelectedIds((current) => reconcileGalleryBatchSelection(current, result.succeeded.map(({ id }) => id), result.failed.map(({ id }) => id)))
  }

  function openTransferDialog(images: GalleryImage[]) {
	const target = projectContext.projects.find((project) => project.id !== selectedProjectID)
	if (!target) {
	  app.notify('error', '暂无可转移的目标项目')
	  return
	}
	setTransferDialog({ ids: images.map((image) => image.id), targetProjectID: target.id })
  }

  async function applyProjectTransfer() {
	if (!transferDialog) return
	setBusyId('transfer')
	try {
	  const result = await userApi.batchTransferGalleryImages(transferDialog.ids, selectedProjectID, transferDialog.targetProjectID)
	  const succeeded = new Set(result.succeeded.map(({ id }) => id))
	  setGalleryPage((current) => removeGalleryPageItems(current, succeeded))
	  reconcileServerBatchSelection(result)
	  setSelected((current) => current && succeeded.has(current.id) ? null : current)
	  setTransferDialog(result.failed.length ? { ...transferDialog, ids: result.failed.map(({ id }) => id) } : null)
	  reportGalleryBatchResult('批量转移项目', result.succeeded.length, result.failed.length)
	} catch (err) {
	  app.notify('error', errorMessage(err))
	} finally {
	  setBusyId(null)
	}
  }

  function reportGalleryBatchResult(action: string, succeeded: number, failed: number) {
    if (!failed) {
      app.notify('success', `${action}成功 ${succeeded} 项`)
      return
    }
    if (!succeeded) {
      app.notify('error', `${action}失败 ${failed} 项，失败项已保留，可重试`)
      return
    }
    app.notify('error', `${action}成功 ${succeeded} 项，失败 ${failed} 项；失败项已保留，可重试`)
  }

  return (
    <div className={galleryClasses.content}>
      <div className={galleryClasses.header}>
        <div>
          <h1 className={galleryClasses.title}>历史资产</h1>
          <p className="mb-0 mt-3 max-w-[56ch] text-sm leading-6 text-[var(--muted)]">筛选、分组和重用已生成的图片，每一张资产都保留原始参数与公开状态。</p>
        </div>
        <div className="flex w-full items-end justify-end gap-2 sm:w-auto">
          <button type="button" className="grid size-10 shrink-0 place-items-center rounded-md border border-[var(--border)] bg-[var(--surface)] text-[var(--muted)] hover:border-[var(--accent)] hover:text-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-45" title="刷新资产" aria-label="刷新资产" disabled={loading} onClick={() => void loadPage(1, 'replace')}>
            <RefreshCw className={cn('size-[18px]', loading && 'animate-spin')} aria-hidden="true" />
          </button>
          <ProjectSelector className="min-w-0 flex-1 sm:w-auto" />
        </div>
      </div>

      <div className={galleryClasses.filterToolbar}>
        <GalleryFilterToolbar
          label="历史资产筛选"
          query={query}
          queryPlaceholder="搜索标题、提示词或模型"
          onQueryChange={setQuery}
          filters={<div className={galleryClasses.filterGroup}>
            <FilterSelect label="类型" value={type} options={typeFilters} onChange={setType} />
            <FilterSelect
              label="分组"
              value={imageGroup}
              options={[
                { value: 'all', label: '全部分组' },
                { value: 'ungrouped', label: '未分组' },
                ...groupFilters.map((group) => ({ value: group, label: group })),
              ]}
              onChange={setImageGroup}
            />
            <FilterSelect label="任务状态" value={status} options={statusFilters} onChange={setStatus} />
            <FilterSelect label="公开状态" value={publishStatus} options={publishFilters} onChange={setPublishStatus} />
          </div>}
          meta={`共 ${filtered.length} 个结果`}
        />
      </div>

          {loading && !rows.length ? <GalleryGridSkeleton /> : null}
          {loadError && !rows.length ? <ErrorState message={loadError} onRetry={() => void loadPage(1, 'replace')} /> : null}
          {!loading && !loadError && !filtered.length ? <EmptyState title="暂无资产" detail="换一个筛选条件，或回工作台创建新任务。" action={<Button onClick={() => app.navigate('genpic')}>继续生成</Button>} /> : null}

          {selectedImages.length ? (
            <OverlayPortal>
              <div className={galleryClasses.batchBar}>
                <div className={cn(rdGallery.batchCount, 'shrink-0')}>已选择 {selectedImages.length} 个已加载资产</div>
                <div className="flex shrink-0 items-center gap-1 pl-2">
                  <button
                    className={cn(galleryClasses.batchSelectAll, allVisibleSelected && galleryClasses.batchSelectAllActive)}
                    type="button"
                    aria-pressed={allVisibleSelected}
                    disabled={allVisibleSelected}
                    onClick={() => selectAllVisible(true)}
                  >
                    <span className={cn(rdGallery.itemCheckbox, allVisibleSelected && rdGallery.itemCheckboxChecked)}>
                      {allVisibleSelected ? '✓' : ''}
                    </span>
                    全选
                  </button>
                  <button className={galleryClasses.batchBtn} type="button" onClick={invertLoadedSelection}><RotateCcw /> 反选</button>
                  <button className={galleryClasses.batchBtn} type="button" onClick={() => setSelectedIds(new Set())}><X /> 清除选择</button>
                  <button className={galleryClasses.batchBtn} type="button" disabled={busyId === 'batch'} onClick={() => void downloadImages(selectedImages)}><ActionIcon name="download" /> 打包下载</button>
                  <button className={galleryClasses.batchBtn} type="button" disabled={busyId === 'batch'} onClick={() => void publishImages(selectedImages, true)}><ActionIcon name="public" /> 公开</button>
                  <button className={galleryClasses.batchBtn} type="button" disabled={busyId === 'batch'} onClick={() => void publishImages(selectedImages, false)}><X /> 取消公开</button>
                  <button className={galleryClasses.batchBtn} type="button" onClick={() => openGroupDialog(selectedImages)}><ActionIcon name="group" /> 设为分组</button>
                  <button className={galleryClasses.batchBtn} type="button" onClick={() => openTransferDialog(selectedImages)}><FolderPlus /> 批量转移项目</button>
                  <div className="mx-1 h-4 w-px bg-[var(--border)]" />
                  <button className={cn(galleryClasses.batchBtn, 'text-[var(--accent-coral)] hover:bg-[color-mix(in_oklch,var(--accent-coral)_10%,transparent)] hover:text-[var(--accent-coral)]')} type="button" disabled={busyId === 'batch'} onClick={() => requestDeleteImages(selectedImages)}><ActionIcon name="delete" /> 删除</button>
                </div>
              </div>
            </OverlayPortal>
          ) : null}

      <div
        ref={selectionSurfaceRef}
        data-gallery-selection-surface
        onPointerDown={beginMarqueeSelection}
        onPointerMove={moveMarqueeSelection}
        onPointerUp={finishMarqueeSelection}
        onPointerCancel={cancelMarqueeSelection}
      >
        <ImageGrid
          rows={filtered}
          accessToken={app.session?.token}
          busyId={busyId}
          selectedIds={selectedIds}
          onToggleSelected={toggleSelected}
          onOpen={(image) => {
            if (suppressGalleryOpenRef.current) return
            if (gallerySelectionClickAction(selectedIds.size) === 'toggle') toggleSelected(image.id)
            else setSelected(image)
          }}
          onCopyPrompt={async (image) => {
            await copyText(image.prompt || image.id)
            app.notify('success', '提示词已复制')
          }}
          onReuse={reuseConfiguration}
          onDownload={(image) => void downloadImage(image)}
				onPublish={handlePublishAction}
          onDelete={(image) => requestDeleteImages([image])}
          onGroup={(image) => openGroupDialog([image])}
          onMediaRefresh={refreshPrivateImage}
        />
      </div>
      {marquee ? <div className="gallery-selection-marquee" style={{ left: marquee.left, top: marquee.top, width: marquee.right - marquee.left, height: marquee.bottom - marquee.top }} aria-hidden="true" /> : null}

      <div ref={loadMoreRef} className="flex min-h-16 items-center justify-center py-4 text-sm text-[var(--muted)]" aria-live="polite">
        {loadError && rows.length ? (
          <Button tone="ghost" type="button" aria-label="加载更多资产" onClick={() => void loadPage(galleryPage.page + 1, 'append')}>重试加载更多</Button>
        ) : galleryPage.hasMore ? (
          <Button tone="ghost" type="button" aria-label="加载更多资产" busy={loadingMore} disabled={loadingMore} onClick={() => void loadPage(galleryPage.page + 1, 'append')}>加载更多资产</Button>
        ) : rows.length ? '已显示全部资产' : null}
      </div>

      <ImageDetailModal
        title="图片详情"
        image={selected}
        imageUrl={selected?.url || selected?.download_url ? assetUrl(selected?.url || selected?.download_url || '') : undefined}
        referenceImages={(selected?.reference_assets ?? []).filter((asset) => asset.preview_url).map((asset) => {
          const url = assetUrl(asset.preview_url || '')
          return { id: asset.id || asset.preview_url || url, url, alt: asset.name || '原图', mediaExpiresAt: asset.preview_expires_at, onMediaRefresh: asset.id ? () => refreshReferenceAsset(asset.id) : undefined }
        })}
        showPublicStats={false}
        onCopyPrompt={async (prompt) => {
          await copyText(prompt)
          app.notify('success', 'Prompt 已复制')
        }}
        actions={selected ? [
          { key: 'reuse', label: '复用配置', icon: <PublicDetailIcon name="edit" />, onClick: () => reuseConfiguration(selected) },
          { key: 'download', label: '下载图片', icon: <PublicDetailIcon name="download" />, onClick: () => downloadImage(selected), disabled: !selected.url && !selected.download_url },
          {
            key: 'public',
            label: publishActionPresentation(selected).label,
            icon: <PublishActionIcon icon={publishActionPresentation(selected).icon} />,
            onClick: () => handlePublishAction(selected),
            disabled: !galleryImageCard(selected).canPublish,
            tone: publishActionDetailTone(publishActionPresentation(selected).tone),
          },
          { key: 'group', label: '设置分组', icon: <PublicDetailIcon name="group" />, onClick: () => openGroupDialog([selected]) },
          { key: 'delete', label: '删除图片', icon: <PublicDetailIcon name="delete" />, onClick: () => requestDeleteImages([selected]), tone: 'danger' },
        ] : []}
        previewSourceLabel="历史资产"
        onMediaRefresh={selected ? () => refreshPrivateImage(selected.id) : undefined}
		onClose={() => setSelected(null)}
	/>
      {publishDialog ? (
        <Modal title={publishDialog.label} onClose={() => setPublishDialog(null)}>
          <div className={galleryClasses.publishConfirm}>
            <div className={galleryClasses.publishConfirmBody}>
              <div className={cn(galleryClasses.publishConfirmMark, publishDialog.label === '取消公开' && galleryClasses.publishConfirmMarkDanger)}>
                <PublishActionIcon icon={publishDialog.label === '取消公开' ? 'unpublish' : 'withdraw'} />
              </div>
              <div className={galleryClasses.publishConfirmCopy}>
                <h3 className={galleryClasses.deleteTitle}>确认{publishDialog.label}？</h3>
                <p className={galleryClasses.deleteText}>{publishDialog.label === '取消公开' ? '图片将立即从公开画廊移除，并恢复为私有状态。' : '图片将从审核队列移除，并恢复为私有状态。'}</p>
              </div>
            </div>
            <div className={galleryClasses.publishConfirmActions}>
              <Button tone="ghost" onClick={() => setPublishDialog(null)} disabled={busyId === publishDialog.image.id}>暂不取消</Button>
              <Button tone={publishDialog.label === '取消公开' ? 'danger' : 'primary'} busy={busyId === publishDialog.image.id} onClick={() => void confirmCancelPublish()}>
                {publishDialog.label === '取消公开' ? '确认取消公开' : '确认取消申请'}
              </Button>
            </div>
          </div>
        </Modal>
      ) : null}
      {deleteDialog ? (
        <Modal title="永久删除图片" onClose={() => setDeleteDialog(null)}>
          <div className={galleryClasses.deleteConfirm}>
            <div className={galleryClasses.deleteMark}><ActionIcon name="delete" /></div>
            <div>
              <h3 className={galleryClasses.deleteTitle}>确认删除 {deleteDialog.images.length} 张图片？</h3>
              <p className={galleryClasses.deleteText}>删除后会同步清理图片文件和数据库记录，无法恢复。公开审核中的图片也会从审核队列移除。</p>
            </div>
            <div className={galleryClasses.deleteList}>
              {deleteDialog.images.slice(0, 4).map((image) => (
                <span className={galleryClasses.deleteListItem} key={image.id}>{image.prompt || image.id}</span>
              ))}
              {deleteDialog.images.length > 4 ? <span className={galleryClasses.deleteListItem}>还有 {deleteDialog.images.length - 4} 张...</span> : null}
            </div>
            <div className={galleryClasses.deleteActions}>
              <Button tone="ghost" onClick={() => setDeleteDialog(null)} disabled={busyId === 'batch'}>取消</Button>
              <Button tone="danger" busy={busyId === 'batch' || busyId === deleteDialog.images[0]?.id} onClick={() => void confirmDeleteImages()}>确认删除</Button>
            </div>
          </div>
        </Modal>
      ) : null}
      {groupDialog ? (
        <Modal title="设置图片分组" onClose={() => setGroupDialog(null)}>
          <div className={galleryClasses.groupEditor}>
            <label className={galleryClasses.groupEditorLabel}>
              <span>分组名称</span>
              <input className={userForm.input} value={groupDraft} onChange={(event) => setGroupDraft(event.target.value)} placeholder="输入新分组，或选择已有分组" list="gallery-groups" autoFocus />
              <datalist id="gallery-groups">
                {allGroupFilters.map((group) => <option key={group} value={group} />)}
              </datalist>
            </label>
            <p className={galleryClasses.groupText}>留空保存会清除所选图片的分组。</p>
            <div className={galleryClasses.groupActions}>
              <Button tone="ghost" onClick={() => setGroupDialog(null)}>取消</Button>
              <Button busy={busyId === 'group'} onClick={() => void applyGroup()}>保存分组</Button>
            </div>
          </div>
        </Modal>
      ) : null}
	  {transferDialog ? (
		<Modal title="批量转移项目" onClose={() => setTransferDialog(null)}>
		  <div className={galleryClasses.groupEditor}>
			<label className={galleryClasses.groupEditorLabel}>
			  <span>目标项目</span>
			  <select className={userForm.input} value={transferDialog.targetProjectID} onChange={(event) => setTransferDialog((current) => current ? { ...current, targetProjectID: event.target.value } : current)}>
				{projectContext.projects.filter((project) => project.id !== selectedProjectID).map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}
			  </select>
			</label>
			<p className={galleryClasses.groupText}>将 {transferDialog.ids.length} 个已加载资产转移到目标项目。</p>
			<div className={galleryClasses.groupActions}>
			  <Button tone="ghost" onClick={() => setTransferDialog(null)} disabled={busyId === 'transfer'}>取消</Button>
			  <Button busy={busyId === 'transfer'} onClick={() => void applyProjectTransfer()}>确认转移</Button>
			</div>
		  </div>
		</Modal>
	  ) : null}
    </div>
  )
}

function ImageGrid({ rows, accessToken, busyId, selectedIds, onToggleSelected, onOpen, onCopyPrompt, onReuse, onDownload, onPublish, onDelete, onGroup, onMediaRefresh }: {
  rows: GalleryImage[]
  accessToken?: string
  busyId: string | null
  selectedIds: Set<string>
  onToggleSelected: (imageID: string, checked?: boolean) => void
  onOpen: (image: GalleryImage) => void
  onCopyPrompt: (image: GalleryImage) => void | Promise<void>
  onReuse: (image: GalleryImage) => void
  onDownload: (image: GalleryImage) => void
  onPublish: (image: GalleryImage) => void
  onDelete: (image: GalleryImage) => void
  onGroup: (image: GalleryImage) => void
  onMediaRefresh: (imageId: string) => string | undefined | void | Promise<string | undefined | void>
}) {
  return (
    <div className={galleryClasses.grid}>
      {rows.map((image) => {
        const card = galleryImageCard(image)
        const publishAction = publishActionPresentation(image)
        return (
          <article key={image.id} data-gallery-image-id={image.id} className={galleryClasses.card}>
            <GalleryImageFrame
              src={card.assetPath ? userApi.imageAssetUrl(card.assetPath, accessToken) : undefined}
              mediaExpiresAt={image.preview_expires_at}
              alt={card.title}
              width={image.width}
              height={image.height}
              aspectRatio={galleryImageAspect({ width: image.width, height: image.height, aspectRatio: image.aspect_ratio })}
              selected={selectedIds.has(image.id)}
              onOpen={() => onOpen(image)}
              onMediaRefresh={() => onMediaRefresh(image.id)}
              imageClassName={galleryClasses.thumbImage}
              topAction={(
	                <button
	                  type="button"
	                  data-gallery-selection-control
	                  className={galleryClasses.assetSelectHitArea}
                  title="选择图片"
                  aria-label={`选择 ${card.title}`}
                  aria-pressed={selectedIds.has(image.id)}
                  onClick={() => onToggleSelected(image.id)}
                >
                  <span className={cn(galleryClasses.assetSelectVisual, selectedIds.has(image.id) && galleryClasses.assetSelectVisualSelected)} aria-hidden="true">
                    {selectedIds.has(image.id) ? <Check strokeWidth={2.5} /> : null}
                  </span>
                </button>
              )}
	              actions={<div className={galleryClasses.iconActions} data-gallery-card-actions>
                {iconButton('复制提示词', <ActionIcon name="copy" />, () => void onCopyPrompt(image), !card.prompt)}
                {iconButton('复用配置', <ActionIcon name="edit" />, () => onReuse(image))}
                {iconButton('下载', <ActionIcon name="download" />, () => onDownload(image), !card.canDownload)}
                {iconButton(publishAction.label, <PublishActionIcon icon={publishAction.icon} />, () => onPublish(image), !card.canPublish, busyId === image.id, publishAction.tone)}
                {iconButton('设置分组', <ActionIcon name="group" />, () => onGroup(image))}
                {iconButton('删除', <ActionIcon name="delete" />, () => onDelete(image), false, busyId === image.id, 'danger')}
              </div>}
            />
                  <div className={galleryClasses.info}>
                    <div className="flex flex-wrap items-center gap-2">
                      <StatusPill status={image.visibility_status || 'private'} />
                      <span className={galleryClasses.groupLabel}>{card.groupLabel}</span>
                    </div>
                    <div className={galleryClasses.titleLine}>{card.title}</div>
                    <div className={galleryClasses.metaLine}>
                      <span>{card.modelLabel}</span>
                      <span>{card.ratioLabel}</span>
                    </div>
                    <div className={galleryClasses.metaLine}>
                      <span>{card.createdAtLabel}</span>
                    </div>
                  </div>
          </article>
        )
      })}
    </div>
  )
}

function GalleryGridSkeleton() {
  return (
    <div className={galleryClasses.grid} aria-hidden="true">
      {Array.from({ length: 8 }).map((_, index) => (
        <div key={index} className="mb-8 break-inside-avoid overflow-hidden rounded-3xl border border-[var(--border)] bg-[var(--surface)]">
          <div className="pg-skeleton aspect-[4/3] w-full" />
          <div className="grid gap-3 p-4">
            <div className="pg-skeleton h-4 rounded-xl" />
            <div className="pg-skeleton h-3 w-2/3 rounded-xl" />
            <div className="pg-skeleton h-3 w-1/2 rounded-xl" />
          </div>
        </div>
      ))}
    </div>
  )
}
