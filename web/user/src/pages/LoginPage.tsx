import { FormEvent, useEffect, useState } from 'react'
import { cn } from '../../../shared/classnames'
import { userApi } from '../../../shared/user-api'
import { siteBrand } from '../brand'
import { useApp } from '../components'
import { rdCommon, rdShell, rdWorkspace } from '../ui/redesign-classes'
import type { RouteId } from '../types'
import { errorMessage } from '../useApiResource'
import { loginCopy, loginLocale, socialLoginUnavailableMessage } from './loginCopy'

const lastLoginEmailKey = 'pic-gallery-last-login-email'

const loginClasses = {
  page: cn(rdShell.main, 'grid place-items-center p-6'),
  card: cn(rdCommon.glass, 'w-[min(500px,100%)] rounded-3xl p-10 md:p-14 shadow-[0_40px_100px_rgba(0,0,0,0.6)] animate-in fade-in zoom-in-95 duration-700'),
  header: 'mb-12 text-center',
  logo: 'mb-6 inline-flex transition-transform hover:scale-110 active:scale-95',
  brandOrb: rdShell.brandOrb,
  subtitle: 'm-0 text-xs font-vault-mono uppercase tracking-[0.2em] text-[var(--muted)]',
  tabs: 'auth-tabs mb-10 flex gap-2 rounded-2xl bg-[var(--bg)]/50 p-1.5 border border-[var(--border)]',
  tab: 'flex-1 rounded-xl py-3 text-center text-[13px] font-bold text-[var(--muted)] transition-all duration-300 hover:text-[var(--fg)] hover:bg-[var(--surface)]',
  tabActive: 'bg-[var(--accent)] text-[var(--bg)] shadow-[0_8px_20px_rgba(var(--accent-rgb),0.2)]',
  field: 'auth-field mb-6 flex flex-col gap-2.5',
  label: 'text-[11px] font-vault-mono uppercase tracking-widest text-[var(--muted)] pl-1',
  input: 'w-full rounded-2xl border border-[var(--border)] bg-[var(--bg)]/50 p-4 text-[var(--fg)] outline-none transition-all focus:border-[var(--accent)]/50 focus:bg-[var(--surface)] focus:ring-4 focus:ring-[var(--accent)]/5',
  passwordWrap: 'relative',
  passwordInput: 'pr-14',
  passwordToggle: 'absolute right-2 top-1/2 grid size-10 -translate-y-1/2 place-items-center rounded-xl border-0 bg-transparent text-[var(--muted)] transition-colors hover:text-[var(--accent)] hover:bg-[var(--accent)]/5',
  forgot: 'self-end border-0 bg-transparent p-0 text-[11px] font-bold text-[var(--muted)] transition-colors hover:text-[var(--accent)]',
  inlineControl: 'flex gap-3',
  codeButton: 'whitespace-nowrap rounded-xl border border-[var(--border)] bg-[var(--bg)]/50 px-5 text-[13px] font-bold text-[var(--fg)] transition-all hover:border-[var(--accent)]/50 hover:bg-[var(--surface)] disabled:opacity-50',
  submit: cn(rdWorkspace.generateBtn, 'mt-4 h-14'),
  divider: 'auth-divider my-10 flex items-center text-[10px] font-vault-mono uppercase tracking-[0.2em] text-[var(--muted)] before:flex-1 before:border-b before:border-[var(--border)] after:flex-1 after:border-b after:border-[var(--border)]',
  dividerText: 'px-4',
  social: 'auth-social flex gap-3',
  socialButton: 'flex h-12 flex-1 items-center justify-center gap-2 rounded-xl border border-[var(--border)] bg-[var(--bg)]/50 text-sm font-black tracking-[0.12em] text-[var(--fg)] transition-all hover:border-[var(--accent)]/50 hover:bg-[var(--surface)] hover:scale-[1.02] active:scale-[0.98]',
  footer: 'auth-footer mt-10 text-center text-[13px] text-[var(--muted)]',
  link: 'font-black text-[var(--accent)] hover:underline underline-offset-4 transition-all',
}

function readLastLoginEmail() {
  try {
    return window.localStorage.getItem(lastLoginEmailKey) ?? ''
  } catch {
    return ''
  }
}

function rememberLoginEmail(email: string) {
  try {
    window.localStorage.setItem(lastLoginEmailKey, email)
  } catch {
    // Ignore storage errors; login itself should not depend on local persistence.
  }
}

export function LoginPage({ returnTo, imageId }: { returnTo?: RouteId; imageId?: string }) {
  const app = useApp()
  const env = import.meta.env as Record<string, string | undefined>
  const copy = loginCopy[loginLocale()]
  const [mode, setMode] = useState<'password' | 'code'>(env.VITE_AUTH_DEFAULT_MODE === 'code' ? 'code' : 'password')
  const [email, setEmail] = useState(() => readLastLoginEmail())
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [resetMode, setResetMode] = useState(false)
  const [resetPassword, setResetPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [cooldown, setCooldown] = useState(0)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (cooldown <= 0) return undefined
    const timer = window.setInterval(() => setCooldown((value) => Math.max(0, value - 1)), 1000)
    return () => window.clearInterval(timer)
  }, [cooldown])

  async function sendCode() {
    try {
      if (resetMode) await userApi.requestPasswordReset(email)
      else await userApi.sendEmailCode(email, 'login')
      setCooldown(60)
      app.notify('success', '验证码已发送，请查看邮箱')
    } catch (err) {
      app.notify('error', `${copy.sendCodeFailed}: ${errorMessage(err)}`)
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    try {
      if (resetMode) {
        await userApi.confirmPasswordReset(email, code, resetPassword)
        app.notify('success', '密码已重置，请使用新密码登录')
        setResetMode(false)
        setMode('password')
        setPassword(resetPassword)
        return
      }
      const result = mode === 'password'
        ? await userApi.loginWithPassword(email, password)
        : await userApi.loginWithEmailCode(email, code)
      const profile = await userApi.getProfileWithToken(result.access_token)
      rememberLoginEmail(email)
      await app.login({ token: result.access_token, profile }, returnTo, { imageId })
      if (result.signup_grant?.granted) {
        app.notify('success', `已领取 ${result.signup_grant.balance.trial_points ?? result.signup_grant.balance.available_points} 体验积分`)
        await app.refreshAccount()
      }
    } catch (err) {
      const title = resetMode ? copy.resetPasswordFailed : mode === 'password' ? copy.passwordLoginFailed : copy.codeLoginFailed
      app.notify('error', `${title}: ${errorMessage(err)}`)
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className={loginClasses.page}>
      <div className={loginClasses.card}>
        {/* Logo */}
        <div className={loginClasses.header}>
          <button
            type="button"
            onClick={() => app.navigate('landing')}
            className={loginClasses.logo}
            aria-label={`${siteBrand.name} 首页`}
          >
            <div className={loginClasses.brandOrb}>M</div>
          </button>
          <p className={loginClasses.subtitle}>邮箱登录后即可进入创作与资产管理</p>
        </div>

        {/* Tabs */}
        <div className={loginClasses.tabs}>
          <button
            type="button"
            className={cn(loginClasses.tab, mode === 'password' && loginClasses.tabActive)}
            onClick={() => setMode('password')}
          >
            账号密码
          </button>
          <button
            type="button"
            className={cn(loginClasses.tab, mode === 'code' && loginClasses.tabActive)}
            onClick={() => setMode('code')}
          >
            免密登录
          </button>
        </div>

        {/* Form */}
        <form onSubmit={submit}>
          <div className={loginClasses.field}>
            <label className={loginClasses.label}>邮箱地址</label>
            <input
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder={copy.emailPlaceholder}
              type="email"
              required
              className={loginClasses.input}
            />
          </div>

          {mode === 'password' ? (
            <div className={loginClasses.field}>
              <div className="flex items-center justify-between pr-1">
                <label className={loginClasses.label}>安全密码</label>
                <button type="button" className={loginClasses.forgot} onClick={() => { setResetMode(true); setMode('code') }}>忘记密码?</button>
              </div>
              <div className={loginClasses.passwordWrap}>
                <input
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={copy.passwordPlaceholder}
                  type={showPassword ? 'text' : 'password'}
                  minLength={6}
                  required
                  className={cn(loginClasses.input, loginClasses.passwordInput)}
                />
                <button
                  type="button"
                  className={loginClasses.passwordToggle}
                  aria-label={showPassword ? '隐藏密码' : '显示密码'}
                  aria-pressed={showPassword}
                  onClick={() => setShowPassword((visible) => !visible)}
                >
                  {showPassword ? <EyeIcon /> : <EyeOffIcon />}
                </button>
              </div>
            </div>
          ) : (
            <div className={loginClasses.field}>
              <label className={loginClasses.label}>{resetMode ? '重置验证码' : '邮箱验证码'}</label>
              <div className={loginClasses.inlineControl}>
                <input
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  placeholder={copy.codePlaceholder}
                  inputMode="numeric"
                  required
                  className={cn(loginClasses.input, 'flex-1')}
                />
                <button
                  type="button"
                  className={loginClasses.codeButton}
                  onClick={sendCode}
                  disabled={cooldown > 0}
                >
                  {cooldown > 0 ? `${cooldown}s` : '获取'}
                </button>
              </div>
            </div>
          )}

          {resetMode ? (
            <div className={loginClasses.field}>
              <label className={loginClasses.label}>新密码</label>
              <input
                value={resetPassword}
                onChange={(e) => setResetPassword(e.target.value)}
                placeholder={copy.resetPasswordPlaceholder}
                type="password"
                minLength={6}
                required
                className={loginClasses.input}
              />
            </div>
          ) : null}

          <button
            type="submit"
            className={loginClasses.submit}
            disabled={busy}
          >
            <div className={rdWorkspace.btnGlow} />
            <span className={rdWorkspace.btnText}>
              {busy ? '正在同步...' : resetMode ? '重置并登录' : '登 录'}
            </span>
          </button>
        </form>

        {/* Divider */}
        <div className={loginClasses.divider}>
          <span className={loginClasses.dividerText}>第三方极速联登</span>
        </div>

        {/* Social Login */}
        <div className={loginClasses.social}>
          <SocialButton label="微信" onClick={() => app.notify('info', socialLoginUnavailableMessage('微信'))} />
          <SocialButton label="钉钉" onClick={() => app.notify('info', socialLoginUnavailableMessage('钉钉'))} />
          <SocialButton label="Google" onClick={() => app.notify('info', socialLoginUnavailableMessage('Google'))} />
        </div>

        {/* Footer */}
        <div className={loginClasses.footer}>
          还没有账号？ <button type="button" className={loginClasses.link} onClick={() => { setMode('code'); setResetMode(false) }}>立即注册</button>
        </div>
      </div>
    </main>
  )
}

function SocialButton({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button type="button" className={loginClasses.socialButton} onClick={onClick}>
      <span>{label}</span>
    </button>
  )
}

function EyeIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="2">
      <path d="M2 12s3.5-6 10-6 10 6 10 6-3.5 6-10 6-10-6-10-6z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  )
}

function EyeOffIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="2">
      <path d="M3 3l18 18" />
      <path d="M10.6 10.6A3 3 0 0012 15a3 3 0 002.4-4.8" />
      <path d="M9.9 5.2A10.6 10.6 0 0112 5c6.5 0 10 7 10 7a18.5 18.5 0 01-3.3 4.4" />
      <path d="M6.7 6.7C3.7 8.6 2 12 2 12s3.5 7 10 7c1.5 0 2.9-.4 4.1-1" />
    </svg>
  )
}
