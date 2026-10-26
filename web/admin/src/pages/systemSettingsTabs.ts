export type SystemSettingsTab = 'general' | 'point-conversion' | 'attachment-policy' | 'media-policy' | 'security' | 'storage' | 'text-models'

export type SystemSettingsSection = {
  id: string
  label: string
  description?: string
  count?: number
  disabled?: boolean
}

const systemSettingsPaths = new Set([
  'system-settings',
  'general-settings',
  'security-settings',
  'security-config',
  'storage-settings',
  'storage-config',
  'media-policy',
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
  if (path === 'media-policy') return 'media-policy'
  if (path === 'text-model-settings') return 'text-models'

  const query = hash.includes('?') ? hash.slice(hash.indexOf('?') + 1) : ''
  const tab = new URLSearchParams(query).get('tab')
  if (tab === 'point-conversion' || tab === 'attachment-policy' || tab === 'media-policy' || tab === 'security' || tab === 'storage' || tab === 'text-models') return tab
  return 'general'
}

/**
 * Static vertical sub-navigation blocks for each settings section.
 * General configuration is dynamic: ConfigPage reports its available
 * categories (site / features / public_gallery) after loading.
 */
export const systemSettingsSections: Record<SystemSettingsTab, SystemSettingsSection[]> = {
  general: [],
  'point-conversion': [
    { id: 'rate', label: '人民币积分汇率', description: '每积分对应的人民币金额' },
  ],
  'attachment-policy': [
    { id: 'image', label: '图片附件', description: '当前生效的上传校验' },
    { id: 'video', label: '视频附件', description: '预留配置' },
    { id: 'audio', label: '音频附件', description: '预留配置' },
    { id: 'document', label: '文档附件', description: '预留配置' },
  ],
  'media-policy': [
    { id: 'formats-quota', label: '格式与配额', description: '格式、体积与存储配额' },
    { id: 'derivatives', label: '派生资源', description: '缩略图与代理生成' },
    { id: 'retention', label: '会话与保留', description: '上传会话与清理周期' },
  ],
  security: [
    { id: 'smtp', label: 'SMTP 邮件', description: '验证码与通知邮件' },
    { id: 'oidc', label: 'OIDC 身份', description: '统一身份登录' },
  ],
  storage: [
    { id: 'instances', label: '存储实例', description: 'Local / S3 / R2' },
  ],
  'text-models': [],
}

export function systemSettingsSectionAnchor(tab: SystemSettingsTab, sectionId: string) {
  return `ss-section-${tab}-${sectionId}`
}

export function scrollToSystemSettingsSection(tab: SystemSettingsTab, sectionId: string) {
  if (typeof document === 'undefined') return
  const target = document.getElementById(systemSettingsSectionAnchor(tab, sectionId))
  target?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

/**
 * Deterministic deep comparison for settings drafts. Object keys are sorted
 * so a server response that reorders keys never marks an untouched form dirty.
 */
export function stableSettingsValue(value: unknown): string {
  return JSON.stringify(canonicalizeSettingsValue(value)) ?? 'null'
}

export function settingsValuesEqual(left: unknown, right: unknown) {
  return stableSettingsValue(left) === stableSettingsValue(right)
}

function canonicalizeSettingsValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalizeSettingsValue)
  if (value && typeof value === 'object') {
    const record = value as Record<string, unknown>
    const next: Record<string, unknown> = {}
    for (const key of Object.keys(record).sort()) {
      if (record[key] === undefined) continue
      next[key] = canonicalizeSettingsValue(record[key])
    }
    return next
  }
  return value
}