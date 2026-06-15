import { useEffect, useMemo, useState } from 'react'
import type { ProviderHealth, ReadinessReport } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock } from '../components'
import { healthProviderRows, healthRefreshTimeLabel, refreshPolicyLabel, taskQueuePressure } from '../healthRows'
import { adminButton } from '../ui/classes'
import { adminDataGrid, adminGridCols } from '../ui/dataGrid'
import { readinessOverallStatusLabel, readinessRows } from './readinessRows'

const monitoringClasses = {
  sectionHeader: 'flex items-end justify-between gap-4',
  sectionTitle: 'flex items-center gap-3 text-sm font-bold uppercase tracking-[0.15em] text-[var(--muted-strong)] before:h-px before:w-6 before:bg-[var(--accent)]',
  healthGrid: 'grid grid-cols-1 gap-6 md:grid-cols-2 xl:grid-cols-3',
  healthCard: 'flex items-center gap-5 rounded-3xl border border-[var(--line)] bg-white/[0.02] p-6 transition-all hover:border-[var(--line-strong)] hover:bg-white/[0.04]',
  healthIcon: 'grid size-12 shrink-0 place-items-center rounded-2xl bg-white/5 text-[var(--accent)]',
  healthLabel: 'text-sm font-bold text-[var(--text)]',
  healthValue: 'font-mono text-xs text-[var(--muted-strong)]',
  healthStatus: 'size-2.5 rounded-full shadow-[0_0_10px_currentColor]',
  chartGrid: 'grid grid-cols-1 gap-8 lg:grid-cols-2',
  panel: 'rounded-3xl border border-[var(--line)] bg-white/[0.02] p-8',
  panelHeader: 'mb-6 flex items-end justify-between gap-4',
  slaValue: 'text-4xl font-black tracking-tighter text-[var(--green)]',
  slaLabel: 'mt-1 text-[10px] font-bold uppercase tracking-widest text-[var(--muted-strong)]',
  metricRow: 'flex items-center justify-between gap-4 rounded-2xl border border-[var(--line)] bg-white/[0.02] p-4',
  latencyCard: 'rounded-2xl border border-[var(--line)] bg-white/[0.03] p-6',
  tableTitle: 'border-b border-[var(--line)] bg-white/[0.02] p-6 text-sm font-bold text-[var(--text)]',
}

export function MonitoringPage() {
  const [providers, setProviders] = useState<ProviderHealth[]>([])
  const [readiness, setReadiness] = useState<ReadinessReport | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  async function load() {
    setLoading(true)
    setError(null)
    try {
      const [dashboard, report] = await Promise.all([adminApi.dashboard(), adminApi.getReadiness()])
      setProviders(dashboard.providers)
      setReadiness(report)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '运维监控载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [])

  const providerRows = useMemo(() => healthProviderRows(providers), [providers])
  const checkRows = useMemo(() => readinessRows(readiness?.checks ?? []), [readiness])
  const summary = useMemo(() => ({
    pass: readiness?.summary?.pass ?? readiness?.checks.filter((item) => item.status === 'pass').length ?? 0,
    warn: readiness?.summary?.warn ?? readiness?.checks.filter((item) => item.status === 'warn').length ?? 0,
    fail: readiness?.summary?.fail ?? readiness?.checks.filter((item) => item.status === 'fail').length ?? 0,
  }), [readiness])
  const providerFailures = providerRows.filter((row) => row.statusTone !== 'success').length
  const healthScore = readiness ? Math.max(0, Math.round((summary.pass / Math.max(readiness.checks.length, 1)) * 10000) / 100) : 100
  const averageLatency = providerRows.length
    ? Math.round(providers.reduce((total, provider) => total + Number(provider.latency_ms || 0), 0) / providerRows.length)
    : 0

  if (loading) return <LoadingBlock label="载入运维监控" />
  if (error) return <ErrorBlock message={error} onRetry={load} />
  if (!providerRows.length && !checkRows.length) return <EmptyBlock title="暂无监控数据" detail="Provider 探针与上线检查均未返回结果。" />

  return (
    <section className="grid gap-10">
      <div className={monitoringClasses.sectionHeader}>
        <h3 className={monitoringClasses.sectionTitle}>基础信息 / Infrastructure</h3>
        <button type="button" className={cn(adminButton.base, adminButton.ghost)} onClick={() => void load()}>重新探测</button>
      </div>

      <div className={monitoringClasses.healthGrid}>
        {providerRows.map((row) => (
          <HealthCard key={row.key} label={row.name} value={`${row.latencyLabel} · ${row.errorRate}`} tone={row.statusTone} />
        ))}
        <HealthCard label="任务队列" value={taskQueuePressure(providers)} tone={providerFailures ? 'warning' : 'success'} />
        <HealthCard label="上线检查" value={readiness ? readinessOverallStatusLabel(readiness.status) : '未返回'} tone={summary.fail ? 'danger' : summary.warn ? 'warning' : 'success'} />
        <HealthCard label="刷新时间" value={healthRefreshTimeLabel(readiness?.generated_at)} tone="neutral" />
      </div>

      <div className={monitoringClasses.chartGrid}>
        <section className={monitoringClasses.panel}>
          <div className={monitoringClasses.panelHeader}>
            <h3 className={monitoringClasses.sectionTitle}>SLA 指标 / Metrics</h3>
            <div className="flex gap-2">
              {['1m', '5m', '1h'].map((item) => <span key={item} className={cn('rounded-full px-3 py-1 text-[10px] font-bold', item === '5m' ? 'bg-[var(--accent)] text-white' : 'bg-white/5 text-[var(--muted)]')}>{item}</span>)}
            </div>
          </div>
          <div className="mb-8 flex items-center justify-between gap-4">
            <div>
              <div className={monitoringClasses.slaValue}>{healthScore.toFixed(2)}%</div>
              <div className={monitoringClasses.slaLabel}>健康总评分 / Health Score</div>
            </div>
            <div className="text-right">
              <div className="text-xl font-bold text-[var(--text)]">{providerFailures}</div>
              <div className={monitoringClasses.slaLabel}>异常探针</div>
            </div>
          </div>
          <div className="grid gap-4">
            <MetricRow label="通过检查" value={`${summary.pass} 项`} tone="success" />
            <MetricRow label="警告检查" value={`${summary.warn} 项`} tone={summary.warn ? 'warning' : 'success'} />
            <MetricRow label="阻塞检查" value={`${summary.fail} 项`} tone={summary.fail ? 'danger' : 'success'} />
          </div>
        </section>

        <section className={monitoringClasses.panel}>
          <div className={monitoringClasses.panelHeader}>
            <h3 className={monitoringClasses.sectionTitle}>生图耗时 / Latency</h3>
          </div>
          <div className="grid grid-cols-2 gap-6">
            <LatencyCard label="Avg" value={`${averageLatency}ms`} />
            <LatencyCard label="Max" value={`${Math.max(0, ...providers.map((provider) => Number(provider.latency_ms || 0)))}ms`} />
            <LatencyCard label="Policy" value={refreshPolicyLabel('30s interval')} />
            <LatencyCard label="Queue" value={taskQueuePressure(providers)} />
          </div>
        </section>
      </div>

      <div className="grid grid-cols-1 gap-8 xl:grid-cols-2">
        <section className={adminDataGrid.root}>
          <div className={monitoringClasses.tableTitle}>Provider 探针 / Providers</div>
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
        </section>

        <section className={adminDataGrid.root}>
          <div className={monitoringClasses.tableTitle}>上线检查 / Readiness</div>
          <div className={cn(adminDataGrid.head, adminGridCols.readiness)}><span>检查项</span><span>状态</span><span>阻塞</span><span>修复入口</span><span>说明</span></div>
          {checkRows.map((check) => (
            <div key={check.key} className={cn(adminDataGrid.row, adminGridCols.readiness)}>
              <div className={adminDataGrid.stackCell}>
                <strong>{check.label}</strong>
                <p className={adminDataGrid.detail}>{check.key}</p>
              </div>
              <Badge tone={check.statusTone}>{check.status}</Badge>
              <Badge tone={check.blockingTone}>{check.blockingLabel}</Badge>
              <a className={cn(adminButton.base, adminButton.small)} href={check.actionHref}>{check.actionLabel}</a>
              <span className={adminDataGrid.cell}>{check.detail}</span>
            </div>
          ))}
        </section>
      </div>
    </section>
  )
}

function HealthCard({ label, value, tone }: { label: string; value: string; tone: 'success' | 'warning' | 'danger' | 'neutral' }) {
  const color = tone === 'success' ? 'text-emerald-500' : tone === 'warning' ? 'text-amber-500' : tone === 'danger' ? 'text-rose-500' : 'text-[var(--accent)]'
  return (
    <div className={monitoringClasses.healthCard}>
      <div className={monitoringClasses.healthIcon}><PulseIcon /></div>
      <div className="min-w-0 flex-1">
        <div className={monitoringClasses.healthLabel}>{label}</div>
        <div className={monitoringClasses.healthValue}>{value}</div>
      </div>
      <span className={cn(monitoringClasses.healthStatus, color)} />
    </div>
  )
}

function MetricRow({ label, value, tone }: { label: string; value: string; tone: 'success' | 'warning' | 'danger' }) {
  const color = tone === 'success' ? 'text-emerald-400' : tone === 'warning' ? 'text-amber-400' : 'text-rose-400'
  return (
    <div className={monitoringClasses.metricRow}>
      <span className="text-xs font-medium text-[var(--muted)]">{label}</span>
      <span className={cn('text-sm font-bold', color)}>{value}</span>
    </div>
  )
}

function LatencyCard({ label, value }: { label: string; value: string }) {
  return (
    <div className={monitoringClasses.latencyCard}>
      <div className="mb-1 text-[10px] font-bold uppercase tracking-widest text-[var(--muted-strong)]">{label}</div>
      <div className="text-2xl font-black text-[var(--text)]">{value}</div>
    </div>
  )
}

const PulseIcon = () => <svg className="size-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M22 12h-4l-3 9L9 3l-3 9H2" /></svg>
