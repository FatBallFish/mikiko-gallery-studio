// @ts-ignore contract scripts run in tsx/node; browser tsconfigs do not include node types.
import { readFileSync } from 'node:fs'
import { API_PATHS, type MediaAccessProjection, type MediaAccessPurpose } from './api-types'
import { userApi } from './user-api'

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message)
}

assert(API_PATHS.agent.imageAccess === '/api/agent/image/v1/images/{image_id}/access', 'private image access path drifted')
assert(API_PATHS.agent.referenceAssetAccess === '/api/agent/image/v1/reference-assets/{asset_id}/access', 'reference asset access path drifted')
assert(API_PATHS.open.galleryImageAccess === '/api/open/image/v1/gallery/images/{image_id}/access', 'public image access path drifted')

const methods: Array<keyof typeof userApi> = [
  'refreshImageAccess',
  'refreshReferenceAssetAccess',
  'refreshPublicImageAccess',
]
assert(methods.length === 3, 'media access clients are incomplete')

const purpose: MediaAccessPurpose = 'download'
const projection: MediaAccessProjection = { url: 'https://objects.example.test/image', expires_at: '2026-08-06T12:05:00Z' }
void [purpose, projection]

const openAPI = readFileSync(new URL('../../api/openapi/openapi.yaml', import.meta.url), 'utf8')
for (const path of [API_PATHS.agent.imageAccess, API_PATHS.agent.referenceAssetAccess, API_PATHS.open.galleryImageAccess]) {
  assert(openAPI.includes(`  ${path}:`), `OpenAPI is missing ${path}`)
}
