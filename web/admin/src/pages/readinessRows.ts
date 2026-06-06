import type { ReadinessCheck } from '../../../shared/api-types'

function tone(status: string): 'success' | 'warning' | 'danger' {
  if (status === 'pass') return 'success'
  if (status === 'warn') return 'warning'
  return 'danger'
}

export type ReadinessRowModel = {
  key: string
  label: string
  status: string
  rawStatus: string
  statusTone: 'success' | 'warning' | 'danger'
  blockingLabel: string
  blockingTone: 'success' | 'danger'
  detail: string
  actionHref: string
  actionLabel: string
}

export function readinessStatusLabel(status: string) {
  if (status === 'pass') return '通过'
  if (status === 'warn') return '警告'
  if (status === 'fail') return '阻塞'
  return status || '未知'
}

export function readinessOverallStatusLabel(status: string) {
  if (status === 'pass') return '全部通过'
  if (status === 'warn') return '存在警告'
  if (status === 'fail') return '存在阻塞'
  return status || '未知'
}

export function readinessRows(checks: ReadinessCheck[]): ReadinessRowModel[] {
  return checks.map((check) => {
    const route = check.fix_route ?? check.action_route ?? 'readiness'
    return {
      key: check.key,
      label: check.label,
      status: readinessStatusLabel(check.status),
      rawStatus: check.status,
      statusTone: tone(check.status),
      blockingLabel: check.blocking ? '阻塞上线' : '非阻塞',
      blockingTone: check.blocking ? 'danger' : 'success',
      detail: check.detail || check.summary || '-',
      actionHref: `#/${route}`,
      actionLabel: check.fix_action ?? check.action_label ?? '查看',
    }
  })
}
