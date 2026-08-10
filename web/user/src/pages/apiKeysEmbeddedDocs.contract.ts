import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./ApiKeysPage.tsx', import.meta.url), 'utf8')

for (const forbidden of ["app.navigate('docs')", '/developer-docs/']) {
  if (source.includes(forbidden)) throw new Error(`API Keys must not use an intermediate or hard-coded documentation route: ${forbidden}`)
}

if (!source.includes("import { openDocsEntry } from '../docsUrl'") || !source.includes("openDocsEntry('api-keys')")) {
  throw new Error('API Keys documentation action must open the centrally resolved documentation URL directly')
}
