import type { GalleryImage } from '../../../shared/api-types'
import { defaultGalleryImportFilter, filterGalleryImportImages, firstGalleryReferenceReuse, galleryImportOptions, galleryImportSuccessMessage, mergeReferenceAssets } from './workspaceGalleryImport'

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

const reuseCapability = {
  task_types: ['text_to_image', 'image_edit'],
  model_groups: [{ id: 'plus', code: 'plus', name: 'Plus', task_types: ['image_edit'], size_modes: ['auto', 'ratio', 'pixel'], base_resolution: ['1K'], aspect_ratios: ['1:1'], pixel_sizes: ['1024x1024'], quality: ['auto'], output_format: ['png', 'webp'], supported_backgrounds: ['opaque', 'transparent'], moderation: ['auto'], supports_reference: true, max_reference_image_count: 4, max_output_image_count: 1 }],
  base_resolution: ['1K'], aspect_ratios: ['1:1'], quality: ['auto'], output_format: ['png', 'webp'], moderation: ['auto'],
} as any
const imported = [{ id: 'ref-first', status: 'ready', created_at: '', generation_snapshot: { task_type: 'image_edit', route_model_code: 'plus', size_mode: 'pixel', requested_size: '9999x9999', quality: 'unsupported', output_format: 'jpeg', background: 'transparent', moderation: 'unsupported', image_count: 6 } }]
const firstReuse = firstGalleryReferenceReuse(0, imported as any, reuseCapability)
if (!firstReuse || firstReuse.values.route_model_code !== 'plus' || firstReuse.values.pixel_size !== '1024x1024' || firstReuse.values.output_format !== 'png' || firstReuse.values.background !== 'transparent' || firstReuse.values.image_count !== 6) {
  throw new Error(`first gallery reference must capability-normalize source parameters without applying model max n: ${JSON.stringify(firstReuse)}`)
}
if (!firstReuse.notices.length) {
  throw new Error(`unsupported source parameters must produce user notices: ${JSON.stringify(firstReuse)}`)
}
const importMessage = galleryImportSuccessMessage(1, [...firstReuse.notices, firstReuse.notices[0]])
if (!importMessage.startsWith('已从资产导入 1 张参考图') || !firstReuse.notices.every((notice: string) => importMessage.includes(notice)) || importMessage.split(firstReuse.notices[0]).length !== 2) {
  throw new Error(`gallery import success must include deduplicated normalization notices once: ${importMessage}`)
}
if (firstGalleryReferenceReuse(1, imported as any, reuseCapability) !== null || firstGalleryReferenceReuse(0, [{ id: 'plain' }] as any, reuseCapability) !== null) {
  throw new Error('later references and imports without a generation snapshot must not overwrite parameters')
}

function galleryImage(patch: Partial<GalleryImage>): GalleryImage {
  return {
    id: patch.id ?? 'image-1',
    task_id: patch.task_id ?? 'task-1',
    prompt: patch.prompt,
    abstract_model: patch.abstract_model,
    route_model_code: patch.route_model_code,
    task_type: patch.task_type ?? 'text_to_image',
    base_resolution: patch.base_resolution ?? '2K',
    quality: patch.quality ?? 'auto',
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
