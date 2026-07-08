import { useEffect, useMemo, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import { type AdminSession, type ConfigItem } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { AdminTabs, Badge, EmptyBlock, ErrorBlock, Field, LoadingBlock, PageHeader } from '../components'
import { canAdmin } from '../types'
import { adminButton, adminPage } from '../ui/classes'
import {
  configFieldMeta,
  configLockedDetail,
  configPermission,
  configTabMeta,
  configValidateValue,
  extractConfigValue,
  generalConfigCategories,
  inferConfigFieldType,
  isRecord,
  isSameConfigValue,
  normalizeDraftValue,
  type ConfigFieldMeta,
  type ConfigValue,
} from './configRows'

type DraftMap = Record<string, ConfigValue>

const configClasses = {
  statusStrip: 'grid grid-cols-4 gap-4 max-[920px]:grid-cols-2 max-[620px]:grid-cols-1',
  statusCell: 'min-w-0 border-r border-[var(--line)] px-4 py-3 last:border-r-0 max-[620px]:border-r-0 max-[620px]:border-b max-[620px]:last:border-b-0',
  statusLabel: 'block text-[10px] font-extrabold uppercase tracking-[.14em] text-[var(--soft)]',
  statusValue: 'mt-1 block truncate text-[var(--text)]',
  statusCard: 'rounded-lg border border-[var(--border)] bg-[var(--surface)] p-5',
  board: 'grid min-h-0 grid-cols-[220px_minmax(0,1fr)] overflow-hidden rounded-lg border border-[var(--border)] bg-[var(--surface)] max-[1260px]:grid-cols-1',
  boardFull: 'grid min-h-0 grid-cols-[220px_minmax(0,1fr)_280px] overflow-hidden rounded-lg border border-[var(--border)] bg-[var(--surface)] max-[1260px]:grid-cols-1',
  rail: 'border-r border-[var(--border)] bg-[var(--surface)] p-3 max-[1260px]:border-r-0 max-[1260px]:border-b',
  lane: 'min-w-0 overflow-y-auto border-r border-[var(--line)] px-[18px] py-4 max-[1260px]:border-r-0',
  head: 'mb-3 flex flex-wrap items-center justify-between gap-3 border-b border-[var(--line)] pb-3',
  headTitle: 'block text-[var(--text)]',
  headDetail: 'm-0 mt-1 text-sm text-[var(--soft)]',
  permissionNote: 'rounded-xl border border-[rgba(184,135,64,.28)] bg-[rgba(184,135,64,.08)] px-3 py-2 text-sm text-[var(--amber)]',
  formGrid: 'mt-4 grid grid-cols-[repeat(2,minmax(220px,1fr))] items-start gap-3.5 max-[760px]:grid-cols-1',
  formItem: 'grid min-w-0 self-start gap-2',
  sideRail: 'grid min-w-0 content-start overflow-y-auto bg-[var(--surface)]',
  sideStrip: 'border-b border-[var(--line)] px-4 py-[15px] last:border-b-0',
  sideLabel: 'block text-[10px] font-extrabold uppercase tracking-[.14em] text-[var(--soft)]',
  sideStrong: 'mt-2 block text-[var(--text)]',
  sideText: 'm-0 mt-2 text-sm text-[var(--soft)]',
  kvList: 'grid gap-2',
  kvRow: 'grid grid-cols-[minmax(90px,.9fr)_minmax(100px,1fr)] gap-2 max-[620px]:grid-cols-1',
}

export function ConfigPage({
  session,
  onFeedback,
  compact = false,
  summaryMode = false,
}: {
  session: AdminSession
  onFeedback: (title: string, detail?: string) => void
  compact?: boolean
  summaryMode?: boolean
}) {
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
      const generalRows = nextRows.filter(isGeneralConfigRow)
      setRows(generalRows)
      setDrafts(Object.fromEntries(generalRows.map((row) => [draftId(row), extractConfigValue(row)])))
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
  const sampleFields = activeRows.slice(0, 4)

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
      const generalRows = nextRows.filter(isGeneralConfigRow)
      setRows(generalRows)
      setDrafts(Object.fromEntries(generalRows.map((row) => [draftId(row), extractConfigValue(row)])))
      setNotice(`${activeMeta.label}已保存，API 节点将在 1 分钟内读取新配置。`)
      onFeedback('通用配置已保存', activeMeta.label)
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

  if (loading) return <LoadingBlock label="载入通用配置" />
  if (error) return <ErrorBlock message={error} onRetry={load} />
  if (!rows.length) return <EmptyBlock title="暂无通用配置项" detail="配置中心尚未返回文档、公开内容等低风险配置。" />

  return (
    <section className={adminPage.stack}>
      {!compact ? (
        <PageHeader
          title="通用配置"
          detail="只维护文档、公开内容等低风险配置；认证、支付、审核和生成限制已拆到对应独立页面。"
          actions={
            <>
              <button type="button" className={cn(adminButton.base, adminButton.ghost)} onClick={revertActiveTab} disabled={saving || !activeDirty}>恢复本类</button>
              <button type="button" className={cn(adminButton.base, adminButton.primary)} onClick={() => void saveActiveTab()} disabled={saving || Boolean(conflicts.length) || !activeDirty || !canEditActiveTab}>{saving ? '保存中...' : '保存本类'}</button>
            </>
          }
        />
      ) : null}

      {!summaryMode ? (
        <section className={configClasses.statusStrip}>
          <div className={configClasses.statusCard}><label className={configClasses.statusLabel}>当前类目</label><strong className={configClasses.statusValue}>{activeMeta.label}</strong></div>
          <div className={configClasses.statusCard}><label className={configClasses.statusLabel}>字段</label><strong className={configClasses.statusValue}>{activeRows.length} 项</strong></div>
          <div className={configClasses.statusCard}><label className={configClasses.statusLabel}>未保存</label><strong className={configClasses.statusValue}>{dirtyKeys.length} 项</strong></div>
          <div className={configClasses.statusCard}><label className={configClasses.statusLabel}>校验</label><strong className={configClasses.statusValue}>{conflicts.length ? `${conflicts.length} 项需处理` : '通过'}</strong></div>
        </section>
      ) : (
        <section className="grid grid-cols-1 gap-6 md:grid-cols-2">
          {sampleFields.map((row, index) => {
            const key = row.config_key ?? row.key
            const value = extractConfigValue(row)
            const label = configFieldMeta(key, row.description).label || key
            return (
              <div key={`${draftId(row)}:summary`} className="space-y-2">
                <label className="text-[10px] font-bold uppercase tracking-widest text-[var(--muted-strong)]">{label}</label>
                <div className="relative">
                  <input readOnly value={summaryValue(value)} className="w-full" />
                  {index === 1 ? <span className="absolute right-4 top-1/2 size-2 -translate-y-1/2 rounded-full bg-emerald-500" /> : null}
                </div>
              </div>
            )
          })}
        </section>
      )}

      {summaryMode ? (
        <details className="group rounded-lg border border-[var(--border)] bg-[var(--surface)] p-4">
          <summary className="cursor-pointer list-none text-sm font-bold text-[var(--accent)]">编辑通用配置</summary>
          <div className="mt-4">
            <ConfigEditor
              activeMeta={activeMeta}
              tabs={tabs}
              activeTab={activeTab}
              setActiveTab={setActiveTab}
              activeRows={activeRows}
              drafts={drafts}
              setDrafts={setDrafts}
              conflicts={conflicts}
              canEditActiveTab={canEditActiveTab}
              lockedDetail={lockedDetail}
              saving={saving}
              activeDirty={activeDirty}
              compact={compact}
              notice={notice}
              revertActiveTab={revertActiveTab}
              saveActiveTab={saveActiveTab}
              summaryMode={summaryMode}
            />
          </div>
        </details>
      ) : (
        <ConfigEditor
          activeMeta={activeMeta}
          tabs={tabs}
          activeTab={activeTab}
          setActiveTab={setActiveTab}
          activeRows={activeRows}
          drafts={drafts}
          setDrafts={setDrafts}
          conflicts={conflicts}
          canEditActiveTab={canEditActiveTab}
          lockedDetail={lockedDetail}
          saving={saving}
          activeDirty={activeDirty}
          compact={compact}
          notice={notice}
          revertActiveTab={revertActiveTab}
          saveActiveTab={saveActiveTab}
        />
      )}
    </section>
  )
}

function isGeneralConfigRow(row: ConfigItem) {
  const category = row.config_category || row.tab
  return generalConfigCategories.includes(category as (typeof generalConfigCategories)[number])
}

function ConfigEditor({
  activeMeta,
  tabs,
  activeTab,
  setActiveTab,
  activeRows,
  drafts,
  setDrafts,
  conflicts,
  canEditActiveTab,
  lockedDetail,
  saving,
  activeDirty,
  compact,
  notice,
  revertActiveTab,
  saveActiveTab,
  summaryMode = false,
}: {
  activeMeta: ReturnType<typeof configTabMeta>
  tabs: Array<{ key: string; label: string }>
  activeTab: string
  setActiveTab: (tab: string) => void
  activeRows: ConfigItem[]
  drafts: DraftMap
  setDrafts: Dispatch<SetStateAction<DraftMap>>
  conflicts: Array<{ key: string; message: string }>
  canEditActiveTab: boolean
  lockedDetail: string
  saving: boolean
  activeDirty: boolean
  compact: boolean
  notice: string
  revertActiveTab: () => void
  saveActiveTab: () => Promise<void>
  summaryMode?: boolean
}) {
  return (
    <section className={summaryMode ? configClasses.board : configClasses.boardFull}>
        <aside className={configClasses.rail}>
          <AdminTabs
            ariaLabel="通用配置类目"
            orientation="vertical"
            items={tabs.map((tab) => ({ id: tab.key, label: tab.label }))}
            value={activeTab}
            onChange={setActiveTab}
          />
        </aside>

        <section className={configClasses.lane}>
          <div className={configClasses.head}>
            <div>
              <strong className={configClasses.headTitle}>{activeMeta.label}</strong>
              <p className={configClasses.headDetail}>{activeMeta.detail}</p>
              {summaryMode ? <p className={cn(configClasses.headDetail, 'text-[var(--accent)]')}>{notice}</p> : null}
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Badge tone={!canEditActiveTab || conflicts.length ? 'warning' : 'success'}>{!canEditActiveTab ? '只读' : conflicts.length ? '需修正' : '可保存'}</Badge>
              {compact ? (
                <>
                  <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={revertActiveTab} disabled={saving || !activeDirty}>恢复本类</button>
                  <button type="button" className={cn(adminButton.base, adminButton.primary, adminButton.small)} onClick={() => void saveActiveTab()} disabled={saving || Boolean(conflicts.length) || !activeDirty || !canEditActiveTab}>{saving ? '保存中...' : '保存本类'}</button>
                </>
              ) : null}
            </div>
          </div>
          {!canEditActiveTab ? <div className={configClasses.permissionNote}>{lockedDetail}</div> : null}

          <div className={configClasses.formGrid}>
            {activeRows.map((row) => {
              const key = row.config_key ?? row.key
              const meta = configFieldMeta(key, row.description)
              const rowDraftId = draftId(row)
              const conflict = configValidateValue(key, drafts[rowDraftId])
              return (
                <div key={rowDraftId} className={configClasses.formItem}>
                  {renderConfigField(row, meta, drafts[rowDraftId], (value) => setDrafts((current) => ({ ...current, [rowDraftId]: value })), conflict, !canEditActiveTab)}
                </div>
              )
            })}
          </div>
        </section>

        {summaryMode ? null : (
          <aside className={configClasses.sideRail}>
            <section className={configClasses.sideStrip}><label className={configClasses.sideLabel}>保存反馈</label><strong className={configClasses.sideStrong}>{notice}</strong></section>
            <section className={configClasses.sideStrip}><label className={configClasses.sideLabel}>提示</label><p className={configClasses.sideText}>鼠标悬停字段名旁的提示符可查看用途说明；复杂列表以结构化文本编辑，保存后仍按原始接口契约提交。</p></section>
          </aside>
        )}
      </section>
  )
}

function summaryValue(value: ConfigValue) {
  if (typeof value === 'boolean') return value ? '开启' : '关闭'
  if (Array.isArray(value)) return `${value.length} 项`
  if (isRecord(value)) return Object.keys(value).join(', ') || '-'
  return String(value ?? '-')
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
