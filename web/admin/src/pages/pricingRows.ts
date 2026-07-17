import type { RouteModel, RouteModelPrice } from '../../../shared/api-types'

export type PricingTone = 'success' | 'warning' | 'danger' | 'neutral'

export type PricingBadge = {
  label: string
  tone: PricingTone
}

export type PricingSummary = {
  totalRoutes: number
  enabledRoutes: number
  totalPrices: number
  enabledPrices: number
  missingEnabledRoutes: number
}

export const pricingStatusOptions = [
  { value: 'enabled', label: '启用' },
  { value: 'disabled', label: '停用' },
] as const

export const pricingFieldHints = {
  dialogDetail: '价格项会参与计费预估和生成扣费；这里配置的是用户积分，不是 Provider 成本。',
  basePoints: '单张图片在该路由模型、任务类型和基础分辨率档位下的基础积分，支持 5 位小数。',
  referenceMultiplier: '带参考图生成时在基础积分上应用的放大倍率；无参考图时按 1.00000 计算。',
} as const

const baseResolutionLabels: Record<string, string> = {
  '1k': '1K 标准',
  '2k': '2K 高清',
  '4k': '4K 超清',
  auto: '自动档位',
}

export const pricingBaseResolutionOptions = [
  { value: '1K', label: pricingBaseResolutionLabel('1K') },
  { value: '2K', label: pricingBaseResolutionLabel('2K') },
  { value: '4K', label: pricingBaseResolutionLabel('4K') },
] as const

export function pricingEnabledBadge(enabled: boolean): PricingBadge {
  return enabled ? { label: '启用', tone: 'success' } : { label: '停用', tone: 'warning' }
}

export function pricingBaseResolutionLabel(quality?: string | null) {
  const normalized = normalize(quality)
  return baseResolutionLabels[normalized] ?? ((quality ?? '').trim() || '未知基础分辨率')
}

export function pricingRouteLabel(routeModelId: string | number, routes: RouteModel[], price?: Pick<RouteModelPrice, 'route_model_code' | 'route_model_name'>) {
  const route = routes.find((item) => String(item.id) === String(routeModelId))
  if (route) return `${route.name} (${route.code})`
  if (price?.route_model_name && price.route_model_code) return `${price.route_model_name} (${price.route_model_code})`
  if (price?.route_model_name) return price.route_model_name
  if (price?.route_model_code) return price.route_model_code
  return String(routeModelId)
}

export function pricingRouteSecondaryLabel(routeModelId: string | number, routes: RouteModel[], price?: Pick<RouteModelPrice, 'route_model_code'>) {
  const route = routes.find((item) => String(item.id) === String(routeModelId))
  return route?.code ?? price?.route_model_code ?? String(routeModelId)
}

export function pricingSummary(routes: RouteModel[], prices: RouteModelPrice[]): PricingSummary {
  return {
    totalRoutes: routes.length,
    enabledRoutes: routes.filter((item) => item.enabled).length,
    totalPrices: prices.length,
    enabledPrices: prices.filter((item) => item.enabled).length,
    missingEnabledRoutes: routes.filter((route) => route.enabled && !prices.some((price) => String(price.route_model_id) === String(route.id))).length,
  }
}

function normalize(value?: string | null) {
  return (value ?? '').trim().toLowerCase()
}
