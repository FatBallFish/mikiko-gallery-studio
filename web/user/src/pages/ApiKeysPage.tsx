import { FormEvent, useMemo, useState } from 'react'
import type { ApiKey } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { userApi } from '../../../shared/user-api'
import { Button, CopyButton, EmptyState, Field, LoadingState, Modal, useApp } from '../components'
import { openDocsEntry } from '../docsUrl'
import { userButton, userForm } from '../ui/classes'
import { SettingsWorkspace } from '../ui/SettingsWorkspace'
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
  content: 'w-full flex-1 p-6 md:p-10',
  header: 'mb-10 flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between',
  title: 'm-0 text-4xl font-black leading-none md:text-6xl',
  detail: 'mt-4 max-w-2xl text-base leading-relaxed text-[var(--muted)]',
  createButton: 'h-12 rounded-2xl bg-[var(--accent)] px-6 text-sm font-bold text-white shadow-lg shadow-[var(--accent)]/20 transition-transform hover:scale-[1.03] active:scale-[0.98]',
  metricGrid: 'mb-8 grid grid-cols-1 gap-4 md:grid-cols-3',
  metric: 'rounded-3xl border border-[var(--border)] bg-[var(--surface)] p-6',
  metricLabel: 'mb-2 text-xs font-bold text-[var(--muted)]',
  metricValue: 'text-3xl font-black text-[var(--fg)]',
  panel: 'mb-8 overflow-hidden rounded-3xl border border-[var(--border)] bg-[var(--surface)]',
  panelHead: 'flex flex-col gap-3 border-b border-[var(--border)] p-6 md:flex-row md:items-center md:justify-between',
  panelTitle: 'text-2xl font-black',
  panelHint: 'mt-1 text-sm text-[var(--muted)]',
  tableWrap: 'overflow-x-auto',
  table: 'w-full min-w-[1160px] border-collapse text-sm',
  tableHeadCell: 'border-b border-[var(--border)] px-5 py-4 text-left font-vault-mono text-[10px] font-bold uppercase tracking-widest text-[var(--muted)]',
  tableCell: 'border-b border-[var(--border)] px-5 py-5 text-left',
  row: 'cursor-pointer transition hover:bg-[var(--accent)]/5',
  rowSelected: 'bg-[var(--accent)]/5 outline outline-1 outline-[rgba(var(--accent-rgb),.42)]',
  mutedSmall: 'text-xs text-[var(--muted)]',
  mutedSmallTop: 'mt-1 text-xs text-[var(--muted)]',
  inlineCode: 'rounded-xl bg-[var(--bg)] px-2 py-1 font-vault-mono text-[var(--accent)]',
  badge: 'rounded-xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_6%,transparent)] px-2 py-1 font-mono text-[11px] font-bold',
  badgeSuccess: 'border-[color-mix(in_oklch,var(--accent)_35%,var(--border))] text-[var(--accent)]',
  actions: 'flex flex-wrap gap-2',
  tableAction: 'min-h-8 rounded-xl px-3 py-1.5 text-xs',
  dangerButton: 'text-[var(--accent-coral)]',
  oneTimeSecret: 'mb-8 rounded-3xl border border-[color-mix(in_oklch,var(--accent)_42%,var(--border))] bg-[color-mix(in_oklch,var(--accent)_8%,transparent)] p-6',
  oneTimeSecretHead: 'mb-3 flex flex-wrap items-center justify-between gap-3',
  oneTimeSecretTitle: 'font-bold text-[var(--fg)]',
  oneTimeSecretText: 'm-0 text-sm text-[var(--muted)]',
  quickstartGrid: 'grid grid-cols-1 gap-6 xl:grid-cols-[1.1fr_0.9fr]',
  quickstartStack: 'overflow-hidden rounded-3xl border border-[var(--border)] bg-[var(--surface)]',
  sectionTitle: 'm-0 text-2xl font-black leading-tight',
  codeCard: 'overflow-hidden',
  codeBar: 'flex items-center justify-between gap-3 border-b border-[var(--border)] bg-[var(--bg)] px-5 py-3 font-mono text-[13px] text-[var(--muted)]',
  codeBlock: 'overflow-x-auto bg-[var(--bg)] p-4 font-mono text-[13px] text-[var(--muted)]',
  codePre: 'm-0 whitespace-pre-wrap',
  docHint: 'p-6 pt-0 text-sm text-[var(--muted)]',
  docLink: 'text-[var(--accent)] underline',
  securityCard: 'rounded-3xl border border-[var(--border)] bg-[var(--surface)] p-6',
  securityList: 'grid gap-3 text-sm text-[var(--muted)]',
  securityItem: 'rounded-2xl border border-[var(--border)] bg-[var(--bg)]/50 p-4',
  modalForm: 'grid gap-4',
  modalActions: 'flex flex-wrap justify-end gap-3 max-[420px]:flex-col max-[420px]:items-stretch',
  modalPrimaryButton: 'min-h-11 rounded-2xl bg-[var(--accent)] px-5 text-sm font-bold text-white shadow-lg shadow-[rgba(var(--accent-rgb),0.24)] transition hover:scale-[1.02]',
  modalGhostButton: 'min-h-11 rounded-2xl border border-[var(--border)] bg-[var(--surface)] px-5 text-sm font-bold text-[var(--fg)] transition hover:border-[var(--accent)] hover:text-[var(--accent)]',
  modalHint: 'm-0 text-[13px] text-[var(--muted)]',
  scopeGrid: 'grid gap-2 sm:grid-cols-2',
  scopeOption: 'inline-flex min-h-12 cursor-pointer items-center gap-3 rounded-2xl border border-[var(--border)] bg-[var(--bg)]/60 px-3 py-2 text-sm font-bold text-[var(--muted)] transition-all hover:border-[var(--accent)]/60 hover:text-[var(--fg)]',
  scopeOptionActive: 'border-[var(--accent)] bg-[var(--accent)]/10 text-[var(--fg)] ring-1 ring-[var(--accent)]/45',
  scopeCheck: 'grid size-5 shrink-0 place-items-center rounded-xl border border-[var(--border)] bg-[var(--surface)] text-[11px] font-black text-transparent transition-colors',
  scopeCheckActive: 'border-[var(--accent)] bg-[var(--accent)] text-white shadow-[0_0_0_1px_color-mix(in_oklch,var(--accent)_40%,transparent)]',
  rpmControl: 'grid gap-3 rounded-2xl border border-[var(--border)] bg-[var(--bg)]/60 p-3',
  rpmRow: 'flex items-center justify-between gap-3',
  rpmValue: 'min-w-20 rounded-xl border border-[var(--border)] bg-[var(--surface)] px-3 py-2 text-center font-vault-mono text-lg font-black text-[var(--accent)]',
  rpmButton: 'grid size-10 cursor-pointer place-items-center rounded-xl border border-[var(--border)] bg-[var(--surface)] text-lg font-black text-[var(--fg)] transition hover:border-[var(--accent)] hover:text-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-45',
  rpmRange: 'accent-[var(--accent)]',
  secretOnce: 'grid gap-2 rounded-xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--fg)_5%,transparent)] p-3',
  secretCode: 'overflow-x-auto rounded-xl bg-[var(--bg)] px-3 py-2 font-mono text-[var(--accent)]',
}

export function ApiKeysPage() {
  const app = useApp()
  const keys = useApiResource(() => userApi.listApiKeys(), [])
  const [creating, setCreating] = useState(false)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [oneTimeSecret, setOneTimeSecret] = useState<{ keyId: string; name: string; secret: string } | null>(null)
  const [editTarget, setEditTarget] = useState<ApiKey | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ApiKey | null>(null)
  const [busyKeyId, setBusyKeyId] = useState<string | null>(null)
  const selectedKey = useMemo(() => keys.data?.find((key) => key.id === selectedId) ?? keys.data?.[0] ?? null, [keys.data, selectedId])
  const quickstart = apiKeyQuickstart(selectedKey)
  const activeCount = useMemo(() => (keys.data ?? []).filter((key) => key.status === 'active').length, [keys.data])

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
      if (updated.secret_preview) setOneTimeSecret({ keyId: updated.id, name: updated.name, secret: updated.secret_preview })
      app.notify('success', 'Secret 已重置，仅展示一次')
      await keys.reload()
      setSelectedId(updated.id)
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
    <SettingsWorkspace
      active="api-keys"
      title="API 密钥"
      detail="管理开放接口调用凭证、限速和额度。Secret 仅在创建或重置时展示一次。"
      action={<button className={apiKeyClasses.createButton} type="button" onClick={() => setCreating(true)}>+ {apiKeyPageLabels.create}</button>}
    >
      {oneTimeSecret ? (
        <div className={apiKeyClasses.oneTimeSecret}>
          <div className={apiKeyClasses.oneTimeSecretHead}>
            <div>
              <div className={apiKeyClasses.oneTimeSecretTitle}>{oneTimeSecret.name} 的 Secret 仅展示一次</div>
              <p className={apiKeyClasses.oneTimeSecretText}>关闭此提示后将无法再次查看明文，请先复制并更新调用方配置。</p>
            </div>
            <button className={cn(userButton.base, userButton.ghost, apiKeyClasses.tableAction)} type="button" onClick={() => setOneTimeSecret(null)}>关闭</button>
          </div>
          <div className={apiKeyClasses.secretOnce}>
            <code className={apiKeyClasses.secretCode}>{oneTimeSecret.secret}</code>
            <CopyButton text={oneTimeSecret.secret} label="复制 Secret" />
          </div>
        </div>
      ) : null}

      <div className={apiKeyClasses.metricGrid}>
        <ApiMetric label="启用密钥" value={`${activeCount} / ${keys.data?.length ?? 0}`} />
        <ApiMetric label="密钥总数" value={String(keys.data?.length ?? 0)} />
        <ApiMetric label="当前 RPM" value={selectedKey ? String(selectedKey.rpm_limit) : '-'} />
      </div>

      <div className={apiKeyClasses.panel}>
        <div className={apiKeyClasses.panelHead}>
          <div>
            <h2 className={apiKeyClasses.panelTitle}>密钥列表</h2>
            <p className={apiKeyClasses.panelHint}>Secret 创建或重置后仍只展示掩码预览。</p>
          </div>
          <button className={cn(userButton.base, userButton.ghost, 'rounded-2xl')} type="button" onClick={() => void keys.reload()}>刷新</button>
        </div>
        {keys.loading ? <ApiKeysTableSkeleton /> : null}
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
                  return (
                    <tr key={key.id} onClick={() => setSelectedId(key.id)} className={cn(apiKeyClasses.row, selectedKey?.id === key.id && apiKeyClasses.rowSelected)}>
                      <td className={apiKeyClasses.tableCell}><strong>{key.name}</strong><div className={apiKeyClasses.mutedSmall}>{row.scopesText}</div></td>
                      <td className={apiKeyClasses.tableCell}><code className={cn('num', apiKeyClasses.inlineCode)}>{row.accessKeyMasked}</code></td>
                      <td className={apiKeyClasses.tableCell}><code className={cn('num', apiKeyClasses.inlineCode)}>{row.secretMasked}</code></td>
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

      <div className={apiKeyClasses.quickstartGrid}>
        <div className={apiKeyClasses.quickstartStack}>
        <div className="flex items-center justify-between border-b border-[var(--border)] px-6 py-4">
          <h2 className={apiKeyClasses.sectionTitle}>{apiKeyPageLabels.quickstartTitle}</h2>
          <CopyButton text={quickstart.code} label={quickstart.copyLabel} />
        </div>
        <div className={apiKeyClasses.codeCard}>
          <div className={apiKeyClasses.codeBar}>
            <span>{quickstart.title}</span>
          </div>
          <div className={apiKeyClasses.codeBlock}>
            <pre className={apiKeyClasses.codePre}>{quickstart.code}</pre>
          </div>
        </div>
        <p className={apiKeyClasses.docHint}>查看完整 <button type="button" className={cn(apiKeyClasses.docLink, 'border-0 bg-transparent p-0')} onClick={() => openDocsEntry('api-keys')}>开发文档</button> 获取更多语言示例。</p>
        </div>
        <div className={apiKeyClasses.securityCard}>
          <h3 className="mb-4 text-xl font-black">安全建议</h3>
          <div className={apiKeyClasses.securityList}>
            <SecurityHint title="按环境拆分" detail="生产、测试和自动化脚本使用不同密钥，便于快速吊销。" />
            <SecurityHint title="设置额度" detail="为高频调用方配置日额度和总额度，避免异常消耗积分。" />
            <SecurityHint title="定期轮换" detail="重置 Secret 后旧密钥立即失效，客户端需同步更新。" />
          </div>
        </div>
      </div>

      {creating ? <CreateKeyModal onClose={() => setCreating(false)} onCreated={async (key) => {
        await keys.reload()
        setSelectedId(key.id)
        if (key.secret_preview) setOneTimeSecret({ keyId: key.id, name: key.name, secret: key.secret_preview })
      }} /> : null}
      {editTarget ? <EditKeyModal keyItem={editTarget} onClose={() => setEditTarget(null)} onSaved={async (key) => { await keys.reload(); setSelectedId(key.id); setEditTarget(null) }} /> : null}
      {deleteTarget ? <DeleteKeyModal keyItem={deleteTarget} busy={busyKeyId === deleteTarget.id} onCancel={() => setDeleteTarget(null)} onConfirm={() => void remove(deleteTarget)} /> : null}
    </SettingsWorkspace>
  )
}

function ApiKeysTableSkeleton() {
  return (
    <div className="grid gap-4 p-6" aria-hidden="true">
      <div className={apiKeyClasses.metricGrid}>
        {Array.from({ length: 3 }).map((_, index) => (
          <div key={index} className={apiKeyClasses.metric}>
            <div className="pg-skeleton h-3 w-20 rounded-xl" />
            <div className="mt-3 pg-skeleton h-8 w-24 rounded-xl" />
          </div>
        ))}
      </div>
      <div className="grid gap-3">
        {Array.from({ length: 5 }).map((_, index) => (
          <div key={index} className="grid gap-3 rounded-2xl border border-[var(--border)] bg-[var(--bg)]/35 p-4">
            <div className="pg-skeleton h-4 w-1/4 rounded-xl" />
            <div className="pg-skeleton h-4 w-2/3 rounded-xl" />
            <div className="pg-skeleton h-4 w-1/2 rounded-xl" />
          </div>
        ))}
      </div>
    </div>
  )
}

function ApiMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className={apiKeyClasses.metric}>
      <div className={apiKeyClasses.metricLabel}>{label}</div>
      <div className={apiKeyClasses.metricValue}>{value}</div>
    </div>
  )
}

function SecurityHint({ title, detail }: { title: string; detail: string }) {
  return (
    <div className={apiKeyClasses.securityItem}>
      <strong className="mb-1 block text-[var(--fg)]">{title}</strong>
      <span>{detail}</span>
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
  const [busy, setBusy] = useState(false)

  function toggleScope(scope: string) {
    setScopes((items) => items.includes(scope) ? items.filter((item) => item !== scope) : [...items, scope])
  }

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    try {
      const key = await userApi.createApiKey(apiKeyCreatePayload({ name, scopes, rpmLimit: rpm, expiresAt, totalQuotaPoints, dailyQuotaPoints }))
      app.notify('success', '密钥已创建，Secret 仅展示一次')
      await onCreated(key)
      onClose()
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
        <Field label="RPM 限制"><RpmControl value={rpm} onChange={setRpm} /></Field>
        <Field label="总额度"><input className={userForm.input} inputMode="decimal" placeholder="不填表示不限额" value={totalQuotaPoints} onChange={(event) => setTotalQuotaPoints(event.target.value)} /></Field>
        <Field label="日额度"><input className={userForm.input} inputMode="decimal" placeholder="不填表示不限额" value={dailyQuotaPoints} onChange={(event) => setDailyQuotaPoints(event.target.value)} /></Field>
        <Field label="过期时间"><input className={cn(userForm.input, 'pg-date-input')} type="date" value={expiresAt} onChange={(event) => setExpiresAt(event.target.value)} /></Field>
        <div className={apiKeyClasses.scopeGrid}>
          {allScopes.map((scope) => {
            const checked = scopes.includes(scope)
            return (
              <button key={scope} type="button" className={cn(apiKeyClasses.scopeOption, checked && apiKeyClasses.scopeOptionActive)} aria-pressed={checked} onClick={() => toggleScope(scope)}>
                <span className={cn(apiKeyClasses.scopeCheck, checked && apiKeyClasses.scopeCheckActive)} aria-hidden="true">{checked ? '✓' : ''}</span>
                {apiKeyScopeLabel(scope)}
              </button>
            )
          })}
        </div>
        <div className={apiKeyClasses.modalActions}>
          <Button type="submit" busy={busy} className={apiKeyClasses.modalPrimaryButton}>创建</Button>
          <Button type="button" tone="ghost" className={apiKeyClasses.modalGhostButton} onClick={onClose}>关闭</Button>
        </div>
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

function RpmControl({ value, onChange }: { value: number; onChange: (value: number) => void }) {
  const nextValue = (next: number) => onChange(Math.min(600, Math.max(1, next)))
  return (
    <div className={apiKeyClasses.rpmControl}>
      <div className={apiKeyClasses.rpmRow}>
        <button className={apiKeyClasses.rpmButton} type="button" onClick={() => nextValue(value - 5)} disabled={value <= 1}>-</button>
        <span className={apiKeyClasses.rpmValue}>{value}</span>
        <button className={apiKeyClasses.rpmButton} type="button" onClick={() => nextValue(value + 5)} disabled={value >= 600}>+</button>
      </div>
      <input
        className={apiKeyClasses.rpmRange}
        type="range"
        min={1}
        max={600}
        step={1}
        value={value}
        onChange={(event) => nextValue(Number(event.target.value))}
      />
    </div>
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
        <Field label="RPM 限制"><RpmControl value={rpm} onChange={setRpm} /></Field>
        <Field label="总额度"><input className={userForm.input} inputMode="decimal" placeholder="不填表示不限额" value={totalQuotaPoints} onChange={(event) => setTotalQuotaPoints(event.target.value)} /></Field>
        <Field label="日额度"><input className={userForm.input} inputMode="decimal" placeholder="不填表示不限额" value={dailyQuotaPoints} onChange={(event) => setDailyQuotaPoints(event.target.value)} /></Field>
        <Field label="过期时间"><input className={cn(userForm.input, 'pg-date-input')} type="date" value={expiresAt} onChange={(event) => setExpiresAt(event.target.value)} /></Field>
        <p className={apiKeyClasses.modalHint}>{apiKeyGroupReadOnlyHint}</p>
        <div className={apiKeyClasses.modalActions}><Button type="submit" busy={busy}>保存</Button><Button type="button" tone="ghost" onClick={onClose}>关闭</Button></div>
      </form>
    </Modal>
  )
}
