export const fallbackCnyPerPoint = 0.03125

export type NormalizedCustomAmount = {
  value: string
  valid: boolean
  amount: number
  error?: string
}

export function normalizeCustomAmount(input: string): NormalizedCustomAmount {
  const amount = Number(input)
  if (!Number.isFinite(amount)) return { value: input, valid: false, amount: 0, error: '请输入有效金额' }
  if (amount < 1) return { value: input, valid: false, amount, error: '自定义金额不能低于 1 元' }
  if (amount > 10000) return { value: input, valid: false, amount, error: '自定义金额不能超过 10000 元' }
  return { value: amount.toFixed(2), valid: true, amount }
}

export function customAmountPoints(amountCny: number, cnyPerPoint?: string) {
  const unit = cnyPerPointValue(cnyPerPoint)
  if (!Number.isFinite(unit) || unit <= 0) return '0.00'
  return (amountCny / unit).toFixed(2)
}

export function cnyPerPointValue(cnyPerPoint?: string) {
  const unit = Number(cnyPerPoint || fallbackCnyPerPoint)
  return Number.isFinite(unit) && unit > 0 ? unit : fallbackCnyPerPoint
}

export function cnyPerPointLabel(cnyPerPoint?: string) {
  return `${cnyPerPointValue(cnyPerPoint).toFixed(5)} 元/积分`
}
