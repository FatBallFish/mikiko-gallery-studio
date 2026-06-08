import type { EndpointDoc } from '../../../shared/api-types'
import type { DocsErrorCode } from '../../../shared/open-api-docs'

const groupLabels: Record<string, string> = {
  All: '全部接口',
  'Agent API': '用户端 API',
  'Open API': '开放 API',
  'OpenAI Compat': 'OpenAI 兼容接口',
  'Ops API': '管理后台 API',
}

const authLabels: Record<string, string> = {
  'Authorization required': '需要登录或访问令牌',
  Public: '公开访问',
}

const rawGroups = ['All', 'Agent API', 'Open API', 'OpenAI Compat', 'Ops API'] as const

export const docsSearchPlaceholder = '搜索路径 / 接口名称 / 鉴权方式'

export const docsSectionLabels = {
  eyebrow: '开发者中心',
  endpointCountSuffix: '个接口',
  realtimeCatalog: 'OpenAPI 实时目录',
  request: '请求示例',
  response: '响应示例',
  authPrefix: '鉴权',
  errors: '错误码示例',
  examples: '接口示例',
} as const

export type DocsErrorInput = DocsErrorCode | [string, string]

export type DocsErrorRow = {
  code: string
  statusLabel: string
  message: string
  retryableLabel: string
  recoveryHint: string
}

export function docsGroupOptions() {
  return rawGroups.map((value) => ({ value, label: docsGroupTagLabel(value) }))
}

export function docsGroupTagLabel(group?: string | null) {
  const normalized = normalize(group)
  return groupLabels[normalized] ?? normalized
}

export function docsAuthLabel(auth?: string | null) {
  const normalized = normalize(auth)
  return authLabels[normalized] ?? normalized
}

export function docsEndpointCountLabel(filtered: number, total: number) {
  return `已显示 ${filtered} / 共 ${total} 个接口`
}

export function filterDocsEndpoints(rows: EndpointDoc[], group: string, query: string) {
  const terms = query.trim().toLowerCase().split(/\s+/).filter(Boolean)
  return rows.filter((item) => {
    const matchesGroup = group === 'All' || item.group === group
    if (!matchesGroup) return false
    if (!terms.length) return true
    const searchText = docsEndpointSearchText(item)
    return terms.every((term) => searchText.includes(term))
  })
}

export function docsErrorRows(rows: DocsErrorInput[]): DocsErrorRow[] {
  return rows.map((row) => {
    const code = Array.isArray(row) ? row[0] : row.code
    const httpStatus = Array.isArray(row) ? undefined : row.http_status
    const retryable = Array.isArray(row) ? undefined : row.retryable
    return {
      code,
      statusLabel: httpStatus ? `HTTP ${httpStatus}` : 'HTTP -',
      message: Array.isArray(row) ? row[1] : row.message,
      retryableLabel: retryable === true ? '可重试' : retryable === false ? '不可重试' : '未标记',
      recoveryHint: docsErrorRecoveryHint(code),
    }
  })
}

function docsEndpointSearchText(item: EndpointDoc) {
  return [
    item.group,
    docsGroupTagLabel(item.group),
    item.method,
    item.path,
    item.title,
    item.auth,
    docsAuthLabel(item.auth),
  ].filter(Boolean).join(' ').toLowerCase()
}

function docsErrorRecoveryHint(code: string) {
  const normalized = normalize(code).toUpperCase()
  if (normalized.includes('MODEL_ROUTE_NOT_FOUND')) return '检查请求中的模型编码是否在能力接口返回范围内，或联系管理员确认路由模型已启用。'
  if (normalized.includes('MODEL_ROUTE_NO_CANDIDATE')) return '当前模型分组没有可用候选模型，请稍后重试或切换模型分组。'
  if (normalized.includes('ROUTE_MODEL_PRICE_MISSING')) return '该模型尚未配置价格，管理员完成价格配置后再发起生成。'
  if (normalized.includes('PAYMENT_METHOD_UNAVAILABLE') || normalized.includes('PAYMENT_PROVIDER_UNAVAILABLE')) return '切换支付方式重试，或联系管理员检查支付方式、渠道实例和金额限制配置。'
  if (normalized.includes('PAYMENT_TOO_MANY_PENDING_ORDERS')) return '先完成或取消已有待支付订单，再重新创建订单。'
  if (normalized.includes('PAYMENT_SIGNATURE_INVALID')) return '检查支付回调密钥、签名算法、商户号和回调地址是否匹配。'
  if (normalized.includes('PAYMENT_AMOUNT_MISMATCH')) return '核对订单金额与渠道回调金额，确认无误后由管理员查单或人工处理。'
  if (normalized.includes('INSUFFICIENT') || normalized.includes('BALANCE')) return '充值或降低生成参数后重试。'
  if (normalized.includes('RATE_LIMIT')) return '降低调用频率，等待限速窗口恢复后重试。'
  if (normalized.includes('SIGNATURE')) return '检查 AK/SK、时间戳、请求体哈希和签名串。'
  if (normalized.includes('PROVIDER')) return '稍后重试，若持续失败请切换模型分组或联系平台管理员。'
  return '请根据错误信息修正请求参数或联系平台管理员。'
}

function normalize(value?: string | null) {
  return (value ?? '').trim()
}
