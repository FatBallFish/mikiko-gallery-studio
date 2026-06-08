import type { LedgerEntry } from '../../../shared/api-types'

export type UserDetailLedgerRow = {
  id: LedgerEntry['id']
  title: string
  bucketLabel: string
  sourceLabel: string
  amount: string
  amountTone: 'credit' | 'debit'
  balanceAfter: string
  expiryText: string
  occurredAt: string
}

export function userDetailLedgerRows(items: LedgerEntry[]): UserDetailLedgerRow[] {
  return items.map((item) => {
    const amount = item.amount ?? formatLedgerAmount(item.change_points)
    return {
      id: item.id,
      title: item.title ?? ledgerTypeLabel(item.ledger_type),
      bucketLabel: balanceBucketLabel(item.balance_bucket ?? item.bucket_type ?? 'recharge'),
      sourceLabel: ledgerSourceLabel(item.source_type, item.source_id),
      amount,
      amountTone: item.type ?? (amount.startsWith('-') ? 'debit' : 'credit'),
      balanceAfter: item.bucket_balance_after ?? item.balance_after ?? '-',
      expiryText: ledgerExpiryText(item.expires_at),
      occurredAt: formatDateTime(item.occurred_at ?? item.created_at),
    }
  })
}

function ledgerTypeLabel(type?: string) {
  if (type === 'trial_grant') return '体验额度发放'
  if (type === 'recharge' || type === 'order_paid') return '充值到账'
  if (type === 'payment_refund') return '支付退款'
  if (type === 'redeem') return '兑换码到账'
  if (type === 'reserve') return '生成预冻结'
  if (type === 'consume') return '生成扣费'
  if (type === 'refund') return '冻结退回'
  if (type === 'expire') return '额度过期'
  if (type === 'admin_adjust') return '后台积分调整'
  return type || '积分变动'
}

function balanceBucketLabel(bucket: string) {
  if (bucket === 'trial') return '体验额度'
  if (bucket === 'subscription') return '订阅额度'
  if (bucket === 'recharge') return '充值额度'
  if (bucket === 'gift') return '赠送额度'
  return bucket
}

function ledgerSourceLabel(source?: string, sourceID?: string | number | null) {
  const suffix = sourceID === undefined || sourceID === null || sourceID === '' ? '' : ` #${sourceID}`
  if (source === 'signup') return `注册赠送${suffix}`
  if (source === 'payment_order') return `支付订单${suffix}`
  if (source === 'redeem_code') return `兑换码${suffix}`
  if (source === 'task') return `生图任务${suffix}`
  if (source === 'admin' || source === 'admin_adjust') return `后台调整${suffix}`
  if (source === 'subscription') return `订阅${suffix}`
  return `${source || '系统'}${suffix}`
}

function formatLedgerAmount(value?: string) {
  if (!value) return '0.00000'
  if (value.startsWith('-') || value.startsWith('+')) return value
  return `+${value}`
}

function ledgerExpiryText(expiresAt?: string | null) {
  if (!expiresAt) return '长期有效'
  return `有效期至 ${formatDate(expiresAt)}`
}

function formatDateTime(value?: string | null) {
  const input = value ?? ''
  const match = input.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!match) return input || '-'
  return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
}

function formatDate(value: string) {
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})/)
  if (!match) return value
  return `${match[1]}/${match[2]}/${match[3]}`
}
