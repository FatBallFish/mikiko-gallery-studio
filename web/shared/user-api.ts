import type {
  ApiKey,
  Balance,
  BackendCreateTaskRequest,
  BackendEstimateRequest,
  BillingPlan,
  CashierOptions,
  CashierOrder,
  CashierOrderSyncResponse,
  CanvasDocument,
  CanvasRun,
  Capability,
  CapabilityTaskOptions,
  CreateApiKeyRequest,
  CreateCashierOrderRequest,
  CreateTaskRequest,
  CreativeCanvas,
  EstimateRequest,
  EstimateResult,
  FeatureFlags,
  GalleryImage,
	GalleryBatchMutationResult,
	GalleryExportStatus,
  ImageResult,
  ImageTask,
  ImageTaskType,
  LedgerEntry,
  LoginResult,
  MediaAccessProjection,
  MediaAccessPurpose,
  MediaAsset,
  MediaAssetFilters,
  MediaAssetPage,
  MediaBatchAction,
  MediaBatchResult,
  MediaCompletedPart,
  MediaExportStatus,
  MediaPartTarget,
  MediaUploadInit,
  MediaUploadSession,
  NormalLoginResponse,
  PageResult,
	Project,
  PromptOptimizationEstimate,
  PromptOptimizationResult,
  ReferenceAsset,
  Subscription,
  UpdatePreferencesRequest,
  UserProfile,
  VideoCapability,
  VideoCapabilityListWire,
  VideoCreateTaskRequest,
  VideoEstimate,
  VideoEstimateRequest,
  VideoTask,
} from './api-types'
import { API_PATHS } from './api-types'
import { fillPath, getDefaultBaseUrl, normalizePage, sharedApiClient, withQuery } from './http-client'
import { mediaAssetURL } from './media-url'
import { normalizeCreativeCanvas } from './canvas-document'

export { resolveGenerationResolution } from './generation-resolution'

function initials(input: string) {
  return input.trim().slice(0, 2).toUpperCase() || 'PG'
}

export function toUserProfile(raw: any): UserProfile {
  const name = raw.nickname || raw.display_name || raw.email?.split('@')[0] || 'Mikiko User'
  const resolution = raw.preferences?.base_resolution ?? raw.preferences?.quality ?? 'auto'
  return {
    ...raw,
    id: String(raw.id ?? raw.user_id ?? ''),
    email: raw.email ?? '',
    has_password: Boolean(raw.has_password),
    display_name: name,
    avatar_initials: initials(name),
    tier: raw.tier ?? 'FREE',
    group: raw.user_group_code ?? raw.group ?? 'DEFAULT',
    signature: raw.bio ?? raw.signature ?? '',
    preferences: {
      model_group: raw.preferences?.model_group ?? 'plus-image',
      base_resolution: resolution,
      quality: raw.preferences?.quality ?? resolution,
      aspect_ratio: raw.preferences?.aspect_ratio ?? '16:9',
      image_count: raw.preferences?.image_count ?? 1,
      theme_mode: raw.preferences?.theme_mode,
      accent_theme: raw.preferences?.accent_theme,
      default_locale: raw.preferences?.default_locale ?? raw.default_locale,
    },
  }
}

export function toBalance(raw: any): Balance {
  return {
    ...raw,
    available_points: raw.available_points ?? '0.00000',
    frozen_points: raw.frozen_points ?? '0.00000',
    plan_name: raw.active_subscription?.plan_name ?? raw.plan_name ?? 'FREE',
    first_purchase_bonus: Boolean(raw.first_purchase_bonus ?? !raw.active_subscription),
  }
}

function normalizeTaskType(type: string): ImageTask['task_type'] {
  if (type === 'image_edit') return 'image_edit'
  return 'text_to_image'
}

function toBackendTaskType(type: ImageTask['task_type'] | string): BackendEstimateRequest['task_type'] {
  if (type === 'image_edit') return 'image_edit'
  return 'text_to_image'
}

function pick<T = unknown>(source: any, ...keys: string[]): T | undefined {
  for (const key of keys) {
    const value = source?.[key]
    if (value !== undefined && value !== null) return value as T
  }
  return undefined
}

function responseResolution(raw: any, fallback = 'auto') {
  return String(raw?.base_resolution ?? raw?.resolved_quality_bucket ?? raw?.resolved_quality ?? raw?.requested_quality ?? raw?.quality ?? fallback)
}

function reducedAspectRatio(width: number, height: number) {
  if (!Number.isSafeInteger(width) || !Number.isSafeInteger(height) || width <= 0 || height <= 0) return null
  let left = width
  let right = height
  while (right !== 0) {
    const remainder = left % right
    left = right
    right = remainder
  }
  return `${width / left}:${height / left}`
}

export function inferTaskAspectRatio(raw: any) {
  const requestedSize = String(raw?.requested_size ?? '').trim()
  const sizeMatch = requestedSize.match(/^(\d+)\s*[xX]\s*(\d+)$/)
  if (sizeMatch) {
    const inferred = reducedAspectRatio(Number(sizeMatch[1]), Number(sizeMatch[2]))
    if (inferred) return inferred
  }

  const aspectRatio = String(raw?.aspect_ratio ?? '').trim()
  const ratioMatch = aspectRatio.match(/^(\d+)\s*:\s*(\d+)$/)
  if (ratioMatch) {
    const fallback = reducedAspectRatio(Number(ratioMatch[1]), Number(ratioMatch[2]))
    if (fallback) return fallback
  }
  return '1:1'
}

export function toReferenceAsset(raw: any): ReferenceAsset {
  return {
    ...raw,
    id: String(raw.id ?? raw.asset_id ?? ''),
    name: raw.name ?? raw.filename ?? raw.id ?? 'reference',
    preview_url: raw.preview_url ?? raw.download_url ?? '',
    download_url: raw.download_url ?? raw.preview_url ?? '',
    status: raw.status ?? 'ready',
    size_bytes: Number(raw.size_bytes ?? raw.file_size ?? 0),
    created_at: raw.created_at ?? '',
  }
}

export function toImageResult(raw: any): ImageResult {
  return {
    ...raw,
    id: String(raw.id ?? raw.image_id ?? ''),
    url: raw.url ?? raw.download_url ?? '',
    width: Number(raw.width ?? 0),
    height: Number(raw.height ?? 0),
    base_resolution: responseResolution(raw),
    publish_status: raw.publish_status ?? raw.visibility_status ?? 'private',
    like_count: Number(raw.like_count ?? 0),
    favorite_count: Number(raw.favorite_count ?? 0),
    liked_by_viewer: Boolean(raw.liked_by_viewer),
    favorited_by_viewer: Boolean(raw.favorited_by_viewer),
  }
}

export function toTask(raw: any): ImageTask {
  const results = (raw.results ?? raw.images ?? raw.image_results ?? []).map(toImageResult)
  const taskType = normalizeTaskType(raw.task_type ?? 'text_to_image')
  const quality = raw.quality ?? raw.requested_quality ?? raw.resolved_quality_bucket ?? 'auto'
  const baseResolution = responseResolution(raw)
  return {
    ...raw,
    id: String(raw.id ?? ''),
    title: raw.title ?? String(raw.prompt ?? 'Untitled generation').slice(0, 54),
    prompt: raw.prompt ?? '',
    task_type: taskType,
    status: raw.status ?? 'queued',
    progress_stage: raw.progress_stage ?? raw.progressStage ?? '',
    progress_message: raw.progress_message ?? raw.progressMessage ?? '',
    route_model_code: raw.route_model_code ?? raw.model_group ?? raw.abstract_model ?? raw.group_code ?? 'basic',
    route_model_name: raw.route_model_name,
    model_group: raw.route_model_code ?? raw.model_group ?? raw.abstract_model ?? raw.group_code ?? 'basic',
    base_resolution: baseResolution,
    size_mode: raw.size_mode ?? 'ratio',
    quality,
    aspect_ratio: inferTaskAspectRatio(raw),
    image_count: Number(raw.image_count ?? raw.requested_output_image_count ?? results.length ?? 1),
    estimate_points: raw.estimate_points ?? raw.estimated_points ?? raw.actual_points ?? '0.00000',
    progress: raw.progress == null ? undefined : Number(raw.progress),
    provider: raw.provider ?? raw.provider_code ?? '',
    route: raw.route ?? raw.route_policy ?? '',
    created_at: raw.created_at ?? '',
    updated_at: raw.updated_at ?? raw.created_at ?? '',
    failure_reason: raw.failure_reason ?? raw.error_message,
    error_code: raw.error_code,
    error_message: raw.error_message,
    request_id: raw.request_id ?? raw.meta?.request_id,
    reference_assets: (raw.reference_assets ?? []).map(toReferenceAsset),
    results,
  }
}

export function buildEstimateWireRequest(req: EstimateRequest): BackendEstimateRequest {
	const sizeMode = req.size_mode === 'auto' ? 'auto' : req.size_mode === 'pixel' ? 'pixel' : 'ratio'
	return {
		task_type: toBackendTaskType(req.task_type),
		route_model_code: req.route_model_code,
		size_mode: sizeMode,
		...(sizeMode === 'ratio' ? { aspect_ratio: req.aspect_ratio, base_resolution: req.base_resolution } : {}),
		...(sizeMode === 'pixel' ? { requested_size: req.pixel_size ?? '' } : {}),
		quality: req.quality ?? 'auto',
		output_format: req.output_format ?? 'png',
		...(req.background ? { background: req.background } : {}),
		output_compression: req.output_compression ?? 100,
		moderation: req.moderation ?? 'auto',
		requested_output_image_count: req.image_count,
		reference_image_count: req.reference_asset_ids?.length ?? 0,
  }
}

export function buildCreateTaskWireRequest(req: CreateTaskRequest): { body: BackendCreateTaskRequest; headers: Record<string, string> } {
  const { reference_image_count: _, ...estimateFields } = buildEstimateWireRequest(req)
  return {
    body: {
      ...estimateFields,
      ...(req.project_id ? { project_id: req.project_id } : {}),
      prompt: req.prompt,
      reference_asset_ids: req.reference_asset_ids ?? [],
      ...(req.reference_bindings?.length ? { reference_bindings: req.reference_bindings } : {}),
      ...(req.prompt_variables?.length ? { prompt_variables: req.prompt_variables } : {}),
      response_mode: 'async',
      ...(req.capability_version ? { capability_version: req.capability_version } : {}),
    },
    headers: req.idempotency_key ? { 'Idempotency-Key': req.idempotency_key } : {},
  }
}

export function buildGalleryReferenceImportRequest(galleryImageIds: string[]) {
  return { gallery_image_ids: galleryImageIds }
}

export function toEstimate(raw: any, req?: EstimateRequest): EstimateResult {
  const points = raw.display_points ?? raw.charged_points ?? raw.estimated_points ?? raw.points ?? '0.00000'
  const requestBaseResolution = req?.base_resolution ?? 'auto'
  const baseResolution = responseResolution(raw, requestBaseResolution)
  return {
    ...raw,
    points,
    charged_points: raw.charged_points ?? raw.estimated_points ?? raw.points,
    display_points: raw.display_points ?? points,
    formula: raw.formula ?? `${req?.route_model_code ?? raw.pricing_snapshot?.route_model_code ?? ''} x ${baseResolution}`,
    resolved_quality: raw.resolved_quality_bucket ?? raw.resolved_quality ?? baseResolution,
    base_resolution: baseResolution,
    sufficient: Boolean(raw.sufficient),
    insufficient_points: raw.insufficient_points ?? '0.00000',
    balance: raw.balance ? toBalance(raw.balance) : undefined,
  }
}

export function toGalleryImage(raw: any): GalleryImage {
  const taskType = raw.task_type ? normalizeTaskType(raw.task_type) : undefined
  return {
    ...raw,
    id: String(raw.id ?? raw.image_id ?? ''),
    task_id: String(raw.task_id ?? ''),
    task_type: taskType,
    reference_asset_ids: raw.reference_asset_ids ?? [],
    reference_assets: (raw.reference_assets ?? []).map(toReferenceAsset),
    url: raw.url ?? raw.download_url ?? '',
    download_url: raw.download_url,
    file_size_bytes: Number(raw.file_size_bytes ?? raw.size_bytes ?? 0),
    width: Number(raw.width ?? 0),
    height: Number(raw.height ?? 0),
    image_group: raw.image_group ?? raw.group ?? '',
    base_resolution: responseResolution(raw),
    visibility_status: raw.visibility_status ?? raw.publish_status ?? 'private',
    like_count: Number(raw.like_count ?? 0),
    favorite_count: Number(raw.favorite_count ?? 0),
    liked_by_viewer: Boolean(raw.liked_by_viewer),
    favorited_by_viewer: Boolean(raw.favorited_by_viewer),
    created_at: raw.created_at ?? '',
  }
}

function optionalOutputCapabilities(source: any) {
	const result: {
		quality?: string[]
		output_format?: string[]
		supports_output_compression: boolean
		supports_custom_size: boolean
		supports_custom_ratio: boolean
		supported_backgrounds?: string[]
		min_width?: number
		max_width?: number
		min_height?: number
		max_height?: number
		capabilities_by_task_type?: Partial<Record<ImageTaskType, CapabilityTaskOptions>>
		moderation?: string[]
	} = { supports_output_compression: false, supports_custom_size: false, supports_custom_ratio: false }
  const quality = pick<string[]>(source, 'quality', 'Quality')
  const outputFormat = pick<string[]>(source, 'output_format', 'OutputFormat')
  const supportsCompression = pick<boolean>(source, 'supports_output_compression', 'SupportsOutputCompression')
	const supportsCustomSize = pick<boolean>(source, 'supports_custom_size', 'SupportsCustomSize')
	const supportsCustomRatio = pick<boolean>(source, 'supports_custom_ratio', 'SupportsCustomRatio')
	const supportedBackgrounds = pick<string[]>(source, 'supported_backgrounds', 'SupportedBackgrounds')
  const capabilitiesByTaskType = pick<Record<string, unknown>>(source, 'capabilities_by_task_type', 'CapabilitiesByTaskType')
  const moderation = pick<string[]>(source, 'moderation', 'Moderation')
  if (quality !== undefined) result.quality = quality
  if (outputFormat !== undefined) result.output_format = outputFormat
  if (supportsCompression !== undefined) result.supports_output_compression = supportsCompression
	if (supportsCustomSize !== undefined) result.supports_custom_size = supportsCustomSize
	if (supportsCustomRatio !== undefined) result.supports_custom_ratio = supportsCustomRatio
	if (supportedBackgrounds !== undefined) result.supported_backgrounds = supportedBackgrounds
	for (const [key, aliases] of Object.entries({
		min_width: ['min_width', 'MinWidth'], max_width: ['max_width', 'MaxWidth'],
		min_height: ['min_height', 'MinHeight'], max_height: ['max_height', 'MaxHeight'],
	})) {
		const value = Number(pick(source, ...aliases) ?? 0)
		if (value > 0) Object.assign(result, { [key]: value })
	}
  if (capabilitiesByTaskType !== undefined) {
    result.capabilities_by_task_type = Object.fromEntries(
      Object.entries(capabilitiesByTaskType).map(([taskType, capability]) => [normalizeTaskType(taskType), normalizeTaskCapability(capability)]),
    )
  }
  if (moderation !== undefined) result.moderation = moderation
  return result
}

function normalizeTaskCapability(source: unknown): CapabilityTaskOptions {
	const result: CapabilityTaskOptions = {
    auto_base_resolution: String(pick(source, 'auto_base_resolution', 'AutoBaseResolution') ?? '').trim().toLowerCase() || undefined,
    supports_output_compression: Boolean(pick(source, 'supports_output_compression', 'SupportsOutputCompression')),
		supports_custom_size: Boolean(pick(source, 'supports_custom_size', 'SupportsCustomSize')),
		supports_custom_ratio: Boolean(pick(source, 'supports_custom_ratio', 'SupportsCustomRatio')),
		min_width: positiveNumber(pick(source, 'min_width', 'MinWidth')),
		max_width: positiveNumber(pick(source, 'max_width', 'MaxWidth')),
		min_height: positiveNumber(pick(source, 'min_height', 'MinHeight')),
		max_height: positiveNumber(pick(source, 'max_height', 'MaxHeight')),
    max_output_image_count: Number(pick(source, 'max_output_image_count', 'MaxOutputImageCount') ?? 0),
    max_reference_image_count: Number(pick(source, 'max_reference_image_count', 'MaxReferenceImageCount') ?? 0),
  }
	const baseResolution = pick<string[]>(source, 'base_resolution', 'BaseResolution')
	const sizeModes = pick<Array<'auto' | 'ratio' | 'pixel' | string>>(source, 'size_modes', 'SizeModes')
	const aspectRatios = pick<string[]>(source, 'aspect_ratios', 'AspectRatios')
	const pixelSizes = pick<string[]>(source, 'pixel_sizes', 'PixelSizes')
	const quality = pick<string[]>(source, 'quality', 'Quality')
	const outputFormat = pick<string[]>(source, 'output_format', 'OutputFormat')
	const backgrounds = pick<string[]>(source, 'supported_backgrounds', 'SupportedBackgrounds')
	const moderation = pick<string[]>(source, 'moderation', 'Moderation')
	if (baseResolution !== undefined) result.base_resolution = normalizeBaseResolutions(baseResolution)
	if (sizeModes !== undefined) result.size_modes = sizeModes
	if (aspectRatios !== undefined) result.aspect_ratios = aspectRatios
	if (pixelSizes !== undefined) result.pixel_sizes = pixelSizes
	if (quality !== undefined) result.quality = quality
	if (outputFormat !== undefined) result.output_format = outputFormat
	if (backgrounds !== undefined) result.supported_backgrounds = backgrounds
	if (moderation !== undefined) result.moderation = moderation
	return result
}

function minimumDisplayPoints(prices: Array<{ charged_points: string }>) {
  const values = prices.map((price) => Number(price.charged_points)).filter(Number.isFinite)
  return values.length ? Math.min(...values).toFixed(2) : undefined
}

export function normalizeCapabilities(raw: any): Capability {
  const models = pick<any[]>(raw, 'model_groups', 'ModelGroups', 'abstract_models', 'AbstractModels', 'models', 'Models', 'items', 'Items') ?? []
  const normalizedModels: Capability['model_groups'] = models.flatMap((item: any) => {
    const taskTypes = pick<string[]>(item, 'task_types', 'TaskTypes') ?? ['text_to_image']
    const normalizedTaskTypes = taskTypes.map(normalizeTaskType)
    const qualities = pick<string[]>(item, 'qualities', 'Qualities', 'supported_qualities', 'SupportedQualities')
      ?? pick<string[]>(raw, 'qualities', 'Qualities', 'supported_qualities', 'SupportedQualities')
      ?? ['auto']
		const baseResolution = normalizeBaseResolutions(pick<string[]>(item, 'base_resolution', 'BaseResolution', 'supported_base_resolution', 'SupportedBaseResolution') ?? qualities)
    const autoBaseResolutionByTaskType = pick<Record<string, string>>(item, 'auto_base_resolution_by_task_type', 'AutoBaseResolutionByTaskType')
    const prices = (pick<any[]>(item, 'prices', 'Prices') ?? []).map((price: any) => {
      const quality = String(pick(price, 'quality', 'Quality', 'base_resolution', 'BaseResolution') ?? 'auto')
      return {
        task_type: normalizeTaskType(pick<string>(price, 'task_type', 'TaskType') ?? 'text_to_image'),
        quality,
        base_resolution: String(pick(price, 'base_resolution', 'BaseResolution') ?? quality),
        base_points: String(pick(price, 'base_points', 'BasePoints') ?? '0.00000'),
        charged_points: String(pick(price, 'charged_points', 'ChargedPoints', 'points', 'Points', 'base_points', 'BasePoints') ?? '0.00000'),
        display_points: String(pick(price, 'display_points', 'DisplayPoints', 'charged_points', 'ChargedPoints', 'points', 'Points', 'base_points', 'BasePoints') ?? '0.00'),
        reference_multiplier: pick<string>(price, 'reference_multiplier', 'ReferenceMultiplier'),
      }
    })
    const code = pick(item, 'code', 'Code', 'route_model_code', 'RouteModelCode', 'abstract_model', 'AbstractModel', 'group_code', 'GroupCode', 'model_code', 'ModelCode', 'id', 'ID')
    const normalizedCode = String(code ?? '').trim()
    if (!normalizedCode || normalizedCode === 'undefined') return []
    const maxReference = Number(pick(item, 'max_reference_image_count', 'MaxReferenceImageCount', 'max_reference_count', 'MaxReferenceCount') ?? 0)
    return [{
      id: normalizedCode,
      code: normalizedCode,
      name: pick<string>(item, 'name', 'Name', 'group_name', 'GroupName', 'model_code', 'ModelCode') ?? normalizedCode,
      description: pick<string>(item, 'description', 'Description') ?? '',
      task_types: normalizedTaskTypes,
      qualities,
      base_resolution: baseResolution,
      auto_base_resolution_by_task_type: autoBaseResolutionByTaskType
        ? Object.fromEntries(Object.entries(autoBaseResolutionByTaskType).map(([taskType, resolution]) => [normalizeTaskType(taskType), String(resolution).trim().toLowerCase()]))
        : undefined,
      size_modes: pick<Array<'auto' | 'ratio' | 'pixel' | string>>(item, 'size_modes', 'SizeModes') ?? ['ratio'],
      aspect_ratios: pick<string[]>(item, 'aspect_ratios', 'AspectRatios') ?? pick<string[]>(raw, 'aspect_ratios', 'AspectRatios', 'supported_ratios', 'SupportedRatios') ?? [],
      pixel_sizes: pick<string[]>(item, 'pixel_sizes', 'PixelSizes', 'supported_pixel_sizes', 'SupportedPixelSizes') ?? [],
      max_output_image_count: Number(pick(item, 'max_output_image_count', 'MaxOutputImageCount', 'max_image_count', 'MaxImageCount') ?? pick(raw, 'max_image_count', 'MaxImageCount') ?? 4),
      max_reference_image_count: maxReference,
      effective_multiplier: pick<string>(item, 'effective_multiplier', 'EffectiveMultiplier'),
      minimum_points: pick<string>(item, 'minimum_points', 'MinimumPoints') ?? minimumDisplayPoints(prices),
      prices,
      supports_reference: Boolean(pick(item, 'supports_reference', 'SupportsReference', 'supports_image_input', 'SupportsImageInput') ?? ((maxReference > 0) || normalizedTaskTypes.includes('image_edit'))),
      display_points: pick<string>(item, 'display_points', 'DisplayPoints') ?? prices[0]?.display_points,
      ...optionalOutputCapabilities(item),
    }]
  })
  const qualities = pick<string[]>(raw, 'qualities', 'Qualities', 'supported_qualities', 'SupportedQualities') ?? normalizedModels[0]?.qualities ?? ['auto', '1K', '2K', '4K']
  return {
    raw,
    unavailable_reason: pick(raw, 'unavailable_reason', 'UnavailableReason') ?? null,
    model_groups: normalizedModels,
    qualities,
		base_resolution: normalizeBaseResolutions(pick<string[]>(raw, 'base_resolution', 'BaseResolution', 'supported_base_resolution', 'SupportedBaseResolution') ?? normalizedModels[0]?.base_resolution ?? qualities),
    size_modes: pick<Array<'auto' | 'ratio' | 'pixel' | string>>(raw, 'size_modes', 'SizeModes') ?? ['ratio'],
    aspect_ratios: pick<string[]>(raw, 'aspect_ratios', 'AspectRatios', 'supported_ratios', 'SupportedRatios') ?? normalizedModels[0]?.aspect_ratios ?? ['1:1', '16:9', '9:16', '4:3'],
    pixel_sizes: pick<string[]>(raw, 'pixel_sizes', 'PixelSizes', 'supported_pixel_sizes', 'SupportedPixelSizes') ?? [],
    max_image_count: Number(pick(raw, 'max_image_count', 'MaxImageCount') ?? 4),
    reference_image_max_mb: Number(pick(raw, 'reference_image_max_mb', 'ReferenceImageMaxMB') ?? 0) || undefined,
    reference_image_max_bytes: Number(pick(raw, 'reference_image_max_bytes', 'ReferenceImageMaxBytes') ?? 0) || undefined,
    reference_image_allowed_formats: pick<string[]>(raw, 'reference_image_allowed_formats', 'ReferenceImageAllowedFormats') ?? [],
    reference_image_allowed_mime_types: pick<string[]>(raw, 'reference_image_allowed_mime_types', 'ReferenceImageAllowedMIMETypes') ?? [],
    task_types: (pick<string[]>(raw, 'task_types', 'TaskTypes') ?? Array.from(new Set(normalizedModels.flatMap((item) => item.task_types)))).map(normalizeTaskType),
    ...optionalOutputCapabilities(raw),
  }
}

function normalizeBaseResolutions(values: string[]) {
	return Array.from(new Set(values.map((value) => value.trim().toLowerCase()).filter((value) => value === '1k' || value === '2k' || value === '4k')))
}

function positiveNumber(value: unknown) {
	const parsed = Number(value ?? 0)
	return parsed > 0 ? parsed : undefined
}

function uniqueValues<T>(values: T[]) {
	return Array.from(new Set(values))
}

export function normalizeVideoCapabilities(raw: VideoCapabilityListWire): VideoCapability {
	const groups = Array.isArray(raw?.groups) ? raw.groups : []
	return {
		capability_version: groups[0]?.capability_version ?? '',
		model_groups: groups.flatMap((group) => {
			const taskTypes = uniqueValues(group.task_types ?? group.combinations.map((item) => item.task_type))
			const first = group.combinations[0]
			if (!first || taskTypes.length === 0) return []
			const optionsByTaskType = Object.fromEntries(taskTypes.flatMap((taskType) => {
				const combinations = group.combinations.filter((item) => item.task_type === taskType)
				if (combinations.length === 0) return []
				return [[taskType, {
					durations: uniqueValues(combinations.map((item) => item.duration_seconds)),
					resolutions: uniqueValues(combinations.map((item) => item.resolution)),
					aspect_ratios: uniqueValues(combinations.map((item) => item.aspect_ratio)),
					audio_generation: combinations.some((item) => item.audio_mode === 'generated'),
					combinations: combinations.map(({ duration_seconds, resolution, aspect_ratio, audio_mode }) => ({ duration_seconds, resolution, aspect_ratio, audio_mode })),
				}]]
			})) as VideoCapability['model_groups'][number]['options_by_task_type']
			return [{
				code: group.route_model_code,
				name: group.name,
				description: group.description,
				minimum_points: '',
				max_output_count: group.max_output_count,
				task_types: taskTypes,
				defaults: {
					task_type: first.task_type,
					duration_seconds: first.duration_seconds,
					resolution: first.resolution,
					aspect_ratio: first.aspect_ratio,
					generate_audio: first.audio_mode === 'generated',
				},
				options_by_task_type: optionsByTaskType,
			}]
		}),
	}
}

export function buildVideoEstimateWireRequest(req: VideoEstimateRequest): VideoEstimateRequest {
	return {
		project_id: req.project_id,
		route_model_code: req.route_model_code,
		task_type: req.task_type,
		prompt_template: req.prompt_template,
		prompt_variables: req.prompt_variables,
		reference_bindings: req.reference_bindings,
		inputs: req.inputs,
		duration_seconds: req.duration_seconds,
		resolution: req.resolution,
		aspect_ratio: req.aspect_ratio,
		audio_mode: req.audio_mode,
		output_count: req.output_count,
	}
}

export function normalizeTaskList(raw: any): ImageTask[] {
  const items = Array.isArray(raw) ? raw : Array.isArray(raw?.items) ? raw.items : []
  return items.map(toTask)
}

export const userApi = {
  configureAuth: sharedApiClient.setAuth.bind(sharedApiClient),
  getFeatureFlags: () => sharedApiClient.request<FeatureFlags>(API_PATHS.agent.features, { auth: false, retryUnauthorized: false }),
  sendEmailCode: (email: string, scene: 'login' | 'register' | 'password_reset' | 'password_change' = 'login') =>
    sharedApiClient.request<{ email: string; scene: string; status: string }>(API_PATHS.agent.sendEmailCode, { method: 'POST', body: { email, scene }, auth: false }),
  loginWithEmailCode: (email: string, code: string) =>
    sharedApiClient.request<LoginResult>(API_PATHS.agent.loginEmailCode, { method: 'POST', body: { email, code }, auth: false }),
  loginWithPassword: (email: string, password: string) =>
    sharedApiClient.request<NormalLoginResponse>(API_PATHS.agent.loginPassword, { method: 'POST', body: { email, password }, auth: false }),
  completePasswordSetup: (password_setup_token: string, new_password: string) =>
    sharedApiClient.request<NormalLoginResponse>(API_PATHS.agent.passwordSetup, { method: 'POST', body: { password_setup_token, new_password }, auth: false }),
  refreshSession: () =>
    sharedApiClient.request<NormalLoginResponse>(API_PATHS.agent.refreshSession, { method: 'POST', auth: false, retryUnauthorized: false }),
  logout: () => sharedApiClient.request<void>(API_PATHS.agent.logout, { method: 'POST' }),
  changePassword: (code: string, new_password: string) =>
    sharedApiClient.request<{ status: string }>(API_PATHS.agent.passwordChange, { method: 'POST', body: { code, new_password } }),
  requestPasswordReset: (email: string) =>
    sharedApiClient.request<{ status: string }>(API_PATHS.agent.passwordResetRequest, { method: 'POST', body: { email }, auth: false }),
  confirmPasswordReset: (email: string, code: string, new_password: string) =>
    sharedApiClient.request<{ ok: boolean }>(API_PATHS.agent.passwordResetConfirm, { method: 'POST', body: { email, code, new_password }, auth: false }),
  getProfile: async () => toUserProfile(await sharedApiClient.request(API_PATHS.agent.profile)),
  getProfileWithToken: async (token: string) => toUserProfile(await sharedApiClient.request(API_PATHS.agent.profile, { auth: false, retryUnauthorized: false, headers: { Authorization: `Bearer ${token}` } })),
  updateProfile: async (patch: Partial<UserProfile>) => toUserProfile(await sharedApiClient.request(API_PATHS.agent.profile, {
    method: 'PUT',
    body: {
      nickname: patch.display_name ?? patch.nickname,
      bio: patch.signature ?? patch.bio,
      avatar_object_key: patch.avatar_object_key,
      default_locale: patch.default_locale,
      theme: patch.theme,
    },
  })),
  updatePreferences: async (preferences: UpdatePreferencesRequest) => toUserProfile(await sharedApiClient.request(API_PATHS.agent.preferences, { method: 'PUT', body: preferences })),
  uploadAvatar: async (file: File) => {
    const formData = new FormData()
    formData.set('file', file)
    return toUserProfile(await sharedApiClient.request(API_PATHS.agent.avatar, { method: 'POST', formData }))
  },
  closeAccount: () => sharedApiClient.request<void>(API_PATHS.agent.accountClose, { method: 'POST' }),
	listProjects: async () => (await sharedApiClient.request<{ items: Project[]; default_project_id: string }>(API_PATHS.agent.projects)).items ?? [],
	createProject: (name: string, idempotencyKey: string = crypto.randomUUID()) => sharedApiClient.request<Project>(API_PATHS.agent.projects, { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: { name } }),
	renameProject: (project_id: string, name: string, expected_version: number) => sharedApiClient.request<Project>(API_PATHS.agent.projectDetail, { method: 'PATCH', pathParams: { project_id }, body: { name, expected_version } }),
	deleteProject: (project_id: string, expected_version: number, target_project_id?: string, idempotencyKey: string = crypto.randomUUID()) => sharedApiClient.request<{ project: Project; transferred: { tasks: number; assets: number } }>(API_PATHS.agent.projectDetail, { method: 'DELETE', pathParams: { project_id }, headers: { 'Idempotency-Key': idempotencyKey }, body: { expected_version, target_project_id } }),
  getBalance: async () => toBalance(await sharedApiClient.request(API_PATHS.agent.balance)),
  getLedger: async (page = 1, page_size = 20) => {
    const result = normalizePage<LedgerEntry>(await sharedApiClient.request(API_PATHS.agent.ledger, { query: { page, page_size } }))
    return result.items
  },
  listPlans: async () => (await sharedApiClient.request<{ items: BillingPlan[] }>(API_PATHS.agent.plans)).items ?? [],
  getSubscription: async () => (await sharedApiClient.request<{ item: Subscription | null }>(API_PATHS.agent.subscription)).item,
  listOrders: async (page = 1, page_size = 20) => normalizePage<CashierOrder>(await sharedApiClient.request(API_PATHS.agent.orders, { query: { page, page_size } })),
  createOrder: (plan_code: string, provider = 'alipay') => sharedApiClient.request<CashierOrder>(API_PATHS.agent.orders, { method: 'POST', body: { plan_code, provider } }),
  getOrder: (order_id: string | number) => sharedApiClient.request<CashierOrder>(API_PATHS.agent.orderDetail, { pathParams: { order_id } }),
  cancelOrder: (order_id: string | number) => sharedApiClient.request<CashierOrder>(API_PATHS.agent.orderCancel, { method: 'POST', pathParams: { order_id } }),
  getCashierOptions: () => sharedApiClient.request<CashierOptions>(API_PATHS.agent.cashierOptions),
  listCashierOrders: async (page = 1, page_size = 20) => normalizePage<CashierOrder>(await sharedApiClient.request(API_PATHS.agent.cashierOrders, { query: { page, page_size } })),
  createCashierOrder: (input: CreateCashierOrderRequest, idempotencyKey: string = crypto.randomUUID()) =>
    sharedApiClient.request<CashierOrder>(API_PATHS.agent.cashierOrders, {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey },
      body: input,
    }),
  getCashierOrder: (order_id: string | number) => sharedApiClient.request<CashierOrder>(API_PATHS.agent.cashierOrderDetail, { pathParams: { order_id } }),
  syncCashierOrder: (order_id: string | number) => sharedApiClient.request<CashierOrderSyncResponse>(API_PATHS.agent.cashierOrderSync, { method: 'POST', pathParams: { order_id } }),
  cancelCashierOrder: (order_id: string | number) => sharedApiClient.request<CashierOrder>(API_PATHS.agent.cashierOrderCancel, { method: 'POST', pathParams: { order_id } }),
  mockPayCashierOrder: (order_id: string | number) => sharedApiClient.request<CashierOrder>(API_PATHS.agent.cashierOrderMockPay, { method: 'POST', pathParams: { order_id } }),
  redeemCode: (code: string, idempotencyKey = crypto.randomUUID()) => sharedApiClient.request(API_PATHS.agent.redeemCode, { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: { code } }),
  getCapabilities: async (): Promise<Capability> => normalizeCapabilities(await sharedApiClient.request(API_PATHS.agent.capabilities)),
	getVideoCapabilities: async (): Promise<VideoCapability> => normalizeVideoCapabilities(await sharedApiClient.request<VideoCapabilityListWire>(API_PATHS.agent.videoCapabilities)),
	estimateVideo: (req: VideoEstimateRequest, signal?: AbortSignal) => sharedApiClient.request<VideoEstimate>(API_PATHS.agent.videoEstimates, { method: 'POST', body: buildVideoEstimateWireRequest(req), signal }),
	createVideoTask: (req: VideoCreateTaskRequest, idempotencyKey: string = crypto.randomUUID()) => sharedApiClient.request<VideoTask>(API_PATHS.agent.videoTasks, { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: { ...buildVideoEstimateWireRequest(req), quote_token: req.quote_token } }),
  listVideoTasks: async (filters: { project_id: string; status?: string; cursor?: string; limit?: number }) => {
    const response = await sharedApiClient.request<{ items?: VideoTask[]; next_cursor?: string } | VideoTask[]>(API_PATHS.agent.videoTasks, { query: filters })
    return Array.isArray(response) ? { items: response } : { items: response.items ?? [], next_cursor: response.next_cursor }
  },
  getVideoTask: (task_id: string) => sharedApiClient.request<VideoTask>(API_PATHS.agent.videoTaskDetail, { pathParams: { task_id } }),
	cancelVideoTask: (task_id: string, idempotencyKey: string = crypto.randomUUID()) => sharedApiClient.request<VideoTask>(API_PATHS.agent.videoTaskCancel, { method: 'POST', pathParams: { task_id }, headers: { 'Idempotency-Key': idempotencyKey } }),
	videoTaskStreamUrl: (accessToken?: string | null, projectID?: string) => apiEventUrl(withQuery(API_PATHS.agent.videoTaskEvents, { project_id: projectID }), accessToken),
	listCanvases: async (filters?: { project_id?: string; search?: string }) => ((await sharedApiClient.request<{ items: CreativeCanvas[] }>('/api/agent/canvas/v1/canvases', { query: filters })).items ?? []).map(normalizeCreativeCanvas),
	createCanvas: async (input: { project_id: string; name: string; template?: 'blank' | 'image_exploration' | 'image_to_video'; document?: CanvasDocument }) => normalizeCreativeCanvas(await sharedApiClient.request<CreativeCanvas>('/api/agent/canvas/v1/canvases', { method: 'POST', body: input })),
	getCanvas: async (canvas_id: string) => normalizeCreativeCanvas(await sharedApiClient.request<CreativeCanvas>('/api/agent/canvas/v1/canvases/{canvas_id}', { pathParams: { canvas_id } })),
	renameCanvas: async (canvas_id: string, name: string, expected_metadata_version: number) => normalizeCreativeCanvas(await sharedApiClient.request<CreativeCanvas>('/api/agent/canvas/v1/canvases/{canvas_id}', { method: 'PATCH', pathParams: { canvas_id }, body: { name, expected_metadata_version } })),
	deleteCanvas: (canvas_id: string, expected_metadata_version: number) => sharedApiClient.request<{ id: string; status: 'deleted' }>('/api/agent/canvas/v1/canvases/{canvas_id}', { method: 'DELETE', pathParams: { canvas_id }, body: { expected_metadata_version } }),
	duplicateCanvas: async (canvas_id: string, input: { name?: string; project_id?: string } = {}) => normalizeCreativeCanvas(await sharedApiClient.request<CreativeCanvas>('/api/agent/canvas/v1/canvases/{canvas_id}:duplicate', { method: 'POST', pathParams: { canvas_id }, body: input })),
	transferCanvas: async (canvas_id: string, target_project_id: string, expected_metadata_version: number) => normalizeCreativeCanvas(await sharedApiClient.request<CreativeCanvas>('/api/agent/canvas/v1/canvases/{canvas_id}:transfer-project', { method: 'POST', pathParams: { canvas_id }, body: { target_project_id, expected_metadata_version } })),
	saveCanvasDocument: async (canvas_id: string, expected_revision: number, document: CanvasDocument) => normalizeCreativeCanvas(await sharedApiClient.request<CreativeCanvas>('/api/agent/canvas/v1/canvases/{canvas_id}/document', { method: 'PUT', pathParams: { canvas_id }, body: { expected_revision, document } })),
	estimateCanvasNode: (canvas_id: string, node_id: string) => sharedApiClient.request<{ points: string; detail?: Record<string, unknown> }>('/api/agent/canvas/v1/canvases/{canvas_id}/nodes/{node_id}:estimate', { method: 'POST', pathParams: { canvas_id, node_id } }),
	generateCanvasNode: (canvas_id: string, node_id: string, idempotencyKey = crypto.randomUUID()) => sharedApiClient.request<CanvasRun>('/api/agent/canvas/v1/canvases/{canvas_id}/nodes/{node_id}:generate', { method: 'POST', pathParams: { canvas_id, node_id }, headers: { 'Idempotency-Key': idempotencyKey } }),
	listCanvasRuns: async (canvas_id: string, refresh = false) => (await sharedApiClient.request<{ items: CanvasRun[] }>('/api/agent/canvas/v1/canvases/{canvas_id}/runs', { pathParams: { canvas_id }, query: { refresh } })).items ?? [],
	attachCanvasRun: (canvas_id: string, run_id: string, recovery_position?: { x: number; y: number }) => sharedApiClient.request<CanvasRun>('/api/agent/canvas/v1/canvases/{canvas_id}/runs/{run_id}:attach-results', { method: 'POST', pathParams: { canvas_id, run_id }, body: recovery_position ? { recovery_position } : undefined }),
	cancelCanvasRun: (canvas_id: string, run_id: string) => sharedApiClient.request<CanvasRun>('/api/agent/canvas/v1/canvases/{canvas_id}/runs/{run_id}:cancel', { method: 'POST', pathParams: { canvas_id, run_id } }),
  listMediaAssets: async (filters: MediaAssetFilters) => {
    const response = await sharedApiClient.request<MediaAssetPage | MediaAsset[]>(API_PATHS.agent.mediaAssets, { query: filters })
    return Array.isArray(response) ? { items: response } : { items: response.items ?? [], next_cursor: response.next_cursor }
  },
  getMediaAsset: (asset_id: string) => sharedApiClient.request<MediaAsset>(API_PATHS.agent.mediaAssetDetail, { pathParams: { asset_id } }),
  updateMediaAsset: (asset: Pick<MediaAsset, 'id' | 'version'>, patch: { name?: string; group_name?: string; project_id?: string }) => sharedApiClient.request<MediaAsset>(API_PATHS.agent.mediaAssetDetail, {
    method: 'PATCH', pathParams: { asset_id: asset.id }, body: { ...patch, expected_version: asset.version },
  }),
  deleteMediaAsset: (asset: Pick<MediaAsset, 'id' | 'version'>) => sharedApiClient.request<MediaAsset>(API_PATHS.agent.mediaAssetDetail, {
    method: 'DELETE', pathParams: { asset_id: asset.id }, body: { expected_version: asset.version },
  }),
  retryMediaAssetProcessing: (asset_id: string) => sharedApiClient.request<MediaAsset>(API_PATHS.agent.mediaAssetRetry, { method: 'POST', pathParams: { asset_id } }),
  getMediaAssetAccess: (asset_id: string, purpose: MediaAccessPurpose = 'preview', signal?: AbortSignal) => sharedApiClient.request<MediaAccessProjection>(API_PATHS.agent.mediaAssetAccess, { pathParams: { asset_id }, query: { purpose }, signal }),
  batchMediaAssets: (action: MediaBatchAction, items: Array<Pick<MediaAsset, 'id' | 'version'>>, options?: { group_name?: string; target_project_id?: string }) => sharedApiClient.request<MediaBatchResult>(API_PATHS.agent.mediaAssetBatch, {
    method: 'POST', pathParams: { action }, body: { items: items.map((item) => ({ id: item.id, expected_version: item.version })), ...options },
  }),
  createMediaExport: (project_id: string, items: Array<Pick<MediaAsset, 'id' | 'version'>>) => sharedApiClient.request<MediaExportStatus>(API_PATHS.agent.mediaAssetBatch, {
    method: 'POST', pathParams: { action: 'download' }, body: { project_id, items: items.map((item) => ({ id: item.id, expected_version: item.version })) },
  }),
  getMediaExportJob: (job_id: string, signal?: AbortSignal) => sharedApiClient.request<MediaExportStatus>(API_PATHS.agent.mediaExportJob, { pathParams: { job_id }, signal }),
  downloadMediaExport: (job_id: string, signal?: AbortSignal) => sharedApiClient.blob(API_PATHS.agent.mediaExportDownload, { pathParams: { job_id }, signal }),
  initMediaUpload: (input: MediaUploadInit, idempotencyKey: string) => sharedApiClient.request<MediaUploadSession>(API_PATHS.agent.mediaUploads, {
    method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: input,
  }),
  getMediaUpload: (upload_id: string) => sharedApiClient.request<MediaUploadSession>(API_PATHS.agent.mediaUploadDetail, { pathParams: { upload_id } }),
  signMediaUploadPart: (upload_id: string, part_number: number, checksum: string) => sharedApiClient.request<MediaPartTarget>(API_PATHS.agent.mediaUploadPartSign, {
    method: 'POST', pathParams: { upload_id, part_number }, body: { checksum },
  }),
  uploadMediaLocalPart: async (upload_id: string, part_number: number, chunk: Blob, checksum: string, accessToken?: string | null, signal?: AbortSignal) => {
    const path = fillPath(API_PATHS.agent.mediaUploadPart, { upload_id, part_number })
    const response = await fetch(`${getDefaultBaseUrl()}${path}`, {
      method: 'PUT', credentials: 'include', body: chunk, signal,
      headers: { 'Content-Type': 'application/octet-stream', 'X-Content-SHA256': checksum, ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}) },
    })
    const payload = await response.json()
    if (!response.ok) throw Object.assign(new Error(payload?.error?.message ?? '分片上传失败'), { status: response.status })
    return (payload?.data ?? payload) as MediaCompletedPart
  },
  completeMediaUpload: (upload_id: string, parts: MediaCompletedPart[]) => sharedApiClient.request<MediaAsset>(API_PATHS.agent.mediaUploadComplete, {
    method: 'POST', pathParams: { upload_id }, body: { parts },
  }),
  abortMediaUpload: (upload_id: string) => sharedApiClient.request<{ status: string }>(API_PATHS.agent.mediaUploadDetail, { method: 'DELETE', pathParams: { upload_id } }),
  estimatePromptOptimization: (prompt: string) => sharedApiClient.request<PromptOptimizationEstimate>(API_PATHS.agent.promptOptimizationEstimate, { method: 'POST', body: { prompt } }),
  optimizePrompt: (prompt: string, quote: string) => sharedApiClient.request<PromptOptimizationResult>(API_PATHS.agent.promptOptimizations, { method: 'POST', body: { prompt, quote } }),
  estimate: async (req: EstimateRequest) => toEstimate(await sharedApiClient.request(API_PATHS.agent.estimate, { query: buildEstimateWireRequest(req) }), req),
  uploadReferenceAsset: async (file: File | string, sizeBytes?: number) => {
    if (typeof file === 'string') {
      return { id: '', name: file, preview_url: '', status: 'ready', size_bytes: sizeBytes ?? 0, created_at: '' } satisfies ReferenceAsset
    }
    const formData = new FormData()
    formData.set('file', file)
    return toReferenceAsset(await sharedApiClient.request(API_PATHS.agent.referenceAssets, { method: 'POST', formData }))
  },
  listReferenceAssets: async () => [] as ReferenceAsset[],
  importReferenceAssetsFromGallery: async (galleryImageIds: string[]) => {
    const response = await sharedApiClient.request<{ items?: any[]; assets?: any[]; references?: any[] } | any[]>(API_PATHS.agent.importReferenceAssetsFromGallery, {
      method: 'POST',
      body: buildGalleryReferenceImportRequest(galleryImageIds),
    })
    const items = Array.isArray(response) ? response : response.items ?? response.assets ?? response.references ?? []
    return items.map(toReferenceAsset)
  },
  importReferenceAssetsFromMedia: async (mediaAssetIds: string[]) => {
    const response = await sharedApiClient.request<{ items?: any[] }>(API_PATHS.agent.importReferenceAssetsFromMedia, {
      method: 'POST', body: { media_asset_ids: mediaAssetIds },
    })
    return (response.items ?? []).map(toReferenceAsset)
  },
  getReferenceAsset: async (asset_id: string) => toReferenceAsset(await sharedApiClient.request(API_PATHS.agent.referenceAssetDetail, { pathParams: { asset_id } })),
  renameReferenceAsset: async (asset_id: string, name: string) => toReferenceAsset(await sharedApiClient.request(API_PATHS.agent.referenceAssetDetail, { method: 'PATCH', pathParams: { asset_id }, body: { name } })),
  refreshReferenceAssetAccess: (asset_id: string, purpose: MediaAccessPurpose = 'preview') =>
    sharedApiClient.request<MediaAccessProjection>(API_PATHS.agent.referenceAssetAccess, { pathParams: { asset_id }, query: { purpose } }),
  refreshImageAccess: (image_id: string, purpose: MediaAccessPurpose = 'preview') =>
    sharedApiClient.request<MediaAccessProjection>(API_PATHS.agent.imageAccess, { pathParams: { image_id }, query: { purpose } }),
  refreshPublicImageAccess: (image_id: string, purpose: MediaAccessPurpose = 'preview') =>
    sharedApiClient.request<MediaAccessProjection>(API_PATHS.open.galleryImageAccess, { pathParams: { image_id }, query: { purpose }, auth: false }),
  deleteReferenceAsset: (asset_id: string) => sharedApiClient.request<void>(API_PATHS.agent.referenceAssetDetail, { method: 'DELETE', pathParams: { asset_id } }),
  imageAssetUrl: (url: string, accessToken?: string | null) => mediaAssetURL(url, accessToken),
  createTask: async (req: CreateTaskRequest) => {
    const wire = buildCreateTaskWireRequest(req)
    return toTask(await sharedApiClient.request(API_PATHS.agent.tasks, { method: 'POST', body: wire.body, headers: wire.headers }))
  },
  getTask: async (task_id: string) => toTask(await sharedApiClient.request(API_PATHS.agent.taskDetail, { pathParams: { task_id } })),
  taskEventsUrl: (task_id: string, accessToken?: string | null) => apiEventUrl(fillPath(API_PATHS.agent.taskEvents, { task_id }), accessToken),
  taskStreamUrl: (accessToken?: string | null, projectID?: string) => apiEventUrl(withQuery(API_PATHS.agent.taskStream, { project_id: projectID }), accessToken),
  listTasks: async (filters?: { query?: string; status?: string; type?: string; project_id?: string }) => {
    const response = await sharedApiClient.request(API_PATHS.agent.tasks, {
	  query: { project_id: filters?.project_id, status: filters?.status === 'all' ? undefined : filters?.status, task_type: filters?.type === 'all' ? undefined : filters?.type, query: filters?.query },
    })
    return normalizeTaskList(response)
  },
  listHistoryTasks: async (filters?: { query?: string; status?: string; type?: string; project_id?: string }) => {
    const response = await sharedApiClient.request(API_PATHS.agent.historyTasks, {
	  query: { project_id: filters?.project_id, status: filters?.status === 'all' ? undefined : filters?.status, task_type: filters?.type === 'all' ? undefined : filters?.type, query: filters?.query },
    })
    return normalizeTaskList(response)
  },
	listGalleryImages: async (page = 1, page_size = 100, project_id?: string) => normalizePage<GalleryImage>(await sharedApiClient.request(API_PATHS.agent.galleryImages, { query: { page, page_size, project_id } })).items.map(toGalleryImage),
	batchPublishGalleryImages: (image_ids: string[], project_id: string, publish = true) => sharedApiClient.request<GalleryBatchMutationResult>(API_PATHS.agent.galleryBatchPublish, { method: 'POST', body: { image_ids, project_id, publish } }),
	batchGroupGalleryImages: (image_ids: string[], project_id: string, image_group: string) => sharedApiClient.request<GalleryBatchMutationResult>(API_PATHS.agent.galleryBatchGroup, { method: 'POST', body: { image_ids, project_id, image_group } }),
	batchDeleteGalleryImages: (image_ids: string[], project_id: string) => sharedApiClient.request<GalleryBatchMutationResult>(API_PATHS.agent.galleryBatchDelete, { method: 'POST', body: { image_ids, project_id } }),
	batchTransferGalleryImages: (image_ids: string[], project_id: string, target_project_id: string) => sharedApiClient.request<GalleryBatchMutationResult>(API_PATHS.agent.galleryBatchTransferProject, { method: 'POST', body: { image_ids, project_id, target_project_id } }),
	batchDownloadGalleryImages: (image_ids: string[], project_id: string) => sharedApiClient.blob(API_PATHS.agent.galleryBatchDownload, { method: 'POST', body: { image_ids, project_id } }) as Promise<Blob | GalleryExportStatus>,
	getGalleryExportJob: (job_id: string, signal?: AbortSignal) => sharedApiClient.request<GalleryExportStatus>(API_PATHS.agent.galleryExportJob, { pathParams: { job_id }, signal }),
	downloadGalleryExport: (job_id: string, signal?: AbortSignal) => sharedApiClient.blob(API_PATHS.agent.galleryExportDownload, { pathParams: { job_id }, signal }),
  retryTask: async (task_id: string) => toTask(await sharedApiClient.request(API_PATHS.agent.historyTaskRetry, { method: 'POST', pathParams: { task_id } })),
  deleteTask: (task_id: string) => sharedApiClient.request<void>(API_PATHS.agent.historyTaskDetail, { method: 'DELETE', pathParams: { task_id } }),
  deleteGalleryImage: (image_id: string) => sharedApiClient.request<void>(API_PATHS.agent.galleryImageDetail, { method: 'DELETE', pathParams: { image_id } }),
  updateGalleryImageGroup: async (image_id: string, image_group: string) => toGalleryImage(await sharedApiClient.request(API_PATHS.agent.galleryImageGroup, { method: 'PUT', pathParams: { image_id }, body: { image_group } })),
  publishImage: async (image_id: string) => toGalleryImage(await sharedApiClient.request(API_PATHS.agent.publishImage, { method: 'POST', pathParams: { image_id } })),
  cancelImagePublish: async (image_id: string) => toGalleryImage(await sharedApiClient.request(API_PATHS.agent.publishImage, { method: 'DELETE', pathParams: { image_id } })),
  likePublicImage: async (image_id: string, active: boolean) => toGalleryImage(await sharedApiClient.request(API_PATHS.agent.likePublicImage, { method: 'POST', pathParams: { image_id }, body: { active } })),
  favoritePublicImage: async (image_id: string, active: boolean) => toGalleryImage(await sharedApiClient.request(API_PATHS.agent.favoritePublicImage, { method: 'POST', pathParams: { image_id }, body: { active } })),
  listApiKeys: async () => ((await sharedApiClient.request<{ items: any[] }>(API_PATHS.agent.apiKeys)).items ?? []).map(toApiKey),
  createApiKey: async (input: CreateApiKeyRequest & { rpm_limit: number; expires_at: string | null }) => toApiKey(await sharedApiClient.request(API_PATHS.agent.apiKeys, {
    method: 'POST',
    body: {
      name: input.name,
      scopes: input.scopes,
      rpm_limit: input.rpm_limit,
      total_quota_points: input.total_quota_points ?? null,
      daily_quota_points: input.daily_quota_points ?? null,
      expires_at: input.expires_at ? new Date(input.expires_at).toISOString() : null,
    },
  })),
  updateApiKey: async (key_id: string | number, patch: Partial<ApiKey>) => toApiKey(await sharedApiClient.request(API_PATHS.agent.apiKeyDetail, {
    method: 'PUT',
    pathParams: { key_id },
    body: { ...patch, expires_at: patch.expires_at ? new Date(patch.expires_at).toISOString() : patch.expires_at },
  })),
  resetApiKeySecret: async (key_id: string | number) => toApiKey(await sharedApiClient.request(API_PATHS.agent.apiKeyResetSecret, { method: 'POST', pathParams: { key_id } })),
  deleteApiKey: (key_id: string | number) => sharedApiClient.request<void>(API_PATHS.agent.apiKeyDetail, { method: 'DELETE', pathParams: { key_id } }),
}

function apiEventUrl(path: string, accessToken?: string | null) {
  return mediaAssetURL(path, accessToken)
}

function toApiKey(raw: any): ApiKey {
  return {
    ...raw,
    id: String(raw.id),
    access_key: raw.access_key ?? raw.key_prefix ?? '',
    secret_preview: raw.secret ?? raw.secret_preview,
    scopes: raw.scopes ?? ['images:write', 'images:read'],
    rpm_limit: Number(raw.rpm_limit ?? 0),
    expires_at: raw.expires_at ?? null,
    created_at: raw.created_at ?? '',
    last_used_at: raw.last_used_at ?? null,
  }
}

export function pageItems<T>(page: PageResult<T>) {
  return page.items
}
