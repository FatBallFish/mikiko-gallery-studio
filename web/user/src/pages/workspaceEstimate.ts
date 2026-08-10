import type { EstimateRequest, EstimateResult } from '../../../shared/api-types'

export type WorkspaceEstimateSnapshot = {
  key: string
  estimate: EstimateResult | null
  error: string
}

export function workspaceEstimateKey(payload: EstimateRequest) {
  return JSON.stringify({
    taskType: payload.task_type,
    model: payload.route_model_code,
    sizeMode: payload.size_mode,
    baseResolution: payload.base_resolution,
    aspectRatio: payload.aspect_ratio,
    pixelSize: payload.pixel_size,
    quality: payload.quality,
    outputFormat: payload.output_format,
    background: payload.background,
    outputCompression: payload.output_compression,
    moderation: payload.moderation,
    imageCount: payload.image_count,
    referenceAssetIds: [...(payload.reference_asset_ids ?? [])],
    modelGroup: payload.model_group,
  })
}

export function currentWorkspaceEstimate(currentKey: string, snapshot: WorkspaceEstimateSnapshot) {
  if (snapshot.key !== currentKey) {
    return { estimate: null, error: '', pending: true }
  }
  return {
    estimate: snapshot.estimate,
    error: snapshot.error,
    pending: !snapshot.estimate && !snapshot.error,
  }
}

export function workspaceDisplayedResolvedSize(
  currentKey: string,
  snapshot: WorkspaceEstimateSnapshot,
  localFallback: string,
) {
  const current = currentWorkspaceEstimate(currentKey, snapshot)
  return current.estimate?.resolved_size?.trim() || localFallback
}
