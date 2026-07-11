import type { Balance, Capability, CreateTaskRequest, EstimateRequest, EstimateResult, ImageResult, ImageTask, OpenAIImageEditRequest, OpenAIImageGenerationRequest, OpenAIImageResponse, OpenAIModelList, OpenReferenceAssetUploadSessionRequest, OpenReferenceAssetUploadSessionResponse, PageResult, ReferenceAsset } from './api-types'
import { API_PATHS } from './api-types'
import { fillPath, normalizePage, sharedApiClient } from './http-client'
import { docsFromOpenApi, normalizeErrors, normalizeExamples } from './open-api-docs'
import { toImageResult, toReferenceAsset, toTask } from './user-api'

export type { DocsErrorCode, DocsExample } from './open-api-docs'

export const openApi = {
  getOpenApiSpec: () => sharedApiClient.request<any>(API_PATHS.docs.openapiJson, { auth: false }),
  listEndpointDocs: async () => docsFromOpenApi(await openApi.getOpenApiSpec()),
  getExamples: async () => normalizeExamples(await sharedApiClient.request<unknown>(API_PATHS.docs.examples, { auth: false })),
  getErrors: async () => normalizeErrors(await sharedApiClient.request<unknown>(API_PATHS.docs.errors, { auth: false })),
  createReferenceAssetUploadSession: (input: OpenReferenceAssetUploadSessionRequest, headers: OpenApiHeaders) =>
    sharedApiClient.request<OpenReferenceAssetUploadSessionResponse>(API_PATHS.open.uploadSessions, { method: 'POST', body: input, headers, auth: false }),
  getReferenceAsset: async (asset_id: string, headers: OpenApiHeaders) => toReferenceAsset(await sharedApiClient.request(API_PATHS.open.referenceAssetDetail, { pathParams: { asset_id }, headers, auth: false })),
  createTask: async (input: CreateTaskRequest, headers: OpenApiHeaders): Promise<ImageTask> => toTask(await sharedApiClient.request(API_PATHS.open.tasks, { method: 'POST', body: toOpenTaskBody(input), headers, auth: false })),
  getTask: async (task_id: string, headers: OpenApiHeaders): Promise<ImageTask> => toTask(await sharedApiClient.request(API_PATHS.open.taskDetail, { pathParams: { task_id }, headers, auth: false })),
  getBalance: (headers: OpenApiHeaders) => sharedApiClient.request<Balance>(API_PATHS.open.balance, { headers, auth: false }),
  getCapabilities: (headers: OpenApiHeaders) => sharedApiClient.request<Capability>(API_PATHS.open.capabilities, { headers, auth: false }),
  estimate: (input: EstimateRequest, headers: OpenApiHeaders) => sharedApiClient.request<EstimateResult>(API_PATHS.open.estimate, { query: toEstimateQuery(input), headers, auth: false }),
  listPublicGallery: async (page = 1, page_size = 20, options?: { sort?: 'latest' | 'hot'; query?: string; liked?: boolean; favorited?: boolean; accessToken?: string | null }): Promise<PageResult<ImageResult>> => {
    const result = normalizePage<any>(await sharedApiClient.request(API_PATHS.open.galleryImages, {
      query: { page, page_size, sort: options?.sort, query: options?.query, liked: options?.liked, favorited: options?.favorited },
      auth: false,
      headers: options?.accessToken ? { Authorization: `Bearer ${options.accessToken}` } : undefined,
    }))
    return { ...result, items: result.items.map(toPublicGalleryImage) }
  },
  getPublicGalleryImage: async (image_id: string, options?: { accessToken?: string | null }) => toPublicGalleryImage(await sharedApiClient.request(API_PATHS.open.galleryImageDetail, {
    pathParams: { image_id },
    auth: false,
    headers: options?.accessToken ? { Authorization: `Bearer ${options.accessToken}` } : undefined,
  })),
  createOpenAIImageGeneration: (input: OpenAIImageGenerationRequest, bearerToken?: string) =>
    sharedApiClient.request<OpenAIImageResponse>(API_PATHS.compat.generations, { method: 'POST', body: input, headers: bearerHeaders(bearerToken), auth: !bearerToken }),
  createOpenAIImageEdit: (input: OpenAIImageEditRequest, bearerToken?: string) =>
    sharedApiClient.request<OpenAIImageResponse>(API_PATHS.compat.edits, { method: 'POST', formData: toOpenAIEditForm(input), headers: bearerHeaders(bearerToken), auth: !bearerToken }),
  listOpenAIModels: (bearerToken?: string) =>
    sharedApiClient.request<OpenAIModelList>(API_PATHS.compat.models, { headers: bearerHeaders(bearerToken), auth: !bearerToken }),
}

function toPublicGalleryImage(raw: any) {
  const image = toImageResult(raw)
  const publicURL = fillPath(API_PATHS.open.galleryImageDownload, { image_id: image.id })
  return { ...image, url: publicURL, download_url: publicURL }
}

export type OpenApiHeaders = {
  'X-Access-Key': string
  'X-Signature': string
  'X-Timestamp': string
  'X-Body-SHA256': string
}

function toEstimateQuery(req: EstimateRequest) {
  const sizeMode = req.size_mode === 'pixel' ? 'pixel' : 'ratio'
  return {
    task_type: req.task_type,
    route_model_code: req.route_model_code,
    size_mode: sizeMode,
    aspect_ratio: sizeMode === 'ratio' ? req.aspect_ratio : undefined,
    base_resolution: sizeMode === 'ratio' ? req.base_resolution : 'auto',
    quality: req.quality ?? 'auto',
    output_format: req.output_format ?? 'png',
    output_compression: req.output_compression ?? 100,
    moderation: req.moderation ?? 'auto',
    requested_size: sizeMode === 'pixel' ? req.pixel_size : 'auto',
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
