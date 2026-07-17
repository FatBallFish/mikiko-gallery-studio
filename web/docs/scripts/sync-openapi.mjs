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
document.paths = Object.fromEntries(
  Object.entries(document.paths ?? {}).filter(([path]) => path.startsWith('/api/open/') || path.startsWith('/v1/')),
)

const visibleTags = new Set(
  Object.values(document.paths).flatMap((pathItem) => Object.values(pathItem ?? {}).flatMap((operation) => operation?.tags ?? [])),
)
document.tags = (document.tags ?? []).filter((tag) => visibleTags.has(tag.name))
document.info = {
  ...document.info,
  title: 'Pic Gallery Developer API',
  description: 'Public image generation APIs for native AK/SK integrations and OpenAI-compatible clients.',
}

writeFileSync(documentPath, stringify(document, { lineWidth: 0 }))
console.log(`Synced developer OpenAPI contract (${Object.keys(document.paths).length} public paths)`)
