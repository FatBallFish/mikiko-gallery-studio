import { FormEvent, useState } from 'react'
import type { AdminSession } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Field, InlineFeedback } from '../components'
import { adminLoginCopy, adminLoginInitialForm, adminLoginValidation, adminLoginVisibleError } from './adminLoginCopy'

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
    <main className="login-screen">
      <section className="login-panel">
        <div className="login-copy">
          <label>{adminLoginCopy.brand}</label>
          <strong>{adminLoginCopy.heroTitle}</strong>
          <p>{adminLoginCopy.heroDetail}</p>
          <div className="login-proof-grid">
            {adminLoginCopy.proofItems.map((item) => <span key={item}>{item}</span>)}
          </div>
        </div>

        <form className="login-form" onSubmit={submit} noValidate>
          <label>{adminLoginCopy.formEyebrow}</label>
          <h1>{adminLoginCopy.heroTitle}</h1>
          {error ? <InlineFeedback tone="danger" message={error} /> : <InlineFeedback tone="neutral" message={adminLoginCopy.idleNotice} />}

          <Field label={adminLoginCopy.emailLabel} error={emailError}>
            <input value={email} onChange={(event) => setEmail(event.target.value)} placeholder={adminLoginCopy.emailPlaceholder} autoComplete="username" />
          </Field>
          <Field label={adminLoginCopy.passwordLabel} error={passwordError}>
            <input value={password} onChange={(event) => setPassword(event.target.value)} type="password" placeholder={adminLoginCopy.passwordPlaceholder} autoComplete="current-password" />
          </Field>

          <button type="submit" className="btn primary wide" disabled={busy || Boolean(emailError || passwordError)}>
            {busy ? adminLoginCopy.submittingLabel : adminLoginCopy.submitLabel}
          </button>
        </form>
      </section>
    </main>
  )
}
