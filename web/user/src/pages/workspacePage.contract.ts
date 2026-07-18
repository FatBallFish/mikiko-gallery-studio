import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./WorkspacePage.tsx', import.meta.url), 'utf8')

for (const required of [
  'createWorkspaceViewModel',
  '<WorkspaceStatusRail',
  'initialTaskId',
  'userApi.getTask(taskId)',
  "app.navigate('genpic', { taskId: nextTask.id })",
  'function selectRecentTask(task: ImageTask)',
  'function openHistoryTaskDialog(task: ImageTask)',
  'workspaceTaskHistoryInteraction',
  "app.navigate('genpic', { taskId: task.id })",
  'onSelectTask={selectRecentTask}',
  'onOpenTaskDialog={openHistoryTaskDialog}',
  'setHistoryTaskDialog(null)',
  'data-workspace-layout="creative"',
  'useCompactWorkspaceViewport()',
  'parametersExpanded',
  'aria-expanded={parametersExpanded}',
  'aria-controls="workspace-parameter-controls"',
  'id="workspace-parameter-controls"',
  'aria-hidden={parametersHidden || undefined}',
  'inert={parametersHidden ? true : undefined}',
  'aria-label="最近创作"',
  'max-[760px]:opacity-100',
  "!parametersExpanded && 'max-[760px]:hidden'",
  'max-[760px]:p-3',
  'aria-label="开始创作（移动端）"',
  'compactViewport && !parametersExpanded',
  'data-workspace-compact-generate="true"',
  'data-workspace-full-actions="true"',
  "disabled={busy || referenceRemainingLimit <= 0}",
  "disabled={busy || editRemainingLimit <= 0}",
  'limitReferenceSelection',
  'data-workspace-sheet-handle="true"',
  'onPointerDown={handleSheetPointerDown}',
  'onPointerMove={handleSheetPointerMove}',
  'onPointerUp={handleSheetPointerUp}',
  'onPointerCancel={handleSheetPointerCancel}',
  'setPointerCapture',
  'releasePointerCapture',
  'workspaceSheetSnap',
  'workspaceSheetDragOffset',
  'translate3d(0, ${sheetDragOffset}px, 0)',
  '<OverlayPortal>{parameterPanel}</OverlayPortal>',
  'singleReferenceAddition',
  'const referenceCount =',
  'const requiredReferencesReady = workspaceRequiredReferencesReady(taskType, referenceCount)',
  '&& requiredReferencesReady',
  'if (!requiredReferencesReady) {',
  'WORKSPACE_REFERENCE_REQUIRED_MESSAGE',
  "source.addEventListener('open'",
  'markWorkspaceStreamHealthy(generation)',
  'markStreamHealthy()',
  'streamTokenRef.current !== token',
  "mergeReferenceAssets(items, [nextAsset], maxReferenceImages)",
  'workspaceOutputOptions(selectedModel)',
  'normalizeWorkspaceOutputParameters',
  '按像素',
  '>基础分辨率</label>',
  '>质量</label>',
  '>输出格式</label>',
  '>压缩质量</label>',
  '>审核等级</label>',
  'output_format: outputFormat',
  'output_compression: compressionVisible ? outputCompression : 100',
  'moderation,',
]) {
  if (!source.includes(required)) throw new Error(`creative workspace should include ${required}`)
}

const estimatePayloadStart = source.indexOf('const estimatePayload')
const estimatePayloadEnd = source.indexOf('const estimateKey', estimatePayloadStart)
if (!(estimatePayloadStart >= 0 && estimatePayloadEnd > estimatePayloadStart)) {
  throw new Error('workspace must construct a stable estimate payload')
}
const estimatePayloadSource = source.slice(estimatePayloadStart, estimatePayloadEnd)
const expectedPayloadKeys = [
  'task_type',
  'route_model_code',
  'size_mode',
  'base_resolution',
  'quality',
  'output_format',
  'output_compression',
  'moderation',
  'aspect_ratio',
  'pixel_size',
  'image_count',
  'reference_asset_ids',
]
for (const key of expectedPayloadKeys) {
  if (!new RegExp(`\\b${key}(?:\\s*:|\\s*,)`).test(estimatePayloadSource)) {
    throw new Error(`workspace estimate payload must include ${key}`)
  }
}
const parametersReadyStart = source.indexOf('const parametersReady')
const parametersReadyEnd = source.indexOf('const estimatePayload', parametersReadyStart)
const parametersReadySource = source.slice(parametersReadyStart, parametersReadyEnd)
if (!parametersReadySource.includes('requiredReferencesReady')) {
  throw new Error('missing required references must keep parameters unready and prevent estimate requests')
}

const createTaskStart = source.indexOf('async function createTask')
const createTaskEnd = source.indexOf('async function applyAsEditSource', createTaskStart)
const createTaskSource = source.slice(createTaskStart, createTaskEnd)
const referenceGuard = createTaskSource.indexOf('if (!requiredReferencesReady)')
const createRequest = createTaskSource.indexOf('userApi.createTask')
if (!(referenceGuard >= 0 && createRequest > referenceGuard)) {
  throw new Error('createTask must defensively reject missing references before issuing the API request')
}

const historyEditStart = source.indexOf('async function applyAsEditSource')
const historyEditEnd = source.indexOf('\n  function removeReferenceAsset', historyEditStart)
const historyEditSource = source.slice(historyEditStart, historyEditEnd)
const historyLimitCheck = historyEditSource.indexOf('singleReferenceAddition')
const historyFetch = historyEditSource.indexOf('await fetch')
if (!(historyLimitCheck >= 0 && historyFetch > historyLimitCheck)) {
  throw new Error('history edit source must enforce the live reference limit before fetching or uploading')
}

const controlsStart = source.indexOf('id="workspace-parameter-controls"')
const fullActions = source.indexOf('data-workspace-full-actions="true"')
const asideEnd = source.indexOf('</aside>', controlsStart)
if (!(controlsStart >= 0 && fullActions > controlsStart && fullActions < asideEnd)) {
  throw new Error('full workspace actions must remain inside the controlled parameter region')
}

if (!/data-workspace-full-actions="true"[\s\S]*?<\/div>\s*<\/aside>/.test(source.slice(fullActions, asideEnd + 8))) {
  throw new Error('full workspace actions must close with the inert parameter region before the aside')
}

if (/hidden\s+group-hover:block/.test(source)) {
  throw new Error('essential workspace result actions must not depend on hover')
}

if (/Math\.round\(task\.progress/.test(source) || /task\.progress\s*\?\?/.test(source)) {
  throw new Error('workspace history must not display synthetic or legacy numeric progress')
}

if (!source.includes("task.size_mode === 'pixel' ? `尺寸: ${task.requested_size || task.aspect_ratio}` : `比例: ${task.aspect_ratio}`")) {
  throw new Error('pixel-mode task results must display requested pixel size instead of an aspect-ratio label')
}

const recentHandlerStart = source.indexOf('function selectRecentTask')
const dialogHandlerStart = source.indexOf('function openHistoryTaskDialog')
const gestureHandlerStart = source.indexOf('function handleSheetPointerDown', dialogHandlerStart)
const recentHandler = source.slice(recentHandlerStart, dialogHandlerStart)
const dialogHandler = source.slice(dialogHandlerStart, gestureHandlerStart)
for (const required of ['setSelectedTaskId(task.id)', "app.navigate('genpic', { taskId: task.id })", "setOutputTab('current')"]) {
  if (!recentHandler.includes(required)) throw new Error(`recent history selection must include ${required}`)
}
if (recentHandler.includes('setHistoryTaskDialog(task)')) {
  throw new Error('recent task selection must not open a task dialog')
}
if (!dialogHandler.includes('setHistoryTaskDialog(task)') || dialogHandler.includes('setSelectedTaskId') || dialogHandler.includes('app.navigate')) {
  throw new Error('multi-image history dialog must not change selected task or hash state')
}
