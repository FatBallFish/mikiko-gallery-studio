import { createCanvasEstimateCoordinator } from './canvasEstimateCoordinator'

function assert(value: unknown, message: string): asserts value { if (!value) throw new Error(message) }
function deferred<T>() { let resolve!: (value: T) => void; let reject!: (error: unknown) => void; const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail }); return { promise, resolve, reject } }
const tick = () => new Promise((resolve) => setTimeout(resolve, 5))
let signature = 'A'
let flushCount = 0
const saved = deferred<boolean>()
const requests: Array<{ nodeID: string; signal: AbortSignal; result: ReturnType<typeof deferred<string>> }> = []
const resolved: string[] = [], errors: unknown[] = []
const coordinator = createCanvasEstimateCoordinator({
  debounceMS: 0,
  flush: () => { flushCount++; return saved.promise },
  isCurrent: (_nodeID, value) => value === signature,
  request: (nodeID, signal) => { const result = deferred<string>(); requests.push({ nodeID, signal, result }); return result.promise },
  prepare: () => undefined, start: () => undefined, clear: () => undefined,
  resolve: (_ticket, result) => { resolved.push(result) },
  reject: (_ticket, error) => { errors.push(error) },
})
coordinator.sync([{ nodeID: 'one', signature: 'A' }])
await tick()
signature = 'B'
coordinator.sync([{ nodeID: 'one', signature: 'B' }, { nodeID: 'two', signature: 'B' }])
await tick()
assert(flushCount === 1, 'all nodes must share a pending flush')
saved.resolve(true)
await tick()
assert(requests.length === 2, 'only latest B may issue HTTP after deferred A flush')
requests[0].result.resolve('B-one'); requests[1].result.resolve('B-two')
await tick()
const count = () => requests.length
coordinator.sync([{ nodeID: 'one', signature: 'B' }, { nodeID: 'two', signature: 'B' }])
await tick()
assert(count() === 2 && flushCount === 1, 'unchanged poll/save renders must not repeat estimates or flush')
signature = 'C'
coordinator.sync([{ nodeID: 'one', signature }])
await tick()
const stale = requests[2]
signature = 'D'
coordinator.sync([{ nodeID: 'one', signature }])
assert(stale.signal.aborted, 'new signatures must abort in-flight HTTP')
stale.result.reject(new Error('superseded'))
await tick()
requests[3].result.resolve('D')
await tick()
assert(errors.length === 0 && !resolved.includes('C'), 'superseded errors/results must stay invisible')
coordinator.sync([{ nodeID: 'one', signature, force: true }])
await tick()
assert(count() === 5, 'forced expiry/manual retry must bypass successful signature cache')
coordinator.sync([])
assert(requests[4].signal.aborted, 'removed or ineligible nodes must abort requests')
requests[4].result.reject(new Error('removed'))
await tick()
coordinator.dispose()
assert(errors.length === 0, 'removed-node errors must stay invisible')
let reusable = true, cachedRequests = 0, cachedResolutions = 0
const cacheCoordinator = createCanvasEstimateCoordinator({
  debounceMS: 0, flush: async () => true, isCurrent: () => true,
  canReuse: () => reusable,
  request: async () => { cachedRequests++; return 'quote' },
  prepare: () => undefined, start: () => undefined, clear: () => undefined, reject: () => undefined,
  resolve: () => { cachedResolutions++ },
})
cacheCoordinator.sync([{ nodeID: 'video', signature: 'A' }])
await tick()
cacheCoordinator.sync([{ nodeID: 'video', signature: 'B' }])
cacheCoordinator.sync([{ nodeID: 'video', signature: 'A' }])
await tick()
assert(cachedRequests === 1, 'valid successful signatures must reuse cache after draft reversion')
cacheCoordinator.sync([{ nodeID: 'video', signature: 'B' }])
reusable = false
const resolutionsBeforeExpiry = cachedResolutions
cacheCoordinator.sync([{ nodeID: 'video', signature: 'A' }])
assert(cachedResolutions === resolutionsBeforeExpiry, 'expired cached quote must never become ready, even briefly')
await tick()
assert(Number(cachedRequests) === 2, 'expired cached signatures must request a fresh quote')
cacheCoordinator.dispose()
console.log('canvas estimate coordinator contracts passed')
