import type { Balance, BalanceBucket } from '../../../shared/api-types'
import { bucketExpiryText, nextExpiringCreditText, normalizeBalanceBuckets } from './profileBalanceModel'

const zeroBalance: Balance = {
  available_points: '0.00000',
  frozen_points: '0.00000',
  trial_points: '0.00000',
  subscription_points: '0.00000',
  recharge_points: '0.00000',
  plan_name: '免费计划',
  first_purchase_bonus: false,
}

const zeroBuckets = normalizeBalanceBuckets(zeroBalance)
const zeroBucketKeys = zeroBuckets.map((bucket) => bucket.bucket).join(',')

if (zeroBucketKeys !== 'subscription,recharge,gift,trial') {
  throw new Error(`profile balance buckets should always show purchased, recharge, gift and trial buckets, got ${zeroBucketKeys}`)
}

const partialBalance: Balance = {
  ...zeroBalance,
  buckets: [{ bucket: 'trial', label: '体验额度', available_points: '5.00000', expire_warning: true, expires_at: '2026-06-10T00:00:00Z' }],
}

const partialBuckets = normalizeBalanceBuckets(partialBalance)
const trial = partialBuckets.find((bucket) => bucket.bucket === 'trial')
const recharge = partialBuckets.find((bucket) => bucket.bucket === 'recharge')

if (partialBuckets.length !== 4 || trial?.available_points !== '5.00000' || recharge?.available_points !== '0.00000') {
  throw new Error('profile balance buckets should preserve server buckets and fill missing defaults')
}

const mixedExpiryBalance: Balance = {
  ...zeroBalance,
  buckets: [{ bucket: 'subscription', available_points: '335.00000', mixed_expiry: true }],
  next_expiring_grant: {
    grant_id: 0,
    grant_type: 'mixed',
    available_points: '330.00000',
    expires_at: '2026-07-05T10:05:00Z',
  },
}
const mixedExpiryBuckets = normalizeBalanceBuckets(mixedExpiryBalance)
if (mixedExpiryBuckets.some((bucket) => bucket.expires_at || bucket.expire_warning)) {
  throw new Error(`next-expiry summary must not mark an entire balance bucket as expiring, got ${JSON.stringify(mixedExpiryBuckets)}`)
}
const mixedBucket = mixedExpiryBuckets.find((bucket) => bucket.bucket === 'subscription') as BalanceBucket
if (!mixedBucket.mixed_expiry || bucketExpiryText(mixedBucket) !== '包含不同有效期积分') {
  throw new Error(`mixed expiry bucket must not claim the whole balance is permanent, got ${JSON.stringify(mixedBucket)}`)
}
if (nextExpiringCreditText(mixedExpiryBalance) !== '最近失效：330.00000 积分 · 2026/07/05 10:05') {
  throw new Error(`profile must explicitly show next-expiring amount and time, got ${nextExpiringCreditText(mixedExpiryBalance)}`)
}
