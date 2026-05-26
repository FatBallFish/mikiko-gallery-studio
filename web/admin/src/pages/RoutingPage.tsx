import { useEffect, useMemo, useState } from 'react'
import type { ID, ModelAccountModel, RouteModel, RouteModelCandidate, RouteModelVisibility, UserGroup } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, Field, GroupOptionGrid, InlineFeedback, LoadingBlock, Modal, PageHeader } from '../components'

type RouteDialog = { row?: RouteModel; code: string; name: string; description: string; visibility: RouteModelVisibility; enabled: boolean; sortOrder: string; groupIds: string[] }
type CandidateDialog = { route: RouteModel; row?: RouteModelCandidate; accountModelId: string; priority: string; weight: string; fallbackOrder: string; enabled: boolean }

export function RoutingPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [routes, setRoutes] = useState<RouteModel[]>([])
  const [groups, setGroups] = useState<UserGroup[]>([])
  const [accountModels, setAccountModels] = useState<ModelAccountModel[]>([])
  const [candidates, setCandidates] = useState<Record<string, RouteModelCandidate[]>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState('路由模型可设置 public / groups / hidden，并绑定多个真实模型候选。')
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
    <section className="page-stack">
      <PageHeader eyebrow="Route Models" title="路由模型" detail="用户可见 Basic / Plus / Pro 等路由模型在这里绑定可见分组和真实上游模型候选。" actions={<><button className="ghost" type="button" onClick={() => void load()}>刷新</button><button className="btn primary" type="button" onClick={() => setRouteDialog(newRouteDialog(groups))}>新增路由模型</button></>} />
      <section className="ops-status-strip compact-strip">
        <div className="status-cell"><label>路由模型</label><strong>{routes.length}</strong></div>
        <div className="status-cell"><label>启用</label><strong>{totals.enabled}</strong></div>
        <div className="status-cell"><label>分组可见</label><strong>{totals.groupVisible}</strong></div>
        <div className="status-cell"><label>候选模型</label><strong>{totals.candidateCount}</strong></div>
      </section>
      <section className="pg-admin-card ops-surface full-main">
        <section className="main-lane table-lane no-divider">
          <InlineFeedback tone="neutral" message={notice} />
          {!routes.length ? <EmptyBlock title="暂无路由模型" detail="创建 Basic / Plus / Pro 后配置候选真实模型。" /> : null}
          <div className="table-head route-model-grid"><span>路由模型</span><span>可见性</span><span>分组</span><span>候选</span><span>状态</span><span>操作</span></div>
          {routes.map((row) => {
            const routeCandidates = candidates[String(row.id)] ?? row.candidates ?? []
            return (
              <div key={String(row.id)} className="table-row route-model-grid">
                <div><strong>{row.name}</strong><p>{row.code} · {row.description || '无描述'}</p></div>
                <Badge tone={row.visibility === 'public' ? 'success' : row.visibility === 'groups' ? 'primary' : 'warning'}>{row.visibility}</Badge>
                <span>{groupNames(row.group_ids, groups)}</span>
                <span>{routeCandidates.length} 个 · {routeCandidates.filter((item) => item.enabled).length} 启用</span>
                <Badge tone={row.enabled ? 'success' : 'warning'}>{row.enabled ? '启用' : '停用'}</Badge>
                <div className="row-actions buttons">
                  <button className="ghost small" type="button" onClick={() => setRouteDialog(editRouteDialog(row))}>编辑</button>
                  <button className="ghost small" type="button" onClick={() => setCandidateDialog(newCandidateDialog(row, accountModels))}>加候选</button>
                </div>
              </div>
            )
          })}
          <div className="lane-divider" />
          <div className="table-head candidate-grid"><span>路由</span><span>真实模型</span><span>Priority</span><span>Weight</span><span>Fallback</span><span>操作</span></div>
          {routes.flatMap((route) => (candidates[String(route.id)] ?? []).map((candidate) => (
            <div key={`${route.id}-${candidate.id}`} className="table-row candidate-grid">
              <strong>{route.code}</strong>
              <span>{candidate.account_name ? `${candidate.account_name} / ` : ''}{candidate.model_code ?? candidate.account_model_id}</span>
              <code>{candidate.priority}</code>
              <code>{candidate.weight}</code>
              <div className="row-actions buttons"><code>{candidate.fallback_order}</code><Badge tone={candidate.enabled ? 'success' : 'warning'}>{candidate.enabled ? '启用' : '停用'}</Badge></div>
              <button className="ghost small" type="button" onClick={() => setCandidateDialog(editCandidateDialog(route, candidate))}>编辑</button>
            </div>
          )))}
        </section>
      </section>
      {routeDialog ? (
        <Modal title={routeDialog.row ? '编辑路由模型' : '新增路由模型'} detail="hidden 不会出现在用户工作台；groups 需要至少绑定一个分组。" onClose={() => setRouteDialog(null)} footer={<><button className="ghost" type="button" disabled={saving} onClick={() => setRouteDialog(null)}>取消</button><button className="btn primary" type="button" disabled={saving || !routeDialog.code || !routeDialog.name} onClick={() => void saveRoute()}>{saving ? '保存中...' : '保存'}</button></>}>
          <div className="form-grid">
            <Field label="Code"><input value={routeDialog.code} onChange={(event) => setRouteDialog({ ...routeDialog, code: event.target.value })} placeholder="basic" /></Field>
            <Field label="名称"><input value={routeDialog.name} onChange={(event) => setRouteDialog({ ...routeDialog, name: event.target.value })} /></Field>
            <Field label="描述"><input value={routeDialog.description} onChange={(event) => setRouteDialog({ ...routeDialog, description: event.target.value })} /></Field>
            <Field label="可见性"><select value={routeDialog.visibility} onChange={(event) => setRouteDialog({ ...routeDialog, visibility: event.target.value, groupIds: event.target.value === 'groups' ? routeDialog.groupIds : [] })}><option value="public">public</option><option value="groups">groups</option><option value="hidden">hidden</option></select></Field>
            {routeDialog.visibility === 'groups' ? (
              <Field label="可见分组"><GroupOptionGrid selected={routeDialog.groupIds} groups={groups} onChange={(groupIds) => setRouteDialog({ ...routeDialog, groupIds })} /></Field>
            ) : null}
            <Field label="排序"><input type="number" value={routeDialog.sortOrder} onChange={(event) => setRouteDialog({ ...routeDialog, sortOrder: event.target.value })} /></Field>
            <Field label="状态"><select value={routeDialog.enabled ? 'enabled' : 'disabled'} onChange={(event) => setRouteDialog({ ...routeDialog, enabled: event.target.value === 'enabled' })}><option value="enabled">启用</option><option value="disabled">停用</option></select></Field>
          </div>
        </Modal>
      ) : null}
      {candidateDialog ? (
        <Modal title={candidateDialog.row ? '编辑候选真实模型' : '新增候选真实模型'} detail={candidateDialog.route.name} onClose={() => setCandidateDialog(null)} footer={<><button className="ghost" type="button" disabled={saving} onClick={() => setCandidateDialog(null)}>取消</button><button className="btn primary" type="button" disabled={saving || !candidateDialog.accountModelId} onClick={() => void saveCandidate()}>{saving ? '保存中...' : '保存'}</button></>}>
          <div className="form-grid">
            <Field label="真实模型"><select value={candidateDialog.accountModelId} onChange={(event) => setCandidateDialog({ ...candidateDialog, accountModelId: event.target.value })}>{accountModels.map((model) => <option key={String(model.id)} value={String(model.id)}>{model.account_name ? `${model.account_name} / ` : ''}{model.model_code}</option>)}</select></Field>
            <Field label="优先级" hint="数值越小越先尝试；同优先级时再参考 fallback 顺序。"><input type="number" min="1" value={candidateDialog.priority} onChange={(event) => setCandidateDialog({ ...candidateDialog, priority: event.target.value })} /></Field>
            <Field label="权重" hint="同一优先级内的流量占比，100 表示默认满权重。"><input type="number" min="0" value={candidateDialog.weight} onChange={(event) => setCandidateDialog({ ...candidateDialog, weight: event.target.value })} /></Field>
            <Field label="Fallback 顺序" hint="候选失败后的兜底顺序，数值越小越早兜底。"><input type="number" min="1" value={candidateDialog.fallbackOrder} onChange={(event) => setCandidateDialog({ ...candidateDialog, fallbackOrder: event.target.value })} /></Field>
            <Field label="状态"><select value={candidateDialog.enabled ? 'enabled' : 'disabled'} onChange={(event) => setCandidateDialog({ ...candidateDialog, enabled: event.target.value === 'enabled' })}><option value="enabled">启用</option><option value="disabled">停用</option></select></Field>
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

function groupNames(ids: ID[] | undefined, groups: UserGroup[]) {
  if (!ids?.length) return '-'
  const names = ids.map((id) => groups.find((group) => String(group.id ?? group.code) === String(id))?.name ?? String(id))
  return names.join(', ')
}
