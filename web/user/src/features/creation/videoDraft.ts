import type { VideoCapability, VideoCapabilityModelGroup, VideoTask, VideoTaskType } from '../../../../shared/api-types'

export type CreationMediaMode = 'image' | 'video'
export type VideoDraft = {
  route_model_code: string
  task_type: VideoTaskType
  prompt_template: string
  prompt_variables: Array<{ name: string; value: string }>
  inputs: Array<{ asset_id: string; role: 'first_frame' | 'last_frame'; ordinal: number }>
  duration_seconds: number
  resolution: string
  aspect_ratio: string
  generate_audio: boolean
  output_count: number
}
export type VideoQuoteState = {
  key: string
  quote_token: string
  quote_expires_at: string
  estimated_points: string
  max_reserved_points: string
  unit_points: string
  available_points?: string
  pricing_mode?: string
  summary?: Record<string, unknown>
  display_points?: string
  sufficient?: boolean
}

export function readCreationEntry(search: string, remembered: CreationMediaMode | null): { mode: CreationMediaMode; assetId?: string; taskType?: VideoTaskType } {
  const params = new URLSearchParams(search.replace(/^\?/, ''))
  const assetId = params.get('asset_id')?.trim() || undefined
  const requested = params.get('media')
  const mode: CreationMediaMode = assetId || requested === 'video' ? 'video' : requested === 'image' ? 'image' : remembered === 'video' ? 'video' : 'image'
  return { mode, assetId, taskType: assetId ? 'image_to_video' : undefined }
}

export function defaultVideoDraft(capability: VideoCapability, preferredTaskType?: VideoTaskType): VideoDraft {
	const model = preferredTaskType
		? capability.model_groups.find((item) => item.task_types.includes(preferredTaskType)) ?? capability.model_groups[0]
		: capability.model_groups[0]
  const requested = preferredTaskType && model?.task_types.includes(preferredTaskType) ? preferredTaskType : model?.defaults.task_type ?? 'text_to_video'
  const options = model?.options_by_task_type[requested]
  return {
    route_model_code: model?.code ?? '', task_type: requested, prompt_template: '', prompt_variables: [], inputs: [],
    duration_seconds: options?.durations[0] ?? model?.defaults.duration_seconds ?? 5,
    resolution: options?.resolutions[0] ?? model?.defaults.resolution ?? '720p',
    aspect_ratio: options?.aspect_ratios[0] ?? model?.defaults.aspect_ratio ?? '16:9',
    generate_audio: Boolean(model?.defaults.generate_audio && options?.audio_generation), output_count: 1,
  }
}

export function applyVideoCapability(draft: VideoDraft, capability: VideoCapability, preferredField?: keyof VideoDraft): { draft: VideoDraft; changes: string[] } {
  const changes: string[] = []
  const model = capability.model_groups.find((item) => item.code === draft.route_model_code) ?? capability.model_groups[0]
  if (!model) return { draft, changes }
  let next = draft
  if (model.code !== draft.route_model_code) {
    next = { ...next, route_model_code: model.code }
    changes.push('模型分组已调整')
  }
  const taskType = model.task_types.includes(next.task_type) ? next.task_type : model.defaults.task_type
  if (taskType !== next.task_type) {
    next = { ...next, task_type: taskType }
    changes.push('生成方式已调整')
  }
  const options = model.options_by_task_type[taskType]
  if (!options) return { draft: next, changes }
  const requestedAudio = next.generate_audio ? 'generated' : 'silent'
  const exact = options.combinations.find((item) => item.duration_seconds === next.duration_seconds && item.resolution === next.resolution && item.aspect_ratio === next.aspect_ratio && item.audio_mode === requestedAudio)
  if (!exact) {
    const preferred = options.combinations.filter((item) => {
      if (preferredField === 'duration_seconds') return item.duration_seconds === next.duration_seconds
      if (preferredField === 'resolution') return item.resolution === next.resolution
      if (preferredField === 'aspect_ratio') return item.aspect_ratio === next.aspect_ratio
      if (preferredField === 'generate_audio') return item.audio_mode === requestedAudio
      return true
    })
    const match = bestCombination(preferred.length ? preferred : options.combinations, next)
    if (match) {
      if (match.duration_seconds !== next.duration_seconds) changes.push('时长已调整')
      if (match.resolution !== next.resolution) changes.push('清晰度已调整')
      if (match.aspect_ratio !== next.aspect_ratio) changes.push('比例已调整')
      if ((match.audio_mode === 'generated') !== next.generate_audio) changes.push('音频模式已调整')
      next = { ...next, duration_seconds: match.duration_seconds, resolution: match.resolution, aspect_ratio: match.aspect_ratio, generate_audio: match.audio_mode === 'generated' }
    }
  }
  return { draft: next, changes }
}

function bestCombination(combinations: NonNullable<VideoCapabilityModelGroup['options_by_task_type'][VideoTaskType]>['combinations'], draft: VideoDraft) {
  return combinations.map((item, index) => ({ item, index, score: Number(item.duration_seconds === draft.duration_seconds) + Number(item.resolution === draft.resolution) + Number(item.aspect_ratio === draft.aspect_ratio) + Number((item.audio_mode === 'generated') === draft.generate_audio) })).sort((left, right) => right.score - left.score || left.index - right.index)[0]?.item
}

export function videoDraftKey(draft: VideoDraft) {
  return JSON.stringify({
    route_model_code: draft.route_model_code, task_type: draft.task_type, prompt_template: draft.prompt_template,
    prompt_variables: draft.prompt_variables, inputs: draft.inputs, duration_seconds: draft.duration_seconds,
    resolution: draft.resolution, aspect_ratio: draft.aspect_ratio, generate_audio: draft.generate_audio, output_count: draft.output_count,
  })
}

export function invalidateVideoQuote(quote: VideoQuoteState | null, previous: VideoDraft, next: VideoDraft) {
  return videoDraftKey(previous) === videoDraftKey(next) ? quote : null
}

export function videoDraftAfterFailure(draft: VideoDraft, error: unknown) {
  return { draft, error: error instanceof Error ? error.message : String(error) }
}

export function reuseVideoTask(task: VideoTask): VideoDraft {
  return {
    route_model_code: task.route_model_code, task_type: task.task_type, prompt_template: task.prompt_template,
    prompt_variables: [], inputs: [], duration_seconds: task.duration_seconds, resolution: task.resolution,
    aspect_ratio: task.aspect_ratio, generate_audio: task.audio_mode ? task.audio_mode === 'generated' : Boolean(task.generate_audio), output_count: task.requested_output_count,
  }
}

export function videoModelForDraft(capability: VideoCapability, draft: VideoDraft): VideoCapabilityModelGroup | undefined {
  return capability.model_groups.find((item) => item.code === draft.route_model_code)
}
