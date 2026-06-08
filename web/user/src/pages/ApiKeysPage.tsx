import { FormEvent, useMemo, useState } from 'react'
import type { ApiKey } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { userApi } from '../../../shared/user-api'
import { Button, CopyButton, EmptyState, Field, LoadingState, Modal, useApp } from '../components'
import { userButton, userCard, userForm, userShell, userText } from '../ui/classes'
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

const apiKeyClasses = {
  content: userShell.content,
  header: 'mb-12 flex flex-wrap items-end justify-between gap-5',
  title: 'm-0 font-[var(--font-display)] text-5xl leading-none',
  panel: cn(userCard.padded, 'mb-8'),
  tableWrap: 'overflow-x-auto',
  table: 'w-full min-w-[1160px] border-collapse text-sm',
  tableHeadCell: 'border-b border-[var(--border)] px-4 py-4 text-left font-mono text-xs font-bold uppercase text-[var(--muted)]',
  tableCell: 'border-b border-[var(--border)] px-4 py-4 text-left',
  row: 'cursor-pointer',
  rowSelected: 'outline outline-1 outline-[rgba(212,157,94,.45)]',
  mutedSmall: 'text-xs text-[var(--muted)]',
  mutedSmallTop: 'mt-1 text-xs text-[var(--muted)]',
  inlineCode: 'rounded bg-[var(--bg)] px-2 py-1 font-mono text-[var(--accent)]',
  badge: 'rounded-md border border-[var(--border)] bg-white/[.06] px-2 py-1 font-mono text-[11px] font-bold',
  badgeSuccess: 'border-[rgba(212,157,94,.35)] text-[var(--accent)]',
  actions: 'flex flex-wrap justify-end gap-2',
  tableAction: 'min-h-8 px-3 py-1.5 text-xs',
  dangerButton: 'text-[oklch(70%_.2_25)]',
  quickstartStack: 'grid gap-6',
  sectionTitle: 'm-0 font-[var(--font-display)] text-3xl leading-tight',
  codeCard: cn(userCard.base, 'overflow-hidden'),
  codeBar: 'flex items-center justify-between gap-3 border-b border-[var(--border)] bg-[var(--bg)] px-5 py-3 font-mono text-[13px] text-[var(--muted)]',
  codeBlock: 'overflow-x-auto bg-[var(--bg)] p-4 font-mono text-[13px] text-[var(--muted)]',
  codePre: 'm-0 whitespace-pre-wrap',
  docHint: 'text-sm text-[var(--muted)]',
  docLink: 'text-[var(--accent)] underline',
  modalForm: 'grid gap-4',
  modalActions: 'flex flex-wrap justify-end gap-3 max-[420px]:flex-col max-[420px]:items-stretch',
  modalHint: 'm-0 text-[13px] text-[var(--muted)]',
  scopeGrid: 'grid gap-2',
  scopeOption: 'inline-flex items-center gap-2 rounded-lg border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] px-3 py-2 text-sm text-[var(--muted)] has-[:checked]:border-[var(--accent)] has-[:checked]:text-[var(--fg)]',
  secretOnce: 'grid gap-2 rounded-xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] p-3',
  secretCode: 'overflow-x-auto rounded-lg bg-[var(--bg)] px-3 py-2 font-mono text-[var(--accent)]',
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
    <div className={apiKeyClasses.content}>
      <div className={apiKeyClasses.header}>
        <div>
          <p className={userText.eyebrow}>{apiKeyPageLabels.eyebrow}</p>
          <h1 className={apiKeyClasses.title}>API 密钥</h1>
        </div>
        <button className={cn(userButton.base, userButton.primary)} type="button" onClick={() => setCreating(true)}>+ {apiKeyPageLabels.create}</button>
      </div>

      <div className={apiKeyClasses.panel}>
        {keys.loading ? <LoadingState /> : null}
        {!keys.loading && !keys.data?.length ? <EmptyState title="暂无密钥" detail="创建一个密钥后即可调试开放 API。" action={<Button onClick={() => setCreating(true)}>创建密钥</Button>} /> : null}
        {keys.data?.length ? (
          <div className={apiKeyClasses.tableWrap}>
            <table className={apiKeyClasses.table}>
              <thead>
                <tr>
                  {apiKeyTableHeaders.map((head) => <th key={head} className={apiKeyClasses.tableHeadCell}>{head}</th>)}
                </tr>
              </thead>
              <tbody>
                {keys.data.map((key) => {
                  const row = apiKeyRow(key)
                  const badge = row.statusBadge
                  const secretPreview = secretPreviews[key.id] ?? key.secret_preview
                  return (
                    <tr key={key.id} onClick={() => setSelectedId(key.id)} className={cn(apiKeyClasses.row, selectedKey?.id === key.id && apiKeyClasses.rowSelected)}>
                      <td className={apiKeyClasses.tableCell}><strong>{key.name}</strong><div className={apiKeyClasses.mutedSmall}>{row.scopesText}</div></td>
                      <td className={apiKeyClasses.tableCell}><code className={cn('num', apiKeyClasses.inlineCode)}>{revealed[key.id] ? key.access_key : row.accessKeyMasked}</code></td>
                      <td className={apiKeyClasses.tableCell}><code className={cn('num', apiKeyClasses.inlineCode)}>{revealed[key.id] ? (secretPreview ?? '仅创建或重置时展示') : 'sk_••••••••••••'}</code></td>
                      <td className={apiKeyClasses.tableCell}><span className={cn(apiKeyClasses.badge, badge.tone === 'success' && apiKeyClasses.badgeSuccess)}>{badge.label}</span></td>
                      <td className={cn('num', apiKeyClasses.tableCell)}>{key.rpm_limit}</td>
                      <td className={cn('num', apiKeyClasses.tableCell)}>
                        <div>{row.totalQuotaLabel}</div>
                        <div className={apiKeyClasses.mutedSmallTop}>{row.dailyQuotaLabel}</div>
                      </td>
                      <td className={cn('num', apiKeyClasses.tableCell)}>{row.createdAtLabel}</td>
                      <td className={cn('num', apiKeyClasses.tableCell)}>{row.lastUsedAtLabel}</td>
                      <td className={cn('num', apiKeyClasses.tableCell)}>
                        <span>{row.expiresAtLabel}</span>
                        {row.expiryHint ? <div className={apiKeyClasses.mutedSmallTop}>{row.expiryHint}</div> : null}
                      </td>
                      <td className={cn(apiKeyClasses.tableCell, 'text-right')}>
                        <div className={apiKeyClasses.actions}>
                          <CopyButton text={key.access_key} label="复制 AK" />
                          {secretPreview ? <CopyButton text={secretPreview} label="复制 SK" /> : null}
                          <button className={cn(userButton.base, userButton.ghost, apiKeyClasses.tableAction)} type="button" disabled={busyKeyId === key.id} onClick={(event) => { event.stopPropagation(); setRevealed((rows) => ({ ...rows, [key.id]: !rows[key.id] })) }}>{revealed[key.id] ? apiKeyPageLabels.hide : apiKeyPageLabels.show}</button>
                          <button className={cn(userButton.base, userButton.ghost, apiKeyClasses.tableAction)} type="button" disabled={busyKeyId === key.id} onClick={(event) => { event.stopPropagation(); setEditTarget(key) }}>{apiKeyPageLabels.edit}</button>
                          <button className={cn(userButton.base, userButton.ghost, apiKeyClasses.tableAction)} type="button" disabled={busyKeyId === key.id} onClick={(event) => { event.stopPropagation(); void toggle(key) }}>{apiKeyStatusToggleLabel(key.status)}</button>
                          <button className={cn(userButton.base, userButton.ghost, apiKeyClasses.tableAction)} type="button" disabled={busyKeyId === key.id} onClick={(event) => { event.stopPropagation(); void resetSecret(key) }}>{apiKeyPageLabels.resetSecret}</button>
                          <button className={cn(userButton.base, userButton.ghost, apiKeyClasses.tableAction, apiKeyClasses.dangerButton)} type="button" disabled={busyKeyId === key.id} onClick={(event) => { event.stopPropagation(); setDeleteTarget(key) }}>{apiKeyPageLabels.delete}</button>
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

      <div className={apiKeyClasses.quickstartStack}>
        <h2 className={apiKeyClasses.sectionTitle}>{apiKeyPageLabels.quickstartTitle}</h2>
        <div className={apiKeyClasses.codeCard}>
          <div className={apiKeyClasses.codeBar}>
            <span>{quickstart.title}</span>
            <CopyButton text={quickstart.code} label={quickstart.copyLabel} />
          </div>
          <div className={apiKeyClasses.codeBlock}>
            <pre className={apiKeyClasses.codePre}>{quickstart.code}</pre>
          </div>
        </div>
        <p className={apiKeyClasses.docHint}>查看完整 <a href="#/docs" className={apiKeyClasses.docLink}>开发文档</a> 获取更多语言示例。</p>
      </div>

      {creating ? <CreateKeyModal onClose={() => setCreating(false)} onCreated={async (key) => { if (key.secret_preview) setSecretPreviews((rows) => ({ ...rows, [key.id]: key.secret_preview ?? '' })); await keys.reload(); setSelectedId(key.id); setRevealed((rows) => ({ ...rows, [key.id]: true })) }} /> : null}
      {editTarget ? <EditKeyModal keyItem={editTarget} onClose={() => setEditTarget(null)} onSaved={async (key) => { await keys.reload(); setSelectedId(key.id); setEditTarget(null) }} /> : null}
      {deleteTarget ? <DeleteKeyModal keyItem={deleteTarget} busy={busyKeyId === deleteTarget.id} onCancel={() => setDeleteTarget(null)} onConfirm={() => void remove(deleteTarget)} /> : null}
    </div>
  )
}

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
      <form className={apiKeyClasses.modalForm} onSubmit={submit}>
        <Field label="密钥名称"><input className={userForm.input} value={name} onChange={(event) => setName(event.target.value)} minLength={3} required /></Field>
        <Field label="RPM 限制"><input className={userForm.input} type="number" min={1} max={600} value={rpm} onChange={(event) => setRpm(Number(event.target.value))} /></Field>
        <Field label="总额度"><input className={userForm.input} inputMode="decimal" placeholder="不填表示不限额" value={totalQuotaPoints} onChange={(event) => setTotalQuotaPoints(event.target.value)} /></Field>
        <Field label="日额度"><input className={userForm.input} inputMode="decimal" placeholder="不填表示不限额" value={dailyQuotaPoints} onChange={(event) => setDailyQuotaPoints(event.target.value)} /></Field>
        <Field label="过期时间"><input className={userForm.input} type="date" value={expiresAt} onChange={(event) => setExpiresAt(event.target.value)} /></Field>
        <div className={apiKeyClasses.scopeGrid}>
          {allScopes.map((scope) => <label key={scope} className={apiKeyClasses.scopeOption}><input type="checkbox" checked={scopes.includes(scope)} onChange={() => toggleScope(scope)} />{apiKeyScopeLabel(scope)}</label>)}
        </div>
        {secret ? <div className={apiKeyClasses.secretOnce}><strong>Secret Key</strong><code className={apiKeyClasses.secretCode}>{secret}</code><CopyButton text={secret} label="复制 Secret" /></div> : null}
        <div className={apiKeyClasses.modalActions}><Button type="submit" busy={busy}>创建</Button><Button type="button" tone="ghost" onClick={onClose}>关闭</Button></div>
      </form>
    </Modal>
  )
}

function DeleteKeyModal({ keyItem, busy, onCancel, onConfirm }: { keyItem: ApiKey; busy: boolean; onCancel: () => void; onConfirm: () => void }) {
  const copy = apiKeyDeleteConfirmText(keyItem)
  return (
    <Modal title={copy.title} onClose={onCancel}>
      <div className={apiKeyClasses.modalForm}>
        <p>{copy.detail}</p>
        <div className={apiKeyClasses.modalActions}>
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
      <form className={apiKeyClasses.modalForm} onSubmit={submit}>
        <Field label="密钥名称"><input className={userForm.input} value={name} onChange={(event) => setName(event.target.value)} minLength={3} required /></Field>
        <Field label="当前分组"><input className={userForm.input} value={initial.groupCode} readOnly aria-readonly="true" /></Field>
        <Field label="RPM 限制"><input className={userForm.input} type="number" min={1} max={600} value={rpm} onChange={(event) => setRpm(Number(event.target.value))} /></Field>
        <Field label="总额度"><input className={userForm.input} inputMode="decimal" placeholder="不填表示不限额" value={totalQuotaPoints} onChange={(event) => setTotalQuotaPoints(event.target.value)} /></Field>
        <Field label="日额度"><input className={userForm.input} inputMode="decimal" placeholder="不填表示不限额" value={dailyQuotaPoints} onChange={(event) => setDailyQuotaPoints(event.target.value)} /></Field>
        <Field label="过期时间"><input className={userForm.input} type="date" value={expiresAt} onChange={(event) => setExpiresAt(event.target.value)} /></Field>
        <p className={apiKeyClasses.modalHint}>{apiKeyGroupReadOnlyHint}</p>
        <div className={apiKeyClasses.modalActions}><Button type="submit" busy={busy}>保存</Button><Button type="button" tone="ghost" onClick={onClose}>关闭</Button></div>
      </form>
    </Modal>
  )
}
