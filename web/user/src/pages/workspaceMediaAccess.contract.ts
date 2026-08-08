import { readFileSync } from 'node:fs'

const read = (file: string) => readFileSync(new URL(file, import.meta.url), 'utf8')
const workspace = read('./WorkspacePage.tsx')
const promptEditor = read('./PromptEditorDialog.tsx')

for (const required of [
  "mediaAccess.preview({ kind: 'image', scope: 'private', id: imageId })",
  "mediaAccess.preview({ kind: 'reference', scope: 'private', id: assetId })",
  "mediaAccess.download({ kind: 'image', scope: 'private', id: image.id })",
  'userApi.importReferenceAssetsFromGallery([addition.item.id], selectedProjectID)',
  'onUseReference: (image: ImageResult) => Promise<void>',
  'setGalleryImages((items) => items.map((image) => image.id === imageId',
  'onMediaRefresh={() => onMediaRefresh(image.id)}',
  'onMediaRefresh={() => refreshWorkspaceReference(asset.id)}',
  'userApi.imageAssetUrl(projection.url, app.session?.token)',
]) {
  if (!workspace.includes(required)) {
    throw new Error(`workspace media access must include ${required}`)
  }
}

for (const removed of [
  'async function refreshWorkspaceMedia()',
  'await fetch(addition.item)',
  'onUseReference(imageUrl)',
  "window.open(downloadUrl, '_blank'",
]) {
  if (workspace.includes(removed)) {
    throw new Error(`workspace media access must remove stale URL flow ${removed}`)
  }
}

if (!promptEditor.includes('RefreshableMediaImage') || !promptEditor.includes('onMediaRefresh')) {
  throw new Error('prompt editor reference previews must refresh the affected reference asset by ID')
}
