import { useMemo, useState } from 'react'
import { Search, X } from 'lucide-react'
import type { CanvasNode } from './core/types'

const labels: Record<CanvasNode['type'], string> = {
  prompt: '提示词', image: '图片', video: '视频', audio: '音频', image_generation: '图片生成', video_generation: '视频生成', note: '便签',
}

export function CanvasNodeSearch({ nodes, onSelect, onClose }: { nodes: CanvasNode[]; onSelect: (node: CanvasNode) => void; onClose: () => void }) {
  const [query, setQuery] = useState('')
  const items = useMemo(() => {
    const key = query.trim().toLowerCase()
    return nodes.filter((node) => !key || `${node.id} ${labels[node.type]} ${String(node.payload?.text ?? '')}`.toLowerCase().includes(key)).slice(0, 30)
  }, [nodes, query])
  return <div className="canvas-search" role="dialog" aria-modal="true" aria-label="搜索画布节点" data-canvas-no-zoom>
    <div className="canvas-search-input"><Search size={16} /><input autoFocus value={query} placeholder="搜索节点" onChange={(event) => setQuery(event.target.value)} /><button type="button" title="关闭" onClick={onClose}><X size={16} /></button></div>
    <div className="canvas-search-results">{items.map((node) => <button key={node.id} type="button" onClick={() => onSelect(node)}><span>{labels[node.type]}</span><strong>{String(node.payload?.title ?? node.payload?.text ?? node.id)}</strong></button>)}</div>
  </div>
}
