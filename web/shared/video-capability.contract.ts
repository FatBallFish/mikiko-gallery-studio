import type { VideoCapabilityListWire } from './api-types'
import { buildVideoEstimateWireRequest, normalizeVideoCapabilities } from './user-api'

const wire: VideoCapabilityListWire = {
  groups: [{
    route_model_code: 'cinema',
    name: '电影质感',
    description: '稳定的短视频生成',
    config_version: 'route-v3',
    capability_version: 'cap-v2',
    max_output_count: 4,
    task_types: ['text_to_video', 'image_to_video'],
    combinations: [
      { task_type: 'text_to_video', duration_seconds: 5, resolution: '720p', aspect_ratio: '16:9', audio_mode: 'silent' },
      { task_type: 'text_to_video', duration_seconds: 5, resolution: '720p', aspect_ratio: '16:9', audio_mode: 'generated' },
      { task_type: 'text_to_video', duration_seconds: 10, resolution: '1080p', aspect_ratio: '9:16', audio_mode: 'silent' },
      { task_type: 'text_to_video', duration_seconds: 5, resolution: '720p', aspect_ratio: '16:9', audio_mode: 'silent' },
      { task_type: 'image_to_video', duration_seconds: 5, resolution: '720p', aspect_ratio: 'adaptive', audio_mode: 'silent' },
    ],
  }],
}

const normalized = normalizeVideoCapabilities(wire)
const model = normalized.model_groups[0]
if (normalized.capability_version !== 'cap-v2' || model.code !== 'cinema' || model.description !== '稳定的短视频生成') {
  throw new Error(`wire metadata must survive normalization: ${JSON.stringify(normalized)}`)
}
const text = model.options_by_task_type.text_to_video
if (!text || text.durations.join(',') !== '5,10' || text.resolutions.join(',') !== '720p,1080p' || text.aspect_ratios.join(',') !== '16:9,9:16' || !text.audio_generation) {
  throw new Error(`combinations must be grouped and deduplicated: ${JSON.stringify(text)}`)
}
if (text.combinations.length !== 4 || text.combinations[1].audio_mode !== 'generated') throw new Error(`normalized options must retain complete legal combinations: ${JSON.stringify(text.combinations)}`)
if (model.defaults.task_type !== 'text_to_video' || model.defaults.duration_seconds !== 5 || model.defaults.generate_audio) {
  throw new Error(`defaults must use the first valid combination: ${JSON.stringify(model.defaults)}`)
}

const request = buildVideoEstimateWireRequest({
  project_id: 'project-1', route_model_code: 'cinema', task_type: 'text_to_video', prompt_template: 'ocean',
  prompt_variables: [], reference_bindings: [], inputs: [], duration_seconds: 5, resolution: '720p', aspect_ratio: '16:9',
  audio_mode: 'generated', output_count: 2,
})
if (request.audio_mode !== 'generated' || 'generate_audio' in request || 'capability_version' in request) {
  throw new Error(`estimate wire request must use the backend contract: ${JSON.stringify(request)}`)
}
