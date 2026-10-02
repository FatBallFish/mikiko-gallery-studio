import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import type { AdminVideoTaskDetail, AdminVideoTaskSummary } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, Drawer, EmptyBlock, ErrorBlock, InlineFeedback, LoadingBlock, PageHeader, RefreshIconButton } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { FilterToolbar } from '../ui/dataTable'

type Filters = { userId: string; taskId: string; providerTaskId: string; projectId: string; routeModelId: string; accountModelId: string; status: string }
const emptyFilters: Filters = { userId: '', taskId: '', providerTaskId: '', projectId: '', routeModelId: '', accountModelId: '', status: '' }
const PAGE_SIZE = 20

// Cursor pagination walks forward only; the visited cursors double as the
// back stack so 上一页 rewinds to the anchor of the previous page.
export function VideoTasksPage() {
  const [filters, setFilters] = useState(emptyFilters)
  const [applied, setApplied] = useState(emptyFilters)
  const [rows, setRows] = useState<AdminVideoTaskSummary[]>([])
  const [cursorStack, setCursorStack] = useState<string[]>([])
  const [nextCursor, setNextCursor] = useState('')
  const [selectedID, setSelectedID] = useState<string | null>(null)
  const [selected, setSelected] = useState<AdminVideoTaskDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [pageLoading, setPageLoading] = useState(false)
  const [recovering, setRecovering] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)

  async function load(cursor: string, push: boolean) {
    rows.length || pageLoading ? setRefreshing(true) : setLoading(true)
    setError(null)
    try {
      const page = await adminApi.listAdminVideoTasks({ user_id: applied.userId || undefined, task_id: applied.taskId || undefined, provider_task_id: applied.providerTaskId || undefined, project_id: applied.projectId || undefined, route_model_id: applied.routeModelId || undefined, account_model_id: applied.accountModelId || undefined, status: applied.status || undefined, cursor: cursor || undefined, limit: PAGE_SIZE })
      setRows(page.items)
      setNextCursor(page.next_cursor ?? '')
      setCursorStack((current) => push && cursor ? [...current, cursor] : current)
      if (selectedID) {
        if (page.items.some((row) => row.id === selectedID)) await selectTask(selectedID)
        else closeTask()
      }
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '视频任务载入失败')
    } finally {
      setLoading(false); setRefreshing(false); setPageLoading(false)
    }
  }

  async function selectTask(id: string) {
    setSelectedID(id)
    try { setSelected(await adminApi.getAdminVideoTask(id)); setError(null) }
    catch (caught) { setError(caught instanceof Error ? caught.message : '视频任务详情载入失败') }
  }

  function closeTask() {
    setSelectedID(null)
    setSelected(null)
  }

  async function retryArtifact(taskId: string, itemId: string) {
    setRecovering(itemId); setNotice(null)
    try {
      const result = await adminApi.retryAdminVideoArtifact(taskId, itemId)
      if (result.provider_generation_requested === false) setNotice('已重新排队转存，未请求模型重新生成，也不会重复扣费。')
      await selectTask(taskId)
    } catch (caught) { setError(caught instanceof Error ? caught.message : '重新转存失败') }
    finally { setRecovering(null) }
  }

  async function retryDerivative(jobId: string) {
    setRecovering(jobId); setNotice(null)
    try {
      const result = await adminApi.retryMediaProcessingJob(jobId)
      if (result.provider_generation_requested === false) setNotice('已重新排队处理派生资源，未请求模型重新生成，也不会重复扣费。')
      if (selected) await selectTask(selected.id)
    } catch (caught) { setError(caught instanceof Error ? caught.message : '重新处理失败') }
    finally { setRecovering(null) }
  }

  async function retrySettlement(taskId: string) {
    setRecovering(`settlement:${taskId}`); setNotice(null)
    try {
      const result = await adminApi.retryAdminVideoSettlement(taskId)
      if (result.provider_generation_requested === false) setNotice('已重新排队结算或退回预留积分，未请求模型重新生成，也不会重复扣费。')
      await selectTask(taskId)
    } catch (caught) { setError(caught instanceof Error ? caught.message : '重新结算失败') }
    finally { setRecovering(null) }
  }

  useEffect(() => { setCursorStack([]); void load('', false) }, [applied])
  const submit = (event: FormEvent) => { event.preventDefault(); setApplied(filters) }

  const page = cursorStack.length + 1
  if (loading && !rows.length) return <LoadingBlock label="载入视频任务" />
  if (error && !rows.length) return <ErrorBlock message={error} onRetry={() => void load(cursorStack[cursorStack.length - 1] ?? '', false)} />
  return (
    <section className={adminPage.stack}>
      <PageHeader title="视频任务" description="按任务、用户、项目、模型和状态定位视频生成链路；恢复操作仅处理转存与派生资源。点击任务行查看详情。" secondaryActions={<RefreshIconButton label="手动刷新视频任务" refreshing={refreshing} onClick={() => void load(cursorStack[cursorStack.length - 1] ?? '', false)} />} />
      <form onSubmit={submit}>
        <FilterToolbar
          fields={[
            { key: 'user', label: '用户 ID', primary: true, control: <input value={filters.userId} onChange={(event) => setFilters({ ...filters, userId: event.target.value })} /> },
            { key: 'task', label: '平台任务 ID', primary: true, control: <input value={filters.taskId} onChange={(event) => setFilters({ ...filters, taskId: event.target.value })} /> },
            { key: 'provider', label: '厂商任务 ID', primary: true, control: <input value={filters.providerTaskId} onChange={(event) => setFilters({ ...filters, providerTaskId: event.target.value })} /> },
            { key: 'status', label: '状态', primary: true, control: <select value={filters.status} onChange={(event) => setFilters({ ...filters, status: event.target.value })}><option value="">全部状态</option><option value="queued">排队中</option><option value="running">执行中</option><option value="succeeded">成功</option><option value="failed">失败</option><option value="partial">部分成功</option></select> },
            { key: 'project', label: '项目 ID', control: <input value={filters.projectId} onChange={(event) => setFilters({ ...filters, projectId: event.target.value })} /> },
            { key: 'route', label: '路由模型', control: <input value={filters.routeModelId} onChange={(event) => setFilters({ ...filters, routeModelId: event.target.value })} inputMode="numeric" /> },
            { key: 'model', label: '真实模型', control: <input value={filters.accountModelId} onChange={(event) => setFilters({ ...filters, accountModelId: event.target.value })} inputMode="numeric" /> },
          ]}
          actions={<><button className={cn(adminButton.base, adminButton.primary, adminButton.small)} type="submit">查询</button><button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => { setFilters(emptyFilters); setApplied(emptyFilters) }}>重置</button></>}
          resultSummary={`第 ${page} 页 · 当前 ${rows.length} 条`}
        />
      </form>
      {error ? <InlineFeedback tone="danger" message={error} /> : null}
      {notice ? <InlineFeedback tone="success" message={notice} /> : null}
      {!rows.length ? <EmptyBlock title="暂无视频任务" detail="提交视频生成后，任务与 attempt 会显示在这里。" /> : (
        <div className="min-w-0 overflow-x-auto border-t border-[var(--border)]">
          <table className="admin-table min-w-[860px]">
            <thead><tr><th>任务 / 用户</th><th>路由</th><th>状态</th><th>结算状态</th><th>实际积分</th><th>创建时间</th></tr></thead>
            <tbody>
              {rows.map((row) => <tr key={row.id} className={cn('cursor-pointer', selectedID === row.id && 'bg-[var(--accent-soft)]')} onClick={() => void selectTask(row.id)}>
                <td><span className="font-mono text-xs font-semibold text-[var(--text)]">{row.id}</span><div className="mt-1 text-xs text-[var(--soft)]">用户 {row.user_id}</div></td>
                <td>{row.route_model_code}</td>
                <td><Badge tone={row.status === 'failed' ? 'danger' : row.status === 'succeeded' ? 'success' : 'neutral'}>{row.status}</Badge></td>
                <td>{row.settlement_status}</td>
                <td>{row.actual_points}</td>
                <td className="whitespace-nowrap text-xs text-[var(--soft)]">{formatAdminTaskTime(row.created_at)}</td>
              </tr>)}
            </tbody>
          </table>
          <div className="flex items-center justify-between gap-3 py-3">
            <span className="text-xs text-[var(--soft)]">每页 {PAGE_SIZE} 条 · 第 {page} 页</span>
            <div className="flex gap-2">
              <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" disabled={pageLoading || !cursorStack.length} onClick={() => { const previous = cursorStack[cursorStack.length - 2] ?? ''; setCursorStack((current) => current.slice(0, -1)); setPageLoading(true); void load(previous, false) }}>上一页</button>
              <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" disabled={pageLoading || !nextCursor} onClick={() => { setPageLoading(true); void load(nextCursor, true) }}>下一页</button>
            </div>
          </div>
        </div>
      )}
      {selectedID ? <Drawer title="视频任务详情" description={selectedID} onClose={closeTask}>
        <TaskDiagnostics detail={selected} recovering={recovering} onRetryArtifact={retryArtifact} onRetryDerivative={retryDerivative} onRetrySettlement={retrySettlement} />
      </Drawer> : null}
    </section>
  )
}

function formatAdminTaskTime(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(date)
}

function TaskDiagnostics({ detail, recovering, onRetryArtifact, onRetryDerivative, onRetrySettlement }: { detail: AdminVideoTaskDetail | null; recovering: string | null; onRetryArtifact: (taskId: string, itemId: string) => void; onRetryDerivative: (jobId: string) => void; onRetrySettlement: (taskId: string) => void }) {
  // Cancelled/queued items legitimately have no attempts yet — the detail API
  // may return null collections, so never assume arrays here.
  if (!detail) return <LoadingBlock label="载入任务详情" />
  const items = Array.isArray(detail.items) ? detail.items : []
  return <section className="grid min-w-0 gap-4">
    <div className="grid gap-3 text-sm sm:grid-cols-2">
      <span>状态 <Badge tone={detail.status === 'failed' ? 'danger' : detail.status === 'succeeded' ? 'success' : 'neutral'}>{detail.status}</Badge></span>
      <span>路由模型 <strong>{detail.route_model_code}</strong></span>
      <span>创建时间 <strong>{formatAdminTaskTime(detail.created_at)}</strong></span>
      <span>更新时间 <strong>{formatAdminTaskTime(detail.updated_at)}</strong></span>
    </div>
    <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border)] pt-4"><div className="grid flex-1 gap-2 text-sm sm:grid-cols-3"><span>预估积分 <strong>{detail.estimated_points}</strong></span><span>预留积分 <strong>{detail.reserved_points}</strong></span><span>结算状态 <strong>{detail.settlement_status}</strong></span></div>{detail.settlement_status !== 'finalized' && ['succeeded', 'failed', 'partial', 'cancelled'].includes(detail.status) ? <button className={cn(adminButton.base, adminButton.secondary, adminButton.small)} type="button" disabled={recovering === `settlement:${detail.id}`} onClick={() => onRetrySettlement(detail.id)}>重新结算</button> : null}</div>
    {items.map((item) => {
      const attempts = Array.isArray(item.attempts) ? item.attempts : []
      return <article key={item.id} className="grid gap-3 rounded-lg border border-[var(--border)] bg-[var(--surface)] p-4">
        <header className="flex flex-wrap items-center justify-between gap-3"><strong>结果 {item.ordinal + 1} · {item.stage}</strong><button className={cn(adminButton.base, adminButton.secondary, adminButton.small)} type="button" disabled={recovering === item.id} onClick={() => onRetryArtifact(detail.id, item.id)}>重新转存</button></header>
        <p className="m-0 text-xs text-[var(--soft)]">结果成本 {item.provider_cost} · 结算积分 {item.actual_points} · {item.error_code || '无错误'}</p>
        {!attempts.length ? <p className="m-0 text-xs text-[var(--soft)]">暂无调用尝试（任务尚未提交到上游或已被取消）。</p> : attempts.map((attempt) => <div key={attempt.id} className="grid gap-2 border-t border-[var(--border)] pt-3 text-xs"><div className="flex flex-wrap justify-between gap-2"><strong>Attempt {attempt.attempt_no} · {attempt.provider_code} / {attempt.model_code}</strong><span>{attempt.status} · provider_cost {attempt.provider_cost}</span></div><code className="max-h-36 overflow-auto rounded bg-[var(--canvas)] p-3 [overflow-wrap:anywhere]">usage_normalized {JSON.stringify(attempt.usage_normalized)}{`\n`}cost_snapshot {JSON.stringify(attempt.cost_snapshot)}</code></div>)}
        {typeof item.artifact_snapshot?.processing_job_id === 'string' ? <button className={cn(adminButton.base, adminButton.ghost, adminButton.small, 'w-fit')} type="button" disabled={recovering === item.artifact_snapshot.processing_job_id} onClick={() => onRetryDerivative(String(item.artifact_snapshot.processing_job_id))}>重新处理</button> : null}
      </article>
    })}
    <details><summary className="cursor-pointer text-sm font-semibold">定价与路由快照</summary><pre className="mt-3 max-h-72 overflow-auto rounded bg-[var(--canvas)] p-3 text-xs">{JSON.stringify({ pricing_snapshot: detail.pricing_snapshot, routing_snapshot: detail.routing_snapshot }, null, 2)}</pre></details>
  </section>
}
