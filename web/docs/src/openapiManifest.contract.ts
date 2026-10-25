import { existsSync, readFileSync } from 'node:fs'
import { parse } from 'yaml'
import { openapiReference } from './openapiManifest'

if (openapiReference.url !== './openapi/openapi.yaml') {
  throw new Error(`OpenAPI source URL drifted: ${openapiReference.url}`)
}
if (!openapiReference.fallbackMessage.includes('接口参考')) {
  throw new Error('OpenAPI failure needs local interface-reference guidance')
}

const document = parse(readFileSync('web/docs/public/openapi/openapi.yaml', 'utf8'))
const paths = Object.keys(document.paths ?? {})
if (!paths.includes('/api/open/v1/image-tasks') || !paths.includes('/api/open/v1/video-tasks') || !paths.includes('/v1/images/generations') || !paths.includes('/mcp') || !paths.includes('/oauth/token')) {
  throw new Error('developer OpenAPI is missing unified developer or OAuth/MCP endpoints')
}
if (paths.some((path) => path.startsWith('/api/agent/') || path.startsWith('/api/ops/') || path.startsWith('/api/open/cluster/') || path.startsWith('/api/v1/oauth/'))) {
  throw new Error('developer OpenAPI must not expose browser, admin or cluster enrollment endpoints')
}
if (paths.some((path) => path.startsWith('/api/open/image/v1/') && !path.includes('/gallery/') && !path.includes('/payments/webhooks/'))) {
  throw new Error('developer OpenAPI must not retain obsolete signed user endpoints')
}
if (document.components?.securitySchemes?.accessSignature) throw new Error('obsolete developer signature auth')
if ((document.tags ?? []).some((tag: { name?: string }) => tag.name?.startsWith('Admin'))) {
  throw new Error('developer OpenAPI must not expose admin tags')
}
if (existsSync('web/docs/public/openapi/openapi_test.go')) {
  throw new Error('developer static assets must not copy Go test files')
}
