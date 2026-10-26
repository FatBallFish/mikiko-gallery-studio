import assert from 'node:assert/strict'
import type { DeveloperEndpoints } from '../../../shared/api-types'
import { ApiError } from '../../../shared/http-client'
import { developerClientTemplates, developerEndpointRows, endpointDisabledReason } from './developerEndpoints'
import { oauthProblem } from './oauthProblem'

const disabled: DeveloperEndpoints = { canonical_configured: false, disabled_reason: 'CANONICAL_URL_UNCONFIGURED', api_origin: '', rest_base: '', openai_base: '', mcp_url: '', authorization_endpoint: '', token_endpoint: '', registration_endpoint: '', protected_resource_metadata_url: '', web_url: '' }
assert.equal(developerClientTemplates(disabled).length, 0)
assert.ok(endpointDisabledReason(disabled).includes('canonical'))
assert.ok(developerEndpointRows(disabled).every(([, url]) => url === ''))
const enabled = { ...disabled, canonical_configured: true, disabled_reason: '', api_origin: 'https://studio.example', rest_base: 'https://studio.example/api/open/v1', openai_base: 'https://studio.example/v1', mcp_url: 'https://studio.example/mcp' }
const templates = developerClientTemplates(enabled)
assert.equal(templates.length, 3)
assert.equal(JSON.parse(templates[0].code).mcp.servers.studio.url, enabled.mcp_url)
assert.match(templates[1].code, /^\[mcp_servers.studio\]\nurl = "https:\/\/studio.example\/mcp"$/)
assert.match(templates[2].code, /^claude mcp add --transport http studio /)
for (const template of templates) assert.doesNotMatch(template.code, /Authorization|Bearer|token|secret|api_key/i)
assert.deepEqual(developerEndpointRows(enabled).map(([, url]) => url), [enabled.api_origin, enabled.rest_base, enabled.openai_base, enabled.mcp_url])
for (const surface of ['apps', 'consent'] as const) {
  assert.equal(oauthProblem(new ApiError('', 503, 'CANONICAL_URL_UNCONFIGURED'), surface).kind, 'unconfigured')
  assert.equal(oauthProblem(new ApiError('', 503, 'temporarily_unavailable'), surface).kind, 'fault')
  assert.equal(oauthProblem(new TypeError('fetch failed'), surface).kind, 'network')
  assert.equal(oauthProblem(new ApiError('', 401, 'login_required'), surface).kind, 'login')
  assert.equal(oauthProblem(new ApiError('', 401, 'invalid_session'), surface).kind, 'login')
  assert.equal(oauthProblem(new ApiError('', 400, 'invalid_request'), surface).kind, 'fault')
}
assert.equal(oauthProblem(new ApiError('', 400, 'authorization_request_expired'), 'consent').kind, 'expired')
assert.equal(oauthProblem(new ApiError('', 400, 'authorization_request_expired'), 'consent').retryable, false)
console.log('OK: trusted endpoint formats, no-token templates, exact OAuth error and recovery classification')
