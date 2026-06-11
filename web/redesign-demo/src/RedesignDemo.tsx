import React, { useState, useEffect } from 'react'
import { cn } from '../shared/classnames'
import { rdShell, rdWorkspace, rdCommon } from './redesign-classes'

// Mock Data
const models = [
  { code: 'v-flux-pro', name: 'Flux Pro', points: '1.2' },
  { code: 'v-midjourney-v6', name: 'Midjourney v6', points: '1.5' },
  { code: 'v-dalle-3', name: 'DALL-E 3', points: '1.0' },
]

const ratios = ['1:1', '4:3', '16:9', '9:16', '3:4']
const qualities = ['Standard', 'High', 'Ultra']

export function RedesignDemo() {
  const [accent, setAccent] = useState('oklch(65% 0.18 280)') // Default Blue-Purple
  const [accentPurple, setAccentPurple] = useState('oklch(60% 0.2 310)')
  const [selectedModel, setSelectedModel] = useState('v-flux-pro')
  const [selectedRatio, setSelectedRatio] = useState('16:9')
  const [selectedQuality, setSelectedQuality] = useState('High')
  const [selectedCount, setSelectedCount] = useState(1)
  const [prompt, setPrompt] = useState('')
  
  // Theme Application
  useEffect(() => {
    const root = document.documentElement
    root.style.setProperty('--accent', accent)
    root.style.setProperty('--accent-purple', accentPurple)
    // Simple RGB conversion for shadow/glow effects
    root.style.setProperty('--accent-rgb', '131, 118, 255') // Approximate for the default
  }, [accent, accentPurple])

  return (
    <div className={rdShell.shell}>
      {/* Sidebar */}
      <aside className={rdShell.sidebar}>
        <div className={rdShell.brand}>
          <div className={rdShell.brandOrb}>V</div>
          <span className={rdShell.brandText}>Vault</span>
        </div>
        
        <nav className={rdShell.nav}>
          <NavItem icon={<HomeIcon />} label="首页" short="HOME" active />
          <NavItem icon={<SparklesIcon />} label="创作" short="GEN" />
          <NavItem icon={<GridIcon />} label="图库" short="IMG" />
        </nav>
        
        <div className="mt-auto px-2 w-full">
          <NavItem icon={<SettingsIcon />} label="设置" short="SET" />
        </div>
      </aside>

      <main className={rdShell.main}>
        {/* Topbar */}
        <header className={rdShell.topbar}>
          <div className={rdShell.menuList}>
            <TopMenuIcon color="oklch(75% 0.15 165)" icon={<TemplateIcon />} title="灵感模板" label="Inspiration" />
            <TopMenuIcon color="oklch(70% 0.12 30)" icon={<GalleryIcon />} title="公开广场" label="Community" />
            <TopMenuIcon color="oklch(65% 0.1 220)" icon={<DocsIcon />} title="开发文档" label="API Docs" />
          </div>

          <div className={rdShell.userTools}>
            {/* Theme Adjuster */}
            <div className="flex gap-2 p-1 bg-[var(--surface)] rounded-full border border-[var(--border)]">
              <button 
                className="size-5 rounded-full bg-[oklch(65%_0.18_280)] border border-white/20" 
                onClick={() => { setAccent('oklch(65% 0.18 280)'); setAccentPurple('oklch(60% 0.2 310)') }} 
              />
              <button 
                className="size-5 rounded-full bg-[oklch(70%_0.15_30)] border border-white/20" 
                onClick={() => { setAccent('oklch(70% 0.15 30)'); setAccentPurple('oklch(65% 0.18 50)') }} 
              />
              <button 
                className="size-5 rounded-full bg-[oklch(75%_0.12_160)] border border-white/20" 
                onClick={() => { setAccent('oklch(75% 0.12 160)'); setAccentPurple('oklch(70% 0.15 140)') }} 
              />
            </div>

            <div className={rdShell.balancePill}>
              <span className={rdShell.balanceText}>Balance</span>
              <span className={rdShell.balanceValue}>1,250.00</span>
              <button className={rdShell.rechargeBtn}>+</button>
            </div>

            <button className={rdShell.avatarBtn}>
              <div className={rdShell.avatarImg}>PG</div>
              <span className={rdShell.userName}>FatBallFish</span>
              <ChevronIcon className={rdShell.userChevron} />
            </button>
          </div>
        </header>

        {/* Workspace Content */}
        <div className={rdWorkspace.root}>
          <aside className={rdWorkspace.sidebar}>
            <div className={rdWorkspace.sidebarSection}>
              <h3 className={rdWorkspace.sectionTitle}>模型配置</h3>
              <div className="flex flex-col gap-3">
                {models.map(m => (
                  <button 
                    key={m.code}
                    className={cn(rdWorkspace.modelItem, selectedModel === m.code && rdWorkspace.selectItemActive)}
                    onClick={() => setSelectedModel(m.code)}
                  >
                    <div className={rdWorkspace.modelInfo}>
                      <span className={rdWorkspace.itemLabel}>{m.name}</span>
                      <span className={rdWorkspace.itemSub}>Advanced Model</span>
                    </div>
                    <span className={rdWorkspace.modelPoints}>{m.points} ◈</span>
                  </button>
                ))}
              </div>
            </div>

            <div className={rdWorkspace.sidebarSection}>
              <h3 className={rdWorkspace.sectionTitle}>提示词</h3>
              <div className={rdWorkspace.promptWrapper}>
                <textarea 
                  className={rdWorkspace.textarea}
                  placeholder="输入你的奇思妙想..."
                  value={prompt}
                  onChange={(e) => setPrompt(e.target.value)}
                />
              </div>
            </div>

            <div className={rdWorkspace.sidebarSection}>
              <h3 className={rdWorkspace.sectionTitle}>参数设置</h3>
              <div className="mb-6">
                <label className="text-[10px] text-[var(--muted)] uppercase tracking-widest mb-3 block">比例 / Aspect Ratio</label>
                <div className={rdWorkspace.grid3}>
                  {ratios.map(r => (
                    <button 
                      key={r}
                      className={cn(rdWorkspace.selectItem, selectedRatio === r && rdWorkspace.selectItemActive)}
                      onClick={() => setSelectedRatio(r)}
                    >
                      <span className={rdWorkspace.itemLabel}>{r}</span>
                    </button>
                  ))}
                </div>
              </div>
              
              <div>
                <label className="text-[10px] text-[var(--muted)] uppercase tracking-widest mb-3 block">质量 / Quality</label>
                <div className={rdWorkspace.grid}>
                  {qualities.map(q => (
                    <button 
                      key={q}
                      className={cn(rdWorkspace.selectItem, selectedQuality === q && rdWorkspace.selectItemActive)}
                      onClick={() => setSelectedQuality(q)}
                    >
                      <span className={rdWorkspace.itemLabel}>{q}</span>
                    </button>
                  ))}
                </div>
              </div>
            </div>

            <div className={rdWorkspace.actionBar}>
              <div className={rdWorkspace.priceRow}>
                <span className={rdWorkspace.priceLabel}>预计消耗</span>
                <span className={rdWorkspace.priceValue}>
                  <SparklesIconSmall /> 1.25
                </span>
              </div>
              <button className={rdWorkspace.generateBtn}>
                <div className={rdWorkspace.btnGlow} />
                <div className={rdWorkspace.btnText}>
                  开始创作 <ArrowRightIcon />
                </div>
              </button>
            </div>
          </aside>

          <section className={rdWorkspace.canvas}>
            <div className={rdWorkspace.feed}>
              {/* Demo Error View */}
              <div className={rdWorkspace.cardShell}>
                <div className={rdWorkspace.cardInner}>
                  <div className="flex items-start gap-6">
                    <div className="size-14 rounded-2xl bg-red-500/10 border border-red-500/20 grid place-items-center text-red-500">
                      <AlertIcon />
                    </div>
                    <div>
                      <h4 className="text-lg font-bold mb-1">图片格式不受支持</h4>
                      <p className="text-sm text-[var(--muted)] leading-relaxed">
                        由于上游模型限制，我们暂不支持 <code className="bg-white/5 px-1.5 py-0.5 rounded text-red-400">.webp</code> 格式的参考图。
                        请将其转换为 <code className="bg-white/5 px-1.5 py-0.5 rounded text-[var(--fg)]">PNG</code> 或 <code className="bg-white/5 px-1.5 py-0.5 rounded text-[var(--fg)]">JPG</code> 后再次尝试。
                      </p>
                      <div className="mt-4 flex gap-3">
                        <button className="px-4 py-2 bg-red-500 text-white rounded-xl text-xs font-bold">了解更多</button>
                        <button className="px-4 py-2 border border-[var(--border)] rounded-xl text-xs font-bold">忽略</button>
                      </div>
                    </div>
                  </div>
                </div>
              </div>

              {/* Demo Record */}
              <div className={rdWorkspace.cardShell}>
                <div className={rdWorkspace.cardInner}>
                  <div className="flex justify-between items-start mb-6">
                    <div>
                      <span className={cn(rdCommon.badge, 'bg-[var(--accent)]/10 text-[var(--accent)] mb-2')}>文生图 / Text to Image</span>
                      <h4 className="text-xl font-bold">Neon city skyline at midnight</h4>
                    </div>
                    <span className="text-xs font-vault-mono text-[var(--muted)]">2026/06/09 14:30</span>
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="aspect-[16/9] rounded-2xl bg-black/40 overflow-hidden border border-white/5 group relative">
                      <div className="absolute inset-0 bg-gradient-to-t from-black/60 to-transparent opacity-0 group-hover:opacity-100 transition-opacity" />
                      <div className="absolute bottom-4 left-4 right-4 flex justify-between items-center translate-y-2 group-hover:translate-y-0 opacity-0 group-hover:opacity-100 transition-all">
                        <span className="text-[10px] font-bold text-white bg-black/40 backdrop-blur px-2 py-1 rounded">1280 x 720</span>
                        <div className="flex gap-2">
                          <button className="size-8 rounded-lg bg-white/10 backdrop-blur grid place-items-center"><DownloadIconSmall /></button>
                          <button className="size-8 rounded-lg bg-[var(--accent)] text-white grid place-items-center"><ExpandIconSmall /></button>
                        </div>
                      </div>
                    </div>
                    <div className="aspect-[16/9] rounded-2xl bg-black/40 overflow-hidden border border-white/5 animate-pulse flex items-center justify-center">
                       <span className="text-xs text-[var(--muted)]">生成中... 65%</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </section>
        </div>
      </main>
    </div>
  )
}

// Sub-components
function NavItem({ icon, label, short, active = false }: { icon: React.ReactNode; label: string; short: string; active?: boolean }) {
  return (
    <a href="#" className={cn(rdShell.navLink, active && rdShell.navLinkActive)}>
      <div className={cn(rdShell.navLinkIndicator, active && rdShell.navLinkIndicatorActive)} />
      {icon}
      <span className={rdShell.navLabel}>{label}</span>
      <span className={rdShell.navShort}>{short}</span>
    </a>
  )
}

function TopMenuIcon({ color, icon, title, label }: { color: string; icon: React.ReactNode; title: string; label: string }) {
  return (
    <a href="#" className={rdShell.menuItem} style={{ '--icon-color': color, '--icon-rgb': '255, 255, 255' } as any}>
      <div className={rdShell.menuIcon}>{icon}</div>
      <div className={rdShell.menuContent}>
        <span className={rdShell.menuTitle}>{title}</span>
        <span className={rdShell.menuLabel}>{label}</span>
      </div>
    </a>
  )
}

// Icons (Simple SVGs)
const HomeIcon = () => <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M3 9l9-7 9 7v11a2 2 0 01-2 2H5a2 2 0 01-2-2z" /></svg>
const SparklesIcon = () => <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M12 2v20M2 12h20M5 5l14 14M5 19L19 5" /></svg>
const GridIcon = () => <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M9 3v18M15 3v18M3 9h18M3 15h18" /></svg>
const SettingsIcon = () => <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 010 2.83 2 2 0 01-2.83 0l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-2 2 2 2 0 01-2-2v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83 0 2 2 0 010-2.83l.06-.06a1.65 1.65 0 00.33-1.82 1.65 1.65 0 00-1.51-1H3a2 2 0 01-2-2 2 2 0 012-2h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 010-2.83 2 2 0 012.83 0l.06.06a1.65 1.65 0 001.82.33H9a1.65 1.65 0 001-1.51V3a2 2 0 012-2 2 2 0 012 2v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 0 2 2 0 010 2.83l-.06.06a1.65 1.65 0 00-.33 1.82V9a1.65 1.65 0 001.51 1H21a2 2 0 012 2 2 2 0 01-2 2h-.09a1.65 1.65 0 00-1.51 1z" /></svg>
const TemplateIcon = () => <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" /></svg>
const GalleryIcon = () => <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M3 9h18M9 21V9" /></svg>
const DocsIcon = () => <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" /><path d="M14 2v6h6M16 13H8M16 17H8M10 9H8" /></svg>
const ChevronIcon = ({ className }: { className?: string }) => <svg className={className} width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M6 9l6 6 6-6" /></svg>
const SparklesIconSmall = () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M12 2v20M2 12h20M5 5l14 14M5 19L19 5" /></svg>
const ArrowRightIcon = () => <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3"><path d="M5 12h14M12 5l7 7-7 7" /></svg>
const AlertIcon = () => <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="10" /><path d="M12 8v4M12 16h.01" /></svg>
const DownloadIconSmall = () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4M7 10l5 5 5-5M12 15V3" /></svg>
const ExpandIconSmall = () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7" /></svg>
