import type { ImageResult, ImageTask } from '../../../shared/api-types'
import { generationSlots, workspaceBaseResolutionLabel, workspaceProgressNodes } from './workspaceTaskProgress'

if (workspaceBaseResolutionLabel('auto') !== '自动') {
  throw new Error(`base resolution auto should localize to 自动, got ${workspaceBaseResolutionLabel('auto')}`)
}

if (workspaceBaseResolutionLabel('2k') !== '2K') {
  throw new Error(`base resolution labels should normalize case, got ${workspaceBaseResolutionLabel('2k')}`)
}

if (workspaceBaseResolutionLabel('experimental') !== 'experimental') {
  throw new Error(`unknown base resolution should preserve backend value, got ${workspaceBaseResolutionLabel('experimental')}`)
}

const runningNodes = workspaceProgressNodes(task({ status: 'running', progress: 96 }))
if (runningNodes.find((item) => item.phase === 'generating')?.status !== 'active') {
  throw new Error(`running without a backend stage should truthfully show generation regardless of legacy percentage, got ${JSON.stringify(runningNodes)}`)
}

const queuedStageNodes = workspaceProgressNodes(task({ status: 'queued', progress: 8, progress_stage: 'queued' }))
if (queuedStageNodes.find((item) => item.phase === 'queued')?.status !== 'active') {
  throw new Error(`known queued stage must override low percentage, got ${JSON.stringify(queuedStageNodes)}`)
}

const runningStageCases = [
  { stage: 'routing', phase: 'generating' },
  { stage: 'provider', phase: 'generating' },
  { stage: 'running', phase: 'generating' },
  { stage: 'persisting', phase: 'storing' },
  { stage: 'settling', phase: 'settling' },
] as const

for (const testCase of runningStageCases) {
  const nodes = workspaceProgressNodes(task({ status: 'running', progress: 8, progress_stage: testCase.stage }))
  if (nodes.find((item) => item.phase === testCase.phase)?.status !== 'active') {
    throw new Error(`backend stage ${testCase.stage} must activate ${testCase.phase}, got ${JSON.stringify(nodes)}`)
  }
}

const failedAtProvider = workspaceProgressNodes(task({ status: 'failed', progress_stage: 'provider' }))
if (failedAtProvider.find((item) => item.phase === 'generating')?.status !== 'failed') {
  throw new Error(`failure must stay at its known backend stage, got ${JSON.stringify(failedAtProvider)}`)
}

const unknownStageFallback = workspaceProgressNodes(task({ status: 'running', progress: 99, progress_stage: 'future_stage' }))
if (unknownStageFallback.find((item) => item.phase === 'generating')?.status !== 'active') {
  throw new Error(`unknown backend stages must fall back to indeterminate generation, got ${JSON.stringify(unknownStageFallback)}`)
}

const succeededNodes = workspaceProgressNodes(task({ status: 'succeeded', progress: 10 }))
if (succeededNodes.some((item) => item.status !== 'done')) {
  throw new Error(`succeeded task should mark all progress nodes done, got ${JSON.stringify(succeededNodes)}`)
}

const failedNodes = workspaceProgressNodes(task({ status: 'failed', progress: 100, progress_stage: 'failed' }))
if (failedNodes.find((item) => item.phase === 'generating')?.status !== 'failed') {
  throw new Error(`failed task without a prior stage must not infer completion from legacy percentage, got ${JSON.stringify(failedNodes)}`)
}

const partialSlots = generationSlots(task({
  status: 'partial_failed',
  image_count: 3,
  results: [image('img-1')],
  error_message: '第二张生成失败',
  error_code: 'PROVIDER_ERROR',
}))
if (partialSlots.length !== 3 || partialSlots[0].kind !== 'image' || partialSlots[1].kind !== 'failed' || partialSlots[2].kind !== 'failed') {
  throw new Error(`partial_failed task should mix image and failed slots, got ${JSON.stringify(partialSlots)}`)
}

const pendingSlots = generationSlots(task({ status: 'running', image_count: 2, results: [] }))
if (pendingSlots.some((item) => item.kind !== 'pending')) {
  throw new Error(`running task without results should show pending slots, got ${JSON.stringify(pendingSlots)}`)
}

function image(id: string): ImageResult {
  return {
    id,
    url: `/images/${id}.png`,
    width: 1024,
    height: 1024,
    publish_status: 'private',
    like_count: 0,
    favorite_count: 0,
    liked_by_viewer: false,
    favorited_by_viewer: false,
  }
}

function task(patch: Partial<ImageTask>): ImageTask {
  return {
    id: patch.id ?? 'task-progress-001',
    title: patch.title ?? '测试任务',
    prompt: patch.prompt ?? '生成一张测试图',
    task_type: patch.task_type ?? 'text_to_image',
    status: patch.status ?? 'queued',
    progress_stage: patch.progress_stage,
    route_model_code: patch.route_model_code,
    model_group: patch.model_group ?? 'plus',
    base_resolution: patch.base_resolution ?? '2K',
    quality: patch.quality ?? 'auto',
    aspect_ratio: patch.aspect_ratio ?? '1:1',
    image_count: patch.image_count ?? 1,
    estimate_points: patch.estimate_points ?? '1.00000',
    progress: patch.progress ?? 0,
    provider: patch.provider ?? 'openai',
    route: patch.route ?? 'default',
    created_at: patch.created_at ?? '2026-06-05T00:00:00Z',
    updated_at: patch.updated_at ?? '2026-06-05T00:00:00Z',
    reference_assets: patch.reference_assets ?? [],
    results: patch.results ?? [],
    failure_reason: patch.failure_reason,
    error_message: patch.error_message,
    error_code: patch.error_code,
  }
}
