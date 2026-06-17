import { cn } from '../../../shared/classnames'
import heroImage from '../../../../docs/template/PicGallery/mpdhezm8-image.png'
import { BrandMark, siteBrand } from '../brand'
import { useApp } from '../components'
import { userButton } from '../ui/classes'
import { rdGallery, rdHome, rdShell } from '../ui/redesign-classes'

const landingClasses = {
  page: cn(rdShell.main, 'min-h-screen overflow-x-hidden'),
  nav: 'topnav sticky top-0 z-[100] border-b border-[var(--border)] bg-[var(--bg)]/70 backdrop-blur-2xl transition-all duration-500',
  container: 'container mx-auto w-full max-w-[1280px] px-6 md:px-10',
  topnavInner: 'topnav-inner flex items-center justify-between gap-4 py-5',
  logo: 'landing-logo border-0 bg-transparent text-[var(--accent)] transition-transform hover:scale-105',
  navLinks: 'hidden md:flex gap-10',
  navLink: 'text-[13px] font-bold tracking-wide text-[var(--muted)] no-underline transition hover:text-[var(--accent)]',
  section: 'section relative py-[clamp(72px,10vw,132px)]',
  hero: cn(rdHome.hero, 'mx-auto mt-8 min-h-[520px] max-w-[1280px] border border-[var(--border)] p-8 text-left md:p-14 max-[760px]:mt-4 max-[760px]:min-h-[520px] max-[760px]:rounded-[2rem]'),
  heroImage: rdHome.heroImg,
  heroOverlay: rdHome.heroOverlay,
  heroContent: cn(rdHome.heroContent, 'max-w-3xl'),
  h1: 'm-0 font-vault-display text-[clamp(3rem,7vw,6.4rem)] font-medium leading-[.9]',
  h2: 'h2 m-0 font-vault-display text-[clamp(2.5rem,6vw,5rem)] font-medium leading-[.95] tracking-[-0.01em]',
  lead: 'lead mx-auto mt-8 max-w-[720px] text-lg leading-[1.6] text-[var(--muted)] md:text-xl',
  heroCta: 'hero-cta mt-12 flex flex-wrap items-center justify-start gap-4 max-[420px]:flex-col max-[420px]:items-stretch',
  grid3: 'grid-3 grid grid-cols-1 gap-8 md:grid-cols-3',
  featureCard: rdHome.statCard,
  featureIcon: 'feature-icon mb-6 inline-flex size-14 items-center justify-center rounded-2xl bg-[var(--accent)]/10 text-[var(--accent)] shadow-[0_0_20px_rgba(var(--accent-rgb),0.1)]',
  featureTitle: 'm-0 mb-3 font-vault-display text-3xl font-bold',
  featureText: 'm-0 text-[15px] leading-relaxed text-[var(--muted)]',
  split: 'split grid items-center gap-16 lg:grid-cols-[1fr_1.1fr]',
  showcaseFrame: cn(rdGallery.itemShell, 'group relative aspect-[4/3] w-full cursor-default overflow-hidden border-0 p-1.5 transition-all duration-700 hover:translate-y-0 hover:shadow-[0_0_50px_rgba(var(--accent-rgb),0.15)]'),
  showcaseInner: rdGallery.itemInner,
  showcaseImage: cn(rdGallery.itemImg, 'opacity-80 grayscale-[0.3] transition-all duration-1000 group-hover:scale-105 group-hover:grayscale-0'),
  promptTag: 'prompt-tag absolute bottom-6 left-6 rounded-2xl border border-[var(--border)] bg-black/60 px-6 py-4 text-xs font-vault-mono text-[var(--fg)] backdrop-blur-xl',
  stat: rdHome.statCard,
  statNum: 'stat-num font-vault-display text-6xl font-black leading-none text-[var(--accent)]',
  statLabel: 'stat-label m-0 mt-3 text-sm font-bold uppercase tracking-widest text-[var(--muted)]',
  ctaHero: 'cta-hero relative overflow-hidden rounded-[3rem] border border-[var(--border)] bg-gradient-to-b from-[var(--surface)] to-[var(--bg)] px-10 py-24 text-center shadow-2xl',
  pagefoot: 'pagefoot border-t border-[var(--border)] bg-[var(--bg)]/50 py-16 text-sm text-[var(--muted)]',
  footerInner: 'flex flex-col items-center justify-between gap-10 md:flex-row',
  footerLinks: 'flex items-center gap-8',
  footerMeta: 'flex flex-col items-center gap-3 md:items-end',
}

const valueProps = [
  {
    title: '多模型路由',
    detail: '集成全球顶尖生图引擎。根据创作意图自动匹配最佳模型路径，确保每一张作品都拥有极致的视觉表达。',
    icon: (
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <path d="M12 2v20M2 12h20M5 5l14 14M5 19L19 5" />
      </svg>
    ),
  },
  {
    title: '统一计费体系',
    detail: '摆脱繁琐的模型账号维护。统一积分结算机制，精确到小数点后 5 位，让您的每一分投入都清晰可感。',
    icon: (
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <rect x="3" y="3" width="18" height="18" rx="2" />
        <path d="M3 12h18M12 3v18" />
      </svg>
    ),
  },
  {
    title: '原生 API 支持',
    detail: '不仅是创作台，更是强大的生图基座。提供 OpenAI 兼容接口，助您轻松将 AI 创作力集成至自有业务。',
    icon: (
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <path d="M12 3l1.912 5.886H20.09l-4.73 3.436 1.806 5.556-4.73-3.436-4.73 3.436 1.806-5.556-4.73-3.436h6.178L12 3z" />
      </svg>
    ),
  },
]

const stats = [
  { num: '3.2', label: '积分 / 元', detail: '透明计费，充值即得' },
  { num: '5', label: '位小数精度', detail: '不浪费任何一分额度' },
  { num: '99.9', suffix: '%', label: '服务可用性', detail: '多模型降级策略保障' },
]

export function LandingPage() {
  const app = useApp()
  return (
    <main className={landingClasses.page}>
      <header className={landingClasses.nav}>
        <div className={`${landingClasses.container} ${landingClasses.topnavInner}`}>
          <button className={landingClasses.logo} type="button" onClick={() => app.navigate('landing')} aria-label={`${siteBrand.name} 首页`}>
            <BrandMark withText />
          </button>
          <nav className={landingClasses.navLinks}>
            <a className={landingClasses.navLink} href="#features">功能特性</a>
            <a className={landingClasses.navLink} href="#showcase">作品展示</a>
            <a className={landingClasses.navLink} href="#pricing">计费规则</a>
          </nav>
          <div className="flex items-center gap-4">
            <button type="button" className={cn(userButton.base, 'hidden border-0 bg-transparent text-[13px] font-bold sm:inline-flex')} onClick={() => app.navigate('login')}>登录</button>
            <button type="button" className={cn(userButton.base, userButton.primary, 'min-h-[44px] px-6 text-[13px] shadow-[0_8px_20px_rgba(var(--accent-rgb),0.25)]')} onClick={() => app.navigate(app.isAuthenticated ? 'genpic' : 'login', { returnTo: 'genpic' })}>进入工作台</button>
          </div>
        </div>
      </header>

      <section className={landingClasses.hero}>
        <img src={heroImage} alt="Mikiko Studio hero" className={landingClasses.heroImage} />
        <div className={landingClasses.heroOverlay} />
        <div className={landingClasses.heroContent}>
          <div className={cn(rdHome.heroBadge, 'animate-in fade-in slide-in-from-bottom-4 duration-1000')}>Luminous Vault Experience</div>
          <h1 className={cn(landingClasses.h1, 'animate-in fade-in slide-in-from-bottom-6 duration-1000 fill-mode-both')}>文字跃然屏上<br />灵感触手可及</h1>
          <p className={cn(landingClasses.lead, 'mx-0 animate-in fade-in slide-in-from-bottom-8 duration-1000 fill-mode-both')}>Mikiko Studio 是为您量身定制的 AI 图片生成工作台。集成模型路由、透明计费、历史图库与 API 接入，让创作入口和主应用保持同一套体验。</p>
          <div className={cn(landingClasses.heroCta, 'animate-in fade-in slide-in-from-bottom-10 duration-1000 fill-mode-both')}>
            <button type="button" className={cn(userButton.base, userButton.primary, 'min-h-[56px] px-10 text-base shadow-[0_12px_30px_rgba(var(--accent-rgb),0.3)]')} onClick={() => app.navigate(app.isAuthenticated ? 'genpic' : 'login', { returnTo: 'genpic' })}>立即免费开始</button>
            <button type="button" className={cn(userButton.base, 'min-h-[56px] border-[var(--border)] bg-[var(--surface)]/50 px-10 text-base backdrop-blur-xl hover:bg-[var(--surface)]')} onClick={() => app.navigate('public-gallery')}>浏览画廊</button>
          </div>
        </div>
      </section>

      <section className={landingClasses.section} id="features">
        <div className={`${landingClasses.container} ${landingClasses.grid3}`}>
          {valueProps.map((prop, idx) => (
            <article className={cn(landingClasses.featureCard, 'animate-in fade-in slide-in-from-bottom-12 duration-1000 fill-mode-both')} style={{ animationDelay: `${idx * 150}ms` }} key={prop.title}>
              <div className={landingClasses.featureIcon}>{prop.icon}</div>
              <h2 className={landingClasses.featureTitle}>{prop.title}</h2>
              <p className={landingClasses.featureText}>{prop.detail}</p>
              <div className={rdHome.statGlow} />
            </article>
          ))}
        </div>
      </section>

      <section className={landingClasses.section} id="showcase">
        <div className={`${landingClasses.container} ${landingClasses.split}`}>
          <div className="animate-in fade-in slide-in-from-left-8 duration-1000">
            <div className={rdHome.heroBadge}>视觉资产生成</div>
            <h2 className={landingClasses.h2}>从文生图到参考生图，<br />掌控每一处细节。</h2>
            <p className={landingClasses.lead}>无论是电商产品图、社交媒体封面，还是极具创意的艺术作品，Mikiko Studio 都能为您提供精准的参数控制与卓越的输出质量。</p>
            <button type="button" className={cn(userButton.base, 'mt-10 min-h-[52px] border-[var(--border)] bg-[var(--surface)] px-8 text-sm font-bold shadow-xl hover:border-[var(--accent)]/50')} onClick={() => app.navigate(app.isAuthenticated ? 'genpic' : 'login', { returnTo: 'genpic' })}>探索创作工具</button>
          </div>
          <div className={cn(landingClasses.showcaseFrame, 'animate-in fade-in slide-in-from-right-8 duration-1000')}>
            <div className={landingClasses.showcaseInner}>
              <img src="/mpdhezm8-image.png" alt="Mikiko Studio showcase" className={landingClasses.showcaseImage} onError={(event) => { event.currentTarget.style.display = 'none' }} />
              <div className={landingClasses.promptTag}>
                <span className="mb-1 block text-[10px] uppercase tracking-widest text-[var(--accent)]">Prompt</span>
                A high-end watch in a cinematic luxury setting...
              </div>
            </div>
          </div>
        </div>
      </section>

      <section className={landingClasses.section} id="pricing">
        <div className={landingClasses.container}>
          <div className="mb-16 text-center">
            <div className={rdHome.heroBadge}>Transparent Billing</div>
            <h2 className={landingClasses.h2}>透明、精准、可控</h2>
          </div>
          <div className={landingClasses.grid3}>
            {stats.map((stat, idx) => (
              <div key={stat.label} className={cn(landingClasses.stat, 'text-center animate-in fade-in slide-in-from-bottom-8 duration-1000 fill-mode-both')} style={{ animationDelay: `${idx * 200}ms` }}>
                <div className={landingClasses.statNum}>{stat.num}{stat.suffix ? <span className="text-[0.4em] opacity-50">{stat.suffix}</span> : null}</div>
                <p className={landingClasses.statLabel}>{stat.label}</p>
                <p className="mt-2 text-sm text-[var(--muted)]">{stat.detail}</p>
                <div className={rdHome.statGlow} />
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className={landingClasses.section}>
        <div className={landingClasses.container}>
          <div className={landingClasses.ctaHero}>
            <div className="relative z-10">
              <h2 className={landingClasses.h2}>准备好开启您的 AI 创作之旅了吗？</h2>
              <p className={landingClasses.lead}>加入数千名创作者的行列，体验最纯粹的 AI 生图。<br className="hidden md:block" />无需繁琐配置，只需放飞灵感。</p>
              <button type="button" className={cn(userButton.base, userButton.primary, 'mt-12 min-h-[64px] px-12 text-lg font-black shadow-[0_20px_50px_rgba(var(--accent-rgb),0.4)]')} onClick={() => app.navigate(app.isAuthenticated ? 'genpic' : 'login', { returnTo: 'genpic' })}>立即注册，领取免费积分</button>
            </div>
            <div className="absolute inset-0 z-0 bg-[radial-gradient(circle_at_50%_50%,var(--accent)/0.1,transparent_70%)] opacity-50" />
          </div>
        </div>
      </section>

      <footer className={landingClasses.pagefoot}>
        <div className={`${landingClasses.container} ${landingClasses.footerInner}`}>
          <div className="flex flex-col items-center gap-6 md:items-start">
            <div className={`${landingClasses.logo} cursor-default`}>
              <BrandMark withText />
            </div>
            <p className="max-w-[300px] text-center text-xs leading-relaxed text-[var(--muted)] md:text-left">
              专业级 AI 图像生成工作台。集成全球顶尖模型，为创作者提供更具艺术感、更高效率的视觉资产生产力。
            </p>
          </div>
          <div className={landingClasses.footerMeta}>
            <div className={landingClasses.footerLinks}>
              <a href="#" className="transition hover:text-[var(--accent)]">隐私政策</a>
              <a href="#" className="transition hover:text-[var(--accent)]">服务条款</a>
              <a href="#" className="transition hover:text-[var(--accent)]">联系我们</a>
            </div>
            <p className="m-0 mt-4 opacity-50">© 2026 Mikiko Studio. All rights reserved.</p>
          </div>
        </div>
      </footer>
    </main>
  )
}
