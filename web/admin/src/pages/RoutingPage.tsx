import { useEffect, useMemo, useState } from 'react'
import type { ModelAccountModel, RouteModel, RouteModelCandidate, RouteModelVisibility, UserGroup } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, Field, GroupOptionGrid, InlineFeedback, LoadingBlock, Modal, PageHeader, StatusCell, StatusStrip } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { adminDataGrid, adminGridCols } from '../ui/dataGrid'
import {
  routeCandidateLabel,
  routeCandidateSummary,
  routeEnabledBadge,
  routeEnabledOptions,
  routingFieldHints,
  routingFieldLabels,
  routeGroupNames,
  routeVisibilityBadge,
  routeVisibilityOptions,
} from './routingRows'

type RouteDialog = { row?: RouteModel; code: string; name: string; description: string; visibility: RouteModelVisibility; enabled: boolean; sortOrder: string; groupIds: string[] }
type CandidateDialog = { route: RouteModel; row?: RouteModelCandidate; accountModelId: string; priority: string; weight: string; fallbackOrder: string; enabled: boolean }

const routingClasses = {
  actionRow: 'flex flex-wrap items-center gap-2',
  surface: adminPage.fullSurface,
  stackCell: cn(adminDataGrid.stackCell, 'gap-0.5'),
  paragraph: 'm-0 text-xs text-[var(--soft)] [overflow-wrap:anywhere]',
  textCell: 'min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-[var(--soft)]',
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
      <PageHeader eyebrow="Route Models" title="路由模型" detail="用户可见 Basic / Plus / Pro 等路由模型在这里绑定可见分组和真实上游模型候选。" actions={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" onClick={() => void load()}>刷新</button><button className={cn(adminButton.base, adminButton.primary)} type="button" onClick={() => setRouteDialog(newRouteDialog(groups))}>新增路由模型</button></>} />
      <StatusStrip columns={4}>
        <StatusCell label="路由模型" value={routes.length} />
        <StatusCell label="启用" value={totals.enabled} />
        <StatusCell label="分组可见" value={totals.groupVisible} />
        <StatusCell label="候选模型" value={totals.candidateCount} />
      </StatusStrip>
      <section className={routingClasses.surface}>
        <section className={adminPage.mainLane}>
          <InlineFeedback tone="neutral" message={notice} />
          {!routes.length ? <EmptyBlock title="暂无路由模型" detail="创建 Basic / Plus / Pro 后配置候选真实模型。" /> : null}
          {routes.length ? (
            <div className={adminDataGrid.root}>
              <div className={cn(adminDataGrid.head, adminGridCols.routeModel)}><span>路由模型</span><span>可见性</span><span>分组</span><span>候选</span><span>状态</span><span>操作</span></div>
              {routes.map((row) => {
                const routeCandidates = candidates[String(row.id)] ?? row.candidates ?? []
                return (
                  <div key={String(row.id)} className={cn(adminDataGrid.row, adminGridCols.routeModel)}>
                    <div className={routingClasses.stackCell}><strong>{row.name}</strong><p className={routingClasses.paragraph}>{row.code} · {row.description || '无描述'}</p></div>
                    <RouteBadge badge={routeVisibilityBadge(row.visibility)} />
                    <span className={routingClasses.textCell}>{routeGroupNames(row.group_ids, groups)}</span>
                    <span className={routingClasses.textCell}>{routeCandidateSummary(routeCandidates)}</span>
                    <RouteBadge badge={routeEnabledBadge(row.enabled)} />
                    <div className={routingClasses.actionRow}>
                      <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => setRouteDialog(editRouteDialog(row))}>编辑</button>
                      <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => setCandidateDialog(newCandidateDialog(row, accountModels))}>加候选</button>
                    </div>
                  </div>
                )
              })}
            </div>
          ) : null}
          {routes.length ? <div className="my-4 h-px bg-[var(--line)]" /> : null}
          {routes.length ? (
            <div className={adminDataGrid.root}>
              <div className={cn(adminDataGrid.head, adminGridCols.candidate)}><span>路由</span><span>真实模型</span><span>{routingFieldLabels.priority}</span><span>{routingFieldLabels.weight}</span><span>{routingFieldLabels.fallbackOrder}</span><span>操作</span></div>
              {routes.flatMap((route) => (candidates[String(route.id)] ?? []).map((candidate) => (
                <div key={`${route.id}-${candidate.id}`} className={cn(adminDataGrid.row, adminGridCols.candidate)}>
                  <strong>{route.code}</strong>
                  <span className={routingClasses.textCell}>{routeCandidateLabel(candidate)}</span>
                  <code className={adminDataGrid.code}>{candidate.priority}</code>
                  <code className={adminDataGrid.code}>{candidate.weight}</code>
                  <div className={routingClasses.actionRow}><code className={adminDataGrid.code}>{candidate.fallback_order}</code><RouteBadge badge={routeEnabledBadge(candidate.enabled)} /></div>
                  <button className={cn(adminButton.base, adminButton.ghost, adminButton.small)} type="button" onClick={() => setCandidateDialog(editCandidateDialog(route, candidate))}>编辑</button>
                </div>
              )))}
            </div>
          ) : null}
        </section>
      </section>
      {routeDialog ? (
        <Modal title={routeDialog.row ? '编辑路由模型' : '新增路由模型'} detail="隐藏不会出现在用户工作台；按分组可见需要至少绑定一个分组。" onClose={() => setRouteDialog(null)} footer={<><button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={saving} onClick={() => setRouteDialog(null)}>取消</button><button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={saving || !routeDialog.code || !routeDialog.name} onClick={() => void saveRoute()}>{saving ? '保存中...' : '保存'}</button></>}>
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
