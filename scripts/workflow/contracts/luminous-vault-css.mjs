import { readFileSync } from 'node:fs'

const root = new URL('../../../', import.meta.url)
const tokensCss = readFileSync(new URL('web/shared/tokens.css', root), 'utf8')
const themeCss = readFileSync(new URL('web/shared/user-theme.css', root), 'utf8')
const userCss = readFileSync(new URL('web/user/src/styles.css', root), 'utf8')

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

function expectMatchingRgb(values, colorName, rgbName, scope) {
  const color = values.get(colorName)
  const rgb = values.get(rgbName)
  const hex = color?.match(/^#([0-9a-f]{6})$/i)?.[1]
  if (!hex || !rgb) throw new Error(`${scope} cannot compare ${colorName} with ${rgbName}`)
  const expected = [0, 2, 4].map((offset) => Number.parseInt(hex.slice(offset, offset + 2), 16)).join(', ')
  if (rgb !== expected) throw new Error(`${scope} ${rgbName}: expected ${expected} from ${color}, received ${rgb}`)
}

const tokenRoot = declarations(block(tokensCss, ':root'))
for (const [name, expected] of Object.entries({
  '--pg-radius-sm': '8px',
  '--pg-radius-md': '12px',
  '--pg-radius-lg': '16px',
  '--pg-radius-xl': '24px',
  '--pg-duration-base': '220ms',
  '--pg-sidebar-user-width': '108px',
  '--pg-topbar-height': '76px',
})) expectValue(tokenRoot, name, expected, 'tokens')

const themeDependentTokens = [
  '--lv-bg', '--lv-canvas', '--lv-surface-1', '--lv-surface-2', '--lv-surface-3',
  '--lv-text-primary', '--lv-text-secondary', '--lv-text-tertiary',
  '--lv-border-subtle', '--lv-border', '--lv-border-strong',
  '--lv-brand-amber', '--lv-brand-amber-rgb',
  '--lv-status-violet', '--lv-status-violet-rgb',
  '--lv-status-emerald', '--lv-status-emerald-rgb',
  '--lv-status-coral', '--lv-status-coral-rgb',
  '--lv-status-blue', '--lv-status-blue-rgb',
  '--lv-image-overlay', '--lv-image-overlay-selected', '--lv-image-action',
  '--lv-shadow-sm', '--lv-shadow-md', '--lv-shadow-lg', '--lv-focus-ring',
  '--lv-motion-fast', '--lv-motion-route', '--lv-motion-slow',
]

const darkTheme = declarations(block(themeCss, ':root'))
const explicitLight = declarations(block(themeCss, "html[data-theme-mode='light']"))
const lightPreference = block(themeCss, '@media (prefers-color-scheme: light)')
const fallbackLight = declarations(block(lightPreference, 'html:not([data-theme-mode])'))

expectValue(darkTheme, 'color-scheme', 'dark', 'dark theme')
expectValue(explicitLight, 'color-scheme', 'light', 'explicit light theme')
expectValue(fallbackLight, 'color-scheme', 'light', 'light preference fallback')
for (const name of themeDependentTokens) {
  if (!darkTheme.has(name)) throw new Error(`dark theme is missing ${name}`)
  if (!explicitLight.has(name)) throw new Error(`explicit light theme is missing ${name}`)
  expectValue(fallbackLight, name, explicitLight.get(name), 'light preference fallback')
}
for (const [colorName, rgbName] of [
  ['--lv-brand-amber', '--lv-brand-amber-rgb'],
  ['--lv-status-violet', '--lv-status-violet-rgb'],
  ['--lv-status-emerald', '--lv-status-emerald-rgb'],
  ['--lv-status-coral', '--lv-status-coral-rgb'],
  ['--lv-status-blue', '--lv-status-blue-rgb'],
]) {
  expectMatchingRgb(darkTheme, colorName, rgbName, 'dark theme')
  expectMatchingRgb(explicitLight, colorName, rgbName, 'explicit light theme')
}

const userRoot = declarations(block(userCss, ':root'))
for (const [name, expected] of Object.entries({
  '--sidebar-w': 'var(--pg-sidebar-user-width)',
  '--topbar-h': 'var(--pg-topbar-height)',
  '--radius-sm': 'var(--pg-radius-sm)',
  '--radius': 'var(--pg-radius-md)',
  '--radius-lg': 'var(--pg-radius-lg)',
  '--radius-xl': 'var(--pg-radius-xl)',
  '--motion-route': 'var(--lv-motion-route)',
})) expectValue(userRoot, name, expected, 'user mappings')

for (const fontName of ['--font-vault-display', '--font-vault-body']) {
  const fontMatch = userCss.match(new RegExp(`${fontName.replaceAll('-', '\\-')}\\s*:\\s*([^;]+);`))
  const value = fontMatch?.[1] ?? ''
  if (!value.includes("'Satoshi'") || !value.includes("'Noto Sans SC'") || !value.includes('sans-serif')) {
    throw new Error(`${fontName} does not include the approved Satoshi and Chinese sans stack`)
  }
}

if (/--accent-rgb\s*:\s*\d/.test(userCss)) {
  throw new Error('--accent-rgb must resolve from theme-aware semantic RGB tokens')
}
for (const [theme, semantic] of Object.entries({
  amber: '--lv-brand-amber-rgb',
  violet: '--lv-status-violet-rgb',
  emerald: '--lv-status-emerald-rgb',
  coral: '--lv-status-coral-rgb',
})) {
  const selector = theme === 'amber' ? 'html[data-accent-theme="amber"]' : `html[data-accent-theme="${theme}"]`
  const values = declarations(block(userCss, selector))
  expectValue(values, '--accent-rgb', `var(${semantic})`, `${theme} accent`)
}

console.log('OK: Luminous Vault CSS contract passed')
