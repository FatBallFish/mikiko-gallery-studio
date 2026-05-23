import { FormEvent, useEffect, useState } from 'react'
import { userApi } from '../../../shared/user-api'
import { useApp } from '../components'
import type { RouteId } from '../types'
import { errorMessage } from '../useApiResource'

export function LoginPage({ returnTo }: { returnTo?: RouteId }) {
  const app = useApp()
  const [mode, setMode] = useState<'password' | 'code'>('password')
  const [email, setEmail] = useState('fatballfish@example.com')
  const [password, setPassword] = useState('vault2026')
  const [code, setCode] = useState('123456')
  const [resetMode, setResetMode] = useState(false)
  const [resetPassword, setResetPassword] = useState('')
  const [cooldown, setCooldown] = useState(0)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (cooldown <= 0) return undefined
    const timer = window.setInterval(() => setCooldown((value) => Math.max(0, value - 1)), 1000)
    return () => window.clearInterval(timer)
  }, [cooldown])

  async function sendCode() {
    setError(null)
    try {
      if (resetMode) await userApi.requestPasswordReset(email)
      else await userApi.sendEmailCode(email, 'login')
      setCooldown(60)
      app.notify('success', '验证码已发送，请查看邮箱')
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError(null)
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
      userApi.configureAuth({ getToken: () => result.access_token })
      const profile = await userApi.getProfile()
      await app.login({ token: result.access_token, profile }, returnTo)
    } catch (err) {
      setError(errorMessage(err))
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
              placeholder="输入邮箱地址"
              type="email"
              required
              className="input"
            />
          </div>

          {mode === 'password' ? (
            <div className="auth-field">
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <label>密码</label>
                <button type="button" className="link-button" style={{ fontSize: 12, color: 'var(--muted)', cursor: 'pointer', background: 'transparent', border: 0 }} onClick={() => { setResetMode(true); setMode('code') }}>忘记密码?</button>
              </div>
              <input
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="输入密码"
                type="password"
                minLength={6}
                required
                className="input"
              />
            </div>
          ) : (
            <div className="auth-field">
              <label>{resetMode ? '重置验证码' : '验证码'}</label>
              <div className="inline-control">
                <input
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  placeholder="6 位验证码"
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
                placeholder="输入新密码"
                type="password"
                minLength={6}
                required
                className="input"
              />
            </div>
          ) : null}

          {error ? <div className="form-error" style={{ marginBottom: 12 }}>{error}</div> : null}

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
          <SocialButton icon={<EyeIcon />}>WeChat</SocialButton>
          <SocialButton icon={<LayersIcon />}>钉钉</SocialButton>
          <SocialButton icon={<GoogleIcon />}>Google</SocialButton>
        </div>

        {/* Footer */}
        <div className="auth-footer">
          还没有账号？ <button type="button" className="link-button" onClick={() => { setMode('code'); setResetMode(false) }}>验证码注册/登录</button>
        </div>
      </div>
    </main>
  )
}

function SocialButton({ children, icon }: { children: React.ReactNode; icon: React.ReactNode }) {
  return (
    <button type="button" className="btn" style={{ borderRadius: 8, padding: 12, fontSize: 14 }}>
      {icon}
      {children}
    </button>
  )
}

function EyeIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
      <path d="M21 12c0 1.2-4 6-9 6s-9-4.8-9-6c0-1.2 4-6 9-6s9 4.8 9 6z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  )
}

function LayersIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
      <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
    </svg>
  )
}

function GoogleIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
      <path d="M21.5 12c0-.8-.1-1.6-.2-2.4H12v4.5h5.4c-.2 1.5-.9 2.7-2 3.6v3h3.2c1.9-1.7 3-4.2 3-6.7z" />
      <path d="M12 21.6c2.7 0 4.9-.9 6.6-2.4l-3.2-3c-.9.6-2 .9-3.4.9-2.6 0-4.8-1.7-5.6-4H3v3.1c1.7 3.4 5.3 5.4 9 5.4z" />
      <path d="M6.4 13.1c-.2-.6-.3-1.2-.3-1.8s.1-1.2.3-1.8V6.4H3C2.3 7.8 2 9.4 2 11c0 1.7.4 3.2 1 4.7l3.4-2.6z" />
      <path d="M12 6.6c1.5 0 2.8.5 3.9 1.5l2.9-2.9C17 3.5 14.7 2.4 12 2.4 8.3 2.4 4.7 4.4 3 7.8l3.4 2.6c.8-2.3 3-4 5.6-4z" />
    </svg>
  )
}
