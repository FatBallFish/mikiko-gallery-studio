import { FormEvent, useEffect, useState } from 'react'
import { userApi } from '../../../shared/user-api'
import { useApp } from '../components'
import type { RouteId } from '../types'
import { errorMessage } from '../useApiResource'

const lastLoginEmailKey = 'pic-gallery-last-login-email'

const loginCopy = {
  zh: {
    emailPlaceholder: '输入邮箱地址',
    passwordPlaceholder: '输入密码',
    codePlaceholder: '6 位验证码',
    resetPasswordPlaceholder: '输入新密码',
    sendCodeFailed: '验证码发送失败',
    passwordLoginFailed: '账号密码登录失败',
    codeLoginFailed: '验证码登录失败',
    resetPasswordFailed: '密码重置失败',
    socialUnavailable: '该登录方式暂不可用',
  },
  en: {
    emailPlaceholder: 'Enter email address',
    passwordPlaceholder: 'Enter password',
    codePlaceholder: '6-digit code',
    resetPasswordPlaceholder: 'Enter new password',
    sendCodeFailed: 'Failed to send verification code',
    passwordLoginFailed: 'Password sign-in failed',
    codeLoginFailed: 'Verification code sign-in failed',
    resetPasswordFailed: 'Password reset failed',
    socialUnavailable: 'This sign-in method is not available yet',
  },
} as const

function loginLocale() {
  return navigator.language.toLowerCase().startsWith('zh') ? 'zh' : 'en'
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

export function LoginPage({ returnTo }: { returnTo?: RouteId }) {
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
      await app.login({ token: result.access_token, profile }, returnTo)
    } catch (err) {
      const title = resetMode ? copy.resetPasswordFailed : mode === 'password' ? copy.passwordLoginFailed : copy.codeLoginFailed
      app.notify('error', `${title}: ${errorMessage(err)}`)
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="auth-page">
      <div className="auth-card">
        {/* Logo */}
        <div style={{ textAlign: 'center', marginBottom: 40 }}>
          <button
            type="button"
            onClick={() => app.navigate('landing')}
            className="auth-logo"
          >
            Pic Gallery
          </button>
          <p style={{ color: 'var(--muted)', fontSize: 14, margin: '8px 0 0' }}>登入您的 AI 创作空间</p>
        </div>

        {/* Tabs */}
        <div className="auth-tabs-line">
          <button
            type="button"
            className={mode === 'password' ? 'active' : ''}
            onClick={() => setMode('password')}
          >
            账号密码登录
          </button>
          <button
            type="button"
            className={mode === 'code' ? 'active' : ''}
            onClick={() => setMode('code')}
          >
            验证码登录
          </button>
        </div>

        {/* Form */}
        <form onSubmit={submit}>
          <div className="auth-field">
            <label>邮箱地址</label>
            <input
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder={copy.emailPlaceholder}
              type="email"
              required
              className="input"
            />
          </div>

          {mode === 'password' ? (
            <div className="auth-field">
              <label>密码</label>
              <div className="password-input-wrap">
                <input
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={copy.passwordPlaceholder}
                  type={showPassword ? 'text' : 'password'}
                  minLength={6}
                  required
                  className="input"
                />
                <button
                  type="button"
                  className="password-toggle"
                  aria-label={showPassword ? '隐藏密码' : '显示密码'}
                  aria-pressed={showPassword}
                  onClick={() => setShowPassword((visible) => !visible)}
                >
                  {showPassword ? <EyeIcon /> : <EyeOffIcon />}
                </button>
              </div>
              <button type="button" className="forgot-password-link" onClick={() => { setResetMode(true); setMode('code') }}>忘记密码?</button>
            </div>
          ) : (
            <div className="auth-field">
              <label>{resetMode ? '重置验证码' : '验证码'}</label>
              <div className="inline-control">
                <input
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  placeholder={copy.codePlaceholder}
                  inputMode="numeric"
                  required
                  className="input"
                  style={{ flex: 1 }}
                />
                <button
                  type="button"
                  className="btn"
                  onClick={sendCode}
                  disabled={cooldown > 0}
                  style={{ whiteSpace: 'nowrap', opacity: cooldown > 0 ? 0.58 : 1 }}
                >
                  {cooldown > 0 ? `${cooldown}s` : '获取验证码'}
                </button>
              </div>
            </div>
          )}

          {resetMode ? (
            <div className="auth-field">
              <label>新密码</label>
              <input
                value={resetPassword}
                onChange={(e) => setResetPassword(e.target.value)}
                placeholder={copy.resetPasswordPlaceholder}
                type="password"
                minLength={6}
                required
                className="input"
              />
            </div>
          ) : null}

          <button
            type="submit"
            className="btn-login"
            disabled={busy}
          >
            {busy ? '提交中...' : resetMode ? '重置密码' : '登 录'}
          </button>
        </form>

        {/* Divider */}
        <div className="auth-divider">
          <span>其他登录方式</span>
        </div>

        {/* Social Login */}
        <div className="auth-social">
          <SocialButton icon={<WeChatIcon />} onClick={() => app.notify('info', copy.socialUnavailable)}>WeChat</SocialButton>
          <SocialButton icon={<DingTalkIcon />} onClick={() => app.notify('info', copy.socialUnavailable)}>钉钉</SocialButton>
          <SocialButton icon={<GoogleIcon />} onClick={() => app.notify('info', copy.socialUnavailable)}>Google</SocialButton>
        </div>

        {/* Footer */}
        <div className="auth-footer">
          还没有账号？ <button type="button" className="link-button" onClick={() => { setMode('code'); setResetMode(false) }}>验证码注册/登录</button>
        </div>
      </div>
    </main>
  )
}

function SocialButton({ children, icon, onClick }: { children: React.ReactNode; icon: React.ReactNode; onClick: () => void }) {
  return (
    <button type="button" className="social-login-btn" onClick={onClick}>
      {icon}
      {children}
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

function WeChatIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
      <path fill="#07C160" d="M9.3 5.1c-4 0-7.3 2.6-7.3 5.9 0 1.9 1.1 3.6 2.8 4.7l-.7 2.2 2.6-1.3c.8.2 1.6.3 2.6.3 4 0 7.3-2.6 7.3-5.9s-3.3-5.9-7.3-5.9z" />
      <path fill="#1AAD19" d="M15.2 10.1c-3.4 0-6.2 2.2-6.2 5 0 2.7 2.8 5 6.2 5 .8 0 1.5-.1 2.2-.3l2.2 1.1-.6-1.9c1.5-.9 2.4-2.3 2.4-3.9 0-2.8-2.8-5-6.2-5z" />
      <circle cx="6.9" cy="9.8" r=".7" fill="#fff" />
      <circle cx="11.5" cy="9.8" r=".7" fill="#fff" />
      <circle cx="13.1" cy="14.3" r=".6" fill="#fff" />
      <circle cx="17" cy="14.3" r=".6" fill="#fff" />
    </svg>
  )
}

function DingTalkIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
      <path fill="#1677FF" d="M20.8 4.4C16.7 2.7 11.4 1.5 4.9 1c-.8-.1-1.2.9-.6 1.4l4.4 3.7-5.4-.5c-.8-.1-1.2.9-.6 1.4l4.7 4-3.1.2c-.7.1-1 .9-.5 1.4l4.2 3.5-.9 4.7c-.1.6.6 1 1.1.6l12.9-11c2.4-2.2 2.3-4.8-.3-6z" />
      <path fill="#fff" d="M9.3 8.6l5.9.8-4.9 1.8 4.1.6-5.2 1.9 1-2.2-3.6-3.1 2.7.2z" opacity=".88" />
    </svg>
  )
}

function GoogleIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
      <path fill="#4285F4" d="M21.6 12.2c0-.7-.1-1.4-.2-2H12v3.8h5.4c-.2 1.2-.9 2.3-1.9 3v2.5h3c1.9-1.7 3.1-4.2 3.1-7.3z" />
      <path fill="#34A853" d="M12 22c2.7 0 5-.9 6.6-2.5l-3-2.5c-.8.6-1.9.9-3.6.9-2.6 0-4.7-1.7-5.5-4.1H3.4v2.6C5 19.7 8.3 22 12 22z" />
      <path fill="#FBBC05" d="M6.5 13.8c-.2-.6-.3-1.2-.3-1.8s.1-1.2.3-1.8V7.6H3.4C2.8 8.9 2.4 10.4 2.4 12s.4 3.1 1 4.4l3.1-2.6z" />
      <path fill="#EA4335" d="M12 6.1c1.5 0 2.8.5 3.8 1.5l2.8-2.8C16.9 3 14.7 2 12 2 8.3 2 5 4.3 3.4 7.6l3.1 2.6c.8-2.4 2.9-4.1 5.5-4.1z" />
    </svg>
  )
}
