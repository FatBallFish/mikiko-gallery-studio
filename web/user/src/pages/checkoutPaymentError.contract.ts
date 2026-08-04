import { ApiError, errorMessage } from '../../../shared/http-client'
import { checkoutPaymentErrorMessage } from './checkoutPaymentError'

const expected = errorMessage(new ApiError('provider unavailable', 502, 'PAYMENT_PROVIDER_UNAVAILABLE'))
const htmlProxyError = new ApiError('request failed (502)', 502, 'request_failed')
if (checkoutPaymentErrorMessage(htmlProxyError) !== expected) {
  throw new Error('checkout must map an unstructured reverse-proxy 502 to a payment-channel error')
}

const paymentError = new ApiError('provider unavailable', 502, 'PAYMENT_PROVIDER_UNAVAILABLE')
if (checkoutPaymentErrorMessage(paymentError) !== expected) {
  throw new Error('checkout must preserve the structured payment-provider error message')
}
