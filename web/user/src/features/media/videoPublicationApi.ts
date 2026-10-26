import { getDefaultBaseUrl, sharedApiClient } from '../../../../shared/http-client'

export type PublicVideo = {
  id: string; title: string; media_type: 'video'; status: 'private' | 'pending_review' | 'approved' | 'rejected'
  prompt?: string; video_url: string; poster_url: string; width?: number; height?: number
  duration_ms?: number; published_at?: string; created_at: string
}
export type VideoPublication = PublicVideo & { review_reason?: string }
export type PublicVideoPage = { items: PublicVideo[]; total: number; page: number; page_size: number }
const ownerPath = (id: string) => `/api/agent/media/v1/video-publications/${encodeURIComponent(id)}`
export const videoPublicationApi = {
  list: (page = 1, pageSize = 12) => sharedApiClient.request<PublicVideoPage>('/api/public/v1/videos', { auth: false, retryUnauthorized: false, query: { page, page_size: pageSize } }),
  detail: (id: string) => sharedApiClient.request<PublicVideo>(`/api/public/v1/videos/${encodeURIComponent(id)}`, { auth: false, retryUnauthorized: false }),
  owner: (id: string) => sharedApiClient.request<VideoPublication>(ownerPath(id)),
  submit: (id: string) => sharedApiClient.request<VideoPublication>(ownerPath(id), { method: 'POST' }),
  withdraw: (id: string) => sharedApiClient.request<VideoPublication>(`${ownerPath(id)}:withdraw`, { method: 'POST' }),
}
// These anonymous endpoints never carry owner access tokens or signed storage URLs.
export function publicVideoURL(path: string) {
  return new URL(path, `${getDefaultBaseUrl() || window.location.origin}/`).toString()
}
export function videoPublicationLabel(status: string) {
  return ({ private: '私有', pending_review: '待审核', approved: '已公开', rejected: '未通过' } as Record<string, string>)[status] ?? '私有'
}
