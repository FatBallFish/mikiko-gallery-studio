import { settingsWorkspaceSections } from './SettingsWorkspace'
import { readFileSync } from 'node:fs'

const expected = [
  ['profile', 'profile', '个人资料'],
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

for (const [page, active] of [['ProfilePage.tsx', 'profile'], ['ApiKeysPage.tsx', 'api-keys'], ['SettingsPage.tsx', 'appearance']] as const) {
  const source = readFileSync(new URL(`../pages/${page}`, import.meta.url), 'utf8')
  if (!source.includes('<SettingsWorkspace') || !source.includes(`active="${active}"`)) {
    throw new Error(`${page} must use the shared settings workspace for ${active}`)
  }
}
