import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { canHideHistoryRecord, canRetryImageHistory, createHistoryActionLock } from './historyRecordActions'

for (const status of ['succeeded', 'partial', 'partial_failed', 'failed', 'cancelled', 'rejected']) assert.equal(canHideHistoryRecord(status), true)
for (const status of ['queued', 'running', 'submitting', 'artifact_pending', 'recovery_required', 'deleted']) assert.equal(canHideHistoryRecord(status), false)
assert.equal(canRetryImageHistory('failed'), true)
assert.equal(canRetryImageHistory('partial_failed'), true)
assert.equal(canRetryImageHistory('succeeded'), false)

const lock = createHistoryActionLock()
let requests = 0
let release!: () => void
const pending = new Promise<void>((resolve) => { release = resolve })
const first = lock.run(async () => { requests++; await pending })
assert.equal(await lock.run(async () => { requests++ }), false)
assert.equal(requests, 1, 'a second click before rendering must not create another task')
release()
assert.equal(await first, true)
await assert.rejects(lock.run(async () => { throw new Error('network failure') }))
assert.equal(await lock.run(async () => { requests++ }), true, 'errors must release the lock')
assert.equal(requests, 2)

const image = readFileSync(new URL('./WorkspacePage.tsx', import.meta.url), 'utf8')
const video = readFileSync(new URL('../features/creation/VideoCreationPanel.tsx', import.meta.url), 'utf8')
for (const [source, names] of [[image, ['hideTaskHistory', 'clearFailedTaskHistory', 'retryTask']], [video, ['hideVideoTaskHistory', 'clearFailedVideoTaskHistory', 'retryVideoTask']]] as const) {
  for (const name of names) assert.ok(source.includes(`userApi.${name}(`), name)
  assert.ok(source.includes('historyActions.feedback'))
  assert.ok(!source.includes('userApi.deleteTask('), 'history must not use execution deletion')
}
assert.ok(image.includes('preserveIds: [task.id]'))
assert.ok(video.includes('setSelectedTaskID(created.id)'))
assert.ok(video.includes("setTaskTab('current')"))
console.log('History record actions contracts passed')
