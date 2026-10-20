import { DEFAULT_FEATURE_FLAGS, featureAvailability } from './featureAvailability'

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message)
}

assert(!DEFAULT_FEATURE_FLAGS.video_creation && !DEFAULT_FEATURE_FLAGS.creative_canvas && !DEFAULT_FEATURE_FLAGS.media_upload, 'new multimedia features must fail closed')
assert(!DEFAULT_FEATURE_FLAGS.oidc_login.enabled && DEFAULT_FEATURE_FLAGS.password_login_enabled && DEFAULT_FEATURE_FLAGS.registration_enabled, 'auth defaults must keep local login open and oidc off')

const closed = featureAvailability(DEFAULT_FEATURE_FLAGS)
assert(!closed.showVideoCreation && !closed.showCanvasEntry && !closed.showMediaUpload, 'disabled feature entry points must be hidden')
assert(!closed.showOIDCLogin && closed.showLocalLoginForm && closed.showRegistrationEntry, 'auth defaults must render the legacy login form')
assert(closed.canOpenVideoHistory({ taskId: 'historical-video', media: 'video' }), 'historical video detail must remain accessible')
assert(closed.canOpenCanvasHistory('historical-canvas'), 'historical canvas detail must remain accessible')
assert(!closed.canOpenCanvasHistory(undefined), 'disabled canvas list and create entry must remain hidden')

const enabled = featureAvailability({ video_creation: true, creative_canvas: true, media_upload: true, oidc_login: { enabled: true, display_name: 'Mikiko ID' }, password_login_enabled: true, registration_enabled: true })
assert(enabled.showVideoCreation && enabled.showCanvasEntry && enabled.showMediaUpload, 'enabled feature entry points must be visible')
assert(enabled.canOpenCanvasHistory(undefined), 'enabled canvas list must remain accessible')
assert(enabled.showOIDCLogin && enabled.oidcLoginLabel === 'Mikiko ID', 'enabled oidc login must expose the display label')

const pureOIDC = featureAvailability({ video_creation: false, creative_canvas: false, media_upload: false, oidc_login: { enabled: true }, password_login_enabled: false, registration_enabled: false })
assert(pureOIDC.showOIDCLogin && !pureOIDC.showLocalLoginForm && !pureOIDC.showRegistrationEntry, 'pure oidc mode must hide local login and registration')
assert(pureOIDC.oidcLoginLabel === '使用 Mikiko ID 登录', 'missing display name must fall back to the default label')
