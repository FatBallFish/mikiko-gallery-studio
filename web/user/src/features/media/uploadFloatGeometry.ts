import type { UploadSnapshot } from './uploadManager'

export const UPLOAD_FLOAT_SIZE = 48
export type FloatPosition = { x: number; y: number; edge: 'left' | 'right' | null }
export type FloatBounds = { left: number; top: number; right: number; bottom: number }
const clamp = (value: number, min: number, max: number) => Math.max(min, Math.min(value, Math.max(min, max)))

export function clampUploadPosition(position: FloatPosition, bounds: FloatBounds): FloatPosition {
  return { ...position, x: position.edge === 'left' ? bounds.left : position.edge === 'right' ? Math.max(bounds.left, bounds.right - 48) : clamp(position.x, bounds.left, bounds.right - 48), y: clamp(position.y, bounds.top, bounds.bottom - 48) }
}

export function snapUploadPosition(position: FloatPosition, bounds: FloatBounds): FloatPosition {
  const next = clampUploadPosition({ ...position, edge: null }, bounds)
  return { ...next, edge: next.x - bounds.left <= 24 ? 'left' : bounds.right - next.x - 48 <= 24 ? 'right' : null }
}

export function uploadPanelGeometry(position: FloatPosition, bounds: FloatBounds) {
  const width = Math.min(360, bounds.right - bounds.left)
  const height = Math.max(48, Math.min(468, bounds.bottom - bounds.top))
  const left = position.x + 24 < (bounds.left + bounds.right) / 2 ? position.x : position.x + 48 - width
  return { left: clamp(left, bounds.left, bounds.right - width), top: clamp(position.y, bounds.top, bounds.bottom - height), width, maxHeight: height }
}

export function restoreUploadPosition(raw: string | null): FloatPosition | null {
  try {
    const value = JSON.parse(raw ?? 'null')
    return value && Number.isFinite(value.x) && Number.isFinite(value.y) && (value.edge === null || value.edge === 'left' || value.edge === 'right') ? value : null
  } catch { return null }
}

export function uploadQueueSummary(items: Pick<UploadSnapshot, 'status'>[]) {
  const pending = items.filter(({ status }) => status !== 'completed' && status !== 'cancelled')
  return { count: pending.length, state: pending.some(({ status }) => status === 'failed' || status === 'needs_file') ? 'error' : pending.some(({ status }) => status === 'paused') ? 'paused' : pending.length ? 'active' : 'idle' }
}

export function uploadDragMoved(startX: number, startY: number, x: number, y: number) {
  return Math.hypot(x - startX, y - startY) >= 6
}
