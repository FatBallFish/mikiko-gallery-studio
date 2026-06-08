import type { Balance, BalanceBucket, Capability, ImageResult, ImageTaskType } from '../../../shared/api-types'

function taskTypeLabel(type: ImageTaskType | string) {
  const labels: Record<string, string> = {
    text_to_image: '文生图',
    reference_to_image: '参考生图',
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
    meta: `${taskTypeLabel(image.task_type ?? 'text_to_image')} · ${image.route_model_code || image.abstract_model || '-'} · ${image.quality || '-'} · ${formatDateTime(image.created_at)}`,
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
