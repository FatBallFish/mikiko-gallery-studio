import type { ImageTaskType, ReferenceAsset } from '../../../shared/api-types'

export const galleryEditContextKey = 'pic-gallery-edit-context'

export type GalleryEditContext = {
  prompt: string
  sources: ReferenceAsset[]
  fallbackImageUrl?: string
  task_type?: ImageTaskType
  route_model_code?: string
  quality?: string
  aspect_ratio?: string
}

type GalleryEditContextInput = {
  prompt?: string | null
  sources?: ReferenceAsset[] | null
  fallbackImageUrl?: string | null
  taskType?: ImageTaskType | null
  task_type?: ImageTaskType | null
  routeModelCode?: string | null
  route_model_code?: string | null
  quality?: string | null
  aspectRatio?: string | null
  aspect_ratio?: string | null
}

function clean(value?: string | null) {
  const trimmed = value?.trim()
  return trimmed || undefined
}

export function createGalleryEditContext(input: GalleryEditContextInput): GalleryEditContext {
  return {
    prompt: input.prompt ?? '',
    sources: input.sources ?? [],
    fallbackImageUrl: clean(input.fallbackImageUrl),
    task_type: input.task_type ?? input.taskType ?? undefined,
    route_model_code: clean(input.route_model_code ?? input.routeModelCode),
    quality: clean(input.quality),
    aspect_ratio: clean(input.aspect_ratio ?? input.aspectRatio),
  }
}

export function parseGalleryEditContext(raw: string): GalleryEditContext | null {
  try {
    const parsed = JSON.parse(raw) as GalleryEditContextInput
    return createGalleryEditContext(parsed)
  } catch {
    return null
  }
}
