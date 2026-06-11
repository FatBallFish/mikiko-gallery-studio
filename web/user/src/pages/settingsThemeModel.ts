import type { AccentTheme, ThemeMode } from '../../../shared/api-types'

export type ThemeModeOption = {
  value: ThemeMode
  label: string
  detail: string
  icon: 'moon' | 'sun'
}

export type AccentThemeOption = {
  value: AccentTheme
  label: string
  detail: string
  color: string
}

export function settingsThemeModeOptions(): ThemeModeOption[] {
  return [
    { value: 'dark', label: '暗色', detail: '低亮度创作工作台', icon: 'moon' },
    { value: 'light', label: '亮色', detail: '更适合白天浏览和文档阅读', icon: 'sun' },
  ]
}

export function settingsAccentThemeOptions(): AccentThemeOption[] {
  return [
    { value: 'amber', label: '琥珀', detail: '温暖、克制、适合默认品牌态', color: 'var(--pg-accent-amber)' },
    { value: 'violet', label: '紫罗兰', detail: '更强科技感和创作感', color: 'var(--pg-accent-violet)' },
    { value: 'emerald', label: '翡翠', detail: '更清爽的正向反馈气质', color: 'var(--pg-accent-emerald)' },
    { value: 'coral', label: '珊瑚', detail: '更醒目的强调色', color: 'var(--pg-accent-coral)' },
  ]
}
