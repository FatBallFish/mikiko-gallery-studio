// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { existsSync, readFileSync } from 'node:fs'

const motionURL = new URL('./adminMotion.ts', import.meta.url)
if (!existsSync(motionURL)) throw new Error('bounded admin motion helper must exist')

const motionSource = readFileSync(motionURL, 'utf8')
const appSource = readFileSync(new URL('../App.tsx', import.meta.url), 'utf8')
const componentsSource = readFileSync(new URL('../components.tsx', import.meta.url), 'utf8')
const reviewSource = readFileSync(new URL('../pages/ReviewPage.tsx', import.meta.url), 'utf8')
const packageSource = readFileSync(new URL('../../package.json', import.meta.url), 'utf8')

for (const dependency of ['"gsap"', '"@gsap/react"']) {
  if (!packageSource.includes(dependency)) throw new Error(`admin motion must bundle ${dependency}`)
}

for (const contract of [
  'useGSAP',
  'revertOnUpdate: true',
  "matchMedia('(prefers-reduced-motion: reduce)')",
  'duration: 0.18',
  'duration: 0.24',
  'clearProps',
]) {
  if (!motionSource.includes(contract)) throw new Error(`admin motion must implement ${contract}`)
}

for (const forbidden of ['ScrollTrigger', 'pin:', 'magnetic']) {
  if (motionSource.includes(forbidden)) throw new Error(`operations motion must not use ${forbidden}`)
}

if (!appSource.includes('useAdminPageMotion') || !componentsSource.includes('useAdminLayerMotion') || !reviewSource.includes('useAdminPreviewMotion')) {
  throw new Error('page, layer, and review-preview motion must use the shared bounded helpers')
}
