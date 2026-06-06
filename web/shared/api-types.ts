export type ApiMeta = { request_id?: string }
export type ApiEnvelope<T> = { data: T; meta?: ApiMeta; code?: string; message?: string; request_id?: string }
export type LegacyApiEnvelope<T> = { code: string; message: string; data: T; request_id: string }
export type ApiErrorPayload = { error: { code?: string; message?: string; status_code?: number }; meta?: ApiMeta }

export const API_PATHS = {
  agent: {
    sendEmailCode: '/api/agent/auth/v1/email/send-code',
    loginEmailCode: '/api/agent/auth/v1/login/email-code',
    loginPassword: '/api/agent/auth/v1/login/password',
    refreshSession: '/api/agent/auth/v1/session/refresh',
    logout: '/api/agent/auth/v1/logout',
    passwordChange: '/api/agent/auth/v1/password/change',
    passwordReset: '/api/agent/auth/v1/password/reset',
    passwordResetRequest: '/api/agent/auth/v1/password/reset/request',
    passwordResetConfirm: '/api/agent/auth/v1/password/reset/confirm',
    profile: '/api/agent/user/v1/profile',
    preferences: '/api/agent/user/v1/preferences',
    avatar: '/api/agent/user/v1/avatar',
    closeAccount: '/api/agent/user/v1/account/close',
    accountClose: '/api/agent/user/v1/account/close',
    balance: '/api/agent/billing/v1/balance',
    ledger: '/api/agent/billing/v1/ledger',
    plans: '/api/agent/billing/v1/plans',
    subscription: '/api/agent/billing/v1/subscription',
    orders: '/api/agent/billing/v1/orders',
    orderDetail: '/api/agent/billing/v1/orders/{order_id}',
    orderCancel: '/api/agent/billing/v1/orders/{order_id}/cancel',
    cashierOptions: '/api/agent/cashier/v1/options',
    cashierOrders: '/api/agent/cashier/v1/orders',
    cashierOrderDetail: '/api/agent/cashier/v1/orders/{order_id}',
    cashierOrderCancel: '/api/agent/cashier/v1/orders/{order_id}/cancel',
    cashierOrderMockPay: '/api/agent/cashier/v1/orders/{order_id}/mock-pay',
    estimate: '/api/agent/billing/v1/estimate',
    redeemCode: '/api/agent/billing/v1/redeem-codes/redeem',
    capabilities: '/api/agent/image/v1/capabilities',
    referenceAssets: '/api/agent/image/v1/reference-assets',
    referenceAssetDetail: '/api/agent/image/v1/reference-assets/{asset_id}',
    referenceAssetDownload: '/api/agent/image/v1/reference-assets/{asset_id}/download',
    imageDownload: '/api/agent/image/v1/images/{image_id}',
    tasks: '/api/agent/image/v1/tasks',
    taskDetail: '/api/agent/image/v1/tasks/{task_id}',
    taskEvents: '/api/agent/image/v1/tasks/{task_id}/events',
    taskStream: '/api/agent/image/v1/tasks/events',
    historyTasks: '/api/agent/image/v1/history/tasks',
    historyTaskDetail: '/api/agent/image/v1/history/tasks/{task_id}',
    galleryImages: '/api/agent/gallery/v1/images',
    galleryImageDetail: '/api/agent/gallery/v1/images/{image_id}',
    galleryImageGroup: '/api/agent/gallery/v1/images/{image_id}/group',
    publishImage: '/api/agent/gallery/v1/images/{image_id}/publish',
    likePublicImage: '/api/agent/gallery/v1/images/{image_id}/like',
    favoritePublicImage: '/api/agent/gallery/v1/images/{image_id}/favorite',
    apiKeys: '/api/agent/account/v1/api-keys',
    apiKeyDetail: '/api/agent/account/v1/api-keys/{key_id}',
    apiKeyResetSecret: '/api/agent/account/v1/api-keys/{key_id}/reset-secret',
    developerApiKeys: '/api/agent/developer/v1/api-keys',
    developerApiKeyDetail: '/api/agent/developer/v1/api-keys/{key_id}',
    developerApiKeyResetSecret: '/api/agent/developer/v1/api-keys/{key_id}/reset-secret',
  },
  open: {
    uploadSessions: '/api/open/image/v1/reference-assets/uploads',
    referenceAssets: '/api/open/image/v1/reference-assets',
    referenceAssetDetail: '/api/open/image/v1/reference-assets/{asset_id}',
    tasks: '/api/open/image/v1/tasks',
    taskDetail: '/api/open/image/v1/tasks/{task_id}',
    balance: '/api/open/image/v1/balance',
    capabilities: '/api/open/image/v1/capabilities',
    estimate: '/api/open/image/v1/estimate',
    galleryImages: '/api/open/image/v1/gallery/images',
    galleryImageDetail: '/api/open/image/v1/gallery/images/{image_id}',
    galleryImageDownload: '/api/open/image/v1/gallery/images/{image_id}/image',
    paymentWebhook: '/api/open/image/v1/payments/webhooks/{channel}',
  },
  compat: {
    generations: '/v1/images/generations',
    edits: '/v1/images/edits',
    models: '/v1/models',
  },
  ops: {
    login: '/api/ops/admin/v1/auth/login',
    logout: '/api/ops/admin/v1/auth/logout',
    auditLogs: '/api/ops/admin/v1/audit-logs',
    users: '/api/ops/admin/v1/users',
    userDetail: '/api/ops/admin/v1/users/{user_id}',
    userStatus: '/api/ops/admin/v1/users/{user_id}/status',
    userPointsAdjustments: '/api/ops/admin/v1/users/{user_id}/points-adjustments',
    userPoints: '/api/ops/admin/v1/users/{user_id}/points-adjustments',
    userResetPassword: '/api/ops/admin/v1/users/{user_id}/reset-password',
    userLimits: '/api/ops/admin/v1/users/{user_id}/limits',
    userGroupAssignment: '/api/ops/admin/v1/users/{user_id}/groups',
    userGroupAssign: '/api/ops/admin/v1/users/{user_id}/groups',
    userGroups: '/api/ops/admin/v1/user-groups',
    userGroupDetail: '/api/ops/admin/v1/user-groups/{group_id}',
    redeemCodes: '/api/ops/admin/v1/redeem-codes',
    redeemCodesBatchCreate: '/api/ops/admin/v1/redeem-codes:batch-create',
    redeemCodesExport: '/api/ops/admin/v1/redeem-codes:export',
    redeemCodeStatus: '/api/ops/admin/v1/redeem-codes/{code_id}/status',
    redeemCodeRedemptions: '/api/ops/admin/v1/redeem-codes/{code_id}/redemptions',
    callRecords: '/api/ops/admin/v1/call-records',
    modelAccounts: '/api/ops/admin/v1/model-accounts',
    modelAccountDetail: '/api/ops/admin/v1/model-accounts/{account_id}',
    modelAccountModels: '/api/ops/admin/v1/model-accounts/{account_id}/models',
    modelAccountModelDetail: '/api/ops/admin/v1/model-accounts/{account_id}/models/{model_id}',
    routeModels: '/api/ops/admin/v1/route-models',
    routeModelDetail: '/api/ops/admin/v1/route-models/{route_model_id}',
    routeModelCandidates: '/api/ops/admin/v1/route-models/{route_model_id}/candidates',
    routeModelCandidateDetail: '/api/ops/admin/v1/route-models/{route_model_id}/candidates/{candidate_id}',
    routeModelPrices: '/api/ops/admin/v1/route-model-prices',
    routeModelPriceDetail: '/api/ops/admin/v1/route-model-prices/{price_id}',
    modelProviders: '/api/ops/admin/v1/model-providers',
    modelProviderDetail: '/api/ops/admin/v1/model-providers/{provider_code}',
    providerModels: '/api/ops/admin/v1/provider-models',
    providerModelDetail: '/api/ops/admin/v1/provider-models/{provider_model_id}',
    modelRoutes: '/api/ops/admin/v1/model-routes',
    modelRouteDetail: '/api/ops/admin/v1/model-routes/{route_id}',
    configTabs: '/api/ops/admin/v1/config-tabs',
    configTabDetail: '/api/ops/admin/v1/config-tabs/{tab_key}',
    imageReviews: '/api/ops/admin/v1/image-reviews',
    imageReviewImage: '/api/ops/admin/v1/image-reviews/{image_id}/image',
    imageReviewApprove: '/api/ops/admin/v1/image-reviews/{image_id}:approve',
    imageReviewReject: '/api/ops/admin/v1/image-reviews/{image_id}:reject',
    imageReviewUnpublish: '/api/ops/admin/v1/image-reviews/{image_id}:unpublish',
    dashboard: '/api/ops/admin/v1/metrics/dashboard',
    readiness: '/api/ops/admin/v1/readiness',
    cashierOverview: '/api/ops/admin/v1/cashier/overview',
    cashierPlans: '/api/ops/admin/v1/cashier/plans',
    cashierPlanDetail: '/api/ops/admin/v1/cashier/plans/{plan_id}',
    cashierCustomAmountConfig: '/api/ops/admin/v1/cashier/custom-amount-config',
    paymentVisibleMethods: '/api/ops/admin/v1/cashier/visible-methods',
    paymentProviderInstances: '/api/ops/admin/v1/cashier/provider-instances',
    paymentProviderInstanceDetail: '/api/ops/admin/v1/cashier/provider-instances/{instance_id}',
    paymentOrders: '/api/ops/admin/v1/cashier/orders',
    paymentOrderDetail: '/api/ops/admin/v1/cashier/orders/{order_id}',
    paymentOrderComplete: '/api/ops/admin/v1/cashier/orders/{order_id}/complete',
    paymentOrderRefund: '/api/ops/admin/v1/cashier/orders/{order_id}/refund',
    paymentOrderChargeback: '/api/ops/admin/v1/cashier/orders/{order_id}/chargeback',
    paymentOrderSync: '/api/ops/admin/v1/cashier/orders/{order_id}/sync',
    paymentWebhookEvents: '/api/ops/admin/v1/cashier/webhook-events',
    paymentWebhookEventRetry: '/api/ops/admin/v1/cashier/webhook-events/{event_id}/retry',
    docsOpenAPIYAML: '/docs/openapi.yaml',
    docsOpenAPIJSON: '/docs/openapi.json',
    docsExamples: '/docs/examples',
    docsErrors: '/docs/errors',
  },
  docs: {
    openapiYaml: '/docs/openapi.yaml',
    openapiJson: '/docs/openapi.json',
    examples: '/docs/examples',
    errors: '/docs/errors',
  },
} as const

export type ID = string | number
export type PageQuery = { page?: number; page_size?: number }
export type Pagination = { page: number; page_size: number; total: number }
export type ApiPagination = Pagination
export type PagedResponse<T> = { items: T[]; pagination: Pagination }
export type PageResult<T> = { items: T[]; total: number; next_cursor?: string; pagination?: Pagination }

export type ImageTaskType = 'text_to_image' | 'reference_to_image' | 'image_edit'
export type ImageTaskStatus = 'queued' | 'running' | 'succeeded' | 'partial_failed' | 'failed' | 'cancelled' | 'rejected' | 'deleted'
export type PublishStatus = 'private' | 'reviewing' | 'pending_review' | 'public' | 'approved' | 'rejected' | 'unpublished'

export type GenerationPreferences = { model_group: string; quality: string; aspect_ratio: string; image_count: number }
export type UserProfile = {
  id: ID
  email: string
  nickname?: string
  bio?: string
  avatar_object_key?: string
  user_group_code?: string
  theme?: string
  default_locale?: string
  display_name: string
  avatar_initials: string
  tier: 'FREE' | 'PLUS' | 'PRO'
  group: string
  signature: string
  preferences: GenerationPreferences
}

export type SendEmailCodeRequest = { email: string; scene?: 'login' | 'register' | 'password_reset' | string }
export type SendEmailCodeResponse = { email: string; scene: string; status: string; cooldown_seconds?: number }
export type PasswordLoginRequest = { email: string; password: string }
export type EmailCodeLoginRequest = { email: string; code: string }
export type ChangePasswordRequest = { old_password: string; new_password: string }
export type PasswordResetRequest = { email: string }
export type PasswordResetConfirmRequest = { email: string; code: string; new_password: string }
export type CloseAccountResponse = { id: ID; status: string; closed_at?: string | null }
export type UpdateProfileRequest = { nickname?: string; bio?: string; avatar_object_key?: string; default_locale?: string; theme?: string }
export type UpdatePreferencesRequest = { theme?: string; default_locale?: string }

export type GrantExpirySummary = { grant_id: number; grant_type: string; available_points: string; expires_at?: string | null }
export type BalanceBucketType = 'trial' | 'subscription' | 'recharge' | string
export type BalanceBucket = {
  bucket: BalanceBucketType
  label?: string
  available_points: string
  frozen_points?: string
  expires_at?: string | null
  next_expiring_at?: string | null
  expire_warning?: boolean
  source_type?: 'signup' | 'payment_order' | 'redeem_code' | 'admin_adjust' | 'subscription' | string
  sort_order?: number
}
export type SubscriptionSummary = {
  id: number
  plan_id: number
  plan_code: string
  plan_name: string
  status: string
  started_at: string
  current_period_start: string
  current_period_end: string
  expired_at?: string | null
  canceled_at?: string | null
  granted_points: string
  remaining_points: string
}
export type Balance = {
  available_points: string
  frozen_points: string
  trial_points?: string
  subscription_points?: string
  gift_points?: string
  recharge_points?: string
  buckets?: BalanceBucket[]
  user_group_multiplier?: string
  cny_per_point?: string
  active_subscription?: SubscriptionSummary | null
  next_expiring_grant?: GrantExpirySummary | null
  plan_name: string
  first_purchase_bonus: boolean
}
export type SignupGrantResult = { granted: boolean; balance: Balance }
export type LoginResponse = { access_token: string; expires_in_seconds: number; expires_in?: number; user_id: ID; profile?: UserProfile; signup_grant?: SignupGrantResult }
export type SubscriptionPlan = {
  id: number
  plan_code: string
  plan_name: string
  status: string
  price_cny: string
  points: string
  bonus_points: string
  duration_days: number
  currency: string
  description?: string
  created_at: string
  updated_at: string
}
export type BillingPlan = SubscriptionPlan
export type Subscription = SubscriptionSummary
export type CashierPlan = SubscriptionPlan & {
  plan_type?: 'points_package' | 'subscription' | string
  purchase_enabled?: boolean
  sort_order?: number
}
export type CashierCustomAmountConfig = {
  enabled: boolean
  min_amount_cny: string
  max_amount_cny: string
  cny_per_point: string
  bonus_rule?: Record<string, unknown> | null
}
export type PaymentVisibleMethod = {
  method: 'alipay' | 'wxpay' | string
  label: string
  enabled: boolean
  source_provider_type?: PaymentProviderType
  scheduler_strategy?: PaymentSchedulerStrategy
  display_order: number
  description?: string
}
export type CashierOptions = {
  plans: CashierPlan[]
  custom_amount: CashierCustomAmountConfig
  visible_methods: PaymentVisibleMethod[]
  order_timeout_seconds: number
}
export type CashierPurchaseType = 'plan' | 'custom_amount' | string
export type PaymentProviderType = 'alipay_direct' | 'wxpay_direct' | 'easypay_alipay' | 'easypay_wxpay' | 'mock' | 'jeepay_alipay' | 'jeepay_wxpay' | string
export type PaymentSchedulerStrategy = 'round_robin' | 'random' | string
export type PaymentDisplay = {
  type: 'qr_code' | 'redirect' | 'form_html' | 'form' | 'jsapi' | 'mock' | 'none' | string
  qr_code?: string
  payment_url?: string
  client_token?: string
  form_html?: string
  expires_at?: string | null
}
export type PaymentOrder = {
  id: number
  order_no: string
  user_id?: number
  plan_id: number
  plan_code: string
  plan_name: string
  provider: string
  purchase_type?: CashierPurchaseType
  visible_method?: string
  provider_type?: PaymentProviderType
  provider_instance_id?: ID
  status: string
  currency: string
  amount_cny: string
  points: string
  bonus_points: string
  trade_no?: string
  refund_trade_no?: string
  refunded_amount_cny?: string
  refunded_points?: string
  payment_url?: string
  qr_code?: string
  client_token?: string
  payment_display?: PaymentDisplay
  failure_reason?: string
  ledger_id?: ID | null
  expires_at: string
  paid_at?: string | null
  completed_at?: string | null
  closed_at?: string | null
  refunded_at?: string | null
  created_at: string
  updated_at: string
}
export type CreatePaymentOrderRequest = { plan_code: string; provider: string }
export type CompletePaymentOrderRequest = {
  provider?: string
  trade_no: string
  reason?: string
}
export type RefundPaymentOrderRequest = {
  refund_trade_no: string
  refund_amount_cny?: string
  reason?: string
}
export type ChargebackPaymentOrderRequest = {
  charge_points: string
  reason: string
}
export type PaymentOrderChargebackResponse = {
  order: PaymentOrder
  balance: Balance
}
export type PaymentOrderSyncStatus = 'pending' | 'paid' | 'closed' | 'failed' | 'refunded'
export type PaymentOrderSyncResult = {
  provider_type: PaymentProviderType
  provider_instance_id?: ID
  query_status: PaymentOrderSyncStatus
  paid: boolean
  completed: boolean
  trade_no?: string
  amount_cny?: string
  message?: string
  raw?: Record<string, unknown>
  synced_at: string
}
export type PaymentOrderSyncResponse = {
  order: PaymentOrder
  sync: PaymentOrderSyncResult
}
export type CreateCashierOrderRequest = {
  purchase_type: CashierPurchaseType
  plan_code?: string
  amount_cny?: string
  visible_method: string
  client_return_url?: string
}
export type CashierOrder = Omit<PaymentOrder, 'plan_id' | 'plan_code' | 'plan_name' | 'provider'> & {
  plan_id?: number
  plan_code?: string
  plan_name?: string
  provider?: string
  purchase_type: CashierPurchaseType
  visible_method: string
  provider_type?: PaymentProviderType
  provider_instance_id?: ID
  payment_display?: PaymentDisplay
}
export type LedgerEntry = {
  id: ID
  user_id?: number
  api_key_id?: number
  task_id?: string
  redeem_code_id?: number
  ledger_type?: string
  change_points?: string
  balance_after?: string
  frozen_after?: string
  balance_bucket?: BalanceBucketType
  bucket_type?: BalanceBucketType
  bucket_balance_after?: string
  source_type?: 'signup' | 'payment_order' | 'task' | 'redeem_code' | 'admin' | 'subscription' | string
  source_id?: string | number | null
  expires_at?: string | null
  reason?: string
  created_at?: string
  title?: string
  occurred_at?: string
  amount?: string
  type?: 'credit' | 'debit'
  detail?: string
}

export type CapabilityItem = {
  route_model_code: string
  task_types: ImageTaskType[]
  qualities: string[]
  aspect_ratios: string[]
  max_output_image_count: number
  max_reference_image_count: number
}
export type RouteModelPriceQuote = {
  task_type: ImageTaskType
  quality: string
  base_points: string
  charged_points: string
  display_points: string
  reference_multiplier?: string
}
export type CapabilityModelGroup = {
  id: string
  code: string
  name: string
  description?: string
  task_types: ImageTaskType[]
  qualities: string[]
  aspect_ratios?: string[]
  max_output_image_count?: number
  max_reference_image_count?: number
  effective_multiplier?: string
  prices: RouteModelPriceQuote[]
  supports_reference: boolean
  display_points?: string
}
export type Capability = {
  items?: CapabilityItem[]
  raw?: unknown
  model_groups: CapabilityModelGroup[]
  qualities: string[]
  aspect_ratios: string[]
  max_image_count: number
  task_types: ImageTaskType[]
}
export type EstimateRequest = { task_type: ImageTaskType; route_model_code: string; quality: string; aspect_ratio: string; image_count: number; reference_asset_ids?: string[]; model_group?: string }
export type BackendEstimateRequest = { task_type: ImageTaskType; route_model_code: string; requested_quality: string; requested_size: string; requested_output_image_count: number; reference_image_count?: number }
export type EstimateResult = {
  resolved_quality_bucket?: string
  estimated_points?: string
  charged_points?: string
  display_points?: string
  user_group_multiplier?: string
  requested_output_image_count?: number
  reference_image_count?: number
  balance?: Balance
  insufficient_points?: string
  points: string
  formula: string
  resolved_quality: string
  sufficient: boolean
}

export type ReferenceAsset = {
  id: string
  name?: string
  preview_url?: string
  status: 'uploaded' | 'processing' | 'ready' | 'failed' | string
  size_bytes?: number
  mime_type?: string
  file_size_bytes?: number
  width?: number
  height?: number
  sha256?: string
  storage_driver?: string
  object_key?: string
  created_at: string
}
export type ImageResult = {
  id: string
  url: string
  download_url?: string
  mime_type?: string
  file_size_bytes?: number
  sha256?: string
  object_key?: string
  storage_driver?: string
  width: number
  height: number
  visibility_status?: PublishStatus
  review_reason?: string
  published_at?: string | null
  publish_status: PublishStatus
  prompt?: string
  prompt_excerpt?: string
  task_type?: ImageTaskType
  quality?: string
  aspect_ratio?: string
  route_model_code?: string
  abstract_model?: string
  author_name?: string
  like_count?: number
  favorite_count?: number
  liked_by_viewer?: boolean
  favorited_by_viewer?: boolean
  created_at?: string
}
export type ImageTask = {
  id: string
  title: string
  prompt: string
  negative_prompt?: string
  task_type: ImageTaskType
  status: ImageTaskStatus
  abstract_model?: string
  route_model_code?: string
  route_model_name?: string
  model_group: string
  requested_quality?: string
  resolved_quality_bucket?: string
  quality: string
  requested_size?: string
  aspect_ratio: string
  requested_output_image_count?: number
  image_count: number
  reference_image_count?: number
  reference_asset_ids?: string[]
  estimated_points?: string
  actual_points?: string
  estimate_points: string
  progress: number
  provider: string
  provider_model_id?: number
  provider_cost?: string
  gross_margin?: string
  fallback_count?: number
  route_snapshot_version?: string
  route: string
  response_mode?: string
  save_policy?: string
  created_at: string
  updated_at: string
  failure_reason?: string
  error_code?: string
  error_message?: string
  request_id?: string
  reference_assets: ReferenceAsset[]
  results: ImageResult[]
}
export type BackendCreateTaskRequest = BackendEstimateRequest & { prompt: string; reference_asset_ids?: string[]; response_mode?: 'async' }
export type CreateTaskRequest = EstimateRequest & { prompt: string; negative_prompt?: string; idempotency_key?: string; response_mode?: 'sync' | 'async' | string }
export type LoginResult = LoginResponse

export type ApiKey = {
  id: string
  name: string
  access_key: string
  secret?: string
  secret_preview?: string
  status: 'active' | 'disabled' | 'revoked' | string
  group_code?: string
  scopes: string[]
  total_quota_points?: string | null
  daily_quota_points?: string | null
  total_quota_used_points?: string
  daily_quota_used_points?: string
  quota_usage_day?: string | null
  rpm_limit?: number | null
  rpm_window_started_at?: string | null
  rpm_window_count?: number
  expires_at: string | null
  created_at: string
  updated_at?: string
  last_used_at: string | null
}
export type CreateApiKeyRequest = { name: string; group_code?: string; total_quota_points?: string | null; daily_quota_points?: string | null; rpm_limit?: number | null; expires_at?: string | null; scopes?: string[] }
export type UpdateApiKeyRequest = Partial<CreateApiKeyRequest> & { status?: string }

export type EndpointDoc = { group: 'Agent API' | 'Open API' | 'OpenAI Compat' | 'Ops API'; method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'; path: string; title: string; auth: string; requestExample: string; responseExample: string }
export type AdminRole = 'super_admin' | 'admin' | string
export type AdminPermission =
  | 'read:all'
  | 'manage:admins'
  | 'manage:users'
  | 'manage:billing'
  | 'manage:cashier'
  | 'manage:models'
  | 'manage:reviews'
  | 'manage:config'
  | 'manage:dangerous_config'
  | 'view:audit'
  | string
export const ADMIN_PERMISSIONS = {
  readOnly: 'read:all',
  manageAdmins: 'manage:admins',
  manageUsers: 'manage:users',
  manageBilling: 'manage:billing',
  manageCashier: 'manage:cashier',
  manageModels: 'manage:models',
  manageReviews: 'manage:reviews',
  manageConfig: 'manage:config',
  manageDangerousConfig: 'manage:dangerous_config',
  viewAudit: 'view:audit',
} as const satisfies Record<string, AdminPermission>
export type AdminSession = { token: string; access_token?: string; expires_in_seconds?: number; admin_id?: number; email?: string; admin_name: string; role: AdminRole; permissions?: AdminPermission[] }
export type AdminLoginResult = { access_token: string; expires_in_seconds: number; admin_id: number; email: string; role: string; permissions?: AdminPermission[] }
export type AdminMetric = { key?: string; label: string; value: string; trend: string; detail?: string; tone: 'good' | 'warn' | 'bad' | 'danger' | 'neutral' }
export type ProviderHealth = { provider: string; provider_code?: string; provider_type?: string; status: 'healthy' | 'degraded' | 'down' | string; health_status?: string; latency_ms: number; error_rate: string; note: string; enabled?: boolean }
export type AdminDashboardOperations = {
  today_order_count: number
  payment_success_rate: string
  failed_webhook_count: number
  refund_compensation_failed_count: number
  refund_compensation_oldest_failed_at?: string | null
  mock_enabled: boolean
  signup_trial_granted_user_count: number
  trial_expiring_user_count: number
  preflight_failure_count: number
  preflight_failures_by_error_code: Record<string, number>
  public_gallery_list_views: number
  public_gallery_detail_login_blocks: number
  enabled_payment_methods: string[]
  generated_at: string
}

export type ConfigItem = {
  tab: string
  key: string
  value: string
  draft_value: string
  state: 'active' | 'draft' | 'conflict'
  version: number
  description: string
  config_category?: string
  config_key?: string
  config_value?: Record<string, unknown>
  scope?: string
}
export type ConfigTab = { tab_key: string; tab_name: string; version: number; items: ConfigItem[]; read_only?: boolean }
export type UpdateConfigTabRequest = { version: number; items: Array<Pick<ConfigItem, 'config_category' | 'config_key' | 'config_value' | 'scope'>> }
export type ModelProvider = { id: number; provider_code: string; provider_type: string; auth_config_encrypted?: string; health_status: string; enabled: boolean; created_at: string; updated_at: string }
export type ModelProviderWriteRequest = { provider_code: string; provider_type: string; auth_config_encrypted?: string; health_status: string; enabled: boolean }
export type ProviderModel = {
  id: number
  provider_id: number
  provider_code: string
  model_code: string
  compat_mode: string
  supports_image_input: boolean
  supports_mask: boolean
  supported_qualities: string[]
  supported_ratios: string[]
  max_image_count: number
  max_reference_image_count: number
  timeout_ms: number
  input_cost: string
  output_cost: string
  currency: string
  health_status: string
  last_health_checked_at?: string | null
  enabled: boolean
  created_at: string
  updated_at: string
}
export type ProviderModelWriteRequest = Omit<ProviderModel, 'id' | 'provider_id' | 'created_at' | 'updated_at'> & { last_health_checked_at?: string | null }
export type ModelRoute = {
  id: string
  scene: string
  provider: string
  policy: string
  priority: number
  enabled: boolean
  note: string
  group_code?: string
  task_type?: string
  provider_model_id?: number
  provider_code?: string
  weight_percent?: number
  fallback_order?: number
  created_at?: string
  updated_at?: string
}
export type ModelRouteWriteRequest = { group_code: string; task_type: string; provider_model_id: number; provider_code: string; priority: number; weight_percent: number; fallback_order: number; enabled: boolean }
export type PriceRow = { id: string; group: string; q1k: string; q2k: string; q4k: string; reference_multiplier: string; version: number; state: 'active' | 'draft' }
export type GalleryImage = { id: string; task_id: string; user_id?: number; prompt?: string; abstract_model?: string; route_model_code?: string; task_type?: ImageTaskType; task_status?: ImageTaskStatus | string; quality?: string; aspect_ratio?: string; actual_points?: string; reference_asset_ids?: string[]; reference_assets?: ReferenceAsset[]; url?: string; download_url?: string; mime_type?: string; file_size_bytes: number; width: number; height: number; sha256?: string; object_key?: string; storage_driver?: string; image_group?: string; visibility_status: PublishStatus; review_reason?: string; published_at?: string | null; author_name?: string; like_count?: number; favorite_count?: number; liked_by_viewer?: boolean; favorited_by_viewer?: boolean; created_at: string }
export type ReviewItem = { id: string; image_id?: string; title: string; owner: string; task_type: ImageTaskType; image_url: string; status: 'pending' | 'pending_review' | 'approved' | 'rejected' | 'unpublished' | string; reason: string; created_at: string; review_reason?: string; visibility_status?: string }
export type AdminUser = { id: string; email: string; display_name: string; nickname?: string; status: 'active' | 'disabled' | 'pending' | 'closed' | string; group: string; user_group_code?: string; user_group_codes?: string[]; user_groups?: UserGroup[]; balance: string; token_version?: number; rpm_limit?: number; concurrency_limit?: number; default_locale?: string; theme?: string; closed_at?: string | null; created_at: string; updated_at?: string; last_seen_at: string }
export type AdminUserDetail = { user: AdminUser; balance: Balance; recent_ledger: LedgerEntry[]; recent_orders?: PaymentOrder[]; recent_tasks?: ImageTask[]; api_keys?: ApiKey[] }
export type AdminUserCreateRequest = { email: string; nickname?: string; status?: string; user_group_code?: string; password?: string; rpm_limit?: number; concurrency_limit?: number; default_locale?: string; theme?: string }
export type UserGroup = { id?: ID; code: string; name: string; group_code: string; group_name: string; multiplier: string; status: string; sort_order?: number; is_default?: boolean; description?: string | null; created_at: string; updated_at: string }
export type UserGroupWriteRequest = { code: string; name: string; multiplier: string; status: string; sort_order?: number; is_default?: boolean; description?: string | null }
export type ModelAccountStatus = 'enabled' | 'disabled' | 'error' | string
export type ModelAccount = {
  id: ID
  name: string
  adapter_type: 'openai_compatible' | 'openrouter' | string
  auth_type: 'api_key' | string
  base_url: string
  credentials_status?: { has_api_key?: boolean; fingerprint?: string; updated_at?: string | null }
  status: ModelAccountStatus
  priority: number
  weight: number
  concurrency_limit: number
  timeout_ms: number
  error_message?: string | null
  last_used_at?: string | null
  extra?: Record<string, unknown>
  created_at: string
  updated_at: string
}
export type ModelAccountWriteRequest = Omit<Partial<ModelAccount>, 'id' | 'credentials_status' | 'created_at' | 'updated_at'> & { name: string; adapter_type: string; auth_type: string; base_url: string; credentials?: { api_key?: string }; status: string }
export type ModelAccountModel = {
  id: ID
  account_id: ID
  account_name?: string
  model_code: string
  display_name: string
  task_types: ImageTaskType[]
  qualities: string[]
  cost_per_image: string
  currency: string
  enabled: boolean
  extra?: Record<string, unknown>
  created_at: string
  updated_at: string
}
export type ModelAccountModelWriteRequest = Omit<Partial<ModelAccountModel>, 'id' | 'account_id' | 'account_name' | 'created_at' | 'updated_at'> & { model_code: string; display_name: string; task_types: ImageTaskType[]; qualities: string[]; cost_per_image: string; currency: string; enabled: boolean }
export type RouteModelVisibility = 'public' | 'groups' | 'hidden' | string
export type RouteModel = {
  id: ID
  code: string
  name: string
  description?: string
  visibility: RouteModelVisibility
  enabled: boolean
  sort_order: number
  group_ids?: ID[]
  groups?: UserGroup[]
  candidates?: RouteModelCandidate[]
  prices?: RouteModelPrice[]
  created_at: string
  updated_at: string
}
export type RouteModelWriteRequest = { code: string; name: string; description?: string; visibility: RouteModelVisibility; enabled: boolean; sort_order: number; group_ids?: ID[] }
export type RouteModelCandidate = {
  id: ID
  route_model_id: ID
  account_model_id: ID
  account_model?: ModelAccountModel
  model_code?: string
  account_name?: string
  priority: number
  weight: number
  fallback_order: number
  enabled: boolean
  created_at?: string
  updated_at?: string
}
export type RouteModelCandidateWriteRequest = { account_model_id: ID; priority: number; weight: number; fallback_order: number; enabled: boolean }
export type RouteModelPrice = {
  id: ID
  route_model_id: ID
  route_model_code?: string
  route_model_name?: string
  task_type: ImageTaskType
  quality: string
  base_points: string
  reference_multiplier: string
  enabled: boolean
  created_at?: string
  updated_at?: string
}
export type RouteModelPriceWriteRequest = { route_model_id: ID; task_type: ImageTaskType; quality: string; base_points: string; reference_multiplier: string; enabled: boolean }
export type RedeemCode = { id: number; batch_id: number; code: string; status: string; reward_type: string; reward_value: string; valid_from: string; valid_until: string; max_redemptions: number; redeemed_count: number; last_redeemed_by?: number | null; created_at: string; updated_at: string }
export type RedeemCodeCreateRequest = { code: string; batch_id: number; status: string; reward_type: string; reward_value: string; valid_from?: string; valid_until: string; max_redemptions: number }
export type RedeemCodeBatchCreateRequest = Omit<RedeemCodeCreateRequest, 'code'> & { count: number }
export type RedeemCodeBatchCreateResult = { items: RedeemCode[]; count: number; batch_id: number }
export type RedeemCodeExportRequest = { status?: string; code?: string; batch_id?: number }
export type RedeemCodeExportResult = { items: RedeemCode[]; count: number; filters: RedeemCodeExportRequest }
export type CallRecord = { id?: number; task_id: string; user_id: number; api_key_id?: number | null; source_channel: string; task_type: ImageTaskType | string; status: string; provider: string; abstract_model: string; quality: string; requested_output_image_count: number; success_output_image_count: number; reference_image_count: number; estimated_points: string; actual_points: string; provider_cost?: string; gross_margin?: string; error_code?: string | null; error_message?: string | null; created_at: string; updated_at: string; started_at?: string | null; finished_at?: string | null; attempt_count: number }
export type AuditLog = { id: ID; actor: string; action: string; target: string; detail: string; created_at: string; actor_type?: string; actor_id?: string; target_type?: string; target_id?: string; result?: string; metadata?: Record<string, unknown>; ip_addr?: string; user_agent?: string; updated_at?: string }
export type AdminDashboardQueueItem = { item: string; count: string; detail: string }
export type AdminDashboard = { operations: AdminDashboardOperations; metrics: AdminMetric[]; providers: ProviderHealth[]; queue: AdminDashboardQueueItem[]; audit: AuditLog[] }
export type ReadinessStatus = 'pass' | 'warn' | 'fail' | string
export type ReadinessCheck = {
  key: string
  label: string
  status: ReadinessStatus
  detail: string
  summary?: string
  fix_route?: string
  fix_action?: string
  action_route?: string
  action_label?: string
  blocking?: boolean
  checked_at?: string
}
export type ReadinessReport = {
  status: ReadinessStatus
  overall_status?: ReadinessStatus
  generated_at: string
  summary?: { pass: number; warn: number; fail: number }
  checks: ReadinessCheck[]
  items?: ReadinessCheck[]
}
export type CashierOverview = {
  today_order_count: number
  today_completed_count: number
  today_amount_cny: string
  success_rate: string
  pending_count: number
  failed_webhook_count: number
  enabled_methods: string[]
  enabled_provider_instances: number
  mock_enabled: boolean
}
export type PaymentProviderInstance = {
  id: ID
  provider_type: PaymentProviderType
  name: string
  enabled: boolean
  supported_methods: string[]
  sort_order: number
  scheduler_weight: number
  limits?: {
    min_amount_cny?: string
    max_amount_cny?: string
    daily_amount_limit_cny?: string
  }
  config?: Record<string, unknown>
  config_status?: 'configured' | 'missing' | 'invalid' | string
  last_error?: string | null
  created_at?: string
  updated_at?: string
}
export type PaymentProviderInstanceWriteRequest = Omit<Partial<PaymentProviderInstance>, 'id' | 'created_at' | 'updated_at'> & {
  provider_type: PaymentProviderType
  name: string
  enabled: boolean
  supported_methods: string[]
}
export type PaymentWebhookEvent = {
  id: ID
  order_id?: ID
  order_no?: string
  provider_type: PaymentProviderType
  provider_instance_id?: ID
  status: 'received' | 'verified' | 'processed' | 'failed' | string
  event_type?: string
  failure_reason?: string | null
  received_at: string
  processed_at?: string | null
}

export type OpenReferenceAssetUploadSessionRequest = { filename: string; mime_type: string; content_base64: string }
export type OpenReferenceAssetUploadSessionResponse = { asset_id: string; status: string; upload_mode: string; asset: ReferenceAsset }
export type OpenAIImageGenerationRequest = { model: string; prompt: string; size?: string; n?: number; quality?: string; response_format?: 'url' | 'b64_json' | string; user?: string }
export type OpenAIImageEditRequest = { model: string; prompt: string; image: File | Blob | Array<File | Blob>; mask?: File | Blob; size?: string; n?: number; quality?: string; response_format?: 'url' | 'b64_json' | string; user?: string }
export type OpenAIImageResponse = { created?: number; data: Array<{ url?: string; b64_json?: string; revised_prompt?: string }> }
export type OpenAIModelList = { object: string; data: Array<{ id: string; object: string; owned_by: string }> }
