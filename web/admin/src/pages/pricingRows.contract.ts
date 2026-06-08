import type { RouteModel, RouteModelPrice } from '../../../shared/api-types'
import {
  pricingEnabledBadge,
  pricingFieldHints,
  pricingQualityLabel,
  pricingQualityOptions,
  pricingRouteLabel,
  pricingRouteSecondaryLabel,
  pricingStatusOptions,
  pricingSummary,
} from './pricingRows'

const routes: RouteModel[] = [
  route({ id: 1, code: 'basic', name: '基础模型', enabled: true }),
  route({ id: 2, code: 'pro', name: '专业模型', enabled: true }),
  route({ id: 3, code: 'legacy', name: '历史模型', enabled: false }),
]

const prices: RouteModelPrice[] = [
  price({ id: 10, route_model_id: 1, route_model_code: 'basic', enabled: true }),
  price({ id: 11, route_model_id: 3, route_model_code: 'legacy', enabled: false }),
]

const summary = pricingSummary(routes, prices)
if (summary.totalRoutes !== 3 || summary.enabledRoutes !== 2 || summary.totalPrices !== 2 || summary.enabledPrices !== 1 || summary.missingEnabledRoutes !== 1) {
  throw new Error(`pricing summary should count only enabled route models as missing price risks, got ${JSON.stringify(summary)}`)
}

const enabled = pricingEnabledBadge(true)
const disabled = pricingEnabledBadge(false)
if (enabled.label !== '启用' || enabled.tone !== 'success' || disabled.label !== '停用' || disabled.tone !== 'warning') {
  throw new Error(`pricing enabled badge should be localized, got ${JSON.stringify({ enabled, disabled })}`)
}

if (pricingQualityLabel('1K') !== '1K 标准' || pricingQualityLabel('2K') !== '2K 高清' || pricingQualityLabel('4K') !== '4K 超清') {
  throw new Error('pricing quality labels should be operator-facing')
}

if (pricingQualityLabel('auto') !== '自动档位' || pricingQualityLabel('8K') !== '8K' || pricingQualityLabel('') !== '未知质量') {
  throw new Error('pricing quality labels should preserve unknown values for troubleshooting')
}

for (const rawValue of ['1K', '2K', '4K']) {
  if (!pricingQualityOptions.some((option) => option.value === rawValue)) {
    throw new Error(`quality options must preserve raw value ${rawValue}`)
  }
}

if (pricingStatusOptions.map((option) => option.label).join(',') !== '启用,停用') {
  throw new Error(`pricing status options should not expose raw enum labels, got ${JSON.stringify(pricingStatusOptions)}`)
}

for (const rawValue of ['enabled', 'disabled']) {
  if (!pricingStatusOptions.some((option) => option.value === rawValue)) {
    throw new Error(`pricing status options must preserve raw value ${rawValue}`)
  }
}

const knownRouteLabel = pricingRouteLabel(1, routes, prices[0])
if (knownRouteLabel !== '基础模型 (basic)' || pricingRouteSecondaryLabel(1, routes, prices[0]) !== 'basic') {
  throw new Error(`known route labels should prefer route model data, got ${knownRouteLabel}`)
}

const fallbackRouteLabel = pricingRouteLabel(99, [], { route_model_name: '缓存模型', route_model_code: 'cached' })
if (fallbackRouteLabel !== '缓存模型 (cached)' || pricingRouteSecondaryLabel(99, [], { route_model_code: 'cached' }) !== 'cached') {
  throw new Error(`price fallback route labels should use denormalized price fields, got ${fallbackRouteLabel}`)
}

const requiredHints = {
  dialogDetail: ['计费预估', '生成扣费', '不是 Provider 成本'],
  basePoints: ['单张图片', '基础积分', '5 位小数'],
  referenceMultiplier: ['参考图', '基础积分', '放大倍率'],
} as const

for (const [key, fragments] of Object.entries(requiredHints)) {
  const hint = pricingFieldHints[key as keyof typeof pricingFieldHints]
  if (!hint) {
    throw new Error(`pricing field hint ${key} should exist`)
  }
  for (const fragment of fragments) {
    if (!hint.includes(fragment)) {
      throw new Error(`pricing field hint ${key} should include ${fragment}, got ${hint}`)
    }
  }
}

const allHints = Object.values(pricingFieldHints).join(' ')
if (/provider_cost|base_points|reference_multiplier|raw/i.test(allHints)) {
  throw new Error(`pricing field hints should be operator-facing, got ${allHints}`)
}

function route(patch: Partial<RouteModel>): RouteModel {
  return {
    id: patch.id ?? 1,
    code: patch.code ?? 'route',
    name: patch.name ?? '路由模型',
    visibility: patch.visibility ?? 'public',
    enabled: patch.enabled ?? true,
    sort_order: patch.sort_order ?? 1,
    created_at: patch.created_at ?? '2026-06-05T00:00:00Z',
    updated_at: patch.updated_at ?? '2026-06-05T00:00:00Z',
  }
}

function price(patch: Partial<RouteModelPrice>): RouteModelPrice {
  return {
    id: patch.id ?? 1,
    route_model_id: patch.route_model_id ?? 1,
    route_model_code: patch.route_model_code,
    route_model_name: patch.route_model_name,
    task_type: patch.task_type ?? 'text_to_image',
    quality: patch.quality ?? '1K',
    base_points: patch.base_points ?? '8.00000',
    reference_multiplier: patch.reference_multiplier ?? '1.00000',
    enabled: patch.enabled ?? true,
  }
}
