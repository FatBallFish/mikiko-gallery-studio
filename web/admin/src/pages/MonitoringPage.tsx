import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
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
  RefreshIconButton,
  SegmentedControl,
} from '../components'
import { healthRefreshTimeLabel } from '../healthRows'
import type { ColumnDef } from '../ui/dataTable'
import { DataTable } from '../ui/dataTable'
import { TimeSeriesChart, type TimeSeriesDatum } from '../ui/timeSeriesChart'
import {
  formatMonitoringBytes,
  formatMonitoringDuration,
  formatMonitoringPercent,
  formatMonitoringQPS,
  formatMonitoringUptime,
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
  const routes = useMemo(() => snapshot ? monitoringRouteRows(snapshot.routes ?? []) : [], [snapshot])
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
    <section data-admin-monitoring-runtime>
      <section className="a-panel a-panel--flush">
        <div className="a-status-strip" style={{ borderBottom: 0 }} aria-label="运行状态条">
          <div className="a-status-cell">
            <span>API</span>
            <strong>{state.label}</strong>
          </div>
          <div className="a-status-cell">
            <span>运行时长</span>
            <strong>{formatMonitoringUptime(snapshot.uptime_seconds)}</strong>
          </div>
          <div className="a-status-cell">
            <span>数据窗口</span>
            <strong>{snapshot.window}</strong>
          </div>
          <div className="a-status-cell">
            <span>指标更新</span>
            <strong>{healthRefreshTimeLabel(lastSuccessfulAt)}</strong>
          </div>
          <div className="a-status-cell">
            <span>诊断更新</span>
            <strong>{healthRefreshTimeLabel(lastDiagnosticsAt)}</strong>
          </div>
        </div>
      </section>

      <div style={{ marginTop: 16 }}>
        <PageHeader
          eyebrow="Observability"
          title="系统健康"
          description="请求负载、响应质量与资源压力。热点接口和诊断项直接给出处理入口。"
          actions={(
            <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8 }}>
              <SegmentedControl value={selectedWindow} options={windowOptions} onChange={handleWindowChange} />
              <label className="a-switch">
                <input
                  type="checkbox"
                  role="switch"
                  aria-checked={autoRefresh}
                  checked={autoRefresh}
                  onChange={(event) => setAutoRefresh(event.target.checked)}
                />
                <span className="a-switch__track" aria-hidden="true" />
                <span>自动刷新</span>
              </label>
              <RefreshIconButton label="刷新系统健康" refreshing={refreshing} onClick={() => void load('refresh', selectedWindow, true)} />
            </div>
          )}
        />
      </div>

      <div className="a-page-stack">
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

        <section className="a-grid a-grid--3" aria-label="运行趋势与资源">
          <TrendPanel
            title="请求负载"
            detail={`最近 ${snapshot.window} · QPS 与并发峰值`}
            legend={[['QPS', 'var(--a-accent)'], ['并发', 'var(--a-success)']]}
            status={<Badge tone={state.tone}>{state.label}</Badge>}
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

          <section className="a-panel a-panel--pad" aria-label="响应质量">
            <div className="a-section-title" style={{ margin: 0 }}>
              <div>
                <h2>响应质量</h2>
                <p>延迟分位与状态码分布</p>
              </div>
              <Badge tone={snapshot.current.server_error_rate >= 1 ? 'warning' : 'success'}>
                5xx {formatMonitoringPercent(snapshot.current.server_error_rate)}
              </Badge>
            </div>
            <div className="mn-gauge" style={{ gridTemplateColumns: '1fr 1fr', marginTop: 14 }}>
              <div><span>P50</span><b>{formatMonitoringDuration(snapshot.current.p50_ms)}</b></div>
              <div><span>P95</span><b>{formatMonitoringDuration(snapshot.current.p95_ms)}</b></div>
              <div><span>P99</span><b>{formatMonitoringDuration(snapshot.current.p99_ms)}</b></div>
              <div><span>5xx</span><b style={{ color: snapshot.current.server_error_rate >= 1 ? 'var(--a-warn)' : 'var(--a-success)' }}>{formatMonitoringPercent(snapshot.current.server_error_rate)}</b></div>
            </div>
            <div style={{ marginTop: 16 }}>
              <div className="a-section-title">
                <div><h2>状态码分布</h2><p>所选窗口共 {snapshot.statuses.total.toLocaleString('en-US')} 次请求</p></div>
              </div>
              <StatusDistribution snapshot={snapshot} />
            </div>
          </section>

          <section className="a-panel a-panel--pad" aria-label="资源压力">
            <div className="a-section-title" style={{ margin: 0 }}>
              <div>
                <h2>资源压力</h2>
                <p>API 进程运行时</p>
              </div>
            </div>
            <div className="mn-gauge" style={{ gridTemplateColumns: '1fr 1fr', marginTop: 14 }}>
              <div><span>CPU</span><b>{formatMonitoringPercent(snapshot.current.cpu_percent)}</b></div>
              <div><span>Go Heap</span><b>{formatMonitoringBytes(snapshot.current.heap_bytes)}</b></div>
              <div><span>Runtime</span><b>{formatMonitoringBytes(snapshot.current.sys_bytes)}</b></div>
              <div><span>Goroutine</span><b>{snapshot.current.goroutines.toLocaleString('en-US')}</b></div>
            </div>
            <div className={cn('a-banner', resourcePressure(snapshot) ? 'a-banner--warn' : 'a-banner--info')} style={{ marginTop: 14, fontSize: 12.5 }}>
              <span>{resourceNote(snapshot)}</span>
            </div>
          </section>
        </section>

        <section className="a-panel a-panel--pad" aria-label="延迟与资源趋势">
          <div className="a-section-title" style={{ margin: 0 }}>
            <div>
              <h2>延迟与资源趋势</h2>
              <p>所选窗口 {snapshot.window} · 真实采样时间序列</p>
            </div>
            <Badge tone={state.tone}>{state.label}</Badge>
          </div>
          <div className="a-grid a-grid--3" style={{ marginTop: 14 }}>
            <ResourceTrend
              title="延迟与 5xx"
              ariaLabel="延迟与错误率趋势"
              data={chartData}
              series={[
                { key: 'p95', label: 'P95', color: 'var(--amber)', format: formatMonitoringDuration },
                { key: 'p99', label: 'P99', color: 'var(--red)', format: formatMonitoringDuration },
                { key: 'errorRate', label: '5xx', color: 'var(--accent)', axis: 'right', format: formatMonitoringPercent },
              ]}
            />
            <ResourceTrend
              title="CPU 与 Heap"
              ariaLabel="CPU 与 Go Heap 趋势"
              data={chartData}
              series={[
                { key: 'cpu', label: 'CPU', color: 'var(--accent-coral)', format: formatMonitoringPercent },
                { key: 'heap', label: 'Heap MB', color: 'var(--accent)', axis: 'right', format: (value) => `${value.toFixed(1)} MB` },
              ]}
            />
            <ResourceTrend
              title="Goroutine"
              ariaLabel="Goroutine 数量趋势"
              data={chartData}
              series={[
                { key: 'goroutines', label: 'Goroutine', color: 'var(--green)', format: (value) => String(Math.round(value)) },
              ]}
            />
          </div>
        </section>

        <section className="a-grid a-grid--main-side" aria-label="运行诊断">
          <section className="a-panel a-panel--flush">
            <div className="a-panel__head">
              <div>
                <h2>热点接口</h2>
                <p>按窗口请求量排序 · 最近 {snapshot.window} · 服务端标准化 Pattern</p>
              </div>
              <div className="a-row-actions">
                <Badge tone={routes.some((row) => row.statusTone === 'danger') ? 'danger' : routes.some((row) => row.statusTone === 'warning') ? 'warning' : 'success'}>
                  {routes.length} 条路由
                </Badge>
                <a className="a-btn a-btn--sm a-btn--quiet" href="#/call-records">查看任务</a>
              </div>
            </div>
            <DataTable
              columns={routeColumns()}
              rows={routes}
              rowKey={(row) => row.route}
              minWidth={860}
              empty={<EmptyBlock variant="inline" title="暂无业务请求" detail="当前窗口内还没有可聚合的 API 请求。" />}
            />
          </section>

          <aside className="a-panel a-panel--pad" aria-label="依赖与诊断">
            <div className="a-section-title" style={{ margin: 0 }}>
              <div>
                <h2>依赖与诊断</h2>
                <p>只展示异常 Provider 与需要处理的配置项。诊断更新 {healthRefreshTimeLabel(lastDiagnosticsAt)}</p>
              </div>
              <Badge tone={diagnostics.some((row) => row.statusTone === 'danger') ? 'danger' : diagnostics.length ? 'warning' : 'success'}>
                {diagnostics.length ? `${diagnostics.length} 项` : '无异常'}
              </Badge>
            </div>
            <div style={{ display: 'grid', gap: 8, marginTop: 14 }}>
              {diagnostics.length ? diagnostics.map((row) => (
                <div key={row.key} className={cn('a-banner', row.statusTone === 'danger' ? 'a-banner--danger' : 'a-banner--warn')}>
                  <Badge tone={row.statusTone === 'danger' ? 'danger' : 'warning'}>{row.statusLabel}</Badge>
                  <div style={{ display: 'grid', gap: 4, minWidth: 0, flex: 1 }}>
                    <strong style={{ fontSize: 12.5 }}>{row.label}</strong>
                    <div className="a-hint">{row.detail}</div>
                    <a className="a-row-btn" href={row.actionHref} style={{ justifySelf: 'start', marginTop: 4 }}>{row.actionLabel}</a>
                  </div>
                </div>
              )) : (
                <EmptyBlock variant="inline" title="依赖状态正常" detail="当前没有异常 Provider 或配置诊断。" />
              )}
            </div>
          </aside>
        </section>
      </div>
    </section>
  )
}

function TrendPanel({
  title,
  detail,
  legend,
  status,
  children,
}: {
  title: string
  detail: string
  legend: Array<[string, string]>
  status: ReactNode
  children: ReactNode
}) {
  return (
    <section className="a-panel a-panel--pad">
      <div className="a-section-title" style={{ margin: 0 }}>
        <div>
          <h2>{title}</h2>
          <p>{detail}</p>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <div className="ov-legend" aria-label={`${title}图例`}>
            {legend.map(([label, color]) => (
              <span key={label}><i style={{ background: color }} />{label}</span>
            ))}
          </div>
          {status}
        </div>
      </div>
      <div style={{ marginTop: 12 }}>{children}</div>
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
  series: Parameters<typeof TimeSeriesChart>[0]['series']
}) {
  return (
    <section style={{ minWidth: 0 }}>
      <h3 className="a-hint" style={{ marginBottom: 8, fontWeight: 600 }}>{title}</h3>
      <TimeSeriesChart ariaLabel={ariaLabel} data={data} series={series} />
    </section>
  )
}

function StatusDistribution({ snapshot }: { snapshot: AdminMonitoringSnapshot }) {
  const rows = [
    { label: '2xx', value: snapshot.statuses.success, color: 'var(--a-success)' },
    { label: '3xx', value: snapshot.statuses.redirect, color: 'var(--a-info)' },
    { label: '4xx', value: snapshot.statuses.client_error, color: 'var(--a-warn)' },
    { label: '5xx', value: snapshot.statuses.server_error, color: 'var(--a-danger)' },
  ]
  const total = Math.max(1, snapshot.statuses.total)
  return (
    <div className="a-dist">
      {rows.map((row) => {
        const percent = (row.value / total) * 100
        return (
          <div key={row.label} className="a-dist__row">
            <span>{row.label} · {row.value.toLocaleString('en-US')}</span>
            <div className="a-dist__bar">
              <i style={{ width: `${Math.min(100, percent)}%`, background: row.color }} />
            </div>
            <b>{formatMonitoringPercent(percent)}</b>
          </div>
        )
      })}
    </div>
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
    { key: 'route', title: '标准化路由', width: 'minmax(260px,2.2fr)', render: (row) => <span className="a-mono" style={{ fontSize: 12.5 }}>{row.route}</span> },
    { key: 'requests', title: '请求数', width: 'minmax(90px,.7fr)', align: 'right', kind: 'number', render: (row) => row.requestsLabel },
    { key: 'qps', title: 'QPS', width: 'minmax(80px,.65fr)', align: 'right', kind: 'number', render: (row) => row.qpsLabel },
    { key: 'p95', title: 'P95', width: 'minmax(90px,.7fr)', align: 'right', kind: 'number', render: (row) => row.p95Label },
    { key: 'errors', title: '5xx', width: 'minmax(80px,.65fr)', align: 'right', kind: 'number', render: (row) => row.serverErrorLabel },
    { key: 'state', title: '状态', width: 'minmax(90px,.7fr)', render: (row) => <Badge tone={row.statusTone}>{row.statusLabel}</Badge> },
  ]
}

function resourcePressure(snapshot: AdminMonitoringSnapshot) {
  const cpu = snapshot.current.cpu_percent
  return cpu != null && Number.isFinite(cpu) && cpu >= 75
}

function resourceNote(snapshot: AdminMonitoringSnapshot) {
  const current = snapshot.current
  if (current.cpu_percent == null) {
    return `CPU 采样将在首个周期后可用；当前 Go Heap ${formatMonitoringBytes(current.heap_bytes)}，Goroutine ${current.goroutines.toLocaleString('en-US')}。`
  }
  if (resourcePressure(snapshot)) {
    return `CPU ${formatMonitoringPercent(current.cpu_percent)} 已达 75% 告警阈值，请检查请求压力与后台任务。`
  }
  return `CPU ${formatMonitoringPercent(current.cpu_percent)}，Go Heap ${formatMonitoringBytes(current.heap_bytes)}，Goroutine ${current.goroutines.toLocaleString('en-US')}，处于稳定区间。`
}