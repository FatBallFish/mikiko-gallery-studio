export type SystemSettingsTab = 'general' | 'security' | 'storage' | 'text-models'

const systemSettingsPaths = new Set([
  'system-settings',
  'general-settings',
  'security-settings',
  'security-config',
  'storage-settings',
  'storage-config',
  'text-model-settings',
])

export function isSystemSettingsHash(hash: string) {
  const path = hash.replace(/^#\/?/, '').split('?')[0].replace(/^\/+|\/+$/g, '')
  return systemSettingsPaths.has(path)
}

export function systemSettingsTabFromHash(hash: string): SystemSettingsTab {
  const path = hash.replace(/^#\/?/, '').split('?')[0].replace(/^\/+|\/+$/g, '')
  if (path === 'security-settings' || path === 'security-config') return 'security'
  if (path === 'storage-settings' || path === 'storage-config') return 'storage'
  if (path === 'text-model-settings') return 'text-models'

  const query = hash.includes('?') ? hash.slice(hash.indexOf('?') + 1) : ''
  const tab = new URLSearchParams(query).get('tab')
  if (tab === 'security' || tab === 'storage' || tab === 'text-models') return tab
  return 'general'
}
