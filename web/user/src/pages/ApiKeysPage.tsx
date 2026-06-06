import { FormEvent, useMemo, useState } from 'react'
import type { ApiKey } from '../../../shared/api-types'
import { userApi } from '../../../shared/user-api'
import { Button, CopyButton, EmptyState, Field, LoadingState, Modal, useApp } from '../components'
import { errorMessage, useApiResource } from '../useApiResource'
import {
  apiKeyCreatePayload,
  apiKeyDeleteConfirmText,
  apiKeyEditForm,
  apiKeyGroupReadOnlyHint,
  apiKeyPageLabels,
  apiKeyQuickstart,
  apiKeyRow,
  apiKeyScopeLabel,
  apiKeyStatusToggleLabel,
  apiKeyTableHeaders,
  apiKeyUpdatePayload,
} from './apiKeyRows'

const allScopes = ['images:write', 'images:read', 'balance:read', 'profile:read']

const pageStyle = {
  content: { padding: 40, maxWidth: 1200, marginInline: 'auto', width: '100%' } as const,
  header: { marginBottom: 48, display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', gap: 20, flexWrap: 'wrap' as const },
  title: { fontSize: 48, margin: 0 },
  card: { background: 'var(--vault-panel)', borderRadius: 12, border: '1px solid var(--vault-line)', padding: 32, marginBottom: 32 } as const,
}

export function ApiKeysPage() {
  const app = useApp()
  const keys = useApiResource(() => userApi.listApiKeys(), [])
  const [creating, setCreating] = useState(false)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [revealed, setRevealed] = useState<Record<string, boolean>>({})
  const [secretPreviews, setSecretPreviews] = useState<Record<string, string>>({})
  const [editTarget, setEditTarget] = useState<ApiKey | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ApiKey | null>(null)
  const [busyKeyId, setBusyKeyId] = useState<string | null>(null)
  const selectedKey = useMemo(() => keys.data?.find((key) => key.id === selectedId) ?? keys.data?.[0] ?? null, [keys.data, selectedId])
  const quickstartKey = selectedKey ? { ...selectedKey, secret_preview: secretPreviews[selectedKey.id] ?? selectedKey.secret_preview } : null
  const quickstart = apiKeyQuickstart(quickstartKey)

  async function toggle(key: ApiKey) {
    setBusyKeyId(key.id)
    try {
      await userApi.updateApiKey(key.id, { status: key.status === 'active' ? 'disabled' : 'active' })
      app.notify('success', key.status === 'active' ? '密钥已禁用' : '密钥已启用')
      await keys.reload()
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusyKeyId(null)
    }
  }

  async function resetSecret(key: ApiKey) {
    setBusyKeyId(key.id)
    try {
      const updated = await userApi.resetApiKeySecret(key.id)
      if (updated.secret_preview) setSecretPreviews((rows) => ({ ...rows, [updated.id]: updated.secret_preview ?? '' }))
      app.notify('success', 'Secret 已重置，仅展示一次')
      await keys.reload()
      setSelectedId(updated.id)
      setRevealed((rows) => ({ ...rows, [updated.id]: true }))
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusyKeyId(null)
    }
  }

  async function remove(key: ApiKey) {
    setBusyKeyId(key.id)
    try {
      await userApi.deleteApiKey(key.id)
      app.notify('success', '密钥已删除')
      if (selectedId === key.id) setSelectedId(null)
      setDeleteTarget(null)
      await keys.reload()
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusyKeyId(null)
    }
  }

  return (
    <div className="content" style={pageStyle.content}>
      <div className="header" style={pageStyle.header}>
        <div>
          <p className="eyebrow">{apiKeyPageLabels.eyebrow}</p>
          <h1 style={pageStyle.title}>API 密钥</h1>
        </div>
        <button className="btn btn-primary" type="button" onClick={() => setCreating(true)}>+ {apiKeyPageLabels.create}</button>
      </div>

      <div className="card" style={pageStyle.card}>
        {keys.loading ? <LoadingState /> : null}
        {!keys.loading && !keys.data?.length ? <EmptyState title="暂无密钥" detail="创建一个密钥后即可调试开放 API。" action={<Button onClick={() => setCreating(true)}>创建密钥</Button>} /> : null}
        {keys.data?.length ? (
          <div className="table-container" style={{ overflowX: 'auto' }}>
            <table className="ds-table" style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14, minWidth: 1160 }}>
              <thead>
                <tr>
                  {apiKeyTableHeaders.map((head) => <th key={head} style={thStyle}>{head}</th>)}
                </tr>
              </thead>
              <tbody>
                {keys.data.map((key) => {
                  const row = apiKeyRow(key)
                  const badge = row.statusBadge
                  const secretPreview = secretPreviews[key.id] ?? key.secret_preview
                  return (
                    <tr key={key.id} onClick={() => setSelectedId(key.id)} style={{ cursor: 'pointer', outline: selectedKey?.id === key.id ? '1px solid rgba(212,157,94,.45)' : undefined }}>
                      <td style={tdStyle}><strong>{key.name}</strong><div style={{ color: 'var(--vault-muted)', fontSize: 12 }}>{row.scopesText}</div></td>
                      <td style={tdStyle}><code className="num" style={inlineCode}>{revealed[key.id] ? key.access_key : row.accessKeyMasked}</code></td>
                      <td style={tdStyle}><code className="num" style={inlineCode}>{revealed[key.id] ? (secretPreview ?? '仅创建或重置时展示') : 'sk_••••••••••••'}</code></td>
                      <td style={tdStyle}><span className={`pill ${badge.tone === 'success' ? 'active' : ''}`} style={{ ...pillStyle, ...(badge.tone === 'success' ? { color: 'var(--vault-gold)', borderColor: 'rgba(212,157,94,.35)' } : {}) }}>{badge.label}</span></td>
                      <td className="num" style={tdStyle}>{key.rpm_limit}</td>
                      <td className="num" style={tdStyle}>
                        <div>{row.totalQuotaLabel}</div>
                        <div style={{ color: 'var(--vault-muted)', fontSize: 12, marginTop: 4 }}>{row.dailyQuotaLabel}</div>
                      </td>
                      <td className="num" style={tdStyle}>{row.createdAtLabel}</td>
                      <td className="num" style={tdStyle}>{row.lastUsedAtLabel}</td>
                      <td className="num" style={tdStyle}>
                        <span>{row.expiresAtLabel}</span>
                        {row.expiryHint ? <div style={{ color: 'var(--vault-muted)', fontSize: 12, marginTop: 4 }}>{row.expiryHint}</div> : null}
                      </td>
                      <td style={{ ...tdStyle, textAlign: 'right' }}>
                        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, flexWrap: 'wrap' }}>
                          <CopyButton text={key.access_key} label="复制 AK" />
                          {secretPreview ? <CopyButton text={secretPreview} label="复制 SK" /> : null}
                          <button className="btn btn-ghost" type="button" disabled={busyKeyId === key.id} onClick={(event) => { event.stopPropagation(); setRevealed((rows) => ({ ...rows, [key.id]: !rows[key.id] })) }}>{revealed[key.id] ? apiKeyPageLabels.hide : apiKeyPageLabels.show}</button>
                          <button className="btn btn-ghost" type="button" disabled={busyKeyId === key.id} onClick={(event) => { event.stopPropagation(); setEditTarget(key) }}>{apiKeyPageLabels.edit}</button>
                          <button className="btn btn-ghost" type="button" disabled={busyKeyId === key.id} onClick={(event) => { event.stopPropagation(); void toggle(key) }}>{apiKeyStatusToggleLabel(key.status)}</button>
                          <button className="btn btn-ghost" type="button" disabled={busyKeyId === key.id} onClick={(event) => { event.stopPropagation(); void resetSecret(key) }}>{apiKeyPageLabels.resetSecret}</button>
                          <button className="btn btn-ghost" type="button" disabled={busyKeyId === key.id} style={{ color: 'oklch(70% 0.2 25)' }} onClick={(event) => { event.stopPropagation(); setDeleteTarget(key) }}>{apiKeyPageLabels.delete}</button>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        ) : null}
      </div>

      <div className="stack" style={{ display: 'grid', gap: 24 }}>
        <h2 className="h2" style={{ fontSize: 32, margin: 0 }}>{apiKeyPageLabels.quickstartTitle}</h2>
        <div className="card" style={{ ...pageStyle.card, padding: 0, overflow: 'hidden' }}>
          <div style={{ background: 'var(--vault-bg)', padding: '12px 20px', borderBottom: '1px solid var(--vault-line)', fontSize: 13, fontFamily: 'JetBrains Mono, monospace', color: 'var(--vault-muted)', display: 'flex', justifyContent: 'space-between', gap: 12 }}>
            <span>{quickstart.title}</span>
            <CopyButton text={quickstart.code} label={quickstart.copyLabel} />
          </div>
          <div className="code-block" style={{ background: 'var(--vault-bg)', padding: 16, fontFamily: 'JetBrains Mono, monospace', fontSize: 13, color: 'var(--vault-muted)' }}>
            <pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{quickstart.code}</pre>
          </div>
        </div>
        <p style={{ fontSize: 14, color: 'var(--vault-muted)' }}>查看完整 <a href="#/docs" style={{ color: 'var(--vault-gold)', textDecoration: 'underline' }}>开发文档</a> 获取更多语言示例。</p>
      </div>

      {creating ? <CreateKeyModal onClose={() => setCreating(false)} onCreated={async (key) => { if (key.secret_preview) setSecretPreviews((rows) => ({ ...rows, [key.id]: key.secret_preview ?? '' })); await keys.reload(); setSelectedId(key.id); setRevealed((rows) => ({ ...rows, [key.id]: true })) }} /> : null}
      {editTarget ? <EditKeyModal keyItem={editTarget} onClose={() => setEditTarget(null)} onSaved={async (key) => { await keys.reload(); setSelectedId(key.id); setEditTarget(null) }} /> : null}
      {deleteTarget ? <DeleteKeyModal keyItem={deleteTarget} busy={busyKeyId === deleteTarget.id} onCancel={() => setDeleteTarget(null)} onConfirm={() => void remove(deleteTarget)} /> : null}
    </div>
  )
}

const thStyle = { padding: 16, textAlign: 'left' as const, borderBottom: '1px solid var(--vault-line)', color: 'var(--vault-muted)', fontWeight: 700, fontFamily: 'JetBrains Mono, monospace', fontSize: 12, letterSpacing: '0.05em', textTransform: 'uppercase' as const }
const tdStyle = { padding: 16, textAlign: 'left' as const, borderBottom: '1px solid var(--vault-line)' }
const inlineCode = { background: 'var(--vault-bg)', padding: '4px 8px', borderRadius: 4, color: 'var(--vault-gold)' }
const pillStyle = { padding: '4px 8px', borderRadius: 6, fontSize: 11, fontFamily: 'JetBrains Mono, monospace', fontWeight: 700, background: 'rgba(255,255,255,.06)', border: '1px solid var(--vault-line)' }

function CreateKeyModal({ onClose, onCreated }: { onClose: () => void; onCreated: (key: ApiKey) => Promise<void> }) {
  const app = useApp()
  const [name, setName] = useState('New Creative Client')
  const [rpm, setRpm] = useState(30)
  const [expiresAt, setExpiresAt] = useState('')
  const [totalQuotaPoints, setTotalQuotaPoints] = useState('')
  const [dailyQuotaPoints, setDailyQuotaPoints] = useState('')
  const [scopes, setScopes] = useState<string[]>(['images:write', 'images:read'])
  const [secret, setSecret] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  function toggleScope(scope: string) {
    setScopes((items) => items.includes(scope) ? items.filter((item) => item !== scope) : [...items, scope])
  }

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    try {
      const key = await userApi.createApiKey(apiKeyCreatePayload({ name, scopes, rpmLimit: rpm, expiresAt, totalQuotaPoints, dailyQuotaPoints }))
      setSecret(key.secret_preview ?? null)
      app.notify('success', '密钥已创建，Secret 仅展示一次')
      await onCreated(key)
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal title="创建 API 密钥" onClose={onClose}>
      <form className="modal-form" onSubmit={submit}>
        <Field label="密钥名称"><input value={name} onChange={(event) => setName(event.target.value)} minLength={3} required /></Field>
        <Field label="RPM 限制"><input type="number" min={1} max={600} value={rpm} onChange={(event) => setRpm(Number(event.target.value))} /></Field>
        <Field label="总额度"><input inputMode="decimal" placeholder="不填表示不限额" value={totalQuotaPoints} onChange={(event) => setTotalQuotaPoints(event.target.value)} /></Field>
        <Field label="日额度"><input inputMode="decimal" placeholder="不填表示不限额" value={dailyQuotaPoints} onChange={(event) => setDailyQuotaPoints(event.target.value)} /></Field>
        <Field label="过期时间"><input type="date" value={expiresAt} onChange={(event) => setExpiresAt(event.target.value)} /></Field>
        <div className="scope-grid">
          {allScopes.map((scope) => <label key={scope}><input type="checkbox" checked={scopes.includes(scope)} onChange={() => toggleScope(scope)} />{apiKeyScopeLabel(scope)}</label>)}
        </div>
        {secret ? <div className="secret-once"><strong>Secret Key</strong><code>{secret}</code><CopyButton text={secret} label="复制 Secret" /></div> : null}
        <div className="action-row"><Button type="submit" busy={busy}>创建</Button><Button type="button" tone="ghost" onClick={onClose}>关闭</Button></div>
      </form>
    </Modal>
  )
}

function DeleteKeyModal({ keyItem, busy, onCancel, onConfirm }: { keyItem: ApiKey; busy: boolean; onCancel: () => void; onConfirm: () => void }) {
  const copy = apiKeyDeleteConfirmText(keyItem)
  return (
    <Modal title={copy.title} onClose={onCancel}>
      <div className="modal-form">
        <p>{copy.detail}</p>
        <div className="action-row">
          <Button type="button" tone="ghost" onClick={onCancel} disabled={busy}>取消</Button>
          <Button type="button" tone="danger" busy={busy} onClick={onConfirm}>确认删除</Button>
        </div>
      </div>
    </Modal>
  )
}

function EditKeyModal({ keyItem, onClose, onSaved }: { keyItem: ApiKey; onClose: () => void; onSaved: (key: ApiKey) => Promise<void> }) {
  const app = useApp()
  const initial = apiKeyEditForm(keyItem)
  const [name, setName] = useState(initial.name)
  const [rpm, setRpm] = useState(initial.rpmLimit)
  const [expiresAt, setExpiresAt] = useState(initial.expiresAt)
  const [totalQuotaPoints, setTotalQuotaPoints] = useState(initial.totalQuotaPoints)
  const [dailyQuotaPoints, setDailyQuotaPoints] = useState(initial.dailyQuotaPoints)
  const [busy, setBusy] = useState(false)

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    try {
      const key = await userApi.updateApiKey(keyItem.id, apiKeyUpdatePayload({ name, rpmLimit: rpm, expiresAt, totalQuotaPoints, dailyQuotaPoints, groupCode: initial.groupCode }))
      app.notify('success', '密钥配置已更新')
      await onSaved(key)
    } catch (err) {
      app.notify('error', errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal title="编辑 API 密钥" onClose={onClose}>
      <form className="modal-form" onSubmit={submit}>
        <Field label="密钥名称"><input value={name} onChange={(event) => setName(event.target.value)} minLength={3} required /></Field>
        <Field label="当前分组"><input value={initial.groupCode} readOnly aria-readonly="true" /></Field>
        <Field label="RPM 限制"><input type="number" min={1} max={600} value={rpm} onChange={(event) => setRpm(Number(event.target.value))} /></Field>
        <Field label="总额度"><input inputMode="decimal" placeholder="不填表示不限额" value={totalQuotaPoints} onChange={(event) => setTotalQuotaPoints(event.target.value)} /></Field>
        <Field label="日额度"><input inputMode="decimal" placeholder="不填表示不限额" value={dailyQuotaPoints} onChange={(event) => setDailyQuotaPoints(event.target.value)} /></Field>
        <Field label="过期时间"><input type="date" value={expiresAt} onChange={(event) => setExpiresAt(event.target.value)} /></Field>
        <p style={{ color: 'var(--vault-muted)', fontSize: 13, margin: 0 }}>{apiKeyGroupReadOnlyHint}</p>
        <div className="action-row"><Button type="submit" busy={busy}>保存</Button><Button type="button" tone="ghost" onClick={onClose}>关闭</Button></div>
      </form>
    </Modal>
  )
}
