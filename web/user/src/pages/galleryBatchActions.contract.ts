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

const invertLoadedGallerySelection = model.invertLoadedGallerySelection as ((current: ReadonlySet<string>, loadedIDs: string[]) => Set<string>) | undefined
const reconcileGalleryBatchSelection = model.reconcileGalleryBatchSelection as ((current: ReadonlySet<string>, succeeded: string[], failed: string[]) => Set<string>) | undefined
const pollGalleryExportJob = model.pollGalleryExportJob as ((initial: ExportStatus, getStatus: (jobID: string, signal?: AbortSignal) => Promise<ExportStatus>, options?: { maxAttempts?: number; wait?: (signal?: AbortSignal) => Promise<void>; signal?: AbortSignal }) => Promise<ExportStatus>) | undefined
if (!invertLoadedGallerySelection || !reconcileGalleryBatchSelection || !pollGalleryExportJob) {
  throw new Error('gallery batch actions need selection reconciliation and bounded export polling')
}

type ExportStatus = { job: { id: string; state: string; error_message?: string } }

const inverted = invertLoadedGallerySelection(new Set(['loaded-1', 'hidden']), ['loaded-1', 'loaded-2'])
if (inverted.has('loaded-1') || !inverted.has('loaded-2') || inverted.has('hidden')) {
	throw new Error('invert must affect current loaded IDs only and prune hidden selections')
}
const retriable = reconcileGalleryBatchSelection(new Set(['one', 'two', 'hidden']), ['one'], ['two'])
if (retriable.has('one') || !retriable.has('two') || !retriable.has('hidden')) {
  throw new Error('successful IDs must clear while failed IDs stay selected for retry')
}

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

const polledStates = ['running', 'succeeded']
const completedExport = await pollGalleryExportJob(
  { job: { id: 'export-1', state: 'queued' } },
  async (jobID) => ({ job: { id: jobID, state: polledStates.shift() ?? 'failed' } }),
  { maxAttempts: 3, wait: async () => undefined },
)
if (completedExport.job.state !== 'succeeded' || polledStates.length !== 0) {
  throw new Error('async export polling must continue through queued/running states until success')
}
let timeoutMessage = ''
try {
  await pollGalleryExportJob(
    { job: { id: 'export-2', state: 'queued' } },
    async (jobID) => ({ job: { id: jobID, state: 'running' } }),
    { maxAttempts: 2, wait: async () => undefined },
  )
} catch (error) {
  timeoutMessage = error instanceof Error ? error.message : String(error)
}
if (!timeoutMessage.includes('timed out')) {
  throw new Error(`async export polling must stop with an explicit timeout, got ${timeoutMessage}`)
}
const controller = new AbortController()
let abortedFetches = 0
const abortedPoll = pollGalleryExportJob(
  { job: { id: 'export-abort', state: 'queued' } },
  async () => { abortedFetches += 1; return { job: { id: 'export-abort', state: 'succeeded' } } },
  { signal: controller.signal, wait: async (signal) => { controller.abort(); signal?.throwIfAborted() } },
)
let abortName = ''
try { await abortedPoll } catch (error) { abortName = error instanceof Error ? error.name : '' }
if (abortName !== 'AbortError' || abortedFetches !== 0) {
  throw new Error(`aborted export polling must stop before fetch, name=${abortName} fetches=${abortedFetches}`)
}

const pageSource = readFileSync(new URL('./GalleryPage.tsx', import.meta.url), 'utf8')
for (const serverBatchAction of ['batchPublishGalleryImages', 'batchGroupGalleryImages', 'batchDeleteGalleryImages']) {
  if (!pageSource.includes(serverBatchAction)) throw new Error(`gallery mutations must use server batch contract: ${serverBatchAction}`)
}
if (!pageSource.includes('AbortController') || !pageSource.includes('controller.abort()') || !pageSource.includes('isAbortError')) {
  throw new Error('gallery export polling must abort on component cleanup and suppress abort notifications')
}
if (pageSource.includes('await Promise.all(images.map') || pageSource.includes('await Promise.all(groupDialog.ids.map')) {
  throw new Error('gallery batch mutations must not short-circuit on the first failed item')
}

for (const contract of ['已加载资产', '反选', '清除选择', '批量转移项目', 'batchDownloadGalleryImages', 'batchTransferGalleryImages']) {
  if (!pageSource.includes(contract)) throw new Error(`gallery batch toolbar needs ${contract}`)
}
if (pageSource.includes('window.setTimeout(() => void downloadImage')) {
  throw new Error('batch download must use one authorized ZIP response instead of repeated browser downloads')
}
if (pageSource.includes("assetSelectVisual: 'grid size-[22px]") && pageSource.includes('opacity-0')) {
  throw new Error('gallery selection must be discoverable without hover')
}
