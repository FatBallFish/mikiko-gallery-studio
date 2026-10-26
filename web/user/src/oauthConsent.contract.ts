import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { oidcAuthorizeHref, sanitizeOIDCReturnTo, targetURLAfterLogin } from './oidcLogin'
import { parseUserHashState, userHashForRoute } from './routeState'

const initial = 'http://localhost:5273/?oauth_request=req%2Bkeep%26exact#/login?returnTo=oauth-consent'
const location = new URL(initial)
Object.defineProperty(globalThis, 'window', { configurable: true, value: { location } })
try {
  const login = parseUserHashState(location.hash)
  assert.equal(login.returnTo, 'oauth-consent')
  // Local login changes only the hash, so the request query survives.
  location.hash = userHashForRoute(login.returnTo!)
  assert.equal(location.searchParams.get('oauth_request'), 'req+keep&exact')
  assert.equal(parseUserHashState(location.hash).route, 'oauth-consent')

  location.href = initial
  const target = targetURLAfterLogin(login.returnTo)
  assert.equal(target, 'http://localhost:5273/?oauth_request=req%2Bkeep%26exact#/oauth-consent')
  const authorize = new URL(oidcAuthorizeHref(target), location.origin)
  assert.equal(authorize.searchParams.get('return_to'), target)
  location.href = 'http://localhost:5273/auth/callback?code=provider-code&state=provider-state'
  const restored = new URL(sanitizeOIDCReturnTo(target))
  assert.equal(restored.searchParams.get('oauth_request'), 'req+keep&exact')
  assert.equal(parseUserHashState(restored.hash).route, 'oauth-consent')
  assert.equal(sanitizeOIDCReturnTo('https://other.example/?oauth_request=bad#/oauth-consent'), 'http://localhost:5273/#/home')
  assert.equal(targetURLAfterLogin('home'), 'http://localhost:5273/auth/callback#/home')

  // Bind behavioral URL checks to the actual callback/consent wiring. The
  // callback must restore the full URL before hash navigation; approval must
  // use the request and server CSRF token, then follow the returned callback.
  const app = readFileSync(new URL('./App.tsx', import.meta.url), 'utf8')
  assert.match(app, /const target = sanitizeOIDCReturnTo\(result.return_to\)[\s\S]*?window.history.replaceState\(null, '', target\)[\s\S]*?const parsed = parseUserHashState/)
  const consent = readFileSync(new URL('./pages/OAuthConsentPage.tsx', import.meta.url), 'utf8')
  assert.match(consent, /new URLSearchParams\(window.location.search\).get\('oauth_request'\)/)
  assert.match(consent, /get<Consent>\('\/api\/v1\/oauth\/consent', \{ query: \{ request_id: id \} \}\)/)
  assert.match(consent, /post<\{ redirect_uri: string \}>\('\/api\/v1\/oauth\/consent', \{ request_id: id, csrf_token: consent.csrf_token, allow \}\)/)
  assert.match(consent, /window.location.assign\(result.redirect_uri\)/)
  const page = readFileSync(new URL('./pages/LoginPage.tsx', import.meta.url), 'utf8')
  assert.match(page, /oidcAuthorizeHref\(targetURLAfterLogin\(returnTo, imageId, taskId\)\)/)
  assert.match(page, /app.login\(\{ token: result.access_token, profile \}, returnTo,/)
  console.log('OK: OAuth consent local/OIDC login request preservation and callback wiring')
} finally {
  Reflect.deleteProperty(globalThis, 'window')
}
