// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
import {
  buildTimeSeriesPath,
  clampChartIndex,
  timeSeriesExtent,
  type TimeSeriesValue,
} from './timeSeriesChart'

const values: TimeSeriesValue[] = [
  { at: '2026-07-12T00:00:00Z', value: 10 },
  { at: '2026-07-12T00:00:05Z', value: 20 },
  { at: '2026-07-12T00:00:10Z', value: 15 },
]

assertEqual(buildTimeSeriesPath([], 640, 220), '', 'empty path')
assertEqual(buildTimeSeriesPath(values.slice(0, 1), 640, 220), '', 'single point path')
const path = buildTimeSeriesPath(values, 640, 220)
if (!path.startsWith('M ') || path.includes('NaN') || path.includes('Infinity')) {
  throw new Error(`valid series should produce a finite SVG path, got ${path}`)
}

const flat = timeSeriesExtent(values.map((point) => ({ ...point, value: 8 })))
if (!(flat.max > flat.min)) {
  throw new Error(`flat extent must be expanded, got ${JSON.stringify(flat)}`)
}
assertEqual(clampChartIndex(-1, values.length), 0, 'lower chart index')
assertEqual(clampChartIndex(99, values.length), 2, 'upper chart index')
assertEqual(clampChartIndex(0, 0), -1, 'empty chart index')

const source = readFileSync(new URL('./timeSeriesChart.tsx', import.meta.url), 'utf8')
for (const contract of [
  'viewBox="0 0 640 220"',
  'tabIndex={0}',
  "event.key === 'ArrowLeft'",
  "event.key === 'ArrowRight'",
  "event.key === 'Home'",
  "event.key === 'End'",
  'aria-live="polite"',
  'pointerIndex',
  'latest',
  'minimum',
  'maximum',
]) {
  if (!source.includes(contract)) {
    throw new Error(`time-series chart should preserve interaction/accessibility contract ${contract}`)
  }
}
for (const forbidden of ['animateMotion', 'fakePoint', 'Math.random', 'rounded-2xl', 'rounded-3xl']) {
  if (source.includes(forbidden)) {
    throw new Error(`time-series chart should not include ${forbidden}`)
  }
}

function assertEqual(actual: unknown, expected: unknown, name: string) {
  if (actual !== expected) {
    throw new Error(`${name}: expected ${String(expected)}, got ${String(actual)}`)
  }
}
