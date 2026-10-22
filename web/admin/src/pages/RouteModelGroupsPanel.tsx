import { useEffect, useRef, useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import type { RouteModel, RouteModelGroupRecord, RouteModelMediaType } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { builtinModelIcons, defaultIconKey, modelIconSrc, siteIcon } from '../../../shared/modelIcons'
import { cn } from '../../../shared/classnames'
import { Badge, Button, Drawer, InlineFeedback, Modal } from '../components'
import { FormSection, LedgerField, SwitchField } from './AdminMediaTabs'

// Shared icon picker: built-in vendor grid (incl. the site fallback), plus
// an SVG upload that stores the document inline. Empty selection + no upload
// means the user web renders the site favicon.
export function ModelIconPicker({ iconKey, iconSvg, onChange, onSvgChange }: {
  iconKey: string
  iconSvg: string
  onChange: (key: string) => void
  onSvgChange: (svg: string) => void
}) {
  const fileRef = useRef<HTMLInputElement>(null)
  const [uploadError, setUploadError] = useState('')
  async function readFile(file: File) {
    setUploadError('')
    if (file.size > 64 * 1024) {
      setUploadError('SVG 超过 64KB 上限')
      return
    }
    const text = await file.text()
    if (!text.trim().toLowerCase().startsWith('<svg')) {
      setUploadError('文件不是 SVG 文档')
      return
    }
    onSvgChange(text)
    onChange('')
  }
  const effective = iconSvg ? 'custom' : iconKey || defaultIconKey
  return (
    <div className="grid gap-2">
      <div className="flex flex-wrap gap-2">
        {[siteIcon, ...builtinModelIcons].map((item) => (
          <button
            key={item.key}
            type="button"
            title={item.label}
            aria-label={item.label}
            aria-pressed={effective === item.key && !iconSvg}
            className={cn('grid size-10 place-items-center rounded-lg border bg-[var(--a-surface)] p-1.5 transition', effective === item.key && !iconSvg ? 'border-[var(--a-accent)] ring-1 ring-[var(--a-accent)]' : 'border-[var(--a-line)] hover:border-[var(--a-accent)]')}
            onClick={() => { onChange(item.key === defaultIconKey ? '' : item.key); onSvgChange('') }}
          >
            <img src={modelIconSrc(item.key)} alt="" width={22} height={22} />
          </button>
        ))}
        <button
          type="button"
          aria-pressed={Boolean(iconSvg)}
          className={cn('grid size-10 place-items-center rounded-lg border border-dashed bg-[var(--a-surface)] p-1.5 text-xs font-bold transition', iconSvg ? 'border-[var(--a-accent)] ring-1 ring-[var(--a-accent)] text-[var(--a-accent)]' : 'border-[var(--a-line)] text-[var(--a-ink-3)] hover:border-[var(--a-accent)]')}
          onClick={() => fileRef.current?.click()}
        >
          {iconSvg ? '自定义' : '上传'}
        </button>
        <input ref={fileRef} type="file" accept=".svg,image/svg+xml" className="hidden" onChange={(event) => { const file = event.target.files?.[0]; if (file) void readFile(file); event.target.value = '' }} />
        {iconSvg ? (
          <button type="button" className="self-center text-xs text-[var(--a-ink-3)] underline" onClick={() => onSvgChange('')}>清除上传图标</button>
        ) : null}
      </div>
      {uploadError ? <p className="m-0 text-xs text-[var(--a-danger)]">{uploadError}</p> : null}
    </div>
  )
}

type GroupDraft = {
  row?: RouteModelGroupRecord
  code: string
  name: string
  description: string
  sortOrder: number
  enabled: boolean
  iconKey: string
  iconSvg: string
  memberIDs: string[]
}

function blankGroup(_media: RouteModelMediaType): GroupDraft {
  return { code: '', name: '', description: '', sortOrder: 10, enabled: true, iconKey: '', iconSvg: '', memberIDs: [] }
}

function editGroup(row: RouteModelGroupRecord): GroupDraft {
  return { row, code: row.code, name: row.name, description: row.description, sortOrder: row.sort_order, enabled: row.enabled, iconKey: row.icon_key ?? '', iconSvg: row.icon_svg ?? '', memberIDs: row.route_model_ids.map(String) }
}

export function RouteModelGroupsPanel({ media, routes, onFeedback, onChanged }: {
  media: RouteModelMediaType
  routes: RouteModel[]
  onFeedback: (title: string, detail?: string) => void
  onChanged: () => void
}) {
  const [groups, setGroups] = useState<RouteModelGroupRecord[]>([])
  const [draft, setDraft] = useState<GroupDraft | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<RouteModelGroupRecord | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const load = async () => {
    try {
      setGroups(await adminApi.listRouteModelGroups(media))
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '分组载入失败')
    }
  }
  useEffect(() => { void load() }, [media])

  async function save() {
    if (!draft) return
    setBusy(true)
    setError('')
    try {
      const payload = {
        code: draft.code, name: draft.name, description: draft.description, media_type: media,
        icon_key: draft.iconKey, icon_svg: draft.iconSvg, sort_order: draft.sortOrder, enabled: draft.enabled,
        route_model_ids: draft.memberIDs.map(Number),
      }
      if (draft.row) await adminApi.updateRouteModelGroup(draft.row.id, payload)
      else await adminApi.createRouteModelGroup(payload)
      setDraft(null)
      await load()
      onChanged()
      onFeedback(draft.row ? '分组已更新' : '分组已创建')
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '保存失败')
    } finally {
      setBusy(false)
    }
  }

  async function remove() {
    if (!deleteTarget) return
    setBusy(true)
    setError('')
    try {
      await adminApi.deleteRouteModelGroup(deleteTarget.id)
      setDeleteTarget(null)
      await load()
      onChanged()
      onFeedback('分组已删除', deleteTarget.name)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '删除失败')
    } finally {
      setBusy(false)
    }
  }

  const routeName = (id: string) => routes.find((route) => String(route.id) === id)?.name ?? `#${id}`
  const groupDirty = draft ? groupDirtyCount(draft) : 0
  const groupErrors = draft ? groupValidation(draft) : {}

  return (
    <section className="a-panel a-panel--pad">
      <div className="a-section-title" style={{ marginBottom: 12 }}>
        <div>
          <h2>模型分组</h2>
          <p>分组决定路由的用户可见性；图标、成员数量与状态在一个列表中维护。</p>
        </div>
        <Button variant="secondary" size="sm" onClick={() => { setError(''); setDraft(blankGroup(media)) }}><Plus className="size-4" />新建分组</Button>
      </div>
      {error && !draft ? <InlineFeedback tone="danger" message={error} /> : null}
      <div className="rt-group-list">
        {groups.map((group) => (
          <div key={String(group.id)} className="rt-group-row">
            <span className="rt-group-icon"><img src={modelIconSrc(group.icon_key, group.icon_svg)} alt="" width={20} height={20} /></span>
            <div className="a-cell-stack">
              <strong>{group.name || group.code}</strong>
              <span>{group.code} · {group.description || '暂无描述'}</span>
            </div>
            <span className="a-hint">{group.route_model_ids.length} 条路由</span>
            <Badge tone={group.enabled ? 'success' : 'neutral'}>{group.enabled ? '启用' : '停用'}</Badge>
            <div className="a-row-actions">
              <button className="a-row-btn" type="button" onClick={() => { setError(''); setDraft(editGroup(group)) }}>编辑</button>
              <button className="a-row-btn a-row-btn--danger" type="button" disabled={busy} onClick={() => { setError(''); setDeleteTarget(group) }} aria-label={`删除分组 ${group.name}`}>删除</button>
            </div>
          </div>
        ))}
        {!groups.length ? <div className="cfg-empty-inline">暂无模型分组；用户端会平铺展示所有路由模型。</div> : null}
      </div>

      {draft ? (
        <Drawer
          title={draft.row ? '编辑模型分组' : '新建模型分组'}
          description={draft.row ? '修改图标、名称、描述与状态；分组代码不可修改。' : '分组代码会出现在路由可见性配置中。'}
          onClose={() => { setDraft(null); setError('') }}
          footer={<>
            <span className={cn('a-badge', Object.keys(groupErrors).length ? 'a-badge--bad' : groupDirty ? 'a-badge--warn' : 'a-badge--ok')}><i />{Object.keys(groupErrors).length ? `${Object.keys(groupErrors).length} 项校验未通过` : groupDirty ? `${groupDirty} 项未保存` : '未修改'}</span>
            <div style={{ flex: 1 }} />
            <Button variant="ghost" size="sm" disabled={busy} onClick={() => setDraft(null)}>取消</Button>
            <Button variant="primary" size="sm" disabled={busy || Object.keys(groupErrors).length > 0} onClick={() => void save()}>{busy ? '保存中...' : draft.row ? '保存分组' : '创建分组'}</Button>
          </>}
        >
          {error ? <InlineFeedback tone="danger" message={error} /> : null}
          <form className="a-form" onSubmit={(event) => event.preventDefault()}>
            <FormSection index={1} title="标识">
              <LedgerField label="分组图标" hint="图标会显示在分组列表、路由可见性与用户端分组标识中。"><ModelIconPicker iconKey={draft.iconKey} iconSvg={draft.iconSvg} onChange={(iconKey) => setDraft({ ...draft, iconKey })} onSvgChange={(iconSvg) => setDraft({ ...draft, iconSvg })} /></LedgerField>
              <LedgerField label="分组代码" required error={groupErrors.code} hint="创建后不可修改；用于路由可见性配置。"><input className="a-input" value={draft.code} disabled={Boolean(draft.row)} onChange={(event) => setDraft({ ...draft, code: event.target.value })} placeholder="例如：studio" style={{ fontFamily: 'var(--font-admin-mono)' }} /></LedgerField>
              <LedgerField label="分组名称" error={groupErrors.name}><input className="a-input" value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} placeholder="例如：Studio" /></LedgerField>
              <LedgerField label="描述"><textarea className="a-textarea" value={draft.description} onChange={(event) => setDraft({ ...draft, description: event.target.value })} placeholder="说明这个分组对谁可见" /></LedgerField>
            </FormSection>
            <FormSection index={2} title="状态与排序">
              <LedgerField label="排序值" error={groupErrors.sortOrder} hint="数值越小越靠前。"><input className="a-input" type="number" value={draft.sortOrder} onChange={(event) => setDraft({ ...draft, sortOrder: Number(event.target.value) })} /></LedgerField>
              <SwitchField label="启用该分组" checked={draft.enabled} onChange={(enabled) => setDraft({ ...draft, enabled })} />
            </FormSection>
            <FormSection index={3} title="成员路由模型" description="一个路由模型可同时属于多个分组；未归组的模型会出现在「其他模型」下。">
              <div className="cfg-checklist" style={{ maxHeight: 260, overflowY: 'auto' }}>
                {routes.length ? routes.map((route) => (
                  <label key={String(route.id)} className="a-switch">
                    <input type="checkbox" checked={draft.memberIDs.includes(String(route.id))} onChange={(event) => setDraft({ ...draft, memberIDs: event.target.checked ? [...draft.memberIDs, String(route.id)] : draft.memberIDs.filter((id) => id !== String(route.id)) })} />
                    <span className="a-switch__track" />
                    <span>{route.name} <span className="a-hint">{route.code}</span></span>
                  </label>
                )) : <div className="a-hint">当前媒体类型暂无路由模型</div>}
              </div>
            </FormSection>
          </form>
          <div className="a-banner a-banner--info">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M12 11v5M12 7.5h.01" /></svg>
            <div><strong style={{ fontSize: 13 }}>分组与用户分组不是同一套配置</strong><div className="a-hint">这里定义路由可见性；用户属于哪个分组在「用户分组」页面维护。</div></div>
          </div>
        </Drawer>
      ) : null}

      {deleteTarget ? (
        <Modal
          title="删除模型分组"
          detail={`${deleteTarget.name} · ${deleteTarget.code}`}
          onClose={() => { if (!busy) setDeleteTarget(null) }}
          footer={<><Button variant="ghost" disabled={busy} onClick={() => setDeleteTarget(null)}>取消</Button><Button variant="danger" disabled={busy} onClick={() => void remove()}>{busy ? '删除中...' : '确认删除'}</Button></>}
        >
          <p className="m-0 text-sm leading-6 text-[var(--a-ink-2)]">删除分组不会删除路由模型本身；路由的可见性会变更为未分组状态。</p>
        </Modal>
      ) : null}
    </section>
  )
}

function groupValidation(draft: GroupDraft): Record<string, string> {
  const errors: Record<string, string> = {}
  if (!draft.code.trim()) errors.code = '请填写分组代码。'
  if (!draft.name.trim()) errors.name = '请填写分组名称。'
  if (!Number.isFinite(draft.sortOrder)) errors.sortOrder = '排序值必须是数字。'
  return errors
}

function groupDirtyCount(draft: GroupDraft): number {
  if (!draft.row) return draft.code || draft.name || draft.description || draft.memberIDs.length || draft.iconKey || draft.iconSvg ? 1 : 0
  return JSON.stringify({
    code: draft.code, name: draft.name, description: draft.description, sortOrder: draft.sortOrder,
    enabled: draft.enabled, iconKey: draft.iconKey, iconSvg: draft.iconSvg, memberIDs: draft.memberIDs,
  }) === JSON.stringify({
    code: draft.row.code, name: draft.row.name, description: draft.row.description, sortOrder: draft.row.sort_order,
    enabled: draft.row.enabled, iconKey: draft.row.icon_key ?? '', iconSvg: draft.row.icon_svg ?? '', memberIDs: draft.row.route_model_ids.map(String),
  }) ? 0 : 1
}