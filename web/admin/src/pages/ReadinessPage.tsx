import { useEffect, useMemo, useState } from 'react'
import type { ReadinessReport } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader, StatusCell, StatusStrip } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { adminDataGrid, adminGridCols } from '../ui/dataGrid'
import { readinessOverallStatusLabel, readinessRows, type ReadinessRowModel } from './readinessRows'

export function ReadinessPage() {
  const [report, setReport] = useState<ReadinessReport | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  async function load() {
    setLoading(true)
    setError(null)
    try {
      setReport(await adminApi.getReadiness())
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '上线检查载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [])

  const summary = useMemo(() => ({
    pass: report?.summary?.pass ?? report?.checks.filter((item) => item.status === 'pass').length ?? 0,
    warn: report?.summary?.warn ?? report?.checks.filter((item) => item.status === 'warn').length ?? 0,
    fail: report?.summary?.fail ?? report?.checks.filter((item) => item.status === 'fail').length ?? 0,
  }), [report])

  if (loading) return <LoadingBlock label="执行上线检查" />
  if (error) return <ErrorBlock message={error} onRetry={load} />
  if (!report) return <EmptyBlock title="暂无上线检查" detail="后台尚未返回 readiness 报告。" />

  return (
    <section className={adminPage.stack}>
      <PageHeader
        eyebrow="Readiness"
        title="上线检查"
        detail="聚合模型账号、路由模型、价格、支付渠道、公开广场与文档接口的上线阻塞项。"
        actions={<button type="button" className={adminButton.base} onClick={() => void load()}>重新检查</button>}
      />
      <StatusStrip columns={4}>
        <StatusCell label="整体状态" value={readinessOverallStatusLabel(report.status)} />
        <StatusCell label="通过" value={summary.pass} />
        <StatusCell label="警告" value={summary.warn} />
        <StatusCell label="阻塞" value={summary.fail} />
      </StatusStrip>
      <section className={adminPage.fullSurface}>
        <section className={adminPage.mainLane}>
          <div className={adminDataGrid.root}>
            <div className={cn(adminDataGrid.head, adminGridCols.readiness)}><span>检查项</span><span>状态</span><span>阻塞</span><span>修复入口</span><span>说明</span></div>
            {readinessRows(report.checks).map((check) => <ReadinessRow key={check.key} check={check} />)}
          </div>
        </section>
      </section>
    </section>
  )
}

function ReadinessRow({ check }: { check: ReadinessRowModel }) {
  return (
    <div className={cn(adminDataGrid.row, adminGridCols.readiness)}>
      <div className={adminDataGrid.stackCell}>
        <strong>{check.label}</strong>
        <p className={adminDataGrid.detail}>{check.key}</p>
      </div>
      <Badge tone={check.statusTone}>{check.status}</Badge>
      <Badge tone={check.blockingTone}>{check.blockingLabel}</Badge>
      <a className={cn(adminButton.base, adminButton.small)} href={check.actionHref}>{check.actionLabel}</a>
      <span className={adminDataGrid.cell}>{check.detail}</span>
    </div>
  )
}
