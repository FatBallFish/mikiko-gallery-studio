import { useEffect, useMemo, useState } from 'react'
import type { ProviderHealth } from '../../../shared/api-types'
import { mockApi } from '../../../shared/mock-api'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader } from '../components'

const probeRows = [
  { name: 'API Gateway', scope: '/api/agent + /api/ops', status: 'healthy', detail: 'P95 118ms · error budget 99.95%' },
  { name: 'Postgres', scope: 'primary + read replica', status: 'healthy', detail: 'lag 21ms · connections 42/180' },
  { name: 'Redis Queue', scope: 'lease + polling', status: 'healthy', detail: 'pending 06 · running 18' },
  { name: 'Object Storage', scope: 'reference + result assets', status: 'healthy', detail: 'signed URL latency 88ms' },
]

export function HealthPage() {
  const [rows, setRows] = useState<ProviderHealth[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      setRows(await mockApi.getHealth())
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '健康探针载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const degraded = useMemo(() => rows.filter((row) => row.status !== 'healthy').length, [rows])

  if (loading) return <LoadingBlock label="执行系统探针" />
  if (error) return <ErrorBlock message={error} onRetry={load} />
  if (!rows.length) return <EmptyBlock title="暂无健康数据" detail="Provider 探针未返回结果。" />

  return (
    <section className="page-stack">
      <PageHeader eyebrow="Health" title="系统健康" detail="Provider、队列、存储、数据库和网关探针合并展示，便于值班快速定位。" actions={<button type="button" className="btn" onClick={() => void load()}>重新探测</button>} />
      <section className="ops-status-strip compact-strip">
        <div className="status-cell"><label>Provider 异常</label><strong>{degraded}</strong></div>
        <div className="status-cell"><label>队列压力</label><strong>{rows.find((row) => row.provider === 'Task Worker')?.note ?? '正常'}</strong></div>
        <div className="status-cell"><label>刷新时间</label><strong>{new Date().toLocaleTimeString('zh-CN', { hour12: false })}</strong></div>
        <div className="status-cell"><label>巡检策略</label><strong>30s interval</strong></div>
      </section>
      <section className="pg-admin-card ops-surface full-main">
        <section className="main-lane table-lane no-divider">
          <div className="table-head health-grid"><span>探针</span><span>状态</span><span>延迟</span><span>错误率</span><span>说明</span></div>
          {rows.map((row) => (
            <div key={row.provider} className="table-row health-grid">
              <strong>{row.provider}</strong>
              <Badge tone={row.status === 'healthy' ? 'success' : row.status === 'degraded' ? 'warning' : 'danger'}>{row.status}</Badge>
              <code>{row.latency_ms}ms</code>
              <code>{row.error_rate}</code>
              <span>{row.note}</span>
            </div>
          ))}
          <div className="lane-divider" />
          <div className="table-head health-grid"><span>基础设施</span><span>状态</span><span>范围</span><span>探针</span><span>说明</span></div>
          {probeRows.map((row) => (
            <div key={row.name} className="table-row health-grid">
              <strong>{row.name}</strong>
              <Badge tone="success">{row.status}</Badge>
              <span>{row.scope}</span>
              <code>pass</code>
              <span>{row.detail}</span>
            </div>
          ))}
        </section>
      </section>
    </section>
  )
}
