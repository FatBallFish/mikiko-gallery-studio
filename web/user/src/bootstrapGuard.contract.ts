// @ts-ignore contract scripts run in Node without project Node typings.
import { readFileSync } from 'node:fs'

const app = readFileSync(new URL('./App.tsx', import.meta.url), 'utf8')
const guard = readFileSync(new URL('../../shared/bootstrap-guard.ts', import.meta.url), 'utf8')
const wrapperEnd = app.indexOf('\nfunction UserApplication()')
if (wrapperEnd < 0) throw new Error('user app must isolate business effects in UserApplication')
const wrapper = app.slice(0, wrapperEnd)
for (const required of [
  'const bootstrap = useBootstrapGuard()',
  "bootstrap.phase === 'ready'",
  '<UserApplication />',
  "bootstrap.phase === 'broken'",
  'bootstrap.retry',
  'role="alert"',
]) {
  if (!wrapper.includes(required)) throw new Error(`user bootstrap wrapper missing ${required}`)
}
for (const forbidden of ['userApi.', 'refreshSession(', 'getProfile(', 'getBalance(', 'localStorage.removeItem']) {
  if (wrapper.includes(forbidden)) throw new Error(`user bootstrap wrapper must not run business/session behavior: ${forbidden}`)
}
for (const required of [
  'window.location.assign(setupURLWithReturnTarget(status.setup_url, window.location.href))',
  "status.phase === 'setup_required' || status.phase === 'initializing' || status.phase === 'restart_pending'",
  "credentials: 'omit'",
]) {
  if (!guard.includes(required)) throw new Error(`shared bootstrap guard missing ${required}`)
}
