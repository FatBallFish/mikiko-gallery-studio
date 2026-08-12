import type { VideoEstimate, VideoTask } from '../../../../shared/api-types'
import { buildVideoQuoteBreakdown, buildVideoTaskAccounting } from './videoAccounting'

const quote = {
  quote_token: 'quote',
  expires_at: '2099-01-01T00:00:00Z',
  capability_version: 'cap-1',
  config_version: 'config-1',
  price_version: 'price-1',
  unit_points: '12.50000',
  estimated_points: '25.00000',
  max_reserved_points: '30.00000',
  pricing_mode: 'per_output',
  summary: { duration_seconds: 5, output_count: 2, audio_mode: 'generated' },
  balance: { available_points: '88.00000', sufficient: true },
} satisfies VideoEstimate

const quoteBreakdown = buildVideoQuoteBreakdown(quote)
if (quoteBreakdown.unitPoints !== '12.50000' || quoteBreakdown.outputCount !== 2 || quoteBreakdown.estimatedPoints !== '25.00000') {
  throw new Error(`quote breakdown must expose unit, quantity and estimated total: ${JSON.stringify(quoteBreakdown)}`)
}
if (quoteBreakdown.maxReservedPoints !== '30.00000' || quoteBreakdown.availablePoints !== '88.00000' || quoteBreakdown.pricingMode !== 'per_output') {
  throw new Error(`quote breakdown must expose reservation, balance and pricing mode: ${JSON.stringify(quoteBreakdown)}`)
}

const task = {
  id: 'task-1',
  project_id: 'project-1',
  route_model_code: 'cinema',
  task_type: 'text_to_video',
  status: 'partial',
  progress_stage: 'partial',
  prompt_template: 'camera move',
  prompt_binding_snapshot: { variables: [{ name: 'scene', value: '海边日落' }], references: [] },
  duration_seconds: 5,
  resolution: '720p',
  aspect_ratio: '16:9',
  audio_mode: 'generated',
  requested_output_count: 2,
  success_output_count: 1,
  estimated_points: '25.00000',
  reserved_points: '30.00000',
  actual_points: '12.50000',
  settlement_status: 'settled',
  pricing_snapshot: { unit_points: '12.50000', price_version: 'price-1' },
  inputs: [{ id: 'input-1', asset_id: 'asset-1', role: 'first_frame', ordinal: 0, asset_snapshot: { name: '首帧' } }],
  items: [
    { id: 'item-1', ordinal: 0, status: 'succeeded', stage: 'succeeded', result_asset_id: 'result-1', actual_output_seconds: '4.800', actual_points: '12.50000' },
    { id: 'item-2', ordinal: 1, status: 'failed', stage: 'failed', actual_output_seconds: '0.000', actual_points: '0.00000', error_code: 'PROVIDER_FAILED', error_message: '上游生成失败' },
  ],
  version: 4,
  created_at: '2026-08-12T01:00:00Z',
  started_at: '2026-08-12T01:01:00Z',
  finished_at: '2026-08-12T01:03:00Z',
} satisfies VideoTask

const accounting = buildVideoTaskAccounting(task)
if (accounting.refundPoints !== '17.50000' || accounting.actualPoints !== '12.50000' || accounting.settlementStatus !== 'settled') {
  throw new Error(`task accounting must reconcile reservation, actual charge and refund: ${JSON.stringify(accounting)}`)
}
if (accounting.timeline.length !== 3 || accounting.timeline[1]?.label !== '开始生成' || accounting.items[0]?.actualSeconds !== '4.800') {
  throw new Error(`task accounting must expose lifecycle and item usage: ${JSON.stringify(accounting)}`)
}
if (accounting.items[1]?.error !== 'PROVIDER_FAILED：上游生成失败' || accounting.inputs[0]?.name !== '首帧') {
  throw new Error(`task detail must expose input snapshots and item errors: ${JSON.stringify(accounting)}`)
}
if (accounting.variables.length !== 1 || accounting.variables[0]?.name !== 'scene' || accounting.variables[0]?.value !== '海边日落') {
  throw new Error(`task detail must expose the variables used for this generation: ${JSON.stringify(accounting.variables)}`)
}
