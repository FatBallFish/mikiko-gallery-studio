import { FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import type { AdminMetric, AdminRole, AdminSession, SystemAdminUser } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { ActionMenu, Badge, Drawer, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, MetricStrip, Modal, PageHeader, RefreshIconButton } from '../components'
import type { ColumnDef } from '../ui/dataTable'
import { DataTable, FilterToolbar, ListPage, Pager } from '../ui/dataTable'
import { FilterIcon, XIcon } from '../ui/listIcons'
import { createLatestListRequestGuard } from './listRefresh'
import './list-redesign.css'

type Filters = {
  query: string
  role: '' | AdminRole
  status: '' | 'active' | 'disabled'
}

type DialogState =
  | { type: 'create'; email: string; password: string; role: AdminRole; status: 'active' | 'disabled' }
  | { type: 'password'; user: SystemAdminUser; password: string }
  | { type: 'delete'; user: SystemAdminUser; confirmEmail: string }

const initialFilters: Filters = { query: '', role: '', status: '' }

export function SystemUsersPage({ session, onFeedback }: { session: AdminSession; onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<SystemAdminUser[]>([])
  const [filters, setFilters] = useState<Filters>(initialFilters)
  const [appliedFilters, setAppliedFilters] = useState<Filters>(initialFilters)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [dialog, setDialog] = useState<DialogState | null>(null)
  const [editTarget, setEditTarget] = useState<SystemAdminUser | null>(null)
  const [editRole, setEditRole] = useState<AdminRole>('admin')
  const [editStatus, setEditStatus] = useState<'active' | 'disabled'>('active')
  const [mutationError, setMutationError] = useState<string | null>(null)
  const requestGuard = useRef(createLatestListRequestGuard()).current
  const [reloadKey, setReloadKey] = useState(0)

  const load = async (nextFilters = appliedFilters, nextPage = page, nextPageize = pageSize) => {
    const request = requestGuard.begin()
    setLoading(true)
    setError(null)
    try {
      const result = await adminApi.systemAdmins.list({ ...nextFilters, page: nextPage, page_size: nextPageize })
      if (!requestGuard.isCurrent(request)) return
      setRows(result.items)
      setTotal(result.total)
    } catch (caught) {
      if (!requestGuard.isCurrent(request)) return
      setError(caught instanceof Error ? caught.message : '系统账户载入失败')
    } finally {
      if (!requestGuard.isCurrent(request)) return
      setLoading(false)
    }
  }

  useEffect(() => {
    void load(appliedFilters, page, pageSize)
    return () => requestGuard.invalidate()
  }, [appliedFilters, page, pageSize, reloadKey])

  const summary = useMemo(() => ({
    total,
    active: rows.filter((row) => row.status === 'active').length,
    superAdmins: rows.filter((row) => row.role === 'super_admin').length,
    disabled: rows.filter((row) => row.status === 'disabled').length,
  }), [rows, total])
  const metrics = useMemo<AdminMetric[]>(() => [
    { label: '账户总数', value: String(summary.total), trend: `当前页 ${rows.length} 个账户`, tone: 'neutral' },
    { label: '当前页启用', value: String(summary.active), trend: '可登录后台', tone: 'good' },
    { label: '超级管理员', value: String(summary.superAdmins), trend: '当前页完整权限账户', tone: 'neutral' },
    { label: '当前页停用', value: String(summary.disabled), trend: summary.disabled ? '无法继续登录' : '无停用账户', tone: summary.disabled ? 'warn' : 'good' },
  ], [rows.length, summary])

  const applyFilters = (event: FormEvent) => {
    event.preventDefault()
    setPage(1)
    setAppliedFilters(filters)
    setReloadKey((value) => value + 1)
  }

  const clearFilters = () => {
    setFilters(initialFilters)
    setAppliedFilters(initialFilters)
    setPage(1)
  }

  const openDialog = (next: DialogState) => {
    setMutationError(null)
    setDialog(next)
  }

  const closeDialog = () => {
    setMutationError(null)
    setDialog(null)
  }

  const openEdit = (user: SystemAdminUser) => {
    setMutationError(null)
    setEditTarget(user)
    setEditRole(normalizeRole(user.role))
    setEditStatus(normalizeStatus(user.status))
  }

  const closeEdit = () => {
    if (dialog?.type === 'password' || dialog?.type === 'delete') return
    setMutationError(null)
    setEditTarget(null)
  }

  const saveEdit = async () => {
    if (!editTarget) return
    setSaving(true)
    setMutationError(null)
    try {
      const updated = await adminApi.systemAdmins.update(editTarget.id, { role: editRole, status: editStatus })
      onFeedback('系统账户已更新', updated.email)
      setEditTarget(null)
      await load(appliedFilters, page)
    } catch (caught) {
      setMutationError(caught instanceof Error ? caught.message : '系统账户操作失败')
    } finally {
      setSaving(false)
    }
  }

  const saveDialog = async () => {
    if (!dialog) return
    setSaving(true)
    setMutationError(null)
    try {
      if (dialog.type === 'create') {
        const created = await adminApi.systemAdmins.create({ email: dialog.email.trim(), password: dialog.password, role: dialog.role, status: dialog.status })
        onFeedback('系统账户已创建', created.email)
      } else if (dialog.type === 'password') {
        await adminApi.systemAdmins.resetPassword(dialog.user.id, dialog.password)
        onFeedback('密码已重置', dialog.user.email)
      } else {
        await adminApi.systemAdmins.delete(dialog.user.id)
        onFeedback('系统账户已删除', dialog.user.email)
        if (editTarget && String(editTarget.id) === String(dialog.user.id)) setEditTarget(null)
      }
      setDialog(null)
      await load(appliedFilters, page)
    } catch (caught) {
      setMutationError(caught instanceof Error ? caught.message : '系统账户操作失败')
    } finally {
      setSaving(false)
    }
  }

  const canSave = dialog
    ? dialog.type === 'create'
      ? dialog.email.trim().includes('@') && dialog.password.length >= 6
      : dialog.type === 'password'
        ? dialog.password.length >= 6
        : dialog.confirmEmail === dialog.user.email
    : false

  if (loading && !rows.length) return <LoadingBlock label="载入系统账户" />

  return (
    <section className="a-page-stack">
      <PageHeader
        eyebrow="Administrators"
        title="管理员"
        description="控制台账户与权限。超级管理员可管理其他管理员；敏感操作全部写入审计日志。"
        primaryAction={<button type="button" className="a-btn a-btn--sm a-btn--primary" onClick={() => openDialog({ type: 'create', email: '', password: '', role: 'admin', status: 'active' })}>创建账户</button>}
        secondaryActions={<RefreshIconButton label="刷新系统账户" refreshing={loading} onClick={() => void load(appliedFilters, page, pageSize)} />}
      />
      <div className="a-banner a-banner--warn">
        <span>!</span>
        <div><strong style={{ fontSize: 13 }}>敏感操作会写入审计日志</strong><div className="a-hint">创建、编辑、重置密码与删除管理员均记录操作人、IP 与时间。</div></div>
      </div>
      {error ? <InlineFeedback tone="danger" message={`系统账户刷新失败：${error}`} /> : null}
      {loading && rows.length ? <InlineFeedback tone="neutral" message="正在刷新系统账户，当前数据会保留到新结果返回。" /> : null}
      <MetricStrip metrics={metrics} />
      <form onSubmit={applyFilters}>
        <FilterToolbar
          fields={[
            { key: 'query', label: '搜索', primary: true, minWidth: '220px', maxWidth: '420px', control: <input className="a-input" value={filters.query} onChange={(event) => setFilters({ ...filters, query: event.target.value })} placeholder="邮箱关键词" /> },
            { key: 'role', label: '角色', primary: true, control: <select className="a-select" value={filters.role} onChange={(event) => setFilters({ ...filters, role: event.target.value as Filters['role'] })}><option value="">全部角色</option><option value="super_admin">超级管理员</option><option value="admin">运营管理员</option></select> },
            { key: 'status', label: '状态', primary: true, control: <select className="a-select" value={filters.status} onChange={(event) => setFilters({ ...filters, status: event.target.value as Filters['status'] })}><option value="">全部状态</option><option value="active">启用</option><option value="disabled">停用</option></select> },
          ]}
          actions={(
            <>
              <button type="submit" className="a-btn a-btn--sm a-btn--primary"><FilterIcon className="size-4" /><span>筛选</span></button>
              <button type="button" className="a-btn a-btn--sm a-btn--quiet" onClick={clearFilters}><XIcon className="size-4" /><span>清空</span></button>
            </>
          )}
          resultSummary={`共 ${total} 个系统账户 · 当前显示 ${rows.length} 个`}
        />
      </form>
      {error && !rows.length ? <ErrorBlock message={error} onRetry={() => void load()} /> : null}
      {rows.length || (!loading && !error) ? (
        <ListPage
          resultSummary={<><h2>管理员列表</h2><p>当前账户不可删除；敏感操作会写入审计日志。</p></>}
          actions={<span className="a-hint">共 {total} 个账户</span>}
          pagination={<Pager page={page} pageSize={pageSize} total={total} onChange={setPage} onPageSizeChange={(size) => { setPageSize(size); setPage(1) }} />}
        >
          <DataTable
            columns={systemUserColumns(session, openEdit, openDialog)}
            rows={rows}
            rowKey={(user) => String(user.id)}
            empty={<EmptyBlock title="暂无系统账户" detail="调整筛选条件，或创建新的后台管理员账户。" />}
            minWidth={1000}
          />
        </ListPage>
      ) : null}

      {dialog?.type === 'create' ? (
        <Drawer
          title="创建管理员账户"
          description="新账户将使用独立后台登录体系"
          onClose={closeDialog}
          footer={(
            <>
              <button type="button" className="a-btn a-btn--sm a-btn--quiet" disabled={saving} onClick={closeDialog}>取消</button>
              <button type="button" className="a-btn a-btn--sm a-btn--primary" disabled={saving || !canSave} onClick={() => void saveDialog()}>{saving ? '保存中...' : '创建账户'}</button>
            </>
          )}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <form className="a-form" onSubmit={(event) => event.preventDefault()}>
            <section className="a-form__section">
              <div className="a-form__head"><h3>账户信息</h3></div>
              <Field label="邮箱"><input className="a-input" type="email" value={dialog.email} onChange={(event) => setDialog({ ...dialog, email: event.target.value })} placeholder="name@mikiko.studio" /></Field>
              <Field label="初始密码" hint="至少 6 位。"><input className="a-input" type="password" value={dialog.password} onChange={(event) => setDialog({ ...dialog, password: event.target.value })} /></Field>
              <Field label="角色"><RoleSelect value={dialog.role} onChange={(role) => setDialog({ ...dialog, role })} /></Field>
              <Field label="状态"><StatusSelect value={dialog.status} onChange={(status) => setDialog({ ...dialog, status })} /></Field>
            </section>
          </form>
        </Drawer>
      ) : null}

      {editTarget ? (
        <Drawer
          title={displayAdminName(editTarget)}
          description={`${editTarget.email} · #${editTarget.id}`}
          headerAside={<Badge tone={editStatus === 'active' ? 'success' : 'warning'}>{statusLabel(editStatus)}</Badge>}
          onClose={closeEdit}
          footer={(
            <>
              <button type="button" className="a-btn a-btn--sm a-btn--danger" disabled={saving || String(editTarget.id) === String(session.admin_id)} onClick={() => openDialog({ type: 'delete', user: editTarget, confirmEmail: '' })}>删除账户</button>
              <div className="lr-spacer" />
              <button type="button" className="a-btn a-btn--sm a-btn--quiet" disabled={saving} onClick={closeEdit}>取消</button>
              <button type="button" className="a-btn a-btn--sm a-btn--primary" disabled={saving} onClick={() => void saveEdit()}>{saving ? '保存中...' : '保存'}</button>
            </>
          )}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <form className="a-form" onSubmit={(event) => event.preventDefault()}>
            <section className="a-form__section">
              <div className="a-form__head"><h3>账户</h3></div>
              <Field label="邮箱" hint="账户邮箱创建后不可修改。"><input className="a-input" value={editTarget.email} disabled /></Field>
              <Field label="角色" hint="超级管理员可管理其他管理员与危险配置。"><RoleSelect value={editRole} onChange={setEditRole} /></Field>
              <Field label="状态" hint="停用后该管理员立即无法继续登录。"><StatusSelect value={editStatus} onChange={setEditStatus} /></Field>
            </section>
            <section className="a-form__section">
              <div className="a-form__head"><h3>安全</h3></div>
              <div className="lr-row">
                <button type="button" className="a-btn a-btn--sm" onClick={() => openDialog({ type: 'password', user: editTarget, password: '' })}>重置密码</button>
                <span className="a-hint">重置后立即生效，需二次确认。</span>
              </div>
              <div className="a-banner a-banner--danger">
                <span>!</span>
                <div><strong style={{ fontSize: 13 }}>不能删除当前登录账户</strong><div className="a-hint">当前账户 {session.admin_name || session.email} 受保护；如需删除其他管理员，请使用下方按钮。</div></div>
              </div>
            </section>
          </form>
        </Drawer>
      ) : null}

      {dialog?.type === 'password' ? (
        <Modal
          title="重置密码"
          detail={dialog.user.email}
          onClose={closeDialog}
          footer={<><button type="button" className="a-btn a-btn--sm a-btn--quiet" disabled={saving} onClick={closeDialog}>取消</button><button type="button" className="a-btn a-btn--sm a-btn--primary" disabled={saving || !canSave} onClick={() => void saveDialog()}>{saving ? '保存中...' : '确认重置'}</button></>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <Field label="新密码" hint="至少 6 位。"><input className="a-input" type="password" value={dialog.password} onChange={(event) => setDialog({ ...dialog, password: event.target.value })} /></Field>
        </Modal>
      ) : null}

      {dialog?.type === 'delete' ? (
        <Modal
          title="删除管理员账户"
          detail={dialog.user.email}
          onClose={closeDialog}
          footer={<><button type="button" className="a-btn a-btn--sm a-btn--quiet" disabled={saving} onClick={closeDialog}>取消</button><button type="button" className="a-btn a-btn--sm a-btn--danger" disabled={saving || !canSave} onClick={() => void saveDialog()}>{saving ? '删除中...' : '确认删除'}</button></>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <div className="lr-confirm-banner" style={{ marginBottom: 14 }}>
            <strong>删除不可恢复</strong>
            <p>请输入完整邮箱 <code className="a-mono">{dialog.user.email}</code> 确认删除。</p>
          </div>
          <Field label="确认邮箱"><input className="a-input" value={dialog.confirmEmail} onChange={(event) => setDialog({ ...dialog, confirmEmail: event.target.value })} placeholder={dialog.user.email} /></Field>
        </Modal>
      ) : null}
    </section>
  )
}

function RoleSelect({ value, onChange }: { value: AdminRole; onChange: (role: AdminRole) => void }) {
  return <select className="a-select" value={value} onChange={(event) => onChange(event.target.value as AdminRole)}><option value="admin">运营管理员</option><option value="super_admin">超级管理员</option></select>
}

function StatusSelect({ value, onChange }: { value: 'active' | 'disabled'; onChange: (status: 'active' | 'disabled') => void }) {
  return <select className="a-select" value={value} onChange={(event) => onChange(event.target.value as 'active' | 'disabled')}><option value="active">启用</option><option value="disabled">停用</option></select>
}

function systemUserColumns(session: AdminSession, openEdit: (user: SystemAdminUser) => void, openDialog: (dialog: DialogState) => void): ColumnDef<SystemAdminUser>[] {
  return [
    {
      key: 'user',
      title: '账户',
      width: 'minmax(250px,2fr)',
      render: (user) => {
        const isSelf = String(user.id) === String(session.admin_id)
        return (
          <span className="a-cell-entity">
            <span className="a-cell-entity__avatar">{displayAdminName(user).slice(0, 1).toUpperCase()}</span>
            <span className="a-cell-entity__copy">
              <span className="a-cell-entity__name">{displayAdminName(user)}{isSelf ? <Badge tone="primary">当前账户</Badge> : null}</span>
              <span className="a-cell-entity__meta">{user.email} · #{user.id}</span>
            </span>
          </span>
        )
      },
    },
    {
      key: 'role',
      title: '角色权限',
      width: 'minmax(170px,1.2fr)',
      render: (user) => <span className="lr-cell"><strong>{roleLabel(user.role)}</strong><span>{rolePermissionHint(user.role)}</span></span>,
    },
    {
      key: 'last_login',
      title: '最近活动',
      width: 'minmax(170px,1.2fr)',
      render: (user) => <span className="lr-cell"><strong style={{ fontSize: 12.5 }}>{formatDateTime(user.updated_at)}</strong><span>创建于 {formatDateTime(user.created_at)}</span></span>,
    },
    {
      key: 'status',
      title: '状态',
      width: 'minmax(90px,.7fr)',
      render: (user) => <Badge tone={user.status === 'active' ? 'success' : user.status === 'disabled' ? 'warning' : 'neutral'}>{statusLabel(user.status)}</Badge>,
    },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(210px,1.4fr)',
      align: 'right',
      render: (user) => {
        const isSelf = String(user.id) === String(session.admin_id)
        return (
          <div className="a-row-actions">
            <button type="button" className="a-row-btn a-row-btn--primary" onClick={() => openEdit(user)}>编辑</button>
            <button type="button" className="a-row-btn" onClick={() => openDialog({ type: 'password', user, password: '' })}>重置密码</button>
            {!isSelf ? <ActionMenu actions={[{ id: `delete-${user.id}`, label: '删除账户', tone: 'danger', run: () => openDialog({ type: 'delete', user, confirmEmail: '' }) }]} /> : null}
          </div>
        )
      },
    },
  ]
}

function roleLabel(role: string) {
  return role === 'super_admin' ? '超级管理员' : role === 'admin' ? '运营管理员' : role
}

function rolePermissionHint(role: string) {
  return role === 'super_admin' ? '完整控制台权限 · 可管理管理员' : role === 'admin' ? '用户 / 交易 / 审核 / 模型 / 配置' : '自定义权限集合'
}

function displayAdminName(user: SystemAdminUser) {
  return user.email.split('@')[0]?.trim() || user.email
}

function statusLabel(status: string) {
  return status === 'active' ? '启用' : status === 'disabled' ? '停用' : status
}

function normalizeRole(role: string): AdminRole {
  return role === 'super_admin' ? 'super_admin' : 'admin'
}

function normalizeStatus(status: string): 'active' | 'disabled' {
  return status === 'disabled' ? 'disabled' : 'active'
}

function formatDateTime(value?: string | null) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}