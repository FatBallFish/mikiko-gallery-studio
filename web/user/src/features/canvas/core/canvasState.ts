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

export function resizeCanvasNode(state: CanvasCommandState, nodeID: string, size: { width: number; height: number }) {
  const current = state.present.nodes.find((node) => node.id === nodeID)
  if (!current) return state
  const minimum = canvasNodeMinimumSize(current.type)
  const nextSize = {
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
  if (nodeType === 'image' || nodeType === 'video') return { width: 220, height: 160 }
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
  const model = supportedModels.find((item) => item.code === currentCode) ?? supportedModels[0]
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

export type CanvasConnectionError = 'node_not_found' | 'illegal_connection' | 'input_role_conflict' | 'output_slot_occupied' | 'cycle'

export function inspectCanvasConnection(document: CanvasDocument, edge: CanvasEdge): CanvasConnectionError | null {
  const source = document.nodes.find((node) => node.id === edge.source)
  const target = document.nodes.find((node) => node.id === edge.target)
  if (!source || !target) return 'node_not_found'
  if (!isLegalConnection(source.type, target.type, edge.input_role)) return 'illegal_connection'
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
  if (source.type === 'prompt') return [
    { type: 'image_generation', role: 'prompt' },
    { type: 'video_generation', role: 'prompt' },
  ]
  if (source.type === 'image') return [
    { type: 'image_generation', role: 'reference' },
    { type: 'video_generation', role: 'first_frame' },
  ]
  if (source.type === 'image_generation') return [{ type: 'image', role: 'result' }]
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
  if (!node || (node.type !== 'image_generation' && node.type !== 'video_generation')) return ''
  const nodeByID = new Map(document.nodes.map((item) => [item.id, item]))
  const inputs = document.edges
    .filter((edge) => edge.target === nodeID && edge.input_role !== 'result')
    .map((edge) => ({ edge, source: nodeByID.get(edge.source) }))
    .sort((a, b) => a.edge.input_role.localeCompare(b.edge.input_role) || (a.edge.ordinal ?? 0) - (b.edge.ordinal ?? 0) || a.edge.id.localeCompare(b.edge.id))
    .map(({ edge, source }) => ({
      edge: { id: edge.id, source: edge.source, role: edge.input_role, ordinal: edge.ordinal ?? 0 },
      source: source ? { id: source.id, type: source.type, assetID: source.asset_id ?? '', payload: source.payload ?? {} } : null,
    }))
  return JSON.stringify({ node: { id: node.id, type: node.type, payload: node.payload ?? {} }, inputs })
}

export function canvasImageTaskType(document: CanvasDocument, nodeID: string): 'text_to_image' | 'image_edit' {
  const nodeByID = new Map(document.nodes.map((node) => [node.id, node]))
  return document.edges.some((edge) => edge.target === nodeID && edge.input_role === 'reference' && Boolean(nodeByID.get(edge.source)?.asset_id)) ? 'image_edit' : 'text_to_image'
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
  const generationIDs = new Set(document.edges.filter((edge) => edge.source === promptNodeID && edge.input_role === 'prompt')
    .map((edge) => document.nodes.find((node) => node.id === edge.target))
    .filter((node): node is CanvasNode => node?.type === 'image_generation')
    .map((node) => node.id))
  const nodeByID = new Map(document.nodes.map((node) => [node.id, node]))
  const seen = new Set<string>()
  const candidates = document.edges.filter((edge) => generationIDs.has(edge.target) && edge.input_role === 'reference')
    .map((edge, edgeIndex) => ({ edge, edgeIndex, node: nodeByID.get(edge.source) }))
    .filter((item): item is { edge: CanvasEdge; edgeIndex: number; node: CanvasNode } => item.node?.type === 'image' && Boolean(item.node.asset_id))
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
