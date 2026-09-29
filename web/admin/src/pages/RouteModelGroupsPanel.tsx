import { useEffect, useRef, useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import type { RouteModel, RouteModelGroupRecord, RouteModelMediaType } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { builtinModelIcons, defaultIconKey, modelIconSrc, siteIcon } from '../../../shared/modelIcons'
import { cn } from '../../../shared/classnames'
import { adminButton } from '../ui/classes'
import { Field, Modal } from '../components'

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
            aria-pressed={effective === item.key && !iconSvg}
            className={cn('grid size-10 place-items-center rounded-xl border bg-[var(--surface)] p-1.5 transition', effective === item.key && !iconSvg ? 'border-[var(--accent)] ring-1 ring-[var(--accent)]' : 'border-[var(--border)] hover:border-[var(--accent)]')}
            onClick={() => { onChange(item.key === defaultIconKey ? '' : item.key); onSvgChange('') }}
          >
            <img src={modelIconSrc(item.key)} alt={item.label} width={22} height={22} />
          </button>
        ))}
        <button
          type="button"
          aria-pressed={Boolean(iconSvg)}
          className={cn('grid size-10 place-items-center rounded-xl border border-dashed bg-[var(--surface)] p-1.5 text-xs font-bold transition', iconSvg ? 'border-[var(--accent)] ring-1 ring-[var(--accent)] text-[var(--accent)]' : 'border-[var(--border)] text-[var(--muted)] hover:border-[var(--accent)]')}
          onClick={() => fileRef.current?.click()}
        >
          {iconSvg ? '自定义' : '上传'}
        </button>
        <input ref={fileRef} type="file" accept=".svg,image/svg+xml" className="hidden" onChange={(event) => { const file = event.target.files?.[0]; if (file) void readFile(file); event.target.value = '' }} />
        {iconSvg ? (
          <button type="button" className="self-center text-xs text-[var(--muted)] underline" onClick={() => onSvgChange('')}>清除上传图标</button>
        ) : null}
      </div>
      {uploadError ? <p className="m-0 text-xs text-[var(--accent-coral)]">{uploadError}</p> : null}
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

  async function remove(row: RouteModelGroupRecord) {
    if (!window.confirm(`删除模型分组“${row.name}”？路由模型本身不会被删除。`)) return
    setBusy(true)
    try {
      await adminApi.deleteRouteModelGroup(row.id)
      await load()
      onChanged()
      onFeedback('分组已删除')
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '删除失败')
    } finally {
      setBusy(false)
    }
  }

  const routeName = (id: string) => routes.find((route) => String(route.id) === id)?.name ?? `#${id}`

  return (
    <section className="mb-6 rounded-xl border border-[var(--border)] bg-[var(--surface)]/60 p-4">
      <header className="mb-3 flex items-center justify-between gap-3">
        <div>
          <h3 className="m-0 text-sm font-bold">模型分组（用户端两级选择的第一级）</h3>
          <p className="m-0 mt-1 text-xs text-[var(--muted)]">把路由模型归入分组；一个路由模型可同时属于多个分组。未归组的模型会出现在「其他模型」下。</p>
        </div>
        <button className={cn(adminButton.base, adminButton.primary)} type="button" onClick={() => setDraft(blankGroup(media))}><Plus size={15} />新建分组</button>
      </header>
      {error ? <p className="mb-2 text-xs text-[var(--accent-coral)]">{error}</p> : null}
      {groups.length ? (
        <ul className="m-0 grid list-none gap-2 p-0">
          {groups.map((group) => (
            <li key={String(group.id)} className="flex flex-wrap items-center gap-3 rounded-xl border border-[var(--border)] bg-[var(--bg)]/40 px-3 py-2">
              <img src={modelIconSrc(group.icon_key, group.icon_svg)} alt="" width={22} height={22} />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2 text-sm font-bold">
                  {group.name}
                  <code className="rounded bg-[var(--surface)] px-1.5 py-0.5 text-xs text-[var(--muted)]">{group.code}</code>
                  {!group.enabled ? <span className="text-xs text-[var(--accent-coral)]">停用</span> : null}
                </div>
                <div className="truncate text-xs text-[var(--muted)]">{group.route_model_ids.length ? group.route_model_ids.map((id) => routeName(String(id))).join(' · ') : '未挂载路由模型'}</div>
              </div>
              <button className={cn(adminButton.base, adminButton.ghost)} type="button" onClick={() => setDraft(editGroup(group))}>编辑</button>
              <button className={cn(adminButton.base, adminButton.ghost)} type="button" disabled={busy} onClick={() => void remove(group)} aria-label={`删除分组 ${group.name}`}><Trash2 size={15} /></button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="m-0 rounded-xl border border-dashed border-[var(--border)] px-3 py-4 text-center text-xs text-[var(--muted)]">暂无模型分组；用户端会平铺展示所有路由模型。</p>
      )}

      {draft ? (
        <Modal
          title={draft.row ? '编辑模型分组' : '新建模型分组'}
          onClose={() => setDraft(null)}
          footer={<>
            <button className={cn(adminButton.base, adminButton.ghost)} type="button" onClick={() => setDraft(null)}>取消</button>
            <button className={cn(adminButton.base, adminButton.primary)} type="button" disabled={busy || !draft.code.trim() || !draft.name.trim()} onClick={() => void save()}>保存</button>
          </>}
        >
          <div className="grid gap-3">
            <Field label="编码"><input value={draft.code} onChange={(e) => setDraft({ ...draft, code: e.target.value })} placeholder="如 jimeng-video" /></Field>
            <Field label="名称"><input value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} placeholder="如 即梦视频" /></Field>
            <Field label="描述"><input value={draft.description} onChange={(e) => setDraft({ ...draft, description: e.target.value })} /></Field>
            <Field label="排序与状态">
              <div className="flex items-center gap-3">
                <input type="number" value={draft.sortOrder} onChange={(e) => setDraft({ ...draft, sortOrder: Number(e.target.value) })} className="w-24" />
                <label className="flex items-center gap-1.5 text-sm"><input type="checkbox" checked={draft.enabled} onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })} />启用</label>
              </div>
            </Field>
            <Field label="图标（内置或上传 SVG，缺省用站点图标）">
              <ModelIconPicker iconKey={draft.iconKey} iconSvg={draft.iconSvg} onChange={(iconKey) => setDraft({ ...draft, iconKey })} onSvgChange={(iconSvg) => setDraft({ ...draft, iconSvg })} />
            </Field>
            <Field label={`成员路由模型（已选 ${draft.memberIDs.length}）`}>
              <div className="grid max-h-56 gap-1.5 overflow-y-auto rounded-xl border border-[var(--border)] p-2">
                {routes.length ? routes.map((route) => (
                  <label key={String(route.id)} className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={draft.memberIDs.includes(String(route.id))}
                      onChange={(e) => setDraft({ ...draft, memberIDs: e.target.checked ? [...draft.memberIDs, String(route.id)] : draft.memberIDs.filter((id) => id !== String(route.id)) })}
                    />
                    <img src={modelIconSrc(route.icon_key, route.icon_svg)} alt="" width={16} height={16} />
                    {route.name}
                    {!route.enabled ? <span className="text-xs text-[var(--accent-coral)]">停用</span> : null}
                  </label>
                )) : <span className="text-xs text-[var(--muted)]">当前媒体类型暂无路由模型</span>}
              </div>
            </Field>
            {error ? <p className="m-0 text-xs text-[var(--accent-coral)]">{error}</p> : null}
          </div>
        </Modal>
      ) : null}
    </section>
  )
}
