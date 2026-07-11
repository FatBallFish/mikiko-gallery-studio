import { readFileSync } from 'node:fs'
import { createElement } from 'react'
import { renderToString } from 'react-dom/server'
import { ImageLightbox } from './components'
import { focusTrapTargetIndex } from './ui/focusTrap'

const ssr = renderToString(createElement(ImageLightbox, {
  image: { url: '/landing/hero-gallery.webp', alt: 'SSR lightbox contract' },
  onClose: () => undefined,
}))
if (ssr !== '') {
  throw new Error(`ImageLightbox must be SSR-safe through OverlayPortal, got ${ssr.slice(0, 120)}`)
}

if (focusTrapTargetIndex(2, 3, false) !== 0 || focusTrapTargetIndex(0, 3, true) !== 2) {
  throw new Error('ImageLightbox shared focus lifecycle must wrap Tab in both directions')
}

const source = readFileSync(new URL('./components.tsx', import.meta.url), 'utf8')
const lightboxStart = source.indexOf('export function ImageLightbox')
const lightboxEnd = source.indexOf('\nfunction LightboxInfo', lightboxStart)
const lightboxSource = source.slice(lightboxStart, lightboxEnd)

for (const required of [
  'useDismissableLayer(Boolean(image), onClose, dialogRef)',
  '<OverlayPortal>',
  'data-focus-layer',
  'ref={dialogRef}',
  'tabIndex={-1}',
  'useDismissableLayer(true, onClose, dialogRef)',
]) {
  if (!lightboxSource.includes(required)) {
    throw new Error(`ImageLightbox and nested zoom must reuse the shared focus lifecycle: ${required}`)
  }
}

if (lightboxSource.includes("window.addEventListener('keydown', close)")) {
  throw new Error('ImageLightbox must not duplicate the shared Escape/focus-trap listener')
}

const focusLayerCount = (lightboxSource.match(/data-focus-layer/g) ?? []).length
if (focusLayerCount < 2) {
  throw new Error(`ImageLightbox and zoom viewer must be separate nested focus layers, got ${focusLayerCount}`)
}
