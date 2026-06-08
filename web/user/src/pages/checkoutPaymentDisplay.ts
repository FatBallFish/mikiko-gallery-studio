import type { CashierOrder, PaymentDisplay } from '../../../shared/api-types'

export type CheckoutPaymentDisplayModel = {
  kind: 'qr_code' | 'redirect' | 'form' | 'mock' | 'unsupported' | 'none'
  label: string
  detail: string
  href?: string
  formHtml?: string
}

export function checkoutPaymentDisplayModel(order: CashierOrder): CheckoutPaymentDisplayModel {
  const display = normalizePaymentDisplay(order)
  if (!display) {
    return {
      kind: 'none',
      label: '等待支付信息',
      detail: '订单已创建，若支付信息未返回，请刷新订单或重新创建。',
    }
  }
  if (display.type === 'jsapi') {
    return {
      kind: 'unsupported',
      label: '微信内支付需在微信环境打开',
      detail: '当前浏览器无法调起微信 JSAPI，请返回支付方式，改选 H5 或扫码支付后重新创建订单。',
    }
  }
  if (display.type === 'mock') {
    return {
      kind: 'mock',
      label: 'Mock 测试支付',
      detail: '当前订单使用测试支付方式，可点击“模拟支付成功”完成联调。',
    }
  }
  if (display.type === 'qr_code' && (display.qr_code || display.payment_url)) {
    return {
      kind: 'qr_code',
      label: '扫码支付',
      detail: '请使用对应支付 App 打开或扫码完成付款。',
      href: display.qr_code ?? display.payment_url,
    }
  }
  if (display.type === 'redirect' && (display.payment_url || display.qr_code)) {
    return {
      kind: 'redirect',
      label: '跳转支付',
      detail: '将在新窗口打开渠道支付页，付款完成后回到本页刷新订单。',
      href: display.payment_url ?? display.qr_code,
    }
  }
  if ((display.type === 'form' || display.type === 'form_html') && display.form_html) {
    return {
      kind: 'form',
      label: '表单支付',
      detail: '渠道返回了支付表单，请在新窗口完成支付。',
      formHtml: display.form_html,
    }
  }
  return {
    kind: 'none',
    label: '支付信息需要刷新',
    detail: '请刷新订单；若仍无法展示，请返回支付方式并改选其他渠道后重新创建订单。',
  }
}

function normalizePaymentDisplay(order: CashierOrder): PaymentDisplay | null {
  if (order.payment_display) return order.payment_display
  if (order.qr_code) return { type: 'qr_code', qr_code: order.qr_code }
  if (order.payment_url) return { type: 'redirect', payment_url: order.payment_url }
  if (order.client_token) return { type: 'jsapi', client_token: order.client_token }
  return null
}
