import type { GalleryImage, ImageTaskStatus, ImageTaskType, PublishStatus } from '../../../shared/api-types'

export type GalleryImageFilter = {
  type: 'all' | ImageTaskType | 'api'
  status: 'all' | ImageTaskStatus | string
  publishStatus: 'all' | PublishStatus | string
  imageGroup: string
  query: string
}

export type AssetCardModel = {
  id: string
  imageUrl: string
  title: string
  prompt: string
  modelLabel: string
  ratioLabel: string
  modelLine: string
  groupLabel: string
  publishLabel: string
  createdAtLabel: string
  assetPath: string
  canPreview: boolean
  canDownload: boolean
	canEdit: boolean
	canPublish: boolean
	publishAction: 'request' | 'cancel' | null
	publishActionLabel: string
}
export type GalleryImageCardModel = AssetCardModel

export type GalleryPublishActionPresentation = {
  action: 'request' | 'cancel' | null
  label: string
  icon: 'publish' | 'withdraw' | 'unpublish'
  tone: 'positive' | 'warning' | 'danger'
}

const publishLabels: Record<string, string> = {
  private: '私有',
  public: '已公开',
  approved: '已公开',
  reviewing: '审核中',
  pending_review: '审核中',
  rejected: '已拒绝',
  unpublished: '已下架',
}

const taskTypeLabels: Record<string, string> = {
  text_to_image: '文生图',
  image_edit: '图片编辑',
}

export function galleryImageCard(image: GalleryImage): GalleryImageCardModel {
  const assetPath = image.url || image.download_url || ''
  const publishStatus = galleryPublishStatus(image)
  const hasAsset = Boolean(assetPath)
	const publishAction = galleryPublishAction(publishStatus, hasAsset)
  const modelLabel = image.route_model_code || image.abstract_model || '-'
  return {
    id: image.id,
    imageUrl: assetPath,
    title: image.prompt || image.id,
    prompt: image.prompt || '',
    modelLabel,
    ratioLabel: image.aspect_ratio || '-',
    modelLine: `${galleryTaskTypeLabel(image.task_type ?? 'text_to_image')} · ${modelLabel}`,
    groupLabel: image.image_group?.trim() || '未分组',
    publishLabel: galleryPublishLabel(image.visibility_status),
    createdAtLabel: galleryDateTime(image.created_at),
    assetPath,
    canPreview: hasAsset,
    canDownload: hasAsset,
    canEdit: hasAsset,
		canPublish: publishAction !== null,
		publishAction,
    publishActionLabel: galleryPublishActionLabel(publishStatus, hasAsset),
  }
}

export function patchGalleryItems<T extends { id: string }>(items: T[], patches: T[]) {
  if (!patches.length) return items
  const patchesByID = new Map(patches.map((patch) => [patch.id, patch]))
  let changed = false
  const next = items.map((item) => {
    const patch = patchesByID.get(item.id)
    if (!patch) return item
    changed = true
    return { ...item, ...patch }
  })
  return changed ? next : items
}

export function removeGalleryItems<T extends { id: string }>(items: T[], removedIDs: ReadonlySet<string>) {
  if (!removedIDs.size) return items
  const next = items.filter((item) => !removedIDs.has(item.id))
  return next.length === items.length ? items : next
}

export function galleryPublishLabel(status?: string | null) {
  const normalized = normalize(status)
  return publishLabels[normalized] ?? (normalized || '私有')
}

export function galleryPublishStatus(image: Pick<GalleryImage, 'visibility_status'>) {
  const normalized = normalize(image.visibility_status)
  if (normalized === 'public') return 'approved'
  if (normalized === 'reviewing') return 'pending_review'
  return normalized || 'private'
}

export function galleryPublishMatches(rowStatus: string | undefined, filterStatus: string) {
  if (filterStatus === 'all') return true
  const normalizedRow = normalizePublishValue(rowStatus)
  const normalizedFilter = normalizePublishValue(filterStatus)
  return normalizedRow === normalizedFilter
}

export function galleryImageSearchText(image: GalleryImage) {
  const model = image.route_model_code || image.abstract_model || ''
  const group = image.image_group?.trim() || '未分组'
  const taskType = image.task_type ?? ''
  const publishStatus = galleryPublishStatus(image)
  return [
    image.id,
    image.prompt,
    model,
    group,
    taskType,
    galleryTaskTypeLabel(taskType),
    image.task_status,
    image.visibility_status,
    publishStatus,
    galleryPublishLabel(image.visibility_status),
  ].filter(Boolean).join(' ').toLowerCase()
}

export function filterGalleryImages(rows: GalleryImage[], filter: GalleryImageFilter) {
  const terms = filter.query.trim().toLowerCase().split(/\s+/).filter(Boolean)
  return rows.filter((image) => {
    const group = image.image_group?.trim() || ''
    const matchesType = filter.type === 'all' || (filter.type === 'api' ? false : image.task_type === filter.type)
    const matchesStatus = filter.status === 'all' || image.task_status === filter.status
    const matchesPublishStatus = galleryPublishMatches(image.visibility_status, filter.publishStatus)
    const matchesGroup = filter.imageGroup === 'all' || (filter.imageGroup === 'ungrouped' ? !group : group === filter.imageGroup)
    const matchesQuery = !terms.length || terms.every((term) => galleryImageSearchText(image).includes(term))
    return matchesType && matchesStatus && matchesPublishStatus && matchesGroup && matchesQuery
  })
}

function normalizePublishValue(value?: string | null) {
  const normalized = normalize(value)
  if (normalized === 'public') return 'approved'
  if (normalized === 'reviewing') return 'pending_review'
  return normalized || 'private'
}

function galleryTaskTypeLabel(taskType: string) {
  return taskTypeLabels[taskType] ?? taskType
}

function galleryPublishActionLabel(publishStatus: string, hasAsset: boolean) {
  return galleryPublishActionPresentation(publishStatus, hasAsset).label
}

function galleryPublishAction(publishStatus: string, hasAsset: boolean): 'request' | 'cancel' | null {
  if (publishStatus === 'approved' || publishStatus === 'pending_review') return 'cancel'
  if (hasAsset && ['private', 'rejected', 'unpublished'].includes(publishStatus)) return 'request'
  return null
}

export function galleryPublishActionPresentation(publishStatus: string, hasAsset: boolean): GalleryPublishActionPresentation {
  const normalized = normalizePublishValue(publishStatus)
  const action = galleryPublishAction(normalized, hasAsset)
  if (normalized === 'approved') {
    return { action, label: '取消公开', icon: 'unpublish', tone: 'danger' }
  }
  if (normalized === 'pending_review') {
    return { action, label: '取消申请', icon: 'withdraw', tone: 'warning' }
  }
  return {
    action,
    label: action ? (normalized === 'rejected' || normalized === 'unpublished' ? '重新申请' : '申请公开') : '无图片文件',
    icon: 'publish',
    tone: 'positive',
  }
}

function galleryDateTime(value?: string) {
  const input = value ?? ''
  const match = input.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!match) return input || '-'
  return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
}

function normalize(value?: string | null) {
  return (value ?? '').trim()
}
