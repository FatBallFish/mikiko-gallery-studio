import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const assetsDir = join(repositoryRoot, 'web/user/dist/assets')
const jsFiles = readdirSync(assetsDir).filter((file) => file.endsWith('.js'))
const landingChunk = jsFiles.find((file) => file.startsWith('LandingPage-'))
if (!landingChunk) throw new Error(`landing route chunk is missing: ${jsFiles.join(', ')}`)

const landingCode = readFileSync(join(assetsDir, landingChunk), 'utf8')
if (!landingCode.includes('ScrollTrigger')) throw new Error('landing route chunk does not contain ScrollTrigger')

const entryFiles = jsFiles.filter((file) => file.startsWith('index-'))
for (const entryFile of entryFiles) {
  const entryCode = readFileSync(join(assetsDir, entryFile), 'utf8')
  if (entryCode.includes('ScrollTrigger')) throw new Error(`ScrollTrigger leaked into authenticated entry chunk: ${entryFile}`)
}

console.log(`OK: landing bundle split contract passed (${landingChunk})`)
