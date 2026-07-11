import type { ImageResult } from '../../../shared/api-types'
import { homeGalleryCardView } from './homeGalleryModel'

const guestImage: ImageResult = {
  id: 'img_anon',
  url: '/image.png',
  width: 512,
  height: 512,
  publish_status: 'approved',
  prompt: 'Full prompt should only appear after authenticated detail fetch',
  prompt_excerpt: 'A soft neon city…',
  route_model_code: 'plus',
  base_resolution: '2K',
  quality: 'auto',
  created_at: '2026-06-05T00:00:00Z',
}

const card = homeGalleryCardView(guestImage)

if (card.title !== 'A soft neon city…') {
  throw new Error(`home gallery card should prefer prompt excerpt for anonymous featured gallery, got ${card.title}`)
}

if (card.title.includes('Full prompt')) {
  throw new Error('home featured gallery card must not expose full prompt in list context')
}

if (!card.meta.includes('plus') || !card.meta.includes('2K')) {
  throw new Error(`home gallery card should keep model and base resolution metadata, got ${card.meta}`)
}

if (!card.meta.includes('2026/06/05 00:00')) {
  throw new Error(`home gallery card should format created_at as readable date time, got ${card.meta}`)
}

if (/T|Z$/.test(card.meta)) {
  throw new Error(`home gallery card metadata should not expose ISO separators, got ${card.meta}`)
}

const invalidDateCard = homeGalleryCardView({ ...guestImage, created_at: 'not-a-date' })
if (!invalidDateCard.meta.includes('not-a-date')) {
  throw new Error(`home gallery card should preserve invalid dates for troubleshooting, got ${invalidDateCard.meta}`)
}
