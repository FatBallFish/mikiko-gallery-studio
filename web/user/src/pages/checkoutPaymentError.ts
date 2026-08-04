import { ApiError, errorMessage } from '../../../shared/http-client'

export function checkoutPaymentErrorMessage(error: unknown) {
  if (error instanceof ApiError && [502, 503, 504].includes(error.status)) {
    return errorMessage(new ApiError(error.message, error.status, 'PAYMENT_PROVIDER_UNAVAILABLE', error.requestId, error.details))
  }
  return errorMessage(error)
}
