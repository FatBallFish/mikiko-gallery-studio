import { useEffect, useState } from 'react'
import { sharedApiClient } from '../../../shared/http-client'

type Grant = { id: string; client_id: string; client_name: string; scopes: string[]; created_at: number }
export function ConnectedAppsPanel() {
  const [items, setItems] = useState<Grant[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState('')
  useEffect(() => { let active = true; void sharedApiClient.get<{ items: Grant[] }>('/api/v1/oauth/connected-apps').then(value => { if (active) setItems(value.items) }).catch(() => { if (active) setError('无法载入已连接应用。') }).finally(() => { if (active) setLoading(false) }); return () => { active = false } }, [])
  async function disconnect(id: string) {
    setBusy(id); setError('')
    try { await sharedApiClient.delete('/api/v1/oauth/connected-apps', { query: { grant_id: id } }); setItems(current => current.filter(item => item.id !== id)) }
    catch { setError('断开连接失败，请重试。') } finally { setBusy('') }
  }
  return <section className="u-plate u-plate--pad" aria-labelledby="connected-apps-title"><div className="u-panel-title"><h3 id="connected-apps-title">已连接应用</h3><span>OAuth 授权</span></div><p className="u-meta">断开连接会立即撤销该授权的访问令牌与刷新令牌。</p>{loading && <p role="status">正在载入…</p>}{error && <p role="alert">{error}</p>}{!loading && !error && items.length === 0 && <p>尚未连接任何应用。</p>}{items.map(item => <article key={item.id} style={{ borderTop: '1px solid var(--border)', padding: '16px 0', overflowWrap: 'anywhere' }}><strong>{item.client_name}</strong><p className="u-meta">{item.client_id}</p><p>{item.scopes.join(' · ')}</p><button type="button" className="u-btn" disabled={busy !== ''} onClick={() => void disconnect(item.id)}>{busy === item.id ? '正在断开…' : '断开连接'}</button></article>)}</section>
}
