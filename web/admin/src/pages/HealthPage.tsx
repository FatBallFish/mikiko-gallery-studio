import { useEffect, useMemo, useState } from 'react'
import type { ProviderHealth, ReadinessReport } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader } from '../components'
import { healthProviderRows, healthReadinessRows, healthRefreshTimeLabel, refreshPolicyLabel, taskQueuePressure } from '../healthRows'

export function HealthPage() {
  const [rows, setRows] = useState<ProviderHealth[]>([])
  const [readiness, setReadiness] = useState<ReadinessReport | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const [dashboard, report] = await Promise.all([adminApi.dashboard(), adminApi.getReadiness()])
      setRows(dashboard.providers)
      setReadiness(report)
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
  const providerRows = healthProviderRows(rows)
  const readinessRunRows = healthReadinessRows(readiness?.checks ?? [])

  if (loading) return <LoadingBlock label="执行系统探针" />
  if (error) return <ErrorBlock message={error} onRetry={load} />
  if (!rows.length && !readinessRunRows.length) return <EmptyBlock title="暂无健康数据" detail="Provider 探针与上线检查均未返回结果。" />

  return (
    <section className="page-stack">
      <PageHeader eyebrow="Health" title="系统健康" detail="Provider 探针与上线检查合并展示，便于值班快速定位真实配置和运行风险。" actions={<button type="button" className="btn" onClick={() => void load()}>重新探测</button>} />
      <section className="ops-status-strip compact-strip">
        <div className="status-cell"><label>Provider 异常</label><strong>{degraded}</strong></div>
        <div className="status-cell"><label>队列压力</label><strong>{taskQueuePressure(rows)}</strong></div>
        <div className="status-cell"><label>刷新时间</label><strong>{healthRefreshTimeLabel(readiness?.generated_at)}</strong></div>
        <div className="status-cell"><label>巡检策略</label><strong>{refreshPolicyLabel('30s interval')}</strong></div>
      </section>
      <section className="pg-admin-card ops-surface full-main">
        <section className="main-lane table-lane no-divider">
          <div className="admin-data-grid health-grid">
            <div className="table-head"><span>探针</span><span>状态</span><span>延迟</span><span>错误率</span><span>说明</span></div>
            {providerRows.map((row) => (
              <div key={row.key} className="table-row">
                <strong>{row.name}</strong>
                <Badge tone={row.statusTone}>{row.statusLabel}</Badge>
                <code>{row.latencyLabel}</code>
                <code>{row.errorRate}</code>
                <span>{row.note}</span>
              </div>
            ))}
          </div>
          <div className="lane-divider" />
          <div className="admin-data-grid health-grid">
            <div className="table-head"><span>运行检查</span><span>状态</span><span>范围</span><span>上线影响</span><span>说明</span></div>
            {readinessRunRows.map((row) => (
              <div key={row.key} className="table-row">
                <strong>{row.name}</strong>
                <Badge tone={row.statusTone}>{row.statusLabel}</Badge>
                <span>{row.scope}</span>
                <a href={row.actionHref}>{row.probeLabel} · {row.actionLabel}</a>
                <span>{row.detail}</span>
              </div>
            ))}
          </div>
        </section>
      </section>
    </section>
  )
}
