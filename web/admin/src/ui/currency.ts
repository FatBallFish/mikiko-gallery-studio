export const adminCurrencyOptions = ['CNY', 'USD', 'HKD', 'JPY', 'EUR', 'GBP', 'SGD', 'USDT']

export function normalizeAdminCurrency(value: string) {
  return value.trim().toUpperCase()
}
