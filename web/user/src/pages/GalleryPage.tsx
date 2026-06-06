import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import type { GalleryImage, ImageTaskStatus, ImageTaskType, PublishStatus } from '../../../shared/api-types'
import { userApi } from '../../../shared/user-api'
import { Button, EmptyState, ImageLightbox, LoadingState, Modal, PublicDetailIcon, PublicImageDetail, copyText, useApp } from '../components'
import { errorMessage, useApiResource } from '../useApiResource'
import { createGalleryEditContext, galleryEditContextKey } from './galleryEditContext'
import { filterGalleryImages, galleryImageCard } from './galleryRows'

const shell = {
  content: { padding: 40 } as const,
  header: { marginBottom: 40, display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', gap: 20, flexWrap: 'wrap' as const },
  title: { fontSize: 48, margin: 0 },
  filters: { display: 'flex', gap: 12, marginBottom: 32, flexWrap: 'wrap' as const },
  filterButton: { padding: '8px 16px', background: 'var(--vault-panel)', border: '1px solid var(--vault-line)', borderRadius: 8, fontSize: 14, color: 'var(--vault-muted)', cursor: 'pointer' },
  activeFilter: { borderColor: 'var(--vault-gold)', color: 'var(--vault-gold)' },
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
    <button type="button" className={`icon-action ${tone}`} title={label} aria-label={label} disabled={disabled || busy} onClick={onClick}>
      {busy ? <span className="spinner" /> : icon}
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
    <div className="content" style={shell.content}>
      <div className="header" style={shell.header}>
        <div>
          <p className="eyebrow">YOUR COLLECTION</p>
          <h1 style={shell.title}>历史资产</h1>
        </div>
        <div style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索标题、提示词或模型" style={{ width: 280, borderRadius: 8 }} />
        </div>
      </div>

      <div className="filters" style={shell.filters}>
            <div style={{ flexBasis: '100%', display: 'flex', gap: 10, flexWrap: 'wrap' }}>
              {typeFilters.map((item) => (
                <button key={item.value} type="button" className={`filter-btn${type === item.value ? ' active' : ''}`} style={{ ...shell.filterButton, ...(type === item.value ? shell.activeFilter : {}) }} onClick={() => setType(item.value)}>{item.label}</button>
              ))}
            </div>
            <div style={{ flexBasis: '100%', display: 'flex', gap: 10, flexWrap: 'wrap' }}>
              <button type="button" className={`filter-btn${imageGroup === 'all' ? ' active' : ''}`} style={{ ...shell.filterButton, ...(imageGroup === 'all' ? shell.activeFilter : {}) }} onClick={() => setImageGroup('all')}>全部分组</button>
              <button type="button" className={`filter-btn${imageGroup === 'ungrouped' ? ' active' : ''}`} style={{ ...shell.filterButton, ...(imageGroup === 'ungrouped' ? shell.activeFilter : {}) }} onClick={() => setImageGroup('ungrouped')}>未分组</button>
              {groupFilters.map((group) => (
                <button key={group} type="button" className={`filter-btn${imageGroup === group ? ' active' : ''}`} style={{ ...shell.filterButton, ...(imageGroup === group ? shell.activeFilter : {}) }} onClick={() => setImageGroup(group)}>{group}</button>
              ))}
            </div>
            <div style={{ flexBasis: '100%', display: 'flex', gap: 10, flexWrap: 'wrap' }}>
              {statusFilters.map((item) => (
                <button key={item.value} type="button" className={`filter-btn${status === item.value ? ' active' : ''}`} style={{ ...shell.filterButton, ...(status === item.value ? shell.activeFilter : {}) }} onClick={() => setStatus(item.value)}>{item.label}</button>
              ))}
            </div>
            <div style={{ flexBasis: '100%', display: 'flex', gap: 10, flexWrap: 'wrap' }}>
              {publishFilters.map((item) => (
                <button key={item.value} type="button" className={`filter-btn${publishStatus === item.value ? ' active' : ''}`} style={{ ...shell.filterButton, ...(publishStatus === item.value ? shell.activeFilter : {}) }} onClick={() => setPublishStatus(item.value)}>{item.label}</button>
              ))}
            </div>
          </div>

          {privateGallery.loading ? <LoadingState label="正在读取历史任务..." /> : null}
          {!privateGallery.loading && !filtered.length ? <EmptyState title="没有匹配的图片" detail="换一个筛选条件，或回工作台创建新任务。" action={<Button onClick={() => app.navigate('genpic')}>继续生成</Button>} /> : null}

          {filtered.length ? (
            <div className="gallery-batchbar">
              <label className="select-check"><input type="checkbox" checked={filtered.length > 0 && selectedImages.length === filtered.length} onChange={(event) => selectAllVisible(event.target.checked)} /> 全选</label>
              <span>{selectedImages.length ? `已选择 ${selectedImages.length} 张` : `共 ${filtered.length} 张`}</span>
              <span style={{ flex: 1 }} />
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
          <div className="delete-confirm">
            <div className="delete-confirm-mark"><Icon name="delete" /></div>
            <div>
              <h3>确认删除 {deleteDialog.images.length} 张图片？</h3>
              <p>删除后会同步清理图片文件和数据库记录，无法恢复。公开审核中的图片也会从审核队列移除。</p>
            </div>
            <div className="delete-confirm-list">
              {deleteDialog.images.slice(0, 4).map((image) => (
                <span key={image.id}>{image.prompt || image.id}</span>
              ))}
              {deleteDialog.images.length > 4 ? <span>还有 {deleteDialog.images.length - 4} 张...</span> : null}
            </div>
            <div className="action-row delete-confirm-actions">
              <Button tone="ghost" onClick={() => setDeleteDialog(null)} disabled={busyId === 'batch'}>取消</Button>
              <Button tone="danger" busy={busyId === 'batch' || busyId === deleteDialog.images[0]?.id} onClick={() => void confirmDeleteImages()}>确认删除</Button>
            </div>
          </div>
        </Modal>
      ) : null}
      {groupDialog ? (
        <Modal title="设置图片分组" onClose={() => setGroupDialog(null)}>
          <div className="group-editor">
            <label>
              <span>分组名称</span>
              <input value={groupDraft} onChange={(event) => setGroupDraft(event.target.value)} placeholder="输入新分组，或选择已有分组" list="gallery-groups" autoFocus />
              <datalist id="gallery-groups">
                {allGroupFilters.map((group) => <option key={group} value={group} />)}
              </datalist>
            </label>
            <p>留空保存会清除所选图片的分组。</p>
            <div className="action-row">
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
    <div className="gallery-grid" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: 24 }}>
      {rows.map((image) => {
        const card = galleryImageCard(image)
        return (
          <article key={image.id} className="asset-card" style={{ background: 'var(--vault-panel)', borderRadius: 12, border: '1px solid var(--vault-line)', overflow: 'hidden', position: 'relative' }}>
            <label className="asset-select" title="选择图片">
              <input type="checkbox" checked={selectedIds.has(image.id)} onChange={(event) => onToggleSelected(image.id, event.target.checked)} />
            </label>
            <button type="button" className="asset-thumb" style={{ width: '100%', aspectRatio: '1', background: 'var(--vault-bg)', overflow: 'hidden', display: 'grid', placeItems: 'center', color: 'var(--vault-muted)' }} onClick={() => onPreview(image)}>
              {card.assetPath ? <img src={userApi.imageAssetUrl(card.assetPath, accessToken)} alt={card.title} style={{ width: '100%', height: '100%', objectFit: 'cover' }} /> : <span>无预览</span>}
            </button>
            <span className="status-pill" style={{ position: 'absolute', top: 12, right: 12, padding: '4px 8px', background: 'rgba(0,0,0,0.6)', borderRadius: 6, fontSize: 10 }}>{card.publishLabel}</span>
            <div className="asset-info" style={{ padding: 16 }}>
              <div className="asset-title" style={{ fontSize: 14, fontWeight: 700, marginBottom: 4, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{card.title}</div>
              <div className="asset-meta" style={{ fontFamily: 'JetBrains Mono, monospace', fontSize: 11, color: 'var(--vault-muted)', display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                <span>{card.modelLine}</span>
                <span>{card.createdAtLabel}</span>
              </div>
              <div className="asset-group-label">{card.groupLabel}</div>
              <div className="asset-icon-actions">
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
