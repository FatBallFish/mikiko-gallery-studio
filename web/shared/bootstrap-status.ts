import type { ApiEnvelope, BootstrapPhase, BootstrapStatus } from './api-types'

const SETUP_PHASES: ReadonlySet<BootstrapPhase> = new Set(['setup_required', 'initializing', 'restart_pending'])
const BOOTSTRAP_STATUS_PATH = '/api/system/v1/bootstrap-status'

export class BootstrapStatusError extends Error {
  constructor(message: string, cause?: unknown) {
    super(message)
    this.name = 'BootstrapStatusError'
    if (cause !== undefined) (this as Error & { cause?: unknown }).cause = cause
  }
}

type NormalizeOptions = {
  apiBaseUrl: string
  frontendOrigin: string
}

type FetchBootstrapStatusOptions = NormalizeOptions & {
  fetchImpl?: typeof fetch
  timeoutMs?: number
  signal?: AbortSignal
  credentials?: 'omit'
}

function parseHTTPURL(value: string, base?: string) {
  if (value.startsWith('//')) throw new BootstrapStatusError('Bootstrap URL must not be protocol-relative')
  let parsed: URL
  try {
    parsed = base ? new URL(value, base) : new URL(value)
  } catch (error) {
    throw new BootstrapStatusError('Bootstrap URL is invalid', error)
  }
  if ((parsed.protocol !== 'http:' && parsed.protocol !== 'https:') || parsed.username || parsed.password) {
    throw new BootstrapStatusError('Bootstrap URL must use HTTP or HTTPS without credentials')
  }
  return parsed
}

function resolveAPIBaseURL(apiBaseUrl: string, frontendOrigin: string) {
  const frontendURL = parseHTTPURL(frontendOrigin)
  const normalizedBase = apiBaseUrl.trim()
  if (!normalizedBase) return new URL(frontendURL.origin)
  return parseHTTPURL(normalizedBase, `${frontendURL.origin}/`)
}

export function resolveSetupURL(setupURL: string, apiBaseUrl: string, frontendOrigin: string) {
  const value = setupURL.trim()
  if (!value) throw new BootstrapStatusError('Bootstrap setup URL is missing')
  if (value.startsWith('//')) throw new BootstrapStatusError('Bootstrap setup URL must not be protocol-relative')

  if (/^[A-Za-z][A-Za-z\d+.-]*:/.test(value)) return parseHTTPURL(value).href

  const apiOrigin = resolveAPIBaseURL(apiBaseUrl, frontendOrigin).origin
  const resolved = parseHTTPURL(value, `${apiOrigin}/`)
  if (resolved.origin !== apiOrigin) {
    throw new BootstrapStatusError('Relative bootstrap setup URL escaped the configured API origin')
  }
  return resolved.href
}

export function setupURLWithReturnTarget(setupURL: string, currentURL: string) {
  const setup = parseHTTPURL(setupURL)
  const returnTarget = parseHTTPURL(currentURL)
  setup.hash = `return_to=${encodeURIComponent(returnTarget.href)}`
  return setup.href
}

function optionalString(value: unknown) {
  return typeof value === 'string' && value.trim() ? value : undefined
}

function optionalRetrySeconds(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? Math.floor(value) : undefined
}

export function normalizeBootstrapStatus(payload: unknown, options: NormalizeOptions): BootstrapStatus {
  if (!payload || typeof payload !== 'object') throw new BootstrapStatusError('Bootstrap status response is malformed')
  const raw = payload as Record<string, unknown>
  const phase = raw.phase

  if (phase === 'ready') return { phase }
  if (phase === 'broken') {
    return { phase, diagnostic_code: optionalString(raw.diagnostic_code) }
  }
  if (typeof phase === 'string' && SETUP_PHASES.has(phase as BootstrapPhase)) {
    if (typeof raw.setup_url !== 'string') throw new BootstrapStatusError('Bootstrap setup URL is missing')
    return {
      phase: phase as 'setup_required' | 'initializing' | 'restart_pending',
      setup_url: resolveSetupURL(raw.setup_url, options.apiBaseUrl, options.frontendOrigin),
      operation_id: optionalString(raw.operation_id),
      retry_after_seconds: optionalRetrySeconds(raw.retry_after_seconds),
    }
  }

  throw new BootstrapStatusError('Bootstrap status phase is invalid')
}

function bootstrapEndpoint(apiBaseUrl: string, frontendOrigin: string) {
  const base = resolveAPIBaseURL(apiBaseUrl, frontendOrigin)
  return `${base.href.replace(/\/+$/, '')}${BOOTSTRAP_STATUS_PATH}`
}

export async function fetchBootstrapStatus(options: FetchBootstrapStatusOptions): Promise<BootstrapStatus> {
  const fetchImpl = options.fetchImpl ?? fetch
  const controller = new AbortController()
  const abort = () => controller.abort(options.signal?.reason)
  if (options.signal?.aborted) abort()
  else options.signal?.addEventListener('abort', abort, { once: true })
  const timeout = globalThis.setTimeout(() => controller.abort(), options.timeoutMs ?? 8000)

  try {
    const response = await fetchImpl(bootstrapEndpoint(options.apiBaseUrl, options.frontendOrigin), {
      method: 'GET',
      credentials: options.credentials ?? 'omit',
      cache: 'no-store',
      headers: { Accept: 'application/json' },
      signal: controller.signal,
    })
    if (!response.ok) throw new BootstrapStatusError(`Bootstrap status request failed (${response.status})`)

    let payload: unknown
    try {
      payload = await response.json()
    } catch (error) {
      throw new BootstrapStatusError('Bootstrap status response is not valid JSON', error)
    }
    const envelope = payload as Partial<ApiEnvelope<unknown>> | null
    return normalizeBootstrapStatus(envelope && typeof envelope === 'object' && 'data' in envelope ? envelope.data : payload, options)
  } finally {
    globalThis.clearTimeout(timeout)
    options.signal?.removeEventListener('abort', abort)
  }
}
