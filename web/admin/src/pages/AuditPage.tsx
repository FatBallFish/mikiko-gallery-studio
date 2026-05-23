import { useEffect, useMemo, useState } from 'react'
import type { AuditLog } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader } from '../components'

const commonActions = [
  'admin.login',
  'admin.logout',
  'user.create',
  'user.delete',
  'user.status_update',
  'user.group_update',
  'user.points_adjust',
  'user.password_reset',
  'user.limits_update',
  'config.update',
  'redeem.create',
  'redeem.status_update',
  'model_provider.create',
  'provider_model.create',
  'model_route.update',
]

export function AuditPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<AuditLog[]>([])
  const [query, setQuery] = useState('')
  const [actionFilter, setActionFilter] = useState('all')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      setRows(await adminApi.listAudit({ page: 1, page_size: 100 }))
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '审计日志载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const actionOptions = useMemo(() => ['all', ...Array.from(new Set([...commonActions, ...rows.map((row) => row.action).filter(Boolean)]))], [rows])
  const visibleRows = useMemo(() => rows.filter((row) => {
    const matchesAction = actionFilter === 'all' || row.action === actionFilter
    const haystack = `${row.actor} ${row.action} ${row.target} ${row.detail}`.toLowerCase()
    return matchesAction && (!query || haystack.includes(query.toLowerCase()))
  }), [actionFilter, query, rows])

  if (loading) return <LoadingBlock label="载入审计日志" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="Audit"
        title="审计日志"
        detail="所有关键写操作都会追加审计行，便于回溯配置、价格、路由、审核与用户变更。"
        actions={<button type="button" className="ghost" onClick={() => onFeedback('审计导出已生成', `${visibleRows.length} 行日志已准备下载`)}>导出日志</button>}
      />
      <section className="pg-admin-card filter-band">
        <form className="filter-row" onSubmit={(event) => event.preventDefault()}>
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索 actor / action / target" />
          <select value={actionFilter} onChange={(event) => setActionFilter(event.target.value)}>
            {actionOptions.map((action) => <option key={action} value={action}>{action === 'all' ? '全部动作' : action}</option>)}
          </select>
          <button type="button" className="btn" onClick={() => void load()}>刷新</button>
        </form>
      </section>
      <section className="pg-admin-card timeline-surface">
        {!visibleRows.length ? <EmptyBlock title="没有匹配审计" detail="放宽关键词或动作筛选。" /> : visibleRows.map((row) => (
          <article key={row.id} className="timeline-item">
            <div><Badge tone={row.actor_type === 'system' ? 'neutral' : 'primary'}>{row.action || '-'}</Badge><strong>{row.target || '-'}</strong><span>{row.created_at}</span></div>
            <p>{formatAuditDetail(row)}</p>
            <small>{row.actor || '-'} · {row.result ?? 'success'} · audit_id {row.id}</small>
          </article>
        ))}
      </section>
    </section>
  )
}

function formatAuditDetail(row: AuditLog) {
  if (row.detail && row.detail !== row.result) return row.detail
  const bits = [
    row.actor_type ? `actor=${row.actor_type}:${row.actor_id ?? ''}` : '',
    row.target_type ? `target=${row.target_type}:${row.target_id ?? ''}` : '',
    row.ip_addr ? `ip=${row.ip_addr}` : '',
  ].filter(Boolean)
  return bits.join(' · ') || row.result || 'success'
}
