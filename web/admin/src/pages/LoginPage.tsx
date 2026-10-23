import { FormEvent, useState } from 'react'
import type { AdminSession } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Field } from '../components'
import { useAdminTheme } from '../layout/useAdminTheme'
import { MoonIcon, SunIcon } from '../ui/icons'
import { adminLoginCopy, adminLoginInitialForm, adminLoginValidation, adminLoginVisibleError } from './adminLoginCopy'

export function LoginPage({ onLogin }: { onLogin: (session: AdminSession) => void }) {
  const { theme, setTheme } = useAdminTheme()
  const env = import.meta.env as Record<string, string | undefined>
  const initialForm = adminLoginInitialForm(env)
  const [email, setEmail] = useState(initialForm.email)
  const [password, setPassword] = useState(initialForm.password)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const { emailError, passwordError } = adminLoginValidation(email, password)
  const proofs = [
    { label: '构建模式', value: env.MODE || 'production' },
    { label: '控制台', value: typeof window === 'undefined' ? '-' : window.location.host },
    { label: '访问控制', value: 'RBAC 权限校验' },
    { label: '会话', value: '受保护 · 审计开启' },
  ]

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (emailError || passwordError || !email || !password) {
      setError(adminLoginCopy.submitValidationError)
      return
    }
    setBusy(true)
    setError(null)
    try {
      const session = await adminApi.login(email, password)
      onLogin(session)
    } catch (caught) {
      setError(adminLoginVisibleError(caught))
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="a-login" data-theme={theme}>
      <section className="a-login__brandcol" aria-label="运营控制台介绍">
        <div className="a-login__brand">
          <i>M</i>
          <span>Mikiko Ops</span>
          <span style={{ marginLeft: 'auto', color: 'rgba(242,240,234,.45)', fontFamily: 'var(--font-admin-mono)', fontSize: 11 }}>
            Command Deck
          </span>
        </div>
        <div className="a-login__statement">
          <span className="a-kicker" style={{ color: 'rgba(242,240,234,.45)' }}>Operations Command Deck</span>
          <h1>看清每一笔消耗，<br />再决定下一步。</h1>
          <p>{adminLoginCopy.heroDetail}</p>
        </div>
        <div className="a-login__proofs">
          {proofs.map((proof) => (
            <div key={proof.label} className="a-login__proof">
              <span>{proof.label}</span>
              <strong style={{ overflowWrap: 'anywhere' }}>{proof.value}</strong>
            </div>
          ))}
        </div>
      </section>

      <section className="a-login__pane">
        <div className="a-login__form">
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, minWidth: 0 }}>
              <span className="a-side__mark" aria-hidden="true">M</span>
              <div style={{ display: 'grid', gap: 1, minWidth: 0 }}>
                <strong style={{ fontSize: 14 }}>Mikiko Ops</strong>
                <span className="a-hint" style={{ fontFamily: 'var(--font-admin-mono)', fontSize: 10.5 }}>{adminLoginCopy.brand}</span>
              </div>
            </div>
            <button
              type="button"
              className="a-icon-btn"
              aria-label={theme === 'dark' ? '切换到亮色模式' : '切换到暗色模式'}
              title={theme === 'dark' ? '切换到亮色模式' : '切换到暗色模式'}
              onClick={() => setTheme((current) => current === 'dark' ? 'light' : 'dark')}
            >
              {theme === 'dark' ? <SunIcon /> : <MoonIcon />}
            </button>
          </div>

          <div style={{ display: 'grid', gap: 6 }}>
            <span className="a-kicker">Admin Access</span>
            <h2>{adminLoginCopy.heroTitle}</h2>
            <p className="a-hint">使用管理员邮箱与密码。连续失败会触发登录限流并写入审计日志。</p>
          </div>

          <form className="a-form" style={{ display: 'grid', gap: 16 }} onSubmit={submit} noValidate>
            <Field label={adminLoginCopy.emailLabel} error={emailError}>
              <input
                className="a-input"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                placeholder={adminLoginCopy.emailPlaceholder}
                autoComplete="username"
                inputMode="email"
                aria-invalid={Boolean(emailError)}
                style={{ minHeight: 46 }}
              />
            </Field>
            <Field label={adminLoginCopy.passwordLabel} error={passwordError} hint="忘记密码请联系超级管理员重置">
              <input
                className="a-input"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                type="password"
                placeholder={adminLoginCopy.passwordPlaceholder}
                autoComplete="current-password"
                aria-invalid={Boolean(passwordError)}
                style={{ minHeight: 46 }}
              />
            </Field>

            {error ? (
              <div className="a-banner a-banner--danger" role="alert">
                <span>{error}</span>
              </div>
            ) : (
              <div className="a-banner a-banner--info" style={{ fontSize: 12.5 }}>
                <span>{adminLoginCopy.idleNotice}</span>
              </div>
            )}

            <button
              type="submit"
              className="a-btn a-btn--primary a-btn--block"
              style={{ minHeight: 46, fontSize: 14.5 }}
              disabled={busy || Boolean(emailError || passwordError)}
              aria-busy={busy}
            >
              {busy ? adminLoginCopy.submittingLabel : adminLoginCopy.submitLabel}
            </button>
          </form>

          <p className="a-hint" style={{ fontFamily: 'var(--font-admin-mono)', fontSize: 11 }}>
            会话受保护 · 登录行为将写入审计日志
          </p>
        </div>
      </section>
    </main>
  )
}