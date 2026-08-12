import type { VideoCapability, VideoTask } from '../../../../shared/api-types'
import { parseUserHashState, userHashForRoute } from '../../routeState'
import {
  applyVideoCapability,
  defaultVideoDraft,
  invalidateVideoQuote,
  readCreationEntry,
  reuseVideoTask,
  videoDraftAfterFailure,
  type VideoDraft,
  type VideoQuoteState,
} from './videoDraft'

const capability: VideoCapability = {
  capability_version: 'cap-1',
  model_groups: [{
    code: 'cinema', name: '电影质感', minimum_points: '20.00000', max_output_count: 4,
    task_types: ['text_to_video', 'image_to_video'],
    defaults: { task_type: 'text_to_video', duration_seconds: 5, resolution: '720p', aspect_ratio: '16:9', generate_audio: false },
    options_by_task_type: {
      text_to_video: { durations: [5, 10], resolutions: ['720p'], aspect_ratios: ['16:9', '9:16'], audio_generation: true, combinations: [
        { duration_seconds: 5, resolution: '720p', aspect_ratio: '16:9', audio_mode: 'silent' },
        { duration_seconds: 10, resolution: '720p', aspect_ratio: '9:16', audio_mode: 'generated' },
      ] },
      image_to_video: { durations: [5], resolutions: ['720p'], aspect_ratios: ['adaptive'], audio_generation: false, combinations: [{ duration_seconds: 5, resolution: '720p', aspect_ratio: 'adaptive', audio_mode: 'silent' }] },
    },
  }],
}

const draft: VideoDraft = {
  route_model_code: 'cinema', task_type: 'text_to_video', prompt_template: 'camera move', prompt_variables: [{ name: 'speed', value: 'slow' }],
  inputs: [], duration_seconds: 10, resolution: '720p', aspect_ratio: '9:16', generate_audio: true, output_count: 2,
}

const entry = readCreationEntry('?media=video&asset_id=asset-1', 'image')
if (entry.mode !== 'video' || entry.assetId !== 'asset-1' || entry.taskType !== 'image_to_video') throw new Error(`asset deep link must enter image-to-video: ${JSON.stringify(entry)}`)
if (readCreationEntry('', 'video').mode !== 'video' || readCreationEntry('', null).mode !== 'image') throw new Error('creation mode memory must preserve video while defaulting new users to image')
const videoAsset = parseUserHashState('#/genpic?media=video&asset_id=asset_video_1')
if (videoAsset.route !== 'genpic' || videoAsset.media !== 'video' || videoAsset.assetId !== 'asset_video_1') throw new Error(`video asset deep link must survive route parsing: ${JSON.stringify(videoAsset)}`)
if (userHashForRoute('genpic', { media: 'video', assetId: ' asset_video_2 ' }) !== '/genpic?media=video&asset_id=asset_video_2') throw new Error('video asset hash must trim and preserve asset_id')
const imageToVideoDefault = defaultVideoDraft({ ...capability, model_groups: [{ ...capability.model_groups[0], task_types: ['text_to_video'], options_by_task_type: { text_to_video: capability.model_groups[0].options_by_task_type.text_to_video } }, capability.model_groups[0]] }, 'image_to_video')
if (imageToVideoDefault.route_model_code !== 'cinema' || imageToVideoDefault.task_type !== 'image_to_video') throw new Error(`asset entry must select a model that supports image-to-video: ${JSON.stringify(imageToVideoDefault)}`)

const narrowed = applyVideoCapability(draft, {
  ...capability,
  capability_version: 'cap-2',
    model_groups: [{ ...capability.model_groups[0], options_by_task_type: { text_to_video: { durations: [5], resolutions: ['720p'], aspect_ratios: ['16:9'], audio_generation: false, combinations: [{ duration_seconds: 5, resolution: '720p', aspect_ratio: '16:9', audio_mode: 'silent' }] } } }],
})
if (narrowed.draft.duration_seconds !== 5 || narrowed.draft.aspect_ratio !== '16:9' || narrowed.draft.generate_audio || narrowed.draft.prompt_template !== draft.prompt_template || narrowed.changes.length !== 3) {
  throw new Error(`capability reducer must only reset invalid fields and list changes: ${JSON.stringify(narrowed)}`)
}

const combinationReset = applyVideoCapability({ ...draft, duration_seconds: 10, resolution: '720p', aspect_ratio: '16:9', generate_audio: true }, capability)
if (combinationReset.draft.aspect_ratio !== '9:16' || combinationReset.draft.resolution !== '720p' || !combinationReset.draft.generate_audio) {
  throw new Error(`complete candidate matching must prevent cross-product combinations: ${JSON.stringify(combinationReset)}`)
}

const quote: VideoQuoteState = { key: 'old', quote_token: 'quote', quote_expires_at: '2099-01-01T00:00:00Z', unit_points: '20.00000', estimated_points: '20.00000', max_reserved_points: '20.00000' }
if (invalidateVideoQuote(quote, draft, { ...draft, prompt_template: 'new' }) !== null) throw new Error('prompt changes must invalidate quote')
if (invalidateVideoQuote(quote, draft, { ...draft }) !== quote) throw new Error('unchanged draft must retain quote identity')

const failed = videoDraftAfterFailure(draft, new Error('network'))
if (failed.draft !== draft || failed.error !== 'network') throw new Error('submission failure must preserve draft identity')

const task = {
  id: 'task-1', project_id: 'project-1', route_model_code: 'cinema', task_type: 'image_to_video', prompt_template: 'move {{speed}}',
  status: 'succeeded', items: [],
  duration_seconds: 5, resolution: '720p', aspect_ratio: 'adaptive', generate_audio: false, requested_output_count: 2,
  prompt_binding_snapshot: { variables: [{ name: 'speed', value: 'private' }] }, inputs: [{ id: 'input-1', asset_id: 'asset-1', role: 'first_frame', ordinal: 0 }],
} as VideoTask
const reused = reuseVideoTask(task)
if (reused.prompt_variables.length !== 0 || reused.inputs.length !== 0 || reused.prompt_template !== task.prompt_template || reused.output_count !== 2) {
  throw new Error(`reuse must keep parameters without variable values or first frame: ${JSON.stringify(reused)}`)
}
