import { readFileSync } from 'node:fs'
import { userApi } from '../../../shared/user-api'
import { publicMediaApi } from '../../../shared/public-media-api'
import { sharedApiClient } from '../../../shared/http-client'

const source = readFileSync(new URL('./HomePage.tsx', import.meta.url), 'utf8')

if (!source.includes("publicMediaApi.list(1, 6, { sort: 'hot', accessToken: null })")) {
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

const originalFetch = globalThis.fetch
sharedApiClient.setAuth({ getToken: () => 'private-home-session' })
try {
  let calls = 0
  globalThis.fetch = async (input, init) => {
    calls += 1
    const url = new URL(String(input), 'http://localhost')
    if (url.pathname !== '/api/public/v1/media' || url.searchParams.get('page') !== '1' || url.searchParams.get('page_size') !== '6' || url.searchParams.get('sort') !== 'hot') throw new Error(`home must request one bounded, server-sorted mixed page: ${url}`)
    if (new Headers(init?.headers).has('Authorization') || url.searchParams.has('access_token')) throw new Error('anonymous home feed must not send the private session')
    return new Response(JSON.stringify({ data: { items: [
      { id: 'same-id', media_type: 'video', video: { id: 'same-id', title: 'clip', media_type: 'video', status: 'approved', video_url: '/video', poster_url: '/poster', created_at: '' } },
      { id: 'same-id', media_type: 'image', image: { id: 'same-id', url: '/public/image', prompt: null, prompt_excerpt: 'summary', visibility_status: 'approved', width: 100, height: 100 } },
    ], pagination: { page: 1, page_size: 6, total: 2 } }, meta: { request_id: 'home-mixed-feed-contract' } }), { status: 200, headers: { 'Content-Type': 'application/json' } })
  }
  const page = await publicMediaApi.list(1, 6, { sort: 'hot', accessToken: null })
  if (calls !== 1 || page.items.length !== 2 || page.items[0].media_type !== 'video' || page.items[1].media_type !== 'image') throw new Error('mixed feed must preserve server order and distinct media kinds even when IDs coincide')
  if (page.items[1].media_type === 'image' && (page.items[1].image.url !== '/public/image' || page.items[1].image.prompt_excerpt !== 'summary')) throw new Error('mixed feed must preserve the image public projection')
} finally {
  globalThis.fetch = originalFetch
  sharedApiClient.setAuth({})
}

const publicURL = userApi.imageAssetUrl('/v1/open/gallery/images/public-1/download', null)
if (publicURL.includes('access_token') || publicURL.includes('session')) {
  throw new Error(`public home asset URL must be token-free, got ${publicURL}`)
}
