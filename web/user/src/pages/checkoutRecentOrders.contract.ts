import type { PaymentOrder } from '../../../shared/api-types'
import { checkoutDateTime, checkoutPaymentMethodOptionModel, checkoutRecentOrderRows } from './checkoutOrderState'

const baseOrder: PaymentOrder = {
  id: 1,
  order_no: 'ord_1',
  plan_id: 1,
  plan_code: 'points-100',
  plan_name: '100 积分包',
  provider: 'mock',
  purchase_type: 'plan',
  visible_method: 'mock',
  status: 'completed',
  currency: 'CNY',
  amount_cny: '19.90000',
  points: '100.00000',
  bonus_points: '0.00000',
  credit_expiry_enabled: true,
  credit_valid_days: 30,
  credited_at: '2026-06-05T10:05:00Z',
  credit_expires_at: '2026-07-05T10:05:00Z',
  expires_at: '2026-06-05T12:00:00Z',
  created_at: '2026-06-05T10:00:00Z',
  updated_at: '2026-06-05T10:00:00Z',
}

const rows = checkoutRecentOrderRows(Array.from({ length: 12 }, (_, index) => ({
  ...baseOrder,
  id: index + 1,
  order_no: `ord_${index + 1}`,
  plan_name: index === 0 ? '' : `套餐 ${index + 1}`,
  purchase_type: index === 0 ? 'custom_amount' : 'plan',
  amount_cny: `${index + 1}.00000`,
  created_at: `2026-06-05T10:${String(index).padStart(2, '0')}:00Z`,
})))

if (rows.length !== 10) {
  throw new Error(`recent checkout orders should show at most 10 rows, got ${rows.length}`)
}

if (rows[0].orderNo !== 'ord_12' || rows[9].orderNo !== 'ord_3') {
  throw new Error(`recent checkout orders should sort newest first, got ${rows.map((row) => row.orderNo).join(',')}`)
}

const customRow = rows.find((row) => row.orderNo === 'ord_1')
if (customRow) {
  throw new Error('oldest row should be truncated before custom row assertion')
}

const oneRow = checkoutRecentOrderRows([{ ...baseOrder, plan_name: '', purchase_type: 'custom_amount', amount_cny: '25.00000' }])[0]
if (oneRow.title !== '自定义金额充值' || oneRow.amount !== '¥25.00' || oneRow.points !== '100.00') {
  throw new Error(`recent checkout order row should normalize custom amount display, got ${JSON.stringify(oneRow)}`)
}

const fixedRow = checkoutRecentOrderRows([{ ...baseOrder, points: '100.00000', bonus_points: '20.00000' }])[0]
if (fixedRow.basePoints !== '100.00' || fixedRow.bonusPoints !== '20.00' || fixedRow.creditValidity !== '有效期至 2026/07/05 10:05') {
  throw new Error(`recent fixed-package order must expose base, gift, and actual expiry, got ${JSON.stringify(fixedRow)}`)
}

const pendingRow = checkoutRecentOrderRows([{ ...baseOrder, status: 'pending', credited_at: null, credit_expires_at: null }])[0]
if (pendingRow.creditValidity !== '到账后 30 天内有效') {
  throw new Error(`pending fixed-package order must explain relative validity, got ${JSON.stringify(pendingRow)}`)
}

const permanentRow = checkoutRecentOrderRows([{ ...baseOrder, credit_expiry_enabled: false, credit_valid_days: null, credited_at: '2026-06-05T10:05:00Z', credit_expires_at: null }])[0]
if (permanentRow.creditValidity !== '积分长期有效') {
  throw new Error(`permanent fixed-package order must explain non-expiry, got ${JSON.stringify(permanentRow)}`)
}

if (oneRow.createdAtLabel !== '2026/06/05 10:00') {
  throw new Error(`recent checkout order row should expose localized created time label, got ${JSON.stringify(oneRow)}`)
}

if (checkoutDateTime('2026-06-05T12:34:56Z') !== '2026/06/05 12:34') {
  throw new Error(`checkout datetime should format ISO strings without raw T/Z markers, got ${checkoutDateTime('2026-06-05T12:34:56Z')}`)
}

if (checkoutDateTime('not-a-date') !== 'not-a-date' || checkoutDateTime('') !== '-') {
  throw new Error('checkout datetime should preserve invalid raw values and fallback empty values')
}

const statusCases: Array<[string, string]> = [
  ['pending', '待支付'],
  ['paid', '已到账'],
  ['completed', '已到账'],
  ['canceled', '已取消'],
  ['cancelled', '已取消'],
  ['closed', '已关闭'],
  ['failed', '支付失败'],
  ['expired', '已过期'],
  ['refunded', '已退款'],
  ['partially_refunded', '部分退款'],
  ['manual_review', 'manual_review'],
]

for (const [status, expected] of statusCases) {
  const row = checkoutRecentOrderRows([{ ...baseOrder, status }])[0]
  if (row.status !== expected) {
    throw new Error(`recent checkout order status ${status} should display as ${expected}, got ${row.status}`)
  }
}

const methodCases: Array<[Partial<PaymentOrder>, string]> = [
  [{ visible_method: 'mock', provider: 'mock', provider_type: 'mock' }, 'Mock 测试'],
  [{ visible_method: 'alipay', provider: '', provider_type: 'alipay_direct' }, '支付宝'],
  [{ visible_method: 'wxpay', provider: '', provider_type: 'wxpay_direct' }, '微信支付'],
  [{ visible_method: '', provider: 'easypay_alipay', provider_type: 'easypay_alipay' }, '易支付支付宝'],
  [{ visible_method: '', provider: 'easypay_wxpay', provider_type: 'easypay_wxpay' }, '易支付微信'],
  [{ visible_method: '', provider: 'jeepay_alipay', provider_type: 'jeepay_alipay' }, 'JeePay 支付宝'],
  [{ visible_method: '', provider: 'jeepay_wxpay', provider_type: 'jeepay_wxpay' }, 'JeePay 微信'],
  [{ visible_method: '', provider: 'manual_alipay', provider_type: 'manual_alipay' }, '人工确认 · 支付宝'],
  [{ visible_method: '', provider: 'manual_wxpay', provider_type: 'manual_wxpay' }, '人工确认 · 微信支付'],
  [{ visible_method: '', provider: 'manual_bank', provider_type: 'manual_bank' }, '人工确认 · 银行转账'],
  [{ visible_method: '', provider: '', provider_type: '' }, '-'],
]

for (const [orderFields, expected] of methodCases) {
  const row = checkoutRecentOrderRows([{ ...baseOrder, ...orderFields }])[0]
  if (row.method !== expected) {
    throw new Error(`recent checkout order method ${JSON.stringify(orderFields)} should display as ${expected}, got ${row.method}`)
  }
}

const alipayMethod = checkoutPaymentMethodOptionModel({ method: 'alipay', label: '支付宝', enabled: true, source_provider_type: 'alipay_direct', display_order: 1 })
if (alipayMethod.label !== '支付宝' || !alipayMethod.detail.includes('支付宝') || alipayMethod.rawMethod !== 'alipay') {
  throw new Error(`checkout payment method option should keep raw method and display user-facing label/detail, got ${JSON.stringify(alipayMethod)}`)
}
if (/^alipay$|^wxpay$|^mock$/.test(alipayMethod.detail)) {
  throw new Error(`checkout payment method detail should not expose raw method as visible helper text, got ${JSON.stringify(alipayMethod)}`)
}

const wxpayMethod = checkoutPaymentMethodOptionModel({ method: 'wxpay', label: '', enabled: true, source_provider_type: 'jeepay_wxpay', display_order: 2 })
if (wxpayMethod.label !== '微信支付' || !wxpayMethod.detail.includes('JeePay 微信')) {
  throw new Error(`checkout wxpay option should fallback to localized method/provider labels, got ${JSON.stringify(wxpayMethod)}`)
}

const mockMethod = checkoutPaymentMethodOptionModel({ method: 'mock', label: '', enabled: true, source_provider_type: 'mock', display_order: 3 })
if (mockMethod.label !== 'Mock 测试' || !mockMethod.detail.includes('测试环境')) {
  throw new Error(`checkout mock option should clearly mark test environment usage, got ${JSON.stringify(mockMethod)}`)
}

const customMethod = checkoutPaymentMethodOptionModel({ method: 'bank_transfer', label: '', enabled: true, source_provider_type: 'manual_bank', display_order: 4 })
if (customMethod.label !== 'bank_transfer' || customMethod.detail !== '人工确认 · 银行转账 渠道') {
  throw new Error(`checkout visible method option should localize known manual provider details while preserving custom visible method labels, got ${JSON.stringify(customMethod)}`)
}
