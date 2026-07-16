import { existsSync, readFileSync } from 'node:fs'

const modelURL = new URL('./galleryBatchActions.ts', import.meta.url)
if (!existsSync(modelURL)) {
  throw new Error('gallery batch actions need an executable all-settled result model')
}

type BatchResult<T, R> = {
  succeeded: Array<{ item: T; value: R }>
  failed: Array<{ item: T; reason: unknown }>
}
type RunBatch = <T, R>(items: T[], action: (item: T) => Promise<R>) => Promise<BatchResult<T, R>>

const model = await import('./galleryBatchActions') as unknown as Record<string, unknown>
const runGalleryBatch = model.runGalleryBatch as RunBatch | undefined
if (!runGalleryBatch) throw new Error('gallery batch actions need runGalleryBatch')

const attempts: string[] = []
const partial = await runGalleryBatch(['image-1', 'image-2', 'image-3'], async (id) => {
  attempts.push(id)
  if (id === 'image-2') throw new Error('rejected')
  return { id, status: 'updated' }
})

if (attempts.join(',') !== 'image-1,image-2,image-3') {
  throw new Error(`each batch item must be attempted exactly once, got ${attempts.join(',')}`)
}
if (partial.succeeded.map((entry) => entry.item).join(',') !== 'image-1,image-3') {
  throw new Error('partial success must retain the exact succeeded items for local reconciliation')
}
if (partial.failed.map((entry) => entry.item).join(',') !== 'image-2') {
  throw new Error('partial success must retain only failed items for explicit retry')
}

const allFailed = await runGalleryBatch(['image-4', 'image-5'], async (id) => {
  throw new Error(`failed:${id}`)
})
if (allFailed.succeeded.length !== 0 || allFailed.failed.length !== 2) {
  throw new Error('an all-failed batch must still produce an authoritative result summary')
}

const pageSource = readFileSync(new URL('./GalleryPage.tsx', import.meta.url), 'utf8')
if (!pageSource.includes('runGalleryBatch')) {
  throw new Error('delete, publish and group paths must use the shared all-settled batch runner')
}
if (pageSource.includes('await Promise.all(images.map') || pageSource.includes('await Promise.all(groupDialog.ids.map')) {
  throw new Error('gallery batch mutations must not short-circuit on the first failed item')
}
