import { useEffect, useState } from 'react'
import { ConnectedAppsPanel } from './ConnectedAppsPanel'
import { cn } from '../../../shared/classnames'
import { useApp } from '../components'
import { settingsAccentThemeOptions, settingsThemeModeOptions } from './settingsThemeModel'

const motionStorageKey = 'pic-gallery-user-motion'

function readMotionPreference() {
  try {
    return window.localStorage.getItem(motionStorageKey) !== 'reduced'
  } catch {
    return true
  }
}

export function SettingsPage() {
  const app = useApp()
  const modeOptions = settingsThemeModeOptions()
  const accentOptions = settingsAccentThemeOptions()
  const activeMode = app.themePreference.mode
  const activeAccent = app.themePreference.accent
  const [motionEnabled, setMotionEnabled] = useState(readMotionPreference)

  useEffect(() => {
    document.documentElement.dataset.userMotion = motionEnabled ? 'on' : 'reduced'
    try {
      window.localStorage.setItem(motionStorageKey, motionEnabled ? 'on' : 'reduced')
    } catch {
      // The preference still applies to this page when storage is unavailable.
    }
  }, [motionEnabled])

  return (
    <div className="acct-page">
      <div className="u-settings u-settings--single">
        <div className="u-settings__stack">
          <div className="u-page-head" style={{ margin: 0 }}>
            <div className="u-page-head__copy">
              <span className="u-eyebrow">Appearance</span>
              <h1 className="u-h1" style={{ fontSize: 'clamp(32px, 3.4vw, 38px)' }}>外观偏好</h1>
              <p className="u-sub">主题与强调色即时生效，并同步到账户。系统默认跟随设备，也可以固定为浅色或深色。</p>
            </div>
          </div>

          <section className="u-plate u-plate--pad" aria-labelledby="settings-theme-title">
            <div className="u-panel-title">
              <h3 id="settings-theme-title">明暗主题</h3>
              <span>即时生效</span>
            </div>
            <div className="st-themes">
              {modeOptions.map((option) => {
                const active = activeMode === option.value
                return (
                  <button
                    key={option.value}
                    type="button"
                    className={cn('st-theme', active && 'is-active')}
                    aria-pressed={active}
                    onClick={() => void app.setThemePreference({ mode: option.value })}
                  >
                    <span className={cn('st-theme__preview', option.value === 'light' && 'st-theme__preview--light')} aria-hidden="true" />
                    <span style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 8 }}>
                      <strong style={{ fontSize: 12.5 }}>{option.value === 'dark' ? '深色 · Luminous' : '浅色 · Paper'}</strong>
                      {active
                        ? <span className="u-status u-status--ok"><span className="u-dot" aria-hidden="true" />当前</span>
                        : <span className="u-meta">切换</span>}
                    </span>
                    <span className="u-meta">{option.detail}</span>
                  </button>
                )
              })}
            </div>
          </section>

          <section className="u-plate u-plate--pad" aria-labelledby="settings-accent-title">
            <div className="u-panel-title">
              <h3 id="settings-accent-title">强调色</h3>
              <span>单一强调</span>
            </div>
            <div className="st-accents" role="group" aria-label="强调色">
              {accentOptions.map((option) => {
                const active = activeAccent === option.value
                return (
                  <button
                    key={option.value}
                    type="button"
                    className={cn('st-accent', active && 'is-active')}
                    aria-pressed={active}
                    title={option.detail}
                    onClick={() => void app.setThemePreference({ accent: option.value })}
                  >
                    <i style={{ background: option.color }} aria-hidden="true" />
                    {option.label}
                  </button>
                )
              })}
            </div>
            <div className="st-demo" aria-label="当前强调色预览">
              <span className="u-btn u-btn--primary u-btn--sm" aria-hidden="true">主操作</span>
              <span className="u-btn u-btn--sm" aria-hidden="true">次操作</span>
              <span className="u-chip u-chip--accent">选中态</span>
              <span className="u-status u-status--ok"><span className="u-dot" aria-hidden="true" />运行正常</span>
              <span className="u-meta">实时预览</span>
            </div>
          </section>

          <ConnectedAppsPanel />

          <section className="u-plate u-plate--pad" aria-labelledby="settings-motion-title">
            <div className="u-panel-title">
              <h3 id="settings-motion-title">动效</h3>
              <span>{motionEnabled ? '已开启' : '已减弱'}</span>
            </div>
            <div className="u-segment" role="group" aria-label="动效偏好">
              <button type="button" className={motionEnabled ? 'is-active' : undefined} aria-pressed={motionEnabled} onClick={() => setMotionEnabled(true)}>标准动效</button>
              <button type="button" className={!motionEnabled ? 'is-active' : undefined} aria-pressed={!motionEnabled} onClick={() => setMotionEnabled(false)}>减弱动效</button>
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}