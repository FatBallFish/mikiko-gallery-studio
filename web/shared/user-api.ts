import type {
  ApiKey,
  Balance,
  BillingPlan,
  CashierOptions,
  CashierOrder,
  Capability,
  CreateApiKeyRequest,
  CreateCashierOrderRequest,
  CreateTaskRequest,
  EstimateRequest,
  EstimateResult,
  GalleryImage,
  ImageAccessURL,
  ImageResult,
  ImageTask,
  LedgerEntry,
  LoginResult,
  PageResult,
  PaymentOrder,
  ReferenceAsset,
  Subscription,
  UpdatePreferencesRequest,
  UserProfile,
} from './api-types'
import { API_PATHS } from './api-types'
import { fillPath, getDefaultBaseUrl, normalizePage, sharedApiClient, withQuery } from './http-client'
import { calculateImageSizeForQuality } from './image-size'

function initials(input: string) {
  return input.trim().slice(0, 2).toUpperCase() || 'PG'
}

export function toUserProfile(raw: any): UserProfile {
  const name = raw.nickname || raw.display_name || raw.email?.split('@')[0] || 'Mikiko User'
  return {
    ...raw,
    id: String(raw.id ?? raw.user_id ?? ''),
    email: raw.email ?? '',
    display_name: name,
    avatar_initials: initials(name),
    tier: raw.tier ?? 'FREE',
    group: raw.user_group_code ?? raw.group ?? 'DEFAULT',
    signature: raw.bio ?? raw.signature ?? '',
    preferences: {
      model_group: raw.preferences?.model_group ?? 'plus-image',
      quality: raw.preferences?.quality ?? 'auto',
      aspect_ratio: raw.preferences?.aspect_ratio ?? '16:9',
      image_count: raw.preferences?.image_count ?? 1,
      theme_mode: raw.preferences?.theme_mode,
      accent_theme: raw.preferences?.accent_theme,
      default_locale: raw.preferences?.default_locale ?? raw.default_locale,
    },
  }
}

export function toBalance(raw: any): Balance {
  return {
    ...raw,
    available_points: raw.available_points ?? '0.00000',
    frozen_points: raw.frozen_points ?? '0.00000',
    plan_name: raw.active_subscription?.plan_name ?? raw.plan_name ?? 'FREE',
    first_purchase_bonus: Boolean(raw.first_purchase_bonus ?? !raw.active_subscription),
  }
}

function normalizeTaskType(type: string): ImageTask['task_type'] {
  if (type === 'image_edit') return 'image_edit'
  if (type === 'reference_to_image' || type === 'reference_generate' || type === 'image_to_image') return 'reference_to_image'
  return 'text_to_image'
}

function toBackendTaskType(type: ImageTask['task_type'] | string) {
  return type === 'reference_to_image' ? 'reference_generate' : type
}

function pick<T = unknown>(source: any, ...keys: string[]): T | undefined {
  for (const key of keys) {
    const value = source?.[key]
    if (value !== undefined && value !== null) return value as T
  }
  return undefined
}

export function toReferenceAsset(raw: any): ReferenceAsset {
  return {
    ...raw,
    id: String(raw.id ?? raw.asset_id ?? ''),
    name: raw.name ?? raw.filename ?? raw.id ?? 'reference',
    preview_url: raw.preview_url ?? raw.download_url ?? '',
    download_url: raw.download_url ?? raw.preview_url ?? '',
    status: raw.status ?? 'ready',
    size_bytes: Number(raw.size_bytes ?? raw.file_size ?? 0),
    created_at: raw.created_at ?? '',
  }
}

export function toImageResult(raw: any): ImageResult {
  return {
    ...raw,
    id: String(raw.id ?? raw.image_id ?? ''),
    url: raw.url ?? raw.asset_url ?? raw.download_url ?? '',
    asset_url: raw.asset_url,
    expires_at: raw.expires_at,
    delivery_mode: raw.delivery_mode,
    width: Number(raw.width ?? 0),
    height: Number(raw.height ?? 0),
    publish_status: raw.publish_status ?? raw.visibility_status ?? 'private',
    like_count: Number(raw.like_count ?? 0),
    favorite_count: Number(raw.favorite_count ?? 0),
    liked_by_viewer: Boolean(raw.liked_by_viewer),
    favorited_by_viewer: Boolean(raw.favorited_by_viewer),
  }
}

export function toTask(raw: any): ImageTask {
  const results = (raw.results ?? raw.images ?? raw.image_results ?? []).map(toImageResult)
  const taskType = normalizeTaskType(raw.task_type ?? 'text_to_image')
  const quality = raw.quality ?? raw.requested_quality ?? raw.resolved_quality_bucket ?? 'auto'
  return {
    ...raw,
    id: String(raw.id ?? ''),
    title: raw.title ?? String(raw.prompt ?? 'Untitled generation').slice(0, 54),
    prompt: raw.prompt ?? '',
    task_type: taskType,
    status: raw.status ?? 'queued',
    progress_stage: raw.progress_stage ?? raw.progressStage ?? '',
    progress_message: raw.progress_message ?? raw.progressMessage ?? '',
    route_model_code: raw.route_model_code ?? raw.model_group ?? raw.abstract_model ?? raw.group_code ?? 'basic',
    route_model_name: raw.route_model_name,
    model_group: raw.route_model_code ?? raw.model_group ?? raw.abstract_model ?? raw.group_code ?? 'basic',
    quality,
    aspect_ratio: raw.aspect_ratio ?? raw.requested_size ?? '1:1',
    image_count: Number(raw.image_count ?? raw.requested_output_image_count ?? results.length ?? 1),
    estimate_points: raw.estimate_points ?? raw.estimated_points ?? raw.actual_points ?? '0.00000',
    progress: Number(raw.progress ?? (raw.status === 'succeeded' || raw.status === 'partial_failed' ? 100 : 0)),
    provider: raw.provider ?? raw.provider_code ?? '',
    route: raw.route ?? raw.route_policy ?? '',
    created_at: raw.created_at ?? '',
    updated_at: raw.updated_at ?? raw.created_at ?? '',
    failure_reason: raw.failure_reason ?? raw.error_message,
    error_code: raw.error_code,
    error_message: raw.error_message,
    request_id: raw.request_id ?? raw.meta?.request_id,
    reference_assets: (raw.reference_assets ?? []).map(toReferenceAsset),
    results,
  }
}

function toEstimateQuery(req: EstimateRequest) {
  return {
    task_type: toBackendTaskType(req.task_type),
    route_model_code: req.route_model_code,
    requested_quality: req.quality,
    requested_size: calculateImageSizeForQuality(req.quality, req.aspect_ratio),
    requested_output_image_count: req.image_count,
    reference_image_count: req.reference_asset_ids?.length ?? 0,
  }
}

function toBackendTask(req: CreateTaskRequest) {
  return {
    task_type: toBackendTaskType(req.task_type),
    prompt: req.negative_prompt ? `${req.prompt}\n\nNegative prompt: ${req.negative_prompt}` : req.prompt,
    route_model_code: req.route_model_code,
    requested_quality: req.quality,
    requested_size: calculateImageSizeForQuality(req.quality, req.aspect_ratio),
    requested_output_image_count: req.image_count,
    reference_asset_ids: req.reference_asset_ids ?? [],
    response_mode: req.response_mode ?? 'async',
    idempotency_key: req.idempotency_key,
  }
}

function toEstimate(raw: any, req?: EstimateRequest): EstimateResult {
  const points = raw.display_points ?? raw.charged_points ?? raw.estimated_points ?? raw.points ?? '0.00000'
  return {
    ...raw,
    points,
    charged_points: raw.charged_points ?? raw.estimated_points ?? raw.points,
    display_points: raw.display_points ?? points,
    formula: raw.formula ?? `${req?.route_model_code ?? raw.pricing_snapshot?.route_model_code ?? ''} x ${req?.quality ?? raw.resolved_quality_bucket ?? ''}`,
    resolved_quality: raw.resolved_quality_bucket ?? raw.resolved_quality ?? req?.quality ?? 'auto',
    sufficient: Boolean(raw.sufficient),
    insufficient_points: raw.insufficient_points ?? '0.00000',
    balance: raw.balance ? toBalance(raw.balance) : undefined,
  }
}

function toGalleryImage(raw: any): GalleryImage {
  const taskType = raw.task_type ? normalizeTaskType(raw.task_type) : undefined
  return {
    ...raw,
    id: String(raw.id ?? raw.image_id ?? ''),
    task_id: String(raw.task_id ?? ''),
    task_type: taskType,
    reference_asset_ids: raw.reference_asset_ids ?? [],
    reference_assets: (raw.reference_assets ?? []).map(toReferenceAsset),
    url: raw.url ?? raw.asset_url ?? raw.download_url ?? '',
    asset_url: raw.asset_url,
    download_url: raw.download_url,
    file_size_bytes: Number(raw.file_size_bytes ?? raw.size_bytes ?? 0),
    width: Number(raw.width ?? 0),
    height: Number(raw.height ?? 0),
    image_group: raw.image_group ?? raw.group ?? '',
    visibility_status: raw.visibility_status ?? raw.publish_status ?? 'private',
    like_count: Number(raw.like_count ?? 0),
    favorite_count: Number(raw.favorite_count ?? 0),
    liked_by_viewer: Boolean(raw.liked_by_viewer),
    favorited_by_viewer: Boolean(raw.favorited_by_viewer),
    created_at: raw.created_at ?? '',
  }
}

export const userApi = {
  configureAuth: sharedApiClient.setAuth.bind(sharedApiClient),
  sendEmailCode: (email: string, scene: 'login' | 'register' | 'password_reset' = 'login') =>
    sharedApiClient.request<{ email: string; scene: string; status: string }>(API_PATHS.agent.sendEmailCode, { method: 'POST', body: { email, scene }, auth: false }),
  loginWithEmailCode: (email: string, code: string) =>
    sharedApiClient.request<LoginResult>(API_PATHS.agent.loginEmailCode, { method: 'POST', body: { email, code }, auth: false }),
  loginWithPassword: (email: string, password: string) =>
    sharedApiClient.request<LoginResult>(API_PATHS.agent.loginPassword, { method: 'POST', body: { email, password }, auth: false }),
  refreshSession: () =>
    sharedApiClient.request<LoginResult>(API_PATHS.agent.refreshSession, { method: 'POST', auth: false, retryUnauthorized: false }),
  logout: () => sharedApiClient.request<void>(API_PATHS.agent.logout, { method: 'POST' }),
  changePassword: (old_password: string, new_password: string) =>
    sharedApiClient.request<{ ok: boolean }>(API_PATHS.agent.passwordChange, { method: 'POST', body: { old_password, new_password } }),
  requestPasswordReset: (email: string) =>
    sharedApiClient.request<{ status: string }>(API_PATHS.agent.passwordResetRequest, { method: 'POST', body: { email }, auth: false }),
  confirmPasswordReset: (email: string, code: string, new_password: string) =>
    sharedApiClient.request<{ ok: boolean }>(API_PATHS.agent.passwordResetConfirm, { method: 'POST', body: { email, code, new_password }, auth: false }),
  getProfile: async () => toUserProfile(await sharedApiClient.request(API_PATHS.agent.profile)),
  getProfileWithToken: async (token: string) => toUserProfile(await sharedApiClient.request(API_PATHS.agent.profile, { auth: false, retryUnauthorized: false, headers: { Authorization: `Bearer ${token}` } })),
  updateProfile: async (patch: Partial<UserProfile>) => toUserProfile(await sharedApiClient.request(API_PATHS.agent.profile, {
    method: 'PUT',
    body: {
      nickname: patch.display_name ?? patch.nickname,
      bio: patch.signature ?? patch.bio,
      avatar_object_key: patch.avatar_object_key,
      default_locale: patch.default_locale ?? patch.preferences?.model_group,
      theme: patch.theme,
    },
  })),
  updatePreferences: async (preferences: UpdatePreferencesRequest) => toUserProfile(await sharedApiClient.request(API_PATHS.agent.preferences, { method: 'PUT', body: preferences })),
  uploadAvatar: async (file: File) => {
    const formData = new FormData()
    formData.set('file', file)
    return toUserProfile(await sharedApiClient.request(API_PATHS.agent.avatar, { method: 'POST', formData }))
  },
  closeAccount: () => sharedApiClient.request<void>(API_PATHS.agent.accountClose, { method: 'POST' }),
  getBalance: async () => toBalance(await sharedApiClient.request(API_PATHS.agent.balance)),
  getLedger: async (page = 1, page_size = 20) => {
    const result = normalizePage<LedgerEntry>(await sharedApiClient.request(API_PATHS.agent.ledger, { query: { page, page_size } }))
    return result.items
  },
  listPlans: async () => (await sharedApiClient.request<{ items: BillingPlan[] }>(API_PATHS.agent.plans)).items ?? [],
  getSubscription: async () => (await sharedApiClient.request<{ item: Subscription | null }>(API_PATHS.agent.subscription)).item,
  listOrders: async (page = 1, page_size = 20) => normalizePage<PaymentOrder>(await sharedApiClient.request(API_PATHS.agent.orders, { query: { page, page_size } })),
  createOrder: (plan_code: string, provider = 'alipay') => sharedApiClient.request<PaymentOrder>(API_PATHS.agent.orders, { method: 'POST', body: { plan_code, provider } }),
  getOrder: (order_id: string | number) => sharedApiClient.request<PaymentOrder>(API_PATHS.agent.orderDetail, { pathParams: { order_id } }),
  cancelOrder: (order_id: string | number) => sharedApiClient.request<PaymentOrder>(API_PATHS.agent.orderCancel, { method: 'POST', pathParams: { order_id } }),
  getCashierOptions: () => sharedApiClient.request<CashierOptions>(API_PATHS.agent.cashierOptions),
  listCashierOrders: async (page = 1, page_size = 20) => normalizePage<CashierOrder>(await sharedApiClient.request(API_PATHS.agent.cashierOrders, { query: { page, page_size } })),
  createCashierOrder: (input: CreateCashierOrderRequest, idempotencyKey: string = crypto.randomUUID()) =>
    sharedApiClient.request<CashierOrder>(API_PATHS.agent.cashierOrders, {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey },
      body: input,
    }),
  getCashierOrder: (order_id: string | number) => sharedApiClient.request<CashierOrder>(API_PATHS.agent.cashierOrderDetail, { pathParams: { order_id } }),
  cancelCashierOrder: (order_id: string | number) => sharedApiClient.request<CashierOrder>(API_PATHS.agent.cashierOrderCancel, { method: 'POST', pathParams: { order_id } }),
  mockPayCashierOrder: (order_id: string | number) => sharedApiClient.request<CashierOrder>(API_PATHS.agent.cashierOrderMockPay, { method: 'POST', pathParams: { order_id } }),
  redeemCode: (code: string, idempotencyKey = crypto.randomUUID()) => sharedApiClient.request(API_PATHS.agent.redeemCode, { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: { code } }),
  getCapabilities: async (): Promise<Capability> => {
    const raw: any = await sharedApiClient.request(API_PATHS.agent.capabilities)
    const models = raw.model_groups ?? raw.abstract_models ?? raw.models ?? []
    const normalizedModels = models.flatMap((item: any) => {
      const taskTypes = pick<string[]>(item, 'task_types', 'TaskTypes') ?? ['text_to_image']
      const normalizedTaskTypes = taskTypes.map(normalizeTaskType)
      const qualities = pick<string[]>(item, 'qualities', 'Qualities', 'supported_qualities', 'SupportedQualities')
        ?? pick<string[]>(raw, 'qualities', 'Qualities', 'supported_qualities', 'SupportedQualities')
        ?? ['auto']
      const prices = (pick<any[]>(item, 'prices', 'Prices') ?? []).map((price: any) => ({
        task_type: normalizeTaskType(pick<string>(price, 'task_type', 'TaskType') ?? 'text_to_image'),
        quality: pick<string>(price, 'quality', 'Quality') ?? 'auto',
        base_points: String(pick(price, 'base_points', 'BasePoints') ?? '0.00000'),
        charged_points: String(pick(price, 'charged_points', 'ChargedPoints', 'points', 'Points', 'base_points', 'BasePoints') ?? '0.00000'),
        display_points: String(pick(price, 'display_points', 'DisplayPoints', 'charged_points', 'ChargedPoints', 'points', 'Points', 'base_points', 'BasePoints') ?? '0.00'),
        reference_multiplier: pick(price, 'reference_multiplier', 'ReferenceMultiplier'),
      }))
      const code = pick(item, 'code', 'Code', 'route_model_code', 'RouteModelCode', 'group_code', 'GroupCode', 'model_code', 'ModelCode', 'id', 'ID')
      const normalizedCode = String(code ?? '').trim()
      if (!normalizedCode || normalizedCode === 'undefined') return []
      const maxReference = Number(pick(item, 'max_reference_image_count', 'MaxReferenceImageCount', 'max_reference_count', 'MaxReferenceCount') ?? 0)
      return [{
        id: normalizedCode,
        code: normalizedCode,
        name: pick(item, 'name', 'Name', 'group_name', 'GroupName', 'model_code', 'ModelCode') ?? normalizedCode,
        description: pick<string>(item, 'description', 'Description') ?? '',
        task_types: normalizedTaskTypes,
        qualities,
        aspect_ratios: pick(item, 'aspect_ratios', 'AspectRatios') ?? pick(raw, 'aspect_ratios', 'AspectRatios', 'supported_ratios', 'SupportedRatios'),
        max_output_image_count: Number(pick(item, 'max_output_image_count', 'MaxOutputImageCount', 'max_image_count', 'MaxImageCount') ?? pick(raw, 'max_image_count', 'MaxImageCount') ?? 4),
        max_reference_image_count: maxReference,
        effective_multiplier: pick(item, 'effective_multiplier', 'EffectiveMultiplier'),
        prices,
        supports_reference: Boolean(pick(item, 'supports_reference', 'SupportsReference', 'supports_image_input', 'SupportsImageInput') ?? ((maxReference > 0) || normalizedTaskTypes.some((type) => type === 'reference_to_image' || type === 'image_edit'))),
        display_points: pick(item, 'display_points', 'DisplayPoints') ?? prices[0]?.display_points,
      }]
    })
    return {
      raw,
      unavailable_reason: raw.unavailable_reason ?? null,
      model_groups: normalizedModels,
      qualities: pick(raw, 'qualities', 'Qualities', 'supported_qualities', 'SupportedQualities') ?? normalizedModels[0]?.qualities ?? ['auto', '1K', '2K', '4K'],
      aspect_ratios: pick(raw, 'aspect_ratios', 'AspectRatios', 'supported_ratios', 'SupportedRatios') ?? ['1:1', '16:9', '9:16', '4:3'],
      max_image_count: pick(raw, 'max_image_count', 'MaxImageCount') ?? 4,
      reference_image_max_mb: Number(pick(raw, 'reference_image_max_mb', 'ReferenceImageMaxMB') ?? 0) || undefined,
      reference_image_max_bytes: Number(pick(raw, 'reference_image_max_bytes', 'ReferenceImageMaxBytes') ?? 0) || undefined,
      task_types: (pick<string[]>(raw, 'task_types', 'TaskTypes') ?? ['text_to_image', 'reference_to_image', 'image_edit']).map(normalizeTaskType),
    } satisfies Capability
  },
  estimate: async (req: EstimateRequest) => toEstimate(await sharedApiClient.request(API_PATHS.agent.estimate, { query: toEstimateQuery(req) }), req),
  uploadReferenceAsset: async (file: File | string, sizeBytes?: number) => {
    if (typeof file === 'string') {
      return { id: '', name: file, preview_url: '', status: 'ready', size_bytes: sizeBytes ?? 0, created_at: '' } satisfies ReferenceAsset
    }
    const formData = new FormData()
    formData.set('file', file)
    return toReferenceAsset(await sharedApiClient.request(API_PATHS.agent.referenceAssets, { method: 'POST', formData }))
  },
  listReferenceAssets: async () => [] as ReferenceAsset[],
  importReferenceAssetsFromGallery: async (galleryImageIds: string[]) => {
    const response = await sharedApiClient.request<{ items?: any[]; assets?: any[]; references?: any[] } | any[]>(API_PATHS.agent.importReferenceAssetsFromGallery, {
      method: 'POST',
      body: { gallery_image_ids: galleryImageIds },
    })
    const items = Array.isArray(response) ? response : response.items ?? response.assets ?? response.references ?? []
    return items.map(toReferenceAsset)
  },
  getReferenceAsset: async (asset_id: string) => toReferenceAsset(await sharedApiClient.request(API_PATHS.agent.referenceAssetDetail, { pathParams: { asset_id } })),
  deleteReferenceAsset: (asset_id: string) => sharedApiClient.request<void>(API_PATHS.agent.referenceAssetDetail, { method: 'DELETE', pathParams: { asset_id } }),
  preferredImageUrl: (image: { asset_url?: string; download_url?: string; url?: string }) => image.asset_url || image.download_url || image.url || '',
  imageAssetUrl: (url: string, accessToken?: string | null) => apiEventUrl(url, accessToken),
  getImageAccessUrl: (image_id: string) => sharedApiClient.request<ImageAccessURL>(API_PATHS.agent.imageAccessUrl, { method: 'POST', pathParams: { image_id } }),
  createTask: async (req: CreateTaskRequest) => toTask(await sharedApiClient.request(API_PATHS.agent.tasks, { method: 'POST', body: toBackendTask(req) })),
  getTask: async (task_id: string) => toTask(await sharedApiClient.request(API_PATHS.agent.taskDetail, { pathParams: { task_id } })),
  taskEventsUrl: (task_id: string, accessToken?: string | null) => apiEventUrl(fillPath(API_PATHS.agent.taskEvents, { task_id }), accessToken),
  taskStreamUrl: (accessToken?: string | null) => apiEventUrl(API_PATHS.agent.taskStream, accessToken),
  listTasks: async (filters?: { query?: string; status?: string; type?: string }) => {
    const page = normalizePage<any>(await sharedApiClient.request(API_PATHS.agent.tasks, {
      query: { status: filters?.status === 'all' ? undefined : filters?.status, task_type: filters?.type === 'all' ? undefined : filters?.type, query: filters?.query },
    }))
    return page.items.map(toTask)
  },
  listHistoryTasks: async (filters?: { query?: string; status?: string; type?: string }) => {
    const page = normalizePage<any>(await sharedApiClient.request(API_PATHS.agent.historyTasks, {
      query: { status: filters?.status === 'all' ? undefined : filters?.status, task_type: filters?.type === 'all' ? undefined : filters?.type, query: filters?.query },
    }))
    return page.items.map(toTask)
  },
  listGalleryImages: async (page = 1, page_size = 100) => normalizePage<GalleryImage>(await sharedApiClient.request(API_PATHS.agent.galleryImages, { query: { page, page_size } })).items.map(toGalleryImage),
  retryTask: async (task_id: string) => toTask(await sharedApiClient.request(API_PATHS.agent.historyTaskRetry, { method: 'POST', pathParams: { task_id } })),
  deleteTask: (task_id: string) => sharedApiClient.request<void>(API_PATHS.agent.historyTaskDetail, { method: 'DELETE', pathParams: { task_id } }),
  deleteGalleryImage: (image_id: string) => sharedApiClient.request<void>(API_PATHS.agent.galleryImageDetail, { method: 'DELETE', pathParams: { image_id } }),
  updateGalleryImageGroup: async (image_id: string, image_group: string) => toGalleryImage(await sharedApiClient.request(API_PATHS.agent.galleryImageGroup, { method: 'PUT', pathParams: { image_id }, body: { image_group } })),
  publishImage: async (image_id: string) => toImageResult(await sharedApiClient.request(API_PATHS.agent.publishImage, { method: 'POST', pathParams: { image_id } })),
  likePublicImage: async (image_id: string, active: boolean) => toGalleryImage(await sharedApiClient.request(API_PATHS.agent.likePublicImage, { method: 'POST', pathParams: { image_id }, body: { active } })),
  favoritePublicImage: async (image_id: string, active: boolean) => toGalleryImage(await sharedApiClient.request(API_PATHS.agent.favoritePublicImage, { method: 'POST', pathParams: { image_id }, body: { active } })),
  listApiKeys: async () => ((await sharedApiClient.request<{ items: any[] }>(API_PATHS.agent.apiKeys)).items ?? []).map(toApiKey),
  createApiKey: async (input: CreateApiKeyRequest & { rpm_limit: number; expires_at: string | null }) => toApiKey(await sharedApiClient.request(API_PATHS.agent.apiKeys, {
    method: 'POST',
    body: {
      name: input.name,
      scopes: input.scopes,
      rpm_limit: input.rpm_limit,
      total_quota_points: input.total_quota_points ?? null,
      daily_quota_points: input.daily_quota_points ?? null,
      expires_at: input.expires_at ? new Date(input.expires_at).toISOString() : null,
    },
  })),
  updateApiKey: async (key_id: string | number, patch: Partial<ApiKey>) => toApiKey(await sharedApiClient.request(API_PATHS.agent.apiKeyDetail, {
    method: 'PUT',
    pathParams: { key_id },
    body: { ...patch, expires_at: patch.expires_at ? new Date(patch.expires_at).toISOString() : patch.expires_at },
  })),
  resetApiKeySecret: async (key_id: string | number) => toApiKey(await sharedApiClient.request(API_PATHS.agent.apiKeyResetSecret, { method: 'POST', pathParams: { key_id } })),
  deleteApiKey: (key_id: string | number) => sharedApiClient.request<void>(API_PATHS.agent.apiKeyDetail, { method: 'DELETE', pathParams: { key_id } }),
}

function apiEventUrl(path: string, accessToken?: string | null) {
  if (/^https?:\/\//i.test(path)) return path
  const baseUrl = getDefaultBaseUrl() || globalThis.location?.origin || ''
  return `${baseUrl}${withQuery(path, { access_token: accessToken })}`
}

function toApiKey(raw: any): ApiKey {
  return {
    ...raw,
    id: String(raw.id),
    access_key: raw.access_key ?? raw.key_prefix ?? '',
    secret_preview: raw.secret ?? raw.secret_preview,
    scopes: raw.scopes ?? ['images:write', 'images:read'],
    rpm_limit: Number(raw.rpm_limit ?? 0),
    expires_at: raw.expires_at ?? null,
    created_at: raw.created_at ?? '',
    last_used_at: raw.last_used_at ?? null,
  }
}

export function pageItems<T>(page: PageResult<T>) {
  return page.items
}
