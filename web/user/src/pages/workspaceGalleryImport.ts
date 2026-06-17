import type { GalleryImage } from '../../../shared/api-types'

export type GalleryImportFilter = {
  query: string
  group: string
  publishStatus: string
  model: string
  ratio: string
}

export const defaultGalleryImportFilter: GalleryImportFilter = {
  query: '',
  group: 'all',
  publishStatus: 'all',
  model: 'all',
  ratio: 'all',
}

export function filterGalleryImportImages(images: GalleryImage[], filter: GalleryImportFilter): GalleryImage[] {
  const queryTerms = filter.query.trim().toLowerCase().split(/\s+/).filter(Boolean)
  return images.filter((image) => {
    const model = image.route_model_code || image.abstract_model || ''
    const haystack = [
      image.prompt,
      model,
      image.image_group,
      image.aspect_ratio,
      image.visibility_status,
    ].filter(Boolean).join(' ').toLowerCase()

    return (
      Boolean(image.asset_url || image.download_url || image.url)
      && queryTerms.every((term) => haystack.includes(term))
      && (filter.group === 'all' || (image.image_group || '') === filter.group)
      && (filter.publishStatus === 'all' || image.visibility_status === filter.publishStatus)
      && (filter.model === 'all' || model === filter.model)
      && (filter.ratio === 'all' || image.aspect_ratio === filter.ratio)
    )
  })
}

export function galleryImportOptions(images: GalleryImage[]) {
  const groups = uniqueValues(images.map((item) => item.image_group || ''))
  const models = uniqueValues(images.map((item) => item.route_model_code || item.abstract_model || ''))
  const ratios = uniqueValues(images.map((item) => item.aspect_ratio || ''))
  const publishStatuses = uniqueValues(images.map((item) => item.visibility_status || ''))
  return { groups, models, ratios, publishStatuses }
}

export function mergeReferenceAssets<T extends { id: string }>(current: T[], incoming: T[], limit: number) {
  const byId = new Map<string, T>()
  for (const item of [...incoming, ...current]) {
    if (!item.id || byId.has(item.id)) continue
    byId.set(item.id, item)
  }
  return Array.from(byId.values()).slice(0, Math.max(0, limit))
}

function uniqueValues(values: string[]) {
  return Array.from(new Set(values.map((value) => value.trim()).filter(Boolean))).sort((a, b) => a.localeCompare(b))
}
