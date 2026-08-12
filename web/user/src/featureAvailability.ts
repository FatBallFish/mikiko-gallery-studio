import type { FeatureFlags } from '../../shared/api-types'

export const DEFAULT_FEATURE_FLAGS: FeatureFlags = {
  video_creation: false,
  creative_canvas: false,
  media_upload: false,
}

export function featureAvailability(flags: FeatureFlags) {
  return {
    showVideoCreation: flags.video_creation,
    showCanvasEntry: flags.creative_canvas,
    showMediaUpload: flags.media_upload,
    canOpenVideoHistory: ({ taskId, media }: { taskId?: string; media?: 'image' | 'video' }) => flags.video_creation || Boolean(taskId && media === 'video'),
    canOpenCanvasHistory: (canvasId?: string) => flags.creative_canvas || Boolean(canvasId),
  }
}
