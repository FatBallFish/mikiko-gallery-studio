import { createApiHttpClient } from './http-client'

const contractTimeout = setTimeout(() => {
  throw new Error('http-client auth contract timed out')
}, 5_000)
void verifyUnauthenticatedUnauthorizedIsTerminal().then(
  () => clearTimeout(contractTimeout),
  (error) => {
    clearTimeout(contractTimeout)
    queueMicrotask(() => { throw error })
  },
)

async function verifyUnauthenticatedUnauthorizedIsTerminal() {
  let sessionToken: string | undefined
  let sessionVersion = 7
  let refreshCalls = 0
  let onErrorCalls = 0
  let fetchCalls = 0
  const client = createApiHttpClient({
    baseUrl: 'http://open.test',
    getToken: () => sessionToken,
    getSessionVersion: () => sessionVersion,
    onUnauthorized: async () => {
      refreshCalls++
      sessionToken = 'unexpected-user-token'
      sessionVersion++
      return sessionToken
    },
    onError: (error) => {
      onErrorCalls++
      if (error.status === 401) {
        sessionToken = undefined
        sessionVersion++
      }
    },
  })
  const originalFetch = globalThis.fetch
  try {
    globalThis.fetch = async () => {
      fetchCalls++
      return new Response(JSON.stringify({ error: { code: 'UNAUTHORIZED', message: 'invalid Open API signature' } }), {
        status: 401,
        headers: { 'Content-Type': 'application/json' },
      })
    }
    const result = await Promise.allSettled([client.get('/api/open/image/v1/balance', { auth: false })])
    if (result[0]?.status !== 'rejected') throw new Error(`auth:false 401 should reject, got ${JSON.stringify(result)}`)
  } finally {
    globalThis.fetch = originalFetch
  }

  if (refreshCalls !== 0 || onErrorCalls !== 0 || fetchCalls !== 1 || sessionToken !== undefined || sessionVersion !== 7) {
    throw new Error(`auth:false 401 must not enter global session handlers, replay, or mutate the user session, refresh=${refreshCalls} onError=${onErrorCalls} fetch=${fetchCalls} token=${sessionToken} version=${sessionVersion}`)
  }
}
