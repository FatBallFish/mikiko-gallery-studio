import { useEffect, useMemo, useRef, useState } from 'react'
import type { AuditLog } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, EmptyBlock, ErrorBlock, InlineFeedback, LoadingBlock, PageHeader, RefreshIconButton } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import type { ColumnDef } from '../ui/dataTable'
import { DataTable, FilterToolbar, ListPage } from '../ui/dataTable'
import { XIcon } from '../ui/listIcons'
import {
  auditActionOptions,
  auditExportFilename,
  auditRowsCSV,
  auditSearchPlaceholder,
  auditSearchText,
  auditTimelineRow,
} from './auditRows'
import { createLatestListRequestGuard } from './listRefresh'

const auditClasses = {
  identity: 'flex min-w-0 items-center gap-3',
  avatar: 'grid size-9 shrink-0 place-items-center rounded-lg bg-[var(--canvas)] text-xs font-semibold text-[var(--muted)]',
  stack: 'grid min-w-0 gap-1',
  title: 'truncate font-semibold text-[var(--fg)]',
  secondary: 'truncate text-xs text-[var(--soft)]',
  detail: 'max-w-[520px] text-xs leading-5 text-[var(--muted)] [overflow-wrap:anywhere]',
}

export function AuditPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<AuditLog[]>([])
  const [query, setQuery] = useState('')
  const [actionFilter, setActionFilter] = useState('all')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const requestGuard = useRef(createLatestListRequestGuard()).current

  const load = async () => {
    const request = requestGuard.begin()
    setLoading(true)
    setError(null)
    try {
      const nextRows = await adminApi.listAudit({ page: 1, page_size: 100 })
      if (!requestGuard.isCurrent(request)) return
      setRows(nextRows)
    } catch (caught) {
      if (!requestGuard.isCurrent(request)) return
      setError(caught instanceof Error ? caught.message : '审计日志载入失败')
    } finally {
      if (!requestGuard.isCurrent(request)) return
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    return () => requestGuard.invalidate()
  }, [])

  const actionOptions = useMemo(() => auditActionOptions(rows), [rows])
  const visibleRows = useMemo(() => rows.filter((row) => {
    const matchesAction = actionFilter === 'all' || row.action === actionFilter
    const haystack = auditSearchText(row)
    return matchesAction && (!query.trim() || haystack.includes(query.trim().toLowerCase()))
  }), [actionFilter, query, rows])

  const clearFilters = () => {
    setQuery('')
    setActionFilter('all')
  }

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

  if (loading && !rows.length) return <LoadingBlock label="载入审计日志" />
  if (error && !rows.length) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className={adminPage.stack}>
      <PageHeader
        eyebrow="Audit"
        title="审计日志"
        detail="查询关键写操作及其操作人、目标、结果与来源信息。"
        primaryAction={<button type="button" className={cn(adminButton.base, adminButton.primary)} onClick={exportVisibleRows} disabled={!visibleRows.length}>导出日志</button>}
        secondaryActions={<RefreshIconButton label="刷新审计日志" refreshing={loading} onClick={() => void load()} />}
      />
      {error ? <InlineFeedback tone="danger" message={`审计日志刷新失败：${error}`} /> : null}
      <ListPage
        filters={(
          <FilterToolbar
            fields={[
              { key: 'query', label: '搜索', primary: true, minWidth: '240px', maxWidth: '420px', control: <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={auditSearchPlaceholder} /> },
              { key: 'action', label: '动作', primary: true, control: <select value={actionFilter} onChange={(event) => setActionFilter(event.target.value)}>{actionOptions.map((action) => <option key={action.value} value={action.value}>{action.label}</option>)}</select> },
            ]}
            actions={(
              <>
                <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={clearFilters}><XIcon className="size-4" /><span>清空</span></button>
              </>
            )}
            resultSummary={`共载入 ${rows.length} 条审计 · 当前显示 ${visibleRows.length} 条`}
          />
        )}
      >
        <DataTable
          columns={auditColumns()}
          rows={visibleRows}
          rowKey={(row) => row.id}
          empty={<EmptyBlock title="没有匹配审计" detail="放宽关键词或动作筛选。" />}
        />
      </ListPage>
    </section>
  )
}

function auditColumns(): ColumnDef<AuditLog>[] {
  return [
    {
      key: 'action',
      title: '动作与对象',
      width: 'minmax(230px,1.7fr)',
      render: (row) => {
        const item = auditTimelineRow(row)
        return <span className={auditClasses.stack}><span className={auditClasses.title}>{item.actionLabel}</span><span className={auditClasses.secondary}>{item.targetLabel} · audit_id {row.id}</span></span>
      },
    },
    {
      key: 'actor',
      title: '操作人',
      width: 'minmax(180px,1.2fr)',
      render: (row) => {
        const item = auditTimelineRow(row)
        return <span className={auditClasses.identity}><span className={auditClasses.avatar}>{item.actorLabel.slice(0, 1).toUpperCase()}</span><span className={auditClasses.stack}><span className={auditClasses.title}>{item.actorLabel}</span><span className={auditClasses.secondary}>{row.ip_addr || '未记录 IP'}</span></span></span>
      },
    },
    {
      key: 'detail',
      title: '操作详情',
      width: 'minmax(280px,2.4fr)',
      render: (row) => <span className={auditClasses.detail}>{auditTimelineRow(row).detailText}</span>,
    },
    {
      key: 'result',
      title: '结果',
      width: 'minmax(90px,.7fr)',
      render: (row) => {
        const result = auditTimelineRow(row).result
        return <Badge tone={result.tone}>{result.label}</Badge>
      },
    },
    {
      key: 'time',
      title: '发生时间',
      width: 'minmax(150px,1fr)',
      kind: 'code',
      render: (row) => auditTimelineRow(row).createdAtLabel,
    },
  ]
}
