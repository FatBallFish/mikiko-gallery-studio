import { DEFAULT_FEATURE_FLAGS, featureAvailability } from './featureAvailability'

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message)
}

assert(!DEFAULT_FEATURE_FLAGS.video_creation && !DEFAULT_FEATURE_FLAGS.creative_canvas && !DEFAULT_FEATURE_FLAGS.media_upload, 'new multimedia features must fail closed')

const closed = featureAvailability(DEFAULT_FEATURE_FLAGS)
assert(!closed.showVideoCreation && !closed.showCanvasEntry && !closed.showMediaUpload, 'disabled feature entry points must be hidden')
assert(closed.canOpenVideoHistory({ taskId: 'historical-video', media: 'video' }), 'historical video detail must remain accessible')
assert(closed.canOpenCanvasHistory('historical-canvas'), 'historical canvas detail must remain accessible')
assert(!closed.canOpenCanvasHistory(undefined), 'disabled canvas list and create entry must remain hidden')

const enabled = featureAvailability({ video_creation: true, creative_canvas: true, media_upload: true })
assert(enabled.showVideoCreation && enabled.showCanvasEntry && enabled.showMediaUpload, 'enabled feature entry points must be visible')
assert(enabled.canOpenCanvasHistory(undefined), 'enabled canvas list must remain accessible')
