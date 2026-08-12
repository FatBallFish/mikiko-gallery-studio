import fs from 'node:fs'

const source = fs.readFileSync(new URL('./CanvasEditorPage.tsx', import.meta.url), 'utf8')
for (const required of [
  'data-canvas-editor', 'data-canvas-world', 'data-canvas-node', 'data-canvas-minimap',
  'onPointerDown', 'setPointerCapture', 'onWheel', 'autoLayoutSelected',
  '手机仅支持查看', 'window.matchMedia', 'CanvasNodeSearch', 'CanvasAssetDrawer',
  'userApi.estimateCanvasNode', 'userApi.generateCanvasNode', 'userApi.listCanvasRuns',
  '确认生成', '预计积分', 'createCanvasRemoteSaveScheduler', 'copySelected', 'pasteClipboard', 'deleteSelected',
  'visibleCanvasNodeIDs', 'getMediaAssetAccess', "'preview'", "'download'", '<video', '<audio',
]) {
  if (!source.includes(required)) throw new Error(`canvas editor must include ${required}`)
}
if (!source.includes('<svg') || !source.includes('<path')) throw new Error('canvas edges must render in an SVG layer')
if (source.includes('<svg') && !source.includes('vectorEffect="non-scaling-stroke"')) throw new Error('edge stroke must remain stable while zooming')
const generateBody = source.slice(source.indexOf('async function generateNode'), source.indexOf('async function attachRun'))
if (generateBody.includes('estimateCanvasNode')) throw new Error('estimate and generation must require separate user actions')
if (!generateBody.includes('nodeEstimates[node.id]') || !generateBody.includes('generateCanvasNode')) throw new Error('generation must require a current estimate before submission')
if (!source.includes('setConflict({ remote, local: store.getState().command.present })')) throw new Error('revision conflict must retain edits made while the save request was in flight')
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
