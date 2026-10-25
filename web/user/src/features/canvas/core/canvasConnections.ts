import { inspectCanvasConnection, type CanvasConnectionError } from './canvasState'
import { canvasNodeMediaKind, isCanvasGenerationNode } from './canvasNodeKinds'
import type { CanvasDocument, CanvasEdge, CanvasNode, CanvasPoint } from './types'

export type CanvasHandle = 'left' | 'right'
export type CanvasConnectionDraft = { pointerID: number | null; sourceID: string; sourceHandle: CanvasHandle; point: CanvasPoint; origin: CanvasPoint; targetID: string; targetHandle?: CanvasHandle; error: CanvasConnectionError | null }
export function canvasHandle(value?: string, fallback: CanvasHandle = 'right'): CanvasHandle { return value === 'left' || value === 'target' ? 'left' : value === 'right' || value === 'source' ? 'right' : fallback }

export function resolveCanvasConnection(document: CanvasDocument, startID: string, endID: string, options: { id: string; startHandle?: CanvasHandle; endHandle?: CanvasHandle }) : { edge: CanvasEdge | null; error: CanvasConnectionError | null } {
  const start = document.nodes.find((node) => node.id === startID)
  const end = document.nodes.find((node) => node.id === endID)
  if (!start || !end) return { edge: null, error: 'node_not_found' }
  if (start === end) return { edge: null, error: 'illegal_connection' }
  const reverse = isCanvasGenerationNode(start) && Boolean(end.asset_id)
  const source = reverse ? end : start
  const target = reverse ? start : end
  const kind = canvasNodeMediaKind(source)
  const draft = target.payload?.draft as Record<string, unknown> | undefined
  const task = String(draft?.task_type ?? '')
  let role: CanvasEdge['input_role'] | undefined
  if (kind === 'image' && target.type === 'image_v2') role = 'reference'
  if (target.type === 'video_v2') {
    if (kind === 'image') {
      if (task === 'reference_to_video') role = 'reference_image'
      else if (task === 'image_to_video') role = 'first_frame'
      else if (!document.edges.some((edge) => edge.target === target.id && edge.input_role === 'first_frame')) role = 'first_frame'
      else role = 'last_frame'
    } else if (kind === 'video') role = 'reference_video'
    else if (kind === 'audio') role = 'reference_audio'
  }
  if (!role) return { edge: null, error: 'illegal_connection' }
  // Default sides also follow the semantic endpoints, independent of gesture order.
  const sourceHandle = reverse ? options.endHandle ?? 'right' : options.startHandle ?? 'right'
  const targetHandle = reverse ? options.startHandle ?? 'left' : options.endHandle ?? 'left'
  const edge: CanvasEdge = { id: options.id, source: source.id, target: target.id, source_handle: sourceHandle, target_handle: targetHandle, input_role: role }
  const error = inspectCanvasConnection(document, edge)
  return { edge: error ? null : edge, error }
}

export function canvasConnectionGeometry(source: CanvasNode, target: CanvasNode | CanvasPoint, sourceHandle: CanvasHandle = 'right', targetHandle: CanvasHandle = 'left') {
  const start = { x: source.position.x + (sourceHandle === 'right' ? source.size.width : 0), y: source.position.y + source.size.height / 2 }
  const end = 'position' in target ? { x: target.position.x + (targetHandle === 'right' ? target.size.width : 0), y: target.position.y + target.size.height / 2 } : target
  const bend = Math.max(60, Math.abs(end.x - start.x) * .45)
  const first = { x: start.x + (sourceHandle === 'right' ? bend : -bend), y: start.y }
  const second = { x: end.x + (targetHandle === 'right' ? bend : -bend), y: end.y }
  return { start, end, first, second, path: `M ${start.x} ${start.y} C ${first.x} ${first.y}, ${second.x} ${second.y}, ${end.x} ${end.y}` }
}

export function canvasPointerIntent(options: { node: boolean; space: boolean; button: number; shift: boolean; connect: boolean }) {
  if (options.space || options.button === 1) return 'pan'
  if (options.connect) return 'connect'
  if (options.node) return 'drag'
  return options.shift ? 'marquee' : 'pan'
}
export function canvasConnectionRelease(draft: CanvasConnectionDraft, point: CanvasPoint, targetID: string) {
  if (draft.pointerID === null) return 'pending'
  if (targetID && targetID !== draft.sourceID) return 'finish'
  return Math.hypot(point.x - draft.origin.x, point.y - draft.origin.y) < 5 ? 'pending' : 'menu'
}
