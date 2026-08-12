import { createCanvasStore } from './canvasStore'
import { canvasDraftKey, decideCanvasDraftRecovery, type CanvasDraftSnapshot } from '../persistence/canvasDraftPersistence'
import type { CanvasDocument } from '../core/types'

const document: CanvasDocument = {
  schema_version: 1, viewport: { x: 0, y: 0, zoom: 1 }, edges: [],
  nodes: [{ id: 'note', type: 'note', position: { x: 0, y: 0 }, size: { width: 200, height: 140 }, payload: { text: 'draft' } }],
}
const store = createCanvasStore(document, 7)
store.getState().select(['note'])
if (store.getState().selectedEdgeIDs.length) throw new Error('node selection must clear selected edges')
store.getState().moveSelected({ x: 30, y: 20 })
if (store.getState().command.present.nodes[0].position.x !== 30 || !store.getState().command.dirty) throw new Error('store move must update the selected node and dirty state')
store.getState().undo()
if (store.getState().command.present.nodes[0].position.x !== 0) throw new Error('store undo must restore the node')
store.getState().redo()
if (store.getState().command.present.nodes[0].position.x !== 30) throw new Error('store redo must restore the move')
store.getState().copySelected()
store.getState().pasteClipboard()
if (store.getState().command.present.nodes.length !== 2 || store.getState().selectedIDs[0] === 'note') throw new Error('store clipboard paste must select a fresh duplicate')
store.getState().deleteSelected()
if (store.getState().command.present.nodes.length !== 1) throw new Error('store delete must remove pasted selection')

const edgeDocument: CanvasDocument = {
  schema_version: 1, viewport: { x: 0, y: 0, zoom: 1 },
  nodes: [
    { id: 'prompt', type: 'prompt', position: { x: 0, y: 0 }, size: { width: 200, height: 140 } },
    { id: 'generate', type: 'image_generation', position: { x: 300, y: 0 }, size: { width: 280, height: 220 } },
  ],
  edges: [{ id: 'prompt-edge', source: 'prompt', target: 'generate', input_role: 'prompt' }],
}
const edgeStore = createCanvasStore(edgeDocument, 3)
edgeStore.getState().selectEdges(['prompt-edge'])
if (edgeStore.getState().selectedIDs.length || edgeStore.getState().selectedEdgeIDs[0] !== 'prompt-edge') throw new Error('edge selection must be independent and clear node selection')
edgeStore.getState().deleteSelected()
if (edgeStore.getState().command.present.edges.length || edgeStore.getState().command.present.nodes.length !== 2) throw new Error('store delete must remove selected edges without deleting endpoint nodes')
edgeStore.getState().undo()
if (edgeStore.getState().command.present.edges.length !== 1) throw new Error('edge deletion must participate in command history')
store.getState().markSaved(document, 8)
if (store.getState().command.revision !== 8 || store.getState().command.dirty) throw new Error('successful remote save must advance revision and clear dirty state')
const recoveredStore = createCanvasStore(document, 7, { recoveredDraft: true })
if (!recoveredStore.getState().command.dirty) throw new Error('a recovered local draft must remain savable')

const concurrentStore = createCanvasStore(document, 7)
concurrentStore.getState().moveSelected({ x: 0, y: 0 })
concurrentStore.getState().updateNode('note', (node) => ({ ...node, payload: { text: 'submitted' } }))
const submitted = concurrentStore.getState().command.present
concurrentStore.getState().updateNode('note', (node) => ({ ...node, payload: { text: 'newer local edit' } }))
concurrentStore.getState().acknowledgeSave(submitted, 8)
if (concurrentStore.getState().command.revision !== 8 || !concurrentStore.getState().command.dirty) throw new Error('save acknowledgement must advance revision while retaining newer dirty edits')
if (concurrentStore.getState().command.present.nodes[0].payload?.text !== 'newer local edit') throw new Error('save acknowledgement must not overwrite edits made while the request was in flight')

if (canvasDraftKey('user/1', 'canvas/2') !== 'mgs:canvas-draft:v1:user%2F1:canvas%2F2') throw new Error('draft key must be versioned and encode user/canvas identity')
const snapshot: CanvasDraftSnapshot = { schema_version: 1, user_id: 'user/1', canvas_id: 'canvas/2', base_revision: 7, saved_at: '2026-08-12T00:00:00Z', document }
if (decideCanvasDraftRecovery(snapshot, 7, false) !== 'recover_local') throw new Error('matching dirty draft must be recoverable')
if (decideCanvasDraftRecovery(snapshot, 8, false) !== 'conflict') throw new Error('remote revision advance must create an explicit conflict')
if (decideCanvasDraftRecovery(snapshot, 7, true) !== 'discard_local') throw new Error('identical clean remote document must discard stale draft')
