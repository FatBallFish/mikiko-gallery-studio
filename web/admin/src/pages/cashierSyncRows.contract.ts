import type { PaymentOrderSyncResult } from '../../../shared/api-types'
import { cashierSyncRow } from './cashierSyncRows'

const failedRisk = cashierSyncRow({
  provider_type: 'mock',
  query_status: 'failed',
  paid: false,
  completed: false,
  message: 'risk_control',
  raw: { status: 'risk_control', error_code: 'RISK_CONTROL' },
  synced_at: '2026-06-05T10:00:00Z',
} satisfies PaymentOrderSyncResult)

if (failedRisk.statusLabel !== '渠道异常' || failedRisk.categoryLabel !== '风控拦截' || failedRisk.tone !== 'danger' || !failedRisk.actionHint.includes('更换支付渠道')) {
  throw new Error(`risk-control sync result should guide channel switching, got ${JSON.stringify(failedRisk)}`)
}

const closed = cashierSyncRow({
  provider_type: 'alipay_direct',
  query_status: 'closed',
  paid: false,
  completed: false,
  message: 'TRADE_CLOSED',
  raw: { trade_status: 'TRADE_CLOSED' },
  synced_at: '2026-06-05T10:00:00Z',
} satisfies PaymentOrderSyncResult)

if (closed.statusLabel !== '已关闭' || closed.categoryLabel !== '渠道已关闭' || closed.tone !== 'warning' || !closed.actionHint.includes('重新创建订单')) {
  throw new Error(`closed sync result should guide order recreation, got ${JSON.stringify(closed)}`)
}

const paid = cashierSyncRow({
  provider_type: 'wxpay_direct',
  query_status: 'paid',
  paid: true,
  completed: true,
  message: 'SUCCESS',
  raw: { trade_state: 'SUCCESS' },
  synced_at: '2026-06-05T10:00:00Z',
} satisfies PaymentOrderSyncResult)

if (paid.statusLabel !== '已支付' || paid.categoryLabel !== '渠道已支付' || paid.tone !== 'success' || !paid.actionHint.includes('到账')) {
  throw new Error(`paid sync result should explain fulfillment status, got ${JSON.stringify(paid)}`)
}

const visibleCopy = [
  failedRisk.statusLabel,
  failedRisk.categoryLabel,
  failedRisk.actionHint,
  closed.statusLabel,
  closed.categoryLabel,
  closed.actionHint,
  paid.statusLabel,
  paid.categoryLabel,
  paid.actionHint,
].join(' ')

if (/risk_control|RISK_CONTROL|TRADE_CLOSED|SUCCESS|query_status|TODO|后续|暂不/.test(visibleCopy)) {
  throw new Error(`sync row visible copy should not expose raw channel status or roadmap wording, got ${visibleCopy}`)
}
