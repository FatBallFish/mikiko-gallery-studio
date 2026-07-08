// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { fileURLToPath } from 'node:url'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { dirname, join } from 'node:path'

const currentDir = dirname(fileURLToPath(import.meta.url))
const componentsSource = readFileSync(join(currentDir, 'components.tsx'), 'utf8')

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
