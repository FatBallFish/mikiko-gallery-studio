import type { ReadinessCheck } from '../../../shared/api-types'

export type OverviewReadinessRow = {
  key: string
  label: string
  status: string
  rawStatus: string
  statusTone: 'success' | 'danger' | 'warning'
  detail: string
  actionHref: string
  actionLabel: string
}

function readinessTone(status: string): 'success' | 'danger' | 'warning' {
  if (status === 'pass') return 'success'
  if (status === 'fail') return 'danger'
  return 'warning'
}

function readinessStatusLabel(status: string) {
  if (status === 'fail') return '阻塞'
  if (status === 'warn') return '警告'
  if (status === 'pass') return '通过'
  return status || '未知'
}

export function overviewReadinessRows(checks: ReadinessCheck[]): OverviewReadinessRow[] {
  return checks
    .filter((item) => item.status === 'fail' || item.status === 'warn')
    .slice(0, 5)
    .map((check) => ({
      key: check.key,
      label: check.label,
      status: readinessStatusLabel(check.status),
      rawStatus: check.status,
      statusTone: readinessTone(check.status),
      detail: check.detail || check.summary || '-',
      actionHref: `#/${check.fix_route ?? check.action_route ?? 'readiness'}`,
      actionLabel: check.fix_action ?? check.action_label ?? '处理',
    }))
}
