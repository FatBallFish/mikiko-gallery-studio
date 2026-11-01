import { useEffect, useRef, useState } from 'react'
import { Search, X } from 'lucide-react'
import type { MediaAsset, MediaType } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { MediaAssetCard } from '../media/MediaAssetCard'
import { MEDIA_ASSETS_CHANGED_EVENT } from '../media/UploadTray'
import { canvasAssetUnavailableReason, mergeCanvasAssetPage } from './canvasAssets'

export function CanvasAssetDrawer({ projectID, mediaType, onSelect, onClose }: { projectID: string; mediaType?: MediaType; onSelect: (asset: MediaAsset) => void; onClose: () => void }) {
  const [items, setItems] = useState<MediaAsset[]>([])
  const [keyword, setKeyword] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [nextCursor, setNextCursor] = useState('')
  const [refresh, setRefresh] = useState(0)
  const loadMoreRef = useRef<(() => void) | null>(null)

  useEffect(() => {
    let alive = true
    let pending = false
    let cursor = ''
    let loaded: MediaAsset[] = []
    let poll: ReturnType<typeof setTimeout> | undefined
    const load = async (append = false) => {
      if (pending || !projectID) return
      pending = true
      setLoading(true)
      setError('')
      try {
        const page = await userApi.listMediaAssets({ project_id: projectID, keyword: keyword.trim(), media_type: mediaType, cursor: append ? cursor : undefined, limit: 40 })
        if (!alive) return
        loaded = mergeCanvasAssetPage(append ? loaded : [], page.items.filter((asset) => asset.project_id === projectID && (!mediaType || asset.media_type === mediaType)))
        cursor = page.next_cursor ?? ''
        setItems(loaded)
        setNextCursor(cursor)
        schedulePoll()
      } catch (caught) {
        if (alive) setError(caught instanceof Error ? caught.message : '资产加载失败')
      } finally {
        pending = false
        if (alive) setLoading(false)
      }
    }
    // Refresh the loaded rows individually so processing updates preserve all pages.
    const schedulePoll = () => {
      clearTimeout(poll)
      const processing = loaded.filter((asset) => ['uploading', 'processing', 'ready_original'].includes(asset.status))
      if (!processing.length) return
      poll = setTimeout(() => {
        void Promise.allSettled(processing.map((asset) => userApi.getMediaAsset(asset.id))).then((results) => {
          if (!alive) return
          const updates = results.flatMap((result) => result.status === 'fulfilled' ? [result.value] : [])
          loaded = mergeCanvasAssetPage(loaded, updates).filter((asset) => asset.project_id === projectID && (!mediaType || asset.media_type === mediaType))
          setItems(loaded)
          schedulePoll()
        })
      }, 3000)
    }
    setItems([])
    setNextCursor('')
    loadMoreRef.current = () => { if (cursor) void load(true) }
    void load()
    return () => { alive = false; clearTimeout(poll); loadMoreRef.current = null }
  }, [keyword, mediaType, projectID, refresh])

  useEffect(() => {
    const changed = (event: Event) => {
      const detail = (event as CustomEvent<{ projectID?: string }>).detail
      if (!detail?.projectID || detail.projectID === projectID) setRefresh((value) => value + 1)
    }
    window.addEventListener(MEDIA_ASSETS_CHANGED_EVENT, changed)
    return () => window.removeEventListener(MEDIA_ASSETS_CHANGED_EVENT, changed)
  }, [projectID])

  const select = (asset: MediaAsset) => {
    const reason = canvasAssetUnavailableReason(asset)
    if (reason) { setError(reason); return }
    onSelect(asset)
  }
  return <aside className="canvas-asset-drawer" data-canvas-no-zoom aria-label="资产抽屉">
    <header><strong>{mediaType === 'image' ? '选择图片' : '添加资产'}</strong><button type="button" title="关闭资产抽屉" onClick={onClose}><X size={17} /></button></header>
    <label><Search size={15} /><input aria-label="搜索画布资产" value={keyword} placeholder="搜索资产" onChange={(event) => setKeyword(event.target.value)} /></label>
    <p>拖到画布，或点击添加</p>
    {error ? <span role="alert">{error}<button type="button" onClick={() => setRefresh((value) => value + 1)}>重新加载</button></span> : null}
    {loading ? <span role="status">正在加载资产</span> : null}
    {!loading && !error && !items.length ? <p>没有符合条件的资产</p> : null}
    <div className="canvas-asset-drawer-grid">
      {items.map((asset) => {
        const reason = canvasAssetUnavailableReason(asset)
        return <div key={asset.id} draggable={!reason} aria-disabled={Boolean(reason)} title={reason || undefined} onDragStart={(event) => {
          if (reason) { event.preventDefault(); setError(reason); return }
          event.dataTransfer.effectAllowed = 'copy'
          event.dataTransfer.setData('application/x-canvas-asset', JSON.stringify(asset))
        }}><MediaAssetCard asset={asset} selectionMode onSelect={select} />{reason ? <small>{reason}</small> : null}</div>
      })}
    </div>
    {nextCursor ? <button type="button" disabled={loading} onClick={() => loadMoreRef.current?.()}>{loading ? '正在加载' : '加载更多'}</button> : null}
  </aside>
}
