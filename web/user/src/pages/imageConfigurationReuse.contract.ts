import { readFileSync } from 'node:fs'
import { nativeImageCountLimit, nativeImageProfile } from '../../../shared/native-image'
import { normalizeWorkspaceImageCount, workspaceTaskImageSafetyLimit } from './workspaceViewModel'

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
const restoreCountStatement = workspace.match(/setCount\(\(current\) => [^\n]+restoreParameters\?\.imageCount[^\n]+/u)?.[0]
if (!restoreCountStatement) throw new Error('parameter restoration must use a functional count update')
function restoredCount(current: number, imageCount: number | undefined, imageCountLimit: number) {
  let result = current
  new Function('setCount', 'restoreParameters', 'imageCountLimit', 'normalizeWorkspaceImageCount', restoreCountStatement! )(
    (update: (current: number) => number) => { result = update(current) },
    imageCount === undefined ? null : { imageCount }, imageCountLimit, normalizeWorkspaceImageCount,
  )
  return result
}
for (const [model, expectedLimit] of [['doubao-seedream-5-0-pro-260628', 1], ['qwen-image-3.0', 6]] as const) {
  const limit = nativeImageCountLimit(nativeImageProfile(model), workspaceTaskImageSafetyLimit)
  if (limit !== expectedLimit || restoredCount(2, 8, limit) !== expectedLimit || restoredCount(8, undefined, limit) !== expectedLimit) {
    throw new Error(`${model} must cap both restored and current counts at its native output limit`)
  }
}
if (restoredCount(4, undefined, workspaceTaskImageSafetyLimit) !== 4
  || restoredCount(8, 2, workspaceTaskImageSafetyLimit) !== 2
  || restoredCount(4, 0, workspaceTaskImageSafetyLimit) !== normalizeWorkspaceImageCount(0)) {
  throw new Error('count restoration must prefer saved counts, normalize them, and preserve later user changes when absent')
}
if (/\[taskType, capability, selectedModel, availableModels, sizeModes, baseResolutionOptionsForModel, ratios, pixelSizes, outputOptions, count\]/.test(workspace)) {
  throw new Error('user count changes must not rerun the whole parameter restoration effect')
}
if (workspace.includes('parseGalleryEditContext') || workspace.includes('galleryEditContextKey')) {
  throw new Error('workspace must not retain the obsolete gallery edit JSON context')
}
