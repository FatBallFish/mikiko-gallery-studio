import { useGSAP } from '@gsap/react'
import { gsap } from 'gsap'

if (typeof useGSAP !== 'function') {
  throw new Error('@gsap/react must expose useGSAP in the contract runtime')
}

if (typeof gsap.registerPlugin !== 'function') {
  throw new Error('the GSAP named export must expose registerPlugin in the contract runtime')
}

await import('./adminMotion')
