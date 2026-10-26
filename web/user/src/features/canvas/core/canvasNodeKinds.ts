import type { CanvasNode, CanvasNodeType } from './types'

type NodeOrType = CanvasNode | CanvasNodeType
const typeOf = (node: NodeOrType) => typeof node === 'string' ? node : node.type

export function isCanvasV2MediaNode(node: NodeOrType) {
  return typeOf(node) === 'image_v2' || typeOf(node) === 'video_v2'
}
export function isLegacyCanvasNode(node: NodeOrType) {
  return ['prompt', 'image', 'video', 'image_generation', 'video_generation'].includes(typeOf(node))
}
export function canvasNodeMediaKind(node: NodeOrType): 'image' | 'video' | 'audio' | undefined {
  const type = typeOf(node)
  if (type === 'image' || type === 'image_v2' || type === 'image_generation') return 'image'
  if (type === 'video' || type === 'video_v2' || type === 'video_generation') return 'video'
  if (type === 'audio') return 'audio'
}
export function isCanvasGenerationNode(node: CanvasNode) { return isCanvasV2MediaNode(node) && !node.asset_id }
export function canCreateCanvasNode(type: CanvasNodeType) { return !isLegacyCanvasNode(type) }
export function canEditCanvasNode(node: NodeOrType) { return !isLegacyCanvasNode(node) }
export function canCopyCanvasNode(node: NodeOrType) { return canEditCanvasNode(node) }
