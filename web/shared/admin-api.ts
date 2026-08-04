import type {
  AdminLoginResult,
  AdminDashboard,
  AdminMonitoringSnapshot,
  AdminMetric,
  AdminPermission,
  AdminSession,
  AdminUser,
  AdminUserCreateRequest,
  AdminUserDetail,
  AuditLog,
  CallRecord,
  ClusterNode,
  CashierCustomAmountConfig,
  CashierOverview,
  CashierPlan,
  ChargebackPaymentOrderRequest,
  ClosePaymentOrderRequest,
  CompletePaymentOrderRequest,
  ConfigItem,
  ConfigTab,
  LedgerEntry,
  ModelProvider,
  MonitoringWindow,
  ModelRoute,
  ModelAccount,
  ModelAccountModel,
  ModelAccountModelWriteRequest,
  ModelAccountTestImageRequest,
  ModelAccountTestImageResult,
  ModelAccountWriteRequest,
  PageResult,
  ProviderHealth,
  ProviderModel,
  PaymentProviderInstance,
  PaymentProviderInstanceWriteRequest,
  PaymentOrder,
  PaymentOrderChargebackResponse,
  PaymentOrderSyncResponse,
  PaymentVisibleMethod,
  PaymentWebhookEvent,
  ReadinessReport,
  RedeemCode,
  RedeemCodeBatchCreateRequest,
  RedeemCodeBatchCreateResult,
  RedeemCodeExportRequest,
  RedeemCodeExportResult,
  ReviewItem,
  RouteModel,
  RouteModelCandidate,
  RouteModelCandidateWriteRequest,
  RouteModelPrice,
  RouteModelPriceWriteRequest,
  RouteModelWriteRequest,
  RefundPaymentOrderRequest,
  SMTPConfigView,
  SMTPConfigWriteRequest,
  SMTPTestResponse,
  StorageConfigView,
  StorageConfigWriteRequest,
  StorageProbeView,
  SystemAdminPasswordResetRequest,
  SystemAdminUser,
  SystemAdminUserCreateRequest,
  SystemAdminUserUpdateRequest,
  TextModel,
  TextModelAccount,
  TextModelAccountWriteRequest,
  TextModelConnectionTest,
  TextModelWriteRequest,
  UserGroup,
  UserGroupWriteRequest,
} from './api-types'
import { API_PATHS } from './api-types'
import { fillPath, normalizePage, sharedApiClient } from './http-client'
import { mediaAssetURL } from './media-url'

const adminSessionMutationLock = 'pic-gallery-admin-session-mutation'
const adminSessionMutationTimeoutMs = 10_000
let adminSessionMutationTail: Promise<void> = Promise.resolve()

function serializeAdminSessionMutation<T>(mutation: (signal: AbortSignal) => Promise<T>): Promise<T> {
  const operation = adminSessionMutationTail.then(() => runAdminSessionMutation(mutation))
  adminSessionMutationTail = operation.then(() => undefined, () => undefined)
  return operation
}

async function runAdminSessionMutation<T>(mutation: (signal: AbortSignal) => Promise<T>): Promise<T> {
  const controller = new AbortController()
  const timeout = globalThis.setTimeout(() => controller.abort(), adminSessionMutationTimeoutMs)
  try {
    return await withAdminSessionMutationLock(controller.signal, mutation)
  } finally {
    globalThis.clearTimeout(timeout)
  }
}

async function withAdminSessionMutationLock<T>(signal: AbortSignal, mutation: (signal: AbortSignal) => Promise<T>): Promise<T> {
  const locks = globalThis.navigator?.locks
  if (!locks) return mutation(signal)
  return await locks.request(adminSessionMutationLock, { signal }, () => mutation(signal))
}

function normalizeGroupIds(ids: Array<string | number>) {
  return ids.map((id) => Number(id)).filter((id) => Number.isFinite(id) && id > 0)
}

function toAdminSession(result: AdminLoginResult): AdminSession {
  const accessToken = typeof result?.access_token === 'string' ? result.access_token.trim() : ''
  const adminId = Number(result?.admin_id)
  if (!accessToken || !Number.isFinite(adminId) || adminId <= 0 || typeof result?.role !== 'string' || !result.role) {
    throw new Error('后台会话刷新响应无效')
  }
  const permissions = (result as AdminLoginResult & { permissions?: AdminPermission[] }).permissions
  return {
    token: accessToken,
    access_token: accessToken,
    expires_in_seconds: Number(result.expires_in_seconds ?? 0),
    admin_name: result.email || `Admin ${adminId}`,
    role: result.role,
    email: result.email,
    admin_id: adminId,
    permissions,
  }
}

export const adminApi = {
  configureAuth: sharedApiClient.setAuth.bind(sharedApiClient),
  login: (email: string, password: string): Promise<AdminSession> => serializeAdminSessionMutation(async (signal) => {
    const result = await sharedApiClient.request<AdminLoginResult>(API_PATHS.ops.login, { method: 'POST', body: { email, password }, auth: false, retryUnauthorized: false, signal })
    return toAdminSession(result)
  }),
  refreshSession: (): Promise<AdminSession> => serializeAdminSessionMutation(async (signal) => {
    const result = await sharedApiClient.request<AdminLoginResult>(API_PATHS.ops.refreshSession, {
      method: 'POST',
      auth: false,
      retryUnauthorized: false,
      signal,
    })
    return toAdminSession(result)
  }),
  logout: () => serializeAdminSessionMutation((signal) => sharedApiClient.request<void>(API_PATHS.ops.logout, { method: 'POST', retryUnauthorized: false, signal })),
  systemAdmins: {
    list: async (query: Record<string, string | number | undefined> = {}): Promise<PageResult<SystemAdminUser>> => {
      const result = normalizePage<any>(await sharedApiClient.request(API_PATHS.ops.adminUsers, { query }))
      return { ...result, items: result.items.map(toSystemAdminUser) }
    },
    create: async (input: SystemAdminUserCreateRequest) => toSystemAdminUser(await sharedApiClient.request(API_PATHS.ops.adminUsers, { method: 'POST', body: input })),
    update: async (admin_id: string | number, input: SystemAdminUserUpdateRequest) =>
      toSystemAdminUser(await sharedApiClient.request(API_PATHS.ops.adminUserDetail, { method: 'PUT', pathParams: { admin_id }, body: input })),
    resetPassword: (admin_id: string | number, input: SystemAdminPasswordResetRequest | string) =>
      sharedApiClient.request<void>(API_PATHS.ops.adminUserResetPassword, {
        method: 'POST',
        pathParams: { admin_id },
        body: typeof input === 'string' ? { new_password: input } : input,
      }),
    delete: (admin_id: string | number) => sharedApiClient.request<SystemAdminUser>(API_PATHS.ops.adminUserDetail, { method: 'DELETE', pathParams: { admin_id } }),
  },
  dashboard: async (): Promise<AdminDashboard> => {
    const raw: any = await sharedApiClient.request(API_PATHS.ops.dashboard)
    return {
      operations: toDashboardOperations(raw.operations),
      metrics: (raw.metrics ?? []).map(toMetric),
      providers: (raw.providers ?? []).map(toProviderHealth),
      queue: raw.queue ?? [],
      audit: (raw.audit ?? []).map(toAudit),
    }
  },
  getMonitoringSnapshot: (window: MonitoringWindow): Promise<AdminMonitoringSnapshot> =>
    sharedApiClient.request<AdminMonitoringSnapshot>(API_PATHS.ops.monitoringSnapshot, { query: { window } }),
  getReadiness: async () => toReadinessReport(await sharedApiClient.request(API_PATHS.ops.readiness)),
  listClusterNodes: async (page = 1, page_size = 20, role = ''): Promise<PageResult<ClusterNode>> => {
    const result = normalizePage<ClusterNode>(await sharedApiClient.request(API_PATHS.ops.clusterNodes, { query: { page, page_size, role: role || undefined } }))
    return result
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
  listUsersPage: async (query = '', page = 1, page_size = 20, filters: Record<string, string | number | undefined> = {}): Promise<PageResult<AdminUser>> => {
    const params = { ...filters, query, page, page_size }
    const result = normalizePage<any>(await sharedApiClient.request(API_PATHS.ops.users, { query: params }))
    return { ...result, items: result.items.map(toAdminUser) }
  },
  createUser: async (input: AdminUserCreateRequest) => toAdminUser(await sharedApiClient.request(API_PATHS.ops.users, { method: 'POST', body: input })),
  getUser: async (user_id: string | number) => toAdminUserDetail(await sharedApiClient.request<AdminUserDetail>(API_PATHS.ops.userDetail, { pathParams: { user_id } })),
  deleteUser: async (user_id: string | number) => toAdminUser(await sharedApiClient.request(API_PATHS.ops.userDetail, { method: 'DELETE', pathParams: { user_id } })),
  updateUserStatus: async (user_id: string | number, status: string) => toAdminUser(await sharedApiClient.request(API_PATHS.ops.userStatus, { method: 'POST', pathParams: { user_id }, body: { status } })),
  adjustUserPoints: (user_id: string | number, change_points: string, reason: string, idempotencyKey?: string) =>
    sharedApiClient.request(API_PATHS.ops.userPoints, { method: 'POST', pathParams: { user_id }, headers: idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : undefined, body: { change_points, reason } }),
  resetUserPassword: (user_id: string | number, new_password: string) =>
    sharedApiClient.request(API_PATHS.ops.userResetPassword, { method: 'POST', pathParams: { user_id }, body: { new_password } }),
  updateUserLimits: (user_id: string | number, rpm_limit: number, concurrency_limit: number) =>
    sharedApiClient.request(API_PATHS.ops.userLimits, { method: 'POST', pathParams: { user_id }, body: { rpm_limit, concurrency_limit } }),
  assignUserGroup: (user_id: string | number, user_group_code: string) =>
    sharedApiClient.request(API_PATHS.ops.userGroupAssign, { method: 'PUT', pathParams: { user_id }, body: { group_ids: normalizeGroupIds([user_group_code]) } }),
  assignUserGroups: (user_id: string | number, group_ids: Array<string | number>) =>
    sharedApiClient.request(API_PATHS.ops.userGroupAssign, { method: 'PUT', pathParams: { user_id }, body: { group_ids: normalizeGroupIds(group_ids) } }),
  listUserGroups: async () => (normalizePage<any>(await sharedApiClient.request(API_PATHS.ops.userGroups))).items.map(toUserGroup),
  createUserGroup: async (group: UserGroupWriteRequest) => toUserGroup(await sharedApiClient.request(API_PATHS.ops.userGroups, { method: 'POST', body: toUserGroupPayload(group) })),
  updateUserGroup: async (group_id: string | number, group: Partial<UserGroupWriteRequest>) => toUserGroup(await sharedApiClient.request(API_PATHS.ops.userGroupDetail, { method: 'PUT', pathParams: { group_id }, body: toUserGroupPayload(group) })),
  deleteUserGroup: (group_id: string | number) => sharedApiClient.request<void>(API_PATHS.ops.userGroupDetail, { method: 'DELETE', pathParams: { group_id } }),
  listAudit: async (query: Record<string, string | number | undefined> = {}) => {
    const result = normalizePage<any>(await sharedApiClient.request(API_PATHS.ops.auditLogs, { query }))
    return result.items.map(toAudit)
  },
  listReviews: async (query: Record<string, string | number | undefined> = {}) => {
    const result = normalizePage<any>(await sharedApiClient.request(API_PATHS.ops.imageReviews, { query }))
    return { ...result, items: result.items.map(toReview) }
  },
  imageReviewUrl: (image_id: string, accessToken?: string | null, projectedURL?: string) => mediaAssetURL(projectedURL || fillPath(API_PATHS.ops.imageReviewImage, { image_id }), accessToken),
  decideReview: async (image_id: string, decision: 'approve' | 'reject' | 'unpublish', reason = '') => {
    const path = decision === 'approve' ? API_PATHS.ops.imageReviewApprove : decision === 'reject' ? API_PATHS.ops.imageReviewReject : API_PATHS.ops.imageReviewUnpublish
    return toReview(await sharedApiClient.request(path, { method: 'POST', pathParams: { image_id }, body: { reason } }))
  },
  listRedeemCodes: async (query: Record<string, string | number | undefined> = {}) => normalizePage<RedeemCode>(await sharedApiClient.request(API_PATHS.ops.redeemCodes, { query })),
  createRedeemCode: (input: Partial<RedeemCode>) => sharedApiClient.request<RedeemCode>(API_PATHS.ops.redeemCodes, { method: 'POST', body: input }),
  batchCreateRedeemCodes: (input: RedeemCodeBatchCreateRequest) => sharedApiClient.request<RedeemCodeBatchCreateResult>(API_PATHS.ops.redeemCodesBatchCreate, { method: 'POST', body: input }),
  exportRedeemCodes: (input: RedeemCodeExportRequest) => sharedApiClient.request<RedeemCodeExportResult>(API_PATHS.ops.redeemCodesExport, { method: 'POST', body: input }),
  updateRedeemCodeStatus: (code_id: string | number, status: string) => sharedApiClient.request<RedeemCode>(API_PATHS.ops.redeemCodeStatus, { method: 'POST', pathParams: { code_id }, body: { status } }),
  listRedeemCodeRedemptions: async (code_id: string | number, page = 1, page_size = 20) => normalizePage<LedgerEntry>(await sharedApiClient.request(API_PATHS.ops.redeemCodeRedemptions, { pathParams: { code_id }, query: { page, page_size } })),
  listCallRecords: async (query: Record<string, string | number | boolean | undefined> = {}) => normalizePage<CallRecord>(await sharedApiClient.request(API_PATHS.ops.callRecords, { query })),
  getCashierOverview: () => sharedApiClient.request<CashierOverview>(API_PATHS.ops.cashierOverview),
  listCashierPlans: async (query: Record<string, string | number | boolean | undefined> = {}) => normalizePage<CashierPlan>(await sharedApiClient.request(API_PATHS.ops.cashierPlans, { query })),
  createCashierPlan: (input: Partial<CashierPlan>) => sharedApiClient.request<CashierPlan>(API_PATHS.ops.cashierPlans, { method: 'POST', body: input }),
  updateCashierPlan: (plan_id: string | number, input: Partial<CashierPlan>) => sharedApiClient.request<CashierPlan>(API_PATHS.ops.cashierPlanDetail, { method: 'PUT', pathParams: { plan_id }, body: input }),
  deleteCashierPlan: (plan_id: string | number) => sharedApiClient.request<CashierPlan>(API_PATHS.ops.cashierPlanDetail, { method: 'DELETE', pathParams: { plan_id } }),
  transitionCashierPlan: (plan_id: string | number, action: 'enable' | 'disable' | 'archive' | 'restore') => sharedApiClient.request<CashierPlan>(API_PATHS.ops.cashierPlanTransition, { method: 'POST', pathParams: { plan_id, action } }),
  getCashierCustomAmountConfig: () => sharedApiClient.request<CashierCustomAmountConfig>(API_PATHS.ops.cashierCustomAmountConfig),
  updateCashierCustomAmountConfig: (input: CashierCustomAmountConfig) => sharedApiClient.request<CashierCustomAmountConfig>(API_PATHS.ops.cashierCustomAmountConfig, { method: 'PUT', body: input }),
  listPaymentVisibleMethods: async () => (await sharedApiClient.request<{ items: PaymentVisibleMethod[] }>(API_PATHS.ops.paymentVisibleMethods)).items ?? [],
  updatePaymentVisibleMethods: (items: PaymentVisibleMethod[]) => sharedApiClient.request<{ items: PaymentVisibleMethod[] }>(API_PATHS.ops.paymentVisibleMethods, { method: 'PUT', body: { items } }),
  listPaymentProviderInstances: async (query: Record<string, string | number | boolean | undefined> = {}) => normalizePage<PaymentProviderInstance>(await sharedApiClient.request(API_PATHS.ops.paymentProviderInstances, { query })),
  createPaymentProviderInstance: (input: PaymentProviderInstanceWriteRequest) => sharedApiClient.request<PaymentProviderInstance>(API_PATHS.ops.paymentProviderInstances, { method: 'POST', body: input }),
  updatePaymentProviderInstance: (instance_id: string | number, input: Partial<PaymentProviderInstanceWriteRequest>) =>
    sharedApiClient.request<PaymentProviderInstance>(API_PATHS.ops.paymentProviderInstanceDetail, { method: 'PUT', pathParams: { instance_id }, body: input }),
  deletePaymentProviderInstance: (instance_id: string | number) =>
    sharedApiClient.request<PaymentProviderInstance>(API_PATHS.ops.paymentProviderInstanceDetail, { method: 'DELETE', pathParams: { instance_id } }),
  getSMTPConfig: () => sharedApiClient.request<SMTPConfigView>(API_PATHS.ops.securitySMTP),
  updateSMTPConfig: (input: SMTPConfigWriteRequest) => sharedApiClient.request<SMTPConfigView>(API_PATHS.ops.securitySMTP, { method: 'PUT', body: input }),
  testSMTPConfig: (email: string, scene = 'smtp_test') => sharedApiClient.request<SMTPTestResponse>(API_PATHS.ops.securitySMTPTest, { method: 'POST', body: { email, scene } }),
  listStorageConfigs: async () => (await sharedApiClient.request<{ items: StorageConfigView[] }>(API_PATHS.ops.storageConfigs)).items ?? [],
  createStorageConfig: (input: StorageConfigWriteRequest) => sharedApiClient.request<StorageConfigView>(API_PATHS.ops.storageConfigs, { method: 'POST', body: input }),
  updateStorageConfig: (storage_config_id: string, input: StorageConfigWriteRequest) =>
    sharedApiClient.request<StorageConfigView>(API_PATHS.ops.storageConfigDetail, { method: 'PUT', pathParams: { storage_config_id }, body: input }),
  probeStorageConfig: (storage_config_id: string) =>
    sharedApiClient.request<StorageConfigView>(API_PATHS.ops.storageConfigDetailProbe, { method: 'POST', pathParams: { storage_config_id } }),
  probeStorageConfigDraft: (input: StorageConfigWriteRequest) => sharedApiClient.request<StorageProbeView>(API_PATHS.ops.storageConfigProbe, { method: 'POST', body: input }),
  setDefaultStorageConfig: (storage_config_id: string, version?: number) =>
    sharedApiClient.request<StorageConfigView>(API_PATHS.ops.storageConfigSetDefault, { method: 'POST', pathParams: { storage_config_id }, body: { version } }),
  setStorageConfigStatus: (storage_config_id: string, input: { version?: number; status: string; read_enabled: boolean; write_enabled: boolean }) =>
    sharedApiClient.request<StorageConfigView>(API_PATHS.ops.storageConfigSetStatus, { method: 'POST', pathParams: { storage_config_id }, body: input }),
  listPaymentOrders: async (query: Record<string, string | number | undefined> = {}) => normalizePage<PaymentOrder>(await sharedApiClient.request(API_PATHS.ops.paymentOrders, { query })),
  getPaymentOrder: (order_id: string | number) => sharedApiClient.request<PaymentOrder>(API_PATHS.ops.paymentOrderDetail, { pathParams: { order_id } }),
  completePaymentOrder: (order_id: string | number, input: CompletePaymentOrderRequest) =>
    sharedApiClient.request<PaymentOrder>(API_PATHS.ops.paymentOrderComplete, { method: 'POST', pathParams: { order_id }, body: input }),
  closePaymentOrder: (order_id: string | number, input: ClosePaymentOrderRequest = {}) =>
    sharedApiClient.request<PaymentOrder>(API_PATHS.ops.paymentOrderClose, { method: 'POST', pathParams: { order_id }, body: input }),
  refundPaymentOrder: (order_id: string | number, input: RefundPaymentOrderRequest) =>
    sharedApiClient.request<PaymentOrder>(API_PATHS.ops.paymentOrderRefund, { method: 'POST', pathParams: { order_id }, body: input }),
  chargebackPaymentOrder: (order_id: string | number, input: ChargebackPaymentOrderRequest, idempotencyKey: string) =>
    sharedApiClient.request<PaymentOrderChargebackResponse>(API_PATHS.ops.paymentOrderChargeback, { method: 'POST', pathParams: { order_id }, headers: { 'Idempotency-Key': idempotencyKey }, body: input }),
  syncPaymentOrder: (order_id: string | number) =>
    sharedApiClient.request<PaymentOrderSyncResponse>(API_PATHS.ops.paymentOrderSync, { method: 'POST', pathParams: { order_id } }),
  listPaymentWebhookEvents: async (query: Record<string, string | number | undefined> = {}) => normalizePage<PaymentWebhookEvent>(await sharedApiClient.request(API_PATHS.ops.paymentWebhookEvents, { query })),
  retryPaymentWebhookEvent: (event_id: string | number) => sharedApiClient.request<PaymentWebhookEvent>(API_PATHS.ops.paymentWebhookEventRetry, { method: 'POST', pathParams: { event_id } }),
  listModelProviders: async (query: Record<string, string | number | boolean | undefined> = {}) => normalizePage<ModelProvider>(await sharedApiClient.request(API_PATHS.ops.modelProviders, { query })),
  createModelProvider: (input: Partial<ModelProvider>) => sharedApiClient.request<ModelProvider>(API_PATHS.ops.modelProviders, { method: 'POST', body: input }),
  updateModelProvider: (provider_code: string, input: Partial<ModelProvider>) => sharedApiClient.request<ModelProvider>(API_PATHS.ops.modelProviderDetail, { method: 'PUT', pathParams: { provider_code }, body: input }),
  deleteModelProvider: (provider_code: string) => sharedApiClient.request<void>(API_PATHS.ops.modelProviderDetail, { method: 'DELETE', pathParams: { provider_code } }),
  listProviderModels: async (query: Record<string, string | number | boolean | undefined> = {}) => normalizePage<ProviderModel>(await sharedApiClient.request(API_PATHS.ops.providerModels, { query })),
  createProviderModel: (input: Partial<ProviderModel>) => sharedApiClient.request<ProviderModel>(API_PATHS.ops.providerModels, { method: 'POST', body: input }),
  updateProviderModel: (provider_model_id: string | number, input: Partial<ProviderModel>) => sharedApiClient.request<ProviderModel>(API_PATHS.ops.providerModelDetail, { method: 'PUT', pathParams: { provider_model_id }, body: input }),
  deleteProviderModel: (provider_model_id: string | number) => sharedApiClient.request<void>(API_PATHS.ops.providerModelDetail, { method: 'DELETE', pathParams: { provider_model_id } }),
  listModelAccounts: async (query: Record<string, string | number | boolean | undefined> = {}) => normalizePage<any>(await sharedApiClient.request(API_PATHS.ops.modelAccounts, { query })).items.map(toModelAccount),
  listModelAccountsPage: async (query: Record<string, string | number | boolean | undefined> = {}) => {
    const result = normalizePage<any>(await sharedApiClient.request(API_PATHS.ops.modelAccounts, { query }))
    return { ...result, items: result.items.map(toModelAccount) }
  },
  createModelAccount: async (input: ModelAccountWriteRequest) => toModelAccount(await sharedApiClient.request(API_PATHS.ops.modelAccounts, { method: 'POST', body: input })),
  updateModelAccount: async (account_id: string | number, input: Partial<ModelAccountWriteRequest>) => toModelAccount(await sharedApiClient.request(API_PATHS.ops.modelAccountDetail, { method: 'PUT', pathParams: { account_id }, body: input })),
  deleteModelAccount: (account_id: string | number) => sharedApiClient.request<void>(API_PATHS.ops.modelAccountDetail, { method: 'DELETE', pathParams: { account_id } }),
  listModelAccountModels: async (account_id: string | number) => (normalizePage<any>(await sharedApiClient.request(API_PATHS.ops.modelAccountModels, { pathParams: { account_id } }))).items.map((row) => toModelAccountModel(row, account_id)),
  createModelAccountModel: async (account_id: string | number, input: ModelAccountModelWriteRequest) => toModelAccountModel(await sharedApiClient.request(API_PATHS.ops.modelAccountModels, { method: 'POST', pathParams: { account_id }, body: input }), account_id),
  updateModelAccountModel: async (account_id: string | number, model_id: string | number, input: Partial<ModelAccountModelWriteRequest>) => toModelAccountModel(await sharedApiClient.request(API_PATHS.ops.modelAccountModelDetail, { method: 'PUT', pathParams: { account_id, model_id }, body: input }), account_id),
  deleteModelAccountModel: (account_id: string | number, model_id: string | number) => sharedApiClient.request<void>(API_PATHS.ops.modelAccountModelDetail, { method: 'DELETE', pathParams: { account_id, model_id } }),
  testModelAccountImage: (account_id: string | number, input: ModelAccountTestImageRequest) =>
    sharedApiClient.request<ModelAccountTestImageResult>(API_PATHS.ops.modelAccountTestImage, { method: 'POST', pathParams: { account_id }, body: input }),
  listTextModelAccounts: async () => (await sharedApiClient.request<{ items: TextModelAccount[] }>(API_PATHS.ops.textModelAccounts)).items ?? [],
  createTextModelAccount: (input: TextModelAccountWriteRequest) => sharedApiClient.request<TextModelAccount>(API_PATHS.ops.textModelAccounts, { method: 'POST', body: input }),
  updateTextModelAccount: (account_id: string | number, input: TextModelAccountWriteRequest) => sharedApiClient.request<TextModelAccount>(API_PATHS.ops.textModelAccountDetail, { method: 'PUT', pathParams: { account_id }, body: input }),
  deleteTextModelAccount: (account_id: string | number) => sharedApiClient.request<void>(API_PATHS.ops.textModelAccountDetail, { method: 'DELETE', pathParams: { account_id } }),
  listTextModels: async (account_id: string | number) => (await sharedApiClient.request<{ items: TextModel[] }>(API_PATHS.ops.textModelAccountModels, { pathParams: { account_id } })).items ?? [],
  createTextModel: (account_id: string | number, input: TextModelWriteRequest) => sharedApiClient.request<TextModel>(API_PATHS.ops.textModelAccountModels, { method: 'POST', pathParams: { account_id }, body: input }),
  updateTextModel: (model_id: string | number, input: TextModelWriteRequest) => sharedApiClient.request<TextModel>(API_PATHS.ops.textModelDetail, { method: 'PUT', pathParams: { model_id }, body: input }),
  deleteTextModel: (model_id: string | number) => sharedApiClient.request<void>(API_PATHS.ops.textModelDetail, { method: 'DELETE', pathParams: { model_id } }),
  setDefaultTextModel: (model_id: string | number) => sharedApiClient.request<TextModel>(API_PATHS.ops.textModelDefault, { method: 'PUT', pathParams: { model_id } }),
  testTextModel: (model_id: string | number) => sharedApiClient.request<TextModelConnectionTest>(API_PATHS.ops.textModelTest, { method: 'POST', pathParams: { model_id } }),
  modelAccountTestImageUrl: (path: string, accessToken?: string | null) => mediaAssetURL(path, accessToken),
  listRouteModels: async (query: Record<string, string | number | boolean | undefined> = {}) => (normalizePage<any>(await sharedApiClient.request(API_PATHS.ops.routeModels, { query }))).items.map(toRouteModel),
  createRouteModel: async (input: RouteModelWriteRequest) => toRouteModel(await sharedApiClient.request(API_PATHS.ops.routeModels, { method: 'POST', body: input })),
  updateRouteModel: async (route_model_id: string | number, input: Partial<RouteModelWriteRequest>) => toRouteModel(await sharedApiClient.request(API_PATHS.ops.routeModelDetail, { method: 'PUT', pathParams: { route_model_id }, body: input })),
  deleteRouteModel: (route_model_id: string | number) => sharedApiClient.request<void>(API_PATHS.ops.routeModelDetail, { method: 'DELETE', pathParams: { route_model_id } }),
  listRouteModelCandidates: async (route_model_id: string | number) => (normalizePage<any>(await sharedApiClient.request(API_PATHS.ops.routeModelCandidates, { pathParams: { route_model_id } }))).items.map((row) => toRouteModelCandidate(row, route_model_id)),
  createRouteModelCandidate: async (route_model_id: string | number, input: RouteModelCandidateWriteRequest) => toRouteModelCandidate(await sharedApiClient.request(API_PATHS.ops.routeModelCandidates, { method: 'POST', pathParams: { route_model_id }, body: input }), route_model_id),
  updateRouteModelCandidate: async (route_model_id: string | number, candidate_id: string | number, input: Partial<RouteModelCandidateWriteRequest>) => toRouteModelCandidate(await sharedApiClient.request(API_PATHS.ops.routeModelCandidateDetail, { method: 'PUT', pathParams: { route_model_id, candidate_id }, body: input }), route_model_id),
  deleteRouteModelCandidate: (route_model_id: string | number, candidate_id: string | number) => sharedApiClient.request<void>(API_PATHS.ops.routeModelCandidateDetail, { method: 'DELETE', pathParams: { route_model_id, candidate_id } }),
  listRouteModelPrices: async (query: Record<string, string | number | boolean | undefined> = {}) => (normalizePage<any>(await sharedApiClient.request(API_PATHS.ops.routeModelPrices, { query }))).items.map(toRouteModelPrice),
  createRouteModelPrice: async (input: RouteModelPriceWriteRequest) => toRouteModelPrice(await sharedApiClient.request(API_PATHS.ops.routeModelPrices, { method: 'POST', body: input })),
  updateRouteModelPrice: async (price_id: string | number, input: Partial<RouteModelPriceWriteRequest>) => toRouteModelPrice(await sharedApiClient.request(API_PATHS.ops.routeModelPriceDetail, { method: 'PUT', pathParams: { price_id }, body: input })),
  deleteRouteModelPrice: (price_id: string | number) => sharedApiClient.request<void>(API_PATHS.ops.routeModelPriceDetail, { method: 'DELETE', pathParams: { price_id } }),
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

function toDashboardOperations(raw: any) {
  return {
    today_order_count: Number(raw?.today_order_count ?? 0),
    payment_success_rate: raw?.payment_success_rate ?? '0.00%',
    failed_webhook_count: Number(raw?.failed_webhook_count ?? 0),
    refund_compensation_failed_count: Number(raw?.refund_compensation_failed_count ?? 0),
    refund_compensation_oldest_failed_at: raw?.refund_compensation_oldest_failed_at ?? null,
    mock_enabled: Boolean(raw?.mock_enabled),
    signup_trial_granted_user_count: Number(raw?.signup_trial_granted_user_count ?? 0),
    trial_expiring_user_count: Number(raw?.trial_expiring_user_count ?? 0),
    preflight_failure_count: Number(raw?.preflight_failure_count ?? 0),
    preflight_failures_by_error_code: raw?.preflight_failures_by_error_code ?? {},
    platform_loss_count: Number(raw?.platform_loss_count ?? 0),
    platform_loss_provider_cost: String(raw?.platform_loss_provider_cost ?? '0.00000'),
    public_gallery_list_views: Number(raw?.public_gallery_list_views ?? 0),
    public_gallery_detail_login_blocks: Number(raw?.public_gallery_detail_login_blocks ?? 0),
    enabled_payment_methods: Array.isArray(raw?.enabled_payment_methods) ? raw.enabled_payment_methods : [],
    generated_at: raw?.generated_at ?? new Date(0).toISOString(),
  }
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

function toReadinessReport(raw: any): ReadinessReport {
  const checks = Array.isArray(raw.checks) ? raw.checks : Array.isArray(raw.items) ? raw.items : []
  const normalizedChecks = checks.map((item: any) => ({
    ...item,
    detail: item.detail ?? item.summary ?? '',
    summary: item.summary ?? item.detail ?? '',
    fix_route: item.fix_route ?? item.action_route,
    fix_action: item.fix_action ?? item.action_label,
    action_route: item.action_route ?? item.fix_route,
    action_label: item.action_label ?? item.fix_action,
  }))
  return {
    ...raw,
    status: raw.status ?? raw.overall_status ?? 'warn',
    overall_status: raw.overall_status ?? raw.status ?? 'warn',
    generated_at: raw.generated_at ?? new Date().toISOString(),
    summary: raw.summary,
    checks: normalizedChecks,
    items: normalizedChecks,
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
  const rawBalance = raw.balance
  const balance = typeof rawBalance === 'object' && rawBalance !== null ? rawBalance.available_points : rawBalance
  const id = raw.id ?? raw.user_id ?? raw.user?.id
  const email = raw.email ?? raw.user?.email ?? ''
  const name = raw.display_name ?? raw.nickname ?? raw.user?.nickname ?? email?.split('@')[0] ?? `User ${id}`
  const rawGroups = raw.user_groups ?? raw.groups ?? raw.memberships ?? []
  const normalizedGroups = Array.isArray(rawGroups) ? rawGroups.map(toUserGroup) : []
  const groupCodes = normalizedGroups.map((group) => group.code)
  return {
    ...raw,
    id: String(id ?? ''),
    email,
    display_name: name,
    status: raw.status ?? raw.user?.status ?? 'active',
    group: groupCodes.length ? groupCodes.join(', ') : raw.user_group_code ?? raw.group ?? raw.user?.user_group_code ?? 'basic',
    user_group_codes: groupCodes.length ? groupCodes : raw.user_group_codes ?? (raw.user_group_code ? [raw.user_group_code] : undefined),
    user_groups: normalizedGroups,
    balance: String(balance ?? raw.available_points ?? raw.user?.available_points ?? '0.00000'),
    created_at: raw.created_at ?? '',
    last_seen_at: raw.last_seen_at ?? raw.updated_at ?? '',
  }
}

function toSystemAdminUser(raw: any): SystemAdminUser {
  return {
    ...raw,
    id: raw.id ?? raw.admin_id,
    email: raw.email ?? '',
    role: raw.role ?? 'admin',
    status: raw.status ?? 'active',
    created_at: raw.created_at ?? '',
    updated_at: raw.updated_at ?? '',
  }
}

function toUserGroup(raw: any): UserGroup {
  const code = raw.code ?? raw.group_code ?? ''
  const name = raw.name ?? raw.group_name ?? code
  return {
    ...raw,
    id: raw.id ?? raw.group_id ?? code,
    code,
    name,
    group_code: code,
    group_name: name,
    multiplier: String(raw.multiplier ?? '1.00000'),
    status: raw.status ?? 'enabled',
    sort_order: Number(raw.sort_order ?? 0),
    is_default: Boolean(raw.is_default ?? false),
    description: raw.description ?? null,
    created_at: raw.created_at ?? '',
    updated_at: raw.updated_at ?? '',
  }
}

function toUserGroupPayload(group: Partial<UserGroupWriteRequest>) {
  return {
    group_code: group.code,
    group_name: group.name,
    multiplier: group.multiplier,
    status: group.status,
    sort_order: group.sort_order,
    is_default: group.is_default,
    description: group.description,
  }
}

function toModelAccount(raw: any): ModelAccount {
  return {
    ...raw,
    id: raw.id ?? raw.account_id,
    name: raw.name ?? raw.account_name ?? '',
    adapter_type: raw.adapter_type ?? 'openai_compatible',
    auth_type: raw.auth_type ?? 'api_key',
    base_url: raw.base_url ?? '',
    credentials_status: raw.credentials_status ?? { has_api_key: Boolean(raw.has_api_key) },
    status: raw.status ?? (raw.enabled === false ? 'disabled' : 'enabled'),
    priority: Number(raw.priority ?? 1),
    weight: Number(raw.weight ?? 100),
    concurrency_limit: Number(raw.concurrency_limit ?? 1),
    timeout_ms: Number(raw.timeout_ms ?? 120000),
    error_message: raw.error_message ?? null,
    last_used_at: raw.last_used_at ?? null,
    extra: raw.extra ?? {},
    created_at: raw.created_at ?? '',
    updated_at: raw.updated_at ?? '',
  }
}

function toModelAccountModel(raw: any, accountId?: string | number): ModelAccountModel {
  return {
    ...raw,
    id: raw.id ?? raw.model_id,
    account_id: raw.account_id ?? accountId ?? raw.model_account_id,
    account_name: raw.account_name ?? raw.model_account?.name,
    model_code: raw.model_code ?? '',
    display_name: raw.display_name ?? raw.name ?? raw.model_code ?? '',
    task_types: raw.task_types ?? ['text_to_image'],
    qualities: raw.qualities ?? raw.supported_qualities ?? ['auto'],
    supported_ratios: Array.isArray(raw.supported_ratios) && raw.supported_ratios.length ? raw.supported_ratios : ['1:1'],
    max_image_count: Math.max(1, Number(raw.max_image_count ?? 1) || 1),
    max_reference_image_count: Math.max(0, Number(raw.max_reference_image_count ?? 0) || 0),
    cost_per_image: String(raw.cost_per_image ?? raw.output_cost ?? '0.00000'),
    currency: raw.currency ?? 'USD',
    enabled: Boolean(raw.enabled ?? true),
    extra: raw.extra ?? {},
    created_at: raw.created_at ?? '',
    updated_at: raw.updated_at ?? '',
  }
}

function toRouteModel(raw: any): RouteModel {
  return {
    ...raw,
    id: raw.id ?? raw.route_model_id,
    code: raw.code ?? raw.route_model_code ?? '',
    name: raw.name ?? raw.display_name ?? raw.code ?? '',
    description: raw.description ?? '',
    visibility: raw.visibility ?? 'hidden',
    enabled: Boolean(raw.enabled ?? true),
    sort_order: Number(raw.sort_order ?? 0),
    group_ids: raw.group_ids ?? (raw.groups ?? []).map((group: any) => group.id ?? group.group_id ?? group.code),
    groups: (raw.groups ?? []).map(toUserGroup),
    candidates: (raw.candidates ?? []).map((row: any) => toRouteModelCandidate(row, raw.id ?? raw.route_model_id)),
    prices: (raw.prices ?? []).map(toRouteModelPrice),
    created_at: raw.created_at ?? '',
    updated_at: raw.updated_at ?? '',
  }
}

function toRouteModelCandidate(raw: any, routeModelId?: string | number): RouteModelCandidate {
  const accountModel = raw.account_model ? toModelAccountModel(raw.account_model) : undefined
  return {
    ...raw,
    id: raw.id ?? raw.candidate_id,
    route_model_id: raw.route_model_id ?? routeModelId,
    account_model_id: raw.account_model_id ?? accountModel?.id,
    account_model: accountModel,
    model_code: raw.model_code ?? accountModel?.model_code,
    account_name: raw.account_name ?? accountModel?.account_name,
    priority: Number(raw.priority ?? 1),
    weight: Number(raw.weight ?? 100),
    fallback_order: Number(raw.fallback_order ?? 1),
    enabled: Boolean(raw.enabled ?? true),
    created_at: raw.created_at ?? '',
    updated_at: raw.updated_at ?? '',
  }
}

function toRouteModelPrice(raw: any): RouteModelPrice {
  return {
    ...raw,
    id: raw.id ?? raw.price_id,
    route_model_id: raw.route_model_id,
    route_model_code: raw.route_model_code ?? raw.route_model?.code,
    route_model_name: raw.route_model_name ?? raw.route_model?.name,
    task_type: raw.task_type ?? 'text_to_image',
    base_resolution: raw.base_resolution ?? 'auto',
    base_points: String(raw.base_points ?? '0.00000'),
    reference_multiplier: String(raw.reference_multiplier ?? '1.00000'),
    enabled: Boolean(raw.enabled ?? true),
    created_at: raw.created_at ?? '',
    updated_at: raw.updated_at ?? '',
  }
}

function toAdminUserDetail(raw: AdminUserDetail | any): AdminUserDetail {
  if (raw?.user) {
    return {
      ...raw,
      user: toAdminUser({
        ...raw.user,
        balance: raw.balance?.available_points ?? raw.user.balance,
      }),
      recent_ledger: raw.recent_ledger ?? [],
      recent_orders: raw.recent_orders ?? [],
      recent_tasks: raw.recent_tasks ?? [],
      api_keys: raw.api_keys ?? [],
    }
  }
  return { user: toAdminUser(raw), balance: raw.balance, recent_ledger: raw.recent_ledger ?? [], recent_orders: raw.recent_orders ?? [], recent_tasks: raw.recent_tasks ?? [], api_keys: raw.api_keys ?? [] }
}

function toAudit(raw: any): AuditLog {
  const id = raw.id ?? raw.ID
  const actorType = raw.actor_type ?? raw.ActorType
  const actorId = raw.actor_id ?? raw.ActorID
  const action = raw.action ?? raw.Action ?? ''
  const targetType = raw.target_type ?? raw.TargetType
  const targetId = raw.target_id ?? raw.TargetID
  const result = raw.result ?? raw.Result
  const metadata = raw.metadata ?? raw.Metadata ?? {}
  const actor = raw.actor ?? [actorType, actorId].filter(Boolean).join(':')
  const target = raw.target ?? [targetType, targetId].filter(Boolean).join(':')
  const detail = raw.detail ?? result ?? (Object.keys(metadata).length ? JSON.stringify(metadata) : '')
  return {
    ...raw,
    id: String(id),
    actor,
    action,
    target,
    detail,
    actor_type: actorType,
    actor_id: actorId,
    target_type: targetType,
    target_id: targetId,
    result,
    metadata,
    ip_addr: raw.ip_addr ?? raw.IPAddr,
    user_agent: raw.user_agent ?? raw.UserAgent,
    created_at: raw.created_at ?? raw.CreatedAt ?? '',
    updated_at: raw.updated_at ?? raw.UpdatedAt ?? '',
  }
}

function toReview(raw: any): ReviewItem {
  const status = normalizeReviewStatus(raw.status ?? raw.visibility_status)
  return {
    ...raw,
    id: String(raw.id ?? raw.image_id),
    image_id: String(raw.image_id ?? raw.id),
    title: raw.title ?? raw.prompt?.slice(0, 32) ?? '公开图片',
    owner: raw.owner ?? raw.author_name ?? raw.user_id ?? '',
    task_type: raw.task_type ?? 'text_to_image',
    image_url: raw.image_url ?? raw.download_url ?? raw.url ?? '',
    status,
    reason: raw.reason ?? raw.review_reason ?? '',
    created_at: raw.created_at ?? '',
  }
}

function normalizeReviewStatus(status: unknown) {
  const value = String(status ?? '').trim()
  if (value === 'pending') return 'pending_review'
  return value || 'private'
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
