import { adminCurrencyOptions, normalizeAdminCurrency } from './currency'

for (const currency of ['CNY', 'USD', 'HKD', 'JPY', 'EUR', 'USDT']) {
  if (!adminCurrencyOptions.includes(currency)) {
    throw new Error(`admin currency options should include common currency ${currency}`)
  }
}

if (normalizeAdminCurrency(' usd ') !== 'USD') {
  throw new Error('admin currency should be trimmed and uppercased before saving')
}

if (normalizeAdminCurrency('custom_token') !== 'CUSTOM_TOKEN') {
  throw new Error('admin currency normalization should preserve custom values instead of rejecting them')
}
