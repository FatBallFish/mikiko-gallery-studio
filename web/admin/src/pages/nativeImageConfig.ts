import { nativeImageProfile, type NativeImageProfile } from '../../../shared/native-image'

export type ConfigObject = Record<string, unknown>
export function configObject(value: unknown): ConfigObject {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as ConfigObject : {}
}
export function configValue(extra: unknown, path: string[]): unknown {
  return path.reduce<unknown>((value, key) => configObject(value)[key], extra)
}
// undefined means inheritance (remove only this key); null, false and zero remain distinct.
export function patchConfig(extra: unknown, path: string[], value: unknown): ConfigObject {
  const [key, ...rest] = path
  const next = { ...configObject(extra) }
  if (rest.length) next[key] = patchConfig(next[key], rest, value)
  else if (value === undefined) delete next[key]
  else next[key] = value
  return next
}
export function effectiveNativeProfile(extra: unknown, accountExtra?: unknown, modelCode = ''): NativeImageProfile | undefined {
  const local = configObject(configValue(extra, ['native_image']))
  const account = configObject(configValue(accountExtra, ['native_image']))
  const code = String(local.model ?? account.model ?? modelCode)
  const base = nativeImageProfile(code)
  if (!base) return undefined
  const override = local.profile === undefined ? account.profile : local.profile
  if (override === undefined || override === null) return base
  const profile = configObject(override)
  if (profile.model !== code || !Array.isArray(profile.operations)) return undefined
  return profile as NativeImageProfile
}
const pricingFields = ['currency', 'model', 'version', 'input_cny', 'free_input_images', 'output_tiers', 'layer_tiers', 'layer_base_cny', 'layer_pricing_confirmed']
export function pricingIdentityMismatch(value: unknown, model: string): boolean {
  const pricing = configObject(value)
  const hasKnownPrice = ['input_cny', 'free_input_images', 'layer_base_cny', 'layer_pricing_confirmed'].some((key) => pricing[key] !== undefined)
    || ['output_tiers', 'layer_tiers'].some((key) => Array.isArray(pricing[key]) && (pricing[key] as unknown[]).some((tier) => ['name', 'max_pixels', 'cny'].some((field) => configObject(tier)[field] !== undefined)))
  return !model || (pricing.model !== undefined && pricing.model !== model) || (pricing.currency !== undefined && pricing.currency !== 'CNY')
    || hasKnownPrice && (pricing.model !== model || pricing.currency !== 'CNY')
}
export function resetNativePricing(value: unknown, model: string, version: string): ConfigObject {
  const pricing = configObject(value)
  const opaque = Object.fromEntries(Object.entries(pricing).filter(([key]) => !pricingFields.includes(key)))
  for (const field of ['output_tiers', 'layer_tiers']) {
    if (Array.isArray(pricing[field])) opaque[field] = (pricing[field] as unknown[]).map((tier) => Object.fromEntries(Object.entries(configObject(tier)).filter(([key]) => !['name', 'max_pixels', 'cny'].includes(key))))
  }
  return { ...opaque, currency: 'CNY', model, version }
}
export function patchNativePricing(value: unknown, model: string, version: string, patch: ConfigObject): ConfigObject | undefined {
  if (pricingIdentityMismatch(value, model)) return undefined
  const pricing = configObject(value)
  return { currency: 'CNY', model, version, ...pricing, ...patch }
}
// Changing model invalidates known prices while retaining opaque vendor metadata.
export function switchNativePreset(extra: unknown, profile: NativeImageProfile): ConfigObject {
  const native = configObject(configValue(extra, ['native_image']))
  if ((native.model ?? configObject(native.profile).model) === profile.model) return configObject(extra)
  const next = { ...native, model: profile.model, profile: { ...configObject(native.profile), ...profile } }
  for (const field of ['pricing']) {
    const old = configObject(native[field])
    const opaque = Object.fromEntries(Object.entries(old).filter(([key]) => !pricingFields.includes(key)))
    // Keep unknown tier properties in place; their old price and threshold are not reusable.
    for (const tiers of ['output_tiers', 'layer_tiers']) {
      if (Array.isArray(old[tiers])) opaque[tiers] = old[tiers].map((tier) => Object.fromEntries(Object.entries(configObject(tier)).filter(([key]) => !['name', 'max_pixels', 'cny'].includes(key))))
    }
    Object.assign(next, { [field]: opaque })
  }
  return patchConfig(extra, ['native_image'], next)
}
function nonnegative(value: unknown) { return typeof value === 'number' && Number.isFinite(value) && value >= 0 }
export function pricingErrors(value: unknown, profile: NativeImageProfile | undefined, enabled: boolean): string[] {
  const pricing = configObject(value)
  if (!enabled) return [] // Disabled drafts may contain incomplete rates.
  const errors: string[] = []
  if (!profile) return ['请选择受支持的精确型号。']
  if (pricing.currency !== 'CNY') errors.push('原生价格币种必须为 CNY。')
  if (pricing.model !== profile.model || !pricing.version) errors.push('价格须属于当前精确型号且具有版本。')
  if (!nonnegative(pricing.input_cny)) errors.push('请填写有限非负的输入单价。')
  if (!nonnegative(pricing.free_input_images) || !Number.isInteger(pricing.free_input_images)) errors.push('免费输入张数需为非负整数。')
  const validateTiers = (value: unknown, label: string) => {
    const tiers = Array.isArray(value) ? value.map(configObject) : []
    if (!tiers.length) { errors.push(`${label}至少配置一个档位。`); return }
    let previous = 0
    for (const tier of tiers) {
      if (typeof tier.name !== 'string' || !tier.name.trim()) errors.push(`${label}请填写档名。`)
      if (!Number.isSafeInteger(tier.max_pixels) || Number(tier.max_pixels) <= previous) errors.push(`${label}总像素上限必须为正整数且严格递增。`)
      if (!nonnegative(tier.cny)) errors.push(`${label}每张单价必须为有限非负数字。`)
      previous = Number(tier.max_pixels)
    }
    if (previous > profile.max_pixels) errors.push(`${label}不能超过型号最大 ${profile.max_pixels} 总像素。`)
    if (previous < profile.max_pixels) errors.push(`${label}末档需覆盖 ${profile.max_pixels} 总像素。`)
  }
  validateTiers(pricing.output_tiers, '普通输出')
  if (pricing.layer_pricing_confirmed === true) {
    if (!profile.layer_decomposition || !profile.operations.includes('layer_decomposition')) errors.push('当前型号不支持拆层。')
    if (!nonnegative(pricing.layer_base_cny)) errors.push('拆层底图单价必须为有限非负数字。')
    validateTiers(pricing.layer_tiers, '拆层输出')
  }
  return [...new Set(errors)]
}
export function nativeConfigurationErrors(extra: unknown, accountExtra: unknown, modelCode: string, enabled: boolean): string[] {
  const profile = effectiveNativeProfile(extra, accountExtra, modelCode)
  const native = configObject(configValue(extra, ['native_image']))
  const account = configObject(configValue(accountExtra, ['native_image']))
  const errors = ['pricing'].flatMap((key) => pricingErrors(native[key] === undefined ? account[key] : native[key], profile, enabled).map((message) => `成本：${message}`))
  if (native.profile !== undefined && native.profile !== null) {
    const current = configObject(native.profile)
    const official = nativeImageProfile(String(native.model ?? account.model ?? modelCode))
    if (!official) errors.push('能力覆盖需要受支持的精确型号。')
    else {
      for (const [key, minimum, maximum] of [
        ['max_reference_images', 0, official.max_reference_images], ['max_output_images', 1, official.max_output_images],
        ['min_pixels', official.min_pixels, Number(current.max_pixels)], ['max_pixels', Number(current.min_pixels), official.max_pixels],
        ['max_aspect_ratio', 1, official.max_aspect_ratio],
        ...(official.max_layers !== undefined ? [['max_layers', 1, official.max_layers] as [string, number, number]] : []),
      ] as Array<[string, number, number]>) {
        const value = current[key]
        if (typeof value !== 'number' || !Number.isFinite(value) || value < minimum || value > maximum || key !== 'max_aspect_ratio' && !Number.isSafeInteger(value)) errors.push(`能力 ${key} 仅可在官方范围内收紧。`)
      }
      for (const key of ['operations', 'output_formats', ...(official.layer_size_modes ? ['layer_size_modes' as const] : [])] as const) {
        if (!Array.isArray(current[key]) || !(current[key] as unknown[]).length || (current[key] as unknown[]).some((value) => !(official[key] as unknown[]).includes(value))) errors.push(`能力 ${key} 至少选择一项且不能超出官方支持范围。`)
      }
      if (current.layer_decomposition === true && !official.layer_decomposition) errors.push('不能为不支持的型号启用拆层。')
    }
  }
  const seed = configValue(extra, ['native_image', 'defaults', 'seed'])
  if (seed !== undefined && seed !== null && (!Number.isInteger(seed) || Number(seed) < 0 || Number(seed) > 2147483647 || profile?.adapter !== 'qwen_image')) errors.push('固定种子仅支持 Qwen，需为 0..2147483647 的整数。')
  return errors
}
export function summarizeConfig(value: unknown): string {
  return JSON.stringify(value, (_key, item) => typeof item === 'number' && !Number.isFinite(item) ? '未填写' : item, 2) ?? '未设置（继承）'
}

export function nativeLayerCostMissing(value: unknown, profile?: NativeImageProfile): string[] {
  if (!profile?.layer_decomposition || !profile.operations.includes('layer_decomposition')) return []
  const price = configObject(value)
  const missing: string[] = []
  if (!profile.max_layers) missing.push('profile.max_layers（最大图层数）')
  if (!profile.layer_size_modes?.length) missing.push('profile.layer_size_modes（拆层尺寸模式）')
  if (price.layer_pricing_confirmed !== true) missing.push('拆层成本口径确认')
  if (!nonnegative(price.layer_base_cny)) missing.push('拆层底图成本 layer_base_cny')
  if (!Array.isArray(price.layer_tiers) || !price.layer_tiers.length) missing.push('拆层图层成本档位 layer_tiers')
  else {
    let previous = 0
    for (const [index, value] of price.layer_tiers.entries()) {
      const tier = configObject(value)
      if (!String(tier.name ?? '').trim()) missing.push(`拆层第 ${index + 1} 档名称`)
      if (!Number.isSafeInteger(tier.max_pixels) || Number(tier.max_pixels) <= previous) missing.push(`拆层第 ${index + 1} 档递增像素上限`)
      if (Number(tier.max_pixels) > profile.max_pixels) missing.push(`拆层第 ${index + 1} 档超出型号最大 ${profile.max_pixels} 像素`)
      if (!nonnegative(tier.cny)) missing.push(`拆层第 ${index + 1} 档每张成本`)
      previous = Number(tier.max_pixels)
    }
    if (previous < profile.max_pixels) missing.push(`拆层末档覆盖 ${profile.max_pixels} 像素`)
  }
  return missing
}
