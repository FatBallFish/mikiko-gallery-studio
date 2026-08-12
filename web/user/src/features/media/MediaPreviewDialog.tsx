import { useEffect, useRef, useState } from 'react'
import { Download, ExternalLink, FolderInput, Pencil, Trash2 } from 'lucide-react'
import type { CreativeCanvas, MediaAccessProjection, MediaAsset, Project } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { Button, Modal } from '../../components'
import { formatBytes } from './MediaAssetCard'
import { mediaAudioCoordinator } from './mediaExperience'
import type { MediaCreationAction } from './mediaExperience'

export function MediaPreviewDialog({ asset, projects, creationActions, onClose, onChanged, onDeleted, onContinue }: {
  asset: MediaAsset
  projects: Project[]
  creationActions: MediaCreationAction[]
  onClose: () => void
  onChanged: (asset: MediaAsset) => void
  onDeleted: (asset: MediaAsset) => void
  onContinue: (options: MediaCreationAction['options']) => void
}) {
  const [access, setAccess] = useState<MediaAccessProjection | null>(null)
  const refreshed = useRef(false)
  const [busy, setBusy] = useState(false)
  const [canvases, setCanvases] = useState<CreativeCanvas[]>([])
  const [canvasID, setCanvasID] = useState('')
  const audioRef = useRef<HTMLAudioElement | null>(null)
  useEffect(() => {
    let alive = true
    refreshed.current = false
    void userApi.getMediaAssetAccess(asset.id, 'preview').then((next) => { if (alive) setAccess(next) }).catch(() => undefined)
    return () => { alive = false; mediaAudioCoordinator.release(asset.id) }
  }, [asset.id, asset.version])
  useEffect(() => {
    let alive = true
    void userApi.listCanvases({ project_id: asset.project_id }).then((items) => {
      if (!alive) return
      setCanvases(items)
      setCanvasID((current) => current || items[0]?.id || '')
    }).catch(() => undefined)
    return () => { alive = false }
  }, [asset.project_id])
  const refreshAccessOnce = async () => {
    if (refreshed.current) return
    refreshed.current = true
    const next = await userApi.getMediaAssetAccess(asset.id, 'preview').catch(() => null)
    if (next) setAccess(next)
  }

  const rename = async () => {
    const name = window.prompt('资产名称', asset.name)?.trim()
    if (!name || name === asset.name) return
    setBusy(true)
    try { onChanged(await userApi.updateMediaAsset(asset, { name })) } finally { setBusy(false) }
  }
  const group = async () => {
    const groupName = window.prompt('分组名称', asset.group_name)?.trim()
    if (groupName === undefined) return
    setBusy(true)
    try { onChanged(await userApi.updateMediaAsset(asset, { group_name: groupName })) } finally { setBusy(false) }
  }
  const transfer = async (projectID: string) => {
    if (!projectID || projectID === asset.project_id) return
    setBusy(true)
    try { onChanged(await userApi.updateMediaAsset(asset, { project_id: projectID })) } finally { setBusy(false) }
  }
  const remove = async () => {
    if (!window.confirm(`删除“${asset.name}”？对象文件将由后台异步清理。`)) return
    setBusy(true)
    try { onDeleted(await userApi.deleteMediaAsset(asset)) } finally { setBusy(false) }
  }
  const download = async () => {
    const projection = await userApi.getMediaAssetAccess(asset.id, 'download')
    const link = document.createElement('a')
    link.href = projection.url
    link.download = asset.name
    link.rel = 'noopener'
    link.click()
  }
  const addToCanvas = async () => {
    const canvas = canvases.find((item) => item.id === canvasID)
    if (!canvas) return
    const count = canvas.document.nodes.length
    const node = {
      id: crypto.randomUUID(),
      type: asset.media_type,
      asset_id: asset.id,
      position: { x: 80 + (count % 4) * 280, y: 80 + Math.floor(count / 4) * 220 },
      size: { width: 240, height: asset.media_type === 'audio' ? 120 : 180 },
      payload: { title: asset.name, mime_type: asset.mime_type },
    } as const
    setBusy(true)
    try {
      const updated = await userApi.saveCanvasDocument(canvas.id, canvas.revision, { ...canvas.document, nodes: [...canvas.document.nodes, node] })
      setCanvases((current) => current.map((item) => item.id === updated.id ? updated : item))
    } finally { setBusy(false) }
  }

  return (
    <Modal title={asset.name} onClose={onClose} className="media-preview-dialog">
      <div className="media-preview-stage">
        {!access ? <span className="media-preview-loading">正在加载预览</span> : asset.media_type === 'image' ? <img src={access.url} alt={asset.name} onError={() => void refreshAccessOnce()} /> : asset.media_type === 'video' ? <video src={access.url} controls playsInline preload="metadata" onError={() => void refreshAccessOnce()} /> : <audio ref={audioRef} src={access.url} controls preload="metadata" onError={() => void refreshAccessOnce()} onPlay={() => mediaAudioCoordinator.activate(asset.id, () => audioRef.current?.pause())} />}
      </div>
      <dl className="media-preview-facts">
        <div><dt>类型</dt><dd>{asset.mime_type}</dd></div>
        <div><dt>大小</dt><dd>{formatBytes(asset.file_size_bytes)}</dd></div>
        {asset.width && asset.height ? <div><dt>尺寸</dt><dd>{asset.width} × {asset.height}</dd></div> : null}
        {asset.duration_ms ? <div><dt>时长</dt><dd>{(asset.duration_ms / 1000).toFixed(1)} 秒</dd></div> : null}
      </dl>
      <div className="media-preview-actions">
        <Button tone="ghost" disabled={busy} onClick={() => void rename()}><Pencil size={16} />重命名</Button>
        <Button tone="ghost" disabled={busy} onClick={() => void group()}><FolderInput size={16} />设置分组</Button>
        <label className="media-preview-project"><span>转移项目</span><select value={asset.project_id} disabled={busy} onChange={(event) => void transfer(event.target.value)}>{projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select></label>
        <Button tone="ghost" onClick={() => void download()}><Download size={16} />下载原件</Button>
        {creationActions.map((action) => <Button key={action.label} tone="ghost" onClick={() => onContinue(action.options)}><ExternalLink size={16} />{action.label}</Button>)}
        <label className="media-preview-project"><span>添加到画布</span><select value={canvasID} disabled={busy || !canvases.length} onChange={(event) => setCanvasID(event.target.value)}><option value="">{canvases.length ? '选择画布' : '暂无画布'}</option>{canvases.map((canvas) => <option key={canvas.id} value={canvas.id}>{canvas.name}</option>)}</select></label>
        <Button tone="ghost" disabled={busy || !canvasID} onClick={() => void addToCanvas()}>添加</Button>
        <Button tone="danger" disabled={busy} onClick={() => void remove()}><Trash2 size={16} />删除</Button>
      </div>
    </Modal>
  )
}
