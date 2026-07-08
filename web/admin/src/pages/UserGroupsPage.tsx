import { FormEvent, useEffect, useState } from 'react'
import type { UserGroup, UserGroupWriteRequest } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { adminApi } from '../../../shared/admin-api'
import { EmptyBlock, ErrorBlock, Field, LoadingBlock, Modal, PageHeader } from '../components'
import { adminButton, adminPage } from '../ui/classes'
import { ColumnDef, DataTable } from '../ui/dataTable'
import { userGroupRows, userGroupSummary } from './userGroupRows'

type GroupAction = { row?: UserGroup; draft: UserGroupWriteRequest }
const groupClasses = {
  code: 'font-mono text-xs font-bold text-[var(--accent)]',
  name: 'font-bold text-[var(--text)]',
  desc: 'max-w-[520px] text-xs leading-relaxed text-[var(--muted-strong)]',
  routeCount: 'text-xs font-bold text-[var(--text)]',
  actionLinks: 'flex flex-wrap items-center gap-3',
  linkButton: 'text-xs font-bold text-[var(--muted)] transition-colors hover:text-[var(--text)]',
  summary: 'rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] px-5 py-4 text-sm text-[var(--muted)]',
}

export function UserGroupsPage({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [groups, setGroups] = useState<UserGroup[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [action, setAction] = useState<GroupAction | null>(null)

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

  const save = async () => {
    if (!action) return
    setSaving(true)
    try {
      const saved = action.row
        ? await adminApi.updateUserGroup(action.row.code, action.draft)
        : await adminApi.createUserGroup(action.draft)
      onFeedback('权益分组已保存', `${saved.name} · ${saved.multiplier}x`)
      setAction(null)
      await load()
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <LoadingBlock label="载入分组列表" />
  if (error) return <ErrorBlock message={error} onRetry={load} />
  const summary = userGroupSummary(groups)

  return (
    <section className={adminPage.stack}>
      <PageHeader
        title="用户分组"
        description="维护用户权益分组，并查看分组对模型可见性和计费倍率的影响。"
        primaryAction={<button className={cn(adminButton.base, adminButton.primary)} type="button" onClick={() => setAction({ draft: blankGroupDraft(groups.length + 1) })}>添加用户分组</button>}
      />
      <details className={groupClasses.summary}>
        <summary className="cursor-pointer list-none font-bold text-[var(--text)]">分组摘要</summary>
        <div className="mt-3 flex flex-wrap gap-4 text-xs">
          <span>分组总数 <strong className="text-[var(--text)]">{summary.total}</strong></span>
          <span>启用 <strong className="text-[var(--text)]">{summary.enabled}</strong></span>
          <span>默认 <strong className="text-[var(--text)]">{summary.defaultName}</strong></span>
          <span>最高倍率 <strong className="text-[var(--text)]">{summary.highestMultiplier}x</strong></span>
        </div>
      </details>
      {!groups.length ? <EmptyBlock title="暂无分组" detail="新增分组后可用于用户归属和路由模型可见性。" /> : (
        <DataTable
          columns={groupColumns(groups, setAction)}
          rows={groups}
          rowKey={(group) => String(group.id ?? group.code)}
        />
      )}
      {action ? (
        <Modal
          title={action.row ? '编辑权益分组' : '新增权益分组'}
          detail="权益分组可用于路由模型可见性和倍率择优。"
          onClose={() => setAction(null)}
          footer={<><button type="button" className={cn(adminButton.base, adminButton.ghost)} disabled={saving} onClick={() => setAction(null)}>取消</button><button type="button" className={cn(adminButton.base, adminButton.primary)} disabled={saving || !canSave(action)} onClick={() => void save()}>{saving ? '保存中...' : '保存'}</button></>}
        >
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

function groupColumns(
  groups: UserGroup[],
  setAction: (action: GroupAction) => void,
): ColumnDef<UserGroup>[] {
  return [
    {
      key: 'code',
      title: '分组代码',
      width: 'minmax(160px,1.5fr)',
      render: (group) => <span className={groupClasses.code}>{group.code}</span>,
    },
    {
      key: 'name',
      title: '分组名称',
      width: 'minmax(140px,1.2fr)',
      render: (group) => <span className={groupClasses.name}>{group.name}</span>,
    },
    {
      key: 'desc',
      title: '描述',
      width: 'minmax(220px,2fr)',
      render: (group) => {
        const row = userGroupRows([group])[0]
        return <span className={groupClasses.desc}>{row.description ? `${row.description} · 倍率 ${row.multiplier}` : `倍率 ${row.multiplier} · ${row.defaultLabel} · ${row.statusLabel}`}</span>
      },
    },
    {
      key: 'routes',
      title: '关联路由模型数',
      width: 'minmax(100px,0.8fr)',
      render: () => <span className={groupClasses.routeCount}>配置模型可见性</span>,
    },
    {
      key: 'actions',
      title: '操作',
      width: 'minmax(120px,1fr)',
      render: (group) => (
        <div className={groupClasses.actionLinks}>
          <a className={groupClasses.linkButton} href="#/routing">配置模型可见性</a>
          <button className={groupClasses.linkButton} type="button" onClick={() => setAction({ row: group, draft: groupToDraft(group) })}>编辑</button>
        </div>
      ),
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
