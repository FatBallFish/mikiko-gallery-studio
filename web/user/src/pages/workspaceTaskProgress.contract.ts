import type { ImageResult, ImageTask } from '../../../shared/api-types'
import { generationSlots, workspaceProgressNodes, workspaceQualityLabel } from './workspaceTaskProgress'

if (workspaceQualityLabel('auto') !== '自动') {
  throw new Error(`quality auto should localize to 自动, got ${workspaceQualityLabel('auto')}`)
}

if (workspaceQualityLabel('HIGH') !== '高清') {
  throw new Error(`quality labels should be case-insensitive, got ${workspaceQualityLabel('HIGH')}`)
}

if (workspaceQualityLabel('experimental') !== 'experimental') {
  throw new Error(`unknown quality should preserve backend value, got ${workspaceQualityLabel('experimental')}`)
}

const runningNodes = workspaceProgressNodes(task({ status: 'running', progress: 40 }))
if (runningNodes.find((item) => item.phase === 'queued')?.status !== 'done') {
  throw new Error(`progress 40 should complete queue node, got ${JSON.stringify(runningNodes)}`)
}
if (runningNodes.find((item) => item.phase === 'generating')?.status !== 'active') {
  throw new Error(`progress 40 should activate generation node, got ${JSON.stringify(runningNodes)}`)
}

const succeededNodes = workspaceProgressNodes(task({ status: 'succeeded', progress: 10 }))
if (succeededNodes.some((item) => item.status !== 'done')) {
  throw new Error(`succeeded task should mark all progress nodes done, got ${JSON.stringify(succeededNodes)}`)
}

const failedNodes = workspaceProgressNodes(task({ status: 'failed', progress: 34 }))
if (!failedNodes.some((item) => item.status === 'failed')) {
  throw new Error(`failed task should mark one progress node failed, got ${JSON.stringify(failedNodes)}`)
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
    route_model_code: patch.route_model_code,
    model_group: patch.model_group ?? 'plus',
    quality: patch.quality ?? '2K',
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
