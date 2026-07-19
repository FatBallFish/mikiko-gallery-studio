import { ADMIN_PERMISSIONS, type AdminPermission, type ConfigItem } from '../../../shared/api-types'

export type ConfigValue = string | number | boolean | string[] | Record<string, unknown> | Array<Record<string, unknown>>
export type ConfigFieldType = 'number' | 'boolean' | 'text' | 'map' | 'list'
export type ConfigFieldMeta = { label: string; hint: string; type?: ConfigFieldType }
export type ConfigTabMeta = { label: string; detail: string }

export const generalConfigCategories = ['site', 'docs', 'public_gallery'] as const
export const forbiddenGeneralConfigCategories = ['auth_security', 'generation_limits', 'moderation', 'payments'] as const

const tabMeta: Record<string, ConfigTabMeta> = {
  auth_security: { label: '认证安全', detail: '登录令牌、刷新 Cookie 和会话安全相关配置。' },
  generation_limits: { label: '生成限制', detail: '控制提示词、参考图和上游单次请求容量的安全上限。' },
  billing_pricing: { label: '积分计费', detail: '积分汇率、任务倍率、自动质量和参考图附加费用。' },
  openai_compat: { label: '兼容接口', detail: 'OpenAI 兼容模型名到平台模型分组的映射。' },
  public_gallery: { label: '公开内容', detail: '图片公开申请、广场展示和内容可见性开关。' },
  moderation: { label: '内容审核', detail: '审核供应商和审核开关配置。' },
  payments: { label: '支付配置', detail: '底层支付配置、可见支付方式和自定义充值边界；日常套餐、订单与渠道实例运营优先在收银台页面完成。' },
  docs: { label: '开发文档', detail: '开放接口文档标题和基础路径。' },
}

const itemMeta: Record<string, ConfigFieldMeta> = {
  access_token_ttl_sec: { label: '访问令牌有效期', hint: 'Access Token 的有效秒数，过短会导致频繁刷新。', type: 'number' },
  refresh_token_ttl_sec: { label: '刷新令牌有效期', hint: 'Refresh Token 的有效秒数，决定用户可静默续期多久。', type: 'number' },
  refresh_cookie_name: { label: '刷新 Cookie 名称', hint: '浏览器保存 Refresh Token 的 HttpOnly Cookie 名称。', type: 'text' },
  max_image_count: { label: '单次最大出图数', hint: '作为模型未单独配置时的单次上游请求容量；任务总量超出后由系统自动分批。', type: 'number' },
  reference_image_max_mb: { label: '参考图大小上限 MB', hint: '单张参考图允许上传的最大体积。', type: 'number' },
  reference_image_max_count: { label: '参考图数量上限', hint: '图生图或参考图生成时允许携带的参考图数量。', type: 'number' },
  prompt_max_chars: { label: '提示词字数上限', hint: '用户主提示词允许输入的最大字符数。', type: 'number' },
  negative_prompt_max_chars: { label: '负面提示词字数上限', hint: '负面提示词允许输入的最大字符数。', type: 'number' },
  cny_per_point: { label: '人民币积分汇率', hint: '每积分对应的人民币金额，用于成本和充值核算。', type: 'text' },
  auto_base_resolution_default_by_group: { label: '自动基础分辨率默认值', hint: '不同模型分组在 auto 基础分辨率下默认解析到的 1K/2K/4K 档位。', type: 'map' },
  task_multipliers: { label: '任务类型倍率', hint: '文生图、图生图、参考图等任务类型的计费倍率。', type: 'map' },
  reference_image_extra: { label: '参考图附加费用', hint: '第一张和后续参考图额外计费系数。', type: 'map' },
  openai_compat_model_map: { label: '兼容模型映射', hint: '把 OpenAI 兼容接口中的模型名映射到平台模型分组。', type: 'map' },
  publish_request_enabled: { label: '允许申请公开', hint: '开启后用户可以把历史图片提交到公开审核。', type: 'boolean' },
  gallery_enabled: { label: '启用图片广场', hint: '控制公开图片广场入口和公开图片展示。', type: 'boolean' },
  provider: { label: '审核供应商', hint: '内容审核使用的供应商标识。', type: 'text' },
  enabled: { label: '启用', hint: '开启或关闭当前类目的核心能力。', type: 'boolean' },
  providers: { label: '支付渠道', hint: '可用支付渠道代码列表。', type: 'list' },
  custom_amount_enabled: { label: '允许自定义金额充值', hint: '开启后用户可在收银台输入自定义金额创建充值订单。', type: 'boolean' },
  custom_amount_min_cny: { label: '自定义金额最低值', hint: '用户自定义金额充值时允许提交的最低人民币金额。', type: 'text' },
  custom_amount_max_cny: { label: '自定义金额最高值', hint: '用户自定义金额充值时允许提交的最高人民币金额。', type: 'text' },
  custom_amount_cny_per_point: { label: '自定义充值汇率', hint: '自定义金额充值换算积分时使用的人民币积分汇率。', type: 'text' },
  visible_methods: { label: '可见支付方式', hint: '用户收银台展示的支付入口列表，包含入口名称、渠道类型、调度策略和排序。', type: 'list' },
  provider_instances: { label: '支付渠道实例', hint: '收银台底层渠道账号配置，包含商户配置、状态、限额和调度参数；密钥不会明文回显。', type: 'list' },
  scheduler_state: { label: '支付调度状态', hint: '多渠道账号轮询调度的运行状态，通常由系统维护。', type: 'map' },
  signup_trial: { label: '注册送体验额度', hint: '控制新用户注册后获得的体验额度金额、有效期、过期提醒和是否每人仅发放一次。', type: 'map' },
  title: { label: '文档标题', hint: '开发文档页面展示的标题。', type: 'text' },
  base_path: { label: '文档基础路径', hint: '开发文档站点挂载路径。', type: 'text' },
}

export function configTabMeta(tabKey: string): ConfigTabMeta {
  return tabMeta[tabKey] ?? { label: tabKey || '配置类目', detail: '该类目由后端配置中心返回，未知字段会保留原始值用于排障。' }
}

export function configFieldMeta(key: string, description?: string): ConfigFieldMeta {
  return itemMeta[key] ?? { label: key || '未知字段', hint: description || '该配置由后端配置中心提供，页面会保留原始值用于排障。' }
}

export function configPermission(tabKey: string): AdminPermission {
  if (tabKey === 'auth_security' || tabKey === 'payments') {
    return ADMIN_PERMISSIONS.manageDangerousConfig
  }
  return ADMIN_PERMISSIONS.manageConfig
}

export function configLockedDetail(permission: AdminPermission): string {
  if (permission === ADMIN_PERMISSIONS.manageDangerousConfig) {
    return '该类目包含支付、认证或密钥相关高危配置，仅超级管理员可保存。'
  }
  return '当前账号没有保存该类目的权限。'
}

export function configTabSummary(rows: ConfigItem[]) {
  const tabs = new Set(rows.map((row) => row.config_category || row.tab).filter(Boolean))
  return {
    tabCount: tabs.size,
    fieldCount: rows.length,
    dangerousTabCount: Array.from(tabs).filter((tab) => configPermission(tab) === ADMIN_PERMISSIONS.manageDangerousConfig).length,
  }
}

export function extractConfigValue(row: ConfigItem): ConfigValue {
  const source = row.config_value ?? safeParse(row.value)
  if (isRecord(source) && 'value' in source) return source.value as ConfigValue
  return source as ConfigValue
}

export function normalizeDraftValue(value: ConfigValue): unknown {
  return value
}

export function configValidateValue(key: string, value: ConfigValue | undefined) {
  if (value === undefined || value === null || value === '') return '配置值不能为空'
  if ((key === 'max_image_count' || key === 'reference_image_max_count') && Number(value) > 8) return '数量超过当前安全上限 8，请降低数量或先扩容任务处理能力。'
  if (key.includes('ttl') && Number(value) < 300) return 'Token TTL 低于 300 秒会触发频繁刷新。'
  if (key.includes('cny_per_point') && Number(value) <= 0) return '积分汇率必须大于 0。'
  if (key.startsWith('custom_amount_') && key.endsWith('_cny') && Number(value) <= 0) return '金额必须大于 0。'
  if (key === 'signup_trial' && isRecord(value)) {
    if (Number(value.points) < 0) return '体验额度金额不能小于 0。'
    if (value.enabled !== false && Number(value.valid_days) <= 0) return '体验额度有效期必须大于 0 天。'
    if (Number(value.expiry_reminder_days ?? 0) < 0) return '过期提醒天数不能小于 0。'
  }
  return null
}

export function inferConfigFieldType(value: ConfigValue): ConfigFieldType {
  if (typeof value === 'boolean') return 'boolean'
  if (typeof value === 'number') return 'number'
  if (Array.isArray(value)) return 'list'
  if (isRecord(value)) return 'map'
  return 'text'
}

export function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value))
}

export function isSameConfigValue(left: ConfigValue | undefined, right: ConfigValue | undefined) {
  return JSON.stringify(left) === JSON.stringify(right)
}

function safeParse(value: string): unknown {
  try {
    return JSON.parse(value)
  } catch {
    return value
  }
}
