// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { fileURLToPath } from 'node:url'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { dirname, join } from 'node:path'

const currentDir = dirname(fileURLToPath(import.meta.url))
const componentsSource = readFileSync(join(currentDir, 'components.tsx'), 'utf8')
const iconsSource = readFileSync(join(currentDir, 'ui/icons.tsx'), 'utf8')
const adminCss = readFileSync(join(currentDir, 'redesign.css'), 'utf8')
const bridgeCss = readFileSync(join(currentDir, 'redesign-bridge.css'), 'utf8')

if (componentsSource.includes('adminShell.statusStrip')) {
  throw new Error('AdminLayout must not render the global Current View/Admin Role status strip')
}

for (const forbiddenLabel of ['Current View', 'Admin Role', 'Queue Alerts', 'Pending Review']) {
  if (componentsSource.includes(forbiddenLabel)) {
    throw new Error(`AdminLayout should not render noisy global label: ${forbiddenLabel}`)
  }
}

if (!componentsSource.includes('aria-label="打开导航"') || !componentsSource.includes('a-side-toggle')) {
  throw new Error('AdminLayout should expose a mobile topbar with an accessible drawer trigger')
}

for (const shellContract of [
  '.a-side',
  'width: var(--a-side-w)',
  '.a-top',
  'height: var(--a-top-h)',
  '.a-content',
]) {
  if (!adminCss.includes(shellContract)) throw new Error(`AdminLayout must use the Command Deck shell geometry via ${shellContract}`)
}

if (!bridgeCss.includes('--pg-sidebar-admin-width: var(--a-side-w)') || !bridgeCss.includes('--pg-topbar-height: var(--a-top-h)')) {
  throw new Error('Command Deck shell aliases must map legacy geometry onto --a-side-w / --a-top-h')
}

for (const focusContract of [
  'navTriggerRef',
  'mobileDrawerRef',
  "event.key === 'Escape'",
  "event.key === 'Tab'",
  'navTriggerRef.current?.focus()',
  'aria-haspopup="menu"',
  'role="menu"',
]) {
  if (!componentsSource.includes(focusContract)) {
    throw new Error(`AdminLayout must provide accessible navigation and account interactions via ${focusContract}`)
  }
}

for (const iconName of ['MenuIcon', 'XIcon', 'LogOutIcon']) {
  if (!iconsSource.includes(`export const ${iconName}`) || !componentsSource.includes(`<${iconName}`)) {
    throw new Error(`AdminLayout must use the shared Lucide ${iconName}`)
  }
}

if (componentsSource.includes('>☰<') || componentsSource.includes('>×<')) {
  throw new Error('AdminLayout must not use text glyphs for navigation controls')
}

console.log('OK: admin shell layout contract passed')