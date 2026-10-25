import { clampUploadPosition, restoreUploadPosition, snapUploadPosition, uploadDragMoved, uploadPanelGeometry, uploadQueueSummary } from './uploadFloatGeometry'
import type { UploadSnapshot } from './uploadManager'

function check(value: unknown, message: string): asserts value { if (!value) throw new Error(message) }
for (const [width, height] of [[320, 568], [375, 667], [390, 844], [667, 375]]) {
  const bounds = { left: 8, top: 8, right: width - 8, bottom: height - 208 }
  for (const x of [-900, 8, width / 2, width + 900]) for (const y of [-900, height / 2, height + 900]) {
    const point = clampUploadPosition({ x, y, edge: null }, bounds)
    check(point.x >= bounds.left && point.x + 48 <= bounds.right && point.y >= bounds.top && point.y + 48 <= bounds.bottom, 'float must stay clear of viewport and bottom CTA')
    const panel = uploadPanelGeometry(point, bounds)
    check(panel.left >= bounds.left && panel.left + panel.width <= bounds.right && panel.top >= bounds.top && panel.top + panel.maxHeight <= bounds.bottom, 'expanded panel must fit available viewport')
  }
  check(snapUploadPosition({ x: 10, y: 50, edge: null }, bounds).edge === 'left', 'near left must dock')
  check(snapUploadPosition({ x: width - 60, y: 50, edge: null }, bounds).edge === 'right', 'near right must dock')
  check(snapUploadPosition({ x: width / 2 - 24, y: 50, edge: null }, bounds).edge === null, 'middle must remain free')
  check(clampUploadPosition({ x: 999, y: 999, edge: 'right' }, bounds).x === bounds.right - 48, 'resize must retain docking')
}
check(!uploadDragMoved(0, 0, 3, 3) && uploadDragMoved(0, 0, 0, 6) && uploadDragMoved(0, 0, 6, 0), 'drag threshold must support both axes and tolerate taps')
for (const raw of [null, '{', '{"x":null,"y":4,"edge":null}', '{"x":4,"y":4,"edge":"bad"}']) check(restoreUploadPosition(raw) === null, 'invalid stored position must be ignored')
check(restoreUploadPosition('{"x":4,"y":5,"edge":"left"}')?.edge === 'left', 'valid local position must restore')
const summary = (...statuses: UploadSnapshot['status'][]) => uploadQueueSummary(statuses.map((status) => ({ status })))
check(summary('completed', 'cancelled').count === 0, 'terminal uploads must not inflate pending badge')
check(summary('completed', 'paused', 'failed', 'needs_file', 'uploading', 'queued', 'initializing', 'completing').count === 7, 'all unfinished states must count')
check(summary('paused').state === 'paused' && summary('needs_file').state === 'error' && summary('failed', 'paused').state === 'error', 'paused and failure states must remain identifiable')
const { readFileSync } = await import('node:fs')
const tray = readFileSync(new URL('./UploadTray.tsx', import.meta.url), 'utf8')
for (const token of ['setPointerCapture', 'releasePointerCapture', 'onPointerCancel', 'onLostPointerCapture', 'suppressClick.current', 'viewport?.addEventListener', 'localStorage.setItem(positionKey', 'hidden={hidden}', 'controllers.current.forEach']) check(tray.includes(token), `persistent upload lifecycle and gestures require ${token}`)
check(!tray.includes('if (hidden) return'), 'canvas hiding must not unmount upload business')
const css = readFileSync(new URL('../../styles.css', import.meta.url), 'utf8')
check(tray.includes('data-dock-side={position?.edge ?? undefined}'), 'badge placement must follow the actual docking side')
for (const side of ['left', 'right']) {
  const inward = side === 'right' ? 'left' : 'right'
  const rule = css.split(`[data-dock-side="${side}"] .media-upload-badge {`)[1]?.split('}')[0] ?? ''
  check(rule.includes(`${inward}: -3px;`) && rule.includes(`${side}: auto;`), 'docked badge must use the visible inner edge')
  for (const width of [320, 375, 390, 667]) {
    const origin = side === 'right' ? width - 8 - 48 + 24 : 8 - 24
    // CSS badge is at the inner edge; cover short and three-digit counts.
    for (const badgeWidth of [19, 30]) {
      const badgeLeft = side === 'right' ? origin - 3 : origin + 48 + 3 - badgeWidth
      check(badgeLeft >= 0 && badgeLeft + badgeWidth <= width, 'half-hidden float badge must remain fully inside viewport')
    }
  }
}
check(css.includes('[data-queue-state="error"] .media-upload-badge { background: var(--accent-coral); }'), 'inner badge must preserve failure color')
// Execute the actual observer callback and opening gate against a DOM/event fixture.
const overlayBody = tray.slice(tray.indexOf('  const mobileOverlayOpen ='), tray.indexOf('  useEffect(() => {\n    try { if (position)'))
let mobile = true
let overlay: string | null = null
let expanded = true
let revealed = false
let mutation: (() => void) | undefined
let resize: (() => void) | undefined
let cleanup: (() => void) | undefined
let disconnected = false
const fixture = new Function('window', 'document', 'setExpanded', 'setRevealed', 'MutationObserver', 'useEffect', `${overlayBody}; return openQueue`)
const openQueue = fixture(
  { matchMedia: () => ({ matches: mobile }), addEventListener: (_: string, callback: () => void) => { resize = callback }, removeEventListener: () => { resize = undefined } },
  { body: {}, querySelector: (selector: string) => { check(selector.includes('.u-controls.is-open') && selector.includes('[aria-expanded="true"]'), 'both params and menu must block uploads'); return overlay } },
  (value: boolean) => { expanded = value }, (value: boolean) => { revealed = value },
  class { constructor(callback: () => void) { mutation = callback } observe(_target: unknown, options: { attributes: boolean; childList: boolean; attributeFilter: string[] }) { check(options.attributes && options.childList && options.attributeFilter.includes('class') && options.attributeFilter.includes('aria-expanded'), 'observe menu mount and open state changes') } disconnect() { disconnected = true } },
  (effect: () => () => void) => { cleanup = effect() },
)
for (const active of ['params', 'model-menu']) {
  overlay = null; openQueue(); check(expanded, 'queue must open when overlays are closed')
  overlay = active; mutation?.(); check(!expanded, 'opening params or model menu must collapse queue')
  revealed = false; openQueue(); check(!expanded && revealed, 'blocked queue must leave float accessible and fully revealed')
}
mobile = false; openQueue(); check(expanded, 'desktop queue behavior must remain unchanged')
mobile = true; resize?.(); check(!expanded, 'entering mobile with an open overlay must collapse queue')
cleanup?.(); check(disconnected && !resize, 'overlay observer and resize listener must clean up')
console.log('upload float geometry and presentation contract passed')
