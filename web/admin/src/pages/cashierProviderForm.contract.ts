import { cashierProviderConfigFields } from './cashierProviderOptions'
import { callbackBaseFromStoredURL, callbackURLFromBase, providerDraftFromInstance, providerPayloadFromDraft } from './cashierProviderForm'
// @ts-ignore contract scripts run in tsx/node; the admin app does not depend on Node types.
import { readFileSync } from 'node:fs'

function expectStorage(provider: string, key: string, storage: 'config' | 'secret') {
  const field = cashierProviderConfigFields(provider).find((item) => item.key === key)
  if (!field || field.storage !== storage) {
    throw new Error(`${provider}.${key} storage should be ${storage}, got ${JSON.stringify(field)}`)
  }
}

for (const [provider, key] of [
  ['jeepay_alipay', 'mch_no'], ['jeepay_alipay', 'app_id'], ['alipay_direct', 'app_id'],
  ['wxpay_direct', 'app_id'], ['wxpay_direct', 'mch_id'], ['easypay_alipay', 'pid'],
] as const) expectStorage(provider, key, 'config')
for (const [provider, key] of [['jeepay_alipay', 'key'], ['alipay_direct', 'app_private_key'], ['wxpay_direct', 'merchant_private_key']] as const) expectStorage(provider, key, 'secret')

const origin = 'https://gallery.example.com'
if (callbackURLFromBase(origin, 'notify_url', 'jeepay_wxpay') !== `${origin}/api/open/image/v1/payments/webhooks/jeepay_wxpay`) {
  throw new Error('notify callback base did not expand to the provider webhook route')
}
if (callbackURLFromBase(origin, 'return_url', 'jeepay_wxpay') !== `${origin}/#/checkout`) {
  throw new Error('return callback base did not expand to checkout')
}
if (callbackBaseFromStoredURL(`${origin}/legacy/provider-return`, origin) !== origin) {
  throw new Error('stored callback URL did not project to its origin')
}

const existing = providerDraftFromInstance({
  id: 9,
  provider_type: 'jeepay_alipay',
  name: 'JeePay',
  enabled: true,
  supported_methods: ['alipay'],
  sort_order: 1,
  scheduler_weight: 100,
  config: { gateway_url: 'https://pay.example.com', mch_no: 'M1', app_id: 'A1', notify_url: `${origin}/legacy-notify`, return_url: `${origin}/legacy-return` },
}, origin)
const unchanged = providerPayloadFromDraft(existing)
if (unchanged.config?.notify_url !== `${origin}/legacy-notify` || unchanged.config?.return_url !== `${origin}/legacy-return`) {
  throw new Error(`untouched legacy callback paths must be preserved: ${JSON.stringify(unchanged.config)}`)
}
if (unchanged.secrets !== undefined) {
  throw new Error('empty edit secrets must not be sent')
}

const changed = providerPayloadFromDraft({ ...existing, callback_bases: { notify_url: 'https://new.example.com', return_url: 'https://new.example.com' } })
if (changed.config?.notify_url !== 'https://new.example.com/api/open/image/v1/payments/webhooks/jeepay_alipay' || changed.config?.return_url !== 'https://new.example.com/#/checkout') {
  throw new Error(`changed callback bases did not expand: ${JSON.stringify(changed.config)}`)
}

const cashierSource = readFileSync(new URL('./CashierPage.tsx', import.meta.url), 'utf8')
for (const hiddenLabel of ['密钥 JSON', '清空密钥字段', '渠道配置 JSON']) {
  if (cashierSource.includes(hiddenLabel)) throw new Error(`cashier provider editor must not expose ${hiddenLabel}`)
}
for (const visibleContract of ['field.required ? \'*\' : \'（选填）\'', 'aria-required={field.required}', "field.kind === 'password'"]) {
  if (!cashierSource.includes(visibleContract)) throw new Error(`cashier provider editor must preserve ${visibleContract}`)
}
