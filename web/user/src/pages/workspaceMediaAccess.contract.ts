import { readFileSync } from 'node:fs'

const read = (file: string) => readFileSync(new URL(file, import.meta.url), 'utf8')
const workspace = read('./WorkspacePage.tsx')
const promptEditor = read('./PromptEditorDialog.tsx')

for (const required of [
  "mediaAccess.preview({ kind: 'image', scope: 'private', id: imageId })",
  "mediaAccess.preview({ kind: 'reference', scope: 'private', id: assetId })",
  "mediaAccess.download({ kind: 'image', scope: 'private', id: image.id })",
  'userApi.importReferenceAssetsFromGallery([image.id])',
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
  'importReferenceAssetsFromGallery([addition.item.id], selectedProjectID)',
]) {
  if (workspace.includes(removed)) {
    throw new Error(`workspace media access must remove stale URL flow ${removed}`)
  }
}

// Reference imports must stay inside the atomic admission callback, after handler validation.
const editHandler = workspace.slice(workspace.indexOf('async function applyAsEditSource'), workspace.indexOf('function removeEditAsset'))
const importCall = editHandler.indexOf('userApi.importReferenceAssetsFromGallery([image.id])')
const formatGuard = editHandler.indexOf("if (invalid) { app.notify('error', invalid); return }")
if (!(formatGuard >= 0 && importCall > formatGuard) || !editHandler.includes('await admitReferences([image], (item) => item.id, () => userApi.importReferenceAssetsFromGallery([image.id]))')) {
  throw new Error('history reference import must validate the asset and invoke its ID-based API only inside atomic admission')
}
const admission = workspace.slice(workspace.indexOf('async function admitReferences'), workspace.indexOf('function referenceAssetError'))
const execute = admission.indexOf('await load()')
for (const guard of ['restoreGeneration.pending(restoreContext.current)', 'reservations.reserve', 'if (!request.ok || request.token === undefined)']) {
  if (admission.indexOf(guard) < 0 || admission.indexOf(guard) >= execute) throw new Error(`reference import must guard ${guard} before executing its API callback`)
}
if (!admission.includes('reserved: reservations.pending(request.token)') || !admission.includes('if (!result.ok) throw new Error(result.reason)')) {
  throw new Error('reference import completion must revalidate current admission before updating references')
}

if (!promptEditor.includes('RefreshableMediaImage') || !promptEditor.includes('onMediaRefresh')) {
  throw new Error('prompt editor reference previews must refresh the affected reference asset by ID')
}
