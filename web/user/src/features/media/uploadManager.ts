import type { MediaCompletedPart, MediaType, MediaUploadSession } from '../../../../shared/api-types'

export const MEDIA_UPLOAD_INIT_TIMEOUT_MS = 25_000

// Race cancellation as well as passing the signal: auth refresh may still be pending.
export async function boundedUploadRequest<T>(operation: (signal: AbortSignal) => Promise<T>, parent: AbortSignal, timeoutMS = MEDIA_UPLOAD_INIT_TIMEOUT_MS): Promise<T> {
  const controller = new AbortController()
  let timer: ReturnType<typeof setTimeout> | undefined
  let onAbort: () => void = () => undefined
  const cancelled = new Promise<never>((_, reject) => {
    onAbort = () => {
      controller.abort()
      reject(new DOMException('Upload paused', 'AbortError'))
    }
    parent.addEventListener('abort', onAbort, { once: true })
    timer = setTimeout(() => {
      controller.abort()
      reject(new Error('请求超时，请重试；将继续使用同一上传会话'))
    }, timeoutMS)
  })
  try {
    if (parent.aborted) onAbort()
    return await Promise.race([cancelled, Promise.resolve().then(() => {
      controller.signal.throwIfAborted()
      return operation(controller.signal)
    })])
  } finally {
    clearTimeout(timer)
    parent.removeEventListener('abort', onAbort)
  }
}

export type UploadStage = 'initializing' | 'uploading' | 'completing'
export function uploadFailure(error: unknown, stage: UploadStage): Pick<UploadSnapshot, 'status' | 'error'> {
  if (error instanceof DOMException && error.name === 'AbortError') return { status: 'paused', error: undefined }
  const label = { initializing: '创建上传会话失败', uploading: '上传分片失败', completing: '完成上传失败' }[stage]
  return { status: 'failed', error: `${label}：${error instanceof Error ? error.message : '请重试'}` }
}

export const MEDIA_UPLOAD_SESSION_KEY = 'mgs.media.uploads.v2'
export const MEDIA_UPLOAD_MAX_BYTES = 1 << 30
export const MEDIA_UPLOAD_CONCURRENCY = 3
export const MEDIA_UPLOAD_RETRIES = 3
export const MEDIA_UPLOAD_FINGERPRINT_SAMPLE_BYTES = 64 * 1024

const acceptedMIMETypes: Record<MediaType, readonly string[]> = {
  image: ['image/jpeg', 'image/png', 'image/webp'],
  video: ['video/mp4'],
  audio: ['audio/mpeg', 'audio/mp4', 'audio/x-m4a', 'audio/wav', 'audio/x-wav'],
}

export type UploadStatus = 'queued' | 'initializing' | 'uploading' | 'paused' | 'needs_file' | 'completing' | 'completed' | 'failed' | 'cancelled'
export type UploadTransport = 'direct' | 'proxy'
export type UploadTarget = { canvasID: string; nodeID: string }

export type UploadSnapshot = {
  localID: string
  initializationKey?: string
  uploadID?: string
  assetID?: string
  projectID: string
  groupName: string
  fileName: string
  fileSize: number
  mimeType: string
  mediaType: MediaType
  status: UploadStatus
  transport: UploadTransport
  storageDriver?: string
  partSize?: number
  partCount?: number
  completedParts: MediaCompletedPart[]
  progress: number
  error?: string
  expiresAt?: string
  contentFingerprint?: string
  target?: UploadTarget
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

export function createUploadSnapshot(file: File, projectID: string, groupName = '', contentFingerprint?: string, target?: UploadTarget): UploadSnapshot {
  const mediaType = mediaTypeForFile(file)
  if (!mediaType) throw new Error('不支持的文件格式')
  return {
    localID: crypto.randomUUID(), projectID, groupName, fileName: file.name, fileSize: file.size,
    mimeType: file.type, mediaType, status: 'queued', transport: 'direct', completedParts: [], progress: 0, contentFingerprint, target,
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
    transport: session.storage_driver === 'local' ? 'proxy' : snapshot.transport ?? 'direct',
    partSize: session.part_size,
    partCount: session.part_count,
    completedParts,
    expiresAt: session.expires_at,
    progress: session.status === 'completed' ? 1 : progress,
    status: session.status === 'completed' ? 'completed' : session.status === 'aborted' ? 'cancelled' : snapshot.status,
  }
}

export function replaceExpiredUpload(snapshot: UploadSnapshot, now = Date.now()): UploadSnapshot {
  if (snapshot.status === 'completed' || !snapshot.expiresAt || new Date(snapshot.expiresAt).getTime() > now) return snapshot
  return { ...snapshot, initializationKey: crypto.randomUUID(), uploadID: undefined, assetID: undefined, completedParts: [], progress: 0, expiresAt: undefined }
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
      const expired = item.status !== 'completed' && (item.expiresAt ? new Date(item.expiresAt).getTime() <= now : false)
      const target = item.target && typeof item.target.canvasID === 'string' && item.target.canvasID.trim() && typeof item.target.nodeID === 'string' && item.target.nodeID.trim()
        ? { canvasID: item.target.canvasID, nodeID: item.target.nodeID }
        : undefined
      return {
        ...replaceExpiredUpload(item, now),
        target,
        transport: item.transport === 'proxy' ? 'proxy' : 'direct',
        completedParts: expired ? [] : Array.isArray(item.completedParts) ? item.completedParts : [],
        status: item.status === 'completed' ? 'completed' : item.status === 'failed' && !expired ? 'failed' : 'needs_file',
        error: expired ? '上传会话已过期，请重新选择文件' : item.error,
      }
    })
  } catch {
    return []
  }
}

export function shouldFallbackToProxy(error: unknown) {
  if (error instanceof DOMException && error.name === 'AbortError') return false
  if (typeof error === 'object' && error !== null) {
    if ('status' in error && typeof error.status === 'number') return false
    if ('code' in error && error.code === 'DIRECT_ETAG_UNAVAILABLE') return true
  }
  return error instanceof TypeError
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

export const MEDIA_UPLOAD_TERMINATED_EVENT = 'mgs:media-upload-terminated'
export type MediaUploadTerminatedDetail = { target?: UploadTarget; status: 'failed' | 'cancelled'; error?: string }
export function notifyUploadTerminated(detail: MediaUploadTerminatedDetail) {
  window.dispatchEvent(new CustomEvent<MediaUploadTerminatedDetail>(MEDIA_UPLOAD_TERMINATED_EVENT, { detail }))
}
