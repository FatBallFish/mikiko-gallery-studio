import { readFileSync } from 'node:fs'
import { userApi } from '../../../shared/user-api'

const source = readFileSync(new URL('./HomePage.tsx', import.meta.url), 'utf8')

if (!source.includes("openApi.listPublicGallery(1, 12, { sort: 'hot', accessToken: null })")) {
  throw new Error('home curated gallery must request public assets without the user session token')
}
if (source.includes('image.prompt || image.prompt_excerpt')) {
  throw new Error('home public cards and lightbox must never prefer a full prompt over prompt_excerpt')
}
if (!source.includes('homePublicDetailImage(resolvedImage)')) {
  throw new Error('home public detail must sanitize public list images before opening the detail modal')
}
for (const required of [
  'openApi.getPublicGalleryImage(image.id, { accessToken: app.session?.token })',
  'detail.prompt',
  'publicDetailRequestRef',
]) {
  if (!source.includes(required)) throw new Error(`home full prompt copy must implement ${required}`)
}

const publicURL = userApi.imageAssetUrl('/v1/open/gallery/images/public-1/download', null)
if (publicURL.includes('access_token') || publicURL.includes('session')) {
  throw new Error(`public home asset URL must be token-free, got ${publicURL}`)
}
