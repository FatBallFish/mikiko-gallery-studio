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

for (const node of nodes.filter((item) => item.type !== 'note' && item.type !== 'audio')) {
  if (moveCanvasNodes(state, [node.id], { x: 20, y: 20 }) !== state) throw new Error('legacy move must be blocked')
  if (resizeCanvasNode(state, node.id, { width: 900, height: 900 }) !== state) throw new Error('legacy resize must be blocked')
  if (updateCanvasNode(state, node.id, () => { throw new Error('must not invoke updater') }) !== state) throw new Error('legacy update must be blocked')
  if (copyCanvasSelection(document, [node.id]).nodes.length) throw new Error('legacy copy must be blocked')
  if (addCanvasNode(state, { ...node, id: `new-${node.id}` }) !== state) throw new Error('legacy creation must be blocked')
  if (pasteCanvasSelection(state, { nodes: [node], edges: [] }, { x: 20, y: 20 }, (id) => `copy-${id}`) !== state) throw new Error('legacy external clipboard must be blocked')
  if (compatibleCanvasTargets(document, node.id).length) throw new Error('legacy connections must be blocked')
  const removed = removeCanvasNodes(state, [node.id])
  if (removed.present.nodes.some((item) => item.id === node.id)) throw new Error('legacy deletion must remain available')
  if (undoCanvasCommand(removed).present.nodes.length !== document.nodes.length) throw new Error('legacy deletion remains undoable')
}
state = moveCanvasNodes(state, ['note'], { x: 80, y: 40 })
if (redoCanvasCommand(undoCanvasCommand(state)).present.nodes.find((node) => node.id === 'note')?.position.x !== 80) throw new Error('editable commands preserve history')

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
if (switchedModelDraft.route_model_code !== 'text-only' || switchedModelDraft.task_type !== 'image_edit') {
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

const updated = updateCanvasNode(state, 'note', (node) => ({ ...node, payload: { text: 'updated' } }))
if (updated.present.nodes.find((node) => node.id === 'note')?.payload?.text !== 'updated') throw new Error('editable node payload update failed')
