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
	job: { id: string; state: string; error_message?: string; processing_timeout_seconds?: number; deadline_at?: string }
}

export async function pollGalleryExportJob<T extends GalleryExportStatusLike>(
  initial: T,
  getStatus: (jobID: string, signal?: AbortSignal) => Promise<T>,
	options: { maxAttempts?: number; wait?: (signal?: AbortSignal) => Promise<void>; signal?: AbortSignal; now?: () => number; marginMs?: number } = {},
): Promise<T> {
	const now = options.now ?? Date.now
	const marginMs = options.marginMs ?? 60_000
	const wait = options.wait ?? abortableGalleryExportDelay
	let status = initial
	const fallbackDeadline = now() + Math.max(1, status.job.processing_timeout_seconds ?? 600) * 1000 + marginMs
	let deadline = galleryExportServerDeadline(status, marginMs) ?? fallbackDeadline
	let attempt = 0
	while (status.job.state === 'queued' || status.job.state === 'running') {
		if (options.maxAttempts !== undefined && attempt >= options.maxAttempts) throw new Error('gallery export polling timed out')
		if (now() >= deadline) throw new Error('gallery export polling timed out')
		options.signal?.throwIfAborted()
		await wait(options.signal)
		options.signal?.throwIfAborted()
		if (now() >= deadline) throw new Error('gallery export polling timed out')
		status = await getStatus(status.job.id, options.signal)
		deadline = galleryExportServerDeadline(status, marginMs) ?? deadline
		attempt += 1
	}
	if (status.job.state !== 'succeeded') {
    throw new Error(status.job.error_message || 'gallery export failed')
  }
  return status
}

function galleryExportServerDeadline(status: GalleryExportStatusLike, marginMs: number) {
	if (status.job.deadline_at) {
		const serverDeadline = Date.parse(status.job.deadline_at)
		if (Number.isFinite(serverDeadline)) return serverDeadline + marginMs
	}
	return undefined
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
