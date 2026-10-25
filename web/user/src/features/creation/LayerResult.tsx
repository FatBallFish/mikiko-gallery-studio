import { useMediaRefreshOnce } from '../../ui/mediaRefresh'
import type { ImageResult } from '../../../../shared/api-types'
import { useState } from 'react'
import type { ImageTask } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'

export function LayerResult({ task, accessToken }: { task: ImageTask; accessToken?: string }) {
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const layers = task.results.filter((result) => result.layer).sort((a, b) => a.layer!.z_index - b.layer!.z_index)
  if (!layers.length) return null
  async function download(archive: boolean) {
    setBusy(true); setError('')
    try {
      const response = archive ? await userApi.downloadImageLayers(task.id) : await userApi.getImageLayers(task.id)
      const blob = archive ? response as Blob : new Blob([JSON.stringify(response, null, 2)], { type: 'application/json' })
      if (!(blob instanceof Blob)) throw new Error('图层下载暂不可用')
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a'); link.href = url; link.download = `layers-${task.id}.${archive ? 'zip' : 'json'}`; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (reason) { setError(reason instanceof Error ? reason.message : '下载失败') } finally { setBusy(false) }
  }
  const base = layers.find((result) => result.layer?.is_base) ?? layers[0]
  return <section aria-label="拆分图层" style={{ padding: 16 }}><h3>图层预览</h3><div style={{ position: 'relative', aspectRatio: `${base.width} / ${base.height}`, maxWidth: 680, background: 'repeating-conic-gradient(#eee 0% 25%, #fff 0% 50%) 0 / 20px 20px' }}>{layers.map((result) => { const bbox = result.layer!.bounding_box?.absolute ?? [0, 0, base.width, base.height]; return <PrivateLayerImage key={result.id} result={result} accessToken={accessToken} alt={result.layer!.name ?? '图层'} style={{ position: 'absolute', left: `${bbox[0] / base.width * 100}%`, top: `${bbox[1] / base.height * 100}%`, width: `${(bbox[2] - bbox[0]) / base.width * 100}%`, height: `${(bbox[3] - bbox[1]) / base.height * 100}%`, objectFit: 'contain' }} /> })}</div><ol>{layers.map((result) => <li key={result.id}>{result.layer!.is_base ? '底图' : result.layer!.name ?? '图层'} · z={result.layer!.z_index} {result.layer!.description}</li>)}</ol><button type="button" disabled={busy} onClick={() => void download(false)}>下载图层清单</button><button type="button" disabled={busy} onClick={() => void download(true)}>下载图层 ZIP</button>{error ? <p role="alert">{error}</p> : null}</section>
}

function PrivateLayerImage({ result, accessToken, ...props }: { result: ImageResult; accessToken?: string } & Omit<React.ImgHTMLAttributes<HTMLImageElement>, 'src'>) {
  const { currentSrc, mediaRetryKey, markMediaLoaded, refreshFailedMedia } = useMediaRefreshOnce(
    userApi.imageAssetUrl(result.url, accessToken),
    async () => { const access = await userApi.refreshImageAccess(result.id, 'preview'); return userApi.imageAssetUrl(access.url, accessToken) },
    result.preview_expires_at,
    true,
  )
  return <img {...props} key={mediaRetryKey} src={currentSrc} onLoad={markMediaLoaded} onError={() => { void refreshFailedMedia() }} />
}
