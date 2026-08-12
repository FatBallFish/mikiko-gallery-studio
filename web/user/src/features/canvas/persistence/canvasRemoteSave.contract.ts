import { createCanvasRemoteSaveScheduler } from './canvasRemoteSave'

const calls: string[] = []
const scheduler = createCanvasRemoteSaveScheduler(async () => { calls.push('save') }, 8)
const callCount = () => calls.length
scheduler.schedule()
scheduler.schedule()
await new Promise((resolve) => setTimeout(resolve, 20))
if (callCount() !== 1) throw new Error(`remote autosave must coalesce rapid edits, got ${callCount()} calls`)

scheduler.schedule()
await scheduler.flush()
if (callCount() !== 2) throw new Error('remote autosave flush must persist the latest pending edit immediately')

scheduler.schedule()
scheduler.cancel()
await new Promise((resolve) => setTimeout(resolve, 20))
if (callCount() !== 2) throw new Error('cancelled remote autosave must not submit after unmount')

let releaseFirstSave!: () => void
const firstSaveBlocked = new Promise<void>((resolve) => { releaseFirstSave = resolve })
let concurrent = 0
let peakConcurrent = 0
let serialCalls = 0
const serialCallCount = () => serialCalls
const serialScheduler = createCanvasRemoteSaveScheduler(async () => {
  serialCalls += 1
  concurrent += 1
  peakConcurrent = Math.max(peakConcurrent, concurrent)
  if (serialCalls === 1) await firstSaveBlocked
  concurrent -= 1
}, 1)
serialScheduler.schedule()
await new Promise((resolve) => setTimeout(resolve, 5))
serialScheduler.schedule()
await new Promise((resolve) => setTimeout(resolve, 5))
if (serialCallCount() !== 1) throw new Error('an edit during an in-flight save must wait for the current revision acknowledgement')
releaseFirstSave()
await serialScheduler.flush()
if (serialCallCount() !== 2) throw new Error(`the latest in-flight edit must be saved in one follow-up request, got ${serialCallCount()}`)
if (peakConcurrent !== 1) throw new Error(`remote saves must be serialized, peak concurrency was ${peakConcurrent}`)
