import { ApiClient, ApiError } from './http-client'

async function verifyOAuthErrors() {
  const originalFetch = globalThis.fetch
  const client = new ApiClient({ baseUrl: 'https://studio.example.test' })
  try {
    for (const [status, code] of [[401, 'invalid_session'], [400, 'authorization_request_expired'], [503, 'temporarily_unavailable']] as const) {
      globalThis.fetch = async () => new Response(JSON.stringify({ error: code, error_description: 'OAuth request failed' }), { status, headers: { 'Content-Type': 'application/json' } })
      try {
        await client.request('/api/v1/oauth/consent', { auth: false, retryUnauthorized: false })
        throw new Error('OAuth error response unexpectedly succeeded')
      } catch (error) {
        if (!(error instanceof ApiError) || error.code !== code || error.status !== status || error.message !== 'OAuth request failed') throw error
      }
    }
    globalThis.fetch = async () => new Response(JSON.stringify({ error: { code: 'CANONICAL_URL_UNCONFIGURED', message: 'not configured' }, meta: { request_id: 'test-request' } }), { status: 503, headers: { 'Content-Type': 'application/json' } })
    try {
      await client.request('/mcp', { auth: false, retryUnauthorized: false })
      throw new Error('Wrapped error response unexpectedly succeeded')
    } catch (error) {
      if (!(error instanceof ApiError) || error.code !== 'CANONICAL_URL_UNCONFIGURED' || error.requestId !== 'test-request') throw error
    }
  } finally {
    globalThis.fetch = originalFetch
  }
}
void verifyOAuthErrors().catch((error) => { queueMicrotask(() => { throw error }) })
