import type { EndpointDoc } from '../../../shared/api-types'
import {
  docsAuthLabel,
  docsEndpointCountLabel,
  docsErrorRows,
  docsGroupOptions,
  docsGroupTagLabel,
  docsSearchPlaceholder,
  docsSectionLabels,
  filterDocsEndpoints,
} from './docsPageModel'

const options = docsGroupOptions()
const optionValues = options.map((option) => option.value).join(',')
const optionLabels = options.map((option) => option.label).join(',')
if (optionValues !== 'All,Agent API,Open API,OpenAI Compat,Ops API') {
  throw new Error(`docs group options must preserve raw filter values, got ${optionValues}`)
}
if (optionLabels !== '全部接口,用户端 API,开放 API,OpenAI 兼容接口,管理后台 API') {
  throw new Error(`docs group options should expose readable labels, got ${optionLabels}`)
}

for (const forbidden of ['endpoint', 'auth', 'title']) {
  if (docsSearchPlaceholder.toLowerCase().includes(forbidden)) {
    throw new Error(`docs search placeholder should not expose raw developer terms, got ${docsSearchPlaceholder}`)
  }
}
if (!docsSearchPlaceholder.includes('路径') || !docsSearchPlaceholder.includes('接口名称') || !docsSearchPlaceholder.includes('鉴权方式')) {
  throw new Error(`docs search placeholder should explain searchable fields, got ${docsSearchPlaceholder}`)
}

const countLabel = docsEndpointCountLabel(3, 12)
if (countLabel !== '已显示 3 / 共 12 个接口') {
  throw new Error(`docs endpoint count should be localized, got ${countLabel}`)
}
if (docsEndpointCountLabel(0, 0).includes('endpoint')) {
  throw new Error('docs endpoint count should not expose English endpoint wording')
}

if (docsGroupTagLabel('Agent API') !== '用户端 API' || docsGroupTagLabel('Open API') !== '开放 API' || docsGroupTagLabel('Ops API') !== '管理后台 API') {
  throw new Error('known docs groups should be localized')
}
if (docsGroupTagLabel('Partner API') !== 'Partner API') {
  throw new Error('unknown docs groups should preserve raw values for troubleshooting')
}

if (docsAuthLabel('Authorization required') !== '需要登录或访问令牌' || docsAuthLabel('Public') !== '公开访问') {
  throw new Error('known docs auth labels should be localized')
}
if (docsAuthLabel('ApiKey') !== 'ApiKey') {
  throw new Error('unknown docs auth labels should preserve raw values')
}

if (docsSectionLabels.eyebrow !== '开发者中心' || docsSectionLabels.errors !== '错误码示例' || docsSectionLabels.examples !== '接口示例') {
  throw new Error(`docs section labels should be localized, got ${JSON.stringify(docsSectionLabels)}`)
}

const errorRows = docsErrorRows([
  { code: 'MODEL_ROUTE_NOT_FOUND', http_status: 404, message: 'route model not found', retryable: false },
  { code: 'PAYMENT_PROVIDER_UNAVAILABLE', http_status: 409, message: 'provider unavailable', retryable: true },
  ['legacy_error', '旧格式错误'],
])
if (errorRows[0]?.statusLabel !== 'HTTP 404' || errorRows[0]?.retryableLabel !== '不可重试' || !errorRows[0]?.recoveryHint.includes('模型编码')) {
  throw new Error(`docs error rows should explain route model recovery, got ${JSON.stringify(errorRows[0])}`)
}
if (errorRows[1]?.retryableLabel !== '可重试' || !errorRows[1]?.recoveryHint.includes('支付方式')) {
  throw new Error(`docs error rows should explain payment provider recovery, got ${JSON.stringify(errorRows[1])}`)
}
if (errorRows[2]?.code !== 'legacy_error' || errorRows[2]?.statusLabel !== 'HTTP -' || errorRows[2]?.retryableLabel !== '未标记' || errorRows[2]?.recoveryHint !== '请根据错误信息修正请求参数或联系平台管理员。') {
  throw new Error(`docs error rows should normalize legacy fallback rows, got ${JSON.stringify(errorRows[2])}`)
}

const rows = [
  endpoint({ group: 'Agent API', path: '/api/agent/image/v1/tasks', title: '创建生图任务', auth: 'Authorization required' }),
  endpoint({ group: 'Open API', path: '/api/open/image/v1/tasks', title: 'Open API 创建任务', auth: 'Public' }),
  endpoint({ group: 'Ops API', path: '/api/ops/admin/v1/readiness', title: '上线检查', auth: 'Authorization required' }),
]
const agentRows = filterDocsEndpoints(rows, 'Agent API', '登录 任务')
if (agentRows.length !== 1 || agentRows[0]?.path !== '/api/agent/image/v1/tasks') {
  throw new Error(`docs endpoint filtering should include localized auth/group/title/path search terms, got ${JSON.stringify(agentRows)}`)
}
const allRows = filterDocsEndpoints(rows, 'All', '管理后台')
if (allRows.length !== 1 || allRows[0]?.group !== 'Ops API') {
  throw new Error(`docs endpoint search should include localized group labels, got ${JSON.stringify(allRows)}`)
}

function endpoint(patch: Partial<EndpointDoc>): EndpointDoc {
  return {
    group: patch.group ?? 'Agent API',
    method: patch.method ?? 'POST',
    path: patch.path ?? '/api/agent/image/v1/tasks',
    title: patch.title ?? '创建生图任务',
    auth: patch.auth ?? 'Authorization required',
    requestExample: patch.requestExample ?? 'curl /api/agent/image/v1/tasks',
    responseExample: patch.responseExample ?? '{}',
  }
}
