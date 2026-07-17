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
if (!paths.includes('/api/open/image/v1/tasks') || !paths.includes('/v1/images/generations')) {
  throw new Error('developer OpenAPI is missing required public image endpoints')
}
if (paths.some((path) => !path.startsWith('/api/open/') && !path.startsWith('/v1/'))) {
  throw new Error('developer OpenAPI must not expose internal agent or admin endpoints')
}
if ((document.tags ?? []).some((tag: { name?: string }) => tag.name?.startsWith('Admin'))) {
  throw new Error('developer OpenAPI must not expose admin tags')
}
if (existsSync('web/docs/public/openapi/openapi_test.go')) {
  throw new Error('developer static assets must not copy Go test files')
}
