import { useEffect, useState } from 'react'
import type { CallRecord } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader } from '../components'

export function CallRecordsPage() {
  const [rows, setRows] = useState<CallRecord[]>([])
  const [status, setStatus] = useState('')
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const result = await adminApi.listCallRecords({ page, page_size: 20, status })
      setRows(result.items)
      setTotal(result.total)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '调用记录载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [page])

  if (loading) return <LoadingBlock label="载入调用记录" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className="page-stack">
      <PageHeader eyebrow="Call Records" title="调用记录" detail="按任务、Provider、渠道、状态追踪真实调用和成本。" actions={<form className="search-form" onSubmit={(event) => { event.preventDefault(); void load() }}><input value={status} onChange={(event) => setStatus(event.target.value)} placeholder="status 过滤" /><button className="btn" type="submit">查询</button></form>} />
      <section className="pg-admin-card ops-surface full-main">
        <section className="main-lane table-lane no-divider">
          <div className="card-header lane-head compact"><span>第 {page} 页 / 共 {total} 条</span><div className="row-actions buttons"><button className="ghost small" type="button" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>上一页</button><button className="ghost small" type="button" disabled={page * 20 >= total} onClick={() => setPage((value) => value + 1)}>下一页</button></div></div>
          {!rows.length ? <EmptyBlock title="暂无调用记录" detail="生成任务执行后会出现在这里。" /> : (
            <>
              <div className="table-head route-grid"><span>任务</span><span>用户</span><span>渠道</span><span>Provider</span><span>状态</span><span>成本</span></div>
              {rows.map((row) => (
                <div key={row.id} className="table-row route-grid">
                  <div><strong>{row.task_id}</strong><p>{row.created_at}</p></div>
                  <span>{row.user_id}</span>
                  <span>{row.source_channel}</span>
                  <span>{row.provider}</span>
                  <Badge tone={row.status === 'succeeded' ? 'success' : row.status === 'failed' ? 'danger' : 'warning'}>{row.status}</Badge>
                  <span>{row.actual_points ?? '-'} / {row.provider_cost ?? '-'}</span>
                </div>
              ))}
            </>
          )}
        </section>
      </section>
    </section>
  )
}
