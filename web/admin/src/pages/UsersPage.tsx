import { FormEvent, useEffect, useMemo, useState } from 'react'
import type { AdminUser, AdminUserCreateRequest, AdminUserDetail, ApiKey, BalanceBucket, ImageTask, LedgerEntry, PaymentOrder, UserGroup } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, EmptyBlock, ErrorBlock, Field, GroupOptionGrid, LoadingBlock, Modal, PageHeader, StatusCell, StatusStrip } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { adminDataGrid, adminGridCols } from '../ui/dataGrid'
import { canSaveUserPointAdjustment, newUserPointAdjustmentKey } from './userPointAdjustment'
import { userDetailLedgerRows } from './userDetailLedgerRows'
import {
  userDetailApiKeyRow,
  userDetailBucketRow,
  userDetailOrderRow,
  userDetailTaskRow,
} from './userDetailResourceRows'
import { adminUserRowView, adminUserStatusBadge, adminUserStatusFilterOptions, adminUserStatusOptions, adminUserSummary } from './userRows'

const pageSize = 20
const userFilterFormClass = 'grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] items-end gap-3 rounded-3xl border border-white/5 bg-white/[0.02] p-4'
const inlineControlClass = 'flex items-center gap-2'
const dangerTextClass = 'text-[var(--red)]'
const creditAmountClass = 'text-[var(--success)]'
const debitAmountClass = 'text-[var(--danger)]'

const userClasses = {
  tableWrap: 'min-w-0 overflow-x-auto rounded-3xl border border-[var(--line)] bg-white/[0.01] shadow-[0_20px_70px_rgba(0,0,0,.18)] backdrop-blur-sm',
  table: 'w-full min-w-[1040px] border-collapse text-left',
  th: 'border-b border-[var(--line)] bg-white/[0.02] px-6 py-4 text-[11px] font-extrabold uppercase tracking-wider text-[var(--muted-strong)]',
  tr: 'border-b border-[var(--line)]/60 transition-colors last:border-b-0 hover:bg-white/[0.03]',
  td: 'px-6 py-4 align-middle text-sm text-[var(--muted)]',
  userCell: 'flex min-w-0 items-center gap-3',
  avatar: 'grid size-10 shrink-0 place-items-center rounded-xl bg-white/5 text-sm font-black text-[var(--muted-strong)]',
  userName: 'font-bold text-[var(--text)]',
  userMeta: 'mt-1 text-[10px] text-[var(--muted-strong)] [overflow-wrap:anywhere]',
  balance: 'font-mono font-bold text-[var(--text)]',
  balanceUnit: 'text-[10px] text-[var(--muted-strong)]',
  actionRow: 'flex flex-wrap items-center gap-2',
  pager: 'flex flex-wrap items-center justify-between gap-3 border-t border-[var(--line)] p-4 text-xs text-[var(--muted)]',
}

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
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [action, setAction] = useState<UserAction | null>(null)
  const [detailTarget, setDetailTarget] = useState<AdminUser | null>(null)
  const [detail, setDetail] = useState<AdminUserDetail | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState<string | null>(null)

  const load = async (nextFilters = appliedFilters, nextPage = page) => {
    setLoading(true)
    setError(null)
    try {
      const userPage = await adminApi.listUsersPage(nextFilters.query.trim(), nextPage, pageSize, userListFilters(nextFilters))
      setRows(userPage.items)
      setTotal(userPage.total)
      if (!groups.length) {
        adminApi.listUserGroups().then(setGroups).catch(() => setGroups([]))
      }
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '用户载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load(appliedFilters, page)
  }, [appliedFilters, page])

  const totals = useMemo(() => adminUserSummary(rows), [rows])

  const saveAction = async () => {
    if (!action) return
    setSaving(true)
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
      setAction(null)
      await load(appliedFilters, page)
    } finally {
      setSaving(false)
    }
  }

  const updateStatus = async (user: AdminUser, status: string) => {
    setSaving(true)
    try {
      const updated = await adminApi.updateUserStatus(user.id, status)
      onFeedback('用户状态已更新', `${updated.display_name} · ${adminUserStatusBadge(updated.status).label}`)
      await load(appliedFilters, page)
    } finally {
      setSaving(false)
    }
  }

  const openDetail = async (user: AdminUser) => {
    setDetailTarget(user)
    setDetail(null)
    setDetailError(null)
    setDetailLoading(true)
    try {
      setDetail(await adminApi.getUser(user.id))
    } catch (caught) {
      setDetailError(caught instanceof Error ? caught.message : '用户详情载入失败')
    } finally {
      setDetailLoading(false)
    }
  }

  if (loading) return <LoadingBlock label="载入用户列表" />
  if (error) return <ErrorBlock message={error} onRetry={() => void load()} />

  return (
    <section className={adminPage.stack}>
      <PageHeader
        eyebrow="Users"
        title="用户管理"
        detail="按用户名、状态和分组定位用户，并支持按积分、最后活跃、创建时间排序。"
        actions={<button className={cn(adminButton.base, adminButton.primary)} type="button" onClick={() => setAction({ type: 'create', draft: { email: '', nickname: '', status: 'active', user_group_code: groups[0]?.code ?? 'basic', password: '', rpm_limit: 0, concurrency_limit: 0, default_locale: 'zh-CN', theme: 'system' } })}>新增用户</button>}
      />
      <StatusStrip columns={4}>
        <StatusCell label="正常" value={totals.active} />
        <StatusCell label="待验证" value={totals.pending} />
        <StatusCell label="禁用" value={totals.disabled} />
        <StatusCell label="权益分组" value={groups.length} />
      </StatusStrip>
      <section className={adminPage.fullSurface}>
        <section className={adminPage.mainLane}>
          <div className={adminPage.toolbar}>
            <form className={userFilterFormClass} onSubmit={(event) => { event.preventDefault(); setPage(1); setAppliedFilters(filters) }}>
              <input value={filters.query} onChange={(event) => setFilters({ ...filters, query: event.target.value })} placeholder="搜索用户名 / 邮箱" />
              <select value={filters.status} onChange={(event) => setFilters({ ...filters, status: event.target.value })} aria-label="状态筛选">
                {adminUserStatusFilterOptions.map((option) => <option key={option.value || 'all'} value={option.value}>{option.label}</option>)}
              </select>
              <GroupSelect value={filters.group} groups={groups} includeAll onChange={(value) => setFilters({ ...filters, group: value })} />
              <select value={filters.sortBy} onChange={(event) => setFilters({ ...filters, sortBy: event.target.value as UserFilters['sortBy'] })} aria-label="排序字段">
                <option value="points">积分</option>
                <option value="last_seen_at">最后活跃</option>
                <option value="created_at">创建时间</option>
              </select>
              <select value={filters.sortDir} onChange={(event) => setFilters({ ...filters, sortDir: event.target.value as UserFilters['sortDir'] })} aria-label="排序方向">
                <option value="desc">降序</option>
                <option value="asc">升序</option>
              </select>
              <button className={adminButton.base} type="submit">筛选</button>
              <button className={cn(adminButton.base, adminButton.ghost)} type="button" onClick={() => { setFilters(initialFilters); setAppliedFilters(initialFilters); setPage(1) }}>清空</button>
            </form>
            <span>第 {page} 页 / 共 {total} 条</span>
          </div>
          {!rows.length ? <EmptyBlock title="没有匹配用户" detail="尝试换一个关键词。" /> : (
            <div className={userClasses.tableWrap}>
              <table className={userClasses.table}>
                <thead>
                  <tr>
                    <th className={userClasses.th}>用户信息</th>
                    <th className={userClasses.th}>所属分组</th>
                    <th className={userClasses.th}>账户余额</th>
                    <th className={userClasses.th}>最后活跃</th>
                    <th className={userClasses.th}>状态</th>
                    <th className={userClasses.th}>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((rawRow) => (
                    <UserTableRow
                      key={rawRow.id}
                      row={adminUserRowView(rawRow)}
                      saving={saving}
                      groups={groups}
                      onOpenDetail={openDetail}
                      onUpdateStatus={updateStatus}
                      onAction={setAction}
                    />
                  ))}
                </tbody>
              </table>
              <div className={userClasses.pager}>
                <span>显示 {(page - 1) * pageSize + 1} 到 {Math.min(page * pageSize, total)} 共 {total} 条记录</span>
                <div className="flex flex-wrap items-center justify-end gap-2">
                  <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>上一页</button>
                  <button className={cn(adminButton.base, adminButton.primary, adminButton.small)} type="button" disabled>{page}</button>
                  <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" disabled={page * pageSize >= total} onClick={() => setPage((value) => value + 1)}>下一页</button>
                </div>
              </div>
            </div>
          )}
        </section>
      </section>
      {action ? (
        <Modal title={actionTitle(action)} detail={actionDetail(action)} onClose={() => setAction(null)} footer={<><button type="button" className={cn(adminButton.base, adminButton.ghost)} disabled={saving} onClick={() => setAction(null)}>取消</button><button type="button" className={cn(adminButton.base, adminButton.primary)} disabled={saving || !canSave(action)} onClick={() => void saveAction()}>{saving ? '保存中...' : '保存'}</button></>}>
          {renderActionForm(action, groups, setAction)}
        </Modal>
      ) : null}
      {detailTarget ? (
        <Modal
          title={`${detailTarget.display_name} · 用户详情`}
          detail={detailTarget.email}
          onClose={() => { setDetailTarget(null); setDetail(null); setDetailError(null) }}
          footer={<button type="button" className={adminButton.base} onClick={() => { setDetailTarget(null); setDetail(null); setDetailError(null) }}>关闭</button>}
        >
          {detailLoading ? <LoadingBlock label="载入用户详情" /> : null}
          {detailError ? <ErrorBlock message={detailError} onRetry={() => void openDetail(detailTarget)} /> : null}
          {detail && !detailLoading && !detailError ? <UserDetailView detail={detail} /> : null}
        </Modal>
      ) : null}
    </section>
  )
}

function UserTableRow({
  row,
  groups,
  saving,
  onOpenDetail,
  onUpdateStatus,
  onAction,
}: {
  row: ReturnType<typeof adminUserRowView>
  groups: UserGroup[]
  saving: boolean
  onOpenDetail: (user: AdminUser) => Promise<void>
  onUpdateStatus: (user: AdminUser, status: string) => Promise<void>
  onAction: (action: UserAction) => void
}) {
  return (
    <tr className={userClasses.tr}>
      <td className={userClasses.td}>
        <div className={userClasses.userCell}>
          <div className={userClasses.avatar}>{row.name.slice(0, 1).toUpperCase()}</div>
          <div className="min-w-0">
            <div className={userClasses.userName}>{row.name}</div>
            <div className={userClasses.userMeta}>{row.subtitle}</div>
          </div>
        </div>
      </td>
      <td className={userClasses.td}>{row.groupLabel}</td>
      <td className={userClasses.td}>
        <div className="flex items-baseline gap-1">
          <span className={userClasses.balance}>{row.balanceLabel}</span>
          <span className={userClasses.balanceUnit}>POINTS</span>
        </div>
      </td>
      <td className={userClasses.td}>{row.lastActiveAtLabel}</td>
      <td className={userClasses.td}><Badge tone={row.statusTone}>{row.statusLabel}</Badge></td>
      <td className={userClasses.td}>
        <div className={userClasses.actionRow}>
          <button className={cn(adminButton.base, adminButton.primary, adminButton.small)} type="button" onClick={() => void onOpenDetail(row.raw)}>详情</button>
          {row.raw.status === 'active' ? <button className={cn(adminButton.base, adminButton.danger, adminButton.small)} type="button" disabled={saving} onClick={() => void onUpdateStatus(row.raw, 'disabled')}>禁用</button> : null}
          {row.raw.status === 'disabled' ? <button className={cn(adminButton.base, adminButton.success, adminButton.small)} type="button" disabled={saving} onClick={() => void onUpdateStatus(row.raw, 'active')}>启用</button> : null}
          <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => onAction({ type: 'group', user: row.raw, groupIds: currentGroupIds(row.raw, groups) })}>分组</button>
          <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => onAction({ type: 'points', user: row.raw, changePoints: '0.00000', reason: '', idempotencyKey: newUserPointAdjustmentKey(row.raw.id) })}>积分</button>
          <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => onAction({ type: 'limits', user: row.raw, rpmLimit: String(row.raw.rpm_limit ?? 0), concurrencyLimit: String(row.raw.concurrency_limit ?? 0) })}>限额</button>
          <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => onAction({ type: 'password', user: row.raw, password: '' })}>密码</button>
          <button className={cn(adminButton.base, adminButton.ghost, adminButton.small, dangerTextClass)} type="button" onClick={() => onAction({ type: 'delete', user: row.raw, confirmEmail: '' })}>删除</button>
        </div>
      </td>
    </tr>
  )
}

function UserDetailView({ detail }: { detail: AdminUserDetail }) {
  const buckets = detail.balance?.buckets ?? []
  return (
    <section className={adminPage.detailStack}>
      <StatusStrip columns={4}>
        <StatusCell label="可用积分" value={detail.balance?.available_points ?? detail.user.balance} />
        <StatusCell label="体验额度" value={detail.balance?.trial_points ?? '0.00000'} />
        <StatusCell label="充值余额" value={detail.balance?.recharge_points ?? '0.00000'} />
        <StatusCell label="冻结积分" value={detail.balance?.frozen_points ?? '0.00000'} />
      </StatusStrip>
      <DetailSection title="余额桶" empty="暂无余额桶">
        {buckets.length ? <BucketGrid buckets={buckets} /> : null}
      </DetailSection>
      <DetailSection title="最近流水" empty="暂无流水">
        {detail.recent_ledger?.length ? <LedgerGrid items={detail.recent_ledger} /> : null}
      </DetailSection>
      <DetailSection title="最近订单" empty="暂无订单">
        {detail.recent_orders?.length ? <OrderGrid items={detail.recent_orders} /> : null}
      </DetailSection>
      <DetailSection title="最近任务" empty="暂无任务">
        {detail.recent_tasks?.length ? <TaskGrid items={detail.recent_tasks} /> : null}
      </DetailSection>
      <DetailSection title="API Key" empty="暂无 API Key">
        {detail.api_keys?.length ? <APIKeyGrid items={detail.api_keys} /> : null}
      </DetailSection>
    </section>
  )
}

function DetailSection({ title, empty, children }: { title: string; empty: string; children: React.ReactNode }) {
  return (
    <section className={adminPage.detailSection}>
      <div className={adminPage.sectionHead}><div className={adminPage.sectionTitle}>{title}</div></div>
      {children || <EmptyBlock title={empty} detail="该用户暂无对应记录。" />}
    </section>
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
        <Field label="初始密码"><input type="password" value={action.draft.password ?? ''} onChange={(event) => setAction({ ...action, draft: { ...action.draft, password: event.target.value } })} placeholder="可选，留空则使用验证码登录" autoComplete="new-password" name="user-initial-password" /></Field>
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
  return <Field label="新密码"><input type="password" value={action.password} onChange={(event) => setAction({ ...action, password: event.target.value })} placeholder="至少 8 位" autoComplete="new-password" name="user-reset-password" /></Field>
}

function GroupSelect({ value, groups, onChange, includeAll = false }: { value: string; groups: UserGroup[]; onChange: (value: string) => void; includeAll?: boolean }) {
  const options = groups.length ? groups : [{ code: 'basic', name: 'basic', group_code: 'basic', group_name: 'basic', multiplier: '1.00000', status: 'enabled', created_at: '', updated_at: '' }] as UserGroup[]
  return <select value={value} onChange={(event) => onChange(event.target.value)} aria-label="分组筛选">{includeAll ? <option value="">全部分组</option> : null}{options.map((group) => <option key={group.code} value={group.code}>{group.name || group.code}</option>)}</select>
}

function currentGroupIds(user: AdminUser, groups: UserGroup[]) {
  const codes = user.user_group_codes?.length ? user.user_group_codes : user.group.split(',').map((item) => item.trim()).filter(Boolean)
  return codes.map((code) => String(groups.find((group) => group.code === code)?.id ?? code))
}
