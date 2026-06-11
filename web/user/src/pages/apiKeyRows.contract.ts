import type { ApiKey } from '../../../shared/api-types'
import {
  apiKeyCreatePayload,
  apiKeyDeleteConfirmText,
  apiKeyEditForm,
  apiKeyUpdatePayload,
  apiKeyExpiryHint,
  apiKeyExpiryText,
  apiKeyGroupReadOnlyHint,
  apiKeyPageLabels,
  apiKeyQuickstart,
  apiKeyRow,
  apiKeyScopeLabel,
  apiKeyStatusBadge,
  apiKeyStatusToggleLabel,
  apiKeyTableHeaders,
  apiKeyQuotaText,
  maskSecretPreview,
  maskToken,
} from './apiKeyRows'

const active = apiKeyStatusBadge('active')
const disabled = apiKeyStatusBadge('disabled')
const expired = apiKeyStatusBadge('expired')
const unknown = apiKeyStatusBadge('rotating')
if (active.label !== '启用中' || active.tone !== 'success' || disabled.label !== '已禁用' || disabled.tone !== 'neutral' || expired.label !== '已过期' || expired.tone !== 'warning') {
  throw new Error(`api key statuses should be localized, got ${JSON.stringify({ active, disabled, expired })}`)
}
if (unknown.label !== 'rotating' || unknown.tone !== 'neutral') {
  throw new Error(`unknown api key statuses should preserve raw values, got ${JSON.stringify(unknown)}`)
}

if (apiKeyStatusToggleLabel('active') !== '禁用' || apiKeyStatusToggleLabel('disabled') !== '启用') {
  throw new Error('api key status toggle labels should be action-oriented')
}

if (apiKeyExpiryText(null) !== '永不过期') {
  throw new Error(`api key expiry should distinguish never-expiring keys, got ${apiKeyExpiryText(null)}`)
}
if (apiKeyExpiryText('2026-06-12T00:00:00Z') !== '2026/06/12') {
  throw new Error(`api key expiry should format ISO dates for users, got ${apiKeyExpiryText('2026-06-12T00:00:00Z')}`)
}
if (apiKeyExpiryText('not-a-date') !== 'not-a-date') {
  throw new Error(`api key expiry should preserve invalid raw values for diagnosis, got ${apiKeyExpiryText('not-a-date')}`)
}
if (apiKeyExpiryHint(null) !== '建议为生产密钥设置有效期') {
  throw new Error(`never-expiring api keys should surface risk guidance, got ${apiKeyExpiryHint(null)}`)
}
if (apiKeyExpiryHint('2026-06-12T00:00:00Z') !== undefined) {
  throw new Error(`dated api keys should not show never-expiring risk guidance, got ${apiKeyExpiryHint('2026-06-12T00:00:00Z')}`)
}

if (apiKeyScopeLabel('images:write') !== '创建图片任务' || apiKeyScopeLabel('balance:read') !== '读取余额' || apiKeyScopeLabel('future:scope') !== 'future:scope') {
  throw new Error('api key scopes should show readable labels and preserve unknown values')
}

if ('eyebrow' in apiKeyPageLabels || apiKeyPageLabels.quickstartTitle !== '快速接入' || apiKeyPageLabels.quickstartCodeTitle !== '示例请求 (cURL)' || apiKeyPageLabels.copyCode !== '复制示例') {
  throw new Error(`api key page labels should be localized, got ${JSON.stringify(apiKeyPageLabels)}`)
}
for (const forbidden of ['DEVELOPER PORTAL', 'Example Request', 'copy', '显示', '隐藏']) {
  if (Object.values(apiKeyPageLabels).join(' ').includes(forbidden)) {
    throw new Error(`api key visible labels should not expose ${forbidden}`)
  }
}

const headers = apiKeyTableHeaders.join(',')
if (headers !== '名称,Access Key,Secret,状态,RPM 限制,额度,创建时间,最近调用,过期时间,操作') {
  throw new Error(`api key table headers should be localized but preserve standard key names, got ${headers}`)
}

if (maskToken('sk_abcdefghijklmnopqrstuvwxyz') !== 'sk_abc••••••••wxyz') {
  throw new Error(`api key maskToken should keep token prefix and suffix only, got ${maskToken('sk_abcdefghijklmnopqrstuvwxyz')}`)
}
if (maskToken('short') !== 'sh••••' || maskToken(null) !== '-') {
  throw new Error(`api key maskToken should handle short or empty values, got ${maskToken('short')} / ${maskToken(null)}`)
}
if (maskSecretPreview('sk_once_abc123') !== 'sk_onc••••••••c123' || maskSecretPreview(null) !== 'sk_••••••••••••') {
  throw new Error(`api key secret previews should be masked in normal rows, got ${maskSecretPreview('sk_once_abc123')} / ${maskSecretPreview(null)}`)
}

const sample = apiKeyQuickstart(apiKey({ access_key: 'pk_test_123456', secret_preview: 'sk_once_abc123' }))
if (!sample.code.includes('Authorization: Bearer sk_onc••••••••c123') || !sample.code.includes('/v1/images/generations')) {
  throw new Error(`api key quickstart should use masked selected secret and OpenAI-compatible path, got ${sample.code}`)
}
if (sample.code.includes('Bearer pk_test_123456') || sample.code.includes('sk_once_abc123') || sample.code.includes('api.picgallery.ai')) {
  throw new Error(`api key quickstart must not expose access keys, raw secrets, or old brand domains, got ${sample.code}`)
}
if (sample.visibleCredentials.accessKey !== 'pk_tes••••••••3456' || sample.visibleCredentials.secretKey !== 'sk_onc••••••••c123') {
  throw new Error(`api key quickstart should expose masked visible credential metadata, got ${JSON.stringify(sample.visibleCredentials)}`)
}
if (sample.title !== '示例请求 (cURL)' || sample.copyLabel !== '复制示例') {
  throw new Error(`api key quickstart labels should be localized, got ${JSON.stringify(sample)}`)
}

const emptySample = apiKeyQuickstart(null)
if (!emptySample.code.includes('sk_live_xxx') || emptySample.code.includes('pk_live_xxx')) {
  throw new Error(`api key quickstart should have a safe placeholder when no key exists, got ${emptySample.code}`)
}

const confirm = apiKeyDeleteConfirmText(apiKey({ name: 'prod client' }))
if (!confirm.title.includes('prod client') || !confirm.detail.includes('不可恢复')) {
  throw new Error(`api key delete confirmation should mention name and irreversible deletion, got ${JSON.stringify(confirm)}`)
}

const row = apiKeyRow(apiKey({ access_key: 'pk_live_4kL8m2z9X1', expires_at: '2026-06-12T00:00:00Z', created_at: '2026-06-05T01:02:03Z', last_used_at: null }))
if (row.accessKeyMasked !== 'pk_live_4k••••••••z9X1') {
  throw new Error(`api key row should mask access keys consistently, got ${row.accessKeyMasked}`)
}
if (row.secretMasked !== 'sk_••••••••••••') {
  throw new Error(`api key row should not expose absent or persisted secret values, got ${row.secretMasked}`)
}
if (row.expiresAtLabel !== '2026/06/12' || /[TZ]/.test(row.expiresAtLabel)) {
  throw new Error(`api key row should expose readable expiry label without raw ISO markers, got ${row.expiresAtLabel}`)
}
if (row.createdAtLabel !== '2026/06/05' || row.lastUsedAtLabel !== '未调用') {
  throw new Error(`api key row should expose readable lifecycle labels, got ${JSON.stringify(row)}`)
}
if (/[TZ]/.test(row.createdAtLabel) || /[TZ]/.test(row.lastUsedAtLabel)) {
  throw new Error(`api key lifecycle labels should not expose raw ISO markers, got ${JSON.stringify(row)}`)
}
if (row.expiryHint !== undefined || row.statusBadge.label !== '启用中' || row.scopesText !== '创建图片任务 · 读取图片任务') {
  throw new Error(`api key row should reuse status and scope models, got ${JSON.stringify(row)}`)
}
if (row.totalQuotaLabel !== '总额度 0.00000 / 不限额' || row.dailyQuotaLabel !== '日额度 0.00000 / 不限额') {
  throw new Error(`api key row should expose unlimited quota defaults, got ${JSON.stringify(row)}`)
}

const quotaRow = apiKeyRow(apiKey({ total_quota_points: '100.00000', total_quota_used_points: '12.50000', daily_quota_points: '10.00000', daily_quota_used_points: '2.00000' }))
if (quotaRow.totalQuotaLabel !== '总额度 12.50000 / 100.00000' || quotaRow.dailyQuotaLabel !== '日额度 2.00000 / 10.00000') {
  throw new Error(`api key row should expose quota usage and limits, got ${JSON.stringify(quotaRow)}`)
}
if (apiKeyQuotaText('总额度', null, undefined) !== '总额度 0.00000 / 不限额') {
  throw new Error(`api key quota text should handle missing values, got ${apiKeyQuotaText('总额度', null, undefined)}`)
}

const neverExpiringRow = apiKeyRow(apiKey({ expires_at: null, last_used_at: '2026-06-06T08:09:10Z' }))
if (neverExpiringRow.expiresAtLabel !== '永不过期' || neverExpiringRow.expiryHint !== '建议为生产密钥设置有效期' || neverExpiringRow.lastUsedAtLabel !== '2026/06/06') {
  throw new Error(`api key row should call out never-expiring keys and format usage time, got ${JSON.stringify(neverExpiringRow)}`)
}

const payload = apiKeyCreatePayload({
  name: 'quota key',
  scopes: ['images:write'],
  rpmLimit: 60,
  expiresAt: '2026-06-12',
  totalQuotaPoints: '100.00000',
  dailyQuotaPoints: '',
})
if (payload.name !== 'quota key' || payload.rpm_limit !== 60 || payload.expires_at !== '2026-06-12' || payload.total_quota_points !== '100.00000' || payload.daily_quota_points !== null || payload.scopes?.[0] !== 'images:write') {
  throw new Error(`api key create payload should include quotas and scopes, got ${JSON.stringify(payload)}`)
}

const editForm = apiKeyEditForm(apiKey({
  name: 'production key',
  rpm_limit: 120,
  total_quota_points: '500.00000',
  daily_quota_points: null,
  expires_at: '2026-06-12T00:00:00Z',
  group_code: 'default',
}))
if (editForm.name !== 'production key' || editForm.rpmLimit !== 120 || editForm.totalQuotaPoints !== '500.00000' || editForm.dailyQuotaPoints !== '' || editForm.expiresAt !== '2026-06-12') {
  throw new Error(`api key edit form should normalize persisted key fields for form controls, got ${JSON.stringify(editForm)}`)
}
if ('group_code' in apiKeyUpdatePayload(editForm)) {
  throw new Error(`api key update payload must not submit group_code because backend forbids user-side group changes, got ${JSON.stringify(apiKeyUpdatePayload(editForm))}`)
}
if (!apiKeyGroupReadOnlyHint.includes('管理员') || !apiKeyGroupReadOnlyHint.includes('账号分组') || !apiKeyGroupReadOnlyHint.includes('密钥可用范围')) {
  throw new Error(`api key group read-only hint should explain admin-owned account grouping and key capability scope, got ${apiKeyGroupReadOnlyHint}`)
}
if (/当前版本|暂不支持|后续|即将|版本/.test(apiKeyGroupReadOnlyHint)) {
  throw new Error(`api key group read-only hint should be productized without roadmap or half-finished wording, got ${apiKeyGroupReadOnlyHint}`)
}

const updatePayload = apiKeyUpdatePayload({
  ...editForm,
  name: 'updated key',
  rpmLimit: 240,
  expiresAt: '',
  totalQuotaPoints: '',
  dailyQuotaPoints: '20.00000',
})
if (updatePayload.name !== 'updated key' || updatePayload.rpm_limit !== 240 || updatePayload.expires_at !== null || updatePayload.total_quota_points !== null || updatePayload.daily_quota_points !== '20.00000') {
  throw new Error(`api key update payload should include editable quota/lifecycle fields and null empty values, got ${JSON.stringify(updatePayload)}`)
}

function apiKey(patch: Partial<ApiKey>): ApiKey {
  return {
    id: patch.id ?? 'key_1',
    name: patch.name ?? 'test key',
    access_key: patch.access_key ?? 'pk_test_xxx',
    secret_preview: patch.secret_preview,
    status: patch.status ?? 'active',
    scopes: patch.scopes ?? ['images:write', 'images:read'],
    total_quota_points: patch.total_quota_points,
    daily_quota_points: patch.daily_quota_points,
    total_quota_used_points: patch.total_quota_used_points,
    daily_quota_used_points: patch.daily_quota_used_points,
    rpm_limit: patch.rpm_limit ?? 30,
    expires_at: patch.expires_at ?? null,
    created_at: patch.created_at ?? '2026-06-05T00:00:00Z',
    updated_at: patch.updated_at ?? '2026-06-05T00:00:00Z',
    last_used_at: patch.last_used_at ?? null,
  }
}
