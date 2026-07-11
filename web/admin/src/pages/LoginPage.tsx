import { FormEvent, useState } from 'react'
import type { AdminSession } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Field, InlineFeedback } from '../components'
import { useAdminTheme } from '../layout/useAdminTheme'
import { adminButton } from '../ui/classes'
import { MoonIcon, SunIcon } from '../ui/icons'
import { adminLoginCopy, adminLoginInitialForm, adminLoginValidation, adminLoginVisibleError } from './adminLoginCopy'

const loginClasses = {
  screen: 'grid min-h-screen place-items-center bg-[var(--bg)] p-6 text-[var(--fg)] selection:bg-[var(--accent)]/30 max-[620px]:p-3',
  panel: 'grid w-[min(460px,100%)] gap-8 rounded-xl border border-[var(--border)] bg-[var(--surface)] p-8 shadow-[var(--pg-shadow-lg)] backdrop-blur-xl max-[620px]:p-5',
  brand: 'flex items-center gap-3',
  brandOrb: 'grid size-10 place-items-center rounded-xl bg-[var(--accent)]/12 text-sm font-black text-[var(--accent)]',
  brandText: 'grid gap-0.5',
  brandName: 'font-[family-name:var(--font-admin-display)] text-lg font-semibold text-[var(--fg)]',
  brandMeta: 'text-xs font-semibold text-[var(--dim)]',
  hero: 'font-[family-name:var(--font-admin-display)] text-3xl font-semibold text-[var(--fg)]',
  detail: 'text-sm leading-6 text-[var(--soft)]',
  proofGrid: 'grid grid-cols-2 gap-2 max-[620px]:grid-cols-1',
  proofItem: 'rounded-lg border border-[var(--border)] bg-[var(--surface-solid)] px-3 py-2.5 text-xs font-semibold text-[var(--muted)]',
  form: 'grid gap-4',
  title: 'm-0 text-lg font-bold text-[var(--fg)]',
  themeButton: 'absolute right-4 top-4 grid size-10 place-items-center rounded-xl border border-[var(--border)] bg-[var(--surface)] text-[var(--muted)] transition hover:text-[var(--fg)]',
}

export function LoginPage({ onLogin }: { onLogin: (session: AdminSession) => void }) {
  const { theme, setTheme } = useAdminTheme()
  const env = import.meta.env as Record<string, string | undefined>
  const initialForm = adminLoginInitialForm(env)
  const [email, setEmail] = useState(initialForm.email)
  const [password, setPassword] = useState(initialForm.password)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const { emailError, passwordError } = adminLoginValidation(email, password)

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
    <main className={cn(loginClasses.screen, 'relative')} data-theme={theme}>
      <button
        type="button"
        className={loginClasses.themeButton}
        aria-label={theme === 'dark' ? '切换到亮色模式' : '切换到暗色模式'}
        title={theme === 'dark' ? '切换到亮色模式' : '切换到暗色模式'}
        onClick={() => setTheme((current) => current === 'dark' ? 'light' : 'dark')}
      >
        {theme === 'dark' ? <SunIcon /> : <MoonIcon />}
      </button>
      <section className={loginClasses.panel}>
        <div className="grid gap-5">
          <div className={loginClasses.brand}>
            <span className={loginClasses.brandOrb}>M</span>
            <div className={loginClasses.brandText}>
              <strong className={loginClasses.brandName}>Mikiko Admin</strong>
              <span className={loginClasses.brandMeta}>{adminLoginCopy.brand}</span>
            </div>
          </div>
          <strong className={loginClasses.hero}>登录运营后台</strong>
          <p className={loginClasses.detail}>{adminLoginCopy.heroDetail}</p>
          <div className={loginClasses.proofGrid}>
            {adminLoginCopy.proofItems.map((item) => <span key={item} className={loginClasses.proofItem}>{item}</span>)}
          </div>
        </div>

        <form className={loginClasses.form} onSubmit={submit} noValidate>
          <label>{adminLoginCopy.formEyebrow}</label>
          <h1 className={loginClasses.title}>管理员登录</h1>
          {error ? <InlineFeedback tone="danger" message={error} /> : <InlineFeedback tone="neutral" message={adminLoginCopy.idleNotice} />}

          <Field label={adminLoginCopy.emailLabel} error={emailError}>
            <input value={email} onChange={(event) => setEmail(event.target.value)} placeholder={adminLoginCopy.emailPlaceholder} autoComplete="username" />
          </Field>
          <Field label={adminLoginCopy.passwordLabel} error={passwordError}>
            <input value={password} onChange={(event) => setPassword(event.target.value)} type="password" placeholder={adminLoginCopy.passwordPlaceholder} autoComplete="current-password" />
          </Field>

          <button type="submit" className={cn(adminButton.base, adminButton.primary, 'w-full')} disabled={busy || Boolean(emailError || passwordError)}>
            {busy ? adminLoginCopy.submittingLabel : adminLoginCopy.submitLabel}
          </button>
        </form>
      </section>
    </main>
  )
}
