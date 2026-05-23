#!/usr/bin/env node

import crypto from 'node:crypto'
import fs from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const ROOT_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const BASE_URL = envUrl('BASE_URL', 'http://127.0.0.1:18080')
const USER_WEB_URL = envUrl('USER_WEB_URL', 'http://127.0.0.1:5173')
const ADMIN_WEB_URL = envUrl('ADMIN_WEB_URL', 'http://127.0.0.1:5174')
const NGINX_URL = envUrl('NGINX_URL', 'http://127.0.0.1:18081')
const REPORT_DIR = path.join(ROOT_DIR, 'tmp/e2e')
const RUN_ID = new Date().toISOString().replace(/[-:.TZ]/g, '').slice(0, 14)
const TINY_PNG_BASE64 = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+y1X8AAAAASUVORK5CYII='

const state = {
  steps: [],
  warnings: [],
  user: {},
  admin: {},
  apiKey: {},
  ids: {
    userId: '1',
    keyId: '1',
    orderId: '1',
    taskId: 'missing-task',
    assetId: 'missing-asset',
    imageId: 'missing-image',
    codeId: '1',
    routeId: '1',
    providerModelId: '1',
    providerCode: `e2e-provider-${RUN_ID}`.toLowerCase(),
    groupCode: `e2e-group-${RUN_ID}`.toLowerCase(),
  },
}

function envUrl(name, fallback) {
  return (process.env[name] || fallback).replace(/\/+$/, '')
}

function record(name, status, detail = {}) {
  state.steps.push({ name, status, detail })
  const marker = status === 'pass' ? 'PASS' : status === 'warn' ? 'WARN' : 'FAIL'
  console.log(`[${marker}] ${name}`)
}

function warn(name, detail = {}) {
  state.warnings.push({ name, detail })
  record(name, 'warn', detail)
}

function fail(message, detail = {}) {
  const error = new Error(message)
  error.detail = detail
  throw error
}

async function step(name, fn) {
  try {
    const detail = await fn()
    record(name, 'pass', detail || {})
  } catch (error) {
    record(name, 'fail', {
      message: error.message,
      detail: error.detail || undefined,
      stack: error.stack,
    })
    throw error
  }
}

async function sleep(ms) {
  await new Promise(resolve => setTimeout(resolve, ms))
}

async function waitFor(url, options = {}) {
  const timeoutMs = options.timeoutMs ?? 180000
  const started = Date.now()
  let lastError = ''
  while (Date.now() - started < timeoutMs) {
    try {
      const response = await fetch(url, { method: options.method || 'GET' })
      if (response.status >= 200 && response.status < 500) {
        return response
      }
      lastError = `${response.status} ${await response.text()}`
    } catch (error) {
      lastError = error.message
    }
    await sleep(1000)
  }
  fail(`Timed out waiting for ${url}`, { lastError })
}

async function request(method, url, options = {}) {
  const headers = new Headers(options.headers || {})
  let body = options.body
  if (body !== undefined && typeof body !== 'string' && !(body instanceof FormData)) {
    body = JSON.stringify(body)
    if (!headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  }
  const response = await fetch(url, { method, headers, body, redirect: options.redirect || 'follow' })
  const text = await response.text()
  let json = null
  if (text) {
    try {
      json = JSON.parse(text)
    } catch {
      json = null
    }
  }
  return { response, text, json, status: response.status, headers: response.headers }
}

async function expectStatus(method, url, expected, options = {}) {
  const result = await request(method, url, options)
  const ok = Array.isArray(expected) ? expected.includes(result.status) : result.status === expected
  if (!ok) {
    fail(`${method} ${url} returned ${result.status}, expected ${expected}`, {
      body: result.text.slice(0, 1000),
    })
  }
  return result
}

function data(result) {
  if (!result.json || typeof result.json !== 'object' || !('data' in result.json)) {
    fail('Expected JSON envelope with data', { body: result.text.slice(0, 1000) })
  }
  return result.json.data
}

function bearer(token) {
  return { Authorization: `Bearer ${token}` }
}

function base64Url(buffer) {
  return Buffer.from(buffer).toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
}

function signNative(method, pathWithQuery, body = '') {
  const timestamp = new Date().toISOString().replace(/\.\d{3}Z$/, 'Z')
  const bodyHash = base64Url(crypto.createHash('sha256').update(body).digest())
  const payload = [method.toUpperCase(), pathWithQuery, timestamp, bodyHash].join('\n')
  const signature = base64Url(crypto.createHmac('sha256', state.apiKey.secret).update(payload).digest())
  return {
    'X-Access-Key': state.apiKey.accessKey,
    'X-Timestamp': timestamp,
    'X-Body-SHA256': bodyHash,
    'X-Signature': signature,
  }
}

async function signedRequest(method, pathWithQuery, bodyObj) {
  const body = bodyObj === undefined ? '' : JSON.stringify(bodyObj)
  const headers = signNative(method, pathWithQuery, body)
  if (body) headers['Content-Type'] = 'application/json'
  return request(method, `${BASE_URL}${pathWithQuery}`, { headers, body: body || undefined })
}

function uniqueEmail(prefix) {
  return `${prefix}-${RUN_ID}@example.com`
}

async function loadOpenAPI() {
  const result = await expectStatus('GET', `${BASE_URL}/docs/openapi.json`, 200)
  return result.json
}

async function checkHtmlApp(name, baseUrl, routes, expectedText) {
  const root = await expectStatus('GET', `${baseUrl}/`, 200)
  if (!root.text.includes('<script') || !root.text.includes('id="root"')) {
    fail(`${name} root did not look like a Vite React app`, { body: root.text.slice(0, 500) })
  }
  if (expectedText && !root.text.includes(expectedText)) {
    warn(`${name} root static HTML did not include expected marker`, { expectedText })
  }
  for (const route of routes) {
    const page = await expectStatus('GET', `${baseUrl}/#/${route}`, 200)
    if (!page.text.includes('id="root"')) {
      fail(`${name} route did not return app shell`, { route })
    }
  }
}

async function frontendApiClientSmoke() {
  const esbuildPath = path.join(ROOT_DIR, 'web/user/node_modules/esbuild/lib/main.js')
  const esbuild = await import(pathToFileURL(esbuildPath).href)
  await fs.mkdir(REPORT_DIR, { recursive: true })
  const entryPath = path.join(REPORT_DIR, `frontend-client-smoke-${RUN_ID}.mjs`)
  const bundlePath = path.join(REPORT_DIR, `frontend-client-smoke-bundle-${RUN_ID}.mjs`)
  await fs.writeFile(entryPath, `
    import { userApi } from ${JSON.stringify(path.join(ROOT_DIR, 'web/shared/user-api.ts'))}
    import { adminApi } from ${JSON.stringify(path.join(ROOT_DIR, 'web/shared/admin-api.ts'))}

    export async function runSmoke() {
      const userEmail = 'frontend-smoke-${RUN_ID}@example.com'
      await userApi.sendEmailCode(userEmail, 'login')
      const login = await userApi.loginWithEmailCode(userEmail, '123456')
      if (!login.access_token) throw new Error('user login was not unwrapped to access_token')
      userApi.configureAuth({ getToken: () => login.access_token })
      const profile = await userApi.getProfile()
      if (profile.email !== userEmail) throw new Error('user authenticated profile request failed')

      const adminSession = await adminApi.login('admin@example.com', 'admin123456')
      if (!adminSession.token) throw new Error('admin login was not mapped to token')
      adminApi.configureAuth({ getToken: () => adminSession.token })
      const dashboard = await adminApi.dashboard()
      if (!Array.isArray(dashboard.metrics)) throw new Error('admin authenticated dashboard request failed')
      return { userEmail, adminEmail: adminSession.email, metricCount: dashboard.metrics.length }
    }
  `)
  await esbuild.build({
    entryPoints: [entryPath],
    outfile: bundlePath,
    bundle: true,
    format: 'esm',
    platform: 'node',
    define: {
      'import.meta.env': JSON.stringify({ VITE_API_BASE_URL: BASE_URL }),
    },
  })
  const mod = await import(`${pathToFileURL(bundlePath).href}?run=${RUN_ID}`)
  return mod.runSmoke()
}

async function bootstrapUser() {
  const email = uniqueEmail('e2e-user')
  await expectStatus('POST', `${BASE_URL}/api/agent/auth/v1/email/send-code`, 202, {
    body: { email, scene: 'login' },
  })
  const login = await expectStatus('POST', `${BASE_URL}/api/agent/auth/v1/login/email-code`, 200, {
    body: { email, code: '123456' },
  })
  const session = data(login)
  state.user.email = email
  state.user.token = session.access_token

  const profile = await expectStatus('GET', `${BASE_URL}/api/agent/user/v1/profile`, 200, {
    headers: bearer(state.user.token),
  })
  const profileData = data(profile)
  state.ids.userId = String(profileData.id)
  return { email, userId: state.ids.userId }
}

async function bootstrapAdmin() {
  const login = await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/auth/login`, 200, {
    body: { email: 'admin@example.com', password: 'admin123456' },
  })
  state.admin.token = data(login).access_token
  return { email: 'admin@example.com' }
}

async function seedPoints() {
  const result = await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/users/${state.ids.userId}/points-adjustments`, 200, {
    headers: {
      ...bearer(state.admin.token),
      'Idempotency-Key': `e2e-seed-points-${RUN_ID}`,
    },
    body: { change_points: '100.00000', reason: `docker e2e seed ${RUN_ID}` },
  })
  return { availablePoints: data(result).available_points || data(result).balance_after }
}

async function happyPathAgentBilling() {
  await expectStatus('GET', `${BASE_URL}/api/agent/billing/v1/plans`, 200, { headers: bearer(state.user.token) })
  await expectStatus('GET', `${BASE_URL}/api/agent/billing/v1/subscription`, 200, { headers: bearer(state.user.token) })
  await expectStatus('GET', `${BASE_URL}/api/agent/billing/v1/balance`, 200, { headers: bearer(state.user.token) })
  await expectStatus('GET', `${BASE_URL}/api/agent/billing/v1/ledger`, 200, { headers: bearer(state.user.token) })
  const estimate = await expectStatus('GET', `${BASE_URL}/api/agent/billing/v1/estimate?task_type=text_to_image&abstract_model=basic&requested_quality=auto&requested_size=1024x1024&requested_output_image_count=1&reference_image_count=0`, 200, {
    headers: bearer(state.user.token),
  })
  if (!data(estimate).estimated_points) fail('Estimate response did not include estimated_points')

  const order = await expectStatus('POST', `${BASE_URL}/api/agent/billing/v1/orders`, 201, {
    headers: bearer(state.user.token),
    body: { plan_code: 'basic-monthly', provider: 'alipay' },
  })
  state.ids.orderId = String(data(order).id)
  await expectStatus('GET', `${BASE_URL}/api/agent/billing/v1/orders`, 200, { headers: bearer(state.user.token) })
  await expectStatus('GET', `${BASE_URL}/api/agent/billing/v1/orders/${state.ids.orderId}`, 200, { headers: bearer(state.user.token) })
  await expectStatus('POST', `${BASE_URL}/api/agent/billing/v1/orders/${state.ids.orderId}/cancel`, [200, 409], { headers: bearer(state.user.token) })
  return { orderId: state.ids.orderId }
}

async function happyPathApiKeys() {
  const created = await expectStatus('POST', `${BASE_URL}/api/agent/account/v1/api-keys`, 201, {
    headers: bearer(state.user.token),
    body: { name: `e2e-key-${RUN_ID}`, total_quota_points: '50.00000', daily_quota_points: '50.00000', rpm_limit: 60 },
  })
  const key = data(created)
  state.ids.keyId = String(key.id)
  state.apiKey.accessKey = key.access_key
  state.apiKey.secret = key.secret
  await expectStatus('GET', `${BASE_URL}/api/agent/account/v1/api-keys`, 200, { headers: bearer(state.user.token) })
  await expectStatus('GET', `${BASE_URL}/api/agent/account/v1/api-keys/${state.ids.keyId}`, 200, { headers: bearer(state.user.token) })
  await expectStatus('PUT', `${BASE_URL}/api/agent/account/v1/api-keys/${state.ids.keyId}`, 200, {
    headers: bearer(state.user.token),
    body: { name: `e2e-key-updated-${RUN_ID}`, total_quota_points: '40.00000', daily_quota_points: '40.00000', rpm_limit: 60 },
  })

  const developerKey = await expectStatus('POST', `${BASE_URL}/api/agent/developer/v1/api-keys`, 201, {
    headers: bearer(state.user.token),
    body: { name: `e2e-developer-key-${RUN_ID}` },
  })
  const developerKeyId = String(data(developerKey).api_key?.id || data(developerKey).id)
  if (!developerKeyId || developerKeyId === 'undefined') fail('Developer API key response did not include an id', { body: developerKey.text })
  await expectStatus('GET', `${BASE_URL}/api/agent/developer/v1/api-keys`, 200, { headers: bearer(state.user.token) })
  await expectStatus('PATCH', `${BASE_URL}/api/agent/developer/v1/api-keys/${developerKeyId}`, 200, {
    headers: bearer(state.user.token),
    body: { name: `e2e-developer-key-updated-${RUN_ID}` },
  })
  await expectStatus('POST', `${BASE_URL}/api/agent/developer/v1/api-keys/${developerKeyId}/reset-secret`, 200, { headers: bearer(state.user.token) })
  return { keyId: state.ids.keyId, developerKeyId, accessKey: state.apiKey.accessKey }
}

async function happyPathAssetsAndTasks() {
  await expectStatus('PUT', `${BASE_URL}/api/agent/user/v1/preferences`, 200, {
    headers: bearer(state.user.token),
    body: { theme: 'dark', default_locale: 'en-US' },
  })
  const avatarForm = new FormData()
  avatarForm.append('file', new Blob([Buffer.from(TINY_PNG_BASE64, 'base64')], { type: 'image/png' }), `avatar-${RUN_ID}.png`)
  await expectStatus('POST', `${BASE_URL}/api/agent/user/v1/avatar`, 200, {
    headers: bearer(state.user.token),
    body: avatarForm,
  })

  await expectStatus('GET', `${BASE_URL}/api/agent/image/v1/capabilities`, 200, { headers: bearer(state.user.token) })
  const uploadForm = new FormData()
  uploadForm.append('file', new Blob([Buffer.from(TINY_PNG_BASE64, 'base64')], { type: 'image/png' }), `e2e-${RUN_ID}.png`)
  const asset = await expectStatus('POST', `${BASE_URL}/api/agent/image/v1/reference-assets`, 201, {
    headers: bearer(state.user.token),
    body: uploadForm,
  })
  state.ids.assetId = data(asset).asset_id || data(asset).id || data(asset).asset?.id
  if (!state.ids.assetId) fail('Reference asset response did not include an asset id', { body: asset.text })
  await expectStatus('GET', `${BASE_URL}/api/agent/image/v1/reference-assets/${state.ids.assetId}`, 200, { headers: bearer(state.user.token) })
  await expectStatus('GET', `${BASE_URL}/api/agent/image/v1/reference-assets/${state.ids.assetId}/download`, 200, { headers: bearer(state.user.token) })

  const task = await expectStatus('POST', `${BASE_URL}/api/agent/image/v1/tasks`, 202, {
    headers: {
      ...bearer(state.user.token),
      'Idempotency-Key': `e2e-agent-task-${RUN_ID}`,
    },
    body: {
      task_type: 'text_to_image',
      prompt: 'docker e2e prompt',
      abstract_model: 'basic',
      requested_quality: 'auto',
      requested_size: '1024x1024',
      requested_output_image_count: 1,
      reference_image_count: 0,
      response_mode: 'async',
    },
  })
  state.ids.taskId = data(task).id
  await expectStatus('GET', `${BASE_URL}/api/agent/image/v1/tasks`, 200, { headers: bearer(state.user.token) })
  await expectStatus('GET', `${BASE_URL}/api/agent/image/v1/tasks/${state.ids.taskId}`, 200, { headers: bearer(state.user.token) })
  await expectStatus('GET', `${BASE_URL}/api/agent/image/v1/history/tasks`, 200, { headers: bearer(state.user.token) })
  await expectStatus('GET', `${BASE_URL}/api/agent/image/v1/history/tasks/${state.ids.taskId}`, 200, { headers: bearer(state.user.token) })
  return { assetId: state.ids.assetId, taskId: state.ids.taskId }
}

async function happyPathOpenAPI() {
  const estimatePath = '/api/open/image/v1/estimate?task_type=text_to_image&abstract_model=basic&requested_quality=auto&requested_size=1024x1024&requested_output_image_count=1&reference_image_count=0'
  const estimate = await signedRequest('GET', estimatePath)
  if (estimate.status !== 200) fail('Native Open API estimate failed', { status: estimate.status, body: estimate.text })
  await signedOk('GET', '/api/open/image/v1/capabilities')
  await signedOk('GET', '/api/open/image/v1/balance')

  const upload = await signedRequest('POST', '/api/open/image/v1/reference-assets/uploads', {
    filename: `open-e2e-${RUN_ID}.png`,
    mime_type: 'image/png',
    content_base64: TINY_PNG_BASE64,
  })
  if (upload.status !== 201) fail('Native Open API inline upload failed', { status: upload.status, body: upload.text })
  const openAssetId = data(upload).asset_id || data(upload).asset?.id
  if (openAssetId) {
    await signedOk('GET', `/api/open/image/v1/reference-assets/${openAssetId}`)
  }

  const task = await signedRequest('POST', '/api/open/image/v1/tasks', {
    task_type: 'text_to_image',
    prompt: 'docker e2e open api prompt',
    abstract_model: 'basic',
    requested_quality: 'auto',
    requested_size: '1024x1024',
    requested_output_image_count: 1,
    response_mode: 'async',
  })
  if (task.status !== 202) fail('Native Open API task create failed', { status: task.status, body: task.text })
  const taskId = data(task).id
  if (taskId) await signedOk('GET', `/api/open/image/v1/tasks/${taskId}`)

  const models = await expectStatus('GET', `${BASE_URL}/v1/models`, 200, {
    headers: { Authorization: `Bearer ${state.apiKey.secret}` },
  })
  if (!Array.isArray(data(models))) fail('OpenAI-compatible models response data was not an array')
  const generation = await request('POST', `${BASE_URL}/v1/images/generations`, {
    headers: { Authorization: `Bearer ${state.apiKey.secret}`, 'Content-Type': 'application/json' },
    body: { prompt: 'docker e2e compat prompt', model: 'basic', n: 1, size: '1024x1024' },
  })
  if (isExpectedProviderUnavailable(generation)) {
    warn('OpenAI-compatible generation returned expected provider-unavailable response', { status: generation.status })
  } else if (generation.status >= 500 || [404, 405].includes(generation.status)) {
    fail('OpenAI-compatible image generation route failed contract check', { status: generation.status, body: generation.text })
  }
  return { openAssetId, openTaskStatus: task.status, compatGenerationStatus: generation.status }
}

async function signedOk(method, pathWithQuery) {
  const result = await signedRequest(method, pathWithQuery)
  if (result.status < 200 || result.status >= 300) {
    fail(`Signed ${method} ${pathWithQuery} failed`, { status: result.status, body: result.text })
  }
  return result
}

async function happyPathAdmin() {
  await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/metrics/dashboard`, 200, { headers: bearer(state.admin.token) })
  await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/config-tabs`, 200, { headers: bearer(state.admin.token) })
  await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/users?page=1&page_size=20`, 200, { headers: bearer(state.admin.token) })
  await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/users/${state.ids.userId}`, 200, { headers: bearer(state.admin.token) })
  const manualEmail = `manual-${RUN_ID}@example.com`
  const manualPassword = `manual-password-${RUN_ID}`
  const createdUser = await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/users`, 201, {
    headers: bearer(state.admin.token),
    body: { email: manualEmail, nickname: `Manual ${RUN_ID}`, status: 'active', user_group_code: 'basic', password: manualPassword, rpm_limit: 30, concurrency_limit: 1 },
  })
  state.ids.manualUserId = String(data(createdUser).id)
  await expectStatus('POST', `${BASE_URL}/api/agent/auth/v1/login/password`, 200, {
    body: { email: manualEmail, password: manualPassword },
  })
  await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/users/${state.ids.manualUserId}/status`, 200, {
    headers: bearer(state.admin.token),
    body: { status: 'disabled' },
  })
  const disabledDetail = await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/users/${state.ids.manualUserId}`, 200, { headers: bearer(state.admin.token) })
  if (data(disabledDetail).user?.status !== 'disabled') fail('Manual user status did not refresh to disabled', { body: disabledDetail.text })
  if (typeof data(disabledDetail).balance?.available_points !== 'string') fail('Manual user detail balance was not a structured balance summary', { body: disabledDetail.text })
  await expectStatus('DELETE', `${BASE_URL}/api/ops/admin/v1/users/${state.ids.manualUserId}`, 200, { headers: bearer(state.admin.token) })
  const hiddenDeleted = await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/users?page=1&page_size=20&query=${encodeURIComponent(manualEmail)}`, 200, { headers: bearer(state.admin.token) })
  if ((data(hiddenDeleted).items ?? []).some((item) => item.email === manualEmail)) fail('Soft-deleted user was still visible in admin list', { body: hiddenDeleted.text })
  await expectStatus('POST', `${BASE_URL}/api/agent/auth/v1/email/send-code`, 202, { body: { email: manualEmail, scene: 'login' } })
  await expectStatus('POST', `${BASE_URL}/api/agent/auth/v1/login/email-code`, 200, { body: { email: manualEmail, code: '123456' } })
  await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/users/${state.ids.userId}/limits`, 200, {
    headers: bearer(state.admin.token),
    body: { rpm_limit: 120, concurrency_limit: 2 },
  })

  const groupCode = state.ids.groupCode
  await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/user-groups`, 201, {
    headers: bearer(state.admin.token),
    body: { group_code: groupCode, group_name: `E2E ${RUN_ID}`, multiplier: '1.00000', status: 'active' },
  })
  await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/user-groups/${groupCode}`, 200, { headers: bearer(state.admin.token) })
  await expectStatus('PUT', `${BASE_URL}/api/ops/admin/v1/user-groups/${groupCode}`, 200, {
    headers: bearer(state.admin.token),
    body: { group_code: groupCode, group_name: `E2E Updated ${RUN_ID}`, multiplier: '1.10000', status: 'active' },
  })
  await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/users/${state.ids.userId}/group`, 200, {
    headers: bearer(state.admin.token),
    body: { user_group_code: groupCode },
  })

  const validUntil = new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString()
  const code = `E2E-${RUN_ID}`
  const redeem = await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/redeem-codes`, 201, {
    headers: bearer(state.admin.token),
    body: { code, status: 'available', reward_type: 'points', reward_value: '2.00000', valid_until: validUntil, max_redemptions: 1 },
  })
  state.ids.codeId = String(data(redeem).id)
  await expectStatus('POST', `${BASE_URL}/api/agent/billing/v1/redeem-codes/redeem`, 200, {
    headers: {
      ...bearer(state.user.token),
      'Idempotency-Key': `e2e-redeem-${RUN_ID}`,
    },
    body: { code },
  })
  await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/redeem-codes`, 200, { headers: bearer(state.admin.token) })
  await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/redeem-codes/${state.ids.codeId}/status`, 200, {
    headers: bearer(state.admin.token),
    body: { status: 'disabled' },
  })
  await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/redeem-codes/${state.ids.codeId}/redemptions`, 200, { headers: bearer(state.admin.token) })

  const providerCode = state.ids.providerCode
  await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/model-providers`, 201, {
    headers: bearer(state.admin.token),
    body: { provider_code: providerCode, provider_type: 'openai', auth_config_encrypted: 'e2e-cipher', health_status: 'healthy', enabled: true },
  })
  await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/model-providers/${providerCode}`, 200, { headers: bearer(state.admin.token) })
  const providerModel = await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/provider-models`, 201, {
    headers: bearer(state.admin.token),
    body: {
      provider_code: providerCode,
      model_code: `e2e-model-${RUN_ID}`,
      compat_mode: 'openai_images',
      supports_image_input: true,
      supports_mask: true,
      supported_qualities: ['1k'],
      supported_ratios: ['1:1'],
      max_image_count: 1,
      max_reference_image_count: 1,
      timeout_ms: 30000,
      input_cost: '0.01000',
      output_cost: '0.02000',
      currency: 'USD',
      health_status: 'healthy',
      enabled: true,
    },
  })
  state.ids.providerModelId = String(data(providerModel).id)
  const route = await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/model-routes`, 201, {
    headers: bearer(state.admin.token),
    body: { group_code: groupCode, task_type: 'text_to_image', provider_code: providerCode, priority: 1, weight_percent: 100, fallback_order: 1, enabled: true },
  })
  state.ids.routeId = String(data(route).id)
  await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/model-routes/${state.ids.routeId}`, 200, { headers: bearer(state.admin.token) })
  await expectStatus('PUT', `${BASE_URL}/api/ops/admin/v1/model-routes/${state.ids.routeId}`, 200, {
    headers: bearer(state.admin.token),
    body: { group_code: groupCode, task_type: 'text_to_image', provider_code: providerCode, priority: 2, weight_percent: 100, fallback_order: 1, enabled: false },
  })
  await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/call-records`, 200, { headers: bearer(state.admin.token) })
  await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/image-reviews`, 200, { headers: bearer(state.admin.token) })
  await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/audit-logs`, 200, { headers: bearer(state.admin.token) })
  return { groupCode, codeId: state.ids.codeId, providerCode, routeId: state.ids.routeId, manualUserId: state.ids.manualUserId }
}

async function corsSweep(openapi) {
  const origins = [USER_WEB_URL.replace('127.0.0.1', 'localhost'), ADMIN_WEB_URL.replace('127.0.0.1', 'localhost')]
  let checked = 0
  for (const [template, ops] of Object.entries(openapi.paths)) {
    for (const method of Object.keys(ops)) {
      if (method.toUpperCase() === 'GET') continue
      const pathValue = materializePath(template)
      for (const origin of origins) {
        const result = await request('OPTIONS', `${BASE_URL}${pathValue}`, {
          headers: {
            Origin: origin,
            'Access-Control-Request-Method': method.toUpperCase(),
            'Access-Control-Request-Headers': 'content-type, authorization, idempotency-key',
          },
        })
        if (result.status !== 204) {
          fail('CORS preflight failed', { method, path: pathValue, origin, status: result.status, body: result.text })
        }
        const allowOrigin = result.headers.get('access-control-allow-origin')
        const allowCredentials = result.headers.get('access-control-allow-credentials')
        if (allowOrigin !== origin || allowCredentials !== 'true') {
          fail('CORS preflight headers were incomplete', { method, path: pathValue, origin, allowOrigin, allowCredentials })
        }
        checked += 1
      }
    }
  }
  return { checked }
}

async function openapiRouteSweep(openapi) {
  const failures = []
  const warnings = []
  let checked = 0
  const operations = []
  for (const [template, ops] of Object.entries(openapi.paths)) {
    for (const [method, operation] of Object.entries(ops)) {
      operations.push({ template, method: method.toUpperCase(), operation })
    }
  }
  operations.sort((a, b) => methodOrder(a.method) - methodOrder(b.method))

  for (const { template, method, operation } of operations) {
    const pathValue = materializePath(template)
    const query = defaultQuery(template)
    const pathWithQuery = `${pathValue}${query}`
    const body = defaultBody(method, template)
    const headers = defaultHeaders(template, method, pathWithQuery, body)
    const result = await request(method, `${BASE_URL}${pathWithQuery}`, { headers, body: body || undefined })
    checked += 1
    if (isExpectedSemanticNotFound(result, template)) {
      warnings.push({ operationId: operation.operationId, method, path: pathWithQuery, status: result.status })
      continue
    }
    if ((result.status >= 500 && !isExpectedProviderUnavailable(result)) || result.status === 404 || result.status === 405) {
      failures.push({
        operationId: operation.operationId,
        method,
        path: pathWithQuery,
        status: result.status,
        body: result.text.slice(0, 600),
      })
    }
  }
  if (failures.length > 0) {
    fail('OpenAPI route sweep found contract failures', { failures })
  }
  if (warnings.length > 0) {
    warn('OpenAPI route sweep hit endpoints with expected missing preconditions', { warnings })
  }
  return { checked, semanticNotFoundWarnings: warnings.length }
}

function isExpectedProviderUnavailable(result) {
  return result.status === 503 && result.json?.error?.code === 'UPSTREAM_UNAVAILABLE'
}

function isExpectedSemanticNotFound(result, template) {
  if (result.status !== 404 || result.json?.error?.code !== 'NOT_FOUND') return false
  return template.includes('{image_id}') ||
    template.includes('/gallery/images') ||
    template.includes('/payments/webhooks/') ||
    template === '/api/ops/admin/v1/config-tabs/{tab_key}'
}

function methodOrder(method) {
  return { GET: 1, POST: 2, PUT: 3, PATCH: 4, DELETE: 9 }[method] || 5
}

function defaultHeaders(template, method, pathWithQuery, body) {
  const headers = {}
  if (body) headers['Content-Type'] = 'application/json'
  if (template.startsWith('/api/agent/')) {
    headers.Authorization = `Bearer ${state.user.token}`
    if (method !== 'GET') headers['Idempotency-Key'] = `sweep-${RUN_ID}-${method}-${template}`.replace(/[^a-zA-Z0-9-]/g, '-')
  } else if (template.startsWith('/api/ops/admin/') && template !== '/api/ops/admin/v1/auth/login') {
    headers.Authorization = `Bearer ${state.admin.token}`
    if (method !== 'GET') headers['Idempotency-Key'] = `sweep-admin-${RUN_ID}-${method}-${template}`.replace(/[^a-zA-Z0-9-]/g, '-')
  } else if (template.startsWith('/api/open/image/v1/') && !template.includes('/payments/webhooks/')) {
    Object.assign(headers, signNative(method, pathWithQuery, body || ''))
  } else if (template.startsWith('/v1/')) {
    headers.Authorization = `Bearer ${state.apiKey.secret}`
  }
  return headers
}

function materializePath(template) {
  return template
    .replace('{key_id}', state.ids.keyId)
    .replace('{order_id}', state.ids.orderId)
    .replace('{task_id}', state.ids.taskId)
    .replace('{asset_id}', state.ids.assetId)
    .replace('{image_id}', state.ids.imageId)
    .replace('{channel}', 'alipay')
    .replace('{tab_key}', 'billing_pricing')
    .replace('{user_id}', state.ids.userId)
    .replace('{group_code}', state.ids.groupCode)
    .replace('{code_id}', state.ids.codeId)
    .replace('{provider_code}', state.ids.providerCode)
    .replace('{route_id}', state.ids.routeId)
    .replace('{provider_model_id}', state.ids.providerModelId)
}

function defaultQuery(template) {
  if (template.includes('/estimate')) {
    return '?task_type=text_to_image&abstract_model=basic&requested_quality=auto&requested_size=1024x1024&requested_output_image_count=1&reference_image_count=0'
  }
  if (template.endsWith('/users')) return '?page=1&page_size=20'
  if (template.endsWith('/audit-logs')) return '?page=1&page_size=20'
  if (template.endsWith('/call-records')) return '?page=1&page_size=20'
  if (template.endsWith('/redeem-codes')) return '?page=1&page_size=20'
  if (template.endsWith('/model-providers')) return '?page=1&page_size=20'
  if (template.endsWith('/provider-models')) return '?page=1&page_size=20'
  if (template.endsWith('/model-routes')) return '?page=1&page_size=20'
  if (template.endsWith('/user-groups')) return '?page=1&page_size=20'
  if (template.endsWith('/gallery/images')) return '?page=1&page_size=20'
  return ''
}

function defaultBody(method, template) {
  if (method === 'GET' || method === 'DELETE') return ''
  const bodies = {
    '/api/agent/account/v1/api-keys': { name: `sweep-key-${RUN_ID}`, total_quota_points: '5.00000', daily_quota_points: '5.00000', rpm_limit: 60 },
    '/api/agent/account/v1/api-keys/{key_id}': { name: `sweep-key-updated-${RUN_ID}`, total_quota_points: '4.00000', daily_quota_points: '4.00000', rpm_limit: 60 },
    '/api/agent/auth/v1/email/send-code': { email: uniqueEmail('sweep-code'), scene: 'login' },
    '/api/agent/auth/v1/login/email-code': { email: state.user.email, code: '123456' },
    '/api/agent/auth/v1/login/password': { email: state.user.email, password: 'not-the-current-password' },
    '/api/agent/auth/v1/password/change': { old_password: 'wrong-password', new_password: 'new-password-123' },
    '/api/agent/auth/v1/password/reset/request': { email: state.user.email },
    '/api/agent/auth/v1/password/reset/confirm': { email: state.user.email, code: '123456', new_password: 'reset-password-123' },
    '/api/agent/billing/v1/orders': { plan_code: 'basic-monthly', provider: 'alipay' },
    '/api/agent/image/v1/reference-assets': { filename: `sweep-${RUN_ID}.png`, mime_type: 'image/png', content_base64: TINY_PNG_BASE64 },
    '/api/agent/image/v1/tasks': { task_type: 'text_to_image', prompt: 'sweep prompt', abstract_model: 'basic', requested_quality: 'auto', requested_size: '1024x1024', requested_output_image_count: 1, response_mode: 'async' },
    '/api/agent/user/v1/profile': { display_name: `E2E ${RUN_ID}` },
    '/api/agent/user/v1/account/close': { reason: 'sweep' },
    '/api/open/image/v1/payments/webhooks/{channel}': { order_no: `missing-${RUN_ID}`, trade_no: `trade-${RUN_ID}` },
    '/api/open/image/v1/reference-assets': { filename: `open-sweep-${RUN_ID}.png`, mime_type: 'image/png' },
    '/api/open/image/v1/reference-assets/uploads': { filename: `open-sweep-${RUN_ID}.png`, mime_type: 'image/png', content_base64: TINY_PNG_BASE64 },
    '/api/open/image/v1/tasks': { task_type: 'text_to_image', prompt: 'sweep open prompt', abstract_model: 'basic', requested_quality: 'auto', requested_size: '1024x1024', requested_output_image_count: 1, response_mode: 'async' },
    '/api/ops/admin/v1/auth/login': { email: 'admin@example.com', password: 'admin123456' },
    '/api/ops/admin/v1/config-tabs/{tab_key}': { settings: {} },
    '/api/ops/admin/v1/users': { email: `sweep-user-${RUN_ID}@example.com`, nickname: `Sweep User ${RUN_ID}`, status: 'active', user_group_code: 'basic' },
    '/api/ops/admin/v1/model-providers': { provider_code: `sweep-provider-${RUN_ID}`.toLowerCase(), provider_type: 'openai', auth_config_encrypted: 'cipher', health_status: 'healthy', enabled: true },
    '/api/ops/admin/v1/model-providers/{provider_code}': { provider_code: state.ids.providerCode, provider_type: 'openai', auth_config_encrypted: 'cipher', health_status: 'healthy', enabled: true },
    '/api/ops/admin/v1/model-routes': { group_code: state.ids.groupCode, task_type: 'image_to_image', provider_code: state.ids.providerCode, priority: 5, weight_percent: 100, fallback_order: 1, enabled: true },
    '/api/ops/admin/v1/model-routes/{route_id}': { group_code: state.ids.groupCode, task_type: 'text_to_image', provider_code: state.ids.providerCode, priority: 3, weight_percent: 100, fallback_order: 1, enabled: true },
    '/api/ops/admin/v1/provider-models': { provider_code: state.ids.providerCode, model_code: `sweep-model-${RUN_ID}`, compat_mode: 'openai_images', supports_image_input: true, supports_mask: true, supported_qualities: ['1k'], supported_ratios: ['1:1'], max_image_count: 1, max_reference_image_count: 1, timeout_ms: 30000, input_cost: '0.01000', output_cost: '0.02000', currency: 'USD', health_status: 'healthy', enabled: true },
    '/api/ops/admin/v1/provider-models/{provider_model_id}': { provider_code: state.ids.providerCode, model_code: `e2e-model-${RUN_ID}`, compat_mode: 'openai_images', supports_image_input: true, supports_mask: true, supported_qualities: ['1k'], supported_ratios: ['1:1'], max_image_count: 1, max_reference_image_count: 1, timeout_ms: 30000, input_cost: '0.01000', output_cost: '0.02000', currency: 'USD', health_status: 'healthy', enabled: true },
    '/api/ops/admin/v1/redeem-codes': { code: `SWEEP-${RUN_ID}`, status: 'available', reward_type: 'points', reward_value: '1.00000', valid_until: new Date(Date.now() + 86400000).toISOString(), max_redemptions: 1 },
    '/api/ops/admin/v1/redeem-codes/{code_id}/status': { status: 'disabled' },
    '/api/ops/admin/v1/redeem-codes:batch-create': { count: 1, status: 'available', reward_type: 'points', reward_value: '1.00000', valid_until: new Date(Date.now() + 86400000).toISOString(), max_redemptions: 1 },
    '/api/ops/admin/v1/user-groups': { group_code: `sweep-group-${RUN_ID}`.toLowerCase(), group_name: `Sweep ${RUN_ID}`, multiplier: '1.00000', status: 'active' },
    '/api/ops/admin/v1/user-groups/{group_code}': { group_code: state.ids.groupCode, group_name: `Sweep Updated ${RUN_ID}`, multiplier: '1.20000', status: 'active' },
    '/api/ops/admin/v1/users/{user_id}/group': { user_group_code: state.ids.groupCode },
    '/api/ops/admin/v1/users/{user_id}/limits': { rpm_limit: 120, concurrency_limit: 2 },
    '/api/ops/admin/v1/users/{user_id}/points-adjustments': { change_points: '1.00000', reason: `sweep ${RUN_ID}` },
    '/api/ops/admin/v1/users/{user_id}/reset-password': { new_password: 'reset-password-123' },
    '/api/ops/admin/v1/users/{user_id}/status': { status: 'active' },
    '/v1/images/edits': { prompt: 'sweep edit prompt', model: 'basic', n: 1, size: '1024x1024' },
    '/v1/images/generations': { prompt: 'sweep generation prompt', model: 'basic', n: 1, size: '1024x1024' },
  }
  return JSON.stringify(bodies[template] || {})
}

async function writeReport() {
  await fs.mkdir(REPORT_DIR, { recursive: true })
  const report = {
    runId: RUN_ID,
    urls: { BASE_URL, USER_WEB_URL, ADMIN_WEB_URL, NGINX_URL },
    status: state.steps.some(item => item.status === 'fail') ? 'FAIL' : 'PASS',
    warnings: state.warnings,
    steps: state.steps,
  }
  const jsonPath = path.join(REPORT_DIR, 'latest-report.json')
  const mdPath = path.join(REPORT_DIR, 'latest-report.md')
  await fs.writeFile(jsonPath, `${JSON.stringify(report, null, 2)}\n`)
  await fs.writeFile(mdPath, [
    `# Pic Gallery Docker E2E ${report.status}`,
    '',
    `- Run ID: ${RUN_ID}`,
    `- API: ${BASE_URL}`,
    `- User Web: ${USER_WEB_URL}`,
    `- Admin Web: ${ADMIN_WEB_URL}`,
    `- Nginx: ${NGINX_URL}`,
    '',
    '## Steps',
    ...state.steps.map(item => `- ${item.status.toUpperCase()} ${item.name}`),
    '',
    state.warnings.length ? '## Warnings' : '',
    ...state.warnings.map(item => `- ${item.name}: ${JSON.stringify(item.detail)}`),
    '',
  ].filter(Boolean).join('\n'))
  return { jsonPath, mdPath }
}

async function main() {
  try {
    await step('wait for API readiness', async () => {
      await waitFor(`${BASE_URL}/readyz`)
      await expectStatus('GET', `${BASE_URL}/readyz`, 200)
    })
    await step('wait for frontend and middleware services', async () => {
      await waitFor(`${USER_WEB_URL}/`)
      await waitFor(`${ADMIN_WEB_URL}/`)
      await waitFor(`${NGINX_URL}/readyz`)
      await waitFor('http://127.0.0.1:9000/minio/health/live')
      await waitFor('http://127.0.0.1:8025/api/v1/info')
    })
    const openapi = await loadOpenAPI()
    await step('user and admin web routes return app shell', async () => {
      await checkHtmlApp('user web', USER_WEB_URL, ['landing', 'login', 'home', 'genpic', 'gallery', 'api-keys', 'profile', 'docs'])
      await checkHtmlApp('admin web', ADMIN_WEB_URL, ['login', 'overview', 'config', 'routing', 'pricing', 'reviews', 'users', 'redeem', 'call-records', 'provider-models', 'audit', 'health'])
    })
    await step('frontend shared API client unwraps auth tokens', frontendApiClientSmoke)
    await step('CORS preflight sweep for browser origins', async () => corsSweep(openapi))
    await step('bootstrap user session', bootstrapUser)
    await step('bootstrap admin session', bootstrapAdmin)
    await step('seed user points through admin API', seedPoints)
    await step('agent billing happy path', happyPathAgentBilling)
    await step('agent API key happy path', happyPathApiKeys)
    await step('agent assets and image task happy path', happyPathAssetsAndTasks)
    await step('native Open API and OpenAI-compatible API happy path', happyPathOpenAPI)
    await step('admin management happy path', happyPathAdmin)
    await step('legacy and non-OpenAPI route coverage', async () => {
      await expectStatus('POST', `${BASE_URL}/api/agent/auth/v1/password/reset`, [200, 400], {
        body: { email: state.user.email, code: '123456', new_password: 'legacy-reset-password-123' },
      })
      return { checked: 1 }
    })
    await step('OpenAPI route coverage sweep', async () => openapiRouteSweep(openapi))
    await step('admin logout route coverage', async () => {
      await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/auth/logout`, 204, {
        headers: bearer(state.admin.token),
      })
      return { checked: 1 }
    })
    const report = await writeReport()
    console.log(`Docker E2E passed. Report: ${report.mdPath}`)
  } catch (error) {
    const report = await writeReport().catch(() => null)
    if (report) console.error(`Docker E2E failed. Report: ${report.mdPath}`)
    console.error(error.message)
    if (error.detail) console.error(JSON.stringify(error.detail, null, 2))
    process.exitCode = 1
  }
}

await main()
