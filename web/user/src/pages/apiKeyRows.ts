import type { ApiKey, UpdateApiKeyRequest } from '../../../shared/api-types'

export type ApiKeyBadgeTone = 'success' | 'warning' | 'neutral'

export type ApiKeyStatusBadge = {
  label: string
  tone: ApiKeyBadgeTone
}

export type ApiKeyRow = {
  key: ApiKey
  statusBadge: ApiKeyStatusBadge
  scopesText: string
  accessKeyMasked: string
  totalQuotaLabel: string
  dailyQuotaLabel: string
  expiresAtLabel: string
  expiryHint?: string
  createdAtLabel: string
  lastUsedAtLabel: string
}

export type ApiKeyCreateForm = {
  name: string
  scopes: string[]
  rpmLimit: number
  expiresAt: string
  totalQuotaPoints: string
  dailyQuotaPoints: string
}

export type ApiKeyEditForm = {
  scopes?: string[]
  name: string
  rpmLimit: number
  expiresAt: string
  totalQuotaPoints: string
  dailyQuotaPoints: string
  groupCode: string
}

export const apiKeyTableHeaders = ['名称', '密钥标识（ak-…）', '状态', 'RPM 限制', '额度', '创建时间', '最近调用', '过期时间', '操作'] as const

export const apiKeyPageLabels = {
  create: '创建新密钥',
  quickstartTitle: '快速接入',
  quickstartCodeTitle: '示例请求 (cURL)',
  copyCode: '复制示例',
  resetSecret: '轮换 API Key',
  delete: '删除',
  edit: '编辑',
} as const

export const apiKeyGroupReadOnlyHint = '密钥分组由账号分组统一决定，管理员调整账号分组后会同步影响密钥可用范围。'

const statusBadges: Record<string, ApiKeyStatusBadge> = {
  active: { label: '启用中', tone: 'success' },
  disabled: { label: '已禁用', tone: 'neutral' },
  revoked: { label: '已删除', tone: 'neutral' },
  expired: { label: '已过期', tone: 'warning' },
}

export const defaultApiKeyScopes = ['account:read', 'models:read', 'images:read', 'images:write']
export const allApiKeyScopes = ['account:read', 'models:read', 'images:read', 'images:write', 'videos:read', 'videos:write', 'videos:delete', 'assets:read', 'assets:write', 'assets:delete', 'uploads:read', 'uploads:write', 'uploads:delete', 'projects:read', 'projects:write', 'projects:delete', 'groups:read', 'groups:write', 'groups:delete']

const scopeLabels: Record<string, string> = {
  'account:read': '读取账号与余额',
  'models:read': '读取模型',
  'videos:read': '读取视频任务', 'videos:write': '创建视频任务', 'videos:delete': '取消视频任务',
  'assets:read': '读取素材', 'assets:write': '修改素材', 'assets:delete': '删除素材',
  'uploads:read': '读取上传状态', 'uploads:write': '上传素材', 'uploads:delete': '终止上传',
  'projects:read': '读取项目', 'projects:write': '创建与修改项目', 'projects:delete': '删除项目',
  'groups:read': '读取分组', 'groups:write': '创建与修改分组', 'groups:delete': '删除分组',
  'images:write': '创建图片任务',
  'images:read': '读取图片任务',
  'balance:read': '读取余额',
  'profile:read': '读取账号资料',
}

export function apiKeyStatusBadge(status?: string | null): ApiKeyStatusBadge {
  const normalized = normalize(status)
  return statusBadges[normalized] ?? { label: normalized || '未知状态', tone: 'neutral' }
}

export function apiKeyStatusToggleLabel(status?: string | null) {
  return normalize(status) === 'active' ? '禁用' : '启用'
}

export function apiKeyExpiryText(value?: string | null) {
  return readableDate(value, '永不过期')
}

export function apiKeyExpiryHint(value?: string | null) {
  return value ? undefined : '建议为生产密钥设置有效期'
}

export function apiKeyScopeLabel(scope: string) {
  return scopeLabels[scope] ?? scope
}


export function maskToken(value: string | null | undefined, head = 6, tail = 4) {
  if (!value) return '-'
  if (value.length <= head + tail) return `${value.slice(0, Math.min(2, value.length))}••••`
  return `${value.slice(0, head)}••••••••${value.slice(-tail)}`
}

export function apiKeyDeleteConfirmText(key: Pick<ApiKey, 'name'>) {
  return {
    title: `确认删除 ${key.name}？`,
    detail: '删除后该密钥不可恢复，使用它的调用方会立即鉴权失败且不会扣费。',
  }
}

export function apiKeyScopesText(scopes: string[]) {
  return scopes.map(apiKeyScopeLabel).join(' · ')
}

export function apiKeyQuotaText(label: string, limit?: string | null, used?: string | null) {
  return `${label} ${used || '0.00000'} / ${limit || '不限额'}`
}

export function apiKeyCreatePayload(form: ApiKeyCreateForm) {
  return {
    name: form.name,
    scopes: form.scopes,
    rpm_limit: form.rpmLimit,
    expires_at: form.expiresAt || null,
    total_quota_points: form.totalQuotaPoints.trim() || null,
    daily_quota_points: form.dailyQuotaPoints.trim() || null,
  }
}

export function apiKeyEditForm(key: ApiKey): ApiKeyEditForm {
  return {
    scopes: [...key.scopes],
    name: key.name,
    rpmLimit: key.rpm_limit ?? 30,
    expiresAt: dateInputValue(key.expires_at),
    totalQuotaPoints: key.total_quota_points ?? '',
    dailyQuotaPoints: key.daily_quota_points ?? '',
    groupCode: key.group_code ?? 'default',
  }
}

export function apiKeyUpdatePayload(form: ApiKeyEditForm): UpdateApiKeyRequest {
  return {
    name: form.name,
    scopes: form.scopes,
    rpm_limit: form.rpmLimit,
    expires_at: form.expiresAt || null,
    total_quota_points: form.totalQuotaPoints.trim() || null,
    daily_quota_points: form.dailyQuotaPoints.trim() || null,
  }
}

export function apiKeyRow(key: ApiKey): ApiKeyRow {
  return {
    key,
    statusBadge: apiKeyStatusBadge(key.status),
    scopesText: apiKeyScopesText(key.scopes),
    accessKeyMasked: maskAccessKey(key.access_key),
    totalQuotaLabel: apiKeyQuotaText('总额度', key.total_quota_points, key.total_quota_used_points),
    dailyQuotaLabel: apiKeyQuotaText('日额度', key.daily_quota_points, key.daily_quota_used_points),
    expiresAtLabel: apiKeyExpiryText(key.expires_at),
    expiryHint: apiKeyExpiryHint(key.expires_at),
    createdAtLabel: readableDate(key.created_at, '-'),
    lastUsedAtLabel: readableDate(key.last_used_at, '未调用'),
  }
}

function normalize(value?: string | null) {
  return (value ?? '').trim().toLowerCase()
}

function maskAccessKey(key: string) {
  return maskToken(key, 10, 4)
}

function readableDate(value: string | null | undefined, emptyLabel: string) {
  if (!value) return emptyLabel
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const year = date.getUTCFullYear()
  const month = String(date.getUTCMonth() + 1).padStart(2, '0')
  const day = String(date.getUTCDate()).padStart(2, '0')
  return `${year}/${month}/${day}`
}

function dateInputValue(value: string | null | undefined) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const year = date.getUTCFullYear()
  const month = String(date.getUTCMonth() + 1).padStart(2, '0')
  const day = String(date.getUTCDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}
