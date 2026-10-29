import { useEffect, useState } from 'react'
import { sharedApiClient } from '../../../shared/http-client'
import type { DeveloperEndpoints } from '../../../shared/api-types'
import { oauthProblem, type OAuthProblem } from './oauthProblem'

type Grant = { id: string; client_id: string; client_name: string; scopes: string[]; created_at: number }
export function ConnectedAppsPanel() {
  const [items, setItems] = useState<Grant[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<OAuthProblem | null>(null)
  const [busy, setBusy] = useState('')
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    let active = true
    setLoading(true); setError(null)
    void (async () => {
      const config = await sharedApiClient.get<DeveloperEndpoints>('/api/v1/public/developer-endpoints', { auth: false })
      if (!config.canonical_configured) {
        if (active) setError({ kind: 'unconfigured', message: '此部署尚未配置 OAuth 公共地址，已连接应用暂不可用。请联系管理员配置后重试。', retryable: true })
        return
      }
      const value = await sharedApiClient.get<{ items: Grant[] }>('/api/v1/oauth/connected-apps')
      if (active) setItems(value.items)
    })().catch(caught => { if (active) setError(oauthProblem(caught, 'apps')) }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [revision])
  async function disconnect(id: string) {
    setBusy(id); setError(null)
    try { await sharedApiClient.delete('/api/v1/oauth/connected-apps', { query: { grant_id: id } }); setItems(current => current.filter(item => item.id !== id)) }
    catch (caught) { setError(oauthProblem(caught, 'apps')) } finally { setBusy('') }
  }
  return <section className="u-plate u-plate--pad" aria-labelledby="connected-apps-title">
    <div className="u-panel-title"><h3 id="connected-apps-title">已连接应用</h3><span>OAuth 授权</span></div>
    <p className="u-meta">断开连接会立即撤销该授权的访问令牌与刷新令牌。</p>
    {loading && <p role="status">正在载入…</p>}
    {error && <div role="alert"><p>{error.message}</p>{error.retryable && <button type="button" className="u-btn" disabled={loading || !!busy} onClick={() => setRevision(value => value + 1)}>重试</button>}{error.kind === 'login' && <a className="u-btn" href="#/login?returnTo=settings">重新登录</a>}</div>}
    {!loading && !error && items.length === 0 && <p>尚未连接任何应用。</p>}
    {!loading && items.map(item => <article key={item.id} style={{ borderTop: '1px solid var(--border)', padding: '16px 0', overflowWrap: 'anywhere' }}><strong>{item.client_name}</strong><p className="u-meta">{item.client_id}</p><p>{item.scopes.join(' · ')}</p><button type="button" className="u-btn" disabled={busy !== ''} onClick={() => void disconnect(item.id)}>{busy === item.id ? '正在断开…' : '断开连接'}</button></article>)}
  </section>
}
