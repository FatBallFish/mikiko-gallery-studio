// @ts-ignore contract scripts run in tsx/node; browser tsconfigs do not include node types.
import assert from 'node:assert/strict'
import { ApiError, errorMessage } from './http-client'

const originalNavigator = Object.getOwnPropertyDescriptor(globalThis, 'navigator')

function setLanguage(language: string) {
  Object.defineProperty(globalThis, 'navigator', {
    configurable: true,
    value: { language },
  })
}

try {
  const error = new ApiError('payment provider instance is unavailable', 502, 'PAYMENT_PROVIDER_UNAVAILABLE')

  setLanguage('zh-CN')
  assert.equal(errorMessage(error), '支付渠道暂时不可用，请稍后重试。')

  setLanguage('en-US')
  assert.equal(errorMessage(error), 'The payment channel is temporarily unavailable. Please try again later.')
} finally {
  if (originalNavigator) Object.defineProperty(globalThis, 'navigator', originalNavigator)
  else delete (globalThis as { navigator?: unknown }).navigator
}

console.log('OK: payment provider error localization contract passed')
