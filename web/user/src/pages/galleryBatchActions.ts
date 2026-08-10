export type GalleryBatchResult<T, R> = {
  succeeded: Array<{ item: T; value: R }>
  failed: Array<{ item: T; reason: unknown }>
}

export async function runGalleryBatch<T, R>(items: T[], action: (item: T) => Promise<R>): Promise<GalleryBatchResult<T, R>> {
  const settled = await Promise.allSettled(items.map((item) => Promise.resolve().then(() => action(item))))
  return settled.reduce<GalleryBatchResult<T, R>>((result, entry, index) => {
    const item = items[index]
    if (entry.status === 'fulfilled') result.succeeded.push({ item, value: entry.value })
    else result.failed.push({ item, reason: entry.reason })
    return result
  }, { succeeded: [], failed: [] })
}

export function invertLoadedGallerySelection(current: ReadonlySet<string>, loadedIDs: string[]) {
	const loaded = new Set(loadedIDs)
	const next = new Set(Array.from(current).filter((id) => loaded.has(id)))
  loadedIDs.forEach((id) => {
    if (next.has(id)) next.delete(id)
    else next.add(id)
  })
  return next
}

export function reconcileGalleryBatchSelection(current: ReadonlySet<string>, succeeded: string[], failed: string[]) {
  const next = new Set(current)
  succeeded.forEach((id) => next.delete(id))
  failed.forEach((id) => next.add(id))
  return next
}

type GalleryExportStatusLike = {
	job: { id: string; state: string; error_message?: string; deadline_at?: string }
}

export async function pollGalleryExportJob<T extends GalleryExportStatusLike>(
  initial: T,
  getStatus: (jobID: string, signal?: AbortSignal) => Promise<T>,
	options: {
		maxAttempts?: number
		wait?: (signal?: AbortSignal) => Promise<void>
		signal?: AbortSignal
		now?: () => number
		marginMs?: number
		setTimer?: (callback: () => void, delayMs: number) => unknown
		clearTimer?: (timer: unknown) => void
	} = {},
): Promise<T> {
	const now = options.now ?? Date.now
	const marginMs = options.marginMs ?? 15_000
	const wait = options.wait ?? abortableGalleryExportDelay
	const setTimer = options.setTimer ?? ((callback, delayMs) => globalThis.setTimeout(callback, delayMs))
	const clearTimer = options.clearTimer ?? ((timer) => globalThis.clearTimeout(timer as ReturnType<typeof setTimeout>))
	let status = initial
	let deadline = requiredGalleryExportServerDeadline(status, marginMs)
	let attempt = 0
	while (status.job.state === 'queued' || status.job.state === 'running') {
		if (options.maxAttempts !== undefined && attempt >= options.maxAttempts) throw new Error('gallery export polling timed out')
		if (now() >= deadline) throw new Error('gallery export polling timed out')
		options.signal?.throwIfAborted()
		await wait(options.signal)
		options.signal?.throwIfAborted()
		if (now() >= deadline) throw new Error('gallery export polling timed out')
		status = await fetchGalleryExportStatus(status.job.id, getStatus, options.signal, deadline, now, setTimer, clearTimer)
		deadline = requiredGalleryExportServerDeadline(status, marginMs)
		attempt += 1
	}
	if (status.job.state !== 'succeeded') {
    throw new Error(status.job.error_message || 'gallery export failed')
  }
  return status
}

async function fetchGalleryExportStatus<T extends GalleryExportStatusLike>(
	jobID: string,
	getStatus: (jobID: string, signal?: AbortSignal) => Promise<T>,
	externalSignal: AbortSignal | undefined,
	deadline: number,
	now: () => number,
	setTimer: (callback: () => void, delayMs: number) => unknown,
	clearTimer: (timer: unknown) => void,
): Promise<T> {
	externalSignal?.throwIfAborted()
	const remainingMs = deadline - now()
	if (remainingMs <= 0) throw galleryExportPollingTimeout()
	const controller = new AbortController()
	let deadlineExpired = false
	let rejectBoundary: (reason: unknown) => void = () => undefined
	const boundary = new Promise<never>((_resolve, reject) => { rejectBoundary = reject })
	const onExternalAbort = () => {
		const reason = externalSignal?.reason ?? new DOMException('Aborted', 'AbortError')
		controller.abort(reason)
		rejectBoundary(reason)
	}
	externalSignal?.addEventListener('abort', onExternalAbort, { once: true })
	const timer = setTimer(() => {
		deadlineExpired = true
		controller.abort(new DOMException('Gallery export polling timed out', 'AbortError'))
		rejectBoundary(galleryExportPollingTimeout())
	}, remainingMs)
	try {
		return await Promise.race([getStatus(jobID, controller.signal), boundary])
	} catch (error) {
		if (deadlineExpired) throw galleryExportPollingTimeout()
		if (externalSignal?.aborted) throw externalSignal.reason ?? new DOMException('Aborted', 'AbortError')
		throw error
	} finally {
		clearTimer(timer)
		externalSignal?.removeEventListener('abort', onExternalAbort)
	}
}

function galleryExportPollingTimeout() {
	return new Error('gallery export polling timed out')
}

function requiredGalleryExportServerDeadline(status: GalleryExportStatusLike, marginMs: number) {
	const serverDeadline = Date.parse(status.job.deadline_at ?? '')
	if (!Number.isFinite(serverDeadline)) throw new Error('gallery export status is missing deadline')
	return serverDeadline + marginMs
}

function abortableGalleryExportDelay(signal?: AbortSignal) {
  return new Promise<void>((resolve, reject) => {
    if (signal?.aborted) { reject(signal.reason ?? new DOMException('Aborted', 'AbortError')); return }
    const timer = window.setTimeout(done, 2000)
    signal?.addEventListener('abort', aborted, { once: true })
    function done() { signal?.removeEventListener('abort', aborted); resolve() }
    function aborted() { window.clearTimeout(timer); reject(signal?.reason ?? new DOMException('Aborted', 'AbortError')) }
  })
}
