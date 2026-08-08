import type { EstimateRequest, EstimateResult } from '../../../shared/api-types'
import { currentWorkspaceEstimate, workspaceDisplayedResolvedSize, workspaceEstimateKey } from './workspaceEstimate'
import { workspaceRatioPixelEstimate } from './workspaceParameters'

for (const [baseResolution, ratio, expected] of [
  ['1K', '16:9', '1280x720'],
  ['2K', '16:9', '2560x1440'],
  ['4K', '1:1', '2880x2880'],
] as const) {
  const actual = workspaceRatioPixelEstimate(baseResolution, ratio)
  if (actual !== expected) throw new Error(`${baseResolution} ${ratio} should preview ${expected}, got ${actual}`)
}

if (workspaceRatioPixelEstimate('', '16:9') !== '' || workspaceRatioPixelEstimate('1K', '') !== '') {
  throw new Error('ratio pixel preview must remain empty until both parameters are selected')
}

if (workspaceRatioPixelEstimate('auto', '16:9', '2k') !== '2560x1440') {
  throw new Error('auto ratio preview must use the backend-resolved task bucket')
}

if (workspaceRatioPixelEstimate('auto', '16:9') !== '') {
  throw new Error('auto ratio preview must remain hidden without an authoritative task bucket')
}

const ratioPayload: EstimateRequest = {
  task_type: 'image_edit',
  route_model_code: 'plus',
  base_resolution: '2K',
  aspect_ratio: '1:1',
  image_count: 2,
  reference_asset_ids: ['ref-a'],
}

const baseKey = workspaceEstimateKey(ratioPayload)
const changes: Array<[string, EstimateRequest]> = [
  ['model', { ...ratioPayload, route_model_code: 'pro' }],
  ['resolution', { ...ratioPayload, base_resolution: '4K' }],
  ['ratio', { ...ratioPayload, aspect_ratio: '16:9' }],
  ['count', { ...ratioPayload, image_count: 3 }],
  ['reference', { ...ratioPayload, reference_asset_ids: ['ref-a', 'ref-b'] }],
]

for (const [label, payload] of changes) {
  if (workspaceEstimateKey(payload) === baseKey) {
    throw new Error(`${label} changes must invalidate the current estimate`)
  }
}

const estimate = estimateFixture()
const matching = currentWorkspaceEstimate(baseKey, { key: baseKey, estimate, error: '' })
if (matching.estimate !== estimate || matching.error || matching.pending) {
  throw new Error(`matching estimate should be current, got ${JSON.stringify(matching)}`)
}

const stale = currentWorkspaceEstimate(workspaceEstimateKey(changes[0][1]), { key: baseKey, estimate, error: '' })
if (stale.estimate !== null || stale.error || !stale.pending) {
  throw new Error(`stale estimate must immediately become pending, got ${JSON.stringify(stale)}`)
}

const tightBoundsEstimate = estimateFixture({ resolved_size: '896x896' })
const tightBoundsDisplay = workspaceDisplayedResolvedSize(
  baseKey,
  { key: baseKey, estimate: tightBoundsEstimate, error: '' },
  '1024x1024',
)
if (tightBoundsDisplay !== '896x896') {
  throw new Error(`server-resolved 1K 1:1 size must override the nominal preview, got ${tightBoundsDisplay}`)
}

const changedDraftKey = workspaceEstimateKey({ ...ratioPayload, aspect_ratio: '16:9' })
const changedDraftDisplay = workspaceDisplayedResolvedSize(
  changedDraftKey,
  { key: baseKey, estimate: tightBoundsEstimate, error: '' },
  '1280x720',
)
if (changedDraftDisplay !== '1280x720') {
  throw new Error(`a changed draft must not keep displaying the prior resolved size, got ${changedDraftDisplay}`)
}

const changedOutputKey = workspaceEstimateKey({ ...ratioPayload, output_format: 'webp', output_compression: 72 })
const changedOutput = currentWorkspaceEstimate(changedOutputKey, { key: baseKey, estimate: tightBoundsEstimate, error: '' })
if (changedOutput.estimate !== null || changedOutput.error || !changedOutput.pending) {
  throw new Error(`any changed estimate field must invalidate the prior server result, got ${JSON.stringify(changedOutput)}`)
}

const matchingError = currentWorkspaceEstimate(baseKey, { key: baseKey, estimate: null, error: '不支持当前组合' })
if (matchingError.estimate !== null || matchingError.error !== '不支持当前组合' || matchingError.pending) {
  throw new Error(`matching estimate error should block without pending, got ${JSON.stringify(matchingError)}`)
}

function estimateFixture(patch: Partial<EstimateResult> = {}): EstimateResult {
  return {
    points: '2.00000',
    formula: '2 x 1',
    display_points: '2.00',
    sufficient: true,
    insufficient_points: '0.00000',
    balance: { available_points: '10.00000', frozen_points: '0.00000', plan_name: 'free', first_purchase_bonus: false },
    ...patch,
  }
}
