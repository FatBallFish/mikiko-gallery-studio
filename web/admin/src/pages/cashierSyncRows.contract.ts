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

const limited = cashierSyncRow({
  provider_type: 'jeepay_wxpay',
  query_status: 'failed',
  risk_category: 'channel_limited',
  paid: false,
  completed: false,
  message: '渠道订单触发限额限制',
  raw: { status: 'limited' },
  synced_at: '2026-06-05T10:00:00Z',
} satisfies PaymentOrderSyncResult)

if (limited.statusLabel !== '渠道异常' || limited.categoryLabel !== '渠道限额' || limited.tone !== 'danger' || !limited.actionHint.includes('切换备用渠道')) {
  throw new Error(`limited sync result should guide provider fallback, got ${JSON.stringify(limited)}`)
}

const signatureError = cashierSyncRow({
  provider_type: 'alipay_direct',
  query_status: 'failed',
  paid: false,
  completed: false,
  message: 'SIGN_ERROR',
  raw: { sub_code: 'SIGN_ERROR' },
  synced_at: '2026-06-05T10:00:00Z',
} satisfies PaymentOrderSyncResult)

if (signatureError.categoryLabel !== '签名配置异常' || signatureError.tone !== 'danger' || !signatureError.actionHint.includes('检查商户密钥')) {
  throw new Error(`signature sync result should infer config guidance from raw payload, got ${JSON.stringify(signatureError)}`)
}

const amountMismatch = cashierSyncRow({
  provider_type: 'mock',
  query_status: 'failed',
  risk_category: 'amount_mismatch',
  paid: false,
  completed: false,
  message: '渠道订单金额与本地订单不一致',
  raw: { status: 'amount_mismatch' },
  synced_at: '2026-06-05T10:00:00Z',
} satisfies PaymentOrderSyncResult)

if (amountMismatch.categoryLabel !== '金额不一致' || amountMismatch.tone !== 'danger' || !amountMismatch.actionHint.includes('暂停到账')) {
  throw new Error(`amount mismatch sync result should block fulfillment guidance, got ${JSON.stringify(amountMismatch)}`)
}

const accountAbnormal = cashierSyncRow({
  provider_type: 'wxpay_direct',
  query_status: 'failed',
  risk_category: 'account_abnormal',
  paid: false,
  completed: false,
  message: '渠道商户账号状态异常',
  raw: { err_code: 'merchant_disabled' },
  synced_at: '2026-06-05T10:00:00Z',
} satisfies PaymentOrderSyncResult)

if (accountAbnormal.categoryLabel !== '商户账号异常' || accountAbnormal.tone !== 'danger' || !accountAbnormal.actionHint.includes('切换备用账号')) {
  throw new Error(`account abnormal sync result should guide account fallback, got ${JSON.stringify(accountAbnormal)}`)
}

const timeout = cashierSyncRow({
  provider_type: 'easypay_alipay',
  query_status: 'failed',
  risk_category: 'channel_timeout',
  paid: false,
  completed: false,
  message: '渠道查单超时或网络异常',
  raw: { error_code: 'gateway_timeout' },
  synced_at: '2026-06-05T10:00:00Z',
} satisfies PaymentOrderSyncResult)

if (timeout.categoryLabel !== '查单超时' || timeout.tone !== 'danger' || !timeout.actionHint.includes('稍后重试')) {
  throw new Error(`timeout sync result should guide retry and network checks, got ${JSON.stringify(timeout)}`)
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
  limited.statusLabel,
  limited.categoryLabel,
  limited.actionHint,
  signatureError.statusLabel,
  signatureError.categoryLabel,
  signatureError.actionHint,
  amountMismatch.statusLabel,
  amountMismatch.categoryLabel,
  amountMismatch.actionHint,
  accountAbnormal.statusLabel,
  accountAbnormal.categoryLabel,
  accountAbnormal.actionHint,
  timeout.statusLabel,
  timeout.categoryLabel,
  timeout.actionHint,
].join(' ')

if (/risk_control|channel_limited|signature_error|amount_mismatch|account_abnormal|channel_timeout|RISK_CONTROL|SIGN_ERROR|TRADE_CLOSED|SUCCESS|query_status|TODO|后续|暂不/.test(visibleCopy)) {
  throw new Error(`sync row visible copy should not expose raw channel status or roadmap wording, got ${visibleCopy}`)
}
