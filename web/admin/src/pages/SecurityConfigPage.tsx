import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import type { SMTPConfigView, SMTPConfigWriteRequest } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, ErrorBlock, Field, InlineFeedback, LoadingBlock, PageHeader, StatusCell, StatusStrip } from '../components'
import { adminButton, adminPage } from '../ui/classes'

type SMTPDraft = {
  enabled: boolean
  host: string
  port: string
  username: string
  from: string
  starttls: boolean
  insecure_skip_verify: boolean
  password: string
  clear_password: boolean
  test_email: string
}

type EditorPhase = 'pristine' | 'dirty' | 'validating' | 'saving' | 'saved' | 'failed'
type SMTPFieldErrors = Partial<Record<'host' | 'port' | 'from', string>>

const securityClasses = {
  summaryList: 'grid gap-2',
  summaryToggle: 'flex items-center justify-between gap-4 rounded-lg bg-[var(--surface-solid)] p-3 transition-colors hover:bg-[var(--elevated)]',
  summaryTitle: 'text-sm font-semibold text-[var(--text)]',
  summaryDetail: 'mt-1 text-xs text-[var(--muted-strong)]',
  switch: 'relative h-6 w-12 rounded-full bg-[var(--accent)]',
  switchOff: 'bg-[var(--surface-solid)]',
  knob: 'absolute top-1 size-4 rounded-full bg-white shadow-sm',
  editor: 'grid gap-4',
  editorGroup: 'grid gap-4 rounded-lg bg-[var(--surface-solid)] p-4',
  section: 'grid gap-4 rounded-lg bg-[var(--surface-solid)] p-4',
  sectionHead: 'flex flex-wrap items-center justify-between gap-2 border-b border-[var(--line)] pb-3',
  form: 'grid gap-4',
  toggle: 'grid grid-cols-[auto_minmax(0,1fr)] items-center gap-2 rounded-lg bg-[var(--surface-solid)] p-2 text-sm has-[:checked]:bg-[var(--accent)]/10',
  actions: 'flex flex-wrap items-center justify-end gap-2',
  note: 'm-0 text-sm text-[var(--soft)] [overflow-wrap:anywhere]',
  secretBox: 'grid gap-3 rounded-lg bg-[var(--surface-solid)] p-4',
  testRow: 'grid grid-cols-[minmax(220px,1fr)_auto] items-end gap-3 max-[620px]:grid-cols-1',
  saveRail: 'sticky bottom-0 z-10 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] p-3 shadow-[0_-8px_24px_rgba(0,0,0,.08)]',
  saveRailStatus: 'flex min-w-0 flex-1 flex-wrap items-center gap-2',
}

export function SecurityConfigPage({ onFeedback, onDirtyChange, onBusyChange, compact = false, summaryMode = false }: { onFeedback?: (title: string, detail?: string) => void; onDirtyChange?: (dirty: boolean) => void; onBusyChange?: (busy: boolean) => void; compact?: boolean; summaryMode?: boolean }) {
  const [config, setConfig] = useState<SMTPConfigView | null>(null)
  const [draft, setDraft] = useState<SMTPDraft | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saveError, setSaveError] = useState('')
  const [testError, setTestError] = useState('')
  const [fieldErrors, setFieldErrors] = useState<SMTPFieldErrors>({})
  const [editorPhase, setEditorPhase] = useState<EditorPhase>('pristine')

  const dirty = useMemo(() => Boolean(draft && config && smtpDraftIsDirty(draft, config)), [config, draft])
  const editorLocked = saving || testing

  async function load() {
    setLoading(true)
    setError(null)
    try {
      const next = await adminApi.getSMTPConfig()
      setConfig(next)
      setDraft(smtpDraftFromConfig(next))
      setEditorPhase('pristine')
      setSaveError('')
      setTestError('')
      setFieldErrors({})
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'SMTP 配置读取失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [])

  useEffect(() => {
    if (editorPhase === 'saving' || editorPhase === 'validating' || editorPhase === 'failed') return
    if (dirty && editorPhase !== 'dirty') setEditorPhase('dirty')
    if (!dirty && editorPhase === 'dirty') setEditorPhase('pristine')
  }, [dirty, editorPhase])

  useEffect(() => {
    onDirtyChange?.(dirty)
  }, [dirty, onDirtyChange])

  useEffect(() => () => onDirtyChange?.(false), [onDirtyChange])

  useEffect(() => {
    onBusyChange?.(editorLocked)
  }, [editorLocked, onBusyChange])

  useEffect(() => () => onBusyChange?.(false), [onBusyChange])

  useEffect(() => {
    if (!dirty) return undefined
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', handleBeforeUnload)
    return () => window.removeEventListener('beforeunload', handleBeforeUnload)
  }, [dirty])

  function reloadWithConfirmation() {
    if (dirty && !window.confirm('当前 SMTP 配置有未保存修改，确认放弃并刷新吗？')) return
    void load()
  }

  function updateConfigDraft(next: SMTPDraft) {
    setDraft(next)
    setSaveError('')
    setFieldErrors({})
    if (editorPhase === 'failed' || editorPhase === 'saved') setEditorPhase('dirty')
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!draft || !config) return
    setEditorPhase('validating')
    setSaveError('')
    const nextFieldErrors = validateSMTPDraft(draft)
    setFieldErrors(nextFieldErrors)
    if (Object.keys(nextFieldErrors).length) {
      setSaveError('请修正标记字段后再保存。')
      setEditorPhase('failed')
      return
    }
    setSaving(true)
    setEditorPhase('saving')
    try {
      const payload = smtpPayloadFromDraft(draft, config)
      const updated = await adminApi.updateSMTPConfig(payload)
      setConfig(updated)
      setDraft(smtpDraftFromConfig(updated, draft.test_email))
      setEditorPhase('saved')
      onFeedback?.('SMTP 配置已保存', smtpSecretSummary(updated))
    } catch (caught) {
      setSaveError(caught instanceof Error ? caught.message : 'SMTP 配置保存失败')
      setEditorPhase('failed')
    } finally {
      setSaving(false)
    }
  }

  async function testSMTP() {
    if (!draft?.test_email.trim()) {
      setTestError('请先填写测试收件邮箱')
      return
    }
    setTesting(true)
    setTestError('')
    try {
      const result = await adminApi.testSMTPConfig(draft.test_email.trim(), 'smtp_test')
      onFeedback?.('测试邮件已发送', result.recipient)
    } catch (caught) {
      setTestError(caught instanceof Error ? caught.message : '测试邮件发送失败')
    } finally {
      setTesting(false)
    }
  }

  if (loading) return <LoadingBlock label="读取安全配置" />
  if (error && !config) return <ErrorBlock message={error} onRetry={load} />

  return (
    <section className={adminPage.stack}>
      {!compact ? (
        <PageHeader
          title="安全配置"
          detail="管理 SMTP 发信服务器等敏感配置；密钥只写不读，后端加密存储。"
          actions={<button type="button" className={adminButton.base} disabled={editorLocked} onClick={reloadWithConfirmation}>刷新</button>}
        />
      ) : null}
      {!summaryMode ? (
        <StatusStrip columns={4}>
          <StatusCell label="SMTP" value={config?.enabled ? '启用' : '未启用'} />
          <StatusCell label="密码状态" value={config?.secret_status?.has_secret ? '已配置' : '未配置'} />
          <StatusCell label="版本" value={String(config?.version ?? 0)} />
          <StatusCell label="指纹" value={config?.secret_status?.fingerprint ?? '-'} />
        </StatusStrip>
      ) : null}
      {error ? <InlineFeedback tone="danger" message={error} /> : null}
      {draft && config ? (
        <>
          {summaryMode ? (
            <section className={securityClasses.summaryList}>
              <SecuritySummaryToggle title="SMTP 发信服务" detail={smtpSecretSummary(config)} enabled={config.enabled} />
              <SecuritySummaryToggle title="强制邮箱验证" detail="登录、注册、密码重置邮件走真实配置中心。" enabled />
              <SecuritySummaryToggle title="启用全站内容审核 (Moderation)" detail="内容策略由系统配置类目控制。" enabled />
              <SecuritySummaryToggle title="限制单用户日最大生图数" detail="当前限制来自生成配置类目。" enabled />
            </section>
          ) : null}
          <details className={summaryMode ? 'group rounded-lg border border-[var(--border)] bg-[var(--surface)] p-4' : ''} open={!summaryMode}>
            {summaryMode ? <summary className="cursor-pointer list-none text-sm font-bold text-[var(--accent)]">SMTP 高级配置</summary> : null}
            <section className={summaryMode ? 'mt-4' : adminPage.fullSurface}>
              <section className={summaryMode ? 'grid gap-4' : adminPage.mainLane}>
                <form className={securityClasses.editor} aria-busy={editorLocked} onSubmit={(event) => void save(event)}>
                  <fieldset disabled={editorLocked} className="contents">
                  <section className={securityClasses.editorGroup} data-security-editor-group="connection">
                    <div className={securityClasses.sectionHead}>
                      <div>
                        <strong>连接与启用状态</strong>
                        <p className={securityClasses.note}>配置 SMTP 服务器地址和端口；启用时必须通过基础字段校验。</p>
                      </div>
                      <Badge tone={config.enabled ? 'success' : 'neutral'}>{config.enabled ? '已启用' : '未启用'}</Badge>
                    </div>
                    <label className={securityClasses.toggle}>
                      <input type="checkbox" checked={draft.enabled} onChange={(event) => updateConfigDraft({ ...draft, enabled: event.target.checked })} />
                      <span>启用 SMTP 发信</span>
                    </label>
                    <div className={adminPage.formGrid}>
                      <Field label="SMTP Host" error={fieldErrors.host}>
                        <input value={draft.host} onChange={(event) => updateConfigDraft({ ...draft, host: event.target.value })} placeholder="smtp.example.com" />
                      </Field>
                      <Field label="SMTP Port" error={fieldErrors.port}>
                        <input value={draft.port} onChange={(event) => updateConfigDraft({ ...draft, port: event.target.value })} inputMode="numeric" placeholder="587" />
                      </Field>
                    </div>
                  </section>

                  <section className={securityClasses.editorGroup} data-security-editor-group="delivery">
                    <div className={securityClasses.sectionHead}>
                      <div>
                        <strong>发件身份与传输安全</strong>
                        <p className={securityClasses.note}>用于登录、注册、密码重置和测试邮件发送。</p>
                      </div>
                    </div>
                    <div className={adminPage.formGrid}>
                      <Field label="Username">
                        <input value={draft.username} onChange={(event) => updateConfigDraft({ ...draft, username: event.target.value })} placeholder="mailer@example.com" />
                      </Field>
                      <Field label="From" error={fieldErrors.from}>
                        <input value={draft.from} onChange={(event) => updateConfigDraft({ ...draft, from: event.target.value })} placeholder="Pic Gallery <noreply@example.com>" />
                      </Field>
                      <label className={securityClasses.toggle}>
                        <input type="checkbox" checked={draft.starttls} onChange={(event) => updateConfigDraft({ ...draft, starttls: event.target.checked })} />
                        <span>启用 STARTTLS</span>
                      </label>
                      <label className={securityClasses.toggle}>
                        <input type="checkbox" checked={draft.insecure_skip_verify} onChange={(event) => updateConfigDraft({ ...draft, insecure_skip_verify: event.target.checked })} />
                        <span>跳过 TLS 证书校验</span>
                      </label>
                    </div>
                  </section>

                  <section className={securityClasses.editorGroup} data-security-editor-group="secret">
                    <div className={securityClasses.sectionHead}>
                      <div>
                        <strong>SMTP 密码</strong>
                        <p className={securityClasses.note}>{smtpSecretSummary(config)}。密码保存后不会明文回显。</p>
                      </div>
                    </div>
                    <div className={adminPage.formGrid}>
                      <Field label="新密码" hint="留空表示保留已保存密码；不要填写星号占位符。">
                        <input
                          value={draft.password}
                          onChange={(event) => updateConfigDraft({ ...draft, password: event.target.value, clear_password: false })}
                          type="password"
                          autoComplete="new-password"
                          placeholder={config.secret_status?.has_secret ? '留空保留当前密码' : '填写 SMTP 密码'}
                        />
                      </Field>
                      <label className={securityClasses.toggle}>
                        <input
                          type="checkbox"
                          checked={draft.clear_password}
                          onChange={(event) => updateConfigDraft({ ...draft, clear_password: event.target.checked, password: event.target.checked ? '' : draft.password })}
                        />
                        <span>清空已保存密码</span>
                      </label>
                    </div>
                  </section>
                  </fieldset>

                  <footer className={securityClasses.saveRail}>
                    <div className={securityClasses.saveRailStatus}>
                      <Badge tone={editorPhaseTone(editorPhase)}>{editorPhaseLabel(editorPhase)}</Badge>
                      <span className={securityClasses.note}>{editorPhaseDetail(editorPhase)}</span>
                      {saveError ? <InlineFeedback tone="danger" message={saveError} /> : null}
                    </div>
                    <div className={securityClasses.actions}>
                      {compact ? <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} disabled={editorLocked} onClick={reloadWithConfirmation}>刷新</button> : null}
                      <button type="submit" className={cn(adminButton.base, adminButton.primary)} disabled={editorLocked || editorPhase === 'validating' || !dirty}>{saving ? '保存中...' : '保存 SMTP'}</button>
                    </div>
                  </footer>
                </form>
                <section className={securityClasses.section}>
              <div className={securityClasses.sectionHead}>
                <div>
                  <strong>发送测试邮件</strong>
                  <p className={securityClasses.note}>使用当前已保存配置发送测试验证码，不修改用户验证码缓存。</p>
                </div>
              </div>
              <div className={securityClasses.testRow}>
                <Field label="测试收件邮箱" error={!draft.test_email.trim() ? testError : undefined}>
                  <input value={draft.test_email} disabled={editorLocked} onChange={(event) => { setDraft({ ...draft, test_email: event.target.value }); setTestError('') }} placeholder="admin@example.com" />
                </Field>
                <button type="button" className={cn(adminButton.base, adminButton.ghost)} disabled={editorLocked} onClick={() => void testSMTP()}>{testing ? '发送中...' : '发送测试邮件'}</button>
              </div>
              {testError && draft.test_email.trim() ? <InlineFeedback tone="danger" message={testError} /> : null}
                </section>
              </section>
            </section>
          </details>
        </>
      ) : null}
    </section>
  )
}

function SecuritySummaryToggle({ title, detail, enabled }: { title: string; detail?: string; enabled: boolean }) {
  return (
    <div className={securityClasses.summaryToggle}>
      <div>
        <h4 className={securityClasses.summaryTitle}>{title}</h4>
        {detail ? <p className={securityClasses.summaryDetail}>{detail}</p> : null}
      </div>
      <div className={cn(securityClasses.switch, !enabled && securityClasses.switchOff)} aria-hidden="true">
        <div className={cn(securityClasses.knob, enabled ? 'right-1' : 'left-1')} />
      </div>
    </div>
  )
}

function smtpDraftIsDirty(draft: SMTPDraft, config: SMTPConfigView) {
  const baseline = smtpDraftFromConfig(config, draft.test_email)
  return draft.enabled !== baseline.enabled
    || draft.host !== baseline.host
    || draft.port !== baseline.port
    || draft.username !== baseline.username
    || draft.from !== baseline.from
    || draft.starttls !== baseline.starttls
    || draft.insecure_skip_verify !== baseline.insecure_skip_verify
    || Boolean(draft.password.trim())
    || draft.clear_password
}

function validateSMTPDraft(draft: SMTPDraft): SMTPFieldErrors {
  if (!draft.enabled) return {}
  const errors: SMTPFieldErrors = {}
  if (!draft.host.trim()) errors.host = '启用 SMTP 时必须填写服务器地址。'
  const port = Number(draft.port)
  if (!Number.isInteger(port) || port < 1 || port > 65535) errors.port = '端口必须是 1-65535 的整数。'
  if (!draft.from.trim()) errors.from = '启用 SMTP 时必须填写发件人。'
  return errors
}

function editorPhaseLabel(phase: EditorPhase) {
  if (phase === 'pristine') return '未修改'
  if (phase === 'dirty') return '有未保存修改'
  if (phase === 'validating') return '正在校验'
  if (phase === 'saving') return '正在保存'
  if (phase === 'saved') return '已保存'
  return '保存失败'
}

function editorPhaseDetail(phase: EditorPhase) {
  if (phase === 'dirty') return '保存后新配置才会生效。'
  if (phase === 'validating') return '正在检查必填字段与端口范围。'
  if (phase === 'saving') return '正在提交敏感配置，请勿关闭页面。'
  if (phase === 'saved') return '服务器已返回最新配置版本。'
  if (phase === 'failed') return '当前草稿仍保留，可修正后重试。'
  return '当前表单与已保存配置一致。'
}

function editorPhaseTone(phase: EditorPhase): 'neutral' | 'warning' | 'primary' | 'success' | 'danger' {
  if (phase === 'dirty' || phase === 'validating') return 'warning'
  if (phase === 'saving') return 'primary'
  if (phase === 'saved') return 'success'
  if (phase === 'failed') return 'danger'
  return 'neutral'
}

function smtpDraftFromConfig(config: SMTPConfigView, testEmail = ''): SMTPDraft {
  return {
    enabled: Boolean(config.enabled),
    host: config.host ?? '',
    port: config.port ? String(config.port) : '587',
    username: config.username ?? '',
    from: config.from ?? '',
    starttls: Boolean(config.starttls),
    insecure_skip_verify: Boolean(config.insecure_skip_verify),
    password: '',
    clear_password: false,
    test_email: testEmail,
  }
}

function smtpPayloadFromDraft(draft: SMTPDraft, config: SMTPConfigView): SMTPConfigWriteRequest {
  const payload: SMTPConfigWriteRequest = {
    version: config.version,
    enabled: draft.enabled,
    host: draft.host.trim(),
    port: Number(draft.port) || 0,
    username: draft.username.trim(),
    from: draft.from.trim(),
    starttls: draft.starttls,
    insecure_skip_verify: draft.insecure_skip_verify,
  }
  const password = draft.password.trim()
  if (password) payload.secrets = { password }
  if (draft.clear_password) payload.clear_secrets = ['password']
  return payload
}

function smtpSecretSummary(config: SMTPConfigView) {
  if (!config.secret_status?.has_secret) return '尚未配置 SMTP 密码'
  const fingerprint = config.secret_status.fingerprint ? `，指纹 ${config.secret_status.fingerprint}` : ''
  return `已配置 SMTP 密码${fingerprint}`
}
