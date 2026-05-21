import { useMemo, useState } from 'react'
import type { ImageTask, ImageTaskStatus, ImageTaskType, PublishStatus } from '../../../shared/api-types'
import { mockApi } from '../../../shared/mock-api'
import { Button, EmptyState, ErrorState, LoadingState, Modal, formatDate, taskTypeLabel, useApp } from '../components'
import { errorMessage, useMockResource } from '../useMockResource'

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
  if (status === 'public') return '已公开'
  if (status === 'reviewing') return '审核中'
  if (status === 'rejected') return '已拒绝'
  return '私有'
}

function taskMatchesDate(task: ImageTask, date: string) {
  if (date === 'all') return true
  if (date === 'week') return task.created_at >= '2026-05-15'
  if (date === 'today') return task.created_at.startsWith('2026-05-21')
  return !task.created_at.startsWith('2026-05-21')
}

export function GalleryPage() {
  const app = useApp()
  const resource = useMockResource(() => mockApi.listTasks(), [])
  const [query, setQuery] = useState('')
  const [type, setType] = useState<(typeof typeFilters)[number]['value']>('all')
  const [status, setStatus] = useState<(typeof statusFilters)[number]['value']>('all')
  const [date, setDate] = useState('week')
  const [selected, setSelected] = useState<ImageTask | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)

  const filtered = useMemo(() => {
    const rows = resource.data ?? []
    return rows.filter((task) => {
      const search = `${task.title} ${task.prompt} ${task.model_group} ${task.provider}`.toLowerCase()
      const matchQuery = !query || search.includes(query.trim().toLowerCase())
      const matchType = type === 'all' || (type === 'api' ? task.route.includes('open') || task.route.includes('api') : task.task_type === type)
      const matchStatus = status === 'all' || task.status === status
      return matchQuery && matchType && matchStatus && taskMatchesDate(task, date)
    })
  }, [resource.data, query, type, status, date])

  async function publishFirst(task: ImageTask) {
    const image = task.results[0]
    if (!image) return
    setBusyId(image.id)
    try {
      const nextStatus = image.publish_status === 'reviewing' || image.publish_status === 'public' ? 'private' : 'reviewing'
      await mockApi.updatePublishStatus(image.id, nextStatus)
      app.notify('success', nextStatus === 'private' ? '已取消公开状态' : '已提交公开审核')
      await resource.reload()
      if (selected?.id === task.id) setSelected(await mockApi.getTask(task.id))
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusyId(null)
    }
  }

  async function deleteTask(task: ImageTask) {
    setBusyId(task.id)
    try {
      await mockApi.deleteTask(task.id)
      app.notify('success', '已从图库隐藏该任务')
      if (selected?.id === task.id) setSelected(null)
      await resource.reload()
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusyId(null)
    }
  }

  function downloadFirst(task: ImageTask) {
    const image = task.results[0]
    if (!image) return
    window.open(image.url, '_blank', 'noopener,noreferrer')
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
          <p className="eyebrow">YOUR COLLECTION</p>
          <h1 style={shell.title}>历史资产</h1>
        </div>
        <div style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索标题、提示词或模型" style={{ width: 280, borderRadius: 8 }} />
          <button type="button" className="filter-btn" style={{ ...shell.filterButton, background: 'var(--vault-gold)', color: 'var(--vault-bg)', border: 'none' }} onClick={exportRecords}>导出记录</button>
        </div>
      </div>

      <div className="filters" style={shell.filters}>
        {typeFilters.map((item) => (
          <button key={item.value} type="button" className={`filter-btn${type === item.value ? ' active' : ''}`} style={{ ...shell.filterButton, ...(type === item.value ? shell.activeFilter : {}) }} onClick={() => setType(item.value)}>{item.label}</button>
        ))}
        <span style={{ flex: 1 }} />
        {statusFilters.map((item) => (
          <button key={item.value} type="button" className={`filter-btn${status === item.value ? ' active' : ''}`} style={{ ...shell.filterButton, ...(status === item.value ? shell.activeFilter : {}) }} onClick={() => setStatus(item.value)}>{item.label}</button>
        ))}
        <button type="button" className={`filter-btn${date === 'week' ? ' active' : ''}`} style={{ ...shell.filterButton, ...(date === 'week' ? shell.activeFilter : {}) }} onClick={() => setDate(date === 'week' ? 'all' : 'week')}>最近 7 天</button>
      </div>

      {resource.loading ? <LoadingState /> : null}
      {resource.error ? <ErrorState message={resource.error} onRetry={resource.reload} /> : null}
      {!resource.loading && !filtered.length ? <EmptyState title="没有匹配的图片" detail="换一个筛选条件，或回工作台创建新任务。" action={<Button onClick={() => app.navigate('genpic')}>继续生成</Button>} /> : null}

      <div className="gallery-grid" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: 24 }}>
        {filtered.map((task) => {
          const image = task.results[0]
          return (
            <article key={task.id} className="asset-card" style={{ background: 'var(--vault-panel)', borderRadius: 12, border: '1px solid var(--vault-line)', overflow: 'hidden', position: 'relative', transition: 'transform 0.2s, border-color 0.2s' }}>
              <button type="button" className="asset-thumb" style={{ width: '100%', aspectRatio: '1', background: 'var(--vault-bg)', overflow: 'hidden', display: 'grid', placeItems: 'center', color: 'var(--vault-muted)' }} onClick={() => setSelected(task)}>
                {image ? <img src={image.url} alt={task.title} style={{ width: '100%', height: '100%', objectFit: 'cover' }} /> : <span>{task.progress}%</span>}
              </button>
              <span className={`status-pill ${image?.publish_status === 'public' ? 'public' : ''}`} style={{ position: 'absolute', top: 12, right: 12, padding: '4px 8px', background: 'rgba(0,0,0,0.6)', backdropFilter: 'blur(4px)', borderRadius: 6, fontSize: 10, color: image?.publish_status === 'public' ? 'var(--vault-gold)' : 'var(--vault-fg)' }}>{publishLabel(image?.publish_status)}</span>
              <div className="asset-info" style={{ padding: 16 }}>
                <div className="asset-title" style={{ fontSize: 14, fontWeight: 700, marginBottom: 4, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{task.title}</div>
                <div className="asset-meta" style={{ fontFamily: 'JetBrains Mono, monospace', fontSize: 11, color: 'var(--vault-muted)', display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                  <span>{taskTypeLabel(task.task_type)} · {task.quality}</span>
                  <span>{formatDate(task.created_at).slice(0, 10)}</span>
                </div>
                <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 14 }}>
                  <Button tone="ghost" onClick={() => setSelected(task)}>预览</Button>
                  <Button tone="ghost" disabled={!image} onClick={() => downloadFirst(task)}>下载</Button>
                  <Button tone="ghost" disabled={!image} busy={busyId === image?.id} onClick={() => void publishFirst(task)}>{image?.publish_status === 'reviewing' || image?.publish_status === 'public' ? '取消公开' : '申请公开'}</Button>
                  <Button tone="danger" busy={busyId === task.id} onClick={() => void deleteTask(task)}>隐藏</Button>
                </div>
              </div>
            </article>
          )
        })}
      </div>

      {selected ? (
        <Modal title={selected.title} onClose={() => setSelected(null)}>
          <div className="preview-drawer">
            <div className="preview-images">
              {selected.results.length ? selected.results.map((image) => <img key={image.id} src={image.url} alt={selected.title} />) : <EmptyState title="任务尚未完成" detail={`${selected.status} / ${selected.progress}%`} />}
            </div>
            <div className="preview-copy">
              <span className="status-pill">{selected.status}</span>
              <p>{selected.prompt}</p>
              <div className="meta-grid">
                <span>模型 <b>{selected.model_group}</b></span>
                <span>质量 <b>{selected.quality}</b></span>
                <span>比例 <b>{selected.aspect_ratio}</b></span>
                <span>费用 <b>{selected.estimate_points}</b></span>
              </div>
              <div className="action-row">
                <Button onClick={() => app.navigate('genpic')}>继续编辑</Button>
                <Button tone="ghost" disabled={!selected.results[0]} onClick={() => downloadFirst(selected)}>下载首图</Button>
                <Button tone="ghost" disabled={!selected.results[0]} busy={busyId === selected.results[0]?.id} onClick={() => void publishFirst(selected)}>{selected.results[0]?.publish_status === 'reviewing' || selected.results[0]?.publish_status === 'public' ? '取消公开' : '申请公开'}</Button>
                <Button tone="danger" busy={busyId === selected.id} onClick={() => void deleteTask(selected)}>隐藏任务</Button>
              </div>
            </div>
          </div>
        </Modal>
      ) : null}
    </div>
  )
}
