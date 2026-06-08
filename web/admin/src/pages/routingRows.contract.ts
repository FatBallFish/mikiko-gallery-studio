import type { RouteModelCandidate, UserGroup } from '../../../shared/api-types'
import {
  routeCandidateLabel,
  routeCandidateSummary,
  routeEnabledBadge,
  routeEnabledOptions,
  routingFieldHints,
  routingFieldLabels,
  routeGroupNames,
  routeVisibilityBadge,
  routeVisibilityOptions,
} from './routingRows'

const publicBadge = routeVisibilityBadge('public')
const groupsBadge = routeVisibilityBadge('groups')
const hiddenBadge = routeVisibilityBadge('hidden')
if (publicBadge.label !== '全员可见' || publicBadge.tone !== 'success' || groupsBadge.label !== '按分组可见' || groupsBadge.tone !== 'primary' || hiddenBadge.label !== '隐藏') {
  throw new Error(`route visibility badges should be localized, got ${JSON.stringify({ publicBadge, groupsBadge, hiddenBadge })}`)
}

if (routingFieldLabels.code !== '路由代码' || routingFieldLabels.priority !== '优先级' || routingFieldLabels.weight !== '权重' || routingFieldLabels.fallbackOrder !== '兜底顺序') {
  throw new Error(`routing field labels should be operator-facing Chinese labels, got ${JSON.stringify(routingFieldLabels)}`)
}

if (/Code|Priority|Weight|Fallback/.test(Object.values(routingFieldLabels).join(' '))) {
  throw new Error(`routing field labels should not expose English internal terms, got ${JSON.stringify(routingFieldLabels)}`)
}

if (!routingFieldHints.priority.includes('兜底顺序') || !routingFieldHints.weight.includes('流量占比') || !routingFieldHints.fallbackOrder.includes('数值越小越早兜底')) {
  throw new Error(`routing field hints should explain candidate sorting in operator-facing wording, got ${JSON.stringify(routingFieldHints)}`)
}

if (/fallback|priority|weight/i.test(Object.values(routingFieldHints).join(' '))) {
  throw new Error(`routing field hints should not expose raw internal field names, got ${JSON.stringify(routingFieldHints)}`)
}

const unknownVisibility = routeVisibilityBadge('tenant_only')
if (unknownVisibility.label !== 'tenant_only' || unknownVisibility.tone !== 'neutral') {
  throw new Error(`unknown visibility should preserve raw value for troubleshooting, got ${JSON.stringify(unknownVisibility)}`)
}

const enabled = routeEnabledBadge(true)
const disabled = routeEnabledBadge(false)
if (enabled.label !== '启用' || enabled.tone !== 'success' || disabled.label !== '停用' || disabled.tone !== 'warning') {
  throw new Error(`route enabled badges should be localized, got ${JSON.stringify({ enabled, disabled })}`)
}

for (const rawValue of ['public', 'groups', 'hidden']) {
  if (!routeVisibilityOptions.some((option) => option.value === rawValue)) {
    throw new Error(`visibility options must preserve raw value ${rawValue}`)
  }
}

for (const option of routeVisibilityOptions) {
  if (String(option.label) === String(option.value)) {
    throw new Error(`visibility option ${option.value} should expose operator-facing label`)
  }
}

for (const rawValue of ['enabled', 'disabled']) {
  if (!routeEnabledOptions.some((option) => option.value === rawValue)) {
    throw new Error(`enabled options must preserve raw value ${rawValue}`)
  }
}

const groups: UserGroup[] = [
  group({ id: 1, code: 'basic', name: '基础分组' }),
  group({ id: 2, code: 'creator', name: '创作者分组' }),
]
if (routeGroupNames([1, 2, 99], groups) !== '基础分组, 创作者分组, 99') {
  throw new Error(`route group names should resolve known groups and preserve unknown ids, got ${routeGroupNames([1, 2, 99], groups)}`)
}

if (routeGroupNames(undefined, groups) !== '全员或未绑定分组' || routeGroupNames([], groups) !== '全员或未绑定分组') {
  throw new Error('empty route group binding should show an operator-facing fallback')
}

const candidates: RouteModelCandidate[] = [
  candidate({ id: 1, enabled: true }),
  candidate({ id: 2, enabled: false }),
  candidate({ id: 3, enabled: true }),
]
if (routeCandidateSummary(candidates) !== '3 个候选 · 2 个启用' || routeCandidateSummary([]) !== '暂无候选') {
  throw new Error(`candidate summary should show total and enabled count, got ${routeCandidateSummary(candidates)}`)
}

if (routeCandidateLabel(candidate({ account_name: 'OpenAI 主账号', model_code: 'gpt-image-1' })) !== 'OpenAI 主账号 / gpt-image-1') {
  throw new Error('candidate label should include account name and model code')
}

if (routeCandidateLabel(candidate({ account_name: '', model_code: undefined, account_model_id: 88 })) !== '88') {
  throw new Error('candidate label should fall back to account model id when model code is absent')
}

function group(patch: Partial<UserGroup>): UserGroup {
  return {
    id: patch.id,
    code: patch.code ?? 'group',
    name: patch.name ?? '分组',
    group_code: patch.group_code ?? patch.code ?? 'group',
    group_name: patch.group_name ?? patch.name ?? '分组',
    multiplier: patch.multiplier ?? '1.00000',
    status: patch.status ?? 'enabled',
    created_at: patch.created_at ?? '2026-06-05T00:00:00Z',
    updated_at: patch.updated_at ?? '2026-06-05T00:00:00Z',
  }
}

function candidate(patch: Partial<RouteModelCandidate>): RouteModelCandidate {
  return {
    id: patch.id ?? 1,
    route_model_id: patch.route_model_id ?? 1,
    account_model_id: patch.account_model_id ?? 1,
    account_name: patch.account_name,
    model_code: patch.model_code,
    priority: patch.priority ?? 1,
    weight: patch.weight ?? 100,
    fallback_order: patch.fallback_order ?? 1,
    enabled: patch.enabled ?? true,
  }
}
