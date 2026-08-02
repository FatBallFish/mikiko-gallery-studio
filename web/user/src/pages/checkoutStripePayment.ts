import type { PaymentDisplay } from '../../../shared/api-types'

export type CheckoutStripePaymentConfig =
  | { supported: true; publishableKey: string; clientSecret: string }
  | { supported: false; error: string }

export function checkoutStripePaymentConfig(display: Pick<PaymentDisplay, 'type' | 'publishable_key' | 'client_secret'>): CheckoutStripePaymentConfig {
  const publishableKey = display.publishable_key?.trim() ?? ''
  const clientSecret = display.client_secret?.trim() ?? ''
  if (!publishableKey || !clientSecret) {
    return { supported: false, error: 'Stripe 支付配置不完整，请联系管理员检查 publishable key 和支付订单配置。' }
  }
  return { supported: true, publishableKey, clientSecret }
}

export function checkoutStripeReturnURL(origin: string) {
  return `${origin.trim().replace(/\/+$/, '')}/#/checkout`
}

export function checkoutStripeConfirmOptions(origin: string) {
  return {
    confirmParams: { return_url: checkoutStripeReturnURL(origin) },
    redirect: 'if_required' as const,
  }
}
