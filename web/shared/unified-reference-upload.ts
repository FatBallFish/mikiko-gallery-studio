import type { MediaAsset, MediaCompletedPart, MediaPartTarget, MediaUploadInit, MediaUploadSession, ReferenceAsset } from './api-types'

export type MediaReadyOptions = { signal?: AbortSignal; timeoutMs?: number; pollIntervalMs?: number }

function cancelled(signal: AbortSignal) {
  if (signal.aborted) throw signal.reason ?? new DOMException('操作已取消', 'AbortError')
}

export async function withMediaTimeout<T>(options: MediaReadyOptions, operation: (signal: AbortSignal) => Promise<T>, message = '资产处理超时，请稍后在资产列表查看处理状态。'): Promise<T> {
  const controller = new AbortController()
  const abort = () => controller.abort(options.signal?.reason)
  if (options.signal?.aborted) abort()
  else options.signal?.addEventListener('abort', abort, { once: true })
  const timer = setTimeout(() => controller.abort(new Error(message)), options.timeoutMs ?? 120_000)
  try {
    cancelled(controller.signal)
    return await operation(controller.signal)
  } catch (error) {
    if (controller.signal.aborted) throw controller.signal.reason
    throw error
  } finally {
    clearTimeout(timer)
    options.signal?.removeEventListener('abort', abort)
  }
}

function pause(ms: number, signal: AbortSignal) {
  return new Promise<void>((resolve, reject) => {
    cancelled(signal)
    const abort = () => { clearTimeout(timer); reject(signal.reason ?? new DOMException('操作已取消', 'AbortError')) }
    const timer = setTimeout(() => { signal.removeEventListener('abort', abort); resolve() }, ms)
    signal.addEventListener('abort', abort, { once: true })
  })
}

export async function waitForReadyMediaAsset(initial: MediaAsset, projectID: string, read: (id: string, signal: AbortSignal) => Promise<MediaAsset>, options: MediaReadyOptions = {}): Promise<MediaAsset> {
  return withMediaTimeout(options, async (signal) => {
    let asset = initial
    for (;;) {
      cancelled(signal)
      if (asset.id !== initial.id || asset.project_id !== projectID) throw new Error('资产所属项目已变化，请重新选择。')
      if (asset.media_type !== 'image') throw new Error('该资产不是图片，不能用作参考图。')
      if (asset.status === 'ready' || asset.status === 'ready_original') return asset
      if (['failed', 'processing_failed'].includes(asset.status)) throw new Error('图片处理失败，请在资产列表重试处理或重新上传。')
      if (['deleted', 'deleting'].includes(asset.status)) throw new Error('图片已删除，不能用作参考图。')
      if (!['pending_upload', 'uploading', 'uploaded', 'pending', 'processing', 'queued'].includes(asset.status)) throw new Error('图片当前不可用，请在资产列表查看状态。')
      await pause(options.pollIntervalMs ?? 1000, signal)
      asset = await read(initial.id, signal)
    }
  })
}

export async function readUnifiedImageReference(asset: MediaAsset, projectID: string, api: {
  getMediaAsset: (id: string, signal?: AbortSignal) => Promise<MediaAsset>
  getReferenceAsset: (id: string, signal?: AbortSignal) => Promise<ReferenceAsset>
}, options: MediaReadyOptions = {}): Promise<ReferenceAsset> {
  return withMediaTimeout(options, async (signal) => {
    const ready = await waitForReadyMediaAsset(asset, projectID, api.getMediaAsset, { ...options, signal })
    const reference = await api.getReferenceAsset(ready.id, signal)
    cancelled(signal)
    if (reference.id !== ready.id) throw new Error('参考图资产标识不一致，请刷新后重试。')
    if (!['ready', 'ready_original'].includes(reference.status)) throw new Error('图片尚未就绪，请稍后重试。')
    return reference
  })
}

async function cleanupMediaUpload(id: string, abortUpload: (id: string, signal?: AbortSignal) => Promise<unknown>, timeoutMs: number) {
  const controller = new AbortController()
  let timer: ReturnType<typeof setTimeout> | undefined
  try {
    const deadline = new Promise<never>((_, reject) => {
      timer = setTimeout(() => {
        const error = new Error('上传会话清理超时，将由服务端过期回收。')
        controller.abort(error)
        reject(error)
      }, timeoutMs)
    })
    // Race also bounds transports that ignore AbortSignal. Cleanup never delays the upload result.
    await Promise.race([abortUpload(id, controller.signal), deadline])
  } catch (error) {
    console.warn('未能清理未完成的上传会话', error)
  } finally {
    clearTimeout(timer)
  }
}

export async function uploadUnifiedImage(file: File, projectID: string, api: {
  initMediaUpload: (input: MediaUploadInit, key: string, signal?: AbortSignal) => Promise<MediaUploadSession>
  uploadMediaProxyPart: (id: string, part: number, chunk: Blob, checksum: string, token?: string | null, signal?: AbortSignal) => Promise<MediaCompletedPart>
  signMediaUploadPart: (id: string, part: number, checksum: string, signal?: AbortSignal) => Promise<MediaPartTarget>
  abortMediaUpload: (id: string, signal?: AbortSignal) => Promise<unknown>
  completeMediaUpload: (id: string, parts: MediaCompletedPart[], signal?: AbortSignal) => Promise<MediaAsset>
}, options: MediaReadyOptions & { accessToken?: string | null; cleanupTimeoutMs?: number } = {}): Promise<MediaAsset> {
  if (!projectID.trim()) throw new Error('请先选择项目再上传参考图。')
  return withMediaTimeout({ ...options, timeoutMs: options.timeoutMs ?? 300_000 }, async (signal) => {
    const session = await api.initMediaUpload({ project_id: projectID, filename: file.name, media_type: 'image', mime_type: file.type, size_bytes: file.size }, crypto.randomUUID(), signal)
    let completed = false
    try {
    if (session.project_id !== projectID || session.part_size <= 0 || session.part_count !== Math.ceil(file.size / session.part_size)) throw new Error('上传会话信息异常，请重新上传。')
    const parts: MediaCompletedPart[] = []
    let proxy = session.storage_driver === 'local'
    for (let part = 1; part <= session.part_count; part++) {
      cancelled(signal)
      const chunk = file.slice((part - 1) * session.part_size, Math.min(file.size, part * session.part_size))
      const digest = await crypto.subtle.digest('SHA-256', await chunk.arrayBuffer())
      const checksum = Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('')
      cancelled(signal)
      if (!proxy) {
        const target = await api.signMediaUploadPart(session.id, part, checksum, signal)
        try {
          const response = await fetch(target.url, { method: 'PUT', body: chunk, headers: target.headers, signal })
          if (!response.ok) throw new Error(`分片 ${part} 上传失败，存储服务返回 ${response.status}。`)
          const etag = response.headers.get('etag')?.replace(/^"|"$/g, '')
          if (etag) { parts.push({ part_number: part, etag, checksum, size_bytes: chunk.size }); continue }
          proxy = true
        } catch (error) {
          cancelled(signal)
          if (!(error instanceof TypeError)) throw error
          proxy = true
        }
      }
      parts.push(await api.uploadMediaProxyPart(session.id, part, chunk, checksum, options.accessToken, signal))
    }
    const asset = await api.completeMediaUpload(session.id, parts, signal)
    completed = true
    return asset
    } finally {
      if (!completed) {
        void cleanupMediaUpload(session.id, api.abortMediaUpload, options.cleanupTimeoutMs ?? 5000)
      }
    }
  }, '参考图上传超时，请检查网络后重试。')
}
