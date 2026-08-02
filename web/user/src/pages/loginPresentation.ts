export type LoginMode = 'password' | 'code'
export type LoginIntent = 'login' | 'register' | 'reset'

export type LoginPresentationInput = {
  mode: LoginMode
  intent: LoginIntent
  busy: boolean
  sending: boolean
  cooldown: number
  passwordSetupRequired?: boolean
}

export type LoginFieldValues = {
  mode: LoginMode
  intent: LoginIntent
  email: string
  password: string
  code: string
  newPassword: string
  confirmPassword?: string
  passwordSetupRequired?: boolean
}

export type LoginFieldErrors = Partial<Record<'email' | 'password' | 'code' | 'newPassword' | 'confirmPassword', string>>
export type LoginFieldName = keyof LoginFieldErrors

export const loginProviders = [
  { id: 'wechat', label: '微信登录暂未开放', disabled: true },
  { id: 'dingtalk', label: '钉钉登录暂未开放', disabled: true },
  { id: 'google', label: 'Google 登录暂未开放', disabled: true },
] as const

export function normalizeLoginEmail(email: string) {
  return email.trim().toLowerCase()
}

export function loginCooldownForEmail(cooldown: number, cooldownEmail: string, currentEmail: string) {
  const deliveredTo = normalizeLoginEmail(cooldownEmail)
  if (!deliveredTo || deliveredTo !== normalizeLoginEmail(currentEmail)) return 0
  return Math.max(0, Math.floor(cooldown))
}

export function nextLoginFlow(target: LoginMode | LoginIntent, cooldown = 0) {
  return {
    mode: target === 'password' || target === 'code' ? target : target === 'login' ? 'password' : 'code',
    intent: target === 'register' || target === 'reset' ? target : 'login',
    cooldown: Math.max(0, Math.floor(cooldown)),
    codeSent: false,
    code: '',
  } as const
}

export function nextLoginModeForKey(current: LoginMode, key: string): LoginMode | null {
  if (key === 'Home') return 'password'
  if (key === 'End') return 'code'
  if (key === 'ArrowLeft' || key === 'ArrowRight') return current === 'password' ? 'code' : 'password'
  return null
}

export function firstLoginInvalidField(errors: LoginFieldErrors, mode: LoginMode, intent: LoginIntent, passwordSetupRequired = false): LoginFieldName | null {
  const order: LoginFieldName[] = passwordSetupRequired
    ? ['newPassword', 'confirmPassword']
    : intent === 'reset'
    ? ['email', 'code', 'newPassword']
    : mode === 'password' ? ['email', 'password'] : ['email', 'code']
  return order.find((field) => Boolean(errors[field])) ?? null
}

export function loginPresentation(input: LoginPresentationInput) {
  if (input.passwordSetupRequired) {
    return {
      title: '创建登录密码',
      summary: '邮箱验证已完成。设置密码后即可进入站点。',
      submitLabel: input.busy ? '正在设置...' : '设置密码并进入',
      submitDisabled: input.busy || input.sending,
      sendCodeLabel: '',
      sendCodeDisabled: true,
      codeScene: 'login' as const,
      showPassword: false,
      showCode: false,
      showNewPassword: true,
      showPasswordConfirmation: true,
    } as const
  }
  const isReset = input.intent === 'reset'
  const isRegister = input.intent === 'register'
  const isCode = input.mode === 'code'

  const title = isReset ? '重置密码' : isRegister ? '创建你的账户' : isCode ? '邮箱免密登录' : '欢迎回来'
  const summary = isReset
    ? '验证邮箱后设置新密码。完成后可使用新密码登录。'
    : isRegister
      ? '通过邮箱验证创建账户，首次登录将自动完成注册。'
      : isCode
        ? '无需记住密码，使用邮箱中的六位验证码继续。'
        : '登录后继续你的创作、任务与图片资产。'

  const submitLabel = input.busy
    ? isReset ? '正在重置...' : isRegister ? '正在创建...' : '正在登录...'
    : isReset ? '确认重置' : isRegister ? '注册并进入' : '进入创作台'

  const sendCodeLabel = input.sending
    ? '发送中...'
    : input.cooldown > 0 ? `${input.cooldown} 秒后重发` : '发送验证码'

  return {
    title,
    summary,
    submitLabel,
    submitDisabled: input.busy || input.sending,
    sendCodeLabel,
    sendCodeDisabled: input.busy || input.sending || input.cooldown > 0,
    codeScene: isReset ? 'password_reset' : 'login',
    showPassword: input.mode === 'password' && !isReset,
    showCode: input.mode === 'code' || isReset,
    showNewPassword: isReset,
    showPasswordConfirmation: false,
  } as const
}

export function validateLoginFields(values: LoginFieldValues): LoginFieldErrors {
  const errors: LoginFieldErrors = {}
  if (values.passwordSetupRequired) {
    if (!values.newPassword) errors.newPassword = '请输入新密码'
    else if (values.newPassword.trim().length < 8) errors.newPassword = '新密码至少需要 8 个字符'
    if (!values.confirmPassword) errors.confirmPassword = '请再次输入新密码'
    else if (values.confirmPassword !== values.newPassword) errors.confirmPassword = '两次输入的密码不一致'
    return errors
  }
  const email = values.email.trim()

  if (!email) errors.email = '请输入邮箱地址'
  else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) errors.email = '请输入有效的邮箱地址'

  if (values.mode === 'password' && values.intent === 'login') {
    if (!values.password) errors.password = '请输入密码'
    else if (values.password.length < 6) errors.password = '密码至少需要 6 个字符'
  }

  if (values.mode === 'code' || values.intent === 'reset') {
    if (!values.code) errors.code = '请输入验证码'
    else if (!/^\d{6}$/.test(values.code.trim())) errors.code = '请输入 6 位数字验证码'
  }

  if (values.intent === 'reset') {
    if (!values.newPassword) errors.newPassword = '请输入新密码'
    else if (values.newPassword.trim().length < 8) errors.newPassword = '新密码至少需要 8 个字符'
  }

  return errors
}
