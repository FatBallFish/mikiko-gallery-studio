import { existsSync, readFileSync } from 'node:fs'
import { checkoutPublicPaymentMethod } from './checkoutPaymentMethods'

const alipay = checkoutPublicPaymentMethod({ method: 'alipay', label: '', enabled: true, display_order: 10 })
if (alipay.label !== '支付宝' || alipay.icon !== 'alipay' || !alipay.detail.includes('支付宝')) {
  throw new Error(`alipay must use a public brand presentation: ${JSON.stringify(alipay)}`)
}

const wechat = checkoutPublicPaymentMethod({ method: 'wxpay', label: '微信支付', enabled: true, display_order: 20 })
if (wechat.label !== '微信支付' || wechat.icon !== 'wechat-pay' || !wechat.detail.includes('微信支付')) {
  throw new Error(`wechat pay must use a public brand presentation: ${JSON.stringify(wechat)}`)
}

const legacyInternal = checkoutPublicPaymentMethod({
  method: 'alipay', label: 'JeePay 支付宝 渠道', enabled: true, display_order: 10,
  source_provider_type: 'jeepay_alipay', description: '内部实例 A',
} as never)
if (/jeepay|easypay|渠道|实例/i.test(`${legacyInternal.label} ${legacyInternal.detail}`)) {
  throw new Error(`legacy provider metadata must never reach visible payment copy: ${JSON.stringify(legacyInternal)}`)
}

for (const asset of ['../assets/payment/alipay.svg', '../assets/payment/wechat-pay.svg', '../assets/payment/stripe.svg']) {
  if (!existsSync(new URL(asset, import.meta.url))) throw new Error(`missing local payment brand asset ${asset}`)
}

const checkoutSource = readFileSync(new URL('./CheckoutPage.tsx', import.meta.url), 'utf8')
for (const marker of ['checkoutPublicPaymentMethod', 'PaymentMethodBrandIcon', 'grid-cols-2']) {
  if (!checkoutSource.includes(marker)) throw new Error(`checkout payment grid must use ${marker}`)
}
if (/method\.method\.includes\(['"](?:wx|ali)/.test(checkoutSource)) {
  throw new Error('payment branding must not be guessed with generic Lucide icons')
}
