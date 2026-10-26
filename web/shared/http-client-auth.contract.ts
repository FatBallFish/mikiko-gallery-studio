import { createApiHttpClient, sharedApiClient } from './http-client'
import { userApi } from './user-api'

const contractTimeout = setTimeout(() => {
  throw new Error('http-client auth contract timed out')
}, 5_000)
void verifyUnauthenticatedUnauthorizedIsTerminal().then(verifyUploadRefresh).then(
  () => clearTimeout(contractTimeout),
  (error) => {
    clearTimeout(contractTimeout)
    queueMicrotask(() => { throw error })
  },
)

async function verifyUploadRefresh() {
  let token = 'expired-token'
  let refreshCalls = 0
  let releaseRefresh!: () => void
  const refreshing = new Promise<void>((resolve) => { releaseRefresh = resolve })
  const chunk = new Blob(['binary part'])
  const authorizations: string[] = []
  sharedApiClient.setAuth({ getToken: () => token, onUnauthorized: async () => {
    refreshCalls++
    await refreshing
    token = 'renewed-token'
    return token
  } })
  const originalFetch = globalThis.fetch
  try {
    globalThis.fetch = async (_url, options) => {
      const auth = new Headers(options?.headers).get('Authorization') ?? ''
      authorizations.push(auth)
      if (options?.method === 'PUT' && options.body !== chunk) throw new Error('parts must replay the exact binary Blob')
      return new Response(JSON.stringify(auth === 'Bearer renewed-token' ? { data: { part_number: 1 }, code: 'OK' } : { error: { message: 'expired' } }), { status: auth === 'Bearer renewed-token' ? 200 : 401, headers: { 'Content-Type': 'application/json' } })
    }
    const parts = [1, 2, 3].map((part) => userApi.uploadMediaProxyPart('session', part, chunk, 'checksum', 'stale-closure-token'))
    const init = userApi.initMediaUpload({ project_id: 'project', filename: 'a.png', mime_type: 'image/png', media_type: 'image', size_bytes: 1 }, 'same-key')
    // Wait for 401 handlers to reach the deliberately deferred shared refresh.
    for (let i = 0; i < 20 && refreshCalls === 0; i++) await new Promise((resolve) => setTimeout(resolve, 0))
    releaseRefresh()
    await Promise.all([...parts, init])
    await userApi.uploadMediaProxyPart('session', 4, chunk, 'checksum', 'stale-closure-token')
    if (refreshCalls !== 1 || authorizations.includes('Bearer stale-closure-token') || authorizations[authorizations.length - 1] !== 'Bearer renewed-token') throw new Error('init and concurrent parts must share one refresh and read the latest token')
  } finally {
    globalThis.fetch = originalFetch
    sharedApiClient.setAuth({})
  }
}

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
