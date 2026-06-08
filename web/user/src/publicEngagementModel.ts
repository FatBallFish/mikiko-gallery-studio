import type { GalleryImage, ImageResult } from '../../shared/api-types'

type PublicEngagementImage = Pick<ImageResult | GalleryImage, 'like_count' | 'favorite_count'>

export function publicEngagementStats(image: PublicEngagementImage) {
  return [
    { key: 'likes' as const, label: '点赞', value: image.like_count ?? 0 },
    { key: 'favorites' as const, label: '收藏', value: image.favorite_count ?? 0 },
  ]
}

export function publicEngagementScore(image: PublicEngagementImage) {
  return (image.like_count ?? 0) * 2 + (image.favorite_count ?? 0) * 3
}
