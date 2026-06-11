import { API_PATHS, type AdminMetric, type AdminUser, type ApiKey, type AuditLog, type Capability, type ConfigItem, type EndpointDoc, type ImageTask, type LedgerEntry, type ModelRoute, type PriceRow, type ProviderHealth, type ReviewItem, type UserProfile } from './api-types'

export const demoProfile: UserProfile = {
  id: 'usr_01JYV4D8FISH',
  email: 'fatballfish@example.com',
  display_name: 'Fatball Fish',
  avatar_initials: 'FF',
  tier: 'PLUS',
  group: 'DEFAULT (1.0x)',
  signature: '用光、材质和构图构建可复用的视觉资产。',
  preferences: { model_group: 'plus-image', quality: 'auto', aspect_ratio: '1:1', image_count: 2, theme_mode: 'dark', accent_theme: 'amber' },
}

export const demoCapability: Capability = {
  model_groups: [
    { id: 'basic', code: 'basic', name: 'Basic', description: 'Fast public route', task_types: ['text_to_image'], qualities: ['auto', '1K'], effective_multiplier: '1.00000', prices: [{ task_type: 'text_to_image', quality: '1K', base_points: '2.00000', charged_points: '2.00000', display_points: '2.00' }], supports_reference: false, max_reference_image_count: 0 },
    { id: 'plus', code: 'plus', name: 'Plus', description: 'Balanced route with reference support', task_types: ['text_to_image', 'reference_to_image', 'image_edit'], qualities: ['auto', '1K', '2K'], effective_multiplier: '0.90000', prices: [{ task_type: 'text_to_image', quality: 'auto', base_points: '5.12500', charged_points: '4.61250', display_points: '4.61' }], supports_reference: true, max_reference_image_count: 3 },
    { id: 'pro', code: 'pro', name: 'Pro Studio', description: 'High quality grouped route', task_types: ['text_to_image', 'reference_to_image', 'image_edit'], qualities: ['auto', '2K', '4K'], effective_multiplier: '0.80000', prices: [{ task_type: 'text_to_image', quality: '2K', base_points: '8.00000', charged_points: '6.40000', display_points: '6.40' }], supports_reference: true, max_reference_image_count: 3 },
  ],
  qualities: ['auto', '1K', '2K', '4K'],
  aspect_ratios: ['1:1', '16:9', '9:16', '4:3', '3:4'],
  max_image_count: 4,
  task_types: ['text_to_image', 'reference_to_image', 'image_edit'],
}

export const demoImages = [
  'https://images.unsplash.com/photo-1519608487953-e999c86e7455?auto=format&fit=crop&w=1200&q=80',
  'https://images.unsplash.com/photo-1500530855697-b586d89ba3ee?auto=format&fit=crop&w=1200&q=80',
  'https://images.unsplash.com/photo-1500534314209-a25ddb2bd429?auto=format&fit=crop&w=1200&q=80',
  'https://images.unsplash.com/photo-1519681393784-d120267933ba?auto=format&fit=crop&w=1200&q=80',
  'https://images.unsplash.com/photo-1493246507139-91e8fad9978e?auto=format&fit=crop&w=1200&q=80',
]

export const initialLedger: LedgerEntry[] = [
  { id: 'led_1004', title: '生图任务: Cinematic watch setting', occurred_at: '2026-05-21 14:32', amount: '-5.12500', type: 'debit', detail: 'Plus Image / auto / 1 张' },
  { id: 'led_1003', title: '兑换码: WELCOME-2026', occurred_at: '2026-05-21 10:15', amount: '+20.00000', type: 'credit', detail: '活动兑换' },
  { id: 'led_1002', title: '生图任务: Futuristic workspace', occurred_at: '2026-05-20 21:44', amount: '-2.00000', type: 'debit', detail: 'Basic Image / 1K / 1 张' },
  { id: 'led_1001', title: '注册奖励', occurred_at: '2026-05-19 08:30', amount: '+25.00000', type: 'credit', detail: '系统发放' },
]

export const initialTasks: ImageTask[] = [
  {
    id: 'task_01JYV4LUXE',
    title: 'Cinematic luxury watch setting',
    prompt: 'Cinematic luxury watch in a dark gallery, amber rim light, glass reflections, ultra detailed.',
    task_type: 'reference_to_image',
    status: 'succeeded',
    model_group: 'plus-image',
    quality: '2K',
    aspect_ratio: '16:9',
    image_count: 2,
    estimate_points: '5.12500',
    progress: 100,
    progress_stage: 'completed',
    progress_message: '生成完成，结果已同步到资产',
    provider: 'OpenAI',
    route: 'primary',
    created_at: '2026-05-21 14:28',
    updated_at: '2026-05-21 14:32',
    reference_assets: [],
    results: [
      { id: 'img_luxe_1', url: demoImages[0], width: 1536, height: 864, publish_status: 'public' },
      { id: 'img_luxe_2', url: demoImages[1], width: 1536, height: 864, publish_status: 'private' },
    ],
  },
  {
    id: 'task_01JYV4NEON',
    title: 'Futuristic creative workspace',
    prompt: 'A futuristic creative workspace with neon glass desk, dark blue and emerald lighting.',
    task_type: 'text_to_image',
    status: 'succeeded',
    model_group: 'basic-image',
    quality: '1K',
    aspect_ratio: '1:1',
    image_count: 1,
    estimate_points: '2.00000',
    progress: 100,
    progress_stage: 'completed',
    progress_message: '生成完成，结果已同步到资产',
    provider: 'OpenRouter',
    route: 'fallback-capability',
    created_at: '2026-05-20 21:40',
    updated_at: '2026-05-20 21:44',
    reference_assets: [],
    results: [{ id: 'img_neon_1', url: demoImages[2], width: 1024, height: 1024, publish_status: 'reviewing' }],
  },
]

export const initialKeys: ApiKey[] = [
  { id: 'key_live', name: 'Production Gallery Bot', access_key: 'pk_live_4kL8m2z9X1', status: 'active', scopes: ['images:write', 'images:read', 'balance:read'], total_quota_points: '500.00000', daily_quota_points: '60.00000', total_quota_used_points: '128.50000', daily_quota_used_points: '12.00000', rpm_limit: 60, expires_at: null, created_at: '2026-05-19', last_used_at: '2026-05-21 13:12' },
  { id: 'key_test', name: 'Local Playground', access_key: 'pk_test_m9R2p3B7', status: 'active', scopes: ['images:write', 'images:read'], total_quota_points: null, daily_quota_points: '20.00000', total_quota_used_points: '3.00000', daily_quota_used_points: '1.00000', rpm_limit: 20, expires_at: '2026-12-31', created_at: '2026-05-20', last_used_at: '2026-05-21 09:22' },
]

export const endpointDocs: EndpointDoc[] = [
  { group: 'Agent API', method: 'POST', path: API_PATHS.agent.sendEmailCode, title: '发送邮箱验证码', auth: 'none', requestExample: '{"email":"you@example.com","scene":"login"}', responseExample: '{"cooldown_seconds":60}' },
  { group: 'Agent API', method: 'POST', path: API_PATHS.agent.loginEmailCode, title: '邮箱验证码登录/注册', auth: 'none', requestExample: '{"email":"you@example.com","code":"123456"}', responseExample: '{"access_token":"mock_access","profile":{...}}' },
  { group: 'Agent API', method: 'GET', path: API_PATHS.agent.profile, title: '获取个人资料', auth: 'Access Token', requestExample: 'curl /api/agent/user/v1/profile', responseExample: '{"id":"usr_01","display_name":"Fatball Fish","preferences":{...}}' },
  { group: 'Agent API', method: 'GET', path: API_PATHS.agent.balance, title: '获取积分余额', auth: 'Access Token', requestExample: 'curl /api/agent/billing/v1/balance', responseExample: '{"available_points":"182.50000","frozen_points":"0.00000"}' },
  { group: 'Agent API', method: 'GET', path: API_PATHS.agent.estimate, title: '价格预估', auth: 'Access Token', requestExample: '{"task_type":"reference_to_image","route_model_code":"plus"}', responseExample: '{"charged_points":"15.37500","display_points":"15.38","resolved_quality":"2K"}' },
  { group: 'Agent API', method: 'POST', path: API_PATHS.agent.referenceAssets, title: '上传参考图', auth: 'Access Token', requestExample: 'multipart/form-data image=@ref.png', responseExample: '{"reference_asset_id":"refasset_01","status":"ready"}' },
  { group: 'Agent API', method: 'POST', path: API_PATHS.agent.tasks, title: '创建图片生成任务', auth: 'Access Token', requestExample: '{"task_type":"reference_to_image","prompt":"...","route_model_code":"plus"}', responseExample: '{"task_id":"task_x","status":"queued","poll_after_ms":2000}' },
  { group: 'Agent API', method: 'GET', path: API_PATHS.agent.historyTasks, title: '历史任务列表', auth: 'Access Token', requestExample: 'curl /api/agent/image/v1/history/tasks', responseExample: '{"items":[{"task_id":"task_x","status":"succeeded"}]}' },
  { group: 'Open API', method: 'POST', path: API_PATHS.open.tasks, title: 'AK/SK 创建生图任务', auth: 'X-Access-Key + X-Signature', requestExample: '{"prompt":"...","response_mode":"sync"}', responseExample: '{"task_id":"task_x","images":[...]}' },
  { group: 'Open API', method: 'GET', path: API_PATHS.open.capabilities, title: '查询 Key 能力矩阵', auth: 'AK/SK', requestExample: 'curl /api/open/image/v1/capabilities', responseExample: '{"model_groups":[...]}' },
  { group: 'OpenAI Compat', method: 'POST', path: API_PATHS.compat.generations, title: 'OpenAI 兼容文生图', auth: 'Bearer sk-*', requestExample: '{"model":"gpt-image-2","prompt":"...","size":"1024x1024"}', responseExample: '{"created":1779300000,"data":[{"url":"https://..."}]}' },
  { group: 'OpenAI Compat', method: 'POST', path: API_PATHS.compat.edits, title: 'OpenAI 兼容图片编辑', auth: 'Bearer sk-*', requestExample: 'multipart image + prompt + model', responseExample: '{"data":[{"b64_json":"..."}]}' },
  { group: 'OpenAI Compat', method: 'GET', path: API_PATHS.compat.models, title: '兼容模型列表', auth: 'Bearer sk-*', requestExample: 'curl /v1/models', responseExample: '{"data":[{"id":"gpt-image-2"}]}' },
  { group: 'Ops API', method: 'GET', path: API_PATHS.ops.dashboard, title: '运营大盘', auth: 'Admin Token', requestExample: 'curl /api/ops/admin/v1/metrics/dashboard', responseExample: '{"metrics":[...],"providers":[...]}' },
  { group: 'Ops API', method: 'GET', path: API_PATHS.ops.configTabs, title: '配置中心数据', auth: 'Admin Token', requestExample: 'curl /api/ops/admin/v1/config-tabs', responseExample: '{"items":[{"key":"generation.max_count"}]}' },
  { group: 'Ops API', method: 'GET', path: API_PATHS.ops.imageReviews, title: '待审核图片列表', auth: 'Admin Token', requestExample: 'curl /api/ops/admin/v1/image-reviews', responseExample: '{"items":[{"image_id":"img_1","status":"pending"}]}' },
]

export const adminMetrics: AdminMetric[] = [
  { label: '今日生成次数', value: '3,184', trend: '+12%', tone: 'good' },
  { label: '今日收入', value: '¥1,240', trend: '+8%', tone: 'good' },
  { label: '平均生成耗时', value: '8.4s', trend: '+1.2s', tone: 'warn' },
  { label: '活跃用户', value: '412', trend: '+5%', tone: 'neutral' },
]

export const providerHealth: ProviderHealth[] = [
  { provider: 'OpenAI', status: 'healthy', latency_ms: 820, error_rate: '0.3%', note: '主路由健康' },
  { provider: 'OpenRouter', status: 'healthy', latency_ms: 1140, error_rate: '0.8%', note: '备用路由可切换' },
  { provider: 'Task Worker', status: 'degraded', latency_ms: 2400, error_rate: '2.4%', note: '队列等待 12' },
]

export const initialConfig: ConfigItem[] = [
  { tab: 'Generation Limits', key: 'max_image_count', value: '{"value":4}', draft_value: '{"value":4}', state: 'active', version: 7, description: '单次最大输出图片数量' },
  { tab: 'Generation Limits', key: 'reference_image_max_count', value: '{"value":3}', draft_value: '{"value":3}', state: 'active', version: 4, description: '参考图最大数量' },
  { tab: 'Billing & Pricing', key: 'cny_per_point', value: '{"value":"0.31250"}', draft_value: '{"value":"0.31250"}', state: 'active', version: 9, description: '人民币兑换积分比例' },
  { tab: 'Billing & Pricing', key: 'task_multipliers', value: '{"value":{"text_to_image":"1.00000","image_edit":"1.25000","reference_generate":"1.15000"}}', draft_value: '{"value":{"text_to_image":"1.00000","image_edit":"1.25000","reference_generate":"1.15000"}}', state: 'active', version: 9, description: '任务类型倍率' },
  { tab: 'OpenAI Compatibility', key: 'openai_compat_model_map', value: '{"value":{"gpt-image-2":"plus"}}', draft_value: '{"value":{"gpt-image-2":"plus"}}', state: 'active', version: 3, description: 'OpenAI 兼容模型 ID 到路由模型 code 的映射' },
  { tab: 'Auth & Security', key: 'access_token_ttl_sec', value: '{"value":600}', draft_value: '{"value":600}', state: 'active', version: 2, description: 'Access Token 有效期' },
  { tab: 'Public Gallery', key: 'publish_request_enabled', value: '{"value":true}', draft_value: '{"value":true}', state: 'active', version: 5, description: '公开图是否允许申请发布' },
]

export const initialRoutes: ModelRoute[] = [
  { id: 'route_gen', scene: 'Compat /images/generations', provider: 'OpenAI', policy: '主路由', priority: 1, enabled: true, note: '失败按错误码决定重试或切换' },
  { id: 'route_edit', scene: 'Compat /images/edits', provider: 'OpenAI', policy: '图输入优先', priority: 1, enabled: true, note: 'mask 与 image 校验前置' },
  { id: 'route_ref', scene: '参考图生成', provider: 'OpenRouter', policy: '能力优先', priority: 2, enabled: true, note: '仅命中支持 image input 的模型' },
]

export const initialPrices: PriceRow[] = [
  { id: 'price_basic', group: 'Basic', q1k: '2.00000', q2k: '4.00000', q4k: '8.00000', reference_multiplier: '1.20', version: 3, state: 'active' },
  { id: 'price_plus', group: 'Plus', q1k: '5.00000', q2k: '8.00000', q4k: '16.00000', reference_multiplier: '1.25', version: 6, state: 'active' },
  { id: 'price_pro', group: 'Pro Studio', q1k: '8.00000', q2k: '14.00000', q4k: '28.00000', reference_multiplier: '1.30', version: 1, state: 'active' },
]

export const initialReviews: ReviewItem[] = [
  { id: 'rev_1', title: 'Amber Cathedral', owner: 'u_1024', task_type: 'reference_to_image', image_url: demoImages[3], status: 'pending', reason: '构图稳定，可进入广场', created_at: '2026-05-21 10:24' },
  { id: 'rev_2', title: 'Mercury Figure', owner: 'u_2468', task_type: 'image_edit', image_url: demoImages[4], status: 'pending', reason: '需检查是否含敏感人物元素', created_at: '2026-05-21 09:52' },
]

export const initialUsers: AdminUser[] = [
  { id: 'u_1024', email: 'artist@example.com', display_name: 'Amber Artist', status: 'active', group: 'DEFAULT', balance: '128.50000', created_at: '2026-05-19', last_seen_at: '2026-05-21 13:10' },
  { id: 'u_2468', email: 'maker@example.com', display_name: 'Mercury Maker', status: 'pending', group: 'CREATOR', balance: '45.00000', created_at: '2026-05-20', last_seen_at: '2026-05-21 12:42' },
  { id: 'u_3141', email: 'ops-test@example.com', display_name: 'Glass Sonata', status: 'disabled', group: 'DEFAULT', balance: '0.12500', created_at: '2026-05-18', last_seen_at: '2026-05-20 18:21' },
]

export const initialAudit: AuditLog[] = [
  { id: 'aud_4', actor: 'Admin Liu', action: 'UPDATE_CONFIG', target: 'generation.max_image_count', detail: '3 -> 4', created_at: '2026-05-21 10:24' },
  { id: 'aud_3', actor: 'Admin Liu', action: 'PUBLISH_ROUTE', target: 'Compat /images/generations', detail: 'OpenAI 主路由生效', created_at: '2026-05-21 10:11' },
  { id: 'aud_2', actor: 'System', action: 'WORKER_HEARTBEAT', target: 'worker-pool', detail: '4 nodes healthy', created_at: '2026-05-21 09:52' },
]
