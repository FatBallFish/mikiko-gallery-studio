import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import type { StorageConfigView, StorageConfigWriteRequest } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { ApiError } from '../../../shared/http-client'
import { Badge, ErrorBlock, Field, InlineFeedback, LoadingBlock, PageHeader, StatusCell, StatusStrip } from '../components'
import { adminButton, adminPage, adminType } from '../ui/classes'
import {
  activateSavedStorageConfig,
  storageActivationLabel,
  storageConfigNeedsProbe,
  storageDraftIsDirty,
  type StorageActivationPhase,
} from './storageActivation'

type StorageDraft = {
  id: string
  version: number
  code: string
  name: string
  driver: string
  provider: string
  status: string
  read_enabled: boolean
  write_enabled: boolean
  endpoint: string
  region: string
  bucket: string
  prefix: string
  force_path_style: boolean
  local_root: string
  public_base_url: string
  access_key_id: string
  secret_access_key: string
}

type StorageEditorState = 'pristine' | 'dirty' | 'validating' | 'saving' | 'saved' | 'failed'
type StorageFeedback = { tone: 'success' | 'warning' | 'danger' | 'neutral'; message: string }

const storageClasses = {
  grid: 'grid min-h-[560px] grid-cols-[280px_minmax(0,1fr)] overflow-hidden border border-[var(--border)] bg-[var(--surface-solid)] max-[920px]:min-h-0 max-[920px]:grid-cols-1',
  list: 'grid content-start gap-2 border-r border-[var(--border)] bg-[var(--surface)] p-3 max-[920px]:border-b max-[920px]:border-r-0',
  row: 'grid min-w-0 gap-2 rounded-lg border border-transparent bg-[var(--surface-solid)] p-3 text-left transition-colors duration-[var(--admin-motion-fast)] hover:border-[var(--border-strong)] hover:bg-[var(--elevated)]',
  rowActive: 'border-[var(--accent)]/30 bg-[var(--accent)]/10',
  rowTitle: 'flex min-w-0 items-center justify-between gap-2',
  rowBadges: 'flex flex-wrap items-center gap-1.5',
  mono: 'font-[family-name:var(--admin-font-mono)] text-xs text-[var(--soft)] [overflow-wrap:anywhere]',
  panel: 'grid min-w-0 content-start bg-[var(--surface-solid)]',
  head: 'flex flex-wrap items-start justify-between gap-3 border-b border-[var(--border)] p-4',
  section: 'grid min-w-0 gap-3 border-b border-[var(--border)] p-4',
  sectionHead: 'grid gap-1',
  actions: 'flex flex-wrap items-center justify-end gap-2',
  toggle: 'grid grid-cols-[auto_minmax(0,1fr)] items-center gap-2 rounded-lg bg-[var(--surface)] p-2 text-sm has-[:checked]:bg-[var(--accent)]/10',
  note: 'm-0 text-sm leading-5 text-[var(--soft)] [overflow-wrap:anywhere]',
  saveRail: 'sticky bottom-0 z-10 flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border)] bg-[var(--surface-solid)]/95 p-3 backdrop-blur',
  stateCopy: 'text-xs text-[var(--soft)]',
}

export function StorageConfigPage({
  onFeedback,
  onDirtyChange,
  onBusyChange,
  compact = false,
  summaryMode = false,
}: {
  onFeedback?: (title: string, detail?: string) => void
  onDirtyChange?: (dirty: boolean) => void
  onBusyChange?: (busy: boolean) => void
  compact?: boolean
  summaryMode?: boolean
}) {
  const [items, setItems] = useState<StorageConfigView[]>([])
  const [draft, setDraft] = useState<StorageDraft>(newStorageDraft())
  const [selectedID, setSelectedID] = useState('')
  const [initialLoading, setInitialLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [probing, setProbing] = useState(false)
  const [activationPhase, setActivationPhase] = useState<StorageActivationPhase>('idle')
  const [loadError, setLoadError] = useState<string | null>(null)
  const [feedback, setFeedback] = useState<StorageFeedback | null>(null)

  const selected = useMemo(() => selectedID ? items.find((item) => item.id === selectedID) ?? null : null, [items, selectedID])
  const defaultItem = items.find((item) => item.is_default)
  const isDirty = useMemo(() => storageEditorIsDirty(draft, selected), [draft, selected])
  const editorState = storageEditorState({ saving, probing, activationPhase, isDirty, feedback })
  const editorLocked = saving || probing || refreshing || activationPhase !== 'idle'
  const namespaceLocked = Boolean(draft.id)
  const probeActionLabel = !draft.id || isDirty ? '测试草稿连接' : '探测已保存配置'

  async function load(refreshID?: string, initial = false) {
    if (initial) {
      setInitialLoading(true)
      setLoadError(null)
    } else {
      setRefreshing(true)
    }
    try {
      const next = await adminApi.listStorageConfigs()
      setItems(next)
      const keep = next.find((item) => item.id === (refreshID ?? selectedID)) ?? next[0]
      if (keep) {
        setSelectedID(keep.id)
        setDraft(draftFromStorage(keep))
      } else {
        setSelectedID('')
        setDraft(newStorageDraft())
      }
      setLoadError(null)
      return next
    } catch (caught) {
      setLoadError(caught instanceof Error ? caught.message : '存储配置读取失败')
      return undefined
    } finally {
      if (initial) setInitialLoading(false)
      else setRefreshing(false)
    }
  }

  useEffect(() => { void load(undefined, true) }, [])

  useEffect(() => {
    onDirtyChange?.(isDirty)
  }, [isDirty, onDirtyChange])

  useEffect(() => () => onDirtyChange?.(false), [onDirtyChange])

  useEffect(() => {
    onBusyChange?.(editorLocked)
  }, [editorLocked, onBusyChange])

  useEffect(() => () => onBusyChange?.(false), [onBusyChange])

  useEffect(() => {
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!isDirty) return
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', handleBeforeUnload)
    return () => window.removeEventListener('beforeunload', handleBeforeUnload)
  }, [isDirty])

  function confirmDiscardChanges() {
    return !isDirty || window.confirm('当前存储配置有未保存修改，确定放弃并离开吗？')
  }

  function selectItem(item: StorageConfigView) {
    if (draft.id === item.id) return
    if (!confirmDiscardChanges()) return
    setSelectedID(item.id)
    setDraft(draftFromStorage(item))
    setFeedback(null)
  }

  function startNewDraft() {
    if (!confirmDiscardChanges()) return
    setDraft(newStorageDraft())
    setSelectedID('')
    setFeedback(null)
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSaving(true)
    setFeedback(null)
    const payload = payloadFromDraft(draft)
    try {
      const saved = draft.id ? await adminApi.updateStorageConfig(draft.id, payload) : await adminApi.createStorageConfig(payload)
      onFeedback?.('存储配置已保存', saved.code)
      setSelectedID(saved.id)
      setDraft(draftFromStorage(saved))
      await load(saved.id)
      setFeedback({ tone: 'success', message: `配置 ${saved.code} 已保存。` })
    } catch (caught) {
      if (draft.id && isVersionConflict(caught)) {
        try {
          if (await recoverVersionConflictAndMaybeRetrySave(draft, payload)) return
        } catch (retryError) {
          setFeedback({ tone: 'danger', message: retryError instanceof Error ? retryError.message : '存储配置保存失败' })
          return
        }
      }
      setFeedback({ tone: 'danger', message: caught instanceof Error ? caught.message : '存储配置保存失败' })
    } finally {
      setSaving(false)
    }
  }

  async function recoverVersionConflictAndMaybeRetrySave(currentDraft: StorageDraft, payload: StorageConfigWriteRequest) {
    const nextItems = await adminApi.listStorageConfigs()
    setItems(nextItems)
    const latest = nextItems.find((item) => item.id === currentDraft.id)
    if (!latest) {
      setSelectedID('')
      setDraft(newStorageDraft())
      setFeedback({ tone: 'danger', message: '存储配置已被删除或不可用，请刷新后再操作。' })
      return true
    }
    setSelectedID(latest.id)
    if (selected && storageEditableSignature(latest) === storageEditableSignature(selected)) {
      const saved = await adminApi.updateStorageConfig(currentDraft.id, { ...payload, version: latest.version })
      onFeedback?.('存储配置已保存', saved.code)
      setDraft(draftFromStorage(saved))
      await load(saved.id)
      setFeedback({ tone: 'success', message: `配置 ${saved.code} 已使用最新版本保存。` })
      return true
    }
    setDraft(draftFromStorage(latest))
    setFeedback({ tone: 'danger', message: '存储配置已被其他操作更新，已刷新最新内容，请确认后再保存。' })
    return true
  }

  async function probeCurrent() {
    const shouldProbeDraft = !draft.id || isDirty
    setProbing(true)
    setFeedback(null)
    try {
      if (shouldProbeDraft) {
        const result = await adminApi.probeStorageConfigDraft(payloadFromDraft(draft))
        setFeedback({ tone: 'success', message: `草稿连接测试：${probeResultSummary(result)}` })
        onFeedback?.('草稿连接测试完成', probeResultSummary(result))
      } else {
        const updated = await adminApi.probeStorageConfig(draft.id)
        setDraft(draftFromStorage(updated))
        await load(updated.id)
        setFeedback({ tone: 'success', message: `已保存配置探测：${probeSummary(updated)}` })
        onFeedback?.('已保存配置探测完成', probeSummary(updated))
      }
    } catch (caught) {
      if (draft.id && !shouldProbeDraft) await load(draft.id)
      setFeedback({ tone: 'danger', message: caught instanceof Error ? caught.message : '连接测试失败' })
    } finally {
      setProbing(false)
    }
  }

  async function setDefault() {
    if (!draft.id || !selected || selected.id !== draft.id) return
    setFeedback(null)
    if (isDirty) {
      setFeedback({ tone: 'danger', message: '当前配置有未保存修改，请先保存后再设为默认。' })
      return
    }
    try {
      const updated = await activateSavedStorageConfig({
        draft,
        saved: selected,
        probe: adminApi.probeStorageConfig,
        setDefault: adminApi.setDefaultStorageConfig,
        onPhase: setActivationPhase,
      })
      onFeedback?.('默认存储已切换', updated.code)
      await load(updated.id)
      setFeedback({ tone: 'success', message: `默认写入目标已切换为 ${updated.code}。` })
    } catch (caught) {
      const message = caught instanceof Error ? caught.message : '默认存储切换失败'
      const refreshed = await load(draft.id)
      const latest = refreshed?.find((item) => item.id === draft.id)
      const probeMessage = latest?.last_probe?.status === 'failed' ? latest.last_probe.message : ''
      setFeedback({ tone: 'danger', message: probeMessage || message })
    } finally {
      setActivationPhase('idle')
    }
  }

  async function setReadOnly() {
    if (!draft.id) return
    if (isDirty) {
      setFeedback({ tone: 'danger', message: '当前配置有未保存修改，请先保存后再设为只读。' })
      return
    }
    setSaving(true)
    setFeedback(null)
    try {
      const updated = await adminApi.setStorageConfigStatus(draft.id, { version: draft.version, status: 'enabled', read_enabled: true, write_enabled: false })
      onFeedback?.('存储已设为只读', updated.code)
      await load(updated.id)
      setFeedback({ tone: 'success', message: `配置 ${updated.code} 已设为只读。` })
    } catch (caught) {
      setFeedback({ tone: 'danger', message: caught instanceof Error ? caught.message : '状态更新失败' })
    } finally {
      setSaving(false)
    }
  }

  function refreshConfigs() {
    if (!confirmDiscardChanges()) return
    setFeedback(null)
    void load(selectedID)
  }

  if (initialLoading) return <LoadingBlock label="读取存储配置" />
  if (loadError && items.length === 0) return <ErrorBlock message={loadError} onRetry={() => void load(undefined, true)} />

  const storageEditor = (
    <section data-admin-storage-editor className={storageClasses.grid}>
      <aside className={storageClasses.list} aria-label="存储对象列表">
        <button type="button" className={cn(storageClasses.row, !draft.id && storageClasses.rowActive)} disabled={editorLocked} onClick={startNewDraft}>
          <strong>新增存储配置</strong>
          <span className={storageClasses.mono}>S3 / R2 / Local</span>
          {!draft.id && isDirty ? <Badge tone="warning">未保存</Badge> : null}
        </button>
        {items.map((item) => (
          <button key={item.id} type="button" className={cn(storageClasses.row, selected?.id === item.id && draft.id && storageClasses.rowActive)} disabled={editorLocked} onClick={() => selectItem(item)}>
            <StorageRow item={item} dirty={Boolean(draft.id === item.id && isDirty)} />
          </button>
        ))}
      </aside>

      <form className={storageClasses.panel} aria-busy={editorLocked} onSubmit={(event) => void save(event)}>
        <header className={storageClasses.head}>
          <div>
            <h2 className={cn('m-0', adminType.sectionTitle)}>{draft.id ? '编辑存储实例' : '新增存储实例'}</h2>
            <p className={cn('mt-1', storageClasses.note)}>Access Key 和 Secret 只写不读；R2 使用 S3 协议，custom_s3 适配任意 S3 兼容端点。</p>
          </div>
          <div className={storageClasses.rowBadges}>
            <EditorStateBadge state={editorState} />
            {draft.id ? <Badge tone={selected?.is_default ? 'success' : 'neutral'}>{selected?.is_default ? '默认写入' : `v${draft.version}`}</Badge> : <Badge tone="primary">新配置</Badge>}
          </div>
        </header>

        {loadError ? <div className="px-4 pt-4"><InlineFeedback tone="danger" message={loadError} /></div> : null}
        {refreshing ? <div className="px-4 pt-4"><InlineFeedback tone="neutral" message="正在刷新配置列表与已保存基线。" /></div> : null}
        {feedback ? <div className="px-4 pt-4"><InlineFeedback tone={feedback.tone} message={feedback.message} /></div> : null}

        <fieldset disabled={editorLocked} className="contents">
        <section data-storage-section="identity" className={storageClasses.section}>
          <div className={storageClasses.sectionHead}>
            <h3 className={cn('m-0', adminType.sectionTitle)}>基本信息</h3>
            <p className={storageClasses.note}>标识实例类型、适配协议和启停状态。</p>
          </div>
          <div className={adminPage.formGrid}>
            <Field label="配置代码"><input value={draft.code} disabled={Boolean(draft.id)} onChange={(event) => setDraft({ ...draft, code: event.target.value })} placeholder="r2-prod" /></Field>
            <Field label="配置名称"><input value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} placeholder="Cloudflare R2 Prod" /></Field>
            <Field label="存储驱动"><select value={draft.driver} disabled={namespaceLocked} onChange={(event) => setDraft(driverTemplate({ ...draft, driver: event.target.value }))}><option value="local">Local</option><option value="s3">S3</option></select></Field>
            <Field label="服务提供方"><select value={draft.provider} disabled={namespaceLocked} onChange={(event) => setDraft(providerTemplate({ ...draft, provider: event.target.value }))}><option value="local">Local</option><option value="r2">Cloudflare R2</option><option value="aws_s3">AWS S3</option><option value="minio">MinIO</option><option value="custom_s3">自定义 S3</option></select></Field>
            <Field label="启用状态"><select value={draft.status} onChange={(event) => setDraft({ ...draft, status: event.target.value })}><option value="enabled">启用</option><option value="disabled">停用</option></select></Field>
          </div>
        </section>

        <section data-storage-section="location" className={storageClasses.section}>
          <div className={storageClasses.sectionHead}>
            <h3 className={cn('m-0', adminType.sectionTitle)}>对象定位</h3>
            <p className={storageClasses.note}>{namespaceLocked ? '对象地址创建后不可修改；迁移时请新建配置、验证后切换默认写入。' : '定义远端端点、桶、对象前缀或本地根目录。'}</p>
          </div>
          <div className={adminPage.formGrid}>
            <Field label="Endpoint"><input value={draft.endpoint} disabled={namespaceLocked} onChange={(event) => setDraft({ ...draft, endpoint: event.target.value })} placeholder="https://account.r2.cloudflarestorage.com" /></Field>
            <Field label="Region"><input value={draft.region} disabled={namespaceLocked} onChange={(event) => setDraft({ ...draft, region: event.target.value })} placeholder="auto" /></Field>
            <Field label="Bucket"><input value={draft.bucket} disabled={namespaceLocked} onChange={(event) => setDraft({ ...draft, bucket: event.target.value })} placeholder="pic-gallery-prod" /></Field>
            <Field label="Prefix"><input value={draft.prefix} disabled={namespaceLocked} onChange={(event) => setDraft({ ...draft, prefix: event.target.value })} placeholder="prod" /></Field>
            <Field label="本地根目录"><input value={draft.local_root} disabled={namespaceLocked} onChange={(event) => setDraft({ ...draft, local_root: event.target.value })} placeholder="/var/lib/pic-gallery/storage" /></Field>
            <Field label="公开访问地址"><input value={draft.public_base_url} onChange={(event) => setDraft({ ...draft, public_base_url: event.target.value })} placeholder="reserved" /></Field>
            <label className={storageClasses.toggle}><input type="checkbox" checked={draft.force_path_style} disabled={namespaceLocked} onChange={(event) => setDraft({ ...draft, force_path_style: event.target.checked })} /><span>使用 Path-style 请求</span></label>
          </div>
        </section>

        <section data-storage-section="access" className={storageClasses.section}>
          <div className={storageClasses.sectionHead}>
            <h3 className={cn('m-0', adminType.sectionTitle)}>访问与凭据</h3>
            <p className={storageClasses.note}>留空凭据字段会保留已有密钥；权限变更在保存后生效。</p>
          </div>
          <div className={adminPage.formGrid}>
            <Field label="Access Key ID"><input value={draft.access_key_id} onChange={(event) => setDraft({ ...draft, access_key_id: event.target.value })} placeholder={selected?.secret_status?.has_secret ? '留空保留' : 'access key id'} /></Field>
            <Field label="Secret Access Key"><input type="password" value={draft.secret_access_key} onChange={(event) => setDraft({ ...draft, secret_access_key: event.target.value })} placeholder={selected?.secret_status?.has_secret ? '留空保留' : 'secret access key'} /></Field>
            <label className={storageClasses.toggle}><input type="checkbox" checked={draft.read_enabled} onChange={(event) => setDraft({ ...draft, read_enabled: event.target.checked })} /><span>允许读取历史对象</span></label>
            <label className={storageClasses.toggle}><input type="checkbox" checked={draft.write_enabled} onChange={(event) => setDraft({ ...draft, write_enabled: event.target.checked })} /><span>允许新写入</span></label>
          </div>
        </section>
        </fieldset>

        <footer className={storageClasses.saveRail}>
          <div>
            <EditorStateBadge state={editorState} />
            <p className={cn('m-0 mt-1', storageClasses.stateCopy)}>{storageEditorStateDescription(editorState)}</p>
          </div>
          <div className={storageClasses.actions}>
            <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} disabled={editorLocked} onClick={() => void probeCurrent()}>{probing ? '验证中...' : probeActionLabel}</button>
            {draft.id && !selected?.is_default ? <button type="button" className={cn(adminButton.base, adminButton.success, adminButton.small)} disabled={saving || probing || refreshing || isDirty || activationPhase !== 'idle'} onClick={() => void setDefault()}>{storageActivationLabel(activationPhase, selected ? storageConfigNeedsProbe(selected) : false)}</button> : null}
            {draft.id && selected?.write_enabled ? <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} disabled={saving || probing || refreshing || isDirty || activationPhase !== 'idle'} onClick={() => void setReadOnly()}>设为只读</button> : null}
            <button type="submit" className={cn(adminButton.base, adminButton.primary)} disabled={saving || probing || refreshing || activationPhase !== 'idle' || !isDirty}>{saving ? '保存中...' : '保存修改'}</button>
          </div>
        </footer>
      </form>
    </section>
  )

  if (summaryMode) {
    return (
      <section className={adminPage.stack}>
        <StatusStrip columns={4}>
          <StatusCell label="默认存储" value={defaultItem?.code ?? '-'} />
          <StatusCell label="驱动" value={defaultItem ? `${defaultItem.driver}/${defaultItem.provider}` : '-'} />
          <StatusCell label="配置数" value={String(items.length)} />
          <StatusCell label="最近测试" value={defaultItem?.last_probe?.status ?? '-'} />
        </StatusStrip>
        {storageEditor}
      </section>
    )
  }

  return (
    <section className={adminPage.stack}>
      {!compact ? <PageHeader title="存储配置" detail="管理 Local/S3/R2 存储实例；草稿测试、持久化探测和默认切换使用不同安全阶段。" actions={<button type="button" className={adminButton.base} disabled={editorLocked} onClick={refreshConfigs}>{refreshing ? '刷新中...' : '刷新'}</button>} /> : null}
      {storageEditor}
    </section>
  )
}

function StorageRow({ item, dirty }: { item: StorageConfigView; dirty: boolean }) {
  const probe = storageProbeBadge(item.last_probe?.status)
  const status = storageStatusBadge(item.status)
  return (
    <>
      <span className={storageClasses.rowTitle}>
        <strong>{item.name || item.code}</strong>
        {dirty ? <Badge tone="warning">未保存</Badge> : null}
      </span>
      <span className={storageClasses.mono}>{item.code} · {item.driver}/{item.provider}</span>
      <span className={storageClasses.mono}>{item.bucket || item.local_root || item.endpoint || '-'}</span>
      <span className={storageClasses.rowBadges}>
        {item.is_default ? <Badge tone="success">默认写入</Badge> : null}
        <Badge tone={status.tone}>{status.label}</Badge>
        <Badge tone={probe.tone}>{probe.label}</Badge>
      </span>
    </>
  )
}

function EditorStateBadge({ state }: { state: StorageEditorState }) {
  const labels: Record<StorageEditorState, { label: string; tone: 'success' | 'warning' | 'danger' | 'neutral' | 'primary' }> = {
    pristine: { label: '未修改', tone: 'neutral' },
    dirty: { label: '有未保存修改', tone: 'warning' },
    validating: { label: '验证中', tone: 'primary' },
    saving: { label: '保存中', tone: 'primary' },
    saved: { label: '操作已完成', tone: 'success' },
    failed: { label: '操作失败', tone: 'danger' },
  }
  const value = labels[state]
  return <Badge tone={value.tone}>{value.label}</Badge>
}

function storageEditorState(input: { saving: boolean; probing: boolean; activationPhase: StorageActivationPhase; isDirty: boolean; feedback: StorageFeedback | null }): StorageEditorState {
  if (input.probing || input.activationPhase === 'validating') return 'validating'
  if (input.saving || input.activationPhase === 'activating') return 'saving'
  if (input.feedback?.tone === 'danger') return 'failed'
  if (input.isDirty) return 'dirty'
  if (input.feedback?.tone === 'success') return 'saved'
  return 'pristine'
}

function storageEditorStateDescription(state: StorageEditorState) {
  if (state === 'dirty') return '保存前不会影响当前持久化配置。'
  if (state === 'validating') return '正在验证连接，不会切换默认写入目标。'
  if (state === 'saving') return '正在提交配置，请保持页面打开。'
  if (state === 'saved') return '服务端已确认本次操作。'
  if (state === 'failed') return '请查看上方错误并修正后重试。'
  return '当前编辑器与已保存配置一致。'
}

function storageEditorIsDirty(draft: StorageDraft, selected: StorageConfigView | null) {
  if (draft.id) return selected ? storageDraftIsDirty(draft, selected) : true
  return JSON.stringify(payloadFromDraft(draft)) !== JSON.stringify(payloadFromDraft(newStorageDraft()))
}

function storageStatusBadge(status: string) {
  if (status === 'enabled') return { label: '已启用', tone: 'primary' as const }
  if (status === 'disabled') return { label: '已停用', tone: 'warning' as const }
  return { label: status || '未知状态', tone: 'neutral' as const }
}

function storageProbeBadge(status?: string) {
  if (status === 'success') return { label: '探测通过', tone: 'success' as const }
  if (status === 'failed') return { label: '探测失败', tone: 'danger' as const }
  if (status === 'running') return { label: '探测中', tone: 'primary' as const }
  return { label: '未探测', tone: 'neutral' as const }
}

function newStorageDraft(): StorageDraft {
  return {
    id: '', version: 0, code: '', name: '', driver: 's3', provider: 'r2', status: 'enabled', read_enabled: true, write_enabled: false,
    endpoint: '', region: 'auto', bucket: '', prefix: '', force_path_style: false, local_root: '', public_base_url: '', access_key_id: '', secret_access_key: '',
  }
}

function draftFromStorage(item: StorageConfigView): StorageDraft {
  return {
    id: item.id, version: item.version, code: item.code, name: item.name, driver: item.driver, provider: item.provider, status: item.status,
    read_enabled: item.read_enabled, write_enabled: item.write_enabled, endpoint: item.endpoint ?? '', region: item.region ?? '', bucket: item.bucket ?? '',
    prefix: item.prefix ?? '', force_path_style: Boolean(item.force_path_style), local_root: item.local_root ?? '', public_base_url: item.public_base_url ?? '',
    access_key_id: '', secret_access_key: '',
  }
}

function payloadFromDraft(draft: StorageDraft): StorageConfigWriteRequest {
  const secrets: StorageConfigWriteRequest['secrets'] = {}
  if (draft.access_key_id.trim()) secrets.access_key_id = draft.access_key_id.trim()
  if (draft.secret_access_key.trim()) secrets.secret_access_key = draft.secret_access_key.trim()
  return {
    version: draft.version || undefined,
    code: draft.code.trim(),
    name: draft.name.trim(),
    driver: draft.driver,
    provider: draft.provider,
    status: draft.status,
    read_enabled: draft.read_enabled,
    write_enabled: draft.write_enabled,
    endpoint: draft.endpoint.trim(),
    region: draft.region.trim(),
    bucket: draft.bucket.trim(),
    prefix: draft.prefix.trim(),
    force_path_style: draft.force_path_style,
    local_root: draft.local_root.trim(),
    public_base_url: draft.public_base_url.trim(),
    secrets: Object.keys(secrets).length ? secrets : undefined,
  }
}

function isVersionConflict(error: unknown) {
  return error instanceof ApiError && error.status === 409 && error.code === 'CONFLICT'
}

function storageEditableSignature(item: StorageConfigView) {
  return JSON.stringify([
    item.code, item.name, item.driver, item.provider, item.status, item.read_enabled, item.write_enabled, item.endpoint ?? '', item.region ?? '',
    item.bucket ?? '', item.prefix ?? '', item.force_path_style, item.public_base_url ?? '', item.local_root ?? '', item.secret_status?.has_secret ?? false,
    item.secret_status?.fingerprint ?? '', (item.secret_status?.secret_fields ?? []).join(','),
  ])
}

function driverTemplate(draft: StorageDraft): StorageDraft {
  if (draft.driver === 'local') return { ...draft, provider: 'local', region: '', force_path_style: false }
  return providerTemplate({ ...draft, provider: draft.provider === 'local' ? 'r2' : draft.provider })
}

function providerTemplate(draft: StorageDraft): StorageDraft {
  if (draft.provider === 'r2') return { ...draft, driver: 's3', region: draft.region || 'auto', force_path_style: false }
  if (draft.provider === 'minio') return { ...draft, driver: 's3', force_path_style: true }
  if (draft.provider === 'local') return { ...draft, driver: 'local' }
  return { ...draft, driver: draft.driver === 'local' ? 's3' : draft.driver }
}

function probeSummary(item: StorageConfigView) {
  return probeResultSummary(item.last_probe, item.code)
}

function probeResultSummary(result: { status?: string; Status?: string; message?: string; Message?: string } | undefined, fallback = '连接测试完成') {
  const status = result?.status ?? result?.Status ?? 'success'
  const message = result?.message ?? result?.Message ?? fallback
  return message ? `${status} · ${message}` : status
}
