import { useEffect, useMemo, useState } from 'react'
import type { ProviderHealth, ReadinessReport } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader, StatusCell, StatusStrip } from '../components'
import { healthProviderRows, healthReadinessRows, healthRefreshTimeLabel, refreshPolicyLabel, taskQueuePressure } from '../healthRows'
import { adminButton, adminPage } from '../ui/classes'
import { adminDataGrid, adminGridCols } from '../ui/dataGrid'

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
    <section className={adminPage.stack}>
      <PageHeader eyebrow="Health" title="系统健康" detail="Provider 探针与上线检查合并展示，便于值班快速定位真实配置和运行风险。" actions={<button type="button" className={adminButton.base} onClick={() => void load()}>重新探测</button>} />
      <StatusStrip columns={4}>
        <StatusCell label="Provider 异常" value={degraded} />
        <StatusCell label="队列压力" value={taskQueuePressure(rows)} />
        <StatusCell label="刷新时间" value={healthRefreshTimeLabel(readiness?.generated_at)} />
        <StatusCell label="巡检策略" value={refreshPolicyLabel('30s interval')} />
      </StatusStrip>
      <section className={adminPage.fullSurface}>
        <section className={adminPage.mainLane}>
          <div className={adminDataGrid.root}>
            <div className={cn(adminDataGrid.head, adminGridCols.health)}><span>探针</span><span>状态</span><span>延迟</span><span>错误率</span><span>说明</span></div>
            {providerRows.map((row) => (
              <div key={row.key} className={cn(adminDataGrid.row, adminGridCols.health)}>
                <strong>{row.name}</strong>
                <Badge tone={row.statusTone}>{row.statusLabel}</Badge>
                <code className={adminDataGrid.code}>{row.latencyLabel}</code>
                <code className={adminDataGrid.code}>{row.errorRate}</code>
                <span className={adminDataGrid.cell}>{row.note}</span>
              </div>
            ))}
          </div>
          <div className="my-4 h-px bg-[var(--line)]" />
          <div className={adminDataGrid.root}>
            <div className={cn(adminDataGrid.head, adminGridCols.health)}><span>运行检查</span><span>状态</span><span>范围</span><span>上线影响</span><span>说明</span></div>
            {readinessRunRows.map((row) => (
              <div key={row.key} className={cn(adminDataGrid.row, adminGridCols.health)}>
                <strong>{row.name}</strong>
                <Badge tone={row.statusTone}>{row.statusLabel}</Badge>
                <span className={adminDataGrid.cell}>{row.scope}</span>
                <a className={adminDataGrid.cell} href={row.actionHref}>{row.probeLabel} · {row.actionLabel}</a>
                <span className={adminDataGrid.cell}>{row.detail}</span>
              </div>
            ))}
          </div>
        </section>
      </section>
    </section>
  )
}
