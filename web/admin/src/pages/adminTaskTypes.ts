import type { ImageTaskType } from '../../../shared/api-types'

export const adminTaskTypeOptions = [
  { value: 'text_to_image', label: '文生图' },
  { value: 'reference_to_image', label: '参考生图' },
  { value: 'image_edit', label: '图片编辑' },
] as const satisfies ReadonlyArray<{ value: ImageTaskType; label: string }>

const taskTypeLabels: Record<string, string> = {
  text_to_image: '文生图',
  reference_to_image: '参考生图',
  reference_generate: '参考生图',
  reference_image: '参考生图',
  image_edit: '图片编辑',
  image_to_image: '图片编辑',
}

export function adminTaskTypeLabel(type?: string | null) {
  const normalized = normalizeTaskType(type)
  return taskTypeLabels[normalized] ?? taskTypeLabelFallback(type)
}

function normalizeTaskType(type?: string | null) {
  return (type ?? '').trim().toLowerCase()
}

function taskTypeLabelFallback(type?: string | null) {
  const normalized = (type ?? '').trim()
  return normalized || '未知类型'
}
