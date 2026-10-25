export function createCanvasRemoteSaveScheduler(save: () => Promise<unknown>, delay = 900) {
  let timer: ReturnType<typeof setTimeout> | null = null
  let pending = false
  let inFlight: Promise<void> | null = null

  function run(): Promise<void> {
    if (timer !== null) clearTimeout(timer)
    timer = null
    if (!inFlight && pending) {
      // Install the shared promise before invoking save, including synchronous callbacks.
      inFlight = Promise.resolve().then(async () => {
        while (pending) {
          pending = false
          if (await save() === false) { pending = false; break }
        }
      }).finally(() => { inFlight = null })
    }
    return inFlight ?? Promise.resolve()
  }

  return {
    schedule() {
      pending = true
      if (timer !== null) clearTimeout(timer)
      timer = setTimeout(() => { void run().catch(() => undefined) }, delay)
    },
    flush: run,
    cancel() {
      if (timer !== null) clearTimeout(timer)
      timer = null
      pending = false
      // The active request remains owned by this scheduler until it settles.
    },
  }
}
