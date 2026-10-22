import { useEffect, useMemo, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import type { AdminMetric, AdminUser, CallRecord, CallRecordAttempt, RouteModel } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { adminApi } from '../../../shared/admin-api'
import { AdminTabs, Badge, Drawer, EmptyBlock, ErrorBlock, InlineFeedback, LoadingBlock, MetricStrip, Modal, PageHeader, RefreshIconButton } from '../components'
import { FilterToolbar, ListPage, Pager } from '../ui/dataTable'
import { callRecordCommonErrorCodes, callRecordFilterCopy, callRecordRepair, callRecordRows, callRecordSourceChannelOptions, callRecordStatusOptions } from './callRecordRows'
import './list-redesign.css'

type CallRecordFilters = {
  status: string
  errorCode: string
  sourceChannel: string
  provider: string
  userId: string
  taskId: string
}

type DetailTab = 'overview' | 'attempts' | 'billing' | 'raw'

const initialFilters: CallRecordFilters = {
  status: '',
  errorCode: '',
  sourceChannel: '',
  provider: '',
  userId: '',
  taskId: '',
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
  const [selected, setSelected] = useState<CallRecord | null>(null)
  const [detailTab, setDetailTab] = useState<DetailTab>('overview')
  const [peekUser, setPeekUser] = useState<AdminUser | null>(null)
  const [copyNotice, setCopyNotice] = useState('')
  const [directoryUsers, setDirectoryUsers] = useState<Map<string, AdminUser>>(new Map())
  const [directoryRoutes, setDirectoryRoutes] = useState<Map<string, RouteModel>>(new Map())
  const requestGenerationRef = useRef(0)
  const [reloadKey, setReloadKey] = useState(0)
  const viewRows = useMemo(() => callRecordRows(rows), [rows])
  const stats = useMemo(() => callRecordStats(rows), [rows])
  const distributions = useMemo(() => callRecordDistributions(rows, directoryRoutes), [directoryRoutes, rows])
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

  useEffect(() => {
    let active = true
    void Promise.all([
      adminApi.listUsersPage('', 1, 200).catch(() => null),
      adminApi.listRouteModels({ page: 1, page_size: 500 }).catch(() => null),
    ]).then(([userPage, routeRows]) => {
      if (!active) return
      if (userPage) setDirectoryUsers(new Map(userPage.items.map((user) => [String(user.id), user])))
      if (routeRows) setDirectoryRoutes(new Map(routeRows.map((route) => [route.code, route])))
    })
    return () => { active = false }
  }, [])

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

  const openDetail = (record: CallRecord) => {
    setPeekUser(null)
    setCopyNotice('')
    setDetailTab('overview')
    setSelected(record)
  }

  const copy = async (value: string, label: string) => {
    try {
      await navigator.clipboard.writeText(value)
      setCopyNotice(`${label}已复制`)
    } catch {
      setCopyNotice('复制失败，请手动选择文本。')
    }
  }

  if (loading && !rows.length) return <LoadingBlock label="载入图片任务" />
  if (error && !rows.length) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className="a-page-stack">
      <PageHeader
        eyebrow="Image Tasks"
        title="图片任务"
        description="按用户、路由、模型与错误码定位任务。用户列展示名称与邮箱，点击可查看账户摘要；调用链与计费快照在抽屉里展开。"
        secondaryActions={<RefreshIconButton label="刷新图片任务" refreshing={refreshing} onClick={() => void load()} />}
      />
      <MetricStrip metrics={metrics} />
      <form onSubmit={submitFilters}>
        <FilterToolbar
          fields={[
            { key: 'userId', label: callRecordFilterCopy.userId.label, primary: true, control: <input className="a-input" value={filters.userId} onChange={(event) => setFilters((value) => ({ ...value, userId: event.target.value }))} placeholder={callRecordFilterCopy.userId.placeholder} inputMode="numeric" /> },
            { key: 'status', label: '任务状态', primary: true, control: <select className="a-select" value={filters.status} onChange={(event) => setFilters((value) => ({ ...value, status: event.target.value }))}>{callRecordStatusOptions.map((option) => <option key={option.value || 'all'} value={option.value}>{option.label}</option>)}</select> },
            { key: 'sourceChannel', label: '调用入口', primary: true, control: <select className="a-select" value={filters.sourceChannel} onChange={(event) => setFilters((value) => ({ ...value, sourceChannel: event.target.value }))}>{callRecordSourceChannelOptions.map((option) => <option key={option.value || 'all'} value={option.value}>{option.label}</option>)}</select> },
            { key: 'provider', label: callRecordFilterCopy.provider.label, control: <input className="a-input" value={filters.provider} onChange={(event) => setFilters((value) => ({ ...value, provider: event.target.value }))} placeholder={callRecordFilterCopy.provider.placeholder} /> },
            { key: 'taskId', label: callRecordFilterCopy.taskId.label, control: <input className="a-input" value={filters.taskId} onChange={(event) => setFilters((value) => ({ ...value, taskId: event.target.value }))} placeholder={callRecordFilterCopy.taskId.placeholder} /> },
            { key: 'errorCode', label: callRecordFilterCopy.errorCode.label, control: <input className="a-input" value={filters.errorCode} onChange={(event) => setFilters((value) => ({ ...value, errorCode: event.target.value }))} list="call-record-error-codes" placeholder={callRecordFilterCopy.errorCode.placeholder} /> },
          ]}
          actions={(
            <>
              <datalist id="call-record-error-codes">
                {callRecordCommonErrorCodes.filter(Boolean).map((code) => <option key={code} value={code} />)}
              </datalist>
              <button className="a-btn a-btn--sm a-btn--primary" type="submit">查询</button>
              <button className="a-btn a-btn--sm a-btn--quiet" type="button" onClick={resetFilters}>重置</button>
            </>
          )}
          resultSummary={`共 ${total} 条调用 · 当前显示 ${rows.length} 条`}
        />
      </form>
      {refreshing ? <InlineFeedback tone="neutral" message="正在刷新图片任务，当前数据会保留到新结果返回。" /> : null}
      {error && rows.length ? <InlineFeedback tone="danger" message={error} /> : null}
      {!rows.length ? <EmptyBlock title="暂无图片任务" detail="生成任务执行后会出现在这里。" /> : (
        <ListPage
          pagination={<Pager page={page} pageSize={pageSize} total={total} onChange={setPage} onPageSizeChange={(size) => { setPageSize(size); setPage(1) }} />}
        >
          <div className="a-table-wrap">
            <table className="a-table" style={{ minWidth: 1180 }}>
              <thead>
                <tr>
                  <th>任务 / 时间</th>
                  <th>用户</th>
                  <th>路由与模型</th>
                  <th>配置 / 结果</th>
                  <th className="is-num">积分</th>
                  <th className="is-num">耗时</th>
                  <th>状态</th>
                  <th className="is-actions">操作</th>
                </tr>
              </thead>
              <tbody>
                {viewRows.map((row, index) => {
                  const record = rows[index]
                  const user = directoryUsers.get(String(record.user_id))
                  const route = record.route_model_code ? directoryRoutes.get(record.route_model_code) : undefined
                  return (
                    <tr key={row.id} className={selected?.task_id === record.task_id ? 'is-selected' : undefined} onDoubleClick={() => openDetail(record)}>
                      <td>
                        <div className="lr-cell">
                          <strong style={{ fontFamily: 'var(--font-admin-mono)', fontSize: 12 }}>{row.fullTaskId}</strong>
                          <span>{row.createdAt}</span>
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
                        ) : (
                          <span className="lr-cell lr-entity-fallback">
                            <strong style={{ fontFamily: 'var(--font-admin-mono)', fontSize: 12 }}>用户 #{record.user_id}</strong>
                            <span>未映射用户</span>
                          </span>
                        )}
                      </td>
                      <td>
                        <div className="lr-route">
                          <strong className={route ? undefined : 'lr-entity-fallback'}>{route?.name || record.route_model_code || row.routeLabel}</strong>
                          <span>{record.route_model_code || '—'} · {record.upstream_model_code || record.abstract_model || '—'}</span>
                          <div className="lr-chain">
                            <span className="a-tag">{channelLabel(record.source_channel)}</span>
                            <i>→</i>
                            <span className="a-tag">{record.provider || '未记录账号'}{record.model_account_id ? ` #${record.model_account_id}` : ''}</span>
                          </div>
                        </div>
                      </td>
                      <td>
                        <div className="lr-cell">
                          <strong style={{ fontWeight: 500 }}>{row.taskDetail}</strong>
                          <span>{record.base_resolution || '—'} · {record.quality || '—'} · {record.requested_output_image_count} 张</span>
                        </div>
                      </td>
                      <td className="is-num">
                        <span className="lr-cell" style={{ justifyItems: 'end' }}>
                          <strong className="lr-num">{emptyDash(record.actual_points)}</strong>
                          <span>估 {emptyDash(record.estimated_points)}</span>
                        </span>
                      </td>
                      <td className="is-num">{latencyLabel(record)}</td>
                      <td>
                        <div className="lr-stack--sm">
                          <Badge tone={row.statusTone}>{row.statusLabel}</Badge>
                          {record.error_code ? <span className="lr-muted" style={{ fontFamily: 'var(--font-admin-mono)', fontSize: 10.5 }}>{record.error_code}</span> : null}
                        </div>
                      </td>
                      <td className="is-actions">
                        <div className="a-row-actions">
                          <button className="a-row-btn a-row-btn--primary" type="button" onClick={() => openDetail(record)}>详情</button>
                          <button className="a-icon-btn" type="button" style={{ width: 32, height: 32 }} aria-label="复制任务 ID" title="复制任务 ID" onClick={() => void copy(record.task_id, '任务 ID')}>⧉</button>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </ListPage>
      )}

      <div className="a-grid a-grid--3">
        <DistributionCard title="路由模型调用量" rows={distributions.routes} />
        <DistributionCard title="底层账号调用量" rows={distributions.providers} />
        <DistributionCard title="底层模型调用量" rows={distributions.models} />
      </div>

      {selected ? (
        <CallRecordDrawer
          record={selected}
          user={directoryUsers.get(String(selected.user_id))}
          route={selected.route_model_code ? directoryRoutes.get(selected.route_model_code) : undefined}
          tab={detailTab}
          copyNotice={copyNotice}
          onTab={setDetailTab}
          onClose={() => { if (!peekUser) { setSelected(null); setCopyNotice('') } }}
          onPeekUser={setPeekUser}
          onCopy={copy}
        />
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
            <div><dt>状态</dt><dd><Badge tone={peekUser.status === 'disabled' ? 'danger' : peekUser.status === 'pending' ? 'warning' : 'success'}>{userStatusLabel(peekUser.status)}</Badge></dd></div>
            <div><dt>积分余额</dt><dd><span className="lr-num">{peekUser.balance}</span></dd></div>
            <div><dt>所属分组</dt><dd>{peekUser.group || '未分组'}</dd></div>
            <div><dt>最近活跃</dt><dd>{peekUser.last_seen_at?.slice(0, 16).replace('T', ' ') || '—'}</dd></div>
          </dl>
        </Modal>
      ) : null}
    </section>
  )
}

function CallRecordDrawer({
  record,
  user,
  route,
  tab,
  copyNotice,
  onTab,
  onClose,
  onPeekUser,
  onCopy,
}: {
  record: CallRecord
  user?: AdminUser
  route?: RouteModel
  tab: DetailTab
  copyNotice: string
  onTab: (tab: DetailTab) => void
  onClose: () => void
  onPeekUser: (user: AdminUser) => void
  onCopy: (value: string, label: string) => void
}) {
  const row = callRecordRows([record])[0]
  const repair = callRecordRepair(record.error_code)
  return (
    <Drawer
      wide
      title="图片任务详情"
      description={`${record.task_id} · ${row.createdAt} · ${channelLabel(record.source_channel)}`}
      headerAside={<Badge tone={row.statusTone}>{row.statusLabel}</Badge>}
      onClose={onClose}
      footer={(
        <>
          <button className="a-btn a-btn--sm" type="button" onClick={() => void onCopy(record.task_id, '任务 ID')}>复制任务 ID</button>
          {repair ? <a className="a-btn a-btn--sm" href={repair.href}>{repair.label}</a> : null}
          <div className="lr-spacer" />
          <button className="a-btn a-btn--sm a-btn--primary" type="button" onClick={onClose}>关闭</button>
        </>
      )}
    >
      {copyNotice ? <InlineFeedback tone="success" message={copyNotice} /> : null}
      <AdminTabs
        ariaLabel="图片任务详情分区"
        className="lr-drawer-tabs"
        value={tab}
        onChange={onTab}
        items={[
          { id: 'overview', label: '概览' },
          { id: 'attempts', label: '调用尝试', badge: record.attempts?.length || undefined },
          { id: 'billing', label: '计费快照' },
          { id: 'raw', label: '原始数据' },
        ]}
      />

      {tab === 'overview' ? (
        <div className="lr-stack">
          <div className="lr-detail-grid lr-detail-grid--2">
            <section className="a-panel a-panel--pad lr-panel-flat">
              <div className="a-section-title" style={{ margin: 0 }}><h2>任务归属</h2><span className="a-hint">{record.task_type}</span></div>
              <dl className="a-kv" style={{ marginTop: 8 }}>
                <div><dt>用户</dt><dd>{user ? (
                  <button className="a-cell-entity" type="button" onClick={() => onPeekUser(user)}>
                    <span className="a-cell-entity__avatar" style={{ width: 24, height: 24, borderRadius: 6, fontSize: 9 }}>{avatarText(user)}</span>
                    <span className="a-cell-entity__copy">
                      <span className="a-cell-entity__name" style={{ fontSize: 12.5 }}>{user.display_name || user.nickname || user.email}</span>
                      <span className="a-cell-entity__meta">{user.email}</span>
                    </span>
                  </button>
                ) : <span className="lr-entity-fallback">用户 #{record.user_id}</span>}</dd></div>
                <div><dt>入口</dt><dd>{channelLabel(record.source_channel)}{record.api_key_id ? ` · API Key #${record.api_key_id}` : ''}</dd></div>
                <div><dt>路由模型</dt><dd>{route?.name || record.route_model_code || row.routeLabel} {record.route_model_code ? <span className="a-tag">{record.route_model_code}</span> : null}</dd></div>
                <div><dt>真实模型</dt><dd>{record.upstream_model_code || record.abstract_model || '—'} {record.provider ? <span className="a-tag">{record.provider}</span> : null}</dd></div>
                <div><dt>任务类型</dt><dd>{row.taskDetail.split(' · ')[0]}</dd></div>
                <div><dt>尺寸</dt><dd>{record.base_resolution || '—'} · {record.quality || '—'} · {record.requested_output_image_count} 张请求 / {record.success_output_image_count} 张成功</dd></div>
                <div><dt>耗时</dt><dd>{latencyLabel(record)} · {record.attempt_count || record.attempts?.length || 0} 次尝试</dd></div>
                <div><dt>失败阶段</dt><dd>{record.failure_phase || (record.error_code ? '调用失败' : '—')}</dd></div>
              </dl>
            </section>
            <div className="lr-stack--sm">
              <section className="a-drawer__section">
                <h3>提示词与参数</h3>
                <div className="lr-prompt">调用记录不保存提示词原文。请结合任务类型、参数、错误码与「原始数据」中的请求快照排查。</div>
                <dl className="a-kv">
                  <div><dt>质量</dt><dd>{record.quality || '—'}</dd></div>
                  <div><dt>基础分辨率</dt><dd>{record.base_resolution || '—'}</dd></div>
                  <div><dt>参考图</dt><dd>{record.reference_image_count} 张</dd></div>
                  <div><dt>输出数量</dt><dd>{record.success_output_image_count} / {record.requested_output_image_count}</dd></div>
                  <div><dt>任务状态</dt><dd><Badge tone={row.statusTone}>{row.statusLabel}</Badge></dd></div>
                </dl>
              </section>
              {record.error_code ? (
                <div className="a-banner a-banner--warn">
                  <span>!</span>
                  <div><strong style={{ fontSize: 13 }}>{record.error_code}</strong><div className="a-hint">{row.failureDetail}</div></div>
                </div>
              ) : (
                <div className="a-banner a-banner--info"><span>i</span><div><strong style={{ fontSize: 13 }}>任务已成功</strong><div className="a-hint">成功输出 {record.success_output_image_count} 张，计费 {emptyDash(record.actual_points)} 积分。</div></div></div>
              )}
            </div>
          </div>
        </div>
      ) : null}

      {tab === 'attempts' ? (
        <div className="lr-stack--sm">
          {!record.attempts?.length ? <EmptyBlock variant="inline" title="暂无调用尝试" detail="该任务尚未提交到上游或没有保存尝试明细。" /> : record.attempts.map((attempt, index) => (
            <AttemptCard key={`${attempt.provider || 'provider'}-${index}`} attempt={attempt} index={index} last={index === (record.attempts?.length ?? 0) - 1} />
          ))}
        </div>
      ) : null}

      {tab === 'billing' ? (
        <div className="lr-stack">
          <div className="a-metrics lr-metrics-3">
            <div className="a-metric"><div className="a-metric__label">预估积分</div><div className="a-metric__value" style={{ fontSize: 22 }}>{emptyDash(record.estimated_points)}</div></div>
            <div className="a-metric"><div className="a-metric__label">实际扣除</div><div className="a-metric__value" style={{ fontSize: 22 }}>{emptyDash(record.actual_points)}</div></div>
            <div className="a-metric"><div className="a-metric__label">上游成本</div><div className="a-metric__value" style={{ fontSize: 22 }}>¥ {emptyDash(record.provider_cost)}</div></div>
          </div>
          <section className="a-panel a-panel--pad lr-panel-flat">
            <div className="a-section-title" style={{ margin: 0 }}><h2>计费快照</h2><span className="a-hint">历史价格上下文</span></div>
            {record.pricing_snapshot && Object.keys(record.pricing_snapshot).length ? (
              <pre className="lr-json" style={{ marginTop: 10 }}>{stableJson(record.pricing_snapshot)}</pre>
            ) : <p className="a-hint" style={{ marginTop: 10 }}>该历史记录未保存计费快照。</p>}
            <dl className="a-kv" style={{ marginTop: 10 }}>
              <div><dt>毛利</dt><dd>{emptyDash(record.gross_margin)}</dd></div>
              <div><dt>平台亏损</dt><dd>{record.platform_loss ? <Badge tone="danger">是</Badge> : '否'}</dd></div>
            </dl>
          </section>
          <div className="a-banner a-banner--info"><span>i</span><div>计费按任务最终成功输出数量结算，失败尝试不重复扣费。</div></div>
        </div>
      ) : null}

      {tab === 'raw' ? (
        <div className="lr-stack--sm">
          <div className="a-section-title" style={{ margin: 0 }}><h2>请求快照</h2><button className="a-row-btn" type="button" onClick={() => void onCopy(requestSnapshot(record), '请求快照')}>复制 JSON</button></div>
          <pre className="lr-snapshot">{requestSnapshot(record)}</pre>
        </div>
      ) : null}
    </Drawer>
  )
}

function AttemptCard({ attempt, index, last }: { attempt: CallRecordAttempt; index: number; last: boolean }) {
  const routeParts = [
    attempt.adapter_type,
    attempt.account_model_id ? `账号模型 #${attempt.account_model_id}` : '',
    attempt.model_account_id ? `模型账号 #${attempt.model_account_id}` : '',
    attempt.model_code,
  ].filter(Boolean)
  const errorSummary = [attempt.error_code, attempt.error_message || attempt.error].filter(Boolean).join(' · ')
  return (
    <article className={cn('lr-attempt', last && !errorSummary && 'lr-attempt--active')}>
      <div className="lr-attempt__head">
        <strong>尝试 {index + 1} · {attempt.provider || '未知 Provider'} / {attempt.model_code || '未知模型'}</strong>
        <Badge tone={errorSummary ? 'danger' : 'success'}>{errorSummary ? (attempt.error_code || '失败') : '成功'}</Badge>
      </div>
      <dl className="a-kv">
        <div><dt>开始 / 结束</dt><dd>{formatAttemptTime(attempt.started_at)} → {formatAttemptTime(attempt.finished_at)}</dd></div>
        <div><dt>适配器</dt><dd>{attempt.adapter_type || '—'}</dd></div>
        <div><dt>账号模型</dt><dd>{routeParts.length ? routeParts.join(' · ') : '未记录底层账号信息'}</dd></div>
        <div><dt>状态</dt><dd>{attempt.status || '—'}</dd></div>
        {errorSummary ? <div><dt>错误</dt><dd className="lr-neg">{errorSummary}</dd></div> : null}
      </dl>
      {attempt.error_detail && Object.keys(attempt.error_detail).length > 0 ? <pre className="lr-json">{stableJson(attempt.error_detail)}</pre> : null}
    </article>
  )
}

function DistributionCard({ title, rows }: { title: string; rows: DistributionRowModel[] }) {
  return (
    <section className="a-panel a-panel--pad">
      <div className="a-section-title" style={{ margin: 0 }}><h2>{title}</h2><span className="a-hint">当前页</span></div>
      <div className="a-dist" style={{ marginTop: 10 }}>
        {rows.length ? rows.map((row) => (
          <div key={row.label} className="a-dist__row">
            <span className="lr-clip" title={row.label}>{row.label}</span>
            <div className="a-dist__bar"><i style={{ width: `${row.percent}%` }} /></div>
            <b>{row.value}</b>
          </div>
        )) : <p className="a-hint">暂无数据</p>}
      </div>
    </section>
  )
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

function callRecordDistributions(records: CallRecord[], routeDirectory: Map<string, RouteModel>) {
  return {
    routes: topDistribution(records.map((record) => {
      const routeName = record.route_model_code ? routeDirectory.get(record.route_model_code)?.name : ''
      return routeName || record.route_model_code || record.abstract_model || '-'
    })),
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

function emptyDash(value?: string | null) {
  return value && value !== '0' ? value : value === '0' ? '0' : '-'
}

function stableJson(value: Record<string, unknown>) {
  return JSON.stringify(value, Object.keys(value).sort(), 2)
}

function requestSnapshot(record: CallRecord) {
  return stableJson({
    task_id: record.task_id,
    user_id: record.user_id,
    api_key_id: record.api_key_id,
    source_channel: record.source_channel,
    task_type: record.task_type,
    status: record.status,
    route_model_code: record.route_model_code,
    account_model_id: record.account_model_id,
    model_account_id: record.model_account_id,
    upstream_model_code: record.upstream_model_code,
    base_resolution: record.base_resolution,
    quality: record.quality,
    requested_output_image_count: record.requested_output_image_count,
    success_output_image_count: record.success_output_image_count,
    reference_image_count: record.reference_image_count,
    error_code: record.error_code,
    created_at: record.created_at,
    started_at: record.started_at,
    finished_at: record.finished_at,
  })
}

function formatAttemptTime(value?: string | null) {
  if (!value) return '-'
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!match) return value
  return `${match[1]}/${match[2]}/${match[3]} ${match[4]}:${match[5]}`
}

function channelLabel(channel?: string) {
  if (channel === 'web') return '用户 Web'
  if (channel === 'open_api') return 'Open API'
  if (channel === 'openai_compatible') return 'OpenAI 兼容'
  return channel || '未知入口'
}

function userStatusLabel(status?: string) {
  if (status === 'active') return '正常'
  if (status === 'pending') return '待验证'
  if (status === 'disabled') return '禁用'
  if (status === 'closed') return '已关闭'
  return status || '未知状态'
}

function avatarText(user: AdminUser) {
  return (user.display_name || user.nickname || user.email || '?').trim().slice(0, 1).toUpperCase()
}