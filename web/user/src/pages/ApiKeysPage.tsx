import { FormEvent, useMemo, useState } from 'react'
import type { ApiKey } from '../../../shared/api-types'
import { mockApi } from '../../../shared/mock-api'
import { Button, CopyButton, EmptyState, ErrorState, Field, LoadingState, Modal, useApp } from '../components'
import { errorMessage, useMockResource } from '../useMockResource'

const allScopes = ['images:write', 'images:read', 'balance:read', 'profile:read']

const pageStyle = {
  content: { padding: 40, maxWidth: 1200, marginInline: 'auto', width: '100%' } as const,
  header: { marginBottom: 48, display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', gap: 20, flexWrap: 'wrap' as const },
  title: { fontSize: 48, margin: 0 },
  card: { background: 'var(--vault-panel)', borderRadius: 12, border: '1px solid var(--vault-line)', padding: 32, marginBottom: 32 } as const,
}

function maskKey(key: string) {
  if (key.length <= 14) return key
  return `${key.slice(0, 10)}...${key.slice(-4)}`
}

export function ApiKeysPage() {
  const app = useApp()
  const keys = useMockResource(() => mockApi.listApiKeys(), [])
  const [creating, setCreating] = useState(false)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [revealed, setRevealed] = useState<Record<string, boolean>>({})
  const selectedKey = useMemo(() => keys.data?.find((key) => key.id === selectedId) ?? keys.data?.[0] ?? null, [keys.data, selectedId])
  const sampleKey = selectedKey?.access_key ?? 'pk_live_xxx'
  const sampleCode = `curl https://api.picgallery.ai/v1/images/generations \\
  -H "Authorization: Bearer ${sampleKey}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "prompt": "A futuristic creative workspace with neon lights",
    "model": "plus",
    "n": 1,
    "size": "1024x1024"
  }'`

  async function toggle(key: ApiKey) {
    try {
      await mockApi.updateApiKey(key.id, { status: key.status === 'active' ? 'disabled' : 'active' })
      app.notify('success', key.status === 'active' ? '密钥已禁用' : '密钥已启用')
      await keys.reload()
    } catch (err) {
      app.notify('error', errorMessage(err))
    }
  }

  async function remove(key: ApiKey) {
    try {
      await mockApi.deleteApiKey(key.id)
      app.notify('success', '密钥已删除')
      if (selectedId === key.id) setSelectedId(null)
      await keys.reload()
    } catch (err) {
      app.notify('error', errorMessage(err))
    }
  }

  return (
    <div className="content" style={pageStyle.content}>
      <div className="header" style={pageStyle.header}>
        <div>
          <p className="eyebrow">DEVELOPER PORTAL</p>
          <h1 style={pageStyle.title}>API 密钥</h1>
        </div>
        <button className="btn btn-primary" type="button" onClick={() => setCreating(true)}>+ 创建新密钥</button>
      </div>

      <div className="card" style={pageStyle.card}>
        {keys.loading ? <LoadingState /> : null}
        {keys.error ? <ErrorState message={keys.error} onRetry={keys.reload} /> : null}
        {!keys.loading && !keys.data?.length ? <EmptyState title="暂无密钥" detail="创建一个密钥后即可调试开放 API。" action={<Button onClick={() => setCreating(true)}>创建密钥</Button>} /> : null}
        {keys.data?.length ? (
          <div className="table-container" style={{ overflowX: 'auto' }}>
            <table className="ds-table" style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14, minWidth: 960 }}>
              <thead>
                <tr>
                  {['名称', 'ACCESS KEY', 'SECRET', '状态', 'RPM 限制', '过期时间', '操作'].map((head) => <th key={head} style={thStyle}>{head}</th>)}
                </tr>
              </thead>
              <tbody>
                {keys.data.map((key) => (
                  <tr key={key.id} onClick={() => setSelectedId(key.id)} style={{ cursor: 'pointer', outline: selectedKey?.id === key.id ? '1px solid rgba(212,157,94,.45)' : undefined }}>
                    <td style={tdStyle}><strong>{key.name}</strong><div style={{ color: 'var(--vault-muted)', fontSize: 12 }}>{key.scopes.join(' · ')}</div></td>
                    <td style={tdStyle}><code className="num" style={inlineCode}>{revealed[key.id] ? key.access_key : maskKey(key.access_key)}</code></td>
                    <td style={tdStyle}><code className="num" style={inlineCode}>{revealed[key.id] ? (key.secret_preview ?? '仅创建时展示') : 'sk_••••••••••••'}</code></td>
                    <td style={tdStyle}><span className={`pill ${key.status === 'active' ? 'active' : ''}`} style={{ ...pillStyle, ...(key.status === 'active' ? { color: 'var(--vault-gold)', borderColor: 'rgba(212,157,94,.35)' } : {}) }}>{key.status === 'active' ? '启用中' : '已禁用'}</span></td>
                    <td className="num" style={tdStyle}>{key.rpm_limit}</td>
                    <td className="num" style={tdStyle}>{key.expires_at ?? '永不过期'}</td>
                    <td style={{ ...tdStyle, textAlign: 'right' }}>
                      <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, flexWrap: 'wrap' }}>
                        <CopyButton text={key.access_key} label="复制 AK" />
                        {key.secret_preview ? <CopyButton text={key.secret_preview} label="复制 SK" /> : null}
                        <button className="btn btn-ghost" type="button" onClick={(event) => { event.stopPropagation(); setRevealed((rows) => ({ ...rows, [key.id]: !rows[key.id] })) }}>{revealed[key.id] ? '隐藏' : '显示'}</button>
                        <button className="btn btn-ghost" type="button" onClick={(event) => { event.stopPropagation(); void toggle(key) }}>{key.status === 'active' ? '禁用' : '启用'}</button>
                        <button className="btn btn-ghost" type="button" style={{ color: 'oklch(70% 0.2 25)' }} onClick={(event) => { event.stopPropagation(); void remove(key) }}>删除</button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
      </div>

      <div className="stack" style={{ display: 'grid', gap: 24 }}>
        <h2 className="h2" style={{ fontSize: 32, margin: 0 }}>快速接入</h2>
        <div className="card" style={{ ...pageStyle.card, padding: 0, overflow: 'hidden' }}>
          <div style={{ background: 'var(--vault-bg)', padding: '12px 20px', borderBottom: '1px solid var(--vault-line)', fontSize: 13, fontFamily: 'JetBrains Mono, monospace', color: 'var(--vault-muted)', display: 'flex', justifyContent: 'space-between', gap: 12 }}>
            <span>Example Request (cURL)</span>
            <CopyButton text={sampleCode} label="copy" />
          </div>
          <div className="code-block" style={{ background: 'var(--vault-bg)', padding: 16, fontFamily: 'JetBrains Mono, monospace', fontSize: 13, color: 'var(--vault-muted)' }}>
            <pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{sampleCode}</pre>
          </div>
        </div>
        <p style={{ fontSize: 14, color: 'var(--vault-muted)' }}>查看完整 <a href="#/docs" style={{ color: 'var(--vault-gold)', textDecoration: 'underline' }}>开发文档</a> 获取更多语言示例。</p>
      </div>

      {creating ? <CreateKeyModal onClose={() => setCreating(false)} onCreated={async (key) => { await keys.reload(); setSelectedId(key.id); setRevealed((rows) => ({ ...rows, [key.id]: true })) }} /> : null}
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
  const [scopes, setScopes] = useState<string[]>(['images:write', 'images:read'])
  const [secret, setSecret] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  function toggleScope(scope: string) {
    setScopes((items) => items.includes(scope) ? items.filter((item) => item !== scope) : [...items, scope])
  }

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const key = await mockApi.createApiKey({ name, scopes, rpm_limit: rpm, expires_at: expiresAt || null })
      setSecret(key.secret_preview ?? null)
      app.notify('success', '密钥已创建，Secret 仅展示一次')
      await onCreated(key)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal title="创建 API 密钥" onClose={onClose}>
      <form className="modal-form" onSubmit={submit}>
        <Field label="密钥名称"><input value={name} onChange={(event) => setName(event.target.value)} minLength={3} required /></Field>
        <Field label="RPM 限制"><input type="number" min={1} max={600} value={rpm} onChange={(event) => setRpm(Number(event.target.value))} /></Field>
        <Field label="过期时间"><input type="date" value={expiresAt} onChange={(event) => setExpiresAt(event.target.value)} /></Field>
        <div className="scope-grid">
          {allScopes.map((scope) => <label key={scope}><input type="checkbox" checked={scopes.includes(scope)} onChange={() => toggleScope(scope)} />{scope}</label>)}
        </div>
        {secret ? <div className="secret-once"><strong>Secret Key</strong><code>{secret}</code><CopyButton text={secret} label="复制 Secret" /></div> : null}
        {error ? <div className="form-error">{error}</div> : null}
        <div className="action-row"><Button type="submit" busy={busy}>创建</Button><Button type="button" tone="ghost" onClick={onClose}>关闭</Button></div>
      </form>
    </Modal>
  )
}
