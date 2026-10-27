import fs from 'node:fs'

const source = fs.readFileSync(new URL('./CanvasEditorPage.tsx', import.meta.url), 'utf8')
const integration = fs.readFileSync(new URL('./canvasPageIntegration.ts', import.meta.url), 'utf8')
for (const required of [
  'data-canvas-editor', 'data-canvas-world', 'data-canvas-node', 'data-canvas-minimap',
  'onPointerDown', 'setPointerCapture', 'releasePointerCapture', 'onWheel', 'autoLayoutSelected',
  '手机仅支持查看', 'window.matchMedia', 'CanvasNodeSearch', 'CanvasAssetDrawer',
  'userApi.estimateCanvasNode', 'userApi.generateCanvasNode', 'userApi.listCanvasRuns',
  'createCanvasRemoteSaveScheduler', 'copySelected', 'pasteClipboard', 'deleteSelected',
  'visibleCanvasNodeIDs', 'data-canvas-edge-hit', 'selectedEdgeIDs', 'selectEdges',
  'data-canvas-port="right"', 'data-canvas-port="left"', 'resolveCanvasConnection',
  'compatibleCanvasTargets', 'data-connect-valid', 'data-connect-invalid', 'onContextMenu',
  'canvas-node-menu', 'application/x-canvas-asset', 'onDragOver', 'onDrop',
  'CanvasMediaNodeV2', 'CanvasGenerationConsole', 'canvasGenerationEstimateEligible',
  "addNode('image_v2')", "addNode('video_v2')", "addNode('note')", 'canvasPointerIntent',
  'canvasConnectionRelease', 'referencePick', "event.key === 'Escape' && !",
  'insertCanvasReference', 'validateCanvasReference', 'uploadTargetsRef', 'MEDIA_UPLOAD_COMPLETED_EVENT',
  'QUEUE_MEDIA_UPLOAD_EVENT', 'canvas-console-anchor', 'onPatchDraft', 'onRemoveReference',
  'canvasGenerationEstimateSignature', 'nodeEstimatesRef.current[node.id]', 'quote_token',
  'canvas-dock-grip', 'clearCanvasTransientState', 'backgroundPointerRef.current', 'dockDragRef.current',
  'resizeCanvasMediaSize(resize.startSize, requested)', 'canCopyCanvasNode',
  'progress_message', 'progress_stage', 'LegacyNodeBody',
]) if (!source.includes(required)) throw new Error(`canvas page integration must include ${required}`)
for (const forbidden of ["addNode('prompt')", "addNode('image')", "addNode('image_generation')", "addNode('video_generation')", 'function GenerationNodeForm', 'function GenerationNodeSummary', 'function PromptNodeBody']) {
  if (source.includes(forbidden)) throw new Error(`legacy editing or creation UI remains: ${forbidden}`)
}
const generateBody = source.slice(source.indexOf('async function generateNode'), source.indexOf('async function attachRun'))
for (const required of ["estimate.status !== 'ready'", 'estimate.signature', 'await flushDocument()', 'isCanvasGenerationNode', 'nodeEstimatesRef.current[node.id] !== estimate', 'crypto.randomUUID()']) {
  if (!generateBody.includes(required)) throw new Error(`generation must revalidate quote and target: ${required}`)
}
if (!fs.readFileSync(new URL('./core/canvasEstimateCoordinator.ts', import.meta.url), 'utf8').includes('options.debounceMS ?? 300')) throw new Error('both media types require debounced estimates')
if (!source.includes('title="资产库"') || !source.includes('setShowAssets(true)')) throw new Error('canvas dock must retain the asset library')
if (source.includes('progress: 50') || source.includes('progress_percent')) throw new Error('page must not invent task percentages')
if (!source.includes('setConflict({ remote, local: store.getState().command.present })')) throw new Error('conflicts must retain latest local edits')
if (!source.includes('vectorEffect="non-scaling-stroke"')) throw new Error('edge strokes must remain readable across zoom')
for (const required of ['getMediaAssetAccess', 'mediaAssetUrl(access.url, token)', "'ready_original'", 'payload?.name', 'inspectCanvasConnection', 'isCanvasGenerationNode(target)', 'document.edges.some', 'Math.min(6, pending.length)']) {
  if (!integration.includes(required)) throw new Error(`reference/hydration integration must include ${required}`)
}
const styles = fs.readFileSync(new URL('./canvas.css', import.meta.url), 'utf8')
for (const required of ['(orientation: portrait)', '(pointer: coarse)', 'min-height: 44px', 'touch-action: none', '.canvas-node-v2', '.canvas-console-anchor', '.canvas-reference-pick']) {
  if (!styles.includes(required)) throw new Error(`canvas CSS must include ${required}`)
}

// Preserve the existing interaction, recovery, legacy-media and role contracts.
for (const required of [
  'onDoubleClick', '<svg', '<path', '<video', '<audio', '查看详情', 'MediaPreviewDialog',
  "pointerType === 'touch'", 'activePointersRef', 'pinchRef', 'longPressRef', 'visualViewport', 'canvas-keyboard-open',
  'data-canvas-drag-handle', 'data-canvas-interactive', 'data-canvas-resize-handle', 'onResizeStart', 'resizeNode',
  'userApi.getCapabilities', 'userApi.getVideoCapabilities',
  "closest('[data-canvas-interactive]')", "app.notify('error', '当前生成关系不能形成循环')",
  'if (readOnly || !store || !imageCapability) return', "runs.filter((run) => run.status === 'unplaced')",
  'recoveryPosition', '恢复到当前视图', '!readOnly ? <button type="button" disabled={busyNodeID === unplacedRuns[0].node_id}',
]) if (!source.includes(required)) throw new Error(`canvas editor must retain ${required}`)
const stateSource = fs.readFileSync(new URL('./core/canvasState.ts', import.meta.url), 'utf8')
for (const required of ["role === 'reference_video'", "role === 'reference_audio'", "['first_frame', 'last_frame', 'reference_image'].includes(role)"]) {
  if (!stateSource.includes(required)) throw new Error(`canvas rules must retain ${required}`)
}
const types = fs.readFileSync(new URL('./core/types.ts', import.meta.url), 'utf8')
for (const role of ['reference_image', 'reference_video', 'reference_audio']) {
  if (!types.includes(`'${role}'`)) throw new Error(`canvas edge roles must declare ${role}`)
}
const drawer = fs.readFileSync(new URL('./CanvasAssetDrawer.tsx', import.meta.url), 'utf8')
if (!drawer.includes('mediaType') || !drawer.includes('asset.media_type === mediaType')) throw new Error('asset drawer must retain media filtering')
if (drawer.includes("status: 'ready'")) throw new Error('drawer must include usable originals after derivative failures')

// Inspect CSS declarations in their actual selectors, rather than TSX source text.
for (const selector of ['.canvas-viewport', '.canvas-node']) {
  const block = styles.slice(styles.indexOf(`${selector} {`)).split('}')[0]
  if (!/cursor:\s*default\s*;/.test(block)) throw new Error(`${selector} must use the default arrow`)
}
const grip = styles.slice(styles.indexOf('.canvas-dock .canvas-dock-grip {')).split('}')[0]
for (const required of ['left: 50%', 'translateX(-50%)', 'cursor: grab', 'touch-action: none']) {
  if (!grip.includes(required)) throw new Error(`centered drag handle must include ${required}`)
}
if (styles.includes('margin-left: 0 !important') || /\.canvas-dock\s*\{[^}]*right: 12px/.test(styles)) throw new Error('small-screen dock must retain horizontal translation')
if (!styles.includes('@media (max-width: 1100px)') || !styles.includes('.canvas-zoom-controls { bottom: 92px; }')) throw new Error('zoom must use a separate row when the dock can overlap')
for (const required of ['ref={dockRef}', 'dock?.parentElement', 'observer.observe(dock)', 'observer.observe(container)', 'setDockOffset((offset) => clampDockOffset(offset))', 'onLostPointerCapture']) {
  if (!source.includes(required)) throw new Error(`dock drag/resize must retain ${required}`)
}
const backgroundDown = source.slice(source.indexOf('      onPointerDown={(event) => {'), source.indexOf('      onPointerMove={(event) => {'))
for (const required of ['[data-canvas-edge-hit]', 'button: event.button', 'shift: event.shiftKey', 'space: spacePressedRef.current', 'background.suppressed = true', 'backgroundPointerRef.current = null']) {
  if (!backgroundDown.includes(required)) throw new Error(`background gesture ownership must include ${required}`)
}
const backgroundUp = source.slice(source.indexOf('      onPointerUp={(event) => {'), source.indexOf('      onPointerCancel={(event) => {'))
for (const required of ['if (!background || background.pointerID !== event.pointerId) return', '!background.moved && !background.suppressed && background.button === 0 && !background.space']) {
  if (!backgroundUp.includes(required)) throw new Error(`only an owned primary blank click may clear selection: ${required}`)
}
if (!backgroundDown.includes('connectionRef.current && connectionRef.current.pointerID !== null')) throw new Error('pending connections must allow background click and pan')
if (!backgroundUp.includes('draft && draft.pointerID !== null && draft.pointerID === event.pointerId')) throw new Error('pending connection release must fall through to owned background click cleanup')
const dockClamp = source.slice(source.indexOf('  function clampDockOffset'), source.indexOf('  const visibleNodeIDs'))
for (const required of ["'.canvas-minimap', '.canvas-zoom-controls'", 'rect.top >= dockBounds.bottom || rect.bottom <= dockBounds.top', 'rect.right + 12', 'rect.left - 12', 'Math.max(minimum, Math.min(maximum, offset))']) {
  if (!dockClamp.includes(required)) throw new Error(`dock limits must reserve overlapping bottom controls: ${required}`)
}
if (!source.includes('button: background.button') || source.includes("} else if (event.key === 'Escape')")) throw new Error('gesture intent must use pointerdown button; Escape must have one cleanup path')
const app = fs.readFileSync(new URL('../../App.tsx', import.meta.url), 'utf8')
const tray = fs.readFileSync(new URL('../media/UploadTray.tsx', import.meta.url), 'utf8')
if (!app.includes("<UploadTray hidden={route === 'oauth-consent' || (route === 'creative-canvas' && Boolean(routeCanvasId))} />")) throw new Error('canvas editor and OAuth consent must hide the global tray without unmounting it')
// Hiding is presentation-only. The final display override must win over float geometry.
const hiddenStyle = /style=\{hidden\s*\?\s*\{\s*display:\s*'none'\s*\}\s*:\s*undefined\}/.test(tray)
  || /style=\{\{\s*\.\.\.floatingStyle,\s*\.\.\.\(hidden\s*\?\s*\{\s*display:\s*'none'\s*\}\s*:\s*\{\}\)\s*\}\}/.test(tray)
if (!tray.includes('hidden={hidden}') || !hiddenStyle || /if\s*\(hidden\)\s*\{?\s*return\b/.test(tray)) throw new Error('hidden upload presentation must retain its input and processing hooks')
for (const required of ['ref={inputRef}', 'type="file"', 'onChange={(event) => void addFiles(event.target.files)}', 'useEffect(', 'window.addEventListener(QUEUE_MEDIA_UPLOAD_EVENT', "window.addEventListener('mgs:open-media-upload'", 'MEDIA_UPLOAD_COMPLETED_EVENT, { detail:', 'controllers.current', 'void addFiles(detail.files']) {
  if (!tray.includes(required)) throw new Error(`hidden upload processing must retain ${required}`)
}

// Assertions for the former inline prompt/parameter UI follow the extracted console.
const consoleSource = fs.readFileSync(new URL('./CanvasGenerationConsole.tsx', import.meta.url), 'utf8')
for (const required of ['estimatePromptOptimization', 'optimizePrompt', 'PromptTemplateEditor', 'PromptVariableForm', 'parsePromptTemplate(template)', 'workspaceTaskImageSafetyLimit', 'type="number"', '参考生视频', 'missingReference', 'estimate?.sufficient === false', 'estimate?.valid === true']) {
  if (!consoleSource.includes(required)) throw new Error(`extracted generation console must retain ${required}`)
}
const mediaSource = fs.readFileSync(new URL('./canvasMediaV2.ts', import.meta.url), 'utf8')
if (!mediaSource.includes('normalizeWorkspaceImageCount')) throw new Error('extracted image draft must retain image-count normalization')

const autoEstimateEffect = source.slice(source.indexOf('const targets: CanvasEstimateTarget[]'), source.indexOf('const timers = Object.entries(nodeEstimates)'))
if (!autoEstimateEffect.includes('generationErrors(node, documentState)') || !autoEstimateEffect.includes('hydratedMedia]')) throw new Error('automatic estimates must share generation validation and rerun after media hydration')
if (source.includes('const pending = uploadTargetsRef.current.get(token)')) throw new Error('active reference uploads must not expire on a wall-clock timer')
