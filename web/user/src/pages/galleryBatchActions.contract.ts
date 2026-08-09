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
const pollGalleryExportJob = model.pollGalleryExportJob as ((initial: ExportStatus, getStatus: (jobID: string, signal?: AbortSignal) => Promise<ExportStatus>, options?: {
	maxAttempts?: number
	wait?: (signal?: AbortSignal) => Promise<void>
	signal?: AbortSignal
	now?: () => number
	marginMs?: number
	setTimer?: (callback: () => void, delayMs: number) => unknown
	clearTimer?: (timer: unknown) => void
}) => Promise<ExportStatus>) | undefined
if (!invertLoadedGallerySelection || !reconcileGalleryBatchSelection || !pollGalleryExportJob) {
  throw new Error('gallery batch actions need selection reconciliation and bounded export polling')
}

type ExportStatus = { job: { id: string; state: string; error_message?: string; deadline_at?: string } }

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
		{ job: { id: 'export-1', state: 'queued', deadline_at: '2026-08-09T00:20:00Z' } },
	  async (jobID) => ({ job: { id: jobID, state: polledStates.shift() ?? 'failed', deadline_at: '2026-08-09T00:20:00Z' } }),
	  { maxAttempts: 3, now: () => Date.parse('2026-08-09T00:00:00Z'), wait: async () => undefined },
)
if (completedExport.job.state !== 'succeeded' || polledStates.length !== 0) {
  throw new Error('async export polling must continue through queued/running states until success')
}

let virtualNow = Date.parse('2026-08-09T00:00:00Z')
let longPolls = 0
const longRunningExport = await pollGalleryExportJob(
	{ job: { id: 'export-long', state: 'queued', deadline_at: '2026-08-09T00:20:00Z' } },
	async (jobID) => ({ job: { id: jobID, state: ++longPolls > 112 ? 'succeeded' : 'running', deadline_at: '2026-08-09T00:20:00Z' } }),
	{ now: () => virtualNow, marginMs: 60_000, wait: async () => { virtualNow += 8_000 } },
)
if (longRunningExport.job.state !== 'succeeded' || virtualNow - Date.parse('2026-08-09T00:00:00Z') <= 10 * 60_000) {
	throw new Error('gallery export polling must cover long queueing within the server lifecycle deadline')
}

virtualNow = Date.parse('2026-08-09T00:00:00Z')
let deadlineMessage = ''
try {
	await pollGalleryExportJob(
			{ job: { id: 'export-deadline', state: 'running', deadline_at: '2026-08-09T00:00:10Z' } },
			async (jobID) => ({ job: { id: jobID, state: 'running', deadline_at: '2026-08-09T00:00:10Z' } }),
		{ now: () => virtualNow, marginMs: 1_000, wait: async () => { virtualNow += 6_000 } },
	)
} catch (error) { deadlineMessage = error instanceof Error ? error.message : String(error) }
if (!deadlineMessage.includes('timed out')) throw new Error(`server export deadline must bound polling, got ${deadlineMessage}`)
virtualNow = Date.parse('2026-08-09T00:00:00Z')
let refreshedDeadlineFetches = 0
let refreshedDeadlineMessage = ''
try {
	await pollGalleryExportJob(
		{ job: { id: 'export-refresh', state: 'queued', deadline_at: '2026-08-09T00:20:00Z' } },
		async (jobID) => {
			refreshedDeadlineFetches += 1
			return { job: { id: jobID, state: 'running', deadline_at: '2026-08-09T00:00:04Z' } }
		},
		{ now: () => virtualNow, marginMs: 0, wait: async () => { virtualNow += 3_000 } },
	)
} catch (error) { refreshedDeadlineMessage = error instanceof Error ? error.message : String(error) }
if (!refreshedDeadlineMessage.includes('timed out') || refreshedDeadlineFetches !== 1) {
	throw new Error(`polling must refresh the authoritative deadline from every response, message=${refreshedDeadlineMessage} fetches=${refreshedDeadlineFetches}`)
}

const stalledDeadline = new Date(Date.now() + 20).toISOString()
const stalledResult = await Promise.race([
	pollGalleryExportJob(
		{ job: { id: 'export-stalled', state: 'running', deadline_at: stalledDeadline } },
		async () => await new Promise<ExportStatus>(() => undefined),
		{ marginMs: 0, wait: async () => undefined },
	).then(() => 'unexpected success', (error) => error instanceof Error ? error.message : String(error)),
	new Promise<string>((resolve) => setTimeout(() => resolve('watchdog expired'), 150)),
])
if (!stalledResult.includes('timed out')) {
	throw new Error(`a never-resolving status fetch must abort at the server deadline, got ${stalledResult}`)
}

const externalController = new AbortController()
let externalAbortName = ''
const externalAbortPoll = pollGalleryExportJob(
	{ job: { id: 'export-unmount', state: 'running', deadline_at: new Date(Date.now() + 60_000).toISOString() } },
	async (_jobID, signal) => await new Promise<ExportStatus>((_resolve, reject) => {
		signal?.addEventListener('abort', () => reject(signal.reason ?? new DOMException('Aborted', 'AbortError')), { once: true })
	}),
	{ signal: externalController.signal, wait: async () => undefined },
)
setTimeout(() => externalController.abort(), 0)
try { await externalAbortPoll } catch (error) { externalAbortName = error instanceof Error ? error.name : '' }
if (externalAbortName !== 'AbortError') {
	throw new Error(`external unmount abort must remain silent AbortError, got ${externalAbortName}`)
}

let scheduledTimers = 0
let clearedTimers = 0
const timerTokens = new Set<object>()
const timerCleanupStatus = await pollGalleryExportJob(
	{ job: { id: 'export-timer-cleanup', state: 'running', deadline_at: new Date(Date.now() + 60_000).toISOString() } },
	async (jobID) => ({ job: { id: jobID, state: 'succeeded', deadline_at: new Date(Date.now() + 60_000).toISOString() } }),
	{
		wait: async () => undefined,
		setTimer: () => {
			const token = {}
			scheduledTimers += 1
			timerTokens.add(token)
			return token
		},
		clearTimer: (timer) => {
			clearedTimers += 1
			timerTokens.delete(timer as object)
		},
	},
)
if (timerCleanupStatus.job.state !== 'succeeded' || scheduledTimers !== 1 || clearedTimers !== 1 || timerTokens.size !== 0) {
	throw new Error(`status fetch deadline timer leaked: scheduled=${scheduledTimers} cleared=${clearedTimers} live=${timerTokens.size}`)
}
let missingDeadlineMessage = ''
try {
  await pollGalleryExportJob(
    { job: { id: 'export-2', state: 'queued' } },
    async (jobID) => ({ job: { id: jobID, state: 'running' } }),
		{ maxAttempts: 2, wait: async () => undefined },
  )
} catch (error) {
	missingDeadlineMessage = error instanceof Error ? error.message : String(error)
}
if (!missingDeadlineMessage.includes('missing deadline')) {
	throw new Error(`async export polling must require the authoritative server deadline, got ${missingDeadlineMessage}`)
}
const controller = new AbortController()
let abortedFetches = 0
const abortedPoll = pollGalleryExportJob(
	{ job: { id: 'export-abort', state: 'queued', deadline_at: '2026-08-09T00:20:00Z' } },
	async () => { abortedFetches += 1; return { job: { id: 'export-abort', state: 'succeeded', deadline_at: '2026-08-09T00:20:00Z' } } },
	{ signal: controller.signal, now: () => Date.parse('2026-08-09T00:00:00Z'), wait: async (signal) => { controller.abort(); signal?.throwIfAborted() } },
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
