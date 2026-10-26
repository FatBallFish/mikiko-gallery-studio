export function canHideHistoryRecord(status: string) {
  return ['succeeded', 'partial', 'partial_failed', 'failed', 'cancelled', 'rejected'].includes(status)
}

export function canRetryImageHistory(status: string) {
  return status === 'failed' || status === 'partial_failed'
}

// A synchronous lock closes the gap before React renders disabled controls.
export function createHistoryActionLock() {
  let pending = false
  return {
    async run(action: () => Promise<void>) {
      if (pending) return false
      pending = true
      try { await action(); return true } finally { pending = false }
    },
  }
}
