import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./ApiKeysPage.tsx', import.meta.url), 'utf8')

for (const forbidden of ['docsUrl', 'openDocsEntry', '/developer-docs/']) {
  if (source.includes(forbidden)) {
    throw new Error(`API Keys must not depend on external documentation wiring: ${forbidden}`)
  }
}

if (!source.includes("app.navigate('docs')")) {
  throw new Error('API Keys documentation action must navigate to the embedded docs page')
}
