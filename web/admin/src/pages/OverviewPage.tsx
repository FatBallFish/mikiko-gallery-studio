import { useEffect, useState } from 'react'
import type { AdminDashboardOperations, AdminMetric, AdminUser, AuditLog, ProviderHealth, ReadinessReport } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, MetricGrid } from '../components'
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
    <section className="content page-stack">
      <header className="header page-header">
        <div>
          <h1>运营总览</h1>
          <p>核心指标、注册用户与后台运营入口集中在同一条扫描路径里。</p>
        </div>
      </header>
      <MetricGrid metrics={metricRows} />
      <ReadinessRiskPanel report={data.readiness} risks={readinessRisks} />

      <section className="card pg-admin-card ops-surface full-main">
        <section className="main-lane table-lane no-divider">
          <div className="card-header lane-head compact">
            <div className="card-title">最新注册用户</div>
            <a className="btn" href="#/users">查看全部</a>
          </div>
          <table className="ds-table" aria-label="最新注册用户">
            <thead>
              <tr>
                <th>用户</th>
                <th>邮箱</th>
                <th>积分余额</th>
                <th>状态</th>
                <th>注册时间</th>
                <th style={{ textAlign: 'right' }}>操作</th>
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
                  <td style={{ textAlign: 'right' }}><a className="btn small" href={user.actionHref}>{user.actionLabel}</a></td>
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
    <section className="card pg-admin-card ops-surface full-main">
      <section className="main-lane table-lane no-divider">
        <div className="card-header lane-head compact">
          <div>
            <div className="card-title">上线检查风险</div>
            <p>阻塞 {summary.fail} 项 / 警告 {summary.warn} 项 / 通过 {summary.pass} 项</p>
          </div>
          <a className="btn" href="#/readiness">查看全部</a>
        </div>
        {risks.length === 0 ? (
          <EmptyBlock title="暂无上线风险" detail="当前关键配置检查均已通过。" />
        ) : (
          <div className="admin-data-grid overview-readiness-grid">
            <div className="table-head"><span>检查项</span><span>状态</span><span>说明</span><span>入口</span></div>
            {risks.map((risk) => (
              <div className="table-row" key={risk.key}>
                <div>
                  <strong>{risk.label}</strong>
                  <p>{risk.key}</p>
                </div>
                <Badge tone={risk.statusTone}>{risk.status}</Badge>
                <span>{risk.detail}</span>
                <a className="btn small" href={risk.actionHref}>{risk.actionLabel}</a>
              </div>
            ))}
          </div>
        )}
      </section>
    </section>
  )
}
