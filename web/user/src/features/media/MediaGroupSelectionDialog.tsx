import { useEffect, useState } from 'react'
import { userApi } from '../../../../shared/user-api'
import { Modal } from '../../components'

export type MediaGroupSelectionDialogProps = {
  existingGroups?: string[]
  projectId?: string
  value?: string
  onSelect: (groupName: string) => void | Promise<void>
  onClose: () => void
}

/** One group_name per asset; creating a group assigns its name on save. */
export function MediaGroupSelectionDialog({ existingGroups = [], projectId, value = '', onSelect, onClose }: MediaGroupSelectionDialogProps) {
  const [groups, setGroups] = useState<string[]>([])
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState(value)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    let alive = true
    setLoading(true)
    setError('')
    void userApi.listMediaGroups(projectId).then((items) => {
      if (alive) setGroups(items)
    }).catch((caught) => {
      if (alive) setError(caught instanceof Error ? caught.message : '分组加载失败')
    }).finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [projectId, revision])
  const allGroups = Array.from(new Set([...groups, ...existingGroups, value].map((name) => name.trim()).filter(Boolean))).sort((a, b) => a.localeCompare(b, 'zh-CN'))
  const name = query.trim()
  const visibleGroups = allGroups.filter((group) => group.toLocaleLowerCase().includes(name.toLocaleLowerCase()))
  const save = async () => {
    setBusy(true)
    setError('')
    try { await onSelect(selected.trim()); onClose() }
    catch (caught) { setError(caught instanceof Error ? caught.message : '设置分组失败') }
    finally { setBusy(false) }
  }
  return <Modal title="设置分组" onClose={() => { if (!busy) onClose() }}>
    <div style={{ display: 'grid', gap: 16 }}>
      <label className="u-field"><span className="u-label">搜索已有分组或输入新名称</span><input className="u-input" autoFocus value={query} disabled={busy} onChange={(event) => setQuery(event.target.value)} placeholder="分组名称" /></label>
      <label className="u-field"><span className="u-label">选择一个分组</span><select className="u-select" aria-label="选择分组" value={selected} disabled={busy} onChange={(event) => setSelected(event.target.value)}>
        <option value="">未分组（清除分组）</option>
        {selected && !visibleGroups.includes(selected) ? <option value={selected}>{selected}</option> : null}
        {visibleGroups.map((group) => <option key={group} value={group}>{group}</option>)}
      </select></label>
      {name && !allGroups.includes(name) ? <button type="button" className="u-btn u-btn--sm" disabled={busy} onClick={() => setSelected(name)}>创建并选择“{name}”</button> : null}
      {loading ? <span className="u-meta" role="status">正在加载全部分组</span> : null}
      {error ? <div role="alert">{error} <button type="button" className="u-action" disabled={busy || loading} onClick={() => setRevision((current) => current + 1)}>重新加载分组</button></div> : null}
      <p className="u-meta">每个资产只能属于一个分组。当前选择：{selected || '未分组'}</p>
      <div className="u-actions"><button type="button" className="u-btn" disabled={busy} onClick={onClose}>取消</button><button type="button" className="u-btn u-btn--primary" disabled={busy} onClick={() => void save()}>{busy ? '保存中' : '保存分组'}</button></div>
    </div>
  </Modal>
}
