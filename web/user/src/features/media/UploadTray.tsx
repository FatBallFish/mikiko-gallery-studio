import { useCallback, useEffect, useRef, useState } from 'react'
import { ChevronDown, ChevronUp, CirclePause, Play, RotateCcw, Trash2, Upload, X } from 'lucide-react'
import type { MediaCompletedPart, MediaUploadSession } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { useProjects } from '../../ProjectContext'
import { useApp } from '../../components'
import {
  acceptUploadFiles,
  completedPartNumbers,
  createUploadSnapshot,
  fileContentFingerprint,
  mapConcurrent,
  MEDIA_UPLOAD_CONCURRENCY,
  mediaUploadSessionKey,
  reconcileUploadSession,
  recoverableUploadSnapshot,
  restoreUploadSnapshots,
  retry,
  serializeUploadSnapshots,
  type UploadSnapshot,
} from './uploadManager'

export const MEDIA_ASSETS_CHANGED_EVENT = 'mgs:media-assets-changed'

async function sha256Hex(blob: Blob) {
  const digest = await crypto.subtle.digest('SHA-256', await blob.arrayBuffer())
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('')
}

function s3CompletedPart(partNumber: number, chunk: Blob, checksum: string, response: Response): MediaCompletedPart {
  const etag = response.headers.get('etag')?.replace(/^"|"$/g, '')
  if (!etag) throw new Error('对象存储未返回 ETag，请检查 CORS ExposeHeaders 配置')
  return { part_number: partNumber, etag, checksum, size_bytes: chunk.size }
}

export function UploadTray() {
  const app = useApp()
  const projects = useProjects()
  const userID = String(app.profile?.id ?? app.session?.profile.id ?? '').trim()
  const storageKey = mediaUploadSessionKey(userID)
  const [items, setItems] = useState<UploadSnapshot[]>(() => restoreUploadSnapshots(sessionStorage.getItem(storageKey)))
  const [expanded, setExpanded] = useState(false)
  const files = useRef(new Map<string, File>())
  const controllers = useRef(new Map<string, AbortController>())
  const inputRef = useRef<HTMLInputElement | null>(null)

  const update = useCallback((localID: string, updater: (current: UploadSnapshot) => UploadSnapshot) => {
    setItems((current) => current.map((item) => item.localID === localID ? updater(item) : item))
  }, [])

  useEffect(() => {
    sessionStorage.setItem(storageKey, serializeUploadSnapshots(items))
  }, [items, storageKey])

  useEffect(() => {
    const open = () => {
      setExpanded(true)
      inputRef.current?.click()
    }
    window.addEventListener('mgs:open-media-upload', open)
    return () => window.removeEventListener('mgs:open-media-upload', open)
  }, [])

  useEffect(() => () => {
    controllers.current.forEach((controller) => controller.abort())
  }, [])

  const runUpload = useCallback(async (initial: UploadSnapshot) => {
    const file = files.current.get(initial.localID)
    if (!file) {
      update(initial.localID, (item) => ({ ...item, status: 'needs_file', error: '请重新选择同名文件以继续上传' }))
      return
    }
    const controller = new AbortController()
    controllers.current.set(initial.localID, controller)
    let snapshot = initial
    try {
      update(initial.localID, (item) => ({ ...item, status: item.uploadID ? 'uploading' : 'initializing', error: undefined }))
      let session: MediaUploadSession
      if (snapshot.uploadID) session = await userApi.getMediaUpload(snapshot.uploadID)
      else {
        session = await userApi.initMediaUpload({
          project_id: snapshot.projectID, group_name: snapshot.groupName, filename: snapshot.fileName,
          media_type: snapshot.mediaType, mime_type: snapshot.mimeType, size_bytes: snapshot.fileSize,
        }, snapshot.localID)
      }
      snapshot = reconcileUploadSession(snapshot, session)
      update(snapshot.localID, () => ({ ...snapshot, status: 'uploading', error: undefined }))
      const uploaded = new Map(snapshot.completedParts.map((part) => [part.part_number, part]))
      const completed = new Set(completedPartNumbers(snapshot))
      const missing = Array.from({ length: session.part_count }, (_, index) => index + 1).filter((part) => !completed.has(part))
      await mapConcurrent(missing, MEDIA_UPLOAD_CONCURRENCY, async (partNumber) => {
        if (controller.signal.aborted) throw new DOMException('Upload paused', 'AbortError')
        const start = (partNumber - 1) * session.part_size
        const chunk = file.slice(start, Math.min(file.size, start + session.part_size))
        const checksum = await sha256Hex(chunk)
        const part = await retry(async () => {
          if (session.storage_driver === 'local') {
            return userApi.uploadMediaLocalPart(session.id, partNumber, chunk, checksum, app.session?.token, controller.signal)
          }
          const target = await userApi.signMediaUploadPart(session.id, partNumber, checksum)
          const response = await fetch(target.url, { method: 'PUT', body: chunk, headers: target.headers, signal: controller.signal })
          if (!response.ok) throw Object.assign(new Error(`分片 ${partNumber} 上传失败`), { status: response.status })
          return s3CompletedPart(partNumber, chunk, checksum, response)
        })
        uploaded.set(partNumber, part)
        const completedParts = Array.from(uploaded.values()).sort((a, b) => a.part_number - b.part_number)
        update(snapshot.localID, (item) => ({ ...item, completedParts, progress: completedParts.length / session.part_count }))
      })
      update(snapshot.localID, (item) => ({ ...item, status: 'completing' }))
      const asset = await userApi.completeMediaUpload(session.id, Array.from(uploaded.values()).sort((a, b) => a.part_number - b.part_number))
      update(snapshot.localID, (item) => ({ ...item, status: 'completed', assetID: asset.id, progress: 1, error: undefined }))
      files.current.delete(snapshot.localID)
      window.dispatchEvent(new CustomEvent(MEDIA_ASSETS_CHANGED_EVENT, { detail: { projectID: asset.project_id, assetID: asset.id } }))
      app.notify('success', `${snapshot.fileName} 上传完成`)
    } catch (caught) {
      const aborted = caught instanceof DOMException && caught.name === 'AbortError'
      update(initial.localID, (item) => ({ ...item, status: aborted ? 'paused' : 'failed', error: aborted ? undefined : caught instanceof Error ? caught.message : '上传失败' }))
    } finally {
      controllers.current.delete(initial.localID)
    }
  }, [app, update])

  const addFiles = async (incoming: FileList | null) => {
    if (!incoming || !projects.selectedProjectID) return
    const result = acceptUploadFiles(incoming)
    const accepted = await Promise.all(result.accepted.map(async (candidate) => ({
      ...candidate,
      contentFingerprint: await fileContentFingerprint(candidate.file),
    })))
    const additions: UploadSnapshot[] = []
    const claimedRecovered = new Set<string>()
    for (const candidate of accepted) {
      const existing = items.find((item) => !claimedRecovered.has(item.localID) && item.status === 'needs_file' && recoverableUploadSnapshot(item, candidate.file, candidate.contentFingerprint))
      if (existing) {
        claimedRecovered.add(existing.localID)
        files.current.set(existing.localID, candidate.file)
        update(existing.localID, (item) => ({ ...item, status: 'paused', error: undefined }))
        queueMicrotask(() => void runUpload({ ...existing, status: 'paused', error: undefined }))
      } else {
        const snapshot = createUploadSnapshot(candidate.file, projects.selectedProjectID, '', candidate.contentFingerprint)
        files.current.set(snapshot.localID, candidate.file)
        additions.push(snapshot)
        queueMicrotask(() => void runUpload(snapshot))
      }
    }
    if (additions.length) setItems((current) => [...additions, ...current])
    result.rejected.forEach(({ file, reason }) => app.notify('error', `${file.name}：${reason}`))
    if (inputRef.current) inputRef.current.value = ''
  }

  const pause = (item: UploadSnapshot) => {
    controllers.current.get(item.localID)?.abort()
    update(item.localID, (current) => ({ ...current, status: 'paused' }))
  }
  const cancel = async (item: UploadSnapshot) => {
    controllers.current.get(item.localID)?.abort()
    if (item.uploadID) await userApi.abortMediaUpload(item.uploadID).catch(() => undefined)
    files.current.delete(item.localID)
    setItems((current) => current.filter((candidate) => candidate.localID !== item.localID))
  }
  const resume = (item: UploadSnapshot) => void runUpload(item)
  const clearFinished = () => setItems((current) => current.filter((item) => item.status !== 'completed' && item.status !== 'cancelled'))

  return (
    <aside className={`media-upload-tray${expanded ? ' is-expanded' : ''}`} aria-label="上传队列">
      <header>
        <button type="button" className="media-upload-title" onClick={() => setExpanded((value) => !value)}>
          <Upload size={17} /><strong>上传</strong>{items.length ? <span>{items.length}</span> : null}{expanded ? <ChevronDown size={16} /> : <ChevronUp size={16} />}
        </button>
        <input ref={inputRef} hidden type="file" multiple accept="image/jpeg,image/png,image/webp,image/heic,image/heif,image/bmp,image/tiff,image/gif,video/mp4,video/quicktime,audio/mpeg,audio/mp4,audio/x-m4a,audio/wav,audio/x-wav" onChange={(event) => void addFiles(event.target.files)} />
        <button type="button" className="media-upload-icon" title="选择文件" aria-label="选择文件" onClick={() => inputRef.current?.click()}><Upload size={16} /></button>
        {items.some((item) => item.status === 'completed') ? <button type="button" className="media-upload-icon" title="清除已完成" aria-label="清除已完成" onClick={clearFinished}><Trash2 size={16} /></button> : null}
      </header>
      {expanded ? <div className="media-upload-list">
        {!items.length ? <button type="button" className="media-upload-empty" onClick={() => inputRef.current?.click()}>选择图片、视频或音频</button> : null}
        {items.map((item) => <div className="media-upload-item" key={item.localID}>
          <div><strong title={item.fileName}>{item.fileName}</strong><span>{uploadStatusLabel(item)}</span></div>
          <progress max={1} value={item.progress} />
          <div className="media-upload-actions">
            {(item.status === 'uploading' || item.status === 'initializing') ? <button type="button" title="暂停" aria-label={`暂停 ${item.fileName}`} onClick={() => pause(item)}><CirclePause size={15} /></button> : null}
            {(item.status === 'paused' || item.status === 'failed') ? <button type="button" title="继续" aria-label={`继续 ${item.fileName}`} onClick={() => resume(item)}>{item.status === 'failed' ? <RotateCcw size={15} /> : <Play size={15} />}</button> : null}
            {item.status === 'needs_file' ? <button type="button" title="重新选择文件" aria-label={`重新选择 ${item.fileName}`} onClick={() => inputRef.current?.click()}><Upload size={15} /></button> : null}
            <button type="button" title="取消并移除" aria-label={`取消 ${item.fileName}`} onClick={() => void cancel(item)}><X size={15} /></button>
          </div>
          {item.error ? <small title={item.error}>{item.error}</small> : null}
        </div>)}
      </div> : null}
    </aside>
  )
}

function uploadStatusLabel(item: UploadSnapshot) {
  const labels: Record<UploadSnapshot['status'], string> = {
    queued: '等待上传', initializing: '正在创建会话', uploading: `${Math.round(item.progress * 100)}%`, paused: '已暂停',
    needs_file: '需要重新选择文件', completing: '正在完成', completed: '已完成', failed: '上传失败', cancelled: '已取消',
  }
  return labels[item.status]
}
