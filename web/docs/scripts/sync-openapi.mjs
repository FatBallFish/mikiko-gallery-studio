import { cpSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parse, stringify } from 'yaml'

const scriptDir = dirname(fileURLToPath(import.meta.url))
const root = resolve(scriptDir, '../../..')
const source = resolve(root, 'api/openapi')
const target = resolve(root, 'web/docs/public/openapi')

rmSync(target, { force: true, recursive: true })
mkdirSync(target, { recursive: true })
cpSync(source, target, {
  recursive: true,
  filter: (path) => !path.endsWith('.go'),
})

const documentPath = resolve(target, 'openapi.yaml')
const document = parse(readFileSync(documentPath, 'utf8'))
// Developer consumers must not receive browser/admin/cluster-enrollment contracts.
// Public gallery routes and callbacks keep their original trust boundaries.
const visiblePath = (path) => path.startsWith('/api/open/v1/')
  || path.startsWith('/v1/')
  || path.startsWith('/api/open/image/v1/gallery/')
  || path.startsWith('/api/public/v1/')
  || path.startsWith('/api/open/image/v1/payments/webhooks/')
  || path.startsWith('/api/open/video/v1/provider-callbacks/')
  || path.startsWith('/.well-known/')
  || path.startsWith('/oauth/')
  || path === '/mcp'
document.paths = Object.fromEntries(
  Object.entries(document.paths ?? {}).filter(([path]) => visiblePath(path)),
)

const visibleTags = new Set(
  Object.values(document.paths).flatMap((pathItem) => Object.values(pathItem ?? {}).flatMap((operation) => operation?.tags ?? [])),
)
document.tags = (document.tags ?? []).filter((tag) => visibleTags.has(tag.name))
document.info = {
  ...document.info,
  title: 'Pic Gallery Developer API',
  description: 'Unified Bearer developer API, OpenAI-compatible images, OAuth MCP, public gallery and separately authenticated callbacks. Generated from api/openapi; do not edit this copy.',
}

writeFileSync(documentPath, stringify(document, { lineWidth: 0 }))
console.log(`Synced developer OpenAPI contract (${Object.keys(document.paths).length} public paths)`)
