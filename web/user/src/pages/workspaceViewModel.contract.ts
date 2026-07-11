import type { Capability, EstimateResult, ImageResult, ImageTask, ImageTaskStatus, ImageTaskType } from '../../../shared/api-types'
import { createWorkspaceViewModel } from './workspaceViewModel'

const capability = capabilityFixture()
const baseInput = {
  capability,
  taskType: 'text_to_image' as ImageTaskType,
  selectedModelCode: 'plus',
  parametersReady: true,
  prompt: '一座雨夜中的未来城市，霓虹倒映在街道上',
  estimatePending: false,
  estimateError: '',
  estimate: estimateFixture(),
  busy: false,
  task: null,
}

const unavailable = createWorkspaceViewModel({
  ...baseInput,
  capability: { ...capability, model_groups: [], unavailable_reason: { code: 'NO_ROUTE', message: 'provider account missing' } },
  selectedModelCode: '',
  parametersReady: false,
  estimate: null,
})
if (unavailable.capability.state !== 'unavailable' || !unavailable.capability.detail.includes('配置中')) {
  throw new Error(`unavailable capability should be actionable without leaking internals, got ${JSON.stringify(unavailable.capability)}`)
}

const ready = createWorkspaceViewModel(baseInput)
if (ready.capability.state !== 'ready' || ready.generate.disabled || ready.estimate.state !== 'ready') {
  throw new Error(`ready workspace should enable generation, got ${JSON.stringify(ready)}`)
}
if (ready.parameters.models.map((item) => item.value).join(',') !== 'plus,pixel-pro') {
  throw new Error(`models must come from live task capabilities, got ${JSON.stringify(ready.parameters.models)}`)
}
if (ready.parameters.baseResolutions.join(',') !== 'auto,2K' || ready.parameters.aspectRatios.join(',') !== '1:1,16:9') {
  throw new Error(`ratio parameters must come from selected live model, got ${JSON.stringify(ready.parameters)}`)
}
if (ready.parameters.referenceLimit !== 4 || ready.parameters.counts.join(',') !== '1,2,3') {
  throw new Error(`limits and image counts must come from selected live model, got ${JSON.stringify(ready.parameters)}`)
}

const pixel = createWorkspaceViewModel({ ...baseInput, selectedModelCode: 'pixel-pro' })
if (pixel.parameters.sizeModes.join(',') !== 'pixel' || pixel.parameters.pixelSizes.join(',') !== '1024x1536,1536x1024') {
  throw new Error(`pixel sizes must come from selected live model, got ${JSON.stringify(pixel.parameters)}`)
}

const insufficient = createWorkspaceViewModel({ ...baseInput, estimate: estimateFixture({ sufficient: false, insufficient_points: '3.25000' }) })
if (insufficient.estimate.state !== 'insufficient' || !insufficient.generate.showRechargeAction || !insufficient.estimate.detail.includes('3.25')) {
  throw new Error(`insufficient balance should expose recharge guidance, got ${JSON.stringify(insufficient)}`)
}

const estimating = createWorkspaceViewModel({ ...baseInput, estimate: null, estimatePending: true })
if (estimating.estimate.state !== 'loading' || !estimating.generate.disabled) {
  throw new Error(`estimate loading should remain visible and block submit, got ${JSON.stringify(estimating)}`)
}

const estimateFailure = createWorkspaceViewModel({ ...baseInput, estimate: null, estimateError: '当前组合暂不支持' })
if (estimateFailure.estimate.state !== 'error' || estimateFailure.estimate.detail !== '当前组合暂不支持' || !estimateFailure.generate.disabled) {
  throw new Error(`estimate failure should stay local and block submit, got ${JSON.stringify(estimateFailure)}`)
}

assertTaskState('queued', 'queued', '排队中')
assertTaskState('running', 'running', '生成中')
assertTaskState('partial_failed', 'partial', '部分完成', [imageFixture('result-1')], 2)
assertTaskState('succeeded', 'success', '创作完成', [imageFixture('result-1')])
assertTaskState('failed', 'failure', '生成失败')

function assertTaskState(status: ImageTaskStatus, expectedState: string, expectedTitle: string, results: ImageResult[] = [], imageCount = 1) {
  const view = createWorkspaceViewModel({
    ...baseInput,
    task: taskFixture({ status, results, image_count: imageCount, progress: status === 'running' ? 54 : status === 'queued' ? 12 : 100 }),
  })
  if (view.task.state !== expectedState || view.task.title !== expectedTitle) {
    throw new Error(`${status} should map to ${expectedState}/${expectedTitle}, got ${JSON.stringify(view.task)}`)
  }
  if (!view.task.rail.length || (status === 'failed' && !view.task.rail.some((item) => item.status === 'failed'))) {
    throw new Error(`${status} should retain a continuous task rail, got ${JSON.stringify(view.task.rail)}`)
  }
}

function capabilityFixture(): Capability {
  return {
    task_types: ['text_to_image', 'reference_to_image', 'image_edit'],
    base_resolution: ['auto', '2K'],
    aspect_ratios: ['1:1', '16:9'],
    pixel_sizes: ['1024x1536', '1536x1024'],
    max_image_count: 4,
    reference_image_max_bytes: 8 * 1024 * 1024,
    model_groups: [
      {
        id: 'plus', code: 'plus', name: 'Plus', task_types: ['text_to_image', 'reference_to_image', 'image_edit'],
        base_resolution: ['auto', '2K'], size_modes: ['ratio'], aspect_ratios: ['1:1', '16:9'],
        max_output_image_count: 3, max_reference_image_count: 4, prices: [], supports_reference: true,
      },
      {
        id: 'pixel-pro', code: 'pixel-pro', name: 'Pixel Pro', task_types: ['text_to_image'],
        base_resolution: [], size_modes: ['pixel'], pixel_sizes: ['1024x1536', '1536x1024'],
        max_output_image_count: 2, max_reference_image_count: 2, prices: [], supports_reference: false,
      },
      {
        id: 'reference-only', code: 'reference-only', name: 'Reference', task_types: ['reference_to_image'],
        base_resolution: ['auto'], size_modes: ['ratio'], aspect_ratios: ['1:1'],
        max_output_image_count: 1, max_reference_image_count: 6, prices: [], supports_reference: true,
      },
    ],
  }
}

function estimateFixture(patch: Partial<EstimateResult> = {}): EstimateResult {
  return {
    points: patch.points ?? '2.50000',
    display_points: patch.display_points ?? '2.50',
    formula: patch.formula ?? 'plus x auto',
    sufficient: patch.sufficient ?? true,
    insufficient_points: patch.insufficient_points,
  }
}

function imageFixture(id: string): ImageResult {
  return { id, url: `/images/${id}.webp`, width: 1024, height: 1024, publish_status: 'private' }
}

function taskFixture(patch: Partial<ImageTask>): ImageTask {
  return {
    id: 'task-001', title: '测试任务', prompt: baseInput.prompt, task_type: 'text_to_image',
    status: patch.status ?? 'queued', route_model_code: 'plus', model_group: 'plus', base_resolution: '2K',
    quality: 'auto', aspect_ratio: '1:1', image_count: patch.image_count ?? 1, estimate_points: '2.50000',
    progress: patch.progress ?? 0, provider: 'openai', route: 'default', created_at: '2026-07-10T08:00:00Z',
    updated_at: '2026-07-10T08:01:00Z', reference_assets: [], results: patch.results ?? [],
    error_code: patch.status === 'failed' ? 'PROVIDER_ERROR' : undefined,
    failure_reason: patch.status === 'failed' ? 'provider unavailable' : undefined,
  }
}
