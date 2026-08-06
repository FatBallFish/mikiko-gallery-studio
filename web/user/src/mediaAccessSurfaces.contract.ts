import { readFileSync } from 'node:fs'

const read = (file: string) => readFileSync(new URL(file, import.meta.url), 'utf8')
const home = read('./pages/HomePage.tsx')
const publicGallery = read('./pages/PublicGalleryPage.tsx')
const gallery = read('./pages/GalleryPage.tsx')
const workspace = read('./pages/WorkspacePage.tsx')
const sharedTypes = read('../../shared/api-types.ts')

for (const typeContract of [
  'preview_expires_at?: string',
  'download_expires_at?: string',
]) {
  if (!sharedTypes.includes(typeContract)) {
    throw new Error(`shared media payloads must expose ${typeContract}`)
  }
}

for (const [name, source] of [['Home', home], ['PublicGallery', publicGallery]] as const) {
  if (!source.includes("mediaAccess.preview({ kind: 'image', scope: 'public'")) {
    throw new Error(`${name} must refresh public previews by stable image ID`)
  }
  if (!source.includes("mediaAccess.download({ kind: 'image', scope: 'public'")) {
    throw new Error(`${name} must refresh public downloads at click time`)
  }
  if (name === 'PublicGallery' && !source.includes('mediaExpiresAt={image.preview_expires_at}')) {
    throw new Error(`${name} must schedule preview refresh from payload expiry metadata`)
  }
}
if (home.includes('onMediaRefresh={() => void publicGallery.reload()}')) {
  throw new Error('Home media refresh must not reload the public gallery list')
}
for (const contract of [
  'const [publicImageAccess, setPublicImageAccess]',
  'setPublicImageAccess((current) =>',
  'publicImageAccess[image.id]?.expires_at ?? image.preview_expires_at',
]) {
  if (!home.includes(contract)) {
    throw new Error(`Home must retain refreshed URL and expiry per card for repeated proactive refresh: missing ${contract}`)
  }
}
if (publicGallery.includes("onMediaRefresh={() => void loadPage(1, 'replace')}")) {
  throw new Error('PublicGallery media refresh must not reload its first page')
}
if (!gallery.includes("mediaAccess.preview({ kind: 'image', scope: 'private'")) {
  throw new Error('Gallery must refresh private previews by stable image ID')
}
if (!gallery.includes("mediaAccess.download({ kind: 'image', scope: 'private'")) {
  throw new Error('Gallery must refresh every download by stable image ID')
}
if (!gallery.includes('userApi.imageAssetUrl(projection.url, app.session?.token)')) {
  throw new Error('Gallery must authenticate relative LocalBackend access projections')
}
if (gallery.includes('reloadLoadedPages')) {
  throw new Error('Gallery media refresh must not reload already loaded pages')
}
if (home.includes("window.open(projection.url, '_blank'")) {
  throw new Error('Home downloads must not rely on a popup after awaiting URL refresh')
}
if (!gallery.includes('mediaExpiresAt={image.preview_expires_at}')) {
  throw new Error('Gallery must schedule preview refresh from payload expiry metadata')
}
if (!workspace.includes('mediaExpiresAt={image.preview_expires_at}')) {
  throw new Error('Workspace must schedule generated-image refresh from payload expiry metadata')
}
if (!workspace.includes('mediaExpiresAt={asset.preview_expires_at}')) {
  throw new Error('Workspace must schedule reference-image refresh from payload expiry metadata')
}
for (const [name, source] of [['Home', home], ['PublicGallery', publicGallery], ['Gallery', gallery], ['Workspace', workspace]] as const) {
  if (!source.includes('preview_expires_at: projection.expires_at')) {
    throw new Error(`${name} must replace preview URL and expiry metadata atomically after refresh`)
  }
}
