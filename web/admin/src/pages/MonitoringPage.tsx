import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import type {
  AdminMonitoringSnapshot,
  MonitoringWindow,
  ReadinessReport,
} from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import {
  Badge,
  EmptyBlock,
  ErrorBlock,
  InlineFeedback,
  LoadingBlock,
  MetricStrip,
  PageHeader,
  SegmentedControl,
} from '../components'
import { healthRefreshTimeLabel } from '../healthRows'
import { adminButton, adminPage, adminSurface, adminType } from '../ui/classes'
import type { ColumnDef } from '../ui/dataTable'
import { DataTable } from '../ui/dataTable'
import { TimeSeriesChart, type TimeSeriesDatum } from '../ui/timeSeriesChart'
import {
  formatMonitoringBytes,
  formatMonitoringDuration,
  formatMonitoringPercent,
  formatMonitoringQPS,
  monitoringDiagnostics,
  monitoringMetricRows,
  monitoringRouteRows,
  monitoringStateView,
  monitoringWindows,
  type MonitoringDiagnosticRow,
  type MonitoringRouteRow,
} from './monitoringRows'

type LoadMode = 'initial' | 'refresh' | 'poll'

const windowOptions: Array<{ value: MonitoringWindow; label: string }> = monitoringWindows.map((value) => ({ value, label: value }))

const monitoringClasses = {
  controlBar: 'flex flex-wrap items-center justify-between gap-3 border-y border-[var(--border)] py-3',
  controlGroup: 'flex min-w-0 flex-wrap items-center gap-3',
  context: 'flex flex-wrap items-center gap-x-5 gap-y-1 text-xs text-[var(--soft)]',
  contextValue: 'font-[family-name:var(--admin-font-mono)] font-semibold tabular-nums text-[var(--fg)]',
  toggle: 'inline-flex min-h-9 cursor-pointer items-center gap-2 rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] px-3 text-xs font-semibold text-[var(--muted)] transition-colors hover:border-[var(--border-strong)]',
  toggleTrack: 'relative h-4 w-7 rounded-full bg-[var(--border-strong)] transition-colors peer-checked:bg-[var(--accent)]',
  toggleThumb: 'absolute left-0.5 top-0.5 size-3 rounded-full bg-white transition-transform peer-checked:translate-x-3',
  chartGrid: 'grid min-w-0 grid-cols-12 gap-4 max-[1024px]:grid-cols-1',
  chartWide: cn(adminSurface.card, 'min-w-0 p-5 lg:col-span-7'),
  chartNarrow: cn(adminSurface.card, 'min-w-0 p-5 lg:col-span-5'),
  chartFull: cn(adminSurface.card, 'min-w-0 p-5 lg:col-span-12'),
  resourceGrid: 'grid min-w-0 grid-cols-3 divide-x divide-[var(--border)] max-[900px]:grid-cols-1 max-[900px]:divide-x-0 max-[900px]:divide-y',
  resourceItem: 'min-w-0 px-4 first:pl-0 last:pr-0 max-[900px]:px-0 max-[900px]:py-4 max-[900px]:first:pt-0 max-[900px]:last:pb-0',
  resourceTitle: 'mb-2 text-xs font-semibold text-[var(--soft)]',
  panelHead: 'mb-4 flex flex-wrap items-start justify-between gap-3 border-b border-[var(--border)] pb-3',
  panelTitle: cn('m-0', adminType.sectionTitle),
  panelDetail: 'm-0 mt-1 text-xs leading-5 text-[var(--soft)]',
  legend: 'flex flex-wrap items-center gap-3 text-xs text-[var(--soft)]',
  legendItem: 'inline-flex items-center gap-1.5',
  legendDot: 'size-1.5 rounded-full',
  diagnosisGrid: 'grid min-w-0 grid-cols-12 gap-4 max-[1024px]:grid-cols-1',
  routes: cn(adminSurface.card, 'min-w-0 overflow-hidden p-0 lg:col-span-8'),
  side: 'grid min-w-0 content-start gap-4 lg:col-span-4',
  sidePanel: cn(adminSurface.card, 'min-w-0 p-5'),
  tableHead: 'flex flex-wrap items-start justify-between gap-3 border-b border-[var(--border)] px-5 py-4',
  tableSurface: 'min-w-0 overflow-hidden',
  distribution: 'grid gap-3',
  distributionRow: 'grid gap-1.5',
  distributionMeta: 'flex items-center justify-between gap-3 text-xs font-semibold',
  distributionTrack: 'h-1.5 overflow-hidden rounded-full bg-[var(--border)]',
  distributionFill: 'h-full rounded-full transition-[width] duration-[var(--admin-motion-base)]',
}

export function MonitoringPage() {
  const [snapshot, setSnapshot] = useState<AdminMonitoringSnapshot | null>(null)
  const [readiness, setReadiness] = useState<ReadinessReport | null>(null)
  const [selectedWindow, setSelectedWindow] = useState<MonitoringWindow>('15m')
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [initialLoading, setInitialLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [refreshError, setRefreshError] = useState<string | null>(null)
  const [lastSuccessfulAt, setLastSuccessfulAt] = useState<string | null>(null)
  const [lastDiagnosticsAt, setLastDiagnosticsAt] = useState<string | null>(null)
  const requestSequence = useRef(0)

  const load = useCallback(async (mode: LoadMode, targetWindow: MonitoringWindow, includeReadiness = false) => {
    const requestID = ++requestSequence.current
    if (mode === 'initial') {
      setInitialLoading(true)
      setError(null)
    } else if (mode === 'refresh') {
      setRefreshing(true)
      setRefreshError(null)
    }
    try {
      const [nextSnapshot, nextReadiness] = await Promise.all([
        adminApi.getMonitoringSnapshot(targetWindow),
        includeReadiness ? adminApi.getReadiness() : Promise.resolve(null),
      ])
      if (requestID !== requestSequence.current) return
      setSnapshot(nextSnapshot)
      const completedAt = new Date().toISOString()
      if (nextReadiness) {
        setReadiness(nextReadiness)
        setLastDiagnosticsAt(completedAt)
      }
      setLastSuccessfulAt(completedAt)
      setRefreshError(null)
    } catch (caught) {
      if (requestID !== requestSequence.current) return
      const message = caught instanceof Error ? caught.message : '系统健康数据载入失败'
      if (mode === 'initial') setError(message)
      else setRefreshError(message)
    } finally {
      if (requestID !== requestSequence.current) return
      if (mode === 'initial') setInitialLoading(false)
      if (mode === 'refresh') setRefreshing(false)
    }
  }, [])

  useEffect(() => {
    void load('initial', '15m', true)
  }, [load])

  useEffect(() => {
    if (!autoRefresh || initialLoading || refreshing) return
    let pollInFlight = false
    const refreshWhenVisible = async () => {
      if (document.visibilityState !== 'visible' || pollInFlight) return
      pollInFlight = true
      try {
        await load('poll', selectedWindow)
      } finally {
        pollInFlight = false
      }
    }
    const interval = window.setInterval(() => { void refreshWhenVisible() }, 5000)
    const handleVisibilityChange = () => { void refreshWhenVisible() }
    document.addEventListener('visibilitychange', handleVisibilityChange)
    return () => {
      window.clearInterval(interval)
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }, [autoRefresh, initialLoading, load, refreshing, selectedWindow])

  const metrics = useMemo(() => snapshot ? monitoringMetricRows(snapshot) : [], [snapshot])
  const routes = useMemo(() => snapshot ? monitoringRouteRows(snapshot.routes) : [], [snapshot])
  const diagnostics = useMemo(
    () => monitoringDiagnostics(snapshot?.providers ?? [], readiness?.checks ?? []),
    [readiness, snapshot],
  )
  const chartData = useMemo(() => snapshot ? monitoringChartData(snapshot) : [], [snapshot])

  if (initialLoading) return <LoadingBlock label="载入系统运行指标" />
  if (error) return <ErrorBlock message={error} onRetry={() => void load('initial', selectedWindow, true)} />
  if (!snapshot) return <EmptyBlock title="暂无运行指标" detail="监控接口尚未返回有效快照。" />

  const state = monitoringStateView(snapshot.state)
  const snapshotWindowStale = snapshot.window !== selectedWindow

  function handleWindowChange(window: MonitoringWindow) {
    setSelectedWindow(window)
    void load('refresh', window)
  }

  return (
    <section className={adminPage.stack} data-admin-monitoring-runtime>
      <PageHeader
        title="系统健康"
        description="实时观察应用请求、响应质量与 API 进程压力，并在异常出现时直接定位热点接口。"
        primaryAction={(
          <button
            type="button"
            className={cn(adminButton.base, adminButton.primary)}
            disabled={refreshing}
            onClick={() => void load('refresh', selectedWindow, true)}
          >
            <RefreshCw className={cn('size-4', refreshing && 'animate-spin')} aria-hidden="true" />
            <span>{refreshing ? '刷新中...' : '立即刷新'}</span>
          </button>
        )}
      />

      <div className={monitoringClasses.controlBar}>
        <div className={monitoringClasses.controlGroup}>
          <Badge tone={state.tone}>{state.label}</Badge>
          <div className={monitoringClasses.context} aria-label="运行监控上下文">
            <span>运行时长 <strong className={monitoringClasses.contextValue}>{formatUptimeContext(snapshot.uptime_seconds)}</strong></span>
            <span>数据窗口 <strong className={monitoringClasses.contextValue}>{snapshot.window}</strong></span>
            <span>运行指标 <strong className={monitoringClasses.contextValue}>{healthRefreshTimeLabel(lastSuccessfulAt)}</strong></span>
          </div>
        </div>
        <div className={monitoringClasses.controlGroup}>
          <SegmentedControl value={selectedWindow} options={windowOptions} onChange={handleWindowChange} />
          <label className={monitoringClasses.toggle}>
            <input
              type="checkbox"
              className="peer sr-only"
              checked={autoRefresh}
              onChange={(event) => setAutoRefresh(event.target.checked)}
            />
            <span className={monitoringClasses.toggleTrack} aria-hidden="true">
              <span className={monitoringClasses.toggleThumb} />
            </span>
            <span>自动刷新</span>
          </label>
        </div>
      </div>

      {refreshing ? <InlineFeedback tone="neutral" message="正在刷新运行指标，当前仍显示上一次成功数据。" /> : null}
      {refreshError ? (
        <InlineFeedback
          tone="danger"
          message={`刷新失败：${refreshError}。当前仍显示上一次成功数据${snapshotWindowStale ? `（${snapshot.window} 窗口）` : ''}。`}
        />
      ) : null}
      {!autoRefresh ? <InlineFeedback tone="warning" message="自动刷新已暂停，当前数据不会继续更新。" /> : null}

      <MetricStrip metrics={metrics} />

      {snapshot.collecting ? (
        <InlineFeedback tone="neutral" message="运行指标正在采集中，完成首个 5 秒采样周期后将显示趋势。" />
      ) : null}

      <section className={monitoringClasses.chartGrid} aria-label="运行趋势">
        <TrendPanel
          className={monitoringClasses.chartWide}
          title="请求负载"
          detail="业务请求速率与同一采样周期内的并发峰值。"
          legend={[['QPS', 'var(--accent)'], ['并发', 'var(--green)']]}
        >
          <TimeSeriesChart
            ariaLabel="请求负载趋势"
            data={chartData}
            series={[
              { key: 'qps', label: 'QPS', color: 'var(--accent)', format: formatMonitoringQPS },
              { key: 'inflight', label: '并发', color: 'var(--green)', axis: 'right', format: (value) => String(Math.round(value)) },
            ]}
          />
        </TrendPanel>
        <TrendPanel
          className={monitoringClasses.chartNarrow}
          title="响应质量"
          detail="延迟分位数与服务端错误率，便于识别长尾退化。"
          legend={[['P50', 'var(--green)'], ['P95', 'var(--amber)'], ['P99', 'var(--red)'], ['5xx', 'var(--accent-coral)']]}
        >
          <TimeSeriesChart
            ariaLabel="响应质量趋势"
            data={chartData}
            series={[
              { key: 'p50', label: 'P50', color: 'var(--green)', format: formatMonitoringDuration },
              { key: 'p95', label: 'P95', color: 'var(--amber)', format: formatMonitoringDuration },
              { key: 'p99', label: 'P99', color: 'var(--red)', format: formatMonitoringDuration },
              { key: 'errorRate', label: '5xx', color: 'var(--accent-coral)', axis: 'right', format: formatMonitoringPercent },
            ]}
          />
        </TrendPanel>
        <TrendPanel
          className={monitoringClasses.chartFull}
          title="资源压力"
          detail="API 进程 CPU、Go Heap 与 Goroutine 变化；指标不代表整台宿主机。"
          legend={[['CPU', 'var(--accent-coral)'], ['Heap MB', 'var(--accent)'], ['Goroutine', 'var(--green)']]}
        >
          <div className={monitoringClasses.resourceGrid}>
            <ResourceTrend
              title="CPU 容量"
              ariaLabel="API 进程 CPU 趋势"
              data={chartData}
              series={{ key: 'cpu', label: 'CPU', color: 'var(--accent-coral)', format: formatMonitoringPercent }}
            />
            <ResourceTrend
              title="Go Heap"
              ariaLabel="Go Heap 内存趋势"
              data={chartData}
              series={{ key: 'heap', label: 'Heap MB', color: 'var(--accent)', format: (value) => `${value.toFixed(1)} MB` }}
            />
            <ResourceTrend
              title="Goroutine"
              ariaLabel="Goroutine 数量趋势"
              data={chartData}
              series={{ key: 'goroutines', label: 'Goroutine', color: 'var(--green)', format: (value) => String(Math.round(value)) }}
            />
          </div>
        </TrendPanel>
      </section>

      <section className={monitoringClasses.diagnosisGrid} aria-label="运行诊断">
        <section className={monitoringClasses.routes}>
          <header className={monitoringClasses.tableHead}>
            <div>
              <h2 className={monitoringClasses.panelTitle}>热点接口</h2>
              <p className={monitoringClasses.panelDetail}>按窗口请求量排序，路由使用服务端标准化 Pattern。</p>
            </div>
            <Badge tone={routes.some((row) => row.statusTone === 'danger') ? 'danger' : routes.some((row) => row.statusTone === 'warning') ? 'warning' : 'success'}>
              {routes.length} 条路由
            </Badge>
          </header>
          <div className={monitoringClasses.tableSurface}>
            <DataTable
              columns={routeColumns()}
              rows={routes}
              rowKey={(row) => row.route}
              empty={<EmptyBlock variant="inline" title="暂无业务请求" detail="当前窗口内还没有可聚合的 API 请求。" />}
            />
          </div>
        </section>

        <aside className={monitoringClasses.side}>
          <StatusDistribution snapshot={snapshot} />
          <section className={monitoringClasses.sidePanel}>
            <div className={monitoringClasses.panelHead}>
              <div>
                <h2 className={monitoringClasses.panelTitle}>依赖与诊断</h2>
                <p className={monitoringClasses.panelDetail}>只展示异常 Provider 与需要处理的配置项。诊断更新 {healthRefreshTimeLabel(lastDiagnosticsAt)}</p>
              </div>
              <Badge tone={diagnostics.some((row) => row.statusTone === 'danger') ? 'danger' : diagnostics.length ? 'warning' : 'success'}>
                {diagnostics.length ? `${diagnostics.length} 项` : '无异常'}
              </Badge>
            </div>
            {diagnostics.length ? (
              <div className="-mx-5 -mb-5">
                <DataTable columns={diagnosticColumns()} rows={diagnostics} rowKey={(row) => row.key} />
              </div>
            ) : (
              <EmptyBlock variant="inline" title="依赖状态正常" detail="当前没有异常 Provider 或配置诊断。" />
            )}
          </section>
        </aside>
      </section>
    </section>
  )
}

function ResourceTrend({
  title,
  ariaLabel,
  data,
  series,
}: {
  title: string
  ariaLabel: string
  data: TimeSeriesDatum[]
  series: Parameters<typeof TimeSeriesChart>[0]['series'][number]
}) {
  return (
    <section className={monitoringClasses.resourceItem}>
      <h3 className={monitoringClasses.resourceTitle}>{title}</h3>
      <TimeSeriesChart ariaLabel={ariaLabel} data={data} series={[series]} />
    </section>
  )
}

function TrendPanel({
  className,
  title,
  detail,
  legend,
  children,
}: {
  className: string
  title: string
  detail: string
  legend: Array<[string, string]>
  children: React.ReactNode
}) {
  return (
    <section className={className}>
      <header className={monitoringClasses.panelHead}>
        <div>
          <h2 className={monitoringClasses.panelTitle}>{title}</h2>
          <p className={monitoringClasses.panelDetail}>{detail}</p>
        </div>
        <div className={monitoringClasses.legend} aria-label={`${title}图例`}>
          {legend.map(([label, color]) => (
            <span key={label} className={monitoringClasses.legendItem}>
              <span className={monitoringClasses.legendDot} style={{ backgroundColor: color }} />
              {label}
            </span>
          ))}
        </div>
      </header>
      {children}
    </section>
  )
}

function StatusDistribution({ snapshot }: { snapshot: AdminMonitoringSnapshot }) {
  const rows = [
    { label: '2xx', value: snapshot.statuses.success, color: 'bg-[var(--green)]' },
    { label: '3xx', value: snapshot.statuses.redirect, color: 'bg-[var(--accent)]' },
    { label: '4xx', value: snapshot.statuses.client_error, color: 'bg-[var(--amber)]' },
    { label: '5xx', value: snapshot.statuses.server_error, color: 'bg-[var(--red)]' },
  ]
  const total = Math.max(1, snapshot.statuses.total)
  return (
    <section className={monitoringClasses.sidePanel}>
      <div className={monitoringClasses.panelHead}>
        <div>
          <h2 className={monitoringClasses.panelTitle}>状态码分布</h2>
          <p className={monitoringClasses.panelDetail}>所选窗口共 {snapshot.statuses.total.toLocaleString('en-US')} 次请求。</p>
        </div>
      </div>
      <div className={monitoringClasses.distribution}>
        {rows.map((row) => (
          <div key={row.label} className={monitoringClasses.distributionRow}>
            <div className={monitoringClasses.distributionMeta}>
              <span className="text-[var(--soft)]">{row.label}</span>
              <span className="font-[family-name:var(--admin-font-mono)] tabular-nums text-[var(--fg)]">
                {row.value.toLocaleString('en-US')} · {formatMonitoringPercent((row.value / total) * 100)}
              </span>
            </div>
            <div className={monitoringClasses.distributionTrack}>
              <div className={cn(monitoringClasses.distributionFill, row.color)} style={{ width: `${(row.value / total) * 100}%` }} />
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}

function monitoringChartData(snapshot: AdminMonitoringSnapshot): TimeSeriesDatum[] {
  return snapshot.series.map((point) => ({
    at: point.at,
    values: {
      qps: point.qps,
      inflight: point.peak_inflight,
      p50: point.p50_ms,
      p95: point.p95_ms,
      p99: point.p99_ms,
      errorRate: point.server_error_rate,
      cpu: point.cpu_percent,
      heap: point.heap_bytes / (1024 * 1024),
      goroutines: point.goroutines,
    },
  }))
}

function routeColumns(): ColumnDef<MonitoringRouteRow>[] {
  return [
    { key: 'route', title: '标准化路由', width: 'minmax(250px,2.2fr)', kind: 'code', render: (row) => <strong className="text-[var(--fg)]">{row.route}</strong> },
    { key: 'requests', title: '请求数', width: 'minmax(90px,.7fr)', align: 'right', kind: 'number', render: (row) => row.requestsLabel },
    { key: 'qps', title: 'QPS', width: 'minmax(80px,.65fr)', align: 'right', kind: 'number', render: (row) => row.qpsLabel },
    { key: 'p95', title: 'P95', width: 'minmax(90px,.7fr)', align: 'right', kind: 'number', render: (row) => row.p95Label },
    { key: 'errors', title: '5xx', width: 'minmax(80px,.65fr)', align: 'right', kind: 'number', render: (row) => row.serverErrorLabel },
    { key: 'state', title: '状态', width: 'minmax(90px,.7fr)', render: (row) => <Badge tone={row.statusTone}>{row.statusLabel}</Badge> },
  ]
}

function diagnosticColumns(): ColumnDef<MonitoringDiagnosticRow>[] {
  return [
    {
      key: 'item',
      title: '异常项',
      width: 'minmax(160px,1.2fr)',
      render: (row) => <div className="min-w-0"><strong className="text-[var(--fg)]">{row.label}</strong><p className="m-0 mt-1 text-xs text-[var(--soft)]">{row.detail}</p></div>,
    },
    { key: 'status', title: '状态', width: 'minmax(80px,.6fr)', render: (row) => <Badge tone={row.statusTone}>{row.statusLabel}</Badge> },
    { key: 'action', title: '处理', width: 'minmax(96px,.6fr)', align: 'right', render: (row) => <a className={cn(adminButton.base, adminButton.small)} href={row.actionHref}>{row.actionLabel}</a> },
  ]
}

function formatUptimeContext(seconds: number) {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${minutes}m`
  return `${minutes}m`
}
