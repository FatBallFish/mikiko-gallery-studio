import assert from 'node:assert/strict'
import type { DeveloperEndpoints } from '../../../shared/api-types'
import { ApiError } from '../../../shared/http-client'
import { developerClientTemplates, developerAPIExamples, endpointDisabledReason } from './developerEndpoints'
import { oauthProblem } from './oauthProblem'

const disabled: DeveloperEndpoints = { canonical_configured: false, disabled_reason: 'CANONICAL_URL_UNCONFIGURED', api_origin: '', rest_base: '', openai_base: '', mcp_url: '', authorization_endpoint: '', token_endpoint: '', registration_endpoint: '', protected_resource_metadata_url: '', web_url: '' }
assert.equal(developerClientTemplates(disabled).length, 0)
assert.ok(endpointDisabledReason(disabled).includes('可信公共地址'))
assert.deepEqual(developerAPIExamples(disabled), [])
const enabled = { ...disabled, canonical_configured: true, disabled_reason: '', api_origin: 'https://studio.example', rest_base: 'https://studio.example/api/open/v1', openai_base: 'https://studio.example/v1', mcp_url: 'https://studio.example/mcp' }
const templates = developerClientTemplates(enabled)
assert.equal(templates.length, 3)
assert.equal(JSON.parse(templates[0].code).mcp.servers.studio.url, enabled.mcp_url)
assert.match(templates[1].code, /^\[mcp_servers.studio\]\nurl = "https:\/\/studio.example\/mcp"$/)
assert.match(templates[2].code, /^claude mcp add --transport http studio /)
for (const template of templates) assert.doesNotMatch(template.code, /Authorization|Bearer|token|secret|api_key/i)
const examples = developerAPIExamples(enabled)
assert.equal(examples.length, 4)
const imageRequest = JSON.parse(examples[3].code)
assert.equal(imageRequest.size_mode, 'pixel', 'pixel_sizes require explicit pixel mode; empty mode falls back to ratio')
assert.equal(imageRequest.task_type, 'text_to_image')
assert.ok(imageRequest.requested_size)
assert.match(examples[3].detail, /size_modes 支持 pixel/)
assert.match(examples[3].detail, /pixel_sizes/)
assert.ok(examples[0].code.includes(' \\\n  -H'), 'curl examples must preserve shell line continuations')
assert.match(examples[0].code, /https:\/\/studio.example\/api\/open\/v1\/image-models/)
assert.match(examples[1].code, /base_url="https:\/\/studio.example\/v1"/)
assert.match(examples[1].code, /images.generate[\s\S]*b64_json[\s\S]*write_bytes/)
const native = examples[2].code
assert.match(native, /quote = call\("https:\/\/studio.example\/api\/open\/v1\/image-tasks\/preflight", payload\)/)
assert.match(native, /quote_token=quote\["quote_token"\]/)
assert.match(native, /task = call\("https:\/\/studio.example\/api\/open\/v1\/image-tasks", creation\)/)
assert.match(native, /task\["id"\]/)
assert.match(native, /for _ in range\(120\)/)
assert.match(native, /\/results\/.*image\["id"\].*\/access/)
assert.match(native, /urlopen\(access\["url"\], timeout=120\)/)
assert.match(native, /image-create.json/)
for (const example of examples) assert.doesNotMatch(example.code, /window.location|studio.example.com|quote_token.*fake|Bearer ak-/)
assert.match(examples[0].code, /sk-…/)
assert.deepEqual(developerAPIExamples({ ...enabled, canonical_configured: false }), [])
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
