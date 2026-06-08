import type { LedgerEntry } from '../../../shared/api-types'
import { userDetailLedgerRows } from './userDetailLedgerRows'

const rows = userDetailLedgerRows([
  {
    id: 1,
    ledger_type: 'trial_grant',
    balance_bucket: 'trial',
    change_points: '20.00000',
    balance_after: '20.00000',
    bucket_balance_after: '20.00000',
    source_type: 'signup',
    expires_at: '2026-06-12T00:00:00Z',
    created_at: '2026-06-05T00:00:00Z',
  },
  {
    id: 2,
    ledger_type: 'payment_refund',
    bucket_type: 'recharge',
    change_points: '-10.00000',
    balance_after: '10.00000',
    source_type: 'payment_order',
    source_id: 'ord_10001',
    expires_at: null,
    created_at: '2026-06-05T01:00:00Z',
  },
] satisfies LedgerEntry[])

if (rows[0]?.bucketLabel !== '体验额度' || rows[0]?.sourceLabel !== '注册赠送') {
  throw new Error(`admin user detail ledger should label bucket/source, got ${JSON.stringify(rows[0])}`)
}

if (rows[0]?.expiryText !== '有效期至 2026/06/12') {
  throw new Error(`admin user detail ledger should format expires_at, got ${rows[0]?.expiryText}`)
}

if (rows[0]?.occurredAt !== '2026/06/05 00:00') {
  throw new Error(`admin user detail ledger should format created_at as stable date-time, got ${rows[0]?.occurredAt}`)
}

if (rows[1]?.bucketLabel !== '充值额度' || rows[1]?.expiryText !== '长期有效') {
  throw new Error(`admin user detail ledger should keep bucket_type compatibility, got ${JSON.stringify(rows[1])}`)
}

if (rows[1]?.amountTone !== 'debit' || rows[1]?.amount !== '-10.00000') {
  throw new Error(`admin user detail ledger should preserve negative amount tone, got ${JSON.stringify(rows[1])}`)
}

const invalidTime = userDetailLedgerRows([{
  id: 3,
  ledger_type: 'admin_adjust',
  balance_bucket: 'recharge',
  change_points: '-1.00000',
  created_at: 'bad-date',
} as LedgerEntry])
if (invalidTime[0]?.occurredAt !== 'bad-date') {
  throw new Error(`admin user detail ledger should preserve invalid occurred time for troubleshooting, got ${invalidTime[0]?.occurredAt}`)
}
