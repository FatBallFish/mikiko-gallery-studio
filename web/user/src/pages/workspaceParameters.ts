import type { CapabilityModelGroup } from '../../../shared/api-types'
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
