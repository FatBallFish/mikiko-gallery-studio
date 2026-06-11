import type { AccentTheme, ThemeMode } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { useApp } from '../components'
import { settingsAccentThemeOptions, settingsThemeModeOptions } from './settingsThemeModel'

const settingsClasses = {
  page: 'w-full flex-1 p-6 md:p-10',
  header: 'mb-10',
  title: 'm-0 text-4xl font-black leading-none md:text-6xl',
  detail: 'mt-4 max-w-2xl text-base leading-relaxed text-[var(--muted)]',
  grid: 'grid grid-cols-1 gap-6 xl:grid-cols-[0.9fr_1.1fr]',
  card: 'rounded-[2.5rem] border border-[var(--border)] bg-[var(--surface)] p-6 md:p-8',
  cardHead: 'mb-6 flex items-start justify-between gap-4',
  cardTitle: 'm-0 text-2xl font-black',
  cardDetail: 'mt-2 text-sm leading-relaxed text-[var(--muted)]',
  previewIcon: 'grid size-12 place-items-center rounded-2xl border border-[var(--border)] bg-[var(--bg)] text-[var(--accent)]',
  modeGrid: 'grid grid-cols-2 gap-3',
  modeButton: 'flex min-h-24 cursor-pointer flex-col items-center justify-center gap-2 rounded-2xl border bg-[var(--bg)]/60 px-3 text-center text-sm font-bold transition-all',
  modeActive: 'border-[var(--accent)] text-[var(--accent)] ring-1 ring-[var(--accent)]',
  modeInactive: 'border-[var(--border)] text-[var(--muted)] hover:border-[var(--accent)]/60 hover:text-[var(--fg)]',
  swatchGrid: 'grid grid-cols-1 gap-3 sm:grid-cols-2',
  swatchButton: 'flex min-h-24 cursor-pointer items-center gap-4 rounded-2xl border bg-[var(--bg)]/60 p-4 text-left transition-all',
  swatch: 'grid size-12 shrink-0 place-items-center rounded-2xl border border-white/20 shadow-[0_18px_36px_rgb(0_0_0/.18)]',
  swatchDot: 'size-4 rounded-full bg-white/80',
  syncPanel: 'rounded-[2rem] border border-[var(--border)] bg-[var(--bg)]/50 p-6',
}

export function SettingsPage() {
  const app = useApp()
  const modeOptions = settingsThemeModeOptions()
  const accentOptions = settingsAccentThemeOptions()
  const activeMode = app.themePreference.mode
  const activeAccent = app.themePreference.accent

  return (
    <div className={settingsClasses.page}>
      <header className={settingsClasses.header}>
        <h1 className={settingsClasses.title}>设置</h1>
        <p className={settingsClasses.detail}>调整 Mikiko Studio 的站点外观偏好。主题模式与主题色会立即应用，并在登录状态下同步到账户偏好。</p>
      </header>

      <div className={settingsClasses.grid}>
        <section className={settingsClasses.card}>
          <div className={settingsClasses.cardHead}>
            <div>
              <h2 className={settingsClasses.cardTitle}>光暗主题</h2>
              <p className={settingsClasses.cardDetail}>右上角也可以快速切换。</p>
            </div>
            <div className={settingsClasses.previewIcon} aria-hidden="true">
              {activeMode === 'dark' ? <MoonIcon /> : <SunIcon />}
            </div>
          </div>
          <div className={settingsClasses.modeGrid}>
            {modeOptions.map((option) => (
              <button
                key={option.value}
                type="button"
                className={cn(settingsClasses.modeButton, activeMode === option.value ? settingsClasses.modeActive : settingsClasses.modeInactive)}
                onClick={() => void app.setThemePreference({ mode: option.value })}
              >
                {option.icon === 'moon' ? <MoonIcon /> : <SunIcon />}
                <span>{option.label}</span>
                <small className="font-normal text-[var(--dim)]">{option.detail}</small>
              </button>
            ))}
          </div>
        </section>

        <section className={settingsClasses.card}>
          <div className={settingsClasses.cardHead}>
            <div>
              <h2 className={settingsClasses.cardTitle}>主题色</h2>
              <p className={settingsClasses.cardDetail}>用于按钮、选中态、焦点态和关键指标。</p>
            </div>
          </div>
          <div className={settingsClasses.swatchGrid}>
            {accentOptions.map((option) => (
              <button
                key={option.value}
                type="button"
                className={cn(settingsClasses.swatchButton, activeAccent === option.value ? settingsClasses.modeActive : settingsClasses.modeInactive)}
                onClick={() => void app.setThemePreference({ accent: option.value })}
              >
                <span className={settingsClasses.swatch} style={{ background: option.color }}>
                  <span className={settingsClasses.swatchDot} />
                </span>
                <span>
                  <strong className="block text-base text-[var(--fg)]">{option.label}</strong>
                  <small className="mt-1 block text-xs leading-relaxed text-[var(--muted)]">{option.detail}</small>
                </span>
              </button>
            ))}
          </div>
        </section>
      </div>

      <section className={cn(settingsClasses.syncPanel, 'mt-6')}>
        <h2 className="m-0 text-xl font-black">偏好同步</h2>
        <p className="m-0 mt-2 text-sm leading-relaxed text-[var(--muted)]">
          当前偏好会先保存在本机，并在登录状态下同步到 Mikiko Studio 账户。若网络异常，本机外观仍会保持当前选择。
        </p>
      </section>
    </div>
  )
}

function SunIcon() {
  return <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" /></svg>
}

function MoonIcon() {
  return <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z" /></svg>
}
