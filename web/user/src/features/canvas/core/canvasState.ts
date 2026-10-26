import { canCreateCanvasNode, canCopyCanvasNode, canEditCanvasNode, isCanvasGenerationNode, isCanvasV2MediaNode, canvasNodeMediaKind } from './canvasNodeKinds'
import { canvasMediaSize, resizeCanvasMediaSize } from './canvasMediaSizing'
import { parsePromptTemplate } from '../../../pages/promptTemplateParser'
import type { CanvasClipboard, CanvasCommandState, CanvasDocument, CanvasEdge, CanvasGroup, CanvasNode, CanvasNodeType, CanvasPoint, CanvasResult } from './types'

const HISTORY_LIMIT = 100

export function createCanvasState(document: CanvasDocument, revision: number): CanvasCommandState {
  return { present: cloneDocument(document), past: [], future: [], revision, dirty: false }
}

export function addCanvasNode(state: CanvasCommandState, node: CanvasNode) {
  if (!canCreateCanvasNode(node.type)) return state
  if (state.present.nodes.some((item) => item.id === node.id)) throw new Error('duplicate_node')
  return commit(state, { ...state.present, nodes: [...state.present.nodes, cloneNode(node)] })
}

export function moveCanvasNodes(state: CanvasCommandState, nodeIDs: string[], delta: { x: number; y: number }) {
  const selected = new Set(state.present.nodes.filter((node) => nodeIDs.includes(node.id) && canEditCanvasNode(node)).map((node) => node.id))
  if (!selected.size || (!delta.x && !delta.y)) return state
  return commit(state, {
    ...state.present,
    nodes: state.present.nodes.map((node) => selected.has(node.id) ? {
      ...node,
      position: { x: node.position.x + delta.x, y: node.position.y + delta.y },
    } : node),
  })
}

export function resizeCanvasNode(state: CanvasCommandState, nodeID: string, size: { width: number; height: number }) {
  const current = state.present.nodes.find((node) => node.id === nodeID)
  if (!current || !canEditCanvasNode(current)) return state
  const minimum = canvasNodeMinimumSize(current.type)
  const nextSize = isCanvasV2MediaNode(current) && current.asset_id ? resizeCanvasMediaSize(current.size, size, canvasNodeAspectRatio(current)) : {
    width: Math.max(minimum.width, Number.isFinite(size.width) ? size.width : current.size.width),
    height: Math.max(minimum.height, Number.isFinite(size.height) ? size.height : current.size.height),
  }
  if (nextSize.width === current.size.width && nextSize.height === current.size.height) return state
  return commit(state, {
    ...state.present,
    nodes: state.present.nodes.map((node) => node.id === nodeID ? { ...node, size: nextSize } : node),
  })
}

export function canvasNodeMinimumSize(nodeType: CanvasNodeType) {
  if (nodeType === 'image_v2' || nodeType === 'video_v2' || nodeType === 'image' || nodeType === 'video') return { width: 220, height: 160 }
  if (nodeType === 'audio') return { width: 240, height: 120 }
  if (nodeType === 'image_generation' || nodeType === 'video_generation') return { width: 280, height: 200 }
  return { width: 220, height: 140 }
}

export function canvasImageSizeDraftPatch(sizeMode: string, options: { base_resolution?: readonly string[]; aspect_ratios?: readonly string[]; pixel_sizes?: readonly string[] }) {
  return {
    size_mode: sizeMode,
    requested_size: sizeMode === 'pixel' ? options.pixel_sizes?.[0] ?? '' : '',
    base_resolution: sizeMode === 'ratio' ? options.base_resolution?.[0] ?? '' : '',
    aspect_ratio: sizeMode === 'ratio' ? options.aspect_ratios?.[0] ?? '' : '',
  }
}

type CanvasImageTaskOptions = {
  size_modes?: readonly string[]
  base_resolution?: readonly string[]
  aspect_ratios?: readonly string[]
  pixel_sizes?: readonly string[]
  quality?: readonly string[]
  output_format?: readonly string[]
}

type CanvasImageModelCapability = CanvasImageTaskOptions & {
  code: string
  task_types: readonly string[]
  capabilities_by_task_type?: Partial<Record<'text_to_image' | 'image_edit', CanvasImageTaskOptions>>
}

export function canvasImageDraftForTask(
  draft: Record<string, unknown>,
  capability: { model_groups: readonly CanvasImageModelCapability[] },
  taskType: 'text_to_image' | 'image_edit',
): Record<string, unknown> & { task_type: 'text_to_image' | 'image_edit' } {
  const supportedModels = capability.model_groups.filter((model) => model.task_types.includes(taskType))
  const currentCode = String(draft.route_model_code ?? draft.abstract_model ?? '').trim()
  const model = currentCode ? supportedModels.find((item) => item.code === currentCode) : supportedModels[0]
  if (!model) return { ...draft, task_type: taskType }

  const scoped = model.capabilities_by_task_type?.[taskType]
  const options = {
    size_modes: scoped?.size_modes ?? model.size_modes ?? ['auto'],
    base_resolution: scoped?.base_resolution ?? model.base_resolution ?? [],
    aspect_ratios: scoped?.aspect_ratios ?? model.aspect_ratios ?? [],
    pixel_sizes: scoped?.pixel_sizes ?? model.pixel_sizes ?? [],
    quality: scoped?.quality ?? model.quality ?? [],
    output_format: scoped?.output_format ?? model.output_format ?? [],
  }
  const currentSizeMode = String(draft.size_mode ?? '').trim()
  const sizeMode = options.size_modes.includes(currentSizeMode)
    ? currentSizeMode
    : options.size_modes.includes('auto') ? 'auto' : options.size_modes[0] ?? ''
  const sizePatch = canvasImageSizeDraftPatch(sizeMode, options)
  if (sizeMode === 'ratio') {
    if (options.base_resolution.includes(String(draft.base_resolution ?? ''))) sizePatch.base_resolution = String(draft.base_resolution)
    if (options.aspect_ratios.includes(String(draft.aspect_ratio ?? ''))) sizePatch.aspect_ratio = String(draft.aspect_ratio)
  }
  if (sizeMode === 'pixel' && options.pixel_sizes.includes(String(draft.requested_size ?? ''))) {
    sizePatch.requested_size = String(draft.requested_size)
  }
  const currentQuality = String(draft.quality ?? '')
  const currentOutputFormat = String(draft.output_format ?? '')
  return {
    ...draft,
    route_model_code: model.code,
    task_type: taskType,
    ...sizePatch,
    quality: options.quality.includes(currentQuality) ? currentQuality : options.quality[0] ?? '',
    output_format: options.output_format.includes(currentOutputFormat) ? currentOutputFormat : options.output_format[0] ?? '',
  }
}

export function canvasImageParameterErrors(
  draft: Record<string, unknown>,
  capability: { model_groups: readonly CanvasImageModelCapability[] },
  taskType: 'text_to_image' | 'image_edit',
  maxOutputImageCount = 10,
) {
  const supportedModels = capability.model_groups.filter((model) => model.task_types.includes(taskType))
  if (!supportedModels.length) return [`当前没有支持${taskType === 'image_edit' ? '图片编辑' : '图片生成'}的模型分组`]
  const modelCode = String(draft.route_model_code ?? draft.abstract_model ?? '').trim()
  const model = supportedModels.find((item) => item.code === modelCode)
  if (!model) return modelCode ? ['模型分组当前不可用'] : ['请选择模型分组']

  const scoped = model.capabilities_by_task_type?.[taskType]
  const options = {
    size_modes: scoped?.size_modes ?? model.size_modes ?? ['auto'],
    base_resolution: scoped?.base_resolution ?? model.base_resolution ?? [],
    aspect_ratios: scoped?.aspect_ratios ?? model.aspect_ratios ?? [],
    pixel_sizes: scoped?.pixel_sizes ?? model.pixel_sizes ?? [],
    quality: scoped?.quality ?? model.quality ?? [],
    output_format: scoped?.output_format ?? model.output_format ?? [],
  }
  const errors: string[] = []
  const sizeMode = String(draft.size_mode ?? '').trim()
  if (!sizeMode || !options.size_modes.includes(sizeMode)) errors.push('请选择有效的尺寸模式')
  if (sizeMode === 'ratio') {
    if (!options.base_resolution.includes(String(draft.base_resolution ?? ''))) errors.push('请选择基础分辨率')
    if (!options.aspect_ratios.includes(String(draft.aspect_ratio ?? ''))) errors.push('请选择图片比例')
  }
  if (sizeMode === 'pixel' && !options.pixel_sizes.includes(String(draft.requested_size ?? ''))) errors.push('请选择像素尺寸')
  if (options.quality.length && !options.quality.includes(String(draft.quality ?? ''))) errors.push('请选择图片质量')
  if (options.output_format.length && !options.output_format.includes(String(draft.output_format ?? ''))) errors.push('请选择输出格式')
  const count = Number(draft.output_image_count ?? 1)
  if (!Number.isInteger(count) || count < 1 || count > maxOutputImageCount) errors.push(`生成数量需为 1-${maxOutputImageCount} 的整数`)
  return errors
}

export function updateCanvasNode(state: CanvasCommandState, nodeID: string, update: (node: CanvasNode) => CanvasNode) {
  const current = state.present.nodes.find((node) => node.id === nodeID)
  if (!current || !canEditCanvasNode(current)) return state
  const next = update(cloneNode(current))
  if (next.id !== nodeID) throw new Error('node_id_immutable')
  if (next.type !== current.type) throw new Error('node_type_immutable')
  if (current.payload?.name && next.payload?.name !== current.payload.name) next.payload = { ...next.payload, name: current.payload.name }
  return commit(state, { ...state.present, nodes: state.present.nodes.map((node) => node.id === nodeID ? cloneNode(next) : node) })
}

export function removeCanvasNodes(state: CanvasCommandState, nodeIDs: string[]) {
  const removed = new Set(nodeIDs)
  if (!removed.size || !state.present.nodes.some((node) => removed.has(node.id))) return state
  const groups = (state.present.groups ?? [])
    .map((group) => ({ ...group, node_ids: group.node_ids.filter((id) => !removed.has(id)) }))
    .filter((group) => group.node_ids.length > 0)
  return commit(state, {
    ...state.present,
    nodes: state.present.nodes.filter((node) => !removed.has(node.id)),
    edges: state.present.edges.filter((edge) => !removed.has(edge.source) && !removed.has(edge.target)),
    groups,
  })
}

export function createCanvasGroup(state: CanvasCommandState, group: CanvasGroup) {
  if (state.present.groups?.some((item) => item.id === group.id)) throw new Error('duplicate_group')
  if (group.node_ids.some((id) => state.present.nodes.some((node) => node.id === id && !canEditCanvasNode(node)))) return state
  const nodeIDs = new Set(state.present.nodes.map((node) => node.id))
  if (!group.node_ids.every((id) => nodeIDs.has(id))) throw new Error('node_not_found')
  return commit(state, { ...state.present, groups: [...(state.present.groups ?? []), { ...group, node_ids: [...group.node_ids] }] })
}

export function updateCanvasGroup(state: CanvasCommandState, groupID: string, patch: Partial<Pick<CanvasGroup, 'label' | 'background'>>) {
  const current = state.present.groups?.find((group) => group.id === groupID)
  if (!current || current.node_ids.some((id) => state.present.nodes.some((node) => node.id === id && !canEditCanvasNode(node)))) return state
  return commit(state, {
    ...state.present,
    groups: (state.present.groups ?? []).map((group) => group.id === groupID ? { ...group, ...patch } : group),
  })
}

export function disbandCanvasGroups(state: CanvasCommandState, groupIDs: string[]) {
  const removed = new Set(groupIDs)
  const groups = (state.present.groups ?? []).filter((group) => !removed.has(group.id) || group.node_ids.some((id) => state.present.nodes.some((node) => node.id === id && !canEditCanvasNode(node))))
  if (groups.length === (state.present.groups ?? []).length) return state
  return commit(state, { ...state.present, groups })
}

export function canvasGroupsForNode(document: CanvasDocument, nodeID: string): CanvasGroup[] {
  return (document.groups ?? []).filter((group) => group.node_ids.includes(nodeID))
}

export function canvasGroupMembers(document: CanvasDocument, nodeIDs: string[]): string[] {
  const groups = new Set<string>()
  nodeIDs.forEach((id) => (document.groups ?? []).forEach((group) => { if (group.node_ids.includes(id)) groups.add(group.id) }))
  const members = new Set(nodeIDs)
  ;(document.groups ?? []).forEach((group) => { if (groups.has(group.id)) group.node_ids.forEach((id) => members.add(id)) })
  return Array.from(members)
}

export function canvasGroupBounds(document: CanvasDocument, group: CanvasGroup, padding = 28) {
  const nodes = group.node_ids
    .map((id) => document.nodes.find((node) => node.id === id))
    .filter((node): node is CanvasNode => Boolean(node))
  if (!nodes.length) return null
  const minX = Math.min(...nodes.map((node) => node.position.x))
  const minY = Math.min(...nodes.map((node) => node.position.y))
  const maxX = Math.max(...nodes.map((node) => node.position.x + node.size.width))
  const maxY = Math.max(...nodes.map((node) => node.position.y + node.size.height))
  return { x: minX - padding, y: minY - padding - 24, width: maxX - minX + padding * 2, height: maxY - minY + padding * 2 + 24 }
}

export function removeCanvasEdges(state: CanvasCommandState, edgeIDs: string[]) {
  const removed = new Set(edgeIDs)
  if (!removed.size || !state.present.edges.some((edge) => removed.has(edge.id))) return state
  return commit(state, { ...state.present, edges: state.present.edges.filter((edge) => !removed.has(edge.id)) })
}

export function copyCanvasSelection(document: CanvasDocument, nodeIDs: string[]): CanvasClipboard {
  const selected = new Set(document.nodes.filter((node) => nodeIDs.includes(node.id) && canCopyCanvasNode(node)).map((node) => node.id))
  return {
    nodes: document.nodes.filter((node) => selected.has(node.id)).map(cloneNode),
    edges: document.edges.filter((edge) => selected.has(edge.source) && selected.has(edge.target)).map((edge) => ({ ...edge })),
  }
}

export function pasteCanvasSelection(state: CanvasCommandState, clipboard: CanvasClipboard, offset: CanvasPoint, idFor: (sourceID: string) => string) {
  clipboard = copyCanvasSelection({ ...state.present, nodes: clipboard.nodes, edges: clipboard.edges }, clipboard.nodes.map((node) => node.id))
  if (!clipboard.nodes.length) return state
  const nodeIDs = new Map(clipboard.nodes.map((node) => [node.id, idFor(node.id)]))
  const existing = new Set(state.present.nodes.map((node) => node.id))
  if (Array.from(nodeIDs.values()).some((id) => existing.has(id)) || new Set(nodeIDs.values()).size !== nodeIDs.size) throw new Error('duplicate_node')
  const nodes = clipboard.nodes.map((node) => ({
    ...cloneNode(node), id: nodeIDs.get(node.id)!, position: { x: node.position.x + offset.x, y: node.position.y + offset.y },
    payload: isCanvasV2MediaNode(node) ? { ...structuredClone(node.payload ?? {}), name: `resource_${nodeIDs.get(node.id)!.replace(/[^a-zA-Z0-9_]/g, '_')}` } : structuredClone(node.payload),
  }))
  const edges = clipboard.edges.map((edge) => ({
    ...edge, id: idFor(edge.id), source: nodeIDs.get(edge.source)!, target: nodeIDs.get(edge.target)!,
  }))
  const aliases = new Map(clipboard.nodes.filter((node) => isCanvasV2MediaNode(node) && node.payload?.name).map((node) => [String(node.payload!.name), String(nodes.find((item) => item.id === nodeIDs.get(node.id))?.payload?.name)]))
  nodes.forEach((node) => {
    if (!isCanvasV2MediaNode(node)) return
    const draft = asCanvasObject(node.payload?.draft)
    if (typeof draft.prompt_template !== 'string') return
    const prompt_template = draft.prompt_template.replace(/\{\{@([^{}]+)\}\}/g, (match, name: string) => aliases.has(name) ? `{{@${aliases.get(name)}}}` : match)
    node.payload = { ...node.payload, draft: { ...draft, prompt_template } }
  })
  return commit(state, { ...state.present, nodes: [...state.present.nodes, ...nodes], edges: [...state.present.edges, ...edges] })
}

export function compareCanvasReferenceEdges(a: CanvasEdge, b: CanvasEdge) {
  return (a.ordinal ?? 0) - (b.ordinal ?? 0) || (a.source < b.source ? -1 : a.source > b.source ? 1 : 0)
}
export function nextCanvasReferenceOrdinal(edges: CanvasEdge[], target: string) {
  return edges.filter((edge) => edge.target === target).reduce((maximum, edge) => Math.max(maximum, edge.ordinal ?? 0), -1) + 1
}

export function connectCanvasNodes(state: CanvasCommandState, edge: CanvasEdge) {
  if (state.present.edges.some((item) => item.id === edge.id)) return state
  const error = inspectCanvasConnection(state.present, edge)
  if (error) throw new Error(error)
  const next = { ...state.present, edges: [...state.present.edges, { ...edge, ordinal: edge.ordinal ?? nextCanvasReferenceOrdinal(state.present.edges, edge.target) }] }
  return commit(state, next)
}

export type CanvasConnectionError = 'node_not_found' | 'illegal_connection' | 'input_role_conflict' | 'output_slot_occupied' | 'cycle'

export function inspectCanvasConnection(document: CanvasDocument, edge: CanvasEdge): CanvasConnectionError | null {
  const source = document.nodes.find((node) => node.id === edge.source)
  const target = document.nodes.find((node) => node.id === edge.target)
  if (!source || !target) return 'node_not_found'
  if (!isLegalConnection(source, target, edge.input_role)) return 'illegal_connection'
  if (document.edges.some((item) => item.target === edge.target && item.input_role === edge.input_role && (item.source === edge.source || document.nodes.find((node) => node.id === item.source)?.asset_id === source.asset_id))) return 'input_role_conflict'
  if (edge.input_role === 'result' && target.asset_id && !document.edges.some((item) => item.source === edge.source && item.target === edge.target && item.input_role === 'result')) {
    return 'output_slot_occupied'
  }
  if ((edge.input_role === 'first_frame' || edge.input_role === 'last_frame') && document.edges.some((item) => item.target === edge.target && item.input_role === edge.input_role)) {
    return 'input_role_conflict'
  }
  return hasDirectedCycle({ ...document, edges: [...document.edges, { ...edge }] }) ? 'cycle' : null
}

export function compatibleCanvasTargets(_document: CanvasDocument, sourceID: string): Array<{ type: CanvasNodeType; role: CanvasEdge['input_role'] }> {
  const source = _document.nodes.find((node) => node.id === sourceID)
  if (!source) return []
  if (!source.asset_id || !canEditCanvasNode(source)) return []
  if (source.type === 'image_v2') return [
    { type: 'image_v2', role: 'reference' },
    { type: 'video_v2', role: 'first_frame' },
    { type: 'video_v2', role: 'last_frame' },
    { type: 'video_v2', role: 'reference_image' },
  ]
  if (source.type === 'video_v2') return [{ type: 'video_v2', role: 'reference_video' }]
  if (source.type === 'audio') return [{ type: 'video_v2', role: 'reference_audio' }]
  return []
}

export type CanvasPromptResourceCandidate = { nodeID: string; assetID: string; name: string; duplicateName: boolean; mimeType?: string; width?: number; height?: number }

export type CanvasEstimateState = {
  status: 'waiting' | 'loading' | 'ready' | 'error'
  signature: string
  requestID: number
  points?: string
  detail?: Record<string, unknown>
  error?: string
}

export function canvasGenerationEstimateSignature(document: CanvasDocument, nodeID: string) {
  const node = document.nodes.find((item) => item.id === nodeID)
  if (!node || !isCanvasGenerationNode(node)) return ''
  const nodeByID = new Map(document.nodes.map((item) => [item.id, item]))
  const inputs = document.edges
    .filter((edge) => edge.target === nodeID && edge.input_role !== 'result')
    .map((edge) => ({ edge, source: nodeByID.get(edge.source) }))
    .sort((a, b) => a.edge.input_role.localeCompare(b.edge.input_role) || (a.edge.ordinal ?? 0) - (b.edge.ordinal ?? 0) || a.edge.id.localeCompare(b.edge.id))
    .map(({ edge, source }) => ({
      edge: { id: edge.id, source: edge.source, role: edge.input_role, ordinal: edge.ordinal ?? 0 },
      source: source ? { id: source.id, type: source.type, assetID: source.asset_id ?? '', payload: source.payload ?? {} } : null,
    }))
  return JSON.stringify({ node: { id: node.id, type: node.type, draft: estimateDraft(node) }, inputs })
}

export function canvasImageTaskType(document: CanvasDocument, nodeID: string): 'text_to_image' | 'image_edit' {
  const nodeByID = new Map(document.nodes.map((node) => [node.id, node]))
  return document.edges.some((edge) => edge.target === nodeID && edge.input_role === 'reference' && nodeByID.get(edge.source)?.type === 'image_v2' && Boolean(nodeByID.get(edge.source)?.asset_id)) ? 'image_edit' : 'text_to_image'
}

export function prepareCanvasEstimate(current: CanvasEstimateState | undefined, signature: string, eligible: boolean, requestID: number) {
  if (!eligible || !signature) return undefined
  if (current?.signature === signature) return current
  return { status: 'waiting' as const, signature, requestID }
}

export function startCanvasEstimate(current: CanvasEstimateState | undefined, signature: string, requestID: number) {
  if (!current || current.signature !== signature || current.requestID !== requestID) return current
  return { ...current, status: 'loading' as const, error: undefined }
}

export function resolveCanvasEstimate(current: CanvasEstimateState | undefined, signature: string, requestID: number, result: { points: string; detail?: Record<string, unknown> }) {
  if (!current || current.signature !== signature || current.requestID !== requestID) return current
  return { status: 'ready' as const, signature, requestID, points: result.points, detail: result.detail }
}

export function rejectCanvasEstimate(current: CanvasEstimateState | undefined, signature: string, requestID: number, error: string) {
  if (!current || current.signature !== signature || current.requestID !== requestID) return current
  return { status: 'error' as const, signature, requestID, error }
}

export function canvasPromptResourceCandidates(document: CanvasDocument, promptNodeID: string): CanvasPromptResourceCandidate[] {
  const direct = document.nodes.find((node) => node.id === promptNodeID && isCanvasV2MediaNode(node))
  const generationIDs = direct ? new Set([direct.id]) : new Set(document.edges.filter((edge) => edge.source === promptNodeID && edge.input_role === 'prompt')
    .map((edge) => document.nodes.find((node) => node.id === edge.target))
    .filter((node): node is CanvasNode => node?.type === 'image_generation')
    .map((node) => node.id))
  const nodeByID = new Map(document.nodes.map((node) => [node.id, node]))
  const seen = new Set<string>()
  const candidates = document.edges.filter((edge) => generationIDs.has(edge.target) && ['reference', 'first_frame', 'last_frame', 'reference_image', 'reference_video', 'reference_audio'].includes(edge.input_role))
    .map((edge, edgeIndex) => ({ edge, edgeIndex, node: nodeByID.get(edge.source) }))
    .filter((item): item is { edge: CanvasEdge; edgeIndex: number; node: CanvasNode } => Boolean(item.node && (isCanvasV2MediaNode(item.node) || item.node.type === 'image' || item.node.type === 'audio') && item.node.asset_id))
    .sort((a, b) => (a.edge.ordinal ?? a.edgeIndex) - (b.edge.ordinal ?? b.edgeIndex))
    .flatMap(({ node }) => {
      if (seen.has(node.id)) return []
      seen.add(node.id)
      const name = String(node.payload?.name ?? '').trim()
      return name ? [{ nodeID: node.id, assetID: node.asset_id!, name, mimeType: String(node.payload?.mime_type ?? '') || undefined, width: Number(node.payload?.width) || undefined, height: Number(node.payload?.height) || undefined }] : []
    })
  const nameCounts = new Map<string, number>()
  candidates.forEach((candidate) => nameCounts.set(candidate.name, (nameCounts.get(candidate.name) ?? 0) + 1))
  return candidates.map((candidate) => ({ ...candidate, duplicateName: (nameCounts.get(candidate.name) ?? 0) > 1 }))
}

export type CanvasPromptOptimizationTarget = { mediaType?: 'image' | 'video'; routeModelCode?: string }

// The optimization guide follows the connected generation node: a video node
// selects its route model's guide, an image node the image prompt, and a
// prompt node with no generation downstream stays untyped.
export function canvasPromptOptimizationTarget(document: CanvasDocument, promptNodeID: string): CanvasPromptOptimizationTarget {
  const direct = document.nodes.find((node) => node.id === promptNodeID && isCanvasV2MediaNode(node))
  if (direct) {
    const draft = asCanvasObject(direct.payload?.draft)
    return { mediaType: direct.type === 'video_v2' ? 'video' : 'image', routeModelCode: String(draft.route_model_code ?? '').trim() || undefined }
  }
  const targets = document.edges.filter((edge) => edge.source === promptNodeID && edge.input_role === 'prompt')
    .map((edge) => document.nodes.find((node) => node.id === edge.target))
    .filter((node): node is CanvasNode => node?.type === 'image_generation' || node?.type === 'video_generation')
  const video = targets.find((node) => node.type === 'video_generation')
  if (video) {
    const draft = video.payload?.draft
    const code = draft && typeof draft === 'object' ? String((draft as Record<string, unknown>).route_model_code ?? '').trim() : ''
    return { mediaType: 'video', routeModelCode: code || undefined }
  }
  return targets.length ? { mediaType: 'image' } : {}
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
  if (results.some((result) => result.layer)) {
    const ordered = [...results].sort((a, b) => (a.layer?.z_index ?? 0) - (b.layer?.z_index ?? 0))
    const base = ordered.find((result) => result.layer?.is_base) ?? ordered[0]
    const scale = source.size.width / (base.width || source.size.width)
    const added: CanvasNode[] = ordered.filter((result) => !state.present.nodes.some((node) => node.id === stableResultNodeID(runID, result.asset_id))).map((result) => {
      const b = result.layer?.bounding_box?.absolute ?? [0, 0, result.width || base.width || 1, result.height || base.height || 1]
      return { id: stableResultNodeID(runID, result.asset_id), type: 'image_v2', asset_id: result.asset_id, position: { x: source.position.x + source.size.width + 80 + b[0] * scale, y: source.position.y + b[1] * scale }, size: { width: (b[2] - b[0]) * scale, height: (b[3] - b[1]) * scale }, payload: { name: result.layer?.name, layer: result.layer, run_id: runID, width: result.width, height: result.height } }
    })
    if (!added.length) return state
    return commit(state, { ...state.present, nodes: [...state.present.nodes, ...added], groups: [...(state.present.groups ?? []), { id: `layers-${runID}`, label: '拆分图层', node_ids: added.map((node) => node.id) }] })
  }
  if (isCanvasV2MediaNode(source)) {
    const kind = canvasNodeMediaKind(source)
    if (results.some((result) => result.media_type !== kind)) throw new Error('result_media_mismatch')
    let nodes = state.present.nodes
    results.forEach((result, index) => {
      if (nodes.some((node) => node.asset_id === result.asset_id && (node.id === sourceNodeID || node.id === stableResultNodeID(runID, result.asset_id)))) return
      const payload = { width: result.width, height: result.height, name: `resource_${stableResultNodeID(runID, result.asset_id).slice(7)}`, run_id: runID }
      const size = canvasMediaSize(result.width ?? source.size.width, result.height ?? source.size.height)
      if (index === 0 && source.asset_id) throw new Error('output_slot_occupied')
      if (index === 0 && !source.asset_id) {
        nodes = nodes.map((node) => node.id === sourceNodeID ? { ...node, asset_id: result.asset_id, payload: { ...node.payload, ...payload, name: node.payload?.name ?? payload.name }, size } : node)
      } else {
        const id = stableResultNodeID(runID, result.asset_id)
        if (nodes.some((node) => node.id === id)) throw new Error('result_node_conflict')
        nodes = [...nodes, { id, type: source.type, asset_id: result.asset_id, payload, size, position: { x: source.position.x + source.size.width + 80, y: source.position.y + index * 280 } }]
      }
    })
    return nodes === state.present.nodes ? state : commit(state, { ...state.present, nodes })
  }
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
    groups: (document.groups ?? []).map((group) => ({ ...group, node_ids: [...group.node_ids] })),
  }
}

function cloneNode(node: CanvasNode): CanvasNode {
  return { ...node, position: { ...node.position }, size: { ...node.size }, payload: node.payload ? structuredClone(node.payload) : undefined }
}

function isLegalConnection(source: CanvasNode, target: CanvasNode, role: CanvasEdge['input_role']) {
  if (!source.asset_id || !canEditCanvasNode(source) || !isCanvasGenerationNode(target)) return false
  if (source.type === 'image_v2' && target.type === 'image_v2') return role === 'reference'
  if (source.type === 'image_v2' && target.type === 'video_v2') return ['first_frame', 'last_frame', 'reference_image'].includes(role)
  if (source.type === 'video_v2' && target.type === 'video_v2') return role === 'reference_video'
  return source.type === 'audio' && target.type === 'video_v2' && role === 'reference_audio'
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

function asCanvasObject(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}

export function canvasNodeAspectRatio(node: CanvasNode): number {
  const width = Number(node.payload?.width)
  const height = Number(node.payload?.height)
  return width > 0 && height > 0 && Number.isFinite(width / height) ? width / height : node.size.width / node.size.height
}

export function canvasInlinePromptTemplate(value: string) {
  return value.replace(/(^|[^\\])\$\{([^{}]+)\}/g, (_match, prefix: string, name: string) => `${prefix}{{$${name}}}`)
}

export function canvasVideoTaskType(document: CanvasDocument, nodeID: string): 'text_to_video' | 'image_to_video' | 'first_last_frame_to_video' | 'reference_to_video' | 'video_edit' | 'video_extend' {
  const node = document.nodes.find((item) => item.id === nodeID)
  const draft = asCanvasObject(node?.payload?.draft)
  const roles = document.edges.filter((edge) => edge.target === nodeID && document.nodes.some((source) => source.id === edge.source && source.asset_id)).map((edge) => edge.input_role)
  if (draft.task_type === 'video_edit' || draft.task_type === 'video_extend') return draft.task_type
  if (roles.includes('reference_image') || roles.includes('reference_video') || roles.includes('reference_audio')) return 'reference_to_video'
  if (roles.includes('last_frame')) return 'first_last_frame_to_video'
  if (roles.includes('first_frame')) return 'image_to_video'
  return 'text_to_video'
}

/** Structural eligibility; callers additionally validate model capabilities and active runs. */
export function canvasGenerationEstimateEligible(document: CanvasDocument, nodeID: string): boolean {
  const node = document.nodes.find((item) => item.id === nodeID)
  if (!node || !isCanvasGenerationNode(node)) return false
  const draft = asCanvasObject(node.payload?.draft)
  const prompt = canvasInlinePromptTemplate(String(draft.prompt_template ?? ''))
  const parsed = parsePromptTemplate(prompt)
  if (!prompt.trim() || parsed.error || !String(draft.route_model_code ?? draft.abstract_model ?? '').trim()) return false
  const variables = asCanvasObject(draft.variables)
  if (parsed.variableNames.some((name) => !String(variables[name] ?? '').trim())) return false
  const inputs = document.edges.filter((edge) => edge.target === nodeID)
  if (inputs.some((edge) => inspectCanvasConnection({ ...document, edges: document.edges.filter((item) => item !== edge) }, edge))) return false
  const resources = canvasPromptResourceCandidates(document, nodeID)
  if (parsed.referenceNames.some((name) => resources.filter((resource) => resource.name === name).length !== 1)) return false
  if (node.type === 'video_v2') {
    const task = canvasVideoTaskType(document, nodeID)
    if ((task === 'image_to_video' || task === 'first_last_frame_to_video') && !inputs.some((edge) => edge.input_role === 'first_frame')) return false
    if (task === 'first_last_frame_to_video' && !inputs.some((edge) => edge.input_role === 'last_frame')) return false
    if ((task === 'video_edit' || task === 'video_extend') && !inputs.some((edge) => edge.input_role === 'reference_video')) return false
  }
  return true
}

function estimateDraft(node: CanvasNode) {
  const { quote_token: _quote, ...draft } = asCanvasObject(node.payload?.draft)
  return draft
}
