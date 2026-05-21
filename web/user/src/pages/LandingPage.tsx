import { useApp } from '../components'

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
    <main className="landing-page">
      <header className="topnav">
        <div className="container topnav-inner">
          <button className="landing-logo" type="button" onClick={() => app.navigate('landing')}>Pic Gallery</button>
          <nav>
            <a href="#features">功能特性</a>
            <a href="#showcase">作品展示</a>
            <a href="#pricing">计费规则</a>
          </nav>
          <button type="button" className="btn btn-primary" onClick={() => app.navigate(app.isAuthenticated ? 'genpic' : 'login', { returnTo: 'genpic' })}>进入工作台</button>
        </div>
      </header>

      <section className="section hero">
        <div className="container">
          <p className="eyebrow">AI Powered Creativity</p>
          <h1>文字跃然屏上<br />灵感触手可及</h1>
          <p className="lead">Pic Gallery 是为您量身定制的 AI 图片生成工作台。集成全球顶尖模型，为您提供极简、艺术、高效的创作体验。</p>
          <div className="hero-cta">
            <button type="button" className="btn btn-primary" onClick={() => app.navigate(app.isAuthenticated ? 'genpic' : 'login', { returnTo: 'genpic' })}>立即免费开始</button>
            <button type="button" className="btn btn-ghost" onClick={() => app.navigate(app.isAuthenticated ? 'gallery' : 'login', { returnTo: 'gallery' })}>浏览画廊</button>
          </div>
        </div>
      </section>

      <section className="section" id="features">
        <div className="container grid-3">
          {valueProps.map((prop) => (
            <article className="feature-card" key={prop.title}>
              <div className="feature-icon">{prop.icon}</div>
              <h2>{prop.title}</h2>
              <p>{prop.detail}</p>
            </article>
          ))}
        </div>
      </section>

      <section className="section" id="showcase">
        <div className="container split">
          <div>
            <p className="eyebrow">Visual Excellence</p>
            <h2 className="h2">从文生图到参考生图，<br />掌控每一处细节。</h2>
            <p className="lead">无论是电商产品图、社交媒体封面，还是极具创意的艺术作品，Pic Gallery 都能为您提供精准的参数控制与卓越的输出质量。</p>
            <button type="button" className="btn btn-ghost" onClick={() => app.navigate(app.isAuthenticated ? 'genpic' : 'login', { returnTo: 'genpic' })}>探索创作工具</button>
          </div>
          <div className="showcase-frame ph-img">
            <img src="/mpdhezm8-image.png" alt="Pic Gallery showcase" onError={(event) => { event.currentTarget.style.display = 'none' }} />
            <div className="prompt-tag">Prompt: A high-end watch in a cinematic luxury setting...</div>
          </div>
        </div>
      </section>

      <section className="section" id="pricing">
        <div className="container grid-3">
          {stats.map((stat) => (
            <div key={stat.label} className="stat">
              <div className="stat-num">{stat.num}{stat.suffix ? <span style={{ fontSize: '0.5em' }}>{stat.suffix}</span> : null}</div>
              <p className="stat-label">{stat.label}<br />{stat.detail}</p>
            </div>
          ))}
        </div>
      </section>

      <section className="section">
        <div className="container">
          <div className="cta-hero">
            <h2 className="h2">准备好开启您的 AI 创作之旅了吗？</h2>
            <p className="lead">加入数千名创作者的行列，体验最纯粹的 AI 生图。无需繁琐配置，只需放飞灵感。</p>
            <button type="button" className="btn btn-primary" style={{ padding: '18px 48px', fontSize: 18 }} onClick={() => app.navigate(app.isAuthenticated ? 'genpic' : 'login', { returnTo: 'genpic' })}>立即注册，领取免费积分</button>
          </div>
        </div>
      </section>

      <footer className="pagefoot">
        <div className="container" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 20 }}>
          <div className="landing-logo" style={{ cursor: 'default' }}>Pic Gallery</div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <div style={{ display: 'flex', gap: 24 }}>
              <a href="#">隐私政策</a>
              <a href="#">服务条款</a>
              <a href="#">联系我们</a>
            </div>
            <p style={{ margin: 0 }}>© 2026 Pic Gallery AI Architecture. All rights reserved.</p>
          </div>
        </div>
      </footer>
    </main>
  )
}
