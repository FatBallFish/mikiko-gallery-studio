import { useEffect, useMemo, useState } from 'react'
import type { AuditLog } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, EmptyBlock, ErrorBlock, LoadingBlock, PageHeader } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { FilterBar } from '../ui/dataTable'
import { FilterIcon } from '../ui/listIcons'
import {
  auditActionOptions,
  auditExportFilename,
  auditRowsCSV,
  auditSearchPlaceholder,
  auditSearchText,
  auditTimelineRow,
} from './auditRows'

const auditClasses = {
  timeline: 'grid gap-2',
  item: 'group flex items-center gap-6 rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-5 transition-all hover:border-[var(--border-strong)] hover:bg-[var(--elevated)] max-[720px]:grid max-[720px]:grid-cols-1',
  avatar: 'grid size-10 shrink-0 place-items-center rounded-xl bg-[var(--canvas)] text-xs font-bold text-[var(--muted-strong)]',
  itemMain: 'min-w-0 flex-1',
  itemHead: 'mb-1 flex min-w-0 flex-wrap items-center gap-3',
  actionText: 'text-xs font-black tracking-widest text-[var(--accent)]',
  itemTitle: 'min-w-0 truncate text-sm font-bold text-[var(--text)]',
  dot: 'size-1 rounded-full bg-[var(--border-strong)]',
  itemText: 'm-0 text-xs leading-relaxed text-[var(--muted-strong)]',
  itemSide: 'shrink-0 text-right max-[720px]:text-left',
  itemActor: 'text-xs font-bold text-[var(--soft)]',
  itemDate: 'mt-0.5 text-[10px] text-[var(--muted-strong)]',
}

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
    <section className={adminPage.stack}>
      <PageHeader
        eyebrow="Audit"
        title="审计日志"
        detail="所有关键写操作都会追加审计行，便于回溯配置、价格、路由、审核与用户变更。"
        actions={<button type="button" className={cn(adminButton.base, adminButton.ghost)} onClick={exportVisibleRows} disabled={!visibleRows.length}>导出日志</button>}
      />
      <FilterBar
        fields={[
          { key: 'query', label: '搜索', primary: true, control: <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={auditSearchPlaceholder} /> },
          { key: 'action', label: '动作', primary: true, control: <select value={actionFilter} onChange={(event) => setActionFilter(event.target.value)}>{actionOptions.map((action) => <option key={action.value} value={action.value}>{action.label}</option>)}</select> },
        ]}
        actions={<button type="button" className={cn(adminButton.base, adminButton.primary, adminButton.small, 'gap-1.5')} onClick={() => void load()}><FilterIcon className="size-4" /><span>刷新</span></button>}
      />
      <section className={auditClasses.timeline}>
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
    <article className={auditClasses.item}>
      <div className={auditClasses.avatar}>{item.actorLabel.slice(0, 1).toUpperCase()}</div>
      <div className={auditClasses.itemMain}>
        <div className={auditClasses.itemHead}>
          <span className={auditClasses.actionText}>{item.actionLabel}</span>
          <span className={auditClasses.dot} />
          <strong className={auditClasses.itemTitle}>{item.targetLabel}</strong>
          <Badge tone={item.result.tone}>{item.result.label}</Badge>
        </div>
        <p className={auditClasses.itemText}>{item.detailText} · audit_id {item.raw.id}</p>
      </div>
      <div className={auditClasses.itemSide}>
        <div className={auditClasses.itemActor}>{item.actorLabel}</div>
        <div className={auditClasses.itemDate}>{item.createdAtLabel}</div>
      </div>
    </article>
  )
}
