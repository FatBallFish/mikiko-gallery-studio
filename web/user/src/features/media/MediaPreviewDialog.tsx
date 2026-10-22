import { useEffect, useRef, useState } from 'react'
import { Download, ExternalLink, FolderInput, Maximize2, Pause, Pencil, Play, Trash2, Volume2, X } from 'lucide-react'
import type { CreativeCanvas, MediaAccessProjection, MediaAsset, Project } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { cn } from '../../../../shared/classnames'
import { Drawer, ImageZoomPreview, useApp } from '../../components'
import { formatBytes } from './MediaAssetCard'
import { mediaAudioCoordinator } from './mediaExperience'
import type { MediaCreationAction } from './mediaExperience'
import { OverlayPortal } from '../../ui/overlayPortal'

const mediaTypeLabels = { image: '图片', video: '视频', audio: '音频' } as const

function assetStatus(asset: MediaAsset) {
  if (asset.status === 'processing') return { label: '处理中', tone: 'u-status--warn' }
  if (asset.status === 'failed') return { label: '失败', tone: 'u-status--bad' }
  if (asset.status === 'ready_original') return { label: '原图就绪', tone: 'u-status--warn' }
  if (asset.status === 'ready') return { label: '就绪', tone: 'u-status--ok' }
  return { label: asset.status, tone: '' }
}

function assetDimensions(asset: MediaAsset) {
  if (asset.width && asset.height) return `${asset.width} × ${asset.height}`
  if (asset.duration_ms) return formatSeconds(asset.duration_ms / 1000)
  return '-'
}

function assetDate(value: string) {
  if (!value) return '-'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleString('zh-CN', { hour12: false })
}

export function MediaPreviewDialog({ asset, projects, creationActions, onClose, onChanged, onDeleted, onContinue }: {
  asset: MediaAsset
  projects: Project[]
  creationActions: MediaCreationAction[]
  onClose: () => void
  onChanged: (asset: MediaAsset) => void
  onDeleted: (asset: MediaAsset) => void
  onContinue: (options: MediaCreationAction['options']) => void
}) {
  const app = useApp()
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
    try { onChanged(await userApi.updateMediaAsset(asset, { name })) }
    catch (caught) { app.notify('error', caught instanceof Error ? caught.message : '重命名失败') }
    finally { setBusy(false) }
  }
  const group = async () => {
    const groupName = window.prompt('分组名称', asset.group_name)?.trim()
    if (groupName === undefined) return
    setBusy(true)
    try { onChanged(await userApi.updateMediaAsset(asset, { group_name: groupName })) }
    catch (caught) { app.notify('error', caught instanceof Error ? caught.message : '设置分组失败') }
    finally { setBusy(false) }
  }
  const transfer = async (projectID: string) => {
    if (!projectID || projectID === asset.project_id) return
    setBusy(true)
    try { onChanged(await userApi.updateMediaAsset(asset, { project_id: projectID })) }
    catch (caught) { app.notify('error', caught instanceof Error ? caught.message : '转移项目失败') }
    finally { setBusy(false) }
  }
  const remove = async () => {
    if (!window.confirm(`删除“${asset.name}”？对象文件将由后台异步清理。`)) return
    setBusy(true)
    try { onDeleted(await userApi.deleteMediaAsset(asset)) }
    catch (caught) { app.notify('error', caught instanceof Error ? caught.message : '删除失败') }
    finally { setBusy(false) }
  }
  const download = async () => {
    try {
      const projection = await userApi.getMediaAssetAccess(asset.id, 'download')
      const link = globalThis.document.createElement('a')
      link.href = projection.url
      link.download = asset.name
      link.rel = 'noopener'
      link.click()
    } catch (caught) {
      app.notify('error', caught instanceof Error ? caught.message : '下载失败')
    }
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
    } catch (caught) {
      app.notify('error', caught instanceof Error ? caught.message : '添加到画布失败')
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
  const status = assetStatus(asset)
  const projectName = projects.find((project) => project.id === asset.project_id)?.name || '默认项目'
  const typeLabel = isDocumentAsset(asset) ? '文档' : mediaTypeLabels[asset.media_type] ?? '文件'
  const facts: Array<{ label: string; value: string }> = [
    { label: '媒体类型', value: typeLabel },
    { label: 'MIME', value: asset.mime_type },
    { label: '大小', value: formatBytes(asset.file_size_bytes) },
    { label: '尺寸 / 时长', value: assetDimensions(asset) },
    { label: '分组', value: asset.group_name || '未分组' },
    { label: '来源', value: asset.source_type || '-' },
    { label: '项目', value: projectName },
    { label: '状态', value: status.label },
  ]

  return (
    <>
      <Drawer open title={asset.name} onClose={onClose} className="media-detail-drawer">
        <div className="u-detail">
          <section className="u-detail__stage-col">
            <div className="u-detail__stage">
              <span className="u-detail__stage-label">
                <span className="u-chip u-chip--mono"><span className={cn('u-status', status.tone)}><span className="u-dot" aria-hidden="true" />{status.label}</span></span>
                <span className="u-chip u-chip--mono">{assetDimensions(asset)}</span>
              </span>

              {asset.media_type === 'image' && access?.url && !imageError ? (
                <button className="media-detail-media" type="button" onClick={() => setFullscreen(true)} aria-label="全屏查看图片">
                  <img key={access.url} src={access.url} alt={asset.name} onError={() => void refreshImageAccess()} />
                </button>
              ) : null}
              {asset.media_type === 'image' && imageError ? <span className="media-detail-message">图片暂时无法显示，请稍后重试或下载原件。</span> : null}

              {asset.media_type === 'video' ? (
                <button className="media-detail-media" type="button" onClick={() => setFullscreen(true)} aria-label="播放视频">
                  {poster?.url ? <img src={poster.url} alt={asset.name} /> : <span className="media-detail-message">缩略图生成中</span>}
                  <span className="media-detail-play"><Play size={24} fill="currentColor" aria-hidden="true" /></span>
                </button>
              ) : null}

              {asset.media_type === 'audio' ? (
                <div className="media-detail-audio">
                  <button type="button" className="media-detail-audio-toggle" onClick={() => void toggleAudio()} aria-label={audioState.playing ? '暂停音频' : '播放音频'}>
                    {audioState.playing ? <Pause size={22} /> : <Play size={22} fill="currentColor" />}
                  </button>
                  <Volume2 size={18} aria-hidden="true" />
                  <input
                    type="range"
                    min={0}
                    max={audioState.duration || 0}
                    step="0.1"
                    value={audioState.current}
                    onChange={(event) => {
                      const value = Number(event.target.value)
                      if (audioRef.current) audioRef.current.currentTime = value
                      setAudioState((current) => ({ ...current, current: value }))
                    }}
                    aria-label="音频播放进度"
                  />
                  <span className="u-meta">{formatSeconds(audioState.current)} / {formatSeconds(audioState.duration)}</span>
                  <audio
                    ref={audioRef}
                    src={access?.url}
                    preload="metadata"
                    onLoadedMetadata={(event) => setAudioState((current) => ({ ...current, duration: event.currentTarget.duration || 0 }))}
                    onTimeUpdate={(event) => setAudioState((current) => ({ ...current, current: event.currentTarget.currentTime }))}
                    onPlay={() => setAudioState((current) => ({ ...current, playing: true }))}
                    onPause={() => setAudioState((current) => ({ ...current, playing: false }))}
                    onEnded={() => { mediaAudioCoordinator.release(asset.id); setAudioState((current) => ({ ...current, playing: false })) }}
                  />
                </div>
              ) : null}

              {isDocumentAsset(asset) ? <DocumentPreview content={documentContent} error={documentError} /> : null}
              {!access && !poster && !documentContent && !isDocumentAsset(asset) && !isUnsupportedAsset(asset) ? <span className="media-detail-message">正在加载预览</span> : null}
              {isUnsupportedAsset(asset) ? <span className="media-detail-message">暂不支持此类型预览，请下载原件查看。</span> : null}
            </div>

            <div className="u-actions" style={{ justifyContent: 'space-between' }}>
              <div className="u-actions">
                <span className="u-chip u-chip--mono">{asset.mime_type}</span>
                <span className="u-chip u-chip--mono">{formatBytes(asset.file_size_bytes)}</span>
              </div>
              <div className="u-actions">
                {asset.media_type === 'image' && access?.url ? (
                  <button className="u-action" type="button" onClick={() => setFullscreen(true)}><Maximize2 size={14} strokeWidth={1.6} aria-hidden="true" />全屏查看</button>
                ) : null}
                {asset.media_type === 'video' ? (
                  <button className="u-action" type="button" onClick={() => setFullscreen(true)}><Play size={14} aria-hidden="true" />全屏播放</button>
                ) : null}
              </div>
            </div>
          </section>

          <section className="u-detail__info">
            <div className="u-detail__info-scroll">
              <div style={{ display: 'grid', gap: 7 }}>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                  <span className="u-chip u-chip--accent">{typeLabel}</span>
                  <span className="u-chip u-chip--mono">{asset.mime_type}</span>
                  <span className="u-chip u-chip--mono">{projectName}</span>
                </div>
                <h2 className="u-detail__title">{asset.name}</h2>
                <div className="u-meta">{assetDate(asset.created_at)} · {asset.source_type || '平台资产'}</div>
              </div>

              <div className="u-detail__section">
                <h4>文件信息</h4>
                <dl className="u-facts">
                  {facts.map((fact) => (
                    <div className="u-fact" key={fact.label}><dt>{fact.label}</dt><dd title={fact.value}>{fact.value}</dd></div>
                  ))}
                </dl>
              </div>

              <div className="u-detail__section">
                <h4>组织与归属</h4>
                <div className="media-detail-fields">
                  <label className="u-field">
                    <span className="u-label">转移项目</span>
                    <select className="u-select" value={asset.project_id} disabled={busy} onChange={(event) => void transfer(event.target.value)}>
                      {projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}
                    </select>
                  </label>
                  <label className="u-field">
                    <span className="u-label">添加到画布</span>
                    <select className="u-select" value={canvasID} disabled={busy || !canvases.length} onChange={(event) => setCanvasID(event.target.value)}>
                      <option value="">{canvases.length ? '选择画布' : '暂无画布'}</option>
                      {canvases.map((canvas) => <option key={canvas.id} value={canvas.id}>{canvas.name}</option>)}
                    </select>
                  </label>
                </div>
                <div className="u-actions">
                  <button className="u-btn u-btn--sm" type="button" disabled={busy || !canvasID} onClick={() => void addToCanvas()}>添加到画布</button>
                </div>
              </div>

              <div className="u-detail__section">
                <h4>更多操作</h4>
                <div className="u-actions">
                  {creationActions.map((action) => (
                    <button key={action.label} className="u-action" type="button" onClick={() => onContinue(action.options)}>
                      <ExternalLink size={14} strokeWidth={1.6} aria-hidden="true" />{action.label}
                    </button>
                  ))}
                  <button className="u-action" type="button" onClick={() => void rename()}><Pencil size={14} strokeWidth={1.6} aria-hidden="true" />重命名</button>
                  <button className="u-action" type="button" onClick={() => void group()}><FolderInput size={14} strokeWidth={1.6} aria-hidden="true" />设置分组</button>
                </div>
              </div>
            </div>

            <div className="u-detail__foot">
              <button className="u-btn u-btn--primary u-btn--sm" type="button" onClick={() => void download()}><Download size={14} strokeWidth={1.6} aria-hidden="true" />下载原件</button>
              <button className="u-btn u-btn--sm" type="button" disabled={busy} onClick={() => void rename()}>重命名</button>
              <button className="u-btn u-btn--sm" type="button" disabled={busy} onClick={() => void group()}>设置分组</button>
              <button className="u-btn u-btn--sm u-btn--danger" type="button" disabled={busy} onClick={() => void remove()}><Trash2 size={14} strokeWidth={1.6} aria-hidden="true" />删除</button>
            </div>
          </section>
        </div>
      </Drawer>
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
  if (error) return <span className="media-detail-message">{error}</span>
  if (content === null) return <span className="media-detail-message">正在加载文档</span>
  return <article className="media-detail-document">{renderSafeMarkdown(content)}</article>
}

function renderSafeMarkdown(content: string) {
  return content.split(/\r?\n/).map((line, index) => {
    const trimmed = line.trim()
    if (!trimmed) return <div key={index} className="media-detail-document-spacer" />
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