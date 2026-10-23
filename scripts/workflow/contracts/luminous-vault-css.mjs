import { readFileSync } from 'node:fs'

// The production skin now consumes the approved prototype design system:
// tokens from prototypes/redesign/assets/tokens.css, component classes from
// user.css/admin.css, and a small alias bridge for legacy feature code.

const root = new URL('../../../', import.meta.url)
const tokensCss = readFileSync(new URL('web/shared/redesign-theme.css', root), 'utf8')
const userCss = readFileSync(new URL('web/user/src/redesign.css', root), 'utf8')
const userBridge = readFileSync(new URL('web/user/src/redesign-bridge.css', root), 'utf8')
const adminCss = readFileSync(new URL('web/admin/src/redesign.css', root), 'utf8')
const adminBridge = readFileSync(new URL('web/admin/src/redesign-bridge.css', root), 'utf8')
const userStyles = readFileSync(new URL('web/user/src/styles.css', root), 'utf8')

function block(source, marker) {
  const markerIndex = source.indexOf(marker)
  if (markerIndex === -1) throw new Error(`missing CSS block: ${marker}`)
  const opening = source.indexOf('{', markerIndex + marker.length)
  if (opening === -1) throw new Error(`missing opening brace: ${marker}`)

  let depth = 0
  for (let index = opening; index < source.length; index += 1) {
    if (source[index] === '{') depth += 1
    if (source[index] === '}') depth -= 1
    if (depth === 0) return source.slice(opening + 1, index)
  }
  throw new Error(`missing closing brace: ${marker}`)
}

function declarations(source) {
  const result = new Map()
  for (const match of source.matchAll(/(?:^|[;\n])\s*(--[\w-]+|color-scheme)\s*:\s*([^;]+);/g)) {
    result.set(match[1], match[2].trim())
  }
  return result
}

function expectValue(values, name, expected, scope) {
  const actual = values.get(name)
  if (actual !== expected) throw new Error(`${scope} ${name}: expected ${expected}, received ${actual ?? 'missing'}`)
}

function expectContains(source, needle, scope) {
  if (!source.includes(needle)) throw new Error(`${scope} is missing ${needle}`)
}

const sharedRoot = declarations(block(tokensCss, ':root'))
for (const [name, expected] of Object.entries({
  '--r-xs': '6px',
  '--r-sm': '10px',
  '--r-md': '14px',
  '--r-lg': '20px',
  '--r-xl': '28px',
  '--t-fast': '140ms',
  '--t-base': '220ms',
  '--t-slow': '420ms',
})) expectValue(sharedRoot, name, expected, 'prototype tokens')

const userDark = declarations(block(tokensCss, '[data-site="user"]'))
for (const [name, expected] of Object.entries({
  '--bg': '#08090d',
  '--ink': '#f5f0e7',
  '--accent': '#d9a86a',
  '--rail-w': '92px',
  '--top-h': '72px',
  '--content-max': '1640px',
})) expectValue(userDark, name, expected, 'user dark theme')

const userLight = declarations(block(tokensCss, '[data-site="user"][data-theme="light"]'))
for (const [name, expected] of Object.entries({
  '--bg': '#f5f3ee',
  '--ink': '#221d16',
  '--accent': '#a86f2c',
})) expectValue(userLight, name, expected, 'user light theme')

const adminRoot = declarations(block(tokensCss, '[data-site="admin"]'))
for (const [name, expected] of Object.entries({
  '--a-bg': '#f2f1ec',
  '--a-surface': '#ffffff',
  '--a-ink': '#16191e',
  '--a-accent': '#2e4a7d',
  '--a-side-w': '248px',
  '--a-top-h': '68px',
})) expectValue(adminRoot, name, expected, 'admin Command Deck tokens')

const adminDark = declarations(block(tokensCss, '[data-site="admin"][data-theme="dark"]'))
for (const name of ['--a-bg', '--a-surface', '--a-ink', '--a-accent', '--a-line']) {
  if (!adminDark.has(name)) throw new Error(`admin dark theme is missing ${name}`)
}

for (const needle of [
  '.u-rail',
  'width: var(--rail-w)',
  '.u-topbar',
  'height: var(--top-h)',
  '.u-detail',
  'grid-template-columns: minmax(0, 1fr) 392px',
  'height: clamp(320px, 58vh, 660px)',
  'object-fit: contain',
  '.u-drawer',
  '.u-segment',
  '.u-card',
  '.u-empty',
]) expectContains(userCss, needle, 'user component CSS')

for (const needle of [
  '.a-side',
  'width: var(--a-side-w)',
  '.a-top',
  'height: var(--a-top-h)',
  '.a-filters',
  '.a-table',
  '.a-pager',
  '.a-drawer',
  '.a-form__section',
  '.a-multiselect',
]) expectContains(adminCss, needle, 'admin component CSS')

for (const [name, expected] of Object.entries({
  '--fg': 'var(--ink)',
  '--muted': 'var(--ink-2)',
  '--dim': 'var(--ink-3)',
  '--border': 'var(--line)',
  '--surface-solid': 'var(--bg-elev)',
})) expectValue(declarations(block(userBridge, "html[data-site='user']")), name, expected, 'user alias bridge')

for (const [name, expected] of Object.entries({
  '--bg': 'var(--a-bg)',
  '--surface': 'var(--a-surface)',
  '--fg': 'var(--a-ink)',
  '--border': 'var(--a-line)',
  '--accent': 'var(--a-accent)',
})) expectValue(declarations(block(adminBridge, "html[data-site='admin']")), name, expected, 'admin alias bridge')

// Fonts are part of the visual contract: Fraunces display, Manrope UI,
// JetBrains Mono, with Chinese fallbacks.
for (const needle of ["'Fraunces'", "'Manrope'", "'JetBrains Mono'", "'Noto Sans SC'"]) {
  expectContains(tokensCss + userCss + userBridge + userStyles, needle, 'user font stack')
}

// Responsive rules must exist at the approved user breakpoints.
for (const width of ['1180px', '980px', '760px', '480px']) {
  expectContains(userCss, `max-width: ${width}`, 'user responsive CSS')
}

// The unified media drawer keeps the stage stable and the info column fixed.
if (!/\.u-detail__stage\s*>\s*img,[\s\S]*?position:\s*absolute;[\s\S]*?object-fit:\s*contain;/.test(userCss)) {
  throw new Error('media detail stage must absolute-fit media with object-fit: contain')
}

// Admin tables must scroll inside their own container, never the document.
if (!/\.a-table-wrap\s*\{[^}]*overflow-x:\s*auto;/.test(adminCss)) {
  throw new Error('admin tables must own their horizontal scroll container')
}

console.log('OK: prototype design-system CSS contract passed')