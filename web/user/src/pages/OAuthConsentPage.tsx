import { useEffect, useState } from 'react'
import { sharedApiClient } from '../../../shared/http-client'
import type { DeveloperEndpoints } from '../../../shared/api-types'
import { oauthProblem, type OAuthProblem } from './oauthProblem'

type Consent = { request_id: string; client: { client_id: string; client_name: string; redirect_uris: string[] }; resource: string; scopes: string[]; csrf_token: string }

export function OAuthConsentPage() {
  const id = new URLSearchParams(window.location.search).get('oauth_request') ?? ''
  const [consent, setConsent] = useState<Consent | null>(null)
  const [error, setError] = useState<OAuthProblem | null>(null)
  const [busy, setBusy] = useState(false)
  const [revision, setRevision] = useState(0)
  const [loading, setLoading] = useState(true)
  useEffect(() => {
    let active = true
    setLoading(true); setError(null); setConsent(null)
    void (async () => {
      const config = await sharedApiClient.get<DeveloperEndpoints>('/api/v1/public/developer-endpoints', { auth: false })
      if (!config.canonical_configured) {
        if (active) setError({ kind: 'unconfigured', message: '此部署尚未配置 OAuth 公共地址，请联系管理员配置后重试。', retryable: true })
        return
      }
      if (!id) { if (active) setError({ kind: 'expired', message: '缺少授权请求，请返回客户端重新连接。', retryable: false }); return }
      const value = await sharedApiClient.get<Consent>('/api/v1/oauth/consent', { query: { request_id: id } })
      if (active) setConsent(value)
    })().catch(caught => { if (active) setError(oauthProblem(caught, 'consent')) }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [id, revision])
  async function decide(allow: boolean) {
    if (!consent) return
    setBusy(true); setError(null)
    try {
      const result = await sharedApiClient.post<{ redirect_uri: string }>('/api/v1/oauth/consent', { request_id: id, csrf_token: consent.csrf_token, allow })
      window.location.assign(result.redirect_uri)
    } catch (caught) { setError(oauthProblem(caught, 'consent')); setBusy(false) }
  }
  return <main className="acct-page"><section className="u-plate u-plate--pad" style={{ maxWidth: 640, margin: '40px auto' }} aria-labelledby="oauth-title">
    <h1 id="oauth-title" className="u-h1">连接应用</h1>
    {error && <div role="alert"><p>{error.message}</p>{error.retryable && <button type="button" className="u-btn" disabled={busy || loading} onClick={() => setRevision(value => value + 1)}>重试</button>}{error.kind === 'login' && <a className="u-btn" href="#/login?returnTo=oauth-consent">重新登录</a>}</div>}
    {loading && !error && <p role="status">正在载入授权请求…</p>}
    {consent && !error && <><h2>{consent.client.client_name}</h2><p style={{ overflowWrap: 'anywhere' }}>{consent.client.client_id}</p><p>此应用请求访问你的 Studio 账户。允许后可随时在设置中断开连接。</p><p style={{ overflowWrap: 'anywhere' }}>资源：{consent.resource}</p><ul>{consent.scopes.map(scope => <li key={scope}>{scope}</li>)}</ul><div style={{ display: 'flex', gap: 12, marginTop: 24 }}><button type="button" className="u-btn" disabled={busy} onClick={() => void decide(false)}>拒绝</button><button type="button" className="u-btn u-btn--primary" disabled={busy} onClick={() => void decide(true)}>允许访问</button></div></>}
  </section></main>
}
