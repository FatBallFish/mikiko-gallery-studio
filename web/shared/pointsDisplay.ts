// User-facing tiered points display. The stored/base unit never changes
// (1 base point = 0.01 CNY); this module only reformats numbers for the eye,
// decomposing the amount into whole diamond/gold/silver plus a base-points
// remainder — the only tier that may carry a decimal fraction.
//
// Tiers (each x100):
//   1 钻石积分 = 100 金币积分 = 10,000 银币积分 = 1,000,000 积分
export type PointsTierKey = 'diamond' | 'gold' | 'silver' | 'base'

export type PointsTier = {
  key: PointsTierKey
  label: string
  short: string
  factor: number
}

export const pointsTiers: readonly PointsTier[] = [
  { key: 'diamond', label: '钻石积分', short: '钻石', factor: 1_000_000 },
  { key: 'gold', label: '金币积分', short: '金币', factor: 10_000 },
  { key: 'silver', label: '银币积分', short: '银币', factor: 100 },
  { key: 'base', label: '积分', short: '积分', factor: 1 },
]

export const POINT_CNY = 0.01

export const pointsExchangeRules: readonly string[] = [
  '1 钻石积分 = 100 金币积分',
  '1 金币积分 = 100 银币积分',
  '1 银币积分 = 100 积分',
  '100 积分 = 1 元（1 积分 = 0.01 元）',
]

export type PointsDisplay = {
  /** Decomposition text, e.g. "1 钻石 23 银币 58 积分". */
  text: string
  /** Same decomposition with full unit names, e.g. "1 钻石积分 23 银币积分 58 积分". */
  fullText: string
  /** Non-empty tier parts in descending order. */
  parts: Array<{ tier: PointsTier; amount: string }>
  /** Raw base points as provided. */
  base: string
}

export function formatPoints(value: string | number | null | undefined): PointsDisplay {
  const base = normalizeBase(value)
  const parts = decompose(base)
  const text = parts.map((part) => `${part.amount} ${part.tier.short}`).join(' ')
  const fullText = parts.map((part) => `${part.amount} ${part.tier.label}`).join(' ')
  return { text, fullText, parts, base }
}

export function pointsText(value: string | number | null | undefined): string {
  return formatPoints(value).text
}

function normalizeBase(value: string | number | null | undefined): string {
  if (value === null || value === undefined || value === '') return '0'
  const raw = String(value).trim().replace(/,/g, '')
  if (!/^-?\d+(\.\d+)?$/.test(raw)) return '0'
  return raw
}

function decompose(base: string): PointsDisplay['parts'] {
  const negative = base.startsWith('-')
  const unsigned = negative ? base.slice(1) : base
  const [int, frac] = unsigned.split('.')
  // Whole part decomposes across tiers; the base tier keeps any fraction.
  let remainder = BigInt(int || '0')
  const parts: PointsDisplay['parts'] = []
  for (const tier of pointsTiers) {
    if (tier.key === 'base') {
      const whole = remainder.toString()
      if (whole !== '0' || frac) {
        const amount = frac ? `${whole}.${frac.replace(/0+$/, '')}` : whole
        if (amount !== '0' && amount !== '0.') parts.push({ tier, amount: amount.replace(/\.$/, '') })
      }
      continue
    }
    const count = remainder / BigInt(tier.factor)
    remainder %= BigInt(tier.factor)
    if (count > 0n) parts.push({ tier, amount: count.toString() })
  }
  if (parts.length === 0) parts.push({ tier: pointsTiers[3], amount: '0' })
  if (negative) {
    return parts.map((part, index) => index === 0 ? { ...part, amount: `-${part.amount}` } : part)
  }
  return parts
}
