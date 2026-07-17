import { navGroups, normalizeRoute, protectedRoutes, routeTitles } from './layout/admin-navigation'

const expectedGroups = ['概览', '用户与内容', '交易', '模型与生成', '系统']
const actualGroups = navGroups.map((group) => group.label)

if (actualGroups.join('|') !== expectedGroups.join('|')) {
  throw new Error(`admin navigation groups should be quiet Chinese labels, got ${actualGroups.join('|')}`)
}

const labels = navGroups.flatMap((group) => [group.label, ...group.items.map((item) => item.label)])
const mixedLanguageLabels = labels.filter((label) => label.includes('/') || /[A-Za-z]{3,}/.test(label))

if (mixedLanguageLabels.length > 0) {
  throw new Error(`admin navigation should not show slash/English labels: ${mixedLanguageLabels.join(', ')}`)
}

if (routeTitles.monitoring !== '系统健康') {
  throw new Error(`monitoring route should be renamed to 系统健康, got ${routeTitles.monitoring}`)
}

if (routeTitles.pricing !== '价格策略') {
  throw new Error(`pricing route should be renamed to 价格策略, got ${routeTitles.pricing}`)
}

for (const route of ['general-settings', 'security-settings', 'storage-settings'] as const) {
  if ((protectedRoutes as readonly string[]).includes(route)) {
    throw new Error(`${route} should not remain a protected first-level admin route`)
  }
}

if (!(protectedRoutes as readonly string[]).includes('system-settings')) {
  throw new Error('system-settings should be the single navigable protected settings route')
}

if (navGroups.length !== 5 || navGroups.some((group) => group.items.length === 0)) {
  throw new Error('admin navigation must keep five populated operational groups')
}

const aliasCases = new Map([
  ['#/system-settings', 'system-settings'],
  ['#/config', 'system-settings'],
  ['#/general-settings', 'system-settings'],
  ['#/security-config', 'system-settings'],
  ['#/storage-config', 'system-settings'],
])

for (const [hash, expected] of aliasCases) {
  const actual = normalizeRoute(hash)
  if (actual !== expected) {
    throw new Error(`${hash} should normalize to ${expected}, got ${actual}`)
  }
}
