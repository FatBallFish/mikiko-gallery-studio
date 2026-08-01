import type { TextModel, TextModelAccount, TextModelAccountWriteRequest, TextModelAPIStyle, TextModelDefaultReadiness, TextModelWriteRequest } from '../../../shared/api-types'

export type TextModelAccountDraft = {
  id?: string | number
  version?: number
  name: string
  platformType: 'openai_compatible'
  apiStyle: TextModelAPIStyle
  baseUrl: string
  enabled: boolean
  hasSecret: boolean
  fingerprint: string
  secretMode: 'keep' | 'replace' | 'clear'
  apiKey: string
}

export type TextModelDraft = {
  id?: string | number
  version?: number
  modelCode: string
  displayName: string
  inputPrice: string
  outputPrice: string
  currency: string
  enabled: boolean
  isDefault: boolean
}

export function emptyTextModelAccountDraft(): TextModelAccountDraft {
  return { name: '', platformType: 'openai_compatible', apiStyle: 'responses', baseUrl: '', enabled: false, hasSecret: false, fingerprint: '', secretMode: 'keep', apiKey: '' }
}

export function accountDraftFromView(account: TextModelAccount): TextModelAccountDraft {
  return {
    id: account.id, version: account.version, name: account.name, platformType: account.platform_type,
    apiStyle: account.api_style, baseUrl: account.base_url, enabled: account.enabled,
    hasSecret: account.secret_status.has_secret, fingerprint: account.secret_status.fingerprint ?? '', secretMode: 'keep', apiKey: '',
  }
}

export function modelDraftFromView(model: TextModel): TextModelDraft {
  return {
    id: model.id, version: model.version, modelCode: model.model_code, displayName: model.display_name,
    inputPrice: model.input_price_per_million_tokens, outputPrice: model.output_price_per_million_tokens,
    currency: model.currency, enabled: model.enabled, isDefault: model.is_default,
  }
}

export function textModelReadiness(accounts: TextModelAccount[], models: TextModel[]): TextModelDefaultReadiness {
  const enabledAccounts = new Map(accounts.filter((account) => account.enabled).map((account) => [String(account.id), account]))
  const eligibleModels = models.filter((model) => model.enabled && enabledAccounts.has(String(model.account_id)))
  const defaults = eligibleModels.filter((model) => model.is_default)
  if (defaults.length === 1) {
    const defaultModel = defaults[0]
    return {
      status: 'ready',
      eligibleCount: eligibleModels.length,
      defaultModel,
      defaultAccount: enabledAccounts.get(String(defaultModel.account_id)),
    }
  }
  return {
    status: eligibleModels.length ? 'selection_required' : 'unavailable',
    eligibleCount: eligibleModels.length,
  }
}

export function validateTextModelAccountDraft(draft: TextModelAccountDraft) {
  if (!draft.name.trim()) return '请输入账号名称'
  try {
    const url = new URL(draft.baseUrl.trim())
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.hash) return 'Base URL 仅支持无凭据的 HTTP(S) 地址'
  } catch {
    return '请输入有效的 Base URL'
  }
  if (draft.secretMode === 'replace' && !draft.apiKey.trim()) return '请输入新的 API 密钥'
  if (draft.enabled && draft.secretMode === 'clear') return '启用账号时不能清除密钥'
  if (draft.enabled && !draft.hasSecret && draft.secretMode !== 'replace') return '启用账号前需要配置 API 密钥'
  return ''
}

export function textModelAccountRequest(draft: TextModelAccountDraft): TextModelAccountWriteRequest {
  const request: TextModelAccountWriteRequest = {
    version: draft.version, name: draft.name.trim(), platform_type: draft.platformType, api_style: draft.apiStyle,
    base_url: draft.baseUrl.trim().replace(/\/$/, ''), enabled: draft.enabled,
  }
  if (draft.secretMode === 'replace') request.secrets = { api_key: draft.apiKey.trim() }
  if (draft.secretMode === 'clear') request.clear_secrets = ['api_key']
  return request
}

export function validateTextModelDraft(draft: TextModelDraft) {
  if (!draft.modelCode.trim()) return '请输入模型代码'
  for (const value of [draft.inputPrice, draft.outputPrice]) {
    if (!/^\d+(?:\.\d{1,6})?$/.test(value.trim()) || Number(value) < 0) return '价格必须是非负数，最多 6 位小数'
  }
  if (!/^[A-Za-z]{3}$/.test(draft.currency.trim())) return '币种必须是 3 位代码'
  return ''
}

function fixedPrice(value: string) {
  return Number(value.trim() || 0).toFixed(6)
}

export function textModelModelRequest(draft: TextModelDraft): TextModelWriteRequest {
  return {
    version: draft.version, model_code: draft.modelCode.trim(), display_name: draft.displayName.trim() || draft.modelCode.trim(),
    input_price_per_million_tokens: fixedPrice(draft.inputPrice), output_price_per_million_tokens: fixedPrice(draft.outputPrice),
    currency: draft.currency.trim().toUpperCase(), enabled: draft.enabled,
  }
}
