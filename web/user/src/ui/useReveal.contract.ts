import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { revealStyle } from './useReveal'

const source = readFileSync(new URL('./useReveal.ts', import.meta.url), 'utf8')

assert.match(source, /import\s*\{[^}]*useCallback[^}]*useLayoutEffect[^}]*\}\s*from\s*['"]react['"]/s)
assert.match(source, /const\s+\[node,\s*setNode\]\s*=\s*useState<T\s*\|\s*null>\(null\)/)
assert.match(source, /const\s+ref\s*=\s*useCallback\([\s\S]*?setNode\([\s\S]*?\),\s*\[\]\s*\)/)
assert.doesNotMatch(source, /\buseRef\b|ref\.current/)
assert.doesNotMatch(source, /\buseEffect\b/)
assert.match(source, /useLayoutEffect\([\s\S]*?\},\s*\[node,\s*once,\s*rootMargin,\s*threshold\]\s*\)/)

assert.deepEqual(revealStyle(false, 75), {
  opacity: 0,
  transform: 'translate3d(0, 16px, 0)',
})
assert.deepEqual(revealStyle(true, 75), {
  opacity: 1,
  transform: 'translate3d(0, 0, 0)',
  transition: 'opacity 480ms var(--pg-ease-out) 75ms, transform 480ms var(--pg-ease-out) 75ms',
})

console.log('OK: useReveal lifecycle contract passed')
