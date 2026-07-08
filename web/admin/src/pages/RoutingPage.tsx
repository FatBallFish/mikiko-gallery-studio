import { useEffect, useMemo, useState } from 'react'
import type { ModelAccountModel, RouteModel, RouteModelCandidate, RouteModelVisibility, UserGroup } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, Field, GroupOptionGrid, LoadingBlock, Modal, PageHeader } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { adminDataGrid } from '../ui/dataGrid'
import { ListPage } from '../ui/dataTable'
import { ChevronDownIcon } from '../ui/icons'
import {
  routeCandidateLabel,
  routeCandidateSummary,
  routeEnabledOptions,
  routingFieldHints,
  routingFieldLabels,
  routeGroupNames,
  routeReadinessBadge,
  routeVisibilityBadge,
  routeVisibilityOptions,
} from './routingRows'

type RouteDialog = { row?: RouteModel; code: string; name: string; description: string; visibility: RouteModelVisibility; enabled: boolean; sortOrder: string; groupIds: string[] }
type CandidateDialog = { route: RouteModel; row?: RouteModelCandidate; accountModelId: string; priority: string; weight: string; fallbackOrder: string; enabled: boolean }

const routingClasses = {
  actionRow: 'flex flex-wrap items-center gap-2',
  surface: adminPage.fullSurface,
  toolbar: 'flex flex-wrap items-end justify-between gap-4',
  toolbarActions: 'flex flex-wrap items-center gap-4',
  searchInput: 'w-64 max-w-full rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] px-4 py-2.5 text-sm text-[var(--text)] placeholder:text-[var(--soft)] focus:border-[var(--accent)]/50 focus:outline-none focus:ring-1 focus:ring-[var(--accent)]/50',
  statDetails: 'rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] px-4 py-3',
  statSummary: 'cursor-pointer text-xs font-extrabold uppercase tracking-[.14em] text-[var(--muted-strong)]',
  statGrid: 'mt-3 grid grid-cols-4 gap-3 max-[860px]:grid-cols-2 max-[520px]:grid-cols-1',
  statCell: 'rounded-xl border border-[var(--border)] bg-[var(--canvas)] p-3',
  statLabel: 'block text-[10px] font-extrabold uppercase tracking-[.12em] text-[var(--soft)]',
  statValue: 'mt-1 block text-xl font-black text-[var(--text)]',
  checkboxCell: 'w-10 px-6 py-4 text-center',
  checkbox: 'size-3.5 accent-[var(--accent)]',
  stackCell: cn(adminDataGrid.stackCell, 'gap-0.5'),
  paragraph: 'm-0 text-xs text-[var(--soft)] [overflow-wrap:anywhere]',
  textCell: 'min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-[var(--soft)]',
  tableWrap: 'min-w-0 overflow-x-auto',
  table: 'admin-table min-w-[920px]',
  tr: 'transition-colors hover:bg-[var(--surface-solid)]',
  trExpanded: 'bg-[var(--canvas)]',
  td: 'text-sm text-[var(--muted)]',
  routeTitle: 'font-bold text-[var(--text)]',
  routeCode: 'font-mono text-[10px] font-bold tracking-tight text-[var(--muted-strong)]',
  candidateButton: 'inline-flex items-center gap-2 text-xs font-extrabold text-[var(--accent)] transition-colors hover:text-[var(--text)]',
  chevron: 'size-4 transition-transform',
  expandedRow: 'bg-[var(--canvas)]',
  expandedCell: 'p-6 pl-20',
  candidatePanel: 'overflow-hidden rounded-lg bg-[var(--surface-solid)]',
  candidateTable: 'admin-table min-w-[680px]',
  candidateTd: 'px-4 py-3 text-xs text-[var(--muted)]',
  candidateName: 'font-bold text-[var(--text)]',
  candidateCode: 'font-mono text-[var(--soft)]',
  activeDot: 'size-2 rounded-full',
}

export function RoutingPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [routes, setRoutes] = useState<RouteModel[]>([])
  const [groups, setGroups] = useState<UserGroup[]>([])
  const [accountModels, setAccountModels] = useState<ModelAccountModel[]>([])
  const [candidates, setCandidates] = useState<Record<string, RouteModelCandidate[]>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState('路由模型可设置全员可见、按分组可见或隐藏，并绑定多个真实模型候选。')
  const [routeDialog, setRouteDialog] = useState<RouteDialog | null>(null)
  const [candidateDialog, setCandidateDialog] = useState<CandidateDialog | null>(null)
  const [expandedRoutes, setExpandedRoutes] = useState<Record<string, boolean>>({})
  const [selectedRoutes, setSelectedRoutes] = useState<Record<string, boolean>>({})
  const [query, setQuery] = useState('')
  const [saving, setSaving] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const [nextRoutes, nextGroups, nextAccounts] = await Promise.all([
        adminApi.listRouteModels({ page_size: 100 }),
        adminApi.listUserGroups(),
        adminApi.listModelAccounts({ page_size: 100 }),
      ])
      const modelLists = await Promise.all(nextAccounts.map((account) => adminApi.listModelAccountModels(account.id)))
      const candidatePairs = await Promise.all(nextRoutes.map(async (route) => [String(route.id), await adminApi.listRouteModelCandidates(route.id)] as const))
      setRoutes(nextRoutes)
      setGroups(nextGroups)
      setAccountModels(modelLists.flat())
      setCandidates(Object.fromEntries(candidatePairs))
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '路由载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [])

  const totals = useMemo(() => ({
    enabled: routes.filter((row) => row.enabled).length,
    groupVisible: routes.filter((row) => row.visibility === 'groups').length,
    candidateCount: Object.values(candidates).flat().length,
  }), [routes, candidates])
  const visibleRoutes = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    if (!keyword) return routes
    return routes.filter((row) => [
      row.name,
      row.code,
      row.description,
      routeGroupNames(row.group_ids, groups),
    ].filter(Boolean).some((value) => String(value).toLowerCase().includes(keyword)))
  }, [groups, query, routes])
  const selectedCount = useMemo(() => visibleRoutes.filter((row) => selectedRoutes[String(row.id)]).length, [selectedRoutes, visibleRoutes])
  const allVisibleSelected = Boolean(visibleRoutes.length) && selectedCount === visibleRoutes.length
  const toggleAllVisible = (checked: boolean) => {
    setSelectedRoutes((current) => {
      const next = { ...current }
      visibleRoutes.forEach((row) => { next[String(row.id)] = checked })
      return next
    })
  }

  async function saveRoute() {
    if (!routeDialog) return
    setSaving(true)
    try {
      const groupIds = routeDialog.visibility === 'groups' ? routeDialog.groupIds.map(Number).filter((id) => Number.isFinite(id) && id > 0) : []
      const payload = {
        code: routeDialog.code,
        name: routeDialog.name,
        description: routeDialog.description,
        visibility: routeDialog.visibility,
        enabled: routeDialog.enabled,
        sort_order: Number(routeDialog.sortOrder),
        group_ids: groupIds,
      }
      const saved = routeDialog.row ? await adminApi.updateRouteModel(routeDialog.row.id, payload) : await adminApi.createRouteModel(payload)
      setRouteDialog(null)
      setNotice(`${saved.name} 路由模型已保存。`)
      onFeedback('路由模型已保存', saved.code)
      await load()
    } finally {
      setSaving(false)
    }
  }

  async function saveCandidate() {
    if (!candidateDialog) return
    setSaving(true)
    try {
      const payload = {
        account_model_id: Number(candidateDialog.accountModelId),
        priority: Number(candidateDialog.priority),
        weight: Number(candidateDialog.weight),
        fallback_order: Number(candidateDialog.fallbackOrder),
        enabled: candidateDialog.enabled,
      }
      if (candidateDialog.row) await adminApi.updateRouteModelCandidate(candidateDialog.route.id, candidateDialog.row.id, payload)
      else await adminApi.createRouteModelCandidate(candidateDialog.route.id, payload)
      setCandidateDialog(null)
      setNotice(`${candidateDialog.route.code} 候选已保存。`)
      await load()
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <LoadingBlock label="载入模型路由" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className={adminPage.stack}>
      <PageHeader
        title="路由模型"
        description="创建用户可见的路由模型，并完成候选模型、可见范围和价格状态配置。"
        primaryAction={<button className={cn(adminButton.base, adminButton.primary)} type="button" onClick={() => setRouteDialog(newRouteDialog(groups))}>新增路由模型</button>}
        secondaryActions={<button className={cn(adminButton.base, adminButton.ghost)} type="button" onClick={() => void load()}>刷新</button>}
      />
      <div className="flex flex-wrap items-center justify-between gap-4">
        <input className={routingClasses.searchInput} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索路由名称或代码..." aria-label="搜索路由名称或代码" />
        <button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={!selectedCount} onClick={() => onFeedback('已选择路由模型', selectedCount ? `${selectedCount} 个` : '请先勾选路由模型')}>批量操作</button>
      </div>
      <details className={routingClasses.statDetails}>
        <summary className={routingClasses.statSummary}>路由模型运行摘要 · 候选模型</summary>
        <div className={routingClasses.statGrid}>
          <span className={routingClasses.statCell}><em className={routingClasses.statLabel}>路由模型</em><strong className={routingClasses.statValue}>{routes.length}</strong></span>
          <span className={routingClasses.statCell}><em className={routingClasses.statLabel}>启用</em><strong className={routingClasses.statValue}>{totals.enabled}</strong></span>
          <span className={routingClasses.statCell}><em className={routingClasses.statLabel}>分组可见</em><strong className={routingClasses.statValue}>{totals.groupVisible}</strong></span>
          <span className={routingClasses.statCell}><em className={routingClasses.statLabel}>候选模型</em><strong className={routingClasses.statValue}>{totals.candidateCount}</strong></span>
        </div>
        <p className={routingClasses.paragraph}>{notice}</p>
      </details>
      {!routes.length ? <EmptyBlock title="暂无路由模型" detail="创建路由模型后继续配置候选真实模型、可见范围和价格。" action={<button className={cn(adminButton.base, adminButton.primary)} type="button" onClick={() => setRouteDialog(newRouteDialog(groups))}>新增路由模型</button>} /> : null}
      {routes.length && !visibleRoutes.length ? <EmptyBlock title="未找到路由模型" detail="换一个名称、代码或分组关键词再试。" /> : null}
      {visibleRoutes.length ? (
        <ListPage>
        <div className={routingClasses.tableWrap}>
          <table className={routingClasses.table}>
            <thead>
              <tr>
                <th className={routingClasses.checkboxCell}>
                  <input className={routingClasses.checkbox} type="checkbox" checked={allVisibleSelected} onChange={(event) => toggleAllVisible(event.target.checked)} aria-label="选择当前路由模型" />
                </th>
                <th>路由模型</th>
                <th>可见性</th>
                <th>可用状态</th>
                <th>已绑候选账号数</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {visibleRoutes.map((row) => {
                const routeCandidates = candidates[String(row.id)] ?? row.candidates ?? []
                const expanded = Boolean(expandedRoutes[String(row.id)])
                return (
                  <RouteTableRow
                    key={String(row.id)}
                    row={row}
                    groups={groups}
                    candidates={routeCandidates}
                    expanded={expanded}
                    selected={Boolean(selectedRoutes[String(row.id)])}
                    onSelect={(checked) => setSelectedRoutes((current) => ({ ...current, [String(row.id)]: checked }))}
                    onToggle={() => setExpandedRoutes((current) => ({ ...current, [String(row.id)]: !expanded }))}
                    onEditRoute={() => setRouteDialog(editRouteDialog(row))}
                    onAddCandidate={() => setCandidateDialog(newCandidateDialog(row, accountModels))}
                    onEditCandidate={(candidate) => setCandidateDialog(editCandidateDialog(row, candidate))}
                  />
                )
              })}
            </tbody>
          </table>
        </div>
        </ListPage>
      ) : null}
      {routeDialog ? (
        <Modal title={routeDialog.row ? '编辑路由模型' : '新增路由模型'} detail="保存后需要继续配置候选真实模型和价格，完成后才会对用户可用。" onClose={() => setRouteDialog(null)} footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={saving} onClick={() => setRouteDialog(null)}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={saving || !routeDialog.code || !routeDialog.name} onClick={() => void saveRoute()}>{saving ? '保存中...' : '保存并继续配置候选'}</button></>}>
          <div className={adminPage.formGrid}>
            <Field label={routingFieldLabels.code}><input value={routeDialog.code} onChange={(event) => setRouteDialog({ ...routeDialog, code: event.target.value })} placeholder="basic" /></Field>
            <Field label="名称"><input value={routeDialog.name} onChange={(event) => setRouteDialog({ ...routeDialog, name: event.target.value })} /></Field>
            <Field label="描述"><input value={routeDialog.description} onChange={(event) => setRouteDialog({ ...routeDialog, description: event.target.value })} /></Field>
            <Field label="可见性"><select value={routeDialog.visibility} onChange={(event) => setRouteDialog({ ...routeDialog, visibility: event.target.value, groupIds: event.target.value === 'groups' ? routeDialog.groupIds : [] })}>{routeVisibilityOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
            {routeDialog.visibility === 'groups' ? (
              <Field label="可见分组"><GroupOptionGrid selected={routeDialog.groupIds} groups={groups} onChange={(groupIds) => setRouteDialog({ ...routeDialog, groupIds })} /></Field>
            ) : null}
            <Field label="排序"><input type="number" value={routeDialog.sortOrder} onChange={(event) => setRouteDialog({ ...routeDialog, sortOrder: event.target.value })} /></Field>
            <Field label="状态"><select value={routeDialog.enabled ? 'enabled' : 'disabled'} onChange={(event) => setRouteDialog({ ...routeDialog, enabled: event.target.value === 'enabled' })}>{routeEnabledOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
          </div>
        </Modal>
      ) : null}
      {candidateDialog ? (
        <Modal title={candidateDialog.row ? '编辑候选真实模型' : '新增候选真实模型'} detail={candidateDialog.route.name} onClose={() => setCandidateDialog(null)} footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={saving} onClick={() => setCandidateDialog(null)}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={saving || !candidateDialog.accountModelId} onClick={() => void saveCandidate()}>{saving ? '保存中...' : '保存'}</button></>}>
          <div className={adminPage.formGrid}>
            <Field label="真实模型"><select value={candidateDialog.accountModelId} onChange={(event) => setCandidateDialog({ ...candidateDialog, accountModelId: event.target.value })}>{accountModels.map((model) => <option key={String(model.id)} value={String(model.id)}>{model.account_name ? `${model.account_name} / ` : ''}{model.model_code}</option>)}</select></Field>
            <Field label={routingFieldLabels.priority} hint={routingFieldHints.priority}><input type="number" min="1" value={candidateDialog.priority} onChange={(event) => setCandidateDialog({ ...candidateDialog, priority: event.target.value })} /></Field>
            <Field label={routingFieldLabels.weight} hint={routingFieldHints.weight}><input type="number" min="0" value={candidateDialog.weight} onChange={(event) => setCandidateDialog({ ...candidateDialog, weight: event.target.value })} /></Field>
            <Field label={routingFieldLabels.fallbackOrder} hint={routingFieldHints.fallbackOrder}><input type="number" min="1" value={candidateDialog.fallbackOrder} onChange={(event) => setCandidateDialog({ ...candidateDialog, fallbackOrder: event.target.value })} /></Field>
            <Field label="状态"><select value={candidateDialog.enabled ? 'enabled' : 'disabled'} onChange={(event) => setCandidateDialog({ ...candidateDialog, enabled: event.target.value === 'enabled' })}>{routeEnabledOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
          </div>
        </Modal>
      ) : null}
    </section>
  )
}

function newRouteDialog(groups: UserGroup[]): RouteDialog {
  return { code: '', name: '', description: '', visibility: 'public', enabled: true, sortOrder: '10', groupIds: [] }
}

function editRouteDialog(row: RouteModel): RouteDialog {
  return { row, code: row.code, name: row.name, description: row.description ?? '', visibility: row.visibility, enabled: row.enabled, sortOrder: String(row.sort_order), groupIds: (row.group_ids ?? []).map(String) }
}

function newCandidateDialog(route: RouteModel, accountModels: ModelAccountModel[]): CandidateDialog {
  return { route, accountModelId: String(accountModels[0]?.id ?? ''), priority: '1', weight: '100', fallbackOrder: '1', enabled: true }
}

function editCandidateDialog(route: RouteModel, row: RouteModelCandidate): CandidateDialog {
  return { route, row, accountModelId: String(row.account_model_id), priority: String(row.priority), weight: String(row.weight), fallbackOrder: String(row.fallback_order), enabled: row.enabled }
}

function RouteBadge({ badge }: { badge: { label: string; tone: 'success' | 'warning' | 'danger' | 'neutral' | 'primary' } }) {
  return <Badge tone={badge.tone}>{badge.label}</Badge>
}

function RouteTableRow({
  row,
  groups,
  candidates,
  expanded,
  selected,
  onSelect,
  onToggle,
  onEditRoute,
  onAddCandidate,
  onEditCandidate,
}: {
  row: RouteModel
  groups: UserGroup[]
  candidates: RouteModelCandidate[]
  expanded: boolean
  selected: boolean
  onSelect: (checked: boolean) => void
  onToggle: () => void
  onEditRoute: () => void
  onAddCandidate: () => void
  onEditCandidate: (candidate: RouteModelCandidate) => void
}) {
  const readiness = routeReadinessBadge({ enabled: row.enabled, candidates })
  return (
    <>
      <tr className={cn(routingClasses.tr, expanded && routingClasses.trExpanded)}>
        <td className={routingClasses.checkboxCell}>
          <input className={routingClasses.checkbox} type="checkbox" checked={selected} onChange={(event) => onSelect(event.target.checked)} aria-label={`选择 ${row.name}`} />
        </td>
        <td className={routingClasses.td}>
          <div className="flex min-w-0 flex-col gap-1">
            <span className={routingClasses.routeTitle}>{row.name}</span>
            <span className={routingClasses.routeCode}>{row.code}</span>
            <p className={routingClasses.paragraph}>{row.description || '无描述'}</p>
          </div>
        </td>
        <td className={routingClasses.td}>
          <div className="flex min-w-0 flex-col gap-2">
            <RouteBadge badge={routeVisibilityBadge(row.visibility)} />
            <span className={routingClasses.textCell}>{routeGroupNames(row.group_ids, groups)}</span>
          </div>
        </td>
        <td className={routingClasses.td}><RouteBadge badge={readiness} /></td>
        <td className={routingClasses.td}>
          <button type="button" className={routingClasses.candidateButton} onClick={onToggle}>
            {routeCandidateSummary(candidates)}
            <ChevronDownIcon className={cn(routingClasses.chevron, expanded && 'rotate-180')} />
          </button>
        </td>
        <td className={routingClasses.td}>
          <div className={routingClasses.actionRow}>
            <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={onEditRoute}>编辑</button>
            <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={onAddCandidate}>加候选</button>
          </div>
        </td>
      </tr>
      {expanded ? (
        <tr className={routingClasses.expandedRow}>
          <td colSpan={6} className={routingClasses.expandedCell}>
            <CandidatePanel candidates={candidates} onAddCandidate={onAddCandidate} onEditCandidate={onEditCandidate} />
          </td>
        </tr>
      ) : null}
    </>
  )
}

function CandidatePanel({
  candidates,
  onAddCandidate,
  onEditCandidate,
}: {
  candidates: RouteModelCandidate[]
  onAddCandidate: () => void
  onEditCandidate: (candidate: RouteModelCandidate) => void
}) {
  if (!candidates.length) {
    return (
      <div className={cn(routingClasses.candidatePanel, 'p-4')}>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <span className="text-sm font-bold text-[var(--muted)]">暂无候选真实模型</span>
          <button type="button" className={cn(adminButton.base, adminButton.primary, adminButton.small)} onClick={onAddCandidate}>新增候选</button>
        </div>
      </div>
    )
  }

  return (
    <div className={routingClasses.candidatePanel}>
      <table className={routingClasses.candidateTable}>
        <thead>
          <tr>
            <th>真实账号</th>
            <th>底层模型</th>
            <th>{routingFieldLabels.priority}</th>
            <th>{routingFieldLabels.weight}</th>
            <th>{routingFieldLabels.fallbackOrder}</th>
            <th>状态</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          {candidates.map((candidate) => (
            <tr key={String(candidate.id)} className="border-b border-[var(--line)]/60 last:border-b-0 hover:bg-white/[0.02]">
              <td className={routingClasses.candidateTd}><span className={routingClasses.candidateName}>{candidate.account_name || '未命名账号'}</span></td>
              <td className={routingClasses.candidateTd}><span className={routingClasses.candidateCode}>{candidate.model_code || routeCandidateLabel(candidate)}</span></td>
              <td className={routingClasses.candidateTd}><code className={adminDataGrid.code}>{candidate.priority}</code></td>
              <td className={routingClasses.candidateTd}><code className={adminDataGrid.code}>{candidate.weight}</code></td>
              <td className={routingClasses.candidateTd}><code className={adminDataGrid.code}>{candidate.fallback_order}</code></td>
              <td className={routingClasses.candidateTd}>
                <span className={cn(routingClasses.activeDot, candidate.enabled ? 'bg-[var(--green)]' : 'bg-white/20')} />
              </td>
              <td className={routingClasses.candidateTd}>
                <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => onEditCandidate(candidate)}>编辑</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
