export type WorkspaceTaskRecord = {
  id: string
  created_at: string
  reference_assets?: readonly unknown[]
}

export type WorkspaceTaskHistoryOptions = {
  limit: number
  preserveIds?: readonly string[]
}

function trimWorkspaceTaskRecords<T extends WorkspaceTaskRecord>(records: Iterable<T>, options: WorkspaceTaskHistoryOptions) {
  const preserveIds = new Set(options.preserveIds ?? [])
  const sorted = Array.from(records).sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime())
  const pinned = sorted.filter((task) => preserveIds.has(task.id))
  const rolling = sorted.filter((task) => !preserveIds.has(task.id)).slice(-Math.max(0, options.limit))
  return [...pinned, ...rolling].sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime())
}

export function mergeWorkspaceTaskRecords<T extends WorkspaceTaskRecord>(records: T[], next: T, options: WorkspaceTaskHistoryOptions) {
  const map = new Map(records.map((item) => [item.id, item]))
  const current = map.get(next.id)
  if (current?.reference_assets?.length && !next.reference_assets?.length) {
    next = { ...next, reference_assets: current.reference_assets } as T
  }
  map.set(next.id, next)
  return trimWorkspaceTaskRecords(map.values(), options)
}

export function replaceWorkspaceTaskRecords<T extends WorkspaceTaskRecord>(current: T[], incoming: T[], options: WorkspaceTaskHistoryOptions) {
  const preserveIds = new Set(options.preserveIds ?? [])
  const map = new Map(current.filter((task) => preserveIds.has(task.id)).map((task) => [task.id, task]))
  incoming.forEach((task) => map.set(task.id, task))
  return trimWorkspaceTaskRecords(map.values(), options)
}
