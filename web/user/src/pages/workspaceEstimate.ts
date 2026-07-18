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
    baseResolution: payload.base_resolution,
    aspectRatio: payload.aspect_ratio,
    imageCount: payload.image_count,
    referenceAssetIds: [...(payload.reference_asset_ids ?? [])],
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
