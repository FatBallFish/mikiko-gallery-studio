export const WORKSPACE_SHEET_SNAP_THRESHOLD = 56

export function workspaceSheetSnap(expanded: boolean, deltaY: number) {
  if (deltaY <= -WORKSPACE_SHEET_SNAP_THRESHOLD) return true
  if (deltaY >= WORKSPACE_SHEET_SNAP_THRESHOLD) return false
  return expanded
}

export function workspaceSheetDragOffset(expanded: boolean, deltaY: number) {
  const minimum = expanded ? -20 : -96
  const maximum = expanded ? 96 : 20
  return Math.max(minimum, Math.min(maximum, deltaY))
}
