import { canvasConnectionGeometry, canvasConnectionRelease, canvasHandle, canvasPointerIntent, resolveCanvasConnection, type CanvasConnectionDraft } from './canvasConnections'
import { connectCanvasNodes, createCanvasState } from './canvasState'
import type { CanvasDocument, CanvasNode } from './types'
function assert(value: unknown, message: string): asserts value { if (!value) throw new Error(message) }
const node = (id: string, type: CanvasNode['type'], asset_id?: string, task_type = ''): CanvasNode => ({ id, type, asset_id, position: { x: 100, y: 80 }, size: { width: 320, height: 240 }, payload: { draft: { task_type } } })
const image = node('image', 'image_v2', 'asset')
const empty = node('empty', 'image_v2')
const video = node('video', 'video_v2', undefined, 'reference_to_video')
const other = node('other', 'image_v2', 'other-asset')
const document: CanvasDocument = { schema_version: 1, viewport: { x: 0, y: 0, zoom: 1 }, nodes: [image, empty, video, other, node('legacy', 'image', 'old'), node('audio', 'audio', 'audio')], edges: [] }
for (const startHandle of ['left', 'right'] as const) for (const endHandle of ['left', 'right'] as const) for (const reverse of [false, true]) {
  const candidate = resolveCanvasConnection(document, reverse ? empty.id : image.id, reverse ? image.id : empty.id, { id: 'edge', startHandle, endHandle })
  assert(candidate.edge && !candidate.error, 'either endpoint and all side combinations connect')
  assert(candidate.edge.source === image.id && candidate.edge.target === empty.id && candidate.edge.input_role === 'reference', 'gesture order never changes generation semantics')
  assert(candidate.edge.source_handle === (reverse ? endHandle : startHandle) && candidate.edge.target_handle === (reverse ? startHandle : endHandle), 'visual sides follow node identity after reversal')
  const state = connectCanvasNodes(createCanvasState(document, 1), candidate.edge)
  assert(state.present.edges.length === 1, 'resolved connection passes command validator')
  assert(resolveCanvasConnection(state.present, empty.id, image.id, { id: 'repeat' }).error === 'input_role_conflict', 'reverse duplicate is rejected')
}
for (const pair of [['image', 'other'], ['empty', 'video'], ['legacy', 'empty'], ['audio', 'empty'], ['empty', 'empty']]) assert(!resolveCanvasConnection(document, pair[0], pair[1], { id: 'invalid' }).edge, 'filled, empty, legacy, wrong kind and self connections rejected')
assert(resolveCanvasConnection(document, 'missing', 'empty', { id: 'missing' }).error === 'node_not_found', 'missing nodes rejected')
assert(resolveCanvasConnection(document, video.id, image.id, { id: 'video', startHandle: 'right' }).edge?.input_role === 'reference_image', 'video mode determines reference role, not port')
const frameDocument = { ...document, nodes: document.nodes.map((n) => n.id === video.id ? node(video.id, 'video_v2', undefined, 'first_last_frame_to_video') : n) }
const first = resolveCanvasConnection(frameDocument, image.id, video.id, { id: 'first' }).edge!
assert(first.input_role === 'first_frame', 'first free frame slot inferred')
assert(resolveCanvasConnection({ ...frameDocument, edges: [first] }, video.id, other.id, { id: 'last' }).edge?.input_role === 'last_frame', 'reverse start fills second frame slot')
const cyclic = { ...document, edges: [{ id: 'historical', source: empty.id, target: image.id, input_role: 'reference' as const }] }
assert(resolveCanvasConnection(cyclic, empty.id, image.id, { id: 'cycle' }).error === 'cycle', 'historical directed cycle stays guarded')
for (const side of ['left', 'right'] as const) for (const endSide of ['left', 'right'] as const) {
  const geometry = canvasConnectionGeometry(image, empty, side, endSide)
  assert((geometry.first.x - geometry.start.x) * (side === 'right' ? 1 : -1) > 0, 'start curve exits outward')
  assert((geometry.second.x - geometry.end.x) * (endSide === 'right' ? 1 : -1) > 0, 'end curve approaches outward')
  assert(canvasConnectionGeometry(image, geometry.end, side, endSide).path === geometry.path, 'ghost and committed geometry share path')
}
assert(canvasHandle('target') === 'left' && canvasHandle('source') === 'right', 'historical visual handles remain compatible')
const intent = { node: false, space: false, button: 0, shift: false, connect: false }
assert(canvasPointerIntent(intent) === 'pan', 'background pans by default')
assert(canvasPointerIntent({ ...intent, node: true }) === 'drag', 'node surface drags')
assert(canvasPointerIntent({ ...intent, node: true, space: true, connect: true }) === 'pan', 'Space pan overrides nodes and connection tool')
assert(canvasPointerIntent({ ...intent, shift: true }) === 'marquee', 'Shift background explicitly selects rectangle')
const draft: CanvasConnectionDraft = { pointerID: 1, sourceID: image.id, sourceHandle: 'left', origin: { x: 0, y: 0 }, point: { x: 0, y: 0 }, targetID: '', error: null }
assert(canvasConnectionRelease(draft, { x: 1, y: 1 }, image.id) === 'pending', 'first click persists pointer ghost')
assert(canvasConnectionRelease({ ...draft, pointerID: null }, { x: 100, y: 10 }, empty.id) === 'pending', 'pending pointerup cannot duplicate second-click action')
assert(canvasConnectionRelease(draft, { x: 100, y: 10 }, empty.id) === 'finish', 'drag ends on supported other node')
assert(canvasConnectionRelease(draft, { x: 100, y: 10 }, '') === 'menu', 'drag to background offers compatible creation')
