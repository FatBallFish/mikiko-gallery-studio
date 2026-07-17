import type { AdminUser } from '../../../shared/api-types'

export type AdminUserStatusTone = 'success' | 'warning' | 'danger' | 'neutral'

export type AdminUserStatusBadge = {
  label: string
  tone: AdminUserStatusTone
}

export type AdminUserSummary = {
  total: number
  active: number
  pending: number
  disabled: number
  closed: number
}

export type AdminUserRowView = {
  raw: AdminUser
  name: string
  subtitle: string
  statusLabel: string
  statusTone: AdminUserStatusTone
  groupLabel: string
  balanceLabel: string
  lastActiveAtLabel: string
  createdAtLabel: string
}

export type AdminUserRowAction = {
  id: string
  label: string
  tone?: 'neutral' | 'danger'
  confirm?: {
    title: string
    expectedValue: string
  }
}

export type AdminUserRowActions = {
  primary: AdminUserRowAction
  secondary?: AdminUserRowAction
  overflow: AdminUserRowAction[]
}

export const adminUserStatusOptions = [
  { value: 'active', label: '正常' },
  { value: 'pending', label: '待验证' },
  { value: 'disabled', label: '禁用' },
] as const

export const adminUserStatusFilterOptions = [
  { value: '', label: '全部状态' },
  ...adminUserStatusOptions,
  { value: 'closed', label: '已关闭' },
] as const

export function adminUserStatusBadge(status?: string | null): AdminUserStatusBadge {
  const normalized = normalize(status)
  if (normalized === 'active') return { label: '正常', tone: 'success' }
  if (normalized === 'pending') return { label: '待验证', tone: 'warning' }
  if (normalized === 'disabled') return { label: '禁用', tone: 'danger' }
  if (normalized === 'closed') return { label: '已关闭', tone: 'neutral' }
  return { label: normalized || '未知状态', tone: 'neutral' }
}

export function adminUserSummary(rows: AdminUser[]): AdminUserSummary {
  return {
    total: rows.length,
    active: rows.filter((row) => normalize(row.status) === 'active').length,
    pending: rows.filter((row) => normalize(row.status) === 'pending').length,
    disabled: rows.filter((row) => normalize(row.status) === 'disabled').length,
    closed: rows.filter((row) => normalize(row.status) === 'closed').length,
  }
}

export function adminUserRowView(user: AdminUser): AdminUserRowView {
  const badge = adminUserStatusBadge(user.status)
  return {
    raw: user,
    name: user.display_name?.trim() || user.email || user.id,
    subtitle: [user.email, user.id].filter(Boolean).join(' · '),
    statusLabel: badge.label,
    statusTone: badge.tone,
    groupLabel: user.group.split(',').map((item) => item.trim()).filter(Boolean).join(', ') || '-',
    balanceLabel: user.balance || '0.00000',
    lastActiveAtLabel: adminUserDateTime(user.last_seen_at || user.updated_at),
    createdAtLabel: adminUserDateTime(user.created_at),
  }
}

export function adminUserRowActions(user: AdminUser): AdminUserRowActions {
  const email = user.email || user.id
  const disabled = normalize(user.status) === 'disabled'
  return {
    primary: { id: 'detail', label: '详情' },
    overflow: [
      {
        id: disabled ? 'enable' : 'disable',
        label: disabled ? '启用' : '禁用',
        tone: disabled ? 'neutral' : 'danger',
        confirm: {
          title: `请输入 ${email} 确认${disabled ? '启用' : '禁用'}该用户`,
          expectedValue: email,
        },
      },
      { id: 'group', label: '调整分组' },
      { id: 'points', label: '调整积分' },
      { id: 'limits', label: '设置限额' },
      { id: 'password', label: '重置密码' },
      {
        id: 'delete',
        label: '删除',
        tone: 'danger',
        confirm: {
          title: `请输入 ${email} 确认删除该用户`,
          expectedValue: email,
        },
      },
    ],
  }
}

function adminUserDateTime(value?: string | null) {
  const input = value ?? ''
  const match = input.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!match) return input || '-'
  return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
}

function normalize(status?: string | null) {
  return (status ?? '').trim().toLowerCase()
}
