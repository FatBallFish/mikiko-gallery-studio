import type { Capability, GalleryImage, MediaAsset, ReferenceAsset } from '../../../shared/api-types'
import { normalizeWorkspaceCreationDraft, workspaceCreationDraftFromSnapshot } from './workspaceCreationDraft'

export type GalleryImportFilter = {
  query: string
  group: string
  publishStatus: string
  model: string
  ratio: string
  source: 'all' | 'generated' | 'uploaded'
}

export const defaultGalleryImportFilter: GalleryImportFilter = {
  query: '',
  group: 'all',
  publishStatus: 'all',
  model: 'all',
  ratio: 'all',
  source: 'all',
}

// Uploads live in the unified media asset store while generated images come from
// the gallery list; the import modal merges both kinds into one selectable list.
export type GalleryImportItem = GalleryImage & { import_kind: 'generated' | 'uploaded' }

export function generatedImageAsGalleryItem(image: GalleryImage): GalleryImportItem {
  return { ...image, import_kind: 'generated' }
}

export function uploadedAssetAsGalleryItem(asset: MediaAsset): GalleryImportItem {
  return {
    id: asset.id,
    media_asset_id: asset.id,
    name: asset.name,
    task_id: '',
    prompt: asset.name,
    image_group: asset.group_name || '',
    visibility_status: asset.visibility_status === 'public' || asset.visibility_status === 'approved' ? 'public' : 'private',
    mime_type: asset.mime_type,
    file_size_bytes: asset.file_size_bytes,
    width: asset.width ?? 0,
    height: asset.height ?? 0,
    url: '',
    download_url: '',
    created_at: asset.created_at,
    import_kind: 'uploaded',
  }
}

export function splitGalleryImportSelection(ids: string[], items: GalleryImportItem[]) {
  const galleryIds: string[] = []
  const uploadIds: string[] = []
  for (const id of ids) {
    if (items.some((item) => item.id === id && item.import_kind === 'uploaded')) uploadIds.push(id)
    else galleryIds.push(id)
  }
  return { galleryIds, uploadIds }
}

export function filterGalleryImportImages(images: Array<GalleryImage & { import_kind?: 'generated' | 'uploaded' }>, filter: GalleryImportFilter): Array<GalleryImage & { import_kind?: 'generated' | 'uploaded' }> {
  const queryTerms = filter.query.trim().toLowerCase().split(/\s+/).filter(Boolean)
  return images.filter((image) => {
    const model = image.route_model_code || image.abstract_model || ''
    const uploaded = image.import_kind === 'uploaded'
    const haystack = [
      image.prompt,
      model,
      image.image_group,
      image.aspect_ratio,
      image.visibility_status,
      uploaded ? '本地上传' : '',
    ].filter(Boolean).join(' ').toLowerCase()

    return (
      (uploaded || Boolean(image.url || image.download_url))
      && (filter.source === 'all' || (filter.source === 'uploaded') === uploaded)
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
  for (const item of [...current, ...incoming]) {
    if (!item.id || byId.has(item.id)) continue
    byId.set(item.id, item)
  }
  const merged = Array.from(byId.values())
  return merged.length > limit ? current : merged
}

export function firstGalleryReferenceReuse(currentReferenceCount: number, imported: ReferenceAsset[], capability: Capability) {
  if (currentReferenceCount !== 0 || !imported.length || !imported[0].generation_snapshot) return null
  const snapshot = imported[0].generation_snapshot
  const draft = workspaceCreationDraftFromSnapshot({
    ...snapshot,
    requested_output_image_count: snapshot.image_count,
  })
  draft.reference_asset_ids = [imported[0].id]
  return normalizeWorkspaceCreationDraft(draft, capability)
}

export function galleryImportSuccessMessage(importedCount: number, notices: string[] = []) {
  const message = `已从资产导入 ${importedCount} 张参考图`
  const uniqueNotices = Array.from(new Set(notices.map((notice) => notice.trim()).filter(Boolean)))
  return uniqueNotices.length ? `${message}；${uniqueNotices.join(' ')}` : message
}

function uniqueValues(values: string[]) {
  return Array.from(new Set(values.map((value) => value.trim()).filter(Boolean))).sort((a, b) => a.localeCompare(b))
}
