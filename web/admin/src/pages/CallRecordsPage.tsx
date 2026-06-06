import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import type { CallRecord } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader } from '../components'
import { callRecordFilterCopy, callRecordRows, callRecordSourceChannelOptions, callRecordStatusOptions } from './callRecordRows'

const pageSize = 20

const commonErrorCodes = [
  '',
  'MODEL_ROUTE_NOT_FOUND',
  'MODEL_ROUTE_NO_CANDIDATE',
  'ROUTE_MODEL_PRICE_MISSING',
  'MODEL_ROUTE_NOT_VISIBLE',
  'INSUFFICIENT_BALANCE',
  'PROVIDER_UNAVAILABLE',
]

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
    <section className="page-stack">
      <PageHeader
        eyebrow="Call Records"
        title="调用记录"
        detail="按任务、Provider、渠道、状态追踪真实调用、前置失败和成本。"
        actions={<button className="btn" type="button" onClick={() => void load()}>刷新</button>}
      />
      <section className="pg-admin-card ops-surface full-main">
        <section className="main-lane table-lane no-divider">
          <form className="search-form call-record-filters filter-band" onSubmit={submitFilters}>
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
              {commonErrorCodes.filter(Boolean).map((code) => <option key={code} value={code} />)}
            </datalist>
            <select value={filters.sourceChannel} onChange={(event) => setFilters((value) => ({ ...value, sourceChannel: event.target.value }))} aria-label={callRecordFilterCopy.sourceChannel.label}>
              {callRecordSourceChannelOptions.map((option) => <option key={option.value || 'all'} value={option.value}>{option.label}</option>)}
            </select>
            <input value={filters.provider} onChange={(event) => setFilters((value) => ({ ...value, provider: event.target.value }))} aria-label={callRecordFilterCopy.provider.label} placeholder={callRecordFilterCopy.provider.placeholder} />
            <input value={filters.userId} onChange={(event) => setFilters((value) => ({ ...value, userId: event.target.value }))} aria-label={callRecordFilterCopy.userId.label} placeholder={callRecordFilterCopy.userId.placeholder} inputMode="numeric" />
            <input className="call-record-task-filter" value={filters.taskId} onChange={(event) => setFilters((value) => ({ ...value, taskId: event.target.value }))} aria-label={callRecordFilterCopy.taskId.label} placeholder={callRecordFilterCopy.taskId.placeholder} />
            <button className="btn" type="submit">查询</button>
            <button className="ghost small" type="button" onClick={resetFilters}>重置</button>
          </form>
          <div className="card-header lane-head compact"><span>第 {page} 页 / 共 {total} 条</span><div className="row-actions buttons"><button className="ghost small" type="button" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>上一页</button><button className="ghost small" type="button" disabled={page * pageSize >= total} onClick={() => setPage((value) => value + 1)}>下一页</button></div></div>
          {!rows.length ? <EmptyBlock title="暂无调用记录" detail="生成任务执行后会出现在这里。" /> : (
            <div className="admin-data-grid call-record-grid">
              <div className="table-head"><span>任务现场</span><span>用户/入口</span><span>模型</span><span>Provider</span><span>状态</span><span>错误</span><span>积分/成本</span><span>时间线</span></div>
              {viewRows.map((row) => (
                <div key={row.id} className="table-row">
                  <div title={row.fullTaskId}>
                    <strong>{row.taskLabel}</strong>
                    <p>{row.taskDetail}</p>
                  </div>
                  <div><strong>{row.userLabel}</strong><p>{row.userDetail}</p></div>
                  <div><strong>{row.routeLabel}</strong><p>{row.routeDetail}</p></div>
                  <div><strong>{row.providerLabel}</strong><p>{row.providerDetail}</p></div>
                  <Badge tone={row.statusTone}>{row.statusLabel}</Badge>
                  <div className={row.statusTone === 'danger' ? 'call-record-failure' : undefined}>
                    <strong>{row.failureLabel}</strong>
                    <p>{row.failureDetail}</p>
                  </div>
                  <div>
                    <strong>{row.amountLabel}</strong>
                    <p>成本 {row.costLabel} · 毛利 {row.marginLabel}</p>
                  </div>
                  <div>
                    <strong>{row.createdAt}</strong>
                    <p>{row.lifecycleLabel}</p>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>
      </section>
    </section>
  )
}
