import type { LedgerEntry, RedeemCode, RedeemCodeBatchCreateRequest } from '../../../shared/api-types'

export type RedeemStatusTone = 'success' | 'warning' | 'danger' | 'neutral'

export type RedeemStatusOption = {
  value: string
  label: string
}

export type RedeemStatusAction = {
  status: string
  label: string
}

export type RedeemCodeRowModel = {
  id: number
  code: string
  status: string
  statusLabel: string
  statusTone: RedeemStatusTone
  rewardLabel: string
  batchLabel: string
  validUntilLabel: string
  redeemedLabel: string
  statusAction: RedeemStatusAction | null
}

export type RedeemBatchCreateForm = {
  count: string
  rewardValue: string
  validDays: string
  maxRedemptions: string
}

export type RedeemRedemptionRowModel = {
  id: LedgerEntry['id']
  userLabel: string
  typeLabel: string
  amount: string
  amountTone: 'credit' | 'debit'
  balanceAfter: string
  sourceLabel: string
  detail: string
  occurredAt: string
}

export const redeemStatusOptions: RedeemStatusOption[] = [
  { value: 'available', label: '可用' },
  { value: 'disabled', label: '已停用' },
]

export function redeemCodeRows(codes: RedeemCode[]): RedeemCodeRowModel[] {
  return codes.map((code) => ({
    id: code.id,
    code: code.code,
    status: code.status,
    statusLabel: redeemStatusLabel(code.status),
    statusTone: redeemStatusTone(code.status),
    rewardLabel: `${redeemRewardTypeLabel(code.reward_type)} / ${code.reward_value}`,
    batchLabel: String(code.batch_id),
    validUntilLabel: redeemDateTimeLabel(code.valid_until),
    redeemedLabel: `${code.redeemed_count}/${code.max_redemptions}`,
    statusAction: redeemStatusAction(code.status),
  }))
}

export function redeemStatusLabel(status?: string | null) {
  const normalized = normalizeStatus(status)
  if (normalized === 'inactive') return '未生效'
  if (normalized === 'available') return '可用'
  if (normalized === 'redeemed') return '已核销'
  if (normalized === 'expired') return '已过期'
  if (normalized === 'disabled') return '已停用'
  return normalized || '未知状态'
}

export function redeemStatusTone(status?: string | null): RedeemStatusTone {
  const normalized = normalizeStatus(status)
  if (normalized === 'available') return 'success'
  if (normalized === 'inactive') return 'warning'
  if (normalized === 'redeemed' || normalized === 'expired' || normalized === 'disabled') return 'danger'
  return 'neutral'
}

export function redeemStatusAction(status?: string | null): RedeemStatusAction | null {
  const normalized = normalizeStatus(status)
  if (normalized === 'available' || normalized === 'inactive') return { status: 'disabled', label: '停用' }
  if (normalized === 'disabled') return { status: 'available', label: '重新启用' }
  return null
}

export function redeemRewardTypeLabel(type?: string | null) {
  const normalized = (type ?? '').trim().toLowerCase()
  if (normalized === 'points') return '积分'
  return normalized || '未知奖励'
}

export function redeemBatchCreatePayload(input: RedeemBatchCreateForm, now: Date | string = new Date()): RedeemCodeBatchCreateRequest {
  const count = parsePositiveInteger(input.count, '生成数量')
  if (count > 100) throw new Error('生成数量不能超过 100 个')
  const rewardValue = input.rewardValue.trim()
  if (!positiveDecimal(rewardValue)) throw new Error('奖励积分必须大于 0')
  const validDays = parsePositiveInteger(input.validDays, '有效天数')
  const maxRedemptions = parsePositiveInteger(input.maxRedemptions, '每码可核销次数')
  const validFrom = dateFrom(now)
  const validUntil = new Date(validFrom.getTime() + validDays * 86400_000)
  return {
    count,
    batch_id: 0,
    status: 'available',
    reward_type: 'points',
    reward_value: rewardValue,
    valid_from: validFrom.toISOString(),
    valid_until: validUntil.toISOString(),
    max_redemptions: maxRedemptions,
  }
}

export function redeemCodesCSV(codes: RedeemCode[]) {
  const header = ['兑换码', '状态', '奖励', '批次', '有效期', '核销上限']
  const body = redeemCodeRows(codes).map((row, index) => [
    row.code,
    row.statusLabel,
    row.rewardLabel,
    row.batchLabel,
    row.validUntilLabel,
    codes[index]?.max_redemptions ?? '',
  ].map(csvCell).join(','))
  return [header.join(','), ...body].join('\n')
}

export function redeemExportFilename(batchID: number | string) {
  return `redeem-codes-${String(batchID).trim() || 'batch'}.csv`
}

export function redeemRedemptionRows(items: LedgerEntry[]): RedeemRedemptionRowModel[] {
  return items.map((item) => {
    const amount = item.amount ?? formatLedgerAmount(item.change_points)
    return {
      id: item.id,
      userLabel: item.user_id ? `用户 #${item.user_id}` : '用户 -',
      typeLabel: redeemLedgerTypeLabel(item.ledger_type),
      amount,
      amountTone: item.type ?? (amount.startsWith('-') ? 'debit' : 'credit'),
      balanceAfter: item.balance_after ?? '-',
      sourceLabel: redeemSourceLabel(item.source_type, item.source_id),
      detail: item.reason || item.detail || item.title || '-',
      occurredAt: redeemDateTimeLabel(item.occurred_at ?? item.created_at),
    }
  })
}

function normalizeStatus(status?: string | null) {
  return (status ?? '').trim().toLowerCase()
}

function redeemDateTimeLabel(value?: string | null) {
  const input = value ?? ''
  const match = input.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!match) return input || '-'
  return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
}

function parsePositiveInteger(value: string, label: string) {
  const parsed = Number(value)
  if (!Number.isInteger(parsed) || parsed <= 0) throw new Error(`${label}必须为正整数`)
  return parsed
}

function positiveDecimal(value: string) {
  const parsed = Number(value)
  return value !== '' && Number.isFinite(parsed) && parsed > 0
}

function dateFrom(value: Date | string) {
  const date = typeof value === 'string' ? new Date(value) : value
  if (Number.isNaN(date.getTime())) throw new Error('当前时间无效')
  return date
}

function csvCell(value: string | number) {
  const text = String(value)
  if (!/[",\n\r]/.test(text)) return text
  return `"${text.replace(/"/g, '""')}"`
}

function redeemLedgerTypeLabel(type?: string) {
  if (type === 'redeem') return '兑换码到账'
  return type || '核销记录'
}

function redeemSourceLabel(source?: string, sourceID?: string | number | null) {
  const suffix = sourceID === undefined || sourceID === null || sourceID === '' ? '' : ` #${sourceID}`
  if (source === 'redeem_code') return `兑换码${suffix}`
  if (source === 'admin' || source === 'admin_adjust') return `后台调整${suffix}`
  if (source === 'payment_order') return `支付订单${suffix}`
  if (source === 'task') return `生图任务${suffix}`
  if (source === 'signup') return `注册赠送${suffix}`
  return `${source || '系统'}${suffix}`
}

function formatLedgerAmount(value?: string) {
  if (!value) return '0.00000'
  if (value.startsWith('-') || value.startsWith('+')) return value
  return `+${value}`
}
