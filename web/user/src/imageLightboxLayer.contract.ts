import { readFileSync } from 'node:fs'
import { createElement } from 'react'
import { renderToString } from 'react-dom/server'
import { ImageDetailModal } from './components'
import { focusTrapTargetIndex } from './ui/focusTrap'
import { overlayLayers } from './ui/redesign-classes'

if (overlayLayers.modal !== 'z-[110]' || overlayLayers.lightbox !== 'z-[120]' || overlayLayers.zoom !== 'z-[130]') {
  throw new Error(`Overlay layers must remain ordered modal < lightbox < zoom: ${JSON.stringify(overlayLayers)}`)
}

const ssr = renderToString(createElement(ImageDetailModal, {
  title: 'SSR detail contract',
  image: { id: 'ssr', url: '/landing/hero-gallery.webp', width: 1024, height: 768, publish_status: 'private' },
  onCopyPrompt: () => undefined,
  onClose: () => undefined,
}))
if (ssr !== '') {
  throw new Error(`ImageDetailModal must be SSR-safe through OverlayPortal, got ${ssr.slice(0, 120)}`)
}

if (focusTrapTargetIndex(2, 3, false) !== 0 || focusTrapTargetIndex(0, 3, true) !== 2) {
  throw new Error('image detail shared focus lifecycle must wrap Tab in both directions')
}

const source = readFileSync(new URL('./components.tsx', import.meta.url), 'utf8')
const modalStart = source.indexOf('export function ImageDetailModal')
const modalEnd = source.indexOf('\nexport function PublicImageDetail', modalStart)
const modalSource = source.slice(modalStart, modalEnd)
const zoomStart = source.indexOf('function ImageZoomViewer')
const zoomEnd = source.indexOf('\nexport function PublicDetailIcon', zoomStart)
const zoomSource = source.slice(zoomStart, zoomEnd)

for (const required of ['<Modal', '<ImageZoomViewer', 'onPreviewImage={setZoomImage}']) {
  if (!modalSource.includes(required)) {
    throw new Error(`ImageDetailModal should own the direct zoom lifecycle: ${required}`)
  }
}
for (const required of ['useDismissableLayer(true, onClose, dialogRef)', '<OverlayPortal>', 'data-focus-layer', 'ref={dialogRef}', 'tabIndex={-1}']) {
  if (!zoomSource.includes(required)) {
    throw new Error(`zoom viewer must reuse the shared focus lifecycle: ${required}`)
  }
}
