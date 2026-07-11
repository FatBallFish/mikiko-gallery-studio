import type { AccentTheme, ThemeMode } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { useApp } from '../components'
import { button as btn, card } from '../ui/redesign-classes'
import { Sun, Moon, Palette } from '../ui/icons'
import { SettingsWorkspace } from '../ui/SettingsWorkspace'
import { settingsAccentThemeOptions, settingsThemeModeOptions } from './settingsThemeModel'

const settingsClasses = {
  page: 'w-full flex-1 p-6 md:p-10',
  header: 'mb-10',
  title: 'm-0 text-4xl font-black leading-none md:text-6xl',
  detail: 'mt-4 max-w-2xl text-base leading-relaxed text-[var(--muted)]',
  grid: 'grid grid-cols-1 gap-6 xl:grid-cols-[0.9fr_1.1fr]',
  card: card.padded,
  cardHead: 'mb-6 flex items-start justify-between gap-4',
  cardTitle: 'm-0 text-2xl font-black',
  cardDetail: 'mt-2 text-sm leading-relaxed text-[var(--muted)]',
  previewIcon: 'grid size-12 place-items-center rounded-2xl border border-[var(--border)] bg-[var(--bg)] text-[var(--accent)]',
  modeGrid: 'grid grid-cols-2 gap-3',
  modeButton: 'flex min-h-24 cursor-pointer flex-col items-center justify-center gap-2 rounded-2xl border bg-[var(--bg)]/60 px-3 text-center text-sm font-bold transition-all duration-200 active:scale-[0.98]',
  modeActive: 'border-[var(--accent)] text-[var(--accent)] ring-1 ring-[var(--accent)] bg-[color-mix(in_oklch,var(--accent)_6%,transparent)]',
  modeInactive: 'border-[var(--border)] text-[var(--muted)] hover:border-[var(--accent)]/60 hover:text-[var(--fg)]',
  swatchGrid: 'grid grid-cols-1 gap-3 sm:grid-cols-2',
  swatchButton: 'flex min-h-24 cursor-pointer items-center gap-4 rounded-2xl border bg-[var(--bg)]/60 p-4 text-left transition-all duration-200 active:scale-[0.98]',
  swatch: 'grid size-12 shrink-0 place-items-center rounded-2xl border border-[color-mix(in_oklch,var(--fg)_12%,transparent)] shadow-[var(--pg-shadow-md)]',
  swatchDot: 'size-4 rounded-full bg-[var(--surface-solid)]',
  syncPanel: 'rounded-3xl border border-[var(--border)] bg-[var(--bg)]/50 p-6',
  // Accent preview mini-card
  previewCard: 'rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4',
  previewRow: 'flex items-center justify-between gap-3',
  previewLabel: 'text-xs text-[var(--muted)]',
  previewBtn: cn(btn.base, btn.primary, 'min-h-0 px-3 py-1.5 text-xs'),
  previewBadge: 'inline-flex items-center gap-1.5 rounded-full bg-[color-mix(in_oklch,var(--accent)_18%,transparent)] px-2.5 py-1 font-vault-mono text-[11px] font-bold text-[var(--accent)]',
}

export function SettingsPage() {
  const app = useApp()
  const modeOptions = settingsThemeModeOptions()
  const accentOptions = settingsAccentThemeOptions()
  const activeMode = app.themePreference.mode
  const activeAccent = app.themePreference.accent

  return (
    <SettingsWorkspace
      active="appearance"
      title="外观偏好"
      detail="主题模式与强调色会即时应用，并在登录状态下同步到你的 Mikiko Studio 账户。"
    >
      <div className={settingsClasses.grid}>
        <section className={settingsClasses.card}>
          <div className={settingsClasses.cardHead}>
            <div>
              <h2 className={settingsClasses.cardTitle}>光暗主题</h2>
              <p className={settingsClasses.cardDetail}>右上角也可以快速切换。</p>
            </div>
            <div className={settingsClasses.previewIcon} aria-hidden="true">
              {activeMode === 'dark' ? <Moon size={20} strokeWidth={1.5} /> : <Sun size={20} strokeWidth={1.5} />}
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
                {option.icon === 'moon' ? <Moon size={20} strokeWidth={1.5} /> : <Sun size={20} strokeWidth={1.5} />}
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
            <div className={settingsClasses.previewIcon} aria-hidden="true">
              <Palette size={20} strokeWidth={1.5} />
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
          {/* Accent live preview */}
          <div className={settingsClasses.previewCard + ' mt-6'}>
            <div className="text-xs font-bold text-[var(--muted)] mb-3">当前主题色预览</div>
            <div className={settingsClasses.previewRow}>
              <span className={settingsClasses.previewLabel}>主按钮</span>
              <span className={settingsClasses.previewBtn}>立即开始</span>
            </div>
            <div className={settingsClasses.previewRow + ' mt-2.5'}>
              <span className={settingsClasses.previewLabel}>状态标签</span>
              <span className={settingsClasses.previewBadge}>已完成</span>
            </div>
          </div>
        </section>
      </div>

      <section className={cn(settingsClasses.syncPanel, 'mt-6')}>
        <h2 className="m-0 text-xl font-black">偏好同步</h2>
        <p className="m-0 mt-2 text-sm leading-relaxed text-[var(--muted)]">
          当前偏好会先保存在本机，并在登录状态下同步到 Mikiko Studio 账户。若网络异常，本机外观仍会保持当前选择。
        </p>
      </section>
    </SettingsWorkspace>
  )
}
