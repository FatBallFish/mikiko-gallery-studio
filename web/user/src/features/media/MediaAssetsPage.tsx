import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { CheckSquare, Download, FolderInput, Globe2, ListRestart, RefreshCw, Search, Trash2, Upload } from 'lucide-react'
import type { MediaAsset, MediaBatchAction, MediaType } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { ProjectSelector, useProjects } from '../../ProjectContext'
import { Button, EmptyState, useApp } from '../../components'
import { userHashForRoute } from '../../routeState'
import { MediaAssetCard } from './MediaAssetCard'
import { MediaPreviewDialog } from './MediaPreviewDialog'
import { buildMediaAssetQuery, mediaCreationActions, reconcileBatchSelection } from './mediaExperience'
import { MEDIA_ASSETS_CHANGED_EVENT } from './UploadTray'
import { pollGalleryExportJob } from '../../pages/galleryBatchActions'

type FilterState = {
  mediaType: '' | MediaType
  sourceType: string
  groupName: string
  status: string
  keyword: string
  sort: string
}

const initialFilters: FilterState = { mediaType: '', sourceType: '', groupName: '', status: '', keyword: '', sort: 'created_at:desc' }

export function MediaAssetsPage() {
  const app = useApp()
  const projects = useProjects()
  const [filters, setFilters] = useState(initialFilters)
  const [items, setItems] = useState<MediaAsset[]>([])
  const [nextCursor, setNextCursor] = useState('')
  const [selected, setSelected] = useState(new Set<string>())
  const [preview, setPreview] = useState<MediaAsset | null>(null)
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [refreshKey, setRefreshKey] = useState(0)
  const [targetProjectID, setTargetProjectID] = useState('')
  const requestVersion = useRef(0)

  useEffect(() => {
    requestVersion.current += 1
    setItems([])
    setNextCursor('')
    setSelected(new Set())
    setPreview(null)
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

  useEffect(() => { void load(false) }, [projects.selectedProjectID, filters.mediaType, filters.sourceType, filters.groupName, filters.status, filters.keyword, filters.sort, refreshKey])
  useEffect(() => {
    const refresh = (event: Event) => {
      const detail = (event as CustomEvent<{ projectID?: string }>).detail
      if (!detail?.projectID || detail.projectID === projects.selectedProjectID) setRefreshKey((value) => value + 1)
    }
    window.addEventListener(MEDIA_ASSETS_CHANGED_EVENT, refresh)
    return () => window.removeEventListener(MEDIA_ASSETS_CHANGED_EVENT, refresh)
  }, [projects.selectedProjectID])

  const groups = useMemo(() => Array.from(new Set(items.map((item) => item.group_name).filter(Boolean))).sort(), [items])
  const selectedItems = useMemo(() => items.filter((item) => selected.has(item.id)), [items, selected])
  const allSelected = items.length > 0 && items.every((item) => selected.has(item.id))
  const toggle = (asset: MediaAsset) => setSelected((current) => {
    const next = new Set(current)
    if (next.has(asset.id)) next.delete(asset.id)
    else next.add(asset.id)
    return next
  })
  const selectAll = () => setSelected(new Set(items.map((item) => item.id)))
  const invert = () => setSelected(new Set(items.filter((item) => !selected.has(item.id)).map((item) => item.id)))

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
      app.notify(failed ? 'error' : 'success', failed ? `${result.items.length - failed} 项成功，${failed} 项失败` : `已处理 ${result.items.length} 项资产`)
    } catch (caught) {
      app.notify('error', caught instanceof Error ? caught.message : '批量操作失败')
    } finally { setBusy(false) }
  }

  const groupSelected = () => {
    const groupName = window.prompt('设置分组名称')
    if (groupName !== null) void batch('group', { group_name: groupName.trim() })
  }
  const publishSelected = async () => {
    const images = selectedItems.filter((item) => item.media_type === 'image' && item.legacy_image_id)
    if (!images.length) { app.notify('error', '所选资产中没有可公开的历史图片'); return }
    setBusy(true)
    try {
      const result = await userApi.batchPublishGalleryImages(images.map((item) => item.legacy_image_id!), projects.selectedProjectID, true)
      const succeededMediaIDs = new Set(images.filter((item) => result.succeeded.some((entry) => entry.id === item.legacy_image_id)).map((item) => item.id))
      setSelected((current) => new Set(Array.from(current).filter((id) => !succeededMediaIDs.has(id))))
      app.notify(result.failed.length ? 'error' : 'success', result.failed.length ? `${result.failed.length} 张图片公开失败` : '已提交图片公开审核')
    } catch (caught) { app.notify('error', caught instanceof Error ? caught.message : '公开失败') } finally { setBusy(false) }
  }
  const continueCreation = (options: Parameters<typeof userHashForRoute>[1]) => {
    window.location.hash = userHashForRoute('genpic', options)
  }
  const patchAsset = (asset: MediaAsset) => {
    if (asset.project_id !== projects.selectedProjectID) {
      setItems((current) => current.filter((item) => item.id !== asset.id))
      setSelected((current) => new Set(Array.from(current).filter((id) => id !== asset.id)))
      setPreview(null)
      return
    }
    setItems((current) => current.map((item) => item.id === asset.id ? asset : item))
    setPreview(asset)
  }

  return (
    <main className="media-assets-page">
      <header className="media-assets-header">
        <div><p>MEDIA LIBRARY</p><h1>资产</h1></div>
        <div className="media-assets-header-actions"><ProjectSelector />{app.featureFlags.media_upload ? <Button tone="ghost" onClick={() => window.dispatchEvent(new Event('mgs:open-media-upload'))}><Upload size={16} />上传文件</Button> : null}<button type="button" className="media-refresh-button" title="刷新" aria-label="刷新资产" onClick={() => setRefreshKey((value) => value + 1)}><RefreshCw size={18} /></button></div>
      </header>
      <section className="media-filter-bar" aria-label="资产筛选">
        <label className="media-search"><Search size={16} /><input value={filters.keyword} placeholder="搜索资产" aria-label="搜索资产" onChange={(event) => setFilters((current) => ({ ...current, keyword: event.target.value }))} /></label>
        <select aria-label="媒体类型" value={filters.mediaType} onChange={(event) => setFilters((current) => ({ ...current, mediaType: event.target.value as FilterState['mediaType'] }))}><option value="">全部类型</option><option value="image">图片</option><option value="video">视频</option><option value="audio">音频</option></select>
        <select aria-label="来源" value={filters.sourceType} onChange={(event) => setFilters((current) => ({ ...current, sourceType: event.target.value }))}><option value="">全部来源</option><option value="generated">平台生成</option><option value="local_upload">本地上传</option></select>
        <select aria-label="分组" value={filters.groupName} onChange={(event) => setFilters((current) => ({ ...current, groupName: event.target.value }))}><option value="">全部分组</option>{groups.map((group) => <option key={group}>{group}</option>)}</select>
        <select aria-label="状态" value={filters.status} onChange={(event) => setFilters((current) => ({ ...current, status: event.target.value }))}><option value="">全部状态</option><option value="ready">可用</option><option value="processing">处理中</option><option value="failed">失败</option></select>
        <select aria-label="排序" value={filters.sort} onChange={(event) => setFilters((current) => ({ ...current, sort: event.target.value }))}><option value="created_at:desc">最新创建</option><option value="created_at:asc">最早创建</option><option value="name:asc">名称 A-Z</option><option value="file_size_bytes:desc">文件由大到小</option><option value="duration_ms:desc">时长由长到短</option></select>
        <button type="button" title="重置筛选" aria-label="重置筛选" onClick={() => setFilters(initialFilters)}><ListRestart size={17} /></button>
      </section>
      {error ? <section className="media-assets-error" role="alert"><span>{error}</span><Button tone="ghost" onClick={() => void load(false)}>重试</Button></section> : null}
      {!loading && !error && !items.length ? <EmptyState title="当前项目暂无资产" detail="" action={app.featureFlags.media_upload ? <Button onClick={() => window.dispatchEvent(new Event('mgs:open-media-upload'))}>上传文件</Button> : undefined} /> : null}
      <section className="media-assets-grid" aria-busy={loading}>{items.map((asset) => <MediaAssetCard key={asset.id} asset={asset} selected={selected.has(asset.id)} selectionMode={selected.size > 0} onSelect={toggle} onOpen={setPreview} onRetry={(item) => void userApi.retryMediaAssetProcessing(item.id).then(patchAsset)} />)}</section>
      {loading ? <p className="media-assets-loading" role="status">正在加载资产</p> : null}
      {nextCursor && !loading ? <div className="media-load-more"><Button tone="ghost" onClick={() => void load(true)}>加载更多</Button></div> : null}
      {items.length ? <button type="button" className="media-select-mode" onClick={allSelected ? () => setSelected(new Set()) : selectAll}><CheckSquare size={16} />{allSelected ? '取消全选' : '全选'}</button> : null}
      {selected.size ? <aside className="media-batch-toolbar" aria-label="批量工具">
        <strong>已选 {selected.size} 项</strong><button type="button" onClick={selectAll}>全选</button><button type="button" onClick={invert}>反选</button>
        <button type="button" disabled={busy} onClick={() => void batch('download')}><Download size={15} />下载</button>
        <button type="button" disabled={busy} onClick={groupSelected}><FolderInput size={15} />分组</button>
        <select aria-label="目标项目" value={targetProjectID} onChange={(event) => setTargetProjectID(event.target.value)}><option value="">转移到项目</option>{projects.projects.filter((project) => project.id !== projects.selectedProjectID).map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select>
        <button type="button" disabled={busy || !targetProjectID} onClick={() => void batch('transfer-project', { target_project_id: targetProjectID })}>转移</button>
        <button type="button" disabled={busy || !selectedItems.some((item) => item.media_type === 'image')} onClick={() => void publishSelected()}><Globe2 size={15} />公开图片</button>
        <button type="button" disabled={busy} className="is-danger" onClick={() => void batch('delete')}><Trash2 size={15} />删除</button>
      </aside> : null}
      {preview ? <MediaPreviewDialog asset={preview} projects={projects.projects} creationActions={mediaCreationActions(preview)} onClose={() => setPreview(null)} onChanged={patchAsset} onDeleted={(asset) => { setItems((current) => current.filter((item) => item.id !== asset.id)); setPreview(null) }} onContinue={continueCreation} /> : null}
    </main>
  )
}
