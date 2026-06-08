import { cn } from '../../../shared/classnames'
import { useApp } from '../components'
import { userButton, userText } from '../ui/classes'

const landingClasses = {
  page: 'landing-page min-h-screen bg-[var(--bg)]',
  nav: 'topnav sticky top-0 z-[100] border-b border-[var(--border)] bg-[color-mix(in_oklch,var(--bg)_82%,transparent)] backdrop-blur-2xl',
  container: 'container mx-auto w-full max-w-[1180px] px-5',
  topnavInner: 'topnav-inner flex items-center justify-between gap-4 py-[18px]',
  logo: 'landing-logo border-0 bg-transparent font-vault-display text-2xl font-medium text-[var(--accent)]',
  navLinks: 'flex gap-7',
  navLink: 'text-sm text-[var(--fg)] no-underline transition hover:text-[var(--accent)]',
  section: 'section py-[clamp(64px,10vw,128px)]',
  hero: 'hero text-center',
  h1: 'm-0 font-vault-display text-[clamp(56px,10vw,128px)] font-medium leading-[.86]',
  h2: 'h2 m-0 font-vault-display text-[clamp(34px,6vw,72px)] font-medium leading-[.95]',
  lead: 'lead max-w-[760px] text-lg leading-[1.7] text-[var(--muted)]',
  heroCta: 'hero-cta mt-8 flex flex-wrap items-center justify-center gap-3 max-[420px]:flex-col max-[420px]:items-stretch',
  grid3: 'grid-3 grid grid-cols-1 gap-6 md:grid-cols-3',
  featureCard: 'feature-card rounded-3xl border border-[var(--border)] bg-[var(--surface)] p-7',
  featureIcon: 'feature-icon mb-4 size-12 text-[var(--accent)]',
  featureTitle: 'm-0 mb-2.5 font-vault-display text-2xl',
  featureText: 'm-0 text-sm leading-[1.6] text-[var(--muted)]',
  split: 'split grid items-center gap-14 lg:grid-cols-[minmax(0,.82fr)_minmax(320px,1fr)]',
  showcaseFrame: 'showcase-frame ph-img relative overflow-hidden rounded-[36px] border border-[var(--border)] bg-[radial-gradient(circle_at_42%_35%,rgba(212,157,94,.35),transparent_22%),linear-gradient(135deg,#161c2c,#0a0d16)]',
  promptTag: 'prompt-tag absolute bottom-5 left-5 rounded-lg bg-black/60 px-5 py-3 text-sm text-[var(--fg)] backdrop-blur',
  stat: 'stat rounded-3xl border border-[var(--border)] bg-[var(--surface)] p-7',
  statNum: 'stat-num font-vault-display text-5xl font-semibold leading-none text-[var(--accent)]',
  statLabel: 'stat-label m-0 mt-2 text-sm leading-normal text-[var(--muted)]',
  ctaHero: 'cta-hero rounded-[40px] bg-[linear-gradient(to_bottom,var(--bg),var(--surface))] px-10 py-20 text-center',
  pagefoot: 'pagefoot border-t border-[var(--border)] py-[60px] text-sm text-[var(--muted)]',
  footerInner: 'flex flex-wrap items-center justify-between gap-5',
  footerLinks: 'flex gap-6',
  footerMeta: 'flex flex-col gap-2',
}

const valueProps = [
  {
    title: '多模型路由',
    detail: '集成 OpenAI, OpenRouter 等底层模型。根据您的需求自动选择最佳路径，确保生成质量与效率。',
    icon: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
        <path d="M12 2v20M2 12h20M5 5l14 14M5 19L19 5" />
      </svg>
    ),
  },
  {
    title: '统一计费体系',
    detail: '无需维护多个模型账号。统一积分结算，精确到小数点后 5 位，让每一分投入都清晰可见。',
    icon: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
        <rect x="3" y="3" width="18" height="18" rx="2" />
        <path d="M3 12h18M12 3v18" />
      </svg>
    ),
  },
  {
    title: '开发者友好',
    detail: '提供原生 API 与 OpenAI 兼容接口。AK/SK 独立管理，轻松将 AI 生图能力集成至您的业务。',
    icon: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
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
          <button className={landingClasses.logo} type="button" onClick={() => app.navigate('landing')}>Pic Gallery</button>
          <nav className={landingClasses.navLinks}>
            <a className={landingClasses.navLink} href="#features">功能特性</a>
            <a className={landingClasses.navLink} href="#showcase">作品展示</a>
            <a className={landingClasses.navLink} href="#pricing">计费规则</a>
          </nav>
          <button type="button" className={cn(userButton.base, userButton.primary)} onClick={() => app.navigate(app.isAuthenticated ? 'genpic' : 'login', { returnTo: 'genpic' })}>进入工作台</button>
        </div>
      </header>

      <section className={`${landingClasses.section} ${landingClasses.hero}`}>
        <div className={landingClasses.container}>
          <p className={userText.eyebrow}>AI Powered Creativity</p>
          <h1 className={landingClasses.h1}>文字跃然屏上<br />灵感触手可及</h1>
          <p className={landingClasses.lead}>Pic Gallery 是为您量身定制的 AI 图片生成工作台。集成全球顶尖模型，为您提供极简、艺术、高效的创作体验。</p>
          <div className={landingClasses.heroCta}>
            <button type="button" className={cn(userButton.base, userButton.primary)} onClick={() => app.navigate(app.isAuthenticated ? 'genpic' : 'login', { returnTo: 'genpic' })}>立即免费开始</button>
            <button type="button" className={cn(userButton.base, userButton.ghost)} onClick={() => app.navigate('public-gallery')}>浏览画廊</button>
          </div>
        </div>
      </section>

      <section className={landingClasses.section} id="features">
        <div className={`${landingClasses.container} ${landingClasses.grid3}`}>
          {valueProps.map((prop) => (
            <article className={landingClasses.featureCard} key={prop.title}>
              <div className={landingClasses.featureIcon}>{prop.icon}</div>
              <h2 className={landingClasses.featureTitle}>{prop.title}</h2>
              <p className={landingClasses.featureText}>{prop.detail}</p>
            </article>
          ))}
        </div>
      </section>

      <section className={landingClasses.section} id="showcase">
        <div className={`${landingClasses.container} ${landingClasses.split}`}>
          <div>
            <p className={userText.eyebrow}>Visual Excellence</p>
            <h2 className={landingClasses.h2}>从文生图到参考生图，<br />掌控每一处细节。</h2>
            <p className={landingClasses.lead}>无论是电商产品图、社交媒体封面，还是极具创意的艺术作品，Pic Gallery 都能为您提供精准的参数控制与卓越的输出质量。</p>
            <button type="button" className={cn(userButton.base, userButton.ghost)} onClick={() => app.navigate(app.isAuthenticated ? 'genpic' : 'login', { returnTo: 'genpic' })}>探索创作工具</button>
          </div>
          <div className={landingClasses.showcaseFrame}>
            <img src="/mpdhezm8-image.png" alt="Pic Gallery showcase" onError={(event) => { event.currentTarget.style.display = 'none' }} />
            <div className={landingClasses.promptTag}>Prompt: A high-end watch in a cinematic luxury setting...</div>
          </div>
        </div>
      </section>

      <section className={landingClasses.section} id="pricing">
        <div className={`${landingClasses.container} ${landingClasses.grid3}`}>
          {stats.map((stat) => (
            <div key={stat.label} className={landingClasses.stat}>
              <div className={landingClasses.statNum}>{stat.num}{stat.suffix ? <span className="text-[0.5em]">{stat.suffix}</span> : null}</div>
              <p className={landingClasses.statLabel}>{stat.label}<br />{stat.detail}</p>
            </div>
          ))}
        </div>
      </section>

      <section className={landingClasses.section}>
        <div className={landingClasses.container}>
          <div className={landingClasses.ctaHero}>
            <h2 className={landingClasses.h2}>准备好开启您的 AI 创作之旅了吗？</h2>
            <p className={landingClasses.lead}>加入数千名创作者的行列，体验最纯粹的 AI 生图。无需繁琐配置，只需放飞灵感。</p>
            <button type="button" className={cn(userButton.base, userButton.primary, 'px-12 py-[18px] text-lg')} onClick={() => app.navigate(app.isAuthenticated ? 'genpic' : 'login', { returnTo: 'genpic' })}>立即注册，领取免费积分</button>
          </div>
        </div>
      </section>

      <footer className={landingClasses.pagefoot}>
        <div className={`${landingClasses.container} ${landingClasses.footerInner}`}>
          <div className={`${landingClasses.logo} cursor-default`}>Pic Gallery</div>
          <div className={landingClasses.footerMeta}>
            <div className={landingClasses.footerLinks}>
              <a href="#">隐私政策</a>
              <a href="#">服务条款</a>
              <a href="#">联系我们</a>
            </div>
            <p className="m-0">© 2026 Pic Gallery AI Architecture. All rights reserved.</p>
          </div>
        </div>
      </footer>
    </main>
  )
}
