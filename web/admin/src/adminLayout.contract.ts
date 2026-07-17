// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { fileURLToPath } from 'node:url'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { dirname, join } from 'node:path'

const currentDir = dirname(fileURLToPath(import.meta.url))
const componentsSource = readFileSync(join(currentDir, 'components.tsx'), 'utf8')
const classesSource = readFileSync(join(currentDir, 'ui/classes.ts'), 'utf8')
const iconsSource = readFileSync(join(currentDir, 'ui/icons.tsx'), 'utf8')

if (componentsSource.includes('adminShell.statusStrip')) {
  throw new Error('AdminLayout must not render the global Current View/Admin Role status strip')
}

for (const forbiddenLabel of ['Current View', 'Admin Role', 'Queue Alerts', 'Pending Review']) {
  if (componentsSource.includes(forbiddenLabel)) {
    throw new Error(`AdminLayout should not render noisy global label: ${forbiddenLabel}`)
  }
}

if (!componentsSource.includes('aria-label="打开导航"') || !componentsSource.includes('adminShell.mobileTopbar')) {
  throw new Error('AdminLayout should expose a mobile topbar with an accessible drawer trigger')
}

for (const shellContract of [
  "sidebar: 'z-20 flex w-[var(--pg-sidebar-admin-width)]",
  "topbar: 'flex h-[var(--pg-topbar-height)]",
  "content: 'flex-1 min-w-0 overflow-x-hidden",
]) {
  if (!classesSource.includes(shellContract)) throw new Error(`AdminLayout must preserve stable shell geometry via ${shellContract}`)
}

if (componentsSource.includes('<span className="text-[10px] text-[var(--dim)]">{currentTitle}</span>')) {
  throw new Error('desktop topbar must not repeat the current page title in the account widget')
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

for (const iconName of ['MenuIcon', 'XIcon', 'LogOutIcon', 'UserMenuIcon']) {
  if (!iconsSource.includes(`export const ${iconName}`) || !componentsSource.includes(`<${iconName}`)) {
    throw new Error(`AdminLayout must use the shared Lucide ${iconName}`)
  }
}

if (componentsSource.includes('>☰<') || componentsSource.includes('>×<')) {
  throw new Error('AdminLayout must not use text glyphs for navigation controls')
}
