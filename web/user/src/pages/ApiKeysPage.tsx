import { FormEvent, useMemo, useState } from 'react'
import type { ApiKey } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { userApi } from '../../../shared/user-api'
import { Button, CopyButton, EmptyState, ErrorState, Modal, copyText, useApp } from '../components'
import { openDocsEntry } from '../docsUrl'
import { Check, Copy, Edit, RotateCcw, Trash2, X } from '../ui/icons'
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

function apiKeyStatusTone(status?: string) {
  if (status === 'active') return 'u-status--ok'
  if (status === 'expired') return 'u-status--warn'
  return ''
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

  async function copyValue(value: string, message: string) {
    try {
      await copyText(value)
      app.notify('success', message)
    } catch (caught) {
      app.notify('error', errorMessage(caught))
    }
  }

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
    <div className="acct-page">
      <div className="u-settings u-settings--single">
        <div className="u-settings__stack">
          <div className="u-page-head" style={{ margin: 0 }}>
            <div className="u-page-head__copy">
              <span className="u-eyebrow">API Access</span>
              <h1 className="u-h1" style={{ fontSize: 'clamp(32px, 3.4vw, 38px)' }}>API 密钥</h1>
              <p className="u-sub">用于服务端调用与第三方集成。Secret 只显示一次；轮换后旧密钥立即失效。</p>
            </div>
            <div className="u-page-head__actions">
              <button className="u-btn u-btn--primary u-btn--sm" type="button" onClick={() => setCreating(true)}>
                {apiKeyPageLabels.create}
              </button>
            </div>
          </div>

          {oneTimeSecret ? (
            <div className="ak-secret">
              <span className="acct-secret-mark" aria-hidden="true">!</span>
              <div className="acct-secret-copy">
                <strong style={{ fontSize: 13 }}>{oneTimeSecret.name} 已创建，请立即保存 Secret</strong>
                <div className="acct-secret-value">
                  <code className="ak-code acct-secret-code">{oneTimeSecret.secret}</code>
                  <button className="u-btn u-btn--sm" type="button" onClick={() => void copyValue(oneTimeSecret.secret, 'Secret 已复制')}>复制</button>
                </div>
                <span className="u-meta">关闭此提示后将无法再次查看完整 Secret。</span>
              </div>
              <button className="u-icon-btn u-icon-btn--plain" type="button" aria-label="关闭提示" onClick={() => setOneTimeSecret(null)}>
                <X size={17} strokeWidth={1.6} aria-hidden="true" />
              </button>
            </div>
          ) : null}

          <div className="u-stat-row">
            <div className="u-stat">
              <span className="u-stat__label">启用密钥</span>
              <span className="u-stat__value">{activeCount} / {keys.data?.length ?? 0}</span>
              <span className="u-stat__foot">当前账号可用凭据</span>
            </div>
            <div className="u-stat">
              <span className="u-stat__label">密钥总数</span>
              <span className="u-stat__value">{keys.data?.length ?? 0}</span>
              <span className="u-stat__foot">包含禁用与已撤销</span>
            </div>
            <div className="u-stat">
              <span className="u-stat__label">RPM 限制</span>
              <span className="u-stat__value">{selectedKey?.rpm_limit ?? '—'}</span>
              <span className="u-stat__foot">{selectedKey ? selectedKey.name : '选择密钥查看'}</span>
            </div>
          </div>

          <div className="u-plate">
            <div className="acct-keys-head">
              <div style={{ display: 'grid', gap: 4, minWidth: 0 }}>
                <h2 className="u-h3">密钥列表</h2>
                <span className="u-meta">Secret 创建或重置后仍只展示掩码预览。</span>
              </div>
              <button className="u-btn u-btn--sm" type="button" disabled={keys.loading} onClick={() => void keys.reload()}>刷新</button>
            </div>
            {keys.error ? <div style={{ padding: 18 }}><ErrorState message={keys.error} onRetry={() => void keys.reload()} /></div> : null}
            {keys.loading && !keys.data?.length ? <ApiKeysTableSkeleton /> : null}
            {!keys.loading && !keys.error && !keys.data?.length ? (
              <div style={{ padding: 18 }}>
                <EmptyState title="暂无密钥" detail="创建一个密钥后即可调试开放 API。" action={<button className="u-btn u-btn--primary u-btn--sm" type="button" onClick={() => setCreating(true)}>创建密钥</button>} />
              </div>
            ) : null}
            {keys.data?.length ? (
              <div className="acct-keys-table-wrap">
                <table className="ak-table">
                  <thead>
                    <tr>
                      {apiKeyTableHeaders.map((head, index) => (
                        <th key={head} style={index === apiKeyTableHeaders.length - 1 ? { textAlign: 'right' } : undefined}>{head}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {keys.data.map((key) => {
                      const row = apiKeyRow(key)
                      const badge = row.statusBadge
                      return (
                        <tr
                          key={key.id}
                          className={selectedKey?.id === key.id ? 'is-selected' : undefined}
                          tabIndex={0}
                          aria-selected={selectedKey?.id === key.id}
                          onClick={() => setSelectedId(key.id)}
                          onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); setSelectedId(key.id) } }}
                        >
                          <td>
                            <div className="acct-key-name">
                              <strong>{key.name}</strong>
                              <span className="u-meta">{row.scopesText}</span>
                            </div>
                          </td>
                          <td><span className="u-chip u-chip--mono acct-key-chip" title={key.access_key}>{row.accessKeyMasked}</span></td>
                          <td><span className="u-chip u-chip--mono acct-key-chip">{row.secretMasked}</span></td>
                          <td><span className={cn('u-status', apiKeyStatusTone(key.status))}><span className="u-dot" aria-hidden="true" />{badge.label}</span></td>
                          <td className="u-num">{key.rpm_limit ?? '—'}</td>
                          <td>
                            <div className="acct-quota">
                              <span>{row.totalQuotaLabel}</span>
                              <span className="u-meta">{row.dailyQuotaLabel}</span>
                            </div>
                          </td>
                          <td className="u-meta">{row.createdAtLabel}</td>
                          <td className="u-meta">{row.lastUsedAtLabel}</td>
                          <td>
                            <div className="acct-quota">
                              <span>{row.expiresAtLabel}</span>
                              {row.expiryHint ? <span className="u-meta">{row.expiryHint}</span> : null}
                            </div>
                          </td>
                          <td>
                            <div className="ak-actions">
                              <button type="button" title="复制 Access Key" aria-label="复制 Access Key" onClick={(event) => { event.stopPropagation(); void copyValue(key.access_key, 'Access Key 已复制') }}>
                                <Copy size={13} strokeWidth={1.7} aria-hidden="true" />
                              </button>
                              <button type="button" title={apiKeyPageLabels.edit} aria-label={apiKeyPageLabels.edit} disabled={busyKeyId === key.id} onClick={(event) => { event.stopPropagation(); setEditTarget(key) }}>
                                <Edit size={13} strokeWidth={1.7} aria-hidden="true" />
                              </button>
                              <button type="button" title={apiKeyStatusToggleLabel(key.status)} aria-label={apiKeyStatusToggleLabel(key.status)} disabled={busyKeyId === key.id} onClick={(event) => { event.stopPropagation(); void toggle(key) }}>
                                {key.status === 'active' ? <X size={13} strokeWidth={1.7} aria-hidden="true" /> : <Check size={13} strokeWidth={1.7} aria-hidden="true" />}
                              </button>
                              <button type="button" title={apiKeyPageLabels.resetSecret} aria-label={apiKeyPageLabels.resetSecret} disabled={busyKeyId === key.id} onClick={(event) => { event.stopPropagation(); void resetSecret(key) }}>
                                <RotateCcw size={13} strokeWidth={1.7} aria-hidden="true" />
                              </button>
                              <button type="button" title={apiKeyPageLabels.delete} aria-label={apiKeyPageLabels.delete} disabled={busyKeyId === key.id} onClick={(event) => { event.stopPropagation(); setDeleteTarget(key) }}>
                                <Trash2 size={13} strokeWidth={1.7} aria-hidden="true" />
                              </button>
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

          <div className="u-split">
            <div className="u-plate u-plate--pad">
              <div className="u-panel-title">
                <h3>{apiKeyPageLabels.quickstartTitle}</h3>
                <CopyButton text={quickstart.code} label={quickstart.copyLabel} />
              </div>
              <pre className="acct-code-block">{quickstart.code}</pre>
              <p className="u-meta" style={{ marginTop: 12 }}>
                查看完整 <button type="button" className="u-link acct-inline-link" onClick={() => openDocsEntry('api-keys')}>开发文档</button> 获取更多语言示例。
              </p>
            </div>
            <div className="u-plate u-plate--pad">
              <div className="u-panel-title"><h3>安全建议</h3></div>
              <ul className="acct-security-list">
                <li className="u-meta">只把 Secret 放在服务端环境变量中，不要提交到仓库。</li>
                <li className="u-meta">为不同集成创建独立密钥，便于单独停用与轮换。</li>
                <li className="u-meta">设置日额度，避免脚本异常消耗积分。</li>
              </ul>
            </div>
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
    </div>
  )
}

function ApiKeysTableSkeleton() {
  return (
    <div style={{ display: 'grid', gap: 10, padding: 18 }} aria-hidden="true">
      {Array.from({ length: 5 }).map((_, index) => (
        <div key={index} style={{ display: 'grid', gap: 8, paddingBottom: 12, borderBottom: '1px solid var(--line, var(--border))' }}>
          <div className="pg-skeleton h-4 w-1/4 rounded-xl" />
          <div className="pg-skeleton h-4 w-2/3 rounded-xl" />
        </div>
      ))}
    </div>
  )
}

function CreateKeyModal({ onClose, onCreated }: { onClose: () => void; onCreated: (key: ApiKey) => Promise<void> }) {
  const app = useApp()
  const [name, setName] = useState('')
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
      const key = await userApi.createApiKey(apiKeyCreatePayload({ name: name.trim(), scopes, rpmLimit: rpm, expiresAt, totalQuotaPoints, dailyQuotaPoints }))
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
    <Modal title="创建 API 密钥" onClose={onClose} className="acct-modal">
      <form style={{ display: 'grid', gap: 14 }} onSubmit={submit}>
        <label className="u-field">
          <span className="u-label">名称</span>
          <input className="u-input" value={name} placeholder="例如：生产服务" minLength={3} required autoFocus onChange={(event) => setName(event.target.value)} />
        </label>
        <div className="acct-form-pair">
          <label className="u-field">
            <span className="u-label">每分钟请求 <span className="u-label__hint">1–600</span></span>
            <input className="u-input" inputMode="numeric" min={1} max={600} value={rpm} onChange={(event) => setRpm(Math.min(600, Math.max(1, Number(event.target.value) || 1)))} />
          </label>
          <label className="u-field">
            <span className="u-label">总额度</span>
            <input className="u-input" inputMode="decimal" placeholder="留空表示不限制" value={totalQuotaPoints} onChange={(event) => setTotalQuotaPoints(event.target.value)} />
          </label>
        </div>
        <label className="u-field">
          <span className="u-label">日额度</span>
          <input className="u-input" inputMode="decimal" placeholder="留空表示不限制" value={dailyQuotaPoints} onChange={(event) => setDailyQuotaPoints(event.target.value)} />
        </label>
        <label className="u-field">
          <span className="u-label">过期时间 <span className="u-label__hint">可选</span></span>
          <input className="u-input" type="date" value={expiresAt} onChange={(event) => setExpiresAt(event.target.value)} />
        </label>
        <div className="u-field">
          <span className="u-label">权限范围</span>
          <div style={{ display: 'grid', gap: 7 }}>
            {allScopes.map((scope) => (
              <label className="u-switch" key={scope}>
                <input type="checkbox" checked={scopes.includes(scope)} onChange={() => toggleScope(scope)} />
                <span className="u-switch__track" aria-hidden="true" />
                {scope} · {apiKeyScopeLabel(scope)}
              </label>
            ))}
          </div>
        </div>
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, flexWrap: 'wrap' }}>
          <button className="u-btn" type="button" onClick={onClose} disabled={busy}>取消</button>
          <Button type="submit" busy={busy} disabled={name.trim().length < 3}>创建密钥</Button>
        </div>
      </form>
    </Modal>
  )
}

function DeleteKeyModal({ keyItem, busy, onCancel, onConfirm }: { keyItem: ApiKey; busy: boolean; onCancel: () => void; onConfirm: () => void }) {
  const copy = apiKeyDeleteConfirmText(keyItem)
  return (
    <Modal title={copy.title} onClose={onCancel} className="acct-modal">
      <div style={{ display: 'grid', gap: 14 }}>
        <p className="u-sub" style={{ margin: 0 }}>{copy.detail}</p>
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, flexWrap: 'wrap' }}>
          <button className="u-btn" type="button" onClick={onCancel} disabled={busy}>取消</button>
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
    <Modal title="编辑 API 密钥" onClose={onClose} className="acct-modal">
      <form style={{ display: 'grid', gap: 14 }} onSubmit={submit}>
        <label className="u-field">
          <span className="u-label">名称</span>
          <input className="u-input" value={name} minLength={3} required autoFocus onChange={(event) => setName(event.target.value)} />
        </label>
        <label className="u-field">
          <span className="u-label">当前分组</span>
          <input className="u-input" value={initial.groupCode} readOnly aria-readonly="true" />
        </label>
        <div className="acct-form-pair">
          <label className="u-field">
            <span className="u-label">每分钟请求 <span className="u-label__hint">1–600</span></span>
            <input className="u-input" inputMode="numeric" min={1} max={600} value={rpm} onChange={(event) => setRpm(Math.min(600, Math.max(1, Number(event.target.value) || 1)))} />
          </label>
          <label className="u-field">
            <span className="u-label">总额度</span>
            <input className="u-input" inputMode="decimal" placeholder="留空表示不限制" value={totalQuotaPoints} onChange={(event) => setTotalQuotaPoints(event.target.value)} />
          </label>
        </div>
        <label className="u-field">
          <span className="u-label">日额度</span>
          <input className="u-input" inputMode="decimal" placeholder="留空表示不限制" value={dailyQuotaPoints} onChange={(event) => setDailyQuotaPoints(event.target.value)} />
        </label>
        <label className="u-field">
          <span className="u-label">过期时间 <span className="u-label__hint">可选</span></span>
          <input className="u-input" type="date" value={expiresAt} onChange={(event) => setExpiresAt(event.target.value)} />
        </label>
        <p className="u-meta" style={{ margin: 0 }}>{apiKeyGroupReadOnlyHint}</p>
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, flexWrap: 'wrap' }}>
          <button className="u-btn" type="button" onClick={onClose} disabled={busy}>关闭</button>
          <Button type="submit" busy={busy} disabled={name.trim().length < 3}>保存</Button>
        </div>
      </form>
    </Modal>
  )
}