import type { CallRecord } from '../../../shared/api-types'
import { callRecordFilterCopy, callRecordRows, callRecordSourceChannelOptions, callRecordStatusOptions } from './callRecordRows'

const rows = callRecordRows([
  {
    id: 1,
    task_id: 'task_1234567890abcdef',
    user_id: 42,
    api_key_id: 7,
    source_channel: 'open_api',
    task_type: 'text_to_image',
    status: 'failed',
    provider: '',
    abstract_model: 'plus',
    quality: '2k',
    requested_output_image_count: 2,
    success_output_image_count: 0,
    reference_image_count: 0,
    estimated_points: '10.00000',
    actual_points: '0.00000',
    provider_cost: '0.00000',
    gross_margin: '0.00000',
    error_code: 'ROUTE_MODEL_PRICE_MISSING',
    error_message: null,
    created_at: '2026-06-05T00:00:00Z',
    updated_at: '2026-06-05T00:00:00Z',
    started_at: null,
    finished_at: null,
    attempt_count: 0,
  },
  {
    id: 2,
    task_id: 'task_succeeded',
    user_id: 9,
    api_key_id: null,
    source_channel: 'web',
    task_type: 'image_to_image',
    status: 'succeeded',
    provider: 'openai-main',
    abstract_model: 'basic',
    quality: '1k',
    requested_output_image_count: 1,
    success_output_image_count: 1,
    reference_image_count: 1,
    estimated_points: '5.00000',
    actual_points: '5.00000',
    provider_cost: '0.12000',
    gross_margin: '4.88000',
    error_code: null,
    error_message: null,
    created_at: '2026-06-05T01:00:00Z',
    updated_at: '2026-06-05T01:00:00Z',
    started_at: '2026-06-05T01:00:01Z',
    finished_at: '2026-06-05T01:00:07Z',
    attempt_count: 1,
  },
] satisfies CallRecord[])

if (rows[0]?.statusTone !== 'danger' || rows[0]?.failureLabel !== 'ROUTE_MODEL_PRICE_MISSING') {
  throw new Error(`failed call records should expose danger error code, got ${JSON.stringify(rows[0])}`)
}

if (rows[0]?.status !== 'failed' || rows[0]?.statusLabel !== '失败') {
  throw new Error(`failed call records should preserve raw status and expose localized label, got ${JSON.stringify(rows[0])}`)
}

if (!rows[0]?.taskDetail.startsWith('文生图 ·')) {
  throw new Error(`call records should localize text-to-image task type, got ${rows[0]?.taskDetail}`)
}

if (!rows[0]?.failureDetail.includes('价格配置')) {
  throw new Error(`route price preflight failure should guide operators to pricing config, got ${rows[0]?.failureDetail}`)
}

if (rows[0]?.routeDetail !== '2k · Open API') {
  throw new Error(`call record route detail should localize source channel labels, got ${rows[0]?.routeDetail}`)
}

if (rows[0]?.userDetail !== 'API Key #7') {
  throw new Error(`API call records should show the key id, got ${rows[0]?.userDetail}`)
}

if (rows[0]?.amountLabel !== '10.00000 / 0.00000') {
  throw new Error(`call records should preserve estimated and actual points, got ${rows[0]?.amountLabel}`)
}

if (rows[0]?.createdAt !== '2026/06/05 00:00' || rows[0]?.lifecycleLabel !== '创建 2026/06/05 00:00') {
  throw new Error(`call records should format created timeline deterministically, got ${JSON.stringify(rows[0])}`)
}

if (rows[1]?.statusTone !== 'success' || rows[1]?.failureLabel !== '无错误') {
  throw new Error(`successful call records should stay quiet on failure, got ${JSON.stringify(rows[1])}`)
}

if (rows[1]?.status !== 'succeeded' || rows[1]?.statusLabel !== '成功') {
  throw new Error(`successful call records should preserve raw status and expose localized label, got ${JSON.stringify(rows[1])}`)
}

if (!rows[1]?.taskDetail.startsWith('图片编辑 ·')) {
  throw new Error(`call records should localize legacy image-to-image task type, got ${rows[1]?.taskDetail}`)
}

if (rows[1]?.providerDetail !== '1 次尝试' || rows[1]?.costLabel !== '0.12000' || rows[1]?.marginLabel !== '4.88000') {
  throw new Error(`successful call records should expose attempts and costs, got ${JSON.stringify(rows[1])}`)
}

if (rows[1]?.createdAt !== '2026/06/05 01:00' || rows[1]?.lifecycleLabel !== '开始 2026/06/05 01:00 · 结束 2026/06/05 01:00') {
  throw new Error(`successful call records should format start/end timeline deterministically, got ${JSON.stringify(rows[1])}`)
}

if (/T|Z$/.test(`${rows[0]?.createdAt}${rows[0]?.lifecycleLabel}${rows[1]?.createdAt}${rows[1]?.lifecycleLabel}`)) {
  throw new Error(`call record timeline should not expose ISO separators, got ${JSON.stringify(rows)}`)
}

const invalidDateRow = callRecordRows([{
  ...rows[0]!,
  id: 3,
  task_id: 'task_invalid_date',
  created_at: 'not-a-date',
  updated_at: 'not-a-date',
  started_at: 'also-not-a-date',
  finished_at: '',
} as unknown as CallRecord])[0]
if (invalidDateRow?.createdAt !== 'not-a-date' || invalidDateRow?.lifecycleLabel !== '开始 also-not-a-date · 结束 -') {
  throw new Error(`call records should preserve invalid dates and use - for missing dates, got ${JSON.stringify(invalidDateRow)}`)
}

const filterCopyText = Object.values(callRecordFilterCopy).flatMap((item) => (
  'placeholder' in item ? [item.label, item.placeholder] : [item.label]
)).join(' ')
for (const internalName of ['error_code', 'source_channel', 'user_id', 'task_id']) {
  if (filterCopyText.includes(internalName)) {
    throw new Error(`call record filter copy should be operator-facing, but still exposes ${internalName}: ${filterCopyText}`)
  }
}

for (const expected of ['错误码', '入口', '用户 ID', '任务 ID']) {
  if (!filterCopyText.includes(expected)) {
    throw new Error(`call record filter copy should include ${expected}, got ${filterCopyText}`)
  }
}

const statusOptionValues = callRecordStatusOptions.map((option) => option.value).join(',')
for (const rawStatus of ['', 'queued', 'running', 'succeeded', 'failed', 'canceled']) {
  if (!callRecordStatusOptions.some((option) => option.value === rawStatus)) {
    throw new Error(`call record status options should preserve query value ${rawStatus}, got ${statusOptionValues}`)
  }
}

for (const option of callRecordStatusOptions) {
  const visibleLabel = String(option.label)
  const queryValue = String(option.value)
  if (queryValue && visibleLabel === queryValue) {
    throw new Error(`call record status option should localize visible label for ${option.value}`)
  }
}

const sourceChannelValues = callRecordSourceChannelOptions.map((option) => option.value).join(',')
for (const rawSourceChannel of ['', 'web', 'open_api', 'openai_compatible']) {
  if (!callRecordSourceChannelOptions.some((option) => option.value === rawSourceChannel)) {
    throw new Error(`call record source channel options should preserve query value ${rawSourceChannel}, got ${sourceChannelValues}`)
  }
}

for (const option of callRecordSourceChannelOptions) {
  const visibleLabel = String(option.label)
  const queryValue = String(option.value)
  if (queryValue && visibleLabel === queryValue) {
    throw new Error(`call record source channel option should expose operator-facing label for ${option.value}`)
  }
}
