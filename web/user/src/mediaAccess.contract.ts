import type { MediaAccessProjection } from '../../shared/api-types'
import { createMediaAccessManager, type MediaAccessClient, type MediaResource } from './mediaAccess'

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message)
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

const calls: string[] = []
const pendingPrivate = deferred<MediaAccessProjection>()
let privateAttempt = 0
const client: MediaAccessClient = {
  refreshImageAccess: async (id, purpose) => {
    calls.push(`private:${id}:${purpose}`)
    privateAttempt += 1
    return privateAttempt === 1 ? pendingPrivate.promise : { url: `https://objects.test/private-${privateAttempt}`, expires_at: '2026-08-06T12:10:00Z' }
  },
  refreshReferenceAssetAccess: async (id, purpose) => {
    calls.push(`reference:${id}:${purpose}`)
    return { url: 'https://objects.test/reference', expires_at: '2026-08-06T12:10:00Z' }
  },
  refreshPublicImageAccess: async (id, purpose) => {
    calls.push(`public:${id}:${purpose}`)
    return { url: 'https://objects.test/public', expires_at: '2026-08-06T12:10:00Z' }
  },
}
const manager = createMediaAccessManager(client, { refreshMarginMs: 30_000 })
const privateImage: MediaResource = { kind: 'image', scope: 'private', id: 'image-1' }

const firstPreview = manager.preview(privateImage, { url: 'https://objects.test/stale', expires_at: '2026-08-06T12:00:10Z' }, Date.parse('2026-08-06T12:00:00Z'))
const coalescedPreview = manager.preview(privateImage, undefined, Date.parse('2026-08-06T12:00:00Z'))
assert(calls.length === 1, `concurrent preview refresh should coalesce, got ${calls.join(',')}`)
pendingPrivate.resolve({ url: 'https://objects.test/private-1', expires_at: '2026-08-06T12:10:00Z' })
assert((await firstPreview).url === 'https://objects.test/private-1', 'preview should resolve the projected URL')
assert((await coalescedPreview).url === 'https://objects.test/private-1', 'coalesced preview should share the projected URL')

const fresh = { url: 'https://objects.test/fresh', expires_at: '2026-08-06T12:05:00Z' }
assert((await manager.preview(privateImage, fresh, Date.parse('2026-08-06T12:00:00Z'))) === fresh, 'fresh preview projection should preserve object identity')
assert(calls.length === 1, 'fresh preview should not issue another request')

await manager.download(privateImage)
await manager.download(privateImage)
assert(calls.filter((call) => call === 'private:image-1:download').length === 2, 'every download must request a fresh URL')

await manager.preview({ kind: 'image', scope: 'public', id: 'public-1' })
await manager.preview({ kind: 'reference', scope: 'private', id: 'reference-1' })
assert(calls.includes('public:public-1:preview'), 'public image should use the open access client')
assert(calls.includes('reference:reference-1:preview'), 'reference asset should use the reference access client')

let failedCalls = 0
const retryManager = createMediaAccessManager({
  ...client,
  refreshImageAccess: async () => {
    failedCalls += 1
    if (failedCalls === 1) throw new Error('temporary failure')
    return { url: 'https://objects.test/recovered' }
  },
})
try {
  await retryManager.preview(privateImage)
  throw new Error('first preview refresh should fail')
} catch (error) {
  if (!(error instanceof Error) || error.message !== 'temporary failure') throw error
}
assert((await retryManager.preview(privateImage)).url === 'https://objects.test/recovered', 'failed preview refresh must be retryable')
assert(failedCalls === 2, `failed in-flight entry should be cleared, got ${failedCalls} calls`)
