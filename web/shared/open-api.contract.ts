import { docsCopyableExamplesText, docsFromOpenApi, normalizeExamples } from './open-api-docs'

const endpoints = docsFromOpenApi({
  openapi: '3.1.0',
  paths: {
    '/api/agent/image/v1/tasks': {
      post: {
        tags: ['Agent Image'],
        summary: '创建生图任务',
        security: [{ bearerAuth: [] }],
        requestBody: {
          content: {
            'application/json': {
              example: {
                prompt: 'A neon studio',
                route_model_code: 'plus',
              },
            },
          },
        },
        responses: { 200: { description: 'OK' } },
      },
    },
    '/api/open/v1/image-tasks': {
      post: {
        summary: 'Open API 创建任务',
        security: [{ developerBearerAuth: [] }],
        requestBody: {
          content: {
            'application/json': {
              example: {
                project_id: 'owned-project',
                route_model_code: 'plus',
                prompt: 'A neon studio',
                quote_token: 'preflight-quote',
                idempotency_key: 'create-operation',
              },
            },
          },
        },
        responses: { 201: { description: 'Created' } },
      },
    },
    '/api/open/v1/image-tasks/preflight': {
      post: {
        summary: 'Open API 完整请求预检',
        security: [{ developerBearerAuth: [] }],
        requestBody: {
          content: {
            'application/json': {
              example: {
                project_id: 'owned-project',
                route_model_code: 'plus',
                prompt: 'A neon studio',
              },
            },
          },
        },
        responses: { 200: { description: 'OK' } },
      },
    },
  },
})

const agentEndpoint = endpoints.find((item) => item.path === '/api/agent/image/v1/tasks')
if (!agentEndpoint || agentEndpoint.group !== 'Agent API' || agentEndpoint.method !== 'POST' || agentEndpoint.auth !== 'Authorization required') {
  throw new Error(`OpenAPI endpoint normalizer should expose parsed endpoint docs, got ${JSON.stringify(endpoints)}`)
}
const requestExample = agentEndpoint.requestExample
for (const requiredSnippet of ['curl -X POST', 'Authorization: Bearer', 'Content-Type: application/json', '"prompt"', '"route_model_code"', '-d']) {
  if (!requestExample.includes(requiredSnippet)) {
    throw new Error(`OpenAPI request examples should be directly copyable and include ${requiredSnippet}, got ${requestExample}`)
  }
}
if (requestExample.trim() === 'curl /api/agent/image/v1/tasks') {
  throw new Error(`OpenAPI request examples should not fall back to path-only curl, got ${requestExample}`)
}
for (const path of ['/api/open/v1/image-tasks', '/api/open/v1/image-tasks/preflight']) {
  const endpoint = endpoints.find((item) => item.path === path)
  const example = endpoint?.requestExample ?? ''
  if (endpoint?.group !== 'Open API' || endpoint.method !== 'POST' || endpoint.auth !== 'Authorization required') {
    throw new Error(`Unified developer endpoint should require authorization, got ${JSON.stringify(endpoint)}`)
  }
  for (const snippet of [`curl -X POST "${path}"`, 'Authorization: Bearer <your_api_key>', 'Content-Type: application/json', '"project_id"', '"route_model_code"', '"prompt"']) {
    if (!example.includes(snippet)) throw new Error(`Unified developer example missing ${snippet}, got ${example}`)
  }
  for (const obsolete of ['X-Access-Key', 'X-Timestamp', 'X-Body-SHA256', 'X-Signature', '/api/open/image/v1/']) {
    if (example.includes(obsolete)) throw new Error(`Unified developer example contains obsolete signing contract ${obsolete}`)
  }
}
const createExample = endpoints.find((item) => item.path === '/api/open/v1/image-tasks')?.requestExample ?? ''
for (const field of ['"quote_token"', '"idempotency_key"']) {
  if (!createExample.includes(field)) throw new Error(`Create example missing ${field}`)
}

try {
  docsFromOpenApi({ name: 'pic-gallery', status: 'bootstrap-ready' })
  throw new Error('non-openapi payload should not normalize to an empty endpoint catalog')
} catch (caught) {
  if (!(caught instanceof Error) || caught.message !== '文档接口不可用') {
    throw caught
  }
}

const examples = normalizeExamples({
  items: [
    {
      id: 'create-task',
      title: '创建任务',
      language: 'curl',
      code: 'curl -X POST /api/open/v1/image-tasks',
    },
    {
      id: 'bad-html',
      title: '错误 HTML',
      language: 'html',
      code: '<!doctype html><div id="root"></div><script src="/src/main.tsx"></script>',
    },
  ],
})
const examplesText = docsCopyableExamplesText(examples)

if (!examplesText.includes('curl -X POST /api/open/v1/image-tasks')) {
  throw new Error(`docs examples copy text should keep real code examples, got ${examplesText}`)
}

if (examplesText.includes('<!doctype html') || examplesText.includes('/src/main.tsx') || examplesText.includes('id="root"')) {
  throw new Error(`docs examples copy text should drop HTML/bootstrap payloads, got ${examplesText}`)
}
