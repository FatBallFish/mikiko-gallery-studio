import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import type { AdminMetric, AdminUser, AdminVideoTaskDetail, AdminVideoTaskSummary, RouteModel } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { AdminTabs, Badge, Drawer, EmptyBlock, ErrorBlock, InlineFeedback, LoadingBlock, MetricStrip, Modal, PageHeader, RefreshIconButton } from '../components'
import { FilterToolbar } from '../ui/dataTable'
import './list-redesign.css'

type Filters = { userId: string; taskId: string; providerTaskId: string; projectId: string; routeModelId: string; accountModelId: string; status: string }
type DetailTab = 'overview' | 'results' | 'pricing'
const emptyFilters: Filters = { userId: '', taskId: '', providerTaskId: '', projectId: '', routeModelId: '', accountModelId: '', status: '' }
const PAGE_SIZE = 20
const TERMINAL_STATUSES = ['succeeded', 'failed', 'partial', 'cancelled']

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
  const [selectedError, setSelectedError] = useState<string | null>(null)
  const [detailTab, setDetailTab] = useState<DetailTab>('overview')
  const [peekUser, setPeekUser] = useState<AdminUser | null>(null)
  const [users, setUsers] = useState<Map<string, AdminUser>>(new Map())
  const [routes, setRoutes] = useState<Map<string, RouteModel>>(new Map())
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [pageLoading, setPageLoading] = useState(false)
  const [recovering, setRecovering] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [recoveryError, setRecoveryError] = useState('')
  const [copyNotice, setCopyNotice] = useState('')

  async function load(cursor: string, push: boolean) {
    if (rows.length || pageLoading) setRefreshing(true)
    else setLoading(true)
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
    setSelected(null)
    setSelectedError(null)
    setDetailTab('overview')
    setCopyNotice('')
    setPeekUser(null)
    try {
      setSelected(await adminApi.getAdminVideoTask(id))
    } catch (caught) {
      setSelectedError(caught instanceof Error ? caught.message : '视频任务详情载入失败')
    }
  }

  function closeTask() {
    setSelectedID(null)
    setSelected(null)
    setSelectedError(null)
    setCopyNotice('')
  }

  async function retryArtifact(taskId: string, itemId: string) {
    setRecovering(itemId); setNotice(null); setRecoveryError('')
    try {
      const result = await adminApi.retryAdminVideoArtifact(taskId, itemId)
      if (result.provider_generation_requested === false) setNotice('已重新排队转存，未请求模型重新生成，也不会重复扣费。')
      await selectTask(taskId)
    } catch (caught) { setRecoveryError(caught instanceof Error ? caught.message : '重新转存失败') }
    finally { setRecovering(null) }
  }

  async function retryDerivative(jobId: string) {
    setRecovering(jobId); setNotice(null); setRecoveryError('')
    try {
      const result = await adminApi.retryMediaProcessingJob(jobId)
      if (result.provider_generation_requested === false) setNotice('已重新排队处理派生资源，未请求模型重新生成，也不会重复扣费。')
      if (selected) await selectTask(selected.id)
    } catch (caught) { setRecoveryError(caught instanceof Error ? caught.message : '重新处理失败') }
    finally { setRecovering(null) }
  }

  async function retrySettlement(taskId: string) {
    setRecovering(`settlement:${taskId}`); setNotice(null); setRecoveryError('')
    try {
      const result = await adminApi.retryAdminVideoSettlement(taskId)
      if (result.provider_generation_requested === false) setNotice('已重新排队结算或退回预留积分，未请求模型重新生成，也不会重复扣费。')
      await selectTask(taskId)
    } catch (caught) { setRecoveryError(caught instanceof Error ? caught.message : '重新结算失败') }
    finally { setRecovering(null) }
  }

  useEffect(() => { setCursorStack([]); void load('', false) }, [applied])

  useEffect(() => {
    let active = true
    void Promise.all([
      adminApi.listUsersPage('', 1, 200).catch(() => null),
      adminApi.listRouteModels({ page: 1, page_size: 500 }).catch(() => null),
    ]).then(([userPage, routeRows]) => {
      if (!active) return
      if (userPage) setUsers(new Map(userPage.items.map((user) => [String(user.id), user])))
      if (routeRows) {
        const next = new Map<string, RouteModel>()
        for (const route of routeRows) {
          next.set(String(route.id), route)
          next.set(route.code, route)
        }
        setRoutes(next)
      }
    })
    return () => { active = false }
  }, [])

  const submit = (event: FormEvent) => { event.preventDefault(); setApplied(filters) }
  const copy = async (value: string, label: string) => {
    try {
      await navigator.clipboard.writeText(value)
      setCopyNotice(`${label}已复制`)
    } catch {
      setCopyNotice('复制失败，请手动选择文本。')
    }
  }

  const metrics = useMemo<AdminMetric[]>(() => {
    const running = rows.filter((row) => row.status === 'running' || row.status === 'queued' || row.status === 'saving').length
    const attention = rows.filter((row) => row.status === 'failed' || row.status === 'partial').length
    const points = rows.reduce((sum, row) => sum + numeric(row.actual_points), 0)
    return [
      { label: '当前页任务', value: String(rows.length), trend: `${cursorStack.length + 1} 页 · 游标分页`, tone: 'neutral' },
      { label: '执行中', value: String(running), trend: '排队、执行与转存中', tone: running ? 'warn' : 'neutral' },
      { label: '需关注', value: String(attention), trend: '失败与部分成功', tone: attention ? 'bad' : 'good' },
      { label: '实际积分', value: points ? points.toFixed(5).replace(/\.?0+$/, '') : '0', trend: '当前页累计结算', tone: 'neutral' },
    ]
  }, [cursorStack.length, rows])

  const page = cursorStack.length + 1
  if (loading && !rows.length) return <LoadingBlock label="载入视频任务" />
  if (error && !rows.length) return <ErrorBlock message={error} onRetry={() => void load(cursorStack[cursorStack.length - 1] ?? '', false)} />

  return (
    <section className="a-page-stack">
      <PageHeader
        eyebrow="Video Tasks"
        title="视频任务"
        description="按用户、项目、路由与状态定位视频生成链路；恢复操作只处理转存、派生资源与结算。"
        secondaryActions={<RefreshIconButton label="手动刷新视频任务" refreshing={refreshing} onClick={() => void load(cursorStack[cursorStack.length - 1] ?? '', false)} />}
      />
      <MetricStrip metrics={metrics} />
      <form onSubmit={submit}>
        <FilterToolbar
          fields={[
            { key: 'user', label: '用户 ID', primary: true, control: <input className="a-input" value={filters.userId} onChange={(event) => setFilters({ ...filters, userId: event.target.value })} placeholder="用户 ID" inputMode="numeric" /> },
            { key: 'task', label: '平台任务 ID', primary: true, control: <input className="a-input" value={filters.taskId} onChange={(event) => setFilters({ ...filters, taskId: event.target.value })} placeholder="vtask_…" /> },
            { key: 'status', label: '状态', primary: true, control: <select className="a-select" value={filters.status} onChange={(event) => setFilters({ ...filters, status: event.target.value })}><option value="">全部状态</option><option value="queued">排队中</option><option value="running">执行中</option><option value="saving">转存中</option><option value="succeeded">成功</option><option value="failed">失败</option><option value="partial">部分成功</option><option value="cancelled">已取消</option></select> },
            { key: 'provider', label: '厂商任务 ID', control: <input className="a-input" value={filters.providerTaskId} onChange={(event) => setFilters({ ...filters, providerTaskId: event.target.value })} placeholder="厂商侧任务 ID" /> },
            { key: 'project', label: '项目 ID', control: <input className="a-input" value={filters.projectId} onChange={(event) => setFilters({ ...filters, projectId: event.target.value })} placeholder="project_…" /> },
            { key: 'route', label: '路由模型', control: <input className="a-input" value={filters.routeModelId} onChange={(event) => setFilters({ ...filters, routeModelId: event.target.value })} inputMode="numeric" placeholder="路由模型 ID" /> },
            { key: 'model', label: '真实模型', control: <input className="a-input" value={filters.accountModelId} onChange={(event) => setFilters({ ...filters, accountModelId: event.target.value })} inputMode="numeric" placeholder="账号模型 ID" /> },
          ]}
          actions={<><button className="a-btn a-btn--sm a-btn--primary" type="submit">查询</button><button className="a-btn a-btn--sm a-btn--quiet" type="button" onClick={() => { setFilters(emptyFilters); setApplied(emptyFilters) }}>重置</button></>}
          resultSummary={`第 ${page} 页 · 当前 ${rows.length} 条 · 游标向后续页`}
        />
      </form>
      {error ? <InlineFeedback tone="danger" message={error} /> : null}
      {notice ? <InlineFeedback tone="success" message={notice} /> : null}
      {refreshing ? <InlineFeedback tone="neutral" message="正在刷新视频任务，当前数据会保留到新结果返回。" /> : null}
      {!rows.length ? <EmptyBlock title="暂无视频任务" detail="提交视频生成后，任务与调用尝试会显示在这里。" /> : (
        <section className="a-panel a-panel--flush">
          <header className="a-panel__head">
            <div><h2>任务列表</h2><p>点击详情查看结果、费用与恢复操作。</p></div>
            <span className="a-hint">按创建时间倒序 · 每页 {PAGE_SIZE} 条</span>
          </header>
          <div className="a-table-wrap">
            <table className="a-table" style={{ minWidth: 1120 }}>
              <thead>
                <tr>
                  <th>任务 / 时间</th>
                  <th>用户</th>
                  <th>路由</th>
                  <th>状态</th>
                  <th>结算状态</th>
                  <th className="is-num">实际积分</th>
                  <th>创建时间</th>
                  <th className="is-actions">操作</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => {
                  const user = users.get(String(row.user_id))
                  const route = routes.get(String(row.route_model_id)) ?? routes.get(row.route_model_code)
                  return (
                    <tr key={row.id} className={selectedID === row.id ? 'is-selected' : undefined} onDoubleClick={() => void selectTask(row.id)}>
                      <td>
                        <div className="lr-cell">
                          <strong style={{ fontFamily: 'var(--font-admin-mono)', fontSize: 12 }}>{row.id}</strong>
                          <span>{formatAdminTaskTime(row.created_at)}</span>
                        </div>
                      </td>
                      <td>
                        {user ? (
                          <button className="a-cell-entity" type="button" onClick={() => setPeekUser(user)}>
                            <span className="a-cell-entity__avatar">{avatarText(user)}</span>
                            <span className="a-cell-entity__copy">
                              <span className="a-cell-entity__name">{user.display_name || user.nickname || user.email}</span>
                              <span className="a-cell-entity__meta">{user.email}</span>
                            </span>
                          </button>
                        ) : <span className="lr-cell lr-entity-fallback"><strong style={{ fontFamily: 'var(--font-admin-mono)', fontSize: 12 }}>用户 #{row.user_id}</strong><span>未映射用户</span></span>}
                      </td>
                      <td>
                        <div className="lr-cell">
                          <strong className={route ? undefined : 'lr-entity-fallback'}>{route?.name || row.route_model_code || '未映射路由'}</strong>
                          <span>{row.route_model_code || '—'} · 路由 #{row.route_model_id}</span>
                        </div>
                      </td>
                      <td><Badge tone={videoTaskStatusTone(row.status)}>{videoTaskStatusLabel(row.status)}</Badge></td>
                      <td><Badge tone={settlementStatusTone(row.settlement_status)}>{settlementStatusLabel(row.settlement_status)}</Badge></td>
                      <td className="is-num">{row.actual_points || '0.00000'}</td>
                      <td className="is-muted">{formatAdminTaskTime(row.created_at)}</td>
                      <td className="is-actions">
                        <div className="a-row-actions">
                          <button className="a-row-btn a-row-btn--primary" type="button" onClick={() => void selectTask(row.id)}>详情</button>
                          <button className="a-icon-btn" type="button" style={{ width: 32, height: 32 }} aria-label="复制任务 ID" title="复制任务 ID" onClick={() => void copy(row.id, '任务 ID')}>⧉</button>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
          <footer className="a-pager">
            <div className="a-pager__info">第 <b>{page}</b> 页 · 本页 <b>{rows.length}</b> 条 · 游标分页</div>
            <div className="a-pager__controls">
              <button className="a-pager__btn" type="button" disabled={pageLoading || !cursorStack.length} onClick={() => { const previous = cursorStack[cursorStack.length - 2] ?? ''; setCursorStack((current) => current.slice(0, -1)); setPageLoading(true); void load(previous, false) }}>‹ 上一页</button>
              <button className="a-pager__btn" type="button" disabled={pageLoading || !nextCursor} onClick={() => { setPageLoading(true); void load(nextCursor, true) }}>下一页 ›</button>
            </div>
            <div className="a-pager__size"><span>每页固定 {PAGE_SIZE} 条</span></div>
          </footer>
        </section>
      )}

      {selectedID ? (
        <Drawer
          wide
          title="视频任务详情"
          description={`${selectedID}${selected ? ` · ${selected.route_model_code}` : ''}`}
          onClose={() => { if (!peekUser) closeTask() }}
          headerAside={<>{selected ? <Badge tone={videoTaskStatusTone(selected.status)}>{videoTaskStatusLabel(selected.status)}</Badge> : null}{selected ? <Badge tone={settlementStatusTone(selected.settlement_status)}>{settlementStatusLabel(selected.settlement_status)}</Badge> : null}</>}
          footer={(
            <>
              <button className="a-btn a-btn--sm" type="button" onClick={() => void copy(selectedID, '任务 ID')}>复制任务 ID</button>
              {selected && selected.settlement_status !== 'finalized' && TERMINAL_STATUSES.includes(selected.status) ? <button className="a-btn a-btn--sm" type="button" disabled={recovering === `settlement:${selected.id}`} onClick={() => void retrySettlement(selected.id)}>重新结算</button> : null}
              {selected ? <button className="a-btn a-btn--sm" type="button" disabled={!selected.items.some((item) => typeof item.artifact_snapshot?.processing_job_id === 'string')} onClick={() => { const item = selected.items.find((entry) => typeof entry.artifact_snapshot?.processing_job_id === 'string'); if (item) void retryDerivative(String(item.artifact_snapshot.processing_job_id)) }}>重新处理</button> : null}
              <div className="lr-spacer" />
              <button className="a-btn a-btn--sm a-btn--primary" type="button" onClick={closeTask}>关闭</button>
            </>
          )}
        >
          {copyNotice ? <InlineFeedback tone="success" message={copyNotice} /> : null}
          {notice ? <InlineFeedback tone="success" message={notice} /> : null}
          {recoveryError ? <InlineFeedback tone="danger" message={recoveryError} /> : null}
          {selectedError ? <ErrorBlock message={selectedError} onRetry={() => void selectTask(selectedID)} /> : null}
          {!selected && !selectedError ? <LoadingBlock label="载入任务详情" /> : null}
          {selected ? (
            <>
              <AdminTabs
                ariaLabel="视频任务详情分区"
                className="lr-drawer-tabs"
                value={detailTab}
                onChange={setDetailTab}
                items={[
                  { id: 'overview', label: '概览' },
                  { id: 'results', label: '结果与费用', badge: selected.items.length || undefined },
                  { id: 'pricing', label: '路由与定价快照' },
                ]}
              />
              {detailTab === 'overview' ? <VideoOverview detail={selected} route={routes.get(String(selected.route_model_id)) ?? routes.get(selected.route_model_code)} user={users.get(String(selected.user_id))} onPeekUser={setPeekUser} /> : null}
              {detailTab === 'results' ? <VideoResults detail={selected} recovering={recovering} onRetryArtifact={retryArtifact} onRetryDerivative={retryDerivative} onCopy={copy} /> : null}
              {detailTab === 'pricing' ? <VideoPricing detail={selected} onCopy={copy} /> : null}
            </>
          ) : null}
        </Drawer>
      ) : null}

      {peekUser ? (
        <Modal
          title="用户摘要"
          detail={peekUser.email}
          onClose={() => setPeekUser(null)}
          footer={<><button className="a-btn a-btn--sm a-btn--quiet" type="button" onClick={() => setPeekUser(null)}>关闭</button><button className="a-btn a-btn--sm a-btn--primary" type="button" onClick={() => { window.location.hash = '/users' }}>打开用户管理</button></>}
        >
          <dl className="a-kv">
            <div><dt>用户</dt><dd>{peekUser.display_name || peekUser.nickname || peekUser.email}</dd></div>
            <div><dt>邮箱</dt><dd>{peekUser.email}</dd></div>
            <div><dt>积分余额</dt><dd><span className="lr-num">{peekUser.balance}</span></dd></div>
            <div><dt>所属分组</dt><dd>{peekUser.group || '未分组'}</dd></div>
            <div><dt>最近活跃</dt><dd>{peekUser.last_seen_at?.slice(0, 16).replace('T', ' ') || '—'}</dd></div>
          </dl>
        </Modal>
      ) : null}
    </section>
  )
}

function VideoOverview({ detail, route, user, onPeekUser }: { detail: AdminVideoTaskDetail; route?: RouteModel; user?: AdminUser; onPeekUser: (user: AdminUser) => void }) {
  return (
    <div className="lr-stack">
      <div className="vt-summary">
        <div><span>预计积分</span><b>{detail.estimated_points}</b></div>
        <div><span>已预留</span><b>{detail.reserved_points}</b></div>
        <div><span>实际积分</span><b>{detail.actual_points === '0.00000' ? '待完成' : detail.actual_points}</b></div>
        <div><span>结算状态</span><b style={{ fontSize: 13, color: 'var(--a-ink)' }}>{settlementStatusLabel(detail.settlement_status)}</b></div>
      </div>
      <div className="lr-detail-grid lr-detail-grid--2">
        <section className="a-panel a-panel--pad lr-panel-flat">
          <div className="a-section-title" style={{ margin: 0 }}><h2>视频结果</h2><span className="a-hint">{detail.items.length} 个结果</span></div>
          <div className="lr-video-result__media" style={{ marginTop: 10 }}>{detail.items.length ? `共 ${detail.items.length} 个视频结果，费用与转存状态见「结果与费用」` : '暂无视频产物'}</div>
          <dl className="a-kv" style={{ marginTop: 10 }}>
            <div><dt>状态</dt><dd><Badge tone={videoTaskStatusTone(detail.status)}>{videoTaskStatusLabel(detail.status)}</Badge></dd></div>
            <div><dt>结算</dt><dd><Badge tone={settlementStatusTone(detail.settlement_status)}>{settlementStatusLabel(detail.settlement_status)}</Badge></dd></div>
          </dl>
        </section>
        <section className="a-panel a-panel--pad lr-panel-flat">
          <div className="a-section-title" style={{ margin: 0 }}><h2>任务归属</h2><span className="a-hint">任务 #{detail.id}</span></div>
          <dl className="a-kv" style={{ marginTop: 8 }}>
            <div><dt>用户</dt><dd>{user ? (
              <button className="a-cell-entity" type="button" onClick={() => onPeekUser(user)}>
                <span className="a-cell-entity__avatar" style={{ width: 24, height: 24, borderRadius: 6, fontSize: 9 }}>{avatarText(user)}</span>
                <span className="a-cell-entity__copy">
                  <span className="a-cell-entity__name" style={{ fontSize: 12.5 }}>{user.display_name || user.nickname || user.email}</span>
                  <span className="a-cell-entity__meta">{user.email}</span>
                </span>
              </button>
            ) : <span className="lr-entity-fallback">用户 #{detail.user_id}</span>}</dd></div>
            <div><dt>路由模型</dt><dd>{route?.name || detail.route_model_code} <span className="a-tag">{detail.route_model_code}</span></dd></div>
            <div><dt>项目</dt><dd><span className="a-mono">{detail.project_id || '—'}</span></dd></div>
            <div><dt>创建时间</dt><dd>{formatAdminTaskTime(detail.created_at)}</dd></div>
            <div><dt>更新时间</dt><dd>{formatAdminTaskTime(detail.updated_at)}</dd></div>
          </dl>
        </section>
      </div>
      <div className="a-banner a-banner--info"><span>i</span><div><strong style={{ fontSize: 13 }}>恢复操作只处理转存与派生资源</strong><div className="a-hint">重新转存、重新处理、重新结算都不会请求模型重新生成，也不会重复扣费。</div></div></div>
    </div>
  )
}

function VideoResults({ detail, recovering, onRetryArtifact, onRetryDerivative, onCopy }: {
  detail: AdminVideoTaskDetail
  recovering: string | null
  onRetryArtifact: (taskId: string, itemId: string) => void
  onRetryDerivative: (jobId: string) => void
  onCopy: (value: string, label: string) => void
}) {
  if (!detail.items.length) return <EmptyBlock variant="inline" title="暂无结果" detail="任务尚未生成结果，或结果已被清理。" />
  return (
    <div className="lr-stack--sm">
      {detail.items.map((item) => {
        const failed = item.status === 'failed' || Boolean(item.error_code)
        const attempts = Array.isArray(item.attempts) ? item.attempts : []
        const jobID = typeof item.artifact_snapshot?.processing_job_id === 'string' ? String(item.artifact_snapshot.processing_job_id) : ''
        return (
          <article key={item.id} className={cn('lr-video-result', failed && 'lr-video-result--failed')}>
            <div className="lr-video-result__media">{item.result_asset_id ? `资产 ${item.result_asset_id}` : '暂无转存产物'}</div>
            <div className="lr-video-result__meta">
              <div className="lr-row" style={{ justifyContent: 'space-between' }}>
                <strong>结果 {item.ordinal + 1} · {item.stage}</strong>
                <Badge tone={item.status === 'succeeded' ? 'success' : item.status === 'failed' ? 'danger' : 'warning'}>{videoItemStatusLabel(item.status)}</Badge>
              </div>
              <div className="lr-video-result__row">
                <span className="a-badge">结果费用 {item.actual_points}</span>
                <span className="a-badge">上游成本 {item.provider_cost}</span>
                {item.result_asset_id ? <span className="a-badge">产物 {item.result_asset_id}</span> : null}
              </div>
              <dl className="a-kv">
                <div><dt>结算积分</dt><dd>{item.actual_points} · {detail.settlement_status === 'finalized' ? '已结算' : '待结算'}</dd></div>
                <div><dt>尝试次数</dt><dd>{attempts.length}</dd></div>
                <div><dt>错误</dt><dd className={item.error_code ? 'lr-neg' : undefined}>{item.error_code || item.error_message || '—'}</dd></div>
              </dl>
              {attempts.length ? (
                <div className="lr-stack--sm">
                  {attempts.map((attempt) => (
                    <div key={attempt.id} className="a-panel a-panel--pad lr-panel-flat" style={{ padding: 12 }}>
                      <div className="lr-row" style={{ justifyContent: 'space-between' }}>
                        <strong style={{ fontSize: 12.5 }}>尝试 {attempt.attempt_no} · {attempt.provider_code} / {attempt.model_code}</strong>
                        <span className={attempt.status === 'failed' ? 'lr-neg' : 'lr-muted'}>{attempt.status} · provider_cost {attempt.provider_cost}</span>
                      </div>
                      <pre className="lr-json" style={{ marginTop: 8 }}>usage_normalized {JSON.stringify(attempt.usage_normalized, null, 2)}{`\n`}cost_snapshot {JSON.stringify(attempt.cost_snapshot, null, 2)}</pre>
                    </div>
                  ))}
                </div>
              ) : <p className="a-hint">暂无调用尝试（任务尚未提交到上游或已被取消）。</p>}
              <div className="a-row-actions" style={{ justifyContent: 'flex-start' }}>
                <button className="a-row-btn a-row-btn--primary" type="button" disabled={recovering === item.id} onClick={() => onRetryArtifact(detail.id, item.id)}>重新转存</button>
                {jobID ? <button className="a-row-btn" type="button" disabled={recovering === jobID} onClick={() => onRetryDerivative(jobID)}>重新处理</button> : null}
                {item.result_asset_id ? <button className="a-row-btn" type="button" onClick={() => void onCopy(String(item.result_asset_id), '产物 ID')}>复制产物 ID</button> : null}
              </div>
            </div>
          </article>
        )
      })}
    </div>
  )
}

function VideoPricing({ detail, onCopy }: { detail: AdminVideoTaskDetail; onCopy: (value: string, label: string) => void }) {
  const pricing = stableJson(detail.pricing_snapshot ?? {})
  const routing = stableJson(detail.routing_snapshot ?? {})
  return (
    <div className="lr-stack--sm">
      <div className="a-section-title" style={{ margin: 0 }}><h2>定价快照</h2><button className="a-row-btn" type="button" onClick={() => void onCopy(pricing, '定价快照')}>复制 JSON</button></div>
      <pre className="lr-snapshot">{pricing}</pre>
      <div className="a-section-title" style={{ margin: 0 }}><h2>路由快照</h2><button className="a-row-btn" type="button" onClick={() => void onCopy(routing, '路由快照')}>复制 JSON</button></div>
      <pre className="lr-snapshot">{routing}</pre>
    </div>
  )
}

function videoTaskStatusLabel(status: string) {
  if (status === 'queued') return '排队中'
  if (status === 'running') return '执行中'
  if (status === 'saving') return '转存中'
  if (status === 'succeeded') return '成功'
  if (status === 'partial') return '部分成功'
  if (status === 'failed') return '失败'
  if (status === 'cancelled' || status === 'canceled') return '已取消'
  return status || '未知状态'
}

function videoTaskStatusTone(status: string): 'success' | 'warning' | 'danger' | 'neutral' | 'primary' {
  if (status === 'succeeded') return 'success'
  if (status === 'failed') return 'danger'
  if (status === 'partial') return 'warning'
  if (status === 'running' || status === 'saving') return 'primary'
  if (status === 'queued') return 'warning'
  return 'neutral'
}

function videoItemStatusLabel(status: string) {
  if (status === 'succeeded') return '成功'
  if (status === 'failed') return '失败'
  if (status === 'processing' || status === 'saving') return '转存中'
  if (status === 'queued') return '排队中'
  if (status === 'cancelled' || status === 'canceled') return '已取消'
  return status || '未知状态'
}

function settlementStatusLabel(status: string) {
  if (status === 'reserved') return '已预留'
  if (status === 'settled' || status === 'finalized') return '已结算'
  if (status === 'refund_pending') return '待退款'
  if (status === 'refunded') return '已退回'
  if (status === 'failed') return '结算失败'
  return status || '未知状态'
}

function settlementStatusTone(status: string): 'success' | 'warning' | 'danger' | 'neutral' | 'primary' {
  if (status === 'settled' || status === 'finalized' || status === 'refunded') return 'success'
  if (status === 'refund_pending') return 'warning'
  if (status === 'failed') return 'danger'
  if (status === 'reserved') return 'primary'
  return 'neutral'
}

function numeric(value?: string | null) {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : 0
}

function stableJson(value: Record<string, unknown>) {
  try {
    return JSON.stringify(value ?? {}, null, 2)
  } catch {
    return '{}'
  }
}

function formatAdminTaskTime(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(date)
}

function avatarText(user: AdminUser) {
  return (user.display_name || user.nickname || user.email || '?').trim().slice(0, 1).toUpperCase()
}