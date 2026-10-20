import type { FeatureFlags } from '../../shared/api-types'

export const DEFAULT_FEATURE_FLAGS: FeatureFlags = {
  video_creation: false,
  creative_canvas: false,
  media_upload: false,
  oidc_login: { enabled: false },
  password_login_enabled: true,
  registration_enabled: true,
}

export function featureAvailability(flags: FeatureFlags) {
  return {
    showVideoCreation: flags.video_creation,
    showCanvasEntry: flags.creative_canvas,
    showMediaUpload: flags.media_upload,
    showOIDCLogin: flags.oidc_login?.enabled ?? false,
    oidcLoginLabel: flags.oidc_login?.display_name || '使用 Mikiko ID 登录',
    showLocalLoginForm: flags.password_login_enabled ?? true,
    showRegistrationEntry: (flags.registration_enabled ?? true) && (flags.password_login_enabled ?? true),
    canOpenVideoHistory: ({ taskId, media }: { taskId?: string; media?: 'image' | 'video' }) => flags.video_creation || Boolean(taskId && media === 'video'),
    canOpenCanvasHistory: (canvasId?: string) => flags.creative_canvas || Boolean(canvasId),
  }
}
