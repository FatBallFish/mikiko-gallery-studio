import { useEffect, useMemo, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import type { AdminMetric, CallRecord, CallRecordAttempt } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, InlineFeedback, LoadingBlock, MetricStrip, PageHeader, RefreshIconButton } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { adminDataGrid } from '../ui/dataGrid'
import { FilterToolbar, ListPage, Pager } from '../ui/dataTable'
import { callRecordCommonErrorCodes, callRecordFilterCopy, callRecordRepair, callRecordRows, callRecordSourceChannelOptions, callRecordStatusOptions } from './callRecordRows'

type CallRecordFilters = {
  status: string
  errorCode: string
  sourceChannel: string
  provider: string
  userId: string
  taskId: string
}

const initialFilters: CallRecordFilters = {
  status: '',
  errorCode: '',
  sourceChannel: '',
  provider: '',
  userId: '',
  taskId: '',
}

const callRecordClasses = {
  stackCell: cn(adminDataGrid.stackCell, 'gap-0.5'),
  dangerCell: cn(adminDataGrid.stackCell, 'gap-0.5 text-[var(--red)]'),
  paragraph: 'm-0 text-xs text-[var(--soft)] [overflow-wrap:anywhere]',
  distributionGrid: 'grid grid-cols-3 gap-4 max-[1100px]:grid-cols-1',
  distributionCard: 'grid gap-3 border-t border-[var(--border)] pt-4',
  distributionTitle: 'text-sm font-semibold text-[var(--text)]',
  distributionRows: 'grid gap-4',
  distributionTrack: 'h-1.5 overflow-hidden rounded-full bg-[var(--canvas)]',
  distributionFill: 'h-full rounded-full bg-[var(--accent)]',
  tableWrap: 'min-w-0 overflow-x-auto',
  tableTop: 'flex flex-wrap items-center justify-between gap-3 px-6 py-4',
  tableTitle: 'text-sm font-bold text-[var(--text)]',
  table: 'admin-table min-w-[1180px]',
  th: '',
  tr: '',
  trFailed: 'bg-[var(--red)]/5',
  td: 'px-6 py-4 align-middle text-sm text-[var(--muted)]',
  taskId: 'font-mono text-xs font-bold text-[var(--text)]',
  routeName: 'text-xs font-bold text-[var(--accent)]',
  promptPill: 'w-fit rounded-md bg-[var(--canvas)] px-1.5 py-0.5 font-mono text-xs text-[var(--muted)]',
  promptText: 'max-w-[220px] truncate text-xs text-[var(--soft)]',
  points: 'font-bold text-[var(--green)]',
  detailPanel: 'min-w-[1180px] bg-[var(--canvas)] px-4 py-4',
  detailGrid: 'grid gap-3 lg:grid-cols-[minmax(0,1.25fr)_minmax(0,1fr)]',
  detailBox: 'min-w-0 rounded-lg bg-[var(--surface-solid)] p-4',
  detailTitle: 'mb-2 text-xs font-semibold text-[var(--soft)]',
  attemptList: 'grid gap-2',
  attemptItem: 'min-w-0 rounded-lg bg-[var(--surface-solid)] p-3',
  detailMeta: 'mt-1 text-xs text-[var(--soft)] [overflow-wrap:anywhere]',
  detailCode: 'mt-2 max-h-[220px] overflow-auto rounded-lg bg-[var(--canvas)] p-3 font-mono text-xs text-[var(--soft)]',
  inlineAction: cn(adminButton.base, adminButton.ghost, adminButton.small, 'mt-1 w-fit'),
}

function callRecordQuery(filters: CallRecordFilters, page: number, pageSize: number): Record<string, string | number | undefined> {
  return {
    page,
    page_size: pageSize,
    status: filters.status || undefined,
    error_code: filters.errorCode || undefined,
    source_channel: filters.sourceChannel || undefined,
    provider: filters.provider || undefined,
    user_id: filters.userId || undefined,
    task_id: filters.taskId || undefined,
  }
}

export function CallRecordsPage() {
  const [rows, setRows] = useState<CallRecord[]>([])
  const [filters, setFilters] = useState<CallRecordFilters>(initialFilters)
  const [appliedFilters, setAppliedFilters] = useState<CallRecordFilters>(initialFilters)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [expandedTaskId, setExpandedTaskId] = useState<string | null>(null)
  const requestGenerationRef = useRef(0)
  const [reloadKey, setReloadKey] = useState(0)
  const viewRows = useMemo(() => callRecordRows(rows), [rows])
  const stats = useMemo(() => callRecordStats(rows), [rows])
  const distributions = useMemo(() => callRecordDistributions(rows), [rows])
  const metrics = useMemo<AdminMetric[]>(() => [
    { label: '当前页调用', value: String(stats.tasks), trend: `筛选结果共 ${total} 条`, tone: 'neutral' },
    { label: '成功出图', value: String(stats.images), trend: '当前页成功结果', tone: 'good' },
    { label: '实际积分', value: String(stats.points), trend: '当前页累计扣费', tone: 'neutral' },
    { label: '平均耗时', value: String(stats.averageLatency), trend: '已完成调用', tone: 'neutral' },
  ], [stats, total])

  const load = async () => {
    const requestGeneration = ++requestGenerationRef.current
    const initialLoad = !rows.length
    if (initialLoad) setLoading(true)
    else setRefreshing(true)
    setError(null)
    try {
      const result = await adminApi.listCallRecords(callRecordQuery(appliedFilters, page, pageSize))
      if (requestGeneration !== requestGenerationRef.current) return
      setRows(result.items)
      setTotal(result.total)
    } catch (caught) {
      if (requestGeneration !== requestGenerationRef.current) return
      setError(caught instanceof Error ? caught.message : '调用记录载入失败')
    } finally {
      if (requestGeneration !== requestGenerationRef.current) return
      setLoading(false)
      setRefreshing(false)
    }
  }

  useEffect(() => { void load() }, [page, appliedFilters, pageSize, reloadKey])

  const submitFilters = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setPage(1)
    setAppliedFilters(filters)
    setReloadKey((value) => value + 1)
  }

  const resetFilters = () => {
    setFilters(initialFilters)
    setAppliedFilters(initialFilters)
    setPage(1)
  }

  if (loading && !rows.length) return <LoadingBlock label="载入调用记录" />
  if (error && !rows.length) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className={adminPage.stack}>
      <PageHeader
        title="调用记录"
        description="优先按任务、用户、Provider 和错误码排查失败调用，统计分布放在日志之后。"
        secondaryActions={<RefreshIconButton label="刷新调用记录" refreshing={refreshing} onClick={() => void load()} />}
      />
      <form onSubmit={submitFilters}>
        <FilterToolbar
          fields={[
            { key: 'userId', label: callRecordFilterCopy.userId.label, primary: true, control: <input value={filters.userId} onChange={(event) => setFilters((value) => ({ ...value, userId: event.target.value }))} placeholder={callRecordFilterCopy.userId.placeholder} inputMode="numeric" /> },
            { key: 'provider', label: callRecordFilterCopy.provider.label, primary: true, control: <input value={filters.provider} onChange={(event) => setFilters((value) => ({ ...value, provider: event.target.value }))} placeholder={callRecordFilterCopy.provider.placeholder} /> },
            { key: 'sourceChannel', label: callRecordFilterCopy.sourceChannel.label, primary: true, control: <select value={filters.sourceChannel} onChange={(event) => setFilters((value) => ({ ...value, sourceChannel: event.target.value }))}>{callRecordSourceChannelOptions.map((option) => <option key={option.value || 'all'} value={option.value}>{option.label}</option>)}</select> },
            { key: 'status', label: '状态', primary: true, control: <select value={filters.status} onChange={(event) => setFilters((value) => ({ ...value, status: event.target.value }))}>{callRecordStatusOptions.map((option) => <option key={option.value || 'all'} value={option.value}>{option.label}</option>)}</select> },
            { key: 'taskId', label: callRecordFilterCopy.taskId.label, control: <input value={filters.taskId} onChange={(event) => setFilters((value) => ({ ...value, taskId: event.target.value }))} placeholder={callRecordFilterCopy.taskId.placeholder} /> },
            { key: 'errorCode', label: callRecordFilterCopy.errorCode.label, control: <input value={filters.errorCode} onChange={(event) => setFilters((value) => ({ ...value, errorCode: event.target.value }))} list="call-record-error-codes" placeholder={callRecordFilterCopy.errorCode.placeholder} /> },
          ]}
          actions={(
            <>
              <datalist id="call-record-error-codes">
                {callRecordCommonErrorCodes.filter(Boolean).map((code) => <option key={code} value={code} />)}
              </datalist>
              <button className={cn(adminButton.base, adminButton.primary, adminButton.small)} type="submit">查询</button>
              <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={resetFilters}>重置</button>
            </>
          )}
          resultSummary={`共 ${total} 条调用 · 当前显示 ${rows.length} 条`}
        />
      </form>
      {refreshing ? <InlineFeedback tone="neutral" message="正在刷新调用记录，当前数据会保留到新结果返回。" /> : null}
      {error && rows.length ? <InlineFeedback tone="danger" message={error} /> : null}
      {!rows.length ? <EmptyBlock title="暂无调用记录" detail="生成任务执行后会出现在这里。" /> : (
        <ListPage
          pagination={<Pager page={page} pageSize={pageSize} total={total} onChange={setPage} onPageSizeChange={(size) => { setPageSize(size); setPage(1) }} />}
        >
        <div className={callRecordClasses.tableWrap}>
          <table className={callRecordClasses.table}>
            <thead>
              <tr>
                <th className={callRecordClasses.th}>任务 ID / 时间</th>
                <th className={callRecordClasses.th}>用户信息</th>
                <th className={callRecordClasses.th}>模型路由链路</th>
                <th className={callRecordClasses.th}>配置 / 提示词</th>
                <th className={callRecordClasses.th}>消耗积分</th>
                <th className={callRecordClasses.th}>耗时</th>
                <th className={callRecordClasses.th}>执行状态</th>
              </tr>
            </thead>
            <tbody>
              {viewRows.map((row, index) => {
                const record = rows[index]
                const expanded = expandedTaskId === row.fullTaskId
                return (
                  <CallRecordTableRows
                    key={row.id}
                    row={row}
                    record={record}
                    expanded={expanded}
                    onToggle={() => setExpandedTaskId(expanded ? null : row.fullTaskId)}
                  />
                )
              })}
            </tbody>
          </table>
        </div>
        </ListPage>
      )}
      <MetricStrip metrics={metrics} />
      <div className={callRecordClasses.distributionGrid}>
        <DistributionCard title="路由模型调用量" rows={distributions.routes} />
        <DistributionCard title="底层账号调用量" rows={distributions.providers} />
        <DistributionCard title="底层模型调用量" rows={distributions.models} />
      </div>
    </section>
  )
}

function DistributionCard({ title, rows }: { title: string; rows: DistributionRowModel[] }) {
  return (
    <section className={callRecordClasses.distributionCard}>
      <h3 className={callRecordClasses.distributionTitle}>{title}</h3>
      <div className={callRecordClasses.distributionRows}>
        {rows.length ? rows.map((row) => <DistributionRow key={row.label} row={row} />) : <p className={callRecordClasses.paragraph}>暂无数据</p>}
      </div>
    </section>
  )
}

function DistributionRow({ row }: { row: DistributionRowModel }) {
  return (
    <div className="grid gap-2">
      <div className="flex justify-between gap-3 text-xs font-semibold">
        <span className="min-w-0 truncate text-[var(--muted)]">{row.label}</span>
        <span className="text-[var(--text)]">{row.value} ({row.percent}%)</span>
      </div>
      <div className={callRecordClasses.distributionTrack}>
        <div className={callRecordClasses.distributionFill} style={{ width: `${row.percent}%` }} />
      </div>
    </div>
  )
}

function CallRecordTableRows({
  row,
  record,
  expanded,
  onToggle,
}: {
  row: ReturnType<typeof callRecordRows>[number]
  record: CallRecord | undefined
  expanded: boolean
  onToggle: () => void
}) {
  const repair = callRecordRepair(record?.error_code)
  return (
    <>
      <tr className={cn(callRecordClasses.tr, row.statusTone === 'danger' && callRecordClasses.trFailed)}>
        <td className={callRecordClasses.td}>
          <div className="flex flex-col gap-1" title={row.fullTaskId}>
            <span className={callRecordClasses.taskId}>{row.taskLabel}</span>
            <span className="text-xs text-[var(--muted-strong)]">{row.createdAt}</span>
          </div>
        </td>
        <td className={callRecordClasses.td}>
          <div className="flex flex-col gap-1">
            <span className="font-bold text-[var(--text)]">{row.userLabel}</span>
            <span className="text-xs text-[var(--soft)]">{row.userDetail}</span>
          </div>
        </td>
        <td className={callRecordClasses.td}>
          <div className="flex flex-col gap-1">
            <span className={callRecordClasses.routeName}>{row.routeLabel}</span>
            <span className="text-xs text-[var(--soft)]">↳ {row.providerLabel} / {row.providerDetail}</span>
          </div>
        </td>
        <td className={callRecordClasses.td}>
          <div className="flex flex-col gap-1">
            <span className={callRecordClasses.promptPill}>{row.routeDetail}</span>
            <span className={callRecordClasses.promptText}>{row.taskDetail}</span>
          </div>
        </td>
        <td className={callRecordClasses.td}>
          <div className="flex flex-col gap-1">
            <span className={row.statusTone === 'danger' ? 'text-[var(--muted-strong)]' : callRecordClasses.points}>{row.amountLabel}</span>
            <span className="text-xs text-[var(--soft)]">成本 {row.costLabel}</span>
          </div>
        </td>
        <td className={callRecordClasses.td}><span className="font-mono text-xs text-[var(--muted)]">{latencyLabel(record)}</span></td>
        <td className={callRecordClasses.td}>
          <div className="flex flex-col items-start gap-2">
            <Badge tone={row.statusTone}>{row.statusLabel}</Badge>
            {record && hasCallRecordDetails(record) ? (
              <button className={callRecordClasses.inlineAction} type="button" aria-expanded={expanded} onClick={onToggle}>{expanded ? '收起详情' : '查看详情'}</button>
            ) : null}
            {row.statusTone === 'danger' ? <span className="max-w-[160px] truncate text-xs text-[var(--red)]">{row.failureLabel}</span> : null}
            {repair ? <a className={cn(adminButton.base, adminButton.ghost, adminButton.small)} href={repair.href}>{repair.label}</a> : null}
          </div>
        </td>
      </tr>
      {record && expanded ? (
        <tr className="border-b border-[var(--line)]/60">
          <td colSpan={7} className="p-0">
            <CallRecordDetail record={record} />
          </td>
        </tr>
      ) : null}
    </>
  )
}

function CallRecordDetail({ record }: { record: CallRecord }) {
  return (
    <section className={callRecordClasses.detailPanel} aria-label="调用错误详情">
      <div className={callRecordClasses.detailGrid}>
        <div className={callRecordClasses.detailBox}>
          <h3 className={callRecordClasses.detailTitle}>调用尝试</h3>
          {!record.attempts?.length ? (
            <p className={callRecordClasses.paragraph}>暂无底层调用尝试明细。</p>
          ) : (
            <div className={callRecordClasses.attemptList}>
              {record.attempts.map((attempt, index) => (
                <AttemptDetail key={`${attempt.provider || 'provider'}-${index}`} attempt={attempt} index={index} />
              ))}
            </div>
          )}
        </div>
        <div className={callRecordClasses.detailBox}>
          <h3 className={callRecordClasses.detailTitle}>错误明细</h3>
          {record.error_detail && Object.keys(record.error_detail).length ? (
            <pre className={callRecordClasses.detailCode}>{stableJson(record.error_detail)}</pre>
          ) : (
            <p className={callRecordClasses.paragraph}>{record.error_message || record.error_code || '暂无结构化错误明细。'}</p>
          )}
        </div>
        <div className={callRecordClasses.detailBox}>
          <h3 className={callRecordClasses.detailTitle}>计费快照</h3>
          {record.pricing_snapshot && Object.keys(record.pricing_snapshot).length ? (
            <pre className={callRecordClasses.detailCode}>{stableJson(record.pricing_snapshot)}</pre>
          ) : (
            <p className={callRecordClasses.paragraph}>该历史记录未保存计费快照。</p>
          )}
        </div>
      </div>
    </section>
  )
}

function AttemptDetail({ attempt, index }: { attempt: CallRecordAttempt; index: number }) {
  const routeParts = [
    attempt.adapter_type,
    attempt.account_model_id ? `账号模型 #${attempt.account_model_id}` : '',
    attempt.model_account_id ? `模型账号 #${attempt.model_account_id}` : '',
    attempt.model_code,
  ].filter(Boolean)
  const errorSummary = [attempt.error_code, attempt.error_message || attempt.error].filter(Boolean).join(' · ')
  return (
    <article className={callRecordClasses.attemptItem}>
      <strong>#{index + 1} {attempt.provider || '-'} · {attempt.status || '-'}</strong>
      <p className={callRecordClasses.detailMeta}>{routeParts.length ? routeParts.join(' · ') : '未记录底层账号信息'}</p>
      <p className={callRecordClasses.detailMeta}>开始 {formatAttemptTime(attempt.started_at)} · 结束 {formatAttemptTime(attempt.finished_at)}</p>
      {errorSummary && <p className={callRecordClasses.detailMeta}>{errorSummary}</p>}
      {attempt.error_detail && Object.keys(attempt.error_detail).length > 0 && (
        <pre className={callRecordClasses.detailCode}>{stableJson(attempt.error_detail)}</pre>
      )}
    </article>
  )
}

function hasCallRecordDetails(record: CallRecord) {
  return Boolean(record.attempts?.length || (record.pricing_snapshot && Object.keys(record.pricing_snapshot).length) || (record.error_detail && Object.keys(record.error_detail).length) || record.error_message || record.error_code)
}

type DistributionRowModel = {
  label: string
  value: number
  percent: number
}

function callRecordStats(records: CallRecord[]) {
  const images = records.reduce((sum, record) => sum + (record.success_output_image_count || 0), 0)
  const points = records.reduce((sum, record) => sum + numeric(record.actual_points), 0)
  const durations = records.map(durationSeconds).filter((value) => value > 0)
  const average = durations.length ? durations.reduce((sum, value) => sum + value, 0) / durations.length : 0
  return {
    tasks: records.length,
    images,
    points: points ? points.toFixed(5).replace(/\.?0+$/, '') : '0',
    averageLatency: average ? `${average.toFixed(1)}s` : '-',
  }
}

function callRecordDistributions(records: CallRecord[]) {
  return {
    routes: topDistribution(records.map((record) => record.abstract_model || '-')),
    providers: topDistribution(records.map((record) => record.provider || '-')),
    models: topDistribution(records.map((record) => record.upstream_model_code || record.provider || '-')),
  }
}

function topDistribution(labels: string[]): DistributionRowModel[] {
  const counts = labels.reduce<Record<string, number>>((acc, label) => {
    acc[label] = (acc[label] ?? 0) + 1
    return acc
  }, {})
  const total = labels.length || 1
  return Object.entries(counts)
    .sort((a, b) => b[1] - a[1])
    .slice(0, 4)
    .map(([label, value]) => ({ label, value, percent: Math.max(1, Math.round((value / total) * 100)) }))
}

function latencyLabel(record?: CallRecord) {
  if (!record) return '-'
  const seconds = durationSeconds(record)
  return seconds ? `${seconds.toFixed(1)}s` : '-'
}

function durationSeconds(record: CallRecord) {
  if (!record.started_at || !record.finished_at) return 0
  const started = new Date(record.started_at).getTime()
  const finished = new Date(record.finished_at).getTime()
  if (!Number.isFinite(started) || !Number.isFinite(finished) || finished <= started) return 0
  return (finished - started) / 1000
}

function numeric(value?: string | null) {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : 0
}

function stableJson(value: Record<string, unknown>) {
  return JSON.stringify(value, Object.keys(value).sort(), 2)
}

function formatAttemptTime(value?: string | null) {
  if (!value) return '-'
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!match) return value
  return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
}
