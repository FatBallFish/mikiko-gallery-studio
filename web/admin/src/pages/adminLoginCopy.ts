export type AdminLoginEnv = Record<string, string | undefined>

export const adminLoginCopy = {
  brand: 'Pic Gallery 运营后台',
  heroTitle: '登录运营后台',
  heroDetail: '进入低噪声运营控制台，处理配置、审核、交易与审计任务。',
  formEyebrow: '管理员登录',
  idleNotice: '请输入管理员邮箱和密码，未登录访问后台页面会自动回到本页。',
  submitValidationError: '请先修正表单校验错误。',
  submitLabel: '进入控制台',
  submittingLabel: '校验中...',
  emailLabel: '管理员邮箱',
  emailPlaceholder: 'ops@example.com',
  passwordLabel: '密码',
  passwordPlaceholder: '至少 6 位',
  proofItems: ['环境：local', 'API：readyz', '版本：dev', '会话：受保护'],
} as const

export function adminLoginInitialForm(env: AdminLoginEnv) {
  return {
    email: env.VITE_DEFAULT_ADMIN_EMAIL ?? '',
    password: env.VITE_DEFAULT_ADMIN_PASSWORD ?? '',
  }
}

export function adminLoginValidation(email: string, password: string) {
  return {
    emailError: email && !/^\S+@\S+\.\S+$/.test(email) ? '请输入有效管理员邮箱' : null,
    passwordError: password && password.length < 6 ? '密码至少 6 位' : null,
  }
}

export function adminLoginVisibleError(error: unknown) {
  const message = error instanceof Error ? error.message : ''
  if (/401|403|unauthorized|forbidden|invalid.*password|invalid.*credential/i.test(message)) {
    return '管理员邮箱或密码不正确，请检查后重试。'
  }
  if (/failed to fetch|network|load failed|connection/i.test(message)) {
    return '无法连接后台服务，请稍后重试或检查部署状态。'
  }
  return message || '管理员登录失败'
}
