import type { ImageResult, ImageTask } from '../../../shared/api-types'

export type WorkspaceProgressPhase = 'validating' | 'routing' | 'queued' | 'generating' | 'storing' | 'settling'

export type WorkspaceProgressNode = {
  phase: WorkspaceProgressPhase
  label: string
  status: 'idle' | 'active' | 'done' | 'failed'
}

export type GenerationSlot =
  | { kind: 'image'; index: number; image: ImageResult }
  | { kind: 'pending'; index: number; label: string }
  | { kind: 'failed'; index: number; title: string; reason: string; code?: string }

const baseResolutionLabelMap: Record<string, string> = {
  auto: '自动',
  '1k': '1K',
  '2k': '2K',
  '4k': '4K',
  standard: '标准',
  low: '标准',
  medium: '高清',
  high: '高清',
  ultra: '超清',
  hd: '高清',
}

const progressContract: Array<{ phase: WorkspaceProgressPhase; label: string }> = [
  { phase: 'validating', label: '参数校验' },
  { phase: 'routing', label: '模型路由' },
  { phase: 'queued', label: '队列调度' },
  { phase: 'generating', label: '图像生成' },
  { phase: 'storing', label: '结果入库' },
  { phase: 'settling', label: '积分结算' },
]

const backendStagePhase: Record<string, WorkspaceProgressPhase> = {
  queued: 'queued',
  routing: 'generating',
  provider: 'generating',
  running: 'generating',
  persisting: 'storing',
  settling: 'settling',
  completed: 'settling',
  failed: 'generating',
}

export function workspaceBaseResolutionLabel(value: string) {
  return baseResolutionLabelMap[value.toLowerCase()] ?? value
}

export function workspaceProgressNodes(task: ImageTask): WorkspaceProgressNode[] {
  const failed = ['failed', 'cancelled', 'rejected', 'deleted'].includes(task.status)
  const succeeded = ['succeeded', 'partial_failed'].includes(task.status)
  const stagePhase = backendStagePhase[(task.progress_stage ?? '').trim().toLowerCase()]
  const fallbackPhase: WorkspaceProgressPhase = task.status === 'queued' ? 'queued' : 'generating'
  const currentPhase = stagePhase ?? fallbackPhase
  const currentIndex = Math.max(0, progressContract.findIndex((item) => item.phase === currentPhase))
  return progressContract.map((item, index) => {
    if (failed) {
      if (index < currentIndex) return { phase: item.phase, label: item.label, status: 'done' }
      return { phase: item.phase, label: item.label, status: index === currentIndex ? 'failed' : 'idle' }
    }
    if (succeeded) return { phase: item.phase, label: item.label, status: 'done' }
    if (index < currentIndex) return { phase: item.phase, label: item.label, status: 'done' }
    if (index === currentIndex) return { phase: item.phase, label: item.label, status: 'active' }
    return { phase: item.phase, label: item.label, status: 'idle' }
  })
}

export function generationSlots(task: ImageTask): GenerationSlot[] {
  const requested = Math.max(Number(task.image_count || 1), task.results.length || 0)
  const terminal = ['succeeded', 'partial_failed', 'failed', 'cancelled', 'rejected', 'deleted'].includes(task.status)
  const slots: GenerationSlot[] = []
  for (let index = 0; index < requested; index += 1) {
    const image = task.results[index]
    if (image) {
      slots.push({ kind: 'image', index, image })
      continue
    }
    if (terminal) {
      slots.push({
        kind: 'failed',
        index,
        title: '生成失败',
        reason: task.error_message || task.failure_reason || '该图片未生成成功',
        code: task.error_code,
      })
      continue
    }
    slots.push({ kind: 'pending', index, label: '生成中' })
  }
  return slots
}
