import { FormEvent, useMemo, useState } from 'react'
import { userApi } from '../../../shared/user-api'
import { Button, Field, useApp } from '../components'
import { form } from '../ui/redesign-classes'
import { errorMessage } from '../useApiResource'
import { createRedeemCodeSubmitter, normalizeRedeemCode } from './redeemCodeState'

export function RedeemCodeForm({ onRedeemed }: { onRedeemed: () => void | Promise<void> }) {
  const app = useApp()
  const [code, setCode] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = useMemo(() => createRedeemCodeSubmitter({
    redeem: (normalizedCode) => userApi.redeemCode(normalizedCode),
    onSuccess: onRedeemed,
  }), [onRedeemed])

  async function redeem(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!normalizeRedeemCode(code) || busy) return
    setBusy(true)
    try {
      const result = await submit(code)
      if (result === 'success') {
        setCode('')
        app.notify('success', '兑换成功，余额已更新')
      }
    } catch (caught) {
      app.notify('error', errorMessage(caught))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form className="grid gap-3" onSubmit={redeem}>
      <Field label="兑换码">
        <input
          className={form.input}
          value={code}
          onChange={(event) => setCode(event.target.value)}
          placeholder="输入兑换码"
          autoComplete="off"
          disabled={busy}
        />
      </Field>
      <Button type="submit" tone="primary" busy={busy} disabled={!normalizeRedeemCode(code)}>
        确认兑换
      </Button>
    </form>
  )
}
