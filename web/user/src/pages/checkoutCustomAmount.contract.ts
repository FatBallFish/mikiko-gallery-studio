import { cnyPerPointLabel, customAmountPoints, fallbackCnyPerPoint, normalizeCustomAmount } from './checkoutCustomAmount'

const invalid = normalizeCustomAmount('abc')
if (invalid.valid || invalid.error !== '请输入有效金额') {
  throw new Error(`invalid custom amount should return a stable error, got ${JSON.stringify(invalid)}`)
}

const tooSmall = normalizeCustomAmount('0.99')
if (tooSmall.valid || tooSmall.error !== '自定义金额不能低于 1 元') {
  throw new Error(`custom amount should enforce min 1 CNY, got ${JSON.stringify(tooSmall)}`)
}

const tooLarge = normalizeCustomAmount('10000.01')
if (tooLarge.valid || tooLarge.error !== '自定义金额不能超过 10000 元') {
  throw new Error(`custom amount should enforce max 10000 CNY, got ${JSON.stringify(tooLarge)}`)
}

const valid = normalizeCustomAmount('25')
if (!valid.valid || valid.value !== '25.00' || valid.amount !== 25) {
  throw new Error(`valid custom amount should normalize to two decimals, got ${JSON.stringify(valid)}`)
}

if (customAmountPoints(25, '0.03125') !== '800.00') {
  throw new Error(`custom amount points should use backend unit price, got ${customAmountPoints(25, '0.03125')}`)
}

if (customAmountPoints(25) !== (25 / fallbackCnyPerPoint).toFixed(2)) {
  throw new Error(`custom amount points should use fallback unit price, got ${customAmountPoints(25)}`)
}

if (cnyPerPointLabel('0.03125') !== '0.03125 元/积分') {
  throw new Error(`custom amount unit price label should be fixed, got ${cnyPerPointLabel('0.03125')}`)
}
