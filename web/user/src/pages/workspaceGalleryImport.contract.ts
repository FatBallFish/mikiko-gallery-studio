import type { GalleryImage } from '../../../shared/api-types'
import { defaultGalleryImportFilter, filterGalleryImportImages, galleryImportOptions, mergeReferenceAssets } from './workspaceGalleryImport'

const images = [
  galleryImage({ id: '1', prompt: 'blue city skyline', route_model_code: 'plus', image_group: '城市', aspect_ratio: '16:9', visibility_status: 'private', url: '/1.png' }),
  galleryImage({ id: '2', prompt: 'green forest', abstract_model: 'pro', image_group: '自然', aspect_ratio: '1:1', visibility_status: 'public', download_url: '/2.png' }),
  galleryImage({ id: '3', prompt: 'red icon', route_model_code: 'plus', image_group: '图标', aspect_ratio: '1:1', visibility_status: 'private', url: '' }),
]

const searched = filterGalleryImportImages(images, { ...defaultGalleryImportFilter, query: 'blue plus' })
if (searched.length !== 1 || searched[0].id !== '1') {
  throw new Error(`gallery import search should match prompt and model terms, got ${JSON.stringify(searched)}`)
}

const filtered = filterGalleryImportImages(images, { ...defaultGalleryImportFilter, group: '自然', publishStatus: 'public', model: 'pro', ratio: '1:1' })
if (filtered.length !== 1 || filtered[0].id !== '2') {
  throw new Error(`gallery import filters should combine group/status/model/ratio, got ${JSON.stringify(filtered)}`)
}

if (filterGalleryImportImages(images, defaultGalleryImportFilter).some((item) => item.id === '3')) {
  throw new Error('gallery import should exclude images without preview or download URL')
}

const options = galleryImportOptions(images)
if (!options.groups.includes('城市') || !options.models.includes('plus') || !options.ratios.includes('16:9') || !options.publishStatuses.includes('public')) {
  throw new Error(`gallery import options should derive filter values, got ${JSON.stringify(options)}`)
}

const merged = mergeReferenceAssets([{ id: 'a' }, { id: 'b' }], [{ id: 'b' }, { id: 'c' }], 2)
if (JSON.stringify(merged.map((item) => item.id)) !== JSON.stringify(['b', 'c'])) {
  throw new Error(`reference merge should prefer incoming assets and respect limit, got ${JSON.stringify(merged)}`)
}

function galleryImage(patch: Partial<GalleryImage>): GalleryImage {
  return {
    id: patch.id ?? 'image-1',
    task_id: patch.task_id ?? 'task-1',
    prompt: patch.prompt,
    abstract_model: patch.abstract_model,
    route_model_code: patch.route_model_code,
    task_type: patch.task_type ?? 'text_to_image',
    quality: patch.quality ?? '2K',
    aspect_ratio: patch.aspect_ratio ?? '1:1',
    reference_asset_ids: patch.reference_asset_ids ?? [],
    reference_assets: patch.reference_assets ?? [],
    url: patch.url,
    download_url: patch.download_url,
    file_size_bytes: patch.file_size_bytes ?? 0,
    width: patch.width ?? 1024,
    height: patch.height ?? 1024,
    image_group: patch.image_group,
    visibility_status: patch.visibility_status ?? 'private',
    like_count: patch.like_count ?? 0,
    favorite_count: patch.favorite_count ?? 0,
    liked_by_viewer: patch.liked_by_viewer ?? false,
    favorited_by_viewer: patch.favorited_by_viewer ?? false,
    created_at: patch.created_at ?? '2026-06-05T00:00:00Z',
  }
}
