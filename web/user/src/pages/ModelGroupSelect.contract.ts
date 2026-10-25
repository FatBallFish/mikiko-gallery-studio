import { readFileSync } from 'node:fs'

const picker = readFileSync(new URL('./ModelGroupSelect.tsx', import.meta.url), 'utf8')
const styles = readFileSync(new URL('../styles.css', import.meta.url), 'utf8')
for (const level of [1, 2]) {
  if (!picker.includes(`data-level="${level}" hidden={level !== ${level}} inert={level !== ${level}} aria-hidden={level !== ${level}}`)) {
    throw new Error(`inactive picker level ${level} must leave hit testing, keyboard navigation and accessibility tree`)
  }
}
if (!/\.model-group-select-level\[hidden\]\s*\{\s*display: none;\s*visibility: hidden;\s*pointer-events: none;/.test(styles)) {
  throw new Error('picker flex layout must not override the hidden attribute')
}
if (styles.includes('.model-group-select-menu.is-level2 .model-group-select-level')) {
  throw new Error('picker layers must share a single React visibility state, not a competing transform state')
}
if (picker.includes('[open, selectedSection]')) throw new Error('refreshed section objects must not reset Back navigation')
if (!/function toggleOpen\(\)\s*\{\s*if \(!open\)/.test(picker) || !picker.includes('onClick={toggleOpen}')) throw new Error('selection group must initialize only when opening')
if (!picker.includes('onClick={showGroups}') || !picker.includes('panelRef.current?.focus()')) throw new Error('Back must move focus outside the outgoing inert layer')
if (!picker.includes('aria-haspopup="dialog"') || !picker.includes('role="dialog"') || !picker.includes("aria-label={hasGroups && level === 1 ? '选择模型分组' : '选择路由模型'}")) throw new Error('popup semantics must describe the active view')
// Run the production positioning function against short screens and shifted visual viewports.
const geometrySource = picker.slice(picker.indexOf('export function modelMenuGeometry'), picker.indexOf('// Progressive drill-down'))
const { createRequire } = await import('node:module')
const ts = createRequire(new URL('../../package.json', import.meta.url))('typescript')
const compiled = ts.transpileModule(geometrySource, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText
const geometryModule = { exports: {} as { modelMenuGeometry: (trigger: any, viewport: any) => any } }
new Function('exports', compiled)(geometryModule.exports)
for (const [width, height, offsetTop] of [[320, 568, 0], [390, 844, 0], [667, 375, 0], [320, 280, 120]]) {
  const viewport = { left: 0, top: offsetTop, width, height }
  for (const triggerTop of [offsetTop + 50, offsetTop + height - 100]) {
    const rect = geometryModule.exports.modelMenuGeometry({ left: 16, top: triggerTop, bottom: triggerTop + 48, width: width - 32 }, viewport)
    if (rect.left < 8 || rect.left + rect.width > width - 8 || rect.top < offsetTop + 8 || rect.top + rect.height > offsetTop + height - 8 || rect.height < 90) throw new Error('menu must fit the short visual viewport and offer scrollable room')
  }
}
for (const required of ['<OverlayPortal>', "position: 'fixed'", "document.addEventListener('scroll', reposition, true)", "viewport?.addEventListener('resize', reposition)", 'panelRef.current?.contains(event.target as Node)', 'panelRef.current?.contains(next)', 'event.stopPropagation()', 'aria-expanded={open}']) {
  if (!picker.includes(required)) throw new Error(`portal menu lifecycle missing ${required}`)
}
const video = readFileSync(new URL('../features/creation/VideoCreationPanel.tsx', import.meta.url), 'utf8')
if (!video.includes('const ParameterHost = compactViewport ? OverlayPortal : Fragment') || !video.includes('<ParameterHost><aside className={cn(\'u-controls\'')) throw new Error('mobile video controls must escape transformed page ancestors through the shared portal')
console.log('model group select layer and viewport contract passed')
