import { FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import type { AdminMetric, AdminUser, AdminUserCreateRequest, AdminUserDetail, ApiKey, BalanceBucket, ImageTask, LedgerEntry, PaymentOrder, UserGroup } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { ActionMenu, AdminTabs, Badge, Drawer, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, MetricStrip, Modal, PageHeader, RefreshIconButton } from '../components'
import { ColumnDef, DataTable, FilterToolbar, ListPage, Pager } from '../ui/dataTable'
import { FilterIcon, XIcon } from '../ui/listIcons'
import { canSaveUserPointAdjustment, newUserPointAdjustmentKey } from './userPointAdjustment'
import { userDetailLedgerRows } from './userDetailLedgerRows'
import {
  userDetailApiKeyRow,
  userDetailBucketRow,
  userDetailOrderRow,
  userDetailTaskRow,
} from './userDetailResourceRows'
import { adminUserRowActions, adminUserRowView, adminUserStatusFilterOptions, adminUserStatusOptions, adminUserSummary } from './userRows'
import { createLatestListRequestGuard } from './listRefresh'
import './list-redesign.css'

type UserAction =
  | { type: 'create'; draft: AdminUserCreateRequest }
  | { type: 'status'; user: AdminUser; status: string }
  | { type: 'group'; user: AdminUser; groupIds: string[] }
  | { type: 'points'; user: AdminUser; changePoints: string; reason: string; idempotencyKey: string }
  | { type: 'limits'; user: AdminUser; rpmLimit: string; concurrencyLimit: string }
  | { type: 'password'; user: AdminUser; password: string }
  | { type: 'delete'; user: AdminUser; confirmEmail: string }

type UserFilters = {
  query: string
  status: string
  group: string
  sortBy: 'points' | 'last_seen_at' | 'created_at'
  sortDir: 'desc' | 'asc'
}

type DetailTab = 'overview' | 'points' | 'resources' | 'security'
type ResourceTab = 'tasks' | 'orders' | 'keys'

const initialFilters: UserFilters = { query: '', status: '', group: '', sortBy: 'created_at', sortDir: 'desc' }

export function UsersPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<AdminUser[]>([])
  const [groups, setGroups] = useState<UserGroup[]>([])
  const [filters, setFilters] = useState<UserFilters>(initialFilters)
  const [appliedFilters, setAppliedFilters] = useState<UserFilters>(initialFilters)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [action, setAction] = useState<UserAction | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [peekUser, setPeekUser] = useState<AdminUser | null>(null)
  const [detailTarget, setDetailTarget] = useState<AdminUser | null>(null)
  const [detail, setDetail] = useState<AdminUserDetail | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState<string | null>(null)
  const [detailFeedback, setDetailFeedback] = useState<string | null>(null)
  const [detailTab, setDetailTab] = useState<DetailTab>('overview')
  const [resourceTab, setResourceTab] = useState<ResourceTab>('tasks')
  const requestGuard = useRef(createLatestListRequestGuard()).current
  const [reloadKey, setReloadKey] = useState(0)

  const load = async (nextFilters = appliedFilters, nextPage = page, nextPageSize = pageSize) => {
    const request = requestGuard.begin()
    setLoading(true)
    setError(null)
    try {
      const [userPage, nextGroups] = await Promise.all([
        adminApi.listUsersPage(nextFilters.query.trim(), nextPage, nextPageSize, userListFilters(nextFilters)),
        groups.length ? Promise.resolve(null) : adminApi.listUserGroups().catch(() => []),
      ])
      if (!requestGuard.isCurrent(request)) return
      setRows(userPage.items)
      setTotal(userPage.total)
      if (nextGroups) setGroups(nextGroups)
    } catch (caught) {
      if (!requestGuard.isCurrent(request)) return
      setError(caught instanceof Error ? caught.message : '用户载入失败')
    } finally {
      if (!requestGuard.isCurrent(request)) return
      setLoading(false)
    }
  }

  useEffect(() => {
    void load(appliedFilters, page, pageSize)
    return () => requestGuard.invalidate()
  }, [appliedFilters, page, pageSize, reloadKey])

  const totals = useMemo(() => adminUserSummary(rows), [rows])
  const summaryMetrics = useMemo<AdminMetric[]>(() => [
    { label: '当前结果', value: String(total), trend: `本页 ${rows.length} 位用户`, tone: 'neutral' },
    { label: '正常', value: String(totals.active), trend: '当前页可用账户', tone: 'good' },
    { label: '待处理', value: String(totals.pending + totals.disabled), trend: `${totals.pending} 待验证 · ${totals.disabled} 禁用`, tone: totals.pending + totals.disabled > 0 ? 'warn' : 'neutral' },
    { label: '权益分组', value: String(groups.length), trend: '已配置分组', tone: 'neutral' },
  ], [groups.length, rows.length, total, totals])

  const saveAction = async () => {
    if (!action) return
    setSaving(true)
    setActionError(null)
    try {
      if (action.type === 'create') {
        const created = await adminApi.createUser(action.draft)
        onFeedback('用户已创建', created.email)
      }
      if (action.type === 'status') {
        const updated = await adminApi.updateUserStatus(action.user.id, action.status)
        onFeedback('用户状态已更新', `${updated.display_name} · ${adminUserRowView(updated).statusLabel}`)
      }
      if (action.type === 'group') {
        await adminApi.assignUserGroups(action.user.id, action.groupIds)
        onFeedback('用户分组已更新', `${action.user.display_name} · ${action.groupIds.length} 个分组`)
      }
      if (action.type === 'points') {
        await adminApi.adjustUserPoints(action.user.id, action.changePoints, action.reason, action.idempotencyKey)
        onFeedback('用户积分已调整', `${action.user.display_name} · ${action.changePoints}`)
      }
      if (action.type === 'limits') {
        await adminApi.updateUserLimits(action.user.id, Number(action.rpmLimit), Number(action.concurrencyLimit))
        onFeedback('用户限额已更新', action.user.display_name)
      }
      if (action.type === 'password') {
        await adminApi.resetUserPassword(action.user.id, action.password)
        onFeedback('用户密码已重置', action.user.display_name)
      }
      if (action.type === 'delete') {
        await adminApi.deleteUser(action.user.id)
        onFeedback('用户已删除', action.user.email)
      }
      const actedUserID = action.type === 'create' ? '' : action.user.id
      setAction(null)
      setActionError(null)
      if (detailTarget && actedUserID && detailTarget.id === actedUserID) {
        if (action.type === 'delete') {
          closeDetail()
        } else {
          setDetailFeedback(null)
          setDetailError(null)
          try {
            const nextDetail = await adminApi.getUser(detailTarget.id)
            setDetail(nextDetail)
            setDetailTarget(nextDetail.user)
            setDetailFeedback('用户资料已更新，详情数据已同步。')
          } catch (caught) {
            const message = caught instanceof Error ? caught.message : '未知错误'
            setDetailError(`操作已完成，但详情刷新失败：${message}`)
          }
        }
      }
      await load(appliedFilters, page)
    } catch (caught) {
      setActionError(caught instanceof Error ? caught.message : '用户操作失败')
    } finally {
      setSaving(false)
    }
  }

  const openDetail = async (user: AdminUser) => {
    setPeekUser(null)
    setDetailTarget(user)
    setDetail(null)
    setDetailError(null)
    setDetailFeedback(null)
    setDetailTab('overview')
    setResourceTab('tasks')
    setDetailLoading(true)
    try {
      const nextDetail = await adminApi.getUser(user.id)
      setDetail(nextDetail)
      setDetailTarget(nextDetail.user)
    } catch (caught) {
      setDetailError(caught instanceof Error ? caught.message : '用户详情载入失败')
    } finally {
      setDetailLoading(false)
    }
  }

  const closeDetail = () => {
    setDetailTarget(null)
    setDetail(null)
    setDetailError(null)
    setDetailFeedback(null)
    setAction(null)
    setActionError(null)
  }

  const closeAction = () => {
    setAction(null)
    setActionError(null)
  }

  const openAction = (next: UserAction) => {
    setPeekUser(null)
    setActionError(null)
    setAction(next)
  }

  if (loading && !rows.length) return <LoadingBlock label="载入用户列表" />
  if (error && !rows.length) return <ErrorBlock message={error} onRetry={() => void load()} />

  return (
    <section className="a-page-stack">
      <PageHeader
        eyebrow="Users"
        title="用户管理"
        description="按关键词、状态与分组定位用户。点击名称查看账户摘要，详情与积分调整在右侧抽屉中完成。"
        primaryAction={<button className="a-btn a-btn--sm a-btn--primary" type="button" onClick={() => openAction({ type: 'create', draft: blankUserDraft(groups) })}>新增用户</button>}
        secondaryActions={<RefreshIconButton label="刷新用户列表" refreshing={loading} onClick={() => void load(appliedFilters, page, pageSize)} />}
      />
      {error ? <InlineFeedback tone="danger" message={`用户列表刷新失败：${error}`} /> : null}
      {loading && rows.length ? <InlineFeedback tone="neutral" message="正在刷新用户列表，当前数据会保留到新结果返回。" /> : null}
      <MetricStrip metrics={summaryMetrics} />
      <ListPage
        resultSummary={<><h2>用户列表</h2><p>点击名称查看账户摘要；点击详情打开完整用户抽屉。</p></>}
        actions={<span className="a-hint">共 {total} 位用户</span>}
        filters={(
          <form onSubmit={(event) => { event.preventDefault(); setPage(1); setAppliedFilters(filters); setReloadKey((value) => value + 1) }}>
            <FilterToolbar
              fields={[
                { key: 'query', label: '关键词', primary: true, control: <input className="a-input" value={filters.query} onChange={(event) => setFilters({ ...filters, query: event.target.value })} placeholder="用户名 / 邮箱 / 用户 ID" /> },
                { key: 'status', label: '用户状态', primary: true, control: <select className="a-select" value={filters.status} onChange={(event) => setFilters({ ...filters, status: event.target.value })} aria-label="状态筛选">{adminUserStatusFilterOptions.map((option) => <option key={option.value || 'all'} value={option.value}>{option.label}</option>)}</select> },
                { key: 'group', label: '所属分组', primary: true, control: <GroupSelect value={filters.group} groups={groups} includeAll onChange={(value) => setFilters({ ...filters, group: value })} /> },
                { key: 'sortBy', label: '排序', primary: true, control: <select className="a-select" value={filters.sortBy} onChange={(event) => setFilters({ ...filters, sortBy: event.target.value as UserFilters['sortBy'] })} aria-label="排序字段"><option value="points">积分</option><option value="last_seen_at">最后活跃</option><option value="created_at">创建时间</option></select> },
                { key: 'sortDir', label: '排序方向', control: <select className="a-select" value={filters.sortDir} onChange={(event) => setFilters({ ...filters, sortDir: event.target.value as UserFilters['sortDir'] })} aria-label="排序方向"><option value="desc">降序</option><option value="asc">升序</option></select> },
              ]}
              actions={(
                <>
                  <button className="a-btn a-btn--sm a-btn--primary" type="submit" aria-label="筛选"><FilterIcon className="size-4" /><span>查询</span></button>
                  <button className="a-btn a-btn--sm a-btn--quiet" type="button" onClick={() => { setFilters(initialFilters); setAppliedFilters(initialFilters); setPage(1) }}><XIcon className="size-4" /><span>重置</span></button>
                </>
              )}
              resultSummary={`共 ${total} 位用户 · 当前显示 ${rows.length} 位`}
            />
          </form>
        )}
        pagination={<Pager page={page} pageSize={pageSize} total={total} onChange={setPage} onPageSizeChange={(size) => { setPageSize(size); setPage(1) }} />}
      >
        {!rows.length ? <EmptyBlock title="没有匹配用户" detail="尝试换一个关键词，或清空筛选条件。" /> : (
          <DataTable
            columns={userColumns(groups, (user) => setPeekUser(user), openDetail, openAction)}
            rows={rows}
            rowKey={(row) => row.id}
            minWidth={1080}
          />
        )}
      </ListPage>

      {peekUser ? (
        <Modal
          title="用户摘要"
          detail={peekUser.email}
          onClose={() => setPeekUser(null)}
          footer={<><button className="a-btn a-btn--sm a-btn--quiet" type="button" onClick={() => setPeekUser(null)}>关闭</button><button className="a-btn a-btn--sm a-btn--primary" type="button" onClick={() => void openDetail(peekUser)}>打开完整用户抽屉</button></>}
        >
          <UserPeek user={peekUser} groups={groups} />
        </Modal>
      ) : null}

      {detailTarget ? (
        <Drawer
          wide
          title={adminUserRowView(detailTarget).name}
          description={`${detailTarget.email} · #${detailTarget.id} · 注册于 ${adminUserRowView(detailTarget).createdAtLabel}`}
          onClose={() => { if (!action) closeDetail() }}
          headerAside={<UserHeaderBadges user={detail?.user ?? detailTarget} groups={groups} />}
          footer={(
            <>
              <button className="a-btn a-btn--sm" type="button" onClick={() => openAction(pointsAction(detail?.user ?? detailTarget))}>调整积分</button>
              <button className="a-btn a-btn--sm" type="button" onClick={() => openAction({ type: 'group', user: detail?.user ?? detailTarget, groupIds: currentGroupIds(detail?.user ?? detailTarget, groups) })}>调整分组</button>
              <div className="lr-spacer" />
              <button className="a-btn a-btn--sm a-btn--primary" type="button" onClick={closeDetail}>关闭</button>
            </>
          )}
        >
          <AdminTabs
            ariaLabel="用户详情分区"
            className="lr-drawer-tabs"
            value={detailTab}
            onChange={(value) => setDetailTab(value)}
            items={[
              { id: 'overview', label: '概览' },
              { id: 'points', label: '积分与流水', badge: detail?.recent_ledger?.length || undefined },
              { id: 'resources', label: '关联资源', badge: (detail?.recent_orders?.length ?? 0) + (detail?.recent_tasks?.length ?? 0) + (detail?.api_keys?.length ?? 0) || undefined },
              { id: 'security', label: '账户与安全' },
            ]}
          />
          {detailLoading ? <LoadingBlock label="载入用户详情" /> : null}
          {detailError ? <ErrorBlock message={detailError} onRetry={() => void openDetail(detailTarget)} /> : null}
          {detailFeedback ? <InlineFeedback tone="success" message={detailFeedback} /> : null}
          {detail && !detailLoading && !detailError ? (
            <UserDetailView
              detail={detail}
              groups={groups}
              tab={detailTab}
              resourceTab={resourceTab}
              onResourceTab={setResourceTab}
              onAction={openAction}
            />
          ) : null}
        </Drawer>
      ) : null}

      {action ? (
        <Drawer
          stacked={Boolean(detailTarget)}
          title={actionTitle(action)}
          description={actionDetail(action)}
          onClose={closeAction}
          footer={(
            <>
              <button className="a-btn a-btn--sm a-btn--quiet" type="button" disabled={saving} onClick={closeAction}>取消</button>
              <button className={cn('a-btn a-btn--sm', action.type === 'delete' ? 'a-btn--danger' : 'a-btn--primary')} type="button" disabled={saving || !canSave(action)} onClick={() => void saveAction()}>{saving ? '保存中...' : '确认'}</button>
            </>
          )}
        >
          {actionError ? <InlineFeedback tone="danger" message={actionError} /> : null}
          {renderActionForm(action, groups, setAction)}
        </Drawer>
      ) : null}
    </section>
  )
}

function userColumns(
  groups: UserGroup[],
  onPeek: (user: AdminUser) => void,
  onOpenDetail: (user: AdminUser) => Promise<void>,
  onAction: (action: UserAction) => void,
): ColumnDef<AdminUser>[] {
  return [
    {
      key: 'user',
      title: '用户',
      width: 'minmax(240px,2.4fr)',
      render: (rawRow) => {
        const row = adminUserRowView(rawRow)
        return (
          <button className="a-cell-entity" type="button" onClick={() => onPeek(rawRow)} title={`${row.name} · ${rawRow.email}`}>
            <span className="a-cell-entity__avatar">{avatarText(row.name)}</span>
            <span className="a-cell-entity__copy">
              <span className="a-cell-entity__name">{row.name}</span>
              <span className="a-cell-entity__meta">{rawRow.email || '未设置邮箱'} · #{rawRow.id}</span>
            </span>
          </button>
        )
      },
    },
    {
      key: 'group',
      title: '所属分组',
      width: 'minmax(130px,1fr)',
      render: (rawRow) => {
        const codes = rawRow.user_group_codes?.length ? rawRow.user_group_codes : rawRow.group.split(',').map((item) => item.trim()).filter(Boolean)
        if (!codes.length) return <span className="lr-muted">未分组</span>
        return <span className="lr-row">{codes.slice(0, 2).map((code) => <Badge key={code} tone="primary">{groupName(groups, code)}</Badge>)}{codes.length > 2 ? <span className="lr-muted">+{codes.length - 2}</span> : null}</span>
      },
    },
    {
      key: 'balance',
      title: '账户余额',
      width: 'minmax(120px,.9fr)',
      align: 'right',
      kind: 'number',
      render: (rawRow) => <span className="lr-num">{adminUserRowView(rawRow).balanceLabel}</span>,
    },
    {
      key: 'last_seen',
      title: '最后活跃',
      width: 'minmax(130px,1fr)',
      render: (rawRow) => <span className="lr-muted">{adminUserRowView(rawRow).lastActiveAtLabel}</span>,
    },
    {
      key: 'status',
      title: '状态',
      width: 'minmax(96px,.8fr)',
      render: (rawRow) => {
        const row = adminUserRowView(rawRow)
        return <Badge tone={row.statusTone}>{row.statusLabel}</Badge>
      },
    },
    {
      key: 'created',
      title: '注册时间',
      width: 'minmax(130px,1fr)',
      render: (rawRow) => <span className="lr-muted">{adminUserRowView(rawRow).createdAtLabel}</span>,
    },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(190px,1.4fr)',
      align: 'right',
      render: (rawRow) => {
        const actions = adminUserRowActions(rawRow)
        const overflowActions = actions.overflow.map(({ confirm: _confirm, ...item }) => ({
          ...item,
          run: () => {
            if (item.id === 'disable') onAction({ type: 'status', user: rawRow, status: 'disabled' })
            if (item.id === 'enable') onAction({ type: 'status', user: rawRow, status: 'active' })
            if (item.id === 'group') onAction({ type: 'group', user: rawRow, groupIds: currentGroupIds(rawRow, groups) })
            if (item.id === 'points') onAction(pointsAction(rawRow))
            if (item.id === 'limits') onAction({ type: 'limits', user: rawRow, rpmLimit: String(rawRow.rpm_limit ?? 0), concurrencyLimit: String(rawRow.concurrency_limit ?? 0) })
            if (item.id === 'password') onAction({ type: 'password', user: rawRow, password: '' })
            if (item.id === 'delete') onAction({ type: 'delete', user: rawRow, confirmEmail: '' })
          },
        }))
        return (
          <div className="a-row-actions">
            <button className="a-row-btn a-row-btn--primary" type="button" onClick={() => void onOpenDetail(rawRow)}>详情</button>
            <button className="a-row-btn" type="button" onClick={() => onAction(pointsAction(rawRow))}>积分</button>
            <ActionMenu actions={overflowActions} />
          </div>
        )
      },
    },
  ]
}

function UserPeek({ user, groups }: { user: AdminUser; groups: UserGroup[] }) {
  const row = adminUserRowView(user)
  return (
    <div className="lr-stack--sm">
      <div className="lr-entity-head">
        <span className="lr-avatar-lg">{avatarText(row.name)}</span>
        <span className="lr-entity-head__copy">
          <strong style={{ fontSize: 15 }}>{row.name}</strong>
          <span className="lr-muted">#{user.id} · {groupName(groups, row.raw.group.split(',')[0]?.trim() || row.raw.group)} · {row.statusLabel}</span>
        </span>
      </div>
      <dl className="a-kv">
        <div><dt>邮箱</dt><dd>{user.email || '未设置邮箱'}</dd></div>
        <div><dt>积分余额</dt><dd><span className="lr-num">{row.balanceLabel}</span></dd></div>
        <div><dt>所属分组</dt><dd>{row.groupLabel || '未分组'}</dd></div>
        <div><dt>最后活跃</dt><dd>{row.lastActiveAtLabel}</dd></div>
        <div><dt>注册时间</dt><dd>{row.createdAtLabel}</dd></div>
      </dl>
    </div>
  )
}

function UserHeaderBadges({ user, groups }: { user: AdminUser; groups: UserGroup[] }) {
  const row = adminUserRowView(user)
  const codes = user.user_group_codes?.length ? user.user_group_codes : user.group.split(',').map((item) => item.trim()).filter(Boolean)
  return (
    <>
      <Badge tone={row.statusTone}>{row.statusLabel}</Badge>
      {codes.slice(0, 2).map((code) => <Badge key={code} tone="primary">{groupName(groups, code)}</Badge>)}
    </>
  )
}

function UserDetailView({
  detail,
  groups,
  tab,
  resourceTab,
  onResourceTab,
  onAction,
}: {
  detail: AdminUserDetail
  groups: UserGroup[]
  tab: DetailTab
  resourceTab: ResourceTab
  onResourceTab: (tab: ResourceTab) => void
  onAction: (action: UserAction) => void
}) {
  const buckets = detail.balance?.buckets ?? []
  const user = detail.user
  const row = adminUserRowView(user)
  const hasResources = Boolean(detail.recent_orders?.length || detail.recent_tasks?.length || detail.api_keys?.length)

  if (tab === 'overview') {
    return (
      <div className="lr-stack">
        <div className="a-metrics lr-metrics-3">
          <div className="a-metric"><div className="a-metric__label">可用积分</div><div className="a-metric__value" style={{ fontSize: 22 }}>{detail.balance?.available_points ?? row.balanceLabel}</div><div className="a-metric__foot">冻结 {detail.balance?.frozen_points ?? '0.00000'}</div></div>
          <div className="a-metric"><div className="a-metric__label">近期任务</div><div className="a-metric__value" style={{ fontSize: 22 }}>{detail.recent_tasks?.length ?? 0}</div><div className="a-metric__foot">最近生成的图片任务</div></div>
          <div className="a-metric"><div className="a-metric__label">关联资源</div><div className="a-metric__value" style={{ fontSize: 22 }}>{(detail.recent_orders?.length ?? 0) + (detail.recent_tasks?.length ?? 0) + (detail.api_keys?.length ?? 0)}</div><div className="a-metric__foot">订单 · 任务 · API 密钥</div></div>
        </div>
        <section className="a-drawer__section" data-admin-user-section="profile">
          <h3>基础资料 <Badge tone={row.statusTone}>{row.statusLabel}</Badge></h3>
          <div className="lr-entity-head">
            <span className="lr-avatar-lg">{avatarText(row.name)}</span>
            <span className="lr-entity-head__copy">
              <strong style={{ fontSize: 15 }}>{row.name}</strong>
              <span className="lr-muted">{user.email || '未设置邮箱'} · #{user.id}</span>
            </span>
          </div>
          <dl className="a-kv">
            <div><dt>用户 ID</dt><dd><code className="a-mono">#{user.id}</code></dd></div>
            <div><dt>邮箱</dt><dd>{user.email || '未设置邮箱'}</dd></div>
            <div><dt>所属分组</dt><dd><span className="lr-row">{detailGroups(user, groups).map((group) => <Badge key={group.code} tone="primary">{group.label}</Badge>)}{!detailGroups(user, groups).length ? <span className="lr-muted">未分组</span> : null}</span></dd></div>
            <div><dt>注册时间</dt><dd>{row.createdAtLabel}</dd></div>
            <div><dt>最后活跃</dt><dd>{row.lastActiveAtLabel}</dd></div>
            <div><dt>限额</dt><dd>RPM {user.rpm_limit ?? 0} · 并发 {user.concurrency_limit ?? 0}</dd></div>
          </dl>
        </section>
        <section className="a-drawer__section">
          <h3>最近任务</h3>
          {detail.recent_tasks?.length ? (
            <DataTable
              columns={taskColumns()}
              rows={detail.recent_tasks}
              rowKey={(task) => task.id}
              minWidth={520}
            />
          ) : <EmptyBlock variant="inline" title="暂无最近任务" detail="该用户最近没有图片生成任务。" />}
        </section>
      </div>
    )
  }

  if (tab === 'points') {
    return (
      <div className="lr-stack" data-admin-user-section="ledger">
        <div className="a-metrics lr-metrics-3">
          <div className="a-metric"><div className="a-metric__label">体验额度</div><div className="a-metric__value" style={{ fontSize: 20 }}>{detail.balance?.trial_points ?? '0.00000'}</div><div className="a-metric__foot">试用与赠送</div></div>
          <div className="a-metric"><div className="a-metric__label">充值余额</div><div className="a-metric__value" style={{ fontSize: 20 }}>{detail.balance?.recharge_points ?? '0.00000'}</div><div className="a-metric__foot">付费充值</div></div>
          <div className="a-metric"><div className="a-metric__label">冻结积分</div><div className="a-metric__value" style={{ fontSize: 20 }}>{detail.balance?.frozen_points ?? '0.00000'}</div><div className="a-metric__foot">暂不可用</div></div>
        </div>
        {buckets.length ? <BucketTable buckets={buckets} /> : null}
        <section className="a-panel a-panel--flush">
          <div className="a-panel__head">
            <div><h2>积分流水</h2><p>最近 {detail.recent_ledger?.length ?? 0} 条记录</p></div>
            <button className="a-btn a-btn--sm" type="button" onClick={() => onAction(pointsAction(user))}>调整积分</button>
          </div>
          {detail.recent_ledger?.length ? (
            <LedgerTable items={detail.recent_ledger} />
          ) : <EmptyBlock variant="inline" title="暂无积分流水" detail="该用户还没有积分变动记录。" />}
        </section>
      </div>
    )
  }

  if (tab === 'resources') {
    return (
      <div className="lr-stack" data-admin-user-section="resources">
        <AdminTabs
          ariaLabel="关联资源分区"
          value={resourceTab}
          onChange={onResourceTab}
          items={[
            { id: 'tasks', label: '生图任务', badge: detail.recent_tasks?.length || undefined },
            { id: 'orders', label: '订单', badge: detail.recent_orders?.length || undefined },
            { id: 'keys', label: 'API 密钥', badge: detail.api_keys?.length || undefined },
          ]}
        />
        {!hasResources ? <EmptyBlock title="暂无关联资源" detail="该用户暂无订单、生成任务或 API Key。" /> : null}
        {resourceTab === 'tasks' && detail.recent_tasks?.length ? <TaskTable items={detail.recent_tasks} /> : null}
        {resourceTab === 'tasks' && !detail.recent_tasks?.length ? <EmptyBlock variant="inline" title="暂无生图任务" detail="该用户还没有任务记录。" /> : null}
        {resourceTab === 'orders' && detail.recent_orders?.length ? <OrderTable items={detail.recent_orders} /> : null}
        {resourceTab === 'orders' && !detail.recent_orders?.length ? <EmptyBlock variant="inline" title="暂无订单" detail="该用户还没有支付订单。" /> : null}
        {resourceTab === 'keys' && detail.api_keys?.length ? <APIKeyTable items={detail.api_keys} /> : null}
        {resourceTab === 'keys' && !detail.api_keys?.length ? <EmptyBlock variant="inline" title="暂无 API 密钥" detail="该用户还没有创建 API 密钥。" /> : null}
      </div>
    )
  }

  return (
    <div className="lr-stack">
      <div className="a-banner a-banner--warn">
        <span>!</span>
        <div><strong style={{ fontSize: 13 }}>敏感操作会写入审计日志</strong><div className="a-hint">调整分组、积分、禁用与重置密码均记录操作人、IP 与原因。</div></div>
      </div>
      <section className="a-drawer__section" data-admin-user-section="limits">
        <h3>限额与权限 <button className="a-row-btn" type="button" onClick={() => onAction({ type: 'limits', user, rpmLimit: String(user.rpm_limit ?? 0), concurrencyLimit: String(user.concurrency_limit ?? 0) })}>调整限额</button></h3>
        <dl className="a-kv">
          <div><dt>RPM 限额</dt><dd><code className="a-mono">{String(user.rpm_limit ?? 0)}</code></dd></div>
          <div><dt>并发限额</dt><dd><code className="a-mono">{String(user.concurrency_limit ?? 0)}</code></dd></div>
          <div><dt>默认语言</dt><dd>{user.default_locale || 'zh-CN'}</dd></div>
          <div><dt>主题</dt><dd>{user.theme || 'system'}</dd></div>
        </dl>
      </section>
      <section className="a-drawer__section" data-admin-user-section="danger">
        <h3>账户与安全操作</h3>
        <div className="lr-action-grid">
          <button className="lr-action-card" type="button" onClick={() => onAction({ type: 'group', user, groupIds: currentGroupIds(user, groups) })}><strong>调整分组</strong><span>{row.groupLabel || '未分组'} · 立即影响模型可见性与计费倍率</span></button>
          <button className="lr-action-card" type="button" onClick={() => onAction(pointsAction(user))}><strong>调整积分</strong><span>增加或扣减，需填写原因与幂等键</span></button>
          <button className="lr-action-card" type="button" onClick={() => onAction({ type: 'password', user, password: '' })}><strong>重置密码</strong><span>生成新的登录密码，立即生效</span></button>
          <button className="lr-action-card" type="button" onClick={() => onAction({ type: 'status', user, status: user.status === 'disabled' ? 'active' : 'disabled' })}><strong>{user.status === 'disabled' ? '启用账户' : '禁用账户'}</strong><span>禁用后无法登录与调用 API，资产保留</span></button>
          <button className="lr-action-card lr-action-card--danger" type="button" onClick={() => onAction({ type: 'delete', user, confirmEmail: '' })}><strong>删除用户</strong><span>不可恢复；历史订单与积分流水保留</span></button>
        </div>
      </section>
    </div>
  )
}

function BucketTable({ buckets }: { buckets: BalanceBucket[] }) {
  const rows = buckets.map(userDetailBucketRow)
  return (
    <div className="a-table-wrap" style={{ border: '1px solid var(--a-line)', borderRadius: 9 }}>
      <table className="a-table" style={{ minWidth: 520 }}>
        <thead><tr><th>额度桶</th><th className="is-num">可用积分</th><th>过期时间</th></tr></thead>
        <tbody>
          {rows.map((item) => (
            <tr key={item.key}>
              <td><strong>{item.label}</strong></td>
              <td className="is-num">{item.availablePoints}</td>
              <td className={item.expiryTone === 'warning' ? 'lr-neg' : 'is-muted'}>{item.expiresAtLabel}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function LedgerTable({ items }: { items: LedgerEntry[] }) {
  const rows = userDetailLedgerRows(items)
  return (
    <div className="a-table-wrap">
      <table className="a-table" style={{ minWidth: 720 }}>
        <thead><tr><th>类型</th><th>额度桶 / 来源</th><th className="is-num">变更</th><th className="is-num">余额</th><th>有效期</th><th>时间</th></tr></thead>
        <tbody>
          {rows.map((item) => (
            <tr key={String(item.id)}>
              <td>{item.title}</td>
              <td className="is-muted">{item.bucketLabel} · {item.sourceLabel}</td>
              <td className={cn('is-num', item.amountTone === 'credit' ? 'lr-pos' : 'lr-neg')}>{item.amount}</td>
              <td className="is-num">{item.balanceAfter}</td>
              <td className="is-muted">{item.expiryText}</td>
              <td className="is-muted">{item.occurredAt}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function taskColumns(): ColumnDef<ImageTask>[] {
  return [
    { key: 'task', title: '任务', width: 'minmax(130px,1.2fr)', render: (task) => <span className="lr-code">{userDetailTaskRow(task).shortId}</span> },
    { key: 'type', title: '类型', width: 'minmax(100px,1fr)', render: (task) => userDetailTaskRow(task).typeLabel },
    { key: 'model', title: '模型', width: 'minmax(120px,1fr)', render: (task) => userDetailTaskRow(task).modelLabel },
    { key: 'points', title: '积分', width: 'minmax(90px,.8fr)', align: 'right', kind: 'number', render: (task) => userDetailTaskRow(task).pointsLabel },
    { key: 'status', title: '状态', width: 'minmax(90px,.8fr)', render: (task) => { const row = userDetailTaskRow(task); return <Badge tone={row.statusTone}>{row.statusLabel}</Badge> } },
  ]
}

function TaskTable({ items }: { items: ImageTask[] }) {
  return <DataTable columns={taskColumns()} rows={items} rowKey={(task) => task.id} minWidth={520} />
}

function OrderTable({ items }: { items: PaymentOrder[] }) {
  const rows = items.map(userDetailOrderRow)
  return (
    <div className="a-table-wrap">
      <table className="a-table" style={{ minWidth: 620 }}>
        <thead><tr><th>订单</th><th>状态</th><th className="is-num">金额</th><th className="is-num">积分</th><th>时间</th></tr></thead>
        <tbody>
          {rows.map((item) => (
            <tr key={item.id}>
              <td><span className="lr-code">{item.orderNo}</span></td>
              <td><Badge tone={item.statusTone}>{item.statusLabel}</Badge></td>
              <td className="is-num">¥ {item.amountCny}</td>
              <td className="is-num">{item.points}</td>
              <td className="is-muted">{item.createdAtLabel}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function APIKeyTable({ items }: { items: ApiKey[] }) {
  const rows = items.map(userDetailApiKeyRow)
  return (
    <div className="a-table-wrap">
      <table className="a-table" style={{ minWidth: 620 }}>
        <thead><tr><th>名称</th><th>状态</th><th>分组</th><th>Access Key</th><th>最近使用</th></tr></thead>
        <tbody>
          {rows.map((item) => (
            <tr key={item.id}>
              <td><strong>{item.name}</strong></td>
              <td><Badge tone={item.statusTone}>{item.statusLabel}</Badge></td>
              <td>{item.groupCode}</td>
              <td><span className="lr-code">{item.accessKey}</span></td>
              <td className="is-muted">{item.lastUsedAtLabel}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function userListFilters(filters: UserFilters): Record<string, string | undefined> {
  return {
    status: filters.status || undefined,
    group_code: filters.group || undefined,
    sort_by: filters.sortBy === initialFilters.sortBy ? undefined : filters.sortBy,
    sort_dir: filters.sortDir === initialFilters.sortDir ? undefined : filters.sortDir,
  }
}

function actionTitle(action: UserAction) {
  if (action.type === 'create') return '新增用户'
  if (action.type === 'status') return '修改用户状态'
  if (action.type === 'group') return '调整用户分组'
  if (action.type === 'points') return '调整积分'
  if (action.type === 'limits') return '调整用户限额'
  if (action.type === 'delete') return '删除用户'
  return '重置用户密码'
}

function actionDetail(action: UserAction) {
  if (action.type === 'create') return '创建后发送验证邮件；积分与限额可在用户详情中调整。'
  return `${action.user.email} · #${action.user.id}`
}

function canSave(action: UserAction) {
  if (action.type === 'create') return Boolean(action.draft.email?.trim())
  if (action.type === 'password') return action.password.length >= 8
  if (action.type === 'points') return canSaveUserPointAdjustment(action)
  if (action.type === 'delete') return action.confirmEmail.trim() === action.user.email
  return true
}

function renderActionForm(action: UserAction, groups: UserGroup[], setAction: (action: UserAction) => void) {
  if (action.type === 'create') {
    return (
      <form className="a-form" onSubmit={(event: FormEvent) => event.preventDefault()}>
        <section className="a-form__section">
          <div className="a-form__head"><h3><span className="a-form__num">1</span>账户信息</h3><p>字段纵向排列，避免两栏高度不一致造成错位。</p></div>
          <Field label="邮箱"><input className="a-input" type="email" value={action.draft.email} onChange={(event) => setAction({ ...action, draft: { ...action.draft, email: event.target.value } })} placeholder="name@example.com" required /></Field>
          <Field label="显示昵称"><input className="a-input" value={action.draft.nickname ?? ''} onChange={(event) => setAction({ ...action, draft: { ...action.draft, nickname: event.target.value } })} placeholder="例如：岑屿" /></Field>
        </section>
        <section className="a-form__section">
          <div className="a-form__head"><h3><span className="a-form__num">2</span>分组与状态</h3></div>
          <Field label="默认分组" hint="决定模型可见范围与计费倍率。"><GroupSelect className="a-select" value={action.draft.user_group_code ?? 'basic'} groups={groups} onChange={(value) => setAction({ ...action, draft: { ...action.draft, user_group_code: value } })} /></Field>
          <Field label="初始状态"><select className="a-select" value={action.draft.status ?? 'active'} onChange={(event) => setAction({ ...action, draft: { ...action.draft, status: event.target.value } })}>{adminUserStatusOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
        </section>
        <section className="a-form__section">
          <div className="a-form__head"><h3><span className="a-form__num">3</span>限额与密码</h3></div>
          <div className="lr-form-pair">
            <Field label="RPM 限额"><input className="a-input" type="number" min="0" value={action.draft.rpm_limit ?? 0} onChange={(event) => setAction({ ...action, draft: { ...action.draft, rpm_limit: Number(event.target.value) } })} /></Field>
            <Field label="并发限额"><input className="a-input" type="number" min="0" value={action.draft.concurrency_limit ?? 0} onChange={(event) => setAction({ ...action, draft: { ...action.draft, concurrency_limit: Number(event.target.value) } })} /></Field>
          </div>
          <Field label="初始密码" hint="可选，留空则使用验证码登录。"><input className="a-input" type="password" value={action.draft.password ?? ''} onChange={(event) => setAction({ ...action, draft: { ...action.draft, password: event.target.value } })} placeholder="至少 8 位" /></Field>
        </section>
      </form>
    )
  }
  if (action.type === 'status') {
    return (
      <form className="a-form" onSubmit={(event: FormEvent) => event.preventDefault()}>
        <section className="a-form__section">
          <div className="a-form__head"><h3>账户状态</h3><p>停用后该用户无法登录与调用 API，资产与历史数据保留。</p></div>
          <Field label="新状态"><select className="a-select" value={action.status} onChange={(event) => setAction({ ...action, status: event.target.value })}>{adminUserStatusOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
        </section>
      </form>
    )
  }
  if (action.type === 'delete') {
    return (
      <form className="a-form" onSubmit={(event: FormEvent) => event.preventDefault()}>
        <section className="a-form__section">
          <div className="a-form__head"><h3>删除确认</h3><p>删除后该用户会从列表隐藏，并撤销登录会话。历史订单与积分流水保留。</p></div>
          <div className="lr-confirm-banner">
            <strong>删除不可恢复</strong>
            <p>请输入完整邮箱 <code className="a-mono">{action.user.email}</code> 确认删除。</p>
            <Field label="确认邮箱"><input className="a-input" value={action.confirmEmail} onChange={(event) => setAction({ ...action, confirmEmail: event.target.value })} placeholder={action.user.email} /></Field>
          </div>
        </section>
      </form>
    )
  }
  if (action.type === 'group') {
    return (
      <form className="a-form" onSubmit={(event: FormEvent) => event.preventDefault()}>
        <section className="a-form__section">
          <div className="a-form__head"><h3>用户分组</h3><p>分组决定模型可见性与计费倍率，可多选。</p></div>
          {groups.length ? groups.map((group) => {
            const id = String(group.id ?? group.code)
            const checked = action.groupIds.includes(id) || action.groupIds.includes(group.code)
            return (
              <label key={id} className="a-switch">
                <input
                  type="checkbox"
                  checked={checked}
                  onChange={(event) => {
                    const next = event.target.checked
                      ? Array.from(new Set([...action.groupIds.filter((item) => item !== group.code), id]))
                      : action.groupIds.filter((item) => item !== id && item !== group.code)
                    setAction({ ...action, groupIds: next })
                  }}
                />
                <span className="a-switch__track" />
                <span>{group.name} · {group.multiplier}x</span>
              </label>
            )
          }) : <p className="a-hint">暂无可选分组。</p>}
        </section>
      </form>
    )
  }
  if (action.type === 'points') {
    return (
      <form className="a-form" onSubmit={(event: FormEvent) => event.preventDefault()}>
        <section className="a-form__section">
          <div className="a-form__head"><h3>变更</h3><p>正数为增加，负数为扣减。操作会写入积分流水与审计日志。</p></div>
          <Field label="变更积分" hint="调整后余额以服务端结算结果为准。"><input className="a-input" value={action.changePoints} onChange={(event) => setAction({ ...action, changePoints: event.target.value })} placeholder="例如 500 或 -120" inputMode="decimal" /></Field>
          <Field label="原因"><textarea className="a-textarea" value={action.reason} onChange={(event) => setAction({ ...action, reason: event.target.value })} placeholder="例如：补偿 10-21 批次生成失败" /></Field>
          <Field label="幂等键" hint="相同幂等键的重复请求不会重复调整。">
            <div className="lr-row">
              <input className="a-input" value={action.idempotencyKey} onChange={(event) => setAction({ ...action, idempotencyKey: event.target.value })} placeholder="必填" />
              <button className="a-btn a-btn--sm" type="button" onClick={() => setAction({ ...action, idempotencyKey: newUserPointAdjustmentKey(action.user.id) })}>生成</button>
            </div>
          </Field>
        </section>
        <div className="a-banner a-banner--warn"><span>!</span><div><strong style={{ fontSize: 13 }}>扣减不会低于 0</strong><div className="a-hint">若余额不足，系统会将可用余额扣至 0 并记录差额。</div></div></div>
      </form>
    )
  }
  if (action.type === 'limits') {
    return (
      <form className="a-form" onSubmit={(event: FormEvent) => event.preventDefault()}>
        <section className="a-form__section">
          <div className="a-form__head"><h3>请求限额</h3><p>0 表示不额外限制，具体以服务端配置为准。</p></div>
          <div className="lr-form-pair">
            <Field label="RPM 限额"><input className="a-input" type="number" min="0" value={action.rpmLimit} onChange={(event) => setAction({ ...action, rpmLimit: event.target.value })} /></Field>
            <Field label="并发限额"><input className="a-input" type="number" min="0" value={action.concurrencyLimit} onChange={(event) => setAction({ ...action, concurrencyLimit: event.target.value })} /></Field>
          </div>
        </section>
      </form>
    )
  }
  return (
    <form className="a-form" onSubmit={(event: FormEvent) => event.preventDefault()}>
      <section className="a-form__section">
        <div className="a-form__head"><h3>重置密码</h3><p>新密码立即生效，用户下次登录需使用新密码。</p></div>
        <Field label="新密码" hint="至少 8 位。"><input className="a-input" type="password" value={action.password} onChange={(event) => setAction({ ...action, password: event.target.value })} placeholder="至少 8 位" /></Field>
      </section>
    </form>
  )
}

function GroupSelect({ value, groups, onChange, includeAll = false, className }: { value: string; groups: UserGroup[]; onChange: (value: string) => void; includeAll?: boolean; className?: string }) {
  const options = groups.length ? groups : [{ code: 'basic', name: 'basic', group_code: 'basic', group_name: 'basic', multiplier: '1.00000', status: 'enabled', created_at: '', updated_at: '' }] as UserGroup[]
  return <select className={className ?? 'a-select'} value={value} onChange={(event) => onChange(event.target.value)} aria-label="分组筛选">{includeAll ? <option value="">全部分组</option> : null}{options.map((group) => <option key={group.code} value={group.code}>{group.name || group.code}</option>)}</select>
}

function currentGroupIds(user: AdminUser, groups: UserGroup[]) {
  const codes = user.user_group_codes?.length ? user.user_group_codes : user.group.split(',').map((item) => item.trim()).filter(Boolean)
  return codes.map((code) => String(groups.find((group) => group.code === code)?.id ?? code))
}

function detailGroups(user: AdminUser, groups: UserGroup[]) {
  const codes = user.user_group_codes?.length ? user.user_group_codes : user.group.split(',').map((item) => item.trim()).filter(Boolean)
  return codes.map((code) => {
    const group = groups.find((item) => item.code === code)
    return { code, label: group ? `${group.name} · ${group.multiplier}x` : code }
  })
}

function groupName(groups: UserGroup[], code: string) {
  const normalized = (code ?? '').trim()
  if (!normalized) return '未分组'
  return groups.find((group) => group.code === normalized)?.name || normalized
}

function avatarText(name: string) {
  return (name || '?').trim().slice(0, 1).toUpperCase()
}

function pointsAction(user: AdminUser): UserAction {
  return { type: 'points', user, changePoints: '0.00000', reason: '', idempotencyKey: newUserPointAdjustmentKey(user.id) }
}

function blankUserDraft(groups: UserGroup[]): AdminUserCreateRequest {
  return { email: '', nickname: '', status: 'active', user_group_code: groups[0]?.code ?? 'basic', password: '', rpm_limit: 0, concurrency_limit: 0, default_locale: 'zh-CN', theme: 'system' }
}