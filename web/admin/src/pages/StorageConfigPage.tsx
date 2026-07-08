import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import type { StorageConfigView, StorageConfigWriteRequest } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, ErrorBlock, Field, LoadingBlock, PageHeader, StatusCell, StatusStrip } from '../components'
import { adminButton, adminPage } from '../ui/classes'

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

const storageClasses = {
  grid: 'grid grid-cols-[minmax(240px,340px)_minmax(0,1fr)] gap-4 max-[920px]:grid-cols-1',
  list: 'grid content-start gap-2',
  row: 'grid min-w-0 gap-2 rounded-lg border border-[var(--border)] bg-[var(--surface)] p-3 text-left transition hover:border-[var(--border-strong)] hover:bg-[var(--elevated)]',
  rowActive: 'border-[var(--accent)]/30 bg-[var(--accent)]/10',
  rowTitle: 'flex min-w-0 items-center justify-between gap-2',
  mono: 'font-mono text-xs text-[var(--soft)] [overflow-wrap:anywhere]',
  panel: 'grid gap-4 rounded-lg border border-[var(--border)] bg-[var(--surface)] p-4',
  head: 'flex flex-wrap items-center justify-between gap-3 border-b border-[var(--line)] pb-3',
  actions: 'flex flex-wrap items-center justify-end gap-2',
  toggle: 'grid grid-cols-[auto_minmax(0,1fr)] items-center gap-2 rounded-lg bg-[var(--surface-solid)] p-2 text-sm has-[:checked]:bg-[var(--accent)]/10',
  note: 'm-0 text-sm text-[var(--soft)] [overflow-wrap:anywhere]',
}

export function StorageConfigPage({ onFeedback, compact = false, summaryMode = false }: { onFeedback?: (title: string, detail?: string) => void; compact?: boolean; summaryMode?: boolean }) {
  const [items, setItems] = useState<StorageConfigView[]>([])
  const [draft, setDraft] = useState<StorageDraft>(newStorageDraft())
  const [selectedID, setSelectedID] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [probing, setProbing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const selected = useMemo(() => items.find((item) => item.id === selectedID) ?? items[0] ?? null, [items, selectedID])
  const defaultItem = items.find((item) => item.is_default)

  async function load(refreshID?: string) {
    setLoading(true)
    setError(null)
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
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '存储配置读取失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [])

  function selectItem(item: StorageConfigView) {
    setSelectedID(item.id)
    setDraft(draftFromStorage(item))
    setError(null)
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSaving(true)
    setError(null)
    try {
      const payload = payloadFromDraft(draft)
      const saved = draft.id ? await adminApi.updateStorageConfig(draft.id, payload) : await adminApi.createStorageConfig(payload)
      onFeedback?.('存储配置已保存', saved.code)
      setSelectedID(saved.id)
      setDraft(draftFromStorage(saved))
      await load(saved.id)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '存储配置保存失败')
    } finally {
      setSaving(false)
    }
  }

  async function probeCurrent() {
    setProbing(true)
    setError(null)
    try {
      if (draft.id) {
        const updated = await adminApi.probeStorageConfig(draft.id)
        onFeedback?.('连接测试完成', probeSummary(updated))
        setDraft(draftFromStorage(updated))
        await load(updated.id)
      } else {
        const result = await adminApi.probeStorageConfigDraft(payloadFromDraft(draft))
        onFeedback?.('连接测试完成', `${result.status}: ${result.message ?? ''}`)
      }
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '连接测试失败')
    } finally {
      setProbing(false)
    }
  }

  async function setDefault() {
    if (!draft.id) return
    setSaving(true)
    setError(null)
    try {
      const updated = await adminApi.setDefaultStorageConfig(draft.id, draft.version)
      onFeedback?.('默认存储已切换', updated.code)
      await load(updated.id)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '默认存储切换失败')
    } finally {
      setSaving(false)
    }
  }

  async function setReadOnly() {
    if (!draft.id) return
    setSaving(true)
    setError(null)
    try {
      const updated = await adminApi.setStorageConfigStatus(draft.id, { version: draft.version, status: 'enabled', read_enabled: true, write_enabled: false })
      onFeedback?.('存储已设为只读', updated.code)
      await load(updated.id)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '状态更新失败')
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <LoadingBlock label="读取存储配置" />
  if (error && items.length === 0) return <ErrorBlock message={error} onRetry={load} />

  const storageEditor = (
    <section className={storageClasses.grid}>
      <aside className={storageClasses.list}>
        <button type="button" className={cn(storageClasses.row, !draft.id && storageClasses.rowActive)} onClick={() => { setDraft(newStorageDraft()); setSelectedID('') }}>
          <strong>新增存储配置</strong>
          <span className={storageClasses.mono}>S3 / R2 / Local</span>
        </button>
        {items.map((item) => (
          <button key={item.id} type="button" className={cn(storageClasses.row, selected?.id === item.id && draft.id && storageClasses.rowActive)} onClick={() => selectItem(item)}>
            <StorageRow item={item} />
          </button>
        ))}
      </aside>
      <form className={storageClasses.panel} onSubmit={(event) => void save(event)}>
        <div className={storageClasses.head}>
          <div>
            <strong>{draft.id ? '编辑存储实例' : '新增存储实例'}</strong>
            <p className={storageClasses.note}>Access Key 和 Secret 只写不读；R2 使用 S3 协议接入，region 默认 auto；custom_s3 适配任意 S3 兼容端点。</p>
          </div>
          <div className={storageClasses.actions}>
            {draft.id ? <Badge tone={selected?.is_default ? 'success' : 'neutral'}>{selected?.is_default ? 'Default' : `v${draft.version}`}</Badge> : <Badge tone="primary">New</Badge>}
            <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} disabled={probing} onClick={() => void probeCurrent()}>{probing ? '测试中...' : '测试连接'}</button>
            {draft.id && !selected?.is_default ? <button type="button" className={cn(adminButton.base, adminButton.success, adminButton.small)} disabled={saving} onClick={() => void setDefault()}>设为默认</button> : null}
            {draft.id && selected?.write_enabled ? <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} disabled={saving} onClick={() => void setReadOnly()}>设为只读</button> : null}
            <button type="submit" className={cn(adminButton.base, adminButton.primary)} disabled={saving}>{saving ? '保存中...' : '保存'}</button>
          </div>
        </div>
        <div className={adminPage.formGrid}>
          <Field label="Code"><input value={draft.code} disabled={Boolean(draft.id)} onChange={(event) => setDraft({ ...draft, code: event.target.value })} placeholder="r2-prod" /></Field>
          <Field label="Name"><input value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} placeholder="Cloudflare R2 Prod" /></Field>
          <Field label="Driver">
            <select value={draft.driver} onChange={(event) => setDraft(driverTemplate({ ...draft, driver: event.target.value }))}>
              <option value="local">local</option>
              <option value="s3">s3</option>
            </select>
          </Field>
          <Field label="Provider">
            <select value={draft.provider} onChange={(event) => setDraft(providerTemplate({ ...draft, provider: event.target.value }))}>
              <option value="local">local</option>
              <option value="r2">r2</option>
              <option value="aws_s3">aws_s3</option>
              <option value="minio">minio</option>
              <option value="custom_s3">custom_s3</option>
            </select>
          </Field>
          <Field label="Endpoint"><input value={draft.endpoint} onChange={(event) => setDraft({ ...draft, endpoint: event.target.value })} placeholder="https://account.r2.cloudflarestorage.com" /></Field>
          <Field label="Region"><input value={draft.region} onChange={(event) => setDraft({ ...draft, region: event.target.value })} placeholder="auto" /></Field>
          <Field label="Bucket"><input value={draft.bucket} onChange={(event) => setDraft({ ...draft, bucket: event.target.value })} placeholder="pic-gallery-prod" /></Field>
          <Field label="Prefix"><input value={draft.prefix} onChange={(event) => setDraft({ ...draft, prefix: event.target.value })} placeholder="prod" /></Field>
          <Field label="Local Root"><input value={draft.local_root} onChange={(event) => setDraft({ ...draft, local_root: event.target.value })} placeholder="/var/lib/pic-gallery/storage" /></Field>
          <Field label="Public Base URL"><input value={draft.public_base_url} onChange={(event) => setDraft({ ...draft, public_base_url: event.target.value })} placeholder="reserved" /></Field>
          <Field label="Access Key ID"><input value={draft.access_key_id} onChange={(event) => setDraft({ ...draft, access_key_id: event.target.value })} placeholder={selected?.secret_status?.has_secret ? '留空保留' : 'access key id'} /></Field>
          <Field label="Secret Access Key"><input type="password" value={draft.secret_access_key} onChange={(event) => setDraft({ ...draft, secret_access_key: event.target.value })} placeholder={selected?.secret_status?.has_secret ? '留空保留' : 'secret access key'} /></Field>
          <label className={storageClasses.toggle}><input type="checkbox" checked={draft.read_enabled} onChange={(event) => setDraft({ ...draft, read_enabled: event.target.checked })} /><span>允许读取历史对象</span></label>
          <label className={storageClasses.toggle}><input type="checkbox" checked={draft.write_enabled} onChange={(event) => setDraft({ ...draft, write_enabled: event.target.checked })} /><span>允许新写入</span></label>
          <label className={storageClasses.toggle}><input type="checkbox" checked={draft.force_path_style} onChange={(event) => setDraft({ ...draft, force_path_style: event.target.checked })} /><span>Force path-style</span></label>
        </div>
      </form>
    </section>
  )

  if (summaryMode) {
    return (
      <section className={adminPage.stack}>
        {error ? <ErrorBlock message={error} onRetry={() => setError(null)} /> : null}
        <StatusStrip columns={4}>
          <StatusCell label="默认存储" value={defaultItem?.code ?? '-'} />
          <StatusCell label="驱动" value={defaultItem ? `${defaultItem.driver}/${defaultItem.provider}` : '-'} />
          <StatusCell label="配置数" value={String(items.length)} />
          <StatusCell label="最近测试" value={defaultItem?.last_probe?.status ?? '-'} />
        </StatusStrip>
        <section className={storageClasses.list}>
          {(items.length ? items : []).map((item) => (
            <button key={item.id} type="button" className={storageClasses.row} onClick={() => selectItem(item)}>
              <StorageRow item={item} />
            </button>
          ))}
          {items.length === 0 ? <p className={storageClasses.note}>暂无存储配置，服务端会从启动配置自动创建 bootstrap 配置。</p> : null}
        </section>
        <details className="group rounded-lg border border-[var(--border)] bg-[var(--surface)] p-4">
          <summary className="cursor-pointer list-none text-sm font-bold text-[var(--accent)]">编辑存储配置</summary>
          <div className="mt-4">{storageEditor}</div>
        </details>
      </section>
    )
  }

  return (
    <section className={adminPage.stack}>
      {!compact ? <PageHeader title="存储配置" detail="管理 Local/S3/R2 存储实例；新生成图片写入默认实例，历史图片按自身实例读取。" actions={<button type="button" className={adminButton.base} onClick={() => void load()}>刷新</button>} /> : null}
      {error ? <ErrorBlock message={error} onRetry={() => setError(null)} /> : null}
      {storageEditor}
    </section>
  )
}

function StorageRow({ item }: { item: StorageConfigView }) {
  return (
    <>
      <span className={storageClasses.rowTitle}>
        <strong>{item.name || item.code}</strong>
        <Badge tone={item.is_default ? 'success' : item.status === 'enabled' ? 'primary' : 'neutral'}>{item.is_default ? 'Default' : item.status}</Badge>
      </span>
      <span className={storageClasses.mono}>{item.code} · {item.driver}/{item.provider}</span>
      <span className={storageClasses.mono}>{item.bucket || item.local_root || item.endpoint || '-'}</span>
    </>
  )
}

function newStorageDraft(): StorageDraft {
  return {
    id: '',
    version: 0,
    code: '',
    name: '',
    driver: 's3',
    provider: 'r2',
    status: 'enabled',
    read_enabled: true,
    write_enabled: false,
    endpoint: '',
    region: 'auto',
    bucket: '',
    prefix: '',
    force_path_style: false,
    local_root: '',
    public_base_url: '',
    access_key_id: '',
    secret_access_key: '',
  }
}

function draftFromStorage(item: StorageConfigView): StorageDraft {
  return {
    id: item.id,
    version: item.version,
    code: item.code,
    name: item.name,
    driver: item.driver,
    provider: item.provider,
    status: item.status,
    read_enabled: item.read_enabled,
    write_enabled: item.write_enabled,
    endpoint: item.endpoint ?? '',
    region: item.region ?? '',
    bucket: item.bucket ?? '',
    prefix: item.prefix ?? '',
    force_path_style: Boolean(item.force_path_style),
    local_root: item.local_root ?? '',
    public_base_url: item.public_base_url ?? '',
    access_key_id: '',
    secret_access_key: '',
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
  return `${item.last_probe?.status ?? 'never'} · ${item.last_probe?.message ?? item.code}`
}
