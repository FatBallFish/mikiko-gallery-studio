import type { CashierOrder } from '../../../shared/api-types'
import { checkoutPaymentDisplayModel } from './checkoutPaymentDisplay'

const baseOrder: CashierOrder = {
  id: 1,
  order_no: 'pay_1',
  purchase_type: 'custom_amount',
  visible_method: 'wxpay',
  status: 'pending',
  currency: 'CNY',
  amount_cny: '10.00000',
  points: '32.00000',
  bonus_points: '0.00000',
  expires_at: '2026-06-05T10:00:00Z',
  created_at: '2026-06-05T09:30:00Z',
  updated_at: '2026-06-05T09:30:00Z',
}

const qrModel = checkoutPaymentDisplayModel({
  ...baseOrder,
  payment_display: { type: 'qr_code', qr_code: 'weixin://wxpay/bizpayurl?pr=test' },
})

if (qrModel.kind !== 'qr_code' || qrModel.href !== 'weixin://wxpay/bizpayurl?pr=test') {
  throw new Error(`qr_code payment display should expose qr href, got ${JSON.stringify(qrModel)}`)
}

const redirectModel = checkoutPaymentDisplayModel({
  ...baseOrder,
  payment_display: { type: 'redirect', payment_url: 'https://pay.example.test/checkout' },
})

if (redirectModel.kind !== 'redirect' || redirectModel.href !== 'https://pay.example.test/checkout') {
  throw new Error(`redirect payment display should expose payment url, got ${JSON.stringify(redirectModel)}`)
}

const formHtml = '<form action="https://pay.example.test/form" method="post"></form>'
const formHtmlModel = checkoutPaymentDisplayModel({
  ...baseOrder,
  payment_display: { type: 'form_html', form_html: formHtml },
})

if (formHtmlModel.kind !== 'form' || formHtmlModel.formHtml !== formHtml || formHtmlModel.href) {
  throw new Error(`form_html payment display should preserve channel form html without a direct href, got ${JSON.stringify(formHtmlModel)}`)
}

const legacyFormModel = checkoutPaymentDisplayModel({
  ...baseOrder,
  payment_display: { type: 'form', form_html: formHtml },
})

if (legacyFormModel.kind !== 'form' || legacyFormModel.formHtml !== formHtml || legacyFormModel.href) {
  throw new Error(`legacy form payment display should remain compatible, got ${JSON.stringify(legacyFormModel)}`)
}

const jsapiModel = checkoutPaymentDisplayModel({
  ...baseOrder,
  payment_display: { type: 'jsapi', client_token: '{"prepay_id":"wx-prepay"}' },
})

if (jsapiModel.kind !== 'unsupported' || !jsapiModel.detail.includes('H5') || jsapiModel.href) {
  throw new Error(`jsapi payment display should be an unsupported first-version state, got ${JSON.stringify(jsapiModel)}`)
}

if (/当前版本|暂不支持|后续|版本|暂未|即将/.test(`${jsapiModel.label}${jsapiModel.detail}`)) {
  throw new Error(`jsapi payment display should give current actionable guidance without roadmap wording, got ${JSON.stringify(jsapiModel)}`)
}

const mockModel = checkoutPaymentDisplayModel({
  ...baseOrder,
  payment_display: { type: 'mock', payment_url: 'mock://checkout/pay_1' },
})

if (mockModel.kind !== 'mock' || !mockModel.label.includes('Mock') || !mockModel.detail.includes('模拟支付成功') || mockModel.href) {
  throw new Error(`mock payment display should guide in-page mock pay without exposing mock:// href, got ${JSON.stringify(mockModel)}`)
}

const stripeModel = checkoutPaymentDisplayModel({
  ...baseOrder,
  visible_method: 'stripe',
  payment_display: {
    type: 'stripe_payment_element',
    client_secret: 'pi_contract_secret_client',
    publishable_key: 'pk_test_contract',
  },
})
if (stripeModel.kind !== 'stripe' || stripeModel.clientSecret !== 'pi_contract_secret_client' || stripeModel.publishableKey !== 'pk_test_contract') {
  throw new Error(`Stripe payment display should expose Payment Element config, got ${JSON.stringify(stripeModel)}`)
}

const malformedStripeModel = checkoutPaymentDisplayModel({
  ...baseOrder,
  visible_method: 'stripe',
  payment_display: { type: 'stripe_payment_element', publishable_key: 'pk_test_contract' },
})
if (malformedStripeModel.kind !== 'unsupported' || !malformedStripeModel.detail.includes('配置不完整')) {
  throw new Error(`incomplete Stripe display should expose unsupported configuration state, got ${JSON.stringify(malformedStripeModel)}`)
}

const emptyModel = checkoutPaymentDisplayModel(baseOrder)

if (emptyModel.kind !== 'none') {
  throw new Error(`missing payment display should render an explicit empty state, got ${JSON.stringify(emptyModel)}`)
}

if (/后续|版本|暂未|即将/.test(`${emptyModel.label}${emptyModel.detail}`)) {
  throw new Error(`missing payment display should give current actionable guidance without weak wording, got ${JSON.stringify(emptyModel)}`)
}

const malformedDisplayModel = checkoutPaymentDisplayModel({
  ...baseOrder,
  payment_display: { type: 'qr_code' },
})

if (malformedDisplayModel.kind !== 'none') {
  throw new Error(`malformed payment display should render an explicit none state, got ${JSON.stringify(malformedDisplayModel)}`)
}

if (/后续|版本|暂未|即将/.test(`${malformedDisplayModel.label}${malformedDisplayModel.detail}`)) {
  throw new Error(`malformed payment display should give current actionable guidance without weak wording, got ${JSON.stringify(malformedDisplayModel)}`)
}
