import type { PaymentSchedulerStrategy, PaymentVisibleMethod } from '../../../shared/api-types'
import { cashierProviderLabel } from './cashierProviderOptions'

const methodTitleLabels: Record<string, string> = {
  alipay: '支付宝入口',
  wxpay: '微信支付入口',
  mock: '测试支付入口',
}

const schedulerLabels: Record<string, string> = {
  round_robin: '轮询调度',
  random: '随机调度',
}

export type CashierVisibleMethodRow = {
  title: string
  detail: string
  rawMethod: string
  rawProviderType: string
  providerLabel: string
  schedulerLabel: string
  permission: 'cashier.visible_methods.write'
}

export function cashierVisibleMethodSchedulerLabel(strategy?: PaymentSchedulerStrategy | string) {
  const raw = strategy || 'round_robin'
  return schedulerLabels[raw] ?? raw
}

export function cashierVisibleMethodRow(method: PaymentVisibleMethod): CashierVisibleMethodRow {
  const rawMethod = method.method || ''
  const rawProviderType = method.source_provider_type || ''
  const title = methodTitleLabels[rawMethod] ?? rawMethod
  return {
    title: title || '支付入口',
    detail: method.label || rawMethod || '-',
    rawMethod,
    rawProviderType,
    providerLabel: rawProviderType ? cashierProviderLabel(rawProviderType) : '-',
    schedulerLabel: cashierVisibleMethodSchedulerLabel(method.scheduler_strategy),
    permission: 'cashier.visible_methods.write',
  }
}
