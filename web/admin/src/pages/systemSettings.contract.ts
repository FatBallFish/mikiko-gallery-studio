import { normalizeRoute, protectedRoutes } from '../layout/admin-navigation'
// @ts-ignore contract scripts run in tsx/node; the admin app tsconfig does not include node types.
import { readFileSync } from 'node:fs'
import { isSystemSettingsHash, systemSettingsTabFromHash } from './systemSettingsTabs'

const systemSettingsSource = readFileSync(new URL('./SystemSettingsPage.tsx', import.meta.url), 'utf8')
const generalConfigSource = readFileSync(new URL('./ConfigPage.tsx', import.meta.url), 'utf8')

for (const parentContract of ['dirtyTabs', 'busyTabs', 'busyTabsRef', 'window.confirm(', 'onDirtyChange=', 'onBusyChange=', '当前分区存在未保存修改', 'activeTabRef.current = tab', 'isSystemSettingsHash(window.location.hash)', "dispatchEvent(new HashChangeEvent('hashchange'))"]) {
  if (!systemSettingsSource.includes(parentContract)) {
    throw new Error(`system settings must guard dirty tab changes with ${parentContract}`)
  }
}

for (const editorContract of ['type ConfigEditorState', 'beforeunload', 'onDirtyChange', 'onBusyChange', 'switchConfigTab', 'if (saving) return', '当前类目存在未保存修改，放弃并切换吗？', 'data-admin-config-save-rail', 'InlineFeedback', 'saveError', 'disabled: saving', 'saving || !canEditActiveTab']) {
  if (!generalConfigSource.includes(editorContract)) {
    throw new Error(`general config editor must implement ${editorContract}`)
  }
}

for (const hash of ['#/system-settings', '#/general-settings', '#/security-settings', '#/storage-settings']) {
  if (!isSystemSettingsHash(hash)) throw new Error(`${hash} should be recognized as a system-settings route`)
}

for (const hash of ['#/users', '#/monitoring', '#/orders']) {
  if (isSystemSettingsHash(hash)) throw new Error(`${hash} must be recognized as leaving system settings`)
}

if (!protectedRoutes.includes('system-settings')) {
  throw new Error('system-settings must be the single protected settings route')
}

for (const route of ['general-settings', 'security-settings', 'storage-settings'] as const) {
  if ((protectedRoutes as readonly string[]).includes(route)) {
    throw new Error(`${route} must stay as a compatibility alias, not a first-level route`)
  }
}

const routeCases = ['#/general-settings', '#/security-settings', '#/storage-settings', '#/system-settings?tab=storage']
for (const hash of routeCases) {
  if (normalizeRoute(hash) !== 'system-settings') {
    throw new Error(`${hash} should normalize to system-settings`)
  }
}

const tabCases = new Map([
  ['#/system-settings', 'general'],
  ['#/general-settings', 'general'],
  ['#/security-settings', 'security'],
  ['#/storage-settings', 'storage'],
  ['#/system-settings?tab=security', 'security'],
  ['#/system-settings?tab=storage', 'storage'],
])

for (const [hash, expected] of tabCases) {
  const actual = systemSettingsTabFromHash(hash)
  if (actual !== expected) throw new Error(`${hash} should select ${expected}, got ${actual}`)
}
