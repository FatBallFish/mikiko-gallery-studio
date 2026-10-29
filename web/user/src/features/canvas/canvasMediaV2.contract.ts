import assert from 'node:assert/strict'
import type { Capability, VideoCapability } from '../../../../shared/api-types'
import { parsePromptTemplate } from '../../pages/promptTemplateParser'
import { readFileSync } from 'node:fs'
import { workspaceTaskImageSafetyLimit } from '../../pages/workspaceViewModel'
import { canvasMediaParameterSummary, canvasConsoleParameterErrors, canvasPromptTemplate, canvasPromptVariables, normalizeCanvasImageDraft, normalizeCanvasVideoDraft, normalizeCanvasVideoGraphDraft } from './canvasMediaV2'

assert.equal(canvasPromptTemplate('Draw ${ subject } with {{@stable_alias}} and \\${literal}'), 'Draw {{$subject}} with {{@stable_alias}} and \\${literal}')
assert.deepEqual(canvasPromptVariables({ variables: { subject: 'moon', bad: 7 }, prompt_variables: [{ name: 'ignored', value: 'x' }] }), { subject: 'moon' })
const image: Capability = { task_types: ['text_to_image'], aspect_ratios: ['1:1'], max_image_count: 10, model_groups: [{ id: 'i', code: 'i', name: 'Image', task_types: ['text_to_image'], prices: [], supports_reference: false, size_modes: ['ratio'], base_resolution: ['1K'], aspect_ratios: ['1:1'], quality: ['auto'], output_format: ['png'], max_output_image_count: 2 }] }
const normalized = normalizeCanvasImageDraft({ route_model_code: 'missing', quality: 'bad', output_image_count: 99, prompt_template: '{{@stable_alias}} ${subject}', variables: { subject: 'moon' } }, image)
assert.equal(normalized.route_model_code, 'missing')
assert.equal(normalized.quality, 'bad')
assert.equal(normalized.output_image_count, 99, 'platform fan-out count must not be clamped by provider max')
assert.deepEqual(normalized.variables, { subject: 'moon' })
assert.ok(canvasConsoleParameterErrors(normalized, image, null, false).length)
assert.equal(normalizeCanvasImageDraft({ ...normalized, output_image_count: workspaceTaskImageSafetyLimit + 1 }, image).output_image_count, workspaceTaskImageSafetyLimit)
assert.ok(canvasConsoleParameterErrors({ ...normalized, output_image_count: workspaceTaskImageSafetyLimit + 1 }, image, null, false).length)
assert.equal(canvasMediaParameterSummary({ base_resolution: '2K', aspect_ratio: '1:1', quality: 'auto', output_image_count: 99 }, false), '1:1 · 2K 清晰度 · 自动 · ×99')
const consoleSource = readFileSync(`${process.cwd()}/web/user/src/features/canvas/CanvasGenerationConsole.tsx`, 'utf8')
const uploadMenu = consoleSource.slice(consoleSource.indexOf('{menu ?'), consoleSource.indexOf('</header>'))
assert.ok(uploadMenu.includes('本地上传') && uploadMenu.includes('资产库上传'))
assert.ok(consoleSource.includes('画布参考') && consoleSource.includes('expanded ? <Shrink'))
assert.ok(!uploadMenu.slice(0, uploadMenu.indexOf('</div> : null}')).includes('onPickCanvasReference'))
const video: VideoCapability = { capability_version: 'v1', model_groups: [{ code: 'v', name: 'Video', minimum_points: '1', max_output_count: 1, task_types: ['image_to_video'], defaults: { task_type: 'image_to_video', duration_seconds: 5, resolution: '720p', aspect_ratio: '16:9', generate_audio: false }, options_by_task_type: { image_to_video: { inputs: { first_frame: { required: true, max_count: 1, media_types: ['image'], formats: ['png'] } }, durations: [5], resolutions: ['720p'], aspect_ratios: ['16:9'], audio_generation: false, combinations: [{ duration_seconds: 5, resolution: '720p', aspect_ratio: '16:9', audio_mode: 'silent' }] } } }] }
const videoDraft = normalizeCanvasVideoDraft({ route_model_code: 'v', task_type: 'image_to_video', variables: { scene: 'night' }, prompt_template: '${scene}', duration_seconds: 100, generate_audio: true, output_count: 10 }, video)
assert.equal(videoDraft.duration_seconds, 5)
assert.equal(videoDraft.generate_audio, false)
assert.equal(videoDraft.output_count, 1)
assert.equal(videoDraft.prompt_template, '{{$scene}}')
assert.deepEqual(parsePromptTemplate(String(videoDraft.prompt_template)).variableNames, ['scene'])
assert.equal(canvasPromptVariables(videoDraft)[parsePromptTemplate(String(videoDraft.prompt_template)).variableNames[0]], 'night')
assert.equal('inputs' in videoDraft, false, 'normalization must not duplicate graph inputs in persisted draft')
assert.equal('prompt_variables' in videoDraft, false)
assert.ok(canvasConsoleParameterErrors(videoDraft, null, video, true).some((message) => message.includes('参考资产')))
assert.deepEqual(canvasConsoleParameterErrors({ ...videoDraft, inputs: [{ asset_id: 'asset', role: 'first_frame', ordinal: 0 }] }, null, video, true), [])
assert.ok(canvasConsoleParameterErrors({ ...videoDraft, inputs: [{ asset_id: 'asset', role: 'reference_audio', ordinal: 0 }] }, null, video, true).length)
const graph: import('./core/types').CanvasDocument = { schema_version: 1, viewport: { x: 0, y: 0, zoom: 1 }, nodes: [
  { id: 'image', type: 'image_v2', asset_id: 'asset', position: { x: 0, y: 0 }, size: { width: 320, height: 240 } },
  { id: 'target', type: 'video_v2', position: { x: 400, y: 0 }, size: { width: 320, height: 240 }, payload: { draft: { ...videoDraft, route_model_code: 'text-only', task_type: 'text_to_video' } } },
], edges: [{ id: 'ref', source: 'image', target: 'target', input_role: 'first_frame' }] }
const inferred = normalizeCanvasVideoGraphDraft(graph, 'target', video)
assert.ok(inferred.error)
assert.equal(inferred.draft.task_type, 'text_to_video', 'graph references preserve the selected task')
assert.equal(inferred.draft.route_model_code, 'text-only', 'references never replace an explicit model')
assert.ok(canvasConsoleParameterErrors({ ...inferred.draft, inputs: [{ asset_id: 'asset', role: 'first_frame', ordinal: 0 }] }, null, video, true).length)
assert.deepEqual(normalizeCanvasVideoGraphDraft({ ...graph, nodes: graph.nodes.map((node) => node.id === 'target' ? { ...node, payload: { draft: inferred.draft } } : node) }, 'target', video).draft, inferred.draft, 'normalization is stable')
const unavailable = normalizeCanvasVideoGraphDraft(graph, 'target', { ...video, model_groups: [] })
assert.ok(unavailable.error)
assert.equal(unavailable.draft.task_type, 'text_to_video', 'unsupported task remains visible for manual correction')
for (const task of ['video_edit', 'video_extend'] as const) {
  const editGraph = { ...graph, nodes: graph.nodes.map((node) => node.id === 'target' ? { ...node, payload: { draft: { ...videoDraft, task_type: task } } } : { ...node, type: 'video_v2' as const }), edges: [{ ...graph.edges[0], input_role: 'reference_video' as const }] }
  const capability: VideoCapability = { ...video, model_groups: [{ ...video.model_groups[0], task_types: [task], options_by_task_type: { [task]: video.model_groups[0].options_by_task_type.image_to_video } }] }
  const edited = normalizeCanvasVideoGraphDraft(editGraph, 'target', capability)
  assert.equal(edited.error, null)
  assert.equal(edited.draft.task_type, task, 'compatible explicit video mode is retained')
  assert.ok(normalizeCanvasVideoGraphDraft({ ...editGraph, edges: graph.edges }, 'target', capability).error, 'incompatible edit inputs are reported without silently changing mode')
}
console.log('canvas media V2 adapters: PASS')

const { nativeImageProfile } = await import('../../../../shared/native-image')
const layerProfile = nativeImageProfile('doubao-seedream-5-0-pro-260628')!
const layerCapability: Capability = { ...image, model_groups: [{ ...image.model_groups[0], task_types: ['text_to_image', 'image_edit'], native_image_profile: layerProfile, native_image_layer_pricing_confirmed: true }] }
const layerDraft = { route_model_code: 'i', operation: 'layer_decomposition', task_type: 'image_edit', output_image_count: 1 }
assert.ok(canvasConsoleParameterErrors({ ...layerDraft, inputs: [] }, layerCapability, null, false).some((error) => error.includes('必填')))
assert.ok(canvasConsoleParameterErrors({ ...layerDraft, inputs: [{ asset_id: 'a' }, { asset_id: 'b' }] }, layerCapability, null, false).some((error) => error.includes('已保留')))
const missingPrice: Capability = { ...layerCapability, model_groups: [{ ...layerCapability.model_groups[0], native_image_layer_pricing_confirmed: false }] }
assert.ok(canvasConsoleParameterErrors({ ...layerDraft, inputs: [{ asset_id: 'a' }] }, missingPrice, null, false).some((error) => error.includes('价格')))
const unsupported: Capability = { ...layerCapability, model_groups: [{ ...layerCapability.model_groups[0], native_image_profile: nativeImageProfile('qwen-image-3.0')! }] }
assert.ok(canvasConsoleParameterErrors({ ...layerDraft, inputs: [{ asset_id: 'a' }] }, unsupported, null, false).some((error) => error.includes('能力')))
assert.equal(normalizeCanvasImageDraft(layerDraft, unsupported).operation, 'layer_decomposition', 'switching to unsupported model preserves mode for explicit validation')
assert.equal(normalizeCanvasImageDraft({ ...layerDraft, inputs: [] }, layerCapability).operation, 'layer_decomposition', 'zero references keep selected mode')
