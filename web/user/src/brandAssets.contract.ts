import { readFileSync } from 'node:fs'

const brandSource = readFileSync(new URL('./brand.tsx', import.meta.url), 'utf8')
const userFavicon = readFileSync(new URL('../public/favicon.svg', import.meta.url), 'utf8')
const adminFavicon = readFileSync(new URL('../../admin/public/favicon.svg', import.meta.url), 'utf8')
const adminIndex = readFileSync(new URL('../../admin/index.html', import.meta.url), 'utf8')
const adminLoginCopy = readFileSync(new URL('../../admin/src/pages/adminLoginCopy.ts', import.meta.url), 'utf8')
const adminComponents = readFileSync(new URL('../../admin/src/components.tsx', import.meta.url), 'utf8')

if (!brandSource.includes("new URL('./assets/mikiko-mark.svg', import.meta.url)")) {
  throw new Error('user brand must use the compact SVG Mikiko mark')
}
if (brandSource.includes('mikiko-studio.png')) {
  throw new Error('the legacy 1 MB raster logo must not ship in the user bundle')
}

for (const [name, source] of [['user', userFavicon], ['admin', adminFavicon]] as const) {
  for (const required of ['viewBox="0 0 64 64"', 'aria-hidden="true"', '<path']) {
    if (!source.includes(required)) throw new Error(`${name} favicon is missing ${required}`)
  }
}

if (userFavicon === adminFavicon) {
  throw new Error('user and admin favicons should share a system while remaining distinguishable')
}

for (const source of [adminIndex, adminLoginCopy, adminComponents]) {
  if (source.includes('Pic Gallery')) throw new Error('admin browser and shell branding must use Mikiko Gallery Studio')
}
if (!adminIndex.includes('<title>Mikiko Gallery Studio Admin</title>')) {
  throw new Error('admin browser title must identify the Mikiko Gallery Studio console')
}

console.log('OK: Mikiko production brand asset contract passed')
