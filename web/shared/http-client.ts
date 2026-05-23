import type { ApiEnvelope, ApiPagination, PageResult } from './api-types'

type QueryValue = string | number | boolean | null | undefined

export class ApiError extends Error {
  status: number
  code: string
  requestId?: string

  constructor(message: string, status: number, code = 'request_failed', requestId?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.requestId = requestId
  }
}

export type ApiClientOptions = {
  baseUrl?: string
  getToken?: () => string | null | undefined
  getAccessToken?: () => string | null | undefined
  onUnauthorized?: () => Promise<string | null | undefined> | string | null | undefined
}

export type RequestOptions = {
  method?: string
  pathParams?: Record<string, string | number>
  query?: Record<string, QueryValue>
  body?: unknown
  formData?: FormData
  headers?: HeadersInit
  auth?: boolean
  retryUnauthorized?: boolean
  unwrapEnvelope?: boolean
  credentials?: RequestCredentials
}

export function getDefaultBaseUrl() {
  const metaEnv = import.meta.env as Record<string, string | undefined>
  return (metaEnv.VITE_API_BASE_URL ?? '').replace(/\/$/, '')
}

export function fillPath(path: string, params: Record<string, string | number> = {}) {
  return Object.entries(params).reduce((next, [key, value]) => next.replace(`{${key}}`, encodeURIComponent(String(value))), path)
}

export { fillPath as path }

export function withQuery(path: string, query: Record<string, QueryValue> = {}) {
  const search = new URLSearchParams()
  Object.entries(query).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') return
    search.set(key, String(value))
  })
  const qs = search.toString()
  return qs ? `${path}${path.includes('?') ? '&' : '?'}${qs}` : path
}

function isEnvelope<T>(value: unknown): value is ApiEnvelope<T> {
  return Boolean(value && typeof value === 'object' && 'code' in value && 'data' in value)
}

async function readResponse(response: Response) {
  if (response.status === 204) return undefined
  const contentType = response.headers.get('content-type') ?? ''
  if (contentType.includes('application/json')) return response.json()
  if (contentType.startsWith('text/') || contentType.includes('yaml')) return response.text()
  return response.blob()
}

function unwrap<T>(payload: unknown): T {
  if (isEnvelope<T>(payload)) return payload.data
  return payload as T
}

function errorFromPayload(payload: unknown, status: number) {
  if (payload && typeof payload === 'object' && 'error' in payload) {
    const wrapped = payload as { error?: { message?: string; code?: string }; meta?: { request_id?: string } }
    const message = wrapped.error?.message ?? '请求失败'
    const requestId = wrapped.meta?.request_id
    return new ApiError(requestId ? `${message} (${requestId})` : message, status, wrapped.error?.code, requestId)
  }
  if (isEnvelope<unknown>(payload)) {
    const message = payload.message ?? '请求失败'
    return new ApiError(
      payload.request_id ? `${message} (${payload.request_id})` : message,
      status,
      payload.code,
      payload.request_id,
    )
  }
  if (payload && typeof payload === 'object' && 'message' in payload) {
    return new ApiError(String((payload as { message?: unknown }).message ?? '请求失败'), status)
  }
  return new ApiError(`请求失败 (${status})`, status)
}

export class ApiClient {
  private baseUrl: string
  private getToken?: ApiClientOptions['getToken']
  private onUnauthorized?: ApiClientOptions['onUnauthorized']
  private refreshPromise: Promise<string | null | undefined> | null = null

  constructor(options: ApiClientOptions = {}) {
    this.baseUrl = (options.baseUrl ?? getDefaultBaseUrl()).replace(/\/$/, '')
    this.getToken = options.getToken ?? options.getAccessToken
    this.onUnauthorized = options.onUnauthorized
  }

  setAuth(options: Pick<ApiClientOptions, 'getToken' | 'onUnauthorized'>) {
    this.getToken = options.getToken
    this.onUnauthorized = options.onUnauthorized
  }

  async request<T>(path: string, options: RequestOptions = {}): Promise<T> {
    return this.send<T>(path, options, Boolean(options.retryUnauthorized ?? true))
  }

  get<T>(path: string, options: RequestOptions = {}) {
    return this.request<T>(path, { ...options, method: 'GET' })
  }

  post<T>(path: string, body?: unknown, options: RequestOptions = {}) {
    return this.request<T>(path, { ...options, method: 'POST', body })
  }

  put<T>(path: string, body?: unknown, options: RequestOptions = {}) {
    return this.request<T>(path, { ...options, method: 'PUT', body })
  }

  patch<T>(path: string, body?: unknown, options: RequestOptions = {}) {
    return this.request<T>(path, { ...options, method: 'PATCH', body })
  }

  delete<T = void>(path: string, options: RequestOptions = {}) {
    return this.request<T>(path, { ...options, method: 'DELETE' })
  }

  postForm<T>(path: string, formData: FormData, options: RequestOptions = {}) {
    return this.request<T>(path, { ...options, method: 'POST', formData })
  }

  async blob(path: string, options: RequestOptions = {}) {
    return this.send<Blob>(path, options, Boolean(options.retryUnauthorized ?? true))
  }

  private async send<T>(path: string, options: RequestOptions, allowRefresh: boolean): Promise<T> {
    const url = `${this.baseUrl}${withQuery(fillPath(path, options.pathParams), options.query)}`
    const headers = new Headers(options.headers)
    const token = options.auth === false ? undefined : this.getToken?.()
    let body: BodyInit | undefined

    if (options.formData) {
      body = options.formData
    } else if (options.body !== undefined) {
      headers.set('Content-Type', 'application/json')
      body = JSON.stringify(options.body)
    }

    if (token) headers.set('Authorization', `Bearer ${token}`)

    const response = await fetch(url, {
      method: options.method ?? (body ? 'POST' : 'GET'),
      credentials: options.credentials ?? 'include',
      headers,
      body,
    })
    const payload = await readResponse(response)
    if (response.ok) return options.unwrapEnvelope === false ? payload as T : unwrap<T>(payload)

    if (response.status === 401 && allowRefresh && this.onUnauthorized) {
      if (!this.refreshPromise) {
        this.refreshPromise = Promise.resolve(this.onUnauthorized()).finally(() => {
          this.refreshPromise = null
        })
      }
      const nextToken = await this.refreshPromise
      if (nextToken) return this.send<T>(path, options, false)
    }

    throw errorFromPayload(payload, response.status)
  }
}

export function normalizePage<T>(payload: { items?: T[]; pagination?: ApiPagination; total?: number }): PageResult<T> {
  return {
    items: payload.items ?? [],
    total: payload.pagination?.total ?? payload.total ?? payload.items?.length ?? 0,
    pagination: payload.pagination,
  }
}

export const sharedApiClient = new ApiClient()

export type ApiHttpClient = ApiClient

export function createApiHttpClient(options: ApiClientOptions = {}) {
  return new ApiClient(options)
}

export function createMemoryTokenStore(initialToken: string | null = null) {
  let token = initialToken
  return {
    getAccessToken: () => token,
    getToken: () => token,
    setAccessToken: (nextToken: string | null | undefined) => {
      token = nextToken ?? null
    },
    clearAccessToken: () => {
      token = null
    },
  }
}

export function errorMessage(error: unknown) {
  if (error instanceof Error) return error.message
  return String(error)
}
