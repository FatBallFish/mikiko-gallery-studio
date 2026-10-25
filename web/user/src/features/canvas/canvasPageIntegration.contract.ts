import assert from 'node:assert/strict'
import { canvasAssetNode, canvasConsoleBounds, canvasEstimateSufficient, canvasQuoteRemainingMS, canvasReferenceAsset, insertCanvasReference, validateCanvasReference } from './canvasPageIntegration'
import { createCanvasStore } from './store/canvasStore'
import type { MediaAsset } from '../../../../shared/api-types'
import type { CanvasDocument, CanvasNode } from './core/types'

const target: CanvasNode = { id: 'target', type: 'image_v2', position: { x: 0, y: 0 }, size: { width: 300, height: 300 }, payload: { draft: { prompt_template: 'A landscape', route_model_code: 'image', task_type: 'image_edit' } } }
const document: CanvasDocument = { schema_version: 1, viewport: { x: 0, y: 0, zoom: 1 }, nodes: [target], edges: [] }
const asset = { id: 'asset-1', media_type: 'image', name: 'Same title', status: 'ready_original', width: 1600, height: 900 } as MediaAsset
const source = canvasAssetNode(asset, { x: 0, y: 0 })
assert.equal(source.type, 'image_v2')
assert.equal(canvasReferenceAsset(source).name, source.payload?.name, 'UI aliases and backend aliases must agree')
assert.equal(canvasReferenceAsset({ ...source, payload: { ...source.payload, title: 'Renamed canonical title' } }).name, source.payload?.name)
const store = createCanvasStore(document, 1)
insertCanvasReference(store, asset, target.id, 'reference')
assert.equal(store.getState().command.present.nodes.find((node) => node.id === target.id)?.asset_id, undefined, 'uploads must not fill generators')
assert.equal(store.getState().command.present.edges.length, 1)
assert.deepEqual(store.getState().selectedIDs, ['target'])
assert.throws(() => insertCanvasReference(store, asset, target.id, 'reference'), /已经添加/)
assert.equal(store.getState().command.present.nodes.length, 2, 'duplicate completion must not add orphan nodes')
const deleted = createCanvasStore({ ...document, nodes: [] }, 1)
assert.throws(() => insertCanvasReference(deleted, asset, target.id, 'reference'), /删除/)
const filled = createCanvasStore({ ...document, nodes: [{ ...target, asset_id: 'result' }] }, 1)
assert.throws(() => insertCanvasReference(filled, asset, target.id, 'reference'), /改变/)
assert.equal(filled.getState().command.present.nodes.length, 1, 'late upload must not mutate a changed target')
assert.match(validateCanvasReference({ ...document, nodes: [target, { ...source, type: 'video_v2' }] }, { ...source, type: 'video_v2' }, target.id, 'reference'), /图片/)
const now = Date.parse('2026-10-24T00:00:00Z')
assert.equal(canvasEstimateSufficient('810.00000', { points: '886.00000' }), false)
assert.equal(canvasEstimateSufficient('810.00000', { points: '700.00000', detail: { max_reserved_points: '886.00000' } }), false, 'reserve amount takes precedence over estimated price')
assert.equal(canvasEstimateSufficient('886.00000', { points: '886.00000' }), true)
assert.equal(canvasEstimateSufficient('886.00000', { points: '886.00001' }), false, 'compare all five decimals without rounding')
assert.equal(canvasEstimateSufficient('9007199254740993.00000', { points: '9007199254740993.00001' }), false, 'comparison must not lose precision at large balances')
assert.equal(canvasEstimateSufficient(undefined, { points: '0.00000' }), false)
assert.equal(canvasEstimateSufficient('810.00000', { points: 'invalid' }), false)
const quote = { expires_at: '2026-10-24T00:01:00Z' }
assert.equal(canvasQuoteRemainingMS(quote, now), 55000)
assert.equal(canvasQuoteRemainingMS(quote, now + 55000), 0, 'invalidate before provider expiry, including unchanged drafts')
assert.ok(canvasQuoteRemainingMS(quote, now + 60001) < 0)
assert.equal(canvasQuoteRemainingMS({ expires_at: 'invalid' }, now), 0)
assert.equal(canvasQuoteRemainingMS(undefined, now), Infinity)

const viewport = { width: 1440, height: 842 }
const bottomConsole = canvasConsoleBounds(viewport, { x: 1300, y: 820 }, 620)
assert.equal(bottomConsole.top, 206, 'expanded parameters must move the console upward to keep the footer visible')
assert.equal(bottomConsole.top + 620, viewport.height - 16)
assert.equal(bottomConsole.left + bottomConsole.width, viewport.width - 16)
const hugeConsole = canvasConsoleBounds(viewport, { x: 400, y: 800 }, 1200)
assert.equal(hugeConsole.top, 16)
assert.equal(hugeConsole.maxHeight, 810, 'oversized forms must scroll entirely inside the viewport')
assert.equal(hugeConsole.top + hugeConsole.maxHeight, viewport.height - 16)
const belowNode = canvasConsoleBounds(viewport, { x: 200, y: 200 }, 300)
assert.equal(belowNode.top, 200, 'retain the node anchor when there is room')
for (const size of [{ width: 390, height: 340 }, { width: 800, height: 260 }, { width: 1, height: 1 }]) {
  const bounds = canvasConsoleBounds(size, { x: -200, y: 9999 }, 900)
  assert.ok(bounds.left >= 0 && bounds.top >= 0)
  assert.ok(bounds.left + bounds.width <= size.width)
  assert.ok(bounds.top + bounds.maxHeight <= size.height)
}
