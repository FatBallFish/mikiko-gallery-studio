import type { EstimateResult } from '../../../shared/api-types'
import { workspaceGenerateReadiness } from './workspaceGenerateReadiness'

const noModel = workspaceGenerateReadiness({
  busy: false,
  hasModel: false,
  parametersReady: false,
  prompt: '一张未来城市里的雨夜街景',
  estimate: null,
})

if (!noModel.disabled || !noModel.reason.includes('平台生图能力正在配置中')) {
  throw new Error(`workspace should explain unavailable model state, got ${noModel.reason}`)
}

if (/后台|账号|route|provider|model account/i.test(noModel.reason)) {
  throw new Error(`workspace unavailable reason should avoid internal terms, got ${noModel.reason}`)
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
