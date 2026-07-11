import { overlayPortalHost, overlayPortalTarget } from './overlayPortal'

const body = {} as HTMLElement
if (overlayPortalHost({ body }) !== body) {
  throw new Error('interactive overlays must render into document.body')
}

if (overlayPortalHost(undefined) !== null || overlayPortalHost(null) !== null) {
  throw new Error('overlay portal must be safe when document is unavailable')
}

if (overlayPortalTarget !== 'document.body') {
  throw new Error(`overlay portal target drifted, got ${overlayPortalTarget}`)
}
