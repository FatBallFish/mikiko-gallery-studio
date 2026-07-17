import { useEffect, useMemo, useRef, useState } from 'react'
import { cn } from '../../../shared/classnames'

export type TimeSeriesValue = {
  at: string
  value: number | null
}

export type TimeSeriesDatum = {
  at: string
  values: Record<string, number | null>
}

export type TimeSeriesDefinition = {
  key: string
  label: string
  color: string
  axis?: 'left' | 'right'
  format?: (value: number) => string
}

type Extent = { min: number; max: number }

const chartWidth = 640
const chartHeight = 220
const chartPadding = { top: 18, right: 42, bottom: 30, left: 42 }

export function timeSeriesExtent(values: TimeSeriesValue[]): Extent {
  const finite = values
    .map((point) => point.value)
    .filter((value): value is number => value != null && Number.isFinite(value))
  if (!finite.length) return { min: 0, max: 1 }
  const minimum = Math.min(...finite)
  const maximum = Math.max(...finite)
  if (minimum !== maximum) {
    return { min: Math.min(0, minimum), max: maximum }
  }
  const padding = Math.max(1, Math.abs(minimum) * 0.05)
  return { min: Math.max(0, minimum - padding), max: maximum + padding }
}

export function buildTimeSeriesPath(
  points: TimeSeriesValue[],
  width: number,
  height: number,
  suppliedExtent?: Extent,
): string {
  const available = points
    .map((point, index) => ({ ...point, index }))
    .filter((point): point is TimeSeriesValue & { value: number; index: number } => point.value != null && Number.isFinite(point.value))
  if (available.length < 2) return ''
  const extent = suppliedExtent ?? timeSeriesExtent(points)
  const innerWidth = Math.max(1, width - chartPadding.left - chartPadding.right)
  const innerHeight = Math.max(1, height - chartPadding.top - chartPadding.bottom)
  const span = Math.max(Number.EPSILON, extent.max - extent.min)
  return available.map((point, pathIndex) => {
    const x = chartPadding.left + (point.index / Math.max(1, points.length - 1)) * innerWidth
    const y = chartPadding.top + (1 - (point.value - extent.min) / span) * innerHeight
    return `${pathIndex === 0 ? 'M' : 'L'} ${x.toFixed(2)} ${y.toFixed(2)}`
  }).join(' ')
}

export function clampChartIndex(index: number, length: number): number {
  if (length <= 0) return -1
  return Math.max(0, Math.min(length - 1, index))
}

export function TimeSeriesChart({
  ariaLabel,
  data,
  series,
  className,
}: {
  ariaLabel: string
  data: TimeSeriesDatum[]
  series: TimeSeriesDefinition[]
  className?: string
}) {
  const [keyboardIndex, setKeyboardIndex] = useState(() => clampChartIndex(data.length - 1, data.length))
  const [pointerIndex, setPointerIndex] = useState<number | null>(null)
  const keyboardInspectionRef = useRef(false)
  const leftExtent = useMemo(() => chartExtentForAxis(data, series, 'left'), [data, series])
  const rightExtent = useMemo(() => chartExtentForAxis(data, series, 'right'), [data, series])
  const hasRightAxis = series.some((definition) => definition.axis === 'right')
  const activeIndex = pointerIndex ?? clampChartIndex(keyboardIndex, data.length)
  const active = activeIndex >= 0 ? data[activeIndex] : null
  const firstTime = data[0]?.at
  const lastTime = data[data.length - 1]?.at

  useEffect(() => {
    setKeyboardIndex((current) => keyboardInspectionRef.current
      ? clampChartIndex(current, data.length)
      : clampChartIndex(data.length - 1, data.length))
  }, [data.length])

  const summaries = series.map((definition) => {
    const finite = data
      .map((point) => point.values[definition.key])
      .filter((value): value is number => value != null && Number.isFinite(value))
    const latest = finite[finite.length - 1]
    const minimum = finite.length ? Math.min(...finite) : null
    const maximum = finite.length ? Math.max(...finite) : null
    return {
      definition,
      latest,
      minimum,
      maximum,
      text: `${definition.label}: latest ${formatSeriesValue(definition, latest)}, minimum ${formatSeriesValue(definition, minimum)}, maximum ${formatSeriesValue(definition, maximum)}`,
    }
  })

  if (!data.length) {
    return (
      <div className={cn('grid min-h-[220px] place-items-center text-sm text-[var(--soft)]', className)} role="status">
        当前窗口暂无请求样本
      </div>
    )
  }

  function inspectPointer(clientX: number, currentTarget: SVGSVGElement) {
    const bounds = currentTarget.getBoundingClientRect()
    const ratio = bounds.width > 0 ? (clientX - bounds.left) / bounds.width : 0
    setPointerIndex(clampChartIndex(Math.round(ratio * (data.length - 1)), data.length))
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'ArrowLeft') {
      event.preventDefault()
      keyboardInspectionRef.current = true
      setKeyboardIndex((current) => clampChartIndex((current < 0 ? data.length : current) - 1, data.length))
    } else if (event.key === 'ArrowRight') {
      event.preventDefault()
      keyboardInspectionRef.current = true
      setKeyboardIndex((current) => clampChartIndex(current + 1, data.length))
    } else if (event.key === 'Home') {
      event.preventDefault()
      keyboardInspectionRef.current = true
      setKeyboardIndex(clampChartIndex(0, data.length))
    } else if (event.key === 'End') {
      event.preventDefault()
      keyboardInspectionRef.current = true
      setKeyboardIndex(clampChartIndex(data.length - 1, data.length))
    }
  }

  return (
    <div
      className={cn('relative min-w-0 outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--surface-solid)]', className)}
      role="group"
      aria-label={ariaLabel}
      tabIndex={0}
      onFocus={() => setKeyboardIndex((current) => clampChartIndex(current < 0 ? data.length - 1 : current, data.length))}
      onBlur={() => {
        keyboardInspectionRef.current = false
        setKeyboardIndex(clampChartIndex(data.length - 1, data.length))
      }}
      onKeyDown={handleKeyDown}
    >
      <svg
        className="block aspect-[640/220] w-full overflow-visible"
        viewBox="0 0 640 220"
        role="img"
        aria-hidden="true"
        onPointerMove={(event) => inspectPointer(event.clientX, event.currentTarget)}
        onPointerLeave={() => setPointerIndex(null)}
      >
        {[0, 0.5, 1].map((ratio) => {
          const y = chartPadding.top + ratio * (chartHeight - chartPadding.top - chartPadding.bottom)
          const value = leftExtent.max - ratio * (leftExtent.max - leftExtent.min)
          return (
            <g key={ratio}>
              <line x1={chartPadding.left} x2={chartWidth - chartPadding.right} y1={y} y2={y} stroke="var(--border)" strokeWidth="1" vectorEffect="non-scaling-stroke" />
              <text x={chartPadding.left - 8} y={y + 4} textAnchor="end" fill="var(--dim)" fontSize="10">{compactAxisValue(value)}</text>
              {hasRightAxis ? (
                <text x={chartWidth - chartPadding.right + 8} y={y + 4} fill="var(--dim)" fontSize="10">
                  {compactAxisValue(rightExtent.max - ratio * (rightExtent.max - rightExtent.min))}
                </text>
              ) : null}
            </g>
          )
        })}
        {series.map((definition) => {
          const values = data.map((point) => ({ at: point.at, value: point.values[definition.key] ?? null }))
          const path = buildTimeSeriesPath(values, chartWidth, chartHeight, definition.axis === 'right' ? rightExtent : leftExtent)
          return path ? (
            <path
              key={definition.key}
              d={path}
              fill="none"
              stroke={definition.color}
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
              vectorEffect="non-scaling-stroke"
            />
          ) : null
        })}
        {active ? (
          <line
            x1={chartX(activeIndex, data.length)}
            x2={chartX(activeIndex, data.length)}
            y1={chartPadding.top}
            y2={chartHeight - chartPadding.bottom}
            stroke="var(--fg)"
            strokeDasharray="3 4"
            strokeWidth="1"
            vectorEffect="non-scaling-stroke"
          />
        ) : null}
        <text x={chartPadding.left} y={chartHeight - 8} fill="var(--dim)" fontSize="10">{formatChartTime(firstTime)}</text>
        <text x={chartWidth - chartPadding.right} y={chartHeight - 8} textAnchor="end" fill="var(--dim)" fontSize="10">{formatChartTime(lastTime)}</text>
      </svg>

      <div className="pointer-events-none absolute right-3 top-3 min-w-[132px] border border-[var(--border)] bg-[color-mix(in_oklch,var(--surface-solid)_94%,transparent)] px-3 py-2 shadow-[var(--pg-shadow-sm)] backdrop-blur-sm">
        <div className="mb-1 font-[family-name:var(--admin-font-mono)] text-[length:var(--admin-type-label)] font-semibold text-[var(--dim)]">
          {formatChartTime(active?.at)}
        </div>
        <div className="grid gap-1">
          {series.map((definition) => (
            <div key={definition.key} className="flex items-center justify-between gap-4 text-xs">
              <span className="inline-flex items-center gap-1.5 text-[var(--soft)]">
                <span className="size-1.5 rounded-full" style={{ backgroundColor: definition.color }} />
                {definition.label}
              </span>
              <strong className="font-[family-name:var(--admin-font-mono)] tabular-nums text-[var(--fg)]">
                {formatSeriesValue(definition, active?.values[definition.key] ?? null)}
              </strong>
            </div>
          ))}
        </div>
      </div>

      <div className="sr-only" aria-live="polite">
        {active ? `${formatChartTime(active.at)}，${series.map((definition) => `${definition.label} ${formatSeriesValue(definition, active.values[definition.key] ?? null)}`).join('，')}` : '暂无样本'}
        {summaries.map((summary) => `；${summary.text}`).join('')}
      </div>
    </div>
  )
}

function chartX(index: number, length: number) {
  const innerWidth = chartWidth - chartPadding.left - chartPadding.right
  return chartPadding.left + (index / Math.max(1, length - 1)) * innerWidth
}

function chartExtentForAxis(
  data: TimeSeriesDatum[],
  series: TimeSeriesDefinition[],
  axis: 'left' | 'right',
) {
  const definitions = series.filter((definition) => (definition.axis ?? 'left') === axis)
  return timeSeriesExtent(
    data.flatMap((point) => definitions.map((definition) => ({ at: point.at, value: point.values[definition.key] ?? null }))),
  )
}

function formatSeriesValue(definition: TimeSeriesDefinition, value: number | null | undefined) {
  if (value == null || !Number.isFinite(value)) return '-'
  return definition.format ? definition.format(value) : value.toLocaleString('en-US', { maximumFractionDigits: 2 })
}

function compactAxisValue(value: number) {
  if (Math.abs(value) >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}m`
  if (Math.abs(value) >= 1_000) return `${(value / 1_000).toFixed(1)}k`
  return value.toLocaleString('en-US', { maximumFractionDigits: 1 })
}

function formatChartTime(value: string | undefined) {
  if (!value) return '-'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(parsed)
}
