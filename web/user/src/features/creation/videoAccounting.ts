import type { VideoEstimate, VideoTask } from '../../../../shared/api-types'

export type VideoQuoteBreakdown = {
  unitPoints: string
  outputCount: number
  estimatedPoints: string
  maxReservedPoints: string
  availablePoints?: string
  pricingMode?: string
}

export type VideoTaskAccounting = {
  estimatedPoints: string
  reservedPoints: string
  actualPoints: string
  refundPoints: string
  settlementStatus: string
  unitPoints?: string
  timeline: Array<{ label: string; value: string }>
  variables: Array<{ name: string; value: string }>
  inputs: Array<{ assetID: string; role: string; name: string }>
  items: Array<{
    id: string
    ordinal: number
    status: string
    actualSeconds?: string
    actualPoints: string
    resultAssetID?: string
    error?: string
  }>
}

export function buildVideoQuoteBreakdown(quote: VideoEstimate): VideoQuoteBreakdown {
  return {
    unitPoints: quote.unit_points,
    outputCount: summaryNumber(quote.summary, 'output_count') ?? outputCountFromQuote(quote),
    estimatedPoints: quote.estimated_points,
    maxReservedPoints: quote.max_reserved_points,
    availablePoints: quote.balance?.available_points,
    pricingMode: quote.pricing_mode,
  }
}

export function buildVideoTaskAccounting(task: VideoTask): VideoTaskAccounting {
  const reservedPoints = task.reserved_points ?? task.estimated_points ?? '0.00000'
  const actualPoints = task.actual_points ?? '0.00000'
  return {
    estimatedPoints: task.estimated_points ?? '0.00000',
    reservedPoints,
    actualPoints,
    refundPoints: subtractDecimalStrings(reservedPoints, actualPoints, 5),
    settlementStatus: task.settlement_status ?? 'pending',
    unitPoints: snapshotString(task.pricing_snapshot, 'unit_points'),
    variables: promptSnapshotVariables(task.prompt_binding_snapshot),
    timeline: [
      task.created_at ? { label: '创建任务', value: task.created_at } : null,
      task.started_at ? { label: '开始生成', value: task.started_at } : null,
      task.finished_at ? { label: '完成任务', value: task.finished_at } : null,
    ].filter((item): item is { label: string; value: string } => item !== null),
    inputs: task.inputs.map((input) => ({
      assetID: input.asset_id,
      role: input.role,
      name: snapshotString(input.asset_snapshot, 'name') ?? input.asset?.name ?? `资产 ${input.ordinal + 1}`,
    })),
    items: task.items.map((item) => ({
      id: item.id,
      ordinal: item.ordinal,
      status: item.status,
      actualSeconds: item.actual_output_seconds,
      actualPoints: item.actual_points ?? '0.00000',
      resultAssetID: item.result_asset_id,
      error: [item.error_code, item.error_message].filter(Boolean).join('：') || undefined,
    })),
  }
}

function promptSnapshotVariables(snapshot: Record<string, unknown> | undefined) {
  if (!Array.isArray(snapshot?.variables)) return []
  return snapshot.variables.flatMap((entry) => {
    if (!entry || typeof entry !== 'object') return []
    const item = entry as Record<string, unknown>
    return typeof item.name === 'string' && typeof item.value === 'string' ? [{ name: item.name, value: item.value }] : []
  })
}

function outputCountFromQuote(quote: VideoEstimate) {
  const unit = parseDecimal(quote.unit_points, 5)
  const total = parseDecimal(quote.estimated_points, 5)
  if (unit <= 0n || total <= 0n || total % unit !== 0n) return 1
  return Number(total / unit)
}

function summaryNumber(summary: Record<string, unknown> | undefined, key: string) {
  const value = summary?.[key]
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? Math.floor(value) : undefined
}

function snapshotString(snapshot: Record<string, unknown> | undefined, key: string) {
  const value = snapshot?.[key]
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}

function subtractDecimalStrings(left: string, right: string, scale: number) {
  const value = parseDecimal(left, scale) - parseDecimal(right, scale)
  return formatDecimal(value > 0n ? value : 0n, scale)
}

function parseDecimal(value: string, scale: number) {
  const normalized = value.trim()
  const match = normalized.match(/^(-?)(\d+)(?:\.(\d+))?$/)
  if (!match) return 0n
  const fraction = (match[3] ?? '').padEnd(scale, '0').slice(0, scale)
  const amount = BigInt(match[2]) * (10n ** BigInt(scale)) + BigInt(fraction || '0')
  return match[1] === '-' ? -amount : amount
}

function formatDecimal(value: bigint, scale: number) {
  const divisor = 10n ** BigInt(scale)
  const whole = value / divisor
  const fraction = (value % divisor).toString().padStart(scale, '0')
  return `${whole}.${fraction}`
}
