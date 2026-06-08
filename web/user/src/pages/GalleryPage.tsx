import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import type { GalleryImage, ImageTaskStatus, ImageTaskType, PublishStatus } from '../../../shared/api-types'
import { userApi } from '../../../shared/user-api'
import { cn } from '../../../shared/classnames'
import { Button, EmptyState, ImageLightbox, LoadingState, Modal, PublicDetailIcon, PublicImageDetail, copyText, useApp } from '../components'
import { errorMessage, useApiResource } from '../useApiResource'
import { userButton, userForm, userState, userText } from '../ui/classes'
import { createGalleryEditContext, galleryEditContextKey } from './galleryEditContext'
import { filterGalleryImages, galleryImageCard } from './galleryRows'

const galleryClasses = {
  content: 'mx-auto w-full max-w-[1200px] p-10 max-[760px]:p-5 max-[420px]:p-4',
  header: 'mb-10 flex flex-wrap items-end justify-between gap-5',
  title: 'm-0 font-vault-display text-5xl font-medium leading-none text-[var(--fg)] max-[620px]:text-4xl',
  searchWrap: 'flex flex-wrap items-center gap-3',
  searchInput: 'w-[280px] max-w-full rounded-lg',
  filters: 'mb-8 flex flex-wrap gap-3',
  filterRow: 'flex basis-full flex-wrap gap-2.5',
  filterButton: 'rounded-lg border border-[var(--border)] bg-[var(--surface)] px-4 py-2 font-vault-mono text-sm text-[var(--muted)] transition hover:-translate-y-px hover:border-[color-mix(in_oklch,var(--accent)_45%,var(--border))] hover:text-[var(--fg)]',
  filterButtonActive: 'border-[var(--accent)] bg-[color-mix(in_oklch,var(--accent)_12%,transparent)] text-[var(--accent)]',
  batchBar: 'mb-[18px] flex flex-wrap items-center gap-2.5 rounded-[10px] border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] px-3 py-2.5 text-[var(--muted)]',
  selectCheck: 'inline-flex items-center gap-2 text-sm text-[var(--fg)]',
  batchSpacer: 'min-w-0 flex-1',
  grid: 'grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-6 max-[760px]:grid-cols-1',
  card: 'relative overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--surface)]',
  assetSelect: 'absolute left-3 top-3 z-10 grid size-7 place-items-center rounded-lg border border-white/15 bg-[#05070db8] backdrop-blur-[10px]',
  thumb: 'grid aspect-square w-full place-items-center overflow-hidden bg-[var(--bg)] p-0 text-[var(--muted)]',
  thumbImage: 'h-full w-full object-cover',
  status: 'absolute right-3 top-3 rounded-md bg-black/60 px-2 py-1 text-[10px] text-[var(--muted)] backdrop-blur',
  info: 'p-4',
  titleLine: 'mb-1 overflow-hidden text-ellipsis whitespace-nowrap text-sm font-bold',
  metaLine: 'flex justify-between gap-3 font-vault-mono text-[11px] text-[var(--muted)]',
  groupLabel: 'mt-2.5 inline-flex w-fit rounded-full bg-[color-mix(in_oklch,var(--accent)_12%,transparent)] px-2 py-1 text-[11px] text-[var(--accent)]',
  iconActions: 'mt-3.5 flex flex-wrap justify-end gap-2',
  iconButton: 'size-[34px] min-h-[34px] rounded-lg p-0 hover:border-[var(--accent)] hover:bg-[color-mix(in_oklch,var(--accent)_12%,transparent)] hover:text-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-45',
  iconButtonDanger: 'hover:border-[var(--accent-coral)] hover:bg-[color-mix(in_oklch,var(--accent-coral)_12%,transparent)] hover:text-[oklch(78%_.14_35)]',
  deleteConfirm: 'grid grid-cols-[42px_minmax(0,1fr)] items-start gap-4',
  deleteMark: 'grid size-[42px] place-items-center rounded-xl border border-[color-mix(in_oklch,var(--accent-coral)_42%,var(--border))] bg-[color-mix(in_oklch,var(--accent-coral)_14%,transparent)] text-[oklch(78%_.14_35)]',
  deleteTitle: 'm-0 mb-2 text-xl',
  deleteText: 'm-0 leading-[1.65] text-[var(--muted)]',
  deleteList: 'col-span-full flex flex-wrap gap-2 rounded-[10px] border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] p-3',
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

function Icon({ name }: { name: 'eye' | 'download' | 'public' | 'delete' | 'edit' | 'copy' | 'group' }) {
  const common = { width: 17, height: 17, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 2, strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const }
  if (name === 'eye') return <svg {...common}><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z" /><circle cx="12" cy="12" r="3" /></svg>
  if (name === 'download') return <svg {...common}><path d="M12 3v12" /><path d="m7 10 5 5 5-5" /><path d="M5 21h14" /></svg>
  if (name === 'public') return <svg {...common}><circle cx="12" cy="12" r="10" /><path d="M2 12h20" /><path d="M12 2a15 15 0 0 1 0 20" /><path d="M12 2a15 15 0 0 0 0 20" /></svg>
  if (name === 'delete') return <svg {...common}><path d="M3 6h18" /><path d="M8 6V4h8v2" /><path d="M19 6l-1 14H6L5 6" /></svg>
  if (name === 'edit') return <svg {...common}><path d="M12 20h9" /><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z" /></svg>
  if (name === 'copy') return <svg {...common}><rect x="9" y="9" width="13" height="13" rx="2" /><rect x="2" y="2" width="13" height="13" rx="2" /></svg>
  return <svg {...common}><path d="M20 12V7a2 2 0 0 0-2-2h-6.2a2 2 0 0 1-1.4-.6L9.6 3.6A2 2 0 0 0 8.2 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-3" /><path d="M16 11h6" /><path d="M19 8v6" /></svg>
}

function iconButton(label: string, icon: ReactNode, onClick: () => void, disabled?: boolean, busy?: boolean, tone = '') {
  return (
    <button
      type="button"
      className={cn(userButton.icon, galleryClasses.iconButton, tone === 'danger' && galleryClasses.iconButtonDanger)}
      title={label}
      aria-label={label}
      disabled={disabled || busy}
      onClick={onClick}
    >
      {busy ? <span className={userState.spinner} /> : icon}
    </button>
  )
}

export function GalleryPage() {
  const app = useApp()
  const privateGallery = useApiResource(() => userApi.listGalleryImages(), [])
  const [query, setQuery] = useState('')
  const [type, setType] = useState<(typeof typeFilters)[number]['value']>('all')
  const [status, setStatus] = useState<(typeof statusFilters)[number]['value']>('all')
  const [publishStatus, setPublishStatus] = useState<(typeof publishFilters)[number]['value']>('all')
  const [imageGroup, setImageGroup] = useState('all')
  const [selected, setSelected] = useState<GalleryImage | null>(null)
  const [imagePreview, setImagePreview] = useState<{ url: string; alt: string } | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(() => new Set())
  const [groupDialog, setGroupDialog] = useState<{ ids: string[] } | null>(null)
  const [groupDraft, setGroupDraft] = useState('')
  const [deleteDialog, setDeleteDialog] = useState<{ images: GalleryImage[] } | null>(null)

  useEffect(() => {
    setImageGroup('all')
  }, [type])

  const rows = privateGallery.data ?? []
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
    ;(privateGallery.data ?? []).forEach((image) => {
      const group = image.image_group?.trim()
      if (group) groups.add(group)
    })
    return Array.from(groups).sort()
  }, [privateGallery.data])

  const filtered = useMemo(() => filterGalleryImages(typeRows, { type: 'all', status, publishStatus, imageGroup, query }), [typeRows, query, status, publishStatus, imageGroup])

  const selectedImages = useMemo(() => filtered.filter((image) => selectedIds.has(image.id)), [filtered, selectedIds])

  async function publishImage(image: GalleryImage) {
    setBusyId(image.id)
    try {
      await userApi.publishImage(image.id)
      app.notify('success', '已提交公开审核')
      await privateGallery.reload()
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
      await privateGallery.reload()
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
      await privateGallery.reload()
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
      quality: image.quality,
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
    setSelectedIds((current) => {
      const next = new Set(current)
      const shouldCheck = checked ?? !next.has(imageID)
      if (shouldCheck) next.add(imageID)
      else next.delete(imageID)
      return next
    })
  }

  function selectAllVisible(checked: boolean) {
    setSelectedIds(checked ? new Set(filtered.map((image) => image.id)) : new Set())
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
      await privateGallery.reload()
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
          <p className={userText.eyebrow}>YOUR COLLECTION</p>
          <h1 className={galleryClasses.title}>历史资产</h1>
        </div>
        <div className={galleryClasses.searchWrap}>
          <input className={cn(userForm.input, galleryClasses.searchInput)} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索标题、提示词或模型" />
        </div>
      </div>

      <div className={galleryClasses.filters}>
            <div className={galleryClasses.filterRow}>
              {typeFilters.map((item) => (
                <button key={item.value} type="button" className={cn(galleryClasses.filterButton, type === item.value && galleryClasses.filterButtonActive)} onClick={() => setType(item.value)}>{item.label}</button>
              ))}
            </div>
            <div className={galleryClasses.filterRow}>
              <button type="button" className={cn(galleryClasses.filterButton, imageGroup === 'all' && galleryClasses.filterButtonActive)} onClick={() => setImageGroup('all')}>全部分组</button>
              <button type="button" className={cn(galleryClasses.filterButton, imageGroup === 'ungrouped' && galleryClasses.filterButtonActive)} onClick={() => setImageGroup('ungrouped')}>未分组</button>
              {groupFilters.map((group) => (
                <button key={group} type="button" className={cn(galleryClasses.filterButton, imageGroup === group && galleryClasses.filterButtonActive)} onClick={() => setImageGroup(group)}>{group}</button>
              ))}
            </div>
            <div className={galleryClasses.filterRow}>
              {statusFilters.map((item) => (
                <button key={item.value} type="button" className={cn(galleryClasses.filterButton, status === item.value && galleryClasses.filterButtonActive)} onClick={() => setStatus(item.value)}>{item.label}</button>
              ))}
            </div>
            <div className={galleryClasses.filterRow}>
              {publishFilters.map((item) => (
                <button key={item.value} type="button" className={cn(galleryClasses.filterButton, publishStatus === item.value && galleryClasses.filterButtonActive)} onClick={() => setPublishStatus(item.value)}>{item.label}</button>
              ))}
            </div>
          </div>

          {privateGallery.loading ? <LoadingState label="正在读取历史任务..." /> : null}
          {!privateGallery.loading && !filtered.length ? <EmptyState title="没有匹配的图片" detail="换一个筛选条件，或回工作台创建新任务。" action={<Button onClick={() => app.navigate('genpic')}>继续生成</Button>} /> : null}

          {filtered.length ? (
            <div className={galleryClasses.batchBar}>
              <label className={galleryClasses.selectCheck}><input type="checkbox" checked={filtered.length > 0 && selectedImages.length === filtered.length} onChange={(event) => selectAllVisible(event.target.checked)} /> 全选</label>
              <span>{selectedImages.length ? `已选择 ${selectedImages.length} 张` : `共 ${filtered.length} 张`}</span>
              <span className={galleryClasses.batchSpacer} />
              {iconButton('批量下载', <Icon name="download" />, () => downloadImages(selectedImages), !selectedImages.length)}
              {iconButton('批量公开', <Icon name="public" />, () => void publishImages(selectedImages), !selectedImages.length, busyId === 'batch')}
              {iconButton('批量设置分组', <Icon name="group" />, () => openGroupDialog(selectedImages), !selectedImages.length)}
              {iconButton('批量删除', <Icon name="delete" />, () => requestDeleteImages(selectedImages), !selectedImages.length, busyId === 'batch', 'danger')}
            </div>
          ) : null}

      <ImageGrid rows={filtered} accessToken={app.session?.token} busyId={busyId} selectedIds={selectedIds} onToggleSelected={toggleSelected} onPreview={setSelected} onContinue={continueEdit} onDownload={(image) => downloadImage(image)} onPublish={publishImage} onDelete={(image) => requestDeleteImages([image])} onGroup={(image) => openGroupDialog([image])} />

      {selected ? (
        <Modal title="图片详情" onClose={() => setSelected(null)}>
          <PublicImageDetail
            image={selected}
            imageUrl={selected.url || selected.download_url ? assetUrl(selected.url || selected.download_url || '') : undefined}
            referenceImages={(selected.reference_assets ?? []).filter((asset) => asset.preview_url).map((asset) => {
              const url = assetUrl(asset.preview_url || '')
              return { id: asset.id || asset.preview_url || url, url, alt: asset.name || '原图', onPreview: () => setImagePreview({ url, alt: asset.name || '原图' }) }
            })}
            showPublicStats={false}
            onPreviewImage={(url, alt) => setImagePreview({ url, alt })}
            onCopyPrompt={async (prompt) => {
              await copyText(prompt)
              app.notify('success', 'Prompt 已复制')
            }}
            actions={[
              { key: 'edit', label: '继续编辑', icon: <PublicDetailIcon name="edit" />, onClick: () => continueEdit(selected) },
              { key: 'download', label: '下载图片', icon: <PublicDetailIcon name="download" />, onClick: () => downloadImage(selected), disabled: !selected.url && !selected.download_url },
              { key: 'public', label: '申请公开', icon: <PublicDetailIcon name="public" />, onClick: () => void publishImage(selected), disabled: !selected.url },
              { key: 'group', label: '设置分组', icon: <PublicDetailIcon name="group" />, onClick: () => openGroupDialog([selected]) },
              { key: 'delete', label: '删除图片', icon: <PublicDetailIcon name="delete" />, onClick: () => requestDeleteImages([selected]), tone: 'danger' },
            ]}
          />
        </Modal>
      ) : null}
      {deleteDialog ? (
        <Modal title="永久删除图片" onClose={() => setDeleteDialog(null)}>
          <div className={galleryClasses.deleteConfirm}>
            <div className={galleryClasses.deleteMark}><Icon name="delete" /></div>
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

function ImageGrid({ rows, accessToken, busyId, selectedIds, onToggleSelected, onPreview, onContinue, onDownload, onPublish, onDelete, onGroup }: {
  rows: GalleryImage[]
  accessToken?: string
  busyId: string | null
  selectedIds: Set<string>
  onToggleSelected: (imageID: string, checked?: boolean) => void
  onPreview: (image: GalleryImage) => void
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
            <label className={galleryClasses.assetSelect} title="选择图片">
              <input type="checkbox" checked={selectedIds.has(image.id)} onChange={(event) => onToggleSelected(image.id, event.target.checked)} />
            </label>
            <button type="button" className={galleryClasses.thumb} onClick={() => onPreview(image)}>
              {card.assetPath ? <img src={userApi.imageAssetUrl(card.assetPath, accessToken)} alt={card.title} className={galleryClasses.thumbImage} /> : <span>无预览</span>}
            </button>
            <span className={galleryClasses.status}>{card.publishLabel}</span>
            <div className={galleryClasses.info}>
              <div className={galleryClasses.titleLine}>{card.title}</div>
              <div className={galleryClasses.metaLine}>
                <span>{card.modelLine}</span>
                <span>{card.createdAtLabel}</span>
              </div>
              <div className={galleryClasses.groupLabel}>{card.groupLabel}</div>
              <div className={galleryClasses.iconActions}>
                {iconButton('编辑', <Icon name="edit" />, () => onContinue(image))}
                {iconButton('下载', <Icon name="download" />, () => onDownload(image), !card.canDownload)}
                {iconButton(card.publishActionLabel, <Icon name="public" />, () => onPublish(image), !card.canPublish, busyId === image.id)}
                {iconButton('设置分组', <Icon name="group" />, () => onGroup(image))}
                {iconButton('删除', <Icon name="delete" />, () => onDelete(image), false, busyId === image.id, 'danger')}
              </div>
            </div>
          </article>
        )
      })}
    </div>
  )
}
