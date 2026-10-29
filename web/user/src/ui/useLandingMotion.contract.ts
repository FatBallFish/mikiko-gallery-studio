import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { landingMotionParadigms, landingMotionSelectors } from './useLandingMotion'

// Also check the hook's actual plugin entry without tsx: loader interop must
// never hide a missing named export in the Node version used by release CI.
const motionSource = readFileSync(new URL('./useLandingMotion.ts', import.meta.url), 'utf8')
const pluginEntry = motionSource.match(/import\s+\{\s*ScrollTrigger\s*\}\s+from\s+(['"])([^'"]+)\1/)?.[2]
assert.ok(pluginEntry, 'Landing hook must import the real ScrollTrigger plugin')
const coreEntry = motionSource.match(/import\s+gsapModule[^\n]*from\s+(['"])([^'"]+)\1/)?.[2]
assert.ok(coreEntry, 'Landing hook must import the real GSAP runtime')
const nativeImport = spawnSync(process.execPath, ['--input-type=module', '-e', `
  import assert from 'node:assert/strict'
  import gsapModule from ${JSON.stringify(coreEntry)}
  import { ScrollTrigger } from ${JSON.stringify(pluginEntry)}
  import { useGSAP } from '@gsap/react'
  import { createElement } from 'react'
  import { act, create } from 'react-test-renderer'
  const gsap = 'gsap' in gsapModule ? gsapModule.gsap : gsapModule
  assert.equal(typeof gsap.registerPlugin, 'function')
  gsap.registerPlugin(useGSAP, ScrollTrigger)
  globalThis.IS_REACT_ACT_ENVIRONMENT = true
  let contexts = 0
  const originalContext = gsap.context
  gsap.context = (...args) => { contexts++; return originalContext(...args) }
  function Probe() { useGSAP(() => {}); return null }
  let tree
  await act(async () => { tree = create(createElement(Probe)) })
  await act(async () => { tree.unmount() })
  gsap.context = originalContext
  assert.ok(contexts > 0, 'useGSAP must use the same core as landing animations')
  const target = { value: 0 }
  const tween = gsap.to(target, { value: 10, duration: 1, paused: true })
  tween.progress(0.5)
  assert.ok(target.value > 0 && target.value < 10)
  tween.kill()
  gsap.ticker.sleep()
  assert.equal(typeof ScrollTrigger, 'function')
  assert.equal(typeof ScrollTrigger.create, 'function')
  assert.equal(typeof ScrollTrigger.refresh, 'function')
`], {
  cwd: fileURLToPath(new URL('../..', import.meta.url)),
  encoding: 'utf8',
  env: { ...process.env, NODE_OPTIONS: '' },
})
assert.equal(nativeImport.status, 0, `ScrollTrigger native Node import failed: ${nativeImport.error ?? nativeImport.stderr}`)

if (landingMotionParadigms.join(',') !== 'scrubbing-text-reveals,image-scale-fade-scroll') {
  throw new Error(`landing motion paradigms drifted: ${landingMotionParadigms.join(',')}`)
}

if (
  landingMotionSelectors.reveal !== '[data-landing-reveal]'
  || landingMotionSelectors.word !== '[data-landing-word]'
  || landingMotionSelectors.image !== '[data-landing-image]'
  || landingMotionSelectors.overlay !== '[data-landing-image-overlay]'
) {
  throw new Error(`landing motion selectors drifted: ${JSON.stringify(landingMotionSelectors)}`)
}

const landingSource = readFileSync(new URL('../pages/LandingPage.tsx', import.meta.url), 'utf8')
for (const requiredSource of [
  'landing-marquee-track',
  'landing-marquee-static',
  'flex-wrap',
  '.landing-marquee-track { display: none !important; }',
  '.landing-marquee-static { display: flex !important; }',
]) {
  if (!landingSource.includes(requiredSource)) {
    throw new Error(`reduced-motion marquee fallback is incomplete: ${requiredSource}`)
  }
}
