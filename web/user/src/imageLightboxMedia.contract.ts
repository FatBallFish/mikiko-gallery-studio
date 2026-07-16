import { existsSync, readFileSync } from 'node:fs'
import { createElement } from 'react'
import { renderToString } from 'react-dom/server'

const modelURL = new URL('./ui/imageMediaModel.ts', import.meta.url)
if (!existsSync(modelURL)) {
  throw new Error('ImageLightbox needs an executable per-image loading and retry model')
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
const lightboxStart = source.indexOf('const lightboxClasses')
const lightboxEnd = source.indexOf('\nfunction LightboxInfo', lightboxStart)
const lightboxSource = source.slice(lightboxStart, lightboxEnd)

if ((lightboxSource.match(/motion-reduce:animate-none/g) ?? []).length < 2) {
  throw new Error('lightbox and zoom entrance animations must both stop under reduced motion')
}
for (const contract of [
  'useImageMediaState(',
  'onLoad={media.markLoaded}',
  'onError={media.markError}',
  '<ImageMediaFallback onRetry={media.retry}',
  'key={media.imageKey}',
]) {
  if (!lightboxSource.includes(contract)) {
    throw new Error(`full-image and zoom media must expose ${contract}`)
  }
}
if ((lightboxSource.match(/useImageMediaState\(/g) ?? []).length < 2) {
  throw new Error('full-image and zoom viewers need independent per-image load state')
}
