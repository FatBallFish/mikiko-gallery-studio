import type { PaymentProviderInstance, PaymentProviderInstanceWriteRequest, PaymentProviderType } from '../../../shared/api-types'

export type CashierProviderFormDraft = {
  row?: PaymentProviderInstance
  provider_type: PaymentProviderType
  name: string
  enabled: boolean
  supported_methods: string
  sort_order: string
  scheduler_weight: string
  min_amount_cny: string
  max_amount_cny: string
  daily_amount_limit_cny: string
  config: Record<string, unknown>
  secrets: Record<string, string>
  callback_bases: { notify_url: string; return_url: string }
  original_callbacks: { notify_url: string; return_url: string }
  original_callback_bases: { notify_url: string; return_url: string }
}

export function newProviderDraft(providerType: PaymentProviderType, origin = browserOrigin()): CashierProviderFormDraft {
  const normalizedOrigin = normalizeCallbackBase(origin)
  return {
    provider_type: providerType,
    name: '',
    enabled: true,
    supported_methods: defaultMethod(providerType),
    sort_order: '10',
    scheduler_weight: '100',
    min_amount_cny: '1.00000',
    max_amount_cny: '999.00000',
    daily_amount_limit_cny: '',
    config: providerType === 'mock' ? { mock_success: 'true' } : {},
    secrets: {},
    callback_bases: { notify_url: normalizedOrigin, return_url: normalizedOrigin },
    original_callbacks: { notify_url: '', return_url: '' },
    original_callback_bases: { notify_url: normalizedOrigin, return_url: normalizedOrigin },
  }
}

export function providerDraftFromInstance(row: PaymentProviderInstance, origin = browserOrigin()): CashierProviderFormDraft {
  const config = { ...(row.config ?? {}) }
  const notifyURL = stringValue(config.notify_url)
  const returnURL = stringValue(config.return_url)
  const notifyBase = callbackBaseFromStoredURL(notifyURL, origin)
  const returnBase = callbackBaseFromStoredURL(returnURL, origin)
  delete config.notify_url
  delete config.return_url
  return {
    row,
    provider_type: row.provider_type,
    name: row.name,
    enabled: Boolean(row.enabled),
    supported_methods: row.supported_methods.join(', '),
    sort_order: String(row.sort_order ?? 0),
    scheduler_weight: String(row.scheduler_weight ?? 100),
    min_amount_cny: row.limits?.min_amount_cny ?? '',
    max_amount_cny: row.limits?.max_amount_cny ?? '',
    daily_amount_limit_cny: row.limits?.daily_amount_limit_cny ?? '',
    config,
    secrets: {},
    callback_bases: { notify_url: notifyBase, return_url: returnBase },
    original_callbacks: { notify_url: notifyURL, return_url: returnURL },
    original_callback_bases: { notify_url: notifyBase, return_url: returnBase },
  }
}

export function providerDraftForTypeChange(draft: CashierProviderFormDraft, providerType: PaymentProviderType): CashierProviderFormDraft {
  const reset = newProviderDraft(providerType, draft.callback_bases.notify_url || draft.callback_bases.return_url)
  return {
    ...reset,
    name: draft.name,
    enabled: draft.enabled,
    sort_order: draft.sort_order,
    scheduler_weight: draft.scheduler_weight,
    min_amount_cny: draft.min_amount_cny,
    max_amount_cny: draft.max_amount_cny,
    daily_amount_limit_cny: draft.daily_amount_limit_cny,
  }
}

export function providerPayloadFromDraft(draft: CashierProviderFormDraft): PaymentProviderInstanceWriteRequest {
  const config = { ...draft.config }
  if (typeof config.channel_extra === 'string') {
    const raw = config.channel_extra.trim()
    if (!raw) delete config.channel_extra
    else {
      try {
        const parsed: unknown = JSON.parse(raw)
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('not an object')
        config.channel_extra = parsed
      } catch {
        throw new Error('渠道参数必须是 JSON 对象')
      }
    }
  }
  if (draft.provider_type !== 'mock') {
    for (const key of ['notify_url', 'return_url'] as const) {
      const base = normalizeCallbackBase(draft.callback_bases[key])
      if (draft.row && base === draft.original_callback_bases[key] && draft.original_callbacks[key]) {
        config[key] = draft.original_callbacks[key]
      } else if (base) {
        config[key] = callbackURLFromBase(base, key, draft.provider_type)
      }
    }
  }
  const secrets = Object.fromEntries(Object.entries(draft.secrets).filter(([, value]) => value.trim() !== ''))
  const payload: PaymentProviderInstanceWriteRequest = {
    provider_type: draft.provider_type,
    name: draft.name,
    enabled: draft.enabled,
    supported_methods: draft.supported_methods.split(',').map((item) => item.trim()).filter(Boolean),
    sort_order: Number(draft.sort_order) || 0,
    scheduler_weight: Number(draft.scheduler_weight) || 100,
    limits: {
      min_amount_cny: draft.min_amount_cny,
      max_amount_cny: draft.max_amount_cny,
      daily_amount_limit_cny: draft.daily_amount_limit_cny || undefined,
    },
    config,
  }
  if (Object.keys(secrets).length) payload.secrets = secrets
  return payload
}

export function callbackBaseFromStoredURL(storedURL: string, fallbackOrigin = browserOrigin()): string {
  try {
    return normalizeCallbackBase(new URL(storedURL).origin)
  } catch {
    return normalizeCallbackBase(fallbackOrigin)
  }
}

export function callbackURLFromBase(base: string, key: 'notify_url' | 'return_url', providerType: PaymentProviderType): string {
  const normalized = normalizeCallbackBase(base)
  if (!normalized) return ''
  return key === 'notify_url'
    ? `${normalized}/api/open/image/v1/payments/webhooks/${providerType}`
    : `${normalized}/#/checkout`
}

export function normalizeCallbackBase(value: string): string {
  const trimmed = value.trim().replace(/\/+$/, '')
  if (!trimmed) return ''
  try {
    const url = new URL(trimmed)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') throw new Error('unsupported protocol')
    return url.origin
  } catch {
    throw new Error('回调基础域名必须是有效的 HTTP 或 HTTPS 地址')
  }
}

function browserOrigin(): string {
  return typeof window === 'undefined' ? '' : window.location.origin
}

function defaultMethod(providerType: PaymentProviderType): string {
  if (providerType === 'mock') return 'mock'
  if (providerType === 'stripe') return 'stripe'
  if (providerType.includes('wxpay')) return 'wxpay'
  return 'alipay'
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value : ''
}
