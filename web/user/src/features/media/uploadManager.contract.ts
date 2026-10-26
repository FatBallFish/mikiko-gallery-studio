import {
  boundedUploadRequest,
  uploadFailure,
  acceptUploadFiles,
  completedPartNumbers,
  createUploadSnapshot,
  fileContentFingerprint,
  MEDIA_UPLOAD_MAX_BYTES,
  mediaUploadSessionKey,
  recoverableUploadSnapshot,
  reconcileUploadSession,
  retry,
  restoreUploadSnapshots,
  serializeUploadSnapshots,
  shouldFallbackToProxy,
} from './uploadManager'

const image = new File(['image'], 'cover.png', { type: 'image/png' })
const video = new File(['video'], 'clip.mp4', { type: 'video/mp4' })
const invalid = new File(['text'], 'notes.txt', { type: 'text/plain' })

const accepted = acceptUploadFiles([image, invalid, video])
if (accepted.accepted.length !== 2 || accepted.rejected.length !== 1 || accepted.rejected[0]?.file.name !== 'notes.txt') {
  throw new Error('invalid files must not block valid files')
}

if (acceptUploadFiles([new File([], 'exact.bin', { type: 'video/mp4' })], 0).rejected[0]?.reason !== '文件为空') {
  throw new Error('empty files must remain invalid at any configured boundary')
}
const exactLimit = { name: 'exact.mp4', type: 'video/mp4', size: MEDIA_UPLOAD_MAX_BYTES } as File
const overLimit = { name: 'over.mp4', type: 'video/mp4', size: MEDIA_UPLOAD_MAX_BYTES + 1 } as File
if (acceptUploadFiles([exactLimit]).accepted.length !== 1) throw new Error('a file exactly at the 1 GiB upload limit must be accepted')
if (acceptUploadFiles([overLimit]).rejected[0]?.reason !== '文件超过 1 GiB 限制') throw new Error('a file one byte over the 1 GiB upload limit must be rejected')

const fingerprint = await fileContentFingerprint(image, 2)
const pending = createUploadSnapshot(image, 'project-a', 'campaign', fingerprint)
if (pending.transport !== 'direct') throw new Error('new uploads must prefer direct object-storage transport')
const restored = restoreUploadSnapshots(serializeUploadSnapshots([pending]))
if (restored.length !== 1 || restored[0]?.fileName !== 'cover.png' || restored[0]?.status !== 'needs_file') {
  throw new Error('persisted uploads must survive navigation without pretending the File object survived reload')
}
if (restoreUploadSnapshots(JSON.stringify([{ ...pending, transport: undefined }]))[0]?.transport !== 'direct') {
  throw new Error('legacy upload snapshots must restore with direct transport preference')
}
const targeted = createUploadSnapshot(image, 'project-a', '', fingerprint, { canvasID: 'canvas-a', nodeID: 'image-frame-a' })
const restoredTarget = restoreUploadSnapshots(serializeUploadSnapshots([targeted]))[0]?.target
if (restoredTarget?.canvasID !== 'canvas-a' || restoredTarget.nodeID !== 'image-frame-a') throw new Error('canvas upload targets must survive upload session persistence')
const malformedTarget = restoreUploadSnapshots(JSON.stringify([{ ...targeted, target: { canvasID: '', nodeID: 7 } }]))[0]?.target
if (malformedTarget !== undefined) throw new Error('malformed persisted canvas upload targets must be discarded')

if (!shouldFallbackToProxy(new TypeError('Failed to fetch'))) throw new Error('browser network failures must trigger proxy fallback')
if (!shouldFallbackToProxy(Object.assign(new Error('ETag unavailable'), { code: 'DIRECT_ETAG_UNAVAILABLE' }))) throw new Error('unreadable ETag must trigger proxy fallback')
if (shouldFallbackToProxy(Object.assign(new Error('forbidden'), { status: 403 }))) throw new Error('explicit object-storage responses must not be disguised as CORS failures')
if (shouldFallbackToProxy(new DOMException('paused', 'AbortError'))) throw new Error('paused uploads must not trigger proxy fallback')

if (mediaUploadSessionKey('user-a') === mediaUploadSessionKey('user-b')) {
  throw new Error('upload persistence keys must be isolated by current user id')
}
if (!mediaUploadSessionKey(' user/a ').endsWith('user%2Fa')) {
  throw new Error('upload persistence keys must normalize and safely encode user ids')
}

const sameMetadataDifferentContent = new File(['other'], image.name, { type: image.type })
const differentFingerprint = await fileContentFingerprint(sameMetadataDifferentContent, 2)
if (fingerprint === differentFingerprint) throw new Error('fingerprint must distinguish same-metadata files with different sampled content')
if (recoverableUploadSnapshot(pending, image, fingerprint) !== pending) throw new Error('matching content fingerprints must resume the existing upload')
if (recoverableUploadSnapshot(pending, sameMetadataDifferentContent, differentFingerprint) !== null) {
  throw new Error('same-name and same-size files with different content must never reuse uploaded parts')
}
if (recoverableUploadSnapshot({ ...pending, contentFingerprint: undefined }, image, fingerprint) !== null) {
  throw new Error('legacy upload snapshots without a content fingerprint must not reuse uploaded parts')
}

const sampledRanges: Array<[number, number]> = []
const largeFile = {
  size: MEDIA_UPLOAD_MAX_BYTES,
  slice(start = 0, end = MEDIA_UPLOAD_MAX_BYTES) {
    sampledRanges.push([start, end])
    return new Blob([new Uint8Array(end - start)])
  },
}
await fileContentFingerprint(largeFile, 8)
if (JSON.stringify(sampledRanges) !== JSON.stringify([[0, 8], [MEDIA_UPLOAD_MAX_BYTES - 8, MEDIA_UPLOAD_MAX_BYTES]])) {
  throw new Error(`large-file fingerprints must read only bounded head and tail samples, got ${JSON.stringify(sampledRanges)}`)
}

const traySource = await import('node:fs').then(({ readFileSync }) => readFileSync(new URL('./UploadTray.tsx', import.meta.url), 'utf8'))
for (const required of ['mediaUploadSessionKey(userID)', 'fileContentFingerprint(candidate.file)', 'recoverableUploadSnapshot(item, candidate.file, candidate.contentFingerprint)', 'uploadMediaProxyPart', "transport: 'proxy'", 'MEDIA_UPLOAD_COMPLETED_EVENT', 'options.target']) {
  if (!traySource.includes(required)) throw new Error(`UploadTray must wire safe user-scoped recovery through ${required}`)
}
if (!traySource.includes('useState(false)')) {
  throw new Error('an empty upload tray must start collapsed so it does not cover page actions or footer links')
}
const stylesSource = await import('node:fs').then(({ readFileSync }) => readFileSync(new URL('../../styles.css', import.meta.url), 'utf8'))
if (!traySource.includes("items.length ? '' : ' is-empty'") || !stylesSource.includes('.canvas-editor ~ .media-upload-tray.is-empty:not(.is-expanded)')) {
  throw new Error('an idle upload tray must collapse to an icon-sized canvas control instead of covering generation actions')
}
if (traySource.includes('sessionStorage.getItem(MEDIA_UPLOAD_SESSION_KEY)')) {
  throw new Error('UploadTray must never restore another account through the legacy global session key')
}
const appSource = await import('node:fs').then(({ readFileSync }) => readFileSync(new URL('../../App.tsx', import.meta.url), 'utf8'))
if (!appSource.includes('<ProjectProvider key={projectUserID} userID={projectUserID}>')) {
  throw new Error('switching accounts must remount the upload tray before reading the next user-scoped session key')
}

const reconciled = reconcileUploadSession(pending, {
  id: 'upload-a',
  project_id: 'project-a',
  original_filename: 'cover.png',
  declared_media_type: 'image',
  declared_mime_type: 'image/png',
  declared_size_bytes: image.size,
  storage_driver: 's3',
  part_size: 4,
  part_count: 2,
  status: 'initialized',
  completed_parts: [{ part_number: 1, etag: 'one' }],
  expires_at: '2099-01-01T00:00:00Z',
})
if (reconciled.uploadID !== 'upload-a' || reconciled.completedParts.length !== 1 || completedPartNumbers(reconciled)[0] !== 1) {
  throw new Error('server upload state must resume from completed parts')
}

let abortedAttempts = 0
await retry(async () => {
  abortedAttempts += 1
  throw new DOMException('paused', 'AbortError')
}).catch(() => undefined)
if (abortedAttempts !== 1) throw new Error('paused uploads must not retry aborted requests')

const expired = restoreUploadSnapshots(serializeUploadSnapshots([{ ...pending, uploadID: 'expired', expiresAt: '2000-01-01' }]))[0]
if (expired.localID !== pending.localID || !expired.initializationKey || expired.initializationKey === pending.localID || expired.uploadID) throw new Error('expired sessions must rotate init identity and preserve UI identity')
const ambiguous = restoreUploadSnapshots(serializeUploadSnapshots([{ ...pending, ...uploadFailure(new Error('timeout'), 'initializing') }]))[0]
if ((ambiguous.initializationKey ?? ambiguous.localID) !== pending.localID) throw new Error('ambiguous init retries must retain the idempotency key')
let timeoutSignal: AbortSignal | undefined
const timeoutError = await boundedUploadRequest((signal) => {
  timeoutSignal = signal
  return new Promise<never>(() => undefined)
}, new AbortController().signal, 5).catch((error: unknown) => error)
if (!timeoutSignal?.aborted || uploadFailure(timeoutError, 'initializing').status !== 'failed' || !uploadFailure(timeoutError, 'initializing').error?.includes('创建上传会话失败')) throw new Error('deferred init must time out into retryable initialization failure')
const paused = new AbortController()
let resolveLate!: (value: string) => void
const pendingInit = boundedUploadRequest(() => new Promise<string>((resolve) => { resolveLate = resolve }), paused.signal)
await Promise.resolve()
paused.abort()
const pauseError = await pendingInit.catch((error: unknown) => error)
resolveLate('late session')
if (uploadFailure(pauseError, 'initializing').status !== 'paused') throw new Error('pause must settle deferred init even before a late response')
if (!uploadFailure(new Error('offline'), 'completing').error?.includes('完成上传失败')) throw new Error('completion failures must identify the correct stage')

console.log('media upload manager contract passed')

const { MEDIA_UPLOAD_TERMINATED_EVENT, notifyUploadTerminated } = await import('./uploadManager')
const terminalEvents: CustomEvent[] = []
const previousWindow = Object.getOwnPropertyDescriptor(globalThis, 'window')
Object.defineProperty(globalThis, 'window', { configurable: true, value: { dispatchEvent: (event: CustomEvent) => { terminalEvents.push(event); return true } } })
try {
  const target = { canvasID: 'canvas', nodeID: 'upload-token' }
  notifyUploadTerminated({ target, status: 'failed', error: 'network failed' })
  notifyUploadTerminated({ target, status: 'cancelled' })
  if (terminalEvents.length !== 2 || terminalEvents.some((event) => event.type !== MEDIA_UPLOAD_TERMINATED_EVENT || event.detail.target !== target)) throw new Error('terminal events must carry the original upload destination')
  if (terminalEvents[0].detail.status !== 'failed' || terminalEvents[1].detail.status !== 'cancelled') throw new Error('failure and cancellation must remain distinct')
} finally {
  if (previousWindow) Object.defineProperty(globalThis, 'window', previousWindow)
  else Reflect.deleteProperty(globalThis, 'window')
}
