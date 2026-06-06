import type { EstimateResult } from '../../../shared/api-types'
import { publicUnavailableReason, workspaceGenerateReadiness } from './workspaceGenerateReadiness'

const noModel = workspaceGenerateReadiness({
  busy: false,
  hasModel: false,
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
  parametersReady: true,
  prompt: '一张未来城市里的雨夜街景',
  estimate: {
    points: '12.00000',
    display_points: '12.00',
    formula: 'plus x auto',
    resolved_quality: 'auto',
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
  parametersReady: true,
  prompt: '太短',
  estimate: {
    points: '2.00000',
    formula: 'plus x auto',
    resolved_quality: 'auto',
    sufficient: true,
  } satisfies EstimateResult,
})

if (!shortPrompt.disabled || !shortPrompt.reason.includes('至少需要 8 个字符')) {
  throw new Error(`workspace should explain prompt minimum, got ${shortPrompt.reason}`)
}

const ready = workspaceGenerateReadiness({
  busy: false,
  hasModel: true,
  parametersReady: true,
  prompt: '一张未来城市里的雨夜街景',
  estimate: {
    points: '2.00000',
    formula: 'plus x auto',
    resolved_quality: 'auto',
    sufficient: true,
  } satisfies EstimateResult,
})

if (ready.disabled || ready.reason) {
  throw new Error(`workspace should enable generation when all readiness checks pass, got ${JSON.stringify(ready)}`)
}
