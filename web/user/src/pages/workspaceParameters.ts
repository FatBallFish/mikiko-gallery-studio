import type { CapabilityModelGroup, ImageTaskType } from '../../../shared/api-types'
import { calculateImageSizeForBaseResolution, normalizeCustomImageSize, type CustomImageSizeNormalization } from '../../../shared/image-size'

export type WorkspaceOutputParameters = {
  quality: string
  outputFormat: string
  outputCompression: number
  moderation: string
}

export function normalizeWorkspaceCustomSize(width: string, height: string): CustomImageSizeNormalization {
  if (!/^\d+$/.test(width.trim()) || !/^\d+$/.test(height.trim())) {
    return normalizeCustomImageSize(Number.NaN, Number.NaN)
  }
  return normalizeCustomImageSize(Number(width), Number(height))
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
    moderation: scoped.moderation ?? model.moderation,
    max_output_image_count: scoped.max_output_image_count ?? model.max_output_image_count,
    max_reference_image_count: scoped.max_reference_image_count ?? model.max_reference_image_count,
  } satisfies CapabilityModelGroup
}

export function workspaceCustomSizeSupported(model: CapabilityModelGroup | undefined) {
  if (!model) return false
  return Boolean(model.supports_custom_size)
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
  const outputFormat = options.outputFormat.includes(current.outputFormat) ? current.outputFormat : options.outputFormat[0]
  return {
    quality: options.quality.includes(current.quality) ? current.quality : options.quality[0],
    outputFormat,
    outputCompression: Math.max(1, Math.min(100, Math.round(current.outputCompression) || 100)),
    moderation: options.moderation.includes(current.moderation) ? current.moderation : options.moderation[0],
  }
}

function normalizedOptions(values: string[] | undefined, fallback: string[]) {
  const normalized = Array.from(new Set((values ?? []).map((value) => value.trim().toLowerCase()).filter(Boolean)))
  return normalized.length ? normalized : fallback
}
