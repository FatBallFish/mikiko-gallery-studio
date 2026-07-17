import { existsSync, readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const repositoryRoot = process.env.PIC_GALLERY_REPOSITORY_ROOT
  ? resolve(process.env.PIC_GALLERY_REPOSITORY_ROOT)
  : resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const distDir = join(repositoryRoot, 'web/user/dist')
const manifestPath = join(distDir, '.vite/manifest.json')

if (!existsSync(manifestPath)) {
  throw new Error(`user-web build manifest is missing: ${manifestPath}`)
}

let manifest
try {
  manifest = JSON.parse(readFileSync(manifestPath, 'utf8'))
} catch (error) {
  throw new Error(`user-web build manifest is invalid: ${manifestPath}`, { cause: error })
}

const manifestEntries = Object.entries(manifest)
const entryKeys = manifestEntries
  .filter(([, chunk]) => chunk?.isEntry === true)
  .map(([key]) => key)
if (entryKeys.length === 0) {
  throw new Error('user-web build manifest has no entry chunks')
}

function collectStaticGraph(startKeys) {
  const visited = new Set()
  const pending = [...startKeys]

  while (pending.length > 0) {
    const key = pending.pop()
    if (visited.has(key)) continue

    const chunk = manifest[key]
    if (!chunk) {
      throw new Error(`user-web build manifest references a missing chunk: ${key}`)
    }

    visited.add(key)
    for (const importedKey of chunk.imports ?? []) {
      pending.push(importedKey)
    }
  }

  return visited
}

function readChunk(key) {
  const file = manifest[key]?.file
  if (!file) {
    throw new Error(`user-web build manifest chunk has no output file: ${key}`)
  }

  const chunkPath = join(distDir, file)
  if (!existsSync(chunkPath)) {
    throw new Error(`user-web build chunk is missing: ${file}`)
  }

  return { code: readFileSync(chunkPath, 'utf8'), file }
}

for (const key of collectStaticGraph(entryKeys)) {
  const { code, file } = readChunk(key)
  if (code.includes('ScrollTrigger')) {
    throw new Error(`ScrollTrigger leaked into authenticated entry graph: ${file}`)
  }
}

const landingEntry = manifestEntries.find(([key, chunk]) => {
  const source = chunk?.src ?? key
  return chunk?.isDynamicEntry === true && /(^|\/)pages\/LandingPage\.[cm]?[jt]sx?$/.test(source)
})
if (!landingEntry) {
  throw new Error('landing route is missing from the user-web build manifest')
}

const [landingKey] = landingEntry
const landingGraph = collectStaticGraph([landingKey])
const landingMarker = [...landingGraph]
  .map((key) => readChunk(key))
  .find(({ code }) => code.includes('ScrollTrigger'))
if (!landingMarker) {
  throw new Error(`landing route graph does not contain ScrollTrigger: ${landingKey}`)
}

console.log(`OK: landing bundle split contract passed (${landingMarker.file})`)
