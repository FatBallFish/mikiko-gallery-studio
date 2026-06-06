import type { Balance, Capability } from '../../../shared/api-types'
import { homeAccountReadinessView, homeModelReadinessView } from './homeGalleryModel'

const loading = homeModelReadinessView(null, true)
if (loading.value !== '检测中' || loading.detail !== '正在检查平台生图能力。' || loading.warning) {
  throw new Error(`loading model readiness should explain capability check, got ${JSON.stringify(loading)}`)
}

const ready = homeModelReadinessView(capability(['basic', 'plus']), false)
if (ready.value !== '2 个模型可用' || ready.detail !== '文生图/参考图按配置开放' || ready.warning) {
  throw new Error(`ready model readiness should show model count and enabled detail, got ${JSON.stringify(ready)}`)
}

const unavailable = homeModelReadinessView(capability([]), false)
if (unavailable.value !== '平台模型配置中') {
  throw new Error(`unavailable model readiness should avoid raw empty-state wording, got ${JSON.stringify(unavailable)}`)
}
if (!unavailable.detail.includes('请稍后再试') || unavailable.detail === '暂不可生成') {
  throw new Error(`unavailable model readiness should give concrete user guidance, got ${JSON.stringify(unavailable)}`)
}
if (!unavailable.warning) {
  throw new Error(`unavailable model readiness should mark warning state, got ${JSON.stringify(unavailable)}`)
}

const accountReady = homeAccountReadinessView({
  available_points: '123.45600',
  frozen_points: '0.00000',
  trial_points: '18.00000',
  recharge_points: '105.45600',
  subscription_points: '0.00000',
  plan_name: 'FREE',
  first_purchase_bonus: false,
  buckets: [
    { bucket: 'trial', label: '体验额度', available_points: '18.00000', expires_at: '2026-06-12T00:00:00Z', expire_warning: true },
    { bucket: 'recharge', label: '充值额度', available_points: '105.45600' },
  ],
} satisfies Balance)
if (accountReady.availableValue !== '123.46 ◈') {
  throw new Error(`home account readiness should format available points to 2 decimals, got ${accountReady.availableValue}`)
}
if (accountReady.trialValue !== '18.00 ◈' || accountReady.trialDetail !== '即将过期：2026/06/12' || !accountReady.trialWarning) {
  throw new Error(`home account readiness should show trial points and stable expiry warning, got ${JSON.stringify(accountReady)}`)
}
if (accountReady.secondaryAction !== 'gallery') {
  throw new Error(`home account readiness should send positive-balance users to gallery, got ${accountReady.secondaryAction}`)
}

const noTrial = homeAccountReadinessView({ ...zeroBalance(), available_points: '0.00000' })
if (noTrial.trialValue !== '暂无' || noTrial.trialDetail !== '可通过活动或兑换码获得' || noTrial.secondaryAction !== 'recharge') {
  throw new Error(`home account readiness should show no-trial guidance and recharge action, got ${JSON.stringify(noTrial)}`)
}

const invalidExpiry = homeAccountReadinessView({
  ...zeroBalance(),
  available_points: '5.00000',
  trial_points: '5.00000',
  buckets: [{ bucket: 'trial', available_points: '5.00000', expires_at: 'bad-date', expire_warning: false }],
})
if (invalidExpiry.trialDetail !== '有效期至 bad-date') {
  throw new Error(`home account readiness should preserve invalid expiry for troubleshooting, got ${invalidExpiry.trialDetail}`)
}

function capability(codes: string[]): Capability {
  return {
    model_groups: codes.map((code) => ({
      id: code,
      code,
      name: code,
      task_types: ['text_to_image'],
      qualities: ['1k'],
      prices: [],
      supports_reference: true,
    })),
    qualities: ['1k'],
    aspect_ratios: ['1:1'],
    max_image_count: 1,
    task_types: ['text_to_image'],
  }
}

function zeroBalance(): Balance {
  return {
    available_points: '0.00000',
    frozen_points: '0.00000',
    trial_points: '0.00000',
    subscription_points: '0.00000',
    recharge_points: '0.00000',
    plan_name: 'FREE',
    first_purchase_bonus: false,
  }
}
