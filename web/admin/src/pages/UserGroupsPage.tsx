import { FormEvent, useEffect, useMemo, useState } from 'react'
import type { AdminMetric, UserGroup, UserGroupWriteRequest } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { ActionMenu, Badge, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, MetricStrip, Modal, PageHeader } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import type { ColumnDef } from '../ui/dataTable'
import { DataTable, FilterToolbar, ListPage } from '../ui/dataTable'
import { XIcon } from '../ui/listIcons'
import { userGroupRows, userGroupStatusTone, userGroupSummary } from './userGroupRows'

type GroupAction = { row?: UserGroup; draft: UserGroupWriteRequest }
type GroupFilters = { query: string; status: string }

const initialFilters: GroupFilters = { query: '', status: '' }
const groupClasses = {
  identity: 'grid min-w-0 gap-1',
  code: 'font-[family-name:var(--admin-font-mono)] text-xs font-semibold text-[var(--accent)]',
  name: 'truncate font-semibold text-[var(--fg)]',
  desc: 'max-w-[480px] text-xs leading-5 text-[var(--muted)] [overflow-wrap:anywhere]',
  number: 'font-[family-name:var(--admin-font-mono)] text-sm font-semibold tabular-nums text-[var(--fg)]',
  actions: 'flex flex-wrap items-center justify-end gap-2',
}

export function UserGroupsPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [groups, setGroups] = useState<UserGroup[]>([])
  const [filters, setFilters] = useState<GroupFilters>(initialFilters)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [action, setAction] = useState<GroupAction | null>(null)
  const [mutationError, setMutationError] = useState<string | null>(null)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      setGroups(await adminApi.listUserGroups())
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '分组载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const summary = useMemo(() => userGroupSummary(groups), [groups])
  const metrics = useMemo<AdminMetric[]>(() => [
    { label: '分组总数', value: String(summary.total), trend: `${summary.enabled} 个启用`, tone: 'neutral' },
    { label: '默认分组', value: summary.defaultName, trend: '新用户默认权益', tone: 'good' },
    { label: '最高倍率', value: `${summary.highestMultiplier}x`, trend: '当前配置上限', tone: 'neutral' },
    { label: '停用分组', value: String(Math.max(0, summary.total - summary.enabled)), trend: '不参与新权益分配', tone: summary.total > summary.enabled ? 'warn' : 'good' },
  ], [summary])
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
    setMutationError(null)
    try {
      await adminApi.deleteUserGroup(group.code)
      onFeedback('权益分组已删除', group.name)
      await load()
    } catch (caught) {
      setMutationError(caught instanceof Error ? caught.message : '权益分组删除失败')
    }
  }

  if (loading) return <LoadingBlock label="载入分组列表" />
  if (error) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className={adminPage.stack}>
      <PageHeader
        title="用户分组"
        description="维护用户权益分组，并查看分组对模型可见性和计费倍率的影响。"
        primaryAction={<button className={cn(adminButton.base, adminButton.primary)} type="button" onClick={() => openAction({ draft: blankGroupDraft(groups.length + 1) })}>添加用户分组</button>}
      />
      <MetricStrip metrics={metrics} />
      {mutationError && !action ? <InlineFeedback tone="danger" message={mutationError} /> : null}
      <ListPage
        filters={(
          <FilterToolbar
            fields={[
              { key: 'query', label: '搜索分组', primary: true, minWidth: '220px', maxWidth: '420px', control: <input value={filters.query} onChange={(event) => setFilters({ ...filters, query: event.target.value })} placeholder="代码、名称或描述" /> },
              { key: 'status', label: '状态', primary: true, control: <select value={filters.status} onChange={(event) => setFilters({ ...filters, status: event.target.value })}><option value="">全部状态</option><option value="enabled">启用</option><option value="disabled">停用</option></select> },
            ]}
            actions={<button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={() => setFilters(initialFilters)}><XIcon className="size-4" /><span>清空</span></button>}
            resultSummary={`共 ${groups.length} 个分组 · 当前显示 ${visibleGroups.length} 个`}
          />
        )}
      >
        <DataTable
          columns={groupColumns(openAction, deleteGroup)}
          rows={visibleGroups}
          rowKey={(group) => String(group.id ?? group.code)}
          empty={<EmptyBlock title="没有匹配的用户分组" detail="清空筛选，或新增一个权益分组。" />}
        />
      </ListPage>
      {action ? (
        <Modal
          title={action.row ? '编辑权益分组' : '新增权益分组'}
          detail="权益分组可用于路由模型可见性和倍率择优。"
          onClose={closeAction}
          footer={<><button type="button" className={cn(adminButton.base, adminButton.ghost)} disabled={saving} onClick={closeAction}>取消</button><button type="button" className={cn(adminButton.base, adminButton.primary)} disabled={saving || !canSave(action)} onClick={() => void save()}>{saving ? '保存中...' : '保存'}</button></>}
        >
          {mutationError ? <InlineFeedback tone="danger" message={mutationError} /> : null}
          <GroupForm action={action} onChange={setAction} />
        </Modal>
      ) : null}
    </section>
  )
}

function GroupForm({ action, onChange }: { action: GroupAction; onChange: (action: GroupAction) => void }) {
  return (
    <form className={adminPage.formGrid} onSubmit={(event: FormEvent) => event.preventDefault()}>
      <Field label="分组代码"><input value={action.draft.code} onChange={(event) => onChange({ ...action, draft: { ...action.draft, code: event.target.value } })} /></Field>
      <Field label="分组名称"><input value={action.draft.name} onChange={(event) => onChange({ ...action, draft: { ...action.draft, name: event.target.value } })} /></Field>
      <Field label="计费倍率"><input value={action.draft.multiplier} onChange={(event) => onChange({ ...action, draft: { ...action.draft, multiplier: event.target.value } })} /></Field>
      <Field label="排序"><input type="number" value={action.draft.sort_order ?? 0} onChange={(event) => onChange({ ...action, draft: { ...action.draft, sort_order: Number(event.target.value) } })} /></Field>
      <Field label="状态"><select value={action.draft.status} onChange={(event) => onChange({ ...action, draft: { ...action.draft, status: event.target.value } })}><option value="enabled">启用</option><option value="disabled">停用</option></select></Field>
      <Field label="默认分组"><select value={action.draft.is_default ? 'true' : 'false'} onChange={(event) => onChange({ ...action, draft: { ...action.draft, is_default: event.target.value === 'true' } })}><option value="false">否</option><option value="true">是</option></select></Field>
      <Field label="描述"><input value={action.draft.description ?? ''} onChange={(event) => onChange({ ...action, draft: { ...action.draft, description: event.target.value } })} /></Field>
    </form>
  )
}

function groupColumns(openAction: (action: GroupAction) => void, deleteGroup: (group: UserGroup) => Promise<void>): ColumnDef<UserGroup>[] {
  return [
    {
      key: 'group', title: '分组', width: 'minmax(210px,1.6fr)', render: (group) => <span className={groupClasses.identity}><span className={groupClasses.name}>{group.name}</span><code className={groupClasses.code}>{group.code}</code></span>,
    },
    {
      key: 'description', title: '描述', width: 'minmax(240px,2fr)', render: (group) => <span className={groupClasses.desc}>{userGroupRows([group])[0]?.description}</span>,
    },
    {
      key: 'multiplier', title: '计费倍率', width: 'minmax(110px,.8fr)', align: 'right', kind: 'number', render: (group) => <span className={groupClasses.number}>{group.multiplier}x</span>,
    },
    {
      key: 'status', title: '状态', width: 'minmax(150px,1fr)', render: (group) => {
        const row = userGroupRows([group])[0]
        return <span className="flex flex-wrap gap-1.5"><Badge tone={userGroupStatusTone(group.status)}>{row?.statusLabel}</Badge><Badge tone={row?.defaultTone}>{row?.defaultLabel}</Badge></span>
      },
    },
    {
      key: 'sort', title: '排序', width: 'minmax(80px,.6fr)', align: 'right', kind: 'number', render: (group) => String(group.sort_order ?? 0),
    },
    {
      key: 'actions', title: '操作', width: 'minmax(180px,1.2fr)', align: 'right', render: (group) => <span className={groupClasses.actions}><button className={cn(adminButton.base, adminButton.secondary, adminButton.small)} type="button" onClick={() => openAction({ row: group, draft: groupToDraft(group) })}>编辑</button><ActionMenu actions={[{ id: `routing-${group.code}`, label: '配置模型可见性', run: () => { window.location.hash = '/routing' } }, { id: `delete-${group.code}`, label: '删除分组', tone: 'danger', confirm: { title: `确认删除分组 ${group.name}？` }, run: () => deleteGroup(group) }]} /></span>,
    },
  ]
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
