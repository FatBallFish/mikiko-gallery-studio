import { FormEvent, useState } from 'react'
import type { AdminSession } from '../../../shared/api-types'
import { adminApi } from '../../../shared/admin-api'
import { Field, InlineFeedback } from '../components'

export function LoginPage({ onLogin }: { onLogin: (session: AdminSession) => void }) {
  const [email, setEmail] = useState('ops@example.com')
  const [password, setPassword] = useState('admin123')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const emailError = email && !/^\S+@\S+\.\S+$/.test(email) ? '请输入有效管理员邮箱' : null
  const passwordError = password && password.length < 6 ? '密码至少 6 位' : null

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (emailError || passwordError || !email || !password) {
      setError('请先修正表单校验错误。')
      return
    }
    setBusy(true)
    setError(null)
    try {
      const session = await adminApi.login(email, password)
      onLogin(session)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '管理员登录失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="login-screen">
      <section className="login-panel">
        <div className="login-copy">
          <label>Soft Grid Ops</label>
          <strong>Pic Gallery Admin</strong>
          <p>面向配置、路由、审核、计费与审计的高密度运营控制台。使用真实后台账号进入管理面板。</p>
          <div className="login-proof-grid">
            <span>Provider 健康</span>
            <span>配置草稿</span>
            <span>审核队列</span>
            <span>审计留痕</span>
          </div>
        </div>

        <form className="login-form" onSubmit={submit} noValidate>
          <label>Admin Access</label>
          <h1>登录运营后台</h1>
          {error ? <InlineFeedback tone="danger" message={error} /> : <InlineFeedback tone="neutral" message="Route guard 已启用，未登录会回到本页。" />}

          <Field label="管理员邮箱" error={emailError}>
            <input value={email} onChange={(event) => setEmail(event.target.value)} placeholder="ops@example.com" autoComplete="username" />
          </Field>
          <Field label="密码" error={passwordError}>
            <input value={password} onChange={(event) => setPassword(event.target.value)} type="password" placeholder="至少 6 位" autoComplete="current-password" />
          </Field>

          <button type="submit" className="btn primary wide" disabled={busy || Boolean(emailError || passwordError)}>
            {busy ? '校验中...' : '进入控制台'}
          </button>
        </form>
      </section>
    </main>
  )
}
