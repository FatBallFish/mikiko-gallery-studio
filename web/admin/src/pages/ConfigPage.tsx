import { useEffect, useMemo, useRef, useState } from 'react'
import { type AdminSession, type ConfigItem } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, EmptyBlock, ErrorBlock, Field, InlineFeedback, LoadingBlock, PageHeader } from '../components'
import { FormSaveBar, MultiSelectField } from './AdminMediaTabs'
import { canAdmin } from '../types'
import { adminButton, adminPage } from '../ui/classes'
import { systemSettingsSectionAnchor, settingsValuesEqual, type SystemSettingsSection } from './systemSettingsTabs'
import {
  configFieldMeta,
  configLockedDetail,
  configPermission,
  configRowAllowed,
  configTabMeta,
  configValidateValue,
  extractConfigValue,
  generalConfigCategories,
  inferConfigFieldType,
  isRecord,
  normalizeDraftValue,
  type ConfigFieldMeta,
  type ConfigValue,
} from './configRows'

type DraftMap = Record<string, ConfigValue>
type ConfigEditorState = 'pristine' | 'dirty' | 'validating' | 'saving' | 'saved' | 'failed'

const configClasses = {
  kvList: 'grid gap-2',
  kvRow: 'grid grid-cols-[minmax(90px,.9fr)_minmax(100px,1fr)] gap-2 max-[620px]:grid-cols-1',
}

const generalSectionLabels: Record<string, string> = {
  site: '站点基础',
  features: '功能开关',
  public_gallery: '公开内容',
}

const generalSectionDetails: Record<string, string> = {
  site: '站点基础信息与访问设置。',
  features: '用户端功能入口开关；关闭后历史资产仍会保留。',
  public_gallery: '图片公开申请、广场展示和内容可见性开关。',
}

function configSectionLabel(key: string) {
  return generalSectionLabels[key] ?? configTabMeta(key).label
}

function configSectionDetail(key: string) {
  return generalSectionDetails[key] ?? configTabMeta(key).detail
}

export function ConfigPage({
  session,
  onFeedback,
  compact = false,
  summaryMode = false,
  categories = generalConfigCategories,
  keys,
  onDirtyChange,
  onBusyChange,
  sectionRequest,
  onSectionsChange,
  onActiveSectionChange,
}: {
  session: AdminSession
  onFeedback: (title: string, detail?: string) => void
  compact?: boolean
  summaryMode?: boolean
  categories?: readonly string[]
  keys?: readonly string[]
  onDirtyChange?: (dirty: boolean) => void
  onBusyChange?: (busy: boolean) => void
  sectionRequest?: { id: string; nonce: number } | null
  onSectionsChange?: (sections: SystemSettingsSection[]) => void
  onActiveSectionChange?: (section: string) => void
}) {
  const [rows, setRows] = useState<ConfigItem[]>([])
  const [drafts, setDrafts] = useState<DraftMap>({})
  const [baseline, setBaseline] = useState<DraftMap>({})
  const [activeTab, setActiveTab] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [notice, setNotice] = useState('配置中心已连接真实 API。')
  const [saveError, setSaveError] = useState<string | null>(null)
  const [editorState, setEditorState] = useState<ConfigEditorState>('pristine')
  const handledSectionRequest = useRef(0)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const nextRows = await adminApi.listConfig()
      const generalRows = nextRows.filter((row) => configRowAllowed(row, categories, keys))
      const nextDrafts = draftMapFromRows(generalRows)
      setRows(generalRows)
      setDrafts(nextDrafts)
      setBaseline(cloneDraftMap(nextDrafts))
      setActiveTab((current) => {
        if (current && generalRows.some((row) => (row.config_category || row.tab) === current)) return current
        return generalRows[0]?.config_category || generalRows[0]?.tab || ''
      })
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
    return keys.map((key) => ({ key, label: configSectionLabel(key) }))
  }, [rows])
  const activeRows = rows.filter((row) => (row.config_category || row.tab) === activeTab)
  // Config categories (for example `features`) are displayed as editor tabs,
  // while writes must target the owning persistence tab (for example `site`).
  const activePersistenceTab = activeRows[0]?.tab || activeTab
  const dirtyKeys = rows.filter((row) => !settingsValuesEqual(drafts[draftId(row)], baseline[draftId(row)])).map(draftId)
  const activeDirty = activeRows.some((row) => dirtyKeys.includes(draftId(row)))
  const dirty = dirtyKeys.length > 0
  const conflicts = activeRows.flatMap((row) => {
    const key = draftId(row)
    const message = configValidateValue(row.config_key ?? row.key, drafts[key])
    return message ? [{ key: row.key, message }] : []
  })
  const activeMetaKey = activeTab || activeRows[0]?.tab || ''
  const activeMeta = { ...configTabMeta(activeMetaKey), label: configSectionLabel(activeMetaKey), detail: configSectionDetail(activeMetaKey) }
  const requiredPermission = configPermission(activeTab)
  const canEditActiveTab = canAdmin(session, requiredPermission)
  const lockedDetail = configLockedDetail(requiredPermission)
  const sectionItems = useMemo<SystemSettingsSection[]>(() => tabs.map((tab) => ({
    id: tab.key,
    label: tab.label,
    count: rows.filter((row) => (row.config_category || row.tab) === tab.key).length,
    disabled: saving,
  })), [rows, saving, tabs])

  useEffect(() => {
    onDirtyChange?.(dirty)
  }, [dirty, onDirtyChange])

  useEffect(() => {
    onBusyChange?.(saving)
  }, [onBusyChange, saving])

  useEffect(() => () => onBusyChange?.(false), [onBusyChange])

  useEffect(() => {
    onSectionsChange?.(sectionItems)
  }, [onSectionsChange, sectionItems])

  useEffect(() => {
    if (activeTab) onActiveSectionChange?.(activeTab)
  }, [activeTab, onActiveSectionChange])

  useEffect(() => {
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!dirty) return
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => window.removeEventListener('beforeunload', onBeforeUnload)
  }, [dirty])

  useEffect(() => {
    if (saving || editorState === 'failed') return
    setEditorState(dirty ? 'dirty' : editorState === 'saved' ? 'saved' : 'pristine')
  }, [dirty, editorState, saving])

  const saveActiveTab = async () => {
    if (!activeRows.length || conflicts.length || !canEditActiveTab) return
    setEditorState('validating')
    setSaving(true)
    setEditorState('saving')
    setSaveError(null)
    try {
      const version = Math.max(...activeRows.map((row) => row.version || 1))
      await adminApi.updateConfigTab(activePersistenceTab, {
        version,
        items: activeRows.map((row) => ({
          config_category: row.config_category ?? activeTab,
          config_key: row.config_key ?? row.key,
          config_value: { value: normalizeDraftValue(drafts[draftId(row)]) },
          scope: row.scope ?? 'global',
        })),
      })
    } catch (caught) {
      setSaveError(caught instanceof Error ? caught.message : '通用配置保存失败')
      setEditorState('failed')
      setSaving(false)
      return
    }

    // The write succeeded: reset the baseline from the values that were sent
    // so the dirty indicator clears before the optional refresh completes.
    const savedDrafts = Object.fromEntries(activeRows.map((row) => [draftId(row), cloneConfigValue(drafts[draftId(row)]) as ConfigValue]))
    setDrafts((current) => ({ ...current, ...savedDrafts }))
    setBaseline((current) => ({ ...current, ...cloneDraftMap(savedDrafts) }))
    setNotice(`${activeMeta.label}已保存，API 节点将在 1 分钟内读取新配置。`)
    setEditorState('saved')
    onFeedback('通用配置已保存', activeMeta.label)

    try {
      const nextRows = await adminApi.listConfig()
      const generalRows = nextRows.filter((row) => configRowAllowed(row, categories, keys))
      const nextDrafts = draftMapFromRows(generalRows)
      setRows(generalRows)
      setDrafts(nextDrafts)
      setBaseline(cloneDraftMap(nextDrafts))
    } catch (caught) {
      setSaveError(caught instanceof Error ? `配置已保存，但重新读取最新值失败：${caught.message}` : '配置已保存，但重新读取最新值失败，请刷新页面确认。')
    } finally {
      setSaving(false)
    }
  }

  const revertActiveTab = () => {
    setDrafts((current) => ({
      ...current,
      ...draftMapFromRows(activeRows, baseline),
    }))
    setNotice(`${activeMeta.label}已恢复为当前生效值。`)
    setSaveError(null)
    setEditorState('pristine')
  }

  const switchConfigTab = (tab: string) => {
    if (saving) return
    if (tab === activeTab) return
    if (!tabs.some((item) => item.key === tab)) return
    if (activeDirty) {
      if (!window.confirm('当前类目存在未保存修改，放弃并切换吗？')) {
        onActiveSectionChange?.(activeTab)
        return
      }
      setDrafts((current) => ({
        ...current,
        ...draftMapFromRows(activeRows, baseline),
      }))
    }
    setSaveError(null)
    setEditorState('pristine')
    setActiveTab(tab)
    onActiveSectionChange?.(tab)
  }

  useEffect(() => {
    if (!sectionRequest || sectionRequest.nonce === handledSectionRequest.current) return
    handledSectionRequest.current = sectionRequest.nonce
    switchConfigTab(sectionRequest.id)
  }, [sectionRequest])

  if (loading) return <LoadingBlock label="载入通用配置" />
  if (error) return <ErrorBlock message={error} onRetry={load} />
  if (!rows.length) return <EmptyBlock title="暂无通用配置项" detail="配置中心尚未返回公开内容等低风险配置。" />

  return (
    <section className={adminPage.stack}>
      {!compact && !summaryMode ? (
        <PageHeader
          title="通用配置"
          detail="只维护公开内容等低风险配置；认证、支付、审核和生成限制已拆到对应独立页面。"
        />
      ) : null}

      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
        <span className="a-hint">{tabs.length > 1 ? '通过左侧区块导航切换类目；每个类目独立保存。' : '配置字段使用单列账本布局；保存后 API 节点将在 1 分钟内读取新值。'}</span>
        <div className="a-row-actions">
          <Badge tone={!canEditActiveTab || conflicts.length ? 'warning' : 'success'}>{!canEditActiveTab ? '只读' : conflicts.length ? `${conflicts.length} 项校验未通过` : '校验通过'}</Badge>
          <Badge tone={dirty ? 'warning' : editorState === 'saved' ? 'success' : 'neutral'}>{dirty ? `${dirtyKeys.length} 项未保存` : configEditorStateLabel(editorState, 0)}</Badge>
        </div>
      </div>

      <section
        id={systemSettingsSectionAnchor('general', activeTab)}
        className={cn('a-panel a-panel--flush', 'ss-block is-active')}
        data-admin-config-save-rail
      >
        <div className="a-panel__head">
          <div>
            <h2>{activeMeta.label}</h2>
            <p>{activeMeta.detail}</p>
          </div>
          <span className="a-hint">{activeRows.length} 个字段</span>
        </div>
        {!canEditActiveTab ? <div style={{ padding: '12px 20px' }}><InlineFeedback tone="warning" message={lockedDetail} /></div> : null}
        {saveError ? <div style={{ padding: '12px 20px' }}><InlineFeedback tone="danger" message={saveError} /></div> : null}
        <form className="a-form" onSubmit={(event) => event.preventDefault()}>
          <section className="a-form__section">
            <div className="a-form__head">
              <h3><span className="a-form__num">1</span>{activeMeta.label}</h3>
              <p>{activeMeta.detail || '配置字段使用单列布局，保存后 API 节点将在 1 分钟内读取新值。'}</p>
            </div>
            {activeRows.map((row) => {
              const key = row.config_key ?? row.key
              const meta = configFieldMeta(key, row.description)
              const rowDraftId = draftId(row)
              const conflict = configValidateValue(key, drafts[rowDraftId])
              return (
                <div key={rowDraftId}>
                  {renderConfigField(row, meta, drafts[rowDraftId], (value) => setDrafts((current) => ({ ...current, [rowDraftId]: value })), conflict, saving || !canEditActiveTab)}
                </div>
              )
            })}
          </section>
        </form>
        <FormSaveBar
          dirtyCount={dirtyKeys.length}
          errorCount={conflicts.length}
          busy={saving}
          note={notice}
          onReset={revertActiveTab}
          onSave={() => void saveActiveTab()}
          saveLabel="保存本类"
          saveDisabled={!canEditActiveTab || !activeDirty}
        />
      </section>
    </section>
  )
}

function configEditorStateLabel(state: ConfigEditorState, conflictCount: number) {
  if (conflictCount) return `${conflictCount} 项校验未通过`
  if (state === 'dirty') return '有未保存修改'
  if (state === 'validating') return '正在校验'
  if (state === 'saving') return '正在保存'
  if (state === 'saved') return '已保存'
  if (state === 'failed') return '保存失败'
  return '未修改'
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
    const values = (Array.isArray(value) ? value : []).map(String)
    return <MultiSelectField label={meta.label} hint={meta.hint} error={error} values={values} options={values} onChange={(next) => onChange(next)} disabled={disabled} customPlaceholder="输入自定义值后添加" placeholder="选择或添加值" />
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
      <div className={configClasses.kvList}>
        {entries.map(([entryKey, entryValue]) => (
          <div key={entryKey} className={configClasses.kvRow}>
            <input value={entryKey} onChange={(event) => updateKey(entryKey, event.target.value)} aria-label={`${label} 键`} disabled={disabled} />
            <input value={String(entryValue ?? '')} onChange={(event) => updateValue(entryKey, event.target.value)} aria-label={`${label} 值`} disabled={disabled} />
          </div>
        ))}
        <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={() => onChange({ ...value, [`key_${entries.length + 1}`]: '' })} disabled={disabled}>新增键值</button>
      </div>
    </Field>
  )
}

function draftId(row: ConfigItem) {
  return `${row.config_category ?? row.tab}:${row.config_key ?? row.key}:${row.scope ?? 'global'}`
}

function draftMapFromRows(rows: ConfigItem[], source: DraftMap = {}): DraftMap {
  return Object.fromEntries(rows.map((row) => {
    const id = draftId(row)
    const value = id in source ? source[id] : extractConfigValue(row)
    return [id, cloneConfigValue(value) as ConfigValue]
  }))
}

function cloneDraftMap(source: DraftMap): DraftMap {
  return Object.fromEntries(Object.entries(source).map(([key, value]) => [key, cloneConfigValue(value) as ConfigValue]))
}

function cloneConfigValue(value: ConfigValue | undefined): ConfigValue | undefined {
  if (value === undefined) return undefined
  try {
    return JSON.parse(JSON.stringify(value)) as ConfigValue
  } catch {
    return value
  }
}
