import type { CapabilityModelGroup, ImageTaskType } from '../../../shared/api-types'
import { calculateImageSizeForBaseResolution, validateCustomImageSize, type CustomImageSizeNormalization } from '../../../shared/image-size'

export type WorkspaceOutputParameters = {
  quality: string
  outputFormat: string
  outputCompression: number
  moderation: string
}

export type WorkspaceSizeMode = 'auto' | 'ratio' | 'pixel'

export function normalizeWorkspaceCustomSize(width: string, height: string, model?: CapabilityModelGroup): CustomImageSizeNormalization {
	if (!/^\d+$/.test(width.trim()) || !/^\d+$/.test(height.trim())) {
		return validateCustomImageSize(Number.NaN, Number.NaN)
	}
	return validateCustomImageSize(Number(width), Number(height), {
		minWidth: model?.min_width,
		maxWidth: model?.max_width,
		minHeight: model?.min_height,
		maxHeight: model?.max_height,
	})
}

export function workspaceModelForTask(model: CapabilityModelGroup | undefined, taskType: ImageTaskType) {
  if (!model) return undefined
  const scoped = model.capabilities_by_task_type?.[taskType]
  if (!scoped) return model
  const autoBaseResolutionByTaskType = { ...model.auto_base_resolution_by_task_type }
  if (scoped.auto_base_resolution) autoBaseResolutionByTaskType[taskType] = scoped.auto_base_resolution
  return {
    ...model,
    base_resolution: scoped.base_resolution ?? model.base_resolution,
    auto_base_resolution_by_task_type: autoBaseResolutionByTaskType,
    size_modes: scoped.size_modes ?? model.size_modes,
    aspect_ratios: scoped.aspect_ratios ?? model.aspect_ratios,
    pixel_sizes: scoped.pixel_sizes ?? model.pixel_sizes,
    quality: scoped.quality ?? model.quality,
    output_format: scoped.output_format ?? model.output_format,
    supports_output_compression: scoped.supports_output_compression ?? model.supports_output_compression,
		supports_custom_size: scoped.supports_custom_size ?? model.supports_custom_size,
		supports_custom_ratio: scoped.supports_custom_ratio ?? model.supports_custom_ratio,
		supported_backgrounds: scoped.supported_backgrounds ?? model.supported_backgrounds,
		min_width: scoped.min_width ?? model.min_width,
		max_width: scoped.max_width ?? model.max_width,
		min_height: scoped.min_height ?? model.min_height,
		max_height: scoped.max_height ?? model.max_height,
    moderation: scoped.moderation ?? model.moderation,
    max_output_image_count: scoped.max_output_image_count ?? model.max_output_image_count,
    max_reference_image_count: scoped.max_reference_image_count ?? model.max_reference_image_count,
  } satisfies CapabilityModelGroup
}

export function workspaceCustomSizeSupported(model: CapabilityModelGroup | undefined) {
  if (!model) return false
  return Boolean(model.supports_custom_size)
}

export function workspaceCustomRatioSupported(model: CapabilityModelGroup | undefined) {
	return Boolean(model?.supports_custom_ratio)
}

export function workspaceSizeModeOptions(model: CapabilityModelGroup | undefined): WorkspaceSizeMode[] {
	const values = model?.size_modes
	if (values === undefined) return model ? ['ratio'] : []
	return Array.from(new Set(values.filter((mode): mode is WorkspaceSizeMode => mode === 'auto' || mode === 'ratio' || mode === 'pixel')))
}

export function chooseWorkspaceSizeMode(options: WorkspaceSizeMode[], restored?: WorkspaceSizeMode, current?: WorkspaceSizeMode) {
  if (restored && options.includes(restored)) return restored
  if (current && options.includes(current)) return current
  if (options.includes('auto')) return 'auto'
  if (options.includes('ratio')) return 'ratio'
  return options[0] ?? 'ratio'
}

export function workspaceRatioOptions(model: CapabilityModelGroup | undefined, legacyFallback: string[]) {
	if (!model) return []
	return model.aspect_ratios === undefined ? legacyFallback : model.aspect_ratios
}

export function workspacePixelOptions(model: CapabilityModelGroup | undefined, legacyFallback: string[]) {
	if (!model) return []
	return model.pixel_sizes === undefined ? legacyFallback : model.pixel_sizes
}

export function workspaceBackgroundOptions(model: CapabilityModelGroup | undefined) {
	return normalizedOptions(model?.supported_backgrounds, [])
}

export function workspaceBackgroundForFormat(model: CapabilityModelGroup | undefined, current: string, outputFormat: string) {
	const options = workspaceBackgroundOptions(model)
	let selected = options.includes(current) ? current : options[0] ?? ''
	if (selected === 'transparent' && !['png', 'webp'].includes(outputFormat.toLowerCase())) {
		selected = options.find((value) => value !== 'transparent') ?? ''
	}
	return selected
}

export function workspaceCustomRatioValid(value: string) {
	const match = value.trim().match(/^(\d+)\s*:\s*(\d+)$/)
	if (!match) return false
	const width = Number(match[1])
	const height = Number(match[2])
	if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) return false
	return Math.max(width / height, height / width) <= 3
}

export function workspaceSizeParameterError(input: {
  sizeMode: WorkspaceSizeMode
  pixelSelection?: 'preset' | 'custom'
  customWidth?: string
  customHeight?: string
  ratio?: string
  customRatio?: string
  customRatioSupported?: boolean
  model?: CapabilityModelGroup
}) {
  if (input.sizeMode === 'ratio' && input.ratio === 'custom' && input.customRatioSupported) {
    const value = input.customRatio?.trim() ?? ''
    if (!/^\d+\s*:\s*\d+$/.test(value)) return '自定义比例请使用“宽:高”格式，例如 16:9。'
    if (!workspaceCustomRatioValid(value)) return '自定义比例必须在 1:3 至 3:1 范围内。'
  }
  if (input.sizeMode !== 'pixel' || input.pixelSelection !== 'custom') return ''
  const widthText = input.customWidth?.trim() ?? ''
  const heightText = input.customHeight?.trim() ?? ''
  if (!/^\d+$/.test(widthText)) return '宽度必须填写正整数。'
  if (!/^\d+$/.test(heightText)) return '高度必须填写正整数。'
  const width = Number(widthText)
  const height = Number(heightText)
  const minWidth = input.model?.min_width ?? 16
  const maxWidth = input.model?.max_width ?? 3840
  const minHeight = input.model?.min_height ?? 16
  const maxHeight = input.model?.max_height ?? 3840
  if (width < minWidth || width > maxWidth) return `宽度必须在 ${minWidth} 至 ${maxWidth} 像素之间。`
  if (height < minHeight || height > maxHeight) return `高度必须在 ${minHeight} 至 ${maxHeight} 像素之间。`
  if (width % 16 !== 0) return '宽度必须为 16 的倍数。'
  if (height % 16 !== 0) return '高度必须为 16 的倍数。'
  const normalized = normalizeWorkspaceCustomSize(widthText, heightText, input.model)
  return normalized.valid ? '' : '像素尺寸需满足 1:3 至 3:1 比例及平台总像素限制。'
}

export function workspaceRatioPixelEstimate(baseResolution: string, ratio: string, autoBaseResolution = '') {
  if (!baseResolution.trim() || !ratio.trim()) return ''
  const effectiveBaseResolution = baseResolution.trim().toLowerCase() === 'auto' ? autoBaseResolution : baseResolution
  if (!effectiveBaseResolution.trim()) return ''
  return calculateImageSizeForBaseResolution(effectiveBaseResolution, ratio)
}

export function workspaceOutputOptions(model?: CapabilityModelGroup) {
  return {
    quality: normalizedOptions(model?.quality, ['auto']),
    outputFormat: normalizedOptions(model?.output_format, ['png']),
    moderation: normalizedOptions(model?.moderation, ['auto']),
  }
}

export function workspaceCompressionVisible(model: CapabilityModelGroup | undefined, outputFormat: string) {
  return Boolean(model?.supports_output_compression && ['jpeg', 'webp'].includes(outputFormat.toLowerCase()))
}

export function normalizeWorkspaceOutputParameters(
  model: CapabilityModelGroup | undefined,
  current: WorkspaceOutputParameters,
): WorkspaceOutputParameters {
  const options = workspaceOutputOptions(model)
	const outputFormat = options.outputFormat.includes(current.outputFormat) ? current.outputFormat : options.outputFormat[0] ?? ''
	return {
		quality: options.quality.includes(current.quality) ? current.quality : options.quality[0] ?? '',
    outputFormat,
    outputCompression: Math.max(1, Math.min(100, Math.round(current.outputCompression) || 100)),
		moderation: options.moderation.includes(current.moderation) ? current.moderation : options.moderation[0] ?? '',
  }
}

function normalizedOptions(values: string[] | undefined, fallback: string[]) {
	if (values === undefined) return fallback
	const normalized = Array.from(new Set((values ?? []).map((value) => value.trim().toLowerCase()).filter(Boolean)))
	return normalized
}
