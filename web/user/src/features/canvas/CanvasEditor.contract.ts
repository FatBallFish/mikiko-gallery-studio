import fs from 'node:fs'

const source = fs.readFileSync(new URL('./CanvasEditorPage.tsx', import.meta.url), 'utf8')
for (const required of [
  'data-canvas-editor', 'data-canvas-world', 'data-canvas-node', 'data-canvas-minimap',
  'onPointerDown', 'setPointerCapture', 'onWheel', 'autoLayoutSelected',
  '手机仅支持查看', 'window.matchMedia', 'CanvasNodeSearch', 'CanvasAssetDrawer',
  'userApi.estimateCanvasNode', 'userApi.generateCanvasNode', 'userApi.listCanvasRuns',
  '确认生成', '预计积分', 'createCanvasRemoteSaveScheduler', 'copySelected', 'pasteClipboard', 'deleteSelected',
  'visibleCanvasNodeIDs', 'getMediaAssetAccess', "'preview'", "'download'", '<video', '<audio',
  'data-canvas-edge-hit', 'selectedEdgeIDs', 'selectEdges', 'data-canvas-port="source"', 'data-canvas-port="target"',
  'inspectCanvasConnection', 'compatibleCanvasTargets', 'data-connect-valid', 'data-connect-invalid',
  'onDoubleClick', 'onContextMenu', 'canvas-node-menu', 'application/x-canvas-asset', 'onDragOver', 'onDrop',
  'estimatePromptOptimization', 'optimizePrompt', 'PromptTemplateEditor', 'PromptVariableForm',
  'userApi.getCapabilities', 'userApi.getVideoCapabilities', 'canvas-generation-inputs', 'canvas-generation-errors', '当前余额',
  'active_prompt_node_id', '提示词来源', '资源绑定', 'buildCanvasPromptBindings',
  '查看详情', '继续生图', '生成视频', '复用参数', 'canvas-media-facts', 'MediaPreviewDialog',
  'pointerType === \'touch\'', 'activePointersRef', 'pinchRef', 'longPressRef', 'visualViewport', 'canvas-keyboard-open',
  'data-canvas-drag-handle', 'data-canvas-interactive', 'data-canvas-resize-handle', 'onResizeStart', 'resizeNode',
  '添加图片框', 'QUEUE_MEDIA_UPLOAD_EVENT', 'MEDIA_UPLOAD_COMPLETED_EVENT', 'canvasPromptResourceCandidates',
  '图片框仅支持 JPG、PNG、WEBP 图片',
  'normalizeWorkspaceImageCount', 'workspaceTaskImageSafetyLimit', 'type="number"', '重新估价', 'canvasGenerationEstimateSignature',
]) {
  if (!source.includes(required)) throw new Error(`canvas editor must include ${required}`)
}
if ((source.match(/查看费用/g) ?? []).length !== 1 || !source.includes("node.type === 'image_generation'")) throw new Error('only video generation may retain the explicit view-cost step')
if (!source.includes('setPointerCapture') || !source.includes('releasePointerCapture')) throw new Error('port dragging must retain pointer ownership until release')
if (!source.includes("closest('[data-canvas-interactive]')")) throw new Error('interactive node controls must never initiate node dragging')
if (!source.includes('parsePromptTemplate(template)')) throw new Error('canvas resource bindings must use the shared parser so escaped placeholders stay literal')
if (!source.includes("app.notify('error', '当前生成关系不能形成循环')")) throw new Error('cycle rejection must use the product copy from the PRD')
if (!source.includes('<svg') || !source.includes('<path')) throw new Error('canvas edges must render in an SVG layer')
if (source.includes('<svg') && !source.includes('vectorEffect="non-scaling-stroke"')) throw new Error('edge stroke must remain stable while zooming')
const generateBody = source.slice(source.indexOf('async function generateNode'), source.indexOf('async function attachRun'))
if (generateBody.includes('estimateCanvasNode')) throw new Error('estimate and generation must require separate user actions')
if (!generateBody.includes('nodeEstimatesRef.current[node.id]') || !generateBody.includes("estimate.status !== 'ready'") || !generateBody.includes('generateCanvasNode')) throw new Error('generation must require a current estimate before submission')
if (!source.includes('setConflict({ remote, local: store.getState().command.present })')) throw new Error('revision conflict must retain edits made while the save request was in flight')
if (!source.includes('if (readOnly || !store || !imageCapability) return')) throw new Error('read-only canvas views must not persist automatic image task normalization')
if (!source.includes("runs.filter((run) => run.status === 'unplaced')") || !source.includes('recoveryPosition') || !source.includes('恢复到当前视图')) {
  throw new Error('unplaced results must expose a canvas-level recovery action at the current viewport center')
}
if (!source.includes("!readOnly ? <button type=\"button\" disabled={busyNodeID === unplacedRuns[0].node_id}")) {
  throw new Error('mobile read-only canvas must not expose the unplaced recovery mutation')
}

const styles = fs.readFileSync(new URL('./canvas.css', import.meta.url), 'utf8')
for (const query of ['(orientation: portrait)', '(pointer: coarse)', 'min-height: 44px', 'touch-action: none']) {
  if (!styles.includes(query)) throw new Error(`canvas responsive CSS must include ${query}`)
}
const drawerSource = fs.readFileSync(new URL('./CanvasAssetDrawer.tsx', import.meta.url), 'utf8')
if (!drawerSource.includes('mediaType') || !drawerSource.includes('asset.media_type === mediaType')) throw new Error('canvas image frames must open an image-only asset drawer')
if (drawerSource.includes("status: 'ready'")) throw new Error('canvas asset drawer must not hide usable originals when derivative processing failed')
