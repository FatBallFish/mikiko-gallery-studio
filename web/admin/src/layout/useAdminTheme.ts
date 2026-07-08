import { useEffect, useState } from 'react'

const adminThemeStorageKey = 'pic_gallery_admin_theme'

export type AdminThemeMode = 'dark' | 'light'

export function useAdminTheme() {
  const [theme, setTheme] = useState<AdminThemeMode>(() => {
    if (typeof window === 'undefined') return 'light'
    return window.localStorage.getItem(adminThemeStorageKey) === 'dark' ? 'dark' : 'light'
  })

  useEffect(() => {
    window.localStorage.setItem(adminThemeStorageKey, theme)
  }, [theme])

  return { theme, setTheme }
}
