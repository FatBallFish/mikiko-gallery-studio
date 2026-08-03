import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./LoginPage.tsx', import.meta.url), 'utf8')

for (const required of [
  'motion-reduce:transition-none',
  'motion-reduce:hover:translate-y-0',
  'motion-reduce:active:scale-100',
  "tabIndex={mode === 'password' ? 0 : -1}",
  "tabIndex={mode === 'code' ? 0 : -1}",
  'onKeyDown={handleTabKeyDown}',
  "role={intent === 'login' && !passwordSetupToken ? 'tabpanel' : undefined}",
  "aria-labelledby={intent === 'login' && !passwordSetupToken ? activeTabId : undefined}",
  'data-auth-field="email"',
  'focusFirstInvalidField',
  'role="alert"',
  "const [passwordSetupToken, setPasswordSetupToken] = useState('')",
  'result.password_setup_required',
  'setPasswordSetupToken(result.password_setup_token)',
  'await userApi.completePasswordSetup(passwordSetupToken, newPassword)',
  'data-auth-field="confirmPassword"',
  '<picture',
  '/landing/studio-showcase-1280.webp',
  '/landing/studio-showcase-1920.webp',
  '/landing/studio-showcase-1280.avif',
  '/landing/studio-showcase-1920.avif',
  'type="image/avif"',
  'type="image/webp"',
  'srcSet=',
  '<BrandMark withText inverse />',
  'md:h-dvh',
  'md:min-h-0',
  'md:overflow-hidden',
  'md:h-full',
  'md:overflow-y-auto',
  'md:overscroll-contain',
  'md:py-10',
  'md:[@media(max-height:860px)]:py-4',
  'md:[@media(max-height:860px)]:p-6',
]) {
  if (!source.includes(required)) throw new Error(`authentication page source contract missing: ${required}`)
}

if (source.includes('/landing/hero-gallery.webp')) {
  throw new Error('login must not serve the legacy test-user hero screenshot')
}

if (/localStorage\.(?:setItem|getItem)\([^\n]*passwordSetupToken/.test(source)) {
  throw new Error('password setup token must remain in React memory and never enter local storage')
}

const setupBranch = source.indexOf('if (result.password_setup_required)')
const setupReturn = source.indexOf('return', setupBranch)
const profileFetch = source.indexOf('userApi.getProfileWithToken', setupBranch)
if (setupBranch < 0 || setupReturn < 0 || profileFetch < 0 || !(setupBranch < setupReturn && setupReturn < profileFetch)) {
  throw new Error('setup-required login must return before fetching profile or installing a session')
}

if (!source.includes("{intent === 'login' && !passwordSetupToken ? (") || !source.includes('role="tablist"')) {
  throw new Error('login mode tabs must only render for the login intent')
}

if (source.includes("role={intent === 'login' ? 'tabpanel' : undefined}")) {
  throw new Error('password setup form must not reference a tab that is no longer rendered')
}

if (!/data-auth-field="email"[\s\S]{0,700}disabled=\{busy \|\| sending\}/.test(source)) {
  throw new Error('email input must be disabled while code delivery or form submission is busy')
}

console.log('OK: authentication page accessibility contract passed')
