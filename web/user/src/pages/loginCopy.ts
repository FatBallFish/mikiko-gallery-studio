export const loginCopy = {
  zh: {
    emailPlaceholder: '输入邮箱地址',
    passwordPlaceholder: '输入密码',
    codePlaceholder: '6 位验证码',
    resetPasswordPlaceholder: '输入新密码',
    sendCodeFailed: '验证码发送失败',
    passwordLoginFailed: '账号密码登录失败',
    codeLoginFailed: '验证码登录失败',
    resetPasswordFailed: '密码重置失败',
    socialUnavailable: '请使用邮箱验证码或账号密码登录',
  },
  en: {
    emailPlaceholder: 'Enter email address',
    passwordPlaceholder: 'Enter password',
    codePlaceholder: '6-digit code',
    resetPasswordPlaceholder: 'Enter new password',
    sendCodeFailed: 'Failed to send verification code',
    passwordLoginFailed: 'Password sign-in failed',
    codeLoginFailed: 'Verification code sign-in failed',
    resetPasswordFailed: 'Password reset failed',
    socialUnavailable: 'Please sign in with email code or password',
  },
} as const

export type LoginLocale = keyof typeof loginCopy

export function loginLocale(_language = typeof navigator === 'undefined' ? '' : navigator.language): LoginLocale {
  return 'zh'
}

export function socialLoginUnavailableMessage(provider: string) {
  return `${provider} 登录入口已预留，请先使用邮箱验证码或账号密码登录。`
}
