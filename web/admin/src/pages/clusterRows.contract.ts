import type { ClusterNode } from '../../../shared/api-types'
import { clusterNodeRows, clusterSummary } from './clusterRows'

const nodes: ClusterNode[] = [
  {
    node_id: 'control-a', installation_id: 'installation-a', role: 'control', source: 'heartbeat',
    application_version: 'v2', runtime_schema_version: 2, config_revision: 9,
    health: 'healthy', effective_health: 'healthy', last_heartbeat_at: '2026-07-23T12:00:00Z',
    application_version_drift: false, runtime_schema_drift: false, config_revision_drift: false,
    created_at: '2026-07-23T11:00:00Z', updated_at: '2026-07-23T12:00:00Z',
  },
  {
    node_id: 'worker-old', installation_id: 'installation-a', role: 'worker', source: 'heartbeat',
    application_version: 'v1', runtime_schema_version: 1, config_revision: 8,
    health: 'healthy', effective_health: 'offline', last_heartbeat_at: '2026-07-23T11:50:00Z',
    application_version_drift: true, runtime_schema_drift: true, config_revision_drift: true,
    created_at: '2026-07-23T10:00:00Z', updated_at: '2026-07-23T11:50:00Z',
  },
]

const rows = clusterNodeRows(nodes)
if (rows[0]?.roleLabel !== '控制节点' || rows[0]?.healthLabel !== '运行正常' || rows[0]?.healthTone !== 'success') {
  throw new Error(`healthy control row is invalid: ${JSON.stringify(rows[0])}`)
}
if (rows[1]?.healthLabel !== '离线' || rows[1]?.healthTone !== 'danger' || rows[1]?.actionLabel !== '检查节点服务与网络') {
  throw new Error(`offline worker row is not actionable: ${JSON.stringify(rows[1])}`)
}
if (rows[1]?.driftLabel !== '应用版本、运行时 Schema、配置版本') {
  throw new Error(`node drift summary is incomplete: ${JSON.stringify(rows[1])}`)
}
if (rows[0]?.sourceLabel !== '心跳注册') {
  throw new Error(`heartbeat node source is not visible: ${JSON.stringify(rows[0])}`)
}
const summary = clusterSummary(nodes)
if (summary.total !== 2 || summary.healthy !== 1 || summary.attention !== 1 || summary.drifted !== 1) {
  throw new Error(`cluster summary is invalid: ${JSON.stringify(summary)}`)
}
