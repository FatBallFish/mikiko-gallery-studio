import type { Balance, BalanceBucket, ID, LedgerEntry } from '../../../shared/api-types'

export function normalizeBalanceBuckets(balance: Balance | null): BalanceBucket[] {
  const defaults: BalanceBucket[] = [
    {
      bucket: 'trial',
      label: '体验额度',
      available_points: balance?.trial_points ?? '0.00000',
      expire_warning: Boolean(balance?.next_expiring_grant?.grant_type === 'trial'),
      expires_at: balance?.next_expiring_grant?.expires_at,
    },
    { bucket: 'subscription', label: '订阅额度', available_points: balance?.subscription_points ?? '0.00000' },
    { bucket: 'recharge', label: '充值额度', available_points: balance?.recharge_points ?? '0.00000' },
  ]
  const serverBuckets = new Map((balance?.buckets ?? []).map((bucket) => [bucket.bucket, bucket]))
  return defaults.map((bucket) => ({ ...bucket, ...serverBuckets.get(bucket.bucket) }))
}

export function balanceBucketLabel(bucket: string) {
  if (bucket === 'trial') return '体验额度'
  if (bucket === 'subscription') return '订阅额度'
  if (bucket === 'recharge') return '充值额度'
  if (bucket === 'gift') return '赠送额度'
  return bucket
}

function ledgerSourceLabel(source?: string) {
  if (source === 'signup') return '注册赠送'
  if (source === 'payment_order') return '支付订单'
  if (source === 'redeem_code') return '兑换码'
  if (source === 'task') return '生图任务'
  if (source === 'admin') return '后台调整'
  if (source === 'subscription') return '订阅'
  return source || '系统'
}

function ledgerTitle(type?: string) {
  if (type === 'trial_grant') return '体验额度发放'
  if (type === 'order_paid') return '充值额度到账'
  if (type === 'recharge') return '充值额度到账'
  if (type === 'payment_refund') return '支付退款'
  if (type === 'redeem') return '兑换码到账'
  if (type === 'reserve') return '生成预冻结'
  if (type === 'consume') return '生成扣费'
  if (type === 'refund') return '冻结退回'
  if (type === 'expire') return '额度过期'
  if (type === 'admin_adjust') return '后台积分调整'
  return '积分变动'
}

function formatLedgerAmount(value?: string) {
  if (!value) return '0.00000'
  if (value.startsWith('-') || value.startsWith('+')) return value
  return `+${value}`
}

export type ProfileLedgerRow = {
  id: ID
  title: string
  bucketLabel: string
  ledgerTypeLabel: string
  sourceLabel: string
  expiryText: string
  occurredAt: string
  detail: string
  amount: string
  amountTone: 'credit' | 'debit'
}

export function profileLedgerRows(entries: LedgerEntry[]): ProfileLedgerRow[] {
  return entries.map((entry) => {
    const change = entry.change_points ?? entry.amount ?? '0.00000'
    const amount = entry.amount ?? formatLedgerAmount(change)
    const amountTone = entry.type ?? (amount.startsWith('-') ? 'debit' : 'credit')
    const sourceId = entry.source_id === undefined || entry.source_id === null || entry.source_id === '' ? '' : ` #${entry.source_id}`
    const detail = entry.detail || entry.reason || `${entry.ledger_type ?? 'ledger'}${sourceId}`
    return {
      id: entry.id,
      title: entry.title || ledgerTitle(entry.ledger_type),
      bucketLabel: balanceBucketLabel(entry.balance_bucket ?? entry.bucket_type ?? 'recharge'),
      ledgerTypeLabel: ledgerTitle(entry.ledger_type),
      sourceLabel: ledgerSourceLabel(entry.source_type),
      expiryText: ledgerExpiryText(entry.expires_at),
      occurredAt: ledgerDateTime(entry.occurred_at ?? entry.created_at),
      detail,
      amount,
      amountTone,
    }
  })
}

function ledgerExpiryText(expiresAt?: string | null) {
  if (!expiresAt) return '长期有效'
  return `有效期至 ${ledgerDate(expiresAt)}`
}

export function bucketExpiryText(bucket: BalanceBucket) {
  if (!bucket.expires_at) return '长期有效'
  const formatted = ledgerDate(bucket.expires_at)
  return bucket.expire_warning ? `即将过期：${formatted}` : `有效期至 ${formatted}`
}

function ledgerDate(value: string) {
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})/)
  if (!match) return value
  return `${match[1]}/${match[2]}/${match[3]}`
}

function ledgerDateTime(value?: string | null) {
  const input = value ?? ''
  const match = input.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!match) return input || '-'
  return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
}
