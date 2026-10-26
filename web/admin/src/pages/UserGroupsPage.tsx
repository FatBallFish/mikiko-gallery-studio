import { FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import type { AdminMetric, RouteModel, UserGroup, UserGroupWriteRequest } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { ActionMenu, Badge, Drawer, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, MetricStrip, Modal, PageHeader, RefreshIconButton } from '../components'
import type { ColumnDef } from '../ui/dataTable'
import { DataTable, FilterToolbar, ListPage } from '../ui/dataTable'
import { XIcon } from '../ui/listIcons'
import { userGroupRows, userGroupStatusTone, userGroupSummary } from './userGroupRows'
import { createLatestListRequestGuard } from './listRefresh'
import './list-redesign.css'

type GroupAction = { row?: UserGroup; draft: UserGroupWriteRequest }
type GroupFilters = { query: string; status: string }

const initialFilters: GroupFilters = { query: '', status: '' }

export function UserGroupsPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [groups, setGroups] = useState<UserGroup[]>([])
  const [routes, setRoutes] = useState<RouteModel[]>([])
  const [routesLoaded, setRoutesLoaded] = useState(false)
  const [filters, setFilters] = useState<GroupFilters>(initialFilters)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [action, setAction] = useState<GroupAction | null>(null)
  const [mutationError, setMutationError] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<UserGroup | null>(null)
  const requestGuard = useRef(createLatestListRequestGuard()).current

  const load = async () => {
    const request = requestGuard.begin()
    setLoading(true)
    setError(null)
    try {
      const [nextGroups, nextRoutes] = await Promise.all([
        adminApi.listUserGroups(),
        adminApi.listRouteModels({ page: 1, page_size: 500 }).catch(() => null),
      ])
      if (!requestGuard.isCurrent(request)) return
      setGroups(nextGroups)
      if (nextRoutes) {
        setRoutes(nextRoutes)
        setRoutesLoaded(true)
      }
    } catch (caught) {
      if (!requestGuard.isCurrent(request)) return
      setError(caught instanceof Error ? caught.message : '分组载入失败')
    } finally {
      if (!requestGuard.isCurrent(request)) return
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    return () => requestGuard.invalidate()
  }, [])

  const summary = useMemo(() => userGroupSummary(groups), [groups])
  const metrics = useMemo<AdminMetric[]>(() => [
    { label: '分组总数', value: String(summary.total), trend: `${summary.enabled} 个启用 · ${Math.max(0, summary.total - summary.enabled)} 个停用`, tone: 'neutral' },
    { label: '默认分组', value: summary.defaultName, trend: '新用户自动加入', tone: 'good' },
    { label: '最高倍率', value: `${summary.highestMultiplier}x`, trend: '当前配置上限', tone: 'neutral' },
    { label: '可见路由', value: routesLoaded ? String(routes.length) : '—', trend: routesLoaded ? '已配置路由模型' : '路由数据载入失败', tone: 'neutral' },
  ], [routes.length, routesLoaded, summary])

  const visibleGroups = useMemo(() => {
    const keyword = filters.query.trim().toLowerCase()
    return groups.filter((group) => {
      const matchesStatus = !filters.status || group.status === filters.status || (filters.status === 'enabled' && group.status === 'active')
      const matchesQuery = !keyword || [group.code, group.name, group.description].some((value) => String(value ?? '').toLowerCase().includes(keyword))
      return matchesStatus && matchesQuery
    })
  }, [filters, groups])

  const openAction = (next: GroupAction) => {
    setMutationError(null)
    setAction(next)
  }

  const closeAction = () => {
    setMutationError(null)
    setAction(null)
  }

  const save = async () => {
    if (!action) return
    setSaving(true)
    setMutationError(null)
    try {
      const saved = action.row
        ? await adminApi.updateUserGroup(action.row.code, action.draft)
        : await adminApi.createUserGroup(action.draft)
      onFeedback('权益分组已保存', `${saved.name} · ${saved.multiplier}x`)
      setAction(null)
      await load()
    } catch (caught) {
      setMutationError(caught instanceof Error ? caught.message : '权益分组保存失败')
    } finally {
      setSaving(false)
    }
  }

  const deleteGroup = async (group: UserGroup) => {
    setSaving(true)
    setMutationError(null)
    try {
      await adminApi.deleteUserGroup(group.code)
      onFeedback('权益分组已删除', group.name)
      setDeleteTarget(null)
      setAction(null)
      await load()
    } catch (caught) {
      setMutationError(caught instanceof Error ? caught.message : '权益分组删除失败')
    } finally {
      setSaving(false)
    }
  }

  if (loading && !groups.length) return <LoadingBlock label="载入分组列表" />
  if (error && !groups.length) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className="a-page-stack">
      <PageHeader
        eyebrow="User Groups"
        title="用户分组"
        description="分组决定路由可见性与计费倍率。成员与可见路由直接展示，编辑在右侧抽屉完成。"
        primaryAction={<button className="a-btn a-btn--sm a-btn--primary" type="button" onClick={() => openAction({ draft: blankGroupDraft(groups.length + 1) })}>添加用户分组</button>}
        secondaryActions={<RefreshIconButton label="刷新用户分组" refreshing={loading} onClick={() => void load()} />}
      />
      {error ? <InlineFeedback tone="danger" message={`用户分组刷新失败：${error}`} /> : null}
      {loading && groups.length ? <InlineFeedback tone="neutral" message="正在刷新分组列表，当前数据会保留到新结果返回。" /> : null}
      {mutationError && !action && !deleteTarget ? <InlineFeedback tone="danger" message={mutationError} /> : null}
      <MetricStrip metrics={metrics} />
      <ListPage
        resultSummary={<><h2>分组列表</h2><p>分组代码创建后不可修改；可见路由在路由模型中维护。</p></>}
        actions={<span className="a-hint">共 {groups.length} 个分组</span>}
        filters={(
          <form onSubmit={(event) => event.preventDefault()}>
            <FilterToolbar
              fields={[
                { key: 'query', label: '搜索分组', primary: true, minWidth: '220px', maxWidth: '420px', control: <input className="a-input" value={filters.query} onChange={(event) => setFilters({ ...filters, query: event.target.value })} placeholder="分组代码或名称" /> },
                { key: 'status', label: '状态', primary: true, control: <select className="a-select" value={filters.status} onChange={(event) => setFilters({ ...filters, status: event.target.value })}><option value="">全部状态</option><option value="enabled">启用</option><option value="disabled">停用</option></select> },
              ]}
              actions={<button className="a-btn a-btn--sm a-btn--quiet" type="button" onClick={() => setFilters(initialFilters)}><XIcon className="size-4" /><span>清空</span></button>}
              resultSummary={`共 ${groups.length} 个分组 · 当前显示 ${visibleGroups.length} 个`}
            />
          </form>
        )}
      >
        <DataTable
          columns={groupColumns(routes, routesLoaded, openAction, (group) => setDeleteTarget(group))}
          rows={visibleGroups}
          rowKey={(group) => String(group.id ?? group.code)}
          empty={<EmptyBlock title="没有匹配的用户分组" detail="清空筛选，或新增一个权益分组。" />}
          minWidth={1040}
        />
      </ListPage>

      {action ? (
        <Drawer
          title={action.row ? `编辑分组 · ${action.row.name}` : '添加用户分组'}
          description={action.row ? `${action.row.code} · 创建于 ${action.row.created_at?.slice(0, 10) || '—'}` : '分组代码创建后不可修改'}
          headerAside={action.row ? <Badge tone={userGroupStatusTone(action.row.status)}>{userGroupRows([action.row])[0]?.statusLabel}</Badge> : undefined}
          onClose={() => { if (!deleteTarget) closeAction() }}
          footer={(
            <>
              {action.row ? <button className="a-btn a-btn--sm a-btn--danger" type="button" disabled={saving} onClick={() => setDeleteTarget(action.row ?? null)}>删除分组</button> : null}
              <div className="lr-spacer" />
              <button className="a-btn a-btn--sm a-btn--quiet" type="button" disabled={saving} onClick={closeAction}>取消</button>
              <button className="a-btn a-btn--sm a-btn--primary" type="button" disabled={saving || !canSave(action)} onClick={() => void save()}>{saving ? '保存中...' : '保存更改'}</button>
            </>
          )}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <GroupForm action={action} routes={routes} routesLoaded={routesLoaded} onChange={setAction} />
        </Drawer>
      ) : null}

      {deleteTarget ? (
        <Modal
          title="删除分组"
          detail={`${deleteTarget.name} · ${deleteTarget.code}`}
          onClose={() => setDeleteTarget(null)}
          footer={(
            <>
              <button className="a-btn a-btn--sm a-btn--quiet" type="button" disabled={saving} onClick={() => setDeleteTarget(null)}>取消</button>
              <button className="a-btn a-btn--sm a-btn--danger" type="button" disabled={saving} onClick={() => void deleteGroup(deleteTarget)}>{saving ? '删除中...' : '确认删除'}</button>
            </>
          )}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <div className="lr-confirm-banner">
            <strong>删除前需先转移成员</strong>
            <p>删除分组不会删除用户，但成员会失去该分组提供的模型可见性与倍率。该操作写入审计日志。</p>
          </div>
        </Modal>
      ) : null}
    </section>
  )
}

function GroupForm({ action, routes, routesLoaded, onChange }: { action: GroupAction; routes: RouteModel[]; routesLoaded: boolean; onChange: (action: GroupAction) => void }) {
  const visibleRoutes = action.row ? routes.filter((route) => routeVisibleToGroup(route, action.row!)) : []
  return (
    <form className="a-form" onSubmit={(event: FormEvent) => event.preventDefault()}>
      <section className="a-form__section">
        <div className="a-form__head"><h3><span className="a-form__num">1</span>基本信息</h3><p>代码用于 API 与持久化；名称用于界面展示。</p></div>
        <Field label="分组代码" hint="创建后不可修改。仅允许小写字母、数字与连字符。"><input className="a-input" value={action.draft.code} disabled={Boolean(action.row)} onChange={(event) => onChange({ ...action, draft: { ...action.draft, code: event.target.value } })} style={{ fontFamily: 'var(--font-admin-mono)' }} /></Field>
        <Field label="分组名称"><input className="a-input" value={action.draft.name} onChange={(event) => onChange({ ...action, draft: { ...action.draft, name: event.target.value } })} /></Field>
        <Field label="描述"><textarea className="a-textarea" value={action.draft.description ?? ''} onChange={(event) => onChange({ ...action, draft: { ...action.draft, description: event.target.value } })} placeholder="说明该分组的适用人群与权益" /></Field>
      </section>
      <section className="a-form__section">
        <div className="a-form__head"><h3><span className="a-form__num">2</span>计费与排序</h3></div>
        <div className="lr-form-pair">
          <Field label="计费倍率" hint={`按基础价格 × ${action.draft.multiplier || '1'} 计费。`}><input className="a-input" value={action.draft.multiplier} inputMode="decimal" onChange={(event) => onChange({ ...action, draft: { ...action.draft, multiplier: event.target.value } })} /></Field>
          <Field label="排序值" hint="数值越小越靠前。"><input className="a-input" type="number" value={action.draft.sort_order ?? 0} onChange={(event) => onChange({ ...action, draft: { ...action.draft, sort_order: Number(event.target.value) } })} /></Field>
        </div>
      </section>
      <section className="a-form__section">
        <div className="a-form__head"><h3><span className="a-form__num">3</span>可见范围</h3><p>成员可在创作台与 API 中看到这些路由。</p></div>
        {!action.row ? <p className="a-hint">保存分组后，可在路由模型中配置该分组的可见范围。</p> : null}
        {action.row && !routesLoaded ? <p className="a-hint">路由数据载入失败，暂时无法展示可见路由。</p> : null}
        {action.row && routesLoaded && !visibleRoutes.length ? <div className="a-banner a-banner--info"><span>i</span><div><strong style={{ fontSize: 13 }}>尚未配置可见路由</strong><div className="a-hint">在路由模型中为该分组开放路由后，成员才能使用对应模型。</div></div></div> : null}
        {visibleRoutes.length ? (
          <div className="lr-stack--sm">
            {visibleRoutes.map((route) => (
              <label key={String(route.id)} className="a-switch">
                <input type="checkbox" checked readOnly disabled />
                <span className="a-switch__track" />
                <span>{route.name} · <code className="a-mono">{route.code}</code></span>
              </label>
            ))}
          </div>
        ) : null}
        <button className="a-btn a-btn--sm" type="button" style={{ justifySelf: 'start' }} onClick={() => { window.location.hash = '/routing' }}>在路由模型中配置完整可见性</button>
      </section>
      <section className="a-form__section">
        <div className="a-form__head"><h3><span className="a-form__num">4</span>状态</h3></div>
        <label className="a-switch">
          <input type="checkbox" checked={action.draft.status !== 'disabled'} onChange={(event) => onChange({ ...action, draft: { ...action.draft, status: event.target.checked ? 'enabled' : 'disabled' } })} />
          <span className="a-switch__track" />
          <span>启用该分组</span>
        </label>
        <label className="a-switch">
          <input type="checkbox" checked={Boolean(action.draft.is_default)} onChange={(event) => onChange({ ...action, draft: { ...action.draft, is_default: event.target.checked } })} />
          <span className="a-switch__track" />
          <span>设为默认分组（新用户自动加入）</span>
        </label>
      </section>
      <div className="a-banner a-banner--info"><span>i</span><div><strong style={{ fontSize: 13 }}>停用分组不会删除用户</strong><div className="a-hint">成员会保留，但在可见范围调整前无法使用该分组的模型。</div></div></div>
    </form>
  )
}

function groupColumns(
  routes: RouteModel[],
  routesLoaded: boolean,
  openAction: (action: GroupAction) => void,
  requestDelete: (group: UserGroup) => void,
): ColumnDef<UserGroup>[] {
  return [
    {
      key: 'group',
      title: '分组',
      width: 'minmax(200px,1.5fr)',
      render: (group) => <span className="lr-cell"><strong>{group.name}</strong><span>{group.code}</span></span>,
    },
    {
      key: 'description',
      title: '描述',
      width: 'minmax(240px,2fr)',
      render: (group) => <span className="lr-muted">{userGroupRows([group])[0]?.description || '—'}</span>,
    },
    {
      key: 'multiplier',
      title: '计费倍率',
      width: 'minmax(110px,.8fr)',
      align: 'right',
      kind: 'number',
      render: (group) => <span className="lr-num">{group.multiplier}x</span>,
    },
    {
      key: 'routes',
      title: '可见路由',
      width: 'minmax(100px,.8fr)',
      align: 'right',
      kind: 'number',
      render: (group) => routesLoaded ? <span className="lr-num">{routes.filter((route) => routeVisibleToGroup(route, group)).length}</span> : <span className="lr-muted">—</span>,
    },
    {
      key: 'status',
      title: '状态',
      width: 'minmax(150px,1.1fr)',
      render: (group) => {
        const row = userGroupRows([group])[0]
        return <span className="lr-row"><Badge tone={userGroupStatusTone(group.status)}>{row?.statusLabel}</Badge>{group.is_default ? <Badge tone="primary">默认</Badge> : null}</span>
      },
    },
    {
      key: 'sort',
      title: '排序',
      width: 'minmax(80px,.6fr)',
      align: 'right',
      kind: 'number',
      render: (group) => String(group.sort_order ?? 0),
    },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(210px,1.4fr)',
      align: 'right',
      render: (group) => (
        <div className="a-row-actions">
          <button className="a-row-btn a-row-btn--primary" type="button" onClick={() => openAction({ row: group, draft: groupToDraft(group) })}>编辑</button>
          <button className="a-row-btn" type="button" onClick={() => { window.location.hash = '/routing' }}>可见路由</button>
          <ActionMenu actions={[{ id: `delete-${group.code}`, label: '删除分组', tone: 'danger', run: () => requestDelete(group) }]} />
        </div>
      ),
    },
  ]
}

function routeVisibleToGroup(route: RouteModel, group: UserGroup) {
  const groupID = String(group.id ?? group.code)
  if (route.groups?.some((item) => String(item.id ?? item.code) === groupID || item.code === group.code)) return true
  return Boolean(route.group_ids?.some((id) => String(id) === groupID || String(id) === group.code))
}

function canSave(action: GroupAction) {
  return Boolean(action.draft.code.trim() && action.draft.name.trim() && action.draft.multiplier.trim())
}

function blankGroupDraft(index: number): UserGroupWriteRequest {
  return { code: `group_${index}`, name: `权益分组 ${index}`, multiplier: '1.00000', status: 'enabled', sort_order: index * 10, is_default: false, description: '' }
}

function groupToDraft(group: UserGroup): UserGroupWriteRequest {
  return { code: group.code, name: group.name, multiplier: group.multiplier, status: group.status, sort_order: group.sort_order ?? 0, is_default: group.is_default ?? false, description: group.description ?? '' }
}