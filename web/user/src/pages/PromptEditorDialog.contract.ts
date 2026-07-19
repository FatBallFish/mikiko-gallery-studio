import { readFileSync } from 'node:fs'

const dialog = readFileSync(new URL('./PromptEditorDialog.tsx', import.meta.url), 'utf8')
const workspace = readFileSync(new URL('./WorkspacePage.tsx', import.meta.url), 'utf8')
const redesignClasses = readFileSync(new URL('../ui/redesign-classes.ts', import.meta.url), 'utf8')

for (const required of ['提示词编辑器', '优化提示词', '图片编辑来源', '不会发送给文本模型', 'estimated_points', '原提示词', '优化后', '应用优化', '撤销提示词优化']) {
  if (!dialog.includes(required)) throw new Error(`expanded prompt editor must include ${required}`)
}
for (const required of ['<PromptEditorActions', 'onExpand={() => setPromptExpanded(true)}', '<PromptEditorDialog', 'estimatePromptOptimization', 'optimizePrompt', 'applyOptimization', 'undoOptimization']) {
  if (!workspace.includes(required)) throw new Error(`workspace prompt optimization must include ${required}`)
}
if ((workspace.match(/<PromptEditorActions/g) ?? []).length !== 1 || !dialog.includes('<PromptEditorActions')) {
  throw new Error('optimize control must appear in both compact and expanded prompt editors')
}
if (!dialog.includes('max-[600px]:h-[calc(100dvh-3rem)]') || !dialog.includes('max-[600px]:max-h-[calc(100dvh-3rem)]')) {
  throw new Error('mobile prompt editor must fit inside the modal backdrop padding without leaving the viewport')
}
if (!redesignClasses.includes("modalBackdrop: 'fixed inset-0 z-[110]")) {
  throw new Error('modal backdrop must render above the toast stack so notifications cannot cover dialog titles')
}
