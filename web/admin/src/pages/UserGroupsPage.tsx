import { FormEvent, useEffect, useState } from 'react'
import type { UserGroup, UserGroupWriteRequest } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, Field, LoadingBlock, Modal, PageHeader } from '../components'
import { userGroupRows, userGroupSummary } from './userGroupRows'

type GroupAction = { row?: UserGroup; draft: UserGroupWriteRequest }

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
    <section className="page-stack">
      <PageHeader
        eyebrow="Groups"
        title="分组管理"
        detail="维护用户权益分组、倍率、排序、默认分组和状态。"
        actions={<button className="btn primary" type="button" onClick={() => setAction({ draft: blankGroupDraft(groups.length + 1) })}>新增分组</button>}
      />
      <section className="ops-status-strip compact-strip">
        <div className="status-cell"><label>分组总数</label><strong>{summary.total}</strong></div>
        <div className="status-cell"><label>启用</label><strong>{summary.enabled}</strong></div>
        <div className="status-cell"><label>默认</label><strong>{summary.defaultName}</strong></div>
        <div className="status-cell"><label>最高倍率</label><strong>{summary.highestMultiplier}x</strong></div>
      </section>
      <section className="pg-admin-card ops-surface full-main">
        <section className="main-lane table-lane no-divider">
          {!groups.length ? <EmptyBlock title="暂无分组" detail="新增分组后可用于用户归属和路由模型可见性。" /> : (
            <div className="admin-data-grid user-group-grid">
              <div className="table-head"><span>分组名称</span><span>倍率</span><span>排序</span><span>默认</span><span>状态</span><span>操作</span></div>
              {userGroupRows(groups).map((row) => (
                <div key={row.id} className="table-row">
                  <div><strong>{row.name}</strong><p>{row.code} · {row.description}</p></div>
                  <code>{row.multiplier}</code>
                  <code>{row.sortOrder}</code>
                  <Badge tone={row.defaultTone}>{row.defaultLabel}</Badge>
                  <Badge tone={row.statusTone}>{row.statusLabel}</Badge>
                  <button className="ghost small" type="button" onClick={() => {
                    const group = groups.find((item) => String(item.id ?? item.code) === row.id)
                    if (group) setAction({ row: group, draft: groupToDraft(group) })
                  }}>编辑</button>
                </div>
              ))}
            </div>
          )}
        </section>
      </section>
      {action ? (
        <Modal
          title={action.row ? '编辑权益分组' : '新增权益分组'}
          detail="权益分组可用于路由模型可见性和倍率择优。"
          onClose={() => setAction(null)}
          footer={<><button type="button" className="ghost" disabled={saving} onClick={() => setAction(null)}>取消</button><button type="button" className="btn primary" disabled={saving || !canSave(action)} onClick={() => void save()}>{saving ? '保存中...' : '保存'}</button></>}
        >
          <GroupForm action={action} onChange={setAction} />
        </Modal>
      ) : null}
    </section>
  )
}

function GroupForm({ action, onChange }: { action: GroupAction; onChange: (action: GroupAction) => void }) {
  return (
    <form className="form-grid" onSubmit={(event: FormEvent) => event.preventDefault()}>
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

function canSave(action: GroupAction) {
  return Boolean(action.draft.code.trim() && action.draft.name.trim() && action.draft.multiplier.trim())
}

function blankGroupDraft(index: number): UserGroupWriteRequest {
  return { code: `group_${index}`, name: `权益分组 ${index}`, multiplier: '1.00000', status: 'enabled', sort_order: index * 10, is_default: false, description: '' }
}

function groupToDraft(group: UserGroup): UserGroupWriteRequest {
  return { code: group.code, name: group.name, multiplier: group.multiplier, status: group.status, sort_order: group.sort_order ?? 0, is_default: group.is_default ?? false, description: group.description ?? '' }
}
