import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import type { OIDCConfigView, OIDCConfigWriteRequest } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, ErrorBlock, Field, InlineFeedback, LoadingBlock, StatusCell, StatusStrip } from '../components'
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

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
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

  return (
    <>
      {!summaryMode ? (
        <StatusStrip columns={4}>
          <StatusCell label="Mikiko ID" value={config?.enabled ? '启用' : '未启用'} />
          <StatusCell label="Client Secret" value={config?.secret_status?.has_secret ? '已配置' : '未配置'} />
          <StatusCell label="版本" value={String(config?.version ?? 0)} />
          <StatusCell label="本地登录" value={config?.allow_local_login !== false ? '开放' : '已关闭'} />
        </StatusStrip>
      ) : null}
      {error ? <InlineFeedback tone="danger" message={error} /> : null}
      {draft && config ? (
        <details className={summaryMode ? 'group rounded-lg border border-[var(--border)] bg-[var(--surface)] p-4' : ''} open={!summaryMode}>
          {summaryMode ? <summary className="cursor-pointer list-none text-sm font-bold text-[var(--accent)]">Mikiko ID 统一认证（OIDC）高级配置</summary> : null}
          <section className={summaryMode ? 'mt-4' : adminPage.fullSurface}>
            <section className={summaryMode ? 'grid gap-4' : adminPage.mainLane}>
              <form className={securityClasses.editor} aria-busy={locked} onSubmit={(event) => void save(event)}>
                <fieldset disabled={locked} className="contents">
                  <section className={securityClasses.editorGroup} data-security-editor-group="oidc-provider">
                    <div className={securityClasses.sectionHead}>
                      <div>
                        <strong>Mikiko ID 统一认证（OIDC）</strong>
                        <p className={securityClasses.note}>对接 Mikiko AI 统一身份（Keycloak realm mikiko）。启用后登录页出现「使用 Mikiko ID 登录」入口，并支持门户静默免登。</p>
                      </div>
                      <Badge tone={config.enabled ? 'success' : 'neutral'}>{config.enabled ? '已启用' : '未启用'}</Badge>
                    </div>
                    <label className={securityClasses.toggle}>
                      <input type="checkbox" checked={draft.enabled} onChange={(event) => updateDraft({ ...draft, enabled: event.target.checked })} />
                      <span>启用 Mikiko ID 登录</span>
                    </label>
                    <div className={adminPage.formGrid}>
                      <Field label="Issuer URL" error={fieldErrors.issuer_url}>
                        <input value={draft.issuer_url} onChange={(event) => updateDraft({ ...draft, issuer_url: event.target.value })} placeholder="https://oauth.mikiko.ai/realms/mikiko" />
                      </Field>
                      <Field label="Client ID" error={fieldErrors.client_id}>
                        <input value={draft.client_id} onChange={(event) => updateDraft({ ...draft, client_id: event.target.value })} placeholder="mikiko_studio" />
                      </Field>
                      <Field label="按钮显示名" hint="留空使用默认「使用 Mikiko ID 登录」。">
                        <input value={draft.display_name} onChange={(event) => updateDraft({ ...draft, display_name: event.target.value })} placeholder="使用 Mikiko ID 登录" />
                      </Field>
                      <Field label="回调地址（只读）" hint="在 Keycloak client 的有效重定向 URI 中精确登记该地址。">
                        <input value={callbackURL} readOnly disabled />
                      </Field>
                    </div>
                  </section>

                  <section className={securityClasses.editorGroup} data-security-editor-group="oidc-login-policy">
                    <div className={securityClasses.sectionHead}>
                      <div>
                        <strong>登录方式开关</strong>
                        <p className={securityClasses.note}>关闭平台账密登录后，密码与邮箱验证码登录一并停用；OIDC 首次登录始终自动注册。</p>
                      </div>
                    </div>
                    <label className={securityClasses.toggle}>
                      <input type="checkbox" checked={draft.allow_local_login} onChange={(event) => updateDraft({ ...draft, allow_local_login: event.target.checked })} />
                      <span>允许平台账密登录（密码 + 邮箱验证码）</span>
                    </label>
                    <label className={securityClasses.toggle}>
                      <input type="checkbox" checked={draft.allow_registration} onChange={(event) => updateDraft({ ...draft, allow_registration: event.target.checked })} />
                      <span>允许通过邮箱验证码注册新账号</span>
                    </label>
                  </section>

                  <section className={securityClasses.editorGroup} data-security-editor-group="oidc-secret">
                    <div className={securityClasses.sectionHead}>
                      <div>
                        <strong>Client Secret</strong>
                        <p className={securityClasses.note}>{oidcSecretSummary(config)}。机密客户端密钥只写不读，后端加密存储。</p>
                      </div>
                    </div>
                    <div className={adminPage.formGrid}>
                      <Field label="新 Client Secret" hint="留空表示保留已保存密钥；不要填写星号占位符。">
                        <input
                          value={draft.client_secret}
                          onChange={(event) => updateDraft({ ...draft, client_secret: event.target.value, clear_client_secret: false })}
                          type="password"
                          autoComplete="new-password"
                          placeholder={config.secret_status?.has_secret ? '留空保留当前密钥' : '填写 Keycloak client secret'}
                        />
                      </Field>
                      <label className={securityClasses.toggle}>
                        <input
                          type="checkbox"
                          checked={draft.clear_client_secret}
                          onChange={(event) => updateDraft({ ...draft, clear_client_secret: event.target.checked, client_secret: event.target.checked ? '' : draft.client_secret })}
                        />
                        <span>清空已保存密钥</span>
                      </label>
                    </div>
                  </section>
                </fieldset>

                <footer className={securityClasses.saveRail}>
                  <div className={securityClasses.saveRailStatus}>
                    <Badge tone={editorPhaseTone(phase)}>{editorPhaseLabel(phase)}</Badge>
                    <span className={securityClasses.note}>{editorPhaseDetail(phase)}</span>
                    {saveError ? <InlineFeedback tone="danger" message={saveError} /> : null}
                  </div>
                  <div className={securityClasses.actions}>
                    <button type="submit" className={cn(adminButton.base, adminButton.primary)} disabled={locked || phase === 'validating' || !dirty}>{saving ? '保存中...' : '保存 Mikiko ID 配置'}</button>
                  </div>
                </footer>
              </form>
              <section className={securityClasses.section}>
                <div className={securityClasses.sectionHead}>
                  <div>
                    <strong>连通性测试</strong>
                    <p className={securityClasses.note}>拉取已保存配置的 discovery 文档，验证 Issuer 可达性与端点信息。</p>
                  </div>
                </div>
                <div className={securityClasses.actions}>
                  <button type="button" className={cn(adminButton.base, adminButton.ghost)} disabled={locked} onClick={() => void testConnection()}>{testing ? '测试中...' : '测试 Discovery 连通性'}</button>
                </div>
                {testResult ? <InlineFeedback tone="success" message={testResult} /> : null}
                {testError ? <InlineFeedback tone="danger" message={testError} /> : null}
              </section>
            </section>
          </section>
        </details>
      ) : null}
    </>
  )
}
