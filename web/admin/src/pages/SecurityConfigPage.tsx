import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import type { SMTPConfigView, SMTPConfigWriteRequest } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, Button, ErrorBlock, InlineFeedback, LoadingBlock, PageHeader } from '../components'
import { FormSaveBar, FormSection, LedgerField, SwitchField } from './AdminMediaTabs'
import { adminButton, adminPage } from '../ui/classes'
import { OIDCConfigEditor } from './SecurityOIDCConfig'

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

export type EditorPhase = 'pristine' | 'dirty' | 'validating' | 'saving' | 'saved' | 'failed'
type SMTPFieldErrors = Partial<Record<'host' | 'port' | 'from', string>>

export const securityClasses = {
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
  const [oidcDirty, setOIDCDirty] = useState(false)
  const [oidcBusy, setOIDCBusy] = useState(false)
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
    onDirtyChange?.(dirty || oidcDirty)
  }, [dirty, oidcDirty, onDirtyChange])

  useEffect(() => () => onDirtyChange?.(false), [onDirtyChange])

  useEffect(() => {
    onBusyChange?.(editorLocked || oidcBusy)
  }, [editorLocked, oidcBusy, onBusyChange])

  useEffect(() => () => onBusyChange?.(false), [onBusyChange])

  useEffect(() => {
    if (!dirty && !oidcDirty) return undefined
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', handleBeforeUnload)
    return () => window.removeEventListener('beforeunload', handleBeforeUnload)
  }, [dirty, oidcDirty])

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

  async function save(event?: FormEvent<HTMLFormElement>) {
    event?.preventDefault()
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

  const liveFieldErrors = draft ? validateSMTPDraft(draft) : {}
  const liveErrorCount = Object.keys(liveFieldErrors).length

  return (
    <section className={adminPage.stack}>
      {!compact ? (
        <PageHeader
          title="安全配置"
          detail="管理 SMTP 发信服务器等敏感配置；密钥只写不读，后端加密存储。"
          actions={<button type="button" className={adminButton.base} disabled={editorLocked} onClick={reloadWithConfirmation}>刷新</button>}
        />
      ) : null}
      {error ? <InlineFeedback tone="danger" message={error} /> : null}

      <section className="a-panel a-panel--flush">
        <div className="a-panel__head">
          <div><h2>SMTP 邮件</h2><p>验证码、密码重置与告警邮件共用此配置。</p></div>
          <div className="a-row-actions">
            <Badge tone={config?.enabled ? 'success' : 'neutral'}>{config?.enabled ? '已启用' : '未启用'}</Badge>
            <Badge tone={config?.secret_status?.has_secret ? 'success' : 'warning'}>{config?.secret_status?.has_secret ? '密码已配置' : '密码未配置'}</Badge>
            <Badge tone={editorPhaseTone(editorPhase)}>{editorPhaseLabel(editorPhase)}</Badge>
          </div>
        </div>
        {draft && config ? (
          <form className="a-form" aria-busy={editorLocked} onSubmit={(event) => void save(event)}>
            <FormSection index={1} title="连接" group="connection" description="配置 SMTP 服务器地址和端口；启用时必须通过基础字段校验。">
              <SwitchField label="启用 SMTP 发信" checked={draft.enabled} onChange={(enabled) => updateConfigDraft({ ...draft, enabled })} disabled={editorLocked} />
              <LedgerField label="SMTP 主机" error={liveFieldErrors.host}><input className="a-input" value={draft.host} onChange={(event) => updateConfigDraft({ ...draft, host: event.target.value })} placeholder="smtp.example.com" /></LedgerField>
              <div className="a-form__pair">
                <LedgerField label="端口" error={liveFieldErrors.port}><input className="a-input" value={draft.port} onChange={(event) => updateConfigDraft({ ...draft, port: event.target.value })} inputMode="numeric" placeholder="587" /></LedgerField>
                <LedgerField label="传输安全"><select className="a-select" value={draft.starttls ? 'starttls' : 'tls'} onChange={(event) => updateConfigDraft({ ...draft, starttls: event.target.value === 'starttls' })}><option value="tls">SSL/TLS</option><option value="starttls">STARTTLS</option></select></LedgerField>
              </div>
              <small className="a-hint">465 端口会自动使用隐式 TLS；587 端口通常需要启用 STARTTLS。</small>
            </FormSection>

            <FormSection index={2} title="发件身份与传输安全" group="delivery">
              <LedgerField label="Username"><input className="a-input" value={draft.username} onChange={(event) => updateConfigDraft({ ...draft, username: event.target.value })} placeholder="mailer@example.com" /></LedgerField>
              <LedgerField label="From" error={liveFieldErrors.from}><input className="a-input" value={draft.from} onChange={(event) => updateConfigDraft({ ...draft, from: event.target.value })} placeholder="Mikiko Studio <noreply@example.com>" /></LedgerField>
              <SwitchField label="启用 STARTTLS（用于 587 等支持明文升级的端口）" checked={draft.starttls} onChange={(starttls) => updateConfigDraft({ ...draft, starttls })} disabled={editorLocked} />
              <SwitchField label="跳过 TLS 证书校验" checked={draft.insecure_skip_verify} onChange={(insecure_skip_verify) => updateConfigDraft({ ...draft, insecure_skip_verify })} disabled={editorLocked} hint="仅建议在受信任的内网环境中开启。" />
            </FormSection>

            <FormSection index={3} title="SMTP 密码" group="secret" description={`${smtpSecretSummary(config)}。密码保存后不会明文回显。`}>
              <LedgerField label="新密码" hint="留空表示保留已保存密码；不要填写星号占位符。"><input className="a-input" value={draft.password} onChange={(event) => updateConfigDraft({ ...draft, password: event.target.value, clear_password: false })} type="password" autoComplete="new-password" placeholder={config.secret_status?.has_secret ? '留空保留当前密码' : '填写 SMTP 密码'} /></LedgerField>
              <SwitchField label="清空已保存密码" checked={draft.clear_password} onChange={(clear_password) => updateConfigDraft({ ...draft, clear_password, password: clear_password ? '' : draft.password })} disabled={editorLocked} />
            </FormSection>

            <FormSection index={4} title="发送测试邮件" description="使用当前已保存配置发送测试验证码，不修改用户验证码缓存。">
              <div className="a-form__pair">
                <LedgerField label="测试收件邮箱" error={!draft.test_email.trim() ? testError : undefined}><input className="a-input" value={draft.test_email} disabled={editorLocked} onChange={(event) => { setDraft({ ...draft, test_email: event.target.value }); setTestError('') }} placeholder="admin@example.com" /></LedgerField>
                <div style={{ display: 'flex', alignItems: 'flex-end' }}><Button variant="secondary" size="sm" disabled={editorLocked} onClick={() => void testSMTP()}>{testing ? '发送中...' : '发送测试邮件'}</Button></div>
              </div>
              {testError && draft.test_email.trim() ? <InlineFeedback tone="danger" message={testError} /> : null}
              {editorPhaseDetail(editorPhase) ? <small className="a-hint">{editorPhaseDetail(editorPhase)}</small> : null}
            </FormSection>
          </form>
        ) : null}
        {draft ? (
          <FormSaveBar
            dirtyCount={dirty ? 1 : 0}
            errorCount={liveErrorCount}
            busy={saving || testing}
            note={saveError || '保存后新配置才会生效；敏感配置写入会记录审计日志。'}
            onReset={reloadWithConfirmation}
            onSave={() => void save()}
            saveLabel="保存 SMTP"
            resetLabel="刷新"
            saveDisabled={!dirty || !config}
          />
        ) : null}
      </section>

      <OIDCConfigEditor onFeedback={onFeedback} onDirtyChange={setOIDCDirty} onBusyChange={setOIDCBusy} summaryMode={summaryMode} />
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

export function editorPhaseLabel(phase: EditorPhase) {
  if (phase === 'pristine') return '未修改'
  if (phase === 'dirty') return '有未保存修改'
  if (phase === 'validating') return '正在校验'
  if (phase === 'saving') return '正在保存'
  if (phase === 'saved') return '已保存'
  return '保存失败'
}

export function editorPhaseDetail(phase: EditorPhase) {
  if (phase === 'dirty') return '保存后新配置才会生效。'
  if (phase === 'validating') return '正在检查必填字段与端口范围。'
  if (phase === 'saving') return '正在提交敏感配置，请勿关闭页面。'
  if (phase === 'saved') return '服务器已返回最新配置版本。'
  if (phase === 'failed') return '当前草稿仍保留，可修正后重试。'
  return '当前表单与已保存配置一致。'
}

export function editorPhaseTone(phase: EditorPhase): 'neutral' | 'warning' | 'primary' | 'success' | 'danger' {
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
