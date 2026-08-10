import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ClusterNode, PageResult } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, InlineFeedback, LoadingBlock, PageHeader, RefreshIconButton, SegmentedControl, StatusCell, StatusStrip } from '../components'
import { DataTable, ListPage, Pager, type ColumnDef } from '../ui/dataTable'
import { clusterNodeRows, clusterSummary, type ClusterNodeRow } from './clusterRows'
import { createLatestListRequestGuard } from './listRefresh'

type RoleFilter = '' | 'single' | 'control' | 'api' | 'worker' | 'web'

export function ClusterPage() {
  const [result, setResult] = useState<PageResult<ClusterNode>>({ items: [], total: 0 })
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [role, setRole] = useState<RoleFilter>('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const requestGuard = useRef(createLatestListRequestGuard()).current

  const load = useCallback(async () => {
    const request = requestGuard.begin()
    setLoading(true)
    setError(null)
    try {
      const nextResult = await adminApi.listClusterNodes(page, pageSize, role)
      if (!requestGuard.isCurrent(request)) return
      setResult(nextResult)
    } catch (caught) {
      if (!requestGuard.isCurrent(request)) return
      setError(caught instanceof Error ? caught.message : '集群节点载入失败')
    } finally {
      if (!requestGuard.isCurrent(request)) return
      setLoading(false)
    }
  }, [page, pageSize, requestGuard, role])

  useEffect(() => {
    void load()
    return () => requestGuard.invalidate()
  }, [load, requestGuard])

  const rows = useMemo(() => clusterNodeRows(result.items), [result.items])
  const summary = useMemo(() => clusterSummary(result.items), [result.items])
  const columns = useMemo<ColumnDef<ClusterNodeRow>[]>(() => [
    { key: 'node', title: '节点', width: 'minmax(190px,1.3fr)', render: (row) => <div className="min-w-0"><strong className="block truncate text-[var(--fg)]">{row.node_id}</strong><span className="mt-1 block truncate text-xs text-[var(--soft)]">{row.installation_id}</span></div> },
    { key: 'role', title: '角色', width: 'minmax(100px,.7fr)', render: (row) => <Badge>{row.roleLabel}</Badge> },
    { key: 'source', title: '来源', width: 'minmax(110px,.8fr)', render: (row) => <span className="text-[var(--muted)]">{row.sourceLabel}</span> },
    { key: 'health', title: '健康', width: 'minmax(110px,.7fr)', render: (row) => <Badge tone={row.healthTone}>{row.healthLabel}</Badge> },
    { key: 'contact', title: '最后联系', width: 'minmax(150px,1fr)', kind: 'code', render: (row) => row.lastContactLabel },
    { key: 'version', title: '应用 / Schema / 配置', width: 'minmax(180px,1.1fr)', kind: 'code', render: (row) => `${row.application_version} / ${row.runtime_schema_version} / ${row.config_revision}` },
    { key: 'drift', title: '漂移', width: 'minmax(160px,1fr)', render: (row) => <span className={row.driftLabel === '一致' ? 'text-[var(--green)]' : 'text-[var(--amber)]'}>{row.driftLabel}</span> },
    { key: 'action', title: '处理建议', width: 'minmax(190px,1.2fr)', render: (row) => <span className="text-[var(--muted)]">{row.last_error || row.actionLabel}</span> },
  ], [])

  return (
    <section className="grid min-h-0 gap-6">
      <PageHeader
        eyebrow="Cluster"
        title="集群节点"
        detail="汇总 API 与 Worker 节点的心跳、运行版本和配置一致性。"
        secondaryActions={<RefreshIconButton label="刷新集群节点" refreshing={loading} onClick={() => void load()} />}
      />
      <StatusStrip columns={4}>
        <StatusCell label="当前页节点" value={summary.total} />
        <StatusCell label="运行正常" value={summary.healthy} />
        <StatusCell label="需要处理" value={summary.attention} />
        <StatusCell label="存在漂移" value={summary.drifted} />
      </StatusStrip>
      <ListPage
        filters={<SegmentedControl value={role} options={[{ value: '', label: '全部' }, { value: 'single', label: '单节点' }, { value: 'control', label: '控制' }, { value: 'api', label: 'API' }, { value: 'worker', label: 'Worker' }, { value: 'web', label: 'Web' }]} onChange={(value) => { setRole(value); setPage(1) }} />}
        resultSummary={`共 ${result.total} 个节点`}
        pagination={<Pager page={page} pageSize={pageSize} total={result.total} onChange={setPage} onPageSizeChange={(size) => { setPageSize(size); setPage(1) }} />}
      >
        {loading && !rows.length ? <LoadingBlock label="载入集群节点" /> : null}
        {error && rows.length ? <InlineFeedback tone="danger" message={`集群节点刷新失败：${error}`} /> : null}
        {error && !rows.length ? <ErrorBlock message={error} onRetry={load} /> : null}
        {rows.length || (!loading && !error) ? <DataTable columns={columns} rows={rows} rowKey={(row) => row.node_id} empty={<EmptyBlock variant="inline" title="暂无节点" detail="当前筛选范围内没有节点记录。" />} /> : null}
      </ListPage>
    </section>
  )
}
