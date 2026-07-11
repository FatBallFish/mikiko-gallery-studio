import { existsSync } from 'node:fs'

const modelURL = new URL('./workspaceTaskHistory.ts', import.meta.url)
if (!existsSync(modelURL)) {
  throw new Error('workspace task history needs an executable merge model with pinned task support')
}

const { mergeWorkspaceTaskRecords, replaceWorkspaceTaskRecords } = await import('./workspaceTaskHistory')

type Task = {
  id: string
  created_at: string
  reference_assets?: Array<{ id: string }>
}

function task(id: string, minute: number): Task {
  return { id, created_at: new Date(Date.UTC(2026, 6, 10, 0, minute)).toISOString() }
}

const deepLinked = task('deep-linked', 0)
const recent = Array.from({ length: 25 }, (_, index) => task(`recent-${index + 1}`, index + 1))

const afterSnapshot = replaceWorkspaceTaskRecords(
  [deepLinked],
  recent,
  { limit: 20, preserveIds: ['deep-linked'] },
)
if (!afterSnapshot.some((item: Task) => item.id === 'deep-linked')) {
  throw new Error('a deep-linked task older than the rolling 20 must survive a history snapshot')
}
if (afterSnapshot.filter((item: Task) => item.id !== 'deep-linked').length !== 20) {
  throw new Error(`history snapshot must retain exactly 20 rolling tasks beside the pinned task, got ${afterSnapshot.length}`)
}

const afterUpdate = mergeWorkspaceTaskRecords(
  afterSnapshot,
  { ...task('recent-25', 25), reference_assets: [{ id: 'updated' }] },
  { limit: 20, preserveIds: ['deep-linked'] },
)
if (!afterUpdate.some((item: Task) => item.id === 'deep-linked')) {
  throw new Error('a deep-linked task must remain selected after subsequent task updates')
}
const newest = afterUpdate[afterUpdate.length - 1]
if (newest?.id !== 'recent-25') {
  throw new Error(`newest task ordering must not fall back to the pinned task, got ${newest?.id}`)
}

const withoutPin = replaceWorkspaceTaskRecords([deepLinked], recent, { limit: 20, preserveIds: [] })
if (withoutPin.some((item: Task) => item.id === 'deep-linked') || withoutPin.length !== 20) {
  throw new Error('unselected tasks outside the rolling window must not be retained accidentally')
}
