import {
  addCanvasNode,
  attachCanvasResults,
  connectCanvasNodes,
  copyCanvasSelection,
  createCanvasState,
  pasteCanvasSelection,
  removeCanvasNodes,
  moveCanvasNodes,
  redoCanvasCommand,
  selectCanvasNodesInRect,
  undoCanvasCommand,
  updateCanvasNode,
} from './canvasState'
import type { CanvasDocument, CanvasNode } from './types'

const nodes: CanvasNode[] = [
  { id: 'prompt', type: 'prompt', position: { x: 0, y: 0 }, size: { width: 240, height: 160 }, payload: { text: 'camera move' } },
  { id: 'image', type: 'image', asset_id: 'asset-image', position: { x: 300, y: 0 }, size: { width: 240, height: 180 } },
  { id: 'video', type: 'video', asset_id: 'asset-video', position: { x: 600, y: 0 }, size: { width: 280, height: 180 } },
  { id: 'audio', type: 'audio', asset_id: 'asset-audio', position: { x: 900, y: 0 }, size: { width: 280, height: 140 } },
  { id: 'image-gen', type: 'image_generation', position: { x: 300, y: 300 }, size: { width: 300, height: 220 } },
  { id: 'video-gen', type: 'video_generation', position: { x: 700, y: 300 }, size: { width: 300, height: 220 } },
  { id: 'note', type: 'note', position: { x: 0, y: 300 }, size: { width: 220, height: 150 }, payload: { text: 'note' } },
]
const document: CanvasDocument = { schema_version: 1, viewport: { x: 0, y: 0, zoom: 1 }, nodes, edges: [] }

let state = createCanvasState(document, 4)
if (new Set(state.present.nodes.map((node) => node.type)).size !== 7) throw new Error('canvas state must preserve all seven P0 node types')

state = moveCanvasNodes(state, ['prompt'], { x: 80, y: 40 })
if (state.present.nodes[0].position.x !== 80) throw new Error('move command must update the selected node')
state = undoCanvasCommand(state)
if (state.present.nodes[0].position.x !== 0) throw new Error('undo must restore the previous document')
state = redoCanvasCommand(state)
if (state.present.nodes[0].position.x !== 80) throw new Error('redo must restore the moved document')

const selected = selectCanvasNodesInRect(state.present.nodes, { x: 0, y: 0, width: 620, height: 560 })
if (!selected.includes('prompt') || !selected.includes('image-gen') || selected.includes('video')) throw new Error(`selection rectangle is incorrect: ${selected}`)

state = connectCanvasNodes(state, { id: 'prompt-image', source: 'prompt', target: 'image-gen', input_role: 'prompt' })
state = connectCanvasNodes(state, { id: 'image-ref', source: 'image', target: 'image-gen', input_role: 'reference' })

const copied = copyCanvasSelection(state.present, ['prompt', 'image-gen'])
const pasted = pasteCanvasSelection(state, copied, { x: 48, y: 32 }, (id) => `copy-${id}`)
if (!pasted.present.nodes.some((node) => node.id === 'copy-prompt' && node.position.x === 128)) throw new Error('paste must create offset nodes with fresh ids')
if (!pasted.present.edges.some((edge) => edge.source === 'copy-prompt' && edge.target === 'copy-image-gen')) throw new Error('paste must recreate internal edges between copied nodes')

const removed = removeCanvasNodes(state, ['image-gen'])
if (removed.present.nodes.some((node) => node.id === 'image-gen')) throw new Error('delete must remove selected nodes')
if (removed.present.edges.some((edge) => edge.source === 'image-gen' || edge.target === 'image-gen')) throw new Error('delete must remove every connected edge')
if (undoCanvasCommand(removed).present.nodes.every((node) => node.id !== 'image-gen')) throw new Error('delete must be undoable')
let illegal = false
try {
  connectCanvasNodes(state, { id: 'bad-audio', source: 'audio', target: 'video-gen', input_role: 'reference' })
} catch (error) {
  illegal = error instanceof Error && error.message.includes('illegal_connection')
}
if (!illegal) throw new Error('illegal connections must be rejected with an actionable code')

const cycleSource = addCanvasNode(state, { id: 'result-image', type: 'image', asset_id: 'asset-result', position: { x: 800, y: 600 }, size: { width: 240, height: 180 } })
const withResult = connectCanvasNodes(cycleSource, { id: 'result-link', source: 'image-gen', target: 'result-image', input_role: 'result' })
let cycle = false
try {
  connectCanvasNodes(withResult, { id: 'cycle-link', source: 'result-image', target: 'image-gen', input_role: 'reference' })
} catch (error) {
  cycle = error instanceof Error && error.message.includes('cycle')
}
if (!cycle) throw new Error('directed generation cycles must be rejected')

const attached = attachCanvasResults(state, 'run-1', 'image-gen', [{ asset_id: 'asset-new', media_type: 'image' }])
const attachedAgain = attachCanvasResults(attached, 'run-1', 'image-gen', [{ asset_id: 'asset-new', media_type: 'image' }])
if (attachedAgain.present.nodes.filter((node) => node.asset_id === 'asset-new').length !== 1) throw new Error('result attachment must be stable and idempotent')

if (state.revision !== 4 || state.dirty !== true) throw new Error('local commands must preserve remote revision and mark the draft dirty')

const updated = updateCanvasNode(state, 'video-gen', (node) => ({ ...node, payload: { draft: { quote_token: 'signed-quote' } } }))
if ((updated.present.nodes.find((node) => node.id === 'video-gen')?.payload?.draft as { quote_token?: string })?.quote_token !== 'signed-quote') throw new Error('node payload updates must support quote-before-generate')
if (undoCanvasCommand(updated).present.nodes.find((node) => node.id === 'video-gen')?.payload?.draft) throw new Error('node payload updates must be undoable')
