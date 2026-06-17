import { cashierJeePayConfigFields, cashierJeePayStructuredConfig, cashierProviderConfigFields, cashierProviderConfigGuide, cashierProviderInstanceFieldHints, cashierProviderLabel, cashierProviderLabels, cashierProviderSupportedMethodOptions, cashierProviderTypesForMethod, cashierSupportedMethodLabel, cashierToggleSupportedMethod, defaultCashierProviderConfigText, updateCashierJeePayStructuredConfig } from './cashierProviderOptions'

type ContainsPlaceholder<T extends string> = T extends `${string}占位${string}` | `${string}placeholder${string}` | `${string}Placeholder${string}` ? true : false
type AssertNoPlaceholder<T extends false> = T

type _JeePayAlipayLabelIsConcrete = AssertNoPlaceholder<ContainsPlaceholder<typeof cashierProviderLabels.jeepay_alipay>>
type _JeePayWxPayLabelIsConcrete = AssertNoPlaceholder<ContainsPlaceholder<typeof cashierProviderLabels.jeepay_wxpay>>

const alipayProviders = cashierProviderTypesForMethod('alipay')
const wxpayProviders = cashierProviderTypesForMethod('wxpay')

if (!alipayProviders.includes('jeepay_alipay') || !wxpayProviders.includes('jeepay_wxpay')) {
  throw new Error(`cashier provider options should expose JeePay adapters, got alipay=${alipayProviders.join(',')} wxpay=${wxpayProviders.join(',')}`)
}

for (const providerType of ['jeepay_alipay', 'jeepay_wxpay'] as const) {
  const label = cashierProviderLabel(providerType)
  if (label.includes('占位') || label.toLowerCase().includes('placeholder')) {
    throw new Error(`JeePay provider ${providerType} should no longer be described as a placeholder, got ${label}`)
  }
}

const alipayMethodOptions = cashierProviderSupportedMethodOptions('alipay_direct', 'alipay')
if (alipayMethodOptions.length !== 1 || alipayMethodOptions[0]?.value !== 'alipay' || alipayMethodOptions[0]?.label !== '支付宝' || alipayMethodOptions[0]?.checked !== true) {
  throw new Error(`alipay provider should expose an operator-facing supported method option, got ${JSON.stringify(alipayMethodOptions)}`)
}

const wxpayMethodOptions = cashierProviderSupportedMethodOptions('jeepay_wxpay', '')
if (wxpayMethodOptions.length !== 1 || wxpayMethodOptions[0]?.value !== 'wxpay' || wxpayMethodOptions[0]?.label !== '微信支付' || wxpayMethodOptions[0]?.checked !== false) {
  throw new Error(`wxpay provider should expose an operator-facing supported method option, got ${JSON.stringify(wxpayMethodOptions)}`)
}

const mockMethodOptions = cashierProviderSupportedMethodOptions('mock', 'mock')
if (mockMethodOptions.length !== 1 || mockMethodOptions[0]?.value !== 'mock' || mockMethodOptions[0]?.label !== 'Mock 测试' || mockMethodOptions[0]?.checked !== true) {
  throw new Error(`mock provider should expose an operator-facing supported method option, got ${JSON.stringify(mockMethodOptions)}`)
}

if (defaultCashierProviderConfigText('mock') !== '{\n  "mock": true\n}') {
  throw new Error(`mock provider should keep a runnable default config, got ${defaultCashierProviderConfigText('mock')}`)
}
for (const providerType of ['alipay_direct', 'wxpay_direct', 'easypay_alipay', 'easypay_wxpay', 'jeepay_alipay', 'jeepay_wxpay'] as const) {
  if (defaultCashierProviderConfigText(providerType) !== '{}') {
    throw new Error(`${providerType} default config should be empty to avoid carrying stale fields across provider switches, got ${defaultCashierProviderConfigText(providerType)}`)
  }
}

for (const option of [...alipayMethodOptions, ...wxpayMethodOptions, ...mockMethodOptions]) {
  if (/\b(alipay|wxpay|mock)\b/.test(option.label)) {
    throw new Error(`supported method labels should not expose raw method values, got ${JSON.stringify(option)}`)
  }
}

if (cashierSupportedMethodLabel('bank_transfer') !== 'bank_transfer') {
  throw new Error('unknown supported methods should preserve raw values for troubleshooting')
}

const toggledOn = cashierToggleSupportedMethod('alipay', 'wxpay', true)
if (toggledOn !== 'alipay, wxpay') {
  throw new Error(`supported method toggle should append raw value without losing existing methods, got ${toggledOn}`)
}

const toggledOff = cashierToggleSupportedMethod('alipay, wxpay', 'alipay', false)
if (toggledOff !== 'wxpay') {
  throw new Error(`supported method toggle should remove raw value and preserve remaining methods, got ${toggledOff}`)
}

const requiredHints = {
  sortOrder: ['越小越优先', '同一支付方式'],
  schedulerWeight: ['多实例调度', '权重越高'],
  minAmount: ['低于该金额', '不会选择'],
  maxAmount: ['高于该金额', '不会选择'],
  dailyLimit: ['为空则不限制', '当日累计'],
} as const

for (const [key, fragments] of Object.entries(requiredHints)) {
  const hint = cashierProviderInstanceFieldHints[key as keyof typeof cashierProviderInstanceFieldHints]
  if (!hint) {
    throw new Error(`cashier provider instance field hint ${key} should exist`)
  }
  for (const fragment of fragments) {
    if (!hint.includes(fragment)) {
      throw new Error(`cashier provider instance hint ${key} should include ${fragment}, got ${hint}`)
    }
  }
}

const allHints = Object.values(cashierProviderInstanceFieldHints).join(' ')
if (/scheduler_weight|sort_order|daily_amount_limit|supported_methods|config_text/.test(allHints)) {
  throw new Error(`cashier provider instance hints should be operator-facing, got ${allHints}`)
}

const guideExpectations = [
  {
    provider: 'alipay_direct',
    title: '支付宝直连配置',
    required: ['app_id', 'app_private_key', 'alipay_public_key'],
    detail: ['沙箱', '回调'],
  },
  {
    provider: 'wxpay_direct',
    title: '微信直连配置',
    required: ['app_id', 'mch_id', 'merchant_private_key', 'merchant_certificate_serial'],
    detail: ['Native', 'H5', 'JSAPI'],
  },
  {
    provider: 'easypay_alipay',
    title: '易支付配置',
    required: ['gateway_url', 'pid', 'key'],
    detail: ['支付宝'],
  },
  {
    provider: 'jeepay_wxpay',
    title: 'JeePay 配置',
    required: ['gateway_url', 'mch_no', 'app_id', 'key', 'way_code'],
    detail: ['模板', 'channel_extra'],
  },
  {
    provider: 'mock',
    title: 'Mock 测试配置',
    required: ['mock'],
    detail: ['测试环境'],
  },
] as const

for (const expectation of guideExpectations) {
  const guide = cashierProviderConfigGuide(expectation.provider)
  if (guide.title !== expectation.title) {
    throw new Error(`provider ${expectation.provider} guide title should be ${expectation.title}, got ${guide.title}`)
  }
  for (const field of expectation.required) {
    if (!guide.requiredFields.includes(field)) {
      throw new Error(`provider ${expectation.provider} guide should list required field ${field}, got ${JSON.stringify(guide)}`)
    }
  }
  for (const fragment of expectation.detail) {
    if (!guide.detail.includes(fragment)) {
      throw new Error(`provider ${expectation.provider} guide detail should include ${fragment}, got ${guide.detail}`)
    }
  }
  if (!guide.secretHint.includes('密钥') || !guide.secretHint.includes('不会回显')) {
    throw new Error(`provider ${expectation.provider} guide should explain secret redaction, got ${guide.secretHint}`)
  }
}

const guideVisibleCopy = guideExpectations.map((expectation) => {
  const guide = cashierProviderConfigGuide(expectation.provider)
  return `${guide.title} ${guide.detail} ${guide.requiredFields.join(' ')} ${guide.optionalFields.join(' ')} ${guide.secretHint}`
}).join(' ')

if (/占位|placeholder|后续|暂未|即将|版本/i.test(guideVisibleCopy)) {
  throw new Error(`provider config guides should be actionable and avoid roadmap/placeholder wording, got ${guideVisibleCopy}`)
}

const jeepayFields = cashierJeePayConfigFields('jeepay_wxpay')
for (const expected of ['网关地址', '商户号', '应用 ID', '支付模式', 'wayCode', '渠道参数']) {
  if (!jeepayFields.some((field) => field.label === expected)) {
    throw new Error(`JeePay structured fields should include ${expected}, got ${JSON.stringify(jeepayFields)}`)
  }
}
if (jeepayFields.some((field) => String(field.key) === 'client_ip' || field.label === '客户端 IP')) {
  throw new Error(`JeePay structured fields should not expose client_ip because backend has a safe fallback, got ${JSON.stringify(jeepayFields)}`)
}
if (cashierJeePayConfigFields('mock').length !== 0) {
  throw new Error('non-JeePay providers should not show JeePay structured fields')
}

const structured = cashierJeePayStructuredConfig(JSON.stringify({
  gateway_url: 'https://pay.example.com',
  mch_no: 'M123',
  app_id: 'A456',
  key: 'secret',
  payment_mode: 'api',
  way_code: 'WX_NATIVE',
  client_ip: '127.0.0.1',
  channel_extra: { profitSharing: true },
  notify_url: 'https://pic.example.com/notify',
}, null, 2))

if (
  structured.gateway_url !== 'https://pay.example.com'
  || structured.mch_no !== 'M123'
  || structured.app_id !== 'A456'
  || structured.payment_mode !== 'api'
  || structured.way_code !== 'WX_NATIVE'
  || structured.channel_extra_text !== '{\n  "profitSharing": true\n}'
) {
  throw new Error(`JeePay structured config should parse known fields, got ${JSON.stringify(structured)}`)
}
if (!structured.raw_config_text.includes('"notify_url"')) {
  throw new Error(`JeePay structured config should preserve raw config text for troubleshooting, got ${structured.raw_config_text}`)
}

const updatedConfig = updateCashierJeePayStructuredConfig(JSON.stringify({
  gateway_url: 'https://pay.example.com',
  mch_no: 'M123',
  app_id: 'A456',
  key: 'secret',
  way_code: 'WX_NATIVE',
  channel_extra: { profitSharing: true },
  notify_url: 'https://pic.example.com/notify',
}, null, 2), {
  gateway_url: 'https://new-pay.example.com',
  way_code: 'WX_JSAPI',
  channel_extra_text: '{\n  "openid": "user-openid"\n}',
})
const updated = JSON.parse(updatedConfig)
if (updated.gateway_url !== 'https://new-pay.example.com' || updated.way_code !== 'WX_JSAPI' || updated.client_ip !== undefined || updated.notify_url !== 'https://pic.example.com/notify') {
  throw new Error(`JeePay structured config should update known fields and preserve unknown fields, got ${updatedConfig}`)
}
if (updated.channel_extra?.openid !== 'user-openid') {
  throw new Error(`JeePay channel_extra should be parsed from structured textarea, got ${updatedConfig}`)
}

let invalidChannelExtraFailed = false
try {
  updateCashierJeePayStructuredConfig('{}', { channel_extra_text: 'not-json' })
} catch (caught) {
  invalidChannelExtraFailed = caught instanceof Error && caught.message.includes('渠道参数')
}
if (!invalidChannelExtraFailed) {
  throw new Error('invalid JeePay channel_extra text should fail with operator-facing message')
}

const structuredVisibleCopy = jeepayFields.map((field) => `${field.label} ${field.hint} ${field.placeholder}`).join(' ')
if (/gateway_url|mch_no|app_id|client_ip|channel_extra|后续|暂未|即将|版本/.test(structuredVisibleCopy)) {
  throw new Error(`JeePay structured field copy should be operator-facing, got ${structuredVisibleCopy}`)
}

for (const providerType of ['wxpay_direct', 'easypay_alipay', 'easypay_wxpay', 'jeepay_alipay', 'jeepay_wxpay'] as const) {
  const fields = cashierProviderConfigFields(providerType)
  if (fields.some((field) => field.key === 'client_ip')) {
    throw new Error(`${providerType} should not expose client_ip in payment instance form, got ${JSON.stringify(fields)}`)
  }
  const guide = cashierProviderConfigGuide(providerType)
  if (guide.optionalFields.includes('client_ip')) {
    throw new Error(`${providerType} guide should not ask operators to fill client_ip, got ${JSON.stringify(guide)}`)
  }
}

const jeepayWayCode = cashierProviderConfigFields('jeepay_wxpay').find((field) => field.key === 'way_code')
if (!jeepayWayCode?.hint.includes('JeePay 场景模板') || !jeepayWayCode.hint.includes('WX_NATIVE')) {
  throw new Error(`JeePay way_code hint should carry scenario template guidance, got ${JSON.stringify(jeepayWayCode)}`)
}

for (const providerType of ['alipay_direct', 'wxpay_direct', 'easypay_alipay', 'jeepay_wxpay'] as const) {
  const fields = cashierProviderConfigFields(providerType)
  const notify = fields.find((field) => field.key === 'notify_url')
  if (!notify?.hint.includes('平台内置')) {
    throw new Error(`${providerType} notify_url hint should point to the built-in callback URL, got ${JSON.stringify(notify)}`)
  }
  const returned = fields.find((field) => field.key === 'return_url')
  if (!returned?.hint.includes('页面会展示')) {
    throw new Error(`${providerType} return_url hint should explain the page-suggested return URL, got ${JSON.stringify(returned)}`)
  }
}
