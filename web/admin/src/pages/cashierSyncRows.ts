import type { PaymentOrderSyncResult } from '../../../shared/api-types'

export type CashierSyncTone = 'success' | 'warning' | 'danger' | 'neutral'

export type CashierSyncRow = {
  statusLabel: string
  categoryLabel: string
  actionHint: string
  tone: CashierSyncTone
}

const statusLabels: Record<string, string> = {
  pending: '等待支付',
  paid: '已支付',
  closed: '已关闭',
  failed: '渠道异常',
  refunded: '已退款',
}

export function cashierSyncRow(sync: PaymentOrderSyncResult): CashierSyncRow {
  const category = sync.risk_category || inferSyncCategory(sync)
  return {
    statusLabel: statusLabels[sync.query_status] ?? (sync.query_status || '-'),
    categoryLabel: syncCategoryLabel(category),
    actionHint: sync.action_hint || syncCategoryActionHint(category, sync),
    tone: syncCategoryTone(category, sync.query_status),
  }
}

function inferSyncCategory(sync: PaymentOrderSyncResult) {
  const raw = sync.raw ?? {}
  const rawText = [
    sync.message,
    raw.status,
    raw.trade_status,
    raw.trade_state,
    raw.error_code,
    raw.err_code,
    raw.sub_code,
  ].map((value) => String(value ?? '').toLowerCase()).join(' ')
  if (/amount[_\s-]?mismatch|money[_\s-]?mismatch|total[_\s-]?amount[_\s-]?mismatch|金额不一致/.test(rawText)) return 'amount_mismatch'
  if (/sign|signature|verify[_\s-]?failed|验签|签名/.test(rawText)) return 'signature_error'
  if (/merchant[_\s-]?disabled|mch[_\s-]?disabled|account[_\s-]?(disabled|abnormal)|merchant[_\s-]?abnormal|商户.*异常|账号.*异常/.test(rawText)) return 'account_abnormal'
  if (/timeout|timed[_\s-]?out|network|gateway[_\s-]?timeout|超时|网络/.test(rawText)) return 'channel_timeout'
  if (/limit|quota|frequency|rate[_\s-]?limited|限额|超限|频率/.test(rawText)) return 'channel_limited'
  if (/risk|fraud|intercept|security|风控|风险/.test(rawText)) return 'risk_control'
  if (sync.query_status === 'paid') return 'paid'
  if (sync.query_status === 'closed') return 'closed'
  if (sync.query_status === 'refunded') return 'refunded'
  if (sync.query_status === 'failed') return 'channel_error'
  return 'pending'
}

function syncCategoryLabel(category: string) {
  if (category === 'channel_limited') return '渠道限额'
  if (category === 'signature_error') return '签名配置异常'
  if (category === 'amount_mismatch') return '金额不一致'
  if (category === 'account_abnormal') return '商户账号异常'
  if (category === 'channel_timeout') return '查单超时'
  if (category === 'risk_control') return '风控拦截'
  if (category === 'paid') return '渠道已支付'
  if (category === 'closed') return '渠道已关闭'
  if (category === 'refunded') return '渠道已退款'
  if (category === 'channel_error') return '渠道处理异常'
  if (category === 'pending') return '等待用户支付'
  return category || '-'
}

function syncCategoryActionHint(category: string, sync: PaymentOrderSyncResult) {
  if (category === 'channel_limited') return '渠道订单触发限额限制，建议切换备用渠道、降低单笔金额或调整渠道实例限额后再重试。'
  if (category === 'signature_error') return '渠道验签或签名配置异常，请检查商户密钥、证书、公钥、回调地址和签名算法配置。'
  if (category === 'amount_mismatch') return '渠道订单金额与本地订单不一致，请暂停到账并核对订单金额、汇率、渠道费率和回调原文。'
  if (category === 'account_abnormal') return '渠道商户账号状态异常，建议切换备用账号并登录渠道后台确认商户状态和产品权限。'
  if (category === 'channel_timeout') return '渠道查单超时或网络异常，建议稍后重试；连续失败时检查网关地址、网络出口和渠道可用性。'
  if (category === 'risk_control') return '渠道侧风控或安全策略拦截，建议让用户更换支付渠道或重新创建订单后再支付。'
  if (category === 'paid') return sync.completed ? '本次查单已完成本地到账；可刷新订单和用户余额确认。' : '渠道已确认支付，本地已存在到账记录，本次不会重复入账。'
  if (category === 'closed') return '渠道订单已关闭，建议取消当前订单并让用户重新创建订单。'
  if (category === 'refunded') return '渠道显示已退款，请核对本地退款流水和用户充值余额是否一致。'
  if (category === 'channel_error') return '渠道返回异常状态，请结合原始响应、商户后台和回调事件继续排查。'
  return sync.message || '渠道仍未确认支付，稍后可再次查单。'
}

function syncCategoryTone(category: string, status: string): CashierSyncTone {
  if (category === 'paid' || status === 'paid') return 'success'
  if (
    category === 'risk_control'
    || category === 'channel_error'
    || category === 'channel_limited'
    || category === 'signature_error'
    || category === 'amount_mismatch'
    || category === 'account_abnormal'
    || category === 'channel_timeout'
    || status === 'failed'
  ) return 'danger'
  if (category === 'closed' || category === 'refunded' || status === 'closed' || status === 'refunded') return 'warning'
  return 'neutral'
}
