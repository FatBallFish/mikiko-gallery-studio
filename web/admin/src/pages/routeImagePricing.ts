import type { ImagePricingConfig, ImagePricingOperation, ImagePricingRule, ImagePricingTier, ModelAccount, ModelAccountModel, RouteModelCandidate, RouteModelPrice } from '../../../shared/api-types'
import { configValue, effectiveNativeProfile, nativeLayerCostMissing } from './nativeImageConfig'

export const imageOperationLabels: Record<ImagePricingOperation, string> = { generate: '图片生成', edit: '参考图编辑', layer_decomposition: '图层拆分' }
// Same declared ceiling as modeladmin.genericCandidateMaxPixels; native labels use profile pixels instead.
export function legacyCandidateMaxPixels(model: ModelAccountModel): number {
  const budgets: Record<string, number> = { '1k': 1024 ** 2, '2k': 2048 ** 2, '4k': 4096 ** 2 }
  const limits = (model.base_resolution ?? []).map((mode) => budgets[mode.toLowerCase()] ?? 0)
  for (const size of model.supported_pixel_sizes ?? []) {
    const match = /^(\d+)[xX](\d+)$/.exec(size)
    if (match) limits.push(Number(match[1]) * Number(match[2]))
  }
  if (model.supports_custom_size) limits.push(Number(model.max_width) * Number(model.max_height))
  return Math.max(0, ...limits.filter((limit) => Number.isSafeInteger(limit) && limit > 0))
}
export function confirmPricingTransition(dirty: boolean, confirm: (message: string) => boolean, message: string): boolean {
  return !dirty || confirm(message)
}
export function routeImageCapabilities(candidates: RouteModelCandidate[], models: ModelAccountModel[], accounts: ModelAccount[]) {
  const active = candidates.flatMap((candidate) => {
    const model = models.find((item) => String(item.id) === String(candidate.account_model_id))
    const account = accounts.find((item) => String(item.id) === String(model?.account_id))
    if (!candidate.enabled || !model?.enabled || account?.status !== 'enabled') return []
    const profile = effectiveNativeProfile(model.extra, account.extra, model.model_code)
    const native = ['qwen_image', 'seedream_image'].includes(account.adapter_type)
    const operations: ImagePricingOperation[] = profile ? profile.operations.filter((operation) => operation !== 'layer_decomposition' || profile.layer_decomposition) : native ? [] : [
      ...(model.task_types.includes('text_to_image') ? ['generate' as const] : []), ...(model.task_types.includes('image_edit') ? ['edit' as const] : []),
    ]
    return [{ model, profile, native, operations, resolutions: profile ? profile.size_modes.filter((mode) => /^\d+(\.\d+)?K$/i.test(mode)) : model.base_resolution.filter((mode) => mode.toLowerCase() !== 'auto') }]
  })
  const operations = (['generate', 'edit', 'layer_decomposition'] as const).filter((operation) => active.length > 0 && active.every((item) => item.operations.includes(operation)))
  const resolutions = active.length ? active[0].resolutions.filter((resolution) => active.every((item) => item.resolutions.includes(resolution))) : []
  const profiles = active.flatMap((item) => item.profile ? [item.profile] : [])
  const ceilings = active.map((item) => item.profile?.max_pixels ?? legacyCandidateMaxPixels(item.model))
  const maxPixels = ceilings.length ? Math.min(...ceilings) : undefined
  const minPixels = profiles.length ? Math.max(...profiles.map((profile) => profile.min_pixels)) : undefined
  const warnings = active.flatMap((item) => {
    const excluded = item.operations.filter((operation) => !operations.includes(operation))
    return [ ...(item.native && !item.profile ? [`${item.model.model_code} 缺少有效能力 profile，无法配置售价。`] : []), ...(excluded.length ? [`${item.model.model_code} 的 ${excluded.map((op) => imageOperationLabels[op]).join('、')} 与其他候选不兼容，路由不开放这些操作。`] : []) ]
  })
  if (minPixels !== undefined && maxPixels !== undefined && minPixels > maxPixels) warnings.push('候选总像素范围无交集，请调整候选。')
  const layerCostMissing = active.flatMap((item) => {
    const account = accounts.find((account) => String(account.id) === String(item.model.account_id))
    const local = configValue(item.model.extra, ['native_image', 'pricing'])
    return nativeLayerCostMissing(local === undefined ? configValue(account?.extra, ['native_image', 'pricing']) : local, item.profile).map((missing) => `${item.model.model_code}：${missing}`)
  })
  const layerSizeModes = profiles.length ? (profiles[0].layer_size_modes ?? []).filter((mode) => profiles.every((profile) => profile.layer_size_modes?.includes(mode))) : []
  const maxLayers = profiles.length && profiles.every((profile) => Number.isInteger(profile.max_layers)) ? Math.min(...profiles.map((profile) => profile.max_layers!)) : undefined
  return { active, operations, resolutions, minPixels, maxPixels, warnings, layerCostMissing, layerSizeModes, maxLayers, hasNative: active.some((item) => item.native) }
}
export type RouteImageCapabilities = ReturnType<typeof routeImageCapabilities>
export const decimalRate = (value: unknown): value is string => typeof value === 'string' && value.length <= 128 && /^\d+(?:\.\d+)?$/.test(value)
export function newRouteImageRule(operation: ImagePricingOperation): ImagePricingRule {
  // Empty fields require a deliberate value; never invent a free rate or confirmation.
  return { operation, mode: 'per_image', currency: 'CNY', version: '', input: { free_images: Number.NaN, per_image: '' }, ...(operation === 'layer_decomposition' ? { layers: { confirmed: false, tiers: [] } } : { output_tiers: [] }) }
}
export function routeTierErrors(tiers: ImagePricingTier[] | undefined, capability: RouteImageCapabilities, label: string, layer = false): string[] {
  if (!tiers?.length) return [`${label}缺少档位。`]
  const errors: string[] = [], names = new Set<string>(), resolutions = new Set<string>()
  let previous = 0
  const pixelKind = tiers[0].max_pixels !== undefined
  if (layer && capability.layerSizeModes.includes('auto') && !pixelKind) errors.push(`${label} auto 按实际尺寸计价，必须用像素阶梯覆盖最大 ${capability.maxPixels} 像素，不能配置 auto 固定价格。`)
  for (const tier of tiers) {
    if (!tier.name.trim() || names.has(tier.name)) errors.push(`${label}档名须非空且唯一。`)
    names.add(tier.name)
    if (!decimalRate(tier.price)) errors.push(`${label}单价需为非负十进制数（0 须明确填写）。`)
    if (tier.max_pixels !== undefined) {
      if (!pixelKind || tier.base_resolution !== undefined || !Number.isSafeInteger(tier.max_pixels) || tier.max_pixels <= previous) errors.push(`${label}像素上限须为严格递增正整数，不能混用基础分辨率。`)
      if (capability.maxPixels !== undefined && tier.max_pixels > capability.maxPixels) errors.push(`${label}超过有效候选最大 ${capability.maxPixels} 像素。`)
      previous = tier.max_pixels
    } else {
      const allowed = layer ? capability.layerSizeModes.filter((mode) => mode !== 'auto') : capability.resolutions
      if (pixelKind || !tier.base_resolution || resolutions.has(tier.base_resolution) || !allowed.includes(tier.base_resolution)) errors.push(`${label}${tier.base_resolution ?? '未选择'} 不受所有有效候选支持或重复。`)
      resolutions.add(tier.base_resolution ?? '')
    }
  }
  if (pixelKind && capability.maxPixels !== undefined && previous < capability.maxPixels) errors.push(`${label}末档需覆盖有效候选最大 ${capability.maxPixels} 像素。`)
  if (!pixelKind) for (const resolution of (layer ? capability.layerSizeModes.filter((mode) => mode !== 'auto') : capability.resolutions)) if (!resolutions.has(resolution)) errors.push(`${label}缺少 ${resolution} 售价。`)
  return [...new Set(errors)]
}
export function routeImagePricingErrors(config: ImagePricingConfig | null | undefined, capability: RouteImageCapabilities): string[] {
  if (!config) return ['尚未配置统一路由图片售价。']
  const errors: string[] = []
  if (capability.active.length && !capability.operations.length) errors.push('有效候选没有共同图片操作。')
  if (capability.minPixels !== undefined && capability.maxPixels !== undefined && capability.minPixels > capability.maxPixels) errors.push('有效候选总像素范围无交集。')
  if (config.schema_version !== 1 || !config.rules.length) errors.push('图片售价需要 schema_version 1 和至少一条规则。')
  const seen = new Set<string>()
  for (const rule of config.rules) {
    const label = imageOperationLabels[rule.operation]
    if (seen.has(rule.operation)) errors.push(`${label}规则重复。`)
    seen.add(rule.operation)
    if (capability.active.length && !capability.operations.includes(rule.operation)) errors.push(`${label}不受所有有效候选支持。`)
    if (!rule.version.trim()) errors.push(`${label}缺少价格版本。`)
    if (!['CNY', 'points'].includes(rule.currency)) errors.push(`${label}币种无效。`)
    if (!Number.isSafeInteger(rule.input.free_images) || rule.input.free_images < 0) errors.push(`${label}免费前 N 张需明确填写非负整数。`)
    if (rule.mode === 'pixel_token') {
      const pixel = rule.pixel_token
      if (rule.operation === 'layer_decomposition') errors.push('拆层初期仅支持按张计费。')
      if (rule.input.per_image !== undefined || rule.output_tiers?.length || rule.layers) errors.push(`${label}像素 Token 不可混用按张价格。`)
      if (!pixel || pixel.definition !== 'platform_pixels_v1' || !Number.isSafeInteger(pixel.pixels_per_token) || pixel.pixels_per_token <= 0 || !decimalRate(pixel.input_per_million) || !decimalRate(pixel.output_per_million)) errors.push(`${label}缺少平台像素 Token 换算或输入 / 输出每百万费率。`)
    } else if (rule.mode === 'per_image') {
      if (!decimalRate(rule.input.per_image)) errors.push(`${label}缺少输入每张单价。`)
      if (rule.pixel_token) errors.push(`${label}按张不可混用像素 Token。`)
      if (rule.operation === 'layer_decomposition') {
        if (rule.output_tiers?.length) errors.push('拆层只使用图层阶梯。')
        if (!rule.layers?.confirmed) errors.push('拆层售价口径尚未确认。')
        if (!decimalRate(rule.layers?.base_price)) errors.push('拆层缺少底图售价。')
        errors.push(...routeTierErrors(rule.layers?.tiers, capability, '拆层', true))
      } else errors.push(...routeTierErrors(rule.output_tiers, capability, label))
    } else errors.push(`${label}计价模式无效。`)
  }
  return [...new Set(errors)]
}
export function legacyRoutePriceErrors(prices: Pick<RouteModelPrice, 'enabled' | 'task_type' | 'base_resolution' | 'base_points'>[], capability: RouteImageCapabilities) {
  if (capability.hasNative) return ['原生图片候选需要统一路由 image_pricing，不使用旧账号售价。']
  const errors: string[] = []
  for (const operation of capability.operations.filter((op) => op !== 'layer_decomposition')) for (const resolution of capability.resolutions) {
    if (!prices.some((price) => price.enabled && price.task_type === (operation === 'generate' ? 'text_to_image' : 'image_edit') && price.base_resolution === resolution && decimalRate(price.base_points))) errors.push(`缺少${imageOperationLabels[operation]} ${resolution} 启用价格。`)
  }
  if (!capability.operations.length || !capability.resolutions.length) errors.push('有效候选缺少共同任务 / 分辨率能力。')
  return errors
}

// Availability is operation scoped: one absent/unfinished operation never hides valid ordinary sales.
export function routeImagePricingAvailability(config: ImagePricingConfig | null | undefined, capability: RouteImageCapabilities) {
  const operations = capability.operations.map((operation) => {
    const rules = config?.rules.filter((rule) => rule.operation === operation) ?? []
    const missing = !rules.length
    const errors = missing ? [`未配置${imageOperationLabels[operation]}售价，未开放。`] : routeImagePricingErrors({ schema_version: config!.schema_version, rules }, capability)
    if (operation === 'layer_decomposition' && !missing) {
      errors.push(...capability.layerCostMissing.map((item) => `拆层成本缺少：${item}`))
      if (!capability.maxLayers || capability.maxLayers < 1) errors.push('拆层缺少有效 max_layers。')
      if (!capability.layerSizeModes.length) errors.push('拆层缺少 layer_size_modes。')
    }
    return { operation, ready: !errors.length, errors }
  })
  const available = operations.filter((item) => item.ready).map((item) => item.operation)
  const warnings = [...capability.warnings, ...operations.filter((item) => !item.ready).flatMap((item) => item.errors)]
  for (const rule of config?.rules ?? []) if (!capability.operations.includes(rule.operation)) warnings.push(`${imageOperationLabels[rule.operation]}候选能力不兼容，未开放。`)
  return { available, warnings, ready: available.length > 0 }
}
