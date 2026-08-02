#!/usr/bin/env node

import crypto from 'node:crypto'
import fs from 'node:fs/promises'
import http from 'node:http'
import path from 'node:path'
import { spawn } from 'node:child_process'
import { fileURLToPath, pathToFileURL } from 'node:url'

const ROOT_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const BASE_URL = envUrl('BASE_URL', 'http://127.0.0.1:8088')
const USER_WEB_URL = envUrl('USER_WEB_URL', 'http://127.0.0.1:8088')
const ADMIN_WEB_URL = envUrl('ADMIN_WEB_URL', 'http://127.0.0.1:8088/admin')
const NGINX_URL = envUrl('NGINX_URL', 'http://127.0.0.1:8088')
const MINIO_URL = envUrl('MINIO_URL', `http://127.0.0.1:${process.env.MINIO_API_PORT || '9000'}`)
const MAILPIT_URL = envUrl('MAILPIT_URL', `http://127.0.0.1:${process.env.MAILPIT_UI_PORT || '8025'}`)
const REPORT_DIR = path.join(ROOT_DIR, 'tmp/e2e')
const RUN_ID = new Date().toISOString().replace(/[-:.TZ]/g, '').slice(0, 14)
const IMAGE_PROVIDER_DELAY_MS = Number.parseInt(process.env.E2E_IMAGE_PROVIDER_DELAY_MS || '0', 10)
const IMAGE_PROVIDER_MARKER = process.env.E2E_IMAGE_PROVIDER_MARKER || ''
const SKIP_MIDDLEWARE_HEALTH = process.env.E2E_SKIP_MIDDLEWARE_HEALTH === 'true'
const E2E_ADMIN_EMAIL = process.env.E2E_ADMIN_EMAIL || 'admin@example.com'
const E2E_ADMIN_PASSWORD = process.env.E2E_ADMIN_PASSWORD || 'admin123456'
const TINY_PNG_BASE64 = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+y1X8AAAAASUVORK5CYII='
const FAKE_PROVIDER_IMAGE_BASE64 = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGNkaGAAAAHAAZcAzSrgAAAAAElFTkSuQmCC'

const state = {
  steps: [],
  warnings: [],
  fakeImageRequests: [],
  fakeTextRequests: [],
  user: {},
  admin: {},
  apiKey: {},
  storageConfig: {},
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
    storageConfigId: 'missing-storage-config',
    textModelAccountId: '1',
    textModelId: '1',
    providerCode: `e2e-provider-${RUN_ID}`.toLowerCase(),
    groupCode: `e2e-group-${RUN_ID}`.toLowerCase(),
    storageConfigId: '1',
    storageConfigVersion: 1,
  },
}

let fakeProviderServer = null
let fakeProviderPort = 0
let delayedImageProviderRequest = false

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

function findLedgerItem(items, ledgerType, balanceBucket, sourceType) {
  return (items || []).find(item =>
    item?.ledger_type === ledgerType &&
    item?.balance_bucket === balanceBucket &&
    item?.bucket_type === balanceBucket &&
    item?.source_type === sourceType
  )
}

function firstField(item, ...keys) {
  for (const key of keys) {
    if (item && item[key] !== undefined && item[key] !== null) return item[key]
  }
  return undefined
}

async function startFakeProvider() {
  if (fakeProviderServer) {
    return {
      hostURL: `http://127.0.0.1:${fakeProviderPort}`,
      containerURL: `http://host.docker.internal:${fakeProviderPort}`,
    }
  }
  const png = Buffer.from(FAKE_PROVIDER_IMAGE_BASE64, 'base64')
  fakeProviderServer = http.createServer(async (req, res) => {
    if (req.method === 'GET' && req.url === '/images/smoke.png') {
      res.writeHead(200, { 'Content-Type': 'image/png', 'Content-Length': String(png.length) })
      res.end(png)
      return
    }
    if (req.method === 'POST' && req.url === '/chat/completions') {
      const chunks = []
      for await (const chunk of req) chunks.push(chunk)
      const body = Buffer.concat(chunks).toString('utf8')
      const prompt = [
        'docker e2e prompt',
        'docker e2e open api prompt',
        'docker e2e compat prompt',
      ].find(value => body.includes(value)) || ''
      const requestNumber = state.fakeImageRequests.length + 1
      state.fakeImageRequests.push({ requestNumber, prompt, bodyLength: Buffer.byteLength(body) })
      if (IMAGE_PROVIDER_MARKER) {
        await fs.mkdir(path.dirname(IMAGE_PROVIDER_MARKER), { recursive: true })
        await fs.appendFile(IMAGE_PROVIDER_MARKER, `${JSON.stringify({ requestNumber, prompt, at: new Date().toISOString() })}\n`)
      }
      if (!delayedImageProviderRequest && Number.isFinite(IMAGE_PROVIDER_DELAY_MS) && IMAGE_PROVIDER_DELAY_MS > 0) {
        delayedImageProviderRequest = true
        await sleep(IMAGE_PROVIDER_DELAY_MS)
      }
      if (res.destroyed) return
      const payload = JSON.stringify({
        choices: [
          {
            message: {
              images: [
                { image_url: { url: `http://host.docker.internal:${fakeProviderPort}/images/smoke.png` } },
              ],
            },
          },
        ],
      })
      res.writeHead(200, { 'Content-Type': 'application/json', 'Content-Length': String(Buffer.byteLength(payload)), 'x-request-id': 'docker-e2e-fake-provider' })
      res.end(payload)
      return
    }
    if (req.method === 'POST' && (req.url === '/v1/chat/completions' || req.url === '/v1/responses')) {
      let body = {}
      try {
        const chunks = []
        for await (const chunk of req) chunks.push(chunk)
        body = JSON.parse(Buffer.concat(chunks).toString('utf8') || '{}')
      } catch {
        res.writeHead(400, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ error: { code: 'invalid_json' } }))
        return
      }
      state.fakeTextRequests.push({
        path: req.url,
        model: typeof body.model === 'string' ? body.model : '',
        authorized: /^Bearer\s+\S+$/i.test(req.headers.authorization || ''),
        max_completion_tokens: Object.hasOwn(body, 'max_completion_tokens'),
        max_output_tokens: Object.hasOwn(body, 'max_output_tokens'),
      })
      const responseBody = req.url === '/v1/responses'
        ? { output_text: 'Optimized responses prompt for Docker E2E', usage: { input_tokens: 12, output_tokens: 7 } }
        : { choices: [{ message: { content: 'Optimized chat prompt for Docker E2E' } }], usage: { prompt_tokens: 11, completion_tokens: 6 } }
      const payload = JSON.stringify(responseBody)
      res.writeHead(200, { 'Content-Type': 'application/json', 'Content-Length': String(Buffer.byteLength(payload)), 'x-request-id': `docker-e2e-text-${req.url.endsWith('responses') ? 'responses' : 'chat'}` })
      res.end(payload)
      return
    }
    res.writeHead(404, { 'Content-Type': 'application/json' })
    res.end(JSON.stringify({ error: { message: 'not found' } }))
  })
  await new Promise((resolve, reject) => {
    fakeProviderServer.once('error', reject)
    fakeProviderServer.listen(0, '0.0.0.0', () => {
      fakeProviderPort = fakeProviderServer.address().port
      resolve()
    })
  })
  await waitFor(`http://127.0.0.1:${fakeProviderPort}/images/smoke.png`, { timeoutMs: 10000 })
  return {
    hostURL: `http://127.0.0.1:${fakeProviderPort}`,
    containerURL: `http://host.docker.internal:${fakeProviderPort}`,
  }
}

async function stopFakeProvider() {
  const server = fakeProviderServer
  fakeProviderServer = null
  fakeProviderPort = 0
  if (!server) return

  await new Promise((resolve, reject) => {
    server.close((error) => error ? reject(error) : resolve())
    server.closeIdleConnections?.()
  })
}

async function ensureRouteModel(code, name, sortOrder) {
  const list = await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/route-models?page=1&page_size=100&keyword=${encodeURIComponent(code)}`, 200, { headers: bearer(state.admin.token) })
  const existing = (data(list).items || []).find(item => item.code === code)
  const body = {
    code,
    name,
    description: `Docker E2E ${code} route model`,
    visibility: 'public',
    enabled: true,
    sort_order: sortOrder,
  }
  if (existing?.id) {
    const updated = await expectStatus('PUT', `${BASE_URL}/api/ops/admin/v1/route-models/${existing.id}`, 200, {
      headers: bearer(state.admin.token),
      body,
    })
    return data(updated)
  }
  const created = await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/route-models`, 201, {
    headers: bearer(state.admin.token),
    body,
  })
  return data(created)
}

async function ensureRoutePrice(routeModelId) {
  const list = await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/route-model-prices?page=1&page_size=100&route_model_id=${routeModelId}&task_type=text_to_image`, 200, { headers: bearer(state.admin.token) })
  const existing = (data(list).items || []).find(item => item.base_resolution === '1k')
  const body = {
    route_model_id: Number(routeModelId),
    task_type: 'text_to_image',
    base_resolution: '1k',
    base_points: '1.00000',
    reference_multiplier: '1.00000',
    enabled: true,
  }
  if (existing?.id) {
    const updated = await expectStatus('PUT', `${BASE_URL}/api/ops/admin/v1/route-model-prices/${existing.id}`, 200, {
      headers: bearer(state.admin.token),
      body,
    })
    return data(updated)
  }
  const created = await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/route-model-prices`, 201, {
    headers: bearer(state.admin.token),
    body,
  })
  return data(created)
}

async function createRouteCandidate(routeModelId) {
  const candidate = await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/route-models/${routeModelId}/candidates`, 201, {
    headers: bearer(state.admin.token),
    body: {
      account_model_id: Number(state.ids.accountModelId),
      priority: 1,
      weight: 100,
      fallback_order: 1,
      enabled: true,
    },
  })
  return data(candidate)
}

async function disableExistingRouteCandidates(routeModelId) {
  const list = await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/route-models/${routeModelId}/candidates`, 200, {
    headers: bearer(state.admin.token),
  })
  for (const candidate of data(list).items || []) {
    await expectStatus('PUT', `${BASE_URL}/api/ops/admin/v1/route-models/${routeModelId}/candidates/${candidate.id}`, 200, {
      headers: bearer(state.admin.token),
      body: {
        account_model_id: Number(candidate.account_model_id),
        priority: Number(candidate.priority),
        weight: Number(candidate.weight),
        fallback_order: Number(candidate.fallback_order),
        enabled: false,
      },
    })
  }
}

async function seedGenerationRoute() {
  const fakeProvider = await startFakeProvider()
  const account = await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/model-accounts`, 201, {
    headers: bearer(state.admin.token),
    body: {
      name: `Docker E2E OpenRouter ${RUN_ID}`,
      adapter_type: 'openrouter',
      auth_type: 'api_key',
      base_url: fakeProvider.containerURL,
      credentials: { api_key: `fake-openrouter-e2e-${RUN_ID}` },
      status: 'enabled',
      priority: 1,
      weight: 100,
      concurrency_limit: 2,
      timeout_ms: 30000,
    },
  })
  state.ids.modelAccountId = String(data(account).id)
  const accountModel = await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/model-accounts/${state.ids.modelAccountId}/models`, 201, {
    headers: bearer(state.admin.token),
    body: {
      model_code: 'openrouter/imagen',
      display_name: 'Docker E2E Image Model',
      task_types: ['text_to_image'],
      base_resolution: ['1k'],
      cost_per_image: '0.00000',
      currency: 'USD',
      enabled: true,
    },
  })
  state.ids.accountModelId = String(data(accountModel).id)
  const routeModel = await ensureRouteModel('basic', 'Basic', 1)
  state.ids.basicRouteModelId = String(routeModel.id)
  await disableExistingRouteCandidates(state.ids.basicRouteModelId)
  const candidate = await createRouteCandidate(state.ids.basicRouteModelId)
  state.ids.routeCandidateId = String(candidate.id)
  const price = await ensureRoutePrice(state.ids.basicRouteModelId)
  state.ids.basicRoutePriceId = String(price.id)

  const compatRouteModel = await ensureRouteModel('plus', 'Plus', 2)
  state.ids.compatRouteModelId = String(compatRouteModel.id)
  await disableExistingRouteCandidates(state.ids.compatRouteModelId)
  const compatCandidate = await createRouteCandidate(state.ids.compatRouteModelId)
  state.ids.compatRouteCandidateId = String(compatCandidate.id)
  const compatPrice = await ensureRoutePrice(state.ids.compatRouteModelId)
  state.ids.compatRoutePriceId = String(compatPrice.id)

  const capabilities = await expectStatus('GET', `${BASE_URL}/api/agent/image/v1/capabilities`, 200, { headers: bearer(state.user.token) })
  const visibleRouteCodes = (data(capabilities).model_groups || []).map(item => firstField(item, 'route_model_code', 'abstract_model', 'code', 'Code'))
  if (!visibleRouteCodes.includes('basic') || !visibleRouteCodes.includes('plus')) {
    fail('Seeded route models were not visible in user capabilities', { visibleRouteCodes, body: capabilities.text })
  }
  return {
    routeModelId: state.ids.basicRouteModelId,
    accountModelId: state.ids.accountModelId,
    routeCandidateId: state.ids.routeCandidateId,
    routePriceId: state.ids.basicRoutePriceId,
    compatRouteModelId: state.ids.compatRouteModelId,
    fakeProviderURL: fakeProvider.hostURL,
  }
}

function mockVisibleMethod() {
  return {
    method: 'mock',
    label: 'Mock 支付',
    enabled: true,
    source_provider_type: 'mock',
    scheduler_strategy: 'round_robin',
    display_order: 10,
    description: 'Docker E2E mock payment method',
  }
}

async function ensureMockCashierVisible() {
  await expectStatus('PUT', `${BASE_URL}/api/ops/admin/v1/cashier/visible-methods`, 200, {
    headers: bearer(state.admin.token),
    body: { items: [mockVisibleMethod()] },
  })
  const options = await expectStatus('GET', `${BASE_URL}/api/agent/cashier/v1/options`, 200, { headers: bearer(state.user.token) })
  const mockMethod = (data(options).visible_methods || []).find(item => item.method === 'mock' && item.enabled)
  if (!mockMethod) {
    fail('Cashier options did not expose mock payment after explicit E2E setup', { body: options.text })
  }
  return { visibleMethods: ['mock'] }
}

function decimalNumber(value) {
  const parsed = Number.parseFloat(String(value ?? '0'))
  return Number.isFinite(parsed) ? parsed : 0
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

async function waitForSucceededTask(label, loadTask, timeoutMs = 90000) {
  const started = Date.now()
  let lastTask = null
  while (Date.now() - started < timeoutMs) {
    const result = await loadTask()
    if (result.status !== 200) {
      fail(`${label} detail request failed`, { status: result.status, body: result.text })
    }
    lastTask = data(result)
    if (lastTask.status === 'succeeded') {
      if (!Array.isArray(lastTask.results) || lastTask.results.length === 0) {
        fail(`${label} succeeded without persisted image results`, { task: lastTask })
      }
      return { taskId: lastTask.id, status: lastTask.status, resultCount: lastTask.results.length }
    }
    if (lastTask.status === 'failed') {
      fail(`${label} failed`, { task: lastTask })
    }
    await sleep(250)
  }
  fail(`${label} did not succeed before timeout`, { lastTask })
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
      const adminEmail = process.env.E2E_ADMIN_EMAIL || 'admin@example.com'
      const adminPassword = process.env.E2E_ADMIN_PASSWORD || 'admin123456'
      await userApi.sendEmailCode(userEmail, 'login')
      const login = await userApi.loginWithEmailCode(userEmail, '123456')
      if (!login.access_token) throw new Error('user login was not unwrapped to access_token')
      userApi.configureAuth({ getToken: () => login.access_token })
      const profile = await userApi.getProfile()
      if (profile.email !== userEmail) throw new Error('user authenticated profile request failed')

      const adminSession = await adminApi.login(adminEmail, adminPassword)
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
  try {
    const mod = await import(`${pathToFileURL(bundlePath).href}?run=${RUN_ID}`)
    return await mod.runSmoke()
  } finally {
    await Promise.all([
      fs.rm(entryPath, { force: true }),
      fs.rm(bundlePath, { force: true }),
    ])
  }
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
  const signupGrant = session.signup_grant
  if (!signupGrant?.granted) fail('New user login did not include granted signup trial credits', { body: login.text })
  if (signupGrant.balance?.trial_points !== '20.00000') fail('Signup trial grant did not return the expected trial bucket balance', { signupGrant })
  if (!signupGrant.balance?.buckets?.some(bucket => bucket.bucket === 'trial' && bucket.expires_at)) {
    fail('Signup trial grant did not expose an expiring trial balance bucket', { signupGrant })
  }

  const profile = await expectStatus('GET', `${BASE_URL}/api/agent/user/v1/profile`, 200, {
    headers: bearer(state.user.token),
  })
  const profileData = data(profile)
  state.ids.userId = String(profileData.id)
  return { email, userId: state.ids.userId }
}

async function bootstrapAdmin() {
  const login = await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/auth/login`, 200, {
    body: { email: E2E_ADMIN_EMAIL, password: E2E_ADMIN_PASSWORD },
  })
  state.admin.token = data(login).access_token
  return { email: E2E_ADMIN_EMAIL }
}

async function enableSignupTrialCredits() {
  const tabsResult = await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/config-tabs`, 200, {
    headers: bearer(state.admin.token),
  })
  const trialTab = (data(tabsResult).items || []).find(item => item.tab_key === 'trial_credits')
  if (!trialTab) fail('Admin config did not expose the trial credits tab', { body: tabsResult.text })

  const signupTrial = (trialTab.items || []).find(item => item.config_key === 'signup_trial')
  if (!signupTrial) fail('Trial credits tab did not expose signup_trial', { trialTab })

  const updated = await expectStatus('PUT', `${BASE_URL}/api/ops/admin/v1/config-tabs/trial_credits`, 200, {
    headers: bearer(state.admin.token),
    body: {
      version: trialTab.version,
      items: trialTab.items.map(item => item.config_key === 'signup_trial'
        ? {
            ...item,
            config_value: {
              value: {
                enabled: true,
                points: '20.00000',
                valid_days: 7,
                expiry_reminder_days: 2,
                grant_once_per_user: true,
              },
            },
          }
        : item),
    },
  })
  const updatedTrial = (data(updated).items || []).find(item => item.config_key === 'signup_trial')
  if (updatedTrial?.config_value?.value?.enabled !== true) {
    fail('Signup trial config was not enabled', { body: updated.text })
  }
  return { version: data(updated).version, points: updatedTrial.config_value.value.points }
}

async function captureDefaultStorageConfig() {
  const result = await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/storage-configs`, 200, {
    headers: bearer(state.admin.token),
  })
  const storageConfig = (data(result).items || []).find(item => item.is_default)
  if (!storageConfig?.id) {
    fail('Storage config list did not expose a default config', { body: result.text })
  }
  state.storageConfig = storageConfig
  state.ids.storageConfigId = String(storageConfig.id)
  return { id: state.ids.storageConfigId, code: storageConfig.code, driver: storageConfig.driver }
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
  const initialBalance = await expectStatus('GET', `${BASE_URL}/api/agent/billing/v1/balance`, 200, { headers: bearer(state.user.token) })
  if (data(initialBalance).trial_points !== '20.00000') fail('Balance summary did not keep signup trial points after admin seed', { body: initialBalance.text })
  if (!data(initialBalance).buckets?.some(bucket => bucket.bucket === 'trial' && bucket.expires_at)) {
    fail('Balance summary did not include the trial bucket after admin seed', { body: initialBalance.text })
  }
  const initialRechargePoints = decimalNumber(data(initialBalance).recharge_points)
  const signupLedger = await expectStatus('GET', `${BASE_URL}/api/agent/billing/v1/ledger`, 200, { headers: bearer(state.user.token) })
  if (!findLedgerItem(data(signupLedger).items, 'trial_grant', 'trial', 'signup')) {
    fail('Ledger did not include signup trial grant bucket metadata', { body: signupLedger.text })
  }
  const estimate = await expectStatus('GET', `${BASE_URL}/api/agent/billing/v1/estimate?task_type=text_to_image&abstract_model=basic&base_resolution=auto&requested_size=1024x1024&requested_output_image_count=1&reference_image_count=0`, 200, {
    headers: bearer(state.user.token),
  })
  if (!data(estimate).estimated_points) fail('Estimate response did not include estimated_points')

  const options = await expectStatus('GET', `${BASE_URL}/api/agent/cashier/v1/options`, 200, { headers: bearer(state.user.token) })
  if (!data(options).visible_methods?.some(method => method.method === 'mock')) fail('Cashier options did not expose mock payment in local E2E', { body: options.text })
  if (!data(options).plans?.some(plan => plan.plan_code === 'basic-monthly')) fail('Cashier options did not expose the basic points package', { body: options.text })

  const order = await expectStatus('POST', `${BASE_URL}/api/agent/cashier/v1/orders`, 201, {
    headers: bearer(state.user.token),
    body: { purchase_type: 'plan', plan_code: 'basic-monthly', visible_method: 'mock' },
  })
  state.ids.orderId = String(data(order).id)
  if (data(order).provider !== 'mock' || data(order).visible_method !== 'mock' || data(order).status !== 'pending') {
    fail('Cashier order did not use the mock provider pending flow', { body: order.text })
  }
  if (data(order).payment_display?.type !== 'mock' || data(order).payment_url || data(order).payment_display?.payment_url) {
    fail('Cashier order did not use in-page mock payment without legacy payment_url', { body: order.text })
  }
  await expectStatus('GET', `${BASE_URL}/api/agent/billing/v1/orders`, 200, { headers: bearer(state.user.token) })
  await expectStatus('GET', `${BASE_URL}/api/agent/cashier/v1/orders/${state.ids.orderId}`, 200, { headers: bearer(state.user.token) })
  const paid = await expectStatus('POST', `${BASE_URL}/api/agent/cashier/v1/orders/${state.ids.orderId}/mock-pay`, 200, { headers: bearer(state.user.token) })
  if (data(paid).status !== 'completed' || !data(paid).ledger_id) fail('Mock cashier payment did not complete and attach a ledger id', { body: paid.text })

  const rechargedBalance = await expectStatus('GET', `${BASE_URL}/api/agent/billing/v1/balance`, 200, { headers: bearer(state.user.token) })
  const rechargeDelta = decimalNumber(data(rechargedBalance).recharge_points) - initialRechargePoints
  if (Math.abs(rechargeDelta - 100) > 0.00001) fail('Mock cashier payment did not credit 100 recharge points', { before: data(initialBalance), after: data(rechargedBalance) })
  const rechargeLedger = await expectStatus('GET', `${BASE_URL}/api/agent/billing/v1/ledger`, 200, { headers: bearer(state.user.token) })
  if (!findLedgerItem(data(rechargeLedger).items, 'recharge', 'recharge', 'payment_order')) {
    fail('Ledger did not include recharge bucket metadata after mock cashier payment', { body: rechargeLedger.text })
  }
  return { orderId: state.ids.orderId, rechargeDelta: rechargeDelta.toFixed(5) }
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
      route_model_code: 'basic',
      requested_quality: 'auto',
      requested_size: '1024x1024',
      requested_output_image_count: 1,
      reference_image_count: 0,
      response_mode: 'async',
    },
  })
  state.ids.taskId = data(task).id
  await expectStatus('GET', `${BASE_URL}/api/agent/image/v1/tasks`, 200, { headers: bearer(state.user.token) })
  const completedTask = await waitForSucceededTask('Agent image task', () => request('GET', `${BASE_URL}/api/agent/image/v1/tasks/${state.ids.taskId}`, { headers: bearer(state.user.token) }))
  await expectStatus('GET', `${BASE_URL}/api/agent/image/v1/history/tasks`, 200, { headers: bearer(state.user.token) })
  await expectStatus('GET', `${BASE_URL}/api/agent/image/v1/history/tasks/${state.ids.taskId}`, 200, { headers: bearer(state.user.token) })
  return { assetId: state.ids.assetId, taskId: state.ids.taskId, resultCount: completedTask.resultCount }
}

async function happyPathOpenAPI() {
  const estimatePath = '/api/open/image/v1/estimate?task_type=text_to_image&route_model_code=basic&requested_quality=auto&requested_size=1024x1024&requested_output_image_count=1&reference_image_count=0'
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
    route_model_code: 'basic',
    requested_quality: 'auto',
    requested_size: '1024x1024',
    requested_output_image_count: 1,
    response_mode: 'async',
  })
  if (task.status !== 202) fail('Native Open API task create failed', { status: task.status, body: task.text })
  const taskId = data(task).id
  const completedTask = await waitForSucceededTask('Native Open API image task', () => signedRequest('GET', `/api/open/image/v1/tasks/${taskId}`))

  const models = await expectStatus('GET', `${BASE_URL}/v1/models`, 200, {
    headers: { Authorization: `Bearer ${state.apiKey.secret}` },
  })
  const compatModels = data(models)
  if (!Array.isArray(compatModels)) fail('OpenAI-compatible models response data was not an array')
  const compatModel = compatModels.find(item => item.id === 'gpt-image-2')?.id
  if (!compatModel) fail('OpenAI-compatible models did not expose gpt-image-2', { models: compatModels })
  const generation = await request('POST', `${BASE_URL}/v1/images/generations`, {
    headers: { Authorization: `Bearer ${state.apiKey.secret}`, 'Content-Type': 'application/json' },
    body: { prompt: 'docker e2e compat prompt', model: compatModel, n: 1, size: '1024x1024' },
  })
  if (generation.status !== 200) {
    fail('OpenAI-compatible image generation failed', { status: generation.status, body: generation.text })
  }
  const generationImages = generation.json?.data
  if (!generation.json?.created || !Array.isArray(generationImages) || generationImages.length !== 1 || (!generationImages[0]?.url && !generationImages[0]?.b64_json)) {
    fail('OpenAI-compatible image generation returned an invalid success payload', { body: generation.text })
  }
  return { openAssetId, openTaskStatus: completedTask.status, openTaskResultCount: completedTask.resultCount, compatModel, compatGenerationStatus: generation.status }
}

async function signedOk(method, pathWithQuery) {
  const result = await signedRequest(method, pathWithQuery)
  if (result.status < 200 || result.status >= 300) {
    fail(`Signed ${method} ${pathWithQuery} failed`, { status: result.status, body: result.text })
  }
  return result
}

async function happyPathPromptOptimization() {
  const fakeProvider = await startFakeProvider()
  const secret = `fake-text-secret-${RUN_ID}`
  const createAccount = async (suffix, apiStyle) => {
    const result = await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/text-model-accounts`, 201, {
      headers: bearer(state.admin.token),
      body: {
        name: `Docker E2E Text ${suffix} ${RUN_ID}`,
        platform_type: 'openai_compatible',
        api_style: apiStyle,
        base_url: fakeProvider.containerURL,
        enabled: true,
        secrets: { api_key: secret },
      },
    })
    if (result.text.includes(secret) || !data(result).secret_status?.has_secret) {
      fail('Text model account response leaked or failed to record its secret', { body: result.text })
    }
    return data(result)
  }
  const chatAccount = await createAccount('Chat', 'chat_completions')
  const responsesAccount = await createAccount('Responses', 'responses')
  const accounts = await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/text-model-accounts`, 200, { headers: bearer(state.admin.token) })
  const accountItems = data(accounts).items || []
  if (!accountItems.some(item => String(item.id) === String(chatAccount.id)) || !accountItems.some(item => String(item.id) === String(responsesAccount.id))) {
    fail('Text model account list did not retain both configured accounts', { body: accounts.text })
  }

  const createModel = async (account, suffix) => data(await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/text-model-accounts/${account.id}/models`, 201, {
    headers: bearer(state.admin.token),
    body: {
      model_code: `docker-e2e-${suffix}`,
      display_name: `Docker E2E ${suffix}`,
      input_price_per_million_tokens: '0.000000',
      output_price_per_million_tokens: '0.000000',
      currency: 'USD',
      enabled: true,
    },
  }))
  const chatModel = await createModel(chatAccount, 'chat')
  const responsesModel = await createModel(responsesAccount, 'responses')
  if (!chatModel.is_default || responsesModel.is_default) {
    fail('The first eligible text model was not retained as the automatic default', {
      chatModel,
      responsesModel,
    })
  }
  state.ids.textModelAccountId = String(chatAccount.id)
  state.ids.textModelId = String(chatModel.id)

  const chatTest = await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/text-models/${chatModel.id}:test`, 200, { headers: bearer(state.admin.token) })
  const responsesTest = await expectStatus('POST', `${BASE_URL}/api/ops/admin/v1/text-models/${responsesModel.id}:test`, 200, { headers: bearer(state.admin.token) })
  if (data(chatTest).status !== 'success' || data(responsesTest).status !== 'success') {
    fail('Text model connection tests did not succeed', { chat: chatTest.text, responses: responsesTest.text })
  }

  const optimizeWithModel = async (model, expectedPrompt) => {
    const prompt = `Turn this into a precise image prompt for ${model.model_code}`
    const estimate = await expectStatus('POST', `${BASE_URL}/api/agent/text/v1/prompt-optimizations/estimate`, 200, {
      headers: bearer(state.user.token),
      body: { prompt },
    })
    const estimateData = data(estimate)
    if (estimateData.estimated_points !== '0.00000' || !estimateData.quote || String(estimateData.model?.id) !== String(model.id)) {
      fail('Prompt optimization estimate was not a zero-point quote for the selected model', { body: estimate.text })
    }
    const optimized = await expectStatus('POST', `${BASE_URL}/api/agent/text/v1/prompt-optimizations`, 200, {
      headers: bearer(state.user.token),
      body: { prompt, quote: estimateData.quote },
    })
    const optimizedData = data(optimized)
    if (optimizedData.actual_points !== '0.00000' || optimizedData.estimated_points !== '0.00000' || optimizedData.optimized_prompt !== expectedPrompt || !optimizedData.run_id) {
      fail('Prompt optimization result was incomplete or charged points', { body: optimized.text })
    }
    return optimizedData
  }
  await optimizeWithModel(chatModel, 'Optimized chat prompt for Docker E2E')
  await expectStatus('PUT', `${BASE_URL}/api/ops/admin/v1/text-models/${responsesModel.id}:default`, 200, { headers: bearer(state.admin.token) })
  await optimizeWithModel(responsesModel, 'Optimized responses prompt for Docker E2E')
  await expectStatus('PUT', `${BASE_URL}/api/ops/admin/v1/text-models/${chatModel.id}:default`, 200, { headers: bearer(state.admin.token) })

  const chatRequest = state.fakeTextRequests.find(item => item.path === '/v1/chat/completions' && item.model === chatModel.model_code && item.max_completion_tokens)
  const responsesRequest = state.fakeTextRequests.find(item => item.path === '/v1/responses' && item.model === responsesModel.model_code && item.max_output_tokens)
  if (!chatRequest?.authorized || !responsesRequest?.authorized) {
    fail('Fake provider did not receive sanitized Chat and Responses request shapes', { requests: state.fakeTextRequests })
  }

  const legacy = await request('POST', `${BASE_URL}/api/agent/image/v1/tasks`, {
    headers: bearer(state.user.token),
    body: {
      task_type: 'reference_generate',
      prompt: 'legacy reference generation must stay deleted',
      route_model_code: 'basic',
      size_mode: 'ratio',
      base_resolution: '1k',
      quality: 'auto',
      output_format: 'png',
      output_compression: 100,
      moderation: 'auto',
      requested_size: '1024x1024',
      aspect_ratio: '1:1',
      requested_output_image_count: 1,
      response_mode: 'async',
    },
  })
  if (legacy.status !== 400 || legacy.json?.error?.code !== 'BAD_REQUEST') {
    fail('Removed reference generation task type was not rejected', { status: legacy.status, body: legacy.text })
  }
  const historyAfterLegacy = await expectStatus('GET', `${BASE_URL}/api/agent/image/v1/history/tasks`, 200, { headers: bearer(state.user.token) })
  const historyTasks = Array.isArray(data(historyAfterLegacy)) ? data(historyAfterLegacy) : data(historyAfterLegacy)?.items ?? []
  if (historyTasks.some(task => task.prompt === 'legacy reference generation must stay deleted' || task.task_type === 'reference_generate')) {
    fail('Removed reference generation task was persisted in user history', { body: historyAfterLegacy.text })
  }
  return { chatAccountId: chatAccount.id, responsesAccountId: responsesAccount.id, defaultModelId: chatModel.id }
}

async function browserPromptWorkflow() {
  const outputDir = path.join(REPORT_DIR, `prompt-workflow-${RUN_ID}`)
  await fs.mkdir(outputDir, { recursive: true })
  const scriptPath = path.join(ROOT_DIR, 'scripts/e2e/prompt-workflow-browser.py')
  return await new Promise((resolve, reject) => {
    const child = spawn('python3', [scriptPath], {
      cwd: ROOT_DIR,
      env: {
        ...process.env,
        BASE_URL,
        USER_WEB_URL,
        ADMIN_WEB_URL,
        E2E_USER_TOKEN: state.user.token,
        E2E_ADMIN_TOKEN: state.admin.token,
        E2E_ADMIN_EMAIL,
        E2E_ADMIN_PASSWORD,
        E2E_RUN_ID: RUN_ID,
        E2E_BROWSER_OUTPUT_DIR: outputDir,
      },
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    let stdout = ''
    let stderr = ''
    child.stdout.on('data', chunk => { stdout += chunk.toString() })
    child.stderr.on('data', chunk => { stderr += chunk.toString() })
    child.once('error', reject)
    child.once('exit', code => {
      if (code !== 0) {
        reject(Object.assign(new Error('Browser prompt workflow failed'), { detail: { stderr: stderr.slice(-4000), stdout: stdout.slice(-2000) } }))
        return
      }
      resolve({ outputDir, result: stdout.trim().slice(-1000) })
    })
  })
}

async function happyPathAdmin() {
  await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/metrics/dashboard`, 200, { headers: bearer(state.admin.token) })
  await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/config-tabs`, 200, { headers: bearer(state.admin.token) })
  const storageConfigs = await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/storage-configs`, 200, { headers: bearer(state.admin.token) })
  const defaultStorageConfig = (data(storageConfigs).items || []).find(item => item.is_default) || data(storageConfigs).items?.[0]
  if (!defaultStorageConfig?.id) {
    fail('Admin storage config list did not expose a config id for OpenAPI sweep', { body: storageConfigs.text })
  }
  state.ids.storageConfigId = String(defaultStorageConfig.id)
  state.ids.storageConfigVersion = Number(defaultStorageConfig.version || 1)
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
      supported_base_resolution: ['1k'],
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
  const missingRouteCode = `missing-route-${RUN_ID}`.toLowerCase()
  const failedTask = await request('POST', `${BASE_URL}/api/agent/image/v1/tasks`, {
    headers: {
      ...bearer(state.user.token),
      'Content-Type': 'application/json',
      'Idempotency-Key': `e2e-missing-route-${RUN_ID}`,
    },
    body: {
      task_type: 'text_to_image',
      prompt: 'docker e2e route preflight failure',
      route_model_code: missingRouteCode,
      base_resolution: 'auto',
      requested_size: '1024x1024',
      requested_output_image_count: 1,
      response_mode: 'async',
    },
  })
  if (failedTask.status !== 404 || failedTask.json?.error?.code !== 'MODEL_ROUTE_NOT_FOUND') {
    fail('Missing route task did not return the expected preflight error', { status: failedTask.status, body: failedTask.text })
  }
  const failedCallRecords = await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/call-records?page=1&page_size=20&status=failed&error_code=MODEL_ROUTE_NOT_FOUND&user_id=${state.ids.userId}&source_channel=web`, 200, { headers: bearer(state.admin.token) })
  const failedRecord = (data(failedCallRecords).items || []).find(item => item.abstract_model === missingRouteCode && item.error_code === 'MODEL_ROUTE_NOT_FOUND')
  if (!failedRecord) {
    fail('Admin call records did not expose the missing route preflight failure through status/error_code filters', { body: failedCallRecords.text })
  }
  await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/image-reviews`, 200, { headers: bearer(state.admin.token) })
  await expectStatus('GET', `${BASE_URL}/api/ops/admin/v1/audit-logs`, 200, { headers: bearer(state.admin.token) })
  return { groupCode, codeId: state.ids.codeId, providerCode, routeId: state.ids.routeId, manualUserId: state.ids.manualUserId, failedCallRecordTaskId: failedRecord.task_id }
}

async function corsSweep(openapi) {
  const origins = [...new Set([
    USER_WEB_URL,
    USER_WEB_URL.replace('127.0.0.1', 'localhost'),
    ADMIN_WEB_URL,
    ADMIN_WEB_URL.replace('127.0.0.1', 'localhost'),
  ].map(url => new URL(url).origin))]
  let checked = 0
  for (const [template, ops] of normalModeOpenAPIPaths(openapi)) {
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
  for (const [template, ops] of normalModeOpenAPIPaths(openapi)) {
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
    rememberStorageConfigVersion(template, result)
  }
  if (failures.length > 0) {
    fail('OpenAPI route sweep found contract failures', { failures })
  }
  if (warnings.length > 0) {
    warn('OpenAPI route sweep hit endpoints with expected missing preconditions', { warnings })
  }
  return { checked, semanticNotFoundWarnings: warnings.length }
}

function normalModeOpenAPIPaths(openapi) {
  return Object.entries(openapi.paths).filter(([template]) => template !== '/setup' && !template.startsWith('/api/setup/'))
}

function rememberStorageConfigVersion(template, result) {
  if (!template.includes('/api/ops/admin/v1/storage-configs/{storage_config_id}')) return
  const version = Number(result.json?.data?.version)
  if (result.status >= 200 && result.status < 300 && Number.isFinite(version) && version > 0) {
    state.ids.storageConfigVersion = version
  }
}

function isExpectedProviderUnavailable(result) {
  return result.status === 503 && result.json?.error?.code === 'UPSTREAM_UNAVAILABLE'
}

function isExpectedSemanticNotFound(result, template) {
  if (result.status !== 404 || result.json?.error?.code !== 'NOT_FOUND') return false
  return template.includes('{image_id}') ||
    template.includes('/gallery/images') ||
    template.includes('/cluster/tokens/{token_id}') ||
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
  const textModelPath = template.includes('/text-model')
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
    .replace('{account_id}', textModelPath ? state.ids.textModelAccountId : state.ids.modelAccountId)
    .replace('{model_id}', textModelPath ? state.ids.textModelId : state.ids.accountModelId)
    .replace('{storage_config_id}', state.ids.storageConfigId)
    .replace('{token_id}', '00000000-0000-0000-0000-000000000000')
}

function defaultQuery(template) {
  if (template.includes('/estimate')) {
    return '?task_type=text_to_image&route_model_code=basic&requested_quality=auto&requested_size=1024x1024&requested_output_image_count=1&reference_image_count=0'
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
    '/api/agent/billing/v1/orders': { plan_code: 'basic-monthly', provider: 'mock' },
    '/api/agent/cashier/v1/orders': { purchase_type: 'plan', plan_code: 'basic-monthly', visible_method: 'mock' },
    '/api/agent/image/v1/reference-assets': { filename: `sweep-${RUN_ID}.png`, mime_type: 'image/png', content_base64: TINY_PNG_BASE64 },
    '/api/agent/image/v1/tasks': { task_type: 'text_to_image', prompt: 'sweep prompt', route_model_code: 'basic', requested_quality: 'auto', requested_size: '1024x1024', requested_output_image_count: 1, response_mode: 'async' },
    '/api/agent/user/v1/profile': { display_name: `E2E ${RUN_ID}` },
    '/api/agent/user/v1/account/close': { reason: 'sweep' },
    '/api/open/image/v1/payments/webhooks/{channel}': { order_no: `missing-${RUN_ID}`, trade_no: `trade-${RUN_ID}` },
    '/api/open/image/v1/reference-assets': { filename: `open-sweep-${RUN_ID}.png`, mime_type: 'image/png' },
    '/api/open/image/v1/reference-assets/uploads': { filename: `open-sweep-${RUN_ID}.png`, mime_type: 'image/png', content_base64: TINY_PNG_BASE64 },
    '/api/open/image/v1/tasks': { task_type: 'text_to_image', prompt: 'sweep open prompt', route_model_code: 'basic', requested_quality: 'auto', requested_size: '1024x1024', requested_output_image_count: 1, response_mode: 'async' },
    '/api/ops/admin/v1/auth/login': { email: E2E_ADMIN_EMAIL, password: E2E_ADMIN_PASSWORD },
    '/api/ops/admin/v1/cashier/custom-amount-config': { enabled: true, min_amount_cny: '1.00000', max_amount_cny: '500.00000', cny_per_point: '1.00000' },
    '/api/ops/admin/v1/cashier/visible-methods': { items: [mockVisibleMethod()] },
    '/api/ops/admin/v1/config-tabs/{tab_key}': { settings: {} },
    '/api/ops/admin/v1/users': { email: `sweep-user-${RUN_ID}@example.com`, nickname: `Sweep User ${RUN_ID}`, status: 'active', user_group_code: 'basic' },
    '/api/ops/admin/v1/model-providers': { provider_code: `sweep-provider-${RUN_ID}`.toLowerCase(), provider_type: 'openai', auth_config_encrypted: 'cipher', health_status: 'healthy', enabled: true },
    '/api/ops/admin/v1/model-providers/{provider_code}': { provider_code: state.ids.providerCode, provider_type: 'openai', auth_config_encrypted: 'cipher', health_status: 'healthy', enabled: true },
    '/api/ops/admin/v1/model-routes': { group_code: state.ids.groupCode, task_type: 'image_to_image', provider_code: state.ids.providerCode, priority: 5, weight_percent: 100, fallback_order: 1, enabled: true },
    '/api/ops/admin/v1/model-routes/{route_id}': { group_code: state.ids.groupCode, task_type: 'text_to_image', provider_code: state.ids.providerCode, priority: 3, weight_percent: 100, fallback_order: 1, enabled: true },
    '/api/ops/admin/v1/provider-models': { provider_code: state.ids.providerCode, model_code: `sweep-model-${RUN_ID}`, compat_mode: 'openai_images', supports_image_input: true, supports_mask: true, supported_qualities: ['1k'], supported_ratios: ['1:1'], max_image_count: 1, max_reference_image_count: 1, timeout_ms: 30000, input_cost: '0.01000', output_cost: '0.02000', currency: 'USD', health_status: 'healthy', enabled: true },
    '/api/ops/admin/v1/provider-models/{provider_model_id}': { provider_code: state.ids.providerCode, model_code: `e2e-model-${RUN_ID}`, compat_mode: 'openai_images', supports_image_input: true, supports_mask: true, supported_qualities: ['1k'], supported_ratios: ['1:1'], max_image_count: 1, max_reference_image_count: 1, timeout_ms: 30000, input_cost: '0.01000', output_cost: '0.02000', currency: 'USD', health_status: 'healthy', enabled: true },
    '/api/ops/admin/v1/model-accounts/{account_id}/models/{model_id}': { model_code: 'openrouter/imagen', display_name: 'Docker E2E Image Model', task_types: ['text_to_image'], qualities: ['1k'], supported_ratios: ['1:1'], max_image_count: 1, max_reference_image_count: 0, cost_per_image: '0.00000', currency: 'USD', enabled: true },
    '/api/ops/admin/v1/storage-configs/{storage_config_id}': {
      version: 0,
      code: state.storageConfig.code,
      name: state.storageConfig.name,
      driver: state.storageConfig.driver,
      provider: state.storageConfig.provider,
      status: state.storageConfig.status,
      read_enabled: true,
      write_enabled: true,
      prefix: state.storageConfig.prefix || '',
      public_base_url: state.storageConfig.public_base_url || '',
      local_root: state.storageConfig.local_root || '',
    },
    '/api/ops/admin/v1/storage-configs/{storage_config_id}:set-default': { version: 0 },
    '/api/ops/admin/v1/storage-configs/{storage_config_id}:set-status': { version: 0, status: 'enabled', read_enabled: true, write_enabled: true },
    '/api/ops/admin/v1/redeem-codes': { code: `SWEEP-${RUN_ID}`, status: 'available', reward_type: 'points', reward_value: '1.00000', valid_until: new Date(Date.now() + 86400000).toISOString(), max_redemptions: 1 },
    '/api/ops/admin/v1/redeem-codes/{code_id}/status': { status: 'disabled' },
    '/api/ops/admin/v1/redeem-codes:batch-create': { count: 1, status: 'available', reward_type: 'points', reward_value: '1.00000', valid_until: new Date(Date.now() + 86400000).toISOString(), max_redemptions: 1 },
    '/api/ops/admin/v1/storage-configs': { code: `sweep-local-${RUN_ID}`.toLowerCase(), name: `Sweep Local ${RUN_ID}`, driver: 'local', provider: 'local', local_root: '/var/lib/pic-gallery/storage', read_enabled: true, write_enabled: false },
    '/api/ops/admin/v1/storage-configs:probe': { name: `Sweep Probe ${RUN_ID}`, driver: 'local', provider: 'local', local_root: '/var/lib/pic-gallery/storage', read_enabled: true, write_enabled: true },
    '/api/ops/admin/v1/storage-configs/{storage_config_id}': { version: state.ids.storageConfigVersion, name: `Sweep Default ${RUN_ID}`, driver: 'local', provider: 'local', local_root: '/var/lib/pic-gallery/storage', read_enabled: true, write_enabled: true },
    '/api/ops/admin/v1/storage-configs/{storage_config_id}:set-default': { version: state.ids.storageConfigVersion },
    '/api/ops/admin/v1/storage-configs/{storage_config_id}:set-status': { version: state.ids.storageConfigVersion, status: 'enabled', read_enabled: true, write_enabled: true },
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
      if (!SKIP_MIDDLEWARE_HEALTH) {
        await waitFor(`${MINIO_URL}/minio/health/live`)
        await waitFor(`${MAILPIT_URL}/api/v1/info`)
      }
    })
    const openapi = await loadOpenAPI()
    await step('user and admin web routes return app shell', async () => {
      await checkHtmlApp('user web', USER_WEB_URL, ['landing', 'login', 'home', 'genpic', 'gallery', 'public-gallery', 'checkout', 'api-keys', 'profile', 'docs'])
      await checkHtmlApp('admin web', ADMIN_WEB_URL, ['login', 'overview', 'readiness', 'config', 'routing', 'pricing', 'reviews', 'users', 'user-groups', 'redeem', 'cashier', 'call-records', 'provider-models', 'audit', 'health'])
    })
    await step('frontend shared API client unwraps auth tokens', frontendApiClientSmoke)
    await step('CORS preflight sweep for browser origins', async () => corsSweep(openapi))
    await step('bootstrap admin session', bootstrapAdmin)
    await step('enable signup trial credits through admin config', enableSignupTrialCredits)
    await step('capture default storage config from the admin API', captureDefaultStorageConfig)
    await step('bootstrap user session', bootstrapUser)
    await step('seed user points through admin API', seedPoints)
    await step('ensure mock cashier payment method is visible', ensureMockCashierVisible)
    await step('agent billing happy path', happyPathAgentBilling)
    await step('agent API key happy path', happyPathApiKeys)
    await step('seed generation route for image task happy paths', seedGenerationRoute)
    await step('agent assets and image task happy path', happyPathAssetsAndTasks)
    await step('native Open API and OpenAI-compatible API happy path', happyPathOpenAPI)
    await step('text model accounts and prompt optimization happy path', happyPathPromptOptimization)
    await step('browser prompt optimization and configuration reuse workflow', browserPromptWorkflow)
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
  } finally {
    try {
      await stopFakeProvider()
    } catch (error) {
      console.error(`Failed to stop the Docker E2E fake provider: ${error.message}`)
      process.exitCode = 1
    }
  }
}

await main()
