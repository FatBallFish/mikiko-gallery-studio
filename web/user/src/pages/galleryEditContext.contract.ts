import { createGalleryEditContext, parseGalleryEditContext } from './galleryEditContext'

const publicSameContext = createGalleryEditContext({
  prompt: 'sunset over glass city',
  routeModelCode: 'plus',
  baseResolution: '2K',
  aspectRatio: '16:9',
})

const routeModelCode: string | undefined = publicSameContext.route_model_code
const baseResolution: string | undefined = publicSameContext.base_resolution
const aspectRatio: string | undefined = publicSameContext.aspect_ratio

if (routeModelCode !== 'plus' || baseResolution !== '2K' || aspectRatio !== '16:9') {
  throw new Error('public same-generation context must preserve generation parameters')
}

if (publicSameContext.fallbackImageUrl) {
  throw new Error('public same-generation context must not turn into image edit implicitly')
}

const parsed = parseGalleryEditContext(JSON.stringify({
  prompt: 'legacy prompt',
  routeModelCode: 'basic',
  aspectRatio: '1:1',
  baseResolution: '1K',
}))

if (!parsed || parsed.route_model_code !== 'basic' || parsed.base_resolution !== '1K' || parsed.quality !== '1K' || parsed.aspect_ratio !== '1:1') {
  throw new Error('workspace restore must accept legacy camelCase context keys')
}

const legacyQuality = parseGalleryEditContext(JSON.stringify({
  prompt: 'legacy quality prompt',
  quality: '2k',
}))

if (!legacyQuality || legacyQuality.base_resolution !== '2k' || legacyQuality.quality !== '2k') {
  throw new Error('legacy gallery context quality must normalize to base_resolution and keep the workspace alias')
}

const currentResolution = parseGalleryEditContext(JSON.stringify({
  prompt: 'current resolution prompt',
  base_resolution: '4K',
}))

if (!currentResolution || currentResolution.base_resolution !== '4K' || currentResolution.quality !== '4K') {
  throw new Error('current gallery context must read base_resolution and keep the transition quality alias')
}
