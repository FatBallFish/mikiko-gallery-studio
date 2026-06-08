import { docsFromOpenApi } from './open-api-docs'

const docs = docsFromOpenApi({
  openapi: '3.1.0',
  paths: {
    '/api/agent/ref-path': { $ref: '#/components/pathItems/AgentRefPath' },
    '/api/open/ref-operation': {
      post: { $ref: '#/components/pathItems/OpenRefOperation/post' },
    },
  },
  components: {
    pathItems: {
      AgentRefPath: {
        get: {
          summary: '引用路径查询',
          operationId: 'getRefPath',
          security: [{ bearerAuth: [] }],
          responses: { '200': { description: 'OK' } },
        },
      },
      OpenRefOperation: {
        post: {
          summary: '引用操作创建',
          operationId: 'createRefOperation',
          security: [],
          requestBody: {
            content: {
              'application/json': {
                example: { prompt: 'hello' },
              },
            },
          },
          responses: { '201': { description: 'Created' } },
        },
      },
    },
  },
})

const refPath = docs.find((item) => item.path === '/api/agent/ref-path')
if (!refPath || refPath.title !== '引用路径查询' || refPath.auth !== 'Authorization required') {
  throw new Error(`docsFromOpenApi should resolve path item $ref, got ${JSON.stringify(refPath)}`)
}

const refOperation = docs.find((item) => item.path === '/api/open/ref-operation')
if (!refOperation || !refOperation.requestExample.includes('"prompt": "hello"') || refOperation.auth !== 'Public') {
  throw new Error(`docsFromOpenApi should resolve operation $ref and examples, got ${JSON.stringify(refOperation)}`)
}
