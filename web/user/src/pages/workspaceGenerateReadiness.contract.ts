import type { EstimateResult } from '../../../shared/api-types'
import { publicUnavailableReason, workspaceGenerateReadiness } from './workspaceGenerateReadiness'

const noModel = workspaceGenerateReadiness({
  busy: false,
  hasModel: false,
  taskType: 'text_to_image',
  referenceCount: 0,
  requiredReferencesReady: true,
  unavailableReason: { code: 'NO_ROUTE_MODEL', message: '平台模型配置中，暂不可生成。' },
  parametersReady: false,
  prompt: '一张未来城市里的雨夜街景',
  estimate: null,
})

if (!noModel.disabled || !noModel.reason.includes('平台模型配置中')) {
  throw new Error(`workspace should explain unavailable model state, got ${noModel.reason}`)
}

if (/后台|账号|route|provider|model account/i.test(noModel.reason)) {
  throw new Error(`workspace unavailable reason should avoid internal terms, got ${noModel.reason}`)
}

const unsafeUnavailableReason = publicUnavailableReason({ code: 'NO_ROUTE_MODEL', message: '后台 route provider model account 未配置' })
if (/后台|route|provider|model account/i.test(unsafeUnavailableReason)) {
  throw new Error(`workspace should sanitize internal unavailable reasons, got ${unsafeUnavailableReason}`)
}

const insufficient = workspaceGenerateReadiness({
  busy: false,
  hasModel: true,
  taskType: 'text_to_image',
  referenceCount: 0,
  requiredReferencesReady: true,
  parametersReady: true,
  prompt: '一张未来城市里的雨夜街景',
  estimate: {
    points: '12.00000',
    display_points: '12.00',
    formula: 'plus x auto',
    base_resolution: 'auto',
    sufficient: false,
    insufficient_points: '7.00000',
  } satisfies EstimateResult,
})

if (!insufficient.disabled || !insufficient.reason.includes('积分不足') || !insufficient.showRechargeAction) {
  throw new Error(`workspace should turn insufficient estimate into recharge guidance, got ${JSON.stringify(insufficient)}`)
}

const shortPrompt = workspaceGenerateReadiness({
  busy: false,
  hasModel: true,
  taskType: 'text_to_image',
  referenceCount: 0,
  requiredReferencesReady: true,
  parametersReady: true,
  prompt: '太短',
  estimate: {
    points: '2.00000',
    formula: 'plus x auto',
    base_resolution: 'auto',
    sufficient: true,
  } satisfies EstimateResult,
})

if (!shortPrompt.disabled || !shortPrompt.reason.includes('至少需要 8 个字符')) {
  throw new Error(`workspace should explain prompt minimum, got ${shortPrompt.reason}`)
}

const unsupportedEstimate = workspaceGenerateReadiness({
  busy: false,
  hasModel: true,
  taskType: 'text_to_image',
  referenceCount: 0,
  requiredReferencesReady: true,
  parametersReady: true,
  prompt: '一张未来城市里的雨夜街景',
  estimate: null,
  estimateError: '当前配置暂不支持生成，请更换类似配置。',
})

if (!unsupportedEstimate.disabled || !unsupportedEstimate.reason.includes('暂不支持生成')) {
  throw new Error(`workspace should block generation when estimate failed, got ${JSON.stringify(unsupportedEstimate)}`)
}

const ready = workspaceGenerateReadiness({
  busy: false,
  hasModel: true,
  taskType: 'text_to_image',
  referenceCount: 0,
  requiredReferencesReady: true,
  parametersReady: true,
  prompt: '一张未来城市里的雨夜街景',
  estimate: {
    points: '2.00000',
    formula: 'plus x auto',
    base_resolution: 'auto',
    sufficient: true,
  } satisfies EstimateResult,
})

if (ready.disabled || ready.reason) {
  throw new Error(`workspace should enable generation when all readiness checks pass, got ${JSON.stringify(ready)}`)
}

for (const taskType of ['reference_to_image', 'image_edit'] as const) {
  const missingReference = workspaceGenerateReadiness({
    busy: false,
    hasModel: true,
    taskType,
    referenceCount: 0,
    requiredReferencesReady: false,
    parametersReady: false,
    prompt: '一张未来城市里的雨夜街景',
    estimate: null,
  })
  if (!missingReference.disabled || !missingReference.reason.includes('至少1张参考图')) {
    throw new Error(`${taskType} without references must explain the required input, got ${JSON.stringify(missingReference)}`)
  }
}

const referenceReady = workspaceGenerateReadiness({
  busy: false,
  hasModel: true,
  taskType: 'reference_to_image',
  referenceCount: 1,
  requiredReferencesReady: true,
  parametersReady: true,
  prompt: '一张未来城市里的雨夜街景',
  estimate: {
    points: '2.00000',
    formula: 'plus x auto',
    sufficient: true,
  },
})
if (referenceReady.disabled || referenceReady.reason) {
  throw new Error(`reference generation with one reference should be ready, got ${JSON.stringify(referenceReady)}`)
}
