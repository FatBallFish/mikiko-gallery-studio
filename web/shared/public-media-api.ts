import type { ImageResult, PageResult } from './api-types'
import { normalizePage, sharedApiClient } from './http-client'
import { toImageResult } from './user-api'

export type PublicFeedVideo = {
  id: string; title: string; media_type: 'video'; status: 'private' | 'pending_review' | 'approved' | 'rejected'
  video_url: string; poster_url: string; width?: number; height?: number; duration_ms?: number; published_at?: string; created_at: string
}
export type PublicMediaItem = { media_type: 'image'; id: string; image: ImageResult } | { media_type: 'video'; id: string; video: PublicFeedVideo }
export const publicMediaApi = {
  async list(page = 1, pageSize = 20, options?: { sort?: 'latest' | 'hot'; query?: string; model?: string; liked?: boolean; favorited?: boolean; accessToken?: string | null }): Promise<PageResult<PublicMediaItem>> {
    const result = normalizePage<PublicMediaItem>(await sharedApiClient.request('/api/public/v1/media', {
      auth: false, retryUnauthorized: false,
      headers: options?.accessToken ? { Authorization: `Bearer ${options.accessToken}` } : undefined,
      query: { page, page_size: pageSize, sort: options?.sort, query: options?.query, route_model_code: options?.model, liked: options?.liked, favorited: options?.favorited },
    }))
    return { ...result, items: result.items.map((item) => item.media_type === 'image' ? { ...item, image: toImageResult(item.image) } : item) }
  },
}
