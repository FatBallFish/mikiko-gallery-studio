import {
  createMediaProjectScope,
  createAudioPlaybackCoordinator,
  createHoverPreviewScheduler,
  mediaCreationActions,
  reconcileBatchSelection,
  withSingleAccessRefresh,
} from './mediaExperience'

const projectScope = createMediaProjectScope('project-a')
const firstRequest = projectScope.begin('project-a')
if (!projectScope.accepts(firstRequest, 'project-a')) throw new Error('current project request must be accepted')
projectScope.switchTo('project-b')
if (projectScope.accepts(firstRequest, 'project-a')) throw new Error('stale project response must be rejected after switching projects')
if (projectScope.contains({ project_id: 'project-a' })) throw new Error('old project assets must not remain actionable after switching projects')

const selected = new Set(['a', 'b', 'c'])
const reconciled = reconcileBatchSelection(selected, [
  { id: 'a', status: 'succeeded' },
  { id: 'b', status: 'failed' },
  { id: 'c', status: 'succeeded' },
])
if (reconciled.size !== 1 || !reconciled.has('b')) throw new Error('failed batch items must remain selected')

let refreshes = 0
const value = await withSingleAccessRefresh(
  async (url) => {
    if (url === 'expired') throw Object.assign(new Error('expired'), { status: 403 })
    return url
  },
  async () => {
    refreshes += 1
    return refreshes === 1 ? 'fresh' : 'unexpected'
  },
  'expired',
)
if (value !== 'fresh' || refreshes !== 1) throw new Error('expired access must refresh exactly once')

const scheduler = createHoverPreviewScheduler({ delayMs: 0, maxActive: 2 })
const started: string[] = []
const releases = [
  scheduler.schedule('a', () => started.push('a')),
  scheduler.schedule('b', () => started.push('b')),
  scheduler.schedule('c', () => started.push('c')),
]
await new Promise((resolve) => setTimeout(resolve, 5))
if (started.join(',') !== 'a,b') throw new Error(`hover previews must cap concurrency at two, got ${started}`)
releases[0]()
await new Promise((resolve) => setTimeout(resolve, 5))
if (started.join(',') !== 'a,b,c') throw new Error('queued hover preview must start after a slot is released')
releases.forEach((release) => release())

const paused: string[] = []
const audio = createAudioPlaybackCoordinator()
audio.activate('first', () => paused.push('first'))
audio.activate('second', () => paused.push('second'))
if (paused.join(',') !== 'first') throw new Error('starting audio must pause the previously active player')

const imageActions = mediaCreationActions({ id: 'image-1', media_type: 'image', source_task_kind: 'image', source_task_id: 'image-task-1' })
if (imageActions.length !== 2 || imageActions[0].label !== '继续生图' || imageActions[0].options.media !== 'image' || imageActions[0].options.taskId !== 'image-task-1') {
  throw new Error(`generated images must offer image continuation with their source task: ${JSON.stringify(imageActions)}`)
}
if (imageActions[1].label !== '生成视频' || imageActions[1].options.media !== 'video' || imageActions[1].options.assetId !== 'image-1') {
  throw new Error(`images must offer image-to-video with their asset id: ${JSON.stringify(imageActions)}`)
}
const videoActions = mediaCreationActions({ id: 'video-1', media_type: 'video', source_task_kind: 'video', source_task_id: 'video-task-1' })
if (videoActions.length !== 1 || videoActions[0].label !== '复用视频参数' || videoActions[0].options.taskId !== 'video-task-1' || videoActions[0].options.assetId) {
  throw new Error(`generated videos must reuse source task parameters without becoming frame input: ${JSON.stringify(videoActions)}`)
}
if (mediaCreationActions({ id: 'upload-1', media_type: 'video' }).length !== 0 || mediaCreationActions({ id: 'audio-1', media_type: 'audio' }).length !== 0) {
  throw new Error('uploaded videos without a source task and audio assets must not expose invalid generation actions')
}

console.log('media experience contract passed')
