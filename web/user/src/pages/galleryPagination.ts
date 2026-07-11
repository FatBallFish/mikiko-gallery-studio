export type GalleryPageState<T> = {
  items: T[]
  page: number
  hasMore: boolean
}

export type GalleryPageUpdate = {
  page: number
  pageSize: number
  mode: 'replace' | 'append'
}

export function initialGalleryPageState<T>(): GalleryPageState<T> {
  return { items: [], page: 0, hasMore: true }
}

export function galleryLoadingForReload() {
  return { loading: true, loadingMore: false } as const
}

export function applyGalleryPage<T extends { id: string }>(state: GalleryPageState<T>, incoming: T[], update: GalleryPageUpdate): GalleryPageState<T> {
  const map = new Map<string, T>()
  if (update.mode === 'append') state.items.forEach((item) => map.set(item.id, item))
  incoming.forEach((item) => map.set(item.id, item))
  return {
    items: Array.from(map.values()),
    page: update.page,
    hasMore: incoming.length >= update.pageSize,
  }
}
