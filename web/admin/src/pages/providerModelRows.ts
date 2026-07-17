import type { ModelAccountModel } from '../../../shared/api-types'
import { adminTaskTypeLabel } from './adminTaskTypes'

export type ProviderModelTone = 'success' | 'warning' | 'danger' | 'neutral'

const adapterLabels: Record<string, string> = {
  openai_compatible: 'OpenAI 兼容',
  openrouter: 'OpenRouter',
}

const authLabels: Record<string, string> = {
  api_key: 'API Key',
}

const accountStatusLabels: Record<string, string> = {
  enabled: '启用',
  disabled: '停用',
  error: '异常',
}

export function providerAdapterLabel(adapterType: string) {
  return adapterLabels[normalize(adapterType)] ?? fallback(adapterType, '未知接入')
}

export function providerAuthLabel(authType: string) {
  return authLabels[normalize(authType)] ?? fallback(authType, '未知鉴权')
}

export function providerAccountDialogDetail() {
  return '填写账号名称、Base URL 和 API Key 后即可启用模型账号；编辑账号时 API Key 留空会保持原密钥。'
}

export function modelAccountStatusLabel(status: string) {
  return accountStatusLabels[normalize(status)] ?? fallback(status, '未知状态')
}

export function modelAccountStatusTone(status: string): ProviderModelTone {
  const normalized = normalize(status)
  if (normalized === 'enabled') return 'success'
  if (normalized === 'error') return 'danger'
  if (normalized === 'disabled') return 'warning'
  return 'neutral'
}

export function credentialsStatusLabel(hasAPIKey?: boolean) {
  return hasAPIKey ? 'API Key 已配置' : '未配置密钥'
}

export function modelEnabledLabel(enabled: boolean) {
  return enabled ? '启用' : '停用'
}

export function modelEnabledTone(enabled: boolean): ProviderModelTone {
  return enabled ? 'success' : 'warning'
}

export function modelCapabilitySummary(model: Pick<ModelAccountModel, 'task_types' | 'base_resolution' | 'cost_per_image' | 'currency'>) {
  const taskTypes = model.task_types.length ? model.task_types.map(adminTaskTypeLabel).join('/') : '未配置任务类型'
  const base_resolution = model.base_resolution.length ? model.base_resolution.join('/') : '未配置基础分辨率'
  return `${taskTypes} · ${base_resolution} · ${model.cost_per_image} ${model.currency}`
}

function normalize(value: string) {
  return value.trim().toLowerCase()
}

function fallback(value: string, emptyText: string) {
  const normalized = value.trim()
  return normalized || emptyText
}
