import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./ProfilePage.tsx', import.meta.url), 'utf8')

for (const required of [
  '修改密码',
  'password_change',
  'userApi.sendEmailCode(profile.email',
  'userApi.changePassword(code, newPassword)',
  '验证码',
  '确认新密码',
  'await app.logout()',
]) {
  if (!source.includes(required)) throw new Error(`profile password flow missing: ${required}`)
}

const changeStart = source.indexOf('await userApi.changePassword(code, newPassword)')
const logout = source.indexOf('await app.logout()', changeStart)
if (changeStart < 0 || logout < 0 || changeStart >= logout) {
  throw new Error('successful password change must invalidate the local session after the backend confirms it')
}
