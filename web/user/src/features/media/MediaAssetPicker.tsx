import { useEffect, useMemo, useState } from 'react'
import { Search } from 'lucide-react'
import type { MediaAsset, MediaType } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { Button, EmptyState, Modal } from '../../components'
import { useProjects } from '../../ProjectContext'
import { MediaAssetCard } from './MediaAssetCard'

export function MediaAssetPicker({ projectID, mediaTypes, allowedTypes, multiple = false, title = '选择资产', onConfirm, onSelect, onClose }: {
  projectID: string
  mediaTypes?: MediaType[]
  allowedTypes?: MediaType[]
  multiple?: boolean
  title?: string
  onConfirm?: (assets: MediaAsset[]) => void
  onSelect?: (asset: MediaAsset) => void
  onClose?: () => void
}) {
  const acceptedTypes = mediaTypes ?? allowedTypes ?? ['image', 'video', 'audio']
  const projects = useProjects()
  const [items, setItems] = useState<MediaAsset[]>([])
  const [selected, setSelected] = useState(new Set<string>())
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [projectFilter, setProjectFilter] = useState(projectID)
  const [keyword, setKeyword] = useState('')
  const [nextCursor, setNextCursor] = useState('')
  const acceptedKey = useMemo(() => acceptedTypes.join(','), [acceptedTypes])
  useEffect(() => {
    let alive = true
    setLoading(true)
    void userApi.listMediaAssets({ project_id: projectFilter || undefined, status: 'ready', keyword: keyword.trim(), limit: 40 }).then((page) => {
      if (alive) {
        setItems(page.items.filter((item) => acceptedTypes.includes(item.media_type)))
        setNextCursor(page.next_cursor ?? '')
        setSelected(new Set())
      }
    }).catch((caught) => { if (alive) setError(caught instanceof Error ? caught.message : '资产加载失败') }).finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [acceptedKey, keyword, projectFilter])

  const loadMore = async () => {
    if (!nextCursor || loading) return
    setLoading(true)
    try {
      const page = await userApi.listMediaAssets({ project_id: projectFilter || undefined, status: 'ready', keyword: keyword.trim(), cursor: nextCursor, limit: 40 })
      setItems((current) => [...current, ...page.items.filter((item) => acceptedTypes.includes(item.media_type) && !current.some((existing) => existing.id === item.id))])
      setNextCursor(page.next_cursor ?? '')
    } catch (caught) { setError(caught instanceof Error ? caught.message : '资产加载失败') } finally { setLoading(false) }
  }

  const toggle = (asset: MediaAsset) => {
    if (!multiple) {
      onSelect?.(asset)
      onConfirm?.([asset])
      return
    }
    setSelected((current) => {
      const next = new Set(current)
      if (next.has(asset.id)) next.delete(asset.id)
      else next.add(asset.id)
      return next
    })
  }

  const content = <>
      <div className="media-picker-filters">
        <label><Search size={15} /><input value={keyword} aria-label="搜索资产" placeholder="搜索资产" onChange={(event) => setKeyword(event.target.value)} /></label>
        <select aria-label="筛选项目" value={projectFilter} onChange={(event) => setProjectFilter(event.target.value)}><option value="">全部项目</option>{projects.projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select>
      </div>
      {error ? <p className="media-picker-error" role="alert">{error}</p> : null}
      {loading ? <p className="media-picker-loading" role="status">正在加载资产</p> : null}
      {!loading && !items.length ? <EmptyState title="暂无可用资产" detail="" /> : null}
      <div className="media-picker-grid">
        {items.map((asset) => <MediaAssetCard key={asset.id} asset={asset} selected={selected.has(asset.id)} selectionMode onSelect={toggle} />)}
      </div>
      {nextCursor ? <div className="media-picker-load-more"><Button tone="ghost" disabled={loading} onClick={() => void loadMore()}>加载更多</Button></div> : null}
      {multiple ? <footer className="media-picker-footer"><span>已选择 {selected.size} 项</span><Button disabled={!selected.size || !onConfirm} onClick={() => onConfirm?.(items.filter((item) => selected.has(item.id)))}>确认选择</Button></footer> : null}
    </>
  return onClose ? <Modal title={title} onClose={onClose} className="media-picker-dialog">{content}</Modal> : <section className="media-picker-inline" aria-label={title}>{content}</section>
}
