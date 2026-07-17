import { avatarMenuItems } from './avatarMenu'

const items = avatarMenuItems()
const visibleCopy = items.map((item) => `${item.label}${item.route}${item.key}`).join(' ')

const expectedRoutes = new Map([
  ['个人中心', 'profile'],
  ['积分充值', 'checkout'],
  ['API 密钥', 'api-keys'],
  ['开发文档', 'docs'],
])

for (const [label, route] of expectedRoutes) {
  const item = items.find((candidate) => candidate.label === label)
  if (!item) throw new Error(`avatar menu should include ${label}, got ${JSON.stringify(items)}`)
  if (item.route !== route) throw new Error(`${label} should route to ${route}, got ${item.route}`)
  if (route === 'docs' && item.external !== true) throw new Error('documentation account entry should be marked as an external destination')
}

if (items.length !== expectedRoutes.size) {
  throw new Error(`avatar menu should only expose real account entries, got ${JSON.stringify(items)}`)
}

if (/暂不可用|后续|即将|版本|not available|TODO|unavailable/i.test(visibleCopy)) {
  throw new Error(`avatar menu should not expose unavailable/dead-end wording, got ${visibleCopy}`)
}

const keys = new Set(items.map((item) => item.key))
if (keys.size !== items.length) {
  throw new Error(`avatar menu item keys should be stable and unique, got ${JSON.stringify(items)}`)
}
