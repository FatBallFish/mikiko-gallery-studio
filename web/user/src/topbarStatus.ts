import type { Balance, BalanceBucket } from '../../shared/api-types'

export type TopbarStatusChip = {
  label: string
  value: string
  detail?: string
  tone?: 'neutral' | 'warning'
}

export function topbarStatusChips(balance: Balance | null | undefined): TopbarStatusChip[] {
  if (!balance) return [{ label: '账户状态', value: '读取中' }]

  const trial = bucketPoints(balance, 'trial') ?? balance.trial_points
  const recharge = bucketPoints(balance, 'recharge') ?? balance.recharge_points
  const chips: TopbarStatusChip[] = []

  if (positivePoints(trial)) {
    const trialBucket = findBucket(balance, 'trial')
    const expiryText = expiryDetail(trialBucket)
    chips.push({
      label: '体验额度',
      value: displayPoints(trial),
      detail: expiryText,
      tone: trialBucket?.expire_warning ? 'warning' : 'neutral',
    })
  }

  if (positivePoints(recharge)) {
    chips.push({ label: '充值余额', value: displayPoints(recharge), detail: '长期有效' })
  }

  if (!chips.length) {
    chips.push({ label: '可用积分', value: displayPoints(balance.available_points) })
  }

  return chips.slice(0, 2)
}

function findBucket(balance: Balance, bucket: string): BalanceBucket | undefined {
  return balance.buckets?.find((item) => item.bucket === bucket)
}

function bucketPoints(balance: Balance, bucket: string) {
  return findBucket(balance, bucket)?.available_points
}

function positivePoints(value?: string) {
  return Number.parseFloat(value ?? '0') > 0
}

function displayPoints(value?: string) {
  const parsed = Number(value ?? '0')
  if (!Number.isFinite(parsed)) return value ?? '0.00000'
  return parsed.toFixed(2)
}

function expiryDetail(bucket?: BalanceBucket) {
  if (!bucket?.expires_at) return undefined
  const match = bucket.expires_at.match(/^(\d{4})-(\d{2})-(\d{2})/)
  const formatted = match ? `${match[1]}/${match[2]}/${match[3]}` : bucket.expires_at
  return bucket.expire_warning ? `即将过期 ${formatted}` : `有效期至 ${formatted}`
}
