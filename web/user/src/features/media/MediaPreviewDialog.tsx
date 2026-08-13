import { useEffect, useRef, useState } from 'react'
import { Download, ExternalLink, FolderInput, Maximize2, Pause, Pencil, Play, Trash2, Volume2, X } from 'lucide-react'
import type { CreativeCanvas, MediaAccessProjection, MediaAsset, Project } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { Button, ImageZoomPreview, Modal } from '../../components'
import { formatBytes } from './MediaAssetCard'
import { mediaAudioCoordinator } from './mediaExperience'
import type { MediaCreationAction } from './mediaExperience'
import { OverlayPortal } from '../../ui/overlayPortal'

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
  const [poster, setPoster] = useState<MediaAccessProjection | null>(null)
  const [documentContent, setDocumentContent] = useState<string | null>(null)
  const [documentError, setDocumentError] = useState('')
  const [imageError, setImageError] = useState(false)
  const imageFallbackTried = useRef(false)
  const [fullscreen, setFullscreen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [canvases, setCanvases] = useState<CreativeCanvas[]>([])
  const [canvasID, setCanvasID] = useState('')
  const audioRef = useRef<HTMLAudioElement | null>(null)
  const [audioState, setAudioState] = useState({ current: 0, duration: 0, playing: false })
  const audioPurpose = asset.media_type === 'audio' ? 'preview' : 'preview'

  useEffect(() => {
    let alive = true
    setAccess(null)
    setPoster(null)
    setDocumentContent(null)
    setDocumentError('')
    setImageError(false)
    imageFallbackTried.current = false
    setFullscreen(false)
    if (asset.media_type === 'image') {
      void userApi.getMediaAssetAccess(asset.id, 'thumbnail').then((next) => { if (alive) setAccess(next) }).catch(async () => {
        const next = await userApi.getMediaAssetAccess(asset.id, 'preview').catch(() => null)
        if (alive && next) setAccess(next)
      })
    } else if (asset.media_type === 'audio') {
      void userApi.getMediaAssetAccess(asset.id, audioPurpose).then((next) => { if (alive) setAccess(next) }).catch(() => undefined)
    } else if (asset.media_type === 'video') {
      void userApi.getMediaAssetAccess(asset.id, 'poster').then((next) => { if (alive) setPoster(next) }).catch(() => undefined)
    } else if (isDocumentAsset(asset)) {
      void userApi.getMediaAssetAccess(asset.id, 'content').then(async (next) => {
        try {
          const response = await fetch(next.url)
          if (!response.ok) throw new Error('文档内容加载失败')
          const text = await response.text()
          if (alive) setDocumentContent(text)
        } catch (error) {
          if (alive) setDocumentError(error instanceof Error ? error.message : '文档内容加载失败')
        }
      }).catch((error) => { if (alive) setDocumentError(error instanceof Error ? error.message : '文档访问失败') })
    }
    return () => { alive = false; audioRef.current?.pause(); mediaAudioCoordinator.release(asset.id) }
  }, [asset.id, asset.media_type, asset.version, audioPurpose])

  const refreshImageAccess = async () => {
    if (imageFallbackTried.current) return undefined
    imageFallbackTried.current = true
    const next = await userApi.getMediaAssetAccess(asset.id, 'preview').catch(() => null)
    if (next) { setAccess(next); setImageError(false); return next.url }
    setImageError(true)
    return undefined
  }

  useEffect(() => {
    let alive = true
    void userApi.listCanvases({ project_id: asset.project_id }).then((items) => {
      if (!alive) return
      setCanvases(items)
      setCanvasID((current) => current || items[0]?.id || '')
    }).catch(() => undefined)
    return () => { alive = false }
  }, [asset.project_id])

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
    const link = globalThis.document.createElement('a')
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
      id: crypto.randomUUID(), type: asset.media_type, asset_id: asset.id,
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
  const toggleAudio = async () => {
    const audio = audioRef.current
    if (!audio || !access) return
    mediaAudioCoordinator.activate(asset.id, () => { audio.pause(); setAudioState((current) => ({ ...current, playing: false })) })
    if (audio.paused) await audio.play().catch(() => undefined)
    else audio.pause()
    setAudioState((current) => ({ ...current, playing: !audio.paused }))
  }

  const imageFullScreen = fullscreen && asset.media_type === 'image' && access?.url
    ? <ImageZoomPreview url={access.url} alt={asset.name} onClose={() => setFullscreen(false)} onMediaRefresh={refreshImageAccess} /> : null
  return (
    <>
      <Modal title={asset.name} onClose={onClose} className="media-preview-dialog">
        <div className={`media-preview-stage${asset.media_type === 'video' || isDocumentAsset(asset) ? ' is-interactive' : ''}`}>
          {asset.media_type === 'image' && access?.url && !imageError ? <button className="media-preview-media-button" type="button" onClick={() => setFullscreen(true)} aria-label="全屏查看图片"><img key={access.url} src={access.url} alt={asset.name} onError={() => void refreshImageAccess()} /><span className="media-preview-expand"><Maximize2 size={16} /></span></button> : null}
          {asset.media_type === 'image' && imageError ? <span className="media-preview-unsupported">图片暂时无法显示，请稍后重试或下载原件。</span> : null}
          {asset.media_type === 'video' ? <button className="media-preview-media-button" type="button" onClick={() => setFullscreen(true)} aria-label="播放视频">{poster?.url ? <img src={poster.url} alt={asset.name} /> : <span className="media-preview-loading">缩略图生成中</span>}<span className="media-preview-play"><Play size={24} fill="currentColor" /></span></button> : null}
          {asset.media_type === 'audio' ? <div className="media-preview-audio"><button type="button" className="media-preview-audio-toggle" onClick={() => void toggleAudio()} aria-label={audioState.playing ? '暂停音频' : '播放音频'}>{audioState.playing ? <Pause size={22} /> : <Play size={22} fill="currentColor" />}</button><Volume2 size={18} /><input type="range" min={0} max={audioState.duration || 0} step="0.1" value={audioState.current} onChange={(event) => { const value = Number(event.target.value); if (audioRef.current) audioRef.current.currentTime = value; setAudioState((current) => ({ ...current, current: value })) }} aria-label="音频播放进度" /><span>{formatSeconds(audioState.current)} / {formatSeconds(audioState.duration)}</span><audio ref={audioRef} src={access?.url} preload="metadata" onLoadedMetadata={(event) => setAudioState((current) => ({ ...current, duration: event.currentTarget.duration || 0 }))} onTimeUpdate={(event) => setAudioState((current) => ({ ...current, current: event.currentTarget.currentTime }))} onPlay={() => setAudioState((current) => ({ ...current, playing: true }))} onPause={() => setAudioState((current) => ({ ...current, playing: false }))} onEnded={() => { mediaAudioCoordinator.release(asset.id); setAudioState((current) => ({ ...current, playing: false })) }} /></div> : null}
          {isDocumentAsset(asset) ? <DocumentPreview content={documentContent} error={documentError} /> : null}
          {!access && !poster && !documentContent && !isDocumentAsset(asset) && !isUnsupportedAsset(asset) ? <span className="media-preview-loading">正在加载预览</span> : null}
          {isUnsupportedAsset(asset) ? <span className="media-preview-unsupported">暂不支持此类型预览，请下载原件查看。</span> : null}
        </div>
        <dl className="media-preview-facts">
          <div><dt>类型</dt><dd>{asset.mime_type}</dd></div><div><dt>大小</dt><dd>{formatBytes(asset.file_size_bytes)}</dd></div>
          {asset.width && asset.height ? <div><dt>尺寸</dt><dd>{asset.width} × {asset.height}</dd></div> : null}
          {asset.duration_ms ? <div><dt>时长</dt><dd>{formatSeconds(asset.duration_ms / 1000)}</dd></div> : null}
        </dl>
        <div className="media-preview-actions">
          <Button tone="ghost" disabled={busy} onClick={() => void rename()}><Pencil size={16} />重命名</Button><Button tone="ghost" disabled={busy} onClick={() => void group()}><FolderInput size={16} />设置分组</Button>
          <label className="media-preview-project"><span>转移项目</span><select value={asset.project_id} disabled={busy} onChange={(event) => void transfer(event.target.value)}>{projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select></label>
          <Button tone="ghost" onClick={() => void download()}><Download size={16} />下载原件</Button>{creationActions.map((action) => <Button key={action.label} tone="ghost" onClick={() => onContinue(action.options)}><ExternalLink size={16} />{action.label}</Button>)}
          <label className="media-preview-project"><span>添加到画布</span><select value={canvasID} disabled={busy || !canvases.length} onChange={(event) => setCanvasID(event.target.value)}><option value="">{canvases.length ? '选择画布' : '暂无画布'}</option>{canvases.map((canvas) => <option key={canvas.id} value={canvas.id}>{canvas.name}</option>)}</select></label><Button tone="ghost" disabled={busy || !canvasID} onClick={() => void addToCanvas()}>添加</Button>
          <Button tone="danger" disabled={busy} onClick={() => void remove()}><Trash2 size={16} />删除</Button>
        </div>
      </Modal>
      {fullscreen && asset.media_type === 'video' ? <VideoFullscreenPreview asset={asset} onClose={() => setFullscreen(false)} /> : null}
      {imageFullScreen}
    </>
  )
}

function VideoFullscreenPreview({ asset, onClose }: { asset: MediaAsset; onClose: () => void }) {
  const [access, setAccess] = useState<MediaAccessProjection | null>(null)
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const shellRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => { void userApi.getMediaAssetAccess(asset.id, 'preview').then(setAccess).catch(() => undefined) }, [asset.id])
  useEffect(() => { window.setTimeout(() => videoRef.current?.play().catch(() => undefined), 0) }, [access?.url])
  return <OverlayPortal><div ref={shellRef} className="media-fullscreen-preview" role="dialog" aria-modal="true" aria-label={`预览视频 ${asset.name}`} onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}><button type="button" className="media-fullscreen-close" onClick={onClose} aria-label="关闭视频预览"><X size={20} /></button>{access?.url ? <video ref={videoRef} src={access.url} controls playsInline preload="metadata" /> : <span>视频预览生成中，请稍后重试</span>}</div></OverlayPortal>
}

function isDocumentAsset(asset: MediaAsset) {
  return asset.media_type === ('document' as MediaAsset['media_type']) || /^(text\/|text\/markdown$)/i.test(asset.mime_type)
}

function isUnsupportedAsset(asset: MediaAsset) {
  return asset.media_type !== 'image' && asset.media_type !== 'video' && asset.media_type !== 'audio' && !isDocumentAsset(asset)
}

function DocumentPreview({ content, error }: { content: string | null; error: string }) {
  if (error) return <span className="media-preview-unsupported">{error}</span>
  if (content === null) return <span className="media-preview-loading">正在加载文档</span>
  return <article className="media-document-content">{renderSafeMarkdown(content)}</article>
}

function renderSafeMarkdown(content: string) {
  return content.split(/\r?\n/).map((line, index) => {
    const trimmed = line.trim()
    if (!trimmed) return <div key={index} className="media-document-spacer" />
    if (trimmed.startsWith('### ')) return <h4 key={index}>{trimmed.slice(4)}</h4>
    if (trimmed.startsWith('## ')) return <h3 key={index}>{trimmed.slice(3)}</h3>
    if (trimmed.startsWith('# ')) return <h2 key={index}>{trimmed.slice(2)}</h2>
    if (trimmed.startsWith('- ')) return <li key={index}>{trimmed.slice(2)}</li>
    return <p key={index}>{trimmed.replace(/[`*_]/g, '')}</p>
  })
}

function formatSeconds(seconds: number) {
  if (!Number.isFinite(seconds) || seconds < 0) return '0:00'
  const whole = Math.floor(seconds)
  return `${Math.floor(whole / 60)}:${String(whole % 60).padStart(2, '0')}`
}
