import type { ImageResult } from '../../../shared/api-types'
import { publicGalleryCardView, publicGallerySearchText, shouldFetchPublicGalleryDetailByID } from './publicGalleryModel'

const guestImage: ImageResult = {
  id: 'img_1',
  url: '/image.png',
  width: 512,
  height: 512,
  publish_status: 'approved',
  prompt: 'This full prompt must stay hidden from list cards and search',
  prompt_excerpt: 'A cinematic poster…',
  route_model_code: 'plus',
  base_resolution: '2K',
  quality: 'auto',
  aspect_ratio: '16:9',
  created_at: '2026-06-05T13:45:30Z',
}

const view = publicGalleryCardView(guestImage)

if (view.title !== 'A cinematic poster…') {
  throw new Error('public gallery card should prefer prompt excerpt for guest list')
}

if (view.title.includes('full prompt')) {
  throw new Error('public gallery list card must not expose full prompt even if it is present on the object')
}

if (view.model !== 'plus' || view.baseResolution !== '2K' || view.aspectRatio !== '16:9') {
  throw new Error('public gallery card should expose model, base resolution and aspect ratio')
}

if (view.date !== '2026/06/05') {
  throw new Error(`public gallery card should format created_at as a readable date, got ${view.date}`)
}

if (/T|Z$/.test(view.date)) {
  throw new Error(`public gallery card date should not expose ISO separators, got ${view.date}`)
}

const invalidDateView = publicGalleryCardView({ ...guestImage, created_at: 'not-a-date' })
if (invalidDateView.date !== 'not-a-date') {
  throw new Error(`public gallery card should preserve invalid dates for troubleshooting, got ${invalidDateView.date}`)
}

const searchText = publicGallerySearchText(guestImage)
if (searchText.includes('full prompt') || !searchText.includes('cinematic poster')) {
  throw new Error(`public gallery list search must use prompt excerpt, got ${searchText}`)
}

if (!shouldFetchPublicGalleryDetailByID({ imageId: 'img_missing', rows: [{ id: 'img_1' }], selectedId: null, busyId: null })) {
  throw new Error('public gallery deep link should fetch detail directly when target image is not in the loaded list')
}

if (shouldFetchPublicGalleryDetailByID({ imageId: 'img_1', rows: [{ id: 'img_1' }], selectedId: null, busyId: null })) {
  throw new Error('public gallery deep link should not bypass the loaded row when target image is already in list')
}

if (shouldFetchPublicGalleryDetailByID({ imageId: 'img_2', rows: [], selectedId: 'img_2', busyId: null })) {
  throw new Error('public gallery deep link should not refetch the currently selected image')
}
