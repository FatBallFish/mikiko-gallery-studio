import type { ID, RouteModelCandidate, RouteModelVisibility, UserGroup } from '../../../shared/api-types'

export type RoutingTone = 'success' | 'warning' | 'danger' | 'neutral' | 'primary'

export type RoutingBadge = {
  label: string
  tone: RoutingTone
}

export const routeVisibilityOptions = [
  { value: 'public', label: '全员可见' },
  { value: 'groups', label: '按分组可见' },
  { value: 'hidden', label: '隐藏' },
] as const satisfies ReadonlyArray<{ value: RouteModelVisibility; label: string }>

export const routeEnabledOptions = [
  { value: 'enabled', label: '启用' },
  { value: 'disabled', label: '停用' },
] as const

export const routingFieldLabels = {
  code: '路由代码',
  priority: '优先级',
  weight: '权重',
  fallbackOrder: '兜底顺序',
} as const

export const routingFieldHints = {
  priority: '数值越小越先尝试；同优先级时再参考兜底顺序。',
  weight: '同一优先级内的流量占比，100 表示默认满权重。',
  fallbackOrder: '候选失败后的兜底顺序，数值越小越早兜底。',
} as const

export function routeVisibilityBadge(visibility?: string | null): RoutingBadge {
  const normalized = normalize(visibility)
  if (normalized === 'public') return { label: '全员可见', tone: 'success' }
  if (normalized === 'groups') return { label: '按分组可见', tone: 'primary' }
  if (normalized === 'hidden') return { label: '隐藏', tone: 'warning' }
  return { label: normalized || '未知可见性', tone: 'neutral' }
}

export function routeEnabledBadge(enabled: boolean): RoutingBadge {
  return enabled ? { label: '启用', tone: 'success' } : { label: '停用', tone: 'warning' }
}

export function routeGroupNames(ids: ID[] | undefined, groups: UserGroup[]) {
  if (!ids?.length) return '全员或未绑定分组'
  const names = ids.map((id) => groups.find((group) => String(group.id ?? group.code) === String(id))?.name ?? String(id))
  return names.join(', ')
}

export function routeCandidateSummary(candidates: Pick<RouteModelCandidate, 'enabled'>[]) {
  const enabled = candidates.filter((item) => item.enabled).length
  if (!candidates.length) return '暂无候选'
  return `${candidates.length} 个候选 · ${enabled} 个启用`
}

export function routeCandidateLabel(candidate: Pick<RouteModelCandidate, 'account_name' | 'model_code' | 'account_model_id'>) {
  const model = candidate.model_code ?? String(candidate.account_model_id)
  return candidate.account_name ? `${candidate.account_name} / ${model}` : model
}

function normalize(value?: string | null) {
  return (value ?? '').trim().toLowerCase()
}
