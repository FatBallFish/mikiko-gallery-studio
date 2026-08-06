import { existsSync, readFileSync } from 'node:fs'
import { createElement } from 'react'
import { renderToString } from 'react-dom/server'

const modelURL = new URL('./ui/imageMediaModel.ts', import.meta.url)
if (!existsSync(modelURL)) {
  throw new Error('image zoom needs an executable per-image loading and retry model')
}

const { imageMediaTransition, initialImageMediaState } = await import('./ui/imageMediaModel')
const { ImageMediaFallback } = await import('./components')

let media = initialImageMediaState('/broken-a.png')
media = imageMediaTransition(media, { type: 'error' })
if (media.status !== 'error') throw new Error('broken full images must enter a local error state')
media = imageMediaTransition(media, { type: 'retry' })
if (media.status !== 'loading' || media.attempt !== 1) {
  throw new Error(`retry must remount the same image in loading state, got ${JSON.stringify(media)}`)
}
media = imageMediaTransition(media, { type: 'reset', url: '/broken-b.png' })
if (media.url !== '/broken-b.png' || media.status !== 'loading' || media.attempt !== 0) {
  throw new Error(`changing images must reset load and retry state, got ${JSON.stringify(media)}`)
}

const fallbackSSR = renderToString(createElement(ImageMediaFallback, { onRetry: () => undefined }))
if (!fallbackSSR.includes('role="alert"') || !fallbackSSR.includes('重试加载')) {
  throw new Error(`lightbox error fallback must remain visible and actionable in SSR, got ${fallbackSSR}`)
}

const source = readFileSync(new URL('./components.tsx', import.meta.url), 'utf8')
const zoomStart = source.indexOf('function ImageZoomViewer')
const zoomEnd = source.indexOf('\nexport function PublicDetailIcon', zoomStart)
const zoomSource = source.slice(zoomStart, zoomEnd)

if (!source.includes('zoomBackdrop:') || !source.includes('motion-reduce:animate-none')) {
  throw new Error('zoom entrance animation must stop under reduced motion')
}
for (const contract of [
  'useImageMediaState(',
  'useMediaRefreshOnce(image.url, image.onMediaRefresh, image.mediaExpiresAt, true)',
  'markMediaLoaded(); media.markLoaded()',
  'if (!refreshed) media.markError()',
  '<ImageMediaFallback onRetry={() => { resetMediaRefresh(); media.retry() }}',
  'src={currentSrc}',
]) {
  if (!zoomSource.includes(contract)) {
    throw new Error(`zoom media must expose ${contract}`)
  }
}

for (const contract of [
  'onMediaRefresh: item.onMediaRefresh ?? onMediaRefresh',
  'onMediaRefresh,',
  'useEffect(() => setZoomImage(null), [image?.id])',
]) {
  if (!source.includes(contract)) {
    throw new Error(`zoom payload must preserve resource-scoped refresh behavior: missing ${contract}`)
  }
}
if (source.includes('useEffect(() => setZoomImage(null), [image?.id, imageUrl])')) {
  throw new Error('refreshing a signed URL must not close an open zoom viewer')
}
for (const contract of [
  '<ImageMediaFallback onRetry={() => { resetMediaRefresh(); setFailed(false) }}',
  "resetMediaRefresh(); setImageState('loading')",
]) {
  if (!source.includes(contract)) {
    throw new Error(`explicit image retry must request a new projection after another failure: missing ${contract}`)
  }
}
