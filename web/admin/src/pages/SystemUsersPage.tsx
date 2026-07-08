import { FormEvent, useEffect, useMemo, useState } from 'react'
import type { AdminRole, AdminSession, SystemAdminUser } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, EmptyBlock, ErrorBlock, Field, LoadingBlock, Modal, PageHeader, StatusCell, StatusStrip } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { ColumnDef, DataTable, FilterBar, ListPage, Pager } from '../ui/dataTable'
import { FilterIcon, XIcon } from '../ui/listIcons'

const systemUserClasses = {
  identity: 'flex min-w-0 items-center gap-3',
  avatar: 'grid size-10 shrink-0 place-items-center rounded-xl bg-[var(--canvas)] text-sm font-black text-[var(--muted-strong)]',
  title: 'block truncate font-bold text-[var(--text)]',
  detail: 'mt-1 block truncate text-[11px] font-medium text-[var(--soft)]',
  role: 'grid gap-1',
  roleName: 'font-bold text-[var(--text)]',
  roleHint: 'text-[11px] text-[var(--soft)]',
  time: 'font-mono text-xs tracking-tight text-[var(--soft)]',
  actions: 'flex flex-wrap items-center justify-end gap-2',
}

type Filters = {
  query: string
  role: '' | AdminRole
  status: '' | 'active' | 'disabled'
}

type DialogState =
  | { type: 'create'; email: string; password: string; role: AdminRole; status: 'active' | 'disabled' }
  | { type: 'edit'; user: SystemAdminUser; role: AdminRole; status: 'active' | 'disabled' }
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

  const load = async (nextFilters = appliedFilters, nextPage = page, nextPageize = pageSize) => {
    setLoading(true)
    setError(null)
    try {
      const result = await adminApi.systemAdmins.list({ ...nextFilters, page: nextPage, page_size: nextPageize })
      setRows(result.items)
      setTotal(result.total)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '系统账户载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load(appliedFilters, page, pageSize)
  }, [appliedFilters, page, pageSize])

  const summary = useMemo(() => ({
    total,
    active: rows.filter((row) => row.status === 'active').length,
    superAdmins: rows.filter((row) => row.role === 'super_admin').length,
    disabled: rows.filter((row) => row.status === 'disabled').length,
  }), [rows, total])

  const applyFilters = (event: FormEvent) => {
    event.preventDefault()
    setPage(1)
    setAppliedFilters(filters)
  }

  const saveDialog = async () => {
    if (!dialog) return
    setSaving(true)
    try {
      if (dialog.type === 'create') {
        const created = await adminApi.systemAdmins.create({ email: dialog.email.trim(), password: dialog.password, role: dialog.role, status: dialog.status })
        onFeedback('系统账户已创建', created.email)
      } else if (dialog.type === 'edit') {
        const updated = await adminApi.systemAdmins.update(dialog.user.id, { role: dialog.role, status: dialog.status })
        onFeedback('系统账户已更新', updated.email)
      } else if (dialog.type === 'password') {
        await adminApi.systemAdmins.resetPassword(dialog.user.id, dialog.password)
        onFeedback('密码已重置', dialog.user.email)
      } else if (dialog.type === 'delete') {
        await adminApi.systemAdmins.delete(dialog.user.id)
        onFeedback('系统账户已删除', dialog.user.email)
      }
      setDialog(null)
      await load(appliedFilters, page)
    } finally {
      setSaving(false)
    }
  }

  const canSave = dialog
    ? dialog.type === 'create'
      ? dialog.email.trim().includes('@') && dialog.password.length >= 6
      : dialog.type === 'password'
        ? dialog.password.length >= 6
        : dialog.type === 'delete'
          ? dialog.confirmEmail === dialog.user.email
          : true
    : false

  return (
    <section className={adminPage.stack}>
      <PageHeader title="系统账户" detail="管理独立后台管理员账号、角色、状态和密码重置。" />
      <div className="rounded-xl border border-[rgba(184,135,64,.28)] bg-[rgba(184,135,64,.08)] px-4 py-3 text-sm text-[var(--amber)]">
        敏感操作会写入审计日志；停用、删除和密码重置前请确认目标账户。
      </div>
      <StatusStrip>
        <StatusCell label="账户总数" value={String(summary.total)} />
        <StatusCell label="当前页启用" value={String(summary.active)} />
        <StatusCell label="超级管理员" value={String(summary.superAdmins)} />
        <StatusCell label="当前页停用" value={String(summary.disabled)} />
      </StatusStrip>

      <form onSubmit={applyFilters}>
        <FilterBar
          fields={[
            { key: 'query', label: '搜索', primary: true, control: <input value={filters.query} onChange={(event) => setFilters({ ...filters, query: event.target.value })} placeholder="邮箱关键词" /> },
            { key: 'role', label: '角色', primary: true, control: <select value={filters.role} onChange={(event) => setFilters({ ...filters, role: event.target.value as Filters['role'] })}><option value="">全部</option><option value="super_admin">超级管理员</option><option value="admin">运营管理员</option></select> },
            { key: 'status', label: '状态', control: <select value={filters.status} onChange={(event) => setFilters({ ...filters, status: event.target.value as Filters['status'] })}><option value="">全部</option><option value="active">启用</option><option value="disabled">停用</option></select> },
          ]}
          actions={(
            <>
              <button type="submit" className={cn(adminButton.base, adminButton.primary, adminButton.small, 'gap-1.5')}><FilterIcon className="size-4" /><span>筛选</span></button>
              <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small, 'gap-1.5')} onClick={() => { setFilters(initialFilters); setAppliedFilters(initialFilters); setPage(1) }}><XIcon className="size-4" /><span>清空</span></button>
            </>
          )}
        />
      </form>

      {loading ? <LoadingBlock label="正在载入系统账户" /> : error ? <ErrorBlock message={error} onRetry={() => void load()} /> : (
        <ListPage
          actions={<button type="button" className={cn(adminButton.base, adminButton.primary)} onClick={() => setDialog({ type: 'create', email: '', password: '', role: 'admin', status: 'active' })}>创建账户</button>}
          pagination={<Pager page={page} pageSize={pageSize} total={total} onChange={setPage} onPageSizeChange={(size) => { setPageSize(size); setPage(1) }} />}
        >
          {!rows.length ? <EmptyBlock title="暂无系统账户" detail="调整筛选条件，或创建新的后台管理员账户。" /> : (
            <DataTable
              columns={systemUserColumns(session, setDialog)}
              rows={rows}
              rowKey={(user) => String(user.id)}
            />
          )}
        </ListPage>
      )}

      {dialog ? (
        <Modal
          title={dialogTitle(dialog)}
          detail={dialogDetail(dialog)}
          onClose={() => setDialog(null)}
          footer={<>
            <button type="button" className={cn(adminButton.base, adminButton.ghost)} onClick={() => setDialog(null)}>取消</button>
            <button type="button" className={cn(adminButton.base, dialog.type === 'delete' ? adminButton.danger : adminButton.primary)} disabled={saving || !canSave} onClick={() => void saveDialog()}>{saving ? '保存中' : '确认'}</button>
          </>}
        >
          <DialogForm dialog={dialog} setDialog={setDialog} />
        </Modal>
      ) : null}
    </section>
  )
}

function DialogForm({ dialog, setDialog }: { dialog: DialogState; setDialog: (next: DialogState) => void }) {
  if (dialog.type === 'create') {
    return (
      <div className={adminPage.formGrid}>
        <Field label="邮箱"><input value={dialog.email} onChange={(event) => setDialog({ ...dialog, email: event.target.value })} /></Field>
        <Field label="初始密码" hint="至少 6 位"><input type="password" value={dialog.password} onChange={(event) => setDialog({ ...dialog, password: event.target.value })} /></Field>
        <RoleField value={dialog.role} onChange={(role) => setDialog({ ...dialog, role })} />
        <StatusField value={dialog.status} onChange={(status) => setDialog({ ...dialog, status })} />
      </div>
    )
  }
  if (dialog.type === 'edit') {
    return (
      <div className={adminPage.formGrid}>
        <RoleField value={dialog.role} onChange={(role) => setDialog({ ...dialog, role })} />
        <StatusField value={dialog.status} onChange={(status) => setDialog({ ...dialog, status })} />
      </div>
    )
  }
  if (dialog.type === 'password') {
    return <Field label="新密码" hint="至少 6 位"><input type="password" value={dialog.password} onChange={(event) => setDialog({ ...dialog, password: event.target.value })} /></Field>
  }
  return <Field label="输入邮箱确认删除"><input value={dialog.confirmEmail} onChange={(event) => setDialog({ ...dialog, confirmEmail: event.target.value })} placeholder={dialog.user.email} /></Field>
}

function RoleField({ value, onChange }: { value: AdminRole; onChange: (role: AdminRole) => void }) {
  return (
    <Field label="角色">
      <select value={value} onChange={(event) => onChange(event.target.value as AdminRole)}>
        <option value="admin">运营管理员</option>
        <option value="super_admin">超级管理员</option>
      </select>
    </Field>
  )
}

function StatusField({ value, onChange }: { value: 'active' | 'disabled'; onChange: (status: 'active' | 'disabled') => void }) {
  return (
    <Field label="状态">
      <select value={value} onChange={(event) => onChange(event.target.value as 'active' | 'disabled')}>
        <option value="active">启用</option>
        <option value="disabled">停用</option>
      </select>
    </Field>
  )
}

function systemUserColumns(
  session: AdminSession,
  setDialog: (dialog: DialogState) => void,
): ColumnDef<SystemAdminUser>[] {
  return [
    {
      key: 'user',
      title: '用户信息',
      width: 'minmax(220px,2fr)',
      render: (user) => (
        <div className={systemUserClasses.identity}>
          <span className={systemUserClasses.avatar}>{user.email.slice(0, 1).toUpperCase()}</span>
          <span className="min-w-0">
            <span className={systemUserClasses.title}>{displayAdminName(user)}</span>
            <span className={systemUserClasses.detail}>{user.email} · ID {user.id}</span>
          </span>
        </div>
      ),
    },
    {
      key: 'role',
      title: '角色权限',
      width: 'minmax(120px,1fr)',
      render: (user) => (
        <span className={systemUserClasses.role}>
          <span className={systemUserClasses.roleName}>{roleLabel(user.role)}</span>
          <span className={systemUserClasses.roleHint}>{rolePermissionHint(user.role)}</span>
        </span>
      ),
    },
    {
      key: 'last_login',
      title: '最后登录',
      width: 'minmax(140px,1fr)',
      render: (user) => (
        <span className="flex flex-col gap-1">
          <span className={systemUserClasses.time}>{formatDateTime(user.updated_at)}</span>
          <span className={systemUserClasses.detail}>创建 {formatDateTime(user.created_at)}</span>
        </span>
      ),
    },
    {
      key: 'status',
      title: '状态',
      width: 'minmax(80px,0.7fr)',
      render: (user) => <Badge tone={user.status === 'active' ? 'success' : 'warning'}>{statusLabel(user.status)}</Badge>,
    },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(180px,1.5fr)',
      align: 'right',
      render: (user) => {
        const isSelf = String(user.id) === String(session.admin_id)
        return (
          <div className={systemUserClasses.actions}>
            {isSelf ? <Badge>当前账户</Badge> : null}
            <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={() => setDialog({ type: 'password', user, password: '' })}>重置密码</button>
            <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={() => setDialog({ type: 'edit', user, role: normalizeRole(user.role), status: normalizeStatus(user.status) })}>编辑</button>
            <button type="button" className={cn(adminButton.base, adminButton.danger, adminButton.small)} disabled={isSelf} onClick={() => setDialog({ type: 'delete', user, confirmEmail: '' })}>删除</button>
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
  return role === 'super_admin' ? '完整控制台权限' : role === 'admin' ? '运营工作台权限' : '自定义权限集合'
}

function displayAdminName(user: SystemAdminUser) {
  const name = user.email.split('@')[0]?.trim()
  return name || user.email
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

function dialogTitle(dialog: DialogState) {
  if (dialog.type === 'create') return '创建系统账户'
  if (dialog.type === 'edit') return '编辑系统账户'
  if (dialog.type === 'password') return '重置系统账户密码'
  return '删除系统账户'
}

function dialogDetail(dialog: DialogState) {
  if (dialog.type === 'create') return '新账户将使用独立后台登录体系。'
  if (dialog.type === 'edit') return `正在修改 ${dialog.user.email}；停用账户会立即阻止其继续登录。`
  if (dialog.type === 'password') return dialog.user.email
  return `删除 ${dialog.user.email} 前需要输入完整邮箱确认。`
}

function formatDateTime(value?: string | null) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}
