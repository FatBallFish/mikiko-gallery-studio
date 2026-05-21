export type ApiEnvelope<T> = {
  code: string
  message: string
  data: T
  request_id: string
}

export const API_PATHS = {
  agent: {
    sendEmailCode: '/api/agent/auth/v1/email/send-code',
    loginEmailCode: '/api/agent/auth/v1/login/email-code',
    refreshSession: '/api/agent/auth/v1/session/refresh',
    logout: '/api/agent/auth/v1/logout',
    profile: '/api/agent/user/v1/profile',
    preferences: '/api/agent/user/v1/preferences',
    balance: '/api/agent/billing/v1/balance',
    ledger: '/api/agent/billing/v1/ledger',
    estimate: '/api/agent/billing/v1/estimate',
    redeemCode: '/api/agent/billing/v1/redeem-codes/redeem',
    capabilities: '/api/agent/image/v1/capabilities',
    referenceAssets: '/api/agent/image/v1/reference-assets',
    tasks: '/api/agent/image/v1/tasks',
    historyTasks: '/api/agent/image/v1/history/tasks',
    publishImage: '/api/agent/gallery/v1/images/{image_id}/publish',
  },
  open: {
    uploadSessions: '/api/open/image/v1/reference-assets/uploads',
    referenceAssets: '/api/open/image/v1/reference-assets',
    tasks: '/api/open/image/v1/tasks',
    balance: '/api/open/image/v1/balance',
    capabilities: '/api/open/image/v1/capabilities',
    estimate: '/api/open/image/v1/estimate',
    galleryImages: '/api/open/image/v1/gallery/images',
  },
  compat: {
    generations: '/v1/images/generations',
    edits: '/v1/images/edits',
    models: '/v1/models',
  },
  ops: {
    login: '/api/ops/admin/v1/auth/login',
    users: '/api/ops/admin/v1/users',
    modelProviders: '/api/ops/admin/v1/model-providers',
    modelRoutes: '/api/ops/admin/v1/model-routes/{route_id}',
    errorPolicies: '/api/ops/admin/v1/error-policies/{provider}',
    configTabs: '/api/ops/admin/v1/config-tabs',
    imageReviews: '/api/ops/admin/v1/image-reviews',
    tasks: '/api/ops/admin/v1/tasks',
    dashboard: '/api/ops/admin/v1/metrics/dashboard',
  },
} as const

export type PageResult<T> = {
  items: T[]
  total: number
  next_cursor?: string
}

export type UserProfile = {
  id: string
  email: string
  display_name: string
  avatar_initials: string
  tier: 'FREE' | 'PLUS' | 'PRO'
  group: string
  signature: string
  preferences: GenerationPreferences
}

export type GenerationPreferences = {
  model_group: string
  quality: string
  aspect_ratio: string
  image_count: number
}

export type Balance = {
  available_points: string
  frozen_points: string
  plan_name: string
  first_purchase_bonus: boolean
}

export type LedgerEntry = {
  id: string
  title: string
  occurred_at: string
  amount: string
  type: 'credit' | 'debit'
  detail: string
}

export type Capability = {
  model_groups: Array<{ id: string; name: string; provider: string; supports_reference: boolean; price_hint: string }>
  qualities: string[]
  aspect_ratios: string[]
  max_image_count: number
  task_types: Array<'text_to_image' | 'reference_to_image' | 'image_edit'>
}

export type EstimateRequest = {
  task_type: ImageTaskType
  model_group: string
  quality: string
  aspect_ratio: string
  image_count: number
  reference_asset_ids?: string[]
}

export type EstimateResult = {
  points: string
  formula: string
  resolved_quality: string
  sufficient: boolean
}

export type ImageTaskType = 'text_to_image' | 'reference_to_image' | 'image_edit'
export type ImageTaskStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled'
export type PublishStatus = 'private' | 'reviewing' | 'public' | 'rejected'

export type ReferenceAsset = {
  id: string
  name: string
  preview_url: string
  status: 'uploaded' | 'processing' | 'ready' | 'failed'
  size_bytes: number
  created_at: string
}

export type ImageResult = {
  id: string
  url: string
  width: number
  height: number
  publish_status: PublishStatus
}

export type ImageTask = {
  id: string
  title: string
  prompt: string
  task_type: ImageTaskType
  status: ImageTaskStatus
  model_group: string
  quality: string
  aspect_ratio: string
  image_count: number
  estimate_points: string
  progress: number
  provider: string
  route: string
  created_at: string
  updated_at: string
  failure_reason?: string
  reference_assets: ReferenceAsset[]
  results: ImageResult[]
}

export type CreateTaskRequest = EstimateRequest & {
  prompt: string
  negative_prompt?: string
  idempotency_key?: string
}

export type ApiKey = {
  id: string
  name: string
  access_key: string
  secret_preview?: string
  status: 'active' | 'disabled'
  scopes: string[]
  rpm_limit: number
  expires_at: string | null
  created_at: string
  last_used_at: string | null
}

export type EndpointDoc = {
  group: 'Agent API' | 'Open API' | 'OpenAI Compat' | 'Ops API'
  method: 'GET' | 'POST' | 'PUT' | 'DELETE'
  path: string
  title: string
  auth: string
  requestExample: string
  responseExample: string
}

export type AdminSession = {
  token: string
  admin_name: string
  role: string
}

export type AdminMetric = {
  label: string
  value: string
  trend: string
  tone: 'good' | 'warn' | 'bad' | 'neutral'
}

export type ProviderHealth = {
  provider: string
  status: 'healthy' | 'degraded' | 'down'
  latency_ms: number
  error_rate: string
  note: string
}

export type ConfigItem = {
  tab: string
  key: string
  value: string
  draft_value: string
  state: 'active' | 'draft' | 'conflict'
  version: number
  description: string
}

export type ModelRoute = {
  id: string
  scene: string
  provider: string
  policy: string
  priority: number
  enabled: boolean
  note: string
}

export type PriceRow = {
  id: string
  group: string
  q1k: string
  q2k: string
  q4k: string
  reference_multiplier: string
  version: number
  state: 'active' | 'draft'
}

export type ReviewItem = {
  id: string
  title: string
  owner: string
  task_type: ImageTaskType
  image_url: string
  status: 'pending' | 'approved' | 'rejected' | 'unpublished'
  reason: string
  created_at: string
}

export type AdminUser = {
  id: string
  email: string
  display_name: string
  status: 'active' | 'disabled' | 'pending'
  group: string
  balance: string
  created_at: string
  last_seen_at: string
}

export type AuditLog = {
  id: string
  actor: string
  action: string
  target: string
  detail: string
  created_at: string
}
