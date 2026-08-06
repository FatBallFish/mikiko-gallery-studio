import type { MediaAccessProjection, MediaAccessPurpose } from '../../shared/api-types'
import { userApi } from '../../shared/user-api'

export type MediaResource =
  | { kind: 'image'; scope: 'private' | 'public'; id: string }
  | { kind: 'reference'; scope: 'private'; id: string }

export type MediaAccessClient = {
  refreshImageAccess: (id: string, purpose: MediaAccessPurpose) => Promise<MediaAccessProjection>
  refreshReferenceAssetAccess: (id: string, purpose: MediaAccessPurpose) => Promise<MediaAccessProjection>
  refreshPublicImageAccess: (id: string, purpose: MediaAccessPurpose) => Promise<MediaAccessProjection>
}

type MediaAccessManagerOptions = {
  refreshMarginMs?: number
}

export type MediaAccessManager = {
  preview: (resource: MediaResource, current?: MediaAccessProjection, nowMs?: number) => Promise<MediaAccessProjection>
  download: (resource: MediaResource) => Promise<MediaAccessProjection>
}

const DEFAULT_REFRESH_MARGIN_MS = 30_000

export function createMediaAccessManager(
  client: MediaAccessClient,
  options: MediaAccessManagerOptions = {},
): MediaAccessManager {
  const refreshMarginMs = options.refreshMarginMs ?? DEFAULT_REFRESH_MARGIN_MS
  const previewRequests = new Map<string, Promise<MediaAccessProjection>>()

  const project = (resource: MediaResource, purpose: MediaAccessPurpose) => {
    if (resource.kind === 'reference') {
      return client.refreshReferenceAssetAccess(resource.id, purpose)
    }
    if (resource.scope === 'public') {
      return client.refreshPublicImageAccess(resource.id, purpose)
    }
    return client.refreshImageAccess(resource.id, purpose)
  }

  return {
    preview(resource, current, nowMs = Date.now()) {
      if (current && isMediaProjectionFresh(current, nowMs, refreshMarginMs)) {
        return Promise.resolve(current)
      }
      const key = mediaResourceKey(resource)
      const existing = previewRequests.get(key)
      if (existing) return existing

      const request = project(resource, 'preview').finally(() => {
        if (previewRequests.get(key) === request) previewRequests.delete(key)
      })
      previewRequests.set(key, request)
      return request
    },
    download(resource) {
      return project(resource, 'download')
    },
  }
}

export function isMediaProjectionFresh(
  projection: MediaAccessProjection,
  nowMs = Date.now(),
  refreshMarginMs = DEFAULT_REFRESH_MARGIN_MS,
) {
  if (!projection.url.trim()) return false
  if (!projection.expires_at) return true
  const expiresAt = Date.parse(projection.expires_at)
  return Number.isFinite(expiresAt) && expiresAt-nowMs > refreshMarginMs
}

export function mediaResourceKey(resource: MediaResource) {
  return `${resource.kind}:${resource.scope}:${resource.id}`
}

export const mediaAccess = createMediaAccessManager(userApi)
