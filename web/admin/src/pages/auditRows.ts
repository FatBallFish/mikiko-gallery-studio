import type { AuditLog } from '../../../shared/api-types'

export type AuditBadgeTone = 'success' | 'warning' | 'danger' | 'neutral' | 'primary'

export type AuditResultBadge = {
  label: string
  tone: AuditBadgeTone
}

export type AuditSubjectLabels = {
  actor: string
  target: string
}

export type AuditTimelineRow = {
  raw: AuditLog
  actionLabel: string
  actorLabel: string
  targetLabel: string
  actorTone: AuditBadgeTone
  createdAtLabel: string
  detailText: string
  result: AuditResultBadge
}

export const auditSearchPlaceholder = '搜索操作人 / 动作 / 对象 / 详情'

const commonActionValues = [
  'admin.login',
  'admin.logout',
  'user.create',
  'user.delete',
  'user.status_update',
  'user.group_update',
  'user.points_adjust',
  'user.password_reset',
  'user.limits_update',
  'config.update',
  'redeem.create',
  'redeem.status_update',
  'redeem.export',
  'model_provider.create',
  'model_provider.update',
  'provider_model.create',
  'provider_model.update',
  'model_route.update',
  'route_model.update',
  'route_model_price.update',
  'image_review.approve',
  'image_review.reject',
  'image_review.unpublish',
  'cashier.plan.update',
  'cashier.order.complete',
] as const

const actionLabels: Record<string, string> = {
  'admin.login': '管理员登录',
  'admin.logout': '管理员退出',
  'user.create': '创建用户',
  'user.delete': '删除用户',
  'user.status_update': '更新用户状态',
  'user.group_update': '更新用户分组',
  'user.points_adjust': '调整用户积分',
  'user.password_reset': '重置用户密码',
  'user.limits_update': '更新用户限额',
  'config.update': '更新系统配置',
  'redeem.create': '创建兑换码',
  'redeem.status_update': '更新兑换码状态',
  'redeem.export': '导出兑换码',
  'model_provider.create': '创建模型供应商',
  'model_provider.update': '更新模型供应商',
  'provider_model.create': '创建供应商模型',
  'provider_model.update': '更新供应商模型',
  'model_route.update': '更新模型路由',
  'route_model.update': '更新路由模型',
  'route_model_price.update': '更新价格配置',
  'image_review.approve': '通过公开审核',
  'image_review.reject': '拒绝公开审核',
  'image_review.unpublish': '下架公开图片',
  'cashier.plan.update': '更新收银台套餐',
  'cashier.order.complete': '确认支付到账',
}

const actorTypeLabels: Record<string, string> = {
  admin: '管理员',
  user: '用户',
  system: '系统',
  api_key: 'API Key',
}

const targetTypeLabels: Record<string, string> = {
  admin: '管理员',
  user: '用户',
  api_key: 'API Key',
  config: '配置',
  redeem: '兑换码',
  redeem_code: '兑换码',
  model_provider: '模型供应商',
  provider_model: '供应商模型',
  model_route: '模型路由',
  route_model: '路由模型',
  route_model_price: '价格配置',
  image: '图片',
  image_review: '公开审核',
  payment_order: '支付订单',
  cashier_plan: '收银台套餐',
}

export function auditActionOptions(rows: AuditLog[]) {
  const values = unique([...commonActionValues, ...rows.map((row) => normalizeRaw(row.action)).filter(Boolean)])
  return [
    { value: 'all', label: '全部动作' },
    ...values.map((value) => ({ value, label: auditActionLabel(value) })),
  ]
}

export function auditActionLabel(action?: string | null) {
  const normalized = normalizeRaw(action)
  if (!normalized) return '未知动作'
  return actionLabels[normalized] ?? normalized
}

export function auditResultBadge(result?: string | null): AuditResultBadge {
  const normalized = normalizeRaw(result) || 'success'
  if (normalized === 'success') return { label: '成功', tone: 'success' }
  if (normalized === 'failure' || normalized === 'failed' || normalized === 'error') return { label: '失败', tone: 'danger' }
  if (normalized === 'rejected' || normalized === 'denied') return { label: '已拒绝', tone: 'warning' }
  return { label: normalized, tone: 'neutral' }
}

export function auditSubjectLabel(row: Pick<AuditLog, 'actor' | 'actor_type' | 'actor_id' | 'target' | 'target_type' | 'target_id'>): AuditSubjectLabels {
  return {
    actor: subjectLabel(row.actor_type, row.actor_id, row.actor, actorTypeLabels, '操作人'),
    target: subjectLabel(row.target_type, row.target_id, row.target, targetTypeLabels, '对象'),
  }
}

export function auditDetailText(row: AuditLog) {
  const detail = normalizeRaw(row.detail)
  const result = normalizeRaw(row.result)
  const bits: string[] = []

  if (detail && detail !== result && detail !== 'success') bits.push(detail)
  if (row.ip_addr) bits.push(`IP ${row.ip_addr}`)
  if (row.user_agent) bits.push(`UA ${row.user_agent}`)
  bits.push(...metadataSummary(row.metadata))

  return bits.join(' · ') || auditResultBadge(result).label
}

export function auditSearchText(row: AuditLog) {
  const subject = auditSubjectLabel(row)
  const result = auditResultBadge(row.result)
  return [
    row.actor,
    subject.actor,
    row.actor_type,
    row.actor_id,
    row.action,
    auditActionLabel(row.action),
    row.target,
    subject.target,
    row.target_type,
    row.target_id,
    row.detail,
    auditDetailText(row),
    row.result,
    result.label,
    row.ip_addr,
    row.user_agent,
    row.created_at,
    ...metadataSearchValues(row.metadata),
  ].filter(Boolean).join(' ').toLowerCase()
}

export function auditTimelineRow(row: AuditLog): AuditTimelineRow {
  const subject = auditSubjectLabel(row)
  return {
    raw: row,
    actionLabel: auditActionLabel(row.action),
    actorLabel: subject.actor,
    targetLabel: subject.target,
    actorTone: row.actor_type === 'system' ? 'neutral' : 'primary',
    createdAtLabel: auditDateTime(row.created_at),
    detailText: auditDetailText(row),
    result: auditResultBadge(row.result),
  }
}

export function auditRowsCSV(rows: AuditLog[]) {
  const header = ['时间', '动作', '操作人', '对象', '结果', '详情', '审计ID']
  const body = rows.map((row) => {
    const item = auditTimelineRow(row)
    return [
      item.createdAtLabel,
      item.actionLabel,
      item.actorLabel,
      item.targetLabel,
      item.result.label,
      item.detailText,
      item.raw.id,
    ].map(csvCell).join(',')
  })
  return [header.join(','), ...body].join('\n')
}

export function auditExportFilename(now: Date | string = new Date()) {
  const parts = dateParts(now)
  return `audit-logs-${parts.date}-${parts.time}.csv`
}

function subjectLabel(type: string | undefined, id: string | undefined, fallback: string | undefined, labels: Record<string, string>, emptyLabel: string) {
  const normalizedType = normalizeRaw(type)
  const normalizedId = normalizeRaw(id)
  const raw = normalizeRaw(fallback)
  if (normalizedType || normalizedId) {
    const label = labels[normalizedType] ?? normalizedType
    return [label, normalizedId].filter(Boolean).join(' ') || raw || emptyLabel
  }
  return raw || emptyLabel
}

function metadataSummary(metadata?: Record<string, unknown>) {
  if (!metadata || !Object.keys(metadata).length) return []
  return Object.entries(metadata)
    .filter(([, value]) => value !== undefined && value !== null && value !== '')
    .slice(0, 6)
    .map(([key, value]) => `${key}: ${formatMetadataValue(value)}`)
}

function metadataSearchValues(metadata?: Record<string, unknown>): string[] {
  if (!metadata) return []
  return Object.entries(metadata).flatMap(([key, value]) => [key, formatMetadataValue(value)])
}

function formatMetadataValue(value: unknown) {
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

function unique(values: string[]) {
  return Array.from(new Set(values))
}

function auditDateTime(value?: string) {
  const input = value ?? ''
  const match = input.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!match) return input || '-'
  return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
}

function dateParts(now: Date | string) {
  if (typeof now === 'string') {
    const match = now.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
    if (match) return { date: `${match[1]}${match[2]}${match[3]}`, time: `${match[4]}${match[5]}` }
  }
  const date = typeof now === 'string' ? new Date(now) : now
  if (Number.isNaN(date.getTime())) return { date: 'unknown-date', time: 'unknown-time' }
  const pad = (value: number) => String(value).padStart(2, '0')
  return {
    date: `${date.getUTCFullYear()}${pad(date.getUTCMonth() + 1)}${pad(date.getUTCDate())}`,
    time: `${pad(date.getUTCHours())}${pad(date.getUTCMinutes())}`,
  }
}

function csvCell(value: string | number) {
  const text = String(value)
  if (!/[",\n\r]/.test(text)) return text
  return `"${text.replace(/"/g, '""')}"`
}

function normalizeRaw(value?: string | null) {
  return (value ?? '').trim()
}
