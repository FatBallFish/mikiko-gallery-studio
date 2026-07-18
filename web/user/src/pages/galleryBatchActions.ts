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
