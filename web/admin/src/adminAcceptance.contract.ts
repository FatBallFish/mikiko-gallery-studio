// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'

const stylesSource = readFileSync(new URL('./styles.css', import.meta.url), 'utf8')
const tokenSource = readFileSync(new URL('../../shared/admin-design-tokens.css', import.meta.url), 'utf8')
const classesSource = readFileSync(new URL('./ui/classes.ts', import.meta.url), 'utf8')
const componentsSource = readFileSync(new URL('./components.tsx', import.meta.url), 'utf8')
const motionSource = readFileSync(new URL('./ui/adminMotion.ts', import.meta.url), 'utf8')

for (const responsiveContract of [
  "content: 'flex-1 min-w-0 overflow-x-hidden",
  "root: 'flex h-screen overflow-hidden",
  "item: 'grid min-h-20",
  'admin-primary-action',
]) {
  if (!classesSource.includes(responsiveContract)) {
    throw new Error(`admin responsive system must preserve ${responsiveContract}`)
  }
}

for (const themeToken of [
  ':root {',
  "[data-theme='dark']",
  '--fg:',
  '--muted:',
  '--border:',
  '--surface-solid:',
]) {
  if (!tokenSource.includes(themeToken)) throw new Error(`shared admin light/dark themes must define ${themeToken}`)
}

for (const cssContract of [
  '@media (max-width: 620px)',
  '.admin-primary-action',
  'min-height: 44px',
  'button:focus-visible',
  'a:focus-visible',
  'overflow-wrap: anywhere',
  '@media (prefers-reduced-motion: reduce)',
  'scroll-behavior: auto !important',
  'animation-duration: 0.01ms !important',
]) {
  if (!stylesSource.includes(cssContract)) throw new Error(`admin acceptance CSS must implement ${cssContract}`)
}

for (const focusContract of [
  'navTriggerRef.current?.focus()',
  'buttonRef.current?.focus()',
  'previousFocus?.focus()',
]) {
  if (!componentsSource.includes(focusContract)) throw new Error(`admin keyboard flows must restore focus with ${focusContract}`)
}

if (!motionSource.includes("matchMedia('(prefers-reduced-motion: reduce)')")) {
  throw new Error('GSAP admin motion must bypass nonessential animation for reduced-motion users')
}
