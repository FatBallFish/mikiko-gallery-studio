import type { CallRecord, EstimateRequest, GalleryImage } from './api-types'
import { mockApi } from './mock-api'
import type { OpenApiHeaders } from './open-api'
import * as openApiModule from './open-api'
import * as generationApi from './user-api'

type AnyFunction = (...args: any[]) => any

function requireFunction(name: string): AnyFunction {
  return requireExport(generationApi as Record<string, unknown>, name, 'user-api')
}

function requireExport(source: Record<string, unknown>, name: string, moduleName: string): AnyFunction {
  const candidate = source[name]
  if (typeof candidate !== 'function') {
    throw new Error(`${moduleName} must export the pure ${name} function`)
  }
  return candidate as AnyFunction
}

function assertDeepEqual(actual: unknown, expected: unknown, message: string) {
  if (JSON.stringify(canonicalize(actual)) !== JSON.stringify(canonicalize(expected))) {
    throw new Error(`${message}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`)
  }
}

function canonicalize(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalize)
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.entries(value).sort(([left], [right]) => left.localeCompare(right)).map(([key, item]) => [key, canonicalize(item)]))
  }
  return value
}

function assertAbsent(record: Record<string, unknown>, keys: string[], message: string) {
  const present = keys.filter((key) => Object.prototype.hasOwnProperty.call(record, key))
  if (present.length) {
    throw new Error(`${message}: unexpected ${present.join(', ')}`)
  }
}

const normalizeCapabilities = requireFunction('normalizeCapabilities')
const buildEstimateWireRequest = requireFunction('buildEstimateWireRequest')
const buildCreateTaskWireRequest = requireFunction('buildCreateTaskWireRequest')
const normalizeTaskList = requireFunction('normalizeTaskList')
const toEstimate = requireFunction('toEstimate')
const toGalleryImage = requireFunction('toGalleryImage')

const capability = normalizeCapabilities({
  ModelGroups: [{
    Code: 'plus-image',
    Name: 'Plus Image',
    Description: 'Current Go capability shape',
    TaskTypes: ['text_to_image', 'image_edit'],
    Qualities: ['auto', '1k', '2k'],
    AutoBaseResolutionByTaskType: { text_to_image: '2k', image_edit: '1k' },
    AspectRatios: ['1:1', '16:9'],
    SupportsCustomSize: true,
    CapabilitiesByTaskType: {
      text_to_image: {
        BaseResolution: ['auto', '2k'], AutoBaseResolution: '2k', SizeModes: ['ratio'], AspectRatios: ['1:1'],
        Quality: ['high'], OutputFormat: ['jpeg'], SupportsOutputCompression: true, SupportsCustomSize: false,
        Moderation: ['auto'], MaxOutputImageCount: 2, MaxReferenceImageCount: 0,
      },
      image_edit: {
        BaseResolution: ['auto', '1k'], AutoBaseResolution: '1k', SizeModes: ['pixel'], PixelSizes: ['1024x1024'],
        Quality: ['low'], OutputFormat: ['webp'], SupportsOutputCompression: false, SupportsCustomSize: true,
        Moderation: ['low'], MaxOutputImageCount: 1, MaxReferenceImageCount: 3,
      },
    },
    MaxOutputImageCount: 4,
    MaxReferenceImageCount: 2,
    Prices: [{
      TaskType: 'text_to_image',
      Quality: '2k',
      BasePoints: '4.00000',
      ChargedPoints: '3.20000',
      DisplayPoints: '3.20',
    }],
  }],
  ReferenceImageMaxMB: 12,
  ReferenceImageMaxBytes: 12 * 1024 * 1024,
})

const model = capability.model_groups[0]
if (!model) throw new Error('Go-style capability should expose its model group')
assertDeepEqual(model.base_resolution, ['1k', '2k'], 'base-resolution options must omit legacy auto')
assertDeepEqual(model.auto_base_resolution_by_task_type, { text_to_image: '2k', image_edit: '1k' }, 'resolved auto buckets should survive capability normalization')
assertDeepEqual(model.qualities, ['auto', '1k', '2k'], 'legacy quality aliases should remain available')
assertDeepEqual(model.aspect_ratios, ['1:1', '16:9'], 'Go aspect ratios should survive normalization')
assertDeepEqual(model.size_modes, ['ratio'], 'the current Go API should expose ratio mode only')
assertDeepEqual(model.pixel_sizes, [], 'the current Go API should not invent pixel-size choices')
assertDeepEqual(model.prices[0]?.base_resolution, '2k', 'price quality should become a base-resolution alias')
assertDeepEqual(model.prices[0]?.quality, '2k', 'legacy price quality should remain available')
assertDeepEqual(model.supports_output_compression, false, 'missing compression support should default to false')
assertDeepEqual(model.supports_custom_size, true, 'Go custom-size support should survive normalization')
assertDeepEqual(model.capabilities_by_task_type, {
  text_to_image: {
    base_resolution: ['2k'], auto_base_resolution: '2k', size_modes: ['ratio'], aspect_ratios: ['1:1'],
    quality: ['high'], output_format: ['jpeg'], supports_output_compression: true, supports_custom_size: false, supports_custom_ratio: false,
    moderation: ['auto'], max_output_image_count: 2, max_reference_image_count: 0,
  },
  image_edit: {
    base_resolution: ['1k'], auto_base_resolution: '1k', size_modes: ['pixel'], pixel_sizes: ['1024x1024'],
    quality: ['low'], output_format: ['webp'], supports_output_compression: false, supports_custom_size: true, supports_custom_ratio: false,
    moderation: ['low'], max_output_image_count: 1, max_reference_image_count: 3,
  },
}, 'complete task-scoped capabilities should survive normalization')
assertAbsent(model, ['quality', 'output_format', 'moderation'], 'normalization must not advertise unsupported option sets')

const explicitEmptyCapability = normalizeCapabilities({ ModelGroups: [{
  Code: 'explicit-empty', TaskTypes: ['text_to_image'], CapabilitiesByTaskType: { text_to_image: {
    BaseResolution: [], SizeModes: [], AspectRatios: [], PixelSizes: [], Quality: [], OutputFormat: [], SupportedBackgrounds: [], Moderation: [],
  } },
}] })
const explicitEmptyTask = explicitEmptyCapability.model_groups[0]?.capabilities_by_task_type?.text_to_image
assertDeepEqual({
  base_resolution: explicitEmptyTask?.base_resolution,
  size_modes: explicitEmptyTask?.size_modes,
  aspect_ratios: explicitEmptyTask?.aspect_ratios,
  pixel_sizes: explicitEmptyTask?.pixel_sizes,
  quality: explicitEmptyTask?.quality,
  output_format: explicitEmptyTask?.output_format,
  supported_backgrounds: explicitEmptyTask?.supported_backgrounds,
  moderation: explicitEmptyTask?.moderation,
}, {
  base_resolution: [], size_modes: [], aspect_ratios: [], pixel_sizes: [], quality: [], output_format: [], supported_backgrounds: [], moderation: [],
}, 'explicit empty task-scoped option sets must remain empty')

const legacyCapability = normalizeCapabilities({ ModelGroups: [{ Code: 'legacy', TaskTypes: ['text_to_image'] }] })
assertDeepEqual(legacyCapability.model_groups[0]?.supports_custom_size, false, 'missing custom-size support should default to false')

const generationContractCapability = normalizeCapabilities({ ModelGroups: [{
  Code: 'generation-contract', TaskTypes: ['text_to_image'], BaseResolution: ['1k'], SizeModes: ['auto', 'ratio', 'pixel'],
  SupportsCustomRatio: true, SupportedBackgrounds: ['auto', 'transparent'], MinWidth: 512, MaxWidth: 2048, MinHeight: 512, MaxHeight: 1536,
}] })
const generationContractModel = generationContractCapability.model_groups[0]
assertDeepEqual(generationContractModel?.size_modes, ['auto', 'ratio', 'pixel'], 'auto size mode must survive capability normalization')
assertDeepEqual(generationContractModel?.supported_backgrounds, ['auto', 'transparent'], 'background capability must survive normalization')
if (!generationContractModel?.supports_custom_ratio || generationContractModel.min_width !== 512 || generationContractModel.max_height !== 1536) {
  throw new Error(`custom ratio and pixel bounds must survive normalization: ${JSON.stringify(generationContractModel)}`)
}

const autoWire = buildEstimateWireRequest({
  task_type: 'text_to_image', route_model_code: 'generation-contract', size_mode: 'auto',
  quality: 'auto', output_format: 'png', background: 'auto', moderation: 'auto', image_count: 1,
})
assertDeepEqual(autoWire, {
  task_type: 'text_to_image', route_model_code: 'generation-contract', size_mode: 'auto', quality: 'auto',
  output_format: 'png', background: 'auto', output_compression: 100, moderation: 'auto', requested_output_image_count: 1, reference_image_count: 0,
}, 'auto mode must omit every size field from the wire request')
assertAbsent(autoWire, ['base_resolution', 'aspect_ratio', 'requested_size'], 'auto mode must not serialize stale size fields')

const ratioRequest = {
  task_type: 'image_edit',
  route_model_code: 'plus-image',
  size_mode: 'ratio',
  base_resolution: '2K',
  quality: 'high',
  aspect_ratio: '16:9',
  pixel_size: '999x999',
  output_format: 'webp',
  background: 'transparent',
  output_compression: 42,
  moderation: 'low',
  image_count: 2,
  reference_asset_ids: ['ref-1'],
}

const baseResolutionOnlyRequest: EstimateRequest = {
  task_type: 'text_to_image',
  route_model_code: 'plus-image',
  base_resolution: '2K',
  aspect_ratio: '16:9',
  image_count: 1,
}
void baseResolutionOnlyRequest

const estimateWire = buildEstimateWireRequest(ratioRequest)
assertDeepEqual(estimateWire, {
  task_type: 'image_edit',
  route_model_code: 'plus-image',
  size_mode: 'ratio',
  aspect_ratio: '16:9',
  base_resolution: '2K',
  quality: 'high',
  output_format: 'webp',
  background: 'transparent',
  output_compression: 42,
  moderation: 'low',
  requested_output_image_count: 2,
  reference_image_count: 1,
}, 'estimate conversion should emit the complete current Go wire contract')
assertAbsent(estimateWire, [
  'pixel_size',
], 'estimate conversion must not leak UI-only fields')

const createWire = buildCreateTaskWireRequest({
  ...ratioRequest,
  project_id: 'project-a',
  prompt: 'Paint a quiet harbor',
  negative_prompt: 'text, watermark',
  capability_version: 'capability-v1',
  response_mode: 'sync',
  idempotency_key: 'generation-idem-1',
})
assertDeepEqual(createWire, {
  body: {
    project_id: 'project-a',
    task_type: 'image_edit',
    prompt: 'Paint a quiet harbor\n\nNegative prompt: text, watermark',
    route_model_code: 'plus-image',
    size_mode: 'ratio',
    aspect_ratio: '16:9',
    base_resolution: '2K',
    quality: 'high',
    output_format: 'webp',
    background: 'transparent',
    output_compression: 42,
    moderation: 'low',
    requested_output_image_count: 2,
    reference_asset_ids: ['ref-1'],
    response_mode: 'async',
    capability_version: 'capability-v1',
  },
  headers: { 'Idempotency-Key': 'generation-idem-1' },
}, 'create conversion should preserve negative prompts and emit the native async Go contract')
assertAbsent(createWire.body, [
  'idempotency_key',
  'negative_prompt',
  'pixel_size',
], 'create conversion must keep header-only and UI-only fields out of the body')

const taskFromResolved = generationApi.toTask({ id: 'task-resolved', resolved_quality_bucket: '4k' })
const taskFromRequested = generationApi.toTask({ id: 'task-requested', requested_quality: '2k' })
const wideTask = generationApi.toTask({ id: 'task-wide', requested_size: '2560x1440', aspect_ratio: '1:1' })
const portraitTask = generationApi.toTask({ id: 'task-portrait', requested_size: '1440x2560', aspect_ratio: '1:1' })
const landscapeTask = generationApi.toTask({ id: 'task-landscape', requested_size: '2048x1536', aspect_ratio: '1:1' })
const fallbackTask = generationApi.toTask({ id: 'task-fallback', requested_size: 'not-a-size', aspect_ratio: '3:2' })
const imageFromQuality = generationApi.toImageResult({ id: 'image-quality', quality: '1k' })
const galleryFromRequested = toGalleryImage({ id: 'gallery-requested', task_id: 'task-1', requested_quality: '2k' })
const estimateFromResolved = toEstimate({ resolved_quality_bucket: '4k', estimated_points: '1.00000' })
const profileFromLegacy = generationApi.toUserProfile({ email: 'legacy@example.com', preferences: { quality: '2k' } })
const profileFromNew = generationApi.toUserProfile({ email: 'new@example.com', preferences: { base_resolution: '4k' } })

assertDeepEqual(taskFromResolved.base_resolution, '4k', 'task resolved bucket should become base_resolution')
assertDeepEqual(taskFromRequested.base_resolution, '2k', 'task requested quality should become base_resolution')
assertDeepEqual(wideTask.aspect_ratio, '16:9', 'explicit requested size must override a default 1:1 aspect ratio')
assertDeepEqual(portraitTask.aspect_ratio, '9:16', 'portrait requested size should normalize to 9:16')
assertDeepEqual(landscapeTask.aspect_ratio, '4:3', 'landscape requested size should normalize to 4:3')
assertDeepEqual(fallbackTask.aspect_ratio, '3:2', 'unparseable requested size should fall back to the trusted aspect ratio')
const inferTaskAspectRatio = requireFunction('inferTaskAspectRatio')
assertDeepEqual(inferTaskAspectRatio({ requested_size: 'bad', aspect_ratio: '16:9' }), '16:9', 'pure aspect inference should preserve a valid fallback')
assertDeepEqual(inferTaskAspectRatio({ requested_size: 'bad', aspect_ratio: 'bad' }), '1:1', 'pure aspect inference should default when neither source is trustworthy')
assertDeepEqual(imageFromQuality.base_resolution, '1k', 'image quality should become base_resolution')
assertDeepEqual(galleryFromRequested.base_resolution, '2k', 'gallery requested quality should become base_resolution')
assertDeepEqual(estimateFromResolved.base_resolution, '4k', 'estimate resolved bucket should become base_resolution')
assertDeepEqual(profileFromLegacy.preferences.base_resolution, '2k', 'legacy profile quality should become base_resolution')
assertDeepEqual(profileFromNew.preferences.quality, '4k', 'new profile base_resolution should retain a legacy quality alias')

const directTasks = normalizeTaskList([
  { id: 'direct-1', prompt: 'one', requested_quality: '1k' },
  { id: 'direct-2', prompt: 'two', quality: '2k' },
])
const pagedTasks = normalizeTaskList({
  items: [{ id: 'paged-1', prompt: 'three', resolved_quality_bucket: '4k' }],
  pagination: { page: 1, page_size: 20, total: 1 },
})
assertDeepEqual(directTasks.map((task: { id: string }) => task.id), ['direct-1', 'direct-2'], 'direct Go task arrays should normalize')
assertDeepEqual(pagedTasks.map((task: { id: string }) => task.id), ['paged-1'], 'paged task responses should normalize')

type LossProtectionFields = Pick<CallRecord, 'platform_loss' | 'artifact_recovery'>
type StorageBindingField = Pick<GalleryImage, 'storage_config_id'>
const typeOnlyRegressionGuard: [LossProtectionFields?, StorageBindingField?] = []
void typeOnlyRegressionGuard

const refreshSessionSource = String(generationApi.userApi.refreshSession)
if (!/retryUnauthorized:\s*false/.test(refreshSessionSource)) {
  throw new Error('user refreshSession must remain non-recursive with retryUnauthorized: false')
}

void verifyBaseResolutionOnlyConsumers().catch((error) => {
  queueMicrotask(() => { throw error })
})

async function verifyBaseResolutionOnlyConsumers() {
  const buildOpenEstimateWire = requireExport(openApiModule as Record<string, unknown>, 'buildOpenEstimateWire', 'open-api')
  const buildOpenCreateTaskWire = requireExport(openApiModule as Record<string, unknown>, 'buildOpenCreateTaskWire', 'open-api')
  const openApi = openApiModule.openApi
  const referenceAssetIDs = ['ref-open-1']
  const request: EstimateRequest = {
    task_type: 'image_edit',
    route_model_code: 'plus-image',
    base_resolution: '2K',
    aspect_ratio: '16:9',
    image_count: 2,
    reference_asset_ids: referenceAssetIDs,
  }
  const estimateDescriptor = buildOpenEstimateWire(request) as OpenWireDescriptor
  const createDescriptor = buildOpenCreateTaskWire({ ...request, prompt: 'Paint a quiet harbor' }) as OpenWireDescriptor
  assertDeepEqual(estimateDescriptor, {
    method: 'GET',
    request_uri: '/api/open/image/v1/estimate?task_type=image_edit&route_model_code=plus-image&requested_quality=2k&requested_size=2560x1440&requested_output_image_count=2&reference_image_count=1',
    serialized_body: '',
  }, 'Open API estimate builder should freeze the exact signable wire request')
  assertDeepEqual(createDescriptor, {
    method: 'POST',
    request_uri: '/api/open/image/v1/tasks',
    serialized_body: '{"task_type":"image_edit","route_model_code":"plus-image","requested_quality":"2k","requested_size":"2560x1440","requested_output_image_count":2,"reference_image_count":1,"prompt":"Paint a quiet harbor","reference_asset_ids":["ref-open-1"],"response_mode":"async"}',
  }, 'Open API create builder should freeze the exact signable wire request')
  assertAbsent(estimateDescriptor as unknown as Record<string, unknown>, ['query', 'body'], 'Open estimate descriptor must not expose mutable derived objects')
  assertAbsent(createDescriptor as unknown as Record<string, unknown>, ['query', 'body'], 'Open create descriptor must not expose mutable derived objects')

  referenceAssetIDs.push('ref-after-signing')
  request.aspect_ratio = '1:1'
  request.base_resolution = '4K'
  if (estimateDescriptor.request_uri !== '/api/open/image/v1/estimate?task_type=image_edit&route_model_code=plus-image&requested_quality=2k&requested_size=2560x1440&requested_output_image_count=2&reference_image_count=1'
    || createDescriptor.serialized_body !== '{"task_type":"image_edit","route_model_code":"plus-image","requested_quality":"2k","requested_size":"2560x1440","requested_output_image_count":2,"reference_image_count":1,"prompt":"Paint a quiet harbor","reference_asset_ids":["ref-open-1"],"response_mode":"async"}') {
    throw new Error('Open descriptors must remain stable after the original request is mutated')
  }

  const timestamp = '2026-07-17T00:00:00Z'
  const secret = 'contract-signing-secret'
  const goCreateBodyHash = 'F-y2oZ4DbiAMUoV5nPDNiDF-sPoMo4wRRW-4e6LPvK0'
  const goCreateSignature = 'KFATqBMyGGl_aUHTGx052h5_mK04hszt0csDxqen2sA'
  const estimateHeaders = await signedHeaders(estimateDescriptor, secret, timestamp)
  const createHeaders = await signedHeaders(createDescriptor, secret, timestamp)
  if (createHeaders['X-Body-SHA256'] !== goCreateBodyHash || createHeaders['X-Signature'] !== goCreateSignature) {
    throw new Error(`TypeScript signing must match the Go canonical vector, got hash=${createHeaders['X-Body-SHA256']} signature=${createHeaders['X-Signature']}`)
  }

  const captured: Array<{ input: RequestInfo | URL; init?: RequestInit }> = []
  const originalFetch = globalThis.fetch
  try {
    globalThis.fetch = async (input, init) => {
      captured.push({ input, init })
      const payload = String(input).includes('/estimate')
        ? { points: '14.86250', formula: 'contract', sufficient: true }
        : { id: 'open-task-1', prompt: 'Paint a quiet harbor', task_type: 'image_edit', status: 'queued' }
      return new Response(JSON.stringify({ data: payload, meta: { request_id: 'generation-contract' } }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }

    await (openApi.estimate as AnyFunction)(estimateDescriptor, estimateHeaders)
    await (openApi.createTask as AnyFunction)(createDescriptor, createHeaders)
  } finally {
    globalThis.fetch = originalFetch
  }

  if (captured.length !== 2) throw new Error(`Open API should issue estimate and create requests, got ${captured.length}`)
  for (const [index, call] of captured.entries()) {
    const descriptor = [estimateDescriptor, createDescriptor][index]
    const expectedHeaders = [estimateHeaders, createHeaders][index]
    const actualURL = new URL(String(call.input), 'http://contract.local')
    const actualRequestURI = `${actualURL.pathname}${actualURL.search}`
    const actualMethod = String(call.init?.method ?? 'GET').toUpperCase()
    const actualBody = typeof call.init?.body === 'string' ? call.init.body : ''
    const actualHeaders = new Headers(call.init?.headers)
    if (actualMethod !== descriptor.method || actualRequestURI !== descriptor.request_uri || actualBody !== descriptor.serialized_body) {
      throw new Error(`fetch must reuse the signed descriptor exactly, descriptor=${JSON.stringify(descriptor)} actual=${JSON.stringify({ method: actualMethod, request_uri: actualRequestURI, serialized_body: actualBody })}`)
    }
    const actualBodyHash = await sha256Base64Url(actualBody)
    if (actualHeaders.get('X-Body-SHA256') !== actualBodyHash) {
      throw new Error(`fetch body hash must cover the actual serialized body, expected ${actualBodyHash}, got ${actualHeaders.get('X-Body-SHA256')}`)
    }
    const canonical = `${actualMethod}\n${actualRequestURI}\n${timestamp}\n${actualBodyHash}`
    const actualSignature = await hmacSha256Base64Url(secret, canonical)
    if (actualHeaders.get('X-Signature') !== actualSignature) {
      throw new Error(`fetch signature must cover the backend canonical payload, expected ${actualSignature}, got ${actualHeaders.get('X-Signature')}`)
    }
    if (descriptor.method === 'POST' && (actualBodyHash !== goCreateBodyHash || actualSignature !== goCreateSignature)) {
      throw new Error(`actual fetch request must match the Go canonical vector, got hash=${actualBodyHash} signature=${actualSignature}`)
    }
    for (const [name, value] of Object.entries(expectedHeaders)) {
      if (actualHeaders.get(name) !== value) throw new Error(`Open API must preserve signed header ${name}`)
    }
    if (actualHeaders.has('Authorization') || actualHeaders.has('Idempotency-Key')) {
      throw new Error(`Open API must not inherit Agent API auth headers, got ${JSON.stringify(Object.fromEntries(actualHeaders))}`)
    }
  }

  const resolveGenerationResolution = (generationApi as Record<string, unknown>).resolveGenerationResolution
  if (typeof resolveGenerationResolution !== 'function') {
    throw new Error('user-api must export the shared pure resolveGenerationResolution function')
  }
  assertDeepEqual(resolveGenerationResolution({ base_resolution: '2K', quality: 'auto', aspect_ratio: '16:9' }), {
    requested_quality: '2k',
    requested_size: '2560x1440',
  }, 'shared resolution mapping should prefer base_resolution over legacy quality')

  mockApi.reset()
  const mockRequest: EstimateRequest = {
    task_type: 'image_edit',
    route_model_code: 'plus-image',
    base_resolution: '2K',
    aspect_ratio: '16:9',
    image_count: 2,
    reference_asset_ids: ['ref-open-1'],
  }
  const baseResolutionEstimate = await mockApi.estimate(mockRequest)
  if (baseResolutionEstimate.base_resolution !== '2k' || baseResolutionEstimate.resolved_quality !== '2k') {
    throw new Error(`mock estimate should expose the selected resolution aliases, got ${JSON.stringify(baseResolutionEstimate)}`)
  }

  const mockTask = await mockApi.createTask({ ...mockRequest, prompt: 'Paint a quiet harbor' })
  if (mockTask.base_resolution !== '2k' || mockTask.quality !== '2k') {
    throw new Error(`mock create should retain the same selected resolution, got ${JSON.stringify(mockTask)}`)
  }
}

type OpenWireDescriptor = {
  method: 'GET' | 'POST'
  request_uri: string
  serialized_body: string
}

async function signedHeaders(descriptor: OpenWireDescriptor, secret: string, timestamp: string): Promise<OpenApiHeaders> {
  const bodyHash = await sha256Base64Url(descriptor.serialized_body)
  const canonical = `${descriptor.method}\n${descriptor.request_uri}\n${timestamp}\n${bodyHash}`
  return {
    'X-Access-Key': 'contract-access-key',
    'X-Signature': await hmacSha256Base64Url(secret, canonical),
    'X-Timestamp': timestamp,
    'X-Body-SHA256': bodyHash,
  }
}

async function sha256Base64Url(value: string) {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(value))
  return bytesToBase64Url(new Uint8Array(digest))
}

async function hmacSha256Base64Url(secret: string, value: string) {
  const key = await crypto.subtle.importKey('raw', new TextEncoder().encode(secret), { name: 'HMAC', hash: 'SHA-256' }, false, ['sign'])
  const digest = await crypto.subtle.sign('HMAC', key, new TextEncoder().encode(value))
  return bytesToBase64Url(new Uint8Array(digest))
}

function bytesToBase64Url(bytes: Uint8Array) {
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}
