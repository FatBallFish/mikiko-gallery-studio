import {
  adminLoginCopy,
  adminLoginInitialForm,
  adminLoginValidation,
  adminLoginVisibleError,
} from './adminLoginCopy'

const defaultForm = adminLoginInitialForm({})
if (defaultForm.email !== '' || defaultForm.password !== '') {
  throw new Error(`admin login should not prefill demo credentials without env defaults, got ${JSON.stringify(defaultForm)}`)
}

const envForm = adminLoginInitialForm({
  VITE_DEFAULT_ADMIN_EMAIL: 'ops@example.com',
  VITE_DEFAULT_ADMIN_PASSWORD: 'secret123',
})
if (envForm.email !== 'ops@example.com' || envForm.password !== 'secret123') {
  throw new Error(`admin login should honor explicit env defaults, got ${JSON.stringify(envForm)}`)
}

const invalid = adminLoginValidation('bad-email', '123')
if (invalid.emailError !== '请输入有效管理员邮箱' || invalid.passwordError !== '密码至少 6 位') {
  throw new Error(`admin login validation should be localized, got ${JSON.stringify(invalid)}`)
}

const empty = adminLoginValidation('', '')
if (empty.emailError !== null || empty.passwordError !== null) {
  throw new Error(`empty admin login form should wait for submit-level validation, got ${JSON.stringify(empty)}`)
}

if (!adminLoginCopy.heroTitle.includes('运营后台') || !adminLoginCopy.heroDetail.includes('配置') || !adminLoginCopy.heroDetail.includes('审计')) {
  throw new Error(`admin login hero copy should be product-facing Chinese copy, got ${JSON.stringify(adminLoginCopy)}`)
}

const visibleCopy = [
  adminLoginCopy.brand,
  adminLoginCopy.heroTitle,
  adminLoginCopy.heroDetail,
  adminLoginCopy.formEyebrow,
  adminLoginCopy.idleNotice,
  ...adminLoginCopy.proofItems,
].join(' ')
if (/Soft Grid|Admin Access|Route guard|provider|draft|queue/i.test(visibleCopy)) {
  throw new Error(`admin login visible copy should not expose internal or mixed-language MVP copy, got ${visibleCopy}`)
}

const unauthorized = adminLoginVisibleError(new Error('401 Unauthorized'))
if (unauthorized !== '管理员邮箱或密码不正确，请检查后重试。') {
  throw new Error(`admin login should map unauthorized errors to localized action copy, got ${unauthorized}`)
}

const network = adminLoginVisibleError(new Error('Failed to fetch'))
if (network !== '无法连接后台服务，请稍后重试或检查部署状态。') {
  throw new Error(`admin login should map network failures to deployable copy, got ${network}`)
}

const custom = adminLoginVisibleError(new Error('ACCOUNT_LOCKED'))
if (custom !== 'ACCOUNT_LOCKED') {
  throw new Error(`admin login should preserve unknown backend error for troubleshooting, got ${custom}`)
}
