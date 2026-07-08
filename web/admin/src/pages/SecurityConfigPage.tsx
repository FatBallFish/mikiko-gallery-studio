import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import type { SMTPConfigView, SMTPConfigWriteRequest } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Badge, ErrorBlock, Field, LoadingBlock, PageHeader, StatusCell, StatusStrip } from '../components'
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

const securityClasses = {
  summaryList: 'space-y-4',
  summaryToggle: 'flex items-center justify-between gap-4 rounded-lg border border-[var(--border)] bg-[var(--surface)] p-6 transition-all hover:bg-[var(--elevated)]',
  summaryTitle: 'text-sm font-bold text-[var(--text)]',
  summaryDetail: 'mt-1 text-xs text-[var(--muted-strong)]',
  switch: 'relative h-6 w-12 rounded-full bg-[var(--accent)]',
  switchOff: 'bg-[var(--surface-solid)]',
  knob: 'absolute top-1 size-4 rounded-full bg-white shadow-lg',
  section: 'grid gap-4 rounded-lg border border-[var(--border)] bg-[var(--surface)] p-6',
  sectionHead: 'flex flex-wrap items-center justify-between gap-2 border-b border-[var(--line)] pb-3',
  form: 'grid gap-4',
  toggle: 'grid grid-cols-[auto_minmax(0,1fr)] items-center gap-2 rounded-lg bg-[var(--surface-solid)] p-2 text-sm has-[:checked]:bg-[var(--accent)]/10',
  actions: 'flex flex-wrap items-center justify-end gap-2',
  note: 'm-0 text-sm text-[var(--soft)] [overflow-wrap:anywhere]',
  secretBox: 'grid gap-3 rounded-lg bg-[var(--surface-solid)] p-4',
  testRow: 'grid grid-cols-[minmax(220px,1fr)_auto] items-end gap-3 max-[620px]:grid-cols-1',
}

export function SecurityConfigPage({ onFeedback, compact = false, summaryMode = false }: { onFeedback?: (title: string, detail?: string) => void; compact?: boolean; summaryMode?: boolean }) {
  const [config, setConfig] = useState<SMTPConfigView | null>(null)
  const [draft, setDraft] = useState<SMTPDraft | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function load() {
    setLoading(true)
    setError(null)
    try {
      const next = await adminApi.getSMTPConfig()
      setConfig(next)
      setDraft(smtpDraftFromConfig(next))
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'SMTP 配置读取失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [])

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!draft || !config) return
    setSaving(true)
    setError(null)
    try {
      const payload = smtpPayloadFromDraft(draft, config)
      const updated = await adminApi.updateSMTPConfig(payload)
      setConfig(updated)
      setDraft(smtpDraftFromConfig(updated, draft.test_email))
      onFeedback?.('SMTP 配置已保存', smtpSecretSummary(updated))
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'SMTP 配置保存失败')
    } finally {
      setSaving(false)
    }
  }

  async function testSMTP() {
    if (!draft?.test_email.trim()) {
      setError('请先填写测试收件邮箱')
      return
    }
    setTesting(true)
    setError(null)
    try {
      const result = await adminApi.testSMTPConfig(draft.test_email.trim(), 'smtp_test')
      onFeedback?.('测试邮件已发送', result.recipient)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '测试邮件发送失败')
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
          actions={<button type="button" className={adminButton.base} onClick={() => void load()}>刷新</button>}
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
      {error ? <ErrorBlock message={error} onRetry={() => setError(null)} /> : null}
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
                <form className={securityClasses.section} onSubmit={(event) => void save(event)}>
              <div className={securityClasses.sectionHead}>
                <div>
                  <strong>SMTP 发信服务器</strong>
                  <p className={securityClasses.note}>用于登录、注册、密码重置和测试邮件发送。密码保存后不会明文回显。</p>
                </div>
                <div className={securityClasses.actions}>
                  <Badge tone={config.enabled ? 'success' : 'neutral'}>{config.enabled ? '已启用' : '未启用'}</Badge>
                  {compact ? <button type="button" className={cn(adminButton.base, adminButton.ghost, adminButton.small)} onClick={() => void load()}>刷新</button> : null}
                  <button type="submit" className={cn(adminButton.base, adminButton.primary)} disabled={saving}>{saving ? '保存中...' : '保存 SMTP'}</button>
                </div>
              </div>
              <label className={securityClasses.toggle}>
                <input type="checkbox" checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} />
                <span>启用 SMTP 发信</span>
              </label>
              <div className={adminPage.formGrid}>
                <Field label="SMTP Host">
                  <input value={draft.host} onChange={(event) => setDraft({ ...draft, host: event.target.value })} placeholder="smtp.example.com" />
                </Field>
                <Field label="SMTP Port">
                  <input value={draft.port} onChange={(event) => setDraft({ ...draft, port: event.target.value })} inputMode="numeric" placeholder="587" />
                </Field>
                <Field label="Username">
                  <input value={draft.username} onChange={(event) => setDraft({ ...draft, username: event.target.value })} placeholder="mailer@example.com" />
                </Field>
                <Field label="From">
                  <input value={draft.from} onChange={(event) => setDraft({ ...draft, from: event.target.value })} placeholder="Pic Gallery <noreply@example.com>" />
                </Field>
                <label className={securityClasses.toggle}>
                  <input type="checkbox" checked={draft.starttls} onChange={(event) => setDraft({ ...draft, starttls: event.target.checked })} />
                  <span>启用 STARTTLS</span>
                </label>
                <label className={securityClasses.toggle}>
                  <input type="checkbox" checked={draft.insecure_skip_verify} onChange={(event) => setDraft({ ...draft, insecure_skip_verify: event.target.checked })} />
                  <span>跳过 TLS 证书校验</span>
                </label>
              </div>
              <section className={securityClasses.secretBox}>
                <div className={securityClasses.sectionHead}>
                  <div>
                    <strong>SMTP 密码</strong>
                    <p className={securityClasses.note}>{smtpSecretSummary(config)}</p>
                  </div>
                </div>
                <div className={adminPage.formGrid}>
                  <Field label="新密码" hint="留空表示保留已保存密码；不要填写星号占位符。">
                    <input
                      value={draft.password}
                      onChange={(event) => setDraft({ ...draft, password: event.target.value, clear_password: false })}
                      type="password"
                      autoComplete="new-password"
                      placeholder={config.secret_status?.has_secret ? '留空保留当前密码' : '填写 SMTP 密码'}
                    />
                  </Field>
                  <label className={securityClasses.toggle}>
                    <input
                      type="checkbox"
                      checked={draft.clear_password}
                      onChange={(event) => setDraft({ ...draft, clear_password: event.target.checked, password: event.target.checked ? '' : draft.password })}
                    />
                    <span>清空已保存密码</span>
                  </label>
                </div>
              </section>
                </form>
                <section className={securityClasses.section}>
              <div className={securityClasses.sectionHead}>
                <div>
                  <strong>发送测试邮件</strong>
                  <p className={securityClasses.note}>使用当前已保存配置发送测试验证码，不修改用户验证码缓存。</p>
                </div>
              </div>
              <div className={securityClasses.testRow}>
                <Field label="测试收件邮箱">
                  <input value={draft.test_email} onChange={(event) => setDraft({ ...draft, test_email: event.target.value })} placeholder="admin@example.com" />
                </Field>
                <button type="button" className={cn(adminButton.base, adminButton.ghost)} disabled={testing} onClick={() => void testSMTP()}>{testing ? '发送中...' : '发送测试邮件'}</button>
              </div>
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
