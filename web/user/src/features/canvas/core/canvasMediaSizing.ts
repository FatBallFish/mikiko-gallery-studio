import type { CanvasSize } from './types'

const positive = (value: number, fallback: number) => Number.isFinite(value) && value > 0 ? value : fallback

/** Fit the preferred box, expanding only to satisfy persisted canvas dimension minima. */
export function canvasMediaSize(width: number, height: number, box: CanvasSize = { width: 320, height: 240 }): CanvasSize {
  const w = positive(width, 4)
  const h = positive(height, 3)
  const scale = Math.max(Math.min(positive(box.width, 320) / w, positive(box.height, 240) / h), 1 / Math.min(w, h), 80 / Math.max(w, h))
  return { width: w * scale, height: h * scale }
}

/** Follow the dimension with the larger proportional drag; a single scale preserves ratio. */
export function resizeCanvasMediaSize(current: CanvasSize, requested: CanvasSize, ratio = current.width / current.height): CanvasSize {
  const r = positive(ratio, 4 / 3)
  const base = { width: positive(current.width, 320), height: positive(current.width, 320) / r }
  const sx = positive(requested.width, base.width) / base.width
  const sy = positive(requested.height, base.height) / base.height
  const scale = Math.abs(sx - 1) >= Math.abs(sy - 1) ? sx : sy
  // Persisted canvas dimensions must each be >= 1; use one scale to preserve ratio.
  const bounded = Math.max(scale, 80 / Math.max(base.width, base.height), 1 / Math.min(base.width, base.height))
  return { width: base.width * bounded, height: base.height * bounded }
}
