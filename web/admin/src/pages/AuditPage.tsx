import { useEffect, useMemo, useRef, useState } from 'react'
import type { AuditLog } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, Drawer, EmptyBlock, ErrorBlock, InlineFeedback, LoadingBlock, PageHeader, RefreshIconButton } from '../components'
import type { ColumnDef } from '../ui/dataTable'
import { DataTable, FilterToolbar, ListPage, Pager } from '../ui/dataTable'
import { XIcon } from '../ui/listIcons'
import {
  auditActionOptions,
  auditExportFilename,
  auditResultBadge,
  auditRowsCSV,
  auditSearchPlaceholder,
  auditSearchText,
  auditTimelineRow,
} from './auditRows'
import { createLatestListRequestGuard } from './listRefresh'
import './list-redesign.css'

export function AuditPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<AuditLog[]>([])
  const [query, setQuery] = useState('')
  const [actionFilter, setActionFilter] = useState('all')
  const [resultFilter, setResultFilter] = useState('all')
  const [selected, setSelected] = useState<AuditLog | null>(null)
  const [copyNotice, setCopyNotice] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
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
  const resultOptions = useMemo(() => {
    const values = Array.from(new Set(rows.map((row) => (row.result || 'success').trim()).filter(Boolean)))
    return [{ value: 'all', label: '全部结果' }, ...values.map((value) => ({ value, label: auditResultBadge(value).label }))]
  }, [rows])
  const visibleRows = useMemo(() => rows.filter((row) => {
    const matchesAction = actionFilter === 'all' || row.action === actionFilter
    const matchesResult = resultFilter === 'all' || (row.result || 'success') === resultFilter
    const haystack = auditSearchText(row)
    return matchesAction && matchesResult && (!query.trim() || haystack.includes(query.trim().toLowerCase()))
  }), [actionFilter, query, resultFilter, rows])
  const pagedRows = useMemo(() => visibleRows.slice((page - 1) * pageSize, page * pageSize), [page, pageSize, visibleRows])

  useEffect(() => { setPage(1) }, [query, actionFilter, resultFilter])

  const clearFilters = () => {
    setQuery('')
    setActionFilter('all')
    setResultFilter('all')
  }

  const exportVisibleRows = () => {
    if (!visibleRows.length) {
      onFeedback('没有可导出的审计日志', '放宽关键词、动作或结果筛选后再导出。')
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

  const copyAuditID = async (id: string) => {
    try {
      await navigator.clipboard.writeText(String(id))
      setCopyNotice('审计 ID 已复制')
    } catch {
      setCopyNotice('复制失败，请手动选择文本。')
    }
  }

  if (loading && !rows.length) return <LoadingBlock label="载入审计日志" />
  if (error && !rows.length) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className="a-page-stack">
      <PageHeader
        eyebrow="Audit Trail"
        title="审计日志"
        description="管理员操作与敏感变更。操作人、对象与结果直接展示，详情在抽屉中查看完整上下文。"
        primaryAction={<button type="button" className="a-btn a-btn--sm a-btn--primary" onClick={exportVisibleRows} disabled={!visibleRows.length}>导出日志</button>}
        secondaryActions={<RefreshIconButton label="刷新审计日志" refreshing={loading} onClick={() => void load()} />}
      />
      {error ? <InlineFeedback tone="danger" message={`审计日志刷新失败：${error}`} /> : null}
      {loading && rows.length ? <InlineFeedback tone="neutral" message="正在刷新审计日志，当前数据会保留到新结果返回。" /> : null}
      <ListPage
        resultSummary={<><h2>操作记录</h2><p>按发生时间倒序；详情包含来源 IP、User Agent 与上下文。</p></>}
        actions={<span className="a-hint">保留 180 天</span>}
        filters={(
          <form onSubmit={(event) => event.preventDefault()}>
            <FilterToolbar
              fields={[
                { key: 'query', label: '搜索', primary: true, minWidth: '240px', maxWidth: '420px', control: <input className="a-input" value={query} onChange={(event) => setQuery(event.target.value)} placeholder={auditSearchPlaceholder} /> },
                { key: 'action', label: '动作', primary: true, control: <select className="a-select" value={actionFilter} onChange={(event) => setActionFilter(event.target.value)}>{actionOptions.map((action) => <option key={action.value} value={action.value}>{action.label}</option>)}</select> },
                { key: 'result', label: '结果', primary: true, control: <select className="a-select" value={resultFilter} onChange={(event) => setResultFilter(event.target.value)}>{resultOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select> },
              ]}
              actions={<button type="button" className="a-btn a-btn--sm a-btn--quiet" onClick={clearFilters}><XIcon className="size-4" /><span>清空筛选</span></button>}
              resultSummary={`共载入 ${rows.length} 条审计 · 匹配 ${visibleRows.length} 条 · 保留 180 天`}
            />
          </form>
        )}
        pagination={<Pager page={page} pageSize={pageSize} total={visibleRows.length} onChange={setPage} onPageSizeChange={(size) => { setPageSize(size); setPage(1) }} />}
      >
        <DataTable
          columns={auditColumns((row) => { setSelected(row); setCopyNotice('') })}
          rows={pagedRows}
          rowKey={(row) => row.id}
          empty={<EmptyBlock title="没有匹配审计" detail="放宽关键词、动作或结果筛选。" />}
          minWidth={1100}
        />
      </ListPage>

      {selected ? (
        <Drawer
          title="审计详情"
          description={`${selected.id} · ${auditTimelineRow(selected).actionLabel} · ${auditTimelineRow(selected).createdAtLabel}`}
          headerAside={<Badge tone={auditTimelineRow(selected).result.tone}>{auditTimelineRow(selected).result.label}</Badge>}
          onClose={() => { setSelected(null); setCopyNotice('') }}
          footer={(
            <>
              <button className="a-btn a-btn--sm" type="button" onClick={() => void copyAuditID(String(selected.id))}>复制审计 ID</button>
              <div className="lr-spacer" />
              <button className="a-btn a-btn--sm a-btn--primary" type="button" onClick={() => { setSelected(null); setCopyNotice('') }}>关闭</button>
            </>
          )}
        >
          {copyNotice ? <InlineFeedback tone="success" message={copyNotice} /> : null}
          <section className="a-drawer__section">
            <h3>基本信息</h3>
            <dl className="a-kv">
              <div><dt>操作人</dt><dd>{auditTimelineRow(selected).actorLabel}</dd></div>
              <div><dt>来源 IP</dt><dd><span className="a-mono">{selected.ip_addr || '未记录'}</span></dd></div>
              <div><dt>动作</dt><dd>{auditTimelineRow(selected).actionLabel}</dd></div>
              <div><dt>对象</dt><dd>{auditTimelineRow(selected).targetLabel}</dd></div>
              <div><dt>结果</dt><dd><Badge tone={auditTimelineRow(selected).result.tone}>{auditTimelineRow(selected).result.label}</Badge></dd></div>
              <div><dt>时间</dt><dd>{auditTimelineRow(selected).createdAtLabel}</dd></div>
              <div><dt>User Agent</dt><dd><span className="lr-code">{selected.user_agent || '未记录'}</span></dd></div>
            </dl>
          </section>
          <section className="a-drawer__section">
            <h3>操作详情</h3>
            <div className="lr-prompt">{auditTimelineRow(selected).detailText}</div>
          </section>
          <section className="a-drawer__section">
            <h3>上下文</h3>
            <pre className="lr-json">{stableJson({
              audit_id: selected.id,
              actor: selected.actor,
              actor_type: selected.actor_type,
              actor_id: selected.actor_id,
              action: selected.action,
              target: selected.target,
              target_type: selected.target_type,
              target_id: selected.target_id,
              result: selected.result,
              ip_addr: selected.ip_addr,
              user_agent: selected.user_agent,
              metadata: selected.metadata,
            })}</pre>
          </section>
        </Drawer>
      ) : null}
    </section>
  )
}

function auditColumns(onOpen: (row: AuditLog) => void): ColumnDef<AuditLog>[] {
  return [
    {
      key: 'action',
      title: '动作与对象',
      width: 'minmax(240px,1.8fr)',
      render: (row) => {
        const item = auditTimelineRow(row)
        return (
          <button className="lr-cell lr-entity-button" type="button" onClick={() => onOpen(row)}>
            <strong>{item.actionLabel}</strong>
            <span>{item.targetLabel} · {String(row.id)}</span>
          </button>
        )
      },
    },
    {
      key: 'actor',
      title: '操作人',
      width: 'minmax(180px,1.2fr)',
      render: (row) => {
        const item = auditTimelineRow(row)
        return (
          <span className="a-cell-entity">
            <span className="a-cell-entity__avatar">{item.actorLabel.slice(0, 1).toUpperCase()}</span>
            <span className="a-cell-entity__copy">
              <span className="a-cell-entity__name">{item.actorLabel}</span>
              <span className="a-cell-entity__meta">{row.ip_addr || '未记录 IP'}</span>
            </span>
          </span>
        )
      },
    },
    {
      key: 'detail',
      title: '详情',
      width: 'minmax(300px,2.4fr)',
      render: (row) => <span className="lr-muted" style={{ display: 'block', maxWidth: 520, lineHeight: 1.55, overflowWrap: 'anywhere' }}>{auditTimelineRow(row).detailText}</span>,
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
      render: (row) => <span className="lr-muted" style={{ fontFamily: 'var(--font-admin-mono)', fontSize: 11.5 }}>{auditTimelineRow(row).createdAtLabel}</span>,
    },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(90px,.7fr)',
      align: 'right',
      render: (row) => <button className="a-row-btn a-row-btn--primary" type="button" onClick={() => onOpen(row)}>详情</button>,
    },
  ]
}

function stableJson(value: Record<string, unknown>) {
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return '{}'
  }
}