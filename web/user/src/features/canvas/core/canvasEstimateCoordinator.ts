export type CanvasEstimateTarget = { nodeID: string; signature: string; force?: boolean }
export type CanvasEstimateTicket = CanvasEstimateTarget & { requestID: number }
export type CanvasEstimateTiming = { nodeID: string; flushMS: number; requestMS: number; commitMS: number; totalMS: number }

type Options<T> = {
  flush: () => Promise<boolean>
  isCurrent: (nodeID: string, signature: string) => boolean
  request: (nodeID: string, signal: AbortSignal) => Promise<T>
  canReuse?: (result: T) => boolean
  commit?: (ticket: CanvasEstimateTicket, result: T, current: () => boolean) => Promise<boolean>
  prepare: (ticket: CanvasEstimateTicket) => void
  start: (ticket: CanvasEstimateTicket) => void
  resolve: (ticket: CanvasEstimateTicket, result: T) => void
  reject: (ticket: CanvasEstimateTicket, error: unknown) => void
  clear: (nodeID: string) => void
  timing?: (timing: CanvasEstimateTiming) => void
  debounceMS?: number
}

// Owns request identity independently of React renders and save acknowledgements.
export function createCanvasEstimateCoordinator<T>(options: Options<T>) {
  type Entry = { ticket: CanvasEstimateTicket; controller: AbortController; timer?: ReturnType<typeof setTimeout> }
  const entries = new Map<string, Entry>()
  const successful = new Map<string, { signature: string; result: T }>()
  let sequence = 0
  let disposed = false
  let pendingFlush: Promise<boolean> | undefined
  function flush() {
    if (!pendingFlush) {
      const pending = Promise.resolve().then(options.flush)
      pendingFlush = pending
      void pending.finally(() => { if (pendingFlush === pending) pendingFlush = undefined }).catch(() => undefined)
    }
    return pendingFlush
  }
  function cancel(nodeID: string) {
    const entry = entries.get(nodeID)
    if (!entry) return
    clearTimeout(entry.timer)
    entry.controller.abort()
    entries.delete(nodeID)
  }
  async function run(entry: Entry) {
    const { ticket, controller } = entry
    const current = () => !disposed && entries.get(ticket.nodeID) === entry && !controller.signal.aborted && options.isCurrent(ticket.nodeID, ticket.signature)
    const started = performance.now()
    let flushed = started, requested = started
    try {
      if (!current()) return
      options.start(ticket)
      const saved = await flush()
      flushed = performance.now()
      // A newer draft can arrive while a slow database-backed save is pending.
      if (!current()) return
      if (!saved) throw new Error('画布尚未保存，暂时无法估价')
      const result = await options.request(ticket.nodeID, controller.signal)
      requested = performance.now()
      if (!current()) return
      if (options.commit && !await options.commit(ticket, result, current)) return
      if (!current()) return
      successful.set(ticket.nodeID, { signature: ticket.signature, result })
      options.resolve(ticket, result)
      options.timing?.({ nodeID: ticket.nodeID, flushMS: flushed - started, requestMS: requested - flushed, commitMS: performance.now() - requested, totalMS: performance.now() - started })
    } catch (error) {
      if (current()) options.reject(ticket, error)
    }
  }
  function sync(targets: CanvasEstimateTarget[]) {
    if (disposed) return
    const retained = new Set(targets.map((target) => target.nodeID))
    for (const nodeID of new Set([...entries.keys(), ...successful.keys()])) {
      if (retained.has(nodeID)) continue
      cancel(nodeID)
      successful.delete(nodeID)
      options.clear(nodeID)
    }
    for (const target of targets) {
      if (!target.force && entries.get(target.nodeID)?.ticket.signature === target.signature) continue
      cancel(target.nodeID)
      const ticket = { ...target, requestID: ++sequence }
      options.prepare(ticket)
      const entry: Entry = { ticket, controller: new AbortController() }
      entries.set(target.nodeID, entry)
      const cached = successful.get(target.nodeID)
      if (!target.force && cached?.signature === target.signature && (options.canReuse?.(cached.result) ?? true)) {
        options.resolve(ticket, cached.result)
        continue
      }
      entry.timer = setTimeout(() => { entry.timer = undefined; void run(entry) }, options.debounceMS ?? 300)
    }
  }
  return {
    sync,
    dispose() {
      disposed = true
      for (const nodeID of entries.keys()) cancel(nodeID)
      successful.clear()
    },
  }
}
