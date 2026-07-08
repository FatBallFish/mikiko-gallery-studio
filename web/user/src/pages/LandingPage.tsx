import { cn } from '../../../shared/classnames'
import { BrandMark, siteBrand } from '../brand'
import { useApp } from '../components'
import { button as btn } from '../ui/redesign-classes'
import { rdCommon, rdGallery, rdHome, rdShell } from '../ui/redesign-classes'
import { Route as RouteIcon, ShieldCheck, Layers } from '../ui/icons'

const landingClasses = {
  page: cn(rdShell.main, 'min-h-screen overflow-x-hidden'),
  nav: 'sticky top-0 z-[100] border-b border-[var(--border)] bg-[var(--bg)]/70 backdrop-blur-2xl transition-all duration-500',
  container: 'container mx-auto w-full max-w-[1280px] px-6 md:px-10',
  topnavInner: 'flex items-center justify-between gap-4 py-4 max-[420px]:gap-2 max-[420px]:py-3',
  logo: 'shrink-0 border-0 bg-transparent text-[var(--accent)] transition-transform hover:scale-105 max-[420px]:scale-[0.92] max-[420px]:origin-left',
  navLinks: 'hidden md:flex gap-10',
  navLink: 'text-[13px] font-bold tracking-wide text-[var(--muted)] no-underline transition hover:text-[var(--accent)]',
  section: 'section relative py-[clamp(80px,12vw,160px)]',
  hero: 'min-h-[540px] overflow-hidden pb-10 pt-20 text-center md:min-h-[620px] max-[420px]:min-h-[460px] max-[420px]:pb-6 max-[420px]:pt-12',
  h1: 'm-0 font-vault-body text-[clamp(3.5rem,10vw,8rem)] font-black leading-[.92] tracking-[-0.04em] max-[420px]:text-[clamp(2.6rem,13vw,3.8rem)]',
  h2: 'm-0 font-vault-body text-[clamp(2.5rem,6vw,5rem)] font-black leading-[1] tracking-[-0.03em] max-[420px]:text-[clamp(2rem,9vw,3rem)]',
  lead: 'mx-auto mt-8 max-w-[720px] text-lg leading-[1.6] text-[var(--muted)] md:text-xl max-[420px]:mt-6 max-[420px]:text-base',
  heroCta: 'mt-12 flex flex-wrap items-center justify-center gap-6 max-[420px]:flex-col max-[420px]:items-stretch',
  // 2+1 asymmetric feature grid (replaces 3-equal AI tell)
  featureGrid: 'grid grid-cols-1 gap-8 md:grid-cols-2',
  featureCard: cn(rdHome.statCard, 'flex flex-col'),
  featureCardWide: cn(rdHome.statCard, 'flex flex-col md:col-span-2'),
  featureIcon: 'mb-6 inline-flex size-14 items-center justify-center rounded-2xl bg-[var(--accent)]/10 text-[var(--accent)]',
  featureTitle: 'm-0 mb-3 font-vault-display text-3xl font-bold',
  featureText: 'm-0 text-[15px] leading-relaxed text-[var(--muted)]',
  split: 'grid items-center gap-16 lg:grid-cols-[1fr_1.1fr]',
  showcaseFrame: cn(rdGallery.itemShell, 'group relative aspect-[4/3] w-full cursor-default overflow-hidden border-0 p-1.5 transition-all duration-700 hover:translate-y-0 hover:shadow-[0_0_50px_rgba(var(--accent-rgb),0.15)]'),
  showcaseInner: cn(rdGallery.itemInner, 'h-full min-h-full'),
  showcaseImage: cn(rdGallery.itemImg, 'size-full opacity-80 grayscale-[0.3] transition-all duration-1000 group-hover:scale-105 group-hover:grayscale-0'),
  promptTag: 'absolute bottom-6 left-6 rounded-2xl border border-[var(--border)] bg-black/60 px-6 py-4 text-xs font-vault-mono text-[var(--fg)] backdrop-blur-xl',
  stat: rdHome.statCard,
  statNum: 'font-vault-display text-6xl font-black leading-none text-[var(--accent)]',
  statLabel: 'm-0 mt-3 text-sm font-bold uppercase tracking-widest text-[var(--muted)]',
  ctaHero: 'relative overflow-hidden rounded-[2rem] border border-[var(--border)] bg-gradient-to-b from-[var(--surface)] to-[var(--bg)] px-10 py-24 text-center shadow-2xl',
  pagefoot: 'border-t border-[var(--border)] bg-[var(--bg)]/50 py-16 text-sm text-[var(--muted)]',
  footerInner: 'flex flex-col items-center justify-between gap-10 md:flex-row',
  footerLinks: 'flex items-center gap-8',
  footerMeta: 'flex flex-col items-center gap-3 md:items-end',
}

const valueProps = [
  {
    title: '多模型路由',
    detail: '集成全球顶尖生图引擎。根据创作意图自动匹配最佳模型路径，确保每一张作品都拥有极致的视觉表达。',
    icon: <RouteIcon size={24} strokeWidth={1.5} />,
    wide: false,
  },
  {
    title: '统一计费体系',
    detail: '摆脱繁琐的模型账号维护。统一积分结算机制，精确到小数点后 5 位，让您的每一分投入都清晰可感。',
    icon: <ShieldCheck size={24} strokeWidth={1.5} />,
    wide: false,
  },
  {
    title: '原生 API 支持',
    detail: '不仅是创作台，更是强大的生图基座。提供 OpenAI 兼容接口，助您轻松将 AI 创作力集成至自有业务。',
    icon: <Layers size={24} strokeWidth={1.5} />,
    wide: true,
  },
]

const stats = [
  { num: '3.2', label: '积分 / 元', detail: '透明计费，充值即得' },
  { num: '5', label: '位小数精度', detail: '不浪费任何一分额度' },
  { num: '99.9', suffix: '%', label: '服务可用性', detail: '多模型降级策略保障' },
]

export function LandingPage() {
  const app = useApp()

  const goGenpic = () => app.navigate(app.isAuthenticated ? 'genpic' : 'login', { returnTo: 'genpic' })

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
          <div className="flex items-center gap-3">
            <button type="button" className={cn(btn.base, btn.ghost, 'max-[420px]:hidden')} onClick={() => app.navigate('login')}>登录</button>
            <button type="button" className={cn(btn.base, btn.primary, 'min-h-[44px] whitespace-nowrap px-6 shadow-[0_8px_20px_-4px_rgba(var(--accent-rgb),0.3)] max-[420px]:min-h-12 max-[420px]:px-4')} onClick={goGenpic}>进入工作台</button>
          </div>
        </div>
      </header>

      <section className={`${landingClasses.section} ${landingClasses.hero}`}>
        <div className={landingClasses.container}>
          <div className={rdHome.heroBadge}>创作工作台</div>
          <h1 className={landingClasses.h1}>文字跃然屏上<br />灵感触手可及</h1>
          <p className={landingClasses.lead}>统一模型入口、参考生图和历史资产管理，帮助你更快产出可交付的视觉内容。</p>
          <div className={landingClasses.heroCta}>
            <button type="button" className={cn(btn.base, btn.primary, 'min-h-[56px] px-10 text-base shadow-[0_12px_30px_-8px_rgba(var(--accent-rgb),0.4)]')} onClick={goGenpic}>立即免费开始</button>
            <button type="button" className={cn(btn.base, 'min-h-[56px] backdrop-blur-xl')} onClick={() => app.navigate('public-gallery')}>浏览画廊</button>
          </div>
        </div>
      </section>

      <section className={landingClasses.section} id="features">
        <div className={landingClasses.container}>
          <div className={landingClasses.featureGrid}>
            {valueProps.map((prop, idx) => (
              <article
                key={prop.title}
                className={cn(prop.wide ? landingClasses.featureCardWide : landingClasses.featureCard, 'pg-enter')}
                style={{ animationDelay: `${idx * 80}ms` }}
              >
                <div className={landingClasses.featureIcon}>{prop.icon}</div>
                <h2 className={landingClasses.featureTitle}>{prop.title}</h2>
                <p className={landingClasses.featureText}>{prop.detail}</p>
                <div className={rdHome.statGlow} />
              </article>
            ))}
          </div>
        </div>
      </section>

      <section className={landingClasses.section} id="showcase">
        <div className={`${landingClasses.container} ${landingClasses.split}`}>
          <div className="pg-enter">
            <div className={rdHome.heroBadge}>视觉资产生成</div>
            <h2 className={landingClasses.h2}>从文生图到参考生图，<br />掌控每一处细节。</h2>
            <p className={landingClasses.lead}>无论是电商产品图、社交媒体封面，还是极具创意的艺术作品，Mikiko Studio 都能为您提供精准的参数控制与卓越的输出质量。</p>
            <button type="button" className={cn(btn.base, 'mt-10 min-h-[52px] px-8 shadow-xl')} onClick={goGenpic}>探索创作工具</button>
          </div>
          <div className={cn(landingClasses.showcaseFrame, 'pg-enter')} style={{ animationDelay: '120ms' }}>
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
          <div className="mb-16 text-center pg-enter">
            <div className={rdHome.heroBadge}>透明计费</div>
            <h2 className={landingClasses.h2}>透明、精准、可控</h2>
          </div>
          <div className="grid grid-cols-1 gap-8 md:grid-cols-3">
            {stats.map((stat, idx) => (
              <div key={stat.label} className={cn(landingClasses.stat, 'text-center pg-enter')} style={{ animationDelay: `${idx * 80}ms` }}>
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
          <div className={cn(landingClasses.ctaHero, 'pg-enter')}>
            <div className="relative z-10">
            <h2 className={landingClasses.h2}>准备开始下一张作品</h2>
            <p className={landingClasses.lead}>登录后即可进入工作台，继续生成、管理和导出你的图片资产。</p>
            <button type="button" className={cn(btn.base, btn.primary, 'mt-12 min-h-[64px] px-12 text-lg font-black shadow-[0_20px_50px_-10px_rgba(var(--accent-rgb),0.5)] max-[420px]:min-h-14 max-[420px]:px-6 max-[420px]:text-base')} onClick={goGenpic}>登录并进入工作台</button>
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
