import type { ReviewItem } from '../../../shared/api-types'
import { adminTaskTypeLabel } from './adminTaskTypes'

export type ReviewDecision = 'approve' | 'reject' | 'unpublish'

export type ReviewListFilters = {
  user: string
  prompt: string
  model: string
  taskType: string
  baseResolution: string
  requestedSize: string
  width: string
  height: string
  aspectRatio: string
  createdFrom: string
  createdTo: string
  publishedFrom: string
  publishedTo: string
}

export function reviewListQuery(filters: ReviewListFilters, status: string, page: number, pageSize: number): Record<string, string | number | undefined> {
  const positiveInt = (value: string) => {
    const parsed = Number(value)
    return Number.isInteger(parsed) && parsed > 0 ? parsed : undefined
  }
  const optional = (value: string) => value.trim() || undefined
  const dateBoundary = (value: string, endOfDay: boolean) => {
    const normalized = value.trim()
    if (!normalized) return undefined
    if (/^\d{4}-\d{2}-\d{2}$/.test(normalized)) return `${normalized}T${endOfDay ? '23:59:59' : '00:00:00'}Z`
    return normalized
  }
  return {
    page,
    page_size: pageSize,
    status: status === 'all' ? undefined : status,
    user: optional(filters.user),
    prompt: optional(filters.prompt),
    model: optional(filters.model),
    task_type: optional(filters.taskType),
    base_resolution: optional(filters.baseResolution),
    requested_size: optional(filters.requestedSize),
    width: positiveInt(filters.width),
    height: positiveInt(filters.height),
    aspect_ratio: optional(filters.aspectRatio),
    created_from: dateBoundary(filters.createdFrom, false),
    created_to: dateBoundary(filters.createdTo, true),
    published_from: dateBoundary(filters.publishedFrom, false),
    published_to: dateBoundary(filters.publishedTo, true),
  }
}

export type ReviewActionModel = {
  decision: ReviewDecision
  label: string
  tone: 'primary' | 'danger'
}

export type ReviewRowView = {
  raw: ReviewItem
  imageID: string
  imageURL: string
  title: string
  owner: string
  context: string
  taskTypeLabel: string
  createdAtLabel: string
  statusLabel: string
  statusTone: 'success' | 'warning' | 'danger' | 'neutral'
  actions: ReviewActionModel[]
  terminalActionLabel: string
}

export const reviewStatusTabs = ['pending_review', 'approved', 'rejected', 'unpublished', 'all'] as const

export function reviewStatusLabel(status: string) {
  if (status === 'private') return '待申请'
  if (status === 'pending' || status === 'pending_review') return '待审核'
  if (status === 'approved' || status === 'public') return '已通过'
  if (status === 'rejected') return '已驳回'
  if (status === 'unpublished') return '已下架'
  if (status === 'reviewing') return '审核中'
  return status || '未知状态'
}

export function reviewStatusTone(status: string): 'success' | 'warning' | 'danger' | 'neutral' {
  if (status === 'approved' || status === 'public') return 'success'
  if (status === 'pending' || status === 'pending_review' || status === 'reviewing') return 'warning'
  if (status === 'rejected' || status === 'unpublished') return 'danger'
  return 'neutral'
}

export function reviewActionsForStatus(status: string): ReviewActionModel[] {
  if (status === 'pending' || status === 'pending_review') {
    return [
      { decision: 'approve', label: '通过', tone: 'primary' },
      { decision: 'reject', label: '驳回', tone: 'danger' },
    ]
  }
  if (status === 'approved' || status === 'public') {
    return [{ decision: 'unpublish', label: '下架', tone: 'danger' }]
  }
  return []
}

export function reviewTerminalActionLabel(item: Pick<ReviewItem, 'status'>) {
  if (reviewActionsForStatus(item.status).length > 0) return ''
  if (item.status === 'rejected') return '已驳回'
  if (item.status === 'unpublished') return '已下架'
  if (item.status === 'private') return '未申请公开'
  return '无可用操作'
}

export function reviewRowView(item: ReviewItem): ReviewRowView {
  const actions = reviewActionsForStatus(item.status)
  return {
    raw: item,
    imageID: item.image_id ?? item.id,
    imageURL: item.image_url ?? '',
    title: item.title || item.id,
    owner: item.owner || '-',
    context: item.reason || item.review_reason || '-',
    taskTypeLabel: adminTaskTypeLabel(item.task_type),
    createdAtLabel: reviewDateTime(item.created_at),
    statusLabel: reviewStatusLabel(item.status),
    statusTone: reviewStatusTone(item.status),
    actions,
    terminalActionLabel: actions.length ? '' : reviewTerminalActionLabel(item),
  }
}

export function reviewDefaultReason(decision: ReviewDecision) {
  if (decision === 'approve') return '内容质量稳定，准许公开展示。'
  if (decision === 'reject') return '内容不符合公开展示规范。'
  return '运营复核后下架公开展示。'
}

function reviewDateTime(value?: string) {
  const input = value ?? ''
  const match = input.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!match) return input || '-'
  return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
}
