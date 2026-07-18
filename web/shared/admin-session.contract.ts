import { adminApi } from './admin-api'
import { createApiHttpClient } from './http-client'

type RefreshAdminSession = () => Promise<{ token: string }>

async function run() {
  const refreshSession = (adminApi as typeof adminApi & { refreshSession?: RefreshAdminSession }).refreshSession
  if (!refreshSession) {
    throw new Error('admin API must expose refreshSession')
  }

  const originalFetch = globalThis.fetch
  try {
    globalThis.fetch = async () => new Response(JSON.stringify({
      data: { name: 'pic-gallery', status: 'bootstrap-ready' },
      meta: { request_id: 'malformed-refresh' },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } })

    let malformedAccepted = false
    try {
      await refreshSession()
      malformedAccepted = true
    } catch {
      // Expected: a successful HTTP status without an access token is not a session.
    }
    if (malformedAccepted) {
      throw new Error('admin refresh must reject a malformed HTTP 200 payload')
    }

    globalThis.fetch = async () => new Response(JSON.stringify({
      data: {
        access_token: 'fresh-admin-access-token',
        expires_in_seconds: 600,
        admin_id: 7,
        email: 'admin@example.com',
        role: 'super_admin',
        permissions: ['read:all'],
      },
      meta: { request_id: 'valid-refresh' },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } })

    const refreshed = await refreshSession()
    if (refreshed.token !== 'fresh-admin-access-token') {
      throw new Error(`admin refresh should preserve the new access token, got ${JSON.stringify(refreshed)}`)
    }
  } finally {
    globalThis.fetch = originalFetch
  }

  await verifySingleflightRefresh()
  await verifyStaggeredUnauthorizedUsesFreshToken()
  await verifyRetriedUnauthorizedStops()
  await verifyStaleRefreshCannotExpireNewSession()
  await verifyLoginUnauthorizedDoesNotRefresh()
  await verifyLogoutUnauthorizedDoesNotRefresh()
  await verifyAdminSessionMutationsAreSerialized()
  await verifyHungAdminSessionMutationIsAborted()
}

async function verifySingleflightRefresh() {
  let token = 'expired-access-token'
  let refreshCalls = 0
  let requestCalls = 0
  const client = createApiHttpClient({
    baseUrl: 'http://admin.test',
    getToken: () => token,
    onUnauthorized: async () => {
      refreshCalls++
      await Promise.resolve()
      token = 'fresh-access-token'
      return token
    },
  })
  const originalFetch = globalThis.fetch
  try {
    globalThis.fetch = async (_input, init) => {
      requestCalls++
      const authorization = new Headers(init?.headers).get('Authorization')
      if (authorization === 'Bearer expired-access-token') {
        return jsonResponse({ error: { code: 'AUTH_ACCESS_EXPIRED', message: 'expired' }, meta: { request_id: 'expired' } }, 401)
      }
      return jsonResponse({ data: { ok: true }, meta: { request_id: 'success' } }, 200)
    }

    await Promise.all([client.get('/dashboard'), client.get('/config-tabs')])
    if (refreshCalls !== 1 || requestCalls !== 4) {
      throw new Error(`concurrent admin 401s should singleflight one refresh and replay once, refresh=${refreshCalls} requests=${requestCalls}`)
    }
  } finally {
    globalThis.fetch = originalFetch
  }
}

async function verifyStaggeredUnauthorizedUsesFreshToken() {
  let token = 'expired-access-token'
  let refreshCalls = 0
  let requestCalls = 0
  let signalRefreshCompleted: () => void = () => undefined
  const refreshCompleted = new Promise<void>((resolve) => {
    signalRefreshCompleted = resolve
  })
  const client = createApiHttpClient({
    baseUrl: 'http://admin.test',
    getToken: () => token,
    onUnauthorized: async () => {
      refreshCalls++
      token = 'fresh-access-token'
      signalRefreshCompleted()
      return token
    },
  })
  const originalFetch = globalThis.fetch
  try {
    globalThis.fetch = async (input, init) => {
      requestCalls++
      const authorization = new Headers(init?.headers).get('Authorization')
      const path = String(input)
      if (authorization === 'Bearer expired-access-token') {
        if (path.endsWith('/config-tabs')) {
          await refreshCompleted
          await Promise.resolve()
          await Promise.resolve()
        }
        return jsonResponse({ error: { code: 'AUTH_ACCESS_EXPIRED', message: 'expired' }, meta: { request_id: 'staggered-expired' } }, 401)
      }
      return jsonResponse({ data: { ok: true }, meta: { request_id: 'staggered-success' } }, 200)
    }

    await Promise.all([client.get('/dashboard'), client.get('/config-tabs')])
    if (refreshCalls !== 1 || requestCalls !== 4) {
      throw new Error(`staggered old-token 401s must reuse the refreshed token, refresh=${refreshCalls} requests=${requestCalls}`)
    }
  } finally {
    globalThis.fetch = originalFetch
  }
}

async function verifyRetriedUnauthorizedStops() {
  let token = 'expired-access-token'
  let refreshCalls = 0
  let requestCalls = 0
  let terminalUnauthorizedCalls = 0
  const client = createApiHttpClient({
    baseUrl: 'http://admin.test',
    getToken: () => token,
    onUnauthorized: async () => {
      refreshCalls++
      token = 'invalid-refreshed-access-token'
      return token
    },
    onError: (error) => {
      if (error.status === 401) terminalUnauthorizedCalls++
    },
  })
  const originalFetch = globalThis.fetch
  try {
    globalThis.fetch = async () => {
      requestCalls++
      return jsonResponse({ error: { code: 'AUTH_ACCESS_EXPIRED', message: 'expired' }, meta: { request_id: 'still-expired' } }, 401)
    }

    const results = await Promise.allSettled([client.get('/dashboard'), client.get('/config-tabs')])
    if (results.some((result) => result.status !== 'rejected')) {
      throw new Error(`retried unauthorized requests must reject, got ${JSON.stringify(results)}`)
    }
    if (refreshCalls !== 1 || requestCalls !== 4 || terminalUnauthorizedCalls !== 2) {
      throw new Error(`retried admin 401s must stop and report terminal unauthorized, refresh=${refreshCalls} requests=${requestCalls} terminal=${terminalUnauthorizedCalls}`)
    }
  } finally {
    globalThis.fetch = originalFetch
  }
}

async function verifyStaleRefreshCannotExpireNewSession() {
  let token = 'old-session-token'
  let sessionVersion = 1
  let releaseRefresh: () => void = () => undefined
  const refreshCanFinish = new Promise<void>((resolve) => {
    releaseRefresh = resolve
  })
  let terminalUnauthorizedCalls = 0
  const client = createApiHttpClient({
    baseUrl: 'http://admin.test',
    getToken: () => token,
    getSessionVersion: () => sessionVersion,
    onUnauthorized: async () => {
      await refreshCanFinish
      return undefined
    },
    onError: (error) => {
      if (error.status === 401) terminalUnauthorizedCalls++
    },
  })
  const originalFetch = globalThis.fetch
  try {
    globalThis.fetch = async () => jsonResponse({ error: { code: 'AUTH_ACCESS_EXPIRED', message: 'expired' } }, 401)
    const staleRequest = client.get('/dashboard')
    await Promise.resolve()
    await Promise.resolve()
    sessionVersion++
    token = 'new-login-token'
    releaseRefresh()
    const result = await Promise.allSettled([staleRequest])
    if (result[0]?.status !== 'rejected') {
      throw new Error(`request from an old admin session must reject after a new login, got ${JSON.stringify(result)}`)
    }
    if (terminalUnauthorizedCalls !== 0 || token !== 'new-login-token') {
      throw new Error(`stale refresh must not expire the new admin session, terminal=${terminalUnauthorizedCalls} token=${token}`)
    }
  } finally {
    globalThis.fetch = originalFetch
  }
}

async function verifyLoginUnauthorizedDoesNotRefresh() {
  let refreshCalls = 0
  adminApi.configureAuth({
    getToken: () => undefined,
    onUnauthorized: async () => {
      refreshCalls++
      return 'unexpected-token'
    },
    onError: () => undefined,
  })
  const originalFetch = globalThis.fetch
  try {
    globalThis.fetch = async () => jsonResponse({ error: { code: 'UNAUTHORIZED', message: 'invalid credentials' } }, 401)
    const result = await Promise.allSettled([adminApi.login('admin@example.com', 'wrong-password')])
    if (result[0]?.status !== 'rejected') {
      throw new Error(`invalid admin login must reject, got ${JSON.stringify(result)}`)
    }
    if (refreshCalls !== 0) {
      throw new Error(`invalid admin login must not trigger session refresh, refresh=${refreshCalls}`)
    }
  } finally {
    globalThis.fetch = originalFetch
  }
}

async function verifyLogoutUnauthorizedDoesNotRefresh() {
  let refreshCalls = 0
  adminApi.configureAuth({
    getToken: () => 'expired-access-token',
    onUnauthorized: async () => {
      refreshCalls++
      return 'unexpected-token'
    },
    onError: () => undefined,
  })
  const originalFetch = globalThis.fetch
  try {
    globalThis.fetch = async () => jsonResponse({ error: { code: 'AUTH_ACCESS_EXPIRED', message: 'expired' } }, 401)
    const result = await Promise.allSettled([adminApi.logout()])
    if (result[0]?.status !== 'rejected') {
      throw new Error(`unauthorized admin logout must settle without restoring a session, got ${JSON.stringify(result)}`)
    }
    if (refreshCalls !== 0) {
      throw new Error(`unauthorized admin logout must not trigger session refresh, refresh=${refreshCalls}`)
    }
  } finally {
    globalThis.fetch = originalFetch
  }
}

async function verifyAdminSessionMutationsAreSerialized() {
  const calls: string[] = []
  let releaseRefresh: () => void = () => undefined
  const refreshCanFinish = new Promise<void>((resolve) => {
    releaseRefresh = resolve
  })
  const originalFetch = globalThis.fetch
  try {
    globalThis.fetch = async (input) => {
      const path = String(input)
      if (path.endsWith('/auth/session/refresh')) {
        calls.push('refresh')
        await refreshCanFinish
        return validAdminSessionResponse('refresh-token-result')
      }
      if (path.endsWith('/auth/logout')) {
        calls.push('logout')
        return new Response(null, { status: 204 })
      }
      if (path.endsWith('/auth/login')) {
        calls.push('login')
        return validAdminSessionResponse('login-token-result')
      }
      throw new Error(`unexpected admin auth path ${path}`)
    }

    const refresh = adminApi.refreshSession()
    const logout = adminApi.logout()
    const login = adminApi.login('admin@example.com', 'new-password')
    await Promise.resolve()
    await Promise.resolve()
    if (calls.join(',') !== 'refresh') {
      throw new Error(`admin session mutations must wait for the in-flight refresh, calls=${calls.join(',')}`)
    }
    releaseRefresh()
    await Promise.all([refresh, logout, login])
    if (calls.join(',') !== 'refresh,logout,login') {
      throw new Error(`admin session mutation order must be refresh,logout,login; got ${calls.join(',')}`)
    }
  } finally {
    globalThis.fetch = originalFetch
  }
}

async function verifyHungAdminSessionMutationIsAborted() {
  let releaseFetch: () => void = () => undefined
  const fetchCanFinish = new Promise<void>((resolve) => {
    releaseFetch = resolve
  })
  const originalFetch = globalThis.fetch
  const originalSetTimeout = globalThis.setTimeout
  try {
    globalThis.fetch = async (_input, init) => {
      await new Promise<void>((resolve, reject) => {
        const signal = init?.signal
        const abort = () => reject(signal?.reason ?? new DOMException('aborted', 'AbortError'))
        if (signal?.aborted) {
          abort()
          return
        }
        signal?.addEventListener('abort', abort, { once: true })
        void fetchCanFinish.then(resolve)
      })
      return validAdminSessionResponse('unexpected-timeout-result')
    }
    globalThis.setTimeout = ((handler: TimerHandler, _timeout?: number, ...args: unknown[]) => {
      queueMicrotask(() => {
        if (typeof handler === 'function') handler(...args)
      })
      return 1
    }) as typeof globalThis.setTimeout

    const mutation = adminApi.refreshSession()
    const outcome = await Promise.race([
      mutation.then(() => 'resolved', () => 'rejected'),
      new Promise<'pending'>((resolve) => originalSetTimeout(() => resolve('pending'), 100)),
    ])
    releaseFetch()
    await mutation.catch(() => undefined)
    if (outcome !== 'rejected') {
      throw new Error(`hung admin session mutation must be aborted, outcome=${outcome}`)
    }
  } finally {
    releaseFetch()
    globalThis.fetch = originalFetch
    globalThis.setTimeout = originalSetTimeout
  }
}

function validAdminSessionResponse(accessToken: string) {
  return jsonResponse({
    data: {
      access_token: accessToken,
      expires_in_seconds: 600,
      admin_id: 7,
      email: 'admin@example.com',
      role: 'super_admin',
      permissions: ['read:all'],
    },
    meta: { request_id: `${accessToken}-request` },
  }, 200)
}

function jsonResponse(payload: unknown, status: number) {
  return new Response(JSON.stringify(payload), { status, headers: { 'Content-Type': 'application/json' } })
}

run().catch((error) => { throw error })
