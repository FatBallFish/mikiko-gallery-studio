import {
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
const restored = restoreUploadSnapshots(serializeUploadSnapshots([pending]))
if (restored.length !== 1 || restored[0]?.fileName !== 'cover.png' || restored[0]?.status !== 'needs_file') {
  throw new Error('persisted uploads must survive navigation without pretending the File object survived reload')
}

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
for (const required of ['mediaUploadSessionKey(userID)', 'fileContentFingerprint(candidate.file)', 'recoverableUploadSnapshot(item, candidate.file, candidate.contentFingerprint)']) {
  if (!traySource.includes(required)) throw new Error(`UploadTray must wire safe user-scoped recovery through ${required}`)
}
if (!traySource.includes('useState(false)')) {
  throw new Error('an empty upload tray must start collapsed so it does not cover page actions or footer links')
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

console.log('media upload manager contract passed')
