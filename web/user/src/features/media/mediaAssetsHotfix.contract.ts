import { readFileSync } from 'node:fs'
import { normalizeAdminVideoConfiguration } from '../../../../shared/admin-api'
import { mediaMarqueeSelection, mediaSelectionDragDistance, mediaSelectionRectangle, shouldSuppressMediaCardAction } from './mediaExperience'

const normalized = normalizeAdminVideoConfiguration({
  capabilities: null,
  rate_cards: null,
  routes: null,
  impacts: null,
  generated_at: '2026-08-13T00:00:00Z',
} as never)

for (const field of ['capabilities', 'rate_cards', 'routes', 'impacts'] as const) {
  if (!Array.isArray(normalized[field]) || normalized[field].length !== 0) throw new Error(`${field} must normalize to []`)
}

if (mediaSelectionDragDistance({ x: 8, y: 9 }, { x: 12, y: 12 }) >= 6) throw new Error('small pointer movement must stay a click')
if (mediaSelectionDragDistance({ x: 8, y: 9 }, { x: 15, y: 9 }) < 6) throw new Error('desktop drag must enter marquee selection')

const rectangle = mediaSelectionRectangle({ x: 120, y: 90 }, { x: 10, y: 20 })
if (JSON.stringify(rectangle) !== JSON.stringify({ left: 10, top: 20, right: 120, bottom: 90 })) throw new Error(`rectangle mismatch: ${JSON.stringify(rectangle)}`)

const candidates = [
  { id: 'one', rect: { left: 10, top: 10, right: 50, bottom: 50 } },
  { id: 'two', rect: { left: 70, top: 10, right: 110, bottom: 50 } },
  { id: 'three', rect: { left: 140, top: 10, right: 180, bottom: 50 } },
]
const replaced = mediaMarqueeSelection(new Set(['three']), candidates, { left: 0, top: 0, right: 120, bottom: 60 }, false)
if (JSON.stringify([...replaced]) !== JSON.stringify(['one', 'two'])) throw new Error(`replacement mismatch: ${JSON.stringify([...replaced])}`)
const appended = mediaMarqueeSelection(new Set(['three']), candidates, { left: 0, top: 0, right: 120, bottom: 60 }, true)
if (JSON.stringify([...appended]) !== JSON.stringify(['three', 'one', 'two'])) throw new Error(`additive mismatch: ${JSON.stringify([...appended])}`)
if (!shouldSuppressMediaCardAction(1250, 1100) || shouldSuppressMediaCardAction(1250, 1251)) throw new Error('drag click suppression must use a deterministic time window')

const pageSource = readFileSync(new URL('./MediaAssetsPage.tsx', import.meta.url), 'utf8')
const cardSource = readFileSync(new URL('./MediaAssetCard.tsx', import.meta.url), 'utf8')
const cssSource = readFileSync(new URL('../../styles.css', import.meta.url), 'utf8')

for (const required of ['OverlayPortal', 'onPointerDown', 'onPointerMove', 'onPointerUp', 'onPointerCancel', 'onLostPointerCapture', 'captureTarget: target', 'target.setPointerCapture(event.pointerId)', 'data-media-asset-id', 'media-selection-marquee']) {
  if (!pageSource.includes(required) && !cardSource.includes(required)) throw new Error(`unified assets must include ${required}`)
}
for (const purpose of ["getMediaAssetAccess(asset.id, 'thumbnail')", "purpose = asset.media_type === 'video' ? 'poster'", "asset.media_type === 'audio' ? 'waveform'"]) {
  if (!cardSource.includes(purpose)) throw new Error(`asset previews must retain ${purpose}`)
}
if (!cardSource.includes("asset.status === 'ready_original'")) throw new Error('generated image originals must remain previewable while thumbnails are processing')
if ((cardSource.match(/draggable=\{false\}/g) ?? []).length < 4) throw new Error('card preview media must not steal marquee drags')
if (!cssSource.includes('.media-selection-marquee') || !/\.media-selection-marquee\s*\{[^}]*position:\s*fixed/s.test(cssSource)) {
  throw new Error('marquee must be fixed to viewport coordinates')
}
if (!/\.media-batch-toolbar\s*\{[^}]*position:\s*fixed/s.test(cssSource)) throw new Error('batch toolbar must remain viewport-fixed')
