// Mirrors internal/domain/nativeimage/contracts.go. Coordinates in prompts are 0..999;
// layer bounding boxes retain both absolute pixels and normalized coordinates.
export type NativeImageOperation = 'generate' | 'edit' | 'layer_decomposition'
export type NativeImageAnnotation = { label?: string; reference_index?: number } & (
  | { point: [number, number]; bbox?: never }
  | { bbox: [number, number, number, number]; point?: never }
)
export type NativeImageOptions = {
  seed?: number
  negative_prompt?: string
  watermark?: boolean
  qwen?: { prompt_extend?: boolean; prompt_extend_mode?: 'direct' | 'agent'; enable_thinking?: boolean }
  seedream?: { optimize_prompt_mode?: 'standard' | 'fast' }
  annotations?: NativeImageAnnotation[]
}
// extra.native_image.defaults: omitted fields inherit account values, then platform defaults.
// Seed is Qwen-only (integer 0..2147483647); Seedream supports standard, or fast on Pro.
export type NativeImageDefaults = {
  seed?: number
  watermark?: boolean
  qwen?: { prompt_extend?: boolean; enable_thinking?: boolean; prompt_extend_mode?: 'direct' | 'agent' }
  seedream?: { optimize_prompt_mode?: 'standard' | 'fast' }
}

export type NativeImageRequestFields = {
  operation?: NativeImageOperation
  native_options?: NativeImageOptions
  negative_prompt?: string
  seed?: number
}
export type NativeImageLayer = {
  z_index: number
  name?: string
  description?: string
  bounding_box?: { absolute: [number, number, number, number]; normalized: [number, number, number, number] }
  is_base: boolean
}
export type NativeImageProfile = {
  version: string
  model: string
  adapter: 'qwen_image' | 'seedream_image'
  operations: NativeImageOperation[]
  min_pixels: number
  max_pixels: number
  max_aspect_ratio: number
  max_reference_images: number
  max_output_images: number
  input_formats?: string[]
  layer_size_modes?: string[]
  max_layers?: number
  output_formats: string[]
  size_modes: string[]
  negative_prompt: boolean
  seed: boolean
  layer_decomposition: boolean
  optimize_prompt_modes?: string[]
}
export const nativeImagePresets = [
  'qwen-image-3.0', 'qwen-image-2.1-pro', 'qwen-image-2.1-turbo', 'qwen-image-3.0-pro',
  'doubao-seedream-5-0-pro-260628', 'doubao-seedream-5-0-flash-260915',
] as const

export function nativeImageProfile(model: string): NativeImageProfile | undefined {
  if (!(nativeImagePresets as readonly string[]).includes(model)) return undefined
  const qwen = model.startsWith('qwen-')
  const third = model.startsWith('qwen-image-3.0')
  return {
    version: '2026-10-25', model, adapter: qwen ? 'qwen_image' : 'seedream_image',
    operations: qwen ? ['generate', 'edit'] : ['generate', 'edit', 'layer_decomposition'],
    min_pixels: qwen ? 262144 : 921600, max_pixels: qwen ? 4194304 : 4624220,
    max_aspect_ratio: qwen ? 8 : 16, max_reference_images: qwen && third ? 3 : 10,
    max_output_images: qwen ? 6 : 1, output_formats: qwen ? ['png'] : ['jpeg', 'png'],
    input_formats: qwen ? ['jpeg', 'png', 'bmp', 'tiff', 'webp', 'gif'] : ['jpeg', 'png', 'webp', 'bmp', 'tiff', 'gif', 'heic', 'heif'],
    ...(!qwen ? { layer_size_modes: ['auto', '1K', '1.5K', '2K'], max_layers: 16 } : {}),
    size_modes: qwen ? ['auto', 'dimensions'] : ['1K', '1.5K', '2K', 'dimensions'],
    negative_prompt: third, seed: qwen, layer_decomposition: !qwen,
    ...(!qwen ? { optimize_prompt_modes: model.includes('-pro-') ? ['standard', 'fast'] : ['standard'] } : {}),
  }
}
export function normalizeNativeImagePoint(x: number, y: number, width: number, height: number): [number, number] {
  if (width <= 0 || height <= 0) throw new Error('参考图尺寸无效')
  return [Math.round(Math.max(0, Math.min(1, x / width)) * 999), Math.round(Math.max(0, Math.min(1, y / height)) * 999)]
}
export function nativeImageFields(value: NativeImageRequestFields): NativeImageRequestFields {
  return {
    ...(value.operation ? { operation: value.operation } : {}),
    ...(value.native_options ? { native_options: value.native_options } : {}),
    ...(value.negative_prompt !== undefined ? { negative_prompt: value.negative_prompt } : {}),
    ...(value.seed !== undefined ? { seed: value.seed } : {}),
  }
}

export type NativeImagePricing = {
  currency: string; model: string; version: string; input_cny: number; free_input_images: number
  output_tiers: Array<{ name: string; max_pixels: number; cny: number }>
  layer_tiers?: Array<{ name: string; max_pixels: number; cny: number }>
  layer_base_cny?: number; layer_pricing_confirmed: boolean
}

export function nativeImageValidation(profile: NativeImageProfile | undefined, value: NativeImageRequestFields, referenceCount: number): string {
  if (!profile) return ''
  const operation = value.operation ?? (referenceCount ? 'edit' : 'generate')
  if (!profile.operations.includes(operation)) return '当前模型不支持此操作'
  if (operation === 'layer_decomposition' && referenceCount !== 1) return '图层拆分需要且仅支持一张参考图'
  if (operation === 'edit' && referenceCount < 1) return '图片编辑需要参考图'
  if (operation === 'generate' && referenceCount > 0) return '生成操作不接受参考图，请选择参考图编辑'
  if (referenceCount > profile.max_reference_images) return `参考图不能超过 ${profile.max_reference_images} 张`
  const options = value.native_options
  if (options?.seed !== undefined && (!profile.seed || !Number.isInteger(options.seed) || options.seed < 0 || options.seed > 2147483647)) return '随机种子需为 0..2147483647 的整数'
  if (options?.negative_prompt && !profile.negative_prompt) return '当前型号不支持负向提示词'
  if (options?.qwen && profile.adapter !== 'qwen_image' || options?.seedream && profile.adapter !== 'seedream_image') return '专属参数与当前模型不匹配，请重新配置'
  if (options?.qwen?.prompt_extend_mode === 'agent' && operation !== 'generate') return 'Agent 提示词优化仅支持文生图'
  return ''
}

// Changing the actual model invalidates every previously confirmed price.
export function nativeImagePresetConfig(previous: Record<string, unknown> | undefined, profile: NativeImageProfile): Record<string, unknown> {
  const oldProfile = previous?.profile as { model?: string } | undefined
  const sameModel = (previous?.model || oldProfile?.model) === profile.model
  const { pricing, sale_pricing, ...rest } = previous ?? {}
  return { ...rest, ...(sameModel ? { ...(pricing ? { pricing } : {}), ...(sale_pricing ? { sale_pricing } : {}) } : {}), model: profile.model, profile }
}
export function resolveNativeImageOperation(operation: NativeImageOperation | undefined, referenceCount: number): NativeImageOperation {
  return operation ?? (referenceCount ? 'edit' : 'generate')
}

export function nativeImageDimensions(profile: NativeImageProfile, widthText: string, heightText: string) {
  const width = Number(widthText), height = Number(heightText)
  const integers = /^\d+$/.test(widthText) && /^\d+$/.test(heightText) && Number.isSafeInteger(width) && Number.isSafeInteger(height) && width > 0 && height > 0
  const valid = integers && width * height >= profile.min_pixels && width * height <= profile.max_pixels && Math.max(width / height, height / width) <= profile.max_aspect_ratio
  return { valid, requestedWidth: width, requestedHeight: height, width: valid ? width : 0, height: valid ? height : 0, size: valid ? `${width}x${height}` : '', wasNormalized: false, error: valid ? '' : `宽高需为正整数，总像素 ${profile.min_pixels}–${profile.max_pixels}，宽高比不超过 ${profile.max_aspect_ratio}:1。` }
}
export function nativeWorkspaceModel<T extends { native_image_profile?: NativeImageProfile }>(model: T): T & { supports_custom_size?: boolean; size_modes?: string[]; pixel_sizes?: string[] } {
  const profile = model.native_image_profile
  if (!profile) return model
  return { ...model, supports_custom_size: profile.size_modes.includes('dimensions'), size_modes: [...(profile.size_modes.includes('auto') ? ['auto'] : []), ...(profile.size_modes.includes('dimensions') ? ['pixel'] : [])], pixel_sizes: ['1024x1024', '1536x1024', '1024x1536', '2048x2048'].filter((size) => { const [w, h] = size.split('x'); return nativeImageDimensions(profile, w, h).valid }) }
}
export function nativeImageCountLimit(profile: NativeImageProfile | undefined, legacyLimit: number) {
  return profile ? Math.max(1, Math.min(legacyLimit, profile.max_output_images)) : legacyLimit
}
const nativeFormatMIME: Record<string, string> = { jpeg: 'image/jpeg', png: 'image/png', bmp: 'image/bmp', tiff: 'image/tiff', webp: 'image/webp', gif: 'image/gif', heic: 'image/heic', heif: 'image/heif' }
export function nativeImageReferencePolicy(profile: NativeImageProfile, operation?: NativeImageOperation) {
  const allowedFormats = operation === 'layer_decomposition' ? ['jpeg', 'png'] : profile.input_formats ?? ['jpeg', 'png']
  return { maxBytes: (profile.adapter === 'qwen_image' ? 10 : 30) * 1024 * 1024, allowedFormats, allowedMIMETypes: allowedFormats.map((format) => nativeFormatMIME[format]).filter(Boolean) }
}
export function nativeImageReferenceAccept(profile: NativeImageProfile, operation?: NativeImageOperation) {
  const policy = nativeImageReferencePolicy(profile, operation)
  return [...policy.allowedMIMETypes, ...policy.allowedFormats.flatMap((format) => format === 'jpeg' ? ['.jpg', '.jpeg'] : format === 'tiff' ? ['.tif', '.tiff'] : [`.${format}`])].join(',')
}
export function nativeImageReferenceFileError(file: { name: string; type: string; size: number }, profile: NativeImageProfile, operation?: NativeImageOperation) {
  const policy = nativeImageReferencePolicy(profile, operation)
  if (file.size > policy.maxBytes) return `${file.name} 超过单张最大 ${policy.maxBytes / 1024 / 1024} MB。`
  const ext = file.name.toLowerCase().split('.').pop() ?? ''
  const format = ext === 'jpg' ? 'jpeg' : ext === 'tif' ? 'tiff' : ext
  if (!policy.allowedFormats.includes(format) || file.type && file.type.toLowerCase() !== nativeFormatMIME[format]) return `${file.name} 格式不受支持；允许格式：${policy.allowedFormats.join('、')}。`
  return ''
}
