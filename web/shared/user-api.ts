import type {
  ApiKey,
  Balance,
  BillingPlan,
  Capability,
  CreateTaskRequest,
  EstimateRequest,
  EstimateResult,
  ImageResult,
  ImageTask,
  LedgerEntry,
  LoginResult,
  PageResult,
  PaymentOrder,
  ReferenceAsset,
  Subscription,
  UserProfile,
} from './api-types'
import { API_PATHS } from './api-types'
import { normalizePage, sharedApiClient } from './http-client'

const sizeMap: Record<string, string> = {
  '1:1': '1024x1024',
  '16:9': '1536x864',
  '9:16': '864x1536',
  '4:3': '1536x1152',
  '3:4': '1152x1536',
}

function initials(input: string) {
  return input.trim().slice(0, 2).toUpperCase() || 'PG'
}

export function toUserProfile(raw: any): UserProfile {
  const name = raw.nickname || raw.display_name || raw.email?.split('@')[0] || 'Pic User'
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
  if (type === 'reference_to_image' || type === 'image_to_image') return 'reference_to_image'
  return 'text_to_image'
}

export function toReferenceAsset(raw: any): ReferenceAsset {
  return {
    ...raw,
    id: String(raw.id ?? raw.asset_id ?? ''),
    name: raw.name ?? raw.filename ?? raw.id ?? 'reference',
    preview_url: raw.preview_url ?? raw.download_url ?? '',
    status: raw.status ?? 'ready',
    size_bytes: Number(raw.size_bytes ?? raw.file_size ?? 0),
    created_at: raw.created_at ?? '',
  }
}

export function toImageResult(raw: any): ImageResult {
  return {
    ...raw,
    id: String(raw.id ?? raw.image_id ?? ''),
    url: raw.url ?? raw.download_url ?? '',
    width: Number(raw.width ?? 0),
    height: Number(raw.height ?? 0),
    publish_status: raw.publish_status ?? raw.visibility_status ?? 'private',
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
    route_model_code: raw.route_model_code ?? raw.model_group ?? raw.abstract_model ?? raw.group_code ?? 'basic',
    route_model_name: raw.route_model_name,
    model_group: raw.route_model_code ?? raw.model_group ?? raw.abstract_model ?? raw.group_code ?? 'basic',
    quality,
    aspect_ratio: raw.aspect_ratio ?? raw.requested_size ?? '1:1',
    image_count: Number(raw.image_count ?? raw.requested_output_image_count ?? results.length ?? 1),
    estimate_points: raw.estimate_points ?? raw.estimated_points ?? raw.actual_points ?? '0.00000',
    progress: Number(raw.progress ?? (raw.status === 'succeeded' ? 100 : raw.status === 'running' ? 48 : 8)),
    provider: raw.provider ?? raw.provider_code ?? '',
    route: raw.route ?? raw.route_policy ?? '',
    created_at: raw.created_at ?? '',
    updated_at: raw.updated_at ?? raw.created_at ?? '',
    failure_reason: raw.failure_reason ?? raw.error_message,
    reference_assets: (raw.reference_assets ?? []).map(toReferenceAsset),
    results,
  }
}

function toEstimateQuery(req: EstimateRequest) {
  return {
    task_type: req.task_type,
    route_model_code: req.route_model_code,
    requested_quality: req.quality,
    requested_size: sizeMap[req.aspect_ratio] ?? req.aspect_ratio,
    requested_output_image_count: req.image_count,
    reference_image_count: req.reference_asset_ids?.length ?? 0,
  }
}

function toBackendTask(req: CreateTaskRequest) {
  return {
    task_type: req.task_type,
    prompt: req.negative_prompt ? `${req.prompt}\n\nNegative prompt: ${req.negative_prompt}` : req.prompt,
    route_model_code: req.route_model_code,
    requested_quality: req.quality,
    requested_size: sizeMap[req.aspect_ratio] ?? req.aspect_ratio,
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
    sufficient: raw.sufficient ?? true,
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
  updatePreferences: async (theme: string, default_locale: string) => toUserProfile(await sharedApiClient.request(API_PATHS.agent.preferences, { method: 'PUT', body: { theme, default_locale } })),
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
  redeemCode: (code: string, idempotencyKey = crypto.randomUUID()) => sharedApiClient.request(API_PATHS.agent.redeemCode, { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: { code } }),
  getCapabilities: async (): Promise<Capability> => {
    const raw: any = await sharedApiClient.request(API_PATHS.agent.capabilities)
    const models = raw.model_groups ?? raw.abstract_models ?? raw.models ?? []
    const normalizedModels = models.map((item: any) => {
      const taskTypes = item.task_types ?? ['text_to_image']
      const qualities = item.qualities ?? item.supported_qualities ?? raw.qualities ?? raw.supported_qualities ?? ['auto']
      const prices = (item.prices ?? []).map((price: any) => ({
        task_type: price.task_type ?? 'text_to_image',
        quality: price.quality ?? 'auto',
        base_points: String(price.base_points ?? '0.00000'),
        charged_points: String(price.charged_points ?? price.points ?? price.base_points ?? '0.00000'),
        display_points: String(price.display_points ?? price.charged_points ?? price.points ?? price.base_points ?? '0.00'),
        reference_multiplier: price.reference_multiplier,
      }))
      const code = item.code ?? item.route_model_code ?? item.group_code ?? item.model_code ?? item.id
      const maxReference = Number(item.max_reference_image_count ?? item.max_reference_count ?? 0)
      return {
        id: String(code),
        code: String(code),
        name: item.name ?? item.group_name ?? item.model_code ?? code,
        description: item.description ?? '',
        task_types: taskTypes,
        qualities,
        aspect_ratios: item.aspect_ratios ?? raw.aspect_ratios ?? raw.supported_ratios,
        max_output_image_count: Number(item.max_output_image_count ?? item.max_image_count ?? raw.max_image_count ?? 4),
        max_reference_image_count: maxReference,
        effective_multiplier: item.effective_multiplier,
        prices,
        supports_reference: Boolean(item.supports_reference ?? item.supports_image_input ?? (maxReference > 0)),
        display_points: item.display_points ?? prices[0]?.display_points,
      }
    })
    return {
      raw,
      model_groups: normalizedModels,
      qualities: raw.qualities ?? raw.supported_qualities ?? ['auto', '1K', '2K', '4K'],
      aspect_ratios: raw.aspect_ratios ?? raw.supported_ratios ?? ['1:1', '16:9', '9:16', '4:3'],
      max_image_count: raw.max_image_count ?? 4,
      task_types: raw.task_types ?? ['text_to_image', 'reference_to_image', 'image_edit'],
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
  getReferenceAsset: async (asset_id: string) => toReferenceAsset(await sharedApiClient.request(API_PATHS.agent.referenceAssetDetail, { pathParams: { asset_id } })),
  deleteReferenceAsset: (asset_id: string) => sharedApiClient.request<void>(API_PATHS.agent.referenceAssetDetail, { method: 'DELETE', pathParams: { asset_id } }),
  createTask: async (req: CreateTaskRequest) => toTask(await sharedApiClient.request(API_PATHS.agent.tasks, { method: 'POST', body: toBackendTask(req) })),
  getTask: async (task_id: string) => toTask(await sharedApiClient.request(API_PATHS.agent.taskDetail, { pathParams: { task_id } })),
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
  deleteTask: (task_id: string) => sharedApiClient.request<void>(API_PATHS.agent.historyTaskDetail, { method: 'DELETE', pathParams: { task_id } }),
  publishImage: async (image_id: string) => toImageResult(await sharedApiClient.request(API_PATHS.agent.publishImage, { method: 'POST', pathParams: { image_id } })),
  listApiKeys: async () => ((await sharedApiClient.request<{ items: any[] }>(API_PATHS.agent.apiKeys)).items ?? []).map(toApiKey),
  createApiKey: async (input: { name: string; scopes?: string[]; rpm_limit: number; expires_at: string | null }) => toApiKey(await sharedApiClient.request(API_PATHS.agent.apiKeys, {
    method: 'POST',
    body: { name: input.name, rpm_limit: input.rpm_limit, expires_at: input.expires_at ? new Date(input.expires_at).toISOString() : null },
  })),
  updateApiKey: async (key_id: string | number, patch: Partial<ApiKey>) => toApiKey(await sharedApiClient.request(API_PATHS.agent.apiKeyDetail, { method: 'PUT', pathParams: { key_id }, body: patch })),
  resetApiKeySecret: async (key_id: string | number) => toApiKey(await sharedApiClient.request(API_PATHS.agent.apiKeyResetSecret, { method: 'POST', pathParams: { key_id } })),
  deleteApiKey: (key_id: string | number) => sharedApiClient.request<void>(API_PATHS.agent.apiKeyDetail, { method: 'DELETE', pathParams: { key_id } }),
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
