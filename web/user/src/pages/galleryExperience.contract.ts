import { readFileSync } from 'node:fs'
import { galleryImageAspect, selectVisibleGalleryImages, toggleGalleryImageSelection } from './galleryExperience'

const galleryModel = await import('./galleryExperience') as unknown as Record<string, unknown>
type GalleryRow = { id: string }
type VisibleSelection = <T extends GalleryRow>(rows: T[], selectedIds: ReadonlySet<string>) => T[]
type AllVisibleSelection = (rows: GalleryRow[], selectedIds: ReadonlySet<string>) => boolean
const selectedVisibleGalleryItems = galleryModel.selectedVisibleGalleryItems as VisibleSelection | undefined
const areAllVisibleGalleryItemsSelected = galleryModel.areAllVisibleGalleryItemsSelected as AllVisibleSelection | undefined

if (!selectedVisibleGalleryItems || !areAllVisibleGalleryItemsSelected) {
  throw new Error('gallery batch actions need executable visible-selection helpers')
}

const componentsSource = readFileSync(new URL('../components.tsx', import.meta.url), 'utf8')
const homeSource = readFileSync(new URL('./HomePage.tsx', import.meta.url), 'utf8')
const privateGallerySource = readFileSync(new URL('./GalleryPage.tsx', import.meta.url), 'utf8')
const publicGallerySource = readFileSync(new URL('./PublicGalleryPage.tsx', import.meta.url), 'utf8')

if (galleryImageAspect({ width: 1600, height: 900 }) !== '1600 / 900') {
  throw new Error('gallery image geometry should prefer concrete image dimensions')
}

if (galleryImageAspect({ width: 0, height: 0, aspectRatio: '3:4' }) !== '3 / 4') {
  throw new Error('gallery image geometry should use a valid API aspect ratio when dimensions are missing')
}

if (galleryImageAspect({ aspectRatio: 'unexpected' }) !== '4 / 3') {
  throw new Error('gallery image geometry should have a stable fallback')
}

const selected = toggleGalleryImageSelection(new Set(['image_1']), 'image_2')
if (!selected.has('image_1') || !selected.has('image_2')) {
  throw new Error('gallery keyboard and pointer selection should share additive toggle behavior')
}

const unselected = toggleGalleryImageSelection(selected, 'image_1')
if (unselected.has('image_1') || !unselected.has('image_2')) {
  throw new Error('gallery selection toggle should remove an already selected image')
}

const allVisible = selectVisibleGalleryImages(new Set(['hidden']), ['visible_1', 'visible_2'], true)
if (!allVisible.has('hidden') || !allVisible.has('visible_1') || !allVisible.has('visible_2')) {
  throw new Error('select-all should preserve hidden selection while selecting the current filtered gallery')
}

const cleared = selectVisibleGalleryImages(allVisible, ['visible_1', 'visible_2'], false)
if (cleared.size !== 1 || !cleared.has('hidden')) {
  throw new Error('clearing visible selection must not discard selection from another filter')
}

const filteredRows = [{ id: 'visible_1' }, { id: 'visible_2' }]
const selectionWithHidden = new Set(['hidden', 'visible_1'])
const visibleBatchItems = selectedVisibleGalleryItems(filteredRows, selectionWithHidden)
if (visibleBatchItems.length !== 1 || visibleBatchItems[0]?.id !== 'visible_1') {
  throw new Error('batch targets must be the strict intersection of filtered rows and selected ids')
}
if (areAllVisibleGalleryItemsSelected(filteredRows, selectionWithHidden)) {
  throw new Error('equal selected and filtered counts must not imply all visible rows are selected')
}
if (!areAllVisibleGalleryItemsSelected(filteredRows, new Set(['hidden', 'visible_1', 'visible_2']))) {
  throw new Error('all-visible-selected must require every filtered id while tolerating hidden selection')
}
if (selectedVisibleGalleryItems(filteredRows, new Set(['hidden'])).length !== 0) {
  throw new Error('a filter with no visible selected items must expose no batch targets')
}

for (const sharedPrimitive of ['export function GalleryFilterToolbar', 'export function GalleryImageFrame']) {
  if (!componentsSource.includes(sharedPrimitive)) {
    throw new Error(`gallery pages need shared primitive: ${sharedPrimitive}`)
  }
}

for (const imageAttribute of ['loading="lazy"', 'decoding="async"', 'onError=']) {
  if (!componentsSource.includes(imageAttribute)) {
    throw new Error(`shared gallery images need ${imageAttribute}`)
  }
}

for (const cachedImageGuard of ['imageRef.current', 'image.complete', 'image.naturalWidth > 0']) {
  if (!componentsSource.includes(cachedImageGuard)) {
    throw new Error(`shared gallery images must reveal cached images via ${cachedImageGuard}`)
  }
}

if (!componentsSource.includes('function DetailImageMedia') || !componentsSource.includes('<DetailImageMedia')) {
  throw new Error('shared image detail must replace broken media with a local error state')
}

if (!componentsSource.includes('max-[620px]:basis-full')) {
  throw new Error('shared gallery toolbar must give search and filters full mobile rows')
}

for (const [name, source] of [['private gallery', privateGallerySource], ['public gallery', publicGallerySource]] as const) {
  if (!source.includes('<GalleryFilterToolbar') || !source.includes('<GalleryImageFrame')) {
    throw new Error(`${name} must use the shared filter toolbar and image frame`)
  }
}

if (homeSource.includes('heroImage') || homeSource.includes('高奢视觉生成工坊') || homeSource.includes('openDocsEntry')) {
  throw new Error('authenticated home must prioritize continuation instead of a marketing hero or documentation')
}

for (const homeFeature of ['homeContinuationView', 'homeRecentTaskView', 'curatedHomeGallery', '<GalleryImageFrame']) {
  if (!homeSource.includes(homeFeature)) throw new Error(`home must render ${homeFeature}`)
}

if (!publicGallerySource.includes('IntersectionObserver') || !publicGallerySource.includes('loadMoreRef')) {
  throw new Error('public gallery needs discoverable infinite-loading behavior')
}

if (!publicGallerySource.includes(': loadError ? null : hasMore')) {
  throw new Error('public gallery must not invite infinite loading while the first page is in an error state')
}

if (!privateGallerySource.includes("filterToolbar: 'mb-8'")) {
  throw new Error('private gallery needs 32px of separation after its filter toolbar')
}

for (const selectionControlContract of [
  "assetSelectHitArea: 'group/select grid size-10",
  'assetSelectVisual:',
  'size-[22px]',
	'opacity-80',
  'group-hover/asset:opacity-100',
  'group-hover/select:opacity-100',
  'group-focus-visible/select:opacity-100',
  'aria-pressed={selectedIds.has(image.id)}',
  '<Check',
]) {
  if (!privateGallerySource.includes(selectionControlContract)) {
    throw new Error(`private gallery selection control needs ${selectionControlContract}`)
  }
}

if (!privateGallerySource.includes("card: 'group/asset")) {
  throw new Error('private gallery selection control must reveal from the whole asset card hover state')
}

if (!privateGallerySource.includes("selectedIds.has(image.id) && galleryClasses.assetSelectVisualSelected")) {
  throw new Error('selected gallery assets must keep the compact checkbox visible')
}

for (const visibleBatchContract of [
  'selectedVisibleGalleryItems(filtered, selectedIds)',
  'areAllVisibleGalleryItemsSelected(filtered, selectedIds)',
]) {
  if (!privateGallerySource.includes(visibleBatchContract)) {
    throw new Error(`private gallery batch actions must wire ${visibleBatchContract}`)
  }
}
