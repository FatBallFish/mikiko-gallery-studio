import { useEffect, useMemo, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import {
  type AdminSession,
  type ConfigItem,
  type StorageConfig,
  type StorageConfigWriteRequest,
  type StorageMigrationResult,
  type StorageStatsItem,
} from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, EmptyBlock, ErrorBlock, Field, LoadingBlock, PageHeader } from '../components'
import { canAdmin } from '../types'
import { adminButton, adminPage } from '../ui/classes'
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
type TopConfigTab = 'general' | 'security' | 'storage' | 'email' | 'payment'

const topConfigTabs: Array<{ key: TopConfigTab; label: string; detail: string }> = [
  { key: 'general', label: '通用设置', detail: '生成限制、公开内容、开发文档和兼容接口。' },
  { key: 'security', label: '安全策略', detail: '登录会话、Token、Cookie 和认证安全。' },
  { key: 'storage', label: '存储配置', detail: '本地存储、BFSS/S3 和对象访问策略。' },
  { key: 'email', label: '邮箱配置', detail: 'SMTP、发信人、TLS 和测试邮件。' },
  { key: 'payment', label: '支付设置', detail: '收银台、支付实例、套餐和充值策略。' },
]

const configClasses = {
  statusStrip: 'grid grid-cols-4 gap-4 max-[920px]:grid-cols-2 max-[620px]:grid-cols-1',
  statusCell: 'min-w-0 border-r border-[var(--line)] px-4 py-3 last:border-r-0 max-[620px]:border-r-0 max-[620px]:border-b max-[620px]:last:border-b-0',
  statusLabel: 'block text-[10px] font-extrabold uppercase tracking-[.14em] text-[var(--soft)]',
  statusValue: 'mt-1 block truncate text-[var(--text)]',
  statusCard: 'rounded-3xl border border-white/5 bg-white/[0.02] p-5',
  board: 'grid min-h-0 grid-cols-[minmax(0,1fr)_280px] overflow-hidden rounded-3xl border border-white/5 bg-white/[0.01] max-[1260px]:grid-cols-1',
  topTabs: 'flex min-w-0 gap-2 overflow-x-auto rounded-3xl border border-[var(--line)] bg-white/[0.02] p-2',
  topTab: 'grid min-h-12 min-w-[138px] shrink-0 gap-1 rounded-2xl border border-transparent px-4 py-2 text-left text-sm font-extrabold text-[var(--soft)] transition hover:border-[var(--accent)]/25 hover:bg-[var(--accent)]/10 hover:text-[var(--accent)]',
  topTabActive: 'border-[var(--accent)]/25 bg-[var(--accent)]/10 text-[var(--accent)]',
  topTabDetail: 'block text-xs font-semibold normal-case leading-snug text-[var(--muted)]',
  lane: 'min-w-0 overflow-y-auto border-r border-[var(--line)] px-[18px] py-4 max-[1260px]:border-r-0',
  head: 'mb-3 flex flex-wrap items-center justify-between gap-3 border-b border-[var(--line)] pb-3',
  headTitle: 'block text-[var(--text)]',
  headDetail: 'm-0 mt-1 text-sm text-[var(--soft)]',
  permissionNote: 'rounded-xl border border-[rgba(184,135,64,.28)] bg-[rgba(184,135,64,.08)] px-3 py-2 text-sm text-[var(--amber)]',
  formGrid: 'mt-4 grid grid-cols-[repeat(2,minmax(220px,1fr))] items-start gap-3.5 max-[760px]:grid-cols-1',
  formItem: 'grid min-w-0 self-start gap-2',
  sideRail: 'grid min-w-0 content-start overflow-y-auto bg-white/[0.02]',
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
  const [activeTopTab, setActiveTopTab] = useState<TopConfigTab>('general')
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
    const keys = Array.from(new Set(rows.map((row) => row.config_category || row.tab)
      .filter((key): key is string => Boolean(key) && topConfigTabForCategory(key) === activeTopTab)))
    return keys.map((key) => ({ key, label: configTabMeta(key).label }))
  }, [activeTopTab, rows])

  useEffect(() => {
    if (!tabs.length) return
    if (!tabs.some((tab) => tab.key === activeTab)) {
      setActiveTab(tabs[0].key)
    }
  }, [activeTab, tabs])

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
    <section className={adminPage.stack}>
      {!compact ? (
        <PageHeader
          eyebrow="Settings"
          title="系统设置"
          detail="按类目维护配置表单，保存时统一提交当前类目。"
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

      {!summaryMode ? (
        <section className={configClasses.topTabs} role="tablist" aria-label="系统设置分类">
          {topConfigTabs.map((tab) => {
            const selected = activeTopTab === tab.key
            const count = rows.filter((row) => topConfigTabForCategory(row.config_category || row.tab || '') === tab.key).length
            return (
              <button
                key={tab.key}
                type="button"
                role="tab"
                aria-selected={selected}
                className={cn(configClasses.topTab, selected && configClasses.topTabActive)}
                onClick={() => {
                  setActiveTopTab(tab.key)
                  const firstCategory = firstCategoryForTopTab(rows, tab.key)
                  if (firstCategory) setActiveTab(firstCategory)
                }}
              >
                <span>{tab.label}{count ? ` · ${count}` : ''}</span>
                <span className={configClasses.topTabDetail}>{tab.detail}</span>
              </button>
            )
          })}
        </section>
      ) : null}

      {summaryMode ? (
        <details className="group rounded-3xl border border-white/5 bg-white/[0.02] p-4">
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
            />
          </div>
        </details>
      ) : activeTopTab === 'storage' ? (
        <StorageSettingsPanel onFeedback={onFeedback} />
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
}) {
  return (
    <section className={configClasses.board}>
        <section className={configClasses.lane}>
          <div className={configClasses.head}>
            <div>
              <strong className={configClasses.headTitle}>{activeMeta.label}</strong>
              <p className={configClasses.headDetail}>{activeMeta.detail}</p>
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
          {tabs.length > 1 ? (
            <nav className={adminPage.microTabs} aria-label="系统设置子类目">
              {tabs.map((tab) => (
                <button
                  key={tab.key}
                  className={cn(adminPage.microTab, activeTab === tab.key && adminPage.microTabActive)}
                  type="button"
                  onClick={() => setActiveTab(tab.key)}
                >
                  {tab.label}
                </button>
              ))}
            </nav>
          ) : null}
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

        <aside className={configClasses.sideRail}>
          <section className={configClasses.sideStrip}><label className={configClasses.sideLabel}>保存反馈</label><strong className={configClasses.sideStrong}>{notice}</strong></section>
          <section className={configClasses.sideStrip}><label className={configClasses.sideLabel}>提示</label><p className={configClasses.sideText}>鼠标悬停字段名旁的提示符可查看用途说明；复杂列表以结构化文本编辑，保存后仍按原始接口契约提交。</p></section>
        </aside>
      </section>
  )
}

const emptyStorageDraft: StorageConfigWriteRequest = {
  code: '',
  name: '',
  driver: 'bfss',
  endpoint: '',
  region: 'us-east-1',
  bucket: '',
  prefix: '',
  force_path_style: true,
  access_key_id: '',
  secret_access_key: '',
  status: 'active',
}

function StorageSettingsPanel({ onFeedback }: { onFeedback: (title: string, detail?: string) => void }) {
  const [configs, setConfigs] = useState<StorageConfig[]>([])
  const [stats, setStats] = useState<StorageStatsItem[]>([])
  const [draft, setDraft] = useState<StorageConfigWriteRequest>(emptyStorageDraft)
  const [editingID, setEditingID] = useState<string | number | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testingID, setTestingID] = useState<string | number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [migration, setMigration] = useState({
    source_storage_config_id: '',
    target_storage_config_id: '',
    object_roles: ['generated_image', 'reference_asset'],
    dry_run: true,
    update_records: true,
  })
  const [migrationResult, setMigrationResult] = useState<StorageMigrationResult | null>(null)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const [nextConfigs, nextStats] = await Promise.all([
        adminApi.listStorageConfigs(),
        adminApi.listStorageStats(),
      ])
      setConfigs(nextConfigs)
      setStats(nextStats)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '存储配置载入失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const editConfig = (item: StorageConfig) => {
    setEditingID(item.id)
    setDraft({
      code: item.code,
      name: item.name,
      driver: item.driver,
      endpoint: item.endpoint ?? '',
      region: item.region ?? '',
      bucket: item.bucket,
      prefix: item.prefix ?? '',
      force_path_style: item.force_path_style,
      access_key_id: '',
      secret_access_key: '',
      status: item.status,
    })
  }

  const resetDraft = () => {
    setEditingID(null)
    setDraft(emptyStorageDraft)
  }

  const save = async () => {
    if (!draft.code.trim() || !draft.name.trim() || !draft.bucket.trim()) {
      setError('请填写存储编码、名称和 bucket/本地根目录。')
      return
    }
    setSaving(true)
    setError(null)
    try {
      const payload = {
        ...draft,
        code: draft.code.trim(),
        name: draft.name.trim(),
        endpoint: draft.endpoint?.trim() ?? '',
        region: draft.region?.trim() ?? '',
        bucket: draft.bucket.trim(),
        prefix: draft.prefix?.trim() ?? '',
      }
      const saved = editingID ? await adminApi.updateStorageConfig(editingID, payload) : await adminApi.createStorageConfig(payload)
      onFeedback(editingID ? '存储配置已更新' : '存储配置已创建', saved.name)
      resetDraft()
      await load()
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '保存存储配置失败')
    } finally {
      setSaving(false)
    }
  }

  const testConfig = async (id: string | number) => {
    setTestingID(id)
    setError(null)
    try {
      const result = await adminApi.testStorageConfig(id)
      onFeedback(result.status === 'passed' ? '连接测试通过' : '连接测试失败', result.error || `${result.latency_ms}ms`)
      await load()
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '连接测试失败')
    } finally {
      setTestingID(null)
    }
  }

  const setDefault = async (id: string | number) => {
    setError(null)
    try {
      const result = await adminApi.setDefaultStorageConfig(id)
      onFeedback('默认写入存储已切换', result.name)
      await load()
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '设置默认写入失败')
    }
  }

  const submitMigration = async () => {
    const targetID = Number(migration.target_storage_config_id)
    if (!Number.isFinite(targetID) || targetID <= 0) {
      setError('请选择目标存储。')
      return
    }
    setSaving(true)
    setError(null)
    try {
      const sourceID = Number(migration.source_storage_config_id)
      const result = await adminApi.createStorageMigration({
        source_storage_config_id: Number.isFinite(sourceID) && sourceID > 0 ? sourceID : null,
        target_storage_config_id: targetID,
        scope: { object_roles: migration.object_roles },
        dry_run: migration.dry_run,
        update_records: migration.update_records,
      })
      setMigrationResult(result)
      onFeedback(migration.dry_run ? '迁移 dry run 已完成' : '迁移任务已执行', `${result.job.processed_items}/${result.job.total_items} 对象`)
      await load()
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '创建迁移任务失败')
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <LoadingBlock label="载入存储配置" />
  if (error && !configs.length) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className="grid gap-4">
      {error ? <ErrorBlock message={error} onRetry={() => setError(null)} /> : null}
      <section className="grid gap-3 rounded-3xl border border-white/5 bg-white/[0.02] p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <strong className="text-[var(--text)]">对象存储配置</strong>
            <p className="m-0 mt-1 text-sm text-[var(--soft)]">支持 local、S3 和 BFSS；默认写入只影响新图片，历史图片按记录读取。</p>
          </div>
          <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={resetDraft}>新建配置</button>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full min-w-[860px] text-left text-sm">
            <thead className="text-xs uppercase tracking-wider text-[var(--soft)]">
              <tr>
                <th className="py-2 pr-3">编码</th>
                <th className="py-2 pr-3">类型</th>
                <th className="py-2 pr-3">Bucket/根目录</th>
                <th className="py-2 pr-3">状态</th>
                <th className="py-2 pr-3">测试</th>
                <th className="py-2 pr-3">操作</th>
              </tr>
            </thead>
            <tbody>
              {configs.map((item) => (
                <tr key={String(item.id)} className="border-t border-[var(--line)] align-top">
                  <td className="py-3 pr-3">
                    <div className="font-bold text-[var(--text)]">{item.code}</div>
                    <div className="text-xs text-[var(--soft)]">{item.name}</div>
                  </td>
                  <td className="py-3 pr-3">{item.driver}{item.is_default_write ? <Badge tone="success">默认写入</Badge> : null}</td>
                  <td className="py-3 pr-3">
                    <div className="font-mono text-xs text-[var(--text)]">{item.bucket}</div>
                    {item.prefix ? <div className="text-xs text-[var(--soft)]">prefix: {item.prefix}</div> : null}
                  </td>
                  <td className="py-3 pr-3"><Badge tone={item.status === 'active' ? 'success' : 'warning'}>{item.status}</Badge></td>
                  <td className="py-3 pr-3">
                    <Badge tone={item.last_test_status === 'passed' ? 'success' : item.last_test_status === 'failed' ? 'danger' : 'warning'}>{item.last_test_status || 'unknown'}</Badge>
                    {item.last_test_error ? <div className="mt-1 max-w-[220px] truncate text-xs text-[var(--danger)]">{item.last_test_error}</div> : null}
                  </td>
                  <td className="py-3 pr-3">
                    <div className="flex flex-wrap gap-2">
                      <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={() => editConfig(item)}>编辑</button>
                      <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={() => void testConfig(item.id)} disabled={testingID === item.id}>{testingID === item.id ? '测试中' : '测试'}</button>
                      <button type="button" className={cn(adminButton.base, adminButton.primary, adminButton.small)} onClick={() => void setDefault(item.id)} disabled={item.is_default_write || item.status !== 'active'}>设为默认</button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section className="grid gap-4 rounded-3xl border border-white/5 bg-white/[0.02] p-4">
        <strong className="text-[var(--text)]">{editingID ? '编辑存储配置' : '新增存储配置'}</strong>
        <div className="grid grid-cols-2 gap-3 max-[760px]:grid-cols-1">
          <Field label="存储编码"><input value={draft.code} onChange={(event) => setDraft((current) => ({ ...current, code: event.target.value }))} /></Field>
          <Field label="展示名称"><input value={draft.name} onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))} /></Field>
          <Field label="驱动类型">
            <select value={draft.driver} onChange={(event) => setDraft((current) => ({ ...current, driver: event.target.value }))}>
              <option value="bfss">BFSS</option>
              <option value="s3">S3</option>
              <option value="local">Local</option>
            </select>
          </Field>
          <Field label={draft.driver === 'local' ? '本地根目录' : 'Bucket'}>
            <input value={draft.bucket} onChange={(event) => setDraft((current) => ({ ...current, bucket: event.target.value }))} placeholder={draft.driver === 'local' ? '/home/pic-gallery/storage' : 'generated-assets'} />
          </Field>
          <Field label="Endpoint"><input value={draft.endpoint ?? ''} onChange={(event) => setDraft((current) => ({ ...current, endpoint: event.target.value }))} disabled={draft.driver === 'local'} /></Field>
          <Field label="Region"><input value={draft.region ?? ''} onChange={(event) => setDraft((current) => ({ ...current, region: event.target.value }))} disabled={draft.driver === 'local'} /></Field>
          <Field label="Prefix"><input value={draft.prefix ?? ''} onChange={(event) => setDraft((current) => ({ ...current, prefix: event.target.value }))} /></Field>
          <Field label="状态">
            <select value={draft.status} onChange={(event) => setDraft((current) => ({ ...current, status: event.target.value }))}>
              <option value="active">active</option>
              <option value="disabled">disabled</option>
            </select>
          </Field>
          <Field label="Access Key"><input value={draft.access_key_id ?? ''} onChange={(event) => setDraft((current) => ({ ...current, access_key_id: event.target.value }))} disabled={draft.driver === 'local'} placeholder={editingID ? '留空则保留原密钥' : ''} autoComplete="off" name="storage-access-key-id" /></Field>
          <Field label="Secret Key"><input type="password" value={draft.secret_access_key ?? ''} onChange={(event) => setDraft((current) => ({ ...current, secret_access_key: event.target.value }))} disabled={draft.driver === 'local'} placeholder={editingID ? '留空则保留原密钥' : ''} autoComplete="new-password" name="storage-secret-access-key-new" /></Field>
        </div>
        <label className="inline-flex items-center gap-2 text-sm font-bold text-[var(--soft)]">
          <input type="checkbox" checked={Boolean(draft.force_path_style)} onChange={(event) => setDraft((current) => ({ ...current, force_path_style: event.target.checked }))} disabled={draft.driver === 'local'} />
          S3 path style
        </label>
        <div className="flex flex-wrap gap-2">
          <button type="button" className={cn(adminButton.base, adminButton.primary)} onClick={() => void save()} disabled={saving}>{saving ? '保存中...' : '保存存储配置'}</button>
          <button type="button" className={cn(adminButton.base, adminButton.ghost)} onClick={resetDraft} disabled={saving}>清空</button>
        </div>
      </section>

      <section className="grid gap-4 rounded-3xl border border-white/5 bg-white/[0.02] p-4">
        <strong className="text-[var(--text)]">容量统计</strong>
        <div className="grid grid-cols-[repeat(auto-fit,minmax(220px,1fr))] gap-3">
          {stats.map((item) => (
            <div key={`${item.storage_config_id ?? 'legacy'}:${item.storage_code}`} className="rounded-2xl border border-[var(--line)] bg-black/10 p-3">
              <label className="text-[10px] font-extrabold uppercase tracking-widest text-[var(--soft)]">{item.storage_code || 'legacy'}</label>
              <strong className="mt-2 block text-[var(--text)]">{formatBytes(item.total_bytes)}</strong>
              <p className="m-0 mt-1 text-sm text-[var(--soft)]">{item.image_count} 张，生成 {item.generated_image_count} / 参考 {item.reference_asset_count}</p>
              <p className="m-0 mt-1 truncate text-xs text-[var(--soft)]">{item.bucket}</p>
            </div>
          ))}
        </div>
      </section>

      <section className="grid gap-4 rounded-3xl border border-white/5 bg-white/[0.02] p-4">
        <strong className="text-[var(--text)]">BFSS 迁移同步</strong>
        <div className="grid grid-cols-2 gap-3 max-[760px]:grid-cols-1">
          <Field label="源存储"><select value={migration.source_storage_config_id} onChange={(event) => setMigration((current) => ({ ...current, source_storage_config_id: event.target.value }))}><option value="">legacy / 未指定</option>{configs.map((item) => <option key={String(item.id)} value={String(item.id)}>{item.code}</option>)}</select></Field>
          <Field label="目标存储"><select value={migration.target_storage_config_id} onChange={(event) => setMigration((current) => ({ ...current, target_storage_config_id: event.target.value }))}><option value="">请选择</option>{configs.filter((item) => item.status === 'active').map((item) => <option key={String(item.id)} value={String(item.id)}>{item.code}</option>)}</select></Field>
        </div>
        <div className="flex flex-wrap gap-4 text-sm font-bold text-[var(--soft)]">
          {['generated_image', 'reference_asset'].map((role) => (
            <label key={role} className="inline-flex items-center gap-2">
              <input type="checkbox" checked={migration.object_roles.includes(role)} onChange={(event) => setMigration((current) => ({ ...current, object_roles: event.target.checked ? [...current.object_roles, role] : current.object_roles.filter((item) => item !== role) }))} />
              {role}
            </label>
          ))}
          <label className="inline-flex items-center gap-2"><input type="checkbox" checked={migration.dry_run} onChange={(event) => setMigration((current) => ({ ...current, dry_run: event.target.checked }))} />Dry run</label>
          <label className="inline-flex items-center gap-2"><input type="checkbox" checked={migration.update_records} onChange={(event) => setMigration((current) => ({ ...current, update_records: event.target.checked }))} />同步更新记录</label>
        </div>
        <button type="button" className={cn(adminButton.base, adminButton.primary)} onClick={() => void submitMigration()} disabled={saving}>{migration.dry_run ? '执行 Dry Run' : '执行迁移'}</button>
        {migrationResult ? (
          <div className="rounded-2xl border border-[var(--line)] bg-black/10 p-3 text-sm text-[var(--soft)]">
            <strong className="block text-[var(--text)]">Job {migrationResult.job.job_id}</strong>
            <span>{migrationResult.job.status} · {migrationResult.job.processed_items}/{migrationResult.job.total_items} · {formatBytes(migrationResult.job.total_bytes)}</span>
          </div>
        ) : null}
      </section>
    </section>
  )
}

function summaryValue(value: ConfigValue) {
  if (typeof value === 'boolean') return value ? '开启' : '关闭'
  if (Array.isArray(value)) return `${value.length} 项`
  if (isRecord(value)) return Object.keys(value).join(', ') || '-'
  return String(value ?? '-')
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = value
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index += 1
  }
  return `${size.toFixed(index === 0 ? 0 : 1)} ${units[index]}`
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

function topConfigTabForCategory(category: string): TopConfigTab {
  const key = category.trim().toLowerCase()
  if (key === 'auth_security' || key.includes('security') || key.includes('auth')) return 'security'
  if (key === 'payments' || key === 'billing_trial' || key.includes('payment') || key.includes('cashier')) return 'payment'
  if (key.includes('storage') || key.includes('bfss') || key.includes('s3')) return 'storage'
  if (key.includes('smtp') || key.includes('email') || key.includes('mail')) return 'email'
  return 'general'
}

function firstCategoryForTopTab(rows: ConfigItem[], topTab: TopConfigTab) {
  return rows.map((row) => row.config_category || row.tab)
    .find((key): key is string => Boolean(key) && topConfigTabForCategory(key) === topTab) || ''
}
