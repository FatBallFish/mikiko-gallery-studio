import { settingsWorkspaceSections } from './SettingsWorkspace'
import { readFileSync } from 'node:fs'

const expected = [
  ['profile', 'profile', '个人资料'],
  ['projects', 'projects', '项目管理'],
  ['billing', 'checkout', '积分与充值'],
  ['api-keys', 'api-keys', 'API 密钥'],
  ['appearance', 'settings', '外观偏好'],
]
const actual = settingsWorkspaceSections.map((item) => [item.id, item.route, item.label])
if (JSON.stringify(actual) !== JSON.stringify(expected)) {
  throw new Error(`settings workspace navigation drifted: ${JSON.stringify(actual)}`)
}

if (new Set(settingsWorkspaceSections.map((item) => item.id)).size !== settingsWorkspaceSections.length) {
  throw new Error('settings workspace section ids must be unique')
}

// Account pages use the prototype single-column settings stack. The shared
// secondary account nav is intentionally no longer rendered on these pages.
for (const page of ['ProfilePage.tsx', 'ApiKeysPage.tsx', 'SettingsPage.tsx'] as const) {
  const source = readFileSync(new URL(`../pages/${page}`, import.meta.url), 'utf8')
  if (source.includes('<SettingsWorkspace')) {
    throw new Error(`${page} must not render the secondary account menu`)
  }
  if (!source.includes('u-settings u-settings--single') || !source.includes('u-settings__stack')) {
    throw new Error(`${page} must use the prototype single-column settings layout`)
  }
}