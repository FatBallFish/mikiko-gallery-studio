import type { UserGroup } from '../../../shared/api-types'

export type UserGroupStatusTone = 'success' | 'warning' | 'neutral'

export type UserGroupRowModel = {
  id: string
  code: string
  name: string
  description: string
  multiplier: string
  sortOrder: number
  defaultLabel: string
  defaultTone: 'primary' | 'neutral'
  status: string
  statusLabel: string
  statusTone: UserGroupStatusTone
}

export type UserGroupSummaryModel = {
  total: number
  enabled: number
  defaultName: string
  highestMultiplier: string
}

export function userGroupRows(groups: UserGroup[]): UserGroupRowModel[] {
  return groups.map((group) => ({
    id: String(group.id ?? group.code),
    code: group.code,
    name: group.name,
    description: group.description || '无描述',
    multiplier: group.multiplier,
    sortOrder: group.sort_order ?? 0,
    defaultLabel: group.is_default ? '默认' : '普通',
    defaultTone: group.is_default ? 'primary' : 'neutral',
    status: group.status,
    statusLabel: userGroupStatusLabel(group.status),
    statusTone: userGroupStatusTone(group.status),
  }))
}

export function userGroupSummary(groups: UserGroup[]): UserGroupSummaryModel {
  return {
    total: groups.length,
    enabled: groups.filter((group) => isEnabledUserGroupStatus(group.status)).length,
    defaultName: groups.find((group) => group.is_default)?.name ?? '未设置',
    highestMultiplier: groups.reduce((max, group) => Math.max(max, Number(group.multiplier) || 0), 0).toFixed(5),
  }
}

export function userGroupStatusLabel(status?: string | null) {
  const normalized = normalizeStatus(status)
  if (normalized === 'enabled' || normalized === 'active') return '启用'
  if (normalized === 'disabled') return '停用'
  return normalized || '未知状态'
}

export function userGroupStatusTone(status?: string | null): UserGroupStatusTone {
  const normalized = normalizeStatus(status)
  if (normalized === 'enabled' || normalized === 'active') return 'success'
  if (normalized === 'disabled') return 'warning'
  return 'neutral'
}

function isEnabledUserGroupStatus(status?: string | null) {
  const normalized = normalizeStatus(status)
  return normalized === 'enabled' || normalized === 'active'
}

function normalizeStatus(status?: string | null) {
  return (status ?? '').trim().toLowerCase()
}
