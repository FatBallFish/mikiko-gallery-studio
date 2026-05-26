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
  onError?: (error: ApiError) => void
}

type ErrorLocale = 'zh' | 'en'

const errorMessages: Record<string, Record<ErrorLocale, string>> = {
  INTERNAL_ERROR: {
    zh: '服务暂时开小差了，请稍后再试。',
    en: 'The service is temporarily unavailable. Please try again later.',
  },
  BAD_REQUEST: {
    zh: '请求参数有误，请检查后再试。',
    en: 'Some request parameters are invalid. Please check and try again.',
  },
  METHOD_NOT_ALLOWED: {
    zh: '当前操作方式不受支持，请刷新页面后重试。',
    en: 'This operation is not supported. Please refresh and try again.',
  },
  UNAUTHORIZED: {
    zh: '登录状态无效，请重新登录。',
    en: 'Your session is invalid. Please sign in again.',
  },
  FORBIDDEN: {
    zh: '你没有权限执行该操作。',
    en: 'You do not have permission to perform this action.',
  },
  NOT_FOUND: {
    zh: '请求的资源不存在或已不可用。',
    en: 'The requested resource does not exist or is no longer available.',
  },
  CONFLICT: {
    zh: '当前数据状态已变化，请刷新后再试。',
    en: 'The data has changed. Please refresh and try again.',
  },
  RATE_LIMITED: {
    zh: '操作太频繁了，请稍后再试。',
    en: 'Too many requests. Please wait a moment and try again.',
  },
  VALIDATION_FAILED: {
    zh: '提交内容未通过校验，请检查后再试。',
    en: 'The submitted content did not pass validation. Please check and try again.',
  },
  AUTH_ACCESS_EXPIRED: {
    zh: '登录已过期，请重新登录。',
    en: 'Your session has expired. Please sign in again.',
  },
  AUTH_REFRESH_EXPIRED: {
    zh: '登录已过期，请重新登录。',
    en: 'Your session has expired. Please sign in again.',
  },
  AUTH_REFRESH_REPLAY_BLOCKED: {
    zh: '登录状态存在风险，请重新登录。',
    en: 'Your session needs to be verified again. Please sign in again.',
  },
  USER_DISABLED: {
    zh: '账号已被禁用，如有疑问请联系管理员。',
    en: 'This account has been disabled. Please contact an administrator.',
  },
  API_KEY_DISABLED: {
    zh: 'API 密钥已停用，请更换或重新启用密钥。',
    en: 'This API key is disabled. Please use another key or re-enable it.',
  },
  BILLING_INSUFFICIENT_POINTS: {
    zh: '积分余额不足，请充值或降低生成规格后重试。',
    en: 'Insufficient points. Please add points or lower the generation settings.',
  },
  IMAGE_CAPABILITY_MISMATCH: {
    zh: '当前模型不支持所选参数，请调整模型、清晰度或图片数量。',
    en: 'The selected model does not support these settings. Please adjust the model, quality, or image count.',
  },
  IMAGE_REFERENCE_REQUIRED: {
    zh: '请先上传参考图后再发起生成。',
    en: 'Please upload a reference image before starting generation.',
  },
  IMAGE_REFERENCE_COUNT_EXCEEDED: {
    zh: '参考图数量超过当前模型上限，请减少后重试。',
    en: 'Too many reference images for the selected model. Please remove some and try again.',
  },
  IMAGE_AUTO_RESOLUTION_UNSUPPORTED: {
    zh: '当前模型不支持所选尺寸或自动清晰度，请调整后重试。',
    en: 'The selected model does not support this size or automatic quality. Please adjust and try again.',
  },
  IMAGE_TASK_FAILED: {
    zh: '图片生成失败，请调整提示词或稍后重试。',
    en: 'Image generation failed. Please adjust the prompt or try again later.',
  },
  IMAGE_STORAGE_FAILED: {
    zh: '图片保存失败，请稍后重试。',
    en: 'Failed to save the image. Please try again later.',
  },
  UPSTREAM_UNAVAILABLE: {
    zh: '上游模型服务暂不可用，请稍后重试。',
    en: 'The upstream model service is temporarily unavailable. Please try again later.',
  },
  UPSTREAM_BAD_REQUEST: {
    zh: '上游模型拒绝了本次请求，请调整参数或提示词后重试。',
    en: 'The upstream model rejected this request. Please adjust the settings or prompt and try again.',
  },
  UPSTREAM_CONTENT_BLOCKED: {
    zh: '内容未通过安全检查，请调整提示词或参考图后重试。',
    en: 'The content did not pass safety checks. Please adjust the prompt or reference image.',
  },
  insufficient_balance: {
    zh: '积分余额不足，请充值或降低生成规格后重试。',
    en: 'Insufficient points. Please add points or lower the generation settings.',
  },
  invalid_signature: {
    zh: 'API 签名校验失败，请检查密钥、时间戳和签名算法。',
    en: 'API signature verification failed. Please check the key, timestamp, and signature.',
  },
  provider_unavailable: {
    zh: '模型服务暂不可用，请稍后重试。',
    en: 'The model service is temporarily unavailable. Please try again later.',
  },
  rate_limit_exceeded: {
    zh: '请求频率过高，请稍后再试。',
    en: 'Rate limit exceeded. Please try again later.',
  },
}

const statusMessages: Record<number, Record<ErrorLocale, string>> = {
  400: errorMessages.BAD_REQUEST,
  401: errorMessages.UNAUTHORIZED,
  403: errorMessages.FORBIDDEN,
  404: errorMessages.NOT_FOUND,
  409: errorMessages.CONFLICT,
  413: {
    zh: '上传内容过大，请压缩或减少文件后重试。',
    en: 'The upload is too large. Please reduce the file size and try again.',
  },
  429: errorMessages.RATE_LIMITED,
  500: errorMessages.INTERNAL_ERROR,
  502: errorMessages.UPSTREAM_UNAVAILABLE,
  503: errorMessages.UPSTREAM_UNAVAILABLE,
  504: errorMessages.UPSTREAM_UNAVAILABLE,
}

function currentErrorLocale(): ErrorLocale {
  const language = globalThis.navigator?.language?.toLowerCase() ?? ''
  return language.startsWith('zh') ? 'zh' : 'en'
}

function networkErrorMessage(locale: ErrorLocale) {
  return locale === 'zh'
    ? '网络连接异常，请检查网络后重试。'
    : 'Network connection failed. Please check your connection and try again.'
}

function fallbackErrorMessage(locale: ErrorLocale) {
  return locale === 'zh'
    ? '请求失败，请稍后重试。'
    : 'Request failed. Please try again later.'
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
  return Boolean(value && typeof value === 'object' && 'data' in value && ('meta' in value || 'code' in value || 'message' in value || 'request_id' in value))
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
  private onError?: ApiClientOptions['onError']
  private refreshPromise: Promise<string | null | undefined> | null = null

  constructor(options: ApiClientOptions = {}) {
    this.baseUrl = (options.baseUrl ?? getDefaultBaseUrl()).replace(/\/$/, '')
    this.getToken = options.getToken ?? options.getAccessToken
    this.onUnauthorized = options.onUnauthorized
    this.onError = options.onError
  }

  setAuth(options: Pick<ApiClientOptions, 'getToken' | 'onUnauthorized' | 'onError'>) {
    this.getToken = options.getToken
    this.onUnauthorized = options.onUnauthorized
    this.onError = options.onError
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

    const error = errorFromPayload(payload, response.status)
    this.onError?.(error)
    throw error
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
  const locale = currentErrorLocale()
  if (error instanceof ApiError) {
    const byCode = errorMessages[error.code]
    if (byCode) return byCode[locale]
    const byStatus = statusMessages[error.status]
    if (byStatus) return byStatus[locale]
    return fallbackErrorMessage(locale)
  }
  if (error instanceof TypeError) return networkErrorMessage(locale)
  return fallbackErrorMessage(locale)
}
