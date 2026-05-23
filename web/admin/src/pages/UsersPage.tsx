import { FormEvent, useEffect, useMemo, useState } from 'react'
import type { AdminUser, AdminUserCreateRequest, UserGroup } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, Field, LoadingBlock, Modal, PageHeader } from '../components'

const pageSize = 20

const userStatusLabel: Record<string, string> = {
  active: '正常',
  disabled: '禁用',
  pending: '待验证',
  closed: '已关闭',
}

type UserAction =
  | { type: 'create'; draft: AdminUserCreateRequest }
  | { type: 'status'; user: AdminUser; status: string }
  | { type: 'group'; user: AdminUser; group: string }
  | { type: 'points'; user: AdminUser; changePoints: string; reason: string }
  | { type: 'limits'; user: AdminUser; rpmLimit: string; concurrencyLimit: string }
  | { type: 'password'; user: AdminUser; password: string }
  | { type: 'delete'; user: AdminUser; confirmEmail: string }

export function UsersPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<AdminUser[]>([])
  const [groups, setGroups] = useState<UserGroup[]>([])
  const [query, setQuery] = useState('')
  const [appliedQuery, setAppliedQuery] = useState('')
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [action, setAction] = useState<UserAction | null>(null)

  const load = async (nextQuery = appliedQuery, nextPage = page) => {
    setLoading(true)
    setError(null)
    try {
      const [userPage, userGroups] = await Promise.all([
        adminApi.listUsersPage(nextQuery, nextPage, pageSize),
        groups.length ? Promise.resolve(groups) : adminApi.listUserGroups(),
      ])
      setRows(userPage.items)
      setTotal(userPage.total)
      setGroups(userGroups)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '用户载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load(appliedQuery, page)
  }, [appliedQuery, page])

  const totals = useMemo(() => ({
    active: rows.filter((row) => row.status === 'active').length,
    disabled: rows.filter((row) => row.status === 'disabled').length,
    pending: rows.filter((row) => row.status === 'pending').length,
  }), [rows])

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
        onFeedback('用户状态已更新', `${updated.display_name} · ${userStatusLabel[updated.status] ?? updated.status}`)
      }
      if (action.type === 'group') {
        await adminApi.assignUserGroup(action.user.id, action.group)
        onFeedback('用户分组已更新', `${action.user.display_name} · ${action.group}`)
      }
      if (action.type === 'points') {
        await adminApi.adjustUserPoints(action.user.id, action.changePoints, action.reason, crypto.randomUUID())
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
      await load(appliedQuery, page)
    } finally {
      setSaving(false)
    }
  }

  const updateStatus = async (user: AdminUser, status: string) => {
    setSaving(true)
    try {
      const updated = await adminApi.updateUserStatus(user.id, status)
      onFeedback('用户状态已更新', `${updated.display_name} · ${userStatusLabel[updated.status] ?? updated.status}`)
      await load(appliedQuery, page)
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <LoadingBlock label="载入用户列表" />
  if (error) return <ErrorBlock message={error} onRetry={() => void load()} />

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="Users"
        title="用户管理"
        detail="用户列表只展示状态，变更动作统一从操作列进入。"
        actions={<button className="btn primary" type="button" onClick={() => setAction({ type: 'create', draft: { email: '', nickname: '', status: 'active', user_group_code: groups[0]?.group_code ?? 'basic', password: '', rpm_limit: 0, concurrency_limit: 0, default_locale: 'zh-CN', theme: 'system' } })}>新增用户</button>}
      />
      <section className="ops-status-strip compact-strip">
        <div className="status-cell"><label>正常</label><strong>{totals.active}</strong></div>
        <div className="status-cell"><label>待验证</label><strong>{totals.pending}</strong></div>
        <div className="status-cell"><label>禁用</label><strong>{totals.disabled}</strong></div>
        <div className="status-cell"><label>总数</label><strong>{total}</strong></div>
      </section>
      <section className="pg-admin-card ops-surface full-main">
        <section className="main-lane table-lane no-divider">
          <div className="list-toolbar">
            <form className="search-form" onSubmit={(event) => { event.preventDefault(); setPage(1); setAppliedQuery(query) }}>
              <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索邮箱 / 昵称" />
              <button className="btn" type="submit">搜索</button>
              <button className="ghost" type="button" onClick={() => { setQuery(''); setAppliedQuery(''); setPage(1) }}>清空</button>
            </form>
            <span>第 {page} 页 / 共 {total} 条</span>
          </div>
          {!rows.length ? <EmptyBlock title="没有匹配用户" detail="尝试换一个关键词。" /> : (
            <>
              <div className="table-head users-grid"><span>用户</span><span>状态</span><span>分组</span><span>积分</span><span>最后活跃</span><span>操作</span></div>
              {rows.map((row) => (
                <div key={row.id} className="table-row users-grid">
                  <div><strong>{row.display_name}</strong><p>{row.email} · {row.id}</p></div>
                  <Badge tone={row.status === 'active' ? 'success' : row.status === 'pending' ? 'warning' : 'danger'}>{userStatusLabel[row.status] ?? row.status}</Badge>
                  <span>{row.group}</span>
                  <code>{row.balance}</code>
                  <span>{row.last_seen_at || row.updated_at || '-'}</span>
                  <div className="row-actions buttons">
                    {row.status === 'active' ? <button className="btn small danger" type="button" disabled={saving} onClick={() => void updateStatus(row, 'disabled')}>禁用</button> : null}
                    {row.status === 'disabled' ? <button className="btn small success" type="button" disabled={saving} onClick={() => void updateStatus(row, 'active')}>启用</button> : null}
                    <button className="ghost small" type="button" onClick={() => setAction({ type: 'group', user: row, group: row.group })}>分组</button>
                    <button className="ghost small" type="button" onClick={() => setAction({ type: 'points', user: row, changePoints: '0.00000', reason: 'manual admin adjustment' })}>积分</button>
                    <button className="ghost small" type="button" onClick={() => setAction({ type: 'limits', user: row, rpmLimit: String(row.rpm_limit ?? 0), concurrencyLimit: String(row.concurrency_limit ?? 0) })}>限额</button>
                    <button className="ghost small" type="button" onClick={() => setAction({ type: 'password', user: row, password: '' })}>密码</button>
                    <button className="ghost small danger-text" type="button" onClick={() => setAction({ type: 'delete', user: row, confirmEmail: '' })}>删除</button>
                  </div>
                </div>
              ))}
              <div className="pagination-row">
                <span>每页 {pageSize} 条</span>
                <div className="row-actions buttons">
                  <button className="ghost small" type="button" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>上一页</button>
                  <button className="ghost small" type="button" disabled={page * pageSize >= total} onClick={() => setPage((value) => value + 1)}>下一页</button>
                </div>
              </div>
            </>
          )}
        </section>
      </section>
      {action ? (
        <Modal title={actionTitle(action)} detail={action.type === 'create' ? '管理后台创建用户不需要邮箱验证码。' : action.user.email} onClose={() => setAction(null)} footer={<><button type="button" className="ghost" disabled={saving} onClick={() => setAction(null)}>取消</button><button type="button" className="btn primary" disabled={saving || !canSave(action)} onClick={() => void saveAction()}>{saving ? '保存中...' : '保存'}</button></>}>
          {renderActionForm(action, groups, setAction)}
        </Modal>
      ) : null}
    </section>
  )
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

function canSave(action: UserAction) {
  if (action.type === 'create') return Boolean(action.draft.email?.trim())
  if (action.type === 'password') return action.password.length >= 8
  if (action.type === 'points') return Boolean(action.changePoints.trim() && action.reason.trim())
  if (action.type === 'delete') return action.confirmEmail.trim() === action.user.email
  return true
}

function renderActionForm(action: UserAction, groups: UserGroup[], setAction: (action: UserAction) => void) {
  if (action.type === 'create') {
    return (
      <form className="form-grid" onSubmit={(event: FormEvent) => event.preventDefault()}>
        <Field label="邮箱"><input value={action.draft.email} onChange={(event) => setAction({ ...action, draft: { ...action.draft, email: event.target.value } })} required /></Field>
        <Field label="昵称"><input value={action.draft.nickname ?? ''} onChange={(event) => setAction({ ...action, draft: { ...action.draft, nickname: event.target.value } })} /></Field>
        <Field label="状态"><select value={action.draft.status ?? 'active'} onChange={(event) => setAction({ ...action, draft: { ...action.draft, status: event.target.value } })}><option value="active">正常</option><option value="pending">待验证</option><option value="disabled">禁用</option></select></Field>
        <Field label="分组"><GroupSelect value={action.draft.user_group_code ?? 'basic'} groups={groups} onChange={(value) => setAction({ ...action, draft: { ...action.draft, user_group_code: value } })} /></Field>
        <Field label="RPM 限额"><input type="number" min="0" value={action.draft.rpm_limit ?? 0} onChange={(event) => setAction({ ...action, draft: { ...action.draft, rpm_limit: Number(event.target.value) } })} /></Field>
        <Field label="并发限额"><input type="number" min="0" value={action.draft.concurrency_limit ?? 0} onChange={(event) => setAction({ ...action, draft: { ...action.draft, concurrency_limit: Number(event.target.value) } })} /></Field>
        <Field label="初始密码"><input type="password" value={action.draft.password ?? ''} onChange={(event) => setAction({ ...action, draft: { ...action.draft, password: event.target.value } })} placeholder="可选，留空则使用验证码登录" /></Field>
      </form>
    )
  }
  if (action.type === 'status') {
    return <Field label="新状态"><select value={action.status} onChange={(event) => setAction({ ...action, status: event.target.value })}><option value="active">正常</option><option value="pending">待验证</option><option value="disabled">禁用</option></select></Field>
  }
  if (action.type === 'delete') {
    return <><p>删除后该用户会从列表隐藏，并撤销登录会话。为避免误操作，请输入用户邮箱确认。</p><Field label="确认邮箱"><input value={action.confirmEmail} onChange={(event) => setAction({ ...action, confirmEmail: event.target.value })} placeholder={action.user.email} /></Field></>
  }
  if (action.type === 'group') {
    return <Field label="新分组"><GroupSelect value={action.group} groups={groups} onChange={(value) => setAction({ ...action, group: value })} /></Field>
  }
  if (action.type === 'points') {
    return <><Field label="变更积分"><input value={action.changePoints} onChange={(event) => setAction({ ...action, changePoints: event.target.value })} /></Field><Field label="原因"><input value={action.reason} onChange={(event) => setAction({ ...action, reason: event.target.value })} /></Field></>
  }
  if (action.type === 'limits') {
    return <div className="form-grid"><Field label="RPM 限额"><input type="number" min="0" value={action.rpmLimit} onChange={(event) => setAction({ ...action, rpmLimit: event.target.value })} /></Field><Field label="并发限额"><input type="number" min="0" value={action.concurrencyLimit} onChange={(event) => setAction({ ...action, concurrencyLimit: event.target.value })} /></Field></div>
  }
  return <Field label="新密码"><input type="password" value={action.password} onChange={(event) => setAction({ ...action, password: event.target.value })} placeholder="至少 8 位" /></Field>
}

function GroupSelect({ value, groups, onChange }: { value: string; groups: UserGroup[]; onChange: (value: string) => void }) {
  const options = groups.length ? groups : [{ group_code: 'basic', group_name: 'basic' }] as UserGroup[]
  return <select value={value} onChange={(event) => onChange(event.target.value)}>{options.map((group) => <option key={group.group_code} value={group.group_code}>{group.group_code}</option>)}</select>
}
