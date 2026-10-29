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
  // Platform branding and external client identity have separate, explicit roles.
  assert.match(consent, /siteBrand\.name/)
  assert.match(consent, /申请连接的外部应用/)
  assert.match(consent, /<h2>\{consent\.client\.client_name\}<\/h2>/)
  assert.doesNotMatch(consent, /client_name\.(?:replace|replaceAll)/)
  assert.match(consent, /app\.profile\?\.email/)
  assert.match(consent, /<details className="oauth-consent__details">/)
  assert.match(consent, /apiKeyScopeLabel\(scope\)/)
  const scopes = ['account:read', 'models:read', 'images:read', 'images:write', 'videos:read', 'videos:write', 'videos:delete', ...['assets', 'uploads', 'projects', 'groups'].flatMap(resource => ['read', 'write', 'delete'].map(action => `${resource}:${action}`))]
  for (const scope of scopes) assert.ok(consent.includes(`'${scope}':`), `missing permission explanation: ${scope}`)
  // Unknown scopes are rendered with their actual key, never filtered out.
  assert.match(consent, /description \? apiKeyScopeLabel\(scope\) : '额外权限'/)
  assert.match(consent, /<code>\{scope\}<\/code>/)
  assert.match(consent, /consent\.scopes\.map\(scope => <ScopePermission/)
  assert.match(consent, /onClick=\{\(\) => void decide\(false\)\}>取消/)
  assert.match(consent, /onClick=\{\(\) => void decide\(true\)\}/)
  assert.match(consent, /disabled=\{busy\}/)
  assert.match(consent, /error\.retryable/)
  assert.match(consent, /error\.kind === 'login'/)
  assert.match(consent, /error\.kind === 'expired'/)
  const css = readFileSync(new URL('./pages/oauth-consent.css', import.meta.url), 'utf8')
  assert.match(css, /\.oauth-consent__body\s*\{[^}]*flex: 1 1 auto;[^}]*min-height: 120px;[^}]*overflow-y: auto;/)
  assert.match(consent, /className="oauth-consent__body" role="region" aria-label="应用身份与授权详情" tabIndex=\{0\}/)
  assert.match(consent, /<details className="oauth-consent__application-name"><summary>查看完整应用名称<\/summary><p>\{consent\.client\.client_name\}<\/p><\/details>/)
  assert.match(css, /\.oauth-consent__application h2\s*\{[^}]*-webkit-line-clamp: 2;/)
  assert.match(css, /\.oauth-consent__permission-scroll\s*\{[^}]*max-height:[^}]*overflow-y: auto;/)
  assert.match(css, /\.oauth-consent__footer\s*\{[^}]*flex: 0 0 auto;/)
  assert.match(css, /overflow-wrap: anywhere/)
  assert.match(css, /:focus-visible/)
  assert.match(app, /<UploadTray hidden=\{route === 'oauth-consent' \|\| \(route === 'creative-canvas' && Boolean\(routeCanvasId\)\)\} \/>/)
  // Keep the queue mounted: hiding this route must not abort ongoing uploads.
  assert.doesNotMatch(app, /route !== 'oauth-consent'[^\n]*<UploadTray/)
  const page = readFileSync(new URL('./pages/LoginPage.tsx', import.meta.url), 'utf8')
  assert.match(page, /oidcAuthorizeHref\(targetURLAfterLogin\(returnTo, imageId, taskId\)\)/)
  assert.match(page, /app.login\(\{ token: result.access_token, profile \}, returnTo,/)
  console.log('OK: OAuth consent local/OIDC login request preservation and callback wiring')
} finally {
  Reflect.deleteProperty(globalThis, 'window')
}
