import { readFileSync } from 'node:fs'
import { userApi } from '../../../shared/user-api'

const source = readFileSync(new URL('./HomePage.tsx', import.meta.url), 'utf8')

if (!source.includes("openApi.listPublicGallery(1, 12, { sort: 'hot', accessToken: null })")) {
  throw new Error('home curated gallery must request public assets without the user session token')
}
if (source.includes('app.session?.token')) {
  throw new Error('home curated public image URLs must never include the authenticated session token')
}
if (source.includes('image.prompt || image.prompt_excerpt')) {
  throw new Error('home public cards and lightbox must never prefer a full prompt over prompt_excerpt')
}
for (const excerptOnly of ['alt: image.prompt_excerpt || image.id', 'prompt: image.prompt_excerpt']) {
  if (!source.includes(excerptOnly)) {
    throw new Error(`home public lightbox must use only prompt_excerpt: ${excerptOnly}`)
  }
}

const publicURL = userApi.imageAssetUrl('/v1/open/gallery/images/public-1/download', null)
if (publicURL.includes('access_token') || publicURL.includes('session')) {
  throw new Error(`public home asset URL must be token-free, got ${publicURL}`)
}
