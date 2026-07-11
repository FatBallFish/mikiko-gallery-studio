import type { RefObject } from 'react'
import { useGSAP } from '@gsap/react'
import { gsap } from 'gsap'

gsap.registerPlugin(useGSAP)

function prefersReducedMotion() {
  return typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

export function useAdminPageMotion(scope: RefObject<HTMLElement | null>, routeKey: string) {
  useGSAP(() => {
    const root = scope.current
    if (!root) return
    const items = gsap.utils.toArray<HTMLElement>('[data-admin-motion-item]', root)
    if (prefersReducedMotion()) {
      gsap.set(items, { clearProps: 'all' })
      return
    }
    gsap.fromTo(items, { opacity: 0, y: 6 }, {
      opacity: 1,
      y: 0,
      duration: 0.18,
      stagger: 0.025,
      ease: 'power2.out',
      clearProps: 'opacity,transform',
    })
  }, { scope, dependencies: [routeKey], revertOnUpdate: true })
}

export function useAdminLayerMotion(scope: RefObject<HTMLElement | null>) {
  useGSAP(() => {
    const root = scope.current
    if (!root) return
    const panel = root.querySelector<HTMLElement>('[data-admin-motion-panel]')
    if (prefersReducedMotion()) {
      gsap.set([root, panel], { clearProps: 'all' })
      return
    }
    gsap.fromTo(root, { opacity: 0 }, { opacity: 1, duration: 0.18, ease: 'power1.out', clearProps: 'opacity' })
    if (panel) {
      gsap.fromTo(panel, { x: 12, opacity: 0.94 }, {
        x: 0,
        opacity: 1,
        duration: 0.24,
        ease: 'power2.out',
        clearProps: 'opacity,transform',
      })
    }
  }, { scope, revertOnUpdate: true })
}

export function useAdminPreviewMotion(scope: RefObject<HTMLElement | null>, itemKey: string) {
  useGSAP(() => {
    const preview = scope.current
    if (!preview) return
    if (prefersReducedMotion()) {
      gsap.set(preview, { clearProps: 'all' })
      return
    }
    gsap.fromTo(preview, { opacity: 0.62, scale: 0.995 }, {
      opacity: 1,
      scale: 1,
      duration: 0.18,
      ease: 'power2.out',
      clearProps: 'opacity,transform',
    })
  }, { scope, dependencies: [itemKey], revertOnUpdate: true })
}
