import type { Balance, Capability, ImageResult, ImageTask } from '../../../shared/api-types'
import { curatedHomeGallery, homeAccountReadinessView, homeContinuationView, homeModelReadinessView, homeRecentTaskView } from './homeGalleryModel'

const noHistory = homeContinuationView([])
if (noHistory.action !== 'create' || noHistory.route !== 'genpic' || noHistory.label !== '开始第一次创作') {
  throw new Error(`home without task history should start creation, got ${JSON.stringify(noHistory)}`)
}

const latestFailed = task({ id: 'task_failed', status: 'failed', created_at: '2026-07-10T09:00:00Z', failure_reason: '模型暂时繁忙' })
const olderSuccess = task({ id: 'task_done', status: 'succeeded', created_at: '2026-07-09T09:00:00Z' })
const continuation = homeContinuationView([olderSuccess, latestFailed])
if (continuation.action !== 'retry' || continuation.route !== 'genpic' || continuation.taskId !== 'task_failed') {
  throw new Error(`home should continue the newest actionable task, got ${JSON.stringify(continuation)}`)
}
if (continuation.route === ('docs' as string) || continuation.route === ('checkout' as string)) {
  throw new Error(`documentation and billing must never become the home primary action, got ${continuation.route}`)
}

const recentFailure = homeRecentTaskView(latestFailed, false)
if (recentFailure.tone !== 'error' || recentFailure.action !== 'retry' || !recentFailure.detail.includes('模型暂时繁忙')) {
  throw new Error(`failed recent task should explain the failure and offer retry, got ${JSON.stringify(recentFailure)}`)
}

const recentRunning = homeRecentTaskView(task({ status: 'running', progress: 42 }), false)
if (recentRunning.tone !== 'warning' || recentRunning.action !== 'continue' || !recentRunning.detail.includes('42%')) {
  throw new Error(`running recent task should expose progress and continuation, got ${JSON.stringify(recentRunning)}`)
}

const recentLoading = homeRecentTaskView(null, true)
if (recentLoading.state !== 'loading' || recentLoading.action !== 'none') {
  throw new Error(`recent task loading should stay non-actionable, got ${JSON.stringify(recentLoading)}`)
}

const curated = curatedHomeGallery(Array.from({ length: 10 }, (_, index) => image(`image_${index}`)), 6)
if (curated.length !== 6 || curated[0]?.id !== 'image_0' || curated[5]?.id !== 'image_5') {
  throw new Error(`home curated gallery should preserve order and stay bounded to six works, got ${curated.map((item) => item.id).join(',')}`)
}

const uniqueCurated = curatedHomeGallery([image('duplicate'), image('duplicate'), image('unique')], 6)
if (uniqueCurated.length !== 2 || uniqueCurated[1]?.id !== 'unique') {
  throw new Error(`home curated gallery should de-duplicate results, got ${uniqueCurated.map((item) => item.id).join(',')}`)
}

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
      base_resolution: ['1k'],
      prices: [],
      supports_reference: true,
    })),
    base_resolution: ['1k'],
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

function task(patch: Partial<ImageTask>): ImageTask {
  return {
    id: patch.id ?? 'task_1',
    title: patch.title ?? '新作品',
    prompt: patch.prompt ?? 'a cinematic landscape',
    task_type: patch.task_type ?? 'text_to_image',
    status: patch.status ?? 'succeeded',
    model_group: patch.model_group ?? 'plus',
    quality: patch.quality ?? 'auto',
    aspect_ratio: patch.aspect_ratio ?? '1:1',
    image_count: patch.image_count ?? 1,
    estimate_points: patch.estimate_points ?? '1.00000',
    progress: patch.progress ?? 100,
    provider: patch.provider ?? 'openai',
    route: patch.route ?? 'plus',
    reference_assets: patch.reference_assets ?? [],
    results: patch.results ?? [],
    created_at: patch.created_at ?? '2026-07-10T08:00:00Z',
    updated_at: patch.updated_at ?? patch.created_at ?? '2026-07-10T08:00:00Z',
    failure_reason: patch.failure_reason,
  }
}

function image(id: string): ImageResult {
  return {
    id,
    url: `/images/${id}.png`,
    width: 1024,
    height: 1024,
    publish_status: 'approved',
  }
}
