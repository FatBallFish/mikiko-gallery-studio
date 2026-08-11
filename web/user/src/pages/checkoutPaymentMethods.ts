import type { PublicPaymentVisibleMethod } from '../../../shared/api-types'

export type CheckoutPaymentBrand = 'alipay' | 'wechat-pay' | 'stripe' | 'mock' | 'generic'

export type CheckoutPublicPaymentMethod = {
  rawMethod: string
  label: string
  detail: string
  icon: CheckoutPaymentBrand
}
const INTERNAL_PAYMENT_COPY = /jeepay|easypay|direct|manual|round.?robin|channel|provider|渠道|实例/i

export function checkoutPublicPaymentMethod(method: PublicPaymentVisibleMethod): CheckoutPublicPaymentMethod {
  const rawMethod = method.method.trim().toLowerCase()
  if (rawMethod === 'alipay' || rawMethod.includes('alipay')) {
    return { rawMethod: method.method, label: '支付宝', detail: '使用支付宝完成支付', icon: 'alipay' }
  }
  if (rawMethod === 'wxpay' || rawMethod.includes('wxpay') || rawMethod.includes('wechat')) {
    return { rawMethod: method.method, label: '微信支付', detail: '使用微信支付完成付款', icon: 'wechat-pay' }
  }
  if (rawMethod === 'stripe') {
    return { rawMethod: method.method, label: '银行卡', detail: '使用银行卡安全支付', icon: 'stripe' }
  }
  if (rawMethod === 'mock') {
    return { rawMethod: method.method, label: '测试支付', detail: '仅用于测试环境', icon: 'mock' }
  }
  const configuredLabel = method.label.trim()
  const label = configuredLabel && !INTERNAL_PAYMENT_COPY.test(configuredLabel) ? configuredLabel : '在线支付'
  return { rawMethod: method.method, label, detail: '通过安全支付页面完成付款', icon: 'generic' }
}
