import {
  addCanvasNode,
  attachCanvasResults,
  connectCanvasNodes,
  canvasGenerationEstimateSignature,
  canvasImageDraftForTask,
  canvasImageParameterErrors,
  canvasImageTaskType,
  canvasImageSizeDraftPatch,
  prepareCanvasEstimate,
  resolveCanvasEstimate,
  canvasPromptResourceCandidates,
  compatibleCanvasTargets,
  copyCanvasSelection,
  createCanvasState,
  inspectCanvasConnection,
  pasteCanvasSelection,
  removeCanvasEdges,
  removeCanvasNodes,
  moveCanvasNodes,
  redoCanvasCommand,
  resizeCanvasNode,
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

const resizeStart = createCanvasState(document, 4)
const resized = resizeCanvasNode(resizeStart, 'prompt', { width: 180, height: 100 })
const resizedPrompt = resized.present.nodes.find((node) => node.id === 'prompt')
if (resizedPrompt?.size.width !== 220 || resizedPrompt.size.height !== 140) throw new Error('prompt resize must clamp to its node-type minimum')
if (resized.past.length !== 1) throw new Error('a completed resize gesture must create exactly one undo entry')
if (undoCanvasCommand(resized).present.nodes.find((node) => node.id === 'prompt')?.size.width !== 240) throw new Error('node resize must be undoable')
if (redoCanvasCommand(undoCanvasCommand(resized)).present.nodes.find((node) => node.id === 'prompt')?.size.width !== 220) throw new Error('node resize must be redoable')
const resizedGeneration = resizeCanvasNode(resizeStart, 'image-gen', { width: 200, height: 120 }).present.nodes.find((node) => node.id === 'image-gen')
if (resizedGeneration?.size.width !== 280 || resizedGeneration.size.height !== 200) throw new Error('generation resize must preserve its larger minimum')

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

const edgeRemoved = removeCanvasEdges(state, ['prompt-image'])
if (edgeRemoved.present.edges.some((edge) => edge.id === 'prompt-image')) throw new Error('edge delete must only remove the selected connection')
if (!edgeRemoved.present.nodes.some((node) => node.id === 'prompt') || !edgeRemoved.present.nodes.some((node) => node.id === 'image-gen')) throw new Error('edge delete must preserve both endpoint nodes')
if (!edgeRemoved.present.edges.some((edge) => edge.id === 'image-ref')) throw new Error('edge delete must preserve unrelated connections')
if (!undoCanvasCommand(edgeRemoved).present.edges.some((edge) => edge.id === 'prompt-image')) throw new Error('edge delete must be undoable')

const promptTargets = compatibleCanvasTargets(state.present, 'prompt')
if (promptTargets.length !== 2 || !promptTargets.some((target) => target.type === 'image_generation' && target.role === 'prompt') || !promptTargets.some((target) => target.type === 'video_generation' && target.role === 'prompt')) {
  throw new Error(`prompt output must offer only compatible generation nodes: ${JSON.stringify(promptTargets)}`)
}
const imageTargets = compatibleCanvasTargets(state.present, 'image')
if (!imageTargets.some((target) => target.type === 'image_generation' && target.role === 'reference') || !imageTargets.some((target) => target.type === 'video_generation' && target.role === 'first_frame')) {
  throw new Error(`image output must expose reference and first-frame targets: ${JSON.stringify(imageTargets)}`)
}
if (compatibleCanvasTargets(state.present, 'video').length || compatibleCanvasTargets(state.present, 'audio').length || compatibleCanvasTargets(state.present, 'note').length) {
  throw new Error('media and note nodes without P0 outputs must not offer connection-created targets')
}
const generationTargets = compatibleCanvasTargets(state.present, 'image-gen')
if (!generationTargets.some((target) => target.type === 'image' && target.role === 'result')) throw new Error('image generation output must offer an empty image frame target')
if (inspectCanvasConnection(state.present, { id: 'occupied-output', source: 'image-gen', target: 'image', input_role: 'result' }) !== 'output_slot_occupied') {
  throw new Error('a bound image asset must not become a new generation output slot')
}
if (inspectCanvasConnection(state.present, { id: 'candidate', source: 'audio', target: 'video-gen', input_role: 'reference' }) !== 'illegal_connection') {
  throw new Error('connection inspection must report an illegal target before mutation')
}
if (inspectCanvasConnection(state.present, { id: 'candidate', source: 'prompt', target: 'image-gen', input_role: 'prompt' }) !== null) {
  throw new Error('connection inspection must accept legal prompt inputs')
}
let illegal = false
try {
  connectCanvasNodes(state, { id: 'bad-audio', source: 'audio', target: 'video-gen', input_role: 'reference' })
} catch (error) {
  illegal = error instanceof Error && error.message.includes('illegal_connection')
}
if (!illegal) throw new Error('illegal connections must be rejected with an actionable code')

const cycleSource = addCanvasNode(state, { id: 'result-image', type: 'image', position: { x: 800, y: 600 }, size: { width: 240, height: 180 } })
const withResult = connectCanvasNodes(cycleSource, { id: 'result-link', source: 'image-gen', target: 'result-image', input_role: 'result' })
let cycle = false
try {
  connectCanvasNodes(withResult, { id: 'cycle-link', source: 'result-image', target: 'image-gen', input_role: 'reference' })
} catch (error) {
  cycle = error instanceof Error && error.message.includes('cycle')
}
if (!cycle) throw new Error('directed generation cycles must be rejected')

const resourceDocument: CanvasDocument = {
  schema_version: 1, viewport: { x: 0, y: 0, zoom: 1 },
  nodes: [
    { id: 'resource-prompt', type: 'prompt', position: { x: 0, y: 0 }, size: { width: 220, height: 140 } },
    { id: 'resource-gen', type: 'image_generation', position: { x: 300, y: 0 }, size: { width: 280, height: 200 } },
    { id: 'resource-a', type: 'image', asset_id: 'asset-a', position: { x: 0, y: 240 }, size: { width: 220, height: 160 }, payload: { name: '主体' } },
    { id: 'resource-b', type: 'image', asset_id: 'asset-b', position: { x: 260, y: 240 }, size: { width: 220, height: 160 }, payload: { name: '主体' } },
  ],
  edges: [
    { id: 'resource-prompt-edge', source: 'resource-prompt', target: 'resource-gen', input_role: 'prompt' },
    { id: 'resource-a-edge', source: 'resource-a', target: 'resource-gen', input_role: 'reference', ordinal: 1 },
    { id: 'resource-b-edge', source: 'resource-b', target: 'resource-gen', input_role: 'reference', ordinal: 2 },
  ],
}
const resources = canvasPromptResourceCandidates(resourceDocument, 'resource-prompt')
if (resources.length !== 2 || resources.some((resource) => resource.name !== '主体' || !resource.duplicateName)) {
  throw new Error(`prompt resources must expose connected image candidates and duplicate names: ${JSON.stringify(resources)}`)
}

const estimateDocument: CanvasDocument = {
  ...resourceDocument,
  nodes: resourceDocument.nodes.map((node) => node.id === 'resource-gen' ? { ...node, payload: { draft: { route_model_code: 'plus', output_image_count: 2 } } } : node),
}
const estimateSignature = canvasGenerationEstimateSignature(estimateDocument, 'resource-gen')
if (canvasImageTaskType(estimateDocument, 'resource-gen') !== 'image_edit') throw new Error('connected references must select image_edit')
if (canvasImageTaskType({ ...estimateDocument, edges: estimateDocument.edges.filter((edge) => edge.input_role !== 'reference') }, 'resource-gen') !== 'text_to_image') {
  throw new Error('an image node without references must select text_to_image')
}
const emptyReferenceDocument: CanvasDocument = {
  ...estimateDocument,
  nodes: estimateDocument.nodes.map((node) => node.id === 'resource-a' ? { ...node, asset_id: undefined } : node),
  edges: estimateDocument.edges.filter((edge) => edge.source !== 'resource-b'),
}
if (canvasImageTaskType(emptyReferenceDocument, 'resource-gen') !== 'text_to_image') {
  throw new Error('an empty connected image frame must not select image_edit')
}
const movedEstimateSignature = canvasGenerationEstimateSignature({ ...estimateDocument, viewport: { x: 200, y: 80, zoom: 0.5 }, nodes: estimateDocument.nodes.map((node) => ({ ...node, position: { x: node.position.x + 100, y: node.position.y + 40 } })) }, 'resource-gen')
if (estimateSignature !== movedEstimateSignature) throw new Error('canvas estimate signatures must ignore layout-only changes')
const changedPromptSignature = canvasGenerationEstimateSignature({ ...estimateDocument, nodes: estimateDocument.nodes.map((node) => node.id === 'resource-prompt' ? { ...node, payload: { text: 'changed prompt' } } : node) }, 'resource-gen')
if (estimateSignature === changedPromptSignature) throw new Error('canvas estimate signatures must change with generation inputs')
const waitingEstimate = prepareCanvasEstimate(undefined, estimateSignature, true, 4)
if (waitingEstimate?.status !== 'waiting') throw new Error('eligible image nodes must enter waiting estimate state')
const readyEstimate = resolveCanvasEstimate(waitingEstimate, estimateSignature, 4, { points: '8.00000' })
const staleEstimate = resolveCanvasEstimate(readyEstimate, estimateSignature, 3, { points: '1.00000' })
if (readyEstimate?.status !== 'ready' || readyEstimate.points !== '8.00000' || staleEstimate !== readyEstimate) throw new Error('out-of-order estimate responses must be ignored')
if (prepareCanvasEstimate(readyEstimate, `${estimateSignature}-changed`, false, 5) !== undefined) throw new Error('incomplete generation inputs must suppress and clear estimates')

const sizeOptions = { base_resolution: ['1k'], aspect_ratios: ['16:9'], pixel_sizes: ['1024x1024'] }
const autoSize = canvasImageSizeDraftPatch('auto', sizeOptions)
if (autoSize.base_resolution || autoSize.aspect_ratio || autoSize.requested_size) throw new Error(`automatic size mode must clear every explicit size field: ${JSON.stringify(autoSize)}`)
const ratioSize = canvasImageSizeDraftPatch('ratio', sizeOptions)
if (ratioSize.base_resolution !== '1k' || ratioSize.aspect_ratio !== '16:9' || ratioSize.requested_size) throw new Error(`ratio size mode must only keep ratio fields: ${JSON.stringify(ratioSize)}`)
const pixelSize = canvasImageSizeDraftPatch('pixel', sizeOptions)
if (pixelSize.requested_size !== '1024x1024' || pixelSize.base_resolution || pixelSize.aspect_ratio) throw new Error(`pixel size mode must only keep requested_size: ${JSON.stringify(pixelSize)}`)

const taskCapability = {
  model_groups: [
    {
      id: 'text-only', code: 'text-only', name: 'Text only', task_types: ['text_to_image'] as const,
      size_modes: ['auto'], aspect_ratios: [], quality: ['auto'], output_format: ['jpeg'],
      max_output_image_count: 2, max_reference_image_count: 0, prices: [], supports_reference: false,
    },
    {
      id: 'both', code: 'both', name: 'Both', task_types: ['text_to_image', 'image_edit'] as const,
      size_modes: ['auto'], aspect_ratios: [], quality: ['auto'], output_format: ['jpeg'],
      max_output_image_count: 4, max_reference_image_count: 2, prices: [], supports_reference: true,
      capabilities_by_task_type: {
        text_to_image: { size_modes: ['auto'], quality: ['auto'], output_format: ['jpeg'], max_output_image_count: 4, max_reference_image_count: 0 },
        image_edit: { size_modes: ['pixel'], pixel_sizes: ['1536x1024'], quality: ['high'], output_format: ['png'], max_output_image_count: 2, max_reference_image_count: 2 },
      },
    },
  ],
} as const
const switchedTaskDraft = canvasImageDraftForTask({
  route_model_code: 'both', task_type: 'text_to_image', size_mode: 'auto', quality: 'auto', output_format: 'jpeg', output_image_count: 3,
}, taskCapability, 'image_edit')
if (switchedTaskDraft.route_model_code !== 'both' || switchedTaskDraft.task_type !== 'image_edit' || switchedTaskDraft.size_mode !== 'pixel'
  || switchedTaskDraft.requested_size !== '1536x1024' || switchedTaskDraft.quality !== 'high' || switchedTaskDraft.output_format !== 'png'
  || switchedTaskDraft.output_image_count !== 3) {
  throw new Error(`task changes must normalize scoped image parameters: ${JSON.stringify(switchedTaskDraft)}`)
}
const switchedModelDraft = canvasImageDraftForTask({ route_model_code: 'text-only', output_image_count: 1 }, taskCapability, 'image_edit')
if (switchedModelDraft.route_model_code !== 'both' || switchedModelDraft.task_type !== 'image_edit') {
  throw new Error(`task changes must select a model that supports the derived task: ${JSON.stringify(switchedModelDraft)}`)
}
const unsupportedEditErrors = canvasImageParameterErrors({ route_model_code: 'text-only', size_mode: 'auto', quality: 'auto', output_format: 'jpeg', output_image_count: 1 }, { model_groups: [taskCapability.model_groups[0]] }, 'image_edit')
if (!unsupportedEditErrors.includes('当前没有支持图片编辑的模型分组')) {
  throw new Error(`unsupported derived tasks must expose an actionable error: ${JSON.stringify(unsupportedEditErrors)}`)
}
if (canvasImageParameterErrors(switchedTaskDraft, taskCapability, 'image_edit').length) {
  throw new Error(`normalized task parameters must pass capability validation: ${JSON.stringify(canvasImageParameterErrors(switchedTaskDraft, taskCapability, 'image_edit'))}`)
}

const attached = attachCanvasResults(state, 'run-1', 'image-gen', [{ asset_id: 'asset-new', media_type: 'image' }])
const attachedAgain = attachCanvasResults(attached, 'run-1', 'image-gen', [{ asset_id: 'asset-new', media_type: 'image' }])
if (attachedAgain.present.nodes.filter((node) => node.asset_id === 'asset-new').length !== 1) throw new Error('result attachment must be stable and idempotent')

if (state.revision !== 4 || state.dirty !== true) throw new Error('local commands must preserve remote revision and mark the draft dirty')

const updated = updateCanvasNode(state, 'video-gen', (node) => ({ ...node, payload: { draft: { quote_token: 'signed-quote' } } }))
if ((updated.present.nodes.find((node) => node.id === 'video-gen')?.payload?.draft as { quote_token?: string })?.quote_token !== 'signed-quote') throw new Error('node payload updates must support quote-before-generate')
if (undoCanvasCommand(updated).present.nodes.find((node) => node.id === 'video-gen')?.payload?.draft) throw new Error('node payload updates must be undoable')
