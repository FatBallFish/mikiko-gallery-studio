import { FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import type { AdminMetric, AdminUser, AdminUserCreateRequest, AdminUserDetail, ApiKey, BalanceBucket, ImageTask, LedgerEntry, PaymentOrder, UserGroup } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { ActionMenu, Badge, Drawer, EmptyBlock, ErrorBlock, Field, GroupOptionGrid, InlineFeedback, LoadingBlock, MetricStrip, Modal, PageHeader, RefreshIconButton } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { adminDataGrid, adminGridCols } from '../ui/dataGrid'
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
import { adminUserRowActions, adminUserRowView, adminUserStatusBadge, adminUserStatusFilterOptions, adminUserStatusOptions, adminUserSummary } from './userRows'
import { createLatestListRequestGuard } from './listRefresh'

const inlineControlClass = 'flex items-center gap-2'
const creditAmountClass = 'text-[var(--success)]'
const debitAmountClass = 'text-[var(--danger)]'

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
  const [detailTarget, setDetailTarget] = useState<AdminUser | null>(null)
  const [detail, setDetail] = useState<AdminUserDetail | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState<string | null>(null)
  const [detailFeedback, setDetailFeedback] = useState<string | null>(null)
  const requestGuard = useRef(createLatestListRequestGuard()).current
  const [reloadKey, setReloadKey] = useState(0)

  const load = async (nextFilters = appliedFilters, nextPage = page, nextPageize = pageSize) => {
    const request = requestGuard.begin()
    setLoading(true)
    setError(null)
    try {
      const [userPage, nextGroups] = await Promise.all([
        adminApi.listUsersPage(nextFilters.query.trim(), nextPage, nextPageize, userListFilters(nextFilters)),
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
        onFeedback('用户状态已更新', `${updated.display_name} · ${adminUserStatusBadge(updated.status).label}`)
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
      if (action.type !== 'create' && detailTarget?.id === action.user.id) {
        if (action.type === 'delete') {
          closeDetail()
        } else {
          setDetailFeedback(null)
          setDetailError(null)
          try {
            setDetail(await adminApi.getUser(action.user.id))
            setDetailFeedback('用户资料已更新，详情数据已同步。')
          } catch (caught) {
            const message = caught instanceof Error ? caught.message : '未知错误'
            setDetailError(`操作已完成，但详情刷新失败：${message}`)
          }
        }
      }
      setAction(null)
      setActionError(null)
      await load(appliedFilters, page)
    } catch (caught) {
      setActionError(caught instanceof Error ? caught.message : '用户操作失败')
    } finally {
      setSaving(false)
    }
  }

  const openDetail = async (user: AdminUser) => {
    setDetailTarget(user)
    setDetail(null)
    setDetailError(null)
    setDetailFeedback(null)
    setDetailLoading(true)
    try {
      setDetail(await adminApi.getUser(user.id))
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
  }

  const closeAction = () => {
    setAction(null)
    setActionError(null)
  }

  if (loading && !rows.length) return <LoadingBlock label="载入用户列表" />
  if (error && !rows.length) return <ErrorBlock message={error} onRetry={() => void load()} />

  return (
    <section className={adminPage.stack}>
      <PageHeader
        eyebrow="Users"
        title="用户管理"
        detail="按用户名、状态和分组定位用户，并支持按积分、最后活跃、创建时间排序。"
        primaryAction={<button className={cn(adminButton.base, adminButton.primary)} type="button" onClick={() => { setActionError(null); setAction({ type: 'create', draft: { email: '', nickname: '', status: 'active', user_group_code: groups[0]?.code ?? 'basic', password: '', rpm_limit: 0, concurrency_limit: 0, default_locale: 'zh-CN', theme: 'system' } }) }}>新增用户</button>}
        secondaryActions={<RefreshIconButton label="刷新用户列表" refreshing={loading} onClick={() => void load(appliedFilters, page, pageSize)} />}
      />
      {error ? <InlineFeedback tone="danger" message={`用户列表刷新失败：${error}`} /> : null}
      <MetricStrip metrics={summaryMetrics} />
      <ListPage
        filters={(
          <form onSubmit={(event) => { event.preventDefault(); setPage(1); setAppliedFilters(filters); setReloadKey((value) => value + 1) }}>
            <FilterToolbar
              fields={[
                { key: 'query', label: '关键词', primary: true, control: <input value={filters.query} onChange={(event) => setFilters({ ...filters, query: event.target.value })} placeholder="用户名 / 邮箱" /> },
                { key: 'status', label: '状态', primary: true, control: <select value={filters.status} onChange={(event) => setFilters({ ...filters, status: event.target.value })} aria-label="状态筛选">{adminUserStatusFilterOptions.map((option) => <option key={option.value || 'all'} value={option.value}>{option.label}</option>)}</select> },
                { key: 'group', label: '分组', control: <GroupSelect value={filters.group} groups={groups} includeAll onChange={(value) => setFilters({ ...filters, group: value })} /> },
                { key: 'sortBy', label: '排序字段', control: <select value={filters.sortBy} onChange={(event) => setFilters({ ...filters, sortBy: event.target.value as UserFilters['sortBy'] })} aria-label="排序字段"><option value="points">积分</option><option value="last_seen_at">最后活跃</option><option value="created_at">创建时间</option></select> },
                { key: 'sortDir', label: '排序方向', control: <select value={filters.sortDir} onChange={(event) => setFilters({ ...filters, sortDir: event.target.value as UserFilters['sortDir'] })} aria-label="排序方向"><option value="desc">降序</option><option value="asc">升序</option></select> },
              ]}
              actions={(
                <>
                  <button className={cn(adminButton.base, adminButton.primary, adminButton.small, 'gap-1.5')} type="submit" aria-label="筛选" title="筛选"><FilterIcon className="size-4" /><span>筛选</span></button>
                  <button className={cn(adminButton.base, adminButton.ghost, adminButton.small, 'gap-1.5')} type="button" aria-label="清空" title="清空" onClick={() => { setFilters(initialFilters); setAppliedFilters(initialFilters); setPage(1) }}><XIcon className="size-4" /><span>清空</span></button>
                </>
              )}
              resultSummary={`共 ${total} 位用户 · 当前显示 ${rows.length} 位`}
            />
          </form>
        )}
        pagination={<Pager page={page} pageSize={pageSize} total={total} onChange={setPage} onPageSizeChange={(size) => { setPageSize(size); setPage(1) }} />}
      >
          {!rows.length ? <EmptyBlock title="没有匹配用户" detail="尝试换一个关键词。" /> : (
            <DataTable
              columns={userColumns(groups, openDetail, setAction)}
              rows={rows}
              rowKey={(row) => row.id}
            />
          )}
      </ListPage>
      {detailTarget && !action ? (
        <Drawer
          title={`${adminUserRowView(detailTarget).name} · 用户详情`}
          description={`${detailTarget.email} · ${detailTarget.id}`}
          onClose={closeDetail}
          footer={<button type="button" className={adminButton.base} onClick={closeDetail}>关闭</button>}
        >
          {detailLoading ? <LoadingBlock label="载入用户详情" /> : null}
          {detailError ? <ErrorBlock message={detailError} onRetry={() => void openDetail(detailTarget)} /> : null}
          {detailFeedback ? <InlineFeedback tone="success" message={detailFeedback} /> : null}
          {detail && !detailLoading && !detailError ? <UserDetailView detail={detail} groups={groups} onAction={setAction} /> : null}
        </Drawer>
      ) : null}
      {action ? (
        <Modal title={actionTitle(action)} detail={actionDetail(action)} onClose={closeAction} footer={<><button type="button" className={cn(adminButton.base, adminButton.ghost)} disabled={saving} onClick={closeAction}>取消</button><button type="button" className={cn(adminButton.base, adminButton.primary)} disabled={saving || !canSave(action)} onClick={() => void saveAction()}>{saving ? '保存中...' : '保存'}</button></>}>
          {actionError ? <InlineFeedback tone="danger" message={actionError} /> : null}
          {renderActionForm(action, groups, setAction)}
        </Modal>
      ) : null}
    </section>
  )
}

function userColumns(
  groups: UserGroup[],
  onOpenDetail: (user: AdminUser) => Promise<void>,
  onAction: (action: UserAction) => void,
): ColumnDef<AdminUser>[] {
  return [
    {
      key: 'user',
      title: '用户信息',
      width: 'minmax(240px,2.5fr)',
      render: (rawRow) => {
        const row = adminUserRowView(rawRow)
        return (
          <div className="flex min-w-0 items-center gap-3">
            <div className="grid size-10 shrink-0 place-items-center rounded-xl bg-[var(--canvas)] text-sm font-black text-[var(--muted-strong)]">{row.name.slice(0, 1).toUpperCase()}</div>
            <div className="min-w-0">
              <div className="font-bold text-[var(--text)]">{row.name}</div>
              <div className="mt-1 text-xs text-[var(--muted-strong)] [overflow-wrap:anywhere]">{row.subtitle}</div>
            </div>
          </div>
        )
      },
    },
    {
      key: 'group',
      title: '所属分组',
      width: 'minmax(100px,1fr)',
      render: (rawRow) => adminUserRowView(rawRow).groupLabel,
    },
    {
      key: 'balance',
      title: '账户余额',
      width: 'minmax(120px,1fr)',
      render: (rawRow) => {
        const row = adminUserRowView(rawRow)
        return (
          <div className="flex items-baseline gap-1">
            <span className="font-mono font-bold text-[var(--text)]">{row.balanceLabel}</span>
            <span className="text-xs text-[var(--muted-strong)]">积分</span>
          </div>
        )
      },
    },
    {
      key: 'last_seen',
      title: '最后活跃',
      width: 'minmax(120px,1fr)',
      render: (rawRow) => adminUserRowView(rawRow).lastActiveAtLabel,
    },
    {
      key: 'status',
      title: '状态',
      width: 'minmax(90px,0.8fr)',
      render: (rawRow) => {
        const row = adminUserRowView(rawRow)
        return <Badge tone={row.statusTone}>{row.statusLabel}</Badge>
      },
    },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(180px,1.5fr)',
      render: (rawRow) => {
        const row = adminUserRowView(rawRow)
        const actions = adminUserRowActions(rawRow)
        const overflowActions = actions.overflow.map((action) => ({
          ...action,
          run: () => {
            if (action.id === 'disable') onAction({ type: 'status', user: rawRow, status: 'disabled' })
            if (action.id === 'enable') onAction({ type: 'status', user: rawRow, status: 'active' })
            if (action.id === 'group') onAction({ type: 'group', user: rawRow, groupIds: currentGroupIds(rawRow, groups) })
            if (action.id === 'points') onAction({ type: 'points', user: rawRow, changePoints: '0.00000', reason: '', idempotencyKey: newUserPointAdjustmentKey(rawRow.id) })
            if (action.id === 'limits') onAction({ type: 'limits', user: rawRow, rpmLimit: String(rawRow.rpm_limit ?? 0), concurrencyLimit: String(rawRow.concurrency_limit ?? 0) })
            if (action.id === 'password') onAction({ type: 'password', user: rawRow, password: '' })
            if (action.id === 'delete') onAction({ type: 'delete', user: rawRow, confirmEmail: '' })
          },
        }))
        return (
          <div className="flex flex-wrap items-center gap-2">
            <button className={cn(adminButton.base, adminButton.primary, adminButton.small)} type="button" onClick={() => void onOpenDetail(rawRow)}>{actions.primary.label}</button>
            <ActionMenu actions={overflowActions} />
          </div>
        )
      },
    },
  ]
}

function UserDetailView({ detail, groups, onAction }: { detail: AdminUserDetail; groups: UserGroup[]; onAction: (action: UserAction) => void }) {
  const buckets = detail.balance?.buckets ?? []
  const user = detail.user
  const row = adminUserRowView(user)
  const hasResources = Boolean(detail.recent_orders?.length || detail.recent_tasks?.length || detail.api_keys?.length)
  return (
    <section className={adminPage.detailStack}>
      <DetailSection dataSection="profile" title="基础资料" empty="暂无基础资料">
        <dl className="grid gap-x-5 gap-y-3 sm:grid-cols-2">
          <DetailFact label="用户" value={row.name} />
          <DetailFact label="状态" value={<Badge tone={row.statusTone}>{row.statusLabel}</Badge>} />
          <DetailFact label="用户 ID" value={<code className={adminDataGrid.code}>{user.id}</code>} />
          <DetailFact label="所属分组" value={row.groupLabel} />
          <DetailFact label="创建时间" value={row.createdAtLabel} />
          <DetailFact label="最后活跃" value={row.lastActiveAtLabel} />
        </dl>
      </DetailSection>

      <DetailSection dataSection="ledger" title="积分与流水" empty="暂无积分记录">
        <MetricStrip metrics={[
          { label: '可用积分', value: detail.balance?.available_points ?? user.balance, trend: '当前可用', tone: 'neutral' },
          { label: '体验额度', value: detail.balance?.trial_points ?? '0.00000', trend: '试用与赠送', tone: 'neutral' },
          { label: '充值余额', value: detail.balance?.recharge_points ?? '0.00000', trend: '付费充值', tone: 'good' },
          { label: '冻结积分', value: detail.balance?.frozen_points ?? '0.00000', trend: '暂不可用', tone: 'warn' },
        ]} />
        {buckets.length ? <BucketGrid buckets={buckets} /> : null}
        {detail.recent_ledger?.length ? <LedgerGrid items={detail.recent_ledger} /> : null}
      </DetailSection>

      <DetailSection dataSection="resources" title="关联资源" empty="暂无关联资源">
        {hasResources ? (
          <div className="grid gap-4">
            {detail.recent_orders?.length ? <OrderGrid items={detail.recent_orders} /> : null}
            {detail.recent_tasks?.length ? <TaskGrid items={detail.recent_tasks} /> : null}
            {detail.api_keys?.length ? <APIKeyGrid items={detail.api_keys} /> : null}
          </div>
        ) : <EmptyBlock title="暂无关联资源" detail="该用户暂无订单、生成任务或 API Key。" />}
      </DetailSection>

      <DetailSection dataSection="limits" title="限额与权限" empty="暂无限额信息">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <dl className="grid flex-1 gap-x-5 gap-y-3 sm:grid-cols-2">
            <DetailFact label="RPM 限额" value={<code className={adminDataGrid.code}>{String(user.rpm_limit ?? 0)}</code>} />
            <DetailFact label="并发限额" value={<code className={adminDataGrid.code}>{String(user.concurrency_limit ?? 0)}</code>} />
          </dl>
          <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => onAction({ type: 'limits', user, rpmLimit: String(user.rpm_limit ?? 0), concurrencyLimit: String(user.concurrency_limit ?? 0) })}>调整限额</button>
        </div>
      </DetailSection>

      <DetailSection dataSection="danger" title="账户与安全操作" empty="暂无可用操作">
        <div className="flex flex-wrap gap-2">
          <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => onAction({ type: 'group', user, groupIds: currentGroupIds(user, groups) })}>调整分组</button>
          <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => onAction({ type: 'points', user, changePoints: '0.00000', reason: '', idempotencyKey: newUserPointAdjustmentKey(user.id) })}>调整积分</button>
          <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => onAction({ type: 'password', user, password: '' })}>重置密码</button>
          <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => onAction({ type: 'status', user, status: user.status === 'disabled' ? 'active' : 'disabled' })}>{user.status === 'disabled' ? '启用账户' : '禁用账户'}</button>
          <button className={cn(adminButton.base, adminButton.danger, adminButton.small)} type="button" onClick={() => onAction({ type: 'delete', user, confirmEmail: '' })}>删除用户</button>
        </div>
      </DetailSection>
    </section>
  )
}

function DetailSection({ dataSection, title, empty, children }: { dataSection: 'profile' | 'ledger' | 'resources' | 'limits' | 'danger'; title: string; empty: string; children: React.ReactNode }) {
  return (
    <section className={adminPage.detailSection} data-admin-user-section={dataSection}>
      <div className={adminPage.sectionHead}><div className={adminPage.sectionTitle}>{title}</div></div>
      {children || <EmptyBlock title={empty} detail="该用户暂无对应记录。" />}
    </section>
  )
}

function DetailFact({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="grid gap-1 border-b border-[var(--border)] pb-3">
      <dt className="text-[11px] font-semibold text-[var(--dim)]">{label}</dt>
      <dd className="min-w-0 text-sm font-medium text-[var(--text)] [overflow-wrap:anywhere]">{value}</dd>
    </div>
  )
}

function BucketGrid({ buckets }: { buckets: BalanceBucket[] }) {
  const rows = buckets.map(userDetailBucketRow)
  return (
    <div className={cn(adminDataGrid.root, adminGridCols.userDetailBucket)}>
      <div className={cn(adminDataGrid.head, adminGridCols.userDetailBucket)}><span>类型</span><span>可用</span><span>过期时间</span></div>
      {rows.map((item) => (
        <div className={cn(adminDataGrid.row, adminGridCols.userDetailBucket)} key={item.key}>
          <strong>{item.label}</strong>
          <code className={adminDataGrid.code}>{item.availablePoints}</code>
          <span>{item.expiresAtLabel}</span>
        </div>
      ))}
    </div>
  )
}

function LedgerGrid({ items }: { items: LedgerEntry[] }) {
  const rows = userDetailLedgerRows(items)
  return (
    <div className={cn(adminDataGrid.root, adminGridCols.userDetailLedger)}>
      <div className={cn(adminDataGrid.head, adminGridCols.userDetailLedger)}><span>类型</span><span>桶/来源</span><span>变更</span><span>余额</span><span>有效期</span><span>时间</span></div>
      {rows.map((item) => (
        <div className={cn(adminDataGrid.row, adminGridCols.userDetailLedger)} key={String(item.id)}>
          <strong>{item.title}</strong>
          <span>{item.bucketLabel} · {item.sourceLabel}</span>
          <code className={cn(adminDataGrid.code, item.amountTone === 'credit' ? creditAmountClass : debitAmountClass)}>{item.amount}</code>
          <code className={adminDataGrid.code}>{item.balanceAfter}</code>
          <span>{item.expiryText}</span>
          <span>{item.occurredAt}</span>
        </div>
      ))}
    </div>
  )
}

function OrderGrid({ items }: { items: PaymentOrder[] }) {
  const rows = items.map(userDetailOrderRow)
  return (
    <div className={cn(adminDataGrid.root, adminGridCols.userDetailOrder)}>
      <div className={cn(adminDataGrid.head, adminGridCols.userDetailOrder)}><span>订单</span><span>状态</span><span>金额</span><span>积分</span><span>时间</span></div>
      {rows.map((item) => (
        <div className={cn(adminDataGrid.row, adminGridCols.userDetailOrder)} key={item.id}>
          <strong>{item.orderNo}</strong>
          <Badge tone={item.statusTone}>{item.statusLabel}</Badge>
          <code className={adminDataGrid.code}>{item.amountCny}</code>
          <code className={adminDataGrid.code}>{item.points}</code>
          <span>{item.createdAtLabel}</span>
        </div>
      ))}
    </div>
  )
}

function TaskGrid({ items }: { items: ImageTask[] }) {
  const rows = items.map(userDetailTaskRow)
  return (
    <div className={cn(adminDataGrid.root, adminGridCols.userDetailTask)}>
      <div className={cn(adminDataGrid.head, adminGridCols.userDetailTask)}><span>任务</span><span>状态</span><span>类型</span><span>模型</span><span>积分</span></div>
      {rows.map((item) => (
        <div className={cn(adminDataGrid.row, adminGridCols.userDetailTask)} key={item.id}>
          <code className={adminDataGrid.code}>{item.shortId}</code>
          <Badge tone={item.statusTone}>{item.statusLabel}</Badge>
          <span>{item.typeLabel}</span>
          <span>{item.modelLabel}</span>
          <code className={adminDataGrid.code}>{item.pointsLabel}</code>
        </div>
      ))}
    </div>
  )
}

function APIKeyGrid({ items }: { items: ApiKey[] }) {
  const rows = items.map(userDetailApiKeyRow)
  return (
    <div className={cn(adminDataGrid.root, adminGridCols.userDetailApiKey)}>
      <div className={cn(adminDataGrid.head, adminGridCols.userDetailApiKey)}><span>名称</span><span>状态</span><span>分组</span><span>Access Key</span><span>最近使用</span></div>
      {rows.map((item) => (
        <div className={cn(adminDataGrid.row, adminGridCols.userDetailApiKey)} key={item.id}>
          <strong>{item.name}</strong>
          <Badge tone={item.statusTone}>{item.statusLabel}</Badge>
          <span>{item.groupCode}</span>
          <code className={adminDataGrid.code}>{item.accessKey}</code>
          <span>{item.lastUsedAtLabel}</span>
        </div>
      ))}
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
  if (action.type === 'points') return '调整用户积分'
  if (action.type === 'limits') return '调整用户限额'
  if (action.type === 'delete') return '删除用户'
  return '重置用户密码'
}

function actionDetail(action: UserAction) {
  if (action.type === 'create') return '管理后台创建用户不需要邮箱验证码。'
  return action.user.email
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
      <form className={adminPage.formGrid} onSubmit={(event: FormEvent) => event.preventDefault()}>
        <Field label="邮箱"><input value={action.draft.email} onChange={(event) => setAction({ ...action, draft: { ...action.draft, email: event.target.value } })} required /></Field>
        <Field label="昵称"><input value={action.draft.nickname ?? ''} onChange={(event) => setAction({ ...action, draft: { ...action.draft, nickname: event.target.value } })} /></Field>
        <Field label="状态"><select value={action.draft.status ?? 'active'} onChange={(event) => setAction({ ...action, draft: { ...action.draft, status: event.target.value } })}>{adminUserStatusOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
        <Field label="默认分组"><GroupSelect value={action.draft.user_group_code ?? 'basic'} groups={groups} onChange={(value) => setAction({ ...action, draft: { ...action.draft, user_group_code: value } })} /></Field>
        <Field label="RPM 限额"><input type="number" min="0" value={action.draft.rpm_limit ?? 0} onChange={(event) => setAction({ ...action, draft: { ...action.draft, rpm_limit: Number(event.target.value) } })} /></Field>
        <Field label="并发限额"><input type="number" min="0" value={action.draft.concurrency_limit ?? 0} onChange={(event) => setAction({ ...action, draft: { ...action.draft, concurrency_limit: Number(event.target.value) } })} /></Field>
        <Field label="初始密码"><input type="password" value={action.draft.password ?? ''} onChange={(event) => setAction({ ...action, draft: { ...action.draft, password: event.target.value } })} placeholder="可选，留空则使用验证码登录" /></Field>
      </form>
    )
  }
  if (action.type === 'status') {
    return <Field label="新状态"><select value={action.status} onChange={(event) => setAction({ ...action, status: event.target.value })}>{adminUserStatusOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
  }
  if (action.type === 'delete') {
    return <><p>删除后该用户会从列表隐藏，并撤销登录会话。为避免误操作，请输入用户邮箱确认。</p><Field label="确认邮箱"><input value={action.confirmEmail} onChange={(event) => setAction({ ...action, confirmEmail: event.target.value })} placeholder={action.user.email} /></Field></>
  }
  if (action.type === 'group') {
    return <Field label="用户分组"><GroupOptionGrid selected={action.groupIds} groups={groups} onChange={(groupIds) => setAction({ ...action, groupIds })} /></Field>
  }
  if (action.type === 'points') {
    return (
      <>
        <Field label="变更积分"><input value={action.changePoints} onChange={(event) => setAction({ ...action, changePoints: event.target.value })} /></Field>
        <Field label="原因"><input value={action.reason} onChange={(event) => setAction({ ...action, reason: event.target.value })} placeholder="必填，写清本次调整原因" /></Field>
        <Field label="幂等键">
          <div className={inlineControlClass}>
            <input value={action.idempotencyKey} onChange={(event) => setAction({ ...action, idempotencyKey: event.target.value })} placeholder="必填，用于防止重复调整" />
            <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => setAction({ ...action, idempotencyKey: newUserPointAdjustmentKey(action.user.id) })}>生成</button>
          </div>
        </Field>
      </>
    )
  }
  if (action.type === 'limits') {
    return <div className={adminPage.formGrid}><Field label="RPM 限额"><input type="number" min="0" value={action.rpmLimit} onChange={(event) => setAction({ ...action, rpmLimit: event.target.value })} /></Field><Field label="并发限额"><input type="number" min="0" value={action.concurrencyLimit} onChange={(event) => setAction({ ...action, concurrencyLimit: event.target.value })} /></Field></div>
  }
  return <Field label="新密码"><input type="password" value={action.password} onChange={(event) => setAction({ ...action, password: event.target.value })} placeholder="至少 8 位" /></Field>
}

function GroupSelect({ value, groups, onChange, includeAll = false }: { value: string; groups: UserGroup[]; onChange: (value: string) => void; includeAll?: boolean }) {
  const options = groups.length ? groups : [{ code: 'basic', name: 'basic', group_code: 'basic', group_name: 'basic', multiplier: '1.00000', status: 'enabled', created_at: '', updated_at: '' }] as UserGroup[]
  return <select value={value} onChange={(event) => onChange(event.target.value)} aria-label="分组筛选">{includeAll ? <option value="">全部分组</option> : null}{options.map((group) => <option key={group.code} value={group.code}>{group.name || group.code}</option>)}</select>
}

function currentGroupIds(user: AdminUser, groups: UserGroup[]) {
  const codes = user.user_group_codes?.length ? user.user_group_codes : user.group.split(',').map((item) => item.trim()).filter(Boolean)
  return codes.map((code) => String(groups.find((group) => group.code === code)?.id ?? code))
}
