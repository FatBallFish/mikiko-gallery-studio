import { API_PATHS, type LedgerEntry, type RedeemCode } from '../../../shared/api-types'
import {
  redeemBatchCreatePayload,
  redeemCodeRows,
  redeemCodesCSV,
  redeemExportFilename,
  redeemRedemptionRows,
  redeemRewardTypeLabel,
  redeemStatusAction,
  redeemStatusLabel,
  redeemStatusOptions,
  redeemStatusTone,
} from './redeemRows'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'

const redeemPageSource = readFileSync(new URL('./RedeemPage.tsx', import.meta.url), 'utf8')

const sample = (override: Partial<RedeemCode>): RedeemCode => ({
  id: 1,
  batch_id: 202606050001,
  code: 'WELCOME-20',
  status: 'available',
  reward_type: 'points',
  reward_value: '20.00000',
  valid_from: '2026-06-05T00:00:00Z',
  valid_until: '2026-07-05T00:00:00Z',
  max_redemptions: 1,
  redeemed_count: 0,
  last_redeemed_by: null,
  created_at: '2026-06-05T00:00:00Z',
  updated_at: '2026-06-05T00:00:00Z',
  ...override,
})

const rows = redeemCodeRows([
  sample({ status: 'available', reward_type: 'points', reward_value: '20.00000' }),
  sample({ id: 2, code: 'USED-1', status: 'redeemed', redeemed_count: 1 }),
  sample({ id: 3, code: 'EXPIRED-1', status: 'expired' }),
  sample({ id: 4, code: 'STOPPED-1', status: 'disabled' }),
])

if (rows[0]?.status !== 'available' || rows[0]?.statusLabel !== '可用' || rows[0]?.statusTone !== 'success') {
  throw new Error(`available redeem code should preserve raw status and expose localized label, got ${JSON.stringify(rows[0])}`)
}

if (rows[0]?.rewardLabel !== '积分 / 20.00000') {
  throw new Error(`redeem reward should localize points reward type, got ${rows[0]?.rewardLabel}`)
}

if (rows[0]?.validUntilLabel !== '2026/07/05 00:00') {
  throw new Error(`redeem code should format valid_until as stable date-time, got ${rows[0]?.validUntilLabel}`)
}

if (rows[0]?.statusAction?.status !== 'disabled' || rows[0]?.statusAction?.label !== '停用') {
  throw new Error(`available redeem code should only expose disable action, got ${JSON.stringify(rows[0]?.statusAction)}`)
}

if (rows[1]?.status !== 'redeemed' || rows[1]?.statusLabel !== '已核销' || rows[1]?.statusAction !== null) {
  throw new Error(`redeemed code must be terminal in admin UI, got ${JSON.stringify(rows[1])}`)
}

if (rows[2]?.statusLabel !== '已过期' || rows[2]?.statusTone !== 'danger' || rows[2]?.statusAction !== null) {
  throw new Error(`expired code must be terminal in admin UI, got ${JSON.stringify(rows[2])}`)
}

if (rows[3]?.statusAction?.status !== 'available' || rows[3]?.statusAction?.label !== '重新启用') {
  throw new Error(`disabled redeem code should expose re-enable action, got ${JSON.stringify(rows[3]?.statusAction)}`)
}

if (redeemStatusLabel(' inactive ') !== '未生效' || redeemStatusTone('inactive') !== 'warning') {
  throw new Error('inactive redeem status should be localized and highlighted as warning')
}

if (redeemStatusLabel('future_state') !== 'future_state' || redeemStatusTone('future_state') !== 'neutral') {
  throw new Error('unknown redeem status should preserve trimmed raw value with neutral tone')
}

if (redeemStatusAction('redeemed') !== null || redeemStatusAction('expired') !== null) {
  throw new Error('redeemed and expired statuses must not expose status mutation shortcuts')
}

if (redeemRewardTypeLabel('points') !== '积分' || redeemRewardTypeLabel('coupon') !== 'coupon') {
  throw new Error('reward type labels should localize known values and preserve unknown raw values')
}

const invalidTimeRows = redeemCodeRows([sample({ valid_until: 'bad-date' })])
if (invalidTimeRows[0]?.validUntilLabel !== 'bad-date') {
  throw new Error(`redeem code should preserve invalid valid_until for troubleshooting, got ${invalidTimeRows[0]?.validUntilLabel}`)
}

for (const rawStatus of ['available', 'disabled']) {
  if (!redeemStatusOptions.some((option) => option.value === rawStatus)) {
    throw new Error(`redeem status options should preserve raw value ${rawStatus}`)
  }
}

const optionLabels = redeemStatusOptions.map((option) => option.label).join(',')
if (optionLabels.includes('available') || optionLabels.includes('disabled')) {
  throw new Error(`redeem status options should show localized labels, got ${optionLabels}`)
}

const batchPayload = redeemBatchCreatePayload({
  count: '12',
  rewardValue: '8.50000',
  validDays: '15',
  maxRedemptions: '2',
}, '2026-06-05T00:00:00Z')
if (batchPayload.count !== 12 || batchPayload.reward_value !== '8.50000' || batchPayload.max_redemptions !== 2 || batchPayload.status !== 'available' || batchPayload.reward_type !== 'points') {
  throw new Error(`batch create payload should preserve operator inputs and fixed redeem semantics, got ${JSON.stringify(batchPayload)}`)
}
if (batchPayload.valid_until !== '2026-06-20T00:00:00.000Z') {
  throw new Error(`batch create payload should derive valid_until from valid days, got ${batchPayload.valid_until}`)
}
if (batchPayload.batch_id !== 0) {
  throw new Error(`batch create payload should let backend allocate a unique batch id, got ${batchPayload.batch_id}`)
}

for (const input of [
  { count: '0', rewardValue: '8.50000', validDays: '15', maxRedemptions: '1' },
  { count: '101', rewardValue: '8.50000', validDays: '15', maxRedemptions: '1' },
  { count: '10', rewardValue: '0', validDays: '15', maxRedemptions: '1' },
  { count: '10', rewardValue: '8.50000', validDays: '0', maxRedemptions: '1' },
  { count: '10', rewardValue: '8.50000', validDays: '15', maxRedemptions: '0' },
]) {
  let failed = false
  try {
    redeemBatchCreatePayload(input, '2026-06-05T00:00:00Z')
  } catch {
    failed = true
  }
  if (!failed) {
    throw new Error(`invalid batch create input should be rejected before API call, got ${JSON.stringify(input)}`)
  }
}

const exportCSV = redeemCodesCSV([
  sample({ code: 'WELCOME,20', status: 'available', reward_value: '20.00000', batch_id: 202606050001 }),
  sample({ id: 2, code: 'QUOTE"CODE', status: 'disabled', reward_value: '5.00000', batch_id: 202606050001, valid_until: 'bad-date' }),
])
if (!exportCSV.startsWith('兑换码,状态,奖励,批次,有效期,核销上限')) {
  throw new Error(`redeem export should include operator-facing headers, got ${exportCSV}`)
}
if (!exportCSV.includes('"WELCOME,20",可用,积分 / 20.00000,202606050001,2026/07/05 00:00,1')) {
  throw new Error(`redeem export should use localized row labels and escape commas, got ${exportCSV}`)
}
if (!exportCSV.includes('"QUOTE""CODE",已停用,积分 / 5.00000,202606050001,bad-date,1')) {
  throw new Error(`redeem export should escape quotes and preserve invalid dates for troubleshooting, got ${exportCSV}`)
}
if (/2026-07-05T00:00:00Z/.test(exportCSV)) {
  throw new Error(`redeem export should not expose raw ISO timestamps, got ${exportCSV}`)
}

if (redeemCodesCSV([]) !== '兑换码,状态,奖励,批次,有效期,核销上限') {
  throw new Error('empty redeem export should still contain CSV headers')
}

const exportFilename = redeemExportFilename(202606050001)
if (exportFilename !== 'redeem-codes-202606050001.csv') {
  throw new Error(`redeem export filename should include batch id, got ${exportFilename}`)
}
if (redeemExportFilename('all') !== 'redeem-codes-all.csv') {
  throw new Error(`redeem export filename should support audited all-code exports, got ${redeemExportFilename('all')}`)
}
if (API_PATHS.ops.redeemCodesExport !== '/api/ops/admin/v1/redeem-codes:export') {
  throw new Error(`redeem export should use audited backend endpoint, got ${API_PATHS.ops.redeemCodesExport}`)
}

const redemptionRows = redeemRedemptionRows([
  ledger({
    id: 11,
    user_id: 42,
    ledger_type: 'redeem',
    change_points: '6.00000',
    balance_after: '26.00000',
    source_type: 'redeem_code',
    source_id: 1,
    created_at: '2026-06-05T08:09:00Z',
    reason: 'WELCOME-20',
  }),
  ledger({
    id: 12,
    user_id: 51,
    ledger_type: 'custom_redeem',
    change_points: '-1.00000',
    balance_after: '0.00000',
    created_at: 'bad-date',
  }),
])
if (redemptionRows[0]?.userLabel !== '用户 #42' || redemptionRows[0]?.typeLabel !== '兑换码到账' || redemptionRows[0]?.amount !== '+6.00000') {
  throw new Error(`redeem redemptions should expose operator-facing user/type/amount labels, got ${JSON.stringify(redemptionRows[0])}`)
}
if (redemptionRows[0]?.occurredAt !== '2026/06/05 08:09' || redemptionRows[0]?.balanceAfter !== '26.00000') {
  throw new Error(`redeem redemptions should format time and balance, got ${JSON.stringify(redemptionRows[0])}`)
}
if (redemptionRows[0]?.sourceLabel !== '兑换码 #1' || redemptionRows[0]?.detail !== 'WELCOME-20') {
  throw new Error(`redeem redemptions should expose source and reason, got ${JSON.stringify(redemptionRows[0])}`)
}
if (redemptionRows[1]?.typeLabel !== 'custom_redeem' || redemptionRows[1]?.amountTone !== 'debit' || redemptionRows[1]?.occurredAt !== 'bad-date') {
  throw new Error(`redeem redemptions should preserve unknown types and invalid time for troubleshooting, got ${JSON.stringify(redemptionRows[1])}`)
}

function ledger(patch: Partial<LedgerEntry>): LedgerEntry {
  return {
    id: patch.id ?? 1,
    user_id: patch.user_id,
    ledger_type: patch.ledger_type,
    change_points: patch.change_points,
    balance_after: patch.balance_after,
    source_type: patch.source_type,
    source_id: patch.source_id,
    reason: patch.reason,
    created_at: patch.created_at,
  }
}

for (const primitive of ['PageHeader', 'FilterToolbar', 'DataTable', 'Badge', 'InlineFeedback', 'ActionMenu', 'Modal', 'Pager']) {
  if (!redeemPageSource.includes(`<${primitive}`)) {
    throw new Error(`redeem operations must use the shared ${primitive} primitive`)
  }
}

for (const apiContract of [
  'adminApi.listRedeemCodes',
  'adminApi.createRedeemCode',
  'adminApi.batchCreateRedeemCodes',
  'adminApi.exportRedeemCodes',
  'adminApi.updateRedeemCodeStatus',
  'adminApi.listRedeemCodeRedemptions',
]) {
  if (!redeemPageSource.includes(apiContract)) {
    throw new Error(`redeem redesign must preserve ${apiContract}`)
  }
}

if (!redeemPageSource.includes("redeemPrimaryActionLabel = '查看核销'")) {
  throw new Error('redeem rows must expose one persistent primary action')
}

if (!redeemPageSource.includes("id: 'change-status'")) {
  throw new Error('redeem status mutation must move into the row ActionMenu')
}

for (const drift of ['adminDataGrid', 'adminGridCols', 'uppercase', 'tracking-[', 'text-[var(--green)]', 'rounded-2xl', 'rounded-3xl']) {
  if (redeemPageSource.includes(drift)) {
    throw new Error(`redeem page must remove visual drift: ${drift}`)
  }
}
