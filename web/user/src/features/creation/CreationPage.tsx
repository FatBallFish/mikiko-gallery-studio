import { useEffect, useState } from 'react'
import { Film, Image as ImageIcon } from 'lucide-react'
import { ImageCreationPanel } from './ImageCreationPanel'
import { VideoCreationPanel } from './VideoCreationPanel'
import { readCreationEntry, type CreationMediaMode } from './videoDraft'

const modeStorageKey = 'mikiko.creation.media-mode.v1'

export function CreationPage({ initialTaskId, initialMedia, initialAssetId, videoCreationEnabled = false }: { initialTaskId?: string; initialMedia?: CreationMediaMode; initialAssetId?: string; videoCreationEnabled?: boolean }) {
  const [mode, setMode] = useState<CreationMediaMode>(() => {
    if (initialTaskId && initialMedia === 'video') return 'video'
    if (!videoCreationEnabled || (initialTaskId && initialMedia !== 'video')) return 'image'
    const remembered = readMode()
    return readCreationEntry(initialMedia ? `?media=${initialMedia}${initialAssetId ? `&asset_id=${encodeURIComponent(initialAssetId)}` : ''}` : '', remembered).mode
  })

  useEffect(() => {
    if (!videoCreationEnabled && !(initialTaskId && initialMedia === 'video') || initialTaskId && initialMedia !== 'video') {
      setMode('image')
      return
    }
    if (initialMedia === 'video' || initialAssetId && videoCreationEnabled) setMode('video')
    else if (initialMedia === 'image') setMode('image')
  }, [initialAssetId, initialMedia, initialTaskId, videoCreationEnabled])

  function selectMode(next: CreationMediaMode) {
    setMode(next)
    try { window.localStorage.setItem(modeStorageKey, next) } catch { /* private storage may be unavailable */ }
  }

  return <div className="creation-page">
    <nav className="creation-mode-switch" aria-label="创作类型">
      <button type="button" aria-pressed={mode === 'image'} onClick={() => selectMode('image')}><ImageIcon size={17} />图片生成</button>
      {videoCreationEnabled ? <button type="button" aria-pressed={mode === 'video'} onClick={() => selectMode('video')}><Film size={17} />视频生成</button> : null}
    </nav>
    {mode === 'image' ? <ImageCreationPanel initialTaskId={initialMedia === 'video' ? undefined : initialTaskId} /> : <VideoCreationPanel initialTaskId={initialMedia === 'video' ? initialTaskId : undefined} initialAssetId={initialAssetId} />}
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
