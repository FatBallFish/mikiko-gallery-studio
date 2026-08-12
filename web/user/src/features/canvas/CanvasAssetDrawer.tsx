import { useEffect, useState } from 'react'
import { Search, X } from 'lucide-react'
import type { MediaAsset } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { MediaAssetCard } from '../media/MediaAssetCard'

export function CanvasAssetDrawer({ projectID, onSelect, onClose }: { projectID: string; onSelect: (asset: MediaAsset) => void; onClose: () => void }) {
  const [items, setItems] = useState<MediaAsset[]>([])
  const [keyword, setKeyword] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let alive = true
    setLoading(true)
    setError('')
    void userApi.listMediaAssets({ project_id: projectID, status: 'ready', keyword: keyword.trim(), limit: 40 }).then((page) => {
      if (alive) setItems(page.items.filter((asset) => ['image', 'video', 'audio'].includes(asset.media_type)))
    }).catch((caught) => { if (alive) setError(caught instanceof Error ? caught.message : '资产加载失败') }).finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [keyword, projectID])

  return <aside className="canvas-asset-drawer" data-canvas-no-zoom aria-label="资产抽屉">
    <header><strong>添加资产</strong><button type="button" title="关闭资产抽屉" onClick={onClose}><X size={17} /></button></header>
    <label><Search size={15} /><input aria-label="搜索画布资产" value={keyword} placeholder="搜索资产" onChange={(event) => setKeyword(event.target.value)} /></label>
    <p>拖到画布，或点击添加</p>
    {error ? <span role="alert">{error}</span> : null}
    {loading ? <span role="status">正在加载资产</span> : null}
    <div className="canvas-asset-drawer-grid">
      {items.map((asset) => <div key={asset.id} draggable onDragStart={(event) => {
        event.dataTransfer.effectAllowed = 'copy'
        event.dataTransfer.setData('application/x-canvas-asset', JSON.stringify(asset))
      }}><MediaAssetCard asset={asset} selectionMode onSelect={onSelect} /></div>)}
    </div>
  </aside>
}
