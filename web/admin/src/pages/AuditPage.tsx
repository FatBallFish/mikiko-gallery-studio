import { useEffect, useMemo, useState } from 'react'
import type { AuditLog } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader } from '../components'
import {
  auditActionOptions,
  auditExportFilename,
  auditRowsCSV,
  auditSearchPlaceholder,
  auditSearchText,
  auditTimelineRow,
} from './auditRows'

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

  const actionOptions = useMemo(() => auditActionOptions(rows), [rows])
  const visibleRows = useMemo(() => rows.filter((row) => {
    const matchesAction = actionFilter === 'all' || row.action === actionFilter
    const haystack = auditSearchText(row)
    return matchesAction && (!query || haystack.includes(query.toLowerCase()))
  }), [actionFilter, query, rows])

  const exportVisibleRows = () => {
    if (!visibleRows.length) {
      onFeedback('没有可导出的审计日志', '放宽关键词或动作筛选后再导出。')
      return
    }
    const blob = new Blob([`\uFEFF${auditRowsCSV(visibleRows)}`], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = auditExportFilename()
    anchor.click()
    URL.revokeObjectURL(url)
    onFeedback('审计日志已导出', `${visibleRows.length} 行 CSV 已下载`)
  }

  if (loading) return <LoadingBlock label="载入审计日志" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="Audit"
        title="审计日志"
        detail="所有关键写操作都会追加审计行，便于回溯配置、价格、路由、审核与用户变更。"
        actions={<button type="button" className="ghost" onClick={exportVisibleRows} disabled={!visibleRows.length}>导出日志</button>}
      />
      <section className="pg-admin-card filter-band">
        <form className="filter-row" onSubmit={(event) => event.preventDefault()}>
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={auditSearchPlaceholder} />
          <select value={actionFilter} onChange={(event) => setActionFilter(event.target.value)}>
            {actionOptions.map((action) => <option key={action.value} value={action.value}>{action.label}</option>)}
          </select>
          <button type="button" className="btn" onClick={() => void load()}>刷新</button>
        </form>
      </section>
      <section className="pg-admin-card timeline-surface">
        {!visibleRows.length ? <EmptyBlock title="没有匹配审计" detail="放宽关键词或动作筛选。" /> : visibleRows.map((row) => (
          <AuditTimelineItem key={row.id} row={row} />
        ))}
      </section>
    </section>
  )
}

function AuditTimelineItem({ row }: { row: AuditLog }) {
  const item = auditTimelineRow(row)
  return (
    <article className="timeline-item">
      <div>
        <Badge tone={item.actorTone}>{item.actionLabel}</Badge>
        <strong>{item.targetLabel}</strong>
        <span>{item.createdAtLabel}</span>
      </div>
      <p>{item.detailText}</p>
      <small>{item.actorLabel} · <Badge tone={item.result.tone}>{item.result.label}</Badge> · audit_id {item.raw.id}</small>
    </article>
  )
}
