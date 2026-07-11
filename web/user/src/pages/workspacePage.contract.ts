import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./WorkspacePage.tsx', import.meta.url), 'utf8')

for (const required of [
  'createWorkspaceViewModel',
  '<WorkspaceStatusRail',
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
  "mergeReferenceAssets(items, [nextAsset], maxReferenceImages)",
  'workspaceOutputOptions(selectedModel)',
  'normalizeWorkspaceOutputParameters',
  'label className={workspaceClasses.fieldLabel}>质量</label>',
  'label className={workspaceClasses.fieldLabel}>输出格式</label>',
  'label className={workspaceClasses.fieldLabel}>压缩质量</label>',
  'label className={workspaceClasses.fieldLabel}>审核等级</label>',
  'output_format: outputFormat',
  'output_compression: compressionVisible ? outputCompression : 100',
  'moderation,',
]) {
  if (!source.includes(required)) throw new Error(`creative workspace should include ${required}`)
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
