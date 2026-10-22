import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import type { StorageConfigView, StorageConfigWriteRequest } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { ApiError } from '../../../shared/http-client'
import { Badge, Button, Drawer, ErrorBlock, InlineFeedback, LoadingBlock, PageHeader, StatusCell, StatusStrip } from '../components'
import { adminPage } from '../ui/classes'
import { FormSection, LedgerField, SwitchField } from './AdminMediaTabs'
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
  const [draft, setDraft] = useState<StorageDraft | null>(null)
  const [initialLoading, setInitialLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [probing, setProbing] = useState(false)
  const [activationID, setActivationID] = useState('')
  const [activationPhase, setActivationPhase] = useState<StorageActivationPhase>('idle')
  const [loadError, setLoadError] = useState<string | null>(null)
  const [feedback, setFeedback] = useState<StorageFeedback | null>(null)

  const defaultItem = items.find((item) => item.is_default)
  const selected = draft?.id ? items.find((item) => item.id === draft.id) ?? null : null
  const isDirty = draft ? (selected ? storageDraftIsDirty(draft, selected) : draftHasContent(draft)) : false
  const editorState = storageEditorState({ saving, probing, activationPhase, isDirty, feedback })
  const editorLocked = saving || probing || refreshing || Boolean(activationID)
  const namespaceLocked = Boolean(draft?.id)
  const validation = draft ? validateStorageDraft(draft) : {}
  const dirtyCount = draft ? storageDirtyCount(draft, selected) : 0
  const probeActionLabel = !draft?.id || isDirty ? '测试草稿连接' : '探测已保存配置'

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
      if (draft?.id) {
        const latest = next.find((item) => item.id === draft.id)
        if (latest && !storageDraftIsDirty(draft, latest)) setDraft(draftFromStorage(latest))
      }
      void refreshID
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
    onBusyChange?.(saving || probing || Boolean(activationID))
  }, [activationID, onBusyChange, probing, saving])

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

  function openCreate() {
    if (editorLocked) return
    if (!confirmDiscardChanges()) return
    setFeedback(null)
    setDraft(newStorageDraft())
  }

  function openEdit(item: StorageConfigView) {
    if (editorLocked) return
    if (draft?.id === item.id) return
    if (!confirmDiscardChanges()) return
    setFeedback(null)
    setDraft(draftFromStorage(item))
  }

  function closeEditor() {
    if (saving || probing) return
    if (!confirmDiscardChanges()) return
    setDraft(null)
    setFeedback(null)
  }

  async function save(event?: FormEvent<HTMLFormElement>) {
    event?.preventDefault()
    if (!draft) return
    if (Object.keys(validation).length) {
      setFeedback({ tone: 'danger', message: Object.values(validation)[0] })
      return
    }
    setSaving(true)
    setFeedback(null)
    const payload = payloadFromDraft(draft)
    try {
      const saved = draft.id ? await adminApi.updateStorageConfig(draft.id, payload) : await adminApi.createStorageConfig(payload)
      onFeedback?.('存储配置已保存', saved.code)
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
      setDraft(null)
      setFeedback({ tone: 'danger', message: '存储配置已被删除或不可用，请刷新后再操作。' })
      return true
    }
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
    if (!draft) return
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

  async function probeItem(item: StorageConfigView) {
    if (editorLocked) return
    setProbing(true)
    setFeedback(null)
    try {
      const updated = await adminApi.probeStorageConfig(item.id)
      await load(updated.id)
      setFeedback({ tone: 'success', message: `${updated.code}：${probeSummary(updated)}` })
      onFeedback?.('存储探测完成', `${updated.code} · ${probeSummary(updated)}`)
    } catch (caught) {
      setFeedback({ tone: 'danger', message: caught instanceof Error ? caught.message : '连接探测失败' })
    } finally {
      setProbing(false)
    }
  }

  async function setDefault(item: StorageConfigView) {
    if (editorLocked || item.is_default) return
    setActivationID(item.id)
    setFeedback(null)
    setActivationPhase('validating')
    try {
      const updated = await activateSavedStorageConfig({
        draft: draftFromStorage(item),
        saved: item,
        probe: adminApi.probeStorageConfig,
        setDefault: adminApi.setDefaultStorageConfig,
        onPhase: setActivationPhase,
      })
      onFeedback?.('默认存储已切换', updated.code)
      await load(updated.id)
      setFeedback({ tone: 'success', message: `默认写入目标已切换为 ${updated.code}。切换默认实例不会迁移历史对象，旧对象仍按原实例读取。` })
    } catch (caught) {
      const message = caught instanceof Error ? caught.message : '默认存储切换失败'
      const refreshed = await load(item.id)
      const latest = refreshed?.find((entry) => entry.id === item.id)
      const probeMessage = latest?.last_probe?.status === 'failed' ? latest.last_probe.message : ''
      setFeedback({ tone: 'danger', message: probeMessage || message })
    } finally {
      setActivationPhase('idle')
      setActivationID('')
    }
  }

  async function setReadOnly() {
    if (!draft?.id || !selected) return
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
      setDraft(draftFromStorage(updated))
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
    void load()
  }

  if (initialLoading) return <LoadingBlock label="读取存储配置" />
  if (loadError && items.length === 0) return <ErrorBlock message={loadError} onRetry={() => void load(undefined, true)} />

  return (
    <section className={adminPage.stack}>
      {!compact && !summaryMode ? <PageHeader title="存储配置" detail="管理 Local/S3/R2 存储实例；编辑在抽屉完成，探测与默认切换直接放在列表操作列。" actions={<Button variant="secondary" disabled={editorLocked} onClick={refreshConfigs}>{refreshing ? '刷新中...' : '刷新'}</Button>} /> : null}
      {summaryMode ? (
        <StatusStrip columns={4}>
          <StatusCell label="默认存储" value={defaultItem?.code ?? '-'} />
          <StatusCell label="驱动" value={defaultItem ? `${defaultItem.driver}/${defaultItem.provider}` : '-'} />
          <StatusCell label="配置数" value={String(items.length)} />
          <StatusCell label="最近测试" value={defaultItem?.last_probe?.status ?? '-'} />
        </StatusStrip>
      ) : null}

      {loadError ? <InlineFeedback tone="danger" message={loadError} /> : null}
      {feedback ? <InlineFeedback tone={feedback.tone} message={feedback.message} /> : null}

      <section className="a-panel a-panel--pad" data-admin-storage-editor>
        <div className="a-section-title" style={{ marginBottom: 12 }}>
          <div>
            <h2>存储实例</h2>
            <p>编辑在抽屉完成；探测连接与设置默认直接放在操作列。</p>
          </div>
          <Button variant="secondary" size="sm" disabled={editorLocked} onClick={openCreate}>新增实例</Button>
        </div>
        <div className="ss-store">
          {items.map((item) => {
            const probe = storageProbeBadge(item.last_probe?.status)
            const status = storageStatusBadge(item.status)
            return (
              <div key={item.id} className={cn('ss-store__row', item.is_default && 'is-active')}>
                <span className="a-cell-entity__avatar" style={{ borderRadius: 7 }}>{item.driver === 'local' ? 'LOC' : 'S3'}</span>
                <div>
                  <strong>{item.name || item.code}</strong>
                  <span>{item.code} · {item.driver}/{item.provider} · {item.bucket || item.local_root || item.endpoint || '-'}</span>
                </div>
                {item.is_default ? <span className="a-badge a-badge--accent"><i />默认写入</span> : null}
                <Badge tone={status.tone}>{status.label}</Badge>
                <Badge tone={probe.tone}>{probe.label}</Badge>
                <div className="ss-store__actions">
                  <button className="a-row-btn a-row-btn--primary" type="button" disabled={editorLocked && draft?.id !== item.id} onClick={() => openEdit(item)}>编辑</button>
                  <button className="a-row-btn" type="button" disabled={probing || Boolean(activationID)} onClick={() => void probeItem(item)}>探测连接</button>
                  <button className="a-row-btn" type="button" disabled={item.is_default || probing || Boolean(activationID)} onClick={() => void setDefault(item)}>{activationID === item.id ? storageActivationLabel(activationPhase, storageConfigNeedsProbe(item)) : item.is_default ? '已是默认' : '设置默认'}</button>
                </div>
              </div>
            )
          })}
          {!items.length ? <div className="cfg-empty-inline">暂无存储实例。新增 S3 / R2 / Local 配置并先探测连接，再切换默认写入。</div> : null}
        </div>
        <div className="a-banner a-banner--info" style={{ marginTop: 12 }}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M12 11v5M12 7.5h.01" /></svg>
          <div><strong style={{ fontSize: 13 }}>切换默认实例不会迁移历史对象</strong><div className="a-hint">旧对象仍按原实例读取；新上传才会写入新的默认实例。</div></div>
        </div>
      </section>

      {draft ? (
        <Drawer
          title={draft.id ? '编辑存储实例' : '新增存储实例'}
          description={draft.id ? `${draft.name || draft.code} · ${draft.driver}/${draft.provider} · 凭据只写不读` : '填写 Endpoint、Bucket 与凭据后先探测连接'}
          headerAside={<Badge tone={editorState === 'failed' ? 'danger' : editorState === 'dirty' ? 'warning' : editorState === 'saved' ? 'success' : 'neutral'}>{editorStateLabel(editorState)}</Badge>}
          onClose={closeEditor}
          footer={<>
            <span className={cn('a-badge', Object.keys(validation).length ? 'a-badge--bad' : isDirty ? 'a-badge--warn' : 'a-badge--ok')}><i />{Object.keys(validation).length ? `${Object.keys(validation).length} 项校验未通过` : isDirty ? `${dirtyCount} 项未保存` : '未修改'}</span>
            <div style={{ flex: 1 }} />
            <Button variant="secondary" size="sm" disabled={editorLocked} onClick={() => void probeCurrent()}>{probing ? '验证中...' : probeActionLabel}</Button>
            {draft.id && selected?.write_enabled ? <Button variant="ghost" size="sm" disabled={saving || probing || isDirty} onClick={() => void setReadOnly()}>设为只读</Button> : null}
            <Button variant="ghost" size="sm" disabled={saving || probing} onClick={closeEditor}>取消</Button>
            <Button variant="primary" size="sm" disabled={saving || probing || !isDirty || Object.keys(validation).length > 0} onClick={() => void save()}>{saving ? '保存中...' : '保存修改'}</Button>
          </>}
        >
          {feedback ? <InlineFeedback tone={feedback.tone} message={feedback.message} /> : null}
          <form className="a-form" aria-busy={editorLocked} onSubmit={(event) => void save(event)}>
            <FormSection index={1} title="基本信息" description="标识实例类型、适配协议和启停状态。">
              <LedgerField label="配置名称" required error={validation.name}><input className="a-input" value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} placeholder="Cloudflare R2 Prod" /></LedgerField>
              <LedgerField label="配置代码" required hint={draft.id ? '配置代码创建后不可修改。' : '用于稳定标识该实例。'} error={validation.code}><input className="a-input" value={draft.code} disabled={namespaceLocked} onChange={(event) => setDraft({ ...draft, code: event.target.value })} placeholder="r2-prod" style={{ fontFamily: 'var(--font-admin-mono)' }} /></LedgerField>
              <div className="a-form__pair">
                <LedgerField label="存储驱动"><select className="a-select" value={draft.driver} disabled={namespaceLocked} onChange={(event) => setDraft(driverTemplate({ ...draft, driver: event.target.value }))}><option value="local">Local</option><option value="s3">S3</option></select></LedgerField>
                <LedgerField label="服务提供方"><select className="a-select" value={draft.provider} disabled={namespaceLocked} onChange={(event) => setDraft(providerTemplate({ ...draft, provider: event.target.value }))}><option value="local">Local</option><option value="r2">Cloudflare R2</option><option value="aws_s3">AWS S3</option><option value="rustfs">RustFS</option><option value="minio">MinIO（兼容旧配置）</option><option value="custom_s3">自定义 S3</option></select></LedgerField>
              </div>
              <div className="a-form__pair">
                <LedgerField label="启用状态"><select className="a-select" value={draft.status} onChange={(event) => setDraft({ ...draft, status: event.target.value })}><option value="enabled">启用</option><option value="disabled">停用</option></select></LedgerField>
                <LedgerField label="默认写入"><span className={cn('a-badge', selected?.is_default ? 'a-badge--ok' : 'a-badge--neutral')}><i />{selected?.is_default ? '当前默认实例' : '通过列表操作切换'}</span></LedgerField>
              </div>
            </FormSection>

            <FormSection index={2} title="对象定位" description={namespaceLocked ? '对象地址创建后不可修改；迁移时请新建配置、验证后切换默认写入。' : '定义远端端点、桶、对象前缀或本地根目录。'}>
              <LedgerField label="Endpoint" error={validation.endpoint}><input className="a-input" value={draft.endpoint} disabled={namespaceLocked} onChange={(event) => setDraft({ ...draft, endpoint: event.target.value })} placeholder="https://account.r2.cloudflarestorage.com" style={{ fontFamily: 'var(--font-admin-mono)', fontSize: 12.5 }} /></LedgerField>
              <div className="a-form__pair">
                <LedgerField label="Region"><input className="a-input" value={draft.region} disabled={namespaceLocked} onChange={(event) => setDraft({ ...draft, region: event.target.value })} placeholder="auto" /></LedgerField>
                <LedgerField label="Bucket" error={validation.bucket}><input className="a-input" value={draft.bucket} disabled={namespaceLocked} onChange={(event) => setDraft({ ...draft, bucket: event.target.value })} placeholder="pic-gallery-prod" /></LedgerField>
              </div>
              <div className="a-form__pair">
                <LedgerField label="Prefix"><input className="a-input" value={draft.prefix} disabled={namespaceLocked} onChange={(event) => setDraft({ ...draft, prefix: event.target.value })} placeholder="prod" /></LedgerField>
                <LedgerField label="公开访问地址"><input className="a-input" value={draft.public_base_url} onChange={(event) => setDraft({ ...draft, public_base_url: event.target.value })} placeholder="reserved" /></LedgerField>
              </div>
              <LedgerField label="本地根目录" error={validation.local_root}><input className="a-input" value={draft.local_root} disabled={namespaceLocked} onChange={(event) => setDraft({ ...draft, local_root: event.target.value })} placeholder="/var/lib/pic-gallery/storage" /></LedgerField>
              <SwitchField label="使用 Path-style 请求" checked={draft.force_path_style} disabled={namespaceLocked} onChange={(force_path_style) => setDraft({ ...draft, force_path_style })} />
            </FormSection>

            <FormSection index={3} title="访问与凭据" description="留空凭据字段会保留已有密钥；权限变更在保存后生效。">
              <LedgerField label="Access Key ID"><input className="a-input" value={draft.access_key_id} onChange={(event) => setDraft({ ...draft, access_key_id: event.target.value })} placeholder={selected?.secret_status?.has_secret ? '留空保留' : 'access key id'} /></LedgerField>
              <LedgerField label="Secret Access Key"><input className="a-input" type="password" value={draft.secret_access_key} onChange={(event) => setDraft({ ...draft, secret_access_key: event.target.value })} placeholder={selected?.secret_status?.has_secret ? '留空保留' : 'secret access key'} /></LedgerField>
              <div className="a-form__pair">
                <SwitchField label="允许读取历史对象" checked={draft.read_enabled} onChange={(read_enabled) => setDraft({ ...draft, read_enabled })} />
                <SwitchField label="允许新写入" checked={draft.write_enabled} onChange={(write_enabled) => setDraft({ ...draft, write_enabled })} />
              </div>
              {draft.driver === 's3' ? <div className="cfg-secret-note"><strong style={{ display: 'block', color: 'var(--a-ink)' }}>浏览器直传 CORS</strong>Bucket 需允许用户端 Origin 使用 PUT，并暴露响应头 ETag。规则缺失时上传会自动切换兼容上传，经同源 API 流式转发。</div> : null}
            </FormSection>
            <div className="a-banner a-banner--info">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M12 11v5M12 7.5h.01" /></svg>
              <div><strong style={{ fontSize: 13 }}>保存前先探测连接</strong><div className="a-hint">探测会验证写入与读取路径；失败时不会保存新的默认实例。设置默认不在编辑表单中，需回到列表操作列。</div></div>
            </div>
          </form>
        </Drawer>
      ) : null}
    </section>
  )
}

function storageEditorState(input: { saving: boolean; probing: boolean; activationPhase: StorageActivationPhase; isDirty: boolean; feedback: StorageFeedback | null }): StorageEditorState {
  if (input.probing || input.activationPhase === 'validating') return 'validating'
  if (input.saving || input.activationPhase === 'activating') return 'saving'
  if (input.feedback?.tone === 'danger') return 'failed'
  if (input.isDirty) return 'dirty'
  if (input.feedback?.tone === 'success') return 'saved'
  return 'pristine'
}

function editorStateLabel(state: StorageEditorState) {
  if (state === 'dirty') return '有未保存修改'
  if (state === 'validating') return '验证中'
  if (state === 'saving') return '保存中'
  if (state === 'saved') return '操作已完成'
  if (state === 'failed') return '操作失败'
  return '未修改'
}

function storageStatusBadge(status: string): { label: string; tone: 'success' | 'warning' | 'danger' | 'neutral' | 'primary' } {
  if (status === 'enabled') return { label: '已启用', tone: 'primary' }
  if (status === 'disabled') return { label: '已停用', tone: 'warning' }
  return { label: status || '未知状态', tone: 'neutral' }
}

function storageProbeBadge(status?: string): { label: string; tone: 'success' | 'warning' | 'danger' | 'neutral' | 'primary' } {
  if (status === 'success') return { label: '探测通过', tone: 'success' }
  if (status === 'failed') return { label: '探测失败', tone: 'danger' }
  if (status === 'running') return { label: '探测中', tone: 'primary' }
  return { label: '未探测', tone: 'neutral' }
}

function validateStorageDraft(draft: StorageDraft): Record<string, string> {
  const errors: Record<string, string> = {}
  if (!draft.name.trim()) errors.name = '请填写配置名称。'
  if (!draft.code.trim()) errors.code = '请填写配置代码。'
  if (draft.driver === 's3') {
    if (!draft.endpoint.trim()) errors.endpoint = 'S3 驱动必须填写 Endpoint。'
    if (!draft.bucket.trim()) errors.bucket = 'S3 驱动必须填写 Bucket。'
  }
  if (draft.driver === 'local' && !draft.local_root.trim()) errors.local_root = 'Local 驱动必须填写本地根目录。'
  return errors
}

function storageDirtyCount(draft: StorageDraft, selected: StorageConfigView | null): number {
  if (!selected) return draftHasContent(draft) ? 1 : 0
  const baseline = draftFromStorage(selected)
  const keys: Array<keyof StorageDraft> = ['code', 'name', 'driver', 'provider', 'status', 'read_enabled', 'write_enabled', 'endpoint', 'region', 'bucket', 'prefix', 'force_path_style', 'local_root', 'public_base_url']
  let count = keys.filter((key) => String(draft[key]) !== String(baseline[key])).length
  if (draft.access_key_id.trim()) count += 1
  if (draft.secret_access_key.trim()) count += 1
  return count
}

function draftHasContent(draft: StorageDraft): boolean {
  return Boolean(draft.name || draft.code || draft.endpoint || draft.bucket || draft.local_root || draft.access_key_id || draft.secret_access_key)
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
  if (draft.provider === 'minio' || draft.provider === 'rustfs') return { ...draft, driver: 's3', force_path_style: true }
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