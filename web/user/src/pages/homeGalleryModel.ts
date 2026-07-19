import type { Balance, BalanceBucket, Capability, ImageResult, ImageTask, ImageTaskType } from '../../../shared/api-types'

export type HomeContinuationView = {
  action: 'create' | 'continue' | 'retry' | 'inspect'
  route: 'genpic'
  label: string
  taskId?: string
}

export type HomeRecentTaskView = {
  state: 'loading' | 'empty' | 'active' | 'complete' | 'failed'
  tone: 'neutral' | 'success' | 'warning' | 'error'
  title: string
  detail: string
  action: 'none' | 'create' | 'continue' | 'retry' | 'inspect'
}

export function homeContinuationView(tasks: ImageTask[]): HomeContinuationView {
  const latest = newestTask(tasks)
  if (!latest) return { action: 'create', route: 'genpic', label: '开始第一次创作' }
  if (isFailedTask(latest.status)) return { action: 'retry', route: 'genpic', label: '重试最近任务', taskId: latest.id }
  if (latest.status === 'queued' || latest.status === 'running') return { action: 'continue', route: 'genpic', label: '继续查看进度', taskId: latest.id }
  return { action: 'inspect', route: 'genpic', label: '继续创作', taskId: latest.id }
}

export function homeRecentTaskView(task: ImageTask | null, loading: boolean): HomeRecentTaskView {
  if (loading) return { state: 'loading', tone: 'neutral', title: '正在读取最近任务', detail: '进度与结果会在这里同步。', action: 'none' }
  if (!task) return { state: 'empty', tone: 'neutral', title: '还没有生成记录', detail: '从一段提示词开始你的第一张作品。', action: 'create' }
  if (isFailedTask(task.status)) {
    return { state: 'failed', tone: 'error', title: task.title || '最近任务未完成', detail: task.failure_reason || '本次未产生可用图片，不会按成功结果扣费。', action: 'retry' }
  }
  if (task.status === 'queued' || task.status === 'running') {
    const progress = Math.max(0, Math.min(100, Math.round(task.progress || 0)))
    return { state: 'active', tone: 'warning', title: task.title || '正在生成', detail: `${task.status === 'queued' ? '已进入队列' : '正在生成'} · ${progress}%`, action: 'continue' }
  }
  return { state: 'complete', tone: 'success', title: task.title || '最近作品已完成', detail: `${taskTypeLabel(task.task_type)} · ${task.route_model_code || task.abstract_model || task.model_group || '-'}`, action: 'inspect' }
}

export function curatedHomeGallery(images: ImageResult[], limit = 6) {
  const boundedLimit = Math.max(0, Math.min(6, Math.floor(limit)))
  const seen = new Set<string>()
  return images.filter((image) => {
    if (!image.id || seen.has(image.id)) return false
    seen.add(image.id)
    return true
  }).slice(0, boundedLimit)
}

export function newestHomeTask(tasks: ImageTask[]) {
  return newestTask(tasks)
}

function newestTask(tasks: ImageTask[]) {
  return [...tasks].sort((left, right) => dateValue(right.created_at) - dateValue(left.created_at))[0] ?? null
}

function dateValue(value?: string) {
  const parsed = Date.parse(value ?? '')
  return Number.isFinite(parsed) ? parsed : 0
}

function isFailedTask(status: ImageTask['status']) {
  return status === 'failed' || status === 'rejected' || status === 'cancelled'
}

function taskTypeLabel(type: ImageTaskType | string) {
  const labels: Record<string, string> = {
    text_to_image: '文生图',
    image_edit: '图片编辑',
  }
  return labels[type] ?? type
}

function formatDateTime(date?: string) {
  const input = date ?? ''
  const match = input.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!match) return input || '-'
  return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
}

export function homeGalleryCardView(image: ImageResult) {
  return {
    title: image.prompt_excerpt || '公开作品',
    meta: `${taskTypeLabel(image.task_type ?? 'text_to_image')} · ${image.route_model_code || image.abstract_model || '-'} · ${image.base_resolution || image.quality || '-'} · ${formatDateTime(image.created_at)}`,
  }
}

export function homeModelReadinessView(capability: Capability | null, loading: boolean) {
  if (loading) {
    return {
      ready: false,
      value: '检测中',
      detail: '正在检查平台生图能力。',
      warning: false,
    }
  }
  const modelCount = capability?.model_groups?.length ?? 0
  if (modelCount > 0) {
    return {
      ready: true,
      value: `${modelCount} 个模型可用`,
      detail: '文生图/参考图按配置开放',
      warning: false,
    }
  }
  return {
    ready: false,
    value: '平台模型配置中',
    detail: '当前没有可用生图模型，请稍后再试。',
    warning: true,
  }
}

export type HomeAccountReadinessView = {
  availableValue: string
  trialValue: string
  trialDetail: string
  trialWarning: boolean
  secondaryAction: 'gallery' | 'recharge'
}

export function homeAccountReadinessView(balance: Balance | null): HomeAccountReadinessView {
  const available = balance?.available_points ?? '0.00000'
  const trial = homeTrialBucket(balance)
  return {
    availableValue: `${displayPoints(available)} ◈`,
    trialValue: trial ? `${displayPoints(trial.available_points)} ◈` : '暂无',
    trialDetail: trial ? homeBucketExpiryText(trial) : '可通过活动或兑换码获得',
    trialWarning: Boolean(trial?.expire_warning),
    secondaryAction: positivePoints(available) ? 'gallery' : 'recharge',
  }
}

export function homePublicGalleryAccess(token: string | null | undefined, imageId: string) {
  if (!token) return { action: 'login' as const, returnTo: 'home' as const, imageId }
  return { action: 'detail' as const, imageId }
}

function homeTrialBucket(balance: Balance | null): BalanceBucket | null {
  const fromBuckets = balance?.buckets?.find((bucket) => bucket.bucket === 'trial' && positivePoints(bucket.available_points))
  if (fromBuckets) return fromBuckets
  if (!positivePoints(balance?.trial_points)) return null
  return {
    bucket: 'trial',
    label: '体验额度',
    available_points: balance?.trial_points ?? '0.00000',
    expires_at: balance?.next_expiring_grant?.grant_type === 'trial' ? balance.next_expiring_grant.expires_at : undefined,
    expire_warning: Boolean(balance?.next_expiring_grant?.grant_type === 'trial'),
  }
}

function positivePoints(value?: string) {
  return Number.parseFloat(value ?? '0') > 0
}

function displayPoints(value?: string) {
  const parsed = Number(value ?? '0')
  if (!Number.isFinite(parsed)) return value ?? '0.00000'
  return parsed.toFixed(2)
}

function homeBucketExpiryText(bucket: BalanceBucket) {
  if (!bucket.expires_at) return '长期有效'
  const input = bucket.expires_at
  const match = input.match(/^(\d{4})-(\d{2})-(\d{2})/)
  const formatted = match ? `${match[1]}/${match[2]}/${match[3]}` : input
  return bucket.expire_warning ? `即将过期：${formatted}` : `有效期至 ${formatted}`
}
