import type { Balance } from '../../shared/api-types'
import { topbarStatusChips } from './topbarStatus'

const balance: Balance = {
  available_points: '123.45600',
  frozen_points: '2.00000',
  trial_points: '18.00000',
  subscription_points: '0.00000',
  recharge_points: '100.00000',
  buckets: [
    { bucket: 'trial', available_points: '18.00000', expires_at: '2026-06-12T00:00:00Z', expire_warning: true },
    { bucket: 'recharge', available_points: '100.00000' },
  ],
  plan_name: 'FREE',
  first_purchase_bonus: false,
}

const chips = topbarStatusChips(balance)
const labels = chips.map((chip) => chip.label)
const values = chips.map((chip) => chip.value)
const visibleCopy = chips.map((chip) => `${chip.label}${chip.value}${chip.detail ?? ''}`).join(' ')

if (labels.includes('消息') || labels.includes('活动') || values.includes('3') || values.includes('2')) {
  throw new Error(`topbar should not expose fake message/activity counters, got ${JSON.stringify(chips)}`)
}

if (!chips.some((chip) => chip.label === '体验额度' && chip.value === '18.00' && chip.detail === '即将过期 2026/06/12')) {
  throw new Error(`topbar should show real trial balance and expiry warning, got ${JSON.stringify(chips)}`)
}

if (!chips.some((chip) => chip.label === '充值余额' && chip.value === '100.00')) {
  throw new Error(`topbar should show real recharge balance, got ${JSON.stringify(chips)}`)
}

if (/undefined|NaN|mock|fake/i.test(visibleCopy)) {
  throw new Error(`topbar status chips should not leak invalid or fake values, got ${visibleCopy}`)
}

const emptyChips = topbarStatusChips(null)
if (emptyChips.length !== 1 || emptyChips[0]?.label !== '账户状态' || emptyChips[0]?.value !== '读取中') {
  throw new Error(`topbar should show loading account state when balance is absent, got ${JSON.stringify(emptyChips)}`)
}

const invalidExpiry = topbarStatusChips({
  ...balance,
  buckets: [{ bucket: 'trial', available_points: '18.00000', expires_at: 'bad-date', expire_warning: false }],
})
if (invalidExpiry[0]?.detail !== '有效期至 bad-date') {
  throw new Error(`topbar should preserve invalid expiry for troubleshooting, got ${JSON.stringify(invalidExpiry)}`)
}
