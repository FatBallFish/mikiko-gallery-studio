import type { RouteModel, RouteModelPrice } from '../../../shared/api-types'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
import { loadAllRouteModelPrices } from './loadAllRouteModelPrices'
import {
  pricingEnabledBadge,
  pricingFieldHints,
  pricingBaseResolutionLabel,
  pricingBaseResolutionOptions,
  pricingRouteLabel,
  pricingRouteSecondaryLabel,
  pricingStatusOptions,
  pricingSummary,
} from './pricingRows'

const pricingPageSource = readFileSync(new URL('./PricingPage.tsx', import.meta.url), 'utf8')

const requestedPages: number[] = []
const allPricePages = await loadAllRouteModelPrices(async ({ page }) => {
  requestedPages.push(page)
  return page === 1 ? [price({ id: 1 }), price({ id: 2 })] : page === 2 ? [price({ id: 3 })] : []
}, 2)
if (allPricePages.length !== 3 || requestedPages.join(',') !== '1,2') {
  throw new Error(`price completeness must consume every API page, got ${JSON.stringify({ allPricePages, requestedPages })}`)
}

const serverCappedPages: number[] = []
const serverCappedPrices = await loadAllRouteModelPrices(async ({ page }) => {
  serverCappedPages.push(page)
  return page === 1 ? Array.from({ length: 100 }, (_, index) => price({ id: index + 1 })) : [price({ id: 101 })]
})
if (serverCappedPrices.length !== 101 || serverCappedPages.join(',') !== '1,2') {
  throw new Error(`price pagination must honor the backend 100-item page-size cap, got ${JSON.stringify({ length: serverCappedPrices.length, serverCappedPages })}`)
}

for (const requiredPrimitive of ['MetricStrip', 'FilterToolbar', 'DataTable', 'ActionMenu', 'InlineFeedback']) {
  if (!pricingPageSource.includes(requiredPrimitive)) {
    throw new Error(`pricing workspace should use the shared ${requiredPrimitive} primitive`)
  }
}

for (const riskContract of ['data-admin-pricing-risk', 'missingEnabledRoutes', '补齐价格', 'newPriceDialogForRoute']) {
  if (!pricingPageSource.includes(riskContract)) {
    throw new Error(`pricing workspace should expose missing-price repair context with ${riskContract}`)
  }
}

if (!pricingPageSource.includes("route.visibility !== 'hidden'")) {
  throw new Error('hidden routes must not be reported as user-facing missing-price risks')
}

if (!pricingPageSource.includes("route.visibility !== 'groups' || Boolean(route.group_ids?.length)")) {
  throw new Error('group-scoped routes without a bound group must not be reported as user-facing missing-price risks')
}

for (const groupingContract of ['expandedGroups', 'adminTaskTypeLabel(group.taskType)', 'pricingBaseResolutionLabel(row.base_resolution)']) {
  if (!pricingPageSource.includes(groupingContract)) {
    throw new Error(`pricing workspace should preserve grouped expansion context with ${groupingContract}`)
  }
}

for (const rowExpansionContract of ['renderAfterRow', 'pricingExpandedGroup(group, openDialog,']) {
  if (!pricingPageSource.includes(rowExpansionContract)) {
    throw new Error(`expanded pricing details must render directly after their route-model row with ${rowExpansionContract}`)
  }
}

if (pricingPageSource.includes('expandedVisibleGroups')) {
  throw new Error('expanded pricing details must not be collected into a global block below the route-model list')
}

for (const filterStateContract of ['hasActiveFilters', "hasActiveFilters ? '已应用筛选' : '全部价格组'"]) {
  if (!pricingPageSource.includes(filterStateContract)) {
    throw new Error(`pricing filter toolbar should expose truthful filter state with ${filterStateContract}`)
  }
}

for (const forbiddenPattern of ['rounded-3xl', 'tracking-[', 'uppercase', 'noticeGrid']) {
  if (pricingPageSource.includes(forbiddenPattern)) {
    throw new Error(`pricing workspace should remove legacy decorative styling ${forbiddenPattern}`)
  }
}

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

if (pricingBaseResolutionLabel('1K') !== '1K 标准' || pricingBaseResolutionLabel('2K') !== '2K 高清' || pricingBaseResolutionLabel('4K') !== '4K 超清') {
  throw new Error('pricing quality labels should be operator-facing')
}

if (pricingBaseResolutionLabel('auto') !== '自动档位' || pricingBaseResolutionLabel('8K') !== '8K' || pricingBaseResolutionLabel('') !== '未知基础分辨率') {
  throw new Error('pricing quality labels should preserve unknown values for troubleshooting')
}

for (const rawValue of ['1K', '2K', '4K']) {
  if (!pricingBaseResolutionOptions.some((option) => option.value === rawValue)) {
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
    base_resolution: patch.base_resolution ?? '1K',
    base_points: patch.base_points ?? '8.00000',
    reference_multiplier: patch.reference_multiplier ?? '1.00000',
    enabled: patch.enabled ?? true,
  }
}
