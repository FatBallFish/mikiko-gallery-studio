import { formatPoints, pointsExchangeRules, pointsText } from '../../shared/pointsDisplay'

// Decomposition: X钻石 X金币 X银币 X积分, remainder-only fractions on base points.
const cases: Array<[string, string]> = [
  ['58', '58 积分'],
  ['99', '99 积分'],
  ['100', '1 银币'],
  ['250', '2 银币 50 积分'],
  ['300', '3 银币'],
  ['3645', '36 银币 45 积分'],
  ['9999', '99 银币 99 积分'],
  ['10000', '1 金币'],
  ['15500', '1 金币 55 银币'],
  ['7290', '72 银币 90 积分'],
  ['7833.00000', '78 银币 33 积分'],
  ['1000000', '1 钻石'],
  ['1230000', '1 钻石 23 金币'],
  ['1234567', '1 钻石 23 金币 45 银币 67 积分'],
  ['0', '0 积分'],
  ['0.50000', '0.5 积分'],
  ['150.75', '1 银币 50.75 积分'],
]
for (const [input, want] of cases) {
  const result = formatPoints(input)
  if (result.text !== want) {
    throw new Error(`formatPoints(${input}) = "${result.text}", want "${want}"`)
  }
}
if (pointsText(300) !== '3 银币') throw new Error(`pointsText(300) = ${pointsText(300)}`)
if (formatPoints('250').fullText !== '2 银币积分 50 积分') throw new Error('fullText broken')
// decomposition must not introduce decimals above the base tier
const check = formatPoints('1234567.89000')
for (const part of check.parts) {
  if (part.tier.key !== 'base' && part.amount.includes('.')) throw new Error('non-base tier must stay whole')
}
if (pointsExchangeRules.length !== 4 || !pointsExchangeRules[3].includes('0.01')) throw new Error('exchange rules incomplete')
console.log('pointsDisplay contract OK')
