import type { ImageResult, ImageTaskType } from '../../../shared/api-types'

function taskTypeLabel(type: ImageTaskType | string) {
  const labels: Record<string, string> = {
    text_to_image: '文生图',
    reference_to_image: '参考生图',
    image_edit: '图片编辑',
  }
  return labels[type] ?? type
}

function publishLabel(status?: string) {
  if (status === 'public' || status === 'approved') return '已公开'
  if (status === 'reviewing' || status === 'pending_review') return '审核中'
  if (status === 'rejected') return '已拒绝'
  if (status === 'unpublished') return '已下架'
  return '私有'
}

function formatDate(date?: string) {
  const input = date ?? ''
  const match = input.match(/^(\d{4})-(\d{2})-(\d{2})/)
  if (!match) return input || '-'
  return `${match[1]}/${match[2]}/${match[3]}`
}

export function publicGalleryCardView(image: ImageResult) {
  const model = image.route_model_code || image.abstract_model || '-'
  return {
    title: image.prompt_excerpt || '登录后查看完整提示词',
    taskType: taskTypeLabel(image.task_type ?? 'text_to_image'),
    model,
    quality: image.quality || '-',
    aspectRatio: image.aspect_ratio || '-',
    author: image.author_name || '匿名用户',
    date: formatDate(image.created_at),
    status: publishLabel(image.publish_status),
  }
}

export function publicGallerySearchText(image: ImageResult) {
  const model = image.route_model_code || image.abstract_model || ''
  return `${image.id} ${image.prompt_excerpt ?? ''} ${model} ${image.author_name ?? ''}`.toLowerCase()
}
