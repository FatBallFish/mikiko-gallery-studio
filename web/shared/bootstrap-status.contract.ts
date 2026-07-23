// @ts-ignore contract scripts run directly in Node without project Node typings.
import { readFileSync } from 'node:fs'
import {
  BootstrapStatusError,
  fetchBootstrapStatus,
  normalizeBootstrapStatus,
  resolveSetupURL,
  setupURLWithReturnTarget,
} from './bootstrap-status.ts'

const apiTypes = readFileSync(new URL('./api-types.ts', import.meta.url), 'utf8')
if (!apiTypes.includes("bootstrapStatus: '/api/system/v1/bootstrap-status'")) {
  throw new Error('API paths must expose the bootstrap status endpoint')
}

const absolute = 'https://api.example.test/setup'
if (resolveSetupURL(absolute, 'https://ignored.example.test', 'https://user.example.test') !== absolute) {
  throw new Error('absolute backend setup URLs must be preserved')
}
if (resolveSetupURL('/setup', 'https://api.example.test/base', 'https://admin.example.test') !== 'https://api.example.test/setup') {
  throw new Error('relative setup URLs must resolve against the configured API origin')
}
if (resolveSetupURL('setup', '/gateway', 'https://user.example.test/app') !== 'https://user.example.test/setup') {
  throw new Error('same-origin relative API bases must resolve setup against the gateway origin')
}
const setupWithReturn = setupURLWithReturnTarget('https://api.example.test/setup', 'https://user.example.test/#/workspace')
if (setupWithReturn !== 'https://api.example.test/setup#return_to=https%3A%2F%2Fuser.example.test%2F%23%2Fworkspace') {
  throw new Error(`setup return target was not encoded into the URL fragment: ${setupWithReturn}`)
}
for (const unsafeReturn of ['javascript:alert(1)', 'https://user:pass@example.test/workspace']) {
  try {
    setupURLWithReturnTarget('https://api.example.test/setup', unsafeReturn)
    throw new Error(`unsafe setup return target was accepted: ${unsafeReturn}`)
  } catch (error) {
    if (!(error instanceof BootstrapStatusError)) throw error
  }
}
for (const unsafe of ['javascript:alert(1)', 'data:text/html,x', 'ftp://api.example.test/setup', '//evil.example/setup', '\\\\evil.example/setup', 'https://user:pass@api.example.test/setup']) {
  try {
    resolveSetupURL(unsafe, 'https://api.example.test', 'https://user.example.test')
    throw new Error(`unsafe setup URL was accepted: ${unsafe}`)
  } catch (error) {
    if (!(error instanceof BootstrapStatusError)) throw error
  }
}

for (const phase of ['setup_required', 'initializing', 'restart_pending'] as const) {
  const status = normalizeBootstrapStatus({ phase, setup_url: '/setup', retry_after_seconds: 2 }, {
    apiBaseUrl: 'https://api.example.test',
    frontendOrigin: 'https://admin.example.test',
  })
  if (status.phase !== phase || status.setup_url !== 'https://api.example.test/setup') {
    throw new Error(`setup phase ${phase} was not normalized for API-hosted navigation`)
  }
}
const ready = normalizeBootstrapStatus({ phase: 'ready', setup_url: 'javascript:ignored' }, {
  apiBaseUrl: 'https://api.example.test', frontendOrigin: 'https://user.example.test',
})
if (ready.phase !== 'ready' || ready.setup_url !== undefined) throw new Error('ready must not retain a setup URL')
const broken = normalizeBootstrapStatus({ phase: 'broken', diagnostic_code: 'RUNTIME_ENV_INVALID' }, {
  apiBaseUrl: 'https://api.example.test', frontendOrigin: 'https://admin.example.test',
})
if (broken.phase !== 'broken' || broken.diagnostic_code !== 'RUNTIME_ENV_INVALID') throw new Error('broken diagnostics were not preserved')
for (const malformed of [null, {}, { phase: 'unknown' }, { phase: 'setup_required' }]) {
  try {
    normalizeBootstrapStatus(malformed, { apiBaseUrl: 'https://api.example.test', frontendOrigin: 'https://user.example.test' })
    throw new Error(`malformed status was accepted: ${JSON.stringify(malformed)}`)
  } catch (error) {
    if (!(error instanceof BootstrapStatusError)) throw error
  }
}

let requestURL = ''
let requestInit: RequestInit | undefined
void fetchBootstrapStatus({
  apiBaseUrl: 'https://api.example.test/root/',
  frontendOrigin: 'https://admin.example.test',
  fetchImpl: async (input, init) => {
    requestURL = String(input)
    requestInit = init
    return new Response(JSON.stringify({ data: { phase: 'ready' } }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  },
}).then((fetched) => {
  if (fetched.phase !== 'ready' || requestURL !== 'https://api.example.test/root/api/system/v1/bootstrap-status') {
    throw new Error(`bootstrap request used the wrong API base: ${requestURL}`)
  }
  if (requestInit?.method !== 'GET' || requestInit.credentials !== 'omit' || requestInit.cache !== 'no-store') {
    throw new Error(`bootstrap request must be credential-free and no-store: ${JSON.stringify(requestInit)}`)
  }
})
