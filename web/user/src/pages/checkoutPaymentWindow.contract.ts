import fs from 'node:fs'
import type { CheckoutPaymentDisplayModel } from './checkoutPaymentDisplay'
import {
  closePaymentWindow,
  dispatchPaymentWindow,
  parsePaymentFormHTML,
  paymentMethodNeedsReservedWindow,
  reservePaymentWindow,
  type PaymentWindow,
} from './checkoutPaymentWindow'

const source = fs.readFileSync(new URL('./checkoutPaymentWindow.ts', import.meta.url), 'utf8')
if (!source.includes("window.open('', '_blank')") || source.includes("window.open('', '_blank', 'noopener,noreferrer')")) {
  throw new Error('payment reservation must retain its window handle before detaching opener access')
}
if (source.includes('document.write') || source.includes('.write(display.formHtml)')) {
  throw new Error('provider form HTML must never execute inside a same-origin payment window')
}
if (!source.includes("new DOMParser().parseFromString(formHtml, 'text/html')") || !source.includes("createElement('form')")) {
  throw new Error('provider form HTML must be parsed and rebuilt from an allowlisted projection')
}
if (paymentMethodNeedsReservedWindow({ method: 'stripe', label: 'Stripe', enabled: true, display_order: 1 })) {
  throw new Error('Stripe should stay in the payment monitor without opening a blank window')
}
if (paymentMethodNeedsReservedWindow({ method: 'mock', label: 'Mock', enabled: true, display_order: 2 })) {
  throw new Error('Mock payment should stay in the payment monitor without opening a blank window')
}
if (!paymentMethodNeedsReservedWindow({ method: 'alipay', label: '支付宝', enabled: true, display_order: 3 })) {
  throw new Error('redirect/form-capable methods must reserve a window during the user gesture')
}

const validProjection = parsePaymentFormHTML('<form></form>', () => ({
  action: 'https://pay.example.test/order',
  method: 'POST',
  fields: [{ name: 'order', value: '1' }],
}))
if (!validProjection || validProjection.action !== 'https://pay.example.test/order' || validProjection.method !== 'post' || validProjection.fields.length !== 1) {
  throw new Error(`safe provider form projection should be preserved, got ${JSON.stringify(validProjection)}`)
}
for (const action of ['javascript:alert(1)', 'data:text/html,boom', 'https://user:pass@pay.example.test/order', 'http://pay.example.test/order']) {
  if (parsePaymentFormHTML('<form></form>', () => ({ action, method: 'POST', fields: [] }))) {
    throw new Error(`unsafe payment form action must be rejected: ${action}`)
  }
}
if (!parsePaymentFormHTML('<form></form>', () => ({ action: 'http://127.0.0.1:8080/pay', method: 'POST', fields: [] }))) {
  throw new Error('loopback HTTP payment endpoints must remain available for local integration testing')
}

type FakePaymentWindow = PaymentWindow & {
  closedCount: number
  replacedWith: string
  statusText: string
  submittedCount: number
  submittedAction: string
  submittedMethod: string
  submittedFields: Array<{ name: string; value: string }>
}

function fakeWindow(): FakePaymentWindow {
  type FakeElement = {
    textContent: string | null
    attributes: Record<string, string>
    children: FakeElement[]
    setAttribute: (name: string, value: string) => void
    append: (...nodes: FakeElement[]) => void
    requestSubmit?: () => void
  }
  const result = {
    closed: false,
    closedCount: 0,
    replacedWith: '',
    statusText: '',
    submittedCount: 0,
    submittedAction: '',
    submittedMethod: '',
    submittedFields: [] as Array<{ name: string; value: string }>,
    opener: {} as Window,
    location: {
      replace(url: string) {
        result.replacedWith = url
      },
    },
    document: {
      body: {
        replaceChildren(...nodes: Array<{ textContent: string | null }>) {
          const first = nodes[0]
          if (first?.textContent) result.statusText = first.textContent
        },
      },
      createElement(tagName: string) {
        const element: FakeElement = {
          textContent: null,
          attributes: {},
          children: [],
          setAttribute(name: string, value: string) { element.attributes[name] = value },
          append(...nodes: FakeElement[]) { element.children.push(...nodes) },
        }
        if (tagName === 'form') {
          element.requestSubmit = () => {
            result.submittedCount += 1
            result.submittedAction = element.attributes.action ?? ''
            result.submittedMethod = element.attributes.method ?? ''
            result.submittedFields = element.children.map((child) => ({ name: child.attributes.name ?? '', value: child.attributes.value ?? '' }))
          }
        }
        return element
      },
    },
    close() {
      result.closed = true
      result.closedCount += 1
    },
  }
  return result as unknown as FakePaymentWindow
}

let openCount = 0
const reservedWindow = fakeWindow()
const reservation = reservePaymentWindow(() => {
  openCount += 1
  return reservedWindow
})
if (openCount !== 1 || reservation !== reservedWindow || reservedWindow.opener !== null || !reservedWindow.statusText.includes('正在创建订单')) {
  throw new Error('payment window should be reserved synchronously and detached from its opener')
}

const redirect: CheckoutPaymentDisplayModel = {
  kind: 'redirect',
  label: '跳转支付',
  detail: '测试',
  href: 'https://pay.example.test/checkout',
}
if (!dispatchPaymentWindow(reservation, redirect) || reservedWindow.replacedWith !== redirect.href || reservedWindow.closed) {
  throw new Error(`redirect should navigate the reserved window, got ${JSON.stringify(reservedWindow)}`)
}

const formWindow = fakeWindow()
const formHtml = '<form action="https://pay.example.test/order" method="post"><input name="order" value="1"></form>'
if (!dispatchPaymentWindow(formWindow, { kind: 'form', label: '表单支付', detail: '测试', formHtml }, () => ({
  action: 'https://pay.example.test/order',
  method: 'post',
  fields: [{ name: 'order', value: '1' }],
}))) {
  throw new Error('form payment should use the reserved window')
}
if (formWindow.submittedAction !== 'https://pay.example.test/order' || formWindow.submittedMethod !== 'post' || formWindow.submittedFields[0]?.name !== 'order' || formWindow.submittedCount !== 1 || formWindow.closed) {
  throw new Error(`form payment should rebuild and submit only allowlisted fields, got ${JSON.stringify(formWindow)}`)
}

for (const href of ['javascript:alert(1)', 'data:text/html,boom']) {
  const unsafe = fakeWindow()
  if (dispatchPaymentWindow(unsafe, { kind: 'redirect', label: '跳转', detail: '测试', href }) || unsafe.closedCount !== 1) {
    throw new Error(`unsafe redirect must close its reservation: ${href}`)
  }
}

for (const display of [
  { kind: 'qr_code', label: '扫码支付', detail: '测试', href: 'weixin://pay' },
  { kind: 'stripe', label: 'Stripe', detail: '测试', publishableKey: 'pk_test', clientSecret: 'secret' },
  { kind: 'mock', label: 'Mock', detail: '测试' },
  { kind: 'none', label: '无', detail: '测试' },
] satisfies CheckoutPaymentDisplayModel[]) {
  const unused = fakeWindow()
  if (dispatchPaymentWindow(unused, display) || unused.closedCount !== 1) {
    throw new Error(`${display.kind} should close an unused reservation`)
  }
}

const explicitlyClosed = fakeWindow()
closePaymentWindow(explicitlyClosed)
closePaymentWindow(explicitlyClosed)
if (explicitlyClosed.closedCount !== 1) {
  throw new Error(`closing a reservation should be idempotent, got ${explicitlyClosed.closedCount}`)
}

if (reservePaymentWindow(() => null) !== null) {
  throw new Error('popup blocker should be represented by a null reservation')
}
