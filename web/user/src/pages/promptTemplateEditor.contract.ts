import { readFileSync } from 'node:fs'
import {
  buildPromptReferenceBindings,
  promptTemplateSegments,
  promptTemplateText,
  promptVariableNames,
  promptVariableValidation,
  renamePromptReference,
} from './promptTemplateEditorModel'

function equal(actual: unknown, expected: unknown, message: string) {
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(`${message}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`)
  }
}

const template = '让 {{@主体}} 穿着 {{$服装}}，站在 {{@背景}} 前。再次使用 {{$服装}}。'
const segments = promptTemplateSegments(template)
equal(promptTemplateText(segments), template, 'editor segments must round-trip canonical template text')
equal(promptVariableNames(template), ['服装'], 'variable form must deduplicate by first occurrence')
equal(promptVariableValidation(template, { 服装: '' }), {
  valid: false,
  missing: ['服装'],
  tooLong: [],
}, 'empty variables must block task creation')
equal(promptVariableValidation(template, { 服装: '蓝色风衣' }), {
  valid: true,
  missing: [],
  tooLong: [],
}, 'filled variables must pass task creation validation')

const assets = [
  { id: 'ref-1', name: '主体' },
  { id: 'ref-2', name: '背景' },
]
equal(buildPromptReferenceBindings(template, assets), {
  bindings: [
    { name: '主体', asset_id: 'ref-1' },
    { name: '背景', asset_id: 'ref-2' },
  ],
  unresolved: [],
}, 'reference bindings must follow first template occurrence')
equal(buildPromptReferenceBindings('{{@主体}} 与 {{@缺失}}', assets), {
  bindings: [{ name: '主体', asset_id: 'ref-1' }],
  unresolved: ['缺失'],
}, 'unresolved references must be reported')
equal(renamePromptReference('{{@主体}} 与 {{$主体}}，再次 {{@主体}}', '主体', '人物'), '{{@人物}} 与 {{$主体}}，再次 {{@人物}}', 'renaming a resource must only update matching resource tokens')

const editorSource = readFileSync(new URL('./PromptTemplateEditor.tsx', import.meta.url), 'utf8')
for (const marker of [
  'PromptTokenNode',
  'data-prompt-token-kind',
  'HistoryPlugin',
  'LexicalPlainTextPlugin',
  'compositionstart',
  'aria-invalid',
  '插入资产',
  '插入变量',
]) {
  if (!editorSource.includes(marker)) throw new Error(`prompt editor must implement ${marker}`)
}
if (editorSource.includes('LexicalRichTextPlugin')) {
  throw new Error('prompt editor must use Lexical plain-text Enter semantics so one Enter serializes to one newline')
}
for (const marker of ['tabIndex = 0', 'onFocus=', 'onBlur=', 'aria-label']) {
  if (!editorSource.includes(marker)) throw new Error(`prompt tokens must expose keyboard preview behavior through ${marker}`)
}

const variableFormSource = readFileSync(new URL('./PromptVariableForm.tsx', import.meta.url), 'utf8')
if (!variableFormSource.includes('<textarea') || variableFormSource.includes('<input')) {
  throw new Error('prompt variable values must use multiline textarea controls')
}

const workspaceSource = readFileSync(new URL('./WorkspacePage.tsx', import.meta.url), 'utf8')
for (const marker of ['reference_bindings:', 'prompt_variables:', '<PromptVariableForm', '<PromptTemplateEditor', 'pendingPromptAssetInsertRef', "insertToken('reference'"]) {
  if (!workspaceSource.includes(marker)) throw new Error(`workspace must wire ${marker}`)
}

const historySource = readFileSync(new URL('./workspaceCreationDraft.ts', import.meta.url), 'utf8')
if (!historySource.includes('reference_asset_ids: []')) {
  throw new Error('history reuse must start without reference assets')
}

const appSource = readFileSync(new URL('../App.tsx', import.meta.url), 'utf8')
if (!appSource.includes("const WorkspacePage = lazy(async () => ({ default: (await import('./pages/WorkspacePage')).WorkspacePage }))")) {
  throw new Error('the Lexical workspace must be route-lazy-loaded out of the initial application bundle')
}
if (!appSource.includes('<Suspense fallback={<WorkspaceRouteFallback />}><WorkspacePage initialTaskId={routeTaskId} /></Suspense>')) {
  throw new Error('the lazy workspace route must render inside a stable Suspense boundary')
}
