import type { EstimateResult } from '../../../shared/api-types'

export type WorkspaceGenerateReadinessInput = {
  busy: boolean
  hasModel: boolean
  unavailableReason?: { code: string; message: string } | null
  parametersReady: boolean
  prompt: string
  estimate: EstimateResult | null
  estimateError?: string
}

export type WorkspaceGenerateReadiness = {
  disabled: boolean
  reason: string
  showRechargeAction: boolean
}

export function workspaceGenerateReadiness(input: WorkspaceGenerateReadinessInput): WorkspaceGenerateReadiness {
  if (input.busy) return { disabled: true, reason: '任务正在提交，请稍候。', showRechargeAction: false }
  if (!input.hasModel) return { disabled: true, reason: publicUnavailableReason(input.unavailableReason), showRechargeAction: false }
  if (!input.parametersReady) return { disabled: true, reason: '请选择完整的模型、基础分辨率、比例和图片数量。', showRechargeAction: false }
  if (input.prompt.trim().length < 8) return { disabled: true, reason: '提示词至少需要 8 个字符。', showRechargeAction: false }
  if (input.estimateError) return { disabled: true, reason: input.estimateError, showRechargeAction: false }
  if (!input.estimate) return { disabled: true, reason: '正在计算本次预计消耗，请稍候。', showRechargeAction: false }
  if (!input.estimate.sufficient) {
    const missing = displayPoints(input.estimate.insufficient_points)
    return { disabled: true, reason: `积分不足，还差 ${missing} 积分，请充值或兑换后再试。`, showRechargeAction: true }
  }
  return { disabled: false, reason: '', showRechargeAction: false }
}

export function publicUnavailableReason(reason?: { code: string; message: string } | null) {
  const message = reason?.message?.trim()
  if (!message) return '平台生图能力正在配置中，请稍后再试。'
  if (/后台|账号|route|provider|model account/i.test(message)) return '平台生图能力正在配置中，请稍后再试。'
  return message
}

export function displayPoints(raw?: string) {
  const value = Number(raw ?? '0')
  if (!Number.isFinite(value)) return raw ?? '0.00000'
  return value.toFixed(2)
}
