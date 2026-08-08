import type { LedgerEntry } from '../../../shared/api-types'
import { profileLedgerRows } from './profileBalanceModel'

const rows = profileLedgerRows([
  {
    id: 1,
    ledger_type: 'trial_grant',
    balance_bucket: 'trial',
    change_points: '20.00000',
    source_type: 'signup',
    source_id: 'signup-1',
    expires_at: '2026-06-12T00:00:00Z',
    created_at: '2026-06-05T00:00:00Z',
  } as LedgerEntry,
  {
    id: 2,
    ledger_type: 'recharge',
    bucket_type: 'recharge',
    change_points: '50.00000',
    source_type: 'payment_order',
    source_id: 'ord_10001',
    expires_at: null,
    created_at: '2026-06-05T01:00:00Z',
    title: '充值到账',
    occurred_at: '2026-06-05T01:00:00Z',
    amount: '+50.00000',
    type: 'credit',
    detail: '订单 ord_10001',
  },
])

if (rows[0]?.bucketLabel !== '体验额度' || rows[0]?.sourceLabel !== '注册赠送') {
  throw new Error('profile ledger rows should label backend balance_bucket and source_type')
}

if (rows[0]?.expiryText !== '有效期至 2026/06/12') {
  throw new Error(`profile ledger rows should expose formatted expires_at, got ${rows[0]?.expiryText}`)
}

if (rows[0]?.occurredAt !== '2026/06/05 00:00') {
  throw new Error(`profile ledger rows should format created_at as stable date-time, got ${rows[0]?.occurredAt}`)
}

if (rows[0]?.amount !== '+20.00000' || rows[0]?.amountTone !== 'credit') {
  throw new Error('profile ledger rows should format positive change_points as credit amount')
}

if (rows[1]?.bucketLabel !== '充值积分' || rows[1]?.expiryText !== '长期有效') {
  throw new Error('profile ledger rows should keep bucket_type compatibility and mark recharge as never expiring')
}

if (rows[1]?.occurredAt !== '2026/06/05 01:00') {
  throw new Error(`profile ledger rows should prefer occurred_at and format it, got ${rows[1]?.occurredAt}`)
}

const invalidTime = profileLedgerRows([{ id: 3, ledger_type: 'admin_adjust', change_points: '-1.00000', created_at: 'bad-date' } as LedgerEntry])
if (invalidTime[0]?.occurredAt !== 'bad-date') {
  throw new Error(`profile ledger rows should preserve invalid occurred time for troubleshooting, got ${invalidTime[0]?.occurredAt}`)
}

const generation = profileLedgerRows([{
  id: 4,
  ledger_type: 'consume',
  change_points: '-12.00000',
  task_id: 'task-partial',
  successful_image_count: 3,
  effective_unit_points: '4.00000',
  total_charged_points: '12.00000',
  partial_success: true,
  created_at: '2026-06-05T02:00:00Z',
} as LedgerEntry])[0]
if (generation.generationDetail !== '成功 3 张 · 单价 4.00000 积分/张 · 合计 12.00000 积分 · 部分成功' || generation.taskId !== 'task-partial') {
  throw new Error(`generation ledger must expose count, unit price, total, partial state, and task id, got ${JSON.stringify(generation)}`)
}

const legacyGiftExpiry = profileLedgerRows([{
  id: 5,
  ledger_type: 'expire',
  balance_bucket: 'gift',
  change_points: '-5.00000',
  source_type: 'payment_order_bonus',
  source_id: 42,
  order_id: 42,
  created_at: '2026-06-05T03:00:00Z',
} as LedgerEntry])[0]
if (legacyGiftExpiry.sourceLabel !== '支付订单') {
  throw new Error(`gift expiry must not expose the internal payment_order_bonus source, got ${JSON.stringify(legacyGiftExpiry)}`)
}
