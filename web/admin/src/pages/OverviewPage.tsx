import { useEffect, useState } from 'react'
import type { AdminDashboardOperations, AdminMetric, AdminUser, AuditLog, ProviderHealth, ReadinessReport } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, MetricGrid, PageHeader } from '../components'
import { adminButton, adminPage, adminSurface } from '../ui/classes'
import { overviewReadinessRows, type OverviewReadinessRow } from './overviewReadinessRows'
import { overviewMetricRows, overviewRecentUserRows } from './overviewRows'

type DashboardData = {
  operations: AdminDashboardOperations
  metrics: AdminMetric[]
  providers: ProviderHealth[]
  queue: Array<{ item: string; count: string; detail: string }>
  audit: AuditLog[]
  users: AdminUser[]
  readiness: ReadinessReport
}

const overviewClasses = {
  content: adminPage.scrollStack,
  surface: cn(adminSurface.card, 'full-main p-0'),
  lane: 'min-w-0 overflow-auto p-5',
  panelHead: 'mb-4 flex flex-wrap items-center justify-between gap-3',
  panelTitle: 'text-sm font-extrabold text-[var(--text)]',
  panelDetail: 'm-0 mt-1 text-sm text-[var(--soft)]',
  table: 'ds-table w-full border-collapse text-sm',
  tableHeadRight: 'text-right',
  tableCellRight: 'text-right',
  dataGrid: 'grid min-w-[760px] overflow-hidden rounded-[var(--pg-radius-sm)] border border-[var(--line)]',
  readinessGrid: '[grid-template-columns:minmax(180px,1.6fr)_minmax(80px,.7fr)_minmax(300px,2.4fr)_minmax(100px,.8fr)]',
  dataHead: 'grid border-b border-[var(--line)] bg-[var(--pg-admin-bg-subtle)] text-[10px] font-extrabold uppercase tracking-[.14em] text-[var(--soft)]',
  dataRow: 'grid border-b border-[var(--line)] last:border-b-0',
  dataCell: 'min-w-0 px-3 py-3',
  keyText: 'm-0 mt-1 font-mono text-xs text-[var(--soft)]',
  detailText: 'min-w-0 px-3 py-3 text-sm text-[var(--soft)] [overflow-wrap:anywhere]',
}

export function OverviewPage() {
  const [data, setData] = useState<DashboardData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const [dashboard, users, readiness] = await Promise.all([adminApi.dashboard(), adminApi.listUsers(), adminApi.getReadiness()])
      setData({
        ...dashboard,
        readiness,
        users,
      })
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '无法载入总览')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  if (loading) return <LoadingBlock />
  if (error) return <ErrorBlock message={error} onRetry={load} />
  if (!data) return <EmptyBlock title="暂无总览数据" detail="后台接口未返回指标。" />

  const readinessRisks = overviewReadinessRows(data.readiness.checks)
  const metricRows = overviewMetricRows(data.metrics)
  const recentUsers = overviewRecentUserRows(data.users)

  return (
    <section className={overviewClasses.content}>
      <PageHeader eyebrow="OVERVIEW" title="运营总览" detail="核心指标、注册用户与后台运营入口集中在同一条扫描路径里。" />
      <MetricGrid metrics={metricRows} />
      <ReadinessRiskPanel report={data.readiness} risks={readinessRisks} />

      <section className={overviewClasses.surface}>
        <section className={overviewClasses.lane}>
          <div className={overviewClasses.panelHead}>
            <div className={overviewClasses.panelTitle}>最新注册用户</div>
            <a className={adminButton.base} href="#/users">查看全部</a>
          </div>
          <table className={overviewClasses.table} aria-label="最新注册用户">
            <thead>
              <tr>
                <th>用户</th>
                <th>邮箱</th>
                <th>积分余额</th>
                <th>状态</th>
                <th>注册时间</th>
                <th className={overviewClasses.tableHeadRight}>操作</th>
              </tr>
            </thead>
            <tbody>
              {recentUsers.map((user) => (
                <tr key={user.id}>
                  <td><strong>{user.displayName}</strong></td>
                  <td>{user.email}</td>
                  <td><code>{user.balance}</code></td>
                  <td><Badge tone={user.statusTone}>{user.statusLabel}</Badge></td>
                  <td>{user.createdAt}</td>
                  <td className={overviewClasses.tableCellRight}><a className={cn(adminButton.base, adminButton.small)} href={user.actionHref}>{user.actionLabel}</a></td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      </section>
    </section>
  )
}

function ReadinessRiskPanel({ report, risks }: { report: ReadinessReport; risks: OverviewReadinessRow[] }) {
  const summary = report.summary ?? {
    pass: report.checks.filter((item) => item.status === 'pass').length,
    warn: report.checks.filter((item) => item.status === 'warn').length,
    fail: report.checks.filter((item) => item.status === 'fail').length,
  }
  return (
    <section className={overviewClasses.surface}>
      <section className={overviewClasses.lane}>
        <div className={overviewClasses.panelHead}>
          <div>
            <div className={overviewClasses.panelTitle}>上线检查风险</div>
            <p className={overviewClasses.panelDetail}>阻塞 {summary.fail} 项 / 警告 {summary.warn} 项 / 通过 {summary.pass} 项</p>
          </div>
          <a className={adminButton.base} href="#/readiness">查看全部</a>
        </div>
        {risks.length === 0 ? (
          <EmptyBlock title="暂无上线风险" detail="当前关键配置检查均已通过。" />
        ) : (
          <div className={cn(overviewClasses.dataGrid, overviewClasses.readinessGrid)}>
            <div className={cn(overviewClasses.dataHead, overviewClasses.readinessGrid)}><span className={overviewClasses.dataCell}>检查项</span><span className={overviewClasses.dataCell}>状态</span><span className={overviewClasses.dataCell}>说明</span><span className={overviewClasses.dataCell}>入口</span></div>
            {risks.map((risk) => (
              <div className={cn(overviewClasses.dataRow, overviewClasses.readinessGrid)} key={risk.key}>
                <div className={overviewClasses.dataCell}>
                  <strong>{risk.label}</strong>
                  <p className={overviewClasses.keyText}>{risk.key}</p>
                </div>
                <div className={overviewClasses.dataCell}><Badge tone={risk.statusTone}>{risk.status}</Badge></div>
                <span className={overviewClasses.detailText}>{risk.detail}</span>
                <div className={overviewClasses.dataCell}><a className={cn(adminButton.base, adminButton.small)} href={risk.actionHref}>{risk.actionLabel}</a></div>
              </div>
            ))}
          </div>
        )}
      </section>
    </section>
  )
}
