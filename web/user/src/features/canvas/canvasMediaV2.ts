import type { Capability, VideoCapability, VideoTaskType } from '../../../../shared/api-types'
import { canvasImageDraftForTask, canvasImageParameterErrors } from './core/canvasState'
import type { CanvasDocument } from './core/types'
import { workspaceModelForTask } from '../../pages/workspaceParameters'
import { normalizeWorkspaceImageCount, workspaceTaskImageSafetyLimit } from '../../pages/workspaceViewModel'
import { applyVideoCapability, defaultVideoDraft, videoDraftInvalidReason, VIDEO_TASK_INPUT_ROLES, videoTaskInputMissing, type VideoDraft } from '../creation/videoDraft'

/** Canvas accepts ${name}; persisted/editor syntax remains {{$name}}. Escapes remain literal. */
export function canvasPromptTemplate(value: string) {
  return value.replace(/(?<!\\)\$\{([^{}]+)\}/g, (_, name: string) => `{{$${name.trim().normalize('NFC')}}}`)
}

export function canvasMediaParameterLabel(value: unknown): string {
  const text = String(value ?? '')
  const labels: Record<string, string> = { auto: '自动', low: '低画质', medium: '标准画质', high: '高画质', standard: '标准画质', hd: '高清', ratio: '比例', pixel: '像素' }
  return labels[text.toLowerCase()] ?? text
}

export function canvasMediaParameterSummary(draft: Record<string, unknown>, video: boolean) {
  return [
    canvasMediaParameterLabel(draft.aspect_ratio || draft.requested_size) || '自动尺寸',
    video ? canvasMediaParameterLabel(draft.resolution) : draft.base_resolution ? `${canvasMediaParameterLabel(draft.base_resolution)} 清晰度` : '',
    video ? `${draft.duration_seconds ?? '—'}s` : canvasMediaParameterLabel(draft.quality),
    `×${video ? draft.output_count ?? 1 : draft.output_image_count ?? 1}`,
  ].filter(Boolean).join(' · ')
}

export function canvasPromptVariables(draft: Record<string, unknown>): Record<string, string> {
  const source = draft.variables ?? draft.prompt_variables
  if (Array.isArray(source)) return Object.fromEntries(source.filter((item) => item && typeof item.name === 'string' && typeof item.value === 'string').map((item) => [item.name, item.value]))
  if (source && typeof source === 'object') return Object.fromEntries(Object.entries(source).filter((entry): entry is [string, string] => typeof entry[1] === 'string'))
  return {}
}

export function canvasImageModel(draft: Record<string, unknown>, capability: Capability) {
  const task = draft.task_type === 'image_edit' ? 'image_edit' : 'text_to_image'
  return workspaceModelForTask(capability.model_groups.find((model) => model.code === String(draft.route_model_code ?? draft.abstract_model ?? '')), task)
}

export function normalizeCanvasImageDraft(draft: Record<string, unknown>, capability: Capability): Record<string, unknown> & { task_type: 'text_to_image' | 'image_edit'; output_image_count: number } {
  const task = draft.task_type === 'image_edit' ? 'image_edit' : 'text_to_image'
  const next = canvasImageDraftForTask(draft, capability, task)
  return { ...next, output_image_count: normalizeWorkspaceImageCount(Number(draft.output_image_count ?? 1)) }
}

export function canvasVideoDraft(draft: Record<string, unknown>, capability: VideoCapability): VideoDraft {
  const base = defaultVideoDraft(capability)
  const task = String(draft.task_type ?? base.task_type) as VideoTaskType
  return {
    ...base, route_model_code: String(draft.route_model_code ?? base.route_model_code), task_type: task,
    prompt_template: canvasPromptTemplate(String(draft.prompt_template ?? '')),
    prompt_variables: Object.entries(canvasPromptVariables(draft)).map(([name, value]) => ({ name, value })),
    inputs: Array.isArray(draft.inputs) ? draft.inputs as VideoDraft['inputs'] : [],
    duration_seconds: Number(draft.duration_seconds ?? base.duration_seconds), resolution: String(draft.resolution ?? base.resolution),
    aspect_ratio: String(draft.aspect_ratio ?? base.aspect_ratio), generate_audio: draft.generate_audio === true || draft.audio_mode === 'generated', output_count: Number(draft.output_count ?? 1),
  }
}

export function normalizeCanvasVideoDraft(draft: Record<string, unknown>, capability: VideoCapability, preferredField?: keyof VideoDraft) {
  const current = canvasVideoDraft(draft, capability)
  const model = capability.model_groups.find((item) => item.code === current.route_model_code)
  if (current.route_model_code && (!model || !model.task_types.includes(current.task_type))) return draft
  const next = applyVideoCapability(current, capability, preferredField).draft
  const { inputs: _inputs, prompt_variables: _variables, ...parameters } = next
  return { ...draft, ...parameters, variables: canvasPromptVariables(draft), audio_mode: next.generate_audio ? 'generated' : 'silent' }
}

export function normalizeCanvasVideoGraphDraft(document: CanvasDocument, nodeID: string, capability: VideoCapability): { draft: Record<string, unknown>; error: string | null } {
  const node = document.nodes.find((item) => item.id === nodeID)
  const draft = node?.payload?.draft && typeof node.payload.draft === 'object' && !Array.isArray(node.payload.draft) ? node.payload.draft as Record<string, unknown> : {}
  const inputs = document.edges.filter((edge) => edge.target === nodeID && document.nodes.some((source) => source.id === edge.source && source.asset_id))
  if (node?.type !== 'video_v2' || node.asset_id || !inputs.length) return { draft, error: null }
  const task = String(draft.task_type ?? 'text_to_video') as VideoTaskType
  if (inputs.some((input) => !VIDEO_TASK_INPUT_ROLES[task]?.includes(input.input_role as VideoDraft['inputs'][number]['role']))) return { draft, error: '引用角色与生成方式不匹配，请调整连接或手动选择生成方式' }
  const model = capability.model_groups.find((item) => item.code === draft.route_model_code)
  if (!model || !model.task_types.includes(task) || !model.options_by_task_type[task]?.combinations.length) return { draft, error: '当前视频型号不支持所选生成方式，请手动选择型号或生成方式' }
  return { draft, error: null }
}

export function canvasConsoleParameterErrors(draft: Record<string, unknown>, image: Capability | null, video: VideoCapability | null, isVideo: boolean) {
  if (!isVideo) {
    if (!image) return ['正在加载图片模型能力']
    const task = draft.task_type === 'image_edit' ? 'image_edit' : 'text_to_image'
    return canvasImageParameterErrors(draft, image, task, workspaceTaskImageSafetyLimit)
  }
  if (!video) return ['正在加载视频模型能力']
  const next = canvasVideoDraft(draft, video)
  const model = video.model_groups.find((item) => item.code === next.route_model_code)
  const options = model?.options_by_task_type[next.task_type]
  if (!model || !model.task_types.includes(next.task_type) || !options) return ['请选择可用的视频模型与生成方式']
  const invalidReason = videoDraftInvalidReason(next, video)
  const errors: string[] = invalidReason ? [invalidReason] : []
  if (!options.combinations.some((item) => item.duration_seconds === next.duration_seconds && item.resolution === next.resolution && item.aspect_ratio === next.aspect_ratio && item.audio_mode === (next.generate_audio ? 'generated' : 'silent'))) errors.push('当前参数组合不可用')
  if (!Number.isInteger(next.output_count) || next.output_count < 1 || next.output_count > model.max_output_count) errors.push('生成数量超出模型范围')
  if (next.inputs.some((input) => !VIDEO_TASK_INPUT_ROLES[next.task_type]?.includes(input.role))) errors.push('引用角色与生成方式不匹配')
  if (videoTaskInputMissing(next.task_type, next.inputs)) errors.push('请添加当前生成方式需要的参考资产')
  return errors
}

/** Capability disappearance preserves marked originals; only a confirmed model change restores them. */
export function canvasReferenceMarksModelChanged(marksModel: unknown, profile: { model: string } | undefined) {
  return Boolean(profile && marksModel && marksModel !== profile.model)
}
