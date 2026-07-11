import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./LoginPage.tsx', import.meta.url), 'utf8')

for (const required of [
  'motion-reduce:transition-none',
  'motion-reduce:hover:translate-y-0',
  'motion-reduce:active:scale-100',
  "tabIndex={mode === 'password' ? 0 : -1}",
  "tabIndex={mode === 'code' ? 0 : -1}",
  'onKeyDown={handleTabKeyDown}',
  "role={intent === 'login' ? 'tabpanel' : undefined}",
  "aria-labelledby={intent === 'login' ? activeTabId : undefined}",
  'data-auth-field="email"',
  'focusFirstInvalidField',
  'role="alert"',
]) {
  if (!source.includes(required)) throw new Error(`authentication page source contract missing: ${required}`)
}

if (!source.includes("{intent === 'login' ? (") || !source.includes('role="tablist"')) {
  throw new Error('login mode tabs must only render for the login intent')
}

if (!/data-auth-field="email"[\s\S]{0,700}disabled=\{busy \|\| sending\}/.test(source)) {
  throw new Error('email input must be disabled while code delivery or form submission is busy')
}

console.log('OK: authentication page accessibility contract passed')
