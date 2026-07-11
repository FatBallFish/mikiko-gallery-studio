import { readFileSync } from 'node:fs'

const landingSource = readFileSync(new URL('./LandingPage.tsx', import.meta.url), 'utf8')
const motionSource = readFileSync(new URL('../ui/useLandingMotion.ts', import.meta.url), 'utf8')
const appSource = readFileSync(new URL('../App.tsx', import.meta.url), 'utf8')

for (const banned of [
  'mpdhezm8-image',
  'mpdhfj5l-image',
  'reference-mother-v1',
  'ViduClaw',
  'PIC GALLERY Atelier',
  'role="listitem"',
]) {
  if (landingSource.includes(banned)) throw new Error(`landing source contains banned asset, copy, or role: ${banned}`)
}

for (const required of [
  'data-landing-title-line',
  'text-[1.5rem]',
  'min-[360px]:text-[1.75rem]',
  '/landing/hero-gallery.webp',
  '/landing/workspace.webp',
  'fetchPriority="high"',
  'loading="eager"',
  'loading="lazy"',
  'decoding="async"',
  'data-landing-image-overlay',
]) {
  if (!landingSource.includes(required)) throw new Error(`landing source is missing required responsive, asset, loading, or motion contract: ${required}`)
}

if (motionSource.includes('filter:')) {
  throw new Error('landing scroll motion must not animate CSS filters')
}
if (!motionSource.includes("overlay: '[data-landing-image-overlay]'")) {
  throw new Error('landing scroll motion must target a separate luminance overlay')
}

if (!appSource.includes("lazy(() => import('./pages/LandingPage'))")) {
  throw new Error('LandingPage must be lazy-loaded from App')
}
if (!appSource.includes('<Suspense')) {
  throw new Error('lazy landing route must render inside Suspense')
}
