import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import type { OIDCConfigView, OIDCConfigWriteRequest } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, Button, ErrorBlock, InlineFeedback, LoadingBlock } from '../components'
import { FormSaveBar, FormSection, LedgerField, SwitchField } from './AdminMediaTabs'
import { adminButton, adminPage } from '../ui/classes'
import { editorPhaseDetail, editorPhaseLabel, editorPhaseTone, securityClasses, type EditorPhase } from './SecurityConfigPage'

type OIDCDraft = {
  enabled: boolean
  issuer_url: string
  client_id: string
  display_name: string
  allow_local_login: boolean
  allow_registration: boolean
  client_secret: string
  clear_client_secret: boolean
}

type OIDCFieldErrors = Partial<Record<'issuer_url' | 'client_id', string>>

function oidcDraftFromConfig(config: OIDCConfigView): OIDCDraft {
  return {
    enabled: Boolean(config.enabled),
    issuer_url: config.issuer_url ?? '',
    client_id: config.client_id ?? '',
    display_name: config.display_name ?? '',
    allow_local_login: config.allow_local_login !== false,
    allow_registration: config.allow_registration !== false,
    client_secret: '',
    clear_client_secret: false,
  }
}

function oidcDraftIsDirty(draft: OIDCDraft, config: OIDCConfigView) {
  const baseline = oidcDraftFromConfig(config)
  return draft.enabled !== baseline.enabled
    || draft.issuer_url !== baseline.issuer_url
    || draft.client_id !== baseline.client_id
    || draft.display_name !== baseline.display_name
    || draft.allow_local_login !== baseline.allow_local_login
    || draft.allow_registration !== baseline.allow_registration
    || Boolean(draft.client_secret.trim())
    || draft.clear_client_secret
}

function validateOIDCDraft(draft: OIDCDraft, config: OIDCConfigView): OIDCFieldErrors {
  const errors: OIDCFieldErrors = {}
  if (!draft.enabled) return errors
  if (!draft.issuer_url.trim()) errors.issuer_url = '启用 OIDC 时必须填写 Issuer 地址。'
  if (!draft.client_id.trim()) errors.client_id = '启用 OIDC 时必须填写 Client ID。'
  if (!draft.issuer_url.trim() || !draft.client_id.trim()) return errors
  if (!config.secret_status?.has_secret && !draft.client_secret.trim() && !draft.clear_client_secret) errors.client_id = '启用 OIDC 时必须先配置 Client Secret。'
  return errors
}

function oidcPayloadFromDraft(draft: OIDCDraft, config: OIDCConfigView): OIDCConfigWriteRequest {
  const payload: OIDCConfigWriteRequest = {
    version: config.version,
    enabled: draft.enabled,
    issuer_url: draft.issuer_url.trim(),
    client_id: draft.client_id.trim(),
    display_name: draft.display_name.trim(),
    allow_local_login: draft.allow_local_login,
    allow_registration: draft.allow_registration,
  }
  const secret = draft.client_secret.trim()
  if (secret) payload.secrets = { client_secret: secret }
  if (draft.clear_client_secret) payload.clear_secrets = ['client_secret']
  return payload
}

function oidcSecretSummary(config: OIDCConfigView) {
  if (!config.secret_status?.has_secret) return '尚未配置 Client Secret'
  const fingerprint = config.secret_status.fingerprint ? `，指纹 ${config.secret_status.fingerprint}` : ''
  return `已配置 Client Secret${fingerprint}`
}

export function OIDCConfigEditor({ onFeedback, onDirtyChange, onBusyChange, summaryMode = false }: { onFeedback?: (title: string, detail?: string) => void; onDirtyChange?: (dirty: boolean) => void; onBusyChange?: (busy: boolean) => void; summaryMode?: boolean }) {
  const [config, setConfig] = useState<OIDCConfigView | null>(null)
  const [draft, setDraft] = useState<OIDCDraft | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saveError, setSaveError] = useState('')
  const [testResult, setTestResult] = useState('')
  const [testError, setTestError] = useState('')
  const [fieldErrors, setFieldErrors] = useState<OIDCFieldErrors>({})
  const [phase, setPhase] = useState<EditorPhase>('pristine')

  const dirty = Boolean(draft && config && oidcDraftIsDirty(draft, config))
  const locked = saving || testing

  async function load() {
    setLoading(true)
    setError(null)
    try {
      const next = await adminApi.getOIDCConfig()
      setConfig(next)
      setDraft(oidcDraftFromConfig(next))
      setPhase('pristine')
      setSaveError('')
      setTestError('')
      setTestResult('')
      setFieldErrors({})
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'OIDC 配置读取失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [])

  useEffect(() => {
    if (phase === 'saving' || phase === 'validating' || phase === 'failed') return
    if (dirty && phase !== 'dirty') setPhase('dirty')
    if (!dirty && phase === 'dirty') setPhase('pristine')
  }, [dirty, phase])

  useEffect(() => { onDirtyChange?.(dirty) }, [dirty, onDirtyChange])
  useEffect(() => () => onDirtyChange?.(false), [onDirtyChange])
  useEffect(() => { onBusyChange?.(locked) }, [locked, onBusyChange])
  useEffect(() => () => onBusyChange?.(false), [onBusyChange])

  function updateDraft(next: OIDCDraft) {
    setDraft(next)
    setSaveError('')
    setFieldErrors({})
    if (phase === 'failed' || phase === 'saved') setPhase('dirty')
  }

  async function save(event?: FormEvent<HTMLFormElement>) {
    event?.preventDefault()
    if (!draft || !config) return
    setPhase('validating')
    setSaveError('')
    if (!draft.allow_local_login && !draft.enabled) {
      setSaveError('未启用 OIDC 时不能关闭平台账密登录，否则将没有任何可用的登录方式。')
      setPhase('failed')
      return
    }
    const nextFieldErrors = validateOIDCDraft(draft, config)
    setFieldErrors(nextFieldErrors)
    if (Object.keys(nextFieldErrors).length) {
      setSaveError('请修正标记字段后再保存。')
      setPhase('failed')
      return
    }
    setSaving(true)
    setPhase('saving')
    try {
      const updated = await adminApi.updateOIDCConfig(oidcPayloadFromDraft(draft, config))
      setConfig(updated)
      setDraft(oidcDraftFromConfig(updated))
      setPhase('saved')
      onFeedback?.('Mikiko ID 配置已保存', oidcSecretSummary(updated))
    } catch (caught) {
      setSaveError(caught instanceof Error ? caught.message : 'OIDC 配置保存失败')
      setPhase('failed')
    } finally {
      setSaving(false)
    }
  }

  async function testConnection() {
    setTesting(true)
    setTestError('')
    setTestResult('')
    try {
      const result = await adminApi.testOIDCConfig()
      setTestResult(`连通正常：${result.issuer}（token ${result.token_endpoint}）`)
    } catch (caught) {
      setTestError(caught instanceof Error ? caught.message : 'OIDC 连通性测试失败')
    } finally {
      setTesting(false)
    }
  }

  if (loading) return <LoadingBlock label="读取 Mikiko ID 配置" />
  if (error && !config) return <ErrorBlock message={error} onRetry={load} />

  const callbackURL = `${window.location.origin}/auth/callback`

  const liveFieldErrors = draft && config ? validateOIDCDraft(draft, config) : {}
  const liveErrorCount = Object.keys(liveFieldErrors).length

  return (
    <section className="a-panel a-panel--flush">
      <div className="a-panel__head">
        <div><h2>OIDC 统一身份</h2><p>Mikiko ID 单点登录；机密客户端密钥只写不读。</p></div>
        <div className="a-row-actions">
          <Badge tone={config?.enabled ? 'success' : 'neutral'}>{config?.enabled ? '已启用' : '未启用'}</Badge>
          <Badge tone={config?.secret_status?.has_secret ? 'success' : 'warning'}>{config?.secret_status?.has_secret ? 'Secret 已配置' : 'Secret 未配置'}</Badge>
          <Badge tone={editorPhaseTone(phase)}>{editorPhaseLabel(phase)}</Badge>
        </div>
      </div>
      {error ? <div style={{ padding: '12px 20px' }}><InlineFeedback tone="danger" message={error} /></div> : null}
      {draft && config ? (
        <form className="a-form" aria-busy={locked} onSubmit={(event) => void save(event)}>
          <FormSection index={1} title="身份提供方" group="oidc-provider" description="启用后登录页出现「使用 Mikiko ID 登录」入口，并支持门户静默免登。">
            <SwitchField label="启用 Mikiko ID 登录" checked={draft.enabled} onChange={(enabled) => updateDraft({ ...draft, enabled })} disabled={locked} />
            <LedgerField label="Issuer URL" error={liveFieldErrors.issuer_url}><input className="a-input" value={draft.issuer_url} onChange={(event) => updateDraft({ ...draft, issuer_url: event.target.value })} placeholder="https://oauth.mikiko.ai/realms/mikiko" style={{ fontFamily: 'var(--font-admin-mono)', fontSize: 12.5 }} /></LedgerField>
            <LedgerField label="Client ID" error={liveFieldErrors.client_id}><input className="a-input" value={draft.client_id} onChange={(event) => updateDraft({ ...draft, client_id: event.target.value })} placeholder="mikiko_studio" style={{ fontFamily: 'var(--font-admin-mono)', fontSize: 12.5 }} /></LedgerField>
            <LedgerField label="按钮显示名" hint="留空使用默认「使用 Mikiko ID 登录」。"><input className="a-input" value={draft.display_name} onChange={(event) => updateDraft({ ...draft, display_name: event.target.value })} placeholder="使用 Mikiko ID 登录" /></LedgerField>
            <LedgerField label="回调地址（只读）" hint="在 Keycloak client 的有效重定向 URI 中精确登记该地址。"><input className="a-input" value={callbackURL} readOnly disabled /></LedgerField>
          </FormSection>

          <FormSection index={2} title="登录方式开关" group="oidc-login-policy" description="关闭平台账密登录后，密码与邮箱验证码登录一并停用；OIDC 首次登录始终自动注册。">
            <SwitchField label="允许平台账密登录（密码 + 邮箱验证码）" checked={draft.allow_local_login} onChange={(allow_local_login) => updateDraft({ ...draft, allow_local_login })} disabled={locked} />
            <SwitchField label="允许通过邮箱验证码注册新账号" checked={draft.allow_registration} onChange={(allow_registration) => updateDraft({ ...draft, allow_registration })} disabled={locked} />
          </FormSection>

          <FormSection index={3} title="Client Secret" group="oidc-secret" description={`${oidcSecretSummary(config)}。机密客户端密钥只写不读，后端加密存储。`}>
            <LedgerField label="新 Client Secret" hint="留空表示保留已保存密钥；不要填写星号占位符。"><input className="a-input" value={draft.client_secret} onChange={(event) => updateDraft({ ...draft, client_secret: event.target.value, clear_client_secret: false })} type="password" autoComplete="new-password" placeholder={config.secret_status?.has_secret ? '留空保留当前密钥' : '填写 Keycloak client secret'} /></LedgerField>
            <SwitchField label="清空已保存密钥" checked={draft.clear_client_secret} onChange={(clear_client_secret) => updateDraft({ ...draft, clear_client_secret, client_secret: clear_client_secret ? '' : draft.client_secret })} disabled={locked} />
          </FormSection>

          <FormSection index={4} title="连通性测试" description="拉取已保存配置的 discovery 文档，验证 Issuer 可达性与端点信息。">
            <div className="a-row-actions">
              <Button variant="secondary" size="sm" disabled={locked} onClick={() => void testConnection()}>{testing ? '测试中...' : '测试 Discovery 连通性'}</Button>
            </div>
            {testResult ? <InlineFeedback tone="success" message={testResult} /> : null}
            {testError ? <InlineFeedback tone="danger" message={testError} /> : null}
          </FormSection>
        </form>
      ) : null}
      {draft ? (
        <FormSaveBar
          dirtyCount={dirty ? 1 : 0}
          errorCount={liveErrorCount}
          busy={saving || testing}
          note={saveError || '保存后新的登录入口与登录策略立即生效。'}
          onReset={() => { if (config) updateDraft(oidcDraftFromConfig(config)) }}
          onSave={() => void save()}
          saveLabel="保存 Mikiko ID 配置"
          resetLabel="恢复"
          saveDisabled={!dirty || !config}
        />
      ) : null}
    </section>
  )
}
