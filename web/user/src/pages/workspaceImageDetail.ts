import type { ImageResult, ImageTask, UserProfile } from '../../../shared/api-types'

function firstText(...values: Array<string | null | undefined>) {
  return values.find((value) => typeof value === 'string' && value.trim())?.trim()
}

export function projectWorkspaceImageDetail(
  image: ImageResult,
  task: ImageTask,
  profile?: Pick<UserProfile, 'display_name'> | null,
): ImageResult {
  return {
    ...image,
    prompt: firstText(image.prompt, task.prompt),
    task_type: image.task_type ?? task.task_type,
    size_mode: firstText(image.size_mode, task.size_mode),
    requested_size: firstText(image.requested_size, task.requested_size),
    base_resolution: firstText(image.base_resolution, task.base_resolution),
    quality: firstText(image.quality, task.quality, task.requested_quality),
    aspect_ratio: firstText(image.aspect_ratio, task.aspect_ratio),
    output_format: firstText(image.output_format, task.output_format),
    output_compression: image.output_compression ?? task.output_compression,
    moderation: firstText(image.moderation, task.moderation),
    requested_output_image_count: image.requested_output_image_count ?? task.requested_output_image_count ?? task.image_count,
    image_count: image.image_count ?? task.image_count,
    reference_asset_ids: image.reference_asset_ids?.length ? image.reference_asset_ids : task.reference_asset_ids,
    route_model_code: firstText(image.route_model_code, task.route_model_code, task.model_group),
    abstract_model: firstText(image.abstract_model, task.abstract_model),
    author_name: firstText(image.author_name, profile?.display_name),
    created_at: firstText(image.created_at, task.created_at),
  }
}
