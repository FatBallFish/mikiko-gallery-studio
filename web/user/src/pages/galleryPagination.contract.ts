import { existsSync, readFileSync } from 'node:fs'

const modelURL = new URL('./galleryPagination.ts', import.meta.url)
if (!existsSync(modelURL)) {
  throw new Error('private gallery needs an executable paginated collection model')
}

const { applyGalleryPage, galleryLoadingForReload, initialGalleryPageState, patchGalleryPageItems, removeGalleryPageItems } = await import('./galleryPagination')

type Asset = { id: string; label: string }
const assets = (start: number, count: number): Asset[] => Array.from(
  { length: count },
  (_, index) => ({ id: `asset-${start + index}`, label: `Asset ${start + index}` }),
)

let state = applyGalleryPage(initialGalleryPageState<Asset>(), assets(1, 50), { page: 1, pageSize: 50, mode: 'replace' })
state = applyGalleryPage(state, assets(50, 50), { page: 2, pageSize: 50, mode: 'append' })
state = applyGalleryPage(state, assets(100, 21), { page: 3, pageSize: 50, mode: 'append' })

if (state.items.length !== 120 || new Set(state.items.map((item: Asset) => item.id)).size !== 120) {
  throw new Error(`private gallery must de-duplicate more than 100 assets across pages, got ${state.items.length}`)
}
if (state.page !== 3 || state.hasMore) {
  throw new Error(`short final page must advance page and stop pagination, got page=${state.page} hasMore=${state.hasMore}`)
}

state = applyGalleryPage(state, [{ id: 'replacement', label: 'Replacement' }], { page: 1, pageSize: 50, mode: 'replace' })
if (state.items.length !== 1 || state.items[0]?.id !== 'replacement' || state.page !== 1 || state.hasMore) {
  throw new Error(`replace mode must reset stale pages, got ${JSON.stringify(state)}`)
}

const reloadFlags = galleryLoadingForReload()
if (!reloadFlags.loading || reloadFlags.loadingMore) {
  throw new Error(`reload must take ownership and clear stale append loading, got ${JSON.stringify(reloadFlags)}`)
}

const localPage = applyGalleryPage(initialGalleryPageState<Asset>(), assets(1, 3), { page: 1, pageSize: 50, mode: 'replace' })
const patchedPage = patchGalleryPageItems(localPage, [{ id: 'asset-2', label: 'Updated' }])
if (patchedPage === localPage || patchedPage.page !== localPage.page || patchedPage.hasMore !== localPage.hasMore) {
  throw new Error('local patch must preserve pagination metadata while replacing page state')
}
if (patchedPage.items[0] !== localPage.items[0] || patchedPage.items[2] !== localPage.items[2] || patchedPage.items[1]?.label !== 'Updated') {
  throw new Error('local page patch must preserve untouched image identity')
}
const removedPage = removeGalleryPageItems(patchedPage, new Set(['asset-2']))
if (removedPage.items.length !== 2 || removedPage.items[0] !== localPage.items[0] || removedPage.items[1] !== localPage.items[2]) {
  throw new Error('local page removal must remove only successful IDs')
}

const pageSource = readFileSync(new URL('./GalleryPage.tsx', import.meta.url), 'utf8')
for (const contract of [
  'userApi.listGalleryImages(pageNumber, GALLERY_PAGE_SIZE)',
  'loadingMore',
  'hasMore',
  'loadMoreRef',
  'IntersectionObserver',
  '加载更多资产',
  'setLoadingMore(reloadFlags.loadingMore)',
  'patchGalleryPageItems',
  'removeGalleryPageItems',
  'publishConfirm',
  'PublishActionIcon',
]) {
  if (!pageSource.includes(contract)) {
    throw new Error(`private gallery pagination must expose ${contract}`)
  }
}

if (pageSource.includes('await reloadLoadedPages()')) {
  throw new Error('gallery mutations must patch local state instead of reloading every loaded page')
}

if (pageSource.includes('var(--accent-amber)')) {
  throw new Error('gallery warning actions must use the defined --pg-accent-amber theme token')
}

if (!pageSource.includes('type="button"') || !pageSource.includes('aria-label="加载更多资产"')) {
  throw new Error('private gallery sentinel needs an accessible explicit load-more fallback')
}
