import { useEffect, useMemo, useState } from 'react'
import { type AdminSession, type ConfigItem } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Badge, EmptyBlock, ErrorBlock, Field, LoadingBlock, PageHeader } from '../components'
import { canAdmin } from '../types'
import {
  configFieldMeta,
  configLockedDetail,
  configPermission,
  configTabMeta,
  configValidateValue,
  extractConfigValue,
  inferConfigFieldType,
  isRecord,
  isSameConfigValue,
  normalizeDraftValue,
  type ConfigFieldMeta,
  type ConfigValue,
} from './configRows'

type DraftMap = Record<string, ConfigValue>

export function ConfigPage({ session, onFeedback }: { session: AdminSession; onFeedback: (title: string, detail?: string) => void }) {
  const [rows, setRows] = useState<ConfigItem[]>([])
  const [drafts, setDrafts] = useState<DraftMap>({})
  const [activeTab, setActiveTab] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [notice, setNotice] = useState('配置中心已连接真实 API。')

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const nextRows = await adminApi.listConfig()
      setRows(nextRows)
      setDrafts(Object.fromEntries(nextRows.map((row) => [draftId(row), extractConfigValue(row)])))
      setActiveTab((current) => current || nextRows[0]?.config_category || nextRows[0]?.tab || '')
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '配置载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const tabs = useMemo(() => {
    const keys = Array.from(new Set(rows.map((row) => row.config_category || row.tab).filter(Boolean)))
    return keys.map((key) => ({ key, label: configTabMeta(key).label }))
  }, [rows])
  const activeRows = rows.filter((row) => (row.config_category || row.tab) === activeTab)
  const dirtyKeys = rows.filter((row) => !isSameConfigValue(drafts[draftId(row)], extractConfigValue(row))).map(draftId)
  const activeDirty = activeRows.some((row) => dirtyKeys.includes(draftId(row)))
  const conflicts = activeRows.flatMap((row) => {
    const key = draftId(row)
    const message = configValidateValue(row.config_key ?? row.key, drafts[key])
    return message ? [{ key: row.key, message }] : []
  })
  const activeMeta = configTabMeta(activeTab || activeRows[0]?.tab || '')
  const requiredPermission = configPermission(activeTab)
  const canEditActiveTab = canAdmin(session, requiredPermission)
  const lockedDetail = configLockedDetail(requiredPermission)

  const saveActiveTab = async () => {
    if (!activeRows.length || conflicts.length || !canEditActiveTab) return
    setSaving(true)
    try {
      const version = Math.max(...activeRows.map((row) => row.version || 1))
      await adminApi.updateConfigTab(activeTab, {
        version,
        items: activeRows.map((row) => ({
          config_category: row.config_category ?? activeTab,
          config_key: row.config_key ?? row.key,
          config_value: { value: normalizeDraftValue(drafts[draftId(row)]) },
          scope: row.scope ?? 'global',
        })),
      })
      const nextRows = await adminApi.listConfig()
      setRows(nextRows)
      setDrafts(Object.fromEntries(nextRows.map((row) => [draftId(row), extractConfigValue(row)])))
      setNotice(`${activeMeta.label}已保存，API 节点将在 1 分钟内读取新配置。`)
      onFeedback('系统设置已保存', activeMeta.label)
    } finally {
      setSaving(false)
    }
  }

  const revertActiveTab = () => {
    setDrafts((current) => ({
      ...current,
      ...Object.fromEntries(activeRows.map((row) => [draftId(row), extractConfigValue(row)])),
    }))
    setNotice(`${activeMeta.label}已恢复为当前生效值。`)
  }

  if (loading) return <LoadingBlock label="载入系统设置" />
  if (error) return <ErrorBlock message={error} onRetry={load} />
  if (!rows.length) return <EmptyBlock title="暂无配置项" detail="配置中心尚未返回可编辑条目。" />

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="Settings"
        title="系统设置"
        detail="按类目维护配置表单，保存时统一提交当前类目。"
        actions={
          <>
            <button type="button" className="ghost" onClick={revertActiveTab} disabled={saving || !activeDirty}>恢复本类</button>
            <button type="button" className="btn primary" onClick={() => void saveActiveTab()} disabled={saving || Boolean(conflicts.length) || !activeDirty || !canEditActiveTab}>{saving ? '保存中...' : '保存本类'}</button>
          </>
        }
      />

      <section className="ops-status-strip compact-strip">
        <div className="status-cell"><label>当前类目</label><strong>{activeMeta.label}</strong></div>
        <div className="status-cell"><label>字段</label><strong>{activeRows.length} 项</strong></div>
        <div className="status-cell"><label>未保存</label><strong>{dirtyKeys.length} 项</strong></div>
        <div className="status-cell"><label>校验</label><strong>{conflicts.length ? `${conflicts.length} 项需处理` : '通过'}</strong></div>
      </section>

      <section className="pg-admin-card config-formboard">
        <aside className="config-category-rail">
          {tabs.map((tab) => <button key={tab.key} className={activeTab === tab.key ? 'active' : ''} type="button" onClick={() => setActiveTab(tab.key)}>{tab.label}</button>)}
        </aside>

        <section className="config-form-lane">
          <div className="section-head compact">
            <div>
              <strong>{activeMeta.label}</strong>
              <p>{activeMeta.detail}</p>
            </div>
            <Badge tone={!canEditActiveTab || conflicts.length ? 'warning' : 'success'}>{!canEditActiveTab ? '只读' : conflicts.length ? '需修正' : '可保存'}</Badge>
          </div>
          {!canEditActiveTab ? <div className="permission-note">{lockedDetail}</div> : null}

          <div className="config-form-grid">
            {activeRows.map((row) => {
              const key = row.config_key ?? row.key
              const meta = configFieldMeta(key, row.description)
              const rowDraftId = draftId(row)
              const conflict = configValidateValue(key, drafts[rowDraftId])
              return (
                <div key={rowDraftId} className="config-form-item">
                  {renderConfigField(row, meta, drafts[rowDraftId], (value) => setDrafts((current) => ({ ...current, [rowDraftId]: value })), conflict, !canEditActiveTab)}
                </div>
              )
            })}
          </div>
        </section>

        <aside className="config-side-rail">
          <section className="side-strip"><label>保存反馈</label><strong>{notice}</strong></section>
          <section className="side-strip"><label>提示</label><p>鼠标悬停字段名旁的提示符可查看用途说明；复杂列表以结构化文本编辑，保存后仍按原始接口契约提交。</p></section>
        </aside>
      </section>
    </section>
  )
}

function renderConfigField(row: ConfigItem, meta: ConfigFieldMeta, value: ConfigValue, onChange: (value: ConfigValue) => void, error: string | null, disabled: boolean) {
  const key = row.config_key ?? row.key
  const type = meta.type ?? inferConfigFieldType(value)
  if (type === 'boolean') {
    return (
      <Field label={meta.label} hint={meta.hint} error={error}>
        <select value={String(Boolean(value))} onChange={(event) => onChange(event.target.value === 'true')} disabled={disabled}>
          <option value="true">开启</option>
          <option value="false">关闭</option>
        </select>
      </Field>
    )
  }
  if (type === 'number') {
    return <Field label={meta.label} hint={meta.hint} error={error}><input type="number" value={String(value ?? '')} onChange={(event) => onChange(Number(event.target.value))} disabled={disabled} /></Field>
  }
  if (type === 'map' && isRecord(value)) {
    return <MapField label={meta.label} hint={meta.hint} value={value} onChange={onChange} error={error} disabled={disabled} />
  }
  if (type === 'list' || Array.isArray(value)) {
    if (Array.isArray(value) && value.some((item) => isRecord(item))) {
      return <StructuredListField label={meta.label} hint={meta.hint} value={value} onChange={onChange} error={error} disabled={disabled} />
    }
    return <Field label={meta.label} hint={meta.hint} error={error}><input value={(Array.isArray(value) ? value : []).join(', ')} onChange={(event) => onChange(event.target.value.split(',').map((item) => item.trim()).filter(Boolean))} disabled={disabled} /></Field>
  }
  return <Field label={meta.label || key} hint={meta.hint} error={error}><input value={String(value ?? '')} onChange={(event) => onChange(event.target.value)} disabled={disabled} /></Field>
}

function StructuredListField({ label, hint, value, onChange, error, disabled }: { label: string; hint: string; value: ConfigValue[]; onChange: (value: ConfigValue) => void; error: string | null; disabled: boolean }) {
  const [text, setText] = useState(() => JSON.stringify(value, null, 2))
  const [parseError, setParseError] = useState<string | null>(null)

  useEffect(() => {
    setText(JSON.stringify(value, null, 2))
    setParseError(null)
  }, [value])

  const update = (nextText: string) => {
    setText(nextText)
    try {
      const parsed = JSON.parse(nextText)
      if (!Array.isArray(parsed)) {
        setParseError('请输入数组结构。')
        return
      }
      setParseError(null)
      onChange(parsed as ConfigValue)
    } catch {
      setParseError('结构化文本格式不正确，请修正后再保存。')
    }
  }

  return (
    <Field label={label} hint={hint} error={parseError || error}>
      <textarea rows={8} value={text} onChange={(event) => update(event.target.value)} disabled={disabled} />
    </Field>
  )
}

function MapField({ label, hint, value, onChange, error, disabled }: { label: string; hint: string; value: Record<string, unknown>; onChange: (value: ConfigValue) => void; error: string | null; disabled: boolean }) {
  const entries = Object.entries(value)
  const updateKey = (oldKey: string, nextKey: string) => {
    const next: Record<string, unknown> = {}
    entries.forEach(([key, entryValue]) => {
      next[key === oldKey ? nextKey : key] = entryValue
    })
    onChange(next)
  }
  const updateValue = (key: string, nextValue: string) => onChange({ ...value, [key]: nextValue })
  return (
    <Field label={label} hint={hint} error={error}>
      <div className="config-kv-list">
        {entries.map(([entryKey, entryValue]) => (
          <div key={entryKey} className="config-kv-row">
            <input value={entryKey} onChange={(event) => updateKey(entryKey, event.target.value)} aria-label={`${label} 键`} disabled={disabled} />
            <input value={String(entryValue ?? '')} onChange={(event) => updateValue(entryKey, event.target.value)} aria-label={`${label} 值`} disabled={disabled} />
          </div>
        ))}
        <button type="button" className="ghost small" onClick={() => onChange({ ...value, [`key_${entries.length + 1}`]: '' })} disabled={disabled}>新增键值</button>
      </div>
    </Field>
  )
}

function draftId(row: ConfigItem) {
  return `${row.config_category ?? row.tab}:${row.config_key ?? row.key}:${row.scope ?? 'global'}`
}
