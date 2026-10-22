import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ClusterNode, PageResult } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import {
  Badge,
  EmptyBlock,
  ErrorBlock,
  InlineFeedback,
  LoadingBlock,
  PageHeader,
  RefreshIconButton,
  SegmentedControl,
} from '../components'
import { healthRefreshTimeLabel } from '../healthRows'
import { DataTable, ListPage, Pager, type ColumnDef } from '../ui/dataTable'
import { clusterNodeRows, clusterSummary, type ClusterNodeRow } from './clusterRows'
import { createLatestListRequestGuard } from './listRefresh'

type RoleFilter = '' | 'single' | 'control' | 'api' | 'worker' | 'web'

const roleOptions: Array<{ value: RoleFilter; label: string }> = [
  { value: '', label: '全部角色' },
  { value: 'single', label: '单节点' },
  { value: 'control', label: '控制' },
  { value: 'api', label: 'API' },
  { value: 'worker', label: 'Worker' },
  { value: 'web', label: 'Web' },
]

export function ClusterPage() {
  const [result, setResult] = useState<PageResult<ClusterNode>>({ items: [], total: 0 })
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [role, setRole] = useState<RoleFilter>('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [lastLoadedAt, setLastLoadedAt] = useState<string | null>(null)
  const requestGuard = useRef(createLatestListRequestGuard()).current

  const load = useCallback(async () => {
    const request = requestGuard.begin()
    setLoading(true)
    setError(null)
    try {
      const nextResult = await adminApi.listClusterNodes(page, pageSize, role)
      if (!requestGuard.isCurrent(request)) return
      setResult(nextResult)
      setLastLoadedAt(new Date().toISOString())
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
    {
      key: 'node',
      title: '节点',
      width: 'minmax(190px,1.3fr)',
      render: (row) => (
        <div className="a-cell-stack">
          <strong>{row.node_id}</strong>
          <span>{row.installation_id}</span>
        </div>
      ),
    },
    {
      key: 'role',
      title: '角色',
      width: 'minmax(100px,.7fr)',
      render: (row) => <Badge>{row.roleLabel}</Badge>,
    },
    {
      key: 'source',
      title: '来源',
      width: 'minmax(110px,.8fr)',
      render: (row) => <span className="a-hint">{row.sourceLabel}</span>,
    },
    {
      key: 'health',
      title: '健康',
      width: 'minmax(120px,.8fr)',
      render: (row) => <Badge tone={row.healthTone}><i />{row.healthLabel}</Badge>,
    },
    {
      key: 'contact',
      title: '最后联系',
      width: 'minmax(160px,1fr)',
      render: (row) => <span className="a-hint">{row.lastContactLabel}</span>,
    },
    {
      key: 'version',
      title: '版本',
      width: 'minmax(190px,1.2fr)',
      render: (row) => (
        <div className="a-cell-stack">
          <span>app {row.application_version}</span>
          <span>schema r{row.runtime_schema_version} · config r{row.config_revision}</span>
        </div>
      ),
    },
    {
      key: 'drift',
      title: '漂移',
      width: 'minmax(160px,1fr)',
      render: (row) => {
        const drifted = row.application_version_drift || row.runtime_schema_drift || row.config_revision_drift
        return (
          <Badge tone={!drifted ? 'success' : row.runtime_schema_drift ? 'danger' : 'warning'}>
            <i />{row.driftLabel}
          </Badge>
        )
      },
    },
    {
      key: 'action',
      title: '处理建议',
      width: 'minmax(220px,1.4fr)',
      render: (row) => <span className="a-hint">{recommendationFor(row)}</span>,
    },
  ], [])

  return (
    <section style={{ minWidth: 0 }}>
      <section className="a-panel a-panel--flush">
        <div className="a-status-strip" style={{ borderBottom: 0 }} aria-label="集群状态条">
          <div className="a-status-cell">
            <span>当前页节点</span>
            <strong>{summary.total}</strong>
          </div>
          <div className="a-status-cell">
            <span>运行正常</span>
            <strong>{summary.healthy}</strong>
          </div>
          <div className="a-status-cell">
            <span>需要处理</span>
            <strong>{summary.attention}</strong>
          </div>
          <div className="a-status-cell">
            <span>存在漂移</span>
            <strong>{summary.drifted}</strong>
          </div>
        </div>
      </section>

      <div style={{ marginTop: 16 }}>
        <PageHeader
          eyebrow="Cluster"
          title="集群节点"
          description="心跳、版本与配置漂移。节点角色决定它承载的进程，异常节点直接给出处理建议。"
          secondaryActions={<RefreshIconButton label="刷新集群节点" refreshing={loading} onClick={() => void load()} />}
        />
      </div>

      <div className="a-page-stack">
        {error && rows.length ? <InlineFeedback tone="danger" message={`集群节点刷新失败：${error}`} /> : null}
        <ListPage
          resultSummary={(
            <span>
              共 <b className="a-num">{result.total}</b> 个节点
              {lastLoadedAt ? ` · 更新 ${healthRefreshTimeLabel(lastLoadedAt)}` : ''}
            </span>
          )}
          actions={(
            <div style={{ maxWidth: '100%', overflowX: 'auto' }}>
              <SegmentedControl
                value={role}
                options={roleOptions}
                onChange={(value) => {
                  setRole(value)
                  setPage(1)
                }}
              />
            </div>
          )}
          pagination={<Pager page={page} pageSize={pageSize} total={result.total} onChange={setPage} onPageSizeChange={(size) => { setPageSize(size); setPage(1) }} />}
        >
          {loading && !rows.length ? <LoadingBlock label="载入集群节点" /> : null}
          {error && !rows.length ? <ErrorBlock message={error} onRetry={load} /> : null}
          {rows.length || (!loading && !error) ? (
            <DataTable
              columns={columns}
              rows={rows}
              rowKey={(row) => row.node_id}
              minWidth={1180}
              empty={<EmptyBlock variant="inline" title="暂无节点" detail="当前筛选范围内没有节点记录。" />}
            />
          ) : null}
        </ListPage>
      </div>
    </section>
  )
}

function recommendationFor(row: ClusterNodeRow) {
  if (row.last_error) return row.last_error
  if (row.actionLabel && row.actionLabel !== '-') return row.actionLabel
  if (row.runtime_schema_drift) return '检查迁移任务，等待 Schema 升级到最新版本'
  if (row.application_version_drift) return '滚动升级未完成，确认新版本后重启'
  if (row.config_revision_drift) return '配置版本落后，确认后重新下发配置'
  return '—'
}