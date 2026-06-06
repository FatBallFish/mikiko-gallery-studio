import type { EndpointDoc } from './api-types'

const docsUnavailableMessage = '文档接口不可用'
const httpMethods = ['get', 'post', 'put', 'delete', 'patch'] as const

export type DocsExample = {
  id: string
  title: string
  language: string
  code: string
  name: string
  description?: string
  method: string
  path: string
  request: unknown
  response: unknown
}

export type DocsErrorCode = {
  code: string
  http_status?: number
  message: string
  retryable?: boolean
}

function endpointGroup(path: string): EndpointDoc['group'] {
  if (path.startsWith('/api/agent')) return 'Agent API'
  if (path.startsWith('/api/open')) return 'Open API'
  if (path.startsWith('/api/ops')) return 'Ops API'
  if (path.startsWith('/v1')) return 'OpenAI Compat'
  return 'Open API'
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value))
}

export function docsFromOpenApi(spec: unknown): EndpointDoc[] {
  if (!isRecord(spec) || typeof spec.openapi !== 'string' || !isRecord(spec.paths)) throw new Error(docsUnavailableMessage)

  return Object.entries(spec.paths).flatMap(([path, methods]) => {
    if (!isRecord(methods)) return []

    return Object.entries(methods).flatMap(([method, operation]) => {
      if (!httpMethods.includes(method as typeof httpMethods[number]) || !isRecord(operation)) return []
      return [{
        group: endpointGroup(path),
        method: method.toUpperCase() as EndpointDoc['method'],
        path,
        title: String(operation.summary ?? operation.operationId ?? path),
        auth: Array.isArray(operation.security) && operation.security.length > 0 ? 'Authorization required' : 'Public',
        requestExample: docsRequestExample(path, method.toUpperCase(), operation),
        responseExample: JSON.stringify(isRecord(operation.responses) ? operation.responses : {}, null, 2).slice(0, 1200),
      }]
    })
  })
}

export function normalizeExamples(payload: unknown): DocsExample[] {
  const rows = Array.isArray(payload)
    ? payload
    : isRecord(payload) && Array.isArray(payload.items)
      ? payload.items
      : isRecord(payload) && Array.isArray(payload.examples)
        ? payload.examples
        : []

  return rows
    .filter(isRecord)
    .map((item) => {
      const method = String(item.method ?? 'GET').toUpperCase()
      const path = String(item.path ?? '')
      const title = String(item.title ?? item.name ?? `${method} ${path}`.trim())
      const language = String(item.language ?? 'json').toLowerCase()
      const code = String(item.code ?? JSON.stringify({ request: item.request ?? {}, response: item.response ?? {} }, null, 2))
      return {
        id: String(item.id ?? `${method}-${path || title}`.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')),
        title,
        language,
        code,
        name: title,
        description: item.description === undefined ? undefined : String(item.description),
        method,
        path,
        request: item.request ?? {},
        response: item.response ?? {},
      }
    })
    .filter((item) => item.id && item.title && item.language && item.code && !isHtmlBootstrapPayload(item.code))
}

export function normalizeErrors(payload: unknown): DocsErrorCode[] {
  const rows = Array.isArray(payload)
    ? payload
    : isRecord(payload) && Array.isArray(payload.items)
      ? payload.items
      : isRecord(payload) && Array.isArray(payload.codes)
        ? payload.codes
        : []

  return rows
    .map((item) => {
      if (Array.isArray(item)) return { code: String(item[0] ?? ''), message: String(item[1] ?? '') }
      if (isRecord(item)) {
        return {
          code: String(item.code ?? ''),
          http_status: typeof item.http_status === 'number' ? item.http_status : undefined,
          message: String(item.message ?? item.detail ?? ''),
          retryable: typeof item.retryable === 'boolean' ? item.retryable : undefined,
        }
      }
      return { code: String(item), message: '' }
    })
    .filter((item) => item.code)
}

export function docsCopyableExamplesText(examples: DocsExample[]): string {
  return examples.length
    ? examples.map((item) => `# ${item.title}\n${item.code}`).join('\n\n')
    : 'curl -G "/docs/examples"'
}

function isHtmlBootstrapPayload(code: string): boolean {
  const normalized = code.trim().toLowerCase()
  return normalized.includes('<!doctype html') || (normalized.includes('id="root"') && normalized.includes('/src/main'))
}

function docsRequestExample(path: string, method: string, operation: Record<string, unknown>) {
  const lines = [`curl -X ${method} "${path}"`]
  lines.push(...docsAuthHeaders(operation))
  const jsonBody = jsonRequestExample(operation)
  if (jsonBody !== undefined) {
    lines.push('  -H "Content-Type: application/json"')
    lines.push(`  -d '${JSON.stringify(jsonBody, null, 2)}'`)
  }
  return lines.join(' \\\n')
}

function jsonRequestExample(operation: Record<string, unknown>): unknown | undefined {
  const requestBody = operation.requestBody
  if (!isRecord(requestBody) || !isRecord(requestBody.content)) return undefined
  const jsonContent = requestBody.content['application/json']
  if (!isRecord(jsonContent)) return {}
  if (jsonContent.example !== undefined) return jsonContent.example
  if (isRecord(jsonContent.examples)) {
    const first = Object.values(jsonContent.examples).find(isRecord)
    if (isRecord(first) && first.value !== undefined) return first.value
  }
  return {}
}

function docsAuthHeaders(operation: Record<string, unknown>) {
  const schemes = securitySchemeNames(operation)
  if (!schemes.length) return []
  if (schemes.includes('accessKeyAuth') || schemes.includes('accessSignature')) {
    return [
      '  -H "X-Access-Key: <your_access_key>"',
      '  -H "X-Timestamp: <rfc3339_timestamp>"',
      '  -H "X-Body-SHA256: <base64url_body_sha256>"',
      '  -H "X-Signature: <hmac_sha256_signature>"',
    ]
  }
  if (schemes.includes('compatBearerAuth')) return ['  -H "Authorization: Bearer <your_sk_key>"']
  if (schemes.includes('bearerAuth')) return ['  -H "Authorization: Bearer <your_access_token>"']
  return []
}

function securitySchemeNames(operation: Record<string, unknown>) {
  if (!Array.isArray(operation.security)) return []
  return operation.security.flatMap((item) => isRecord(item) ? Object.keys(item) : [])
}
