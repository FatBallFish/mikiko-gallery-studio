import { useGSAP } from '@gsap/react'
import gsapModule from 'gsap'
import { ScrollTrigger } from 'gsap/dist/ScrollTrigger'
import type { RefObject } from 'react'
import { prefersReducedMotion } from './motion'

// Node 22/tsx wraps GSAP's root ESM entry; browsers expose its default core.
// Keep the root entry shared with @gsap/react instead of bundling a second core.
const gsap = ('gsap' in gsapModule ? gsapModule.gsap : gsapModule) as typeof gsapModule
gsap.registerPlugin(useGSAP, ScrollTrigger)

export const landingMotionParadigms = [
  'scrubbing-text-reveals',
  'image-scale-fade-scroll',
] as const

export const landingMotionSelectors = {
  reveal: '[data-landing-reveal]',
  word: '[data-landing-word]',
  image: '[data-landing-image]',
  overlay: '[data-landing-image-overlay]',
} as const

export function useLandingMotion(scope: RefObject<HTMLElement | null>): void {
  useGSAP(() => {
    if (prefersReducedMotion()) return

    const animations: (ReturnType<typeof gsap.fromTo> | ReturnType<typeof gsap.timeline>)[] = []

    gsap.utils.toArray<HTMLElement>(landingMotionSelectors.reveal).forEach((reveal) => {
      const words = reveal.querySelectorAll<HTMLElement>(landingMotionSelectors.word)
      if (!words.length) return

      animations.push(gsap.fromTo(words, {
        opacity: 0.12,
      }, {
        opacity: 1,
        stagger: 0.08,
        ease: 'none',
        scrollTrigger: {
          trigger: reveal,
          start: 'top 82%',
          end: 'bottom 48%',
          scrub: 0.65,
        },
      }))
    })

    gsap.utils.toArray<HTMLElement>(landingMotionSelectors.image).forEach((image) => {
      // Animate the picture wrapper, never the <img> itself: the prototype's
      // hover transitions own the image transform, and an inline GSAP
      // transform on the image would override them permanently.
      const target = image.closest('picture') ?? image
      const overlay = target.parentElement?.querySelector<HTMLElement>(landingMotionSelectors.overlay)
      const timeline = gsap.timeline({
        scrollTrigger: {
          trigger: target,
          start: 'top 94%',
          end: 'bottom 6%',
          scrub: 0.75,
        },
      })
      timeline
        .fromTo(target, {
          scale: 0.82,
          opacity: 0.38,
        }, {
          scale: 1,
          opacity: 1,
          ease: 'none',
          duration: 0.58,
        })
        .to(target, {
          opacity: 0.2,
          ease: 'none',
          duration: 0.42,
        })
      if (overlay) {
        timeline
          .fromTo(overlay, { opacity: 0.38 }, { opacity: 0, ease: 'none', duration: 0.58 }, 0)
          .to(overlay, { opacity: 0.52, ease: 'none', duration: 0.42 })
      }
      animations.push(timeline)
    })

    return () => {
      animations.forEach((animation) => {
        animation.scrollTrigger?.kill()
        animation.kill()
      })
    }
  }, { scope })
}
