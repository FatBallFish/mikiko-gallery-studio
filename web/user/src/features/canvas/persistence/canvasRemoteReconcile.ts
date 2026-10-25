import { inspectCanvasConnection } from '../core/canvasState'
import type { CanvasCommandState, CanvasDocument, CanvasEdge } from '../core/types'

const equal = (a: unknown, b: unknown): boolean => {
  if (Object.is(a, b)) return true
  if (!a || !b || typeof a !== 'object' || typeof b !== 'object') return false
  if (Array.isArray(a) || Array.isArray(b)) return Array.isArray(a) && Array.isArray(b) && a.length === b.length && a.every((value, index) => equal(value, b[index]))
  const left = a as Record<string, unknown>, right = b as Record<string, unknown>
  const keys = new Set([...Object.keys(left), ...Object.keys(right)])
  return [...keys].every((key) => equal(left[key], right[key]))
}
const canonical = (document: CanvasDocument): CanvasDocument => ({ ...document, groups: document.groups ?? [] })
export const canvasRemoteDocumentsEqual = (left: CanvasDocument, right: CanvasDocument) => equal(canonical(left), canonical(right))

/** Merge independent fields; deletion versus modification and competing field edits conflict. */
export function mergeCanvasRemoteDocument(base: CanvasDocument, local: CanvasDocument, remote: CanvasDocument) {
  const conflicts: string[] = []
  function merge(b: unknown, l: unknown, r: unknown, path: string): unknown {
    if (equal(l, b)) return r
    if (equal(r, b) || equal(l, r)) return l
    if (Array.isArray(b) && Array.isArray(l) && Array.isArray(r) && ['nodes', 'edges', 'groups'].includes(path)) {
      const keyed = (items: unknown[]) => new Map(items.map((item) => [(item as { id: string }).id, item]))
      const bm = keyed(b), lm = keyed(l), rm = keyed(r)
      return [...new Set([...lm.keys(), ...rm.keys(), ...bm.keys()])].flatMap((id) => {
        const value = merge(bm.get(id), lm.get(id), rm.get(id), `${path}.${id}`)
        return value === undefined ? [] : [value]
      })
    }
    if (b && l && r && !Array.isArray(b) && !Array.isArray(l) && !Array.isArray(r) && typeof b === 'object' && typeof l === 'object' && typeof r === 'object') {
      const bo = b as Record<string, unknown>, lo = l as Record<string, unknown>, ro = r as Record<string, unknown>
      return Object.fromEntries([...new Set([...Object.keys(bo), ...Object.keys(lo), ...Object.keys(ro)])].flatMap((key) => {
        const value = merge(bo[key], lo[key], ro[key], path ? `${path}.${key}` : key)
        return value === undefined ? [] : [[key, value]]
      }))
    }
    conflicts.push(path)
    return l
  }
  const document = structuredClone(merge(canonical(base), canonical(local), canonical(remote), '') as CanvasDocument)
  // Validate the combined graph, including interactions between independently added edges.
  const nodes = new Set(document.nodes.map((node) => node.id))
  for (const [index, edge] of document.edges.entries()) {
    const existing = (candidate: CanvasEdge) => equal(candidate, edge)
    const inherited = base.edges.some(existing) || remote.edges.some(existing)
    // Filled generators retain their input references. Validate those references against
    // their generator form; interactive eligibility intentionally excludes filled targets.
    const graph = { ...document, nodes: document.nodes.map((node) => node.id === edge.target && edge.input_role !== 'result' && (node.type === 'image_v2' || node.type === 'video_v2') ? { ...node, asset_id: undefined } : node), edges: document.edges.filter((_, other) => other !== index) }
    const error = inspectCanvasConnection(graph, edge)
    const legacy = document.nodes.some((node) => (node.id === edge.source || node.id === edge.target) && !['image_v2', 'video_v2', 'audio'].includes(node.type))
    // Historical edges remain viewable, but cannot be introduced by a merge.
    if (error && !(inherited && legacy && error === 'illegal_connection')) conflicts.push(`edges.${edge.id}.${error}`)
  }
  for (const group of document.groups ?? []) if (group.node_ids.some((id) => !nodes.has(id))) conflicts.push(`groups.${group.id}`)
  return { document, conflicts }
}

export function rebaseCanvasRemoteState(command: CanvasCommandState, base: CanvasDocument, remote: CanvasDocument, revision: number) {
  const merged = mergeCanvasRemoteDocument(base, command.present, remote)
  if (merged.conflicts.length) return { command, conflicts: merged.conflicts }
  const history = (documents: CanvasDocument[]) => documents.flatMap((document) => {
    const result = mergeCanvasRemoteDocument(base, document, remote)
    return result.conflicts.length ? [] : [result.document]
  })
  return {
    command: { ...command, present: merged.document, revision, dirty: !equal(canonical(merged.document), canonical(remote)), past: history(command.past), future: history(command.future) },
    conflicts: [],
  }
}

/** A canvas session owns one queue, shared by manual saves, autosaves and remote refresh. */
export function createCanvasRemoteOperationQueue() {
  let tail: Promise<unknown> = Promise.resolve()
  return <T>(operation: () => Promise<T>): Promise<T> => {
    const result = tail.then(operation)
    tail = result.catch(() => undefined)
    return result
  }
}
