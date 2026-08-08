import type { Balance } from '../../../shared/api-types'
import { normalizeBalanceBuckets } from './profileBalanceModel'

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
