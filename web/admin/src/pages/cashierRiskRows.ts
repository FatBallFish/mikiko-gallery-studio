import type { PaymentOrder, PaymentWebhookEvent } from '../../../shared/api-types'

export type CashierRiskTone = 'success' | 'warning' | 'danger' | 'neutral'

export type CashierRiskRow = {
  key: string
  label: string
  value: string
  detail: string
  tone: CashierRiskTone
}

export function cashierOrderRiskRows(order: PaymentOrder): CashierRiskRow[] {
  const rows: CashierRiskRow[] = []
  const amount = parseAmount(order.amount_cny)
  const points = parseAmount(order.points)
  const refundedAmount = parseAmount(order.refunded_amount_cny)
  const refundedPoints = parseAmount(order.refunded_points)
  const remainingAmount = amount === null || refundedAmount === null ? null : Math.max(amount - refundedAmount, 0)
  const remainingPoints = points === null || refundedPoints === null ? null : Math.max(points - refundedPoints, 0)
  const chargebackPoints = parseAmount(order.chargeback_points)

  if (chargebackPoints !== null && chargebackPoints > 0) {
    rows.push({
      key: 'chargeback-dispute',
      label: '争议追扣',
      value: `已追扣 ${formatPoints(chargebackPoints)} 积分`,
      detail: order.chargeback_reason || '渠道侧拒付或争议已确认，系统已按运营处理记录扣减用户余额。',
      tone: 'danger',
    })
  }

  if (order.status === 'failed') {
    rows.push({
      key: 'payment-failed',
      label: '支付异常',
      value: '失败',
      detail: order.failure_reason || '渠道未返回明确原因，请结合回调事件和渠道后台排查。',
      tone: 'danger',
    })
  } else if (order.status === 'pending') {
    rows.push({
      key: 'payment-pending',
      label: '支付进度',
      value: '待支付',
      detail: '可先查单确认渠道状态；若渠道后台已收款，可使用人工补单完成到账。',
      tone: 'warning',
    })
  } else if (order.status === 'partially_refunded') {
    rows.push({
      key: 'refund-partial',
      label: '退款进度',
      value: `已退 ${formatMoney(refundedAmount)} / 剩余可退 ${formatMoney(remainingAmount)}`,
      detail: `已回退 ${formatPoints(refundedPoints)} 积分，仍可继续退 ${formatPoints(remainingPoints)} 积分。`,
      tone: 'warning',
    })
  } else if (order.status === 'refunded') {
    rows.push({
      key: 'refund-full',
      label: '退款进度',
      value: `已全额退款 ${formatMoney(refundedAmount ?? amount)}`,
      detail: `充值余额已回退 ${formatPoints(refundedPoints ?? points)} 积分；如渠道侧仍有争议，可使用追扣记录运营处理。`,
      tone: 'neutral',
    })
  } else if (order.status === 'completed') {
    rows.push({
      key: 'refund-available',
      label: '可退余额',
      value: `${formatMoney(remainingAmount ?? amount)} / ${formatPoints(remainingPoints ?? points)} 积分`,
      detail: '仅未消费的充值余额可退；退款前系统会冻结本次可退积分，避免并发消费。',
      tone: 'success',
    })
  }

  if (order.refund_trade_no) {
    rows.push({
      key: 'refund-trade',
      label: '最近退款单号',
      value: order.refund_trade_no,
      detail: '同一退款单号重放会幂等返回，不会重复扣减用户充值余额。',
      tone: order.status === 'partially_refunded' ? 'warning' : 'neutral',
    })
  }

  if (order.trade_no && order.provider) {
    rows.push({
      key: 'channel-trade',
      label: '渠道交易',
      value: order.trade_no,
      detail: `支付来源已记录为 ${order.provider}，查单、退款和审计会围绕该渠道交易号闭环。`,
      tone: 'neutral',
    })
  }

  return rows.length ? rows : [{
    key: 'order-normal',
    label: '运营状态',
    value: '暂无异常',
    detail: '当前订单没有待处理退款、失败回调或人工追扣风险。',
    tone: 'success',
  }]
}

export function cashierWebhookRiskRow(event: PaymentWebhookEvent): CashierRiskRow {
  const reason = event.failure_reason || '未返回明确失败原因'
  if (event.status === 'failed' && event.event_type === 'refund.local_finalize_failed') {
    return {
      key: 'refund-finalize-failed',
      label: '退款补偿',
      value: '本地落账失败',
      detail: `${reason}；请重试该事件，系统 worker 也会继续自动补偿。`,
      tone: 'danger',
    }
  }
  if (event.status === 'failed' && event.event_type === 'payment.retryable_failed') {
    return {
      key: 'payment-retryable-failed',
      label: '支付回调',
      value: '待重试',
      detail: `${reason}；重试前请确认渠道交易号和金额一致。`,
      tone: 'danger',
    }
  }
  if (event.status === 'failed') {
    return {
      key: 'webhook-failed',
      label: '回调处理',
      value: '失败',
      detail: `${reason}；可重试失败事件并观察处理时间。`,
      tone: 'danger',
    }
  }
  if (event.status === 'processed') {
    return {
      key: 'webhook-processed',
      label: '回调处理',
      value: '已处理',
      detail: event.processed_at ? '事件已经完成验签、幂等检查和本地落账。' : '事件已处理，处理时间未返回。',
      tone: 'success',
    }
  }
  return {
    key: 'webhook-waiting',
    label: '回调处理',
    value: '等待处理',
    detail: '事件尚未进入失败状态，通常无需人工介入。',
    tone: 'warning',
  }
}

function parseAmount(value?: string | null) {
  const parsed = Number(value ?? '')
  return Number.isFinite(parsed) ? parsed : null
}

function formatMoney(value: number | null) {
  return value === null ? '-' : `¥${value.toFixed(2)}`
}

function formatPoints(value: number | null) {
  return value === null ? '-' : value.toFixed(2)
}
