import { FormEvent, KeyboardEvent, useEffect, useId, useRef, useState } from 'react'
import { ArrowLeft, Check, Eye, EyeOff, LoaderCircle, Moon, Sun } from 'lucide-react'
import { cn } from '../../../shared/classnames'
import type { SignupGrantResult } from '../../../shared/api-types'
import { userApi } from '../../../shared/user-api'
import { BrandMark, siteBrand } from '../brand'
import { useApp } from '../components'
import type { RouteId } from '../types'
import { errorMessage } from '../useApiResource'
import { landingAssetUrl } from './landingContent'
import { loginCopy, loginLocale } from './loginCopy'
import {
  loginPresentation,
  loginProviders,
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
  page: 'relative grid min-h-svh w-full max-w-full overflow-x-hidden bg-[var(--bg)] font-vault-body text-[var(--fg)] md:grid-cols-[minmax(320px,1.08fr)_minmax(460px,0.92fr)]',
  scene: 'pointer-events-none absolute inset-0 overflow-hidden md:relative md:min-h-svh',
  sceneImage: 'size-full object-cover object-[44%_center] opacity-55 transition-[filter,opacity,transform] duration-700 md:opacity-100 md:hover:scale-[1.015] motion-reduce:transition-none motion-reduce:hover:scale-100',
  sceneShade: 'absolute inset-0 bg-[linear-gradient(180deg,rgba(5,6,10,0.34)_0%,rgba(5,6,10,0.74)_58%,rgba(5,6,10,0.94)_100%)] md:bg-[linear-gradient(90deg,rgba(5,6,10,0.16)_0%,rgba(5,6,10,0.28)_58%,rgba(5,6,10,0.76)_100%)]',
  sceneCopy: 'absolute bottom-10 left-10 hidden max-w-[560px] text-[#f8f4ed] lg:block xl:bottom-14 xl:left-14',
  sceneTitle: 'm-0 max-w-[560px] font-vault-display text-[clamp(2.5rem,4.2vw,4.8rem)] font-bold leading-[1.02] tracking-[0]',
  sceneSummary: 'mt-6 max-w-[420px] text-base leading-7 text-white/68',
  brand: 'pointer-events-auto absolute left-4 top-4 z-30 rounded-xl border-0 bg-black/30 p-2 text-white backdrop-blur-md transition-transform duration-200 hover:scale-[1.03] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#d7a566] active:scale-[0.98] motion-reduce:transition-none motion-reduce:hover:scale-100 motion-reduce:active:scale-100 sm:left-6 sm:top-6 md:left-8 md:top-8',
  panel: 'relative z-10 flex min-h-svh items-center justify-center bg-[color-mix(in_oklch,var(--bg)_88%,transparent)] px-3 py-20 backdrop-blur-xl min-[360px]:px-5 sm:px-8 md:bg-[var(--bg)] md:px-10 md:py-24 md:backdrop-blur-none',
  themeButton: 'absolute right-4 top-4 grid size-11 place-items-center rounded-xl border border-[var(--border)] bg-[var(--surface)] text-[var(--muted)] shadow-[var(--shadow-sm)] transition-all duration-200 hover:-translate-y-px hover:border-[var(--border-strong)] hover:text-[var(--fg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] active:translate-y-0 active:scale-[0.96] motion-reduce:transition-none motion-reduce:hover:translate-y-0 motion-reduce:active:scale-100 sm:right-6 sm:top-6 md:right-8 md:top-8',
  formSurface: 'w-full max-w-[448px] rounded-2xl border border-[var(--border)] bg-[color-mix(in_oklch,var(--surface-solid)_92%,transparent)] p-5 shadow-[0_30px_90px_-48px_rgba(0,0,0,0.72)] backdrop-blur-xl min-[360px]:p-6 sm:p-8 md:bg-[var(--surface)] md:p-9',
  back: 'mb-5 inline-flex min-h-10 items-center gap-2 rounded-lg border-0 bg-transparent px-1 text-sm font-semibold text-[var(--muted)] transition-colors hover:text-[var(--fg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]',
  eyebrow: 'm-0 font-vault-mono text-[11px] font-semibold uppercase tracking-[0.16em] text-[var(--accent)]',
  title: 'mb-0 mt-3 font-vault-display text-[clamp(1.8rem,8vw,2.6rem)] font-bold leading-[1.08] tracking-[0] text-[var(--fg)]',
  summary: 'mb-0 mt-3 text-sm leading-6 text-[var(--muted)]',
  tabs: 'mt-7 grid grid-cols-2 rounded-xl border border-[var(--border)] bg-[var(--bg)] p-1',
  tab: 'min-h-10 rounded-lg border-0 bg-transparent px-3 text-sm font-semibold text-[var(--muted)] transition-[background-color,color,box-shadow,transform] duration-200 hover:text-[var(--fg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] active:scale-[0.98] motion-reduce:transition-none motion-reduce:active:scale-100 disabled:cursor-wait disabled:opacity-55',
  tabActive: 'bg-[var(--elevated)] text-[var(--fg)] shadow-[var(--shadow-sm)]',
  form: 'mt-6',
  field: 'mb-5',
  label: 'mb-2 block text-sm font-semibold text-[var(--fg)]',
  input: 'min-h-12 w-full rounded-xl border border-[var(--border)] bg-[var(--bg)] px-4 text-base text-[var(--fg)] shadow-inner shadow-black/5 transition-[border-color,box-shadow,background-color] duration-200 placeholder:text-[var(--dim)] hover:border-[var(--border-strong)] focus:border-[var(--accent)] focus:bg-[var(--surface-solid)] focus:outline-none focus:ring-2 focus:ring-[color-mix(in_oklch,var(--accent)_24%,transparent)] disabled:cursor-wait disabled:opacity-60',
  inputError: 'border-[var(--accent-coral)] focus:border-[var(--accent-coral)] focus:ring-[color-mix(in_oklch,var(--accent-coral)_22%,transparent)]',
  passwordWrap: 'relative',
  passwordInput: 'pr-14',
  iconButton: 'absolute right-1.5 top-1/2 grid size-9 -translate-y-1/2 place-items-center rounded-lg border-0 bg-transparent text-[var(--muted)] transition-colors hover:bg-[var(--elevated)] hover:text-[var(--fg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]',
  labelRow: 'mb-2 flex items-center justify-between gap-3',
  forgot: 'min-h-8 shrink-0 rounded-lg border-0 bg-transparent px-1 text-xs font-bold text-[var(--accent)] transition-colors hover:text-[var(--fg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]',
  codeRow: 'grid grid-cols-[minmax(0,1fr)_auto] gap-2',
  codeButton: 'min-h-12 max-w-[132px] rounded-xl border border-[var(--border-strong)] bg-[var(--surface-solid)] px-3 text-xs font-bold text-[var(--fg)] transition-all duration-200 hover:-translate-y-px hover:border-[var(--accent)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] active:translate-y-0 active:scale-[0.98] motion-reduce:transition-none motion-reduce:hover:translate-y-0 motion-reduce:active:scale-100 disabled:cursor-not-allowed disabled:translate-y-0 disabled:opacity-55',
  error: 'mb-0 mt-2 text-xs leading-5 text-[var(--accent-coral)]',
  notice: 'mb-0 mt-2 flex items-center gap-1.5 text-xs leading-5 text-[var(--accent-emerald)]',
  formError: 'mb-4 rounded-xl border border-[color-mix(in_oklch,var(--accent-coral)_34%,var(--border))] bg-[color-mix(in_oklch,var(--accent-coral)_8%,transparent)] px-3 py-2.5 text-sm leading-5 text-[var(--accent-coral)]',
  submit: 'mt-1 inline-flex min-h-13 w-full items-center justify-center gap-2 rounded-xl border border-[var(--accent)] bg-[var(--accent)] px-5 text-sm font-bold text-[#111218] shadow-[0_18px_42px_-24px_rgba(var(--accent-rgb),0.9)] transition-all duration-200 hover:-translate-y-0.5 hover:shadow-[0_22px_52px_-22px_rgba(var(--accent-rgb),0.9)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] focus-visible:ring-offset-3 focus-visible:ring-offset-[var(--surface-solid)] active:translate-y-0 active:scale-[0.98] motion-reduce:transition-none motion-reduce:hover:translate-y-0 motion-reduce:active:scale-100 disabled:cursor-wait disabled:translate-y-0 disabled:opacity-60',
  providers: 'mt-6 border-t border-[var(--border-subtle)] pt-5',
  providerLabel: 'mb-3 text-xs font-semibold text-[var(--dim)]',
  providerGrid: 'grid grid-cols-1 gap-2 min-[360px]:grid-cols-3',
  provider: 'min-h-9 rounded-lg border border-[var(--border-subtle)] bg-transparent px-2 text-[10px] font-semibold leading-tight text-[var(--dim)] disabled:cursor-not-allowed disabled:opacity-75',
  footer: 'mb-0 mt-6 text-center text-sm text-[var(--muted)]',
  footerAction: 'min-h-9 rounded-lg border-0 bg-transparent px-1 font-bold text-[var(--accent)] transition-colors hover:text-[var(--fg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]',
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

          {intent === 'login' && !passwordSetupToken ? (
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

          {!passwordSetupToken ? <div className={loginClasses.providers} aria-label="第三方登录状态">
            <p className={loginClasses.providerLabel}>第三方登录正在接入，当前请使用邮箱。</p>
            <div className={loginClasses.providerGrid}>
              {loginProviders.map((provider) => (
                <button key={provider.id} type="button" className={loginClasses.provider} disabled={provider.disabled} aria-disabled={provider.disabled}>
                  {provider.label}
                </button>
              ))}
            </div>
          </div> : null}

          {!passwordSetupToken ? <p className={loginClasses.footer}>
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
