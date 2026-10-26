import { notifyUploadTerminated } from './uploadManager'
import { useCallback, useEffect, useRef, useState, type CSSProperties, type PointerEvent as ReactPointerEvent } from 'react'
import { clampUploadPosition, restoreUploadPosition, snapUploadPosition, uploadDragMoved, uploadPanelGeometry, uploadQueueSummary, type FloatBounds, type FloatPosition } from './uploadFloatGeometry'
import { ChevronDown, ChevronUp, CirclePause, Play, RotateCcw, Trash2, Upload, X } from 'lucide-react'
import type { MediaAsset, MediaCompletedPart, MediaUploadSession } from '../../../../shared/api-types'
import { userApi } from '../../../../shared/user-api'
import { useProjects } from '../../ProjectContext'
import { useApp } from '../../components'
import {
  replaceExpiredUpload,
  boundedUploadRequest,
  uploadFailure,
  type UploadStage,
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
  shouldFallbackToProxy,
  type UploadSnapshot,
  type UploadTarget,
} from './uploadManager'

export const MEDIA_ASSETS_CHANGED_EVENT = 'mgs:media-assets-changed'
export const QUEUE_MEDIA_UPLOAD_EVENT = 'mgs:queue-media-upload'
export const MEDIA_UPLOAD_COMPLETED_EVENT = 'mgs:media-upload-completed'
export type QueueMediaUploadDetail = { files?: File[]; projectID?: string; target?: UploadTarget }
export type MediaUploadCompletedDetail = { projectID: string; assetID: string; mediaType: MediaAsset['media_type']; target?: UploadTarget; asset: MediaAsset }

async function sha256Hex(blob: Blob) {
  const digest = await crypto.subtle.digest('SHA-256', await blob.arrayBuffer())
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('')
}

function s3CompletedPart(partNumber: number, chunk: Blob, checksum: string, response: Response): MediaCompletedPart {
  const etag = response.headers.get('etag')?.replace(/^"|"$/g, '')
  if (!etag) throw Object.assign(new Error('对象存储 ETag 不可读'), { code: 'DIRECT_ETAG_UNAVAILABLE' })
  return { part_number: partNumber, etag, checksum, size_bytes: chunk.size }
}

export function UploadTray({ hidden = false }: { hidden?: boolean }) {
  const app = useApp()
  const projects = useProjects()
  const userID = String(app.profile?.id ?? app.session?.profile.id ?? '').trim()
  const storageKey = mediaUploadSessionKey(userID)
  const [items, setItems] = useState<UploadSnapshot[]>(() => restoreUploadSnapshots(sessionStorage.getItem(storageKey)))
  const [expanded, setExpanded] = useState(false)
  const files = useRef(new Map<string, File>())
  const controllers = useRef(new Map<string, AbortController>())
  const inputRef = useRef<HTMLInputElement | null>(null)
  const trayRef = useRef<HTMLElement | null>(null)
  const positionKey = `mgs:upload-float:${encodeURIComponent(userID)}`
  const [position, setPosition] = useState<FloatPosition | null>(() => {
    try { return restoreUploadPosition(localStorage.getItem(positionKey)) } catch { return null }
  })
  const [bounds, setBounds] = useState<FloatBounds | null>(null)
  const [revealed, setRevealed] = useState(false)
  const drag = useRef<{ id: number; startX: number; startY: number; origin: FloatPosition; moved: boolean } | null>(null)
  const suppressClick = useRef(false)
  const summary = uploadQueueSummary(items)
  const mobileOverlayOpen = () => window.matchMedia('(max-width: 760px)').matches
    && Boolean(document.querySelector('.u-controls.is-open, .model-group-select-trigger[aria-expanded="true"]'))
  const openQueue = () => {
    setRevealed(true)
    setExpanded(!mobileOverlayOpen())
  }

  useEffect(() => {
    const closeForOverlay = () => {
      if (mobileOverlayOpen()) setExpanded(false)
    }
    closeForOverlay()
    const observer = new MutationObserver(closeForOverlay)
    observer.observe(document.body, { subtree: true, childList: true, attributes: true, attributeFilter: ['class', 'aria-expanded'] })
    window.addEventListener('resize', closeForOverlay)
    return () => { observer.disconnect(); window.removeEventListener('resize', closeForOverlay) }
  }, [])

  useEffect(() => {
    try { if (position) localStorage.setItem(positionKey, JSON.stringify(position)) } catch { /* Storage may be disabled. */ }
  }, [position, positionKey])

  useEffect(() => {
    const viewport = window.visualViewport
    const measure = () => {
      if (!window.matchMedia('(max-width: 760px)').matches) { setBounds(null); return }
      const style = trayRef.current ? getComputedStyle(trayRef.current) : null
      const safeBottom = parseFloat(style?.getPropertyValue('--upload-safe-bottom') ?? '') || 0
      const safeTop = parseFloat(style?.getPropertyValue('--upload-safe-top') ?? '') || 0
      const left = viewport?.offsetLeft ?? 0
      const top = (viewport?.offsetTop ?? 0) + safeTop + 8
      const viewportBottom = (viewport?.offsetTop ?? 0) + (viewport?.height ?? window.innerHeight)
      let bottom = viewportBottom - safeBottom - 76
      // Reserve the collapsed generation CTA; an open sheet leaves room above itself.
      const controlsElement = document.querySelector('.u-controls')
      const controls = controlsElement?.getBoundingClientRect()
      const footer = controlsElement?.querySelector('.u-controls__foot')?.getBoundingClientRect()
      if (footer && footer.top > top + 48) bottom = Math.min(bottom, footer.top - 8)
      if (controls && controls.top > top + 96 && controls.top < viewportBottom) bottom = Math.min(bottom, controls.top - 8)
      const next = { left: left + 8, top, right: left + (viewport?.width ?? window.innerWidth) - 8, bottom: Math.max(top + 48, bottom) }
      setBounds(next)
      setPosition((current) => clampUploadPosition(current ?? { x: next.right - 48, y: next.bottom - 48, edge: 'right' }, next))
    }
    measure()
    window.addEventListener('resize', measure)
    viewport?.addEventListener('resize', measure)
    viewport?.addEventListener('scroll', measure)
    let controls = document.querySelector('.u-controls')
    const observer = new ResizeObserver(measure)
    if (controls) observer.observe(controls)
    const routeObserver = new MutationObserver(() => {
      const next = document.querySelector('.u-controls')
      if (next === controls) return
      observer.disconnect()
      controls = next
      if (controls) observer.observe(controls)
      measure()
    })
    routeObserver.observe(document.body, { childList: true, subtree: true })
    return () => { window.removeEventListener('resize', measure); viewport?.removeEventListener('resize', measure); viewport?.removeEventListener('scroll', measure); observer.disconnect(); routeObserver.disconnect() }
  }, [hidden])

  const finishDrag = (event: ReactPointerEvent<HTMLButtonElement>, cancelled = false) => {
    const current = drag.current
    if (!current || current.id !== event.pointerId) return
    drag.current = null
    suppressClick.current = current.moved || cancelled
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
    if (bounds) setPosition((value) => value ? snapUploadPosition(value, bounds) : value)
    setRevealed(false)
  }
  const floatingStyle: CSSProperties | undefined = bounds && position ? expanded
    ? { ...uploadPanelGeometry(position, bounds), right: 'auto', bottom: 'auto' }
    : { left: position.x, top: position.y, right: 'auto', bottom: 'auto', transform: !revealed && position.edge ? `translateX(${position.edge === 'left' ? '-24px' : '24px'})` : undefined }
    : undefined

  const update = useCallback((localID: string, updater: (current: UploadSnapshot) => UploadSnapshot) => {
    setItems((current) => current.map((item) => item.localID === localID ? updater(item) : item))
  }, [])

  useEffect(() => {
    sessionStorage.setItem(storageKey, serializeUploadSnapshots(items))
  }, [items, storageKey])

  useEffect(() => {
    const open = () => {
      openQueue()
      inputRef.current?.click()
    }
    window.addEventListener('mgs:open-media-upload', open)
    return () => window.removeEventListener('mgs:open-media-upload', open)
  }, [])

  useEffect(() => {
    const queueUpload = (event: Event) => {
      const detail = (event as CustomEvent<QueueMediaUploadDetail>).detail
      if (!detail?.files?.length) return
      void addFiles(detail.files, { projectID: detail.projectID, target: detail.target }).catch((error) => {
        notifyUploadTerminated({ target: detail.target, status: 'failed', error: error instanceof Error ? error.message : '上传准备失败' })
      })
    }
    window.addEventListener(QUEUE_MEDIA_UPLOAD_EVENT, queueUpload)
    return () => window.removeEventListener(QUEUE_MEDIA_UPLOAD_EVENT, queueUpload)
  })

  useEffect(() => () => {
    controllers.current.forEach((controller) => controller.abort())
  }, [])

  const runUpload = useCallback(async (initial: UploadSnapshot) => {
    const file = files.current.get(initial.localID)
    if (!file) {
      update(initial.localID, (item) => ({ ...item, status: 'needs_file', error: '请重新选择同名文件以继续上传' }))
      return
    }
    if (controllers.current.has(initial.localID)) return
    const controller = new AbortController()
    controllers.current.set(initial.localID, controller)
    let snapshot = replaceExpiredUpload(initial)
    let stage: UploadStage = snapshot.uploadID ? 'uploading' : 'initializing'
    try {
      update(initial.localID, () => ({ ...snapshot, status: stage, error: undefined }))
      let session: MediaUploadSession
      if (snapshot.uploadID) session = await boundedUploadRequest((signal) => userApi.getMediaUpload(snapshot.uploadID!, signal), controller.signal)
      else {
        session = await boundedUploadRequest((signal) => userApi.initMediaUpload({
          project_id: snapshot.projectID, group_name: snapshot.groupName, filename: snapshot.fileName,
          media_type: snapshot.mediaType, mime_type: snapshot.mimeType, size_bytes: snapshot.fileSize,
        }, snapshot.initializationKey ?? snapshot.localID, signal), controller.signal)
      }
      controller.signal.throwIfAborted()
      snapshot = reconcileUploadSession(snapshot, session)
      if (snapshot.status === 'cancelled') throw new Error('上传会话已取消')
      stage = 'uploading'
      update(snapshot.localID, () => ({ ...snapshot, status: 'uploading', error: undefined }))
      const transport = { current: snapshot.transport }
      const uploaded = new Map(snapshot.completedParts.map((part) => [part.part_number, part]))
      const completed = new Set(completedPartNumbers(snapshot))
      const missing = Array.from({ length: session.part_count }, (_, index) => index + 1).filter((part) => !completed.has(part))
      await mapConcurrent(missing, MEDIA_UPLOAD_CONCURRENCY, async (partNumber) => {
        if (controller.signal.aborted) throw new DOMException('Upload paused', 'AbortError')
        const start = (partNumber - 1) * session.part_size
        const chunk = file.slice(start, Math.min(file.size, start + session.part_size))
        const checksum = await sha256Hex(chunk)
        const part = await retry(() => boundedUploadRequest(async (signal) => {
          if (transport.current === 'proxy') {
            try {
              return await userApi.uploadMediaProxyPart(session.id, partNumber, chunk, checksum, undefined, signal)
            } catch (caught) {
              throw uploadPartError(partNumber, caught)
            }
          }
          const target = await userApi.signMediaUploadPart(session.id, partNumber, checksum, signal)
          try {
            const response = await fetch(target.url, { method: 'PUT', body: chunk, headers: target.headers, signal })
            if (!response.ok) throw Object.assign(new Error(`分片 ${partNumber}：对象存储返回 ${response.status}`), { status: response.status })
            return s3CompletedPart(partNumber, chunk, checksum, response)
          } catch (caught) {
            if (!shouldFallbackToProxy(caught)) throw caught
            transport.current = 'proxy'
            update(snapshot.localID, (item) => ({ ...item, transport: 'proxy', error: undefined }))
            try {
              return await userApi.uploadMediaProxyPart(session.id, partNumber, chunk, checksum, undefined, signal)
            } catch (proxyError) {
              throw uploadPartError(partNumber, proxyError)
            }
          }
        }, controller.signal, 120_000))
        controller.signal.throwIfAborted()
        uploaded.set(partNumber, part)
        const completedParts = Array.from(uploaded.values()).sort((a, b) => a.part_number - b.part_number)
        update(snapshot.localID, (item) => ({ ...item, completedParts, progress: completedParts.length / session.part_count }))
      })
      controller.signal.throwIfAborted()
      stage = 'completing'
      update(snapshot.localID, (item) => ({ ...item, status: 'completing' }))
      const asset = await boundedUploadRequest((signal) => userApi.completeMediaUpload(session.id, Array.from(uploaded.values()).sort((a, b) => a.part_number - b.part_number), signal), controller.signal)
      controller.signal.throwIfAborted()
      update(snapshot.localID, (item) => ({ ...item, status: 'completed', assetID: asset.id, progress: 1, error: undefined }))
      files.current.delete(snapshot.localID)
      window.dispatchEvent(new CustomEvent(MEDIA_ASSETS_CHANGED_EVENT, { detail: { projectID: asset.project_id, assetID: asset.id, mediaType: asset.media_type } }))
      window.dispatchEvent(new CustomEvent<MediaUploadCompletedDetail>(MEDIA_UPLOAD_COMPLETED_EVENT, { detail: { projectID: asset.project_id, assetID: asset.id, mediaType: asset.media_type, target: snapshot.target, asset } }))
      app.notify('success', `${snapshot.fileName} 上传完成`)
    } catch (caught) {
      const failure = uploadFailure(caught, stage)
      controller.abort() // Stop sibling parts before allowing the same session to resume.
      update(initial.localID, (item) => ({ ...item, ...failure }))
      if (failure.status === 'failed') notifyUploadTerminated({ target: initial.target, status: 'failed', error: failure.error })
    } finally {
      controllers.current.delete(initial.localID)
    }
  }, [app, update])

  const addFiles = async (incoming: Iterable<File> | FileList | null, options: { projectID?: string; target?: UploadTarget } = {}) => {
    const projectID = options.projectID || projects.selectedProjectID
    if (!incoming) return
    if (!projectID) { notifyUploadTerminated({ target: options.target, status: 'failed', error: '请选择上传项目' }); return }
    const result = acceptUploadFiles(incoming)
    const accepted = await Promise.all(result.accepted.map(async (candidate) => ({
      ...candidate,
      contentFingerprint: await fileContentFingerprint(candidate.file),
    })))
    const additions: UploadSnapshot[] = []
    const claimedRecovered = new Set<string>()
    for (const candidate of accepted) {
      const existing = items.find((item) => !claimedRecovered.has(item.localID) && item.status === 'needs_file' && (!options.target || (item.target?.canvasID === options.target.canvasID && item.target.nodeID === options.target.nodeID)) && recoverableUploadSnapshot(item, candidate.file, candidate.contentFingerprint))
      if (existing) {
        claimedRecovered.add(existing.localID)
        files.current.set(existing.localID, candidate.file)
        update(existing.localID, (item) => ({ ...item, status: 'paused', error: undefined }))
        queueMicrotask(() => void runUpload({ ...existing, status: 'paused', error: undefined }))
      } else {
        const snapshot = createUploadSnapshot(candidate.file, projectID, '', candidate.contentFingerprint, options.target)
        files.current.set(snapshot.localID, candidate.file)
        additions.push(snapshot)
        queueMicrotask(() => void runUpload(snapshot))
      }
    }
    if (additions.length) setItems((current) => [...additions, ...current])
    result.rejected.forEach(({ file, reason }) => {
      app.notify('error', `${file.name}：${reason}`)
      notifyUploadTerminated({ target: options.target, status: 'failed', error: reason })
    })
    if (inputRef.current) inputRef.current.value = ''
  }

  const pause = (item: UploadSnapshot) => {
    controllers.current.get(item.localID)?.abort()
    update(item.localID, (current) => ({ ...current, status: 'paused' }))
  }
  const cancel = async (item: UploadSnapshot) => {
    controllers.current.get(item.localID)?.abort()
    notifyUploadTerminated({ target: item.target, status: 'cancelled' })
    if (item.uploadID) await userApi.abortMediaUpload(item.uploadID).catch(() => undefined)
    files.current.delete(item.localID)
    setItems((current) => current.filter((candidate) => candidate.localID !== item.localID))
  }
  const resume = (item: UploadSnapshot) => void runUpload(item)
  const clearFinished = () => setItems((current) => current.filter((item) => item.status !== 'completed' && item.status !== 'cancelled'))

  return (
    <aside ref={trayRef} style={{ ...floatingStyle, ...(hidden ? { display: 'none' } : {}) }} hidden={hidden} data-dock-side={position?.edge ?? undefined} data-queue-state={summary.state} className={`media-upload-tray${expanded ? ' is-expanded' : ''}${items.length ? '' : ' is-empty'}`} aria-label="上传队列">
      <button type="button" className="media-upload-float" aria-label={`展开上传队列，${summary.count} 个待处理${summary.state === 'error' ? '，有失败或需重选文件' : summary.state === 'paused' ? '，有暂停任务' : ''}`} aria-expanded={expanded}
        onFocus={() => setRevealed(true)} onBlur={() => setRevealed(false)}
        onPointerDown={(event) => {
          if (!bounds || !position || !event.isPrimary || event.button !== 0) return
          suppressClick.current = false
          drag.current = { id: event.pointerId, startX: event.clientX, startY: event.clientY, origin: position, moved: false }
          event.currentTarget.setPointerCapture(event.pointerId)
        }}
        onPointerMove={(event) => {
          const current = drag.current
          if (!current || current.id !== event.pointerId || !bounds) return
          current.moved ||= uploadDragMoved(current.startX, current.startY, event.clientX, event.clientY)
          if (!current.moved) return
          setRevealed(true)
          setPosition(clampUploadPosition({ x: current.origin.x + event.clientX - current.startX, y: current.origin.y + event.clientY - current.startY, edge: null }, bounds))
        }}
        onPointerUp={(event) => finishDrag(event)} onPointerCancel={(event) => finishDrag(event, true)} onLostPointerCapture={(event) => finishDrag(event, true)}
        onClick={(event) => { if (event.detail !== 0 && suppressClick.current) { suppressClick.current = false; return }; openQueue() }}>
        {summary.state === 'paused' ? <CirclePause size={21} /> : <Upload size={21} />}
        {summary.count > 0 ? <span className="media-upload-badge">{summary.count}</span> : null}
      </button>
      <header>
        <button type="button" className="media-upload-title" onClick={() => expanded ? setExpanded(false) : openQueue()}>
          <Upload size={17} /><strong>上传</strong>{items.length ? <span>{items.length}</span> : null}{expanded ? <ChevronDown size={16} /> : <ChevronUp size={16} />}
        </button>
        <input ref={inputRef} hidden type="file" multiple accept=".jpg,.jpeg,.png,.webp,.mp4,.mp3,.m4a,.wav,image/jpeg,image/png,image/webp,video/mp4,audio/mpeg,audio/mp4,audio/x-m4a,audio/wav,audio/x-wav" onChange={(event) => void addFiles(event.target.files)} />
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
    queued: '等待上传', initializing: '正在创建会话', uploading: `${item.transport === 'proxy' ? '兼容上传 · ' : ''}${Math.round(item.progress * 100)}%`, paused: '已暂停',
    needs_file: '需要重新选择文件', completing: '正在完成', completed: '已完成', failed: '上传失败', cancelled: '已取消',
  }
  return labels[item.status]
}

function uploadPartError(partNumber: number, error: unknown) {
  if (error instanceof DOMException && error.name === 'AbortError') return error
  const message = error instanceof Error ? error.message : '上传失败'
  return new Error(`分片 ${partNumber}：${message}`)
}
