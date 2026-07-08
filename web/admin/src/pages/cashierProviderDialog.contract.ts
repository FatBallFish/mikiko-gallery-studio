// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { fileURLToPath } from 'node:url'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { dirname, join } from 'node:path'
import { Drawer } from '../components'

if (typeof Drawer !== 'function') {
  throw new Error('Drawer should be exported for complex admin configuration flows')
}

const source = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'CashierPage.tsx'), 'utf8')
const providerDialogStart = source.indexOf('{instanceDialog ? (')
const providerDialogEnd = source.indexOf('{orderDetail ? (')
const providerDialogSource = source.slice(providerDialogStart, providerDialogEnd)

if (!providerDialogSource.includes('<Drawer')) {
  throw new Error('payment provider instance flow should use Drawer instead of a long Modal')
}

if (providerDialogSource.includes('<Modal')) {
  throw new Error('payment provider instance flow must not render Modal')
}
