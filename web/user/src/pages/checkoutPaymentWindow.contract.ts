import fs from 'node:fs'
import type { CheckoutPaymentDisplayModel } from './checkoutPaymentDisplay'
import {
  closePaymentWindow,
  dispatchPaymentWindow,
  reservePaymentWindow,
  type PaymentWindow,
} from './checkoutPaymentWindow'

const source = fs.readFileSync(new URL('./checkoutPaymentWindow.ts', import.meta.url), 'utf8')
if (!source.includes("window.open('', '_blank')") || source.includes("window.open('', '_blank', 'noopener,noreferrer')")) {
  throw new Error('payment reservation must retain its window handle before detaching opener access')
}

type FakePaymentWindow = PaymentWindow & {
  closedCount: number
  replacedWith: string
  writtenHTML: string
  submittedCount: number
}

function fakeWindow(): FakePaymentWindow {
  const result = {
    closed: false,
    closedCount: 0,
    replacedWith: '',
    writtenHTML: '',
    submittedCount: 0,
    opener: {} as Window,
    location: {
      replace(url: string) {
        result.replacedWith = url
      },
    },
    document: {
      open() {},
      write(html: string) {
        result.writtenHTML = html
      },
      close() {},
      querySelector(selector: string) {
        if (selector !== 'form' || !result.writtenHTML.includes('<form')) return null
        return {
          requestSubmit() {
            result.submittedCount += 1
          },
        }
      },
    },
    close() {
      result.closed = true
      result.closedCount += 1
    },
  }
  return result
}

let openCount = 0
const reservedWindow = fakeWindow()
const reservation = reservePaymentWindow(() => {
  openCount += 1
  return reservedWindow
})
if (openCount !== 1 || reservation !== reservedWindow || reservedWindow.opener !== null) {
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
if (!dispatchPaymentWindow(formWindow, { kind: 'form', label: '表单支付', detail: '测试', formHtml })) {
  throw new Error('form payment should use the reserved window')
}
if (formWindow.writtenHTML !== formHtml || formWindow.submittedCount !== 1 || formWindow.closed) {
  throw new Error(`form payment should write and submit the channel form, got ${JSON.stringify(formWindow)}`)
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
