import type { Balance, Capability, CreateTaskRequest, EndpointDoc, EstimateRequest, EstimateResult, ImageResult, ImageTask, OpenAIImageEditRequest, OpenAIImageGenerationRequest, OpenAIImageResponse, OpenAIModelList, OpenReferenceAssetUploadSessionRequest, OpenReferenceAssetUploadSessionResponse, PageResult, ReferenceAsset } from './api-types'
import { API_PATHS } from './api-types'
import { normalizePage, sharedApiClient } from './http-client'
import { toImageResult, toReferenceAsset, toTask } from './user-api'

function endpointGroup(path: string): EndpointDoc['group'] {
  if (path.startsWith('/api/agent')) return 'Agent API'
  if (path.startsWith('/api/open')) return 'Open API'
  if (path.startsWith('/api/ops')) return 'Ops API'
  if (path.startsWith('/v1')) return 'OpenAI Compat'
  return 'Open API'
}

function docsFromOpenApi(spec: any): EndpointDoc[] {
  const paths = spec.paths ?? {}
  return Object.entries(paths).flatMap(([path, methods]) => (
    Object.entries(methods as Record<string, any>)
      .filter(([method]) => ['get', 'post', 'put', 'delete', 'patch'].includes(method))
      .map(([method, operation]) => ({
        group: endpointGroup(path),
        method: method.toUpperCase() as EndpointDoc['method'],
        path,
        title: operation.summary ?? operation.operationId ?? path,
        auth: (operation.security?.length ?? 0) > 0 ? 'Authorization required' : 'Public',
        requestExample: `curl ${path}`,
        responseExample: JSON.stringify(operation.responses ?? {}, null, 2).slice(0, 1200),
      }))
  ))
}

export const openApi = {
  getOpenApiSpec: () => sharedApiClient.request<any>(API_PATHS.docs.openapiJson, { auth: false }),
  listEndpointDocs: async () => docsFromOpenApi(await openApi.getOpenApiSpec()),
  getExamples: () => sharedApiClient.request<any>(API_PATHS.docs.examples, { auth: false }),
  getErrors: () => sharedApiClient.request<any>(API_PATHS.docs.errors, { auth: false }),
  createReferenceAssetUploadSession: (input: OpenReferenceAssetUploadSessionRequest, headers: OpenApiHeaders) =>
    sharedApiClient.request<OpenReferenceAssetUploadSessionResponse>(API_PATHS.open.uploadSessions, { method: 'POST', body: input, headers, auth: false }),
  getReferenceAsset: async (asset_id: string, headers: OpenApiHeaders) => toReferenceAsset(await sharedApiClient.request(API_PATHS.open.referenceAssetDetail, { pathParams: { asset_id }, headers, auth: false })),
  createTask: async (input: CreateTaskRequest, headers: OpenApiHeaders): Promise<ImageTask> => toTask(await sharedApiClient.request(API_PATHS.open.tasks, { method: 'POST', body: toOpenTaskBody(input), headers, auth: false })),
  getTask: async (task_id: string, headers: OpenApiHeaders): Promise<ImageTask> => toTask(await sharedApiClient.request(API_PATHS.open.taskDetail, { pathParams: { task_id }, headers, auth: false })),
  getBalance: (headers: OpenApiHeaders) => sharedApiClient.request<Balance>(API_PATHS.open.balance, { headers, auth: false }),
  getCapabilities: (headers: OpenApiHeaders) => sharedApiClient.request<Capability>(API_PATHS.open.capabilities, { headers, auth: false }),
  estimate: (input: EstimateRequest, headers: OpenApiHeaders) => sharedApiClient.request<EstimateResult>(API_PATHS.open.estimate, { query: toEstimateQuery(input), headers, auth: false }),
  listPublicGallery: async (page = 1, page_size = 20): Promise<PageResult<ImageResult>> => {
    const result = normalizePage<any>(await sharedApiClient.request(API_PATHS.open.galleryImages, { query: { page, page_size }, auth: false }))
    return { ...result, items: result.items.map(toImageResult) }
  },
  getPublicGalleryImage: async (image_id: string) => toImageResult(await sharedApiClient.request(API_PATHS.open.galleryImageDetail, { pathParams: { image_id }, auth: false })),
  createOpenAIImageGeneration: (input: OpenAIImageGenerationRequest, bearerToken?: string) =>
    sharedApiClient.request<OpenAIImageResponse>(API_PATHS.compat.generations, { method: 'POST', body: input, headers: bearerHeaders(bearerToken), auth: !bearerToken }),
  createOpenAIImageEdit: (input: OpenAIImageEditRequest, bearerToken?: string) =>
    sharedApiClient.request<OpenAIImageResponse>(API_PATHS.compat.edits, { method: 'POST', formData: toOpenAIEditForm(input), headers: bearerHeaders(bearerToken), auth: !bearerToken }),
  listOpenAIModels: (bearerToken?: string) =>
    sharedApiClient.request<OpenAIModelList>(API_PATHS.compat.models, { headers: bearerHeaders(bearerToken), auth: !bearerToken }),
}

export type OpenApiHeaders = {
  'X-Access-Key': string
  'X-Signature': string
  'X-Timestamp': string
  'X-Body-SHA256': string
}

function toEstimateQuery(req: EstimateRequest) {
  return {
    task_type: req.task_type,
    route_model_code: req.route_model_code,
    requested_quality: req.quality,
    requested_size: req.aspect_ratio,
    requested_output_image_count: req.image_count,
    reference_image_count: req.reference_asset_ids?.length ?? 0,
  }
}

function toOpenTaskBody(req: CreateTaskRequest) {
  return {
    ...toEstimateQuery(req),
    prompt: req.prompt,
    reference_asset_ids: req.reference_asset_ids ?? [],
    response_mode: 'async',
  }
}

function bearerHeaders(token?: string) {
  return token ? { Authorization: `Bearer ${token}` } : undefined
}

function toOpenAIEditForm(input: OpenAIImageEditRequest) {
  const formData = new FormData()
  formData.set('model', input.model)
  formData.set('prompt', input.prompt)
  for (const image of Array.isArray(input.image) ? input.image : [input.image]) formData.append('image', image)
  if (input.mask) formData.set('mask', input.mask)
  if (input.size) formData.set('size', input.size)
  if (input.n !== undefined) formData.set('n', String(input.n))
  if (input.quality) formData.set('quality', input.quality)
  if (input.response_format) formData.set('response_format', input.response_format)
  if (input.user) formData.set('user', input.user)
  return formData
}
