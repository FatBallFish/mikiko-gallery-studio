import assert from 'node:assert/strict'
import { mock } from 'node:test'
import { readFileSync } from 'node:fs'
import { canvasRemoteDocumentsEqual, createCanvasRemoteOperationQueue, mergeCanvasRemoteDocument, rebaseCanvasRemoteState } from './canvasRemoteReconcile'
import { createCanvasRemoteSaveScheduler } from './canvasRemoteSave'
import type { CanvasDocument } from '../core/types'

const base: CanvasDocument = { schema_version: 1, viewport: { x: 0, y: 0, zoom: 1 }, nodes: [{ id: 'n', type: 'image_v2', position: { x: 0, y: 0 }, size: { width: 420, height: 340 }, payload: { draft: { prompt: 'original' }, title: 'node' } }], edges: [] }
assert.equal(canvasRemoteDocumentsEqual(base, { ...base, viewport: { zoom: 1, y: 0, x: 0 }, groups: [] }), true)
const local = structuredClone(base)
local.nodes[0].position.x = 70
local.nodes[0].payload!.draft = { prompt: 'edited' }
const remote = structuredClone(base)
remote.nodes[0].asset_id = 'result'
remote.nodes[0].payload!.title = 'result title'
const merged = mergeCanvasRemoteDocument(base, local, remote)
assert.deepEqual(merged.conflicts, [])
assert.equal(merged.document.nodes[0].position.x, 70)
assert.equal(merged.document.nodes[0].asset_id, 'result')
assert.deepEqual(merged.document.nodes[0].payload!.draft, { prompt: 'edited' })
const competing = structuredClone(remote)
competing.nodes[0].payload!.draft = { prompt: 'other editor' }
assert.deepEqual(mergeCanvasRemoteDocument(base, local, competing).conflicts, ['nodes.n.payload.draft.prompt'])
assert.ok(mergeCanvasRemoteDocument(base, { ...local, nodes: [] }, remote).conflicts.length)
const rebased = rebaseCanvasRemoteState({ present: local, past: [base], future: [], revision: 1, dirty: true }, base, remote, 2)
assert.equal(rebased.command.revision, 2)
assert.equal(rebased.command.dirty, true)
assert.equal(rebased.command.past[0].nodes[0].asset_id, 'result', 'undo must retain attached result')
const acknowledged = rebaseCanvasRemoteState(rebased.command, rebased.command.present, rebased.command.present, 3)
assert.equal(acknowledged.command.dirty, false)

// The conflict-copy action must leave the blocked original and open the copy.
const page = readFileSync(new URL('../CanvasEditorPage.tsx', import.meta.url), 'utf8')
const copyAction = page.slice(page.indexOf('onClick={() => void userApi.createCanvas('), page.indexOf('复制本地版本'))
assert.ok(copyAction.includes("app.navigate('creative-canvas', { canvasId: copy.id })"))
assert.ok(!copyAction.includes('setConflict(null)'), 'copying must not dismiss the blocked original session')

const graphBase: CanvasDocument = { ...base, nodes: [
  { ...base.nodes[0], id: 'a', asset_id: 'asset-a' },
  { ...base.nodes[0], id: 'b', asset_id: 'asset-b' },
  { ...base.nodes[0], id: 'video', type: 'video_v2' },
] }
const leftFrame = { ...graphBase, edges: [{ id: 'left-frame', source: 'a', target: 'video', input_role: 'first_frame' as const }] }
const rightFrame = { ...graphBase, edges: [{ id: 'right-frame', source: 'b', target: 'video', input_role: 'first_frame' as const }] }
assert.deepEqual(mergeCanvasRemoteDocument(graphBase, graphBase, leftFrame).conflicts, [])
assert.ok(mergeCanvasRemoteDocument(graphBase, leftFrame, rightFrame).conflicts.some((path) => path.endsWith('input_role_conflict')), 'independent first-frame additions must conflict')
const cycleLeft = { ...graphBase, edges: [{ id: 'ab', source: 'a', target: 'b', input_role: 'reference' as const }] }
const cycleRight = { ...graphBase, edges: [{ id: 'ba', source: 'b', target: 'a', input_role: 'reference' as const }] }
assert.ok(mergeCanvasRemoteDocument(graphBase, cycleLeft, cycleRight).conflicts.some((path) => path.endsWith('cycle')))
const incompatible = { ...graphBase, edges: [{ id: 'bad', source: 'a', target: 'video', input_role: 'reference_audio' as const }] }
assert.ok(mergeCanvasRemoteDocument(graphBase, graphBase, incompatible).conflicts.some((path) => path.endsWith('illegal_connection')))
const invalidHistory = rebaseCanvasRemoteState({ present: graphBase, past: [leftFrame], future: [leftFrame], revision: 1, dirty: false }, graphBase, rightFrame, 2)
assert.deepEqual(invalidHistory.conflicts, [])
assert.equal(invalidHistory.command.past.length, 0, 'undo snapshots with merged role collisions must be discarded')
assert.equal(invalidHistory.command.future.length, 0, 'redo snapshots with merged role collisions must be discarded')

const deferred = () => { let resolve!: () => void; const promise = new Promise<void>((done) => { resolve = done }); return { promise, resolve } }
const tick = async () => { for (let i = 0; i < 12; i++) await Promise.resolve() }
mock.timers.enable({ apis: ['setTimeout'] })
try {
  const first = deferred()
  const queue = createCanvasRemoteOperationQueue()
  let active = 0, peak = 0, calls = 0
  let save = async () => {
    calls++; active++; peak = Math.max(peak, active)
    if (calls === 1) await first.promise
    active--
    return true
  }
  const scheduler = createCanvasRemoteSaveScheduler(() => queue(() => save()), 900)
  scheduler.schedule(); mock.timers.tick(900); await tick()
  assert.equal(calls, 1)
  // Effect rerenders update the callback without replacing scheduler/queue.
  const originalSave = save
  save = () => originalSave()
  scheduler.schedule()
  const manual = scheduler.flush()
  const direct = queue(() => save())
  await tick()
  assert.equal(calls, 1)
  first.resolve(); await Promise.all([manual, direct])
  assert.equal(calls, 3)
  assert.equal(peak, 1)
  scheduler.schedule(); scheduler.cancel(); scheduler.schedule()
  mock.timers.tick(900); await scheduler.flush()
  assert.equal(calls, 4, 'new scheduled work survives earlier cancellation')
  const failed = createCanvasRemoteSaveScheduler(async () => { failed.schedule(); return false }, 900)
  failed.schedule(); await failed.flush(); failed.cancel()
  scheduler.cancel()
} finally { mock.timers.reset() }
console.log('canvas remote reconciliation contracts passed')
