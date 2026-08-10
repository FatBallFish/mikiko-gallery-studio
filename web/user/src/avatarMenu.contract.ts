import { avatarMenuItems } from './avatarMenu'

const items = avatarMenuItems()
const visibleCopy = items.map((item) => `${item.label}${item.route}${item.key}`).join(' ')

const expectedRoutes = new Map([
  ['个人中心', 'profile'],
  ['积分充值', 'checkout'],
  ['API 密钥', 'api-keys'],
])

for (const [label, route] of expectedRoutes) {
  const item = items.find((candidate) => candidate.label === label)
  if (!item) throw new Error(`avatar menu should include ${label}, got ${JSON.stringify(items)}`)
  if (item.route !== route) throw new Error(`${label} should route to ${route}, got ${item.route}`)
}

const docsItem = items.find((candidate) => candidate.label === '开发文档')
if (!docsItem || docsItem.external !== true || 'route' in docsItem) {
  throw new Error(`documentation account entry must be external without an internal route, got ${JSON.stringify(docsItem)}`)
}

if (items.length !== expectedRoutes.size + 1) {
  throw new Error(`avatar menu should only expose real account entries, got ${JSON.stringify(items)}`)
}

if (/暂不可用|后续|即将|版本|not available|TODO|unavailable/i.test(visibleCopy)) {
  throw new Error(`avatar menu should not expose unavailable/dead-end wording, got ${visibleCopy}`)
}

const keys = new Set(items.map((item) => item.key))
if (keys.size !== items.length) {
  throw new Error(`avatar menu item keys should be stable and unique, got ${JSON.stringify(items)}`)
}
