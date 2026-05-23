import { useMemo, useState } from 'react'
import type { ImageResult, ImageTask, ImageTaskStatus, ImageTaskType, PublishStatus } from '../../../shared/api-types'
import { openApi } from '../../../shared/open-api'
import { userApi } from '../../../shared/user-api'
import { Button, EmptyState, ErrorState, LoadingState, Modal, formatDate, taskTypeLabel, useApp } from '../components'
import { errorMessage, useApiResource } from '../useApiResource'

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

function publishLabel(status?: PublishStatus) {
  if (status === 'public' || status === 'approved') return '已公开'
  if (status === 'reviewing' || status === 'pending_review') return '审核中'
  if (status === 'rejected') return '已拒绝'
  if (status === 'unpublished') return '已下架'
  return '私有'
}

export function GalleryPage() {
  const app = useApp()
  const privateGallery = useApiResource(() => userApi.listHistoryTasks(), [])
  const publicGallery = useApiResource(() => openApi.listPublicGallery().then((page) => page.items), [])
  const [view, setView] = useState<'private' | 'public'>('private')
  const [query, setQuery] = useState('')
  const [type, setType] = useState<(typeof typeFilters)[number]['value']>('all')
  const [status, setStatus] = useState<(typeof statusFilters)[number]['value']>('all')
  const [selected, setSelected] = useState<ImageTask | null>(null)
  const [publicSelected, setPublicSelected] = useState<ImageResult | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)

  const filtered = useMemo(() => {
    const rows = privateGallery.data ?? []
    return rows.filter((task) => {
      const search = `${task.title} ${task.prompt} ${task.model_group} ${task.provider}`.toLowerCase()
      const matchQuery = !query || search.includes(query.trim().toLowerCase())
      const matchType = type === 'all' || (type === 'api' ? task.route.includes('open') || task.route.includes('api') : task.task_type === type)
      const matchStatus = status === 'all' || task.status === status
      return matchQuery && matchType && matchStatus
    })
  }, [privateGallery.data, query, type, status])

  async function publishFirst(task: ImageTask) {
    const image = task.results[0]
    if (!image) return
    setBusyId(image.id)
    try {
      await userApi.publishImage(image.id)
      app.notify('success', '已提交公开审核')
      await privateGallery.reload()
      if (selected?.id === task.id) setSelected(await userApi.getTask(task.id))
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusyId(null)
    }
  }

  async function deleteTask(task: ImageTask) {
    setBusyId(task.id)
    try {
      await userApi.deleteTask(task.id)
      app.notify('success', '已从图库隐藏该任务')
      if (selected?.id === task.id) setSelected(null)
      await privateGallery.reload()
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusyId(null)
    }
  }

  function downloadImage(image?: ImageResult) {
    if (!image) return
    window.open(image.download_url ?? image.url, '_blank', 'noopener,noreferrer')
  }

  function exportRecords() {
    const blob = new Blob([JSON.stringify(filtered, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = 'pic-gallery-assets.json'
    link.click()
    URL.revokeObjectURL(url)
    app.notify('success', `已导出 ${filtered.length} 条记录`)
  }

  return (
    <div className="content" style={shell.content}>
      <div className="header" style={shell.header}>
        <div>
          <p className="eyebrow">{view === 'private' ? 'YOUR COLLECTION' : 'PUBLIC GALLERY'}</p>
          <h1 style={shell.title}>{view === 'private' ? '历史资产' : '公开广场'}</h1>
        </div>
        <div style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
          <Button tone={view === 'private' ? 'primary' : 'ghost'} onClick={() => setView('private')}>私有图库</Button>
          <Button tone={view === 'public' ? 'primary' : 'ghost'} onClick={() => setView('public')}>公开广场</Button>
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索标题、提示词或模型" style={{ width: 280, borderRadius: 8 }} />
          {view === 'private' ? <button type="button" className="filter-btn" style={{ ...shell.filterButton, background: 'var(--vault-gold)', color: 'var(--vault-bg)', border: 'none' }} onClick={exportRecords}>导出记录</button> : null}
        </div>
      </div>

      {view === 'private' ? (
        <>
          <div className="filters" style={shell.filters}>
            {typeFilters.map((item) => (
              <button key={item.value} type="button" className={`filter-btn${type === item.value ? ' active' : ''}`} style={{ ...shell.filterButton, ...(type === item.value ? shell.activeFilter : {}) }} onClick={() => setType(item.value)}>{item.label}</button>
            ))}
            <span style={{ flex: 1 }} />
            {statusFilters.map((item) => (
              <button key={item.value} type="button" className={`filter-btn${status === item.value ? ' active' : ''}`} style={{ ...shell.filterButton, ...(status === item.value ? shell.activeFilter : {}) }} onClick={() => setStatus(item.value)}>{item.label}</button>
            ))}
          </div>

          {privateGallery.loading ? <LoadingState label="正在读取历史任务..." /> : null}
          {privateGallery.error ? <ErrorState message={privateGallery.error} onRetry={privateGallery.reload} /> : null}
          {!privateGallery.loading && !filtered.length ? <EmptyState title="没有匹配的图片" detail="换一个筛选条件，或回工作台创建新任务。" action={<Button onClick={() => app.navigate('genpic')}>继续生成</Button>} /> : null}

          <TaskGrid rows={filtered} busyId={busyId} onPreview={setSelected} onDownload={(task) => downloadImage(task.results[0])} onPublish={publishFirst} onDelete={deleteTask} />
        </>
      ) : (
        <>
          {publicGallery.loading ? <LoadingState label="正在读取公开广场..." /> : null}
          {publicGallery.error ? <ErrorState message={publicGallery.error} onRetry={publicGallery.reload} /> : null}
          {!publicGallery.loading && !publicGallery.data?.length ? <EmptyState title="暂无公开作品" detail="公开广场未开启或暂无审核通过的图片。" /> : null}
          <div className="gallery-grid" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))', gap: 24 }}>
            {(publicGallery.data ?? []).filter((image) => !query || image.id.toLowerCase().includes(query.toLowerCase())).map((image) => (
              <article key={image.id} className="asset-card" style={{ background: 'var(--vault-panel)', borderRadius: 12, border: '1px solid var(--vault-line)', overflow: 'hidden' }}>
                <button type="button" className="asset-thumb" style={{ width: '100%', aspectRatio: '1', background: 'var(--vault-bg)', overflow: 'hidden' }} onClick={() => setPublicSelected(image)}>
                  {image.url ? <img src={image.url} alt={image.id} style={{ width: '100%', height: '100%', objectFit: 'cover' }} /> : null}
                </button>
                <div className="asset-info" style={{ padding: 16 }}>
                  <div className="asset-title" style={{ fontSize: 14, fontWeight: 700 }}>{image.id}</div>
                  <div className="asset-meta" style={{ fontSize: 12, color: 'var(--vault-muted)' }}>{publishLabel(image.publish_status)}</div>
                  <div style={{ display: 'flex', gap: 8, marginTop: 14 }}>
                    <Button tone="ghost" onClick={() => setPublicSelected(image)}>预览</Button>
                    <Button tone="ghost" onClick={() => downloadImage(image)}>下载</Button>
                  </div>
                </div>
              </article>
            ))}
          </div>
        </>
      )}

      {selected ? (
        <Modal title={selected.title} onClose={() => setSelected(null)}>
          <Preview task={selected} busyId={busyId} onContinue={() => app.navigate('genpic')} onDownload={() => downloadImage(selected.results[0])} onPublish={() => void publishFirst(selected)} onDelete={() => void deleteTask(selected)} />
        </Modal>
      ) : null}
      {publicSelected ? (
        <Modal title={publicSelected.id} onClose={() => setPublicSelected(null)}>
          <div className="preview-images">{publicSelected.url ? <img src={publicSelected.url} alt={publicSelected.id} /> : null}</div>
          <div className="action-row"><Button onClick={() => downloadImage(publicSelected)}>下载</Button></div>
        </Modal>
      ) : null}
    </div>
  )
}

function TaskGrid({ rows, busyId, onPreview, onDownload, onPublish, onDelete }: {
  rows: ImageTask[]
  busyId: string | null
  onPreview: (task: ImageTask) => void
  onDownload: (task: ImageTask) => void
  onPublish: (task: ImageTask) => void
  onDelete: (task: ImageTask) => void
}) {
  return (
    <div className="gallery-grid" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: 24 }}>
      {rows.map((task) => {
        const image = task.results[0]
        return (
          <article key={task.id} className="asset-card" style={{ background: 'var(--vault-panel)', borderRadius: 12, border: '1px solid var(--vault-line)', overflow: 'hidden', position: 'relative' }}>
            <button type="button" className="asset-thumb" style={{ width: '100%', aspectRatio: '1', background: 'var(--vault-bg)', overflow: 'hidden', display: 'grid', placeItems: 'center', color: 'var(--vault-muted)' }} onClick={() => onPreview(task)}>
              {image ? <img src={image.url} alt={task.title} style={{ width: '100%', height: '100%', objectFit: 'cover' }} /> : <span>{task.progress}%</span>}
            </button>
            <span className="status-pill" style={{ position: 'absolute', top: 12, right: 12, padding: '4px 8px', background: 'rgba(0,0,0,0.6)', borderRadius: 6, fontSize: 10 }}>{publishLabel(image?.publish_status)}</span>
            <div className="asset-info" style={{ padding: 16 }}>
              <div className="asset-title" style={{ fontSize: 14, fontWeight: 700, marginBottom: 4, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{task.title}</div>
              <div className="asset-meta" style={{ fontFamily: 'JetBrains Mono, monospace', fontSize: 11, color: 'var(--vault-muted)', display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                <span>{taskTypeLabel(task.task_type)} · {task.quality}</span>
                <span>{formatDate(task.created_at).slice(0, 10)}</span>
              </div>
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 14 }}>
                <Button tone="ghost" onClick={() => onPreview(task)}>预览</Button>
                <Button tone="ghost" disabled={!image} onClick={() => onDownload(task)}>下载</Button>
                <Button tone="ghost" disabled={!image} busy={busyId === image?.id} onClick={() => onPublish(task)}>申请公开</Button>
                <Button tone="danger" busy={busyId === task.id} onClick={() => onDelete(task)}>隐藏</Button>
              </div>
            </div>
          </article>
        )
      })}
    </div>
  )
}

function Preview({ task, busyId, onContinue, onDownload, onPublish, onDelete }: {
  task: ImageTask
  busyId: string | null
  onContinue: () => void
  onDownload: () => void
  onPublish: () => void
  onDelete: () => void
}) {
  return (
    <div className="preview-drawer">
      <div className="preview-images">
        {task.results.length ? task.results.map((image) => <img key={image.id} src={image.url} alt={task.title} />) : <EmptyState title="任务尚未完成" detail={`${task.status} / ${task.progress}%`} />}
      </div>
      <div className="preview-copy">
        <span className="status-pill">{task.status}</span>
        <p>{task.prompt}</p>
        <div className="meta-grid">
          <span>模型 <b>{task.model_group}</b></span>
          <span>质量 <b>{task.quality}</b></span>
          <span>比例 <b>{task.aspect_ratio}</b></span>
          <span>费用 <b>{task.estimate_points}</b></span>
        </div>
        <div className="action-row">
          <Button onClick={onContinue}>继续编辑</Button>
          <Button tone="ghost" disabled={!task.results[0]} onClick={onDownload}>下载首图</Button>
          <Button tone="ghost" disabled={!task.results[0]} busy={busyId === task.results[0]?.id} onClick={onPublish}>申请公开</Button>
          <Button tone="danger" busy={busyId === task.id} onClick={onDelete}>隐藏任务</Button>
        </div>
      </div>
    </div>
  )
}
