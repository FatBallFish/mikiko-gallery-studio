import { FormEvent, useState } from 'react'
import { Elements, PaymentElement, useElements, useStripe } from '@stripe/react-stripe-js'
import { loadStripe, type Stripe } from '@stripe/stripe-js'
import { Button } from '../components'
import { checkoutStripeConfirmOptions } from './checkoutStripePayment'

const stripePromises = new Map<string, Promise<Stripe | null>>()

function stripePromise(publishableKey: string) {
  const cached = stripePromises.get(publishableKey)
  if (cached) return cached
  const next = loadStripe(publishableKey)
  stripePromises.set(publishableKey, next)
  return next
}

export function StripePaymentPanel({
  publishableKey,
  clientSecret,
  disabled,
  onConfirmed,
}: {
  publishableKey: string
  clientSecret: string
  disabled?: boolean
  onConfirmed: () => void | Promise<void>
}) {
  return (
    <Elements
      key={clientSecret}
      stripe={stripePromise(publishableKey)}
      options={{
        clientSecret,
        appearance: {
          theme: 'stripe',
          variables: {
            borderRadius: '8px',
            fontFamily: 'Inter, ui-sans-serif, system-ui, sans-serif',
            spacingUnit: '4px',
          },
        },
      }}
    >
      <StripePaymentForm disabled={disabled} onConfirmed={onConfirmed} />
    </Elements>
  )
}

function StripePaymentForm({ disabled, onConfirmed }: { disabled?: boolean; onConfirmed: () => void | Promise<void> }) {
  const stripe = useStripe()
  const elements = useElements()
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  async function confirm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!stripe || !elements || submitting || disabled) return
    setSubmitting(true)
    setError('')
    const result = await stripe.confirmPayment({
      elements,
      ...checkoutStripeConfirmOptions(window.location.origin),
    })
    if (result.error) {
      setError(result.error.message?.trim() || 'Stripe 未能确认付款，请检查支付信息后重试。')
      setSubmitting(false)
      return
    }
    await onConfirmed()
    setSubmitting(false)
  }

  return (
    <form className="grid gap-4" onSubmit={confirm}>
      <PaymentElement options={{ layout: 'tabs' }} />
      {error ? <p className="m-0 text-sm leading-relaxed text-[var(--danger)]" role="alert">{error}</p> : null}
      <Button type="submit" tone="primary" busy={submitting} disabled={disabled || !stripe || !elements}>
        确认支付
      </Button>
    </form>
  )
}
