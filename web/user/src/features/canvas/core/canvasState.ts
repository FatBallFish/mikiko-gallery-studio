import type { CanvasClipboard, CanvasCommandState, CanvasDocument, CanvasEdge, CanvasNode, CanvasNodeType, CanvasPoint, CanvasResult } from './types'

const HISTORY_LIMIT = 100

export function createCanvasState(document: CanvasDocument, revision: number): CanvasCommandState {
  return { present: cloneDocument(document), past: [], future: [], revision, dirty: false }
}

export function addCanvasNode(state: CanvasCommandState, node: CanvasNode) {
  if (state.present.nodes.some((item) => item.id === node.id)) throw new Error('duplicate_node')
  return commit(state, { ...state.present, nodes: [...state.present.nodes, cloneNode(node)] })
}

export function moveCanvasNodes(state: CanvasCommandState, nodeIDs: string[], delta: { x: number; y: number }) {
  const selected = new Set(nodeIDs)
  if (!selected.size || (!delta.x && !delta.y)) return state
  return commit(state, {
    ...state.present,
    nodes: state.present.nodes.map((node) => selected.has(node.id) ? {
      ...node,
      position: { x: node.position.x + delta.x, y: node.position.y + delta.y },
    } : node),
  })
}

export function updateCanvasNode(state: CanvasCommandState, nodeID: string, update: (node: CanvasNode) => CanvasNode) {
  const current = state.present.nodes.find((node) => node.id === nodeID)
  if (!current) return state
  const next = update(cloneNode(current))
  if (next.id !== nodeID) throw new Error('node_id_immutable')
  return commit(state, { ...state.present, nodes: state.present.nodes.map((node) => node.id === nodeID ? cloneNode(next) : node) })
}

export function removeCanvasNodes(state: CanvasCommandState, nodeIDs: string[]) {
  const removed = new Set(nodeIDs)
  if (!removed.size || !state.present.nodes.some((node) => removed.has(node.id))) return state
  return commit(state, {
    ...state.present,
    nodes: state.present.nodes.filter((node) => !removed.has(node.id)),
    edges: state.present.edges.filter((edge) => !removed.has(edge.source) && !removed.has(edge.target)),
  })
}

export function removeCanvasEdges(state: CanvasCommandState, edgeIDs: string[]) {
  const removed = new Set(edgeIDs)
  if (!removed.size || !state.present.edges.some((edge) => removed.has(edge.id))) return state
  return commit(state, { ...state.present, edges: state.present.edges.filter((edge) => !removed.has(edge.id)) })
}

export function copyCanvasSelection(document: CanvasDocument, nodeIDs: string[]): CanvasClipboard {
  const selected = new Set(nodeIDs)
  return {
    nodes: document.nodes.filter((node) => selected.has(node.id)).map(cloneNode),
    edges: document.edges.filter((edge) => selected.has(edge.source) && selected.has(edge.target)).map((edge) => ({ ...edge })),
  }
}

export function pasteCanvasSelection(state: CanvasCommandState, clipboard: CanvasClipboard, offset: CanvasPoint, idFor: (sourceID: string) => string) {
  if (!clipboard.nodes.length) return state
  const nodeIDs = new Map(clipboard.nodes.map((node) => [node.id, idFor(node.id)]))
  const existing = new Set(state.present.nodes.map((node) => node.id))
  if (Array.from(nodeIDs.values()).some((id) => existing.has(id)) || new Set(nodeIDs.values()).size !== nodeIDs.size) throw new Error('duplicate_node')
  const nodes = clipboard.nodes.map((node) => ({
    ...cloneNode(node), id: nodeIDs.get(node.id)!, position: { x: node.position.x + offset.x, y: node.position.y + offset.y },
  }))
  const edges = clipboard.edges.map((edge) => ({
    ...edge, id: idFor(edge.id), source: nodeIDs.get(edge.source)!, target: nodeIDs.get(edge.target)!,
  }))
  return commit(state, { ...state.present, nodes: [...state.present.nodes, ...nodes], edges: [...state.present.edges, ...edges] })
}

export function connectCanvasNodes(state: CanvasCommandState, edge: CanvasEdge) {
  if (state.present.edges.some((item) => item.id === edge.id)) return state
  const error = inspectCanvasConnection(state.present, edge)
  if (error) throw new Error(error)
  const next = { ...state.present, edges: [...state.present.edges, { ...edge }] }
  return commit(state, next)
}

export type CanvasConnectionError = 'node_not_found' | 'illegal_connection' | 'input_role_conflict' | 'cycle'

export function inspectCanvasConnection(document: CanvasDocument, edge: CanvasEdge): CanvasConnectionError | null {
  const source = document.nodes.find((node) => node.id === edge.source)
  const target = document.nodes.find((node) => node.id === edge.target)
  if (!source || !target) return 'node_not_found'
  if (!isLegalConnection(source.type, target.type, edge.input_role)) return 'illegal_connection'
  if ((edge.input_role === 'first_frame' || edge.input_role === 'last_frame') && document.edges.some((item) => item.target === edge.target && item.input_role === edge.input_role)) {
    return 'input_role_conflict'
  }
  return hasDirectedCycle({ ...document, edges: [...document.edges, { ...edge }] }) ? 'cycle' : null
}

export function compatibleCanvasTargets(_document: CanvasDocument, sourceID: string): Array<{ type: CanvasNodeType; role: CanvasEdge['input_role'] }> {
  const source = _document.nodes.find((node) => node.id === sourceID)
  if (!source) return []
  if (source.type === 'prompt') return [
    { type: 'image_generation', role: 'prompt' },
    { type: 'video_generation', role: 'prompt' },
  ]
  if (source.type === 'image') return [
    { type: 'image_generation', role: 'reference' },
    { type: 'video_generation', role: 'first_frame' },
  ]
  return []
}

export function selectCanvasNodesInRect(nodes: CanvasNode[], rect: { x: number; y: number; width: number; height: number }) {
  const right = rect.x + rect.width
  const bottom = rect.y + rect.height
  return nodes.filter((node) => (
    node.position.x >= rect.x && node.position.y >= rect.y
    && node.position.x + node.size.width <= right
    && node.position.y + node.size.height <= bottom
  )).map((node) => node.id)
}

export function attachCanvasResults(state: CanvasCommandState, runID: string, sourceNodeID: string, results: CanvasResult[]) {
  const source = state.present.nodes.find((node) => node.id === sourceNodeID)
  if (!source) return state
  let nodes = state.present.nodes
  let edges = state.present.edges
  results.forEach((result, index) => {
    const nodeID = stableResultNodeID(runID, result.asset_id)
    if (nodes.some((node) => node.id === nodeID)) return
    const node: CanvasNode = {
      id: nodeID,
      type: result.media_type,
      asset_id: result.asset_id,
      position: { x: source.position.x + source.size.width + 80, y: source.position.y + index * 220 },
      size: result.media_type === 'video' ? { width: 320, height: 200 } : { width: 280, height: 220 },
    }
    nodes = [...nodes, node]
    edges = [...edges, { id: `edge-${nodeID}`, source: sourceNodeID, target: nodeID, input_role: 'result' }]
  })
  return nodes === state.present.nodes ? state : commit(state, { ...state.present, nodes, edges })
}

export function undoCanvasCommand(state: CanvasCommandState): CanvasCommandState {
  const previous = state.past[state.past.length - 1]
  if (!previous) return state
  return { ...state, present: previous, past: state.past.slice(0, -1), future: [state.present, ...state.future], dirty: true }
}

export function redoCanvasCommand(state: CanvasCommandState): CanvasCommandState {
  const next = state.future[0]
  if (!next) return state
  return { ...state, present: next, past: [...state.past, state.present].slice(-HISTORY_LIMIT), future: state.future.slice(1), dirty: true }
}

export function markCanvasSaved(state: CanvasCommandState, document: CanvasDocument, revision: number): CanvasCommandState {
  return { ...state, present: cloneDocument(document), revision, dirty: false }
}

export function acknowledgeCanvasSave(state: CanvasCommandState, submitted: CanvasDocument, revision: number): CanvasCommandState {
  const unchanged = JSON.stringify(state.present) === JSON.stringify(submitted)
  return unchanged ? markCanvasSaved(state, submitted, revision) : { ...state, revision, dirty: true }
}

function commit(state: CanvasCommandState, document: CanvasDocument): CanvasCommandState {
  return { ...state, present: document, past: [...state.past, state.present].slice(-HISTORY_LIMIT), future: [], dirty: true }
}

function cloneDocument(document: CanvasDocument): CanvasDocument {
  return {
    schema_version: 1,
    viewport: { ...document.viewport },
    nodes: document.nodes.map(cloneNode),
    edges: document.edges.map((edge) => ({ ...edge })),
  }
}

function cloneNode(node: CanvasNode): CanvasNode {
  return { ...node, position: { ...node.position }, size: { ...node.size }, payload: node.payload ? { ...node.payload } : undefined }
}

function isLegalConnection(source: CanvasNodeType, target: CanvasNodeType, role: CanvasEdge['input_role']) {
  if (source === 'prompt' && (target === 'image_generation' || target === 'video_generation')) return role === 'prompt'
  if (source === 'image' && target === 'image_generation') return role === 'reference'
  if (source === 'image' && target === 'video_generation') return role === 'first_frame' || role === 'last_frame'
  if ((source === 'image_generation' && target === 'image') || (source === 'video_generation' && target === 'video')) return role === 'result'
  return false
}

function hasDirectedCycle(document: CanvasDocument) {
  const adjacency = new Map(document.nodes.map((node) => [node.id, [] as string[]]))
  document.edges.forEach((edge) => adjacency.get(edge.source)?.push(edge.target))
  const visiting = new Set<string>()
  const visited = new Set<string>()
  function visit(nodeID: string): boolean {
    if (visiting.has(nodeID)) return true
    if (visited.has(nodeID)) return false
    visiting.add(nodeID)
    for (const target of adjacency.get(nodeID) ?? []) if (visit(target)) return true
    visiting.delete(nodeID)
    visited.add(nodeID)
    return false
  }
  return document.nodes.some((node) => visit(node.id))
}

function stableResultNodeID(runID: string, assetID: string) {
  let hash = 2166136261
  const input = `${runID}\0${assetID}`
  for (let index = 0; index < input.length; index += 1) {
    hash ^= input.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  return `result-${(hash >>> 0).toString(16).padStart(8, '0')}`
}
