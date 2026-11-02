const assert = {
  equal(actual: unknown, expected: unknown) { if (actual !== expected) throw new Error(`Expected ${String(expected)}, got ${String(actual)}`) },
  deepEqual(actual: unknown, expected: unknown, message = 'Values differ') { if (JSON.stringify(actual) !== JSON.stringify(expected)) throw new Error(message) },
  ok(value: unknown) { if (!value) throw new Error('Expected truthy value') },
  match(value: string, pattern: RegExp) { if (!pattern.test(value)) throw new Error(`Unexpected value: ${value}`) },
  async rejects(promise: Promise<unknown>, expected: RegExp | ((error: Error) => boolean)) {
    try { await promise } catch (error) {
      if (error instanceof Error && (expected instanceof RegExp ? expected.test(error.message) : expected(error))) return
      throw error
    }
    throw new Error('Expected rejection')
  },
}
import type { MediaAsset, MediaUploadSession, ReferenceAsset } from './api-types'
import { readUnifiedImageReference, uploadUnifiedImage, waitForReadyMediaAsset } from './unified-reference-upload'

const asset = { id: 'unified-image', project_id: 'project-a', media_type: 'image', status: 'processing' } as MediaAsset
const reference = { id: asset.id, status: 'ready', name: 'image.png' } as ReferenceAsset

async function main() {
  const order: string[] = []
  const projection = await readUnifiedImageReference(asset, 'project-a', {
    getMediaAsset: async (id) => { assert.equal(id, asset.id); order.push('ready'); return { ...asset, status: 'ready_original' } },
    getReferenceAsset: async (id) => { assert.equal(id, asset.id); order.push('projection'); return reference },
  }, { pollIntervalMs: 1 })
  assert.equal(projection.id, asset.id)
  assert.deepEqual(order, ['ready', 'projection'], 'projection must be a read after processing completes')

  for (const [patch, message] of [
    [{ status: 'failed' }, /图片处理失败/],
    [{ status: 'processing_failed' }, /图片处理失败/],
    [{ status: 'deleting' }, /图片已删除/],
    [{ status: 'deleted' }, /图片已删除/],
    [{ media_type: 'video' }, /不是图片/],
    [{ project_id: 'project-b' }, /所属项目已变化/],
    [{ status: 'unknown' }, /当前不可用/],
  ] as const) {
    await assert.rejects(waitForReadyMediaAsset({ ...asset, ...patch }, 'project-a', async () => { throw new Error('must not poll unusable assets') }), message)
  }
  await assert.rejects(readUnifiedImageReference({ ...asset, status: 'ready' }, 'project-a', {
    getMediaAsset: async () => asset,
    getReferenceAsset: async () => ({ ...reference, id: 'alias-id' }),
  }), /标识不一致/)
  await assert.rejects(readUnifiedImageReference({ ...asset, status: 'ready' }, 'project-a', {
    getMediaAsset: async () => asset,
    getReferenceAsset: async () => ({ ...reference, status: 'processing' }),
  }), /尚未就绪/)
  await assert.rejects(waitForReadyMediaAsset(asset, 'project-a', async () => asset, { timeoutMs: 10, pollIntervalMs: 1 }), /资产处理超时/)
  const controller = new AbortController()
  const waiting = waitForReadyMediaAsset(asset, 'project-a', async () => { throw new Error('must not poll after cancel') }, { signal: controller.signal, pollIntervalMs: 1000 })
  controller.abort()
  await assert.rejects(waiting, (error: Error) => error.name === 'AbortError')

  const uploaded: string[] = []
  const file = new File(['abcdefg'], 'image.png', { type: 'image/png' }) as unknown as globalThis.File
  const completed = await uploadUnifiedImage(file, 'project-a', {
    initMediaUpload: async (input, key, signal) => {
      assert.equal(input.project_id, 'project-a')
      assert.equal(input.media_type, 'image')
      assert.equal(input.filename, file.name)
      assert.equal(input.size_bytes, 7)
      assert.ok(key && signal)
      uploaded.push('init')
      return { id: 'upload', project_id: 'project-a', part_size: 3, part_count: 3, storage_driver: 'local' } as MediaUploadSession
    },
    signMediaUploadPart: async () => { throw new Error('local must use proxy') },
    abortMediaUpload: async () => { throw new Error('completed session must not abort') },
    uploadMediaProxyPart: async (id, number, chunk, checksum, token, signal) => {
      assert.equal(id, 'upload')
      assert.equal(token, 'token')
      assert.ok(signal)
      assert.match(checksum, /^[a-f0-9]{64}$/)
      uploaded.push(await chunk.text())
      return { part_number: number, etag: `etag-${number}`, checksum, size_bytes: chunk.size }
    },
    completeMediaUpload: async (id, parts, signal) => {
      assert.equal(id, 'upload')
      assert.deepEqual(parts.map((part) => part.part_number), [1, 2, 3])
      assert.ok(signal)
      uploaded.push('complete')
      return asset
    },
  }, { accessToken: 'token' })
  assert.equal(completed.id, asset.id)
  assert.deepEqual(uploaded, ['init', 'abc', 'def', 'g', 'complete'])
  await assert.rejects(uploadUnifiedImage(file, '', {} as never), /请先选择项目/)
  const originalFetch = globalThis.fetch
  try {
    for (const mode of ['direct', 'cors', 'etag', 'http', 'cancel'] as const) {
      const calls: string[] = []
      const abort = new AbortController()
      globalThis.fetch = async () => {
        calls.push('direct')
        if (mode === 'cancel') { abort.abort(); throw new DOMException('cancel', 'AbortError') }
        if (mode === 'cors') throw new TypeError('CORS failure')
        if (mode === 'http') return new Response('', { status: 500 })
        return new Response('', { headers: mode === 'etag' ? {} : { etag: '"s3-etag"' } })
      }
      const operation = uploadUnifiedImage(file, 'project-a', {
        initMediaUpload: async () => ({ id: 's3-session', project_id: 'project-a', part_size: 7, part_count: 1, storage_driver: 's3' } as MediaUploadSession),
        signMediaUploadPart: async () => { calls.push('sign'); return { url: 'https://storage.example/part', expires_at: '' } },
        uploadMediaProxyPart: async () => { calls.push('proxy'); return { part_number: 1, etag: 'proxy-etag' } },
        completeMediaUpload: async (_, parts) => { calls.push('complete'); assert.equal(parts[0].etag, mode === 'direct' ? 's3-etag' : 'proxy-etag'); return asset },
        abortMediaUpload: async () => { calls.push('abort') },
      }, { signal: abort.signal })
      if (mode === 'http') await assert.rejects(operation, /500/)
      else if (mode === 'cancel') await assert.rejects(operation, (error) => error.name === 'AbortError')
      else await operation
      assert.deepEqual(calls, mode === 'direct' ? ['sign', 'direct', 'complete'] : ['http', 'cancel'].includes(mode) ? ['sign', 'direct', 'abort'] : ['sign', 'direct', 'proxy', 'complete'])
    }
  } finally { globalThis.fetch = originalFetch }
  const originalWarn = console.warn
  try {
    const warnings: unknown[][] = []
    console.warn = (...args) => { warnings.push(args) }
    for (const mode of ['cancel', 'timeout'] as const) {
      const controller = new AbortController()
      const originalError = new Error('original cancellation')
      let cleanupSignal: AbortSignal | undefined
      let cleanupStarted = false
      const operation = uploadUnifiedImage(file, 'project-a', {
        initMediaUpload: async () => ({ id: 'hung-cleanup', project_id: 'project-a', part_size: 7, part_count: 1, storage_driver: 'local' } as MediaUploadSession),
        signMediaUploadPart: async () => { throw new Error('must use proxy') },
        uploadMediaProxyPart: async (_, __, ___, ____, _____, signal) => {
          if (mode === 'cancel') { controller.abort(originalError); throw originalError }
          return new Promise((_, reject) => signal?.addEventListener('abort', () => reject(signal.reason), { once: true }))
        },
        completeMediaUpload: async () => { throw new Error('must not complete failed upload') },
        abortMediaUpload: async (_, signal) => {
          cleanupStarted = true
          cleanupSignal = signal
          assert.ok(signal && !signal.aborted)
          assert.ok(signal !== controller.signal)
          return new Promise(() => {}) // Deliberately ignores cancellation forever.
        },
      }, { signal: controller.signal, timeoutMs: 15, cleanupTimeoutMs: 40 })
      let watchdog: ReturnType<typeof setTimeout> | undefined
      try {
        await assert.rejects(Promise.race([operation, new Promise<never>((_, reject) => {
          watchdog = setTimeout(() => reject(new Error('upload rejection blocked by cleanup')), 200)
        })]), mode === 'cancel' ? (error) => error === originalError : /参考图上传超时/)
      } finally { clearTimeout(watchdog) }
      assert.ok(cleanupStarted)
      assert.ok(cleanupSignal && !cleanupSignal.aborted)
      await new Promise((resolve) => setTimeout(resolve, 60))
      assert.ok(cleanupSignal?.aborted)
    }
    assert.equal(warnings.length, 2)
  } finally { console.warn = originalWarn }
  console.log('unified reference upload contracts passed')
}
void main().catch((error) => { setTimeout(() => { throw error }, 0) })
