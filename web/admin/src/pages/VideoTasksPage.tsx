import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import type { AdminVideoTaskDetail, AdminVideoTaskSummary } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, EmptyBlock, ErrorBlock, InlineFeedback, LoadingBlock, PageHeader, RefreshIconButton } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { FilterToolbar } from '../ui/dataTable'

type Filters = { userId: string; taskId: string; providerTaskId: string; projectId: string; routeModelId: string; accountModelId: string; status: string }
const emptyFilters: Filters = { userId: '', taskId: '', providerTaskId: '', projectId: '', routeModelId: '', accountModelId: '', status: '' }

export function VideoTasksPage() {
  const [filters, setFilters] = useState(emptyFilters)
  const [applied, setApplied] = useState(emptyFilters)
  const [rows, setRows] = useState<AdminVideoTaskSummary[]>([])
  const [selected, setSelected] = useState<AdminVideoTaskDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [recovering, setRecovering] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)

  async function load() {
    rows.length ? setRefreshing(true) : setLoading(true)
    setError(null)
    try {
      const page = await adminApi.listAdminVideoTasks({ user_id: applied.userId || undefined, task_id: applied.taskId || undefined, provider_task_id: applied.providerTaskId || undefined, project_id: applied.projectId || undefined, route_model_id: applied.routeModelId || undefined, account_model_id: applied.accountModelId || undefined, status: applied.status || undefined, limit: 100 })
      setRows(page.items)
      if (selected && page.items.some((row) => row.id === selected.id)) await selectTask(selected.id)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '视频任务载入失败')
    } finally {
      setLoading(false); setRefreshing(false)
    }
  }

  async function selectTask(id: string) {
    try { setSelected(await adminApi.getAdminVideoTask(id)); setError(null) }
    catch (caught) { setError(caught instanceof Error ? caught.message : '视频任务详情载入失败') }
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

  useEffect(() => { void load() }, [applied])
  const submit = (event: FormEvent) => { event.preventDefault(); setApplied(filters) }

  if (loading && !rows.length) return <LoadingBlock label="载入视频任务" />
  if (error && !rows.length) return <ErrorBlock message={error} onRetry={load} />
  return (
    <section className={adminPage.stack}>
      <PageHeader title="视频任务" description="按任务、用户、项目、模型和状态定位视频生成链路；恢复操作仅处理转存与派生资源。" secondaryActions={<RefreshIconButton label="手动刷新视频任务" refreshing={refreshing} onClick={() => void load()} />} />
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
          resultSummary={`当前 ${rows.length} 条`}
        />
      </form>
      {error ? <InlineFeedback tone="danger" message={error} /> : null}
      {notice ? <InlineFeedback tone="success" message={notice} /> : null}
      {!rows.length ? <EmptyBlock title="暂无视频任务" detail="提交视频生成后，任务与 attempt 会显示在这里。" /> : (
        <div className="grid min-w-0 gap-5 xl:grid-cols-[minmax(360px,.85fr)_minmax(0,1.4fr)]">
          <div className="min-w-0 overflow-x-auto border-t border-[var(--border)]">
            <table className="admin-table min-w-[720px]"><thead><tr><th>任务 / 用户</th><th>路由</th><th>状态</th><th>结算状态</th><th>实际积分</th></tr></thead><tbody>{rows.map((row) => <tr key={row.id} className={selected?.id === row.id ? 'bg-[var(--accent-soft)]' : ''} onClick={() => void selectTask(row.id)}><td><button className="text-left font-mono text-xs font-semibold text-[var(--text)]" type="button">{row.id}</button><div className="mt-1 text-xs text-[var(--soft)]">用户 {row.user_id}</div></td><td>{row.route_model_code}</td><td><Badge tone={row.status === 'failed' ? 'danger' : row.status === 'succeeded' ? 'success' : 'neutral'}>{row.status}</Badge></td><td>{row.settlement_status}</td><td>{row.actual_points}</td></tr>)}</tbody></table>
          </div>
          <TaskDiagnostics detail={selected} recovering={recovering} onRetryArtifact={retryArtifact} onRetryDerivative={retryDerivative} />
        </div>
      )}
    </section>
  )
}

function TaskDiagnostics({ detail, recovering, onRetryArtifact, onRetryDerivative }: { detail: AdminVideoTaskDetail | null; recovering: string | null; onRetryArtifact: (taskId: string, itemId: string) => void; onRetryDerivative: (jobId: string) => void }) {
  if (!detail) return <EmptyBlock title="选择一个视频任务" detail="查看 attempt、usage、成本、错误、价格与路由快照。" />
  return <section className="grid min-w-0 gap-4 border-t border-[var(--border)] pt-4">
    <div className="grid gap-2 text-sm sm:grid-cols-3"><span>预估积分 <strong>{detail.estimated_points}</strong></span><span>预留积分 <strong>{detail.reserved_points}</strong></span><span>结算状态 <strong>{detail.settlement_status}</strong></span></div>
    {detail.items.map((item) => <article key={item.id} className="grid gap-3 rounded-lg border border-[var(--border)] bg-[var(--surface)] p-4">
      <header className="flex flex-wrap items-center justify-between gap-3"><strong>结果 {item.ordinal + 1} · {item.stage}</strong><button className={cn(adminButton.base, adminButton.secondary, adminButton.small)} type="button" disabled={recovering === item.id} onClick={() => onRetryArtifact(detail.id, item.id)}>重新转存</button></header>
      <p className="m-0 text-xs text-[var(--soft)]">结果成本 {item.provider_cost} · 结算积分 {item.actual_points} · {item.error_code || '无错误'}</p>
      {item.attempts.map((attempt) => <div key={attempt.id} className="grid gap-2 border-t border-[var(--border)] pt-3 text-xs"><div className="flex flex-wrap justify-between gap-2"><strong>Attempt {attempt.attempt_no} · {attempt.provider_code} / {attempt.model_code}</strong><span>{attempt.status} · provider_cost {attempt.provider_cost}</span></div><code className="max-h-36 overflow-auto rounded bg-[var(--canvas)] p-3 [overflow-wrap:anywhere]">usage_normalized {JSON.stringify(attempt.usage_normalized)}{`\n`}cost_snapshot {JSON.stringify(attempt.cost_snapshot)}</code></div>)}
      {typeof item.artifact_snapshot?.processing_job_id === 'string' ? <button className={cn(adminButton.base, adminButton.ghost, adminButton.small, 'w-fit')} type="button" disabled={recovering === item.artifact_snapshot.processing_job_id} onClick={() => onRetryDerivative(String(item.artifact_snapshot.processing_job_id))}>重新处理</button> : null}
    </article>)}
    <details><summary className="cursor-pointer text-sm font-semibold">定价与路由快照</summary><pre className="mt-3 max-h-72 overflow-auto rounded bg-[var(--canvas)] p-3 text-xs">{JSON.stringify({ pricing_snapshot: detail.pricing_snapshot, routing_snapshot: detail.routing_snapshot }, null, 2)}</pre></details>
  </section>
}
