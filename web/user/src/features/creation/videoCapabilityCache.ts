import type { VideoCapability } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'

// Video capabilities change only when an operator edits model routing, so
// remounting the creation panel (e.g. switching the 图片/视频 tab) must not
// blank the console behind a spinner: serve the last snapshot synchronously
// and refresh in the background.
let cached: VideoCapability | null = null
let inflight: Promise<VideoCapability> | null = null

export function cachedVideoCapability(): VideoCapability | null {
  return cached
}

export function loadVideoCapability(): Promise<VideoCapability> {
  if (inflight) return inflight
  inflight = userApi.getVideoCapabilities().then((next) => {
    cached = next
    inflight = null
    return next
  }).catch((error) => {
    inflight = null
    throw error
  })
  return inflight
}
