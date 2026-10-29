import { useEffect, useState } from 'react'
import { sharedApiClient } from '../../../shared/http-client'
import type { DeveloperEndpoints } from '../../../shared/api-types'
import { oauthProblem, type OAuthProblem } from './oauthProblem'
import { siteBrand } from '../brand'
import { useApp } from '../components'
import { Check, ShieldCheck } from '../ui/icons'
import { apiKeyScopeLabel } from './apiKeyRows'
import './oauth-consent.css'

export const oauthScopeDescriptions: Record<string, string> = {
  'account:read': '查看账号基本信息和可用余额。',
  'models:read': '查看可用模型及其生成能力。',
  'images:read': '查看图片任务的进度和生成结果。',
  'images:write': '提交图片生成任务，可能消耗账户积分。',
  'videos:read': '查看视频任务的进度和生成结果。',
  'videos:write': '提交视频生成任务，可能消耗账户积分。',
  'videos:delete': '取消正在进行的视频任务。',
  'assets:read': '查看素材信息并获取素材访问地址。',
  'assets:write': '修改素材信息及其所属项目或分组。',
  'assets:delete': '删除账户中的素材。',
  'uploads:read': '查看素材上传状态与已上传的分片。',
  'uploads:write': '创建上传任务并上传素材。',
  'uploads:delete': '终止尚未完成的上传任务。',
  'projects:read': '查看项目列表及项目信息。',
  'projects:write': '创建项目并修改项目信息。',
  'projects:delete': '删除项目。',
  'groups:read': '查看项目中的素材分组。',
  'groups:write': '创建分组并修改分组信息。',
  'groups:delete': '删除素材分组。',
}

function ScopePermission({ scope }: { scope: string }) {
  const description = oauthScopeDescriptions[scope]
  return <li className="oauth-consent__permission">
    <span className="oauth-consent__permission-icon" aria-hidden="true"><Check size={15} strokeWidth={1.7} /></span>
    <div><h3>{description ? apiKeyScopeLabel(scope) : '额外权限'}</h3><p>{description ?? '应用请求了此权限，请确认其用途后再授权。'}</p><code>{scope}</code></div>
  </li>
}

type Consent = { request_id: string; client: { client_id: string; client_name: string; redirect_uris: string[] }; resource: string; scopes: string[]; csrf_token: string }

export function OAuthConsentPage() {
  const app = useApp()
  const account = app.profile?.email || app.profile?.display_name || app.session?.profile.email || '当前已登录账号'
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
    if (!consent || busy) return
    setBusy(true); setError(null)
    try {
      const result = await sharedApiClient.post<{ redirect_uri: string }>('/api/v1/oauth/consent', { request_id: id, csrf_token: consent.csrf_token, allow })
      window.location.assign(result.redirect_uri)
    } catch (caught) { setError(oauthProblem(caught, 'consent')); setBusy(false) }
  }
  return <main className="oauth-consent">
    <section className="oauth-consent__card" aria-labelledby="oauth-title" aria-busy={loading || busy}>
      <header className="oauth-consent__brand"><img src={siteBrand.logoUrl} alt="" width={28} height={28} /><span>{siteBrand.name}</span><span className="oauth-consent__brand-caption">应用授权</span></header>
      <div className="oauth-consent__body" role="region" aria-label="应用身份与授权详情" tabIndex={0}>
        <div className="oauth-consent__heading"><span className="oauth-consent__seal" aria-hidden="true"><ShieldCheck size={24} strokeWidth={1.4} /></span><h1 id="oauth-title">应用请求访问</h1><p>请确认以下应用及其申请的权限。</p></div>
        {error && <div className="oauth-consent__state" role="alert"><h2>{error.kind === 'expired' ? '授权请求已失效' : error.kind === 'login' ? '请先登录' : '暂时无法继续授权'}</h2><p>{error.message}</p><div className="oauth-consent__state-actions">{error.retryable && <button type="button" className="u-btn" disabled={busy || loading} onClick={() => setRevision(value => value + 1)}>重试</button>}{error.kind === 'login' && <a className="u-btn u-btn--primary" href="#/login?returnTo=oauth-consent">重新登录</a>}</div></div>}
        {loading && !error && <div className="oauth-consent__state" role="status"><span className="oauth-consent__loader" aria-hidden="true" /><p>正在载入授权请求…</p></div>}
        {consent && !error && <>
          <div className="oauth-consent__application"><span className="oauth-consent__eyebrow">申请连接的外部应用</span><h2>{consent.client.client_name}</h2><details className="oauth-consent__application-name"><summary>查看完整应用名称</summary><p>{consent.client.client_name}</p></details><p>外部应用，名称由应用提供。</p></div>
          <div className="oauth-consent__account"><span>授权账号</span><strong>{account}</strong></div>
          <section className="oauth-consent__permissions" aria-labelledby="oauth-permissions-title"><div className="oauth-consent__section-heading"><h2 id="oauth-permissions-title">此应用将能够</h2><span>{consent.scopes.length} 项权限</span></div><div className="oauth-consent__permission-scroll" role="region" aria-label="申请的访问权限" tabIndex={0}><ul>{consent.scopes.map(scope => <ScopePermission key={scope} scope={scope} />)}</ul>{consent.scopes.length === 0 && <p className="oauth-consent__empty">此请求未申请额外权限。</p>}</div></section>
          <details className="oauth-consent__details"><summary>查看应用标识与授权资源</summary><dl><dt>应用标识</dt><dd>{consent.client.client_id}</dd><dt>授权资源</dt><dd>{consent.resource}</dd></dl></details>
        </>}
      </div>
      {consent && !error && <footer className="oauth-consent__footer"><p>允许后，应用可按上述权限访问你的账户。你可以随时在设置中断开连接。</p><div className="oauth-consent__actions"><button type="button" className="u-btn" disabled={busy} onClick={() => void decide(false)}>取消</button><button type="button" className="u-btn u-btn--primary" disabled={busy} onClick={() => void decide(true)}>{busy ? '正在处理…' : '允许访问'}</button></div></footer>}
    </section>
    <p className="oauth-consent__platform-note">Mikiko Studio · 仅向你信任的应用授权</p>
  </main>
}
