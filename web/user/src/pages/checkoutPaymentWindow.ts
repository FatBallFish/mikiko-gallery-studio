import type { PublicPaymentVisibleMethod } from '../../../shared/api-types'
import type { CheckoutPaymentDisplayModel } from './checkoutPaymentDisplay'

type PaymentWindowElement = {
  textContent: string | null
  setAttribute: (name: string, value: string) => void
  append: (...nodes: PaymentWindowElement[]) => void
  requestSubmit?: () => void
  submit?: () => void
}

export type PaymentWindow = {
  closed: boolean
  opener: unknown
  location: { replace: (url: string) => void }
  document: {
    body: { replaceChildren: (...nodes: PaymentWindowElement[]) => void }
    createElement: (tagName: string) => PaymentWindowElement
  }
  close: () => void
}

type OpenPaymentWindow = () => PaymentWindow | null
type PaymentFormField = { name: string; value: string }
export type PaymentFormProjection = { action: string; method: 'get' | 'post'; fields: PaymentFormField[] }
type PaymentFormCandidate = { action: string; method: string; fields: PaymentFormField[] }
type PaymentFormParser = (formHTML: string) => PaymentFormCandidate | null

const maximumPaymentFormHTMLLength = 256 * 1024
const maximumPaymentFormFields = 256

export function reservePaymentWindow(openWindow: OpenPaymentWindow = defaultOpenPaymentWindow): PaymentWindow | null {
  const paymentWindow = openWindow()
  if (paymentWindow) {
    paymentWindow.opener = null
    replacePaymentWindowContent(paymentWindow, '正在创建订单，请稍候...')
  }
  return paymentWindow
}

export function paymentMethodNeedsReservedWindow(method: PublicPaymentVisibleMethod | undefined) {
  const methodCode = method?.method.trim().toLowerCase()
  return methodCode !== 'stripe' && methodCode !== 'mock'
}

export function dispatchPaymentWindow(
  paymentWindow: PaymentWindow | null,
  display: CheckoutPaymentDisplayModel,
  formParser: PaymentFormParser = parsePaymentFormDocument,
): boolean {
  if (!paymentWindow || paymentWindow.closed) return false
  if (display.kind === 'redirect' && display.href) {
    const destination = safePaymentURL(display.href)
    if (!destination) {
      closePaymentWindow(paymentWindow)
      return false
    }
    paymentWindow.location.replace(destination)
    return true
  }
  if (display.kind === 'form' && display.formHtml) {
    const projection = parsePaymentFormHTML(display.formHtml, formParser)
    if (!projection) {
      closePaymentWindow(paymentWindow)
      return false
    }
    const form = paymentWindow.document.createElement('form')
    form.setAttribute('action', projection.action)
    form.setAttribute('method', projection.method)
    for (const field of projection.fields) {
      const input = paymentWindow.document.createElement('input')
      input.setAttribute('type', 'hidden')
      input.setAttribute('name', field.name)
      input.setAttribute('value', field.value)
      form.append(input)
    }
    paymentWindow.document.body.replaceChildren(form)
    if (form.requestSubmit) form.requestSubmit()
    else form.submit?.()
    return true
  }
  closePaymentWindow(paymentWindow)
  return false
}

export function parsePaymentFormHTML(formHtml: string, parser: PaymentFormParser = parsePaymentFormDocument): PaymentFormProjection | null {
  if (!formHtml.trim() || formHtml.length > maximumPaymentFormHTMLLength) return null
  let candidate: PaymentFormCandidate | null
  try {
    candidate = parser(formHtml)
  } catch {
    return null
  }
  if (!candidate || candidate.fields.length > maximumPaymentFormFields) return null
  const action = safePaymentURL(candidate.action)
  const method = candidate.method.trim().toLowerCase()
  if (!action || (method !== 'get' && method !== 'post')) return null
  const fields: PaymentFormField[] = []
  for (const field of candidate.fields) {
    const name = field.name.trim()
    if (!name || name.length > 256 || field.value.length > 65_536) return null
    fields.push({ name, value: field.value })
  }
  return { action, method, fields }
}

export function navigatePaymentWindow(paymentWindow: PaymentWindow | null, href: string, allowAppScheme = false) {
  if (!paymentWindow || paymentWindow.closed) return false
  const destination = safePaymentURL(href, allowAppScheme)
  if (!destination) {
    closePaymentWindow(paymentWindow)
    return false
  }
  paymentWindow.location.replace(destination)
  return true
}

export function closePaymentWindow(paymentWindow: PaymentWindow | null): void {
  if (paymentWindow && !paymentWindow.closed) paymentWindow.close()
}

function defaultOpenPaymentWindow(): PaymentWindow | null {
  return window.open('', '_blank') as unknown as PaymentWindow | null
}

function replacePaymentWindowContent(paymentWindow: PaymentWindow, message: string) {
  const status = paymentWindow.document.createElement('p')
  status.textContent = message
  status.setAttribute('role', 'status')
  paymentWindow.document.body.replaceChildren(status)
}

function parsePaymentFormDocument(formHtml: string): PaymentFormCandidate | null {
  const document = new DOMParser().parseFromString(formHtml, 'text/html')
  const form = document.querySelector('form')
  if (!form) return null
  const fields = Array.from(form.querySelectorAll('input')).flatMap((input) => {
    const name = input.getAttribute('name') ?? ''
    const type = (input.getAttribute('type') ?? 'text').trim().toLowerCase()
    if (['button', 'submit', 'reset', 'image', 'file'].includes(type)) return []
    if ((type === 'checkbox' || type === 'radio') && !input.checked) return []
    return [{ name, value: input.value }]
  })
  return {
    action: form.getAttribute('action') ?? '',
    method: form.getAttribute('method') ?? 'get',
    fields,
  }
}

function safePaymentURL(value: string, allowAppScheme = false) {
  try {
    const target = new URL(value)
    const loopbackHTTP = target.protocol === 'http:' && ['localhost', '127.0.0.1', '[::1]'].includes(target.hostname)
    const allowedProtocol = target.protocol === 'https:' || loopbackHTTP || (allowAppScheme && ['weixin:', 'alipays:', 'alipay:'].includes(target.protocol))
    if (!allowedProtocol || target.username || target.password) return null
    return target.href
  } catch {
    return null
  }
}
