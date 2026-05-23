import { useEffect, useState } from 'react'
import type { AdminMetric, AdminUser, AuditLog, ProviderHealth } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, MetricGrid } from '../components'

type DashboardData = {
  metrics: AdminMetric[]
  providers: ProviderHealth[]
  queue: Array<{ item: string; count: string; detail: string }>
  audit: AuditLog[]
  users: AdminUser[]
}

const statusLabel: Record<AdminUser['status'], string> = {
  active: '正常',
  disabled: '已禁用',
  pending: '待验证',
}

const statusTone: Record<AdminUser['status'], 'success' | 'danger' | 'warning'> = {
  active: 'success',
  disabled: 'danger',
  pending: 'warning',
}

export function OverviewPage() {
  const [data, setData] = useState<DashboardData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const [dashboard, users] = await Promise.all([adminApi.dashboard(), adminApi.listUsers()])
      setData({
        ...dashboard,
        users: users
          .slice()
          .sort((left, right) => right.created_at.localeCompare(left.created_at))
          .slice(0, 5),
      })
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '无法载入总览')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  if (loading) return <LoadingBlock />
  if (error) return <ErrorBlock message={error} onRetry={load} />
  if (!data) return <EmptyBlock title="暂无总览数据" detail="后台接口未返回指标。" />

  return (
    <section className="content page-stack">
      <header className="header page-header">
        <div>
          <h1>运营总览</h1>
          <p>核心指标、注册用户与后台运营入口集中在同一条扫描路径里。</p>
        </div>
      </header>
      <MetricGrid metrics={data.metrics} />

      <section className="card pg-admin-card ops-surface full-main">
        <section className="main-lane table-lane no-divider">
          <div className="card-header lane-head compact">
            <div className="card-title">最新注册用户</div>
            <a className="btn" href="#/users">查看全部</a>
          </div>
          <table className="ds-table" aria-label="最新注册用户">
            <thead>
              <tr>
                <th>用户</th>
                <th>邮箱</th>
                <th>积分余额</th>
                <th>状态</th>
                <th>注册时间</th>
                <th style={{ textAlign: 'right' }}>操作</th>
              </tr>
            </thead>
            <tbody>
              {data.users.map((user) => (
                <tr key={user.id}>
                  <td><strong>{user.display_name}</strong></td>
                  <td>{user.email}</td>
                  <td><code>{user.balance}</code></td>
                  <td><Badge tone={statusTone[user.status]}>{statusLabel[user.status]}</Badge></td>
                  <td>{user.created_at}</td>
                  <td style={{ textAlign: 'right' }}><a className="btn small" href="#/users">管理</a></td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      </section>
    </section>
  )
}
