import { loginCopy, loginLocale, socialLoginUnavailableMessage } from './loginCopy'

const zh = loginCopy.zh

if (loginLocale('zh-CN') !== 'zh' || loginLocale('en-US') !== 'zh') {
  throw new Error('MVP should default login copy to Chinese for both zh-CN and non-Chinese browsers')
}

if (zh.passwordLoginFailed !== '账号密码登录失败' || zh.codeLoginFailed !== '验证码登录失败' || zh.resetPasswordFailed !== '密码重置失败') {
  throw new Error(`login failure titles should be Chinese operator-facing copy, got ${JSON.stringify(zh)}`)
}

const allVisibleCopy = Object.values(zh).join(' ')
if (/Password sign-in failed|Verification code sign-in failed|Password reset failed|not available yet|暂不可用|后续|即将|版本/.test(allVisibleCopy)) {
  throw new Error(`login visible copy should not expose English or weak roadmap wording, got ${allVisibleCopy}`)
}

for (const provider of ['微信', '钉钉', 'Google']) {
  const message = socialLoginUnavailableMessage(provider)
  if (!message.includes(provider) || !message.includes('邮箱验证码') || !message.includes('账号密码')) {
    throw new Error(`social login unavailable copy should name ${provider} and guide to usable login methods, got ${message}`)
  }
  if (/暂不可用|后续|即将|版本|not available/i.test(message)) {
    throw new Error(`social login unavailable copy should avoid weak roadmap wording, got ${message}`)
  }
}
