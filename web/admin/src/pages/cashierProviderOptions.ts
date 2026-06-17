import type { PaymentProviderType } from '../../../shared/api-types'

export const cashierProviderLabels = {
  alipay_direct: '支付宝直连',
  wxpay_direct: '微信直连',
  easypay_alipay: '易支付 · 支付宝',
  easypay_wxpay: '易支付 · 微信',
  jeepay_alipay: 'JeePay · 支付宝',
  jeepay_wxpay: 'JeePay · 微信',
  mock: 'Mock 测试',
} as const satisfies Record<PaymentProviderType, string>

export const cashierProviderTypes: PaymentProviderType[] = ['mock', 'alipay_direct', 'wxpay_direct', 'easypay_alipay', 'easypay_wxpay', 'jeepay_alipay', 'jeepay_wxpay']

export const cashierProviderInstanceFieldHints = {
  sortOrder: '同一支付方式下排序越小越优先；排序相同再按调度策略选择实例。',
  schedulerWeight: '用于多实例调度，权重越高越容易被随机调度选中；轮询调度仍会按可用实例顺序切换。',
  minAmount: '订单金额低于该金额时不会选择此实例；为空表示不设置最低金额。',
  maxAmount: '订单金额高于该金额时不会选择此实例；为空表示不设置最高金额。',
  dailyLimit: '限制该实例当日累计收款金额；为空则不限制。',
} as const

export function cashierProviderLabel(providerType: PaymentProviderType | string) {
  return (cashierProviderLabels as Record<string, string>)[providerType] ?? providerType
}

export function cashierProviderTypesForMethod(method: string): PaymentProviderType[] {
  if (method === 'mock') return ['mock']
  if (method === 'wxpay') return ['wxpay_direct', 'easypay_wxpay', 'jeepay_wxpay', 'mock']
  return ['alipay_direct', 'easypay_alipay', 'jeepay_alipay', 'mock']
}

const supportedMethodLabels: Record<string, string> = {
  alipay: '支付宝',
  wxpay: '微信支付',
  mock: 'Mock 测试',
}

export type CashierSupportedMethodOption = {
  value: string
  label: string
  checked: boolean
}

export type CashierProviderConfigGuide = {
  title: string
  detail: string
  requiredFields: string[]
  optionalFields: string[]
  secretHint: string
}

export type CashierJeePayConfigField = {
  key: keyof CashierJeePayStructuredConfig
  label: string
  hint: string
  placeholder: string
  multiline?: boolean
}

export type CashierProviderConfigField = {
  key: string
  label: string
  hint: string
  placeholder?: string
  secret?: boolean
  required?: boolean
  multiline?: boolean
  options?: Array<{ value: string; label: string }>
}

export type CashierJeePayStructuredConfig = {
  gateway_url: string
  mch_no: string
  app_id: string
  key: string
  payment_mode: string
  way_code: string
  channel_extra_text: string
  raw_config_text: string
}

const defaultSecretHint = '密钥、私钥和证书类字段保存后不会回显明文，请在轮换时重新填写。'

const providerConfigGuides: Record<string, CashierProviderConfigGuide> = {
  mock: {
    title: 'Mock 测试配置',
    detail: '用于测试环境快速跑通下单、模拟支付、到账和流水链路，线上环境不会对用户展示。',
    requiredFields: ['mock'],
    optionalFields: ['query_status', 'query_trade_no', 'query_amount_cny'],
    secretHint: defaultSecretHint,
  },
  alipay_direct: {
    title: '支付宝直连配置',
    detail: '填写支付宝开放平台应用信息；沙箱验证可使用沙箱网关，回调地址默认由平台生成。',
    requiredFields: ['app_id', 'app_private_key', 'alipay_public_key'],
    optionalFields: ['gateway_url', 'notify_url', 'return_url'],
    secretHint: defaultSecretHint,
  },
  wxpay_direct: {
    title: '微信直连配置',
    detail: '支持 Native、H5、JSAPI 预下单；JSAPI 还需要 openid，回调验签需要微信支付公钥或平台证书配置。',
    requiredFields: ['app_id', 'mch_id', 'merchant_private_key', 'merchant_certificate_serial'],
    optionalFields: ['api_v3_key', 'wechat_pay_public_key', 'wechat_pay_public_key_id', 'payment_mode', 'openid', 'gateway_url'],
    secretHint: defaultSecretHint,
  },
  easypay_alipay: {
    title: '易支付配置',
    detail: '面向易支付支付宝通道，填写网关、商户 PID 和密钥后可生成跳转支付链接或 API 预下单。',
    requiredFields: ['gateway_url', 'pid', 'key'],
    optionalFields: ['payment_mode', 'notify_url', 'return_url'],
    secretHint: defaultSecretHint,
  },
  easypay_wxpay: {
    title: '易支付配置',
    detail: '面向易支付微信通道，填写网关、商户 PID 和密钥后可生成跳转支付链接或 API 预下单。',
    requiredFields: ['gateway_url', 'pid', 'key'],
    optionalFields: ['payment_mode', 'notify_url', 'return_url'],
    secretHint: defaultSecretHint,
  },
  jeepay_alipay: {
    title: 'JeePay 配置',
    detail: '填写 JeePay 网关、商户号、应用 ID、密钥和 way_code；可用模板补齐常见 wayCode 与 channel_extra。',
    requiredFields: ['gateway_url', 'mch_no', 'app_id', 'key', 'way_code'],
    optionalFields: ['payment_mode', 'notify_url', 'return_url', 'channel_extra'],
    secretHint: defaultSecretHint,
  },
  jeepay_wxpay: {
    title: 'JeePay 配置',
    detail: '填写 JeePay 网关、商户号、应用 ID、密钥和 way_code；可用模板补齐常见 wayCode 与 channel_extra。',
    requiredFields: ['gateway_url', 'mch_no', 'app_id', 'key', 'way_code'],
    optionalFields: ['payment_mode', 'notify_url', 'return_url', 'channel_extra'],
    secretHint: defaultSecretHint,
  },
}

export function cashierSupportedMethodLabel(method: string) {
  return supportedMethodLabels[method] ?? method
}

export function cashierProviderConfigGuide(providerType: PaymentProviderType | string): CashierProviderConfigGuide {
  return providerConfigGuides[providerType] ?? {
    title: '支付渠道配置',
    detail: '按渠道文档填写商户号、密钥、网关和回调参数，保存前请先在测试环境验证。',
    requiredFields: [],
    optionalFields: [],
    secretHint: defaultSecretHint,
  }
}

export function cashierProviderSupportedMethodOptions(providerType: PaymentProviderType | string, selectedMethods: string): CashierSupportedMethodOption[] {
  const available = methodsForProviderType(providerType)
  const selected = parseSupportedMethods(selectedMethods)
  return available.map((method) => ({
    value: method,
    label: cashierSupportedMethodLabel(method),
    checked: selected.includes(method),
  }))
}

export function cashierToggleSupportedMethod(selectedMethods: string, method: string, enabled: boolean) {
  const selected = parseSupportedMethods(selectedMethods)
  const next = enabled
    ? [...selected, method]
    : selected.filter((item) => item !== method)
  return Array.from(new Set(next)).join(', ')
}

const jeepayStructuredFields: CashierJeePayConfigField[] = [
  { key: 'gateway_url', label: '网关地址', hint: 'JeePay 服务网关，例如沙箱或正式环境支付网关。', placeholder: 'https://pay.example.com' },
  { key: 'mch_no', label: '商户号', hint: 'JeePay 商户号，用于下单、查单、退款和回调匹配。', placeholder: 'M1234567890' },
  { key: 'app_id', label: '应用 ID', hint: 'JeePay 应用 ID；微信小程序或 JSAPI 场景仍在渠道参数中补充用户标识。', placeholder: 'A1234567890' },
  { key: 'key', label: '商户密钥', hint: '保存后不会明文回显；轮换密钥时重新填写。', placeholder: '支付密钥' },
  { key: 'payment_mode', label: '支付模式', hint: 'API 预下单填 api；跳转收银台填 popup 或保持渠道要求。', placeholder: 'api' },
  { key: 'way_code', label: 'wayCode', hint: 'JeePay 通道编码。JeePay 场景模板可在 wayCode 字段旁选择：ALI_PC、ALI_JSAPI、WX_NATIVE、WX_H5、WX_JSAPI 等。', placeholder: 'WX_NATIVE' },
  { key: 'channel_extra_text', label: '渠道参数', hint: '填写 openid、buyerUserId、服务商子商户、分账接收方等通道专属 JSON。', placeholder: '{\n  \"openid\": \"user-openid\"\n}', multiline: true },
]

const paymentModeOptions = [
  { value: '', label: '默认' },
  { value: 'api', label: 'API 预下单' },
  { value: 'popup', label: '跳转/收银台' },
  { value: 'native', label: 'Native 扫码' },
  { value: 'h5', label: 'H5 支付' },
  { value: 'jsapi', label: 'JSAPI' },
]

const providerConfigFields: Record<string, CashierProviderConfigField[]> = {
  mock: [
    { key: 'mock_success', label: '默认支付结果', hint: '测试环境模拟支付是否默认成功；留空时后端按默认成功处理。', placeholder: 'true' },
    { key: 'mock_trade_no_prefix', label: '交易号前缀', hint: 'Mock 支付生成渠道交易号时使用的前缀。', placeholder: 'MOCK' },
  ],
  alipay_direct: [
    { key: 'gateway_url', label: '网关地址', hint: '支付宝正式或沙箱网关。', placeholder: 'https://openapi-sandbox.dl.alipaydev.com/gateway.do', required: true },
    { key: 'notify_url', label: '异步回调地址', hint: '平台内置支付结果通知地址；页面会展示当前部署域名下的推荐地址。', placeholder: 'https://example.com/api/open/image/v1/payments/webhooks/alipay_direct' },
    { key: 'return_url', label: '同步返回地址', hint: '用户支付完成后的浏览器跳转地址；页面会展示当前用户端域名下的推荐地址。', placeholder: 'https://example.com/checkout/return' },
    { key: 'payment_mode', label: '支付模式', hint: '沙箱验证常用 popup；API 通道按渠道要求选择。', options: paymentModeOptions },
    { key: 'app_id', label: '应用 ID', hint: '支付宝开放平台应用 app_id。', placeholder: '2021000000000000', secret: true, required: true },
    { key: 'app_private_key', label: '应用私钥', hint: '支付宝应用私钥，保存后不回显。', placeholder: '-----BEGIN PRIVATE KEY-----', secret: true, required: true, multiline: true },
    { key: 'alipay_public_key', label: '支付宝公钥', hint: '用于验签的支付宝公钥，保存后不回显。', placeholder: '-----BEGIN PUBLIC KEY-----', secret: true, required: true, multiline: true },
  ],
  wxpay_direct: [
    { key: 'gateway_url', label: '网关地址', hint: '微信支付 API 网关；为空时使用微信官方默认网关。', placeholder: 'https://api.mch.weixin.qq.com' },
    { key: 'notify_url', label: '异步回调地址', hint: '平台内置微信支付结果通知地址；页面会展示当前部署域名下的推荐地址。', placeholder: 'https://example.com/api/open/image/v1/payments/webhooks/wxpay_direct' },
    { key: 'return_url', label: '同步返回地址', hint: 'H5 支付完成后的返回地址；页面会展示当前用户端域名下的推荐地址。', placeholder: 'https://example.com/checkout/return' },
    { key: 'payment_mode', label: '支付模式', hint: 'Native 扫码、H5 或 JSAPI。', options: paymentModeOptions },
    { key: 'openid', label: 'OpenID', hint: 'JSAPI 场景使用的测试用户 OpenID。', placeholder: 'wx-openid-001' },
    { key: 'app_id', label: '应用 ID', hint: '微信支付 appid。', placeholder: 'wx1234567890abcdef', secret: true, required: true },
    { key: 'mch_id', label: '商户号', hint: '微信支付商户号。', placeholder: '1900000001', secret: true, required: true },
    { key: 'api_v3_key', label: 'API v3 密钥', hint: '微信支付 API v3 密钥，保存后不回显。', secret: true, required: true },
    { key: 'merchant_private_key', label: '商户私钥', hint: '商户 API 证书私钥 PEM。', secret: true, required: true, multiline: true },
    { key: 'merchant_certificate_serial', label: '商户证书序列号', hint: '商户 API 证书序列号。', secret: true, required: true },
    { key: 'wechat_pay_public_key', label: '微信支付公钥', hint: '用于验签的微信支付公钥或平台证书公钥。', secret: true, multiline: true },
    { key: 'wechat_pay_public_key_id', label: '微信支付公钥 ID', hint: '微信支付公钥 ID 或平台证书序列号。', secret: true },
  ],
  easypay_alipay: [
    { key: 'gateway_url', label: '网关地址', hint: '易支付 submit.php 或 API 网关地址。', placeholder: 'https://pay.example.com/submit.php', required: true },
    { key: 'notify_url', label: '异步回调地址', hint: '平台内置易支付异步通知地址；页面会展示当前部署域名下的推荐地址。', placeholder: 'https://example.com/api/open/image/v1/payments/webhooks/easypay_alipay' },
    { key: 'return_url', label: '同步返回地址', hint: '用户支付完成后的跳转地址；页面会展示当前用户端域名下的推荐地址。', placeholder: 'https://example.com/checkout/return' },
    { key: 'payment_mode', label: '支付模式', hint: 'popup 为跳转支付，api 为接口预下单。', options: paymentModeOptions },
    { key: 'pid', label: '商户 PID', hint: '易支付商户 PID。', secret: true, required: true },
    { key: 'key', label: '商户密钥', hint: '易支付签名密钥，保存后不回显。', secret: true, required: true },
  ],
  easypay_wxpay: [
    { key: 'gateway_url', label: '网关地址', hint: '易支付 submit.php 或 API 网关地址。', placeholder: 'https://pay.example.com/submit.php', required: true },
    { key: 'notify_url', label: '异步回调地址', hint: '平台内置易支付异步通知地址；页面会展示当前部署域名下的推荐地址。', placeholder: 'https://example.com/api/open/image/v1/payments/webhooks/easypay_wxpay' },
    { key: 'return_url', label: '同步返回地址', hint: '用户支付完成后的跳转地址；页面会展示当前用户端域名下的推荐地址。', placeholder: 'https://example.com/checkout/return' },
    { key: 'payment_mode', label: '支付模式', hint: 'popup 为跳转支付，api 为接口预下单。', options: paymentModeOptions },
    { key: 'pid', label: '商户 PID', hint: '易支付商户 PID。', secret: true, required: true },
    { key: 'key', label: '商户密钥', hint: '易支付签名密钥，保存后不回显。', secret: true, required: true },
  ],
  jeepay_alipay: [
    { key: 'gateway_url', label: '网关地址', hint: 'JeePay 服务网关。', placeholder: 'https://pay.example.com', required: true },
    { key: 'notify_url', label: '异步回调地址', hint: '平台内置 JeePay 支付结果通知地址；页面会展示当前部署域名下的推荐地址。', placeholder: 'https://example.com/api/open/image/v1/payments/webhooks/jeepay_alipay' },
    { key: 'return_url', label: '同步返回地址', hint: '用户支付完成后的跳转地址；页面会展示当前用户端域名下的推荐地址。', placeholder: 'https://example.com/checkout/return' },
    { key: 'payment_mode', label: '支付模式', hint: 'api 为统一下单接口，popup 为跳转收银台。', options: paymentModeOptions },
    { key: 'way_code', label: 'wayCode', hint: 'JeePay 通道编码。JeePay 场景模板：ALI_PC 支付宝 PC、ALI_JSAPI 支付宝 JSAPI、ALI_PC_SUB_MCH 服务商等，可在字段旁一键套用。', placeholder: 'ALI_PC', required: true },
    { key: 'channel_extra', label: '渠道参数 JSON', hint: '服务商、分账或行业参数。', placeholder: '{\n  "buyerUserId": "2088..."\n}', multiline: true },
    { key: 'mch_no', label: '商户号', hint: 'JeePay 商户号。', secret: true, required: true },
    { key: 'app_id', label: '应用 ID', hint: 'JeePay 应用 ID。', secret: true, required: true },
    { key: 'key', label: '商户密钥', hint: 'JeePay 签名密钥，保存后不回显。', secret: true, required: true },
  ],
  jeepay_wxpay: [
    { key: 'gateway_url', label: '网关地址', hint: 'JeePay 服务网关。', placeholder: 'https://pay.example.com', required: true },
    { key: 'notify_url', label: '异步回调地址', hint: '平台内置 JeePay 支付结果通知地址；页面会展示当前部署域名下的推荐地址。', placeholder: 'https://example.com/api/open/image/v1/payments/webhooks/jeepay_wxpay' },
    { key: 'return_url', label: '同步返回地址', hint: '用户支付完成后的跳转地址；页面会展示当前用户端域名下的推荐地址。', placeholder: 'https://example.com/checkout/return' },
    { key: 'payment_mode', label: '支付模式', hint: 'api 为统一下单接口，popup 为跳转收银台。', options: paymentModeOptions },
    { key: 'way_code', label: 'wayCode', hint: 'JeePay 通道编码。JeePay 场景模板：WX_NATIVE 微信扫码、WX_H5 微信 H5、WX_JSAPI 微信 JSAPI、WX_LITE 小程序等，可在字段旁一键套用。', placeholder: 'WX_NATIVE', required: true },
    { key: 'channel_extra', label: '渠道参数 JSON', hint: 'openid、服务商子商户或分账参数。', placeholder: '{\n  "openid": "wx-openid-001"\n}', multiline: true },
    { key: 'mch_no', label: '商户号', hint: 'JeePay 商户号。', secret: true, required: true },
    { key: 'app_id', label: '应用 ID', hint: 'JeePay 应用 ID。', secret: true, required: true },
    { key: 'key', label: '商户密钥', hint: 'JeePay 签名密钥，保存后不回显。', secret: true, required: true },
  ],
}

export function cashierProviderConfigFields(providerType: PaymentProviderType | string): CashierProviderConfigField[] {
  return providerConfigFields[providerType] ?? []
}

export function defaultCashierProviderConfigText(providerType: PaymentProviderType | string) {
  if (providerType === 'mock') return JSON.stringify({ mock: true }, null, 2)
  return '{}'
}

export function cashierJeePayConfigFields(providerType: PaymentProviderType | string): CashierJeePayConfigField[] {
  if (providerType !== 'jeepay_alipay' && providerType !== 'jeepay_wxpay') return []
  return jeepayStructuredFields
}

export function cashierJeePayStructuredConfig(rawConfig: string): CashierJeePayStructuredConfig {
  const config = parseConfigText(rawConfig)
  return {
    gateway_url: stringFromConfig(config.gateway_url),
    mch_no: stringFromConfig(config.mch_no),
    app_id: stringFromConfig(config.app_id),
    key: stringFromConfig(config.key),
    payment_mode: stringFromConfig(config.payment_mode),
    way_code: stringFromConfig(config.way_code),
    channel_extra_text: stringifyNestedConfig(config.channel_extra),
    raw_config_text: JSON.stringify(config, null, 2),
  }
}

export function updateCashierJeePayStructuredConfig(rawConfig: string, patch: Partial<Omit<CashierJeePayStructuredConfig, 'raw_config_text'>>): string {
  const config = parseConfigText(rawConfig)
  for (const key of ['gateway_url', 'mch_no', 'app_id', 'key', 'payment_mode', 'way_code'] as const) {
    if (!(key in patch)) continue
    const value = (patch[key] ?? '').trim()
    if (value) {
      config[key] = value
    } else {
      delete config[key]
    }
  }
  if ('channel_extra_text' in patch) {
    const rawChannelExtra = (patch.channel_extra_text ?? '').trim()
    if (rawChannelExtra) {
      let parsedChannelExtra: unknown
      try {
        parsedChannelExtra = JSON.parse(rawChannelExtra)
      } catch {
        throw new Error('渠道参数必须是 JSON 对象')
      }
      if (!isPlainRecord(parsedChannelExtra)) {
        throw new Error('渠道参数必须是 JSON 对象')
      }
      config.channel_extra = parsedChannelExtra
    } else {
      delete config.channel_extra
    }
  }
  return JSON.stringify(config, null, 2)
}

function methodsForProviderType(providerType: PaymentProviderType | string) {
  if (providerType === 'wxpay_direct' || providerType === 'easypay_wxpay' || providerType === 'jeepay_wxpay') return ['wxpay']
  if (providerType === 'mock') return ['mock']
  return ['alipay']
}

function parseSupportedMethods(methods: string) {
  return methods.split(',').map((item) => item.trim()).filter(Boolean)
}

function parseConfigText(rawConfig: string): Record<string, unknown> {
  const trimmed = rawConfig.trim()
  if (!trimmed) return {}
  const parsed = JSON.parse(trimmed)
  if (!isPlainRecord(parsed)) {
    throw new Error('渠道配置必须是 JSON 对象')
  }
  return parsed
}

function stringifyNestedConfig(value: unknown) {
  if (value === undefined || value === null || value === '') return ''
  if (isPlainRecord(value) || Array.isArray(value)) return JSON.stringify(value, null, 2)
  return String(value)
}

function stringFromConfig(value: unknown) {
  if (value === undefined || value === null) return ''
  return String(value)
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}
