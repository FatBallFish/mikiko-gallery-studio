import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import type { CallRecord, CallRecordAttempt } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { adminDataGrid, adminGridCols } from '../ui/dataGrid'
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
  surface: adminPage.fullSurface,
  filterForm: 'mb-4 flex flex-wrap items-center gap-2 rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-white/70 p-3',
  taskFilter: 'min-w-[220px] flex-1',
  laneHead: 'mb-4 flex flex-wrap items-center justify-between gap-3 border-b border-[var(--line)] pb-3',
  pageActions: 'flex flex-wrap items-center gap-2',
  stackCell: cn(adminDataGrid.stackCell, 'gap-0.5'),
  dangerCell: cn(adminDataGrid.stackCell, 'gap-0.5 text-[var(--red)]'),
  paragraph: 'm-0 text-xs text-[var(--soft)] [overflow-wrap:anywhere]',
  detailPanel: 'min-w-[1180px] border-b border-[var(--line)] bg-[var(--pg-admin-bg-subtle)] px-4 py-4 last:border-b-0',
  detailGrid: 'grid gap-3 lg:grid-cols-[minmax(0,1.25fr)_minmax(0,1fr)]',
  detailBox: 'min-w-0 rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-white/80 p-3',
  detailTitle: 'mb-2 text-xs font-extrabold uppercase tracking-[.12em] text-[var(--soft)]',
  attemptList: 'grid gap-2',
  attemptItem: 'min-w-0 rounded-[var(--pg-radius-sm)] border border-[var(--line)] bg-white p-3',
  detailMeta: 'mt-1 text-xs text-[var(--soft)] [overflow-wrap:anywhere]',
  detailCode: 'mt-2 max-h-[220px] overflow-auto rounded-[var(--pg-radius-sm)] bg-[rgba(20,31,46,.04)] p-3 font-mono text-xs text-[var(--ink)]',
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
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [expandedTaskId, setExpandedTaskId] = useState<string | null>(null)
  const viewRows = useMemo(() => callRecordRows(rows), [rows])

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
      <PageHeader
        eyebrow="Call Records"
        title="调用记录"
        detail="按任务、Provider、渠道、状态追踪真实调用、前置失败和成本。"
        actions={<button className={adminButton.base} type="button" onClick={() => void load()}>刷新</button>}
      />
      <section className={callRecordClasses.surface}>
        <section className={adminPage.mainLane}>
          <form className={callRecordClasses.filterForm} onSubmit={submitFilters}>
            <select value={filters.status} onChange={(event) => setFilters((value) => ({ ...value, status: event.target.value }))} aria-label="按状态过滤">
              {callRecordStatusOptions.map((option) => <option key={option.value || 'all'} value={option.value}>{option.label}</option>)}
            </select>
            <input
              value={filters.errorCode}
              onChange={(event) => setFilters((value) => ({ ...value, errorCode: event.target.value }))}
              list="call-record-error-codes"
              aria-label={callRecordFilterCopy.errorCode.label}
              placeholder={callRecordFilterCopy.errorCode.placeholder}
            />
            <datalist id="call-record-error-codes">
              {callRecordCommonErrorCodes.filter(Boolean).map((code) => <option key={code} value={code} />)}
            </datalist>
            <select value={filters.sourceChannel} onChange={(event) => setFilters((value) => ({ ...value, sourceChannel: event.target.value }))} aria-label={callRecordFilterCopy.sourceChannel.label}>
              {callRecordSourceChannelOptions.map((option) => <option key={option.value || 'all'} value={option.value}>{option.label}</option>)}
            </select>
            <input value={filters.provider} onChange={(event) => setFilters((value) => ({ ...value, provider: event.target.value }))} aria-label={callRecordFilterCopy.provider.label} placeholder={callRecordFilterCopy.provider.placeholder} />
            <input value={filters.userId} onChange={(event) => setFilters((value) => ({ ...value, userId: event.target.value }))} aria-label={callRecordFilterCopy.userId.label} placeholder={callRecordFilterCopy.userId.placeholder} inputMode="numeric" />
            <input className={callRecordClasses.taskFilter} value={filters.taskId} onChange={(event) => setFilters((value) => ({ ...value, taskId: event.target.value }))} aria-label={callRecordFilterCopy.taskId.label} placeholder={callRecordFilterCopy.taskId.placeholder} />
            <button className={adminButton.base} type="submit">查询</button>
            <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={resetFilters}>重置</button>
          </form>
          <div className={callRecordClasses.laneHead}>
            <span>第 {page} 页 / 共 {total} 条</span>
            <div className={callRecordClasses.pageActions}>
              <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>上一页</button>
              <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" disabled={page * pageSize >= total} onClick={() => setPage((value) => value + 1)}>下一页</button>
            </div>
          </div>
          {!rows.length ? <EmptyBlock title="暂无调用记录" detail="生成任务执行后会出现在这里。" /> : (
            <div className={adminDataGrid.root}>
              <div className={cn(adminDataGrid.head, adminGridCols.callRecords)}><span>任务现场</span><span>用户/入口</span><span>模型</span><span>Provider</span><span>状态</span><span>错误</span><span>积分/成本</span><span>时间线</span></div>
              {viewRows.map((row, index) => {
                const record = rows[index]
                const expanded = expandedTaskId === row.fullTaskId
                return (
                  <div key={row.id}>
                    <div className={cn(adminDataGrid.row, adminGridCols.callRecords)}>
                      <div className={callRecordClasses.stackCell} title={row.fullTaskId}>
                        <strong>{row.taskLabel}</strong>
                        <p className={callRecordClasses.paragraph}>{row.taskDetail}</p>
                      </div>
                      <div className={callRecordClasses.stackCell}>
                        <strong>{row.userLabel}</strong>
                        <p className={callRecordClasses.paragraph}>{row.userDetail}</p>
                      </div>
                      <div className={callRecordClasses.stackCell}>
                        <strong>{row.routeLabel}</strong>
                        <p className={callRecordClasses.paragraph}>{row.routeDetail}</p>
                      </div>
                      <div className={callRecordClasses.stackCell}>
                        <strong>{row.providerLabel}</strong>
                        <p className={callRecordClasses.paragraph}>{row.providerDetail}</p>
                      </div>
                      <Badge tone={row.statusTone}>{row.statusLabel}</Badge>
                      <div className={row.statusTone === 'danger' ? callRecordClasses.dangerCell : callRecordClasses.stackCell}>
                        <strong>{row.failureLabel}</strong>
                        <p className={callRecordClasses.paragraph}>{row.failureDetail}</p>
                        {record && hasCallRecordDetails(record) && (
                          <button
                            className={callRecordClasses.inlineAction}
                            type="button"
                            aria-expanded={expanded}
                            onClick={() => setExpandedTaskId(expanded ? null : row.fullTaskId)}
                          >
                            {expanded ? '收起详情' : '查看详情'}
                          </button>
                        )}
                      </div>
                      <div className={callRecordClasses.stackCell}>
                        <strong>{row.amountLabel}</strong>
                        <p className={callRecordClasses.paragraph}>成本 {row.costLabel} · 毛利 {row.marginLabel}</p>
                      </div>
                      <div className={callRecordClasses.stackCell}>
                        <strong>{row.createdAt}</strong>
                        <p className={callRecordClasses.paragraph}>{row.lifecycleLabel}</p>
                      </div>
                    </div>
                    {record && expanded && <CallRecordDetail record={record} />}
                  </div>
                )
              })}
            </div>
          )}
        </section>
      </section>
    </section>
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

function stableJson(value: Record<string, unknown>) {
  return JSON.stringify(value, Object.keys(value).sort(), 2)
}

function formatAttemptTime(value?: string | null) {
  if (!value) return '-'
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!match) return value
  return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
}
