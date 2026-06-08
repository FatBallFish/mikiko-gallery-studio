import { FormEvent, useState } from 'react'
import type { AdminSession } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { cn } from '../../../shared/classnames'
import { Field, InlineFeedback } from '../components'
import { adminButton } from '../ui/classes'
import { adminLoginCopy, adminLoginInitialForm, adminLoginValidation, adminLoginVisibleError } from './adminLoginCopy'

const loginClasses = {
  screen: 'grid min-h-screen place-items-center bg-[linear-gradient(90deg,rgba(87,117,185,0.08)_1px,transparent_1px),linear-gradient(0deg,rgba(87,117,185,0.06)_1px,transparent_1px),radial-gradient(circle_at_25%_20%,rgba(87,117,185,0.16),transparent_28%),var(--pg-admin-bg-app)] bg-[length:42px_42px,42px_42px,auto,auto] p-6 max-[620px]:p-2.5',
  panel: 'grid min-h-[560px] w-[min(980px,100%)] grid-cols-[minmax(0,1.1fr)_420px] overflow-hidden rounded-[22px] border border-[var(--line)] bg-[var(--surface-frost)] shadow-[var(--pg-shadow-sm)] backdrop-blur-[14px] max-[920px]:min-h-0 max-[920px]:grid-cols-1',
  copy: 'grid content-end gap-3.5 bg-[linear-gradient(135deg,rgba(87,117,185,0.18),rgba(255,255,255,0.16)),rgba(248,250,251,0.72)] p-[clamp(26px,5vw,54px)]',
  hero: 'max-w-[10ch] text-[clamp(2rem,5vw,4.4rem)] font-medium leading-[.94] text-[var(--text)]',
  detail: 'max-w-[56ch]',
  proofGrid: 'mt-2.5 grid grid-cols-2 gap-2 max-[620px]:grid-cols-1',
  proofItem: 'rounded-xl bg-white/60 px-3 py-2.5 text-[0.82rem] font-extrabold text-[var(--text)]',
  form: 'grid content-center gap-3.5 bg-white/80 p-[clamp(26px,5vw,54px)]',
  title: 'm-0 text-[1.7rem] font-medium text-[var(--text)]',
}

export function LoginPage({ onLogin }: { onLogin: (session: AdminSession) => void }) {
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
    <main className={loginClasses.screen}>
      <section className={loginClasses.panel}>
        <div className={loginClasses.copy}>
          <label>{adminLoginCopy.brand}</label>
          <strong className={loginClasses.hero}>{adminLoginCopy.heroTitle}</strong>
          <p className={loginClasses.detail}>{adminLoginCopy.heroDetail}</p>
          <div className={loginClasses.proofGrid}>
            {adminLoginCopy.proofItems.map((item) => <span key={item} className={loginClasses.proofItem}>{item}</span>)}
          </div>
        </div>

        <form className={loginClasses.form} onSubmit={submit} noValidate>
          <label>{adminLoginCopy.formEyebrow}</label>
          <h1 className={loginClasses.title}>{adminLoginCopy.heroTitle}</h1>
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
