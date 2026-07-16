import { WORKSPACE_SHEET_SNAP_THRESHOLD, workspaceSheetDragOffset, workspaceSheetSnap } from './workspaceSheetGesture'

if (WORKSPACE_SHEET_SNAP_THRESHOLD !== 56) {
  throw new Error(`workspace sheet snap threshold must remain deliberate, got ${WORKSPACE_SHEET_SNAP_THRESHOLD}`)
}

const snapCases = [
  { expanded: false, deltaY: -56, expected: true },
  { expanded: false, deltaY: -55, expected: false },
  { expanded: false, deltaY: 90, expected: false },
  { expanded: true, deltaY: 56, expected: false },
  { expanded: true, deltaY: 55, expected: true },
  { expanded: true, deltaY: -90, expected: true },
]

for (const testCase of snapCases) {
  const actual = workspaceSheetSnap(testCase.expanded, testCase.deltaY)
  if (actual !== testCase.expected) {
    throw new Error(`workspace sheet snap mismatch for ${JSON.stringify(testCase)}, got ${actual}`)
  }
}

const collapsedOffsets = [-200, -40, 50].map((deltaY) => workspaceSheetDragOffset(false, deltaY))
if (JSON.stringify(collapsedOffsets) !== JSON.stringify([-96, -40, 20])) {
  throw new Error(`collapsed drag feedback must resist invalid direction and clamp travel, got ${JSON.stringify(collapsedOffsets)}`)
}

const expandedOffsets = [-50, 40, 200].map((deltaY) => workspaceSheetDragOffset(true, deltaY))
if (JSON.stringify(expandedOffsets) !== JSON.stringify([-20, 40, 96])) {
  throw new Error(`expanded drag feedback must resist invalid direction and clamp travel, got ${JSON.stringify(expandedOffsets)}`)
}
