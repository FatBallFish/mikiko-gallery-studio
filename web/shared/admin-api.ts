import type {
  AdminLoginResult,
  AdminMetric,
  AdminSession,
  AdminUser,
  AuditLog,
  CallRecord,
  ConfigItem,
  ConfigTab,
  ModelProvider,
  ModelRoute,
  PageResult,
  ProviderHealth,
  ProviderModel,
  RedeemCode,
  ReviewItem,
  UserGroup,
} from './api-types'
import { API_PATHS } from './api-types'
import { normalizePage, sharedApiClient } from './http-client'

export const adminApi = {
  configureAuth: sharedApiClient.setAuth.bind(sharedApiClient),
  login: async (email: string, password: string): Promise<AdminSession> => {
    const result = await sharedApiClient.request<AdminLoginResult>(API_PATHS.ops.login, { method: 'POST', body: { email, password }, auth: false })
    return { token: result.access_token, admin_name: result.email || `Admin ${result.admin_id}`, role: result.role, email: result.email, admin_id: result.admin_id }
  },
  logout: () => sharedApiClient.request<void>(API_PATHS.ops.logout, { method: 'POST' }),
  dashboard: async () => {
    const raw: any = await sharedApiClient.request(API_PATHS.ops.dashboard)
    return {
      metrics: (raw.metrics ?? []).map(toMetric),
      providers: (raw.providers ?? []).map(toProviderHealth),
      queue: raw.queue ?? [],
      audit: (raw.audit ?? []).map(toAudit),
    }
  },
  listConfig: async () => {
    const tabs = (await sharedApiClient.request<{ items: any[] }>(API_PATHS.ops.configTabs)).items ?? []
    return tabs.flatMap(toConfigItems)
  },
  listConfigTabs: async () => (await sharedApiClient.request<{ items: ConfigTab[] }>(API_PATHS.ops.configTabs)).items ?? [],
  updateConfigTab: (tab_key: string, payload: { version: number; items: Array<{ config_category: string; config_key: string; config_value: Record<string, unknown>; scope: string }> }) =>
    sharedApiClient.request<ConfigTab>(API_PATHS.ops.configTabDetail, { method: 'PUT', pathParams: { tab_key }, body: payload }),
  listUsers: async (query = '', page = 1, page_size = 20): Promise<AdminUser[]> => {
    const result = normalizePage<any>(await sharedApiClient.request(API_PATHS.ops.users, { query: { query, page, page_size } }))
    return result.items.map(toAdminUser)
  },
  listUsersPage: async (query = '', page = 1, page_size = 20): Promise<PageResult<AdminUser>> => {
    const result = normalizePage<any>(await sharedApiClient.request(API_PATHS.ops.users, { query: { query, page, page_size } }))
    return { ...result, items: result.items.map(toAdminUser) }
  },
  getUser: async (user_id: string | number) => toAdminUser(await sharedApiClient.request(API_PATHS.ops.userDetail, { pathParams: { user_id } })),
  updateUserStatus: async (user_id: string | number, status: string) => toAdminUser(await sharedApiClient.request(API_PATHS.ops.userStatus, { method: 'POST', pathParams: { user_id }, body: { status } })),
  adjustUserPoints: (user_id: string | number, change_points: string, reason: string, idempotencyKey?: string) =>
    sharedApiClient.request(API_PATHS.ops.userPoints, { method: 'POST', pathParams: { user_id }, headers: idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : undefined, body: { change_points, reason } }),
  resetUserPassword: (user_id: string | number, new_password: string) =>
    sharedApiClient.request(API_PATHS.ops.userResetPassword, { method: 'POST', pathParams: { user_id }, body: { new_password } }),
  updateUserLimits: (user_id: string | number, rpm_limit: number, concurrency_limit: number) =>
    sharedApiClient.request(API_PATHS.ops.userLimits, { method: 'POST', pathParams: { user_id }, body: { rpm_limit, concurrency_limit } }),
  assignUserGroup: (user_id: string | number, user_group_code: string) =>
    sharedApiClient.request(API_PATHS.ops.userGroupAssign, { method: 'POST', pathParams: { user_id }, body: { user_group_code } }),
  listUserGroups: async () => (normalizePage<UserGroup>(await sharedApiClient.request(API_PATHS.ops.userGroups))).items,
  createUserGroup: (group: UserGroup) => sharedApiClient.request<UserGroup>(API_PATHS.ops.userGroups, { method: 'POST', body: group }),
  updateUserGroup: (group_code: string, group: Partial<UserGroup>) => sharedApiClient.request<UserGroup>(API_PATHS.ops.userGroupDetail, { method: 'PUT', pathParams: { group_code }, body: group }),
  deleteUserGroup: (group_code: string) => sharedApiClient.request<void>(API_PATHS.ops.userGroupDetail, { method: 'DELETE', pathParams: { group_code } }),
  listAudit: async (query: Record<string, string | number | undefined> = {}) => {
    const result = normalizePage<any>(await sharedApiClient.request(API_PATHS.ops.auditLogs, { query }))
    return result.items.map(toAudit)
  },
  listReviews: async (status = '', page = 1, page_size = 20) => {
    const result = normalizePage<any>(await sharedApiClient.request(API_PATHS.ops.imageReviews, { query: { status, page, page_size } }))
    return result.items.map(toReview)
  },
  decideReview: async (image_id: string, decision: 'approve' | 'reject' | 'unpublish', reason = '') => {
    const path = decision === 'approve' ? API_PATHS.ops.imageReviewApprove : decision === 'reject' ? API_PATHS.ops.imageReviewReject : API_PATHS.ops.imageReviewUnpublish
    return toReview(await sharedApiClient.request(path, { method: 'POST', pathParams: { image_id }, body: { reason } }))
  },
  listRedeemCodes: async (query: Record<string, string | number | undefined> = {}) => normalizePage<RedeemCode>(await sharedApiClient.request(API_PATHS.ops.redeemCodes, { query })),
  createRedeemCode: (input: Partial<RedeemCode>) => sharedApiClient.request<RedeemCode>(API_PATHS.ops.redeemCodes, { method: 'POST', body: input }),
  batchCreateRedeemCodes: (input: Record<string, unknown>) => sharedApiClient.request(API_PATHS.ops.redeemCodesBatchCreate, { method: 'POST', body: input }),
  updateRedeemCodeStatus: (code_id: string | number, status: string) => sharedApiClient.request<RedeemCode>(API_PATHS.ops.redeemCodeStatus, { method: 'POST', pathParams: { code_id }, body: { status } }),
  listRedeemCodeRedemptions: (code_id: string | number, page = 1, page_size = 20) => sharedApiClient.request(API_PATHS.ops.redeemCodeRedemptions, { pathParams: { code_id }, query: { page, page_size } }),
  listCallRecords: async (query: Record<string, string | number | undefined> = {}) => normalizePage<CallRecord>(await sharedApiClient.request(API_PATHS.ops.callRecords, { query })),
  listModelProviders: async (query: Record<string, string | number | boolean | undefined> = {}) => normalizePage<ModelProvider>(await sharedApiClient.request(API_PATHS.ops.modelProviders, { query })),
  createModelProvider: (input: Partial<ModelProvider>) => sharedApiClient.request<ModelProvider>(API_PATHS.ops.modelProviders, { method: 'POST', body: input }),
  updateModelProvider: (provider_code: string, input: Partial<ModelProvider>) => sharedApiClient.request<ModelProvider>(API_PATHS.ops.modelProviderDetail, { method: 'PUT', pathParams: { provider_code }, body: input }),
  deleteModelProvider: (provider_code: string) => sharedApiClient.request<void>(API_PATHS.ops.modelProviderDetail, { method: 'DELETE', pathParams: { provider_code } }),
  listProviderModels: async (query: Record<string, string | number | boolean | undefined> = {}) => normalizePage<ProviderModel>(await sharedApiClient.request(API_PATHS.ops.providerModels, { query })),
  createProviderModel: (input: Partial<ProviderModel>) => sharedApiClient.request<ProviderModel>(API_PATHS.ops.providerModels, { method: 'POST', body: input }),
  updateProviderModel: (provider_model_id: string | number, input: Partial<ProviderModel>) => sharedApiClient.request<ProviderModel>(API_PATHS.ops.providerModelDetail, { method: 'PUT', pathParams: { provider_model_id }, body: input }),
  deleteProviderModel: (provider_model_id: string | number) => sharedApiClient.request<void>(API_PATHS.ops.providerModelDetail, { method: 'DELETE', pathParams: { provider_model_id } }),
  listRoutes: async () => {
    const result = normalizePage<any>(await sharedApiClient.request(API_PATHS.ops.modelRoutes))
    return result.items.map(toRoute)
  },
  createRoute: (input: Partial<ModelRoute>) => sharedApiClient.request<ModelRoute>(API_PATHS.ops.modelRoutes, { method: 'POST', body: input }),
  updateRoute: async (route_id: string | number, input: Partial<ModelRoute>) => toRoute(await sharedApiClient.request(API_PATHS.ops.modelRouteDetail, { method: 'PUT', pathParams: { route_id }, body: input })),
  deleteRoute: (route_id: string | number) => sharedApiClient.request<void>(API_PATHS.ops.modelRouteDetail, { method: 'DELETE', pathParams: { route_id } }),
}

function toMetric(raw: any): AdminMetric {
  return { label: raw.label ?? raw.key ?? 'Metric', value: String(raw.value ?? ''), trend: raw.trend ?? raw.detail ?? '', tone: raw.tone ?? 'neutral', key: raw.key, detail: raw.detail }
}

function toProviderHealth(raw: any): ProviderHealth {
  const status = raw.status ?? raw.health_status ?? (raw.enabled === false ? 'down' : 'healthy')
  return {
    ...raw,
    provider: raw.provider ?? raw.provider_code ?? 'Provider',
    status: status === 'healthy' ? 'healthy' : status === 'down' ? 'down' : 'degraded',
    latency_ms: Number(raw.latency_ms ?? 0),
    error_rate: raw.error_rate ?? '0%',
    note: raw.note ?? raw.health_status ?? (raw.enabled ? 'enabled' : 'disabled'),
  }
}

function toConfigItems(tab: any): ConfigItem[] {
  return (tab.items ?? []).map((item: any) => {
    const value = JSON.stringify(item.config_value ?? item.value ?? {})
    return {
      tab: tab.tab_name ?? tab.tab_key ?? item.config_category ?? 'config',
      key: item.config_key ?? item.key,
      value,
      draft_value: value,
      state: 'active',
      version: Number(tab.version ?? item.version ?? 1),
      description: item.description ?? item.scope ?? '',
      ...item,
    }
  })
}

function toAdminUser(raw: any): AdminUser {
  const name = raw.display_name ?? raw.nickname ?? raw.email?.split('@')[0] ?? `User ${raw.id}`
  return {
    ...raw,
    id: String(raw.id ?? raw.user_id),
    email: raw.email ?? '',
    display_name: name,
    status: raw.status ?? 'active',
    group: raw.user_group_code ?? raw.group ?? 'DEFAULT',
    balance: raw.balance ?? raw.available_points ?? '0.00000',
    created_at: raw.created_at ?? '',
    last_seen_at: raw.last_seen_at ?? raw.updated_at ?? '',
  }
}

function toAudit(raw: any): AuditLog {
  return {
    ...raw,
    id: String(raw.id),
    actor: raw.actor ?? `${raw.actor_type ?? ''}:${raw.actor_id ?? ''}`,
    action: raw.action ?? '',
    target: raw.target ?? `${raw.target_type ?? ''}:${raw.target_id ?? ''}`,
    detail: raw.detail ?? raw.result ?? '',
    created_at: raw.created_at ?? '',
  }
}

function toReview(raw: any): ReviewItem {
  return {
    ...raw,
    id: String(raw.id ?? raw.image_id),
    image_id: String(raw.image_id ?? raw.id),
    title: raw.title ?? raw.prompt?.slice(0, 32) ?? '公开图片',
    owner: raw.owner ?? raw.user_id ?? '',
    task_type: raw.task_type ?? 'text_to_image',
    image_url: raw.image_url ?? raw.download_url ?? raw.url ?? '',
    status: raw.status ?? raw.visibility_status ?? 'pending',
    reason: raw.reason ?? raw.review_reason ?? '',
    created_at: raw.created_at ?? '',
  }
}

function toRoute(raw: any): ModelRoute {
  return {
    ...raw,
    id: String(raw.id),
    scene: raw.scene ?? `${raw.group_code ?? ''}/${raw.task_type ?? ''}`,
    provider: raw.provider ?? raw.provider_code ?? '',
    policy: raw.policy ?? `priority ${raw.priority ?? 0} / weight ${raw.weight_percent ?? 0}`,
    priority: Number(raw.priority ?? 0),
    enabled: Boolean(raw.enabled),
    note: raw.note ?? `fallback ${raw.fallback_order ?? 0}`,
  }
}
