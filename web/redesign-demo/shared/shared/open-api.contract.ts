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
    '/api/open/image/v1/tasks': {
      post: {
        summary: 'Open API 创建任务',
        security: [{ accessKeyAuth: [], accessTimestamp: [], accessBodySHA256: [], accessSignature: [] }],
        requestBody: {
          content: {
            'application/json': {
              example: {
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
const openApiExample = endpoints.find((item) => item.path === '/api/open/image/v1/tasks')?.requestExample ?? ''
for (const requiredSnippet of ['X-Access-Key', 'X-Timestamp', 'X-Body-SHA256', 'X-Signature']) {
  if (!openApiExample.includes(requiredSnippet)) {
    throw new Error(`Open API request examples should include AK/SK signature header ${requiredSnippet}, got ${openApiExample}`)
  }
}
if (openApiExample.includes('Authorization: Bearer')) {
  throw new Error(`Open API request examples should not use bearer auth, got ${openApiExample}`)
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
      code: 'curl -X POST /api/open/image/v1/tasks',
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

if (!examplesText.includes('curl -X POST /api/open/image/v1/tasks')) {
  throw new Error(`docs examples copy text should keep real code examples, got ${examplesText}`)
}

if (examplesText.includes('<!doctype html') || examplesText.includes('/src/main.tsx') || examplesText.includes('id="root"')) {
  throw new Error(`docs examples copy text should drop HTML/bootstrap payloads, got ${examplesText}`)
}
