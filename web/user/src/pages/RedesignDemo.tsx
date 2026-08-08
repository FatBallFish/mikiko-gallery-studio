import React, { useState, useEffect, createContext, useContext, useRef } from 'react'
import { cn } from '../../../shared/classnames'
import { openDocsEntry } from '../docsUrl'
import { rdShell, rdWorkspace, rdCommon, rdHome, rdGallery, rdBilling } from '../ui/redesign-classes'
import heroImage from '../../../../docs/template/PicGallery/mpdhezm8-image.png'

type LightboxPayload = {
  src: string
  prompt?: string
  width?: number
  height?: number
  model?: string
  ratio?: string
}

type BillingPlanId = 'basic' | 'pro' | 'master' | 'custom'
type PaymentMethodId = 'alipay' | 'wechat'
type DemoTab = 'home' | 'studio' | 'gallery' | 'billing' | 'apiKeys' | 'profile' | 'settings'
type GalleryAsset = {
  id: string
  title: string
  group: string
  status: string
  model: string
  ratio: string
  width: number
  height: number
  src: string
}

type ThemeMode = 'dark' | 'light'
type AccentTheme = {
  name: string
  accent: string
  accentPurple: string
  accentRgb: string
}
type GenerationResultItem = {
  id: string
  status: 'success' | 'failed'
  src?: string
  error?: string
}

type ApiKeyMock = {
  id: string
  name: string
  accessKey: string
  secretKey: string
  status: 'active' | 'disabled' | 'expired'
  scopes: string[]
  rpm: number
  quota: string
  createdAt: string
  lastUsedAt: string
  expiresAt: string
}

const DEFAULT_LIGHTBOX_PROMPT = 'Cinematic lighting, hyper detailed, 8k resolution, masterpiece.'
const GALLERY_PROMPT = 'Cinematic product visualization with polished industrial materials, dramatic rim lighting, precise reflection control, editorial composition, premium studio mood, subtle depth of field, color-accurate rendering, highly detailed surfaces, elegant contrast, and a refined commercial campaign finish designed for a high-end creative portfolio.'
const POINT_UNIT_PRICE = 0.03125
const ACCENT_THEMES: AccentTheme[] = [
  { name: '星云紫', accent: 'oklch(65% 0.18 280)', accentPurple: 'oklch(60% 0.2 310)', accentRgb: '131, 118, 255' },
  { name: '落日金', accent: 'oklch(70% 0.15 30)', accentPurple: 'oklch(65% 0.18 50)', accentRgb: '212, 157, 94' },
  { name: '森屿绿', accent: 'oklch(75% 0.12 160)', accentPurple: 'oklch(70% 0.15 140)', accentRgb: '65, 185, 149' },
]
const THEME_TOKENS: Record<ThemeMode, Record<string, string>> = {
  dark: {
    '--bg': 'oklch(12% 0.015 260)',
    '--surface': 'oklch(16% 0.02 260)',
    '--fg': 'oklch(95% 0.01 80)',
    '--muted': 'oklch(90% 0.01 80 / 0.68)',
    '--border': 'oklch(100% 0 0 / 0.1)',
    '--canvas-bg': 'color-mix(in oklch, var(--surface) 40%, black)',
    '--image-overlay': 'linear-gradient(to top, rgb(0 0 0 / 0.9), rgb(0 0 0 / 0.2), rgb(0 0 0 / 0.4))',
    '--image-overlay-selected': 'rgb(0 0 0 / 0.4)',
    '--image-card-text': 'white',
    '--image-card-muted': 'rgb(255 255 255 / 0.62)',
    '--image-action-bg': 'rgb(0 0 0 / 0.45)',
    '--image-action-border': 'rgb(255 255 255 / 0.1)',
    '--image-action-text': 'rgb(255 255 255 / 0.72)',
    '--image-action-hover-bg': 'rgb(255 255 255 / 0.1)',
    '--image-action-hover-text': 'white',
    '--image-checkbox-bg': 'rgb(0 0 0 / 0.45)',
    '--image-checkbox-border': 'rgb(255 255 255 / 0.3)',
    '--lightbox-backdrop': 'rgb(0 0 0 / 0.9)',
    '--lightbox-stage-bg': 'black',
      '--lightbox-close-bg': 'rgb(0 0 0 / 0.5)',
    '--lightbox-close-text': 'white',
    '--lightbox-close-border': 'rgb(255 255 255 / 0.2)',
  },
  light: {
    '--bg': 'oklch(97% 0.012 260)',
    '--surface': 'oklch(100% 0.004 260)',
    '--fg': 'oklch(18% 0.018 260)',
    '--muted': 'oklch(42% 0.02 260 / 0.72)',
    '--border': 'oklch(20% 0.01 260 / 0.12)',
    '--canvas-bg': 'color-mix(in oklch, var(--surface) 86%, var(--accent) 4%)',
    '--image-overlay': 'linear-gradient(to top, rgb(255 255 255 / 0.92), rgb(255 255 255 / 0.58), rgb(255 255 255 / 0.72))',
    '--image-overlay-selected': 'rgb(255 255 255 / 0.58)',
    '--image-card-text': 'var(--fg)',
    '--image-card-muted': 'color-mix(in oklch, var(--fg) 58%, transparent)',
    '--image-action-bg': 'rgb(255 255 255 / 0.82)',
    '--image-action-border': 'color-mix(in oklch, var(--fg) 12%, transparent)',
    '--image-action-text': 'color-mix(in oklch, var(--fg) 72%, transparent)',
    '--image-action-hover-bg': 'color-mix(in oklch, var(--accent) 12%, white)',
    '--image-action-hover-text': 'var(--accent)',
    '--image-checkbox-bg': 'rgb(255 255 255 / 0.82)',
    '--image-checkbox-border': 'color-mix(in oklch, var(--fg) 22%, transparent)',
    '--lightbox-backdrop': 'rgb(22 24 31 / 0.5)',
    '--lightbox-stage-bg': 'color-mix(in oklch, var(--surface) 90%, var(--accent) 3%)',
    '--lightbox-close-bg': 'rgb(255 255 255 / 0.86)',
    '--lightbox-close-text': 'var(--fg)',
    '--lightbox-close-border': 'color-mix(in oklch, var(--fg) 14%, transparent)',
  },
}
const MOCK_GALLERY_ASSETS: GalleryAsset[] = Array.from({ length: 12 }).map((_, index) => {
  const id = `asset-${index + 1}`
  const height = [600, 800, 1000, 1200][(index + 1) % 4]
  return {
    id,
    title: `Visual Artifact #${index + 1}`,
    group: ['默认画集', '产品设计', '人像摄影'][index % 3],
    status: index % 3 === 0 ? '已公开' : '仅自己可见',
    model: index % 2 === 0 ? 'Flux Pro' : 'Midjourney v6',
    ratio: ['16:9 横屏', '9:16 竖屏', '1:1 方形'][index % 3],
    width: 800,
    height,
    src: `https://picsum.photos/seed/${index + 43}/800/${height}`,
  }
})
const MOCK_API_KEYS: ApiKeyMock[] = [
  {
    id: 'api-key-1',
    name: 'Production Render Worker',
    accessKey: 'ak_live_8F2A6D9C1B4E9C31',
    secretKey: 'sk_live_Kf92aQx7nV3pDk6R18',
    status: 'active',
    scopes: ['images:write', 'images:read', 'balance:read'],
    rpm: 120,
    quota: '8,500 / 10,000 ◈',
    createdAt: '2026/05/22',
    lastUsedAt: '2 分钟前',
    expiresAt: '2026/12/31',
  },
  {
    id: 'api-key-2',
    name: 'Staging Playground',
    accessKey: 'ak_test_35B8C02E7A91F0D5',
    secretKey: 'sk_test_uY72mNq4tLp9W3sA01',
    status: 'disabled',
    scopes: ['images:write', 'images:read'],
    rpm: 30,
    quota: '240 / 1,000 ◈',
    createdAt: '2026/04/18',
    lastUsedAt: '昨天',
    expiresAt: '永不过期',
  },
  {
    id: 'api-key-3',
    name: 'Archive Automation',
    accessKey: 'ak_live_E77F490BC212AB68',
    secretKey: 'sk_live_Zp48Rb2JmK01Ya6L53',
    status: 'expired',
    scopes: ['images:read', 'profile:read'],
    rpm: 10,
    quota: '930 / 2,000 ◈',
    createdAt: '2026/03/02',
    lastUsedAt: '2026/05/30',
    expiresAt: '2026/06/01',
  },
]

const LightboxContext = createContext<{
  openLightbox: (payload: LightboxPayload) => void
}>({
  openLightbox: () => {},
})

export function RedesignDemo() {
  const [activeTab, setActiveTab] = useState<DemoTab>('home')
  const [accentTheme, setAccentTheme] = useState(ACCENT_THEMES[0])
  const [themeMode, setThemeMode] = useState<ThemeMode>('dark')
  const [avatarMenuOpen, setAvatarMenuOpen] = useState(false)
  const avatarMenuRef = useRef<HTMLDivElement | null>(null)
  
  const [lightboxOpen, setLightboxOpen] = useState(false)
  const [lightboxImage, setLightboxImage] = useState<LightboxPayload | null>(null)

  const openLightbox = (payload: LightboxPayload) => {
    setLightboxImage(payload)
    setLightboxOpen(true)
  }

  const closeLightbox = () => {
    setLightboxOpen(false)
  }
  
  useEffect(() => {
    const root = document.documentElement
    Object.entries(THEME_TOKENS[themeMode]).forEach(([key, value]) => root.style.setProperty(key, value))
    root.style.setProperty('--accent', accentTheme.accent)
    root.style.setProperty('--accent-purple', accentTheme.accentPurple)
    root.style.setProperty('--accent-rgb', accentTheme.accentRgb)
  }, [accentTheme, themeMode])

  useEffect(() => {
    if (!avatarMenuOpen) return undefined
    const close = (event: MouseEvent) => {
      if (!avatarMenuRef.current?.contains(event.target as Node)) setAvatarMenuOpen(false)
    }
    window.addEventListener('mousedown', close)
    return () => window.removeEventListener('mousedown', close)
  }, [avatarMenuOpen])

  useEffect(() => {
    if (!lightboxOpen) return undefined
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') closeLightbox()
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [lightboxOpen])

  const navigate = (tab: DemoTab) => {
    setActiveTab(tab)
    setAvatarMenuOpen(false)
  }

  return (
    <LightboxContext.Provider value={{ openLightbox }}>
      <div className={cn(rdShell.shellWrapper, 'redesign-demo-scope')}>
        <div className={rdShell.shell}>
        <aside className={rdShell.sidebar}>
          <div className={rdShell.brand} onClick={() => navigate('home')}>
            <div className={rdShell.brandOrb}>V</div>
          </div>
          
          <nav className={rdShell.nav}>
            <NavItem icon={<HomeIcon />} label="首页" active={activeTab === 'home'} onClick={() => navigate('home')} />
            <NavItem icon={<SparklesIcon />} label="创作" active={activeTab === 'studio'} onClick={() => navigate('studio')} />
            <NavItem icon={<GridIcon />} label="资产" active={activeTab === 'gallery'} onClick={() => navigate('gallery')} />
            <NavItem icon={<CreditCardIcon />} label="积分" active={activeTab === 'billing'} onClick={() => navigate('billing')} />
            <NavItem icon={<KeyIcon />} label="密钥" active={activeTab === 'apiKeys'} onClick={() => navigate('apiKeys')} />
            <NavItem icon={<SettingsIcon />} label="设置" active={activeTab === 'settings'} onClick={() => navigate('settings')} />
          </nav>
        </aside>

        <main className={rdShell.main}>
          <header className={rdShell.topbar}>
            <div className={rdShell.userTools}>
              <button
                type="button"
                aria-label={themeMode === 'dark' ? '切换浅色主题' : '切换深色主题'}
                title={themeMode === 'dark' ? '切换浅色主题' : '切换深色主题'}
                className="grid size-11 place-items-center rounded-2xl border border-[var(--border)] bg-[var(--surface)] text-[var(--fg)] transition-all hover:border-[var(--accent)] hover:text-[var(--accent)]"
                onClick={() => setThemeMode(mode => mode === 'dark' ? 'light' : 'dark')}
              >
                {themeMode === 'dark' ? <MoonIcon /> : <SunIcon />}
              </button>

              <div className={rdShell.balancePill}>
                <span className={rdShell.balanceText}>◈</span>
                <span className={rdShell.balanceValue}>1,250.00</span>
                <button className={rdShell.rechargeBtn} onClick={() => navigate('billing')}>+</button>
              </div>

              <div className="relative" ref={avatarMenuRef}>
                <button className={rdShell.avatarBtn} aria-haspopup="menu" aria-expanded={avatarMenuOpen} onClick={() => setAvatarMenuOpen(open => !open)}>
                  <div className={rdShell.avatarImg}>PG</div>
                  <span className={rdShell.userName}>FatBallFish</span>
                  <ChevronIcon className={cn(rdShell.userChevron, avatarMenuOpen && 'rotate-180')} />
                </button>
                {avatarMenuOpen && (
                  <div className="absolute right-0 top-[calc(100%+12px)] z-50 w-56 rounded-2xl border border-[var(--border)] bg-[var(--surface)]/95 p-2 shadow-2xl shadow-black/30 backdrop-blur-2xl" role="menu">
                    <AvatarMenuItem icon={<UserIcon />} label="个人中心" onClick={() => navigate('profile')} />
                    <AvatarMenuItem icon={<CreditCardIcon />} label="积分充值" onClick={() => navigate('billing')} />
                    <AvatarMenuItem icon={<KeyIcon />} label="API 密钥" onClick={() => navigate('apiKeys')} />
                    <AvatarMenuItem icon={<DocsIcon />} label="开发文档" onClick={() => openDocsEntry('account-menu')} />
                    <div className="my-2 h-px bg-[var(--border)]" />
                    <AvatarMenuItem icon={<LogoutIcon />} label="退出登录" tone="danger" onClick={() => setAvatarMenuOpen(false)} />
                  </div>
                )}
              </div>
            </div>
          </header>

          <div className={rdShell.contentConstrain}>
            <div className="flex-1 flex flex-col min-h-0">
              {activeTab === 'home' && <HomeView onStart={() => setActiveTab('studio')} />}
              {activeTab === 'studio' && <StudioView />}
              {activeTab === 'gallery' && <GalleryView />}
              {activeTab === 'billing' && <BillingView />}
              {activeTab === 'apiKeys' && <ApiKeysView />}
              {activeTab === 'profile' && <ProfileView />}
              {activeTab === 'settings' && <SettingsView activeTheme={accentTheme} onThemeChange={setAccentTheme} themeMode={themeMode} onThemeModeChange={setThemeMode} />}
            </div>

            <footer className={rdShell.footer}>
              <div className={rdShell.footerContent}>
                <div className="flex flex-col md:flex-row items-center gap-2">
                  <span>© 2026 Mikiko Studio. All rights reserved.</span>
                  <span className="hidden md:inline text-[var(--border)]">|</span>
                </div>
                <div className={rdShell.footerLinks}>
                  <span className={rdShell.footerLink}>服务协议</span>
                  <span className={rdShell.footerLink}>隐私条款</span>
                  <button className={cn(rdShell.footerLink, 'border-0 bg-transparent p-0 text-inherit')} type="button" onClick={() => openDocsEntry('footer')}>API 文档</button>
                </div>
              </div>
            </footer>
          </div>

          <nav className={rdShell.mobileNav}>
            <button className={cn(rdShell.mobileNavLink, activeTab === 'home' && rdShell.mobileNavLinkActive)} onClick={() => navigate('home')}>
              <HomeIcon />
              <span className="text-[10px] font-bold">首页</span>
            </button>
            <button className={cn(rdShell.mobileNavLink, activeTab === 'studio' && rdShell.mobileNavLinkActive)} onClick={() => navigate('studio')}>
              <SparklesIcon />
              <span className="text-[10px] font-bold">创作</span>
            </button>
            <button className={cn(rdShell.mobileNavLink, activeTab === 'gallery' && rdShell.mobileNavLinkActive)} onClick={() => navigate('gallery')}>
              <GridIcon />
              <span className="text-[10px] font-bold">资产</span>
            </button>
            <button className={cn(rdShell.mobileNavLink, activeTab === 'apiKeys' && rdShell.mobileNavLinkActive)} onClick={() => navigate('apiKeys')}>
              <KeyIcon />
              <span className="text-[10px] font-bold">密钥</span>
            </button>
          </nav>
        </main>
      </div>
      </div>

      {lightboxOpen && lightboxImage && (
        <div className="fixed inset-0 z-[100] bg-[var(--lightbox-backdrop)] backdrop-blur-xl flex items-center justify-center p-4 sm:p-10 animate-in fade-in duration-300" onClick={closeLightbox}>
          <div className="bg-[var(--bg)] border border-[var(--border)] rounded-3xl overflow-hidden max-w-6xl w-full max-h-[92vh] flex flex-col md:flex-row shadow-2xl relative" onClick={event => event.stopPropagation()}>
            <button onClick={closeLightbox} className="absolute top-4 right-4 z-10 w-8 h-8 rounded-full bg-[var(--lightbox-close-bg)] text-[var(--lightbox-close-text)] flex items-center justify-center border border-[var(--lightbox-close-border)] shadow-lg hover:scale-105 transition">✕</button>
            <div className="flex-1 bg-[var(--lightbox-stage-bg)] flex items-center justify-center p-6">
              <img src={lightboxImage.src} alt="Enlarged" className="max-h-[80vh] w-auto object-contain rounded-xl shadow-xl" />
            </div>
            <div className="w-full md:w-96 border-t md:border-t-0 md:border-l border-[var(--border)] p-8 flex flex-col justify-between gap-6 bg-[var(--surface)] overflow-y-auto">
              <div className="flex flex-col gap-5">
                <span className="text-[10px] font-bold text-[var(--accent)] uppercase tracking-wider">画卷配置详情</span>
                <div className="grid grid-cols-2 gap-3">
                  <LightboxInfo label="像素" value={formatImagePixels(lightboxImage)} />
                  <LightboxInfo label="比例" value={lightboxImage.ratio ?? deriveAspectRatio(lightboxImage)} />
                  <LightboxInfo label="模型" value={lightboxImage.model ?? 'Flux Pro'} />
                  <LightboxInfo label="来源" value="Mikiko Studio" />
                </div>
                <div>
                  <span className="text-xs text-[var(--muted)] block mb-2">提示词</span>
                  <p className="text-sm text-[var(--fg)] leading-relaxed bg-[var(--bg)] p-4 rounded-2xl border border-[var(--border)] max-h-44 overflow-y-auto">
                    "{lightboxImage.prompt ?? DEFAULT_LIGHTBOX_PROMPT}"
                  </p>
                </div>
              </div>
              <div className="flex flex-col gap-3">
                <button onClick={closeLightbox} className="w-full py-3 rounded-xl bg-[var(--accent)] text-white font-bold text-sm hover:scale-[1.02] transition-transform">导入该作参数复刻</button>
                <button className="w-full py-3 rounded-xl bg-[var(--bg)] border border-[var(--border)] hover:border-[var(--accent)] text-[var(--fg)] font-bold text-sm transition-colors">无损下载原图</button>
              </div>
            </div>
          </div>
        </div>
      )}
    </LightboxContext.Provider>
  )
}

function HomeView({ onStart }: { onStart: () => void }) {
  return (
    <div className="p-6 md:p-10 flex-1 w-full">
      <section className={rdHome.hero}>
        <img src={heroImage} alt="Hero" className={rdHome.heroImg} />
        <div className={rdHome.heroOverlay} />
        <div className={rdHome.heroContent}>
          <h1 className={rdHome.heroTitle}>Cinematic Product Visualization</h1>
          <p className={rdHome.heroText}>探索 AI 创作的无限可能。从精准的工业设计到梦幻的场景构建，Mikiko Studio 为你提供专业级的生图引擎与 API 支持。</p>
          <div className={rdHome.heroActions}>
            <button className="h-14 px-8 rounded-2xl bg-[var(--accent)] text-white font-bold text-lg hover:scale-105 transition-transform" onClick={onStart}>开始创作</button>
            <button className="h-14 px-8 rounded-2xl border border-white/20 backdrop-blur-xl text-white font-bold text-lg hover:bg-white/5 transition-colors">查看文档</button>
          </div>
        </div>
      </section>

      <section>
        <div className="flex flex-col md:flex-row justify-between items-start md:items-end mb-10 gap-4">
          <div>
            <h2 className="text-3xl md:text-4xl font-black mb-2">灵感发现</h2>
            <p className="text-sm md:text-base text-[var(--muted)]">来自社区最前沿的创意展示</p>
          </div>
          <div className="flex gap-2">
            <button className="px-5 py-2 rounded-xl bg-[var(--accent)] text-white font-bold text-sm">精选</button>
            <button className="px-5 py-2 rounded-xl border border-[var(--border)] text-[var(--muted)] font-bold text-sm">最新</button>
          </div>
        </div>
        <div className={rdGallery.masonry}>
           {[1,2,3,4,5,6].map(i => <GalleryItem key={i} index={i} isBatchMode={false} context="home" />)}
        </div>
      </section>
    </div>
  )
}

function StudioView() {
  const { openLightbox } = useContext(LightboxContext)
  const qualityOptions = [
    { value: 'Standard', label: '标准' },
    { value: 'High', label: '高清' },
    { value: 'Ultra', label: '超清' },
  ]
  const [selectedModel, setSelectedModel] = useState('v-flux-pro')
  const [selectedRatio, setSelectedRatio] = useState('16:9')
  const [selectedQuality, setSelectedQuality] = useState('High')
  const [selectedCount, setSelectedCount] = useState(1)
  const [isUploadOpen, setIsUploadOpen] = useState(false)
  const [isGalleryImportOpen, setIsGalleryImportOpen] = useState(false)
  const [uploadedImages, setUploadedImages] = useState<string[]>([])
  
  const [isGenerating, setIsGenerating] = useState(false)
  const [progress, setProgress] = useState(0)
  const [stage, setStage] = useState('')
  const [result, setResult] = useState<GenerationResultItem[] | null>(null)
  
  const [prompt, setPrompt] = useState('')
  const [isEnhancing, setIsEnhancing] = useState(false)
  
  const handleUploadClick = () => {
    if (uploadedImages.length < 5) {
      setUploadedImages([...uploadedImages, `https://picsum.photos/seed/${Math.random()}/200/200`])
    }
  }

  const handleGalleryImport = (assets: GalleryAsset[]) => {
    const imported = assets.map(asset => asset.src)
    setUploadedImages(current => [...current, ...imported].slice(0, 5))
    setIsUploadOpen(true)
  }

  const handleEnhance = () => {
    setIsEnhancing(true)
    setTimeout(() => {
      setPrompt("Cinematic hyperrealistic 8k render, breathtaking landscape, volumetric lighting, unreal engine 5, masterpiece, trending on artstation.")
      setIsEnhancing(false)
    }, 2000)
  }

  const handleGenerate = () => {
    setIsGenerating(true)
    setResult(null)
    setProgress(5)
    setStage("正在分析提示词结构...")
    
    setTimeout(() => { setProgress(35); setStage("神经网络噪音注入完成，开始主去噪循环...") }, 1000)
    setTimeout(() => { setProgress(65); setStage("深度扩散合成中 (步数 20/30)...") }, 2500)
    setTimeout(() => { setProgress(85); setStage("超分辨率细节增强与色彩校正中...") }, 4000)
    setTimeout(() => { setProgress(98); setStage("正在渲染最终无损图像...") }, 5000)
    
    setTimeout(() => {
      setIsGenerating(false)
      const mockResults = Array.from({ length: selectedCount }).map((_, i) => {
        const shouldFail = selectedCount > 1 ? i === selectedCount - 1 : Math.random() > 0.82
        if (shouldFail) {
          return {
            id: `failed-${Date.now()}-${i}`,
            status: 'failed' as const,
            error: i % 2 === 0 ? '上游内容安全策略拒绝了这张图片。' : '对象存储写入超时，未扣除该图积分。',
          }
        }
        return {
          id: `success-${Date.now()}-${i}`,
          status: 'success' as const,
          src: `https://picsum.photos/seed/${Math.random()}/1280/720`,
        }
      })
      setResult(mockResults)
    }, 6000)
  }

  const openGeneratedImage = (src: string) => {
    openLightbox({
      src,
      prompt: prompt.trim() || DEFAULT_LIGHTBOX_PROMPT,
      width: 1280,
      height: 720,
      ratio: selectedRatio,
      model: selectedModel === 'v-flux-pro' ? 'Flux Pro' : 'Midjourney v6',
    })
  }
  
  return (
    <div className={rdWorkspace.root}>
      <aside className={rdWorkspace.sidebar}>
        <div className={rdWorkspace.sidebarScroll}>
          <div className={rdWorkspace.sidebarSection}>
            <h3 className={rdWorkspace.sectionTitle}>选择模型</h3>
            <div className="flex flex-col gap-2">
              <ModelSelect active={selectedModel === 'v-flux-pro'} name="Flux Pro" points="1.2" onClick={() => setSelectedModel('v-flux-pro')} />
              <ModelSelect active={selectedModel === 'v-mj-v6'} name="Midjourney v6" points="1.5" onClick={() => setSelectedModel('v-mj-v6')} />
            </div>
          </div>

          <div className={rdWorkspace.sidebarSection}>
            <button className={rdWorkspace.uploadTrigger} onClick={() => setIsUploadOpen(!isUploadOpen)}>
              <div className="flex items-center gap-2 text-[var(--fg)]">
                <ImageIcon className="size-4 text-[var(--accent)]" />
                <span className="text-[11px] font-bold uppercase tracking-wider">参考图片 ({uploadedImages.length}/5)</span>
              </div>
              <ChevronIcon className={cn("size-4 transition-transform", isUploadOpen && "rotate-180")} />
            </button>
            <div className={rdWorkspace.uploadSection} style={{ maxHeight: isUploadOpen ? '300px' : '0' }}>
              <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                <div className={rdWorkspace.uploadBox} onClick={handleUploadClick}>
                  <div className={rdWorkspace.uploadDashed}>
                    <UploadIcon className="size-5 text-[var(--muted)]" />
                    <span className="text-[10px] text-[var(--muted)]">本地上传</span>
                  </div>
                </div>
                <button className={rdWorkspace.uploadBox} type="button" onClick={() => setIsGalleryImportOpen(true)}>
                  <div className={rdWorkspace.uploadDashed}>
                    <GridIcon />
                    <span className="text-[10px] text-[var(--muted)]">从图库导入</span>
                  </div>
                </button>
              </div>
              <div className="mt-2 text-[10px] text-[var(--muted)]">
                最多选择 5 张参考图，图库导入默认支持批量选择。
              </div>
              {uploadedImages.length > 0 && (
                <div className={rdWorkspace.uploadGrid}>
                  {uploadedImages.map((img, i) => (
                    <div key={`${img}-${i}`} className={rdWorkspace.uploadThumb}>
                      <img src={img} className={rdWorkspace.uploadImg} alt="Uploaded" />
                      <button 
                        className={rdWorkspace.uploadRemove} 
                        onClick={(e) => { e.stopPropagation(); setUploadedImages(uploadedImages.filter((_, idx) => idx !== i)) }}
                      >
                        ✕
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>

          <div className={rdWorkspace.sidebarSection}>
            <div className="flex items-center justify-between mb-3">
              <h3 className={cn(rdWorkspace.sectionTitle, "mb-0")}>提示词</h3>
              <button 
                onClick={handleEnhance} 
                disabled={isEnhancing} 
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-[var(--accent)]/10 text-[var(--accent)] hover:bg-[var(--accent)]/20 transition-colors text-[10px] font-bold disabled:opacity-50 disabled:cursor-wait"
              >
                <WandIcon className={cn("size-3", isEnhancing && "animate-spin")} />
                {isEnhancing ? '优化中...' : '魔法优化'}
              </button>
            </div>
            <div className={rdWorkspace.promptWrapper}>
              <textarea 
                className={cn(rdWorkspace.textarea, 'redesign-prompt-input')} 
                placeholder="描述画面细节、风格和光影..." 
                value={prompt}
                onChange={e => setPrompt(e.target.value)}
              />
            </div>
          </div>

          <div className={rdWorkspace.sidebarSection}>
            <h3 className={rdWorkspace.sectionTitle}>比例</h3>
            <div className={rdWorkspace.grid3}>
              {['1:1', '4:3', '16:9', '9:16', '3:4', '2:3'].map(r => (
                <SelectItem key={r} active={selectedRatio === r} label={r} onClick={() => setSelectedRatio(r)}>
                  <AspectRatioIcon ratio={r} active={selectedRatio === r} />
                </SelectItem>
              ))}
            </div>
          </div>

          <div className={rdWorkspace.sidebarSection}>
            <h3 className={rdWorkspace.sectionTitle}>质量</h3>
            <div className={rdWorkspace.grid3}>
              {qualityOptions.map(q => <SelectItem key={q.value} active={selectedQuality === q.value} label={q.label} onClick={() => setSelectedQuality(q.value)} />)}
            </div>
          </div>

          <div className={rdWorkspace.sidebarSection}>
            <h3 className={rdWorkspace.sectionTitle}>数量</h3>
            <div className={rdWorkspace.grid4}>
              {[1, 2, 3, 4].map(n => <SelectItem key={n} active={selectedCount === n} label={`${n}`} onClick={() => setSelectedCount(n)} />)}
            </div>
          </div>
        </div>

        <div className={rdWorkspace.actionBar}>
          <div className={rdWorkspace.priceRow}>
            <span className={rdWorkspace.priceLabel}>预计消耗</span>
            <span className={rdWorkspace.priceValue}>◈ {(1.25 * selectedCount).toFixed(2)}</span>
          </div>
          <button className={rdWorkspace.generateBtn} onClick={handleGenerate} disabled={isGenerating}>
            <div className={rdWorkspace.btnGlow} />
            <div className={rdWorkspace.btnText}>
              {isGenerating ? '生成中...' : '开始创作'} <ArrowRightIcon />
            </div>
          </button>
        </div>
      </aside>

      <section className={rdWorkspace.canvas}>
        <div className={rdWorkspace.outputPanel}>
          {!isGenerating && !result && (
            <div className="flex flex-col items-center justify-center text-center max-w-sm mx-auto opacity-50 m-auto">
              <SparklesIcon />
              <h3 className="text-xl font-bold mt-4 mb-2">准备就绪</h3>
              <p className="text-sm text-[var(--muted)]">在左侧设置参数并输入提示词，点击「开始创作」即可生成惊艳画面。</p>
            </div>
          )}

          {isGenerating && (
            <div className={rdWorkspace.outputLoading + ' m-auto'}>
              <div className={rdWorkspace.outputRing}>
                <div className={rdWorkspace.outputRingInner1} />
                <div className={rdWorkspace.outputRingInner2} />
                <div className={rdWorkspace.outputRingCore}>
                  <SparklesIcon />
                </div>
              </div>
              <h4 className={rdWorkspace.outputStage}>Mikiko Studio 引擎解算中</h4>
              <p className={rdWorkspace.outputStageText}>{stage}</p>
              <div className={rdWorkspace.outputProgressWrap}>
                <div className={rdWorkspace.outputProgressBar} style={{ width: `${progress}%` }} />
              </div>
              <div className="w-full flex justify-between items-center mt-2 text-[10px] text-[var(--muted)]">
                <span>深度模型计算</span>
                <span className="font-mono font-medium text-[var(--accent)]">{progress}%</span>
              </div>
            </div>
          )}

          {result && !isGenerating && (
            <div className={rdWorkspace.outputImageWrap}>
              
              {result.length === 1 ? (
                <GenerationResultCard item={result[0]} variant="single" onOpen={openGeneratedImage} />
              ) : (
                <div className={rdWorkspace.outputGridMultiple}>
                   {result.map((item) => (
                     <GenerationResultCard key={item.id} item={item} variant="grid" onOpen={openGeneratedImage} />
                   ))}
                </div>
              )}
              
              <div className={rdWorkspace.outputActions}>
                <button className={rdWorkspace.outputBtn} title="下载"><DownloadIconSmall /> {result.filter(item => item.status === 'success').length > 1 ? '全部' : '下载'}</button>
                <div className="w-px h-4 bg-[var(--border)] mx-1" />
                <button className={rdWorkspace.outputBtn} title="复制提示词"><CopyIcon /> 提示词</button>
                <div className="w-px h-4 bg-[var(--border)] mx-1" />
                <button className={rdWorkspace.outputBtn} title="再次编辑"><EditIcon /> 编辑</button>
                <div className="w-px h-4 bg-[var(--border)] mx-1" />
                {result.find(item => item.status === 'success' && item.src) && (
                  <button className={rdWorkspace.outputBtn} title="全屏查看" onClick={() => {
                    const firstSuccess = result.find(item => item.status === 'success' && item.src)
                    if (firstSuccess?.src) openGeneratedImage(firstSuccess.src)
                  }}><ExpandIconSmall /> 全屏</button>
                )}
              </div>
            </div>
          )}

          <div className={rdWorkspace.outputMetaRow}>
            <div className="flex flex-col sm:flex-row items-start sm:items-center gap-2 sm:gap-4">
              <span className="flex items-center gap-1.5"><ClockIcon /> 生成耗时: 6.2s</span>
              <span className="hidden sm:inline">|</span>
              <span>模型: {selectedModel === 'v-flux-pro' ? 'Flux Pro' : 'Midjourney v6'}</span>
            </div>
            <div className="flex flex-col sm:flex-row items-end sm:items-center gap-2 sm:gap-4">
              <span>节点: AWS-SG-MultiCloud</span>
              <span className="hidden sm:inline">|</span>
              <span>分辨率: 1280x720</span>
            </div>
          </div>
        </div>
      </section>
      {isGalleryImportOpen && (
        <GalleryImportModal
          selectedLimit={Math.max(0, 5 - uploadedImages.length)}
          onClose={() => setIsGalleryImportOpen(false)}
          onConfirm={handleGalleryImport}
        />
      )}
    </div>
  )
}

function GalleryView() {
  const [isBatchMode, setIsBatchMode] = useState(false)

  return (
    <div className="p-6 md:p-10 flex-1 w-full">
      <div className="flex flex-col md:flex-row justify-between items-start md:items-end mb-8 gap-6">
        <div>
          <h1 className="text-4xl md:text-6xl font-black">历史资产</h1>
        </div>
        <div className="flex flex-col sm:flex-row gap-4 w-full md:w-auto">
          <div className="relative w-full sm:w-80">
             <input type="text" className="h-12 w-full pl-10 pr-4 rounded-xl bg-[var(--surface)] border border-[var(--border)] text-sm focus:outline-none focus:border-[var(--accent)] focus:ring-1 focus:ring-[var(--accent)]/50 transition-all" placeholder="搜索提示词或风格特征..." />
             <SearchIcon className="absolute left-3 top-3.5 size-5 text-[var(--muted)]" />
          </div>
          <button 
            className={cn("h-12 px-6 rounded-xl border text-sm font-bold transition-all whitespace-nowrap", isBatchMode ? "bg-[var(--accent)] text-white border-[var(--accent)] shadow-lg shadow-[var(--accent)]/20" : "border-[var(--border)] bg-[var(--surface)] hover:border-[var(--accent)] text-[var(--fg)]")} 
            onClick={() => setIsBatchMode(!isBatchMode)}
          >
            {isBatchMode ? '完成选择' : '批量操作'}
          </button>
        </div>
      </div>

      <div className={rdGallery.toolbar}>
        <div className={rdGallery.filterGroup}>
          <CustomSelect options={['全部分组', '默认画集', '产品设计', '人像摄影']} />
          <CustomSelect options={['公开状态', '仅自己可见', '已公开']} />
          <CustomSelect options={['所有模型', 'Flux Pro', 'Midjourney v6']} />
          <CustomSelect options={['所有比例', '16:9 横屏', '9:16 竖屏', '1:1 方形']} />
        </div>
        <div className="text-xs text-[var(--muted)] whitespace-nowrap">共 1,248 个结果</div>
      </div>
      
      <div className={rdGallery.masonry}>
        {[1,2,3,4,5,6,7,8,9,10,11,12].map(i => <GalleryItem key={i} index={i} isBatchMode={isBatchMode} />)}
      </div>

      {isBatchMode && (
        <div className={rdGallery.batchBar}>
          <div className={rdGallery.batchCount}>已选择 3 项</div>
          <div className="flex items-center gap-1 pl-2">
            <button className={rdGallery.batchBtn}><DownloadIconSmall /> 打包下载</button>
            <button className={rdGallery.batchBtn}><FolderIcon /> 设为分组</button>
            <button className={rdGallery.batchBtn}><GlobeIcon /> 公开</button>
            <div className="w-px h-4 bg-[var(--border)] mx-1" />
            <button className={cn(rdGallery.batchBtn, "text-red-500 hover:text-red-400 hover:bg-red-500/10")}><TrashIcon /> 删除</button>
          </div>
        </div>
      )}
    </div>
  )
}

function BillingView() {
  const [selectedPlan, setSelectedPlan] = useState<BillingPlanId>('pro')
  const [selectedPayment, setSelectedPayment] = useState<PaymentMethodId>('alipay')
  const [customAmount, setCustomAmount] = useState('100')
  const customAmountValue = clampAmount(Number(customAmount) || 0)
  const customPoints = customAmountValue / POINT_UNIT_PRICE
  const planDetails: Record<BillingPlanId, { name: string; points: number; price: number }> = {
    basic: { name: '创作者入门', points: 50, price: 49 },
    pro: { name: '专业创作者', points: 140, price: 99 },
    master: { name: '大师工作坊', points: 360, price: 199 },
    custom: { name: '自定义金额', points: customPoints, price: customAmountValue },
  }
  const selectedPlanDetails = planDetails[selectedPlan]

  const handleCustomAmountChange = (value: string) => {
    const normalized = value.replace(/[^\d.]/g, '')
    const parts = normalized.split('.')
    const nextValue = parts.length > 1 ? `${parts[0]}.${parts.slice(1).join('').slice(0, 2)}` : parts[0]
    setCustomAmount(nextValue)
    setSelectedPlan('custom')
  }
  
  return (
    <div className="p-6 md:p-10 flex-1 w-full">
      <div className="mb-12">
        <h1 className="text-4xl md:text-6xl font-black mb-4">充值积分</h1>
        <p className="text-lg text-[var(--muted)]">选择适合您的创作额度，积分永久有效。</p>
      </div>

      <div className={rdBilling.layout}>
        <div className="flex flex-col gap-8">
          <div className={rdBilling.card}>
            <h3 className="text-sm font-bold text-[var(--muted)] mb-8">选择积分包</h3>
            <div className={rdBilling.planGrid}>
               <PlanItem active={selectedPlan === 'basic'} name="创作者入门" points="50.00" price="¥ 49" onClick={() => setSelectedPlan('basic')} />
               <PlanItem active={selectedPlan === 'pro'} name="专业创作者" points="120.00" price="¥ 99" bonus="+ 20 ◈" onClick={() => setSelectedPlan('pro')} />
               <PlanItem active={selectedPlan === 'master'} name="大师工作坊" points="300.00" price="¥ 199" bonus="+ 60 ◈" onClick={() => setSelectedPlan('master')} />
               <PlanItem active={selectedPlan === 'custom'} name="自定义金额" points={formatPoints(customPoints)} price="¥0.03125 / 积分" onClick={() => setSelectedPlan('custom')}>
                 <div className="mt-4 w-full rounded-2xl border border-[var(--border)] bg-[var(--surface)]/80 px-4 py-3" onClick={e => { e.stopPropagation(); setSelectedPlan('custom') }}>
                   <label className="mb-2 block text-[10px] font-vault-mono uppercase tracking-widest text-[var(--muted)]">支付金额 / 1-10000</label>
                   <div className="flex items-center gap-2">
                     <span className="text-sm font-bold text-[var(--muted)]">¥</span>
                     <input
                       value={customAmount}
                       inputMode="decimal"
                       min="1"
                       max="10000"
                       onFocus={() => setSelectedPlan('custom')}
                       onChange={e => handleCustomAmountChange(e.target.value)}
                       onBlur={() => setCustomAmount(String(clampAmount(Number(customAmount) || 1)))}
                       className="w-full bg-transparent text-xl font-black text-[var(--fg)] outline-none placeholder:text-[var(--muted)]"
                       placeholder="100"
                     />
                   </div>
                 </div>
               </PlanItem>
            </div>
          </div>

          <div className={rdBilling.card}>
            <h3 className="text-sm font-bold text-[var(--muted)] mb-8">支付方式</h3>
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
               <button 
                 className={cn("group flex items-center gap-3 p-6 rounded-[2rem] border bg-[var(--bg)]/50 transition-all duration-300 hover:border-[var(--accent)] cursor-pointer", selectedPayment === 'alipay' ? rdBilling.planActive : "border-[var(--border)] text-[var(--muted)]")}
                 onClick={() => setSelectedPayment('alipay')}
               >
                 <div className="size-10 rounded-lg bg-white/10 grid place-items-center"><AliPayIcon /></div>
                 <span className="font-bold">支付宝</span>
               </button>
               <button 
                 className={cn("group flex items-center gap-3 p-6 rounded-[2rem] border bg-[var(--bg)]/50 transition-all duration-300 hover:border-[var(--accent)] cursor-pointer", selectedPayment === 'wechat' ? rdBilling.planActive : "border-[var(--border)] text-[var(--muted)]")}
                 onClick={() => setSelectedPayment('wechat')}
               >
                 <div className="size-10 rounded-lg bg-white/5 grid place-items-center"><WeChatIcon /></div>
                 <span className="font-bold">微信支付</span>
               </button>
            </div>
          </div>
        </div>

        <aside>
          <div className={rdBilling.orderPanel}>
            <h3 className={rdBilling.orderTitle}>订单详情</h3>
            <div className={rdBilling.orderRow}>
              <span className="text-[var(--muted)]">购买项目</span>
              <span className="font-bold">{selectedPlanDetails.name}</span>
            </div>
            <div className={rdBilling.orderRow}>
              <span className="text-[var(--muted)]">到账积分</span>
              <span className="font-bold text-[var(--accent)]">{formatPoints(selectedPlanDetails.points)} ◈</span>
            </div>
            <div className={rdBilling.orderRow}>
              <span className="text-[var(--muted)]">支付金额</span>
              <span className="font-bold">¥ {selectedPlanDetails.price.toFixed(2)}</span>
            </div>
            <div className={rdBilling.orderRow + ' mt-4'}>
              <span className="text-xl font-black">应付总额</span>
              <span className={rdBilling.orderTotal}>¥ {selectedPlanDetails.price.toFixed(2)}</span>
            </div>
            <button className={rdShell.rechargeBtn + ' w-full h-16 rounded-2xl mt-4 scale-100'}>
               <span className="text-lg font-bold">立即支付</span>
            </button>
          </div>
        </aside>
      </div>
    </div>
  )
}

function ApiKeysView() {
  const activeCount = MOCK_API_KEYS.filter(key => key.status === 'active').length

  return (
    <div className="p-6 md:p-10 flex-1 w-full">
      <div className="mb-10 flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <h1 className="text-4xl md:text-6xl font-black mb-4">API 密钥</h1>
          <p className="max-w-2xl text-base text-[var(--muted)] leading-relaxed">管理开放接口调用凭证、限速和额度。密钥仅以首尾明文展示，中间内容始终隐藏。</p>
        </div>
        <button type="button" className="h-12 rounded-2xl bg-[var(--accent)] px-6 text-sm font-bold text-white shadow-lg shadow-[var(--accent)]/20 transition-transform hover:scale-[1.03]">
          创建新密钥
        </button>
      </div>

      <div className="mb-8 grid grid-cols-1 gap-4 md:grid-cols-3">
        <ApiMetric label="启用密钥" value={`${activeCount} / ${MOCK_API_KEYS.length}`} />
        <ApiMetric label="本月调用" value="18,942" />
        <ApiMetric label="平均延迟" value="1.8s" />
      </div>

      <div className="mb-8 overflow-hidden rounded-[2.5rem] border border-[var(--border)] bg-[var(--surface)]">
        <div className="flex flex-col gap-3 border-b border-[var(--border)] p-6 md:flex-row md:items-center md:justify-between">
          <div>
            <h2 className="text-2xl font-black">密钥列表</h2>
            <p className="mt-1 text-sm text-[var(--muted)]">Secret 创建或重置后仍只展示掩码预览。</p>
          </div>
          <div className="relative w-full md:w-72">
            <input className="h-11 w-full rounded-xl border border-[var(--border)] bg-[var(--bg)]/60 pl-10 pr-4 text-sm text-[var(--fg)] placeholder:text-[var(--muted)]" placeholder="搜索密钥名称..." />
            <SearchIcon className="absolute left-3 top-3 size-5 text-[var(--muted)]" />
          </div>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full min-w-[980px] border-collapse text-sm">
            <thead>
              <tr className="text-left text-[10px] font-vault-mono uppercase tracking-widest text-[var(--muted)]">
                {['名称', 'Access Key', 'Secret Key', '状态', 'RPM', '额度', '最近调用', '过期时间', '操作'].map(head => (
                  <th key={head} className="border-b border-[var(--border)] px-5 py-4 font-bold">{head}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {MOCK_API_KEYS.map(key => (
                <tr key={key.id} className="group border-b border-[var(--border)] last:border-b-0 hover:bg-[var(--accent)]/5">
                  <td className="px-5 py-5">
                    <strong className="block text-[var(--fg)]">{key.name}</strong>
                    <span className="mt-1 block text-xs text-[var(--muted)]">{key.scopes.join(' · ')}</span>
                  </td>
                  <td className="px-5 py-5"><MaskedCode value={maskToken(key.accessKey)} /></td>
                  <td className="px-5 py-5"><MaskedCode value={maskToken(key.secretKey)} /></td>
                  <td className="px-5 py-5"><ApiStatusBadge status={key.status} /></td>
                  <td className="px-5 py-5 font-vault-mono text-[var(--fg)]">{key.rpm}</td>
                  <td className="px-5 py-5 font-vault-mono text-[var(--muted)]">{key.quota}</td>
                  <td className="px-5 py-5 text-[var(--muted)]">{key.lastUsedAt}</td>
                  <td className="px-5 py-5 text-[var(--muted)]">{key.expiresAt}</td>
                  <td className="px-5 py-5">
                    <div className="flex flex-wrap gap-2">
                      <PrototypeActionButton>复制 AK</PrototypeActionButton>
                      <PrototypeActionButton>{key.status === 'active' ? '禁用' : '启用'}</PrototypeActionButton>
                      <PrototypeActionButton>重置</PrototypeActionButton>
                      <PrototypeActionButton tone="danger">删除</PrototypeActionButton>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-[1.1fr_0.9fr]">
        <div className="overflow-hidden rounded-[2rem] border border-[var(--border)] bg-[var(--surface)]">
          <div className="flex items-center justify-between border-b border-[var(--border)] px-6 py-4">
            <span className="text-sm font-bold">快速接入</span>
            <PrototypeActionButton>复制示例</PrototypeActionButton>
          </div>
          <pre className="overflow-x-auto p-6 text-xs leading-relaxed text-[var(--muted)]"><code>{`curl https://api.example.com/v1/images/generations \\
  -H "Authorization: Bearer ${maskToken(MOCK_API_KEYS[0].secretKey)}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "plus",
    "prompt": "A cinematic product render in a glass studio",
    "n": 1,
    "size": "1024x1024"
  }'`}</code></pre>
        </div>
        <div className="rounded-[2rem] border border-[var(--border)] bg-[var(--surface)] p-6">
          <h3 className="mb-4 text-xl font-black">安全建议</h3>
          <div className="grid gap-3 text-sm text-[var(--muted)]">
            <SecurityHint title="按环境拆分" detail="生产、测试和自动化脚本使用不同密钥，便于快速吊销。" />
            <SecurityHint title="设置额度" detail="为高频调用方配置日额度和总额度，避免异常消耗积分。" />
            <SecurityHint title="定期轮换" detail="重置 Secret 后旧密钥立即失效，客户端需同步更新。" />
          </div>
        </div>
      </div>
    </div>
  )
}

function ProfileView() {
  return (
    <PlaceholderView
      title="个人中心"
      description="这里预留用户资料、头像、邮箱验证和账号安全信息，后续可直接承接正式用户中心。"
    />
  )
}

function SettingsView({
  activeTheme,
  onThemeChange,
  themeMode,
  onThemeModeChange,
}: {
  activeTheme: AccentTheme
  onThemeChange: (theme: AccentTheme) => void
  themeMode: ThemeMode
  onThemeModeChange: (mode: ThemeMode) => void
}) {
  return (
    <div className="p-6 md:p-10 flex-1 w-full">
      <div className="mb-10">
        <h1 className="text-4xl md:text-6xl font-black mb-4">设置</h1>
        <p className="max-w-2xl text-base text-[var(--muted)] leading-relaxed">调整站点外观偏好，主题色与光暗模式会实时应用到当前原型。</p>
      </div>

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-[0.9fr_1.1fr]">
        <section className="rounded-[2.5rem] border border-[var(--border)] bg-[var(--surface)] p-6 md:p-8">
          <div className="mb-6 flex items-start justify-between gap-4">
            <div>
              <h2 className="text-2xl font-black">光暗主题</h2>
              <p className="mt-2 text-sm text-[var(--muted)]">右上角也可快速切换。</p>
            </div>
            <div className="grid size-12 place-items-center rounded-2xl border border-[var(--border)] bg-[var(--bg)] text-[var(--accent)]">
              {themeMode === 'dark' ? <MoonIcon /> : <SunIcon />}
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            {(['dark', 'light'] as ThemeMode[]).map(mode => (
              <button
                key={mode}
                type="button"
                onClick={() => onThemeModeChange(mode)}
                className={cn(
                  "flex h-24 flex-col items-center justify-center gap-2 rounded-2xl border bg-[var(--bg)]/60 text-sm font-bold transition-all",
                  themeMode === mode ? "border-[var(--accent)] text-[var(--accent)] ring-1 ring-[var(--accent)]" : "border-[var(--border)] text-[var(--muted)] hover:border-[var(--accent)]/60 hover:text-[var(--fg)]"
                )}
              >
                {mode === 'dark' ? <MoonIcon /> : <SunIcon />}
                {mode === 'dark' ? '深色' : '浅色'}
              </button>
            ))}
          </div>
        </section>

        <section className="rounded-[2.5rem] border border-[var(--border)] bg-[var(--surface)] p-6 md:p-8">
          <div className="mb-6">
            <h2 className="text-2xl font-black">站点主题色</h2>
            <p className="mt-2 text-sm text-[var(--muted)]">原顶部色彩选择器已移动到这里，避免占用主工作区。</p>
          </div>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            {ACCENT_THEMES.map(theme => (
              <button
                key={theme.name}
                type="button"
                onClick={() => onThemeChange(theme)}
                className={cn(
                  "group rounded-2xl border bg-[var(--bg)]/60 p-4 text-left transition-all hover:-translate-y-1",
                  activeTheme.name === theme.name ? "border-[var(--accent)] ring-1 ring-[var(--accent)] shadow-lg shadow-[var(--accent)]/15" : "border-[var(--border)] hover:border-[var(--accent)]/60"
                )}
              >
                <div className="mb-4 h-24 rounded-xl border border-white/10" style={{ background: `linear-gradient(135deg, ${theme.accent}, ${theme.accentPurple})` }} />
                <div className="flex items-center justify-between gap-3">
                  <span className="font-bold text-[var(--fg)]">{theme.name}</span>
                  <span className={cn("grid size-6 place-items-center rounded-lg border", activeTheme.name === theme.name ? "border-[var(--accent)] bg-[var(--accent)] text-white" : "border-[var(--border)] text-transparent")}>
                    <CheckIcon />
                  </span>
                </div>
              </button>
            ))}
          </div>
        </section>
      </div>
    </div>
  )
}

function PlaceholderView({ title, description, empty = false }: { title: string; description: string; empty?: boolean }) {
  return (
    <div className="p-6 md:p-10 flex-1 w-full">
      <div className="mb-10">
        <h1 className="text-4xl md:text-6xl font-black mb-4">{title}</h1>
        {!empty && <p className="max-w-2xl text-base text-[var(--muted)] leading-relaxed">{description}</p>}
      </div>
      <div className="grid min-h-[420px] place-items-center rounded-[2.5rem] border border-dashed border-[var(--border)] bg-[var(--surface)]/50">
        <div className="max-w-sm text-center">
          <div className="mx-auto mb-5 grid size-14 place-items-center rounded-2xl border border-[var(--border)] bg-[var(--bg)] text-[var(--accent)]">
            <SettingsIcon />
          </div>
          <p className="text-sm text-[var(--muted)]">{description}</p>
        </div>
      </div>
    </div>
  )
}

function GalleryImportModal({ selectedLimit, onClose, onConfirm }: { selectedLimit: number; onClose: () => void; onConfirm: (assets: GalleryAsset[]) => void }) {
  const [query, setQuery] = useState('')
  const [group, setGroup] = useState('全部分组')
  const [status, setStatus] = useState('公开状态')
  const [model, setModel] = useState('所有模型')
  const [ratio, setRatio] = useState('所有比例')
  const [selectedIds, setSelectedIds] = useState<string[]>([])
  const filteredAssets = MOCK_GALLERY_ASSETS.filter(asset => {
    const keyword = query.trim().toLowerCase()
    const matchesKeyword = !keyword || `${asset.title} ${asset.group} ${asset.model}`.toLowerCase().includes(keyword)
    const matchesGroup = group === '全部分组' || asset.group === group
    const matchesStatus = status === '公开状态' || asset.status === status
    const matchesModel = model === '所有模型' || asset.model === model
    const matchesRatio = ratio === '所有比例' || asset.ratio === ratio
    return matchesKeyword && matchesGroup && matchesStatus && matchesModel && matchesRatio
  })
  const selectedAssets = MOCK_GALLERY_ASSETS.filter(asset => selectedIds.includes(asset.id))
  const canSelectMore = selectedIds.length < selectedLimit

  const toggleAsset = (asset: GalleryAsset) => {
    setSelectedIds(current => {
      if (current.includes(asset.id)) return current.filter(id => id !== asset.id)
      if (current.length >= selectedLimit) return current
      return [...current, asset.id]
    })
  }

  const confirm = () => {
    if (selectedAssets.length === 0) return
    onConfirm(selectedAssets)
    onClose()
  }

  return (
    <div className="fixed inset-0 z-[110] flex items-center justify-center bg-black/75 p-4 backdrop-blur-xl">
      <div className="flex max-h-[90vh] w-full max-w-6xl flex-col overflow-hidden rounded-[2.5rem] border border-[var(--border)] bg-[var(--bg)] shadow-2xl">
        <div className="flex flex-col gap-5 border-b border-[var(--border)] p-5 md:p-6">
          <div className="flex items-start justify-between gap-4">
            <div>
              <span className="text-[10px] font-vault-mono uppercase tracking-[0.2em] text-[var(--accent)]">Import From Gallery</span>
              <h2 className="mt-2 text-2xl font-black md:text-3xl">从图库导入参考图</h2>
              <p className="mt-1 text-sm text-[var(--muted)]">已默认开启批量选择，本次还可选择 {selectedLimit} 张。</p>
            </div>
            <button type="button" onClick={onClose} className="grid size-9 shrink-0 place-items-center rounded-full border border-[var(--border)] bg-[var(--surface)] text-[var(--muted)] transition-colors hover:text-[var(--fg)]">✕</button>
          </div>

          <div className="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
            <div className="relative w-full xl:max-w-sm">
              <input
                value={query}
                onChange={event => setQuery(event.target.value)}
                className="h-11 w-full rounded-xl border border-[var(--border)] bg-[var(--surface)] pl-10 pr-4 text-sm text-[var(--fg)] placeholder:text-[var(--muted)]"
                placeholder="搜索图片标题、分组或模型..."
              />
              <SearchIcon className="absolute left-3 top-3 size-5 text-[var(--muted)]" />
            </div>
            <div className={rdGallery.filterGroup}>
              <CustomSelect options={['全部分组', '默认画集', '产品设计', '人像摄影']} value={group} onChange={setGroup} />
              <CustomSelect options={['公开状态', '仅自己可见', '已公开']} value={status} onChange={setStatus} />
              <CustomSelect options={['所有模型', 'Flux Pro', 'Midjourney v6']} value={model} onChange={setModel} />
              <CustomSelect options={['所有比例', '16:9 横屏', '9:16 竖屏', '1:1 方形']} value={ratio} onChange={setRatio} />
            </div>
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto p-5 md:p-6">
          {selectedLimit === 0 && (
            <div className="mb-5 rounded-2xl border border-[var(--accent)]/25 bg-[var(--accent)]/10 px-4 py-3 text-sm text-[var(--accent)]">
              参考图数量已达上限，请先移除已有图片后再导入。
            </div>
          )}
          <div className="grid grid-cols-2 gap-4 md:grid-cols-3 xl:grid-cols-4">
            {filteredAssets.map(asset => {
              const selected = selectedIds.includes(asset.id)
              const disabled = !selected && !canSelectMore
              return (
                <button
                  key={asset.id}
                  type="button"
                  disabled={disabled}
                  onClick={() => toggleAsset(asset)}
                  className={cn(
                    "group relative overflow-hidden rounded-2xl border bg-[var(--surface)] text-left transition-all",
                    selected ? "border-[var(--accent)] ring-1 ring-[var(--accent)] shadow-lg shadow-[var(--accent)]/15" : "border-[var(--border)] hover:border-[var(--accent)]/60",
                    disabled && "cursor-not-allowed opacity-45"
                  )}
                >
                  <div className="relative aspect-[4/3] overflow-hidden bg-[var(--border)]">
                    <img src={asset.src} alt={asset.title} className="size-full object-cover transition-transform duration-500 group-hover:scale-105" />
                    <div className="absolute left-3 top-3 grid size-6 place-items-center rounded-lg border border-[var(--image-checkbox-border)] bg-[var(--image-checkbox-bg)] text-[var(--image-card-text)] backdrop-blur">
                      {selected && <CheckIcon />}
                    </div>
                  </div>
                  <div className="p-4">
                    <div className="truncate text-sm font-bold text-[var(--fg)]">{asset.title}</div>
                    <div className="mt-1 flex items-center justify-between gap-3 text-[10px] font-vault-mono uppercase text-[var(--muted)]">
                      <span>{asset.model}</span>
                      <span>{asset.ratio}</span>
                    </div>
                  </div>
                </button>
              )
            })}
          </div>
          {filteredAssets.length === 0 && (
            <div className="grid min-h-[240px] place-items-center rounded-2xl border border-dashed border-[var(--border)] text-sm text-[var(--muted)]">
              没有匹配的图库图片
            </div>
          )}
        </div>

        <div className="flex flex-col gap-3 border-t border-[var(--border)] bg-[var(--surface)]/80 p-5 md:flex-row md:items-center md:justify-between">
          <div className="text-sm text-[var(--muted)]">已选择 <span className="font-vault-mono font-bold text-[var(--accent)]">{selectedIds.length}</span> 张</div>
          <div className="flex gap-3">
            <button type="button" onClick={onClose} className="h-11 rounded-xl border border-[var(--border)] px-5 text-sm font-bold text-[var(--muted)] transition-colors hover:text-[var(--fg)]">取消</button>
            <button type="button" disabled={selectedIds.length === 0} onClick={confirm} className="h-11 rounded-xl bg-[var(--accent)] px-6 text-sm font-bold text-white transition-all hover:scale-[1.03] disabled:cursor-not-allowed disabled:opacity-50">确定导入</button>
          </div>
        </div>
      </div>
    </div>
  )
}

function NavItem({ icon, label, active, onClick }: any) {
  return (
    <button type="button" onClick={onClick} className={cn(rdShell.navLink, active && rdShell.navLinkActive)}>
      <div className={cn(rdShell.navLinkIndicator, active && rdShell.navLinkIndicatorActive)} />
      {icon}
      <span className={rdShell.navLabel}>{label}</span>
    </button>
  )
}

function AvatarMenuItem({ icon, label, tone = 'default', onClick }: { icon: React.ReactNode; label: string; tone?: 'default' | 'danger'; onClick: () => void }) {
  return (
    <button
      type="button"
      role="menuitem"
      onClick={onClick}
      className={cn(
        "flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm font-bold transition-colors",
        tone === 'danger' ? "text-red-400 hover:bg-red-500/10" : "text-[var(--fg)] hover:bg-[var(--accent)]/10 hover:text-[var(--accent)]"
      )}
    >
      <span className="grid size-5 place-items-center">{icon}</span>
      {label}
    </button>
  )
}

function ApiMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-[2rem] border border-[var(--border)] bg-[var(--surface)] p-6">
      <span className="text-[10px] font-vault-mono uppercase tracking-widest text-[var(--muted)]">{label}</span>
      <div className="mt-3 text-3xl font-black text-[var(--fg)]">{value}</div>
    </div>
  )
}

function MaskedCode({ value }: { value: string }) {
  return <code className="rounded-xl border border-[var(--border)] bg-[var(--bg)] px-3 py-2 font-vault-mono text-xs font-bold text-[var(--accent)]">{value}</code>
}

function ApiStatusBadge({ status }: { status: ApiKeyMock['status'] }) {
  const copy = {
    active: '启用中',
    disabled: '已禁用',
    expired: '已过期',
  }[status]
  return (
    <span className={cn(
      "inline-flex rounded-full border px-3 py-1 text-xs font-bold",
      status === 'active' && "border-[var(--accent)]/35 bg-[var(--accent)]/10 text-[var(--accent)]",
      status === 'disabled' && "border-[var(--border)] bg-white/5 text-[var(--muted)]",
      status === 'expired' && "border-amber-400/30 bg-amber-400/10 text-amber-300"
    )}>
      {copy}
    </span>
  )
}

function PrototypeActionButton({ children, tone = 'default' }: { children: React.ReactNode; tone?: 'default' | 'danger' }) {
  return (
    <button type="button" className={cn(
      "rounded-lg border border-[var(--border)] bg-[var(--bg)]/60 px-3 py-1.5 text-xs font-bold transition-colors hover:border-[var(--accent)] hover:text-[var(--accent)]",
      tone === 'danger' && "hover:border-red-400/50 hover:bg-red-500/10 hover:text-red-400"
    )}>
      {children}
    </button>
  )
}

function SecurityHint({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="rounded-2xl border border-[var(--border)] bg-[var(--bg)]/50 p-4">
      <strong className="mb-1 block text-[var(--fg)]">{title}</strong>
      <span>{detail}</span>
    </div>
  )
}

function GenerationResultCard({ item, variant, onOpen }: { item: GenerationResultItem; variant: 'single' | 'grid'; onOpen: (src: string) => void }) {
  const successClass = variant === 'single' ? rdWorkspace.outputGridSingle : rdWorkspace.outputGridImage
  const shellClass = variant === 'single'
    ? "relative max-h-[65vh] min-h-64 w-full max-w-4xl cursor-pointer overflow-hidden rounded-xl"
    : "relative group/item min-h-56 w-full h-full overflow-hidden rounded-xl"

  if (item.status === 'failed') {
    return (
      <div className={cn(
        shellClass,
        "grid place-items-center border border-red-500/25 bg-red-500/10 p-6 text-center"
      )}>
        <div className="flex max-w-sm flex-col items-center">
          <div className="mb-4 grid size-12 place-items-center rounded-2xl border border-red-500/25 bg-red-500/10 text-red-400">
            <AlertIcon />
          </div>
          <h4 className="mb-2 text-sm font-black text-red-300">该图片生成失败</h4>
          <p className="text-xs leading-relaxed text-[var(--muted)]">{item.error ?? '上游模型未返回可用图片。'}</p>
          <span className="mt-4 rounded-full border border-red-500/20 bg-red-500/10 px-3 py-1 text-[10px] font-vault-mono uppercase tracking-widest text-red-300">未扣费</span>
        </div>
      </div>
    )
  }

  if (!item.src) return null

  return (
    <div className={shellClass} onClick={() => onOpen(item.src!)}>
      <ImageWithSkeleton src={item.src} alt="Generated Output" className={successClass} />
      {variant === 'grid' && (
        <div className="absolute top-2 right-2 bg-[var(--image-action-bg)] text-[var(--image-action-text)] backdrop-blur rounded p-1.5 opacity-0 group-hover/item:opacity-100 transition-opacity">
          <DownloadIconSmall />
        </div>
      )}
    </div>
  )
}

function StatCard({ label, value }: any) {
  return (
    <div className={rdHome.statCard}>
      <div className={rdHome.statGlow} />
      <span className={rdHome.statLabel}>{label}</span>
      <div className={rdHome.statValue}>{value}</div>
    </div>
  )
}

function GalleryItem({ index, isBatchMode = false, context = 'gallery' }: any) {
  const { openLightbox } = useContext(LightboxContext)
  const [checked, setChecked] = useState(false)
  const imageHeight = [600, 800, 1000, 1200][index%4]
  const imgUrl = `https://picsum.photos/seed/${index+42}/800/${imageHeight}`
  const openGalleryLightbox = () => {
    openLightbox({
      src: imgUrl,
      prompt: GALLERY_PROMPT,
      width: 800,
      height: imageHeight,
      ratio: deriveAspectRatio({ width: 800, height: imageHeight, src: imgUrl }),
      model: 'Flux Pro',
    })
  }
  
  return (
    <div className={rdGallery.item}>
      <div className={rdGallery.itemShell} onClick={() => {
        if (isBatchMode) setChecked(!checked);
        else openGalleryLightbox();
      }}>
        <div className={rdGallery.itemInner} style={{ paddingBottom: `${(imageHeight / 800) * 100}%` }}>
          <ImageWithSkeleton src={imgUrl} className="absolute inset-0 w-full h-full object-cover transition-transform duration-700 group-hover:scale-110" alt="Gallery" />
          <div className={cn(rdGallery.itemOverlay, checked && isBatchMode && "opacity-100 [background:var(--image-overlay-selected)]")}>
             <div className={rdGallery.itemHeader}>
                {isBatchMode ? (
                  <div className={cn(rdGallery.itemCheckbox, checked && rdGallery.itemCheckboxChecked)}>
                    {checked && <CheckIcon />}
                  </div>
                ) : (
                  <span className={rdGallery.itemBadge}>PUBLIC</span>
                )}
                
                {!isBatchMode && context === 'gallery' && (
                  <div className={cn(rdGallery.itemActionGroup, "opacity-0 group-hover:opacity-100 transition-opacity")} onClick={e => e.stopPropagation()}>
                     <button className={rdGallery.itemActionBtn} title="再次编辑"><EditIcon /></button>
                     <button className={rdGallery.itemActionBtn} title="下载" onClick={openGalleryLightbox}><DownloadIconSmall /></button>
                     <button className={rdGallery.itemActionBtn} title="设为公开"><GlobeIcon /></button>
                     <button className={rdGallery.itemActionBtn} title="分组"><FolderIcon /></button>
                     <button className={cn(rdGallery.itemActionBtn, "hover:bg-red-500/20 hover:text-red-400")} title="删除"><TrashIcon /></button>
                  </div>
                )}
                {!isBatchMode && context === 'home' && (
                  <div className={cn(rdGallery.itemActionGroup, "opacity-0 group-hover:opacity-100 transition-opacity")} onClick={e => e.stopPropagation()}>
                     <button className={rdGallery.itemActionBtn} title="复制提示词"><CopyIcon /></button>
                     <button className={cn(rdGallery.itemActionBtn, "hover:text-red-400")} title="喜爱/收藏"><HeartIcon /></button>
                  </div>
                )}
             </div>

             <div className={rdGallery.itemFooter}>
               <span className={rdGallery.itemTitle}>Visual Artifact #{index} with highly detailed cinematic lighting</span>
               <div className={rdGallery.itemMeta}>
                 <span>Flux Pro • 16:9</span>
                 <span>2026/06/09</span>
               </div>
             </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function ImageWithSkeleton({ src, alt, className }: { src: string; alt: string; className: string }) {
  const [loaded, setLoaded] = useState(false);
  return (
    <>
      {!loaded && (
        <div className={cn(className, "absolute inset-0 animate-pulse bg-[var(--border)] flex items-center justify-center")}>
          <ImageIcon className="size-8 text-[var(--muted)] opacity-20" />
        </div>
      )}
      <img 
        src={src} 
        alt={alt} 
        className={cn(className, !loaded && "opacity-0")} 
        onLoad={() => setLoaded(true)} 
      />
    </>
  )
}

function ModelSelect({ active, name, points, onClick }: any) {
  return (
    <button className={cn(rdWorkspace.modelItem, active && rdWorkspace.modelItemActive)} onClick={onClick}>
      <div className={rdWorkspace.modelInfo}>
        <span className={rdWorkspace.itemLabel}>{name}</span>
      </div>
      <span className={rdWorkspace.modelPoints}>{points} ◈</span>
    </button>
  )
}

function SelectItem({ active, label, children, onClick }: any) {
  return (
    <button className={cn(rdWorkspace.selectItem, active && rdWorkspace.selectItemActive)} onClick={onClick}>
      {children && <div className={cn(rdWorkspace.itemIcon, active && rdWorkspace.itemIconActive)}>{children}</div>}
      <span className={rdWorkspace.itemLabel}>{label}</span>
    </button>
  )
}

function AspectRatioIcon({ ratio, active }: { ratio: string; active?: boolean }) {
  const common = "stroke-current fill-none";
  const color = active ? "text-[var(--accent)]" : "text-[var(--muted)]";
  
  if (ratio === '1:1') return <svg className={color} width="16" height="16" viewBox="0 0 24 24"><rect x="4" y="4" width="16" height="16" rx="2" className={common} strokeWidth="2" /></svg>;
  if (ratio === '4:3') return <svg className={color} width="20" height="15" viewBox="0 0 24 18"><rect x="2" y="2" width="20" height="14" rx="2" className={common} strokeWidth="2.5" /></svg>;
  if (ratio === '16:9') return <svg className={color} width="22" height="12" viewBox="0 0 24 13.5"><rect x="1" y="1" width="22" height="11.5" rx="2" className={common} strokeWidth="3" /></svg>;
  if (ratio === '9:16') return <svg className={color} width="12" height="22" viewBox="0 0 13.5 24"><rect x="1" y="1" width="11.5" height="22" rx="2" className={common} strokeWidth="3" /></svg>;
  if (ratio === '3:4') return <svg className={color} width="15" height="20" viewBox="0 0 18 24"><rect x="2" y="2" width="14" height="20" rx="2" className={common} strokeWidth="2.5" /></svg>;
  if (ratio === '2:3') return <svg className={color} width="14" height="21" viewBox="0 0 16 24"><rect x="1" y="1" width="14" height="22" rx="2" className={common} strokeWidth="3" /></svg>;
  return null;
}

function LightboxInfo({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-[var(--border)] bg-[var(--bg)]/70 p-3">
      <span className="block text-[10px] font-vault-mono uppercase tracking-widest text-[var(--muted)] mb-1">{label}</span>
      <span className="text-sm font-black text-[var(--fg)]">{value}</span>
    </div>
  )
}

function formatImagePixels(image: LightboxPayload) {
  if (!image.width || !image.height) return '未知'
  return `${image.width} x ${image.height}`
}

function deriveAspectRatio(image: Pick<LightboxPayload, 'width' | 'height' | 'src'>) {
  if (!image.width || !image.height) return '未知'
  const divisor = gcd(image.width, image.height)
  return `${image.width / divisor}:${image.height / divisor}`
}

function gcd(a: number, b: number): number {
  return b === 0 ? a : gcd(b, a % b)
}

function clampAmount(amount: number) {
  return Math.min(10000, Math.max(1, Number(amount.toFixed(2))))
}

function formatPoints(points: number) {
  return points.toLocaleString('zh-CN', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })
}

function PlanItem({ active, name, points, price, bonus, onClick, children }: any) {
  return (
    <div
      role="button"
      tabIndex={0}
      className={cn(rdBilling.planItem, active && rdBilling.planActive, "cursor-pointer")}
      onClick={onClick}
      onKeyDown={event => {
        if (event.target instanceof HTMLInputElement) return
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          onClick()
        }
      }}
    >
      <div className="flex flex-col items-start gap-2 w-full">
        <div className="flex justify-between items-center w-full">
          <span className="text-sm font-bold text-[var(--fg)]">{name}</span>
          <span className="text-lg font-medium text-[var(--muted)]">{price}</span>
        </div>
        <div className="flex items-end gap-1.5 mt-2">
          <span className="text-3xl font-black text-[var(--accent)]">{points}</span>
          <span className="text-xs text-[var(--muted)] mb-1">积分</span>
        </div>
        {bonus && (
          <div className="mt-1 inline-flex items-center gap-1.5 rounded-lg bg-[var(--accent)]/10 px-2 py-1 text-[11px] font-bold text-[var(--accent)] border border-[var(--accent)]/20">
            <SparklesIcon className="size-3" /> 赠送 {bonus}
          </div>
        )}
        {children}
      </div>
    </div>
  )
}

function maskToken(value: string) {
  if (value.length <= 14) return value
  return `${value.slice(0, 12)}${'•'.repeat(10)}${value.slice(-4)}`
}

function CustomSelect({ options, value, onChange }: { options: string[]; value?: string; onChange?: (value: string) => void }) {
  const [open, setOpen] = useState(false)
  const [internalSelected, setInternalSelected] = useState(options[0])
  const closeTimer = React.useRef<number | null>(null)
  const selected = value ?? internalSelected

  const keepOpen = () => {
    if (closeTimer.current !== null) window.clearTimeout(closeTimer.current)
    closeTimer.current = null
    setOpen(true)
  }

  const scheduleClose = () => {
    closeTimer.current = window.setTimeout(() => setOpen(false), 160)
  }

  useEffect(() => {
    return () => {
      if (closeTimer.current !== null) window.clearTimeout(closeTimer.current)
    }
  }, [])

  const selectOption = (option: string) => {
    setInternalSelected(option)
    onChange?.(option)
    setOpen(false)
  }

  return (
    <div
      className={cn(rdGallery.filterSelectWrap, "pb-3 -mb-3")}
      onMouseEnter={keepOpen}
      onMouseLeave={scheduleClose}
      onFocus={() => setOpen(true)}
      onBlur={event => {
        if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false)
      }}
    >
      <button className={rdGallery.filterSelectBtn} onClick={() => setOpen(!open)}>
        {selected}
        <ChevronIcon className={cn("size-3 text-[var(--muted)] transition-transform", open && "rotate-180")} />
      </button>
      {open && (
        <div className={rdGallery.filterSelectDropdown}>
          {options.map(opt => (
            <div 
              key={opt} 
              className={cn(rdGallery.filterOption, selected === opt && rdGallery.filterOptionActive)}
              onClick={() => selectOption(opt)}
            >
              {opt}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// --- Icons ---
const HomeIcon = () => <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M3 9l9-7 9 7v11a2 2 0 01-2 2H5a2 2 0 01-2-2z" /></svg>
const SparklesIcon = ({ className }: any) => <svg className={className} width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M12 2v20M2 12h20M5 5l14 14M5 19L19 5" /></svg>
const SunIcon = () => <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" /></svg>
const MoonIcon = () => <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z" /></svg>
const GridIcon = () => <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M9 3v18M15 3v18M3 9h18M3 15h18" /></svg>
const KeyIcon = () => <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="7.5" cy="14.5" r="4.5" /><path d="M11 11l9-9M16 6l2 2M14 8l2 2" /></svg>
const SettingsIcon = () => <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.7 1.7 0 00.34 1.87l.06.06a2 2 0 01-2.83 2.83l-.06-.06A1.7 1.7 0 0015 19.4a1.7 1.7 0 00-1 1.55V21a2 2 0 01-4 0v-.09a1.7 1.7 0 00-1-1.55 1.7 1.7 0 00-1.87.34l-.06.06a2 2 0 01-2.83-2.83l.06-.06A1.7 1.7 0 004.6 15a1.7 1.7 0 00-1.55-1H3a2 2 0 010-4h.09a1.7 1.7 0 001.55-1 1.7 1.7 0 00-.34-1.87l-.06-.06a2 2 0 012.83-2.83l.06.06A1.7 1.7 0 009 4.6a1.7 1.7 0 001-1.55V3a2 2 0 014 0v.09a1.7 1.7 0 001 1.55 1.7 1.7 0 001.87-.34l.06-.06a2 2 0 012.83 2.83l-.06.06A1.7 1.7 0 0019.4 9a1.7 1.7 0 001.55 1H21a2 2 0 010 4h-.09a1.7 1.7 0 00-1.55 1z" /></svg>
const UserIcon = () => <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M20 21a8 8 0 10-16 0" /><circle cx="12" cy="7" r="4" /></svg>
const DocsIcon = () => <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" /><path d="M14 2v6h6M8 13h8M8 17h6" /></svg>
const LogoutIcon = () => <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M9 21H5a2 2 0 01-2-2V5a2 2 0 012-2h4" /><path d="M16 17l5-5-5-5M21 12H9" /></svg>
const CreditCardIcon = () => <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="1" y="4" width="22" height="16" rx="2" /><path d="M1 10h22" /></svg>
const ChevronIcon = ({ className }: any) => <svg className={className} width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M6 9l6 6 6-6" /></svg>
const SearchIcon = ({ className }: any) => <svg className={className} width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="11" cy="11" r="8" /><path d="M21 21l-4.35-4.35" /></svg>
const ArrowRightIcon = () => <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3"><path d="M5 12h14M12 5l7 7-7 7" /></svg>
const AliPayIcon = () => <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm4.3 14.3l-1.4 1.4L12 14.8l-2.9 2.9-1.4-1.4 2.9-2.9-2.9-2.9 1.4-1.4 2.9 2.9 2.9-2.9 1.4 1.4-2.9 2.9 2.9 2.9z"/></svg>
const WeChatIcon = () => <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1.5 15H11v-1.5h2.5V17zm2.5-4h-1.5v-1.5H16V13zm-5.5-1.5H9V10h1.5v1.5z"/></svg>
const ImageIcon = ({ className }: any) => <svg className={className} width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="3" y="3" width="18" height="18" rx="2" /><circle cx="8.5" cy="8.5" r="1.5" /><path d="M21 15l-5-5L5 21" /></svg>
const UploadIcon = ({ className }: any) => <svg className={className} width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4M17 8l-5-5-5 5M12 3v12" /></svg>
const DownloadIconSmall = () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4M7 10l5 5 5-5M12 15V3" /></svg>
const ExpandIconSmall = () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7" /></svg>
const CopyIcon = () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="9" y="9" width="13" height="13" rx="2" /><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1" /></svg>
const EditIcon = () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7" /><path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z" /></svg>
const ClockIcon = () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="10" /><path d="M12 6v6l4 2" /></svg>
const CheckIcon = () => <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3"><path d="M5 13l4 4L19 7" /></svg>
const FolderIcon = () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M22 19a2 2 0 01-2 2H4a2 2 0 01-2-2V5a2 2 0 012-2h5l2 3h9a2 2 0 012 2v11z" /></svg>
const GlobeIcon = () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="10" /><path d="M2 12h20M12 2a15.3 15.3 0 014 10 15.3 15.3 0 01-4 10 15.3 15.3 0 01-4-10 15.3 15.3 0 014-10z" /></svg>
const TrashIcon = () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M3 6h18M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2M10 11v6M14 11v6" /></svg>
const AlertIcon = () => <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="10" /><path d="M12 8v4M12 16h.01" /></svg>
const HeartIcon = () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M20.84 4.61a5.5 5.5 0 00-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 00-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 000-7.78z" /></svg>
const WandIcon = ({ className }: any) => <svg className={className} width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M2 12h4M18 12h4M12 2v4M12 18v4M4.9 4.9l2.8 2.8M16.3 16.3l2.8 2.8M4.9 19.1l2.8-2.8M16.3 7.7l2.8-2.8" /><circle cx="12" cy="12" r="3" /></svg>
