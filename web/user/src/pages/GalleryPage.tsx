import { useEffect, useMemo, useRef, useState } from 'react'
import type { MouseEvent, ReactNode } from 'react'
import type { GalleryImage, ImageTaskStatus, ImageTaskType, PublishStatus } from '../../../shared/api-types'
import { userApi } from '../../../shared/user-api'
import { cn } from '../../../shared/classnames'
import { Button, EmptyState, ErrorState, GalleryFilterToolbar, GalleryImageFrame, ImageDetailModal, ImageLightbox, Modal, PublicDetailIcon, StatusPill, copyText, useApp, type ImageLightboxPayload } from '../components'
import { errorMessage } from '../useApiResource'
import { userForm, userState } from '../ui/classes'
import { rdGallery } from '../ui/redesign-classes'
import { Check, Copy, Download, Edit, FolderPlus, Globe, Trash2 } from '../ui/icons'
import { createGalleryEditContext, galleryEditContextKey } from './galleryEditContext'
import { galleryImageAspect, selectVisibleGalleryImages, toggleGalleryImageSelection } from './galleryExperience'
import { applyGalleryPage, galleryLoadingForReload, initialGalleryPageState } from './galleryPagination'
import { filterGalleryImages, galleryImageCard } from './galleryRows'

const GALLERY_PAGE_SIZE = 50

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
  batchBar: rdGallery.batchBar,
  selectCheck: 'inline-flex items-center gap-2 text-sm text-[var(--fg)]',
  batchSelectAll: 'flex items-center gap-1.5 rounded-xl px-4 py-2 text-xs font-medium text-[var(--fg)] transition-colors hover:bg-[var(--surface)] hover:text-[var(--accent)]',
  batchSelectAllActive: 'bg-[var(--accent)]/12 text-[var(--accent)] ring-1 ring-[var(--accent)]/35',
  batchSpacer: 'min-w-0 flex-1',
  batchBtn: rdGallery.batchBtn,
  grid: rdGallery.masonry,
  card: 'group/asset mb-8 block w-full break-inside-avoid',
  assetSelectHitArea: 'group/select grid size-10 place-items-center rounded-lg border-0 bg-transparent p-0 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]',
  assetSelectVisual: 'grid size-[22px] place-items-center rounded-md border border-[var(--image-action-border)] bg-[var(--image-action-bg)] text-[var(--image-action-text)] opacity-0 shadow-sm backdrop-blur transition-[opacity,transform,background-color,border-color,color] duration-200 group-hover/asset:opacity-100 group-hover/select:opacity-100 group-focus-visible/select:opacity-100 group-active/select:scale-90 [@media(pointer:coarse)]:opacity-60 motion-reduce:transition-none [&_svg]:size-3.5',
  assetSelectVisualSelected: 'border-[var(--accent)] bg-[var(--accent)] text-white opacity-100',
  thumbImage: 'object-cover',
  info: 'grid gap-2 pt-3',
  titleLine: rdGallery.itemTitle,
  metaLine: rdGallery.itemMeta,
  groupLabel: 'inline-flex items-center rounded-full border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_7%,transparent)] px-2.5 py-1 font-vault-mono text-[10px] text-[var(--muted)]',
  iconActions: 'flex flex-wrap items-center justify-end gap-1 rounded-xl border border-[var(--image-action-border)] bg-[var(--image-action-bg)] p-1 backdrop-blur',
  iconButton: 'grid size-10 place-items-center rounded-lg p-1 text-[var(--image-action-text)] transition-colors hover:bg-[var(--image-action-hover-bg)] hover:text-[var(--image-action-hover-text)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-[var(--focus-ring)] active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-45 [&_svg]:size-4',
  iconButtonDanger: 'hover:border-[var(--accent-coral)] hover:bg-[color-mix(in_oklch,var(--accent-coral)_12%,transparent)] hover:text-[var(--accent-coral)]',
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
  { value: 'reference_to_image', label: '参考生图' },
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

function iconButton(label: string, icon: ReactNode, onClick: () => void, disabled?: boolean, busy?: boolean, tone = '') {
  return (
    <button
      type="button"
      className={cn(galleryClasses.iconButton, tone === 'danger' && galleryClasses.iconButtonDanger)}
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
  const [imagePreview, setImagePreview] = useState<ImageLightboxPayload | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(() => new Set())
  const [groupDialog, setGroupDialog] = useState<{ ids: string[] } | null>(null)
  const [groupDraft, setGroupDraft] = useState('')
  const [deleteDialog, setDeleteDialog] = useState<{ images: GalleryImage[] } | null>(null)

  async function loadPage(pageNumber: number, mode: 'replace' | 'append') {
    const generation = ++loadGenerationRef.current
    if (mode === 'replace') setLoading(true)
    else setLoadingMore(true)
    setLoadError('')
    try {
      const incoming = await userApi.listGalleryImages(pageNumber, GALLERY_PAGE_SIZE)
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

  async function reloadLoadedPages() {
    const generation = ++loadGenerationRef.current
    const reloadFlags = galleryLoadingForReload()
    setLoading(reloadFlags.loading)
    setLoadingMore(reloadFlags.loadingMore)
    setLoadError('')
    try {
      const lastPage = Math.max(1, galleryPage.page)
      let refreshed = initialGalleryPageState<GalleryImage>()
      for (let pageNumber = 1; pageNumber <= lastPage; pageNumber += 1) {
        const incoming = await userApi.listGalleryImages(pageNumber, GALLERY_PAGE_SIZE)
        if (generation !== loadGenerationRef.current) return
        refreshed = applyGalleryPage(refreshed, incoming, {
          page: pageNumber,
          pageSize: GALLERY_PAGE_SIZE,
          mode: pageNumber === 1 ? 'replace' : 'append',
        })
        if (!refreshed.hasMore) break
      }
      setGalleryPage(refreshed)
    } catch (err) {
      if (generation === loadGenerationRef.current) {
        const message = errorMessage(err)
        setLoadError(message)
        app.notify('error', message)
      }
    } finally {
      if (generation === loadGenerationRef.current) setLoading(false)
    }
  }

  useEffect(() => {
    void loadPage(1, 'replace')
    return () => { loadGenerationRef.current += 1 }
  }, [])

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

  const selectedImages = useMemo(() => rows.filter((image) => selectedIds.has(image.id)), [rows, selectedIds])

  async function publishImage(image: GalleryImage) {
    setBusyId(image.id)
    try {
      await userApi.publishImage(image.id)
      app.notify('success', '已提交公开审核')
      await reloadLoadedPages()
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusyId(null)
    }
  }

  async function publishImages(images: GalleryImage[]) {
    if (!images.length) return
    setBusyId('batch')
    try {
      await Promise.all(images.map((image) => userApi.publishImage(image.id)))
      app.notify('success', `已提交 ${images.length} 张图片公开审核`)
      await reloadLoadedPages()
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
      await Promise.all(images.map((image) => userApi.deleteGalleryImage(image.id)))
      const deleted = new Set(images.map((image) => image.id))
      setSelectedIds((current) => new Set(Array.from(current).filter((id) => !deleted.has(id))))
      if (selected && deleted.has(selected.id)) setSelected(null)
      setDeleteDialog(null)
      app.notify('success', `已永久删除 ${images.length} 张图片`)
      await reloadLoadedPages()
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusyId(null)
    }
  }

  function continueEdit(image: GalleryImage) {
    const sources = image.reference_assets?.length ? image.reference_assets : []
    window.sessionStorage.setItem(galleryEditContextKey, JSON.stringify(createGalleryEditContext({
      prompt: image.prompt ?? '',
      sources,
      fallbackImageUrl: sources.length ? '' : assetUrl(image.url || image.download_url || ''),
      task_type: sources.length || image.url || image.download_url ? 'image_edit' : 'text_to_image',
      route_model_code: image.route_model_code || image.abstract_model,
      base_resolution: image.base_resolution,
      aspect_ratio: image.aspect_ratio,
    })))
    app.navigate('genpic')
  }

  function downloadImage(image?: Pick<GalleryImage, 'url' | 'download_url' | 'id'>) {
    const url = image?.download_url ?? image?.url
    if (!image || !url) return
    const link = document.createElement('a')
    link.href = assetUrl(url)
    link.download = downloadFilename(image)
    link.rel = 'noopener noreferrer'
    document.body.appendChild(link)
    link.click()
    link.remove()
  }

  function assetUrl(url: string) {
    return userApi.imageAssetUrl(url, app.session?.token)
  }

  function downloadImages(images: GalleryImage[]) {
    images.forEach((image, index) => {
      window.setTimeout(() => downloadImage(image), index * 120)
    })
    app.notify('success', `已开始下载 ${images.length} 张图片`)
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

  function selectAllVisible(checked: boolean) {
    setSelectedIds((current) => selectVisibleGalleryImages(current, filtered.map((image) => image.id), checked))
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
      const updated = await Promise.all(groupDialog.ids.map((id) => userApi.updateGalleryImageGroup(id, name)))
      await reloadLoadedPages()
      if (selected) {
        const nextSelected = updated.find((image) => image.id === selected.id)
        if (nextSelected) setSelected(nextSelected)
      }
      setGroupDialog(null)
      setGroupDraft('')
      app.notify('success', name ? '已设置图片分组' : '已清除图片分组')
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusyId(null)
    }
  }

  return (
    <div className={galleryClasses.content}>
      <div className={galleryClasses.header}>
        <div>
          <h1 className={galleryClasses.title}>历史资产</h1>
          <p className="mb-0 mt-3 max-w-[56ch] text-sm leading-6 text-[var(--muted)]">筛选、分组和重用已生成的图片，每一张资产都保留原始参数与公开状态。</p>
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
            <div className={galleryClasses.batchBar}>
              <div className={rdGallery.batchCount}>已选择 {selectedImages.length} 项</div>
              <div className="flex items-center gap-1 pl-2">
                <button
                  className={cn(galleryClasses.batchSelectAll, filtered.length > 0 && selectedImages.length === filtered.length && galleryClasses.batchSelectAllActive)}
                  type="button"
                  aria-pressed={filtered.length > 0 && selectedImages.length === filtered.length}
                  onClick={() => selectAllVisible(!(filtered.length > 0 && selectedImages.length === filtered.length))}
                >
                  <span className={cn(rdGallery.itemCheckbox, filtered.length > 0 && selectedImages.length === filtered.length && rdGallery.itemCheckboxChecked)}>
                    {filtered.length > 0 && selectedImages.length === filtered.length ? '✓' : ''}
                  </span>
                  全选
                </button>
                <button className={galleryClasses.batchBtn} type="button" onClick={() => downloadImages(selectedImages)}><ActionIcon name="download" /> 打包下载</button>
                <button className={galleryClasses.batchBtn} type="button" disabled={busyId === 'batch'} onClick={() => void publishImages(selectedImages)}><ActionIcon name="public" /> 公开</button>
                <button className={galleryClasses.batchBtn} type="button" onClick={() => openGroupDialog(selectedImages)}><ActionIcon name="group" /> 设为分组</button>
                <div className="mx-1 h-4 w-px bg-[var(--border)]" />
                <button className={cn(galleryClasses.batchBtn, 'text-[var(--accent-coral)] hover:bg-[color-mix(in_oklch,var(--accent-coral)_10%,transparent)] hover:text-[var(--accent-coral)]')} type="button" disabled={busyId === 'batch'} onClick={() => requestDeleteImages(selectedImages)}><ActionIcon name="delete" /> 删除</button>
              </div>
            </div>
          ) : null}

      <ImageGrid
        rows={filtered}
        accessToken={app.session?.token}
        busyId={busyId}
        selectedIds={selectedIds}
        onToggleSelected={toggleSelected}
        onOpen={setSelected}
        onCopyPrompt={async (image) => {
          await copyText(image.prompt || image.id)
          app.notify('success', '提示词已复制')
        }}
        onContinue={continueEdit}
        onDownload={(image) => downloadImage(image)}
        onPublish={publishImage}
        onDelete={(image) => requestDeleteImages([image])}
        onGroup={(image) => openGroupDialog([image])}
      />

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
          return { id: asset.id || asset.preview_url || url, url, alt: asset.name || '原图', onPreview: () => setImagePreview({ url, downloadUrl: url, alt: asset.name || '原图', source: '原图引用' }) }
        })}
        showPublicStats={false}
        onPreviewImage={setImagePreview}
        onCopyPrompt={async (prompt) => {
          await copyText(prompt)
          app.notify('success', 'Prompt 已复制')
        }}
        actions={selected ? [
          { key: 'edit', label: '继续编辑', icon: <PublicDetailIcon name="edit" />, onClick: () => continueEdit(selected) },
          { key: 'download', label: '下载图片', icon: <PublicDetailIcon name="download" />, onClick: () => downloadImage(selected), disabled: !selected.url && !selected.download_url },
          { key: 'public', label: '申请公开', icon: <PublicDetailIcon name="public" />, onClick: () => void publishImage(selected), disabled: !selected.url },
          { key: 'group', label: '设置分组', icon: <PublicDetailIcon name="group" />, onClick: () => openGroupDialog([selected]) },
          { key: 'delete', label: '删除图片', icon: <PublicDetailIcon name="delete" />, onClick: () => requestDeleteImages([selected]), tone: 'danger' },
        ] : []}
        previewSourceLabel="历史资产"
        onClose={() => setSelected(null)}
      />
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
      <ImageLightbox image={imagePreview} onClose={() => setImagePreview(null)} />
    </div>
  )
}

function ImageGrid({ rows, accessToken, busyId, selectedIds, onToggleSelected, onOpen, onCopyPrompt, onContinue, onDownload, onPublish, onDelete, onGroup }: {
  rows: GalleryImage[]
  accessToken?: string
  busyId: string | null
  selectedIds: Set<string>
  onToggleSelected: (imageID: string, checked?: boolean) => void
  onOpen: (image: GalleryImage) => void
  onCopyPrompt: (image: GalleryImage) => void | Promise<void>
  onContinue: (image: GalleryImage) => void
  onDownload: (image: GalleryImage) => void
  onPublish: (image: GalleryImage) => void
  onDelete: (image: GalleryImage) => void
  onGroup: (image: GalleryImage) => void
}) {
  return (
    <div className={galleryClasses.grid}>
      {rows.map((image) => {
        const card = galleryImageCard(image)
        return (
          <article key={image.id} className={galleryClasses.card}>
            <GalleryImageFrame
              src={card.assetPath ? userApi.imageAssetUrl(card.assetPath, accessToken) : undefined}
              alt={card.title}
              width={image.width}
              height={image.height}
              aspectRatio={galleryImageAspect({ width: image.width, height: image.height, aspectRatio: image.aspect_ratio })}
              selected={selectedIds.has(image.id)}
              onOpen={() => onOpen(image)}
              imageClassName={galleryClasses.thumbImage}
              topAction={(
                <button
                  type="button"
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
              actions={<div className={galleryClasses.iconActions}>
                {iconButton('复制提示词', <ActionIcon name="copy" />, () => void onCopyPrompt(image), !card.prompt)}
                {iconButton('继续编辑', <ActionIcon name="edit" />, () => onContinue(image))}
                {iconButton('下载', <ActionIcon name="download" />, () => onDownload(image), !card.canDownload)}
                {iconButton(card.publishActionLabel, <ActionIcon name="public" />, () => onPublish(image), !card.canPublish, busyId === image.id)}
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
