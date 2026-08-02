import type { CheckoutPaymentDisplayModel } from './checkoutPaymentDisplay'

type PaymentForm = {
  requestSubmit?: () => void
  submit?: () => void
}

export type PaymentWindow = {
  closed: boolean
  opener: unknown
  location: { replace: (url: string) => void }
  document: {
    open: () => void
    write: (html: string) => void
    close: () => void
    querySelector: (selector: string) => PaymentForm | null
  }
  close: () => void
}

type OpenPaymentWindow = () => PaymentWindow | null

export function reservePaymentWindow(openWindow: OpenPaymentWindow = defaultOpenPaymentWindow): PaymentWindow | null {
  const paymentWindow = openWindow()
  if (paymentWindow) paymentWindow.opener = null
  return paymentWindow
}

export function dispatchPaymentWindow(
  paymentWindow: PaymentWindow | null,
  display: CheckoutPaymentDisplayModel,
): boolean {
  if (!paymentWindow || paymentWindow.closed) return false
  if (display.kind === 'redirect' && display.href) {
    paymentWindow.location.replace(display.href)
    return true
  }
  if (display.kind === 'form' && display.formHtml) {
    paymentWindow.document.open()
    paymentWindow.document.write(display.formHtml)
    paymentWindow.document.close()
    const form = paymentWindow.document.querySelector('form')
    if (!form) {
      closePaymentWindow(paymentWindow)
      return false
    }
    if (form.requestSubmit) form.requestSubmit()
    else form.submit?.()
    return true
  }
  closePaymentWindow(paymentWindow)
  return false
}

export function closePaymentWindow(paymentWindow: PaymentWindow | null): void {
  if (paymentWindow && !paymentWindow.closed) paymentWindow.close()
}

function defaultOpenPaymentWindow(): PaymentWindow | null {
  return window.open('', '_blank', 'noopener,noreferrer') as unknown as PaymentWindow | null
}
