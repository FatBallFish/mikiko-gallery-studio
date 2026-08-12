export function createCanvasRemoteSaveScheduler(save: () => Promise<unknown>, delay = 900) {
  let timer: ReturnType<typeof setTimeout> | null = null
  let pending = false
  let inFlight: Promise<void> | null = null

  function run(): Promise<void> {
    if (timer) clearTimeout(timer)
    timer = null
    if (!inFlight && pending) {
      inFlight = (async () => {
        while (pending) {
          pending = false
          await save()
        }
      })().finally(() => { inFlight = null })
    }
    return inFlight ?? Promise.resolve()
  }

  return {
    schedule() {
      pending = true
      if (timer) clearTimeout(timer)
      timer = setTimeout(() => { void run() }, delay)
    },
    async flush() {
      return run()
    },
    cancel() {
      if (timer) clearTimeout(timer)
      timer = null
      pending = false
    },
  }
}
