import { useEffect, useMemo, useState } from 'react'
import type { ProviderHealth, ReadinessReport } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, EmptyBlock, ErrorBlock, InlineFeedback, LoadingBlock, MetricStrip, PageHeader } from '../components'
import { healthProviderRows, healthRefreshTimeLabel, taskQueuePressure } from '../healthRows'
import type { HealthProviderRow } from '../healthRows'
import { adminButton, adminPage, adminType } from '../ui/classes'
import type { ColumnDef } from '../ui/dataTable'
import { DataTable } from '../ui/dataTable'
import { MonitoringIcon } from '../ui/icons'
import type { ReadinessRowModel } from './readinessRows'
import { readinessOverallStatusLabel, readinessRows } from './readinessRows'

type LoadMode = 'initial' | 'refresh'

const monitoringClasses = {
  contextBar: 'flex flex-wrap items-center gap-x-6 gap-y-2 border-y border-[var(--border)] py-3 text-xs text-[var(--soft)]',
  contextItem: 'inline-flex items-center gap-2',
  contextValue: 'font-[family-name:var(--admin-font-mono)] font-semibold text-[var(--fg)]',
  section: 'grid min-w-0 gap-3',
  sectionHead: 'flex flex-wrap items-end justify-between gap-3',
  sectionDescription: 'mt-1 text-xs leading-5 text-[var(--soft)]',
  workbench: 'grid min-w-0 grid-cols-1 gap-6 xl:grid-cols-2',
  blockerBand: 'grid min-w-0 gap-3 border-b border-[var(--border)] pb-5',
  tableSurface: 'min-w-0 overflow-hidden border border-[var(--border)] bg-[var(--surface-solid)]',
}

export function MonitoringPage() {
  const [providers, setProviders] = useState<ProviderHealth[]>([])
  const [readiness, setReadiness] = useState<ReadinessReport | null>(null)
  const [initialLoading, setInitialLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [refreshError, setRefreshError] = useState<string | null>(null)

  async function load(mode: LoadMode) {
    if (mode === 'initial') {
      setInitialLoading(true)
      setError(null)
    } else {
      setRefreshing(true)
      setRefreshError(null)
    }
    try {
      const [dashboard, report] = await Promise.all([adminApi.dashboard(), adminApi.getReadiness()])
      setProviders(dashboard.providers)
      setReadiness(report)
    } catch (caught) {
      const message = caught instanceof Error ? caught.message : '运维监控载入失败'
      if (mode === 'initial') setError(message)
      else setRefreshError(message)
    } finally {
      if (mode === 'initial') setInitialLoading(false)
      else setRefreshing(false)
    }
  }

  useEffect(() => { void load('initial') }, [])

  const providerRows = useMemo(() => healthProviderRows(providers), [providers])
  const checkRows = useMemo(() => sortReadinessRows(readinessRows(readiness?.checks ?? [])), [readiness])
  const blockingRows = checkRows.filter((row) => row.rawStatus === 'fail' && row.blockingTone === 'danger')
  const summary = useMemo(() => ({
    pass: readiness?.summary?.pass ?? readiness?.checks.filter((item) => item.status === 'pass').length ?? 0,
    warn: readiness?.summary?.warn ?? readiness?.checks.filter((item) => item.status === 'warn').length ?? 0,
    fail: readiness?.summary?.fail ?? readiness?.checks.filter((item) => item.status === 'fail').length ?? 0,
  }), [readiness])
  const providerFailures = providerRows.filter((row) => row.statusTone !== 'success').length
  const averageLatency = providerRows.length
    ? Math.round(providers.reduce((total, provider) => total + Number(provider.latency_ms || 0), 0) / providerRows.length)
    : 0
  const maxLatency = Math.max(0, ...providers.map((provider) => Number(provider.latency_ms || 0)))
  const metrics = [
    { label: '上线阻断', value: String(blockingRows.length), trend: blockingRows.length ? '需要优先修复' : '当前无阻断项', tone: blockingRows.length ? 'bad' as const : 'good' as const },
    { label: '检查警告', value: String(summary.warn), trend: `通过 ${summary.pass} · 失败 ${summary.fail}`, tone: summary.warn ? 'warn' as const : 'good' as const },
    { label: '异常探针', value: String(providerFailures), trend: `共 ${providerRows.length} 个上游探针`, tone: providerFailures ? 'bad' as const : 'good' as const },
    { label: '平均延迟', value: `${averageLatency}ms`, trend: `最大 ${maxLatency}ms`, tone: 'neutral' as const },
  ]

  if (initialLoading) return <LoadingBlock label="载入运维监控" />
  if (error) return <ErrorBlock message={error} onRetry={() => void load('initial')} />

  return (
    <section className={adminPage.stack}>
      <PageHeader
        title="系统健康"
        description="阻断项、上线检查和真实上游探针保持在同一诊断工作台，便于值班直接定位并修复。"
        primaryAction={(
          <button type="button" className={cn(adminButton.base, adminButton.primary)} disabled={refreshing} onClick={() => void load('refresh')}>
            <MonitoringIcon className={cn('size-4', refreshing && 'animate-pulse')} />
            <span>{refreshing ? '探测中...' : '重新探测'}</span>
          </button>
        )}
      />
      <MetricStrip metrics={metrics} />

      {refreshing ? <InlineFeedback tone="neutral" message="正在刷新监控数据，当前仍显示上一次诊断结果。" /> : null}
      {refreshError ? <InlineFeedback tone="danger" message={`刷新失败：${refreshError}。当前仍显示上一次诊断结果。`} /> : null}

      <div className={monitoringClasses.contextBar} aria-label="监控上下文">
        <span className={monitoringClasses.contextItem}>上线检查 <strong className={monitoringClasses.contextValue}>{readiness ? readinessOverallStatusLabel(readiness.status) : '未返回'}</strong></span>
        <span className={monitoringClasses.contextItem}>任务队列 <strong className={monitoringClasses.contextValue}>{taskQueuePressure(providers)}</strong></span>
        <span className={monitoringClasses.contextItem}>最近检查 <strong className={monitoringClasses.contextValue}>{healthRefreshTimeLabel(readiness?.generated_at)}</strong></span>
      </div>

      <section data-admin-monitoring-blockers className={monitoringClasses.blockerBand}>
        <header className={monitoringClasses.sectionHead}>
          <div>
            <h2 className={cn('m-0', adminType.sectionTitle)}>优先处理</h2>
            <p className={monitoringClasses.sectionDescription}>阻塞上线的失败检查排在最前，并保留服务端返回的直接修复入口。</p>
          </div>
          <Badge tone={blockingRows.length ? 'danger' : 'success'}>{blockingRows.length ? `${blockingRows.length} 项阻断` : '无阻断'}</Badge>
        </header>
        {blockingRows.length ? (
          <div className={monitoringClasses.tableSurface}>
            <DataTable columns={readinessColumns()} rows={blockingRows} rowKey={(row) => row.key} />
          </div>
        ) : (
          <InlineFeedback tone="success" message="当前没有阻塞上线的检查项。" />
        )}
      </section>

      <div className={monitoringClasses.workbench}>
        <section className={monitoringClasses.section}>
          <header className={monitoringClasses.sectionHead}>
            <div>
              <h2 className={cn('m-0', adminType.sectionTitle)}>上游探针</h2>
              <p className={monitoringClasses.sectionDescription}>展示每个 Provider 的真实状态、延迟、错误率和诊断说明。</p>
            </div>
            <Badge tone={providerFailures ? 'warning' : 'success'}>{providerFailures ? `${providerFailures} 个异常` : '全部正常'}</Badge>
          </header>
          <div className={monitoringClasses.tableSurface}>
            <DataTable
              columns={providerColumns()}
              rows={providerRows}
              rowKey={(row) => row.key}
              empty={<EmptyBlock title="暂无上游探针" detail="Dashboard API 尚未返回 Provider 探针数据。" />}
            />
          </div>
        </section>

        <section className={monitoringClasses.section}>
          <header className={monitoringClasses.sectionHead}>
            <div>
              <h2 className={cn('m-0', adminType.sectionTitle)}>完整上线检查</h2>
              <p className={monitoringClasses.sectionDescription}>阻断、失败和警告优先排序，仍展示全部检查上下文。</p>
            </div>
            <Badge tone={summary.fail ? 'danger' : summary.warn ? 'warning' : 'success'}>{readiness ? readinessOverallStatusLabel(readiness.status) : '未返回'}</Badge>
          </header>
          <div className={monitoringClasses.tableSurface}>
            <DataTable
              columns={readinessColumns()}
              rows={checkRows}
              rowKey={(row) => row.key}
              empty={<EmptyBlock title="暂无上线检查" detail="Readiness API 尚未返回诊断检查。" />}
            />
          </div>
        </section>
      </div>
    </section>
  )
}

function providerColumns(): ColumnDef<HealthProviderRow>[] {
  return [
    { key: 'provider', title: '探针', width: 'minmax(150px,1.4fr)', render: (row) => <strong className="text-[var(--fg)]">{row.name}</strong> },
    { key: 'status', title: '状态', width: 'minmax(90px,.8fr)', render: (row) => <Badge tone={row.statusTone}>{row.statusLabel}</Badge> },
    { key: 'latency', title: '延迟', width: 'minmax(90px,.8fr)', align: 'right', kind: 'number', render: (row) => row.latencyLabel },
    { key: 'errorRate', title: '错误率', width: 'minmax(90px,.8fr)', align: 'right', kind: 'number', render: (row) => row.errorRate },
    { key: 'note', title: '说明', width: 'minmax(180px,2fr)', render: (row) => <span className="[overflow-wrap:anywhere]">{row.note}</span> },
  ]
}

function readinessColumns(): ColumnDef<ReadinessRowModel>[] {
  return [
    {
      key: 'check',
      title: '检查项',
      width: 'minmax(180px,1.5fr)',
      render: (row) => <div className="min-w-0"><strong className="text-[var(--fg)]">{row.label}</strong><div className="mt-1 font-[family-name:var(--admin-font-mono)] text-xs text-[var(--soft)]">{row.key}</div></div>,
    },
    { key: 'status', title: '状态', width: 'minmax(90px,.7fr)', render: (row) => <Badge tone={row.statusTone}>{row.status}</Badge> },
    { key: 'impact', title: '上线影响', width: 'minmax(100px,.9fr)', render: (row) => <Badge tone={row.blockingTone}>{row.blockingLabel}</Badge> },
    { key: 'detail', title: '诊断说明', width: 'minmax(220px,2.2fr)', render: (row) => <span className="[overflow-wrap:anywhere]">{row.detail}</span> },
    { key: 'action', title: '修复入口', width: 'minmax(120px,1fr)', align: 'right', render: (row) => <a className={cn(adminButton.base, adminButton.secondary, adminButton.small)} href={row.actionHref}>{row.actionLabel}</a> },
  ]
}

function sortReadinessRows(rows: ReadinessRowModel[]) {
  return rows.slice().sort((left, right) => readinessPriority(left) - readinessPriority(right))
}

function readinessPriority(row: ReadinessRowModel) {
  if (row.rawStatus === 'fail' && row.blockingTone === 'danger') return 0
  if (row.rawStatus === 'fail') return 1
  if (row.rawStatus === 'warn') return 2
  return 3
}
