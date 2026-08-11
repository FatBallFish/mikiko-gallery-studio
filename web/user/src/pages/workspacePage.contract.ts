import { existsSync, readFileSync } from 'node:fs'

const source = readFileSync(new URL('./WorkspacePage.tsx', import.meta.url), 'utf8')

for (const removed of ['模型选择', '限制词', 'negative_prompt: negative']) {
  if (source.includes(removed)) throw new Error(`creative workspace must remove ${removed}`)
}
for (const required of ['模型分组', '<ModelGroupSelect', 'options={availableModels}']) {
  if (!source.includes(required)) throw new Error(`creative workspace model group selector must include ${required}`)
}
if (source.includes('<select id="workspace-model-group"')) {
  throw new Error('creative workspace model group selector must not use the browser-native select')
}

const modelGroupSelectURL = new URL('./ModelGroupSelect.tsx', import.meta.url)
if (!existsSync(modelGroupSelectURL)) throw new Error('creative workspace must provide the platform-style ModelGroupSelect')
const modelGroupSelectSource = readFileSync(modelGroupSelectURL, 'utf8')
for (const required of ['role="listbox"', 'role="option"', 'aria-haspopup="listbox"', 'aria-expanded', '◈', 'description', 'minimum_points']) {
  if (!modelGroupSelectSource.includes(required)) throw new Error(`model group listbox must implement ${required}`)
}

for (const required of ['className="reference-name-row"', 'title={asset.name}', '<Modal title="重命名资产"']) {
  if (!source.includes(required)) throw new Error(`reference asset naming must implement ${required}`)
}
if (source.includes('className="reference-name-editor"')) {
  throw new Error('reference asset naming must use the full-size rename dialog instead of the thumbnail editor')
}
const modelGroupPosition = source.indexOf('模型分组')
const referencePosition = source.indexOf('图片编辑来源')
const promptPosition = source.indexOf('>提示词</label>')
const sizeModePosition = source.indexOf('>尺寸模式</label>')
if (!(modelGroupPosition >= 0 && modelGroupPosition < referencePosition && referencePosition < promptPosition && promptPosition < sizeModePosition)) {
  throw new Error('creative workspace order must be model group, references, prompt, then generation parameters')
}

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
  'mergeReferenceAssets(items, imported, maxReferenceImages)',
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
  'capability_version: estimate?.capability_version',
  "err.code === 'capability_changed'",
  'userApi.estimate(estimatePayload)',
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
const capabilityChangedStart = createTaskSource.indexOf("err.code === 'capability_changed'")
const capabilityChangedEnd = createTaskSource.indexOf("app.notify('error'", capabilityChangedStart)
const capabilityChangedSource = createTaskSource.slice(capabilityChangedStart, capabilityChangedEnd)
for (const required of [
	'await userApi.getCapabilities()',
	'setCapability(nextCapability)',
	"setEstimateSnapshot({ key: '', estimate: null, error: '' })",
]) {
  if (!capabilityChangedSource.includes(required)) {
    throw new Error(`capability_changed recovery must refresh capabilities and invalidate the stale estimate: missing ${required}`)
  }
}
if (capabilityChangedSource.includes('userApi.estimate(')) {
  throw new Error('capability_changed recovery must let normalized capability state drive the next estimate instead of reusing the stale payload')
}

const promptTemplateStaleStart = createTaskSource.indexOf("err.code === 'PROMPT_TEMPLATE_STALE'")
const promptTemplateStaleEnd = createTaskSource.indexOf("app.notify('error'", promptTemplateStaleStart)
const promptTemplateStaleSource = createTaskSource.slice(promptTemplateStaleStart, promptTemplateStaleEnd)
for (const required of [
  'userApi.getReferenceAsset(asset.id)',
  'setEditRefs(',
  '引用资产名称已变化',
  '请检查红色标记',
]) {
  if (!promptTemplateStaleSource.includes(required)) {
    throw new Error(`PROMPT_TEMPLATE_STALE recovery must refresh reference state and require explicit confirmation: missing ${required}`)
  }
}
if (promptTemplateStaleSource.includes('setPrompt(')) {
  throw new Error('PROMPT_TEMPLATE_STALE recovery must not silently rewrite the prompt template')
}

const historyEditStart = source.indexOf('async function applyAsEditSource')
const historyEditEnd = source.indexOf('\n  function removeEditAsset', historyEditStart)
const historyEditSource = source.slice(historyEditStart, historyEditEnd)
const historyLimitCheck = historyEditSource.indexOf('singleReferenceAddition')
const historyImport = historyEditSource.indexOf('userApi.importReferenceAssetsFromGallery')
if (!(historyLimitCheck >= 0 && historyImport > historyLimitCheck)) {
  throw new Error('history edit source must enforce the live reference limit before importing by image ID')
}
if (historyEditSource.includes('await fetch')) {
  throw new Error('history edit source must not fetch an expiring object-storage URL in the browser')
}

for (const removed of ['参考生图', 'reference_to_image', "openGalleryImport('reference')", "uploadReference(event, 'reference')"]) {
  if (source.includes(removed)) throw new Error(`creative workspace must not retain removed reference-generation path ${removed}`)
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

if (!source.includes('<PromptTemplateEditor') || source.includes('redesign-prompt-input pb-11 pr-20')) {
  throw new Error('the outer prompt editor must use the full width and render template tokens')
}
for (const required of [
  'function HistoryTaskGalleryModal',
  '历史创作总览',
  'onPreviewImage',
  'setHistoryTaskDialog(null)',
]) {
  if (!source.includes(required)) throw new Error(`multi-image history must open an overview before image detail: missing ${required}`)
}
const historyOverviewStart = source.indexOf('function HistoryTaskGalleryModal')
const generationOutputStart = source.indexOf('function GenerationOutput', historyOverviewStart)
const historyOverviewSource = source.slice(historyOverviewStart, generationOutputStart)
if (historyOverviewSource.includes('<ImageDetailModal')) {
  throw new Error('the multi-image history overview must not duplicate the shared image detail modal')
}
for (const stateContract of [
  'historyTaskDialog && !previewImage',
  'onPreviewImage={openHistoryPreview}',
  'onClose={() => setPreviewImage(null)}',
  'historyPreviewReturnTarget',
  'data-history-image-id={image.id}',
  "document.querySelectorAll<HTMLElement>('[data-history-image-id]')",
  'window.requestAnimationFrame',
  'setHistoryPreviewReturnTarget(null)',
]) {
  if (!source.includes(stateContract)) {
    throw new Error(`closing shared image detail must restore the task overview: missing ${stateContract}`)
  }
}

const focusRestoreEffect = source.indexOf("document.querySelectorAll<HTMLElement>('[data-history-image-id]')")
const historyOverviewRender = source.indexOf('{historyTaskDialog && !previewImage ? (')
if (!(focusRestoreEffect >= 0 && focusRestoreEffect < historyOverviewRender)) {
  throw new Error('closing image detail must restore focus after the history overview remounts')
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
