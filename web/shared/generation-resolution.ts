import type { EstimateRequest } from './api-types'
import { calculateImageSizeForQuality } from './image-size'

export type GenerationResolutionInput = Pick<EstimateRequest, 'base_resolution' | 'quality' | 'aspect_ratio'>

export function resolveGenerationResolution(input: GenerationResolutionInput) {
  const requestedQuality = String(input.base_resolution?.trim() || input.quality?.trim() || 'auto').toLowerCase()
  return {
    requested_quality: requestedQuality,
    requested_size: calculateImageSizeForQuality(requestedQuality, input.aspect_ratio ?? '1:1'),
  }
}
