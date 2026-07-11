import { useEffect, useState } from 'react'
import type { AdminDashboardOperations, AdminMetric, AdminUser, AuditLog, ProviderHealth, ReadinessReport } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, MetricStrip, PageHeader } from '../components'
import { adminButton, adminPage, adminSurface } from '../ui/classes'
import { overviewReadinessRows, type OverviewReadinessRow } from './overviewReadinessRows'
import { overviewRecentUserRows } from './overviewRows'

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
  table: 'admin-table min-w-0',
  tableHeadRight: 'text-right',
  tableCellRight: 'text-right',
  dataGrid: 'grid min-w-[760px] overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--surface)]',
  readinessGrid: '[grid-template-columns:minmax(180px,1.6fr)_minmax(80px,.7fr)_minmax(300px,2.4fr)_minmax(100px,.8fr)]',
  dataHead: 'grid border-b border-[var(--border)] bg-[var(--surface-solid)] text-[length:var(--admin-type-label)] font-semibold text-[var(--muted-strong)]',
  dataRow: 'grid border-b border-[var(--border)] last:border-b-0',
  dataCell: 'min-w-0 px-3 py-3',
  keyText: 'm-0 mt-1 font-mono text-xs text-[var(--soft)]',
  detailText: 'min-w-0 px-3 py-3 text-sm text-[var(--soft)] [overflow-wrap:anywhere]',
  insightGrid: 'grid grid-flow-dense grid-cols-12 gap-4 max-lg:grid-cols-1',
  chartPanel: cn(adminSurface.card, 'min-w-0 p-5 lg:col-span-8'),
  rankPanel: cn(adminSurface.card, 'min-w-0 p-5 lg:col-span-4'),
  sectionHeader: 'mb-5 flex flex-wrap items-center justify-between gap-3 border-b border-[var(--line)] pb-4',
  sectionTitle: 'text-base font-semibold text-[var(--fg)]',
  distributionBox: 'grid min-h-[220px] content-center gap-4 rounded-xl border border-[var(--border)] bg-[var(--surface-solid)] p-5',
  distributionRow: 'grid gap-2',
  distributionMeta: 'flex justify-between gap-3 text-xs font-bold',
  distributionTrack: 'h-1.5 overflow-hidden rounded-full bg-white/5',
  distributionFill: 'h-full rounded-full bg-[var(--accent)]',
  modelStats: 'mt-6 grid grid-cols-3 gap-4 max-[760px]:grid-cols-1',
  modelStat: 'rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-4',
  modelName: 'mb-1 text-[length:var(--admin-type-label)] font-semibold text-[var(--muted-strong)]',
  modelValue: 'font-[family-name:var(--admin-font-mono)] text-lg font-semibold tabular-nums text-[var(--text)]',
  modelDetail: 'mt-1 text-xs text-[var(--accent)]',
  rankList: 'grid gap-2',
  rankItem: 'group flex items-center justify-between gap-3 rounded-lg border border-transparent p-3 transition-all hover:border-[var(--border)] hover:bg-[var(--surface-solid)]',
  rankAvatar: 'grid size-8 shrink-0 place-items-center rounded-lg bg-[var(--surface-solid)] text-xs font-bold text-[var(--muted-strong)]',
  rankName: 'text-sm font-bold text-[var(--text)] transition-colors group-hover:text-[var(--accent)]',
  rankMeta: 'text-xs text-[var(--muted-strong)]',
  rankValue: 'font-[family-name:var(--admin-font-mono)] text-sm font-semibold tabular-nums text-[var(--green)]',
  alertList: 'grid gap-2',
  alertItem: 'grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 rounded-lg border border-[var(--border)] bg-[var(--canvas)] px-3 py-2.5',
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
  const metricRows = overviewHeroMetricRows(data.metrics, data.operations)
  const recentUsers = overviewRecentUserRows(data.users)

  return (
    <section className={overviewClasses.content}>
      <PageHeader title="运营总览" description="今日生成、积分消耗、待处理风险与关键运营明细。" />
      <MetricStrip metrics={metricRows} />
      <OperationsInsightPanel providers={data.providers} users={data.users} operations={data.operations} queue={data.queue} risks={readinessRisks} />
      <section className="grid gap-4" aria-label="运营支持信息">
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
    </section>
  )
}

function OperationsInsightPanel({
  providers,
  users,
  operations,
  queue,
  risks,
}: {
  providers: ProviderHealth[]
  users: AdminUser[]
  operations: AdminDashboardOperations
  queue: Array<{ item: string; count: string; detail: string }>
  risks: OverviewReadinessRow[]
}) {
  const providerRows = providerDistributionRows(providers)
  const topUsers = users
    .slice()
    .sort((left, right) => Number(right.balance || 0) - Number(left.balance || 0))
    .slice(0, 5)

  return (
    <section className={overviewClasses.insightGrid}>
      <div className={overviewClasses.chartPanel}>
        <div className={overviewClasses.sectionHeader}>
          <h3 className={overviewClasses.sectionTitle}>模型调用分布</h3>
        </div>
        <div className={overviewClasses.distributionBox}>
          {providerRows.length ? providerRows.map((row) => (
            <div key={row.label} className={overviewClasses.distributionRow}>
              <div className={overviewClasses.distributionMeta}>
                <span className="min-w-0 truncate text-[var(--muted)]">{row.label}</span>
                <span className="text-[var(--text)]">{row.value} ({row.percent}%)</span>
              </div>
              <div className={overviewClasses.distributionTrack}>
                <div className={overviewClasses.distributionFill} style={{ width: `${row.percent}%` }} />
              </div>
            </div>
          )) : <EmptyBlock variant="inline" title="暂无模型调用" detail="配置模型账号并产生调用后展示分布。" />}
        </div>
        <div className={overviewClasses.modelStats}>
          <ModelStat name="上游健康" value={providerHealthLabel(providers)} detail={`${providers.length} 个上游实例`} />
          <ModelStat name="前置失败" value={String(operations.preflight_failure_count)} detail="生成前校验阻断" />
          <ModelStat name="广场访问" value={String(operations.public_gallery_list_views)} detail="公开广场浏览" />
        </div>
      </div>
      <OperationalAlertRail queue={queue} risks={risks} topUsers={topUsers} />
    </section>
  )
}

function OperationalAlertRail({ queue, risks, topUsers }: { queue: Array<{ item: string; count: string; detail: string }>; risks: OverviewReadinessRow[]; topUsers: AdminUser[] }) {
  const pending = queue.filter((item) => Number(item.count) > 0)
  return (
    <aside className={overviewClasses.rankPanel} aria-label="待处理与用户排行">
      <div className={overviewClasses.sectionHeader}><h3 className={overviewClasses.sectionTitle}>待处理</h3></div>
      <div className={overviewClasses.alertList}>
        {pending.map((item) => (
          <div key={item.item} className={overviewClasses.alertItem}>
            <div className="min-w-0"><strong className="block truncate text-sm">{item.item}</strong><span className="text-xs text-[var(--soft)]">{item.detail}</span></div>
            <Badge tone="warning">{item.count}</Badge>
          </div>
        ))}
        {risks.slice(0, 3).map((risk) => (
          <a key={risk.key} className={cn(overviewClasses.alertItem, 'text-inherit no-underline hover:border-[var(--border-strong)]')} href={risk.actionHref}>
            <div className="min-w-0"><strong className="block truncate text-sm">{risk.label}</strong><span className="text-xs text-[var(--soft)]">{risk.detail}</span></div>
            <Badge tone={risk.statusTone}>{risk.status}</Badge>
          </a>
        ))}
        {!pending.length && !risks.length ? <EmptyBlock variant="inline" title="暂无待处理事项" detail="当前队列和上线检查没有需要立即处理的项目。" /> : null}
      </div>
      <div className="mb-3 mt-5 border-t border-[var(--border)] pt-4"><h3 className={overviewClasses.sectionTitle}>用户消费榜</h3></div>
      <div className={overviewClasses.rankList}>
        {topUsers.length ? topUsers.map((user) => <UserRank key={user.id} user={user} />) : <EmptyBlock variant="inline" title="暂无用户排行" detail="用户产生余额或消费后会出现在这里。" />}
      </div>
    </aside>
  )
}

function ModelStat({ name, value, detail }: { name: string; value: string; detail: string }) {
  return (
    <div className={overviewClasses.modelStat}>
      <div className={overviewClasses.modelName}>{name}</div>
      <div className={overviewClasses.modelValue}>{value}</div>
      <div className={overviewClasses.modelDetail}>{detail}</div>
    </div>
  )
}

function UserRank({ user }: { user: AdminUser }) {
  const name = user.display_name?.trim() || user.email
  return (
    <div className={overviewClasses.rankItem}>
      <div className="flex min-w-0 items-center gap-3">
        <div className={overviewClasses.rankAvatar}>{name.slice(0, 1).toUpperCase()}</div>
        <div className="min-w-0">
          <div className={overviewClasses.rankName}>{name}</div>
          <div className={overviewClasses.rankMeta}>{user.email}</div>
        </div>
      </div>
      <div className="shrink-0 text-right">
        <div className={overviewClasses.rankValue}>{Number(user.balance || 0).toFixed(2)}</div>
        <div className={overviewClasses.rankMeta}>积分</div>
      </div>
    </div>
  )
}

function overviewHeroMetricRows(metrics: AdminMetric[], operations: AdminDashboardOperations): AdminMetric[] {
  const byKey = new Map(metrics.map((metric) => [metric.key ?? metric.label, metric]))
  const activeUsers = byKey.get('active_users')
  const generationSuccessRate = byKey.get('generation_success_rate')
  const actualPoints = byKey.get('actual_points')

  return [
    activeUsers
      ? { ...activeUsers, label: '总用户数', trend: activeUsers.trend || '当前可登录用户' }
      : { key: 'active_users', label: '总用户数', value: '0', trend: '当前可登录用户', tone: 'neutral' },
    {
      key: 'generation_success_rate',
      label: '生成成功率',
      value: generationSuccessRate?.value ?? '-',
      trend: generationSuccessRate?.trend ?? '暂无调用',
      tone: generationSuccessRate?.tone ?? 'neutral',
    },
    {
      key: 'today_points',
      label: '今日消耗积分',
      value: actualPoints?.value ?? '0.00000',
      trend: operations.preflight_failure_count ? `${operations.preflight_failure_count} 次前置失败` : '生成链路正常',
      tone: actualPoints?.tone ?? 'neutral',
    },
    {
      key: 'preflight_failures',
      label: '生成前置失败',
      value: String(operations.preflight_failure_count),
      trend: operations.preflight_failure_count ? '需要检查参数、余额或路由' : '当前无阻断',
      tone: operations.preflight_failure_count ? 'warn' : 'good',
    },
  ]
}

function providerDistributionRows(providers: ProviderHealth[]) {
  const rows = providers
  const total = rows.reduce((sum, row) => sum + providerWeight(row), 0) || 1
  return rows.slice(0, 4).map((row) => {
    const value = providerWeight(row)
    return {
      label: row.provider || row.provider_code || 'Provider',
      value: `${row.latency_ms || 0}ms`,
      percent: Math.max(4, Math.round((value / total) * 100)),
    }
  })
}

function providerWeight(provider: Pick<ProviderHealth, 'latency_ms' | 'status'>) {
  const latency = Number(provider.latency_ms || 0)
  const health = provider.status === 'healthy' ? 100 : provider.status === 'degraded' ? 45 : 12
  return Math.max(8, health - Math.min(60, latency / 10))
}

function providerHealthLabel(providers: ProviderHealth[]) {
  if (!providers.length) return '0 / 0'
  const healthy = providers.filter((provider) => provider.status === 'healthy').length
  return `${healthy} / ${providers.length}`
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
          <a className={adminButton.base} href="#/monitoring">查看全部</a>
        </div>
        {risks.length === 0 ? (
          <EmptyBlock variant="inline" title="暂无上线风险" detail="当前关键配置检查均已通过。" />
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
