// @ts-ignore contract scripts run in Node without project Node typings.
import { readFileSync } from 'node:fs'

const app = readFileSync(new URL('./App.tsx', import.meta.url), 'utf8')
const guard = readFileSync(new URL('../../shared/bootstrap-guard.ts', import.meta.url), 'utf8')
const wrapperEnd = app.indexOf('\nfunction AdminApplication()')
if (wrapperEnd < 0) throw new Error('admin app must isolate auth and shell effects in AdminApplication')
const wrapper = app.slice(0, wrapperEnd)
for (const required of [
  'const bootstrap = useBootstrapGuard()',
  "bootstrap.phase === 'ready'",
  '<AdminApplication />',
  "bootstrap.phase === 'broken'",
  'bootstrap.retry',
  'role="alert"',
]) {
  if (!wrapper.includes(required)) throw new Error(`admin bootstrap wrapper missing ${required}`)
}
for (const forbidden of ['adminApi.', 'refreshSession(', 'dashboard(', 'listConfig(', 'sessionStorage.removeItem']) {
  if (wrapper.includes(forbidden)) throw new Error(`admin bootstrap wrapper must not run auth/business behavior: ${forbidden}`)
}
for (const required of ['window.location.assign(setupURLWithReturnTarget(status.setup_url, window.location.href))', "credentials: 'omit'"]) {
  if (!guard.includes(required)) throw new Error(`shared bootstrap guard missing ${required}`)
}
