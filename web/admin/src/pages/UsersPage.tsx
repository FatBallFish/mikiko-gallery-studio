import { useEffect, useMemo, useState } from 'react'
import type { AdminUser } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader } from '../components'

const userStatusLabel: Record<AdminUser['status'], string> = {
  active: '正常',
  disabled: '禁用',
  pending: '待验证',
}

export function UsersPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<AdminUser[]>([])
  const [query, setQuery] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [savingId, setSavingId] = useState<string | null>(null)

  const load = async (nextQuery = query) => {
    setLoading(true)
    setError(null)
    try {
      setRows(await adminApi.listUsers(nextQuery))
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '用户载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load('')
  }, [])

  const totals = useMemo(() => ({
    active: rows.filter((row) => row.status === 'active').length,
    disabled: rows.filter((row) => row.status === 'disabled').length,
    pending: rows.filter((row) => row.status === 'pending').length,
  }), [rows])

  const updateUser = async (user: AdminUser, patch: Partial<AdminUser>) => {
    setSavingId(user.id)
    try {
      if (patch.status) await adminApi.updateUserStatus(user.id, patch.status)
      if (patch.group) await adminApi.assignUserGroup(user.id, patch.group)
      if (patch.balance && patch.balance !== user.balance) await adminApi.adjustUserPoints(user.id, patch.balance, 'manual admin adjustment', crypto.randomUUID())
      const updated = await adminApi.getUser(user.id)
      setRows((current) => current.map((item) => item.id === user.id ? updated : item))
      onFeedback('用户状态已更新', `${updated.display_name} · ${userStatusLabel[updated.status]}`)
    } finally {
      setSavingId(null)
    }
  }

  if (loading) return <LoadingBlock label="载入用户列表" />
  if (error) return <ErrorBlock message={error} onRetry={() => void load()} />

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="Users"
        title="用户管理"
        detail="搜索、状态、分组与积分调整保持在同一张高密度表里。"
        actions={<form className="search-form" onSubmit={(event) => { event.preventDefault(); void load(query) }}><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索邮箱 / 昵称 / ID" /><button className="btn" type="submit">搜索</button></form>}
      />
      <section className="ops-status-strip compact-strip">
        <div className="status-cell"><label>正常</label><strong>{totals.active}</strong></div>
        <div className="status-cell"><label>待验证</label><strong>{totals.pending}</strong></div>
        <div className="status-cell"><label>禁用</label><strong>{totals.disabled}</strong></div>
        <div className="status-cell"><label>筛选</label><strong>{query || '全部用户'}</strong></div>
      </section>
      <section className="pg-admin-card ops-surface full-main">
        <section className="main-lane table-lane no-divider">
          {!rows.length ? <EmptyBlock title="没有匹配用户" detail="尝试换一个关键词。" /> : (
            <>
              <div className="table-head users-grid"><span>用户</span><span>状态</span><span>分组</span><span>积分</span><span>最后活跃</span><span>操作</span></div>
              {rows.map((row) => (
                <div key={row.id} className="table-row users-grid editable-row">
                  <div><strong>{row.display_name}</strong><p>{row.email} · {row.id}</p></div>
                  <select value={row.status} onChange={(event) => void updateUser(row, { status: event.target.value as AdminUser['status'] })} disabled={savingId === row.id}>
                    <option value="active">正常</option><option value="pending">待验证</option><option value="disabled">禁用</option>
                  </select>
                  <select value={row.group} onChange={(event) => void updateUser(row, { group: event.target.value })} disabled={savingId === row.id}>
                    <option>DEFAULT</option><option>CREATOR</option><option>PARTNER</option><option>RISK</option>
                  </select>
                  <input value={row.balance} onChange={(event) => setRows((current) => current.map((item) => item.id === row.id ? { ...item, balance: event.target.value } : item))} />
                  <span>{row.last_seen_at}</span>
                  <div className="row-actions buttons"><Badge tone={row.status === 'active' ? 'success' : row.status === 'pending' ? 'warning' : 'danger'}>{userStatusLabel[row.status]}</Badge><button className="btn small" type="button" onClick={() => void updateUser(row, { balance: row.balance })} disabled={savingId === row.id}>{savingId === row.id ? '保存中' : '保存积分'}</button></div>
                </div>
              ))}
            </>
          )}
        </section>
      </section>
    </section>
  )
}
