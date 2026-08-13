import { useEffect, useRef, useState } from 'react'
import { AudioLines, Check, FileImage, Film, LoaderCircle, Play, RotateCcw } from 'lucide-react'
import type { MediaAccessProjection, MediaAsset } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { canHoverPreview, mediaAudioCoordinator, mediaHoverScheduler } from './mediaExperience'

type Props = {
  asset: MediaAsset
  selected?: boolean
  selectionMode?: boolean
  onSelect?: (asset: MediaAsset) => void
  onOpen?: (asset: MediaAsset) => void
  onRetry?: (asset: MediaAsset) => void
}

function useImagePreview(asset: MediaAsset) {
  const [access, setAccess] = useState<MediaAccessProjection | null>(null)
  const refreshed = useRef(false)
  useEffect(() => {
    let alive = true
    refreshed.current = false
    if (asset.media_type !== 'image' || (asset.status !== 'ready' && asset.status !== 'ready_original')) return undefined
    void userApi.getMediaAssetAccess(asset.id, 'thumbnail').then((next) => {
      if (alive) setAccess(next)
    }).catch(() => undefined)
    return () => { alive = false }
  }, [asset.id, asset.media_type, asset.status, asset.version])
  return {
    access,
    refreshOnce: async () => {
      if (refreshed.current) return false
      refreshed.current = true
      try {
        const next = await userApi.getMediaAssetAccess(asset.id, 'thumbnail')
        setAccess(next)
        return true
      } catch {
        return false
      }
    },
  }
}

export function MediaAssetCard({ asset, selected = false, selectionMode = false, onSelect, onOpen, onRetry }: Props) {
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

  return (
    <article data-media-asset-id={asset.id} className={`media-asset-card${selected ? ' is-selected' : ''}`} onMouseEnter={beginHover} onMouseLeave={endHover}>
      {onSelect && !selectionMode ? <button data-media-selection-control className="media-asset-select" type="button" aria-label={`${selected ? '取消选择' : '选择'} ${asset.name}`} title={selected ? '取消选择' : '选择'} onClick={() => onSelect(asset)}><Check size={15} /></button> : null}
      <button className="media-asset-stage" type="button" onClick={open} aria-label={`${selectionMode ? '选择' : '预览'} ${asset.name}`}>
        {asset.media_type === 'image' && imagePreview.access?.url ? <img src={imagePreview.access.url} alt="" loading="lazy" draggable={false} onError={() => void imagePreview.refreshOnce()} /> : null}
        {asset.media_type === 'video' && !hoverURL && posterURL ? <img src={posterURL} alt="" loading="lazy" draggable={false} /> : null}
        {asset.media_type === 'video' && hoverURL ? <video ref={videoRef} src={hoverURL} muted loop autoPlay playsInline preload="metadata" draggable={false} onError={() => void refreshHoverOnce()} /> : null}
        {asset.media_type === 'audio' && waveformURL ? <img src={waveformURL} alt="" loading="lazy" draggable={false} /> : null}
        {!imagePreview.access?.url && !posterURL && !hoverURL && !waveformURL ? (
          <span className={`media-asset-placeholder${asset.media_type === 'audio' ? ' is-audio' : ''}`} aria-hidden="true">
            {asset.status === 'processing' || asset.status === 'ready_original' ? <LoaderCircle className="animate-spin" /> : asset.media_type === 'video' ? <Film /> : asset.media_type === 'audio' ? <AudioLines /> : <FileImage />}
            {asset.media_type === 'audio' ? <span className="media-audio-waveform">{Array.from({ length: 24 }, (_, index) => <i key={index} />)}</span> : null}
          </span>
        ) : null}
        {selected ? <span className="media-asset-check"><Check size={15} /></span> : null}
        {asset.media_type === 'video' && !hoverURL ? <span className="media-asset-play"><Play size={16} fill="currentColor" /></span> : null}
      </button>
      <div className="media-asset-meta">
        <div className="min-w-0">
          <strong title={asset.name}>{asset.name}</strong>
          <span>{asset.media_type.toUpperCase()} · {formatBytes(asset.file_size_bytes)}</span>
        </div>
        {asset.media_type === 'audio' && asset.status === 'ready' ? (
          <button data-media-selection-control className="media-asset-audio" type="button" onClick={() => void toggleAudio()} aria-label={`播放 ${asset.name}`} title="播放">
            <Play size={15} fill="currentColor" />
          </button>
        ) : null}
        {asset.status === 'failed' && onRetry ? <button data-media-selection-control className="media-asset-audio" type="button" onClick={() => onRetry(asset)} aria-label="重试处理" title="重试处理"><RotateCcw size={15} /></button> : null}
      </div>
      {asset.group_name ? <span className="media-asset-group">{asset.group_name}</span> : null}
      {audioURL ? <audio ref={audioRef} src={audioURL} preload="metadata" onError={() => void refreshAudioOnce()} onPlay={() => mediaAudioCoordinator.activate(asset.id, () => audioRef.current?.pause())} onEnded={() => mediaAudioCoordinator.release(asset.id)} /> : null}
    </article>
  )
}

export function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const index = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)))
  return `${(bytes / (1024 ** index)).toFixed(index ? 1 : 0)} ${units[index]}`
}
