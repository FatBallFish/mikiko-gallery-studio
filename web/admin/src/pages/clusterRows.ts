import type { ClusterNode } from '../../../shared/api-types'
import type { ToastTone } from '../types'

export type ClusterNodeRow = ClusterNode & {
  roleLabel: string
  healthLabel: string
  healthTone: ToastTone | 'success' | 'primary'
  driftLabel: string
  actionLabel: string
  lastContactLabel: string
}

const roleLabels: Record<string, string> = {
  single: '单节点',
  control: '控制节点',
  api: 'API 节点',
  worker: 'Worker 节点',
  web: 'Web 节点',
}

const healthViews: Record<string, { label: string; tone: ClusterNodeRow['healthTone']; action: string }> = {
  healthy: { label: '运行正常', tone: 'success', action: '-' },
  joining: { label: '等待心跳', tone: 'warning', action: '确认节点服务已启动' },
  degraded: { label: '运行降级', tone: 'warning', action: '对齐节点版本与配置' },
  unready: { label: '未就绪', tone: 'danger', action: '检查运行时配置与 Schema' },
  offline: { label: '离线', tone: 'danger', action: '检查节点服务与网络' },
}

export function clusterNodeRows(nodes: ClusterNode[]): ClusterNodeRow[] {
  return nodes.map((node) => {
    const health = healthViews[node.effective_health] ?? { label: node.effective_health || '未知', tone: 'neutral' as const, action: '检查节点状态' }
    const drift = [
      node.application_version_drift ? '应用版本' : '',
      node.runtime_schema_drift ? '运行时 Schema' : '',
      node.config_revision_drift ? '配置版本' : '',
    ].filter(Boolean)
    return {
      ...node,
      roleLabel: roleLabels[node.role] ?? node.role,
      healthLabel: health.label,
      healthTone: health.tone,
      driftLabel: drift.length ? drift.join('、') : '一致',
      actionLabel: health.action,
      lastContactLabel: formatClusterContact(node.last_heartbeat_at ?? node.created_at),
    }
  })
}

export function clusterSummary(nodes: ClusterNode[]) {
  return {
    total: nodes.length,
    healthy: nodes.filter((node) => node.effective_health === 'healthy').length,
    attention: nodes.filter((node) => node.effective_health !== 'healthy').length,
    drifted: nodes.filter((node) => node.application_version_drift || node.runtime_schema_drift || node.config_revision_drift).length,
  }
}

function formatClusterContact(value?: string | null) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}
