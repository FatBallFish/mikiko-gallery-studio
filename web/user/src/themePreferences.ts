import type { AccentTheme, ThemeMode, UserProfile, UserThemePreference } from '../../shared/api-types'

export const themeStorageKey = 'pic-gallery-user-theme'

export const defaultThemePreference: UserThemePreference = {
  mode: 'light',
  accent: 'amber',
}

export const themeModes: ThemeMode[] = ['dark', 'light']
export const accentThemes: AccentTheme[] = ['amber', 'violet', 'emerald', 'coral']

export function normalizeThemePreference(input: unknown): UserThemePreference {
  const value = input && typeof input === 'object' ? input as Partial<UserThemePreference> : {}
  return {
    mode: themeModes.includes(value.mode as ThemeMode) ? value.mode as ThemeMode : defaultThemePreference.mode,
    accent: accentThemes.includes(value.accent as AccentTheme) ? value.accent as AccentTheme : defaultThemePreference.accent,
  }
}

export function themePreferenceFromProfile(profile?: Pick<UserProfile, 'preferences' | 'theme'> | null): UserThemePreference | null {
  if (!profile) return null
  const preferences = profile.preferences
  if (preferences?.theme_mode || preferences?.accent_theme) {
    return normalizeThemePreference({
      mode: preferences.theme_mode,
      accent: preferences.accent_theme,
    })
  }
  if (profile.theme) {
    const [mode, accent] = profile.theme.split(':')
    return normalizeThemePreference({ mode, accent })
  }
  return null
}

export function serializeThemePreference(preference: UserThemePreference) {
  const normalized = normalizeThemePreference(preference)
  return `${normalized.mode}:${normalized.accent}`
}

export function readLocalThemePreference(storage: Storage = window.localStorage): UserThemePreference {
  try {
    const raw = storage.getItem(themeStorageKey)
    return raw ? normalizeThemePreference(JSON.parse(raw)) : defaultThemePreference
  } catch {
    return defaultThemePreference
  }
}

export function writeLocalThemePreference(preference: UserThemePreference, storage: Storage = window.localStorage) {
  try {
    storage.setItem(themeStorageKey, JSON.stringify(normalizeThemePreference(preference)))
  } catch {
    // Theme should still apply even when browser storage is unavailable.
  }
}

export function applyThemePreference(preference: UserThemePreference, target: HTMLElement = document.documentElement) {
  const normalized = normalizeThemePreference(preference)
  target.dataset.themeMode = normalized.mode
  target.dataset.accentTheme = normalized.accent
}
