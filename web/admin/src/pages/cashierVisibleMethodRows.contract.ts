import { cashierVisibleMethodRow, cashierVisibleMethodSchedulerLabel } from './cashierVisibleMethodRows'

const alipayRow = cashierVisibleMethodRow({
  method: 'alipay',
  label: '支付宝',
  enabled: true,
  source_provider_type: 'jeepay_alipay',
  scheduler_strategy: 'round_robin',
  display_order: 1,
})

if (alipayRow.title !== '支付宝入口' || alipayRow.detail !== '支付宝') {
  throw new Error(`cashier visible method row should expose operator-facing title/detail, got ${JSON.stringify(alipayRow)}`)
}

if (alipayRow.rawMethod !== 'alipay' || alipayRow.rawProviderType !== 'jeepay_alipay') {
  throw new Error(`cashier visible method row should preserve raw contract values, got ${JSON.stringify(alipayRow)}`)
}

if (alipayRow.providerLabel !== 'JeePay · 支付宝' || alipayRow.schedulerLabel !== '轮询调度') {
  throw new Error(`cashier visible method row should localize provider and scheduler labels, got ${JSON.stringify(alipayRow)}`)
}

if (alipayRow.permission !== 'cashier.visible_methods.write') {
  throw new Error(`cashier visible method row should reserve future RBAC permission hook, got ${JSON.stringify(alipayRow)}`)
}

const wxpayRow = cashierVisibleMethodRow({
  method: 'wxpay',
  label: '',
  enabled: false,
  source_provider_type: 'wxpay_direct',
  scheduler_strategy: 'random',
  display_order: 2,
})

if (wxpayRow.title !== '微信支付入口' || wxpayRow.detail !== 'wxpay' || wxpayRow.schedulerLabel !== '随机调度') {
  throw new Error(`cashier visible method row should fall back to localized method title and raw detail, got ${JSON.stringify(wxpayRow)}`)
}

const mockRow = cashierVisibleMethodRow({
  method: 'mock',
  label: '',
  enabled: true,
  source_provider_type: 'mock',
  scheduler_strategy: '',
  display_order: 3,
})

if (mockRow.title !== '测试支付入口' || mockRow.schedulerLabel !== '轮询调度') {
  throw new Error(`cashier visible method row should localize mock and default scheduler, got ${JSON.stringify(mockRow)}`)
}

const customRow = cashierVisibleMethodRow({
  method: 'bank_transfer',
  label: '',
  enabled: true,
  source_provider_type: 'manual_bank',
  scheduler_strategy: 'weighted',
  display_order: 4,
})

if (customRow.title !== 'bank_transfer' || customRow.providerLabel !== 'manual_bank' || customRow.schedulerLabel !== 'weighted') {
  throw new Error(`cashier visible method row should keep unknown values raw for troubleshooting, got ${JSON.stringify(customRow)}`)
}

if (cashierVisibleMethodSchedulerLabel('round_robin') !== '轮询调度' || cashierVisibleMethodSchedulerLabel('random') !== '随机调度') {
  throw new Error('cashier visible method scheduler labels should localize known strategies')
}
