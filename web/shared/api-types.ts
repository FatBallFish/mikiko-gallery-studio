export type ApiMeta = { request_id?: string }
export type FeatureFlags = { video_creation: boolean; creative_canvas: boolean; media_upload: boolean }
export type ApiEnvelope<T> = { data: T; meta?: ApiMeta; code?: string; message?: string; request_id?: string }
export type LegacyApiEnvelope<T> = { code: string; message: string; data: T; request_id: string }
export type ApiErrorPayload = { error: { code?: string; message?: string; status_code?: number }; meta?: ApiMeta }

export type BootstrapPhase = 'setup_required' | 'initializing' | 'restart_pending' | 'ready' | 'broken'
export type BootstrapSetupStatus = {
  phase: 'setup_required' | 'initializing' | 'restart_pending'
  setup_url: string
  operation_id?: string
  retry_after_seconds?: number
}
export type BootstrapReadyStatus = { phase: 'ready'; setup_url?: never }
export type BootstrapBrokenStatus = { phase: 'broken'; diagnostic_code?: string; setup_url?: never }
export type BootstrapStatus = BootstrapSetupStatus | BootstrapReadyStatus | BootstrapBrokenStatus

export const API_PATHS = {
  system: {
    bootstrapStatus: '/api/system/v1/bootstrap-status',
  },
  agent: {
    features: '/api/agent/features/v1',
    sendEmailCode: '/api/agent/auth/v1/email/send-code',
    loginEmailCode: '/api/agent/auth/v1/login/email-code',
    loginPassword: '/api/agent/auth/v1/login/password',
    passwordSetup: '/api/agent/auth/v1/password/setup',
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
	projects: '/api/agent/project/v1/projects',
	projectDetail: '/api/agent/project/v1/projects/{project_id}',
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
    cashierOrderSync: '/api/agent/cashier/v1/orders/{order_id}/sync',
    cashierOrderMockPay: '/api/agent/cashier/v1/orders/{order_id}/mock-pay',
    estimate: '/api/agent/billing/v1/estimate',
    promptOptimizationEstimate: '/api/agent/text/v1/prompt-optimizations/estimate',
    promptOptimizations: '/api/agent/text/v1/prompt-optimizations',
    redeemCode: '/api/agent/billing/v1/redeem-codes/redeem',
    capabilities: '/api/agent/image/v1/capabilities',
    videoCapabilities: '/api/agent/video/v1/capabilities',
    videoEstimates: '/api/agent/video/v1/estimates',
    videoTasks: '/api/agent/video/v1/tasks',
    videoTaskDetail: '/api/agent/video/v1/tasks/{task_id}',
    videoTaskCancel: '/api/agent/video/v1/tasks/{task_id}:cancel',
    videoTaskEvents: '/api/agent/video/v1/tasks/events',
    mediaAssets: '/api/agent/media/v1/assets',
    mediaAssetDetail: '/api/agent/media/v1/assets/{asset_id}',
    mediaAssetAccess: '/api/agent/media/v1/assets/{asset_id}/access',
    mediaAssetRetry: '/api/agent/media/v1/assets/{asset_id}:retry-processing',
    mediaAssetBatch: '/api/agent/media/v1/assets:batch-{action}',
    mediaExportJob: '/api/agent/media/v1/export-jobs/{job_id}',
    mediaExportDownload: '/api/agent/media/v1/export-jobs/{job_id}/download',
    mediaUploads: '/api/agent/media/v1/uploads',
    mediaUploadDetail: '/api/agent/media/v1/uploads/{upload_id}',
    mediaUploadPart: '/api/agent/media/v1/uploads/{upload_id}/parts/{part_number}',
    mediaUploadPartSign: '/api/agent/media/v1/uploads/{upload_id}/parts/{part_number}:sign',
    mediaUploadComplete: '/api/agent/media/v1/uploads/{upload_id}:complete',
    referenceAssets: '/api/agent/image/v1/reference-assets',
    importReferenceAssetsFromGallery: '/api/agent/image/v1/reference-assets:import-from-gallery',
    referenceAssetDetail: '/api/agent/image/v1/reference-assets/{asset_id}',
    referenceAssetDownload: '/api/agent/image/v1/reference-assets/{asset_id}/download',
    referenceAssetAccess: '/api/agent/image/v1/reference-assets/{asset_id}/access',
    imageDownload: '/api/agent/image/v1/images/{image_id}',
    imageAccess: '/api/agent/image/v1/images/{image_id}/access',
    tasks: '/api/agent/image/v1/tasks',
    taskDetail: '/api/agent/image/v1/tasks/{task_id}',
    taskEvents: '/api/agent/image/v1/tasks/{task_id}/events',
    taskStream: '/api/agent/image/v1/tasks/events',
    historyTasks: '/api/agent/image/v1/history/tasks',
    historyTaskDetail: '/api/agent/image/v1/history/tasks/{task_id}',
    historyTaskRetry: '/api/agent/image/v1/history/tasks/{task_id}/retry',
    galleryImages: '/api/agent/gallery/v1/images',
    galleryImageDetail: '/api/agent/gallery/v1/images/{image_id}',
    galleryImageGroup: '/api/agent/gallery/v1/images/{image_id}/group',
	galleryBatchPublish: '/api/agent/gallery/v1/images:batch-publish',
	galleryBatchGroup: '/api/agent/gallery/v1/images:batch-group',
	galleryBatchDelete: '/api/agent/gallery/v1/images:batch-delete',
	galleryBatchTransferProject: '/api/agent/gallery/v1/images:batch-transfer-project',
	galleryBatchDownload: '/api/agent/gallery/v1/images:batch-download',
	galleryExportJob: '/api/agent/gallery/v1/export-jobs/{job_id}',
	galleryExportDownload: '/api/agent/gallery/v1/export-jobs/{job_id}/download',
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
    galleryImageAccess: '/api/open/image/v1/gallery/images/{image_id}/access',
    paymentWebhook: '/api/open/image/v1/payments/webhooks/{channel}',
  },
  compat: {
    generations: '/v1/images/generations',
    edits: '/v1/images/edits',
    models: '/v1/models',
  },
  ops: {
    login: '/api/ops/admin/v1/auth/login',
    refreshSession: '/api/ops/admin/v1/auth/session/refresh',
    logout: '/api/ops/admin/v1/auth/logout',
    auditLogs: '/api/ops/admin/v1/audit-logs',
    clusterNodes: '/api/ops/admin/v1/cluster/nodes',
    adminUsers: '/api/ops/admin/v1/admin-users',
    adminUserDetail: '/api/ops/admin/v1/admin-users/{admin_id}',
    adminUserResetPassword: '/api/ops/admin/v1/admin-users/{admin_id}/reset-password',
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
    modelAccountTestImage: '/api/ops/admin/v1/model-accounts/{account_id}/test-image',
    textModelAccounts: '/api/ops/admin/v1/text-model-accounts',
    textModelAccountDetail: '/api/ops/admin/v1/text-model-accounts/{account_id}',
    textModelAccountModels: '/api/ops/admin/v1/text-model-accounts/{account_id}/models',
    textModelDetail: '/api/ops/admin/v1/text-models/{model_id}',
    textModelDefault: '/api/ops/admin/v1/text-models/{model_id}:default',
    textModelTest: '/api/ops/admin/v1/text-models/{model_id}:test',
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
    storageConfigs: '/api/ops/admin/v1/storage-configs',
    storageConfigProbe: '/api/ops/admin/v1/storage-configs:probe',
    storageConfigDetail: '/api/ops/admin/v1/storage-configs/{storage_config_id}',
    storageConfigDetailProbe: '/api/ops/admin/v1/storage-configs/{storage_config_id}:probe',
    storageConfigSetDefault: '/api/ops/admin/v1/storage-configs/{storage_config_id}:set-default',
    storageConfigSetStatus: '/api/ops/admin/v1/storage-configs/{storage_config_id}:set-status',
    imageReviews: '/api/ops/admin/v1/image-reviews',
    imageReviewImage: '/api/ops/admin/v1/image-reviews/{image_id}/image',
    imageReviewApprove: '/api/ops/admin/v1/image-reviews/{image_id}:approve',
    imageReviewReject: '/api/ops/admin/v1/image-reviews/{image_id}:reject',
    imageReviewUnpublish: '/api/ops/admin/v1/image-reviews/{image_id}:unpublish',
    dashboard: '/api/ops/admin/v1/metrics/dashboard',
    monitoringSnapshot: '/api/ops/admin/v1/monitoring/snapshot',
    readiness: '/api/ops/admin/v1/readiness',
    videoConfiguration: '/api/ops/admin/v1/video/configuration',
    videoModelCapability: '/api/ops/admin/v1/model-account-models/{id}/video-capability',
    videoModelCostRules: '/api/ops/admin/v1/model-account-models/{id}/video-cost-rules',
    videoModelCostRuleDetail: '/api/ops/admin/v1/model-account-models/{id}/video-cost-rules/{rule_id}',
    videoPricingStrategies: '/api/ops/admin/v1/video-pricing-strategies',
    videoPricingStrategyDetail: '/api/ops/admin/v1/video-pricing-strategies/{id}',
    videoPricingStrategySimulate: '/api/ops/admin/v1/video-pricing-strategies/{id}:simulate',
    videoPricingStrategyRecalculate: '/api/ops/admin/v1/video-pricing-strategies/{id}:recalculate',
    videoPriceRules: '/api/ops/admin/v1/video-price-rules',
    videoPriceRuleDetail: '/api/ops/admin/v1/video-price-rules/{id}',
    routeVideoConfig: '/api/ops/admin/v1/route-models/{id}/video-config',
    routeVideoImpact: '/api/ops/admin/v1/route-models/{id}/video-impact',
    adminVideoTasks: '/api/ops/admin/v1/video-tasks',
    adminVideoTaskDetail: '/api/ops/admin/v1/video-tasks/{task_id}',
    adminVideoTaskRetryArtifact: '/api/ops/admin/v1/video-tasks/{task_id}:retry-artifact',
    mediaProcessingJobRetry: '/api/ops/admin/v1/media-processing-jobs/{job_id}:retry',
    mediaPolicy: '/api/ops/admin/v1/media-policy',
    cashierOverview: '/api/ops/admin/v1/cashier/overview',
    cashierPlans: '/api/ops/admin/v1/cashier/plans',
    cashierPlanDetail: '/api/ops/admin/v1/cashier/plans/{plan_id}',
    cashierPlanTransition: '/api/ops/admin/v1/cashier/plans/{plan_id}/{action}',
    cashierCustomAmountConfig: '/api/ops/admin/v1/cashier/custom-amount-config',
    paymentVisibleMethods: '/api/ops/admin/v1/cashier/visible-methods',
    paymentProviderInstances: '/api/ops/admin/v1/cashier/provider-instances',
    paymentProviderInstanceDetail: '/api/ops/admin/v1/cashier/provider-instances/{instance_id}',
    securitySMTP: '/api/ops/admin/v1/security/smtp',
    securitySMTPTest: '/api/ops/admin/v1/security/smtp/test',
    paymentOrders: '/api/ops/admin/v1/cashier/orders',
    paymentOrderDetail: '/api/ops/admin/v1/cashier/orders/{order_id}',
    paymentOrderComplete: '/api/ops/admin/v1/cashier/orders/{order_id}/complete',
    paymentOrderClose: '/api/ops/admin/v1/cashier/orders/{order_id}/close',
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

export type ImageTaskType = 'text_to_image' | 'image_edit'
export type VideoTaskType = 'text_to_video' | 'image_to_video' | 'first_last_frame_to_video'
export type VideoTaskStatus = 'queued' | 'running' | 'saving' | 'succeeded' | 'partial' | 'failed' | 'cancelled' | string
export type VideoTaskCombination = { duration_seconds: number; resolution: string; aspect_ratio: string; audio_mode: 'silent' | 'generated' }
export type VideoTaskOptions = { durations: number[]; resolutions: string[]; aspect_ratios: string[]; audio_generation: boolean; combinations: VideoTaskCombination[] }
export type VideoCapabilityModelGroup = {
  code: string
  name: string
  description?: string
  minimum_points: string
  max_output_count: number
  task_types: VideoTaskType[]
  defaults: { task_type: VideoTaskType; duration_seconds: number; resolution: string; aspect_ratio: string; generate_audio: boolean }
  options_by_task_type: Partial<Record<VideoTaskType, VideoTaskOptions>>
}
export type VideoCapability = { capability_version: string; model_groups: VideoCapabilityModelGroup[] }
export type VideoCapabilityCombinationWire = { task_type: VideoTaskType; duration_seconds: number; resolution: string; aspect_ratio: string; audio_mode: 'silent' | 'generated' }
export type VideoCapabilityGroupWire = {
  route_model_code: string; name: string; description?: string; config_version: string; capability_version: string
  max_output_count: number; task_types: VideoTaskType[]; combinations: VideoCapabilityCombinationWire[]
}
export type VideoCapabilityListWire = { groups: VideoCapabilityGroupWire[] }
export type VideoInput = { id?: string; asset_id: string; role: 'first_frame' | 'last_frame'; ordinal: number; asset_snapshot?: Record<string, unknown>; asset?: { id: string; name?: string; preview_url?: string } }
export type VideoEstimateRequest = {
  project_id: string; route_model_code: string; task_type: VideoTaskType; prompt_template: string
  prompt_variables: Array<{ name: string; value: string }>; inputs: Array<Omit<VideoInput, 'id' | 'asset'>>
  reference_bindings: Array<{ name: string; asset_id: string }>
  duration_seconds: number; resolution: string; aspect_ratio: string; audio_mode: 'silent' | 'generated'; output_count: number
}
export type VideoEstimate = {
	quote_token: string; expires_at: string; capability_version: string; config_version: string; price_version: string
  unit_points: string; estimated_points: string; max_reserved_points: string; display_points?: string; pricing_mode?: string
  summary?: Record<string, unknown>; balance?: { available_points: string; sufficient: boolean }
}
export type VideoTaskItem = { id: string; ordinal: number; status: string; stage: string; result_asset_id?: string; actual_output_seconds?: string; actual_points?: string; error_code?: string; error_message?: string }
export type VideoTask = {
  id: string; project_id: string; route_model_code: string; task_type: VideoTaskType; status: VideoTaskStatus; progress_stage?: string; progress_message?: string
  prompt_template: string; prompt_binding_snapshot?: Record<string, unknown>; duration_seconds: number; resolution: string; aspect_ratio: string; audio_mode?: 'silent' | 'generated'; generate_audio?: boolean
  requested_output_count: number; success_output_count?: number; estimated_points?: string; reserved_points?: string; actual_points?: string
  settlement_status?: string; pricing_snapshot?: Record<string, unknown>; routing_snapshot?: Record<string, unknown>; inputs: VideoInput[]; items: VideoTaskItem[]
  version?: number; created_at?: string; updated_at?: string; started_at?: string; finished_at?: string
}
export type VideoCreateTaskRequest = VideoEstimateRequest & { quote_token: string }
export type CanvasNodeType = 'prompt' | 'image' | 'video' | 'audio' | 'image_generation' | 'video_generation' | 'note'
export type CanvasInputRole = 'prompt' | 'reference' | 'first_frame' | 'last_frame' | 'result'
export type CanvasDocument = {
  schema_version: 1
  viewport: { x: number; y: number; zoom: number }
  nodes: Array<{ id: string; type: CanvasNodeType; asset_id?: string; position: { x: number; y: number }; size: { width: number; height: number }; payload?: Record<string, unknown> }>
  edges: Array<{ id: string; source: string; target: string; source_handle?: string; target_handle?: string; input_role: CanvasInputRole; ordinal?: number }>
}
export type CreativeCanvas = {
  id: string; project_id: string; name: string; revision: number; metadata_version: number; document: CanvasDocument
  node_count: number; edge_count: number; running_task_count: number; failed_task_count: number; status: 'active' | 'deleted'; created_at?: string; updated_at?: string
}
export type CanvasRun = {
  id: string; canvas_id: string; node_id: string; submitted_revision: number; task_kind: 'image' | 'video'; task_id: string
  status: 'submitting' | 'queued' | 'running' | 'saving' | 'succeeded' | 'failed' | 'canceled' | 'attached' | 'unplaced'
  result_asset_ids?: string[]; attached_revision?: number; error_code?: string; error_message?: string
}
export type AdminVideoImpact = { route_model_id?: number; pricing_strategy_id?: number; code: string; summary: string; blocking: boolean; fix_route: 'pricing' | 'routing' | string }
export type AdminVideoConfiguration = {
  capabilities: Array<{ account_model_id: number; capability_version: string; validation_status: string; capability: Record<string, unknown>; enabled: boolean }>
  cost_rules: Array<{ id: number; account_model_id: number; billing_mode: string; rule_version: number; currency: string; rates: Record<string, unknown>; validation_status: string; effective_at: string; expires_at?: string; enabled: boolean }>
  pricing_strategies: AdminVideoPricingStrategy[]
  price_rules: Array<{ id: number; pricing_strategy_id: number; task_type: string; resolution: string; audio_mode: string; rule_version: number; safety_points: string; sales_points: string; candidate_cost_upper_cny: string; enabled: boolean }>
  routes: AdminVideoRouteConfig[]
  point_products?: Array<{id: number; code: string; price_cny: string; points: string; bonus_points: string; enabled: boolean}>
  impacts: AdminVideoImpact[]
  generated_at: string
}
export type AdminVideoPricingStrategy = {
  id: number; code: string; name: string; strategy_version: number; minimum_net_point_income_cny: string; target_margin_rate: string
  provider_cost_buffer_rate: string; payment_fee_rate: string; platform_fixed_cost_cny: string; platform_output_second_cost_cny: string
  platform_reference_cost_cny: string; enabled: boolean
}
export type AdminVideoVisibleCombination = { task_type: string; resolution: string; aspect_ratio?: string; audio_mode: string; duration_seconds: number }
export type AdminVideoRouteConfig = {
  route_model_id: number; route_code: string; route_name: string; config_version: string; pricing_strategy_id: number; candidate_count: number
  candidate_account_model_ids?: number[]; task_types: string[]; visible_options: Record<string, unknown>; defaults: Record<string, unknown>; max_output_count: number; enabled: boolean
}
export type AdminVideoCapabilityWrite = { expected_version: string; capability_version: string; capability: Record<string, unknown>; validation_status: string; enabled: boolean }
export type AdminVideoCostRuleWrite = { id?: number; expected_rule_version: number; billing_mode: string; currency: string; rates: Record<string, unknown>; validation_status: string; effective_at: string; expires_at?: string; enabled: boolean }
export type AdminVideoStrategyWrite = Partial<AdminVideoPricingStrategy> & { expected_version: number; code: string; name: string; enabled: boolean }
export type AdminVideoSimulationRequest = { route_model_id: number; task_type: string; resolution: string; audio_mode: string; duration_seconds: number; reference_image_count?: number }
export type AdminVideoSimulationResult = { worst_candidate_cost_cny: string; safety_points: string; net_point_income_cny: string; candidate_account_model_id: number }
export type AdminVideoPriceRuleWrite = { id?: number; route_model_id: number; pricing_strategy_id: number; expected_version: number; task_type: string; resolution: string; audio_mode: string; duration_seconds: number; effective_at: string; minimum_task_points: string; safety_points?: string; enabled: boolean; [key: string]: unknown }
export type AdminVideoRouteConfigWrite = { expected_version: string; config_version: string; pricing_strategy_id: number; task_types: string[]; visible_options: Record<string, unknown>; defaults: Record<string, unknown>; visible_combinations: AdminVideoVisibleCombination[]; max_output_count: number; enabled: boolean }
export type AdminVideoTaskSummary = { id: string; user_id: number; project_id: string; route_model_id: number; route_model_code: string; status: string; settlement_status: string; estimated_points: string; actual_points: string; created_at: string; updated_at: string }
export type AdminVideoAttempt = { id: string; item_id: string; attempt_no: number; provider_code: string; model_code: string; provider_job_id?: string; status: string; usage_raw: Record<string, unknown>; usage_normalized: Record<string, unknown>; cost_snapshot: Record<string, unknown>; provider_cost: string; error_category?: string; error_code?: string; error_message?: string; started_at?: string; finished_at?: string }
export type AdminVideoTaskDetail = AdminVideoTaskSummary & { pricing_snapshot: Record<string, unknown>; routing_snapshot: Record<string, unknown>; reserved_points: string; items: Array<{ id: string; ordinal: number; status: string; stage: string; result_asset_id?: string; actual_points: string; provider_cost: string; artifact_snapshot: Record<string, unknown>; error_code?: string; error_message?: string; attempts: AdminVideoAttempt[] }> }
export type AdminVideoTaskPage = { items: AdminVideoTaskSummary[]; next_cursor?: string }
export type AdminVideoRecovery = { task_id?: string; job_id?: string; recovery: 'artifact' | 'derivative'; provider_generation_requested: false }
export type MediaPolicy = {
  version: number; allowed_formats: Record<'image' | 'video' | 'audio', string[]>; single_file_max_bytes: number; video_max_duration_seconds: number; user_quota_bytes: number
  image_thumbnail_widths: number[]; video_poster_enabled: boolean; video_hover_preview_enabled: boolean; video_proxy_enabled: boolean; audio_proxy_enabled: boolean; audio_waveform_enabled: boolean
  upload_session_ttl_hours: number; failed_processing_retention_days: number; soft_delete_retention_days: number; applies_to: 'new_objects_and_derivative_versions'
}
export type ImageTaskStatus = 'queued' | 'running' | 'succeeded' | 'partial_failed' | 'failed' | 'cancelled' | 'rejected' | 'deleted'
export type PublishStatus = 'private' | 'reviewing' | 'pending_review' | 'public' | 'approved' | 'rejected' | 'unpublished'
export type ThemeMode = 'dark' | 'light'
export type AccentTheme = 'amber' | 'violet' | 'emerald' | 'coral'
export type UserThemePreference = { mode: ThemeMode; accent: AccentTheme }

type GenerationPreferencesBase = {
  model_group: string
  aspect_ratio: string
  image_count: number
  theme_mode?: ThemeMode
  accent_theme?: AccentTheme
  default_locale?: string
}
export type GenerationPreferences = GenerationPreferencesBase & (
  | { base_resolution: string; quality?: string }
  | { base_resolution?: string; quality: string }
)
export type UserProfile = {
  id: ID
  email: string
  has_password: boolean
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

export type SendEmailCodeRequest = { email: string; scene?: 'login' | 'register' | 'password_reset' | 'password_change' | string }
export type SendEmailCodeResponse = { email: string; scene: string; status: string; cooldown_seconds?: number }
export type PasswordLoginRequest = { email: string; password: string }
export type EmailCodeLoginRequest = { email: string; code: string }
export type ChangePasswordRequest = { code: string; new_password: string }
export type PasswordResetRequest = { email: string }
export type PasswordResetConfirmRequest = { email: string; code: string; new_password: string }
export type CloseAccountResponse = { id: ID; status: string; closed_at?: string | null }
export type UpdateProfileRequest = { nickname?: string; bio?: string; avatar_object_key?: string; default_locale?: string; theme?: string }
export type UpdatePreferencesRequest = Partial<{
  theme: string
  model_group: string
  base_resolution: string
  quality: string
  aspect_ratio: string
  image_count: number
  theme_mode: ThemeMode
  accent_theme: AccentTheme
  default_locale: string
}>

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
  mixed_expiry?: boolean
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
export type SignupGrantResult = {
  granted: boolean
  grant_id?: number
  grant_type?: 'trial' | string
  points?: string
  expires_at?: string | null
  balance: Balance
}
export type NormalLoginResponse = { password_setup_required?: false; access_token: string; expires_in_seconds: number; expires_in?: number; user_id: ID; profile?: UserProfile; signup_grant?: SignupGrantResult }
export type PasswordSetupRequiredLoginResponse = { password_setup_required: true; password_setup_token: string; password_setup_expires_in_seconds: number; user_id: ID; signup_grant?: SignupGrantResult; access_token?: never }
export type LoginResponse = NormalLoginResponse | PasswordSetupRequiredLoginResponse
export type SubscriptionPlan = {
  id: number
  plan_code: string
  plan_name: string
  status: string
  price_cny: string
  points: string
  bonus_points: string
  credit_expiry_enabled?: boolean
  duration_days?: number | null
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
export type CashierPlanStatus = 'active' | 'disabled' | 'archived'
export type CashierPlanTransitionAction = 'enable' | 'disable' | 'archive' | 'restore'
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
export type PublicPaymentVisibleMethod = Pick<PaymentVisibleMethod, 'method' | 'label' | 'enabled' | 'display_order'>
export type CashierOptions = {
  plans: CashierPlan[]
  custom_amount: CashierCustomAmountConfig
  visible_methods: PublicPaymentVisibleMethod[]
  order_timeout_seconds: number
}
export type CashierPurchaseType = 'plan' | 'custom_amount' | string
export type PaymentProviderType = 'alipay_direct' | 'wxpay_direct' | 'easypay_alipay' | 'easypay_wxpay' | 'mock' | 'jeepay_alipay' | 'jeepay_wxpay' | 'stripe' | string
export type PaymentSchedulerStrategy = 'round_robin' | 'random' | string
export type PaymentDisplay = {
  type: 'qr_code' | 'redirect' | 'form_html' | 'form' | 'jsapi' | 'mock' | 'none' | string
  qr_code?: string
  payment_url?: string
  client_token?: string
  client_secret?: string
  publishable_key?: string
  form_html?: string
  expires_at?: string | null
}
export type PaymentOrder = {
  id: number
  order_no: string
  user_id?: number
	user_email?: string
	user_nickname?: string
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
	total_points?: string
  credit_expiry_enabled?: boolean
  credit_valid_days?: number | null
  credited_at?: string | null
  credit_expires_at?: string | null
  trade_no?: string
  refund_trade_no?: string
  refunded_amount_cny?: string
  refunded_points?: string
  chargeback_points?: string
  chargeback_reason?: string
  chargeback_at?: string | null
  chargeback_idempotency_key?: string
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
export type ClosePaymentOrderRequest = {
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
  risk_category?: 'pending' | 'paid' | 'closed' | 'refunded' | 'channel_error' | 'risk_control' | 'channel_limited' | 'signature_error' | 'amount_mismatch' | 'account_abnormal' | 'channel_timeout' | string
  action_hint?: string
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
export type CashierOrder = {
  id: number
  order_no: string
  plan_id?: number
  plan_code?: string
  plan_name?: string
  purchase_type: CashierPurchaseType
  visible_method: string
  status: string
  currency: string
  amount_cny: string
  points: string
  bonus_points: string
  credit_expiry_enabled?: boolean
  credit_valid_days?: number | null
  credited_at?: string | null
  credit_expires_at?: string | null
  payment_url?: string
  qr_code?: string
  client_token?: string
  payment_display?: PaymentDisplay
  failure_reason?: string
  expires_at: string
  paid_at?: string | null
  completed_at?: string | null
  closed_at?: string | null
  refunded_at?: string | null
  created_at: string
  updated_at: string
}
export type CashierOrderSyncResult = {
  query_status: PaymentOrderSyncStatus
  risk_category?: PaymentOrderSyncResult['risk_category']
  paid: boolean
  completed: boolean
  amount_cny?: string
  message?: string
  synced_at: string
}
export type CashierOrderSyncResponse = {
  order: CashierOrder
  sync: CashierOrderSyncResult
}
export type MediaAccessPurpose = 'thumbnail' | 'poster' | 'hover' | 'preview' | 'waveform' | 'download'
export type MediaAccessProjection = {
  url: string
  expires_at?: string
  range_supported?: boolean
}

export type MediaType = 'image' | 'video' | 'audio'
export type MediaAsset = {
  id: string
  user_id?: number
  project_id: string
  legacy_image_id?: string
  name: string
  group_name: string
  media_type: MediaType
  source_type: string
  source_task_kind?: 'image' | 'video'
  source_task_id?: string
  source_canvas_id?: string
  status: string
  visibility_status: string
  storage_driver: string
  mime_type: string
  container?: string
  codec?: string
  file_size_bytes: number
  sha256?: string
  width?: number
  height?: number
  duration_ms?: number
  frame_rate_milli?: number
  audio_codec?: string
  channels?: number
  sample_rate?: number
  version: number
  created_at: string
  updated_at: string
}
export type MediaAssetFilters = {
  project_id?: string
  media_type?: MediaType | ''
  source_type?: string
  group_name?: string
  status?: string
  keyword?: string
  sort_by?: 'created_at' | 'updated_at' | 'name' | 'file_size_bytes' | 'duration_ms'
  sort_order?: 'asc' | 'desc'
  cursor?: string
  limit?: number
}
export type MediaAssetPage = { items: MediaAsset[]; next_cursor?: string }
export type MediaCompletedPart = { part_number: number; etag: string; checksum?: string; size_bytes?: number }
export type MediaUploadSession = {
  id: string
  project_id: string
  group_name?: string
  original_filename: string
  declared_media_type: MediaType
  declared_mime_type: string
  declared_size_bytes: number
  declared_checksum?: string
  storage_driver: string
  part_size: number
  part_count: number
  status: string
  completed_parts?: MediaCompletedPart[]
  asset_id?: string
  expires_at: string
  completed_at?: string
}
export type MediaUploadInit = {
  project_id: string
  group_name?: string
  filename: string
  media_type: MediaType
  mime_type: string
  size_bytes: number
  checksum?: string
}
export type MediaPartTarget = { url: string; headers?: Record<string, string>; expires_at: string }
export type MediaBatchAction = 'download' | 'group' | 'transfer-project' | 'delete'
export type MediaBatchItemResult = {
  id: string
  status: 'succeeded' | 'failed' | string
  asset?: MediaAsset
  access?: MediaAccessProjection
  error?: { code?: string; message?: string }
}
export type MediaBatchResult = { items: MediaBatchItemResult[] }
export type MediaExportJob = Omit<GalleryExportJob, 'image_ids'> & { image_ids?: string[] }
export type MediaExportStatus = { job: MediaExportJob; status_url?: string; download_url?: string }
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
  successful_image_count?: number
  effective_unit_points?: string
  total_charged_points?: string
  partial_success?: boolean
}

export type CapabilityItem = {
  route_model_code: string
  task_types: ImageTaskType[]
  qualities?: string[]
  base_resolution?: string[]
  auto_base_resolution_by_task_type?: Partial<Record<ImageTaskType, string>>
  size_modes?: Array<'auto' | 'ratio' | 'pixel' | string>
  aspect_ratios: string[]
  pixel_sizes?: string[]
  max_output_image_count: number
  max_reference_image_count: number
  quality?: string[]
  output_format?: string[]
  supports_output_compression?: boolean
  supports_custom_size?: boolean
  supports_custom_ratio?: boolean
  supported_backgrounds?: string[]
  min_width?: number
  max_width?: number
  min_height?: number
  max_height?: number
  capabilities_by_task_type?: Partial<Record<ImageTaskType, CapabilityTaskOptions>>
  moderation?: string[]
}
export type CapabilityTaskOptions = {
  base_resolution?: string[]
  auto_base_resolution?: string
  size_modes?: Array<'auto' | 'ratio' | 'pixel' | string>
  aspect_ratios?: string[]
  pixel_sizes?: string[]
  quality?: string[]
  output_format?: string[]
  supports_output_compression?: boolean
  supports_custom_size?: boolean
  supports_custom_ratio?: boolean
  supported_backgrounds?: string[]
  min_width?: number
  max_width?: number
  min_height?: number
  max_height?: number
  moderation?: string[]
  max_output_image_count?: number
  max_reference_image_count?: number
}
export type RouteModelPriceQuote = {
  task_type: ImageTaskType
  quality?: string
  base_resolution?: string
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
  qualities?: string[]
  base_resolution?: string[]
  auto_base_resolution_by_task_type?: Partial<Record<ImageTaskType, string>>
  size_modes?: Array<'auto' | 'ratio' | 'pixel' | string>
  aspect_ratios?: string[]
  pixel_sizes?: string[]
  max_output_image_count?: number
  max_reference_image_count?: number
  quality?: string[]
  output_format?: string[]
  supports_output_compression?: boolean
  supports_custom_size?: boolean
  supports_custom_ratio?: boolean
  supported_backgrounds?: string[]
  min_width?: number
  max_width?: number
  min_height?: number
  max_height?: number
  capabilities_by_task_type?: Partial<Record<ImageTaskType, CapabilityTaskOptions>>
  moderation?: string[]
  effective_multiplier?: string
  minimum_points?: string
  prices: RouteModelPriceQuote[]
  supports_reference: boolean
  display_points?: string
}
export type Capability = {
  items?: CapabilityItem[]
  raw?: unknown
  unavailable_reason?: { code: string; message: string } | null
  model_groups: CapabilityModelGroup[]
  qualities?: string[]
  base_resolution?: string[]
  size_modes?: Array<'auto' | 'ratio' | 'pixel' | string>
  aspect_ratios: string[]
  pixel_sizes?: string[]
  quality?: string[]
  output_format?: string[]
  supports_output_compression?: boolean
  supports_custom_size?: boolean
  supports_custom_ratio?: boolean
  supported_backgrounds?: string[]
  min_width?: number
  max_width?: number
  min_height?: number
  max_height?: number
  moderation?: string[]
  max_image_count: number
  reference_image_max_mb?: number
  reference_image_max_bytes?: number
  reference_image_allowed_formats?: string[]
  reference_image_allowed_mime_types?: string[]
  task_types: ImageTaskType[]
}
export type EstimateRequest = { task_type: ImageTaskType; route_model_code: string; size_mode?: 'auto' | 'ratio' | 'pixel' | string; base_resolution?: string; quality?: string; output_format?: string; background?: string; output_compression?: number; moderation?: string; aspect_ratio?: string; pixel_size?: string; image_count: number; reference_asset_ids?: string[]; model_group?: string }
export type BackendImageTaskType = ImageTaskType
export type BackendEstimateRequest = { task_type: BackendImageTaskType; route_model_code: string; size_mode?: string; aspect_ratio?: string; base_resolution?: string; quality?: string; output_format?: string; background?: string; output_compression?: number; moderation?: string; requested_size?: string; requested_output_image_count: number; reference_image_count?: number }
export type EstimateResult = {
	capability_version?: string
  resolved_quality_bucket?: string
  base_resolution?: string
  size_mode?: string
  requested_size?: string
  resolved_size?: string | null
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
  resolved_quality?: string
  sufficient: boolean
}

export type ProjectStatus = 'active' | 'transferring' | 'deleted' | string
export type Project = {
  id: string
  user_id?: number
  name: string
  name_key?: string
  is_default: boolean
  status: ProjectStatus
  version: number
	task_count?: number
	asset_count?: number
  created_at: string
  updated_at: string
}
export type ProjectSnapshot = Pick<Project, 'id' | 'name' | 'is_default'>
export type ReferenceAsset = {
  id: string
  name: string
  preview_url?: string
  download_url?: string
  preview_expires_at?: string
  download_expires_at?: string
  status: 'uploaded' | 'processing' | 'ready' | 'failed' | string
  size_bytes?: number
  mime_type?: string
  file_size_bytes?: number
  width?: number
  height?: number
  sha256?: string
  storage_config_id?: string
  storage_driver?: string
  object_key?: string
  source_image_result_id?: string | null
  owns_object?: boolean
  generation_snapshot?: ReferenceGenerationSnapshot
  created_at: string
}
export type PromptReferenceBinding = { name: string; asset_id: string }
export type PromptVariableInput = { name: string; value: string }
export type ReferenceGenerationSnapshot = {
  task_type?: ImageTaskType
  abstract_model?: string
  route_model_code?: string
  capability_version?: string
  size_mode?: string
  requested_size?: string
  base_resolution?: string
  aspect_ratio?: string
  quality?: string
  background?: string
  output_format?: string
  output_compression?: number
  moderation?: string
  image_count?: number
}
export type ImageResult = {
  id: string
  project_id?: string | null
  project?: ProjectSnapshot
  url: string
  download_url?: string
  preview_expires_at?: string
  download_expires_at?: string
  mime_type?: string
  file_size_bytes?: number
  sha256?: string
  storage_config_id?: string
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
  size_mode?: string
  requested_size?: string
  base_resolution?: string
  quality?: string
  aspect_ratio?: string
  output_format?: string
  background?: string
  output_compression?: number
  moderation?: string
  requested_output_image_count?: number
  image_count?: number
  reference_asset_ids?: string[]
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
  project_id?: string | null
  project?: ProjectSnapshot
  title: string
  prompt: string
  task_type: ImageTaskType
  status: ImageTaskStatus
  progress_stage?: string
  progress_message?: string
  abstract_model?: string
  route_model_code?: string
  route_model_name?: string
  model_group: string
  requested_quality?: string
  resolved_quality_bucket?: string
  base_resolution?: string
  size_mode?: string
  quality?: string
  requested_size?: string
  resolved_width?: number
  resolved_height?: number
  aspect_ratio: string
  output_format?: string
  background?: string
  output_compression?: number
  moderation?: string
  requested_output_image_count?: number
  image_count: number
  reference_image_count?: number
  reference_asset_ids?: string[]
  estimated_points?: string
  actual_points?: string
  estimate_points: string
  progress?: number
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
export type BackendCreateTaskRequest = Omit<BackendEstimateRequest, 'reference_image_count'> & { project_id?: string; prompt: string; reference_asset_ids?: string[]; reference_bindings?: PromptReferenceBinding[]; prompt_variables?: PromptVariableInput[]; response_mode: 'async'; capability_version?: string }
export type CreateTaskRequest = EstimateRequest & { project_id?: string; prompt: string; reference_bindings?: PromptReferenceBinding[]; prompt_variables?: PromptVariableInput[]; idempotency_key?: string; response_mode?: 'sync' | 'async' | string; capability_version?: string }
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
export type ClusterNodeRole = 'single' | 'control' | 'api' | 'worker' | 'web' | string
export type ClusterNodeHealth = 'joining' | 'healthy' | 'degraded' | 'unready' | 'offline' | string
export type ClusterNode = {
  node_id: string
  installation_id: string
  role: ClusterNodeRole
  source: 'logical-single' | 'heartbeat' | string
  application_version: string
  runtime_schema_version: number
  config_revision: number
  health: ClusterNodeHealth
  effective_health: ClusterNodeHealth
  last_error?: string
  last_heartbeat_at?: string | null
  application_version_drift: boolean
  runtime_schema_drift: boolean
  config_revision_drift: boolean
  created_at: string
  updated_at: string
}
export type AdminLoginResult = { access_token: string; expires_in_seconds: number; admin_id: number; email: string; role: string; permissions?: AdminPermission[] }
export type AdminMetric = { key?: string; label: string; value: string; trend: string; detail?: string; tone: 'good' | 'warn' | 'bad' | 'danger' | 'neutral' }
export type ProviderHealth = { provider: string; provider_code?: string; provider_type?: string; status: 'healthy' | 'degraded' | 'down' | string; health_status?: string; latency_ms: number; error_rate: string; note: string; enabled?: boolean }
export type MonitoringWindow = '5m' | '15m' | '30m' | '60m'
export type AdminMonitoringCurrent = {
  inflight: number
  peak_inflight: number
  qps: number
  p50_ms: number
  p95_ms: number
  p99_ms: number
  server_error_rate: number
  cpu_percent: number | null
  heap_bytes: number
  sys_bytes: number
  goroutines: number
  gc_pause_ms: number
}
export type AdminMonitoringPoint = {
  at: string
  qps: number
  peak_inflight: number
  p50_ms: number
  p95_ms: number
  p99_ms: number
  server_error_rate: number
  cpu_percent: number | null
  heap_bytes: number
  sys_bytes: number
  goroutines: number
}
export type AdminMonitoringStatuses = {
  total: number
  success: number
  redirect: number
  client_error: number
  server_error: number
}
export type AdminMonitoringRoute = {
  route: string
  requests: number
  qps: number
  p95_ms: number
  client_error_rate: number
  server_error_rate: number
}
export type AdminMonitoringProvider = {
  provider_code: string
  provider_type: string
  status: string
  enabled: boolean
}
export type AdminMonitoringSnapshot = {
  generated_at: string
  window: MonitoringWindow
  sample_interval_seconds: number
  collecting: boolean
  uptime_seconds: number
  state: 'collecting' | 'healthy' | 'pressured' | 'critical' | string
  state_reasons: string[]
  current: AdminMonitoringCurrent
  series: AdminMonitoringPoint[]
  statuses: AdminMonitoringStatuses
  routes: AdminMonitoringRoute[]
  providers: AdminMonitoringProvider[]
}
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
  platform_loss_count: number
  platform_loss_provider_cost: string
  public_gallery_list_views: number
  public_gallery_detail_login_blocks: number
  enabled_payment_methods: string[]
  generated_at: string
}

export type AdminCallDistributionGroup = { key: string; calls: number; percentage: number }
export type AdminCallDistribution = {
  window: { from: string; to: string }
  total_calls: number
  groups: AdminCallDistributionGroup[]
  preflight_failure_count: number
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
  supported_base_resolution: string[]
  quality: string[]
  supported_ratios: string[]
  output_format: string[]
  output_compression: number
  moderation: string[]
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
export type GalleryImage = { id: string; task_id: string; user_id?: number; project_id?: string | null; project?: ProjectSnapshot; prompt?: string; abstract_model?: string; route_model_code?: string; task_type?: ImageTaskType; task_status?: ImageTaskStatus | string; size_mode?: string; requested_size?: string; base_resolution?: string; quality?: string; aspect_ratio?: string; output_format?: string; output_compression?: number; moderation?: string; requested_output_image_count?: number; image_count?: number; actual_points?: string; reference_asset_ids?: string[]; reference_assets?: ReferenceAsset[]; url?: string; download_url?: string; preview_expires_at?: string; download_expires_at?: string; mime_type?: string; file_size_bytes: number; width: number; height: number; sha256?: string; storage_config_id?: string; object_key?: string; storage_driver?: string; image_group?: string; visibility_status: PublishStatus; review_reason?: string; published_at?: string | null; author_name?: string; like_count?: number; favorite_count?: number; liked_by_viewer?: boolean; favorited_by_viewer?: boolean; created_at: string }
export type GalleryBatchFailure = { id: string; code: string; message: string }
export type GalleryBatchMutationResult = { succeeded: Array<{ id: string; entity: GalleryImage }>; failed: GalleryBatchFailure[] }
export type GalleryExportJob = {
  id: string
  project_id: string
  image_ids: string[]
  state: 'queued' | 'running' | 'succeeded' | 'failed' | 'expired' | string
  estimated_bytes: number
  archive_size_bytes?: number
  attempt_count?: number
	deadline_at: string
  expires_at?: string
  error_code?: string
  error_message?: string
  created_at: string
  updated_at: string
}
export type GalleryExportStatus = { job: GalleryExportJob; status_url?: string; download_url?: string }
export type ReviewItem = { id: string; image_id?: string; title: string; owner: string; task_type: ImageTaskType; image_url: string; status: 'pending' | 'pending_review' | 'approved' | 'rejected' | 'unpublished' | string; reason: string; created_at: string; review_reason?: string; visibility_status?: string }
export type AdminUser = { id: string; email: string; display_name: string; nickname?: string; status: 'active' | 'disabled' | 'pending' | 'closed' | string; group: string; user_group_code?: string; user_group_codes?: string[]; user_groups?: UserGroup[]; balance: string; token_version?: number; rpm_limit?: number; concurrency_limit?: number; default_locale?: string; theme?: string; closed_at?: string | null; created_at: string; updated_at?: string; last_seen_at: string }
export type AdminUserDetail = { user: AdminUser; balance: Balance; recent_ledger: LedgerEntry[]; recent_orders?: PaymentOrder[]; recent_tasks?: ImageTask[]; api_keys?: ApiKey[] }
export type AdminUserCreateRequest = { email: string; nickname?: string; status?: string; user_group_code?: string; password?: string; rpm_limit?: number; concurrency_limit?: number; default_locale?: string; theme?: string }
export type SystemAdminUser = { id: ID; email: string; role: AdminRole; status: 'active' | 'disabled' | string; created_at: string; updated_at: string }
export type SystemAdminUserCreateRequest = { email: string; password: string; role: AdminRole; status?: string }
export type SystemAdminUserUpdateRequest = { role?: AdminRole; status?: string }
export type SystemAdminPasswordResetRequest = { new_password: string }
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
  base_resolution: string[]
  quality: string[]
  max_reference_image_count: number
  max_image_count: number
  size_modes: string[]
  supported_ratios: string[]
  supported_pixel_sizes: string[]
  supports_custom_ratio?: boolean
  output_format: string[]
  supported_backgrounds?: string[]
  output_compression?: number
  supports_output_compression: boolean
  supports_custom_size: boolean
  min_width?: number
  max_width?: number
  min_height?: number
  max_height?: number
  moderation: string[]
  cost_per_image: string
  currency: string
  enabled: boolean
  extra?: Record<string, unknown>
  created_at: string
  updated_at: string
}
export type ModelAccountModelWriteRequest = Omit<Partial<ModelAccountModel>, 'id' | 'account_id' | 'account_name' | 'created_at' | 'updated_at'> & { model_code: string; display_name: string; task_types: ImageTaskType[]; base_resolution: string[]; quality: string[]; max_reference_image_count: number; max_image_count: number; size_modes: string[]; supported_ratios: string[]; supported_pixel_sizes: string[]; supports_custom_ratio: boolean; supported_backgrounds: string[]; min_width: number; max_width: number; min_height: number; max_height: number; output_format: string[]; supports_output_compression: boolean; supports_custom_size?: boolean; moderation: string[]; cost_per_image: string; currency: string; enabled: boolean }

export type ObjectDeletionJobState = 'pending' | 'running' | 'retry' | 'done' | 'blocked'
export type ObjectDeletionJob = {
  id: string
  storage_config_id?: string | null
  storage_driver: string
  bucket: string
  object_key: string
  state: ObjectDeletionJobState
  attempt_count: number
  next_attempt_at?: string | null
  last_error_code?: string | null
  last_error_message?: string | null
  completed_at?: string | null
  created_at: string
  updated_at: string
}
export type ModelAccountTestImageRequest = { model_id?: ID; model_code?: string; prompt?: string; source_mode?: 'images' | 'codex_responses' | string; size_mode?: string; requested_size?: string; base_resolution?: string; quality?: string; output_format?: string; background?: string; output_compression?: number; moderation?: string; aspect_ratio?: string }
export type ModelAccountTestImageResult = {
  status: string
  image_url?: string
  width?: number
  height?: number
  provider_request_id?: string
  actual_params?: Record<string, string>
  elapsed_ms: number
}
export type TextModelPlatformType = 'openai_compatible'
export type TextModelAPIStyle = 'chat_completions' | 'responses'
export type TextModelSecretStatus = { has_secret: boolean; fingerprint?: string; updated_at?: string | null }
export type TextModelAccount = {
  id: ID
  name: string
  platform_type: TextModelPlatformType
  api_style: TextModelAPIStyle
  base_url: string
  enabled: boolean
  secret_status: TextModelSecretStatus
  version: number
  created_at: string
  updated_at: string
}
export type TextModelAccountWriteRequest = {
  version?: number
  name: string
  platform_type: TextModelPlatformType
  api_style: TextModelAPIStyle
  base_url: string
  enabled: boolean
  secrets?: { api_key?: string }
  clear_secrets?: Array<'api_key'>
}
export type TextModel = {
  id: ID
  account_id: ID
  model_code: string
  display_name: string
  input_price_per_million_tokens: string
  output_price_per_million_tokens: string
  currency: string
  enabled: boolean
  is_default: boolean
  version: number
  created_at: string
  updated_at: string
}
export type TextModelDefaultReadiness = {
  status: 'ready' | 'selection_required' | 'unavailable'
  eligibleCount: number
  defaultModel?: TextModel
  defaultAccount?: TextModelAccount
}
export type TextModelWriteRequest = {
  version?: number
  model_code: string
  display_name: string
  input_price_per_million_tokens: string
  output_price_per_million_tokens: string
  currency: string
  enabled: boolean
}
export type TextModelConnectionTest = { status: 'success'; model_id: ID; model_code: string; api_style: TextModelAPIStyle; latency_ms: number }
export type PromptOptimizationModelSummary = { id: ID; model_code: string; display_name: string; api_style: TextModelAPIStyle }
export type PromptOptimizationEstimate = { quote: string; expires_at: string; estimated_points: string; model: PromptOptimizationModelSummary }
export type PromptOptimizationResult = { run_id: string; optimized_prompt: string; input_tokens: number; output_tokens: number; estimated_points: string; actual_points: string }
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
  base_resolution: string
  base_points: string
  reference_multiplier: string
  enabled: boolean
  created_at?: string
  updated_at?: string
}
export type RouteModelPriceWriteRequest = { route_model_id: ID; task_type: ImageTaskType; base_resolution: string; base_points: string; reference_multiplier: string; enabled: boolean }
export type RedeemCode = { id: number; batch_id: number; code: string; status: string; reward_type: string; reward_value: string; valid_from: string; valid_until: string; max_redemptions: number; redeemed_count: number; last_redeemed_by?: number | null; created_at: string; updated_at: string }
export type RedeemCodeCreateRequest = { code: string; batch_id: number; status: string; reward_type: string; reward_value: string; valid_from?: string; valid_until: string; max_redemptions: number }
export type RedeemCodeBatchCreateRequest = Omit<RedeemCodeCreateRequest, 'code'> & { count: number }
export type RedeemCodeBatchCreateResult = { items: RedeemCode[]; count: number; batch_id: number }
export type RedeemCodeExportRequest = { status?: string; code?: string; batch_id?: number }
export type RedeemCodeExportResult = { items: RedeemCode[]; count: number; filters: RedeemCodeExportRequest }
export type CallRecordAttempt = { provider?: string; adapter_type?: string; account_model_id?: number | null; model_account_id?: number | null; model_code?: string; status?: string; error?: string; error_code?: string; error_message?: string; error_detail?: Record<string, unknown>; started_at?: string | null; finished_at?: string | null }
export type ArtifactDiagnostic = { code?: string; stage?: string; attempt?: number; url_host?: string; url_path?: string; http_status?: number; content_type?: string; content_length?: number; bytes_read?: number; duration_ms?: number; storage_config_id?: string; storage_version?: number; retryable: boolean; cause?: string; started_at?: string; finished_at?: string }
export type ArtifactRecoverySummary = { status: string; attempt_count: number; last_diagnostic?: ArtifactDiagnostic; diagnostics?: ArtifactDiagnostic[] }
export type CallRecord = { id?: number; task_id: string; user_id: number; api_key_id?: number | null; source_channel: string; task_type: ImageTaskType | string; status: string; provider: string; route_model_code?: string; account_model_id?: number | null; model_account_id?: number | null; upstream_model_code?: string; abstract_model: string; base_resolution: string; quality: string; requested_output_image_count: number; success_output_image_count: number; reference_image_count: number; estimated_points: string; actual_points: string; provider_request_id?: string; provider_cost: string; gross_margin: string; pricing_snapshot?: Record<string, unknown>; upstream_succeeded_at?: string | null; failure_phase?: 'preflight' | 'upstream' | 'artifact_persistence' | string; platform_loss: boolean; artifact_recovery?: ArtifactRecoverySummary; error_code?: string | null; error_message?: string | null; error_detail?: Record<string, unknown>; attempts?: CallRecordAttempt[]; created_at: string; updated_at: string; started_at?: string | null; finished_at?: string | null; attempt_count: number }
export type AuditLog = { id: ID; actor: string; action: string; target: string; detail: string; created_at: string; actor_type?: string; actor_id?: string; target_type?: string; target_id?: string; result?: string; metadata?: Record<string, unknown>; ip_addr?: string; user_agent?: string; updated_at?: string }
export type AdminDashboardQueueItem = { item: string; count: string; detail: string }
export type AdminDashboard = { operations: AdminDashboardOperations; call_distribution: AdminCallDistribution; metrics: AdminMetric[]; providers: ProviderHealth[]; queue: AdminDashboardQueueItem[]; audit: AuditLog[] }
export type ReadinessStatus = 'pass' | 'warn' | 'fail' | string
export type ReadinessCheck = {
  key: string
  label: string
  status: ReadinessStatus
  availability?: 'healthy' | 'degraded' | 'unavailable' | string
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
  credentials_status?: SecretStatus
  last_error?: string | null
  created_at?: string
  updated_at?: string
}
export type PaymentProviderInstanceWriteRequest = Omit<Partial<PaymentProviderInstance>, 'id' | 'created_at' | 'updated_at'> & {
  provider_type: PaymentProviderType
  name: string
  enabled: boolean
  supported_methods: string[]
  secrets?: Record<string, unknown>
  clear_secrets?: string[]
}
export type SecretStatus = { has_secret: boolean; fingerprint?: string; updated_at?: string | null; secret_fields?: string[] }
export type SMTPConfigView = { enabled: boolean; host: string; port: number; username: string; from: string; starttls: boolean; insecure_skip_verify: boolean; secret_status: SecretStatus; version: number; updated_at?: string }
export type SMTPConfigWriteRequest = { version?: number; enabled: boolean; host: string; port: number; username: string; from: string; starttls: boolean; insecure_skip_verify: boolean; secrets?: { password?: string }; clear_secrets?: string[] }
export type SMTPTestResponse = { status: string; recipient: string }
export type StorageProbeView = { status: 'never' | 'success' | 'failed' | string; checked_at?: string | null; latency_ms?: number; message?: string }
export type StorageConfigView = {
  id: string
  code: string
  name: string
  driver: 'local' | 's3' | string
  provider: 'local' | 'aws_s3' | 'minio' | 'r2' | 'custom_s3' | string
  status: 'enabled' | 'disabled' | 'deleted' | string
  read_enabled: boolean
  write_enabled: boolean
  is_default: boolean
  endpoint?: string
  region?: string
  bucket?: string
  prefix?: string
  force_path_style: boolean
  public_base_url?: string
  local_root?: string
  secret_status: SecretStatus
  last_probe: StorageProbeView
  version: number
  updated_by?: number
  created_at?: string
  updated_at?: string
}
export type StorageConfigWriteRequest = {
  version?: number
  code?: string
  name: string
  driver: 'local' | 's3' | string
  provider: 'local' | 'aws_s3' | 'minio' | 'r2' | 'custom_s3' | string
  status?: 'enabled' | 'disabled' | string
  read_enabled?: boolean
  write_enabled?: boolean
  endpoint?: string
  region?: string
  bucket?: string
  prefix?: string
  force_path_style?: boolean
  public_base_url?: string
  local_root?: string
  secrets?: { access_key_id?: string; secret_access_key?: string }
  clear_secrets?: string[]
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
  signature_status?: 'verified' | 'recorded' | 'not_recorded' | 'failed' | string
  result_summary?: string | null
  payload_preview?: string | null
  received_at: string
  processed_at?: string | null
}

export type OpenReferenceAssetUploadSessionRequest = { filename: string; mime_type: string; content_base64: string }
export type OpenReferenceAssetUploadSessionResponse = { asset_id: string; status: string; upload_mode: string; asset: ReferenceAsset }
export type OpenAIImageGenerationRequest = { model: string; prompt: string; size?: string; n?: number; quality?: string; response_format?: 'url' | 'b64_json' | string; user?: string }
export type OpenAIImageEditRequest = { model: string; prompt: string; image: File | Blob | Array<File | Blob>; mask?: File | Blob; size?: string; n?: number; quality?: string; response_format?: 'url' | 'b64_json' | string; user?: string }
export type OpenAIImageResponse = { created?: number; data: Array<{ url?: string; b64_json?: string; revised_prompt?: string }> }
export type OpenAIModelList = { object: string; data: Array<{ id: string; object: string; owned_by: string }> }
