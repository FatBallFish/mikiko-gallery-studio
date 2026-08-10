import { useEffect, useMemo, useRef, useState } from 'react'
import type { AdminMetric, ModelAccount, ModelAccountModel, RouteModel, RouteModelCandidate, RouteModelPrice, RouteModelVisibility, UserGroup } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { adminApi } from '../../../shared/admin-api'
import { Trash2 } from 'lucide-react'
import { Badge, EmptyBlock, ErrorBlock, Field, GroupOptionGrid, InlineFeedback, LoadingBlock, MetricStrip, Modal, PageHeader, RefreshIconButton, TooltipIconButton } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { adminDataGrid } from '../ui/dataGrid'
import { FilterToolbar } from '../ui/dataTable'
import { loadAllRouteModelPrices } from './loadAllRouteModelPrices'
import { modelLifecycleErrorMessage } from './adminModelLifecycle'
import { createLatestListRequestGuard } from './listRefresh'
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
type DeleteTarget = { kind: 'route'; route: RouteModel } | { kind: 'candidate'; route: RouteModel; candidate: RouteModelCandidate }

const routingClasses = {
  actionRow: 'flex flex-wrap items-center gap-2',
  workspace: 'grid min-h-[560px] min-w-0 overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--surface-solid)] lg:grid-cols-[minmax(240px,320px)_minmax(0,1fr)]',
  master: 'min-w-0 border-b border-[var(--border)] bg-[var(--surface)] lg:border-b-0 lg:border-r',
  masterHead: 'border-b border-[var(--border)] p-4',
  masterList: 'grid max-h-[640px] overflow-y-auto p-2',
  masterItem: 'grid min-h-[64px] w-full gap-1 rounded-lg border border-transparent px-3 py-2 text-left transition-colors duration-[var(--admin-motion-fast)] hover:bg-[var(--elevated)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]',
  masterItemActive: 'border-[var(--border-strong)] bg-[var(--surface-solid)]',
  detail: 'min-w-0 p-4 sm:p-5',
  detailHead: 'flex flex-wrap items-start justify-between gap-4 border-b border-[var(--border)] pb-4',
  paragraph: 'm-0 text-xs leading-5 text-[var(--soft)] [overflow-wrap:anywhere]',
  routeTitle: 'text-sm font-semibold text-[var(--text)]',
  routeCode: 'font-[family-name:var(--admin-font-mono)] text-xs text-[var(--dim)]',
  statusGrid: 'grid gap-3 py-4 sm:grid-cols-2 xl:grid-cols-4',
  statusRow: 'grid gap-1 rounded-lg border border-[var(--border)] bg-[var(--surface)] p-3',
  statusLabel: 'text-[11px] font-semibold text-[var(--dim)]',
  candidatePanel: 'min-w-0 overflow-hidden rounded-lg border border-[var(--border)]',
  candidateTable: 'admin-table min-w-[760px]',
  candidateTd: 'px-3 py-3 text-xs text-[var(--muted)]',
  candidateName: 'font-semibold text-[var(--text)]',
  candidateCode: 'font-[family-name:var(--admin-font-mono)] text-[var(--soft)]',
  activeDot: 'size-2 rounded-full',
}

export function RoutingPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [routes, setRoutes] = useState<RouteModel[]>([])
  const [groups, setGroups] = useState<UserGroup[]>([])
  const [modelAccounts, setModelAccounts] = useState<ModelAccount[]>([])
  const [accountModels, setAccountModels] = useState<ModelAccountModel[]>([])
  const [candidates, setCandidates] = useState<Record<string, RouteModelCandidate[]>>({})
  const [prices, setPrices] = useState<RouteModelPrice[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState('路由模型可设置全员可见、按分组可见或隐藏，并绑定多个真实模型候选。')
  const [routeDialog, setRouteDialog] = useState<RouteDialog | null>(null)
  const [candidateDialog, setCandidateDialog] = useState<CandidateDialog | null>(null)
  const [selectedRouteId, setSelectedRouteId] = useState('')
  const [query, setQuery] = useState('')
  const [saving, setSaving] = useState(false)
  const [mutationError, setMutationError] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null)
  const [deleting, setDeleting] = useState(false)
  const requestGuard = useRef(createLatestListRequestGuard()).current

  const load = async (preferredRouteId?: string) => {
    const request = requestGuard.begin()
    setLoading(true)
    setError(null)
    try {
      const [nextRoutes, nextGroups, nextAccounts, nextPrices] = await Promise.all([
        adminApi.listRouteModels({ page_size: 100 }),
        adminApi.listUserGroups(),
        adminApi.listModelAccounts({ page_size: 100 }),
        loadAllRouteModelPrices((priceQuery) => adminApi.listRouteModelPrices(priceQuery)),
      ])
      const modelLists = await Promise.all(nextAccounts.map((account) => adminApi.listModelAccountModels(account.id)))
      const candidatePairs = await Promise.all(nextRoutes.map(async (route) => [String(route.id), await adminApi.listRouteModelCandidates(route.id)] as const))
      if (!requestGuard.isCurrent(request)) return
      setRoutes(nextRoutes)
      setGroups(nextGroups)
      setModelAccounts(nextAccounts)
      setAccountModels(modelLists.flat())
      setCandidates(Object.fromEntries(candidatePairs))
      setPrices(nextPrices)
      setSelectedRouteId((current) => {
        const preferred = preferredRouteId || current
        return nextRoutes.some((route) => String(route.id) === preferred) ? preferred : String(nextRoutes[0]?.id ?? '')
      })
    } catch (caught) {
      if (!requestGuard.isCurrent(request)) return
      setError(caught instanceof Error ? caught.message : '路由载入失败')
    } finally {
      if (!requestGuard.isCurrent(request)) return
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    return () => requestGuard.invalidate()
  }, [])

  const totals = useMemo(() => ({
    enabled: routes.filter((row) => row.enabled).length,
    groupVisible: routes.filter((row) => row.visibility === 'groups').length,
    candidateCount: Object.values(candidates).flat().length,
    blocked: routes.filter((route) => routeReadinessBadge({
      enabled: route.enabled,
      visibility: route.visibility,
      groupCount: route.group_ids?.length ?? 0,
      candidates: effectiveRouteCandidates(candidates[String(route.id)] ?? route.candidates ?? [], accountModels, modelAccounts),
      prices: prices.filter((price) => String(price.route_model_id) === String(route.id)),
    }).state !== 'ready').length,
  }), [routes, candidates, prices, accountModels, modelAccounts])
  const summaryMetrics = useMemo<AdminMetric[]>(() => [
    { label: '路由模型', value: String(routes.length), trend: `${totals.enabled} 个启用`, tone: 'neutral' },
    { label: '候选模型', value: String(totals.candidateCount), trend: '已绑定真实模型', tone: 'neutral' },
    { label: '分组可见', value: String(totals.groupVisible), trend: '受权益分组约束', tone: 'neutral' },
    { label: '阻断项', value: String(totals.blocked), trend: totals.blocked ? '需要完成配置' : '全部可用', tone: totals.blocked ? 'warn' : 'good' },
  ], [routes.length, totals])
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
  useEffect(() => {
    if (visibleRoutes.length && !visibleRoutes.some((route) => String(route.id) === selectedRouteId)) {
      setSelectedRouteId(String(visibleRoutes[0].id))
    }
  }, [selectedRouteId, visibleRoutes])
  const selectedRoute = useMemo(() => visibleRoutes.find((route) => String(route.id) === selectedRouteId), [visibleRoutes, selectedRouteId])

  async function saveRoute() {
    if (!routeDialog) return
    setSaving(true)
    setMutationError(null)
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
      const created = !routeDialog.row
      setRouteDialog(null)
      setMutationError(null)
      setNotice(`${saved.name} 路由模型已保存。`)
      onFeedback('路由模型已保存', saved.code)
      setSelectedRouteId(String(saved.id))
      if (created) setQuery('')
      await load(String(saved.id))
      if (created) setCandidateDialog(newCandidateDialog(saved, accountModels))
    } catch (caught) {
      setMutationError(caught instanceof Error ? caught.message : '路由模型保存失败')
    } finally {
      setSaving(false)
    }
  }

  async function saveCandidate() {
    if (!candidateDialog) return
    setSaving(true)
    setMutationError(null)
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
      setMutationError(null)
      setNotice(`${candidateDialog.route.code} 候选已保存。`)
      await load()
    } catch (caught) {
      setMutationError(caught instanceof Error ? caught.message : '候选模型保存失败')
    } finally {
      setSaving(false)
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return
    setDeleting(true)
    setMutationError(null)
    try {
      if (deleteTarget.kind === 'route') await adminApi.deleteRouteModel(deleteTarget.route.id)
      else await adminApi.deleteRouteModelCandidate(deleteTarget.route.id, deleteTarget.candidate.id)
      const deletedLabel = deleteTarget.kind === 'route' ? deleteTarget.route.code : deleteTarget.candidate.model_code || String(deleteTarget.candidate.id)
      setDeleteTarget(null)
      setNotice(`${deletedLabel} 已删除。`)
      await load()
    } catch (caught) {
      setMutationError(modelLifecycleErrorMessage(caught, '删除路由配置失败'))
    } finally {
      setDeleting(false)
    }
  }

  const openRouteDialog = (dialog: RouteDialog) => {
    setMutationError(null)
    setRouteDialog(dialog)
  }

  const openCandidateDialog = (dialog: CandidateDialog) => {
    setMutationError(null)
    setCandidateDialog(dialog)
  }

  const closeRouteDialog = () => {
    setMutationError(null)
    setRouteDialog(null)
  }

  const closeCandidateDialog = () => {
    setMutationError(null)
    setCandidateDialog(null)
  }

  if (loading && !routes.length) return <LoadingBlock label="载入模型路由" />
  if (error && !routes.length) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className={adminPage.stack}>
      <PageHeader
        title="路由模型"
        description="创建用户可见的路由模型，并完成候选模型、可见范围和价格状态配置。"
        primaryAction={<button className={cn(adminButton.base, adminButton.primary)} type="button" onClick={() => openRouteDialog(newRouteDialog(groups))}>新增路由模型</button>}
        secondaryActions={<RefreshIconButton label="刷新路由模型" refreshing={loading} onClick={() => void load()} />}
      />
      {error ? <InlineFeedback tone="danger" message={`路由模型刷新失败：${error}`} /> : null}
      <MetricStrip metrics={summaryMetrics} />
      <FilterToolbar
        fields={[{
          key: 'query',
          label: '搜索路由',
          primary: true,
          minWidth: '240px',
          maxWidth: '420px',
          control: <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="名称、代码或分组" aria-label="搜索路由名称或代码" />,
        }]}
        resultSummary={`共 ${routes.length} 个路由 · 当前显示 ${visibleRoutes.length} 个`}
      />
      <InlineFeedback tone={totals.blocked ? 'warning' : 'success'} message={notice} />
      {!routes.length ? <EmptyBlock title="暂无路由模型" detail="创建路由模型后继续配置候选真实模型、可见范围和价格。" action={<button className={cn(adminButton.base, adminButton.primary)} type="button" onClick={() => openRouteDialog(newRouteDialog(groups))}>新增路由模型</button>} /> : null}
      {routes.length && !visibleRoutes.length ? <EmptyBlock title="未找到路由模型" detail="换一个名称、代码或分组关键词再试。" /> : null}
      {visibleRoutes.length ? (
        <section className={routingClasses.workspace} data-admin-routing-workspace>
          <aside className={routingClasses.master} aria-label="路由模型列表">
            <div className={routingClasses.masterHead}>
              <strong className="text-sm font-semibold text-[var(--text)]">路由模型</strong>
              <p className={routingClasses.paragraph}>选择路由后检查候选、能力与价格完整性。</p>
            </div>
            <div className={routingClasses.masterList}>
              {visibleRoutes.map((route) => {
                const routeCandidates = candidates[String(route.id)] ?? route.candidates ?? []
                const routePrices = prices.filter((price) => String(price.route_model_id) === String(route.id))
                const readiness = routeReadinessBadge({ enabled: route.enabled, visibility: route.visibility, groupCount: route.group_ids?.length ?? 0, candidates: effectiveRouteCandidates(routeCandidates, accountModels, modelAccounts), prices: routePrices })
                const active = String(route.id) === selectedRouteId
                return (
                  <button
                    key={String(route.id)}
                    className={cn(routingClasses.masterItem, active && routingClasses.masterItemActive)}
                    type="button"
                    aria-pressed={active}
                    onClick={() => setSelectedRouteId(String(route.id))}
                  >
                    <span className="flex min-w-0 items-center justify-between gap-2">
                      <strong className="truncate text-sm text-[var(--text)]">{route.name}</strong>
                      <RouteBadge badge={readiness} />
                    </span>
                    <span className={routingClasses.routeCode}>{route.code}</span>
                  </button>
                )
              })}
            </div>
          </aside>
          {selectedRoute ? (
            <RouteDetailWorkspace
              route={selectedRoute}
              groups={groups}
              candidates={candidates[String(selectedRoute.id)] ?? selectedRoute.candidates ?? []}
              prices={prices.filter((price) => String(price.route_model_id) === String(selectedRoute.id))}
              accountModels={accountModels}
              modelAccounts={modelAccounts}
              onEditRoute={() => openRouteDialog(editRouteDialog(selectedRoute))}
              onAddCandidate={() => openCandidateDialog(newCandidateDialog(selectedRoute, accountModels))}
              onEditCandidate={(candidate) => openCandidateDialog(editCandidateDialog(selectedRoute, candidate))}
              onDeleteRoute={() => { setMutationError(null); setDeleteTarget({ kind: 'route', route: selectedRoute }) }}
              onDeleteCandidate={(candidate) => { setMutationError(null); setDeleteTarget({ kind: 'candidate', route: selectedRoute, candidate }) }}
            />
          ) : <EmptyBlock title="选择路由模型" detail="从左侧选择一个路由，查看候选、能力与价格状态。" />}
        </section>
      ) : null}
      {routeDialog ? (
        <Modal title={routeDialog.row ? '编辑路由模型' : '新增路由模型'} detail="保存后需要继续配置候选真实模型和价格，完成后才会对用户可用。" onClose={closeRouteDialog} footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={saving} onClick={closeRouteDialog}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={saving || !routeDialog.code || !routeDialog.name} onClick={() => void saveRoute()}>{saving ? '保存中...' : '保存并继续配置候选'}</button></>}>
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
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
        <Modal title={candidateDialog.row ? '编辑候选真实模型' : '新增候选真实模型'} detail={candidateDialog.route.name} onClose={closeCandidateDialog} footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={saving} onClick={closeCandidateDialog}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={saving || !candidateDialog.accountModelId} onClick={() => void saveCandidate()}>{saving ? '保存中...' : '保存'}</button></>}>
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <div className={adminPage.formGrid}>
            <Field label="真实模型"><select value={candidateDialog.accountModelId} onChange={(event) => setCandidateDialog({ ...candidateDialog, accountModelId: event.target.value })}>{accountModels.map((model) => <option key={String(model.id)} value={String(model.id)}>{model.account_name ? `${model.account_name} / ` : ''}{model.model_code}</option>)}</select></Field>
            <Field label={routingFieldLabels.priority} hint={routingFieldHints.priority}><input type="number" min="1" value={candidateDialog.priority} onChange={(event) => setCandidateDialog({ ...candidateDialog, priority: event.target.value })} /></Field>
            <Field label={routingFieldLabels.weight} hint={routingFieldHints.weight}><input type="number" min="0" value={candidateDialog.weight} onChange={(event) => setCandidateDialog({ ...candidateDialog, weight: event.target.value })} /></Field>
            <Field label={routingFieldLabels.fallbackOrder} hint={routingFieldHints.fallbackOrder}><input type="number" min="1" value={candidateDialog.fallbackOrder} onChange={(event) => setCandidateDialog({ ...candidateDialog, fallbackOrder: event.target.value })} /></Field>
            <Field label="状态"><select value={candidateDialog.enabled ? 'enabled' : 'disabled'} onChange={(event) => setCandidateDialog({ ...candidateDialog, enabled: event.target.value === 'enabled' })}>{routeEnabledOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
          </div>
        </Modal>
      ) : null}
      {deleteTarget ? (
        <Modal
          title={deleteTarget.kind === 'route' ? '删除路由模型' : '删除候选模型'}
          detail={deleteTarget.kind === 'route' ? deleteTarget.route.name : `${deleteTarget.route.name} / ${deleteTarget.candidate.model_code || deleteTarget.candidate.id}`}
          onClose={() => { if (!deleting) setDeleteTarget(null) }}
          footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={deleting} onClick={() => setDeleteTarget(null)}>取消</button><button className={cn(adminButton.base, adminButton.danger)} type="button" disabled={deleting} onClick={() => void confirmDelete()}>{deleting ? '删除中...' : '确认删除'}</button></>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <p className="m-0 text-sm leading-6 text-[var(--muted)]">删除后配置不再参与新请求，历史调用继续使用任务中保存的路由与价格快照。</p>
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

function RouteDetailWorkspace({
  route,
  groups,
  candidates: routeCandidates,
  prices: routePrices,
  accountModels,
  modelAccounts,
  onEditRoute,
  onAddCandidate,
  onEditCandidate,
  onDeleteRoute,
  onDeleteCandidate,
}: {
  route: RouteModel
  groups: UserGroup[]
  candidates: RouteModelCandidate[]
  prices: RouteModelPrice[]
  accountModels: ModelAccountModel[]
  modelAccounts: ModelAccount[]
  onEditRoute: () => void
  onAddCandidate: () => void
  onEditCandidate: (candidate: RouteModelCandidate) => void
  onDeleteRoute: () => void
  onDeleteCandidate: (candidate: RouteModelCandidate) => void
}) {
  const readiness = routeReadinessBadge({
    enabled: route.enabled,
    visibility: route.visibility,
    groupCount: route.group_ids?.length ?? 0,
    candidates: effectiveRouteCandidates(routeCandidates, accountModels, modelAccounts),
    prices: routePrices,
  })
  const enabledPrices = routePrices.filter((price) => price.enabled)
  return (
    <article className={routingClasses.detail} aria-label={`${route.name} 路由详情`}>
      <header className={routingClasses.detailHead}>
        <div className="grid min-w-0 gap-1">
          <span className="flex flex-wrap items-center gap-2">
            <strong className="text-base font-semibold text-[var(--text)]">{route.name}</strong>
            <RouteBadge badge={readiness} />
          </span>
          <code className={routingClasses.routeCode}>{route.code}</code>
          <p className={routingClasses.paragraph}>{route.description || '暂无路由描述。'}</p>
        </div>
        <div className={routingClasses.actionRow}>
          <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={onEditRoute}>编辑路由</button>
          <TooltipIconButton label={`删除路由 ${route.code}`} onClick={onDeleteRoute}><Trash2 /></TooltipIconButton>
          <button className={cn(adminButton.base, adminButton.primary, adminButton.small)} type="button" onClick={onAddCandidate}>配置候选</button>
        </div>
      </header>

      <section className={routingClasses.statusGrid} aria-label="路由完整性">
        <StatusFact label="可见范围" value={<RouteBadge badge={routeVisibilityBadge(route.visibility)} />} detail={routeGroupNames(route.group_ids, groups)} />
        <StatusFact label="候选模型" value={routeCandidateSummary(routeCandidates)} detail={`${routeCandidates.filter((candidate) => candidate.enabled).length} 个可参与路由`} />
        <StatusFact label="价格配置" value={`${enabledPrices.length} 个启用价格`} detail={routePrices.length ? `共 ${routePrices.length} 条价格策略` : '尚未配置价格'} />
        <StatusFact label="排序顺序" value={<code className={adminDataGrid.code}>{String(route.sort_order)}</code>} detail="数值越小展示越靠前" />
      </section>

      {readiness.state !== 'ready' ? (
        <InlineFeedback
          tone={readiness.tone === 'danger' ? 'danger' : 'warning'}
          message={routeReadinessMessage(route, readiness.state)}
        />
      ) : null}

      <div className="my-4 flex flex-wrap gap-2">
        {readiness.state === 'missing_candidate' ? <button className={cn(adminButton.base, adminButton.primary, adminButton.small)} type="button" onClick={onAddCandidate}>配置候选</button> : null}
        {readiness.state === 'missing_price' ? <a className={cn(adminButton.base, adminButton.primary, adminButton.small)} href="#/pricing">配置价格</a> : null}
        {readiness.state === 'disabled' ? <button className={cn(adminButton.base, adminButton.primary, adminButton.small)} type="button" onClick={onEditRoute}>{route.enabled ? '调整可见性' : '启用路由'}</button> : null}
        <a className={cn(adminButton.base, adminButton.ghost, adminButton.small)} href="#/pricing">查看价格</a>
      </div>

      <section className="grid gap-3 border-t border-[var(--border)] pt-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <strong className="text-sm font-semibold text-[var(--text)]">候选与模型能力</strong>
            <p className={routingClasses.paragraph}>按优先级、权重和兜底顺序执行；能力来自真实模型配置。</p>
          </div>
          <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={onAddCandidate}>新增候选</button>
        </div>
        <CandidatePanel candidates={routeCandidates} accountModels={accountModels} modelAccounts={modelAccounts} onAddCandidate={onAddCandidate} onEditCandidate={onEditCandidate} onDeleteCandidate={onDeleteCandidate} />
      </section>
    </article>
  )
}

function StatusFact({ label, value, detail }: { label: string; value: React.ReactNode; detail: string }) {
  return (
    <div className={routingClasses.statusRow}>
      <span className={routingClasses.statusLabel}>{label}</span>
      <strong className="text-sm font-semibold text-[var(--text)]">{value}</strong>
      <span className={routingClasses.paragraph}>{detail}</span>
    </div>
  )
}

function CandidatePanel({
  candidates,
  accountModels,
  modelAccounts,
  onAddCandidate,
  onEditCandidate,
  onDeleteCandidate,
}: {
  candidates: RouteModelCandidate[]
  accountModels: ModelAccountModel[]
  modelAccounts: ModelAccount[]
  onAddCandidate: () => void
  onEditCandidate: (candidate: RouteModelCandidate) => void
  onDeleteCandidate: (candidate: RouteModelCandidate) => void
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
            <th>模型能力</th>
            <th>状态</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          {candidates.map((candidate) => {
            const model = accountModels.find((item) => String(item.id) === String(candidate.account_model_id))
            const account = modelAccounts.find((item) => String(item.id) === String(model?.account_id))
            const effective = candidate.enabled && Boolean(model?.enabled) && Boolean(account && account.status === 'enabled')
            return (
            <tr key={String(candidate.id)} className="border-b border-[var(--border)] last:border-b-0 hover:bg-[var(--surface)]">
              <td className={routingClasses.candidateTd}><span className={routingClasses.candidateName}>{candidate.account_name || '未命名账号'}</span></td>
              <td className={routingClasses.candidateTd}><span className={routingClasses.candidateCode}>{candidate.model_code || routeCandidateLabel(candidate)}</span></td>
              <td className={routingClasses.candidateTd}><code className={adminDataGrid.code}>{candidate.priority}</code></td>
              <td className={routingClasses.candidateTd}><code className={adminDataGrid.code}>{candidate.weight}</code></td>
              <td className={routingClasses.candidateTd}><code className={adminDataGrid.code}>{candidate.fallback_order}</code></td>
              <td className={routingClasses.candidateTd}>{candidateCapabilitySummary(model)}</td>
              <td className={routingClasses.candidateTd}><Badge tone={effective ? 'success' : 'warning'}>{effective ? '可路由' : candidate.enabled ? '底层不可用' : '候选停用'}</Badge></td>
              <td className={routingClasses.candidateTd}>
                <span className={routingClasses.actionRow}><button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => onEditCandidate(candidate)}>编辑</button><TooltipIconButton label={`删除候选 ${candidate.model_code || candidate.id}`} onClick={() => onDeleteCandidate(candidate)}><Trash2 /></TooltipIconButton></span>
              </td>
            </tr>
          )})}
        </tbody>
      </table>
    </div>
  )
}

function candidateCapabilitySummary(model?: ModelAccountModel) {
  if (!model) return '能力配置不可用'
  const tasks = model.task_types ?? []
  const resolutions = model.base_resolution ?? []
  const formats = model.output_format ?? []
  const compression = model.supports_output_compression ? '支持压缩' : '不支持压缩'
  return `${tasks.length ? `${tasks.length} 类任务` : '缺任务类型'} · ${resolutions.length ? resolutions.join('/') : '缺分辨率'} · ${formats.length ? formats.join('/') : '缺输出格式'} · ${compression}`
}

function routeReadinessMessage(route: RouteModel, state: ReturnType<typeof routeReadinessBadge>['state']) {
  if (state === 'missing_candidate') return '当前路由缺少启用候选，配置候选后才能接收用户请求。'
  if (state === 'missing_price') return '当前路由缺少启用价格，生成请求会被计费校验阻断。'
  if (route.visibility === 'hidden') return '当前路由已隐藏，不会向用户展示或接收请求。'
  if (route.visibility === 'groups' && !route.group_ids?.length) return '当前路由尚未绑定可见分组，不会对任何用户生效。'
  return '当前路由已停用，不会向用户展示或接收请求。'
}

function effectiveRouteCandidates(candidates: RouteModelCandidate[], models: ModelAccountModel[], accounts: ModelAccount[]) {
  return candidates.map((candidate) => {
    const model = models.find((item) => String(item.id) === String(candidate.account_model_id))
    const account = accounts.find((item) => String(item.id) === String(model?.account_id))
    const accountEnabled = account ? account.status === 'enabled' : false
    return { enabled: candidate.enabled && Boolean(model?.enabled) && accountEnabled }
  })
}
