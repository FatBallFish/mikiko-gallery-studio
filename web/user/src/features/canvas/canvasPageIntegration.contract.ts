import assert from 'node:assert/strict'
import { canvasReferenceAdmission, canvasAssetNode, canvasConsoleBounds, canvasEstimateSufficient, canvasQuoteRemainingMS, canvasReferenceAsset, insertCanvasReference, validateCanvasReference } from './canvasPageIntegration'
import { createCanvasStore } from './store/canvasStore'
import type { Capability, MediaAsset } from '../../../../shared/api-types'
import type { CanvasDocument, CanvasNode } from './core/types'

const target: CanvasNode = { id: 'target', type: 'image_v2', position: { x: 0, y: 0 }, size: { width: 300, height: 300 }, payload: { draft: { prompt_template: 'A landscape', route_model_code: 'image', task_type: 'image_edit' } } }
const document: CanvasDocument = { schema_version: 1, viewport: { x: 0, y: 0, zoom: 1 }, nodes: [target], edges: [] }
const asset = { id: 'asset-1', media_type: 'image', name: 'Same title', status: 'ready_original', width: 1600, height: 900 } as MediaAsset
const source = canvasAssetNode(asset, { x: 0, y: 0 })
assert.equal(source.type, 'image_v2')
assert.equal(canvasReferenceAsset(source).name, source.payload?.name, 'UI aliases and backend aliases must agree')
assert.equal(canvasReferenceAsset({ ...source, payload: { ...source.payload, title: 'Renamed canonical title' } }).name, source.payload?.name)
const context = { image: { model_groups: [{ code: 'image', task_types: ['text_to_image', 'image_edit'], capabilities_by_task_type: { text_to_image: { max_reference_image_count: 0 }, image_edit: { max_reference_image_count: 1 } } }] } as Capability, video: null }
const store = createCanvasStore(document, 1)
insertCanvasReference(store, asset, target.id, 'reference', context)
assert.equal(store.getState().command.present.nodes.find((node) => node.id === target.id)?.asset_id, undefined, 'uploads must not fill generators')
assert.equal(store.getState().command.present.edges.length, 1)
assert.deepEqual(store.getState().selectedIDs, ['target'])
assert.throws(() => insertCanvasReference(store, asset, target.id, 'reference', context), /已经添加/)
assert.equal(store.getState().command.present.nodes.length, 2, 'duplicate completion must not add orphan nodes')
const deleted = createCanvasStore({ ...document, nodes: [] }, 1)
assert.throws(() => insertCanvasReference(deleted, asset, target.id, 'reference', context), /删除/)
const filled = createCanvasStore({ ...document, nodes: [{ ...target, asset_id: 'result' }] }, 1)
assert.throws(() => insertCanvasReference(filled, asset, target.id, 'reference', context), /改变/)
assert.equal(filled.getState().command.present.nodes.length, 1, 'late upload must not mutate a changed target')
assert.match(validateCanvasReference({ ...document, nodes: [target, { ...source, type: 'video_v2' }] }, { ...source, type: 'video_v2' }, target.id, 'reference', context), /图片/)
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

const before = store.getState().command.present
assert.throws(() => insertCanvasReference(store, { ...asset, id: 'asset-2' }, target.id, 'reference', context), /最多支持/)
assert.equal(store.getState().command.present, before, 'rejected admission is atomic, without orphan nodes')
assert.equal(canvasReferenceAdmission(document, target.id, [{ id: 'queued', kind: 'image' }], { ...context, reserved: { image: 1 } }).ok, false, 'pending uploads reserve the remaining slot')
assert.equal(canvasReferenceAdmission(document, target.id, [{ id: 'first', kind: 'image' }], context).ok, true, 'first reference uses edit capability, not generate max0')
assert.match(canvasReferenceAdmission(document, target.id, [{ id: 'first', kind: 'image' }], { image: null, video: null }).reason, /加载/)
assert.equal(canvasReferenceAdmission(before, target.id, [], { ...context, image: { ...context.image, model_groups: [] } }).ok, false)
assert.equal(store.getState().command.present, before, 'existing invalid references remain intact')

const videoTarget: CanvasNode = { ...target, type: 'video_v2', payload: { draft: { route_model_code: 'video', task_type: 'reference_to_video' } } }
const videoDocument = { ...document, nodes: [videoTarget] }
const videoContext = { image: null, video: { model_groups: [{ code: 'video', task_types: ['reference_to_video'], options_by_task_type: { reference_to_video: { inputs: { reference_image: { required: false, max_count: 2, media_types: ['image'], formats: ['png'] }, reference_video: { required: false, max_count: 1, media_types: ['video'], formats: ['mp4'] } } } } }] } as import('../../../../shared/api-types').VideoCapability }
assert.equal(canvasReferenceAdmission(videoDocument, target.id, [{ id: 'i1', kind: 'image' }, { id: 'i2', kind: 'image' }, { id: 'v1', kind: 'video' }], videoContext).ok, true, 'image and video counts have separate model limits')
assert.equal(canvasReferenceAdmission(videoDocument, target.id, [{ id: 'v1', kind: 'video' }, { id: 'v2', kind: 'video' }], videoContext).ok, false)
assert.equal(canvasReferenceAdmission(videoDocument, target.id, [{ id: 'a1', kind: 'audio' }], videoContext).ok, false, 'unknown audio capability blocks rather than inventing capacity')
assert.equal(canvasReferenceAdmission(document, target.id, [{ id: 'one', kind: 'image' }, { id: 'one', kind: 'image' }], context).added.length, 1, 'batch deduplication precedes capacity validation')

const { settleCanvasReferenceUpload } = await import('./canvasPageIntegration')
const { createReferenceReservations } = await import('../../../../shared/reference-admission')
const uploads = createReferenceReservations<'image'>()
const reservedUpload = uploads.reserve({ limits: { image: 1 } }, [], [{ kind: 'image' as const }], (item) => item.kind)
const targets = new Map([['slow-upload', { nodeID: 'target', reservation: reservedUpload.token }]])
assert.equal(uploads.pending().image, 1)
// No wall-clock expiry: a still-running slow upload retains its destination and reservation.
assert.equal(targets.has('slow-upload'), true)
settleCanvasReferenceUpload(targets, 'slow-upload', 'failed', (target) => { if (target.reservation !== undefined) uploads.release(target.reservation) })
assert.equal(uploads.pending().image, undefined, 'failure releases the slot immediately')
assert.equal(targets.get('slow-upload')?.nodeID, 'target', 'resumed success retains the original destination')
settleCanvasReferenceUpload(targets, 'slow-upload', 'cancelled', (target) => { if (target.reservation !== undefined) uploads.release(target.reservation) })
assert.equal(targets.has('slow-upload'), false, 'cancel prevents any late attachment')

const { videoDraftInvalidReason } = await import('../creation/videoDraft')
const hydratedDraft = { route_model_code: 'video', task_type: 'reference_to_video', inputs: [{ asset_id: 'hydrating', role: 'reference_image', ordinal: 0 }] } as import('../creation/videoDraft').VideoDraft
assert.match(videoDraftInvalidReason(hydratedDraft, videoContext.video, {}), /读取|不可用/, 'estimate and generation wait for hydrated assets')
const incompatible = { id: 'hydrating', media_type: 'image', mime_type: 'image/jpeg', name: 'bad.jpg', file_size_bytes: 10 } as MediaAsset
assert.match(videoDraftInvalidReason(hydratedDraft, videoContext.video, { hydrating: incompatible }), /格式/)
const sizeCapability = structuredClone(videoContext.video)
sizeCapability.model_groups[0].options_by_task_type.reference_to_video!.inputs!.reference_image!.max_bytes = 5
assert.match(videoDraftInvalidReason(hydratedDraft, sizeCapability, { hydrating: { ...incompatible, name: 'large.png', mime_type: 'image/png' } }), /大小/)
assert.equal(videoDraftInvalidReason(hydratedDraft, videoContext.video, { hydrating: { ...incompatible, name: 'ok.png', mime_type: 'image/png' } }), '')
