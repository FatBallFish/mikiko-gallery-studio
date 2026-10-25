import { useEffect, useState } from 'react'
import { sharedApiClient } from '../../../shared/http-client'

type Consent = { request_id: string; client: { client_id: string; client_name: string; redirect_uris: string[] }; resource: string; scopes: string[]; csrf_token: string }

export function OAuthConsentPage() {
  const id = new URLSearchParams(window.location.search).get('oauth_request') ?? ''
  const [consent, setConsent] = useState<Consent | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => { let active = true; void sharedApiClient.get<Consent>('/api/v1/oauth/consent', { query: { request_id: id } }).then(value => { if (active) setConsent(value) }).catch(() => { if (active) setError('授权请求已失效，请返回应用重新连接。') }); return () => { active = false } }, [id])
  async function decide(allow: boolean) {
    if (!consent) return
    setBusy(true); setError('')
    try {
      const result = await sharedApiClient.post<{ redirect_uri: string }>('/api/v1/oauth/consent', { request_id: id, csrf_token: consent.csrf_token, allow })
      window.location.assign(result.redirect_uri)
    } catch { setError('无法提交授权，请返回应用重新连接。'); setBusy(false) }
  }
  return <main className="acct-page"><section className="u-plate u-plate--pad" style={{ maxWidth: 640, margin: '40px auto' }} aria-labelledby="oauth-title">
    <h1 id="oauth-title" className="u-h1">连接应用</h1>
    {error && <p role="alert">{error}</p>}
    {!consent && !error && <p role="status">正在载入授权请求…</p>}
    {consent && <><h2>{consent.client.client_name}</h2><p style={{ overflowWrap: 'anywhere' }}>{consent.client.client_id}</p><p>此应用请求访问你的 Studio 账户。允许后可随时在设置中断开连接。</p><p style={{ overflowWrap: 'anywhere' }}>资源：{consent.resource}</p><ul>{consent.scopes.map(scope => <li key={scope}>{scope}</li>)}</ul><div style={{ display: 'flex', gap: 12, marginTop: 24 }}><button type="button" className="u-btn" disabled={busy} onClick={() => void decide(false)}>拒绝</button><button type="button" className="u-btn u-btn--primary" disabled={busy} onClick={() => void decide(true)}>允许访问</button></div></>}
  </section></main>
}
