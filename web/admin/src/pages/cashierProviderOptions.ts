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
  configJSON: '填写商户号、网关地址、支付模式和渠道参数；密钥不会回显明文，JeePay 可用上方模板补齐 wayCode 和 channelExtra。',
} as const

export function cashierProviderLabel(providerType: PaymentProviderType | string) {
  return (cashierProviderLabels as Record<string, string>)[providerType] ?? providerType
}

export function cashierProviderTypesForMethod(method: string): PaymentProviderType[] {
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
    optionalFields: ['payment_mode', 'notify_url', 'return_url', 'client_ip'],
    secretHint: defaultSecretHint,
  },
  easypay_wxpay: {
    title: '易支付配置',
    detail: '面向易支付微信通道，填写网关、商户 PID 和密钥后可生成跳转支付链接或 API 预下单。',
    requiredFields: ['gateway_url', 'pid', 'key'],
    optionalFields: ['payment_mode', 'notify_url', 'return_url', 'client_ip'],
    secretHint: defaultSecretHint,
  },
  jeepay_alipay: {
    title: 'JeePay 配置',
    detail: '填写 JeePay 网关、商户号、应用 ID、密钥和 way_code；可用模板补齐常见 wayCode 与 channel_extra。',
    requiredFields: ['gateway_url', 'mch_no', 'app_id', 'key', 'way_code'],
    optionalFields: ['payment_mode', 'notify_url', 'return_url', 'client_ip', 'channel_extra'],
    secretHint: defaultSecretHint,
  },
  jeepay_wxpay: {
    title: 'JeePay 配置',
    detail: '填写 JeePay 网关、商户号、应用 ID、密钥和 way_code；可用模板补齐常见 wayCode 与 channel_extra。',
    requiredFields: ['gateway_url', 'mch_no', 'app_id', 'key', 'way_code'],
    optionalFields: ['payment_mode', 'notify_url', 'return_url', 'client_ip', 'channel_extra'],
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

function methodsForProviderType(providerType: PaymentProviderType | string) {
  if (providerType === 'wxpay_direct' || providerType === 'easypay_wxpay' || providerType === 'jeepay_wxpay') return ['wxpay']
  if (providerType === 'mock') return ['mock']
  return ['alipay']
}

function parseSupportedMethods(methods: string) {
  return methods.split(',').map((item) => item.trim()).filter(Boolean)
}
