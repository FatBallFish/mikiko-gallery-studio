import { useEffect, useRef, useState } from 'react'
import type {
  AdminCallDistribution,
  AdminDashboardOperations,
  AdminMetric,
  AdminMonitoringSnapshot,
  AdminUser,
  AuditLog,
  MonitoringWindow,
  ProviderHealth,
  ReadinessReport,
} from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import {
  Badge,
  EmptyBlock,
  ErrorBlock,
  InlineFeedback,
  LoadingBlock,
  PageHeader,
  RefreshIconButton,
  SegmentedControl,
} from '../components'
import { healthRefreshTimeLabel } from '../healthRows'
import type { ColumnDef } from '../ui/dataTable'
import { DataTable } from '../ui/dataTable'
import { buildTimeSeriesPath, timeSeriesExtent } from '../ui/timeSeriesChart'
import { createLatestListRequestGuard } from './listRefresh'
import { monitoringWindows, formatMonitoringPercent, formatMonitoringQPS, monitoringStateView } from './monitoringRows'
import { overviewReadinessRows, type OverviewReadinessRow } from './overviewReadinessRows'
import { overviewRecentUserRows } from './overviewRows'
import { readinessOverallStatusLabel } from './readinessRows'
import { adminUserRowView } from './userRows'
import './overview-redesign.css'

type DashboardData = {
  operations: AdminDashboardOperations
  call_distribution: AdminCallDistribution
  metrics: AdminMetric[]
  providers: ProviderHealth[]
  queue: Array<{ item: string; count: string; detail: string }>
  audit: AuditLog[]
  users: AdminUser[]
  readiness: ReadinessReport
}

type PendingRow = {
  key: string
  label: string
  detail: string
  countValue: number
  countLabel: string
  tone: 'success' | 'warning' | 'danger' | 'neutral'
  href: string
}

type MetricCard = {
  key: string
  label: string
  value: string
  foot: string
  href: string
  status: { label: string; color: string } | null
}

export function OverviewPage() {
  const [data, setData] = useState<DashboardData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [trendWindow, setTrendWindow] = useState<MonitoringWindow>('60m')
  const [trendSnapshot, setTrendSnapshot] = useState<AdminMonitoringSnapshot | null>(null)
  const [trendLoading, setTrendLoading] = useState(true)
  const [trendError, setTrendError] = useState<string | null>(null)
  const requestGuard = useRef(createLatestListRequestGuard()).current
  const trendRequest = useRef(0)

  const load = async () => {
    const request = requestGuard.begin()
    setLoading(true)
    setError(null)
    try {
      const [dashboard, users, readiness] = await Promise.all([adminApi.dashboard(), adminApi.listUsers(), adminApi.getReadiness()])
      if (!requestGuard.isCurrent(request)) return
      setData({
        ...dashboard,
        readiness,
        users,
      })
    } catch (caught) {
      if (!requestGuard.isCurrent(request)) return
      setError(caught instanceof Error ? caught.message : '无法载入总览')
    } finally {
      if (!requestGuard.isCurrent(request)) return
      setLoading(false)
    }
  }

  const loadTrend = async (targetWindow: MonitoringWindow) => {
    const request = ++trendRequest.current
    setTrendLoading(true)
    setTrendError(null)
    try {
      const snapshot = await adminApi.getMonitoringSnapshot(targetWindow)
      if (request !== trendRequest.current) return
      setTrendSnapshot(snapshot)
    } catch (caught) {
      if (request !== trendRequest.current) return
      setTrendError(caught instanceof Error ? caught.message : '无法载入请求趋势')
    } finally {
      if (request === trendRequest.current) setTrendLoading(false)
    }
  }

  useEffect(() => {
    void load()
    return () => requestGuard.invalidate()
  }, [])

  useEffect(() => {
    void loadTrend(trendWindow)
    return () => {
      trendRequest.current += 1
    }
  }, [trendWindow])

  if (loading && !data) return <LoadingBlock />
  if (error && !data) return <ErrorBlock message={error} onRetry={load} />
  if (!data) return <EmptyBlock title="暂无总览数据" detail="后台接口未返回指标。" />

  const readinessRisks = overviewReadinessRows(data.readiness.checks)
  const metricRows = overviewHeroMetricRows(data.metrics, data.operations)
  const recentUsers = overviewRecentUserRows(data.users)
  const pendingRows = buildPendingRows(data, readinessRisks)
  const pendingTotal = pendingRows.reduce((sum, row) => sum + row.countValue, 0)
  const readinessSummary = data.readiness.summary ?? {
    pass: data.readiness.checks.filter((item) => item.status === 'pass').length,
    warn: data.readiness.checks.filter((item) => item.status === 'warn').length,
    fail: data.readiness.checks.filter((item) => item.status === 'fail').length,
  }
  const readinessStatus = data.readiness.overall_status ?? data.readiness.status
  const enabledProviders = data.providers.filter((provider) => provider.enabled !== false)
  const healthyProviders = enabledProviders.filter((provider) => provider.status === 'healthy')
  const metricCards = buildMetricCards(metricRows)

  return (
    <section className="ov-page">
      <section className="a-status-strip ov-status-strip" aria-label="运营状态条">
        {trendSnapshot ? (
          <div className="a-status-cell">
            <span>API 状态</span>
            <strong>{monitoringStateView(trendSnapshot.state).label}</strong>
          </div>
        ) : null}
        <div className="a-status-cell">
          <span>上线检查</span>
          <strong>{readinessOverallStatusLabel(readinessStatus)}</strong>
        </div>
        <div className="a-status-cell">
          <span>待处理</span>
          <strong>{pendingTotal > 0 ? `${pendingTotal} 项` : '无待处理'}</strong>
        </div>
        <div className="a-status-cell">
          <span>模型通道</span>
          <strong>{enabledProviders.length > 0 ? `${healthyProviders.length} / ${enabledProviders.length} 可用` : '未上报'}</strong>
        </div>
        <div className="a-status-cell">
          <span>调用窗口</span>
          <strong>24 小时 · {data.call_distribution.total_calls.toLocaleString('en-US')} 次</strong>
        </div>
        <div className="a-status-cell">
          <span>数据更新</span>
          <strong>{healthRefreshTimeLabel(data.operations.generated_at)}</strong>
        </div>
      </section>

      <PageHeader
        eyebrow={`Overview · ${formatDate(new Date())}`}
        title="运营总览"
        description="生成质量、积分消耗、待处理风险与关键运营明细。所有指标来自真实后台接口。"
        secondaryActions={(
          <>
            <a className="a-btn a-btn--sm a-btn--quiet" href="#/monitoring">系统健康</a>
            <RefreshIconButton
              label="刷新运营总览"
              refreshing={loading}
              onClick={() => {
                void load()
                void loadTrend(trendWindow)
              }}
            />
          </>
        )}
      />

      {error ? <InlineFeedback tone="danger" message={`总览刷新失败：${error}`} /> : null}

      <div className="a-metrics" aria-label="关键运营指标">
        {metricCards.map((card) => (
          <a key={card.key} className="a-metric ov-metric-link" href={card.href}>
            <div className="a-metric__label">
              <span>{card.label}</span>
              {card.status ? <span className="a-metric__delta" style={{ color: card.status.color }}>{card.status.label}</span> : null}
            </div>
            <div className="a-metric__value">{card.value}</div>
            <div className="a-metric__foot">{card.foot}</div>
          </a>
        ))}
      </div>

      <div className="a-grid a-grid--main-side">
        <TrendPanel
          snapshot={trendSnapshot}
          window={trendWindow}
          loading={trendLoading}
          error={trendError}
          onWindowChange={setTrendWindow}
          onRetry={() => void loadTrend(trendWindow)}
        />
        <PendingPanel rows={pendingRows} total={pendingTotal} />
      </div>

      <div className="a-grid a-grid--main-side">
        <LatestUsersPanel users={recentUsers} />
        <aside className="ov-side-stack">
          <DistributionPanel distribution={data.call_distribution} />
          <TopUsersPanel users={data.users} />
        </aside>
      </div>

      <ReadinessRiskPanel report={data.readiness} summary={readinessSummary} risks={readinessRisks} />
    </section>
  )
}

function TrendPanel({
  snapshot,
  window: selectedWindow,
  loading,
  error,
  onWindowChange,
  onRetry,
}: {
  snapshot: AdminMonitoringSnapshot | null
  window: MonitoringWindow
  loading: boolean
  error: string | null
  onWindowChange: (value: MonitoringWindow) => void
  onRetry: () => void
}) {
  const chart = snapshot ? buildTrendChart(snapshot) : null
  return (
    <section className="a-panel a-panel--pad ov-chart-card" aria-label="请求负载趋势">
      <div className="ov-chart-head">
        <div className="a-section-title" style={{ margin: 0 }}>
          <div>
            <h2>请求负载</h2>
            <p>最近 {selectedWindow} 采样 · 真实 API 请求；24 小时汇总见模型调用分布。</p>
          </div>
          <div className="ov-chart-controls">
            {loading && snapshot ? <span className="a-hint">刷新中...</span> : null}
            {snapshot ? (
              <div className="ov-legend" aria-label="趋势图例">
                <span><i style={{ background: 'var(--a-accent)' }} />QPS {formatMonitoringQPS(snapshot.current.qps)}</span>
                <span><i style={{ background: 'var(--a-danger)' }} />5xx {formatMonitoringPercent(snapshot.current.server_error_rate)}</span>
              </div>
            ) : null}
            <SegmentedControl
              value={selectedWindow}
              options={monitoringWindows.map((value) => ({ value, label: value }))}
              onChange={onWindowChange}
            />
          </div>
        </div>
      </div>

      {error ? (
        <div className="a-banner a-banner--warn" role="status">
          <span>趋势刷新失败：{error}</span>
          <button type="button" className="a-row-btn" onClick={onRetry}>重试</button>
        </div>
      ) : null}

      {loading && !snapshot ? (
        <div className="ov-chart-empty" role="status">正在载入请求采样...</div>
      ) : chart ? (
        <>
          <div className="a-chart" role="img" aria-label={chart.ariaLabel}>
            <svg viewBox="0 0 640 220" preserveAspectRatio="none" aria-hidden="true">
              {[18, 75, 133, 190].map((y) => (
                <line key={y} className="a-chart__grid" x1="0" y1={y} x2="640" y2={y} />
              ))}
              <path className="a-chart__area" d={chart.areaPath} />
              <path className="a-chart__line" d={chart.qpsPath} vectorEffect="non-scaling-stroke" />
              <path d={chart.errorPath} fill="none" stroke="var(--a-danger)" strokeWidth="2" strokeDasharray="4 4" strokeLinecap="round" vectorEffect="non-scaling-stroke" />
            </svg>
          </div>
          <div className="ov-chart-foot">
            <span>{chart.rangeLabel}</span>
            <span>{chart.peakLabel}</span>
          </div>
        </>
      ) : (
        <EmptyBlock variant="inline" title="暂无请求采样" detail="所选窗口内还没有可绘制的请求采样，自动刷新开启后会继续更新。" />
      )}
    </section>
  )
}

function PendingPanel({ rows, total }: { rows: PendingRow[]; total: number }) {
  const visibleRows = rows.slice(0, 7)
  return (
    <aside className="a-panel a-panel--pad" aria-label="待处理">
      <div className="ov-panel-heading">
        <div>
          <h2>待处理</h2>
          <p>需要人工确认的项目</p>
        </div>
        <Badge tone={total > 0 ? 'warning' : 'success'}>{total > 0 ? total : '0'}</Badge>
      </div>
      {visibleRows.length ? (
        <div className="ov-queue">
          {visibleRows.map((row) => (
            <a key={row.key} className="ov-queue__row" href={row.href}>
              <div>
                <strong>{row.label}</strong>
                <span>{row.detail}</span>
              </div>
              <Badge tone={row.tone}>{row.countLabel}</Badge>
            </a>
          ))}
          {rows.length > visibleRows.length ? (
            <p className="a-hint" style={{ paddingTop: 10 }}>另有 {rows.length - visibleRows.length} 项，请进入对应页面处理。</p>
          ) : null}
        </div>
      ) : (
        <div className="ov-queue__empty">
          <EmptyBlock variant="inline" title="暂无待处理事项" detail="当前队列和上线检查没有需要立即处理的项目。" />
        </div>
      )}
    </aside>
  )
}

function LatestUsersPanel({ users }: { users: ReturnType<typeof overviewRecentUserRows> }) {
  const columns: ColumnDef<(typeof users)[number]>[] = [
    {
      key: 'user',
      title: '用户',
      render: (row) => (
        <span className="a-cell-entity">
          <span className="a-cell-entity__avatar">{row.displayName.slice(0, 1).toUpperCase()}</span>
          <span className="a-cell-entity__copy">
            <span className="a-cell-entity__name">{row.displayName}</span>
            <span className="a-cell-entity__meta">{row.email} · #{row.id}</span>
          </span>
        </span>
      ),
    },
    {
      key: 'group',
      title: '所属分组',
      render: (row) => {
        const view = adminUserRowView(row.raw)
        return view.groupLabel === '-' ? <span className="a-hint">-</span> : <Badge tone="primary">{view.groupLabel}</Badge>
      },
    },
    {
      key: 'balance',
      title: '积分余额',
      align: 'right',
      render: (row) => <span className="a-num">{formatPoints(row.balance)}</span>,
    },
    {
      key: 'status',
      title: '状态',
      render: (row) => <Badge tone={row.statusTone}>{row.statusLabel}</Badge>,
    },
    {
      key: 'created',
      title: '注册时间',
      render: (row) => <span className="a-hint">{adminUserRowView(row.raw).createdAtLabel}</span>,
    },
    {
      key: 'action',
      title: '操作',
      align: 'right',
      render: (row) => <a className="a-row-btn" style={{ fontFamily: 'var(--font-admin)' }} href={row.actionHref}>{row.actionLabel}</a>,
    },
  ]

  return (
    <section className="a-panel a-panel--flush" aria-label="最新注册用户">
      <div className="a-panel__head">
        <div>
          <h2>最新注册用户</h2>
          <p>最近载入用户 · 按注册时间倒序</p>
        </div>
        <a className="a-btn a-btn--sm a-btn--quiet" href="#/users">查看全部</a>
      </div>
      <DataTable
        columns={columns}
        rows={users}
        rowKey={(row) => row.id}
        minWidth={760}
        empty={<EmptyBlock variant="inline" title="暂无注册用户" detail="用户接口尚未返回账户记录。" />}
      />
    </section>
  )
}

function DistributionPanel({ distribution }: { distribution: AdminCallDistribution }) {
  const rows = callDistributionRows(distribution)
  return (
    <section className="a-panel a-panel--pad" aria-label="模型调用分布">
      <div className="ov-panel-heading">
        <div>
          <h2>模型调用分布</h2>
          <p>24 小时窗口 · 真实上游调用 {distribution.total_calls.toLocaleString('en-US')} 次</p>
        </div>
      </div>
      {rows.length ? rows.map((row) => (
        <div key={row.label} className="ov-model-row">
          <span className="ov-model-row__name" title={row.label}>{row.label}</span>
          <div className="ov-model-bar" aria-hidden="true"><i style={{ width: `${row.percent}%` }} /></div>
          <b className="a-num" style={{ fontSize: 12, textAlign: 'right' }}>{row.calls.toLocaleString('en-US')}</b>
        </div>
      )) : (
        <EmptyBlock variant="inline" title="暂无模型调用" detail="当前 24 小时窗口内没有真实上游调用。" />
      )}
    </section>
  )
}

function TopUsersPanel({ users }: { users: AdminUser[] }) {
  const topUsers = users
    .slice()
    .sort((left, right) => Number(right.balance || 0) - Number(left.balance || 0))
    .slice(0, 5)
  return (
    <section className="a-panel a-panel--pad" aria-label="用户余额排行">
      <div className="ov-panel-heading">
        <div>
          <h2>用户余额榜</h2>
          <p>按当前载入用户余额排序 · 非消费流水</p>
        </div>
        <a className="a-btn a-btn--sm a-btn--quiet" href="#/users">全部</a>
      </div>
      {topUsers.length ? (
        <div className="ov-rank">
          {topUsers.map((user, index) => {
            const view = adminUserRowView(user)
            return (
              <div key={user.id} className="ov-rank__row">
                <span className="ov-rank__n">{String(index + 1).padStart(2, '0')}</span>
                <span className="ov-rank__name" title={view.name}>
                  {view.name}
                  {view.groupLabel !== '-' ? <span className="a-hint"> {view.groupLabel}</span> : null}
                </span>
                <span className="ov-rank__value">{formatPoints(user.balance)}</span>
              </div>
            )
          })}
        </div>
      ) : (
        <EmptyBlock variant="inline" title="暂无用户排行" detail="用户产生余额或消费后会出现在这里。" />
      )}
    </section>
  )
}

function ReadinessRiskPanel({
  report,
  summary,
  risks,
}: {
  report: ReadinessReport
  summary: { pass: number; warn: number; fail: number }
  risks: OverviewReadinessRow[]
}) {
  return (
    <section className="a-panel a-panel--flush" aria-label="上线检查风险">
      <div className="a-panel__head">
        <div>
          <h2>上线检查风险</h2>
          <p>阻塞 {summary.fail} 项 / 警告 {summary.warn} 项 / 通过 {summary.pass} 项</p>
        </div>
        <a className="a-btn a-btn--sm a-btn--quiet" href="#/monitoring">查看全部</a>
      </div>
      <DataTable
        columns={overviewReadinessColumns()}
        rows={risks}
        rowKey={(risk) => risk.key}
        minWidth={760}
        empty={<EmptyBlock variant="inline" title="暂无上线风险" detail="当前关键配置检查均已通过。" />}
      />
      <div className="a-panel__body" style={{ display: 'flex', flexWrap: 'wrap', gap: 8, borderTop: '1px solid var(--a-line)' }}>
        <Badge tone={report.status === 'fail' ? 'danger' : report.status === 'warn' ? 'warning' : 'success'}>
          {readinessOverallStatusLabel(report.overall_status ?? report.status)}
        </Badge>
        <span className="a-hint">最近检查 {healthRefreshTimeLabel(report.generated_at)}</span>
      </div>
    </section>
  )
}

function overviewReadinessColumns(): ColumnDef<OverviewReadinessRow>[] {
  return [
    {
      key: 'check',
      title: '检查项',
      render: (risk) => (
        <div className="a-cell-stack">
          <strong>{risk.label}</strong>
          <span>{risk.key}</span>
        </div>
      ),
    },
    {
      key: 'status',
      title: '状态',
      render: (risk) => <Badge tone={risk.statusTone}>{risk.status}</Badge>,
    },
    {
      key: 'detail',
      title: '说明',
      render: (risk) => <span style={{ overflowWrap: 'anywhere' }}>{risk.detail}</span>,
    },
    {
      key: 'action',
      title: '入口',
      align: 'right',
      render: (risk) => <a className="a-row-btn" style={{ fontFamily: 'var(--font-admin)' }} href={risk.actionHref}>{risk.actionLabel}</a>,
    },
  ]
}

function buildMetricCards(metrics: AdminMetric[]): MetricCard[] {
  const hrefs = ['#/users', '#/call-records', '#/call-records', '#/monitoring']
  return metrics.map((metric, index) => ({
    key: metric.key ?? metric.label,
    label: metric.label,
    value: metric.value,
    foot: metric.trend,
    href: hrefs[index] ?? '#/monitoring',
    status: metricStatus(metric.tone),
  }))
}

function metricStatus(tone: AdminMetric['tone']): MetricCard['status'] {
  if (tone === 'good') return { label: '正常', color: 'var(--a-success)' }
  if (tone === 'warn') return { label: '关注', color: 'var(--a-warn)' }
  if (tone === 'bad' || tone === 'danger') return { label: '异常', color: 'var(--a-danger)' }
  return null
}

function overviewHeroMetricRows(metrics: AdminMetric[], operations: AdminDashboardOperations): AdminMetric[] {
  const byKey = new Map(metrics.map((metric) => [metric.key ?? metric.label, metric]))
  const activeUsers = byKey.get('active_users')
  const generationSuccessRate = byKey.get('generation_success_rate')
  const actualPoints = byKey.get('actual_points')

  return [
    activeUsers
      ? { ...activeUsers, label: '活跃用户', trend: activeUsers.trend || '当前可登录用户' }
      : { key: 'active_users', label: '活跃用户', value: '0', trend: '当前可登录用户', tone: 'neutral' },
    {
      key: 'generation_success_rate',
      label: '生成成功率',
      value: generationSuccessRate?.value ?? '-',
      trend: generationSuccessRate?.trend ?? '暂无调用',
      tone: generationSuccessRate?.tone ?? 'neutral',
    },
    {
      key: 'actual_points',
      label: '积分消耗',
      value: actualPoints?.value ?? '0.00000',
      trend: actualPoints?.trend ?? (operations.preflight_failure_count ? `${operations.preflight_failure_count} 次前置失败` : '最近调用累计'),
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

function callDistributionRows(distribution: AdminCallDistribution) {
  return distribution.groups.slice(0, 8).map((group) => {
    const percent = Number.isFinite(group.percentage) ? Math.max(0, Math.min(100, group.percentage)) : 0
    return {
      label: group.key === 'unrouted' ? '未路由调用' : group.key,
      calls: group.calls,
      percent,
    }
  })
}

function buildPendingRows(data: DashboardData, risks: OverviewReadinessRow[]): PendingRow[] {
  const rows: PendingRow[] = []
  for (const item of data.queue) {
    const count = Number(item.count)
    if (!Number.isFinite(count) || count <= 0 || item.item.includes('用户总量')) continue
    const isReview = item.item.includes('审核')
    rows.push({
      key: `queue-${item.item}`,
      label: item.item,
      detail: item.detail,
      countValue: count,
      countLabel: String(count),
      tone: isReview ? 'warning' : 'danger',
      href: isReview ? '#/reviews' : '#/call-records',
    })
  }
  if (data.operations.failed_webhook_count > 0) {
    rows.push({
      key: 'failed-webhooks',
      label: '支付回调异常',
      detail: `失败回调 ${data.operations.failed_webhook_count} 条`,
      countValue: data.operations.failed_webhook_count,
      countLabel: String(data.operations.failed_webhook_count),
      tone: 'danger',
      href: '#/cashier-config',
    })
  }
  if (data.operations.refund_compensation_failed_count > 0) {
    rows.push({
      key: 'refund-compensation',
      label: '退款补偿失败',
      detail: `待补偿 ${data.operations.refund_compensation_failed_count} 笔`,
      countValue: data.operations.refund_compensation_failed_count,
      countLabel: String(data.operations.refund_compensation_failed_count),
      tone: 'danger',
      href: '#/cashier-config',
    })
  }
  const unhealthyProviders = data.providers.filter((provider) => provider.enabled !== false && provider.status !== 'healthy')
  if (unhealthyProviders.length) {
    rows.push({
      key: 'providers',
      label: '接入账号异常',
      detail: unhealthyProviders.map((provider) => provider.provider).join(' · '),
      countValue: unhealthyProviders.length,
      countLabel: String(unhealthyProviders.length),
      tone: 'warning',
      href: '#/access-accounts',
    })
  }
  for (const risk of risks.slice(0, 3)) {
    rows.push({
      key: `readiness-${risk.key}`,
      label: risk.label,
      detail: risk.detail,
      countValue: 1,
      countLabel: risk.status,
      tone: risk.statusTone === 'danger' ? 'danger' : 'warning',
      href: risk.actionHref,
    })
  }
  return rows
}

function buildTrendChart(snapshot: AdminMonitoringSnapshot) {
  const series = snapshot.series ?? []
  if (series.length < 2) return null
  const qpsValues = series.map((point) => ({ at: point.at, value: point.qps }))
  const errorValues = series.map((point) => ({ at: point.at, value: point.server_error_rate }))
  const qpsPath = buildTimeSeriesPath(qpsValues, 640, 220, timeSeriesExtent(qpsValues))
  const errorPath = buildTimeSeriesPath(errorValues, 640, 220, timeSeriesExtent(errorValues))
  if (!qpsPath) return null
  const finiteQps = qpsValues.map((point) => point.value).filter((value): value is number => value != null && Number.isFinite(value))
  const finiteError = errorValues.map((point) => point.value).filter((value): value is number => value != null && Number.isFinite(value))
  const firstAt = series[0]?.at
  const lastAt = series[series.length - 1]?.at
  return {
    qpsPath,
    errorPath,
    areaPath: `${qpsPath} L 598 220 L 42 220 Z`,
    ariaLabel: `${snapshot.window} 请求负载趋势：QPS 当前 ${formatMonitoringQPS(snapshot.current?.qps)}，5xx 错误率 ${formatMonitoringPercent(snapshot.current?.server_error_rate)}`,
    rangeLabel: `${formatClock(firstAt)} – ${formatClock(lastAt)} · ${series.length} 个采样点`,
    peakLabel: `QPS 峰值 ${formatMonitoringQPS(finiteQps.length ? Math.max(...finiteQps) : null)} · 5xx 峰值 ${formatMonitoringPercent(finiteError.length ? Math.max(...finiteError) : null)}`,
  }
}

function formatClock(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(date)
}

function formatDate(value: Date) {
  const year = value.getFullYear()
  const month = String(value.getMonth() + 1).padStart(2, '0')
  const day = String(value.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function formatPoints(value: string) {
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) return value || '-'
  return numeric.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}