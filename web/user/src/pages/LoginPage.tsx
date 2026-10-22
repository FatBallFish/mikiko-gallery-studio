import { FormEvent, KeyboardEvent, useEffect, useId, useRef, useState } from 'react'
import { ArrowLeft, Check, Eye, EyeOff, Fingerprint, LoaderCircle, Moon, Sun } from 'lucide-react'
import { cn } from '../../../shared/classnames'
import type { SignupGrantResult } from '../../../shared/api-types'
import { userApi } from '../../../shared/user-api'
import { BrandMark, siteBrand } from '../brand'
import { useApp } from '../components'
import { featureAvailability } from '../featureAvailability'
import { oidcAuthorizeHref, targetURLAfterLogin } from '../oidcLogin'
import type { RouteId } from '../types'
import { errorMessage } from '../useApiResource'
import { landingAssetUrl } from './landingContent'
import { loginCopy, loginLocale } from './loginCopy'
import {
  loginPresentation,
  firstLoginInvalidField,
  loginCooldownForEmail,
  nextLoginFlow,
  nextLoginModeForKey,
  normalizeLoginEmail,
  validateLoginFields,
  type LoginFieldErrors,
  type LoginIntent,
  type LoginMode,
} from './loginPresentation'

const lastLoginEmailKey = 'pic-gallery-last-login-email'

const loginClasses = {
  page: 'u-login',
  scene: 'u-login__scene',
  sceneImage: '',
  sceneShade: '',
  sceneCopy: 'u-login__scene-copy',
  sceneTitle: '',
  sceneSummary: '',
  brand: 'u-login__brand',
  panel: 'u-login__pane',
  themeButton: 'u-icon-btn u-login__theme',
  formSurface: 'u-login__form',
  back: 'u-btn u-btn--quiet u-btn--sm',
  eyebrow: 'u-eyebrow',
  title: 'u-h2',
  summary: 'u-sub',
  tabs: 'u-login__tabs',
  tab: '',
  tabActive: 'is-active',
  form: '',
  field: 'u-field',
  label: '',
  input: 'u-input',
  inputError: 'is-invalid',
  passwordWrap: 'relative',
  passwordInput: 'u-input--password',
  iconButton: 'u-icon-btn u-icon-btn--plain u-login__password-toggle',
  labelRow: 'u-label-row',
  forgot: 'u-btn u-btn--quiet u-btn--sm',
  codeRow: 'u-login__code-row',
  codeButton: 'u-btn u-btn--sm u-login__code-button',
  error: 'u-field__error',
  notice: 'u-hint',
  formError: 'u-banner u-banner--danger',
  submit: 'u-btn u-btn--primary u-btn--lg u-btn--block',
  providers: 'u-login__providers',
  providerLabel: 'u-meta',
  providerGrid: 'u-login__provider-grid',
  provider: 'u-btn u-btn--sm u-login__provider',
  oidcButton: 'u-btn u-btn--block',
  footer: 'u-login__footer',
  footerAction: 'u-link',
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
    // Login must not depend on local persistence.
  }
}

export function LoginPage({ returnTo, imageId, taskId }: { returnTo?: RouteId; imageId?: string; taskId?: string }) {
  const app = useApp()
  const env = import.meta.env as Record<string, string | undefined>
  const copy = loginCopy[loginLocale()]
  const formId = useId()
  const formRef = useRef<HTMLFormElement>(null)
  const passwordTabRef = useRef<HTMLButtonElement>(null)
  const codeTabRef = useRef<HTMLButtonElement>(null)
  const [mode, setMode] = useState<LoginMode>(env.VITE_AUTH_DEFAULT_MODE === 'code' ? 'code' : 'password')
  const [intent, setIntent] = useState<LoginIntent>('login')
  const [email, setEmail] = useState(() => readLastLoginEmail())
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [passwordSetupToken, setPasswordSetupToken] = useState('')
  const [pendingSignupGrant, setPendingSignupGrant] = useState<SignupGrantResult | undefined>(undefined)
  const [showPassword, setShowPassword] = useState(false)
  const [showNewPassword, setShowNewPassword] = useState(false)
  const [cooldown, setCooldown] = useState(0)
  const [cooldownEmail, setCooldownEmail] = useState('')
  const [sending, setSending] = useState(false)
  const [busy, setBusy] = useState(false)
  const [errors, setErrors] = useState<LoginFieldErrors>({})
  const [formError, setFormError] = useState('')
  const [codeSent, setCodeSent] = useState(false)

  const effectiveCooldown = loginCooldownForEmail(cooldown, cooldownEmail, email)
  const presentation = loginPresentation({ mode, intent, busy, sending, cooldown: effectiveCooldown, passwordSetupRequired: Boolean(passwordSetupToken) })
  const availability = featureAvailability(app.featureFlags)
  const showLocalEntry = availability.showLocalLoginForm || Boolean(passwordSetupToken)
  const isDark = app.themePreference.mode === 'dark'
  const activeTabId = `${formId}-${mode}-tab`
  const studioShowcase1280Webp = landingAssetUrl(import.meta.env.BASE_URL, '/landing/studio-showcase-1280.webp')
  const studioShowcase1920Webp = landingAssetUrl(import.meta.env.BASE_URL, '/landing/studio-showcase-1920.webp')
  const studioShowcase1280Avif = landingAssetUrl(import.meta.env.BASE_URL, '/landing/studio-showcase-1280.avif')
  const studioShowcase1920Avif = landingAssetUrl(import.meta.env.BASE_URL, '/landing/studio-showcase-1920.avif')

  useEffect(() => {
    if (cooldown <= 0) return undefined
    const timer = window.setInterval(() => setCooldown((value) => Math.max(0, value - 1)), 1000)
    return () => window.clearInterval(timer)
  }, [cooldown])

  function clearFieldError(field: keyof LoginFieldErrors) {
    setErrors((current) => {
      if (!current[field]) return current
      const next = { ...current }
      delete next[field]
      return next
    })
    setFormError('')
  }

  function focusFirstInvalidField(validation: LoginFieldErrors) {
    const field = firstLoginInvalidField(validation, mode, intent, Boolean(passwordSetupToken))
    if (!field) return
    window.requestAnimationFrame(() => {
      formRef.current?.querySelector<HTMLInputElement>(`[data-auth-field="${field}"]`)?.focus()
    })
  }

  function changeMode(nextMode: LoginMode) {
    const next = nextLoginFlow(nextMode, cooldown)
    setMode(next.mode)
    setIntent(next.intent)
    setCooldown(next.cooldown)
    setCode(next.code)
    setPasswordSetupToken('')
    setPendingSignupGrant(undefined)
    setNewPassword('')
    setConfirmPassword('')
    setErrors({})
    setFormError('')
    setCodeSent(next.codeSent)
  }

  function changeIntent(nextIntent: LoginIntent) {
    const next = nextLoginFlow(nextIntent, cooldown)
    setIntent(next.intent)
    setMode(next.mode)
    setCooldown(next.cooldown)
    setCode(next.code)
    setPasswordSetupToken('')
    setPendingSignupGrant(undefined)
    setNewPassword('')
    setConfirmPassword('')
    setErrors({})
    setFormError('')
    setCodeSent(next.codeSent)
  }

  function handleTabKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    const nextMode = nextLoginModeForKey(mode, event.key)
    if (!nextMode) return
    event.preventDefault()
    changeMode(nextMode)
    const target = nextMode === 'password' ? passwordTabRef : codeTabRef
    window.requestAnimationFrame(() => target.current?.focus())
  }

  function startOIDCLogin() {
    window.location.href = oidcAuthorizeHref(targetURLAfterLogin(returnTo, imageId, taskId))
  }

  async function sendCode() {
    const validation = validateLoginFields({ mode: 'code', intent, email, password, code: '123456', newPassword: intent === 'reset' ? '123456' : '', confirmPassword: '', passwordSetupRequired: false })
    if (validation.email) {
      setErrors((current) => ({ ...current, email: validation.email }))
      focusFirstInvalidField({ email: validation.email })
      return
    }

    setSending(true)
    setFormError('')
    setCodeSent(false)
    try {
      if (intent === 'reset') await userApi.requestPasswordReset(email.trim())
      else await userApi.sendEmailCode(email.trim(), presentation.codeScene)
      setCooldownEmail(normalizeLoginEmail(email))
      setCooldown(60)
      setCodeSent(true)
      app.notify('success', '验证码已发送，请查看邮箱')
    } catch (caught) {
      const message = `${copy.sendCodeFailed}: ${errorMessage(caught)}`
      setFormError(message)
      app.notify('error', message)
    } finally {
      setSending(false)
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const validation = validateLoginFields({ mode, intent, email, password, code, newPassword, confirmPassword, passwordSetupRequired: Boolean(passwordSetupToken) })
    setErrors(validation)
    setFormError('')
    if (Object.keys(validation).length > 0) {
      focusFirstInvalidField(validation)
      return
    }

    setBusy(true)
    try {
      if (passwordSetupToken) {
        const result = await userApi.completePasswordSetup(passwordSetupToken, newPassword)
        const profile = await userApi.getProfileWithToken(result.access_token)
        rememberLoginEmail(email.trim())
        setPasswordSetupToken('')
        setNewPassword('')
        setConfirmPassword('')
        await app.login({ token: result.access_token, profile }, returnTo, { imageId, taskId })
        if (pendingSignupGrant?.granted) {
          app.notify('success', `已领取 ${pendingSignupGrant.balance.trial_points ?? pendingSignupGrant.balance.available_points} 体验积分`)
          await app.refreshAccount()
        }
        return
      }

      if (intent === 'reset') {
        await userApi.confirmPasswordReset(email.trim(), code.trim(), newPassword)
        app.notify('success', '密码已重置，请使用新密码登录')
        setPassword(newPassword)
        setCode('')
        setNewPassword('')
        changeIntent('login')
        return
      }

      const result = mode === 'password'
        ? await userApi.loginWithPassword(email.trim(), password)
        : await userApi.loginWithEmailCode(email.trim(), code.trim())
      if (result.password_setup_required) {
        setPasswordSetupToken(result.password_setup_token)
        setPendingSignupGrant(result.signup_grant)
        setPassword('')
        setCode('')
        setNewPassword('')
        setConfirmPassword('')
        return
      }
      const profile = await userApi.getProfileWithToken(result.access_token)
      rememberLoginEmail(email.trim())
      await app.login({ token: result.access_token, profile }, returnTo, { imageId, taskId })
      if (result.signup_grant?.granted) {
        app.notify('success', `已领取 ${result.signup_grant.balance.trial_points ?? result.signup_grant.balance.available_points} 体验积分`)
        await app.refreshAccount()
      }
    } catch (caught) {
      const title = intent === 'reset' ? copy.resetPasswordFailed : mode === 'password' ? copy.passwordLoginFailed : copy.codeLoginFailed
      const message = `${title}: ${errorMessage(caught)}`
      setFormError(message)
      app.notify('error', message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className={loginClasses.page}>
      <section className={loginClasses.scene} aria-hidden="true">
        <picture className="block size-full">
          <source type="image/avif" srcSet={`${studioShowcase1280Avif} 1280w, ${studioShowcase1920Avif} 1920w`} sizes="(min-width: 768px) 54vw, 100vw" />
          <source type="image/webp" srcSet={`${studioShowcase1280Webp} 1280w, ${studioShowcase1920Webp} 1920w`} sizes="(min-width: 768px) 54vw, 100vw" />
          <img className={loginClasses.sceneImage} src={studioShowcase1280Webp} alt="" width={1280} height={720} decoding="async" fetchPriority="high" />
        </picture>
        <div className={loginClasses.sceneShade} />
        <div className={loginClasses.sceneCopy}>
          <h1 className={loginClasses.sceneTitle}>让每一次生成，都回到同一座创作暗房。</h1>
          <p className={loginClasses.sceneSummary}>任务、生成结果与图片资产在一个连续工作空间中沉淀。</p>
        </div>
      </section>

      <button type="button" className={loginClasses.brand} onClick={() => app.navigate('landing')} aria-label={`${siteBrand.name} 首页`}>
        <BrandMark withText inverse />
      </button>

      <section className={loginClasses.panel} aria-labelledby={`${formId}-title`}>
        <button
          type="button"
          className={loginClasses.themeButton}
          onClick={() => void app.setThemePreference({ mode: isDark ? 'light' : 'dark' })}
          aria-label={isDark ? '切换到浅色主题' : '切换到深色主题'}
          title={isDark ? '切换到浅色主题' : '切换到深色主题'}
        >
          {isDark ? <Sun size={18} aria-hidden="true" /> : <Moon size={18} aria-hidden="true" />}
        </button>

        <div className={loginClasses.formSurface}>
          {intent !== 'login' ? (
            <button type="button" className={loginClasses.back} disabled={busy || sending} onClick={() => changeIntent('login')}>
              <ArrowLeft size={16} aria-hidden="true" />
              返回登录
            </button>
          ) : null}

          <p className={loginClasses.eyebrow}>Mikiko Studio Account</p>
          <h2 id={`${formId}-title`} className={loginClasses.title}>{presentation.title}</h2>
          <p className={loginClasses.summary}>{presentation.summary}</p>

          {intent === 'login' && !passwordSetupToken && showLocalEntry ? (
          <div className={loginClasses.tabs} role="tablist" aria-label="登录方式">
            <button
              id={`${formId}-password-tab`}
              ref={passwordTabRef}
              type="button"
              role="tab"
              aria-selected={mode === 'password'}
              aria-controls={`${formId}-form`}
              tabIndex={mode === 'password' ? 0 : -1}
              disabled={busy || sending}
              className={cn(loginClasses.tab, mode === 'password' && loginClasses.tabActive)}
              onClick={() => changeMode('password')}
              onKeyDown={handleTabKeyDown}
            >
              密码登录
            </button>
            <button
              id={`${formId}-code-tab`}
              ref={codeTabRef}
              type="button"
              role="tab"
              aria-selected={mode === 'code'}
              aria-controls={`${formId}-form`}
              tabIndex={mode === 'code' ? 0 : -1}
              disabled={busy || sending}
              className={cn(loginClasses.tab, mode === 'code' && loginClasses.tabActive)}
              onClick={() => changeMode('code')}
              onKeyDown={handleTabKeyDown}
            >
              邮箱验证码
            </button>
          </div>
          ) : null}

          {showLocalEntry ? (
          <form
            ref={formRef}
            id={`${formId}-form`}
            className={loginClasses.form}
            role={intent === 'login' && !passwordSetupToken ? 'tabpanel' : undefined}
            aria-labelledby={intent === 'login' && !passwordSetupToken ? activeTabId : undefined}
            onSubmit={submit}
            noValidate
          >
            {!passwordSetupToken ? <div className={loginClasses.field}>
              <label className={loginClasses.label} htmlFor={`${formId}-email`}>邮箱地址</label>
              <input
                id={`${formId}-email`}
                data-auth-field="email"
                className={cn(loginClasses.input, errors.email && loginClasses.inputError)}
                type="email"
                inputMode="email"
                autoComplete="email"
                value={email}
                placeholder={copy.emailPlaceholder}
                disabled={busy || sending}
                aria-invalid={Boolean(errors.email)}
                aria-describedby={errors.email ? `${formId}-email-error` : undefined}
                onChange={(event) => {
                  const nextEmail = event.target.value
                  if (normalizeLoginEmail(nextEmail) !== normalizeLoginEmail(email)) {
                    setCode('')
                    setCodeSent(false)
                  }
                  setEmail(nextEmail)
                  clearFieldError('email')
                }}
              />
              {errors.email ? <p id={`${formId}-email-error`} className={loginClasses.error} role="alert">{errors.email}</p> : null}
            </div> : null}

            {presentation.showPassword ? (
              <div className={loginClasses.field}>
                <div className={loginClasses.labelRow}>
                  <label className="text-sm font-semibold text-[var(--fg)]" htmlFor={`${formId}-password`}>密码</label>
                  <button type="button" className={loginClasses.forgot} disabled={busy} onClick={() => changeIntent('reset')}>忘记密码</button>
                </div>
                <div className={loginClasses.passwordWrap}>
                  <input
                    id={`${formId}-password`}
                    data-auth-field="password"
                    className={cn(loginClasses.input, loginClasses.passwordInput, errors.password && loginClasses.inputError)}
                    type={showPassword ? 'text' : 'password'}
                    autoComplete="current-password"
                    value={password}
                    placeholder={copy.passwordPlaceholder}
                    disabled={busy}
                    aria-invalid={Boolean(errors.password)}
                    aria-describedby={errors.password ? `${formId}-password-error` : undefined}
                    onChange={(event) => { setPassword(event.target.value); clearFieldError('password') }}
                  />
                  <button type="button" className={loginClasses.iconButton} onClick={() => setShowPassword((visible) => !visible)} aria-label={showPassword ? '隐藏密码' : '显示密码'} aria-pressed={showPassword}>
                    {showPassword ? <EyeOff size={18} aria-hidden="true" /> : <Eye size={18} aria-hidden="true" />}
                  </button>
                </div>
                {errors.password ? <p id={`${formId}-password-error`} className={loginClasses.error} role="alert">{errors.password}</p> : null}
              </div>
            ) : null}

            {presentation.showCode ? (
              <div className={loginClasses.field}>
                <label className={loginClasses.label} htmlFor={`${formId}-code`}>{intent === 'reset' ? '重置验证码' : '邮箱验证码'}</label>
                <div className={loginClasses.codeRow}>
                  <input
                    id={`${formId}-code`}
                    data-auth-field="code"
                    className={cn(loginClasses.input, errors.code && loginClasses.inputError)}
                    type="text"
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    pattern="[0-9]*"
                    maxLength={6}
                    value={code}
                    placeholder={copy.codePlaceholder}
                    disabled={busy}
                    aria-invalid={Boolean(errors.code)}
                    aria-describedby={errors.code ? `${formId}-code-error` : codeSent ? `${formId}-code-notice` : undefined}
                    onChange={(event) => { setCode(event.target.value.replace(/\D/g, '').slice(0, 6)); clearFieldError('code') }}
                  />
                  <button type="button" className={loginClasses.codeButton} disabled={presentation.sendCodeDisabled} onClick={() => void sendCode()}>
                    {sending ? <LoaderCircle className="mx-auto animate-spin motion-reduce:animate-none" size={17} aria-hidden="true" /> : presentation.sendCodeLabel}
                  </button>
                </div>
                {errors.code ? <p id={`${formId}-code-error`} className={loginClasses.error} role="alert">{errors.code}</p> : null}
                {codeSent && effectiveCooldown > 0 && !errors.code ? <p id={`${formId}-code-notice`} className={loginClasses.notice}><Check size={14} aria-hidden="true" />验证码已发送，请查看邮箱</p> : null}
              </div>
            ) : null}

            {presentation.showNewPassword ? (
              <div className={loginClasses.field}>
                <label className={loginClasses.label} htmlFor={`${formId}-new-password`}>新密码</label>
                <div className={loginClasses.passwordWrap}>
                  <input
                    id={`${formId}-new-password`}
                    data-auth-field="newPassword"
                    className={cn(loginClasses.input, loginClasses.passwordInput, errors.newPassword && loginClasses.inputError)}
                    type={showNewPassword ? 'text' : 'password'}
                    autoComplete="new-password"
                    value={newPassword}
                    placeholder={copy.resetPasswordPlaceholder}
                    disabled={busy}
                    aria-invalid={Boolean(errors.newPassword)}
                    aria-describedby={errors.newPassword ? `${formId}-new-password-error` : undefined}
                    onChange={(event) => { setNewPassword(event.target.value); clearFieldError('newPassword') }}
                  />
                  <button type="button" className={loginClasses.iconButton} onClick={() => setShowNewPassword((visible) => !visible)} aria-label={showNewPassword ? '隐藏新密码' : '显示新密码'} aria-pressed={showNewPassword}>
                    {showNewPassword ? <EyeOff size={18} aria-hidden="true" /> : <Eye size={18} aria-hidden="true" />}
                  </button>
                </div>
                {errors.newPassword ? <p id={`${formId}-new-password-error`} className={loginClasses.error} role="alert">{errors.newPassword}</p> : null}
              </div>
            ) : null}

            {presentation.showPasswordConfirmation ? (
              <div className={loginClasses.field}>
                <label className={loginClasses.label} htmlFor={`${formId}-confirm-password`}>确认新密码</label>
                <div className={loginClasses.passwordWrap}>
                  <input
                    id={`${formId}-confirm-password`}
                    data-auth-field="confirmPassword"
                    className={cn(loginClasses.input, loginClasses.passwordInput, errors.confirmPassword && loginClasses.inputError)}
                    type={showNewPassword ? 'text' : 'password'}
                    autoComplete="new-password"
                    value={confirmPassword}
                    placeholder="再次输入新密码"
                    disabled={busy}
                    aria-invalid={Boolean(errors.confirmPassword)}
                    aria-describedby={errors.confirmPassword ? `${formId}-confirm-password-error` : undefined}
                    onChange={(event) => { setConfirmPassword(event.target.value); clearFieldError('confirmPassword') }}
                  />
                </div>
                {errors.confirmPassword ? <p id={`${formId}-confirm-password-error`} className={loginClasses.error} role="alert">{errors.confirmPassword}</p> : null}
              </div>
            ) : null}

            {formError ? <p className={loginClasses.formError} role="alert">{formError}</p> : null}

            <button type="submit" className={loginClasses.submit} style={{ color: '#111218' }} disabled={presentation.submitDisabled}>
              {busy ? <LoaderCircle className="animate-spin motion-reduce:animate-none" size={17} aria-hidden="true" /> : null}
              {presentation.submitLabel}
            </button>
          </form>
          ) : null}

          {!passwordSetupToken && availability.showOIDCLogin ? (
            <div className={loginClasses.providers}>
              {showLocalEntry ? <p className={loginClasses.providerLabel}>或使用统一身份登录</p> : null}
              <button type="button" className={loginClasses.oidcButton} onClick={startOIDCLogin}>
                <Fingerprint size={17} aria-hidden="true" />
                {availability.oidcLoginLabel}
              </button>
            </div>
          ) : null}

          {!passwordSetupToken && availability.showRegistrationEntry ? <p className={loginClasses.footer}>
            {intent === 'register' ? '已经有账户？' : '还没有账户？'}{' '}
            <button type="button" className={loginClasses.footerAction} disabled={busy || sending} onClick={() => changeIntent(intent === 'register' ? 'login' : 'register')}>
              {intent === 'register' ? '返回登录' : '通过邮箱注册'}
            </button>
          </p> : null}
        </div>
      </section>
    </main>
  )
}
