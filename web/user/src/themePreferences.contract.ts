import {
  applyThemePreference,
  defaultThemePreference,
  normalizeThemePreference,
  serializeThemePreference,
  themePreferenceFromProfile,
} from './themePreferences'

const invalid = normalizeThemePreference({ mode: 'sepia', accent: 'blue' })
if (invalid.mode !== defaultThemePreference.mode || invalid.accent !== defaultThemePreference.accent) {
  throw new Error(`invalid theme values should fall back to defaults, got ${JSON.stringify(invalid)}`)
}

const profilePreference = themePreferenceFromProfile({
  preferences: {
    model_group: 'plus-image',
    base_resolution: 'auto',
    aspect_ratio: '16:9',
    image_count: 1,
    theme_mode: 'light',
    accent_theme: 'violet',
  },
})
if (profilePreference?.mode !== 'light' || profilePreference.accent !== 'violet') {
  throw new Error(`profile theme fields should map to theme preference, got ${JSON.stringify(profilePreference)}`)
}

const legacyPreference = themePreferenceFromProfile({
  theme: 'dark:emerald',
  preferences: {
    model_group: 'plus-image',
    base_resolution: 'auto',
    aspect_ratio: '16:9',
    image_count: 1,
  },
})
if (legacyPreference?.mode !== 'dark' || legacyPreference.accent !== 'emerald') {
  throw new Error(`legacy theme should remain readable, got ${JSON.stringify(legacyPreference)}`)
}

if (serializeThemePreference({ mode: 'light', accent: 'violet' }) !== 'light:violet') {
  throw new Error('theme preference should serialize to the backend theme field')
}

const target = { dataset: {} } as HTMLElement
applyThemePreference({ mode: 'light', accent: 'coral' }, target)
if (target.dataset.themeMode !== 'light' || target.dataset.accentTheme !== 'coral') {
  throw new Error(`theme application should write data attributes, got ${JSON.stringify(target.dataset)}`)
}
