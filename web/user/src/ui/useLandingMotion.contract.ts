import { readFileSync } from 'node:fs'
import { landingMotionParadigms, landingMotionSelectors } from './useLandingMotion'

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
