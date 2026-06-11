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

const qualityLabelMap: Record<string, string> = {
  auto: '自动',
  standard: '标准',
  low: '标准',
  medium: '高清',
  high: '高清',
  ultra: '超清',
  hd: '高清',
}

const progressContract: Array<{ phase: WorkspaceProgressPhase; label: string; threshold: number }> = [
  { phase: 'validating', label: '参数校验', threshold: 5 },
  { phase: 'routing', label: '模型路由', threshold: 18 },
  { phase: 'queued', label: '队列调度', threshold: 30 },
  { phase: 'generating', label: '图像生成', threshold: 78 },
  { phase: 'storing', label: '结果入库', threshold: 92 },
  { phase: 'settling', label: '积分结算', threshold: 100 },
]

export function workspaceQualityLabel(value: string) {
  return qualityLabelMap[value.toLowerCase()] ?? value
}

export function workspaceProgressNodes(task: ImageTask): WorkspaceProgressNode[] {
  const failed = ['failed', 'cancelled', 'rejected', 'deleted'].includes(task.status)
  const succeeded = ['succeeded', 'partial_failed'].includes(task.status)
  const progress = succeeded ? 100 : Math.max(0, Math.min(100, Number(task.progress ?? 0)))
  let activeIndex = -1
  for (let index = progressContract.length - 1; index >= 0; index -= 1) {
    if (progress >= progressContract[index].threshold) {
      activeIndex = index
      break
    }
  }
  return progressContract.map((item, index) => {
    if (failed && index >= Math.max(0, activeIndex)) return { phase: item.phase, label: item.label, status: index === Math.max(0, activeIndex) ? 'failed' : 'idle' }
    if (succeeded || progress >= item.threshold) return { phase: item.phase, label: item.label, status: 'done' }
    if (index === activeIndex + 1) return { phase: item.phase, label: item.label, status: 'active' }
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
