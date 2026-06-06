import type { PaymentProviderType } from '../../../shared/api-types'

export type JeePayWayCodeTemplate = {
  way_code: string
  label: string
  description: string
  provider_types: PaymentProviderType[]
  config: Record<string, unknown>
}

export const jeepayWayCodeTemplates: JeePayWayCodeTemplate[] = [
  {
    way_code: 'ALI_PC',
    label: '支付宝 PC',
    description: '桌面浏览器跳转支付宝收银台。',
    provider_types: ['jeepay_alipay'],
    config: {
      payment_mode: 'api',
      way_code: 'ALI_PC',
    },
  },
  {
    way_code: 'ALI_JSAPI',
    label: '支付宝 JSAPI',
    description: '适合支付宝内网页支付，需要 buyerUserId。',
    provider_types: ['jeepay_alipay'],
    config: {
      payment_mode: 'api',
      way_code: 'ALI_JSAPI',
      channel_extra: {
        buyerUserId: '<alipay-user-id>',
      },
    },
  },
  {
    way_code: 'ALI_PC_SUB_MCH',
    label: '支付宝服务商',
    description: '支付宝服务商/子商户场景，补齐 appAuthToken 与子商户标识。',
    provider_types: ['jeepay_alipay'],
    config: {
      payment_mode: 'api',
      way_code: 'ALI_PC',
      channel_extra: {
        appAuthToken: '<app-auth-token>',
        subMerchantId: '<sub-merchant-id>',
      },
    },
  },
  {
    way_code: 'WX_NATIVE',
    label: '微信扫码',
    description: '返回二维码，适合桌面扫码支付。',
    provider_types: ['jeepay_wxpay'],
    config: {
      payment_mode: 'api',
      way_code: 'WX_NATIVE',
    },
  },
  {
    way_code: 'WX_JSAPI',
    label: '微信 JSAPI',
    description: '需要 openid，适合微信内网页支付。',
    provider_types: ['jeepay_wxpay'],
    config: {
      payment_mode: 'api',
      way_code: 'WX_JSAPI',
      channel_extra: {
        openid: '<user-openid>',
      },
    },
  },
  {
    way_code: 'WX_H5',
    label: '微信 H5',
    description: '适合普通移动浏览器拉起微信支付。',
    provider_types: ['jeepay_wxpay'],
    config: {
      payment_mode: 'api',
      way_code: 'WX_H5',
      channel_extra: {
        sceneInfo: {
          type: 'Wap',
          wap_url: 'https://your-domain.example',
          wap_name: 'Pic Gallery',
        },
      },
    },
  },
  {
    way_code: 'WX_LITE',
    label: '微信小程序',
    description: '小程序支付，需要小程序 appId 与用户 openId。',
    provider_types: ['jeepay_wxpay'],
    config: {
      payment_mode: 'api',
      way_code: 'WX_LITE',
      channel_extra: {
        appId: '<mini-program-app-id>',
        openId: '<mini-program-openid>',
      },
    },
  },
  {
    way_code: 'WX_NATIVE_SUB_MCH',
    label: '微信服务商',
    description: '微信服务商/子商户扫码支付，补齐子商户号和子应用信息。',
    provider_types: ['jeepay_wxpay'],
    config: {
      payment_mode: 'api',
      way_code: 'WX_NATIVE',
      channel_extra: {
        subMchId: '<sub-merchant-id>',
        subAppId: '<sub-app-id>',
      },
    },
  },
  {
    way_code: 'WX_NATIVE_PROFIT_SHARING',
    label: '微信分账',
    description: '扫码支付并声明分账接收方示例配置，适合先跑通测试链路。',
    provider_types: ['jeepay_wxpay'],
    config: {
      payment_mode: 'api',
      way_code: 'WX_NATIVE',
      channel_extra: {
        profitSharing: true,
        profitSharingReceivers: [
          {
            type: 'MERCHANT_ID',
            account: '<receiver-account>',
            amount: 1,
            description: 'Pic Gallery revenue share',
          },
        ],
      },
    },
  },
]

export function jeepayTemplatesForProvider(providerType: PaymentProviderType | string): JeePayWayCodeTemplate[] {
  return jeepayWayCodeTemplates.filter((template) => template.provider_types.includes(providerType as PaymentProviderType))
}

export function applyJeePayWayCodeTemplate(rawConfig: string, wayCode: string): string {
  const template = jeepayWayCodeTemplates.find((item) => item.way_code === wayCode)
  if (!template) throw new Error(`未知 JeePay wayCode 模板：${wayCode}`)
  const current = parseConfigText(rawConfig)
  return JSON.stringify(mergeConfigObjects(current, template.config), null, 2)
}

function parseConfigText(raw: string): Record<string, unknown> {
  const trimmed = raw.trim()
  if (!trimmed) return {}
  const parsed = JSON.parse(trimmed)
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('渠道配置必须是 JSON 对象')
  }
  return parsed as Record<string, unknown>
}

function mergeConfigObjects(base: Record<string, unknown>, patch: Record<string, unknown>): Record<string, unknown> {
  const merged: Record<string, unknown> = { ...base }
  for (const [key, patchValue] of Object.entries(patch)) {
    const baseValue = merged[key]
    if (isPlainRecord(baseValue) && isPlainRecord(patchValue)) {
      merged[key] = mergeConfigObjects(baseValue, patchValue)
    } else if (baseValue === undefined || baseValue === null || baseValue === '') {
      merged[key] = patchValue
    } else if (key === 'payment_mode' || key === 'way_code') {
      merged[key] = patchValue
    }
  }
  return merged
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}
