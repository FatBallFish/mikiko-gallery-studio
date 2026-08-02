import { checkoutStripeConfirmOptions, checkoutStripePaymentConfig, checkoutStripeReturnURL } from './checkoutStripePayment'

const config = checkoutStripePaymentConfig({
  type: 'stripe_payment_element',
  client_secret: 'pi_contract_secret_client',
  publishable_key: 'pk_test_contract',
})
if (!config.supported || config.clientSecret !== 'pi_contract_secret_client' || config.publishableKey !== 'pk_test_contract') {
  throw new Error(`valid Stripe display should be supported, got ${JSON.stringify(config)}`)
}

for (const display of [
  { type: 'stripe_payment_element', publishable_key: 'pk_test_contract' },
  { type: 'stripe_payment_element', client_secret: 'pi_contract_secret_client' },
]) {
  const missing = checkoutStripePaymentConfig(display)
  if (missing.supported || !missing.error.includes('配置不完整')) {
    throw new Error(`incomplete Stripe display should expose an actionable configuration error, got ${JSON.stringify(missing)}`)
  }
}

if (checkoutStripeReturnURL('https://gallery.example.com/') !== 'https://gallery.example.com/#/checkout') {
  throw new Error('Stripe return URL should normalize the origin and preserve the checkout hash route')
}

const confirmOptions = checkoutStripeConfirmOptions('https://gallery.example.com')
if (confirmOptions.redirect !== 'if_required' || confirmOptions.confirmParams.return_url !== 'https://gallery.example.com/#/checkout') {
  throw new Error(`unexpected Stripe confirmation options ${JSON.stringify(confirmOptions)}`)
}
