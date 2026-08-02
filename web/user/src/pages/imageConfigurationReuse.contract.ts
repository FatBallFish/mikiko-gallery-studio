import { readFileSync } from 'node:fs'

const read = (path: string) => readFileSync(new URL(path, import.meta.url), 'utf8')
const components = read('../components.tsx')
const gallery = read('./GalleryPage.tsx')
const publicGallery = read('./PublicGalleryPage.tsx')
const workspace = read('./WorkspacePage.tsx')

if (components.includes('const copyConfig') || components.includes('>复制配置</button>')) {
  throw new Error('image detail must not copy JSON configuration text')
}
for (const [name, source] of [['gallery', gallery], ['public gallery', publicGallery]] as const) {
  for (const required of ['workspaceCreationDraftFromSnapshot', 'stageWorkspaceCreationDraft', "app.navigate('genpic')", "label: '复用配置'"]) {
    if (!source.includes(required)) throw new Error(`${name} must stage and navigate a typed creation draft: ${required}`)
  }
  if (source.includes('galleryEditContextKey') || source.includes('createGalleryEditContext')) {
    throw new Error(`${name} must not retain the obsolete gallery edit JSON context`)
  }
}
for (const required of [
  'consumeWorkspaceCreationDraft',
  'pendingCreationDraftRef',
  'normalizeWorkspaceCreationDraft',
  'userApi.getReferenceAsset(assetID)',
  'setOutputFormat(values.output_format)',
  'setOutputCompression(values.output_compression)',
  'setModeration(values.moderation)',
  '<ImageDetailModal',
  "label: '复用配置'",
  "app.notify('success', 'Prompt 已复制')",
]) {
  if (!workspace.includes(required)) throw new Error(`workspace history reuse/detail integration is missing: ${required}`)
}
if (!workspace.includes('setCount((current) => normalizeWorkspaceImageCount(restoreParameters?.imageCount ?? current))')) {
  throw new Error('parameter restoration must preserve later user count changes with a functional state update')
}
if (/\[taskType, capability, selectedModel, availableModels, sizeModes, baseResolutionOptionsForModel, ratios, pixelSizes, outputOptions, count\]/.test(workspace)) {
  throw new Error('user count changes must not rerun the whole parameter restoration effect')
}
if (workspace.includes('parseGalleryEditContext') || workspace.includes('galleryEditContextKey')) {
  throw new Error('workspace must not retain the obsolete gallery edit JSON context')
}
