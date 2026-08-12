import type { MediaCompletedPart, MediaType, MediaUploadSession } from '../../../../shared/api-types'

export const MEDIA_UPLOAD_SESSION_KEY = 'mgs.media.uploads.v2'
export const MEDIA_UPLOAD_MAX_BYTES = 1 << 30
export const MEDIA_UPLOAD_CONCURRENCY = 3
export const MEDIA_UPLOAD_RETRIES = 3
export const MEDIA_UPLOAD_FINGERPRINT_SAMPLE_BYTES = 64 * 1024

const acceptedMIMETypes: Record<MediaType, readonly string[]> = {
  image: ['image/jpeg', 'image/png', 'image/webp', 'image/heic', 'image/heif', 'image/bmp', 'image/tiff', 'image/gif'],
  video: ['video/mp4', 'video/quicktime'],
  audio: ['audio/mpeg', 'audio/mp4', 'audio/x-m4a', 'audio/wav', 'audio/x-wav'],
}

export type UploadStatus = 'queued' | 'initializing' | 'uploading' | 'paused' | 'needs_file' | 'completing' | 'completed' | 'failed' | 'cancelled'

export type UploadSnapshot = {
  localID: string
  uploadID?: string
  assetID?: string
  projectID: string
  groupName: string
  fileName: string
  fileSize: number
  mimeType: string
  mediaType: MediaType
  status: UploadStatus
  storageDriver?: string
  partSize?: number
  partCount?: number
  completedParts: MediaCompletedPart[]
  progress: number
  error?: string
  expiresAt?: string
  contentFingerprint?: string
}

export type UploadCandidate = { file: File; mediaType: MediaType }
export type UploadRejection = { file: File; reason: string }

export function mediaTypeForFile(file: Pick<File, 'type'>): MediaType | null {
  const type = file.type.toLowerCase()
  for (const [mediaType, mimeTypes] of Object.entries(acceptedMIMETypes) as Array<[MediaType, readonly string[]]>) {
    if (mimeTypes.includes(type)) return mediaType
  }
  return null
}

export function acceptUploadFiles(files: Iterable<File>, maxBytes = MEDIA_UPLOAD_MAX_BYTES) {
  const accepted: UploadCandidate[] = []
  const rejected: UploadRejection[] = []
  for (const file of files) {
    const mediaType = mediaTypeForFile(file)
    if (!mediaType) rejected.push({ file, reason: '不支持的文件格式' })
    else if (file.size <= 0) rejected.push({ file, reason: '文件为空' })
    else if (file.size > maxBytes) rejected.push({ file, reason: '文件超过 1 GiB 限制' })
    else accepted.push({ file, mediaType })
  }
  return { accepted, rejected }
}

export function mediaUploadSessionKey(userID: string | number) {
  return `${MEDIA_UPLOAD_SESSION_KEY}.${encodeURIComponent(String(userID).trim())}`
}

export async function fileContentFingerprint(file: Pick<File, 'size' | 'slice'>, sampleBytes = MEDIA_UPLOAD_FINGERPRINT_SAMPLE_BYTES) {
  const sampleSize = Math.max(1, Math.floor(sampleBytes))
  const head = new Uint8Array(await file.slice(0, Math.min(file.size, sampleSize)).arrayBuffer())
  const tailStart = Math.max(head.byteLength, file.size - sampleSize)
  const tail = new Uint8Array(await file.slice(tailStart, file.size).arrayBuffer())
  const size = new TextEncoder().encode(String(file.size))
  const input = new Uint8Array(size.byteLength + 1 + head.byteLength + tail.byteLength)
  input.set(size)
  input[size.byteLength] = 0
  input.set(head, size.byteLength + 1)
  input.set(tail, size.byteLength + 1 + head.byteLength)
  const digest = await crypto.subtle.digest('SHA-256', input)
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('')
}

export function recoverableUploadSnapshot(snapshot: UploadSnapshot, file: Pick<File, 'name' | 'size' | 'type'>, contentFingerprint: string) {
  if (!snapshot.contentFingerprint || snapshot.contentFingerprint !== contentFingerprint) return null
  return snapshot.fileName === file.name && snapshot.fileSize === file.size && snapshot.mimeType === file.type ? snapshot : null
}

export function createUploadSnapshot(file: File, projectID: string, groupName = '', contentFingerprint?: string): UploadSnapshot {
  const mediaType = mediaTypeForFile(file)
  if (!mediaType) throw new Error('不支持的文件格式')
  return {
    localID: crypto.randomUUID(), projectID, groupName, fileName: file.name, fileSize: file.size,
    mimeType: file.type, mediaType, status: 'queued', completedParts: [], progress: 0, contentFingerprint,
  }
}

export function completedPartNumbers(snapshot: UploadSnapshot) {
  return snapshot.completedParts.map((part) => part.part_number).sort((a, b) => a - b)
}

export function reconcileUploadSession(snapshot: UploadSnapshot, session: MediaUploadSession): UploadSnapshot {
  const completedParts = session.completed_parts ?? []
  const progress = session.part_count > 0 ? Math.min(1, completedParts.length / session.part_count) : 0
  return {
    ...snapshot,
    uploadID: session.id,
    assetID: session.asset_id,
    projectID: session.project_id,
    groupName: session.group_name ?? snapshot.groupName,
    fileName: session.original_filename,
    fileSize: session.declared_size_bytes,
    mimeType: session.declared_mime_type,
    mediaType: session.declared_media_type,
    storageDriver: session.storage_driver,
    partSize: session.part_size,
    partCount: session.part_count,
    completedParts,
    expiresAt: session.expires_at,
    progress: session.status === 'completed' ? 1 : progress,
    status: session.status === 'completed' ? 'completed' : session.status === 'aborted' ? 'cancelled' : snapshot.status,
  }
}

export function serializeUploadSnapshots(snapshots: UploadSnapshot[]) {
  return JSON.stringify(snapshots.filter((item) => item.status !== 'cancelled'))
}

export function restoreUploadSnapshots(raw: string | null): UploadSnapshot[] {
  if (!raw) return []
  try {
    const items = JSON.parse(raw) as UploadSnapshot[]
    if (!Array.isArray(items)) return []
    const now = Date.now()
    return items.filter((item) => item && typeof item.localID === 'string').map((item) => {
      const expired = item.expiresAt ? new Date(item.expiresAt).getTime() <= now : false
      return {
        ...item,
        ...(expired ? { uploadID: undefined, completedParts: [], progress: 0, expiresAt: undefined } : {}),
        completedParts: expired ? [] : Array.isArray(item.completedParts) ? item.completedParts : [],
        status: item.status === 'completed' ? 'completed' : item.status === 'failed' && !expired ? 'failed' : 'needs_file',
        error: expired ? '上传会话已过期，请重新选择文件' : item.error,
      }
    })
  } catch {
    return []
  }
}

export async function mapConcurrent<T>(items: T[], concurrency: number, operation: (item: T) => Promise<void>) {
  let index = 0
  const workers = Array.from({ length: Math.min(Math.max(1, concurrency), items.length) }, async () => {
    while (index < items.length) {
      const item = items[index++]
      await operation(item)
    }
  })
  await Promise.all(workers)
}

export async function retry<T>(operation: () => Promise<T>, attempts = MEDIA_UPLOAD_RETRIES) {
  let lastError: unknown
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    try {
      return await operation()
    } catch (error) {
      lastError = error
      if (error instanceof DOMException && error.name === 'AbortError') throw error
      if (attempt + 1 < attempts) await new Promise((resolve) => setTimeout(resolve, 250 * (2 ** attempt)))
    }
  }
  throw lastError
}
