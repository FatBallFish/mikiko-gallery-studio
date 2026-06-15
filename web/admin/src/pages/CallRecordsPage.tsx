import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import type { CallRecord, CallRecordAttempt } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { adminDataGrid } from '../ui/dataGrid'
import { callRecordCommonErrorCodes, callRecordFilterCopy, callRecordRows, callRecordSourceChannelOptions, callRecordStatusOptions } from './callRecordRows'

const pageSize = 20

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
  filterShell: 'grid gap-4 rounded-3xl border border-[var(--line)] bg-white/[0.02] p-6 shadow-[var(--pg-shadow-sm)] backdrop-blur-sm',
  filterHead: 'flex flex-wrap items-center justify-between gap-3',
  filterTitle: 'text-sm font-extrabold uppercase tracking-widest text-[var(--text)]',
  timePills: 'flex flex-wrap items-center gap-2',
  timePill: 'min-h-8 rounded-xl border border-[var(--line)] bg-white/5 px-3 py-1.5 text-xs font-extrabold text-[var(--muted)] transition hover:bg-white/10 hover:text-[var(--text)]',
  timePillActive: 'border-[var(--accent)] bg-[var(--accent)] text-white',
  filterForm: 'grid gap-3 border-t border-[var(--line)] pt-4',
  filterPrimaryRow: 'flex flex-wrap items-center gap-4',
  filterAdvancedRow: 'grid grid-cols-[repeat(3,minmax(180px,1fr))] gap-3 rounded-2xl border border-[var(--line)] bg-white/[0.02] p-3 max-[860px]:grid-cols-1',
  filterControl: 'w-48 max-w-full',
  filterGrowControl: 'min-w-[220px] flex-1',
  laneHead: 'mb-4 flex flex-wrap items-center justify-between gap-3 border-b border-[var(--line)] pb-3',
  pageActions: 'flex flex-wrap items-center gap-2',
  stackCell: cn(adminDataGrid.stackCell, 'gap-0.5'),
  dangerCell: cn(adminDataGrid.stackCell, 'gap-0.5 text-[var(--red)]'),
  paragraph: 'm-0 text-xs text-[var(--soft)] [overflow-wrap:anywhere]',
  statGrid: 'grid grid-cols-4 gap-3 max-[1100px]:grid-cols-2 max-[620px]:grid-cols-1',
  statCard: 'rounded-3xl border border-[var(--line)] bg-white/[0.02] px-5 py-7 shadow-[var(--pg-shadow-sm)]',
  statLabel: 'text-[10px] font-extrabold uppercase tracking-[.14em] text-[var(--muted-strong)]',
  statValue: 'mt-2 text-3xl font-black tracking-tight text-[var(--text)]',
  distributionGrid: 'grid grid-cols-3 gap-4 max-[1100px]:grid-cols-1',
  distributionCard: 'rounded-3xl border border-[var(--line)] bg-white/[0.02] p-4 shadow-[var(--pg-shadow-sm)]',
  distributionTitle: 'mb-4 text-xs font-extrabold uppercase tracking-widest text-[var(--muted-strong)]',
  distributionRows: 'grid gap-4',
  distributionTrack: 'h-1.5 overflow-hidden rounded-full bg-white/5',
  distributionFill: 'h-full rounded-full bg-[var(--accent)]',
  tableWrap: 'min-w-0 overflow-x-auto rounded-3xl border border-[var(--line)] bg-white/[0.01] shadow-[0_20px_70px_rgba(0,0,0,.18)] backdrop-blur-sm',
  tableTop: 'flex flex-wrap items-center justify-between gap-3 border-b border-[var(--line)] bg-white/[0.01] px-6 py-4',
  tableTitle: 'text-sm font-bold text-[var(--text)]',
  table: 'w-full min-w-[1180px] border-collapse text-left',
  th: 'border-b border-[var(--line)] bg-white/[0.02] px-6 py-4 text-[11px] font-extrabold uppercase tracking-wider text-[var(--muted-strong)]',
  tr: 'border-b border-[var(--line)]/60 transition-colors last:border-b-0 hover:bg-white/[0.03]',
  trFailed: 'bg-[var(--red)]/5',
  td: 'px-6 py-4 align-middle text-sm text-[var(--muted)]',
  taskId: 'font-mono text-xs font-bold text-[var(--text)]',
  routeName: 'text-xs font-bold text-[var(--accent)]',
  promptPill: 'w-fit rounded-md bg-white/5 px-1.5 py-0.5 font-mono text-[10px] text-[var(--muted)]',
  promptText: 'max-w-[220px] truncate text-xs text-[var(--soft)]',
  points: 'font-bold text-[var(--green)]',
  detailPanel: 'min-w-[1180px] border-b border-white/[0.03] bg-white/[0.02] px-4 py-4 last:border-b-0',
  detailGrid: 'grid gap-3 lg:grid-cols-[minmax(0,1.25fr)_minmax(0,1fr)]',
  detailBox: 'min-w-0 rounded-3xl border border-white/5 bg-white/[0.02] p-4',
  detailTitle: 'mb-2 text-xs font-extrabold uppercase tracking-[.12em] text-[var(--soft)]',
  attemptList: 'grid gap-2',
  attemptItem: 'min-w-0 rounded-2xl border border-white/5 bg-white/[0.02] p-3',
  detailMeta: 'mt-1 text-xs text-[var(--soft)] [overflow-wrap:anywhere]',
  detailCode: 'mt-2 max-h-[220px] overflow-auto rounded-2xl bg-black/25 p-3 font-mono text-xs text-[var(--soft)]',
  inlineAction: cn(adminButton.base, adminButton.ghost, adminButton.small, 'mt-1 w-fit'),
}

function callRecordQuery(filters: CallRecordFilters, page: number): Record<string, string | number | undefined> {
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
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [expandedTaskId, setExpandedTaskId] = useState<string | null>(null)
  const viewRows = useMemo(() => callRecordRows(rows), [rows])
  const stats = useMemo(() => callRecordStats(rows), [rows])
  const distributions = useMemo(() => callRecordDistributions(rows), [rows])

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const result = await adminApi.listCallRecords(callRecordQuery(appliedFilters, page))
      setRows(result.items)
      setTotal(result.total)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '调用记录载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [page, appliedFilters])

  const submitFilters = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setPage(1)
    setAppliedFilters(filters)
  }

  const resetFilters = () => {
    setFilters(initialFilters)
    setAppliedFilters(initialFilters)
    setPage(1)
  }

  if (loading) return <LoadingBlock label="载入调用记录" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className={adminPage.stack}>
      <section className={callRecordClasses.filterShell}>
        <div className={callRecordClasses.filterHead}>
          <h3 className={callRecordClasses.filterTitle}>调用记录查询 / Call Records</h3>
          <div className={callRecordClasses.timePills} aria-label="快捷时间范围">
            {['今天', '昨天', '近 7 天', '近 30 天', '自定义区间'].map((label, index) => (
              <button key={label} type="button" className={cn(callRecordClasses.timePill, index === 0 && callRecordClasses.timePillActive)}>{label}</button>
            ))}
          </div>
        </div>
        <form className={callRecordClasses.filterForm} onSubmit={submitFilters}>
          <div className={callRecordClasses.filterPrimaryRow}>
            <input className={callRecordClasses.filterControl} value={filters.userId} onChange={(event) => setFilters((value) => ({ ...value, userId: event.target.value }))} aria-label={callRecordFilterCopy.userId.label} placeholder={callRecordFilterCopy.userId.placeholder} inputMode="numeric" />
            <input className={callRecordClasses.filterControl} value={filters.provider} onChange={(event) => setFilters((value) => ({ ...value, provider: event.target.value }))} aria-label={callRecordFilterCopy.provider.label} placeholder={callRecordFilterCopy.provider.placeholder} />
            <select className={callRecordClasses.filterControl} value={filters.sourceChannel} onChange={(event) => setFilters((value) => ({ ...value, sourceChannel: event.target.value }))} aria-label={callRecordFilterCopy.sourceChannel.label}>
              {callRecordSourceChannelOptions.map((option) => <option key={option.value || 'all'} value={option.value}>{option.label}</option>)}
            </select>
            <select className={callRecordClasses.filterControl} value={filters.status} onChange={(event) => setFilters((value) => ({ ...value, status: event.target.value }))} aria-label="按状态过滤">
              {callRecordStatusOptions.map((option) => <option key={option.value || 'all'} value={option.value}>{option.label}</option>)}
            </select>
            <button className={cn(adminButton.base, adminButton.primary, 'px-8')} type="submit">查询</button>
            <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" aria-expanded={advancedOpen} onClick={() => setAdvancedOpen((value) => !value)}>高级筛选</button>
            <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => void load()}>刷新</button>
          </div>
          {advancedOpen ? (
            <div className={callRecordClasses.filterAdvancedRow}>
              <input
                className={callRecordClasses.filterGrowControl}
                value={filters.taskId}
                onChange={(event) => setFilters((value) => ({ ...value, taskId: event.target.value }))}
                aria-label={callRecordFilterCopy.taskId.label}
                placeholder={callRecordFilterCopy.taskId.placeholder}
              />
              <input
                className={callRecordClasses.filterGrowControl}
                value={filters.errorCode}
                onChange={(event) => setFilters((value) => ({ ...value, errorCode: event.target.value }))}
                list="call-record-error-codes"
                aria-label={callRecordFilterCopy.errorCode.label}
                placeholder={callRecordFilterCopy.errorCode.placeholder}
              />
              <datalist id="call-record-error-codes">
                {callRecordCommonErrorCodes.filter(Boolean).map((code) => <option key={code} value={code} />)}
              </datalist>
              <button className={cn(adminButton.base, adminButton.primary)} type="submit">查询</button>
              <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={resetFilters}>重置</button>
            </div>
          ) : null}
        </form>
      </section>
      <div className={callRecordClasses.statGrid}>
        <StatCard label="区间总任务数" value={stats.tasks} />
        <StatCard label="区间生图数" value={stats.images} />
        <StatCard label="区间消耗积分" value={stats.points} />
        <StatCard label="平均生图耗时" value={stats.averageLatency} />
      </div>
      <div className={callRecordClasses.distributionGrid}>
        <DistributionCard title="路由模型调用量" rows={distributions.routes} />
        <DistributionCard title="底层账号调用量" rows={distributions.providers} />
        <DistributionCard title="底层模型调用量" rows={distributions.models} />
      </div>
      {!rows.length ? <EmptyBlock title="暂无调用记录" detail="生成任务执行后会出现在这里。" /> : (
        <div className={callRecordClasses.tableWrap}>
          <div className={callRecordClasses.tableTop}>
            <h3 className={callRecordClasses.tableTitle}>调用记录明细 / Detailed Logs</h3>
            <div className={callRecordClasses.pageActions}>
              <span className="text-xs text-[var(--muted)]">第 {page} 页 / 共 {total} 条</span>
              <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>上一页</button>
              <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" disabled={page * pageSize >= total} onClick={() => setPage((value) => value + 1)}>下一页</button>
            </div>
          </div>
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
      )}
    </section>
  )
}

function StatCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div className={callRecordClasses.statCard}>
      <div className={callRecordClasses.statLabel}>{label}</div>
      <div className={callRecordClasses.statValue}>{value}</div>
    </div>
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
      <div className="flex justify-between gap-3 text-[10px] font-extrabold uppercase tracking-wider">
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
  return (
    <>
      <tr className={cn(callRecordClasses.tr, row.statusTone === 'danger' && callRecordClasses.trFailed)}>
        <td className={callRecordClasses.td}>
          <div className="flex flex-col gap-1" title={row.fullTaskId}>
            <span className={callRecordClasses.taskId}>{row.taskLabel}</span>
            <span className="text-[10px] text-[var(--muted-strong)]">{row.createdAt}</span>
          </div>
        </td>
        <td className={callRecordClasses.td}>
          <div className="flex flex-col gap-1">
            <span className="font-bold text-[var(--text)]">{row.userLabel}</span>
            <span className="text-[10px] text-[var(--soft)]">{row.userDetail}</span>
          </div>
        </td>
        <td className={callRecordClasses.td}>
          <div className="flex flex-col gap-1">
            <span className={callRecordClasses.routeName}>{row.routeLabel}</span>
            <span className="text-[10px] text-[var(--soft)]">↳ {row.providerLabel} / {row.providerDetail}</span>
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
            <span className="text-[10px] text-[var(--soft)]">成本 {row.costLabel}</span>
          </div>
        </td>
        <td className={callRecordClasses.td}><span className="font-mono text-xs text-[var(--muted)]">{latencyLabel(record)}</span></td>
        <td className={callRecordClasses.td}>
          <div className="flex flex-col items-start gap-2">
            <Badge tone={row.statusTone}>{row.statusLabel}</Badge>
            {record && hasCallRecordDetails(record) ? (
              <button className={callRecordClasses.inlineAction} type="button" aria-expanded={expanded} onClick={onToggle}>{expanded ? '收起详情' : '查看详情'}</button>
            ) : null}
            {row.statusTone === 'danger' ? <span className="max-w-[160px] truncate text-[10px] text-[var(--red)]">{row.failureLabel}</span> : null}
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
  return Boolean(record.attempts?.length || (record.error_detail && Object.keys(record.error_detail).length) || record.error_message || record.error_code)
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
