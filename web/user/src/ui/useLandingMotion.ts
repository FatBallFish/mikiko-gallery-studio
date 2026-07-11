import { useGSAP } from '@gsap/react'
import { gsap } from 'gsap'
import { ScrollTrigger } from 'gsap/ScrollTrigger'
import type { RefObject } from 'react'
import { prefersReducedMotion } from './motion'

gsap.registerPlugin(ScrollTrigger)

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

    const animations: gsap.core.Animation[] = []

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
      const overlay = image.parentElement?.querySelector<HTMLElement>(landingMotionSelectors.overlay)
      const timeline = gsap.timeline({
        scrollTrigger: {
          trigger: image,
          start: 'top 94%',
          end: 'bottom 6%',
          scrub: 0.75,
        },
      })
      timeline
        .fromTo(image, {
          scale: 0.82,
          opacity: 0.38,
        }, {
          scale: 1,
          opacity: 1,
          ease: 'none',
          duration: 0.58,
        })
        .to(image, {
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
