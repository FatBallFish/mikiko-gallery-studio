import { settingsAccentThemeOptions, settingsThemeModeOptions } from './settingsThemeModel'

const modes = settingsThemeModeOptions()
const modeValues = modes.map((item) => item.value)
if (JSON.stringify(modeValues) !== JSON.stringify(['dark', 'light'])) {
  throw new Error(`settings mode options should stay aligned with theme modes, got ${JSON.stringify(modeValues)}`)
}

const modeIcons = modes.map((item) => `${item.value}:${item.icon}`)
if (!modeIcons.includes('dark:moon') || !modeIcons.includes('light:sun')) {
  throw new Error(`settings mode icons should match visible theme state, got ${JSON.stringify(modeIcons)}`)
}

const accents = settingsAccentThemeOptions()
const accentValues = accents.map((item) => item.value)
if (JSON.stringify(accentValues) !== JSON.stringify(['amber', 'violet', 'emerald', 'coral'])) {
  throw new Error(`settings accent options should stay aligned with accent themes, got ${JSON.stringify(accentValues)}`)
}

const labels = accents.map((item) => item.label).join(' ')
if (/Pic Gallery|Vault|TODO|coming soon/i.test(labels)) {
  throw new Error(`settings accent options should use production labels, got ${labels}`)
}
