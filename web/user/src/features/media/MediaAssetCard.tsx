import { useEffect, useRef, useState, type ReactNode } from 'react'
import { AudioLines, Check, FileImage, Film, LoaderCircle, Pause, Play, RotateCcw } from 'lucide-react'
import type { MediaAccessProjection, MediaAsset } from '../../../../shared/api-types'
import { cn } from '../../../../shared/classnames'
import { userApi } from '../../../../shared/user-api'
import { formatDate, useApp } from '../../components'
import { canHoverPreview, mediaAssetMetadata, mediaAudioCoordinator, mediaHoverScheduler } from './mediaExperience'

type Props = {
  asset: MediaAsset
  selected?: boolean
  selectionMode?: boolean
  onSelect?: (asset: MediaAsset) => void
  onOpen?: (asset: MediaAsset) => void
  onRetry?: (asset: MediaAsset) => void
  footer?: ReactNode
}

const mediaTypeLabels = { image: '图片', video: '视频', audio: '音频' } as const

function useImagePreview(asset: MediaAsset) {
  const [access, setAccess] = useState<MediaAccessProjection | null>(null)
  const [purpose, setPurpose] = useState<'thumbnail' | 'preview'>('thumbnail')
  const refreshed = useRef(false)
  useEffect(() => {
    let alive = true
    refreshed.current = false
    if (asset.media_type !== 'image' || (asset.status !== 'ready' && asset.status !== 'ready_original')) return undefined
    void userApi.getMediaAssetAccess(asset.id, 'thumbnail').then((next) => {
      if (alive) setAccess(next)
    }).catch(async () => {
      if (refreshed.current) return
      refreshed.current = true
      const next = await userApi.getMediaAssetAccess(asset.id, 'preview').catch(() => null)
      if (alive && next) {
        setPurpose('preview')
        setAccess(next)
      }
    })
    return () => { alive = false }
  }, [asset.id, asset.media_type, asset.status, asset.version])
  return {
    access,
    purpose,
    refreshOnce: async () => {
      if (refreshed.current) return false
      refreshed.current = true
      try {
        setPurpose('preview')
        const next = await userApi.getMediaAssetAccess(asset.id, 'preview')
        setAccess(next)
        return true
      } catch {
        return false
      }
    },
  }
}

function mediaStatusBadge(asset: MediaAsset) {
  if (asset.status === 'processing' || asset.status === 'ready_original') return { label: '处理中', tone: 'u-status--warn' }
  if (asset.status === 'failed') return { label: '失败', tone: 'u-status--bad' }
  return null
}

export function MediaAssetCard({ asset, selected = false, selectionMode = false, onSelect, onOpen, onRetry, footer }: Props) {
  const app = useApp()
  const imagePreview = useImagePreview(asset)
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const audioRef = useRef<HTMLAudioElement | null>(null)
  const hoverRelease = useRef<null | (() => void)>(null)
  const hoverController = useRef<AbortController | null>(null)
  const hoverRefreshed = useRef(false)
  const audioRefreshed = useRef(false)
  const [hoverURL, setHoverURL] = useState('')
  const [audioURL, setAudioURL] = useState('')
  const [posterURL, setPosterURL] = useState('')
  const [waveformURL, setWaveformURL] = useState('')
  const [audioPlaying, setAudioPlaying] = useState(false)

  useEffect(() => {
    let alive = true
    const purpose = asset.media_type === 'video' ? 'poster' : asset.media_type === 'audio' ? 'waveform' : null
    if (!purpose || asset.status !== 'ready') return undefined
    void userApi.getMediaAssetAccess(asset.id, purpose).then((access) => {
      if (!alive) return
      if (purpose === 'poster') setPosterURL(access.url)
      else setWaveformURL(access.url)
    }).catch(() => undefined)
    return () => { alive = false }
  }, [asset.id, asset.media_type, asset.status, asset.version])

  useEffect(() => () => {
    hoverRelease.current?.()
    hoverController.current?.abort()
    mediaAudioCoordinator.release(asset.id)
  }, [asset.id])

  const open = () => selectionMode ? onSelect?.(asset) : onOpen?.(asset)
  const beginHover = () => {
    if (asset.media_type !== 'video' || asset.status !== 'ready' || !canHoverPreview()) return
    hoverRelease.current?.()
    hoverRefreshed.current = false
    hoverRelease.current = mediaHoverScheduler.schedule(asset.id, () => {
      const controller = new AbortController()
      hoverController.current = controller
      void userApi.getMediaAssetAccess(asset.id, 'hover', controller.signal).then((access) => {
        if (!controller.signal.aborted) setHoverURL(access.url)
      }).catch(() => undefined)
    })
  }
  const endHover = () => {
    hoverRelease.current?.()
    hoverRelease.current = null
    hoverController.current?.abort()
    hoverController.current = null
    videoRef.current?.pause()
    setHoverURL('')
  }
  const refreshHoverOnce = async () => {
    if (hoverRefreshed.current) return
    hoverRefreshed.current = true
    const access = await userApi.getMediaAssetAccess(asset.id, 'hover').catch(() => null)
    if (access) setHoverURL(access.url)
  }
  const refreshAudioOnce = async () => {
    if (audioRefreshed.current) return
    audioRefreshed.current = true
    const access = await userApi.getMediaAssetAccess(asset.id, 'preview').catch(() => null)
    if (access) setAudioURL(access.url)
  }
  const toggleAudio = async () => {
    if (!audioURL) {
      audioRefreshed.current = false
      const access = await userApi.getMediaAssetAccess(asset.id, 'preview')
      setAudioURL(access.url)
      requestAnimationFrame(() => void audioRef.current?.play())
      return
    }
    const player = audioRef.current
    if (!player) return
    if (player.paused) {
      mediaAudioCoordinator.activate(asset.id, () => player.pause())
      await player.play()
    } else {
      player.pause()
      mediaAudioCoordinator.release(asset.id)
    }
  }

  const statusBadge = mediaStatusBadge(asset)
  const typeLabel = mediaTypeLabels[asset.media_type] ?? '文件'
  const meta = mediaAssetMetadata(asset)
  const hasPreview = Boolean(imagePreview.access?.url || posterURL || hoverURL || waveformURL)

  return (
    <article
      data-media-asset-id={asset.id}
      className={cn('u-card gl-card', selected && 'is-selected')}
    >
      {onSelect ? (
        <button
          data-media-selection-control
          className="gl-check"
          type="button"
          aria-label={`${selected ? '取消选择' : '选择'} ${asset.name}`}
          aria-pressed={selected}
          title={selected ? '取消选择' : '选择'}
          onClick={() => onSelect(asset)}
        >
          {selected ? <Check size={13} strokeWidth={2.5} aria-hidden="true" /> : null}
        </button>
      ) : null}
      <button className="u-card__media gl-card__stage" type="button" onMouseEnter={beginHover} onMouseLeave={endHover} onClick={open} aria-label={`${selectionMode ? (selected ? '取消选择' : '选择') : '预览'} ${asset.name}`}>
        {asset.media_type === 'image' && imagePreview.access?.url ? <img key={`${imagePreview.purpose}:${imagePreview.access.url}`} src={userApi.mediaAssetUrl(imagePreview.access.url, app.session?.token)} alt="" loading="lazy" draggable={false} onError={() => void imagePreview.refreshOnce()} /> : null}
        {asset.media_type === 'video' && !hoverURL && posterURL ? <img src={userApi.mediaAssetUrl(posterURL, app.session?.token)} alt="" loading="lazy" draggable={false} /> : null}
        {asset.media_type === 'video' && hoverURL ? <video ref={videoRef} src={userApi.mediaAssetUrl(hoverURL, app.session?.token)} muted loop autoPlay playsInline preload="metadata" draggable={false} onError={() => void refreshHoverOnce()} /> : null}
        {asset.media_type === 'audio' && waveformURL ? <img src={userApi.mediaAssetUrl(waveformURL, app.session?.token)} alt="" loading="lazy" draggable={false} /> : null}
        {!hasPreview ? (
          <span className={cn('gl-card__placeholder', asset.media_type === 'audio' && 'is-audio')} aria-hidden="true">
            {asset.status === 'processing' || asset.status === 'ready_original' ? <LoaderCircle className="animate-spin" /> : asset.media_type === 'video' ? <Film /> : asset.media_type === 'audio' ? <AudioLines /> : <FileImage />}
            {asset.media_type === 'audio' ? <span className="gl-card__wave">{Array.from({ length: 24 }, (_, index) => <i key={index} />)}</span> : null}
          </span>
        ) : null}
        <span className="u-card__tag">{typeLabel}</span>
        {asset.media_type === 'video' && !hoverURL ? <span className="u-play"><span><Play size={18} fill="currentColor" aria-hidden="true" /></span></span> : null}
        {statusBadge ? (
          <span className="gl-status">
            <span className="u-chip"><span className={cn('u-status', statusBadge.tone)}><span className="u-dot" aria-hidden="true" />{statusBadge.label}</span></span>
          </span>
        ) : null}
      </button>
      <div className="u-card__body">
        <div className="gl-card__head">
          <div className="u-card__title" title={asset.name}>{asset.name}</div>
          {asset.media_type === 'audio' && asset.status === 'ready' ? (
            <button
              data-media-selection-control
              className="gl-card__action"
              type="button"
              onClick={() => void toggleAudio()}
              aria-label={`${audioPlaying ? '暂停' : '播放'} ${asset.name}`}
              title={audioPlaying ? '暂停' : '播放'}
            >
              {audioPlaying ? <Pause size={14} fill="currentColor" aria-hidden="true" /> : <Play size={14} fill="currentColor" aria-hidden="true" />}
            </button>
          ) : null}
          {asset.status === 'failed' && onRetry ? (
            <button data-media-selection-control className="gl-card__action" type="button" onClick={() => onRetry(asset)} aria-label="重试处理" title="重试处理">
              <RotateCcw size={14} aria-hidden="true" />
            </button>
          ) : null}
        </div>
        <div className="u-card__meta gl-card__meta">
          <span title={meta}>{meta || typeLabel}</span>
          <span>{formatDate(asset.created_at)}</span>
        </div>
      </div>
      {footer || asset.group_name ? <footer className="gl-card__footer" data-media-selection-control>{footer}{asset.group_name ? <span className="media-asset-group" title={asset.group_name}>{asset.group_name}</span> : null}</footer> : null}
      {audioURL ? (
        <audio
          ref={audioRef}
          src={userApi.mediaAssetUrl(audioURL, app.session?.token)}
          preload="metadata"
          onError={() => void refreshAudioOnce()}
          onPlay={() => { setAudioPlaying(true); mediaAudioCoordinator.activate(asset.id, () => audioRef.current?.pause()) }}
          onPause={() => setAudioPlaying(false)}
          onEnded={() => { setAudioPlaying(false); mediaAudioCoordinator.release(asset.id) }}
        />
      ) : null}
    </article>
  )
}

export function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const index = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)))
  return `${(bytes / (1024 ** index)).toFixed(index ? 1 : 0)} ${units[index]}`
}