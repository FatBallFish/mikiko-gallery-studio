import { useState } from 'react'
import { AlertCircle, Image, LoaderCircle, Play, Video } from 'lucide-react'
import './canvasMediaV2.css'

export type CanvasMediaNodeV2Data = { id: string; type: 'image_v2' | 'video_v2'; asset_id?: string; payload?: Record<string, unknown>; size?: { width: number; height: number } }
export type CanvasMediaRun = { status: string; stage?: string; progress?: number; progress_message?: string; error?: string }
export type CanvasMediaNodeV2Props = {
  node: CanvasMediaNodeV2Data
  title?: string
  mediaUrl?: string
  posterUrl?: string
  aspectRatio?: number | string
  selected?: boolean
  run?: CanvasMediaRun | null
  onRename?: (title: string) => void
  onPreview?: () => void
  onMediaDimensions?: (size: { width: number; height: number }) => void
}

export function canvasMediaRunLabel(run?: CanvasMediaRun | null) {
  if (!run) return ''
  const labels: Record<string, string> = { queued: '等待生成', pending: '等待生成', running: '正在生成', saving: '正在保存', succeeded: '生成完成', partial: '部分完成', failed: '生成失败', cancelled: '已取消', canceled: '已取消', attached: '生成完成', unplaced: '生成完成，等待放置到画布', submitted: '已提交', generating: '正在生成', downloading: '正在保存', provider: '正在生成', persisting: '正在保存', settling: '正在结算' }
  if (['attached', 'unplaced', 'canceled', 'cancelled', 'failed'].includes(run.status)) return labels[run.status]
  return run.progress_message || labels[run.stage ?? ''] || labels[run.status] || '处理中'
}

export function CanvasMediaNodeV2({ node, title, mediaUrl, posterUrl, aspectRatio, selected, run, onRename, onPreview, onMediaDimensions }: CanvasMediaNodeV2Props) {
  const [failedUrl, setFailedUrl] = useState('')
  const video = node.type === 'video_v2'
  const Icon = video ? Video : Image
  const name = title || String(node.payload?.title ?? '') || (video ? '视频' : '图片')
  const active = Boolean(run && !['succeeded', 'partial', 'failed', 'cancelled', 'canceled', 'attached', 'unplaced'].includes(run.status))
  const failed = run?.status === 'failed' || Boolean(mediaUrl && failedUrl === mediaUrl)
  const progress = run?.progress
  const realProgress = typeof progress === 'number' && Number.isFinite(progress) && progress >= 0 && progress <= 100
  const ratio = typeof aspectRatio === 'string' ? aspectRatio.replace(':', '/') : aspectRatio
  return <div className="canvas-media-v2" data-selected={selected || undefined} data-filled={Boolean(mediaUrl) || undefined}>
    <div className="canvas-media-v2__title" onPointerDown={(event) => event.stopPropagation()}>
      <Icon size={15} aria-hidden="true" />
      {onRename ? <input aria-label="媒体名称" key={`${node.id}:${name}`} defaultValue={name} maxLength={200} onBlur={(event) => { const value = event.target.value.trim(); if (value && value !== name) onRename(value); else event.target.value = name }} onKeyDown={(event) => { event.stopPropagation(); if (event.key === 'Enter') event.currentTarget.blur(); if (event.key === 'Escape') { event.currentTarget.value = name; event.currentTarget.blur() } }} /> : <span title={name}>{name}</span>}
    </div>
    <div className="canvas-media-v2__body" style={{ aspectRatio: ratio || (video ? '16/9' : '1/1') }}>
      {mediaUrl && !failed ? video ? <video src={mediaUrl} poster={posterUrl} muted preload="metadata" playsInline tabIndex={-1} aria-label={`${name}预览`} disablePictureInPicture onLoadedMetadata={(event) => { const media = event.currentTarget; if (media.videoWidth && media.videoHeight) onMediaDimensions?.({ width: media.videoWidth, height: media.videoHeight }) }} onError={() => setFailedUrl(mediaUrl)} /> : <img src={mediaUrl} alt={name} draggable={false} onLoad={(event) => onMediaDimensions?.({ width: event.currentTarget.naturalWidth, height: event.currentTarget.naturalHeight })} onError={() => setFailedUrl(mediaUrl)} /> : <div className="canvas-media-v2__placeholder" role="status" aria-live="polite">
        {active ? <LoaderCircle className="canvas-media-v2__spinner" size={26} aria-hidden="true" /> : failed ? <AlertCircle size={26} aria-hidden="true" /> : <Icon size={30} strokeWidth={1.2} aria-hidden="true" />}
        <span>{active || run?.status === 'unplaced' || run?.status === 'attached' ? canvasMediaRunLabel(run) : failed ? '生成或预览失败' : video ? '描述你的下一个镜头' : '让想象成为画面'}</span>
        {active && realProgress ? <progress value={progress} max={100} aria-label="生成进度" /> : null}
        {failed ? <small>{run?.error || '媒体暂时无法加载，请稍后重试'}</small> : !active ? <small>选择节点，开始创作</small> : null}
      </div>}
      {mediaUrl && !failed && selected && onPreview ? <button type="button" className="canvas-media-preview-button" data-canvas-interactive aria-label={video ? `播放并查看${name}` : `查看${name}详情`} onPointerDown={(event) => event.stopPropagation()} onClick={(event) => { event.stopPropagation(); onPreview() }}>{video ? <Play size={16} aria-hidden="true" /> : <Image size={16} aria-hidden="true" />}{video ? '播放 / 详情' : '查看详情'}</button> : null}
    </div>
  </div>
}
