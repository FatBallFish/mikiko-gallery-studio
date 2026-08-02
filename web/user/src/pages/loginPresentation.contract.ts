import {
  loginProviders,
  loginPresentation,
  firstLoginInvalidField,
  loginCooldownForEmail,
  normalizeLoginEmail,
  nextLoginFlow,
  nextLoginModeForKey,
  validateLoginFields,
  type LoginPresentationInput,
} from './loginPresentation'
import type { SendEmailCodeRequest } from '../../../shared/api-types'
import { loginCopy } from './loginCopy'

function state(overrides: Partial<LoginPresentationInput> = {}): LoginPresentationInput {
  return {
    mode: 'password',
    intent: 'login',
    busy: false,
    sending: false,
    cooldown: 0,
    ...overrides,
  }
}

const password = loginPresentation(state())
if (password.title !== '欢迎回来' || password.submitLabel !== '进入创作台' || !password.showPassword) {
  throw new Error(`password login presentation drifted: ${JSON.stringify(password)}`)
}

const code = loginPresentation(state({ mode: 'code' }))
if (code.title !== '邮箱免密登录' || code.codeScene !== 'login' || code.showPassword || !code.showCode) {
  throw new Error(`email-code login presentation drifted: ${JSON.stringify(code)}`)
}

const register = loginPresentation(state({ mode: 'code', intent: 'register' }))
const backendRegistrationScene: NonNullable<SendEmailCodeRequest['scene']> = register.codeScene
if (register.title !== '创建你的账户' || backendRegistrationScene !== 'login' || register.submitLabel !== '注册并进入') {
  throw new Error(`registration presentation drifted: ${JSON.stringify(register)}`)
}

const reset = loginPresentation(state({ mode: 'code', intent: 'reset' }))
if (reset.title !== '重置密码' || reset.codeScene !== 'password_reset' || !reset.showNewPassword || reset.submitLabel !== '确认重置') {
  throw new Error(`password reset presentation drifted: ${JSON.stringify(reset)}`)
}

const passwordSetup = loginPresentation(state({ mode: 'code', passwordSetupRequired: true }))
if (passwordSetup.title !== '创建登录密码' || !passwordSetup.showNewPassword || !passwordSetup.showPasswordConfirmation || passwordSetup.showCode || passwordSetup.submitLabel !== '设置密码并进入') {
  throw new Error(`mandatory password setup presentation drifted: ${JSON.stringify(passwordSetup)}`)
}

const sending = loginPresentation(state({ mode: 'code', sending: true }))
if (sending.sendCodeLabel !== '发送中...' || !sending.sendCodeDisabled) {
  throw new Error(`sending state is not explicit: ${JSON.stringify(sending)}`)
}

const cooldown = loginPresentation(state({ mode: 'code', cooldown: 42 }))
if (cooldown.sendCodeLabel !== '42 秒后重发' || !cooldown.sendCodeDisabled) {
  throw new Error(`code cooldown state drifted: ${JSON.stringify(cooldown)}`)
}

const submitting = loginPresentation(state({ busy: true }))
if (submitting.submitLabel !== '正在登录...' || !submitting.submitDisabled) {
  throw new Error(`submit busy state is not explicit: ${JSON.stringify(submitting)}`)
}

const emptyErrors = validateLoginFields({ mode: 'password', intent: 'login', email: '', password: '', code: '', newPassword: '' })
if (!emptyErrors.email || !emptyErrors.password) throw new Error('password login must report local email and password errors')

const invalidEmail = validateLoginFields({ mode: 'code', intent: 'login', email: 'not-an-email', password: '', code: '123', newPassword: '' })
if (!invalidEmail.email || !invalidEmail.code) throw new Error('email-code login must validate email and six-digit code locally')

const resetErrors = validateLoginFields({ mode: 'code', intent: 'reset', email: 'studio@example.com', password: '', code: '123456', newPassword: '123' })
if (!resetErrors.newPassword) throw new Error('password reset must validate the new password locally')

for (const newPassword of ['123456', '1234567', ' 1234567 ']) {
  const shortReset = validateLoginFields({ mode: 'code', intent: 'reset', email: 'studio@example.com', password: '', code: '123456', newPassword })
  if (!shortReset.newPassword) throw new Error(`reset password must reject fewer than 8 trimmed characters: ${JSON.stringify(newPassword)}`)
}
const validReset = validateLoginFields({ mode: 'code', intent: 'reset', email: 'studio@example.com', password: '', code: '123456', newPassword: '12345678' })
if (validReset.newPassword) throw new Error(`eight-character reset password was rejected: ${validReset.newPassword}`)
if (!loginCopy.zh.resetPasswordPlaceholder.includes('8')) throw new Error('reset password copy must state the 8-character requirement')

const valid = validateLoginFields({ mode: 'code', intent: 'register', email: 'studio@example.com', password: '', code: '123456', newPassword: '' })
if (Object.keys(valid).length !== 0) throw new Error(`valid registration fields were rejected: ${JSON.stringify(valid)}`)

if (!loginProviders.length || loginProviders.some((provider) => !provider.disabled || !provider.label.includes('暂未开放'))) {
  throw new Error(`unsupported third-party providers must be disabled and clearly unavailable: ${JSON.stringify(loginProviders)}`)
}

const setupMismatch = validateLoginFields({ mode: 'code', intent: 'login', passwordSetupRequired: true, email: '', password: '', code: '', newPassword: 'password-123', confirmPassword: 'password-456' })
if (!setupMismatch.confirmPassword || setupMismatch.email || setupMismatch.code) {
  throw new Error(`password setup must only validate password fields, got ${JSON.stringify(setupMismatch)}`)
}
const validSetup = validateLoginFields({ mode: 'code', intent: 'login', passwordSetupRequired: true, email: '', password: '', code: '', newPassword: 'password-123', confirmPassword: 'password-123' })
if (Object.keys(validSetup).length !== 0) throw new Error(`valid password setup was rejected: ${JSON.stringify(validSetup)}`)

const resetTransition = nextLoginFlow('reset', 37)
if (resetTransition.mode !== 'code' || resetTransition.intent !== 'reset' || resetTransition.cooldown !== 37 || resetTransition.codeSent) {
  throw new Error(`changing auth purpose must preserve the delivery cooldown: ${JSON.stringify(resetTransition)}`)
}

for (const [current, key, expected] of [
  ['password', 'ArrowRight', 'code'],
  ['code', 'ArrowRight', 'password'],
  ['code', 'ArrowLeft', 'password'],
  ['password', 'ArrowLeft', 'code'],
  ['code', 'Home', 'password'],
  ['password', 'End', 'code'],
] as const) {
  if (nextLoginModeForKey(current, key) !== expected) {
    throw new Error(`unexpected auth tab keyboard target for ${current} + ${key}`)
  }
}
if (nextLoginModeForKey('password', 'Enter') !== null) throw new Error('unhandled tab keys must not change mode')

if (firstLoginInvalidField({ password: 'required', email: 'required' }, 'password', 'login') !== 'email') {
  throw new Error('password validation must focus email before password')
}
if (firstLoginInvalidField({ code: 'required', newPassword: 'required' }, 'code', 'reset') !== 'code') {
  throw new Error('reset validation must focus code before new password')
}
if (firstLoginInvalidField({}, 'code', 'register') !== null) throw new Error('valid fields must not produce a focus target')

if (normalizeLoginEmail('  Studio@Example.COM ') !== 'studio@example.com') {
  throw new Error('login email normalization must trim whitespace and ignore case')
}
if (loginCooldownForEmail(42, 'studio@example.com', '  STUDIO@example.com ') !== 42) {
  throw new Error('cooldown must remain active for the same normalized email')
}
if (loginCooldownForEmail(42, 'studio@example.com', 'other@example.com') !== 0) {
  throw new Error('cooldown must not disable delivery for a different email')
}
if (loginCooldownForEmail(42, '', 'studio@example.com') !== 0) {
  throw new Error('cooldown without a successful delivery email must be ineffective')
}

console.log('OK: authentication presentation contract passed')
