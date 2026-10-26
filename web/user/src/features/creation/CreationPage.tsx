import { creationReuseEvent } from '../media/mediaCreationNavigation'
import { parseUserHashState } from '../../routeState'
import { useEffect, useState } from 'react'
import { Film, Image as ImageIcon } from 'lucide-react'
import { cn } from '../../../../shared/classnames'
import { ImageCreationPanel } from './ImageCreationPanel'
import { VideoCreationPanel } from './VideoCreationPanel'
import { readCreationEntry, type CreationMediaMode } from './videoDraft'
import './creation-redesign.css'

const modeStorageKey = 'mikiko.creation.media-mode.v1'

export function CreationPage({ initialTaskId, initialMedia, initialAssetId, videoCreationEnabled = false }: { initialTaskId?: string; initialMedia?: CreationMediaMode; initialAssetId?: string; videoCreationEnabled?: boolean }) {
  const [mode, setMode] = useState<CreationMediaMode>(() => {
    if (initialTaskId && initialMedia === 'video') return 'video'
    if (!videoCreationEnabled || (initialTaskId && initialMedia !== 'video')) return 'image'
    const remembered = readMode()
    if (initialMedia === 'image') return 'image'
    return readCreationEntry(initialMedia ? `?media=${initialMedia}${initialAssetId ? `&asset_id=${encodeURIComponent(initialAssetId)}` : ''}` : '', remembered).mode
  })

  useEffect(() => {
    if (!videoCreationEnabled && !(initialTaskId && initialMedia === 'video') || initialTaskId && initialMedia !== 'video') {
      setMode('image')
      return
    }
    if (initialMedia === 'video' || initialMedia !== 'image' && initialAssetId && videoCreationEnabled) setMode('video')
    else if (initialMedia === 'image') setMode('image')
  }, [initialAssetId, initialMedia, initialTaskId, videoCreationEnabled])

  const [videoEntryRevision, setVideoEntryRevision] = useState(0)
  useEffect(() => {
    const restore = () => {
      const entry = parseUserHashState(window.location.hash)
      if (entry.media === 'video') { setMode('video'); setVideoEntryRevision((value) => value + 1) }
      else if (entry.media === 'image') setMode('image')
    }
    window.addEventListener(creationReuseEvent, restore)
    return () => window.removeEventListener(creationReuseEvent, restore)
  }, [])

  function selectMode(next: CreationMediaMode) {
    setMode(next)
    try { window.localStorage.setItem(modeStorageKey, next) } catch { /* private storage may be unavailable */ }
  }

  return <div className="creation-page">
    <div className="cr-switch">
      <div className="u-segment" role="group" aria-label="创作类型">
        <button
          type="button"
          className={cn(mode === 'image' && 'is-active')}
          aria-pressed={mode === 'image'}
          onClick={() => selectMode('image')}
        >
          <ImageIcon size={16} strokeWidth={1.8} aria-hidden="true" />
          图片生成
        </button>
        {videoCreationEnabled ? (
          <button
            type="button"
            className={cn(mode === 'video' && 'is-active')}
            aria-pressed={mode === 'video'}
            onClick={() => selectMode('video')}
          >
            <Film size={16} strokeWidth={1.8} aria-hidden="true" />
            视频生成
          </button>
        ) : null}
      </div>
    </div>
    {mode === 'image' ? <ImageCreationPanel initialTaskId={initialMedia === 'video' ? undefined : initialTaskId} /> : <VideoCreationPanel key={videoEntryRevision} initialTaskId={initialMedia === 'video' ? initialTaskId : undefined} initialAssetId={initialAssetId} />}
  </div>
}

function readMode(): CreationMediaMode | null {
  try {
    const value = window.localStorage.getItem(modeStorageKey)
    return value === 'image' || value === 'video' ? value : null
  } catch {
    return null
  }
}