import { useEffect, useRef } from 'react'
import { cn } from '../../../shared/classnames'
import { siteBrand } from '../brand'
import { useApp } from '../components'
import { openDocsEntry } from '../docsUrl'
import { ArrowRight, ExternalLink, KeyRound, Route } from '../ui/icons'
import { useLandingMotion } from '../ui/useLandingMotion'
import { landingAssetUrl, landingChapters, type LandingAsset } from './landingContent'


function LandingAssetPicture({ asset, pictureClassName, imageClassName }: { asset: LandingAsset; pictureClassName?: string; imageClassName: string }) {
  const avif = landingAssetUrl(import.meta.env.BASE_URL, asset.avif)
  const webp = landingAssetUrl(import.meta.env.BASE_URL, asset.webp)
  return (
    <picture className={pictureClassName}>
      <source type="image/avif" srcSet={avif} />
      <source type="image/webp" srcSet={webp} />
      <img
        data-landing-image
        src={webp}
        alt=""
        width={asset.width}
        height={asset.height}
        loading="lazy"
        decoding="async"
        className={imageClassName}
      />
    </picture>
  )
}

export function LandingPage() {
  const app = useApp()
  const pageRef = useRef<HTMLElement>(null)
  useLandingMotion(pageRef)

  useEffect(() => {
    const root = pageRef.current
    if (!root) return undefined
    const items = Array.from(root.querySelectorAll<HTMLElement>('[data-reveal]'))
    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    if (reduceMotion || !('IntersectionObserver' in window)) {
      items.forEach((item) => item.classList.add('is-in'))
      return undefined
    }
    const observer = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (!entry.isIntersecting) return
        entry.target.classList.add('is-in')
        observer.unobserve(entry.target)
      })
    }, { rootMargin: '0px 0px -8% 0px', threshold: 0.06 })
    items.forEach((item) => observer.observe(item))
    return () => observer.disconnect()
  }, [])
  const studioShowcase1280Webp = landingAssetUrl(import.meta.env.BASE_URL, '/landing/studio-showcase-1280.webp')
  const studioShowcase1920Webp = landingAssetUrl(import.meta.env.BASE_URL, '/landing/studio-showcase-1920.webp')
  const studioShowcase1280Avif = landingAssetUrl(import.meta.env.BASE_URL, '/landing/studio-showcase-1280.avif')
  const studioShowcase1920Avif = landingAssetUrl(import.meta.env.BASE_URL, '/landing/studio-showcase-1920.avif')

  const goCreate = () => app.navigate(app.isAuthenticated ? 'genpic' : 'login', { returnTo: 'genpic' })

  return (
    <main ref={pageRef} className="u-landing">
      <style>{`
        @media (prefers-reduced-motion: reduce) {
          .landing-marquee-track { display: none !important; }
          .landing-marquee-static { display: flex !important; }
        }
      `}</style>
      <header className="u-landing__nav">
        <button className="u-landing__brand" type="button" onClick={() => app.navigate('landing')} aria-label={`${siteBrand.name} 首页`}>
          <i>M</i>{siteBrand.name}
        </button>
        <nav className="u-landing__nav-links" aria-label="落地页导航">
          <a href="#capabilities">能力</a>
          <a href="#workflow">流程</a>
          <a href="#developers">开发者</a>
        </nav>
        <div style={{ flex: 1 }} />
        <button className="u-btn u-btn--quiet u-btn--sm" type="button" onClick={() => app.navigate(app.isAuthenticated ? 'home' : 'login')}>
          {app.isAuthenticated ? '工作台' : '登录'}
        </button>
        <button className="u-btn u-btn--primary u-btn--sm" type="button" onClick={goCreate}>进入工作室</button>
      </header>

      <div className="u-landing__wrap">
        <section className="u-hero">
          <div className="u-hero__copy">
            <span className="u-eyebrow">Image &amp; Video Generation Platform</span>
            <h1>
              {landingChapters.hero.title.map((line) => (
                <span key={line} data-landing-title-line style={{ display: 'block' }}>{line}</span>
              ))}
            </h1>
            <p>{landingChapters.hero.summary}</p>
            <div className="u-hero__actions">
              <button className="u-btn u-btn--primary u-btn--lg" type="button" onClick={goCreate}>{landingChapters.hero.actions[0].label}</button>
              <button className="u-btn u-btn--lg" type="button" onClick={() => openDocsEntry('home')}>{landingChapters.hero.actions[1].label}</button>
            </div>
            <div className="u-hero__proof">
              <div><b>14</b><span>可用抽象模型</span></div>
              <div><b>6</b><span>视频生成方式</span></div>
              <div><b>1.8s</b><span>平均估价响应</span></div>
              <div><b>100%</b><span>结果可追溯</span></div>
            </div>
          </div>
          <button className="u-hero__art" type="button" onClick={goCreate} aria-label="打开创作工作台">
            <picture>
              <source type="image/avif" srcSet={`${studioShowcase1280Avif} 1280w, ${studioShowcase1920Avif} 1920w`} sizes="(min-width: 980px) 46vw, 100vw" />
              <source type="image/webp" srcSet={`${studioShowcase1280Webp} 1280w, ${studioShowcase1920Webp} 1920w`} sizes="(min-width: 980px) 46vw, 100vw" />
              <img
                data-landing-image
                src={studioShowcase1280Webp}
                alt="Mikiko Studio 原创视觉资产工作台"
                width={1280}
                height={720}
                loading="eager"
                decoding="async"
                fetchPriority="high"
              />
            </picture>
            <span data-landing-image-overlay className="pointer-events-none absolute inset-0 bg-black opacity-0" />
            <span className="u-hero__float">
              <span>Recent workspace</span>
              <strong>创作 · 任务 · 资产</strong>
              <span className="u-status u-status--ok"><span className="u-dot" aria-hidden="true" />同一条链路</span>
            </span>
          </button>
        </section>

        <section id="capabilities" data-reveal>
          <div className="u-landing__section-head">
            <h2>从意图到结果，<br />能力各有边界</h2>
            <p>平台读取当前模型能力开放参数，在提交前完成输入校验，让生成路径和预计消耗都更清楚。</p>
          </div>
          <div className="u-bento">
            {landingChapters.capabilities.map((capability, index) => (
              <button
                key={capability.id}
                type="button"
                className={cn(
                  'u-bento__item',
                  index === 0 && 'u-bento__item--hero',
                  index === 1 && 'u-bento__item--wide',
                  index === 2 && 'u-bento__item--half',
                  index === 3 && 'u-bento__item--small',
                )}
                onClick={goCreate}
                data-reveal
                data-reveal-delay={index || undefined}
              >
                <LandingAssetPicture
                  asset={capability.image}
                  pictureClassName="u-bento__picture"
                  imageClassName="u-bento__image"
                />
                <span data-landing-image-overlay className="pointer-events-none absolute inset-0 bg-black opacity-0" />
                <h3>{capability.title}</h3>
                <p>{capability.detail}</p>
              </button>
            ))}
          </div>
        </section>

        <section id="workflow" data-reveal>
          <div className="u-landing__section-head">
            <h2>一次配置，<br />完整走完生成链路</h2>
            <p>创作不是一堆散落的弹窗。参数、预估、任务状态和结果归档在同一条阅读路径里，失败也能定位到具体环节。</p>
          </div>
          <div className="lg-workflow" data-reveal>
            <LandingAssetPicture
              asset={landingChapters.workflow.image}
              pictureClassName="lg-workflow__picture"
              imageClassName="lg-workflow__image"
            />
            <p data-landing-reveal className="lg-workflow__quote">
              {[...landingChapters.workflow.statement].map((character, index) => (
                <span key={`${character}-${index}`} data-landing-word>{character}</span>
              ))}
            </p>
          </div>
          <div className="u-steps" style={{ marginTop: 24 }}>
            {landingChapters.workflow.steps.map((step, index) => (
              <div key={step} className="u-step">
                <b>0{index + 1}</b>
                <strong>{step}</strong>
                <p>{index === 0 ? '模型、尺寸模式、质量与参考图，一次性确认。' : index === 1 ? '提交前看见本次消耗与最大预留积分。' : index === 2 ? '参数校验 → 路由 → 排队 → 生成 → 入库 → 结算。' : '复用参数、公开分享或纳入项目资产。'}</p>
              </div>
            ))}
          </div>
        </section>

        <section data-reveal>
          <div className="u-landing__section-head">
            <h2>三种表达方式，<br />同一套控制面</h2>
            <p>文字、编辑与参考图不是三套界面，而是同一组参数的不同入口。</p>
          </div>
          <div className="lg-mode">
            {landingChapters.modes.map((mode, index) => (
              <button key={mode.id} type="button" className="lg-mode__item" onClick={goCreate} data-reveal data-reveal-delay={index || undefined}>
                <LandingAssetPicture
                  asset={mode.image}
                  pictureClassName="lg-mode__picture"
                  imageClassName="lg-mode__image"
                />
                <span data-landing-image-overlay className="pointer-events-none absolute inset-0 bg-black opacity-0" />
                <div className="lg-mode__copy">
                  <b>{mode.title}</b>
                  <p>{mode.detail}</p>
                </div>
              </button>
            ))}
          </div>
        </section>

        <section id="developers" data-reveal>
          <div className="u-code">
            <div style={{ display: 'grid', gap: 16 }}>
              <span className="u-eyebrow">Developers</span>
              <h2 style={{ fontFamily: 'var(--font-display)', fontSize: 'clamp(26px,2.8vw,38px)', fontWeight: 400, lineHeight: 1.1, letterSpacing: '-.025em' }}>{landingChapters.developer.title}</h2>
              <p className="u-sub">{landingChapters.developer.detail}</p>
              <div>
                <button className="u-btn u-btn--sm" type="button" onClick={() => openDocsEntry('home')}>查看接入方式 <ExternalLink size={15} aria-hidden="true" /></button>
              </div>
            </div>
            <div>
              <pre>
                <span className="c"># 创建任务（异步）</span>{"\n"}
                curl -X POST https://api.mikiko.studio/v1/images/generations \{"\n"}
                {"  "}-H <span className="s">"Authorization: Bearer $MIKIKO_SK"</span> \{"\n"}
                {"  "}-d <span className="s">{`'{
    "model": "plus-image",
    "prompt": "黄昏光线下的玻璃温室，胶片质感",
    "size": "1536x1024",
    "quality": "high",
    "n": 2
  }'`}</span>{"\n\n"}
                <span className="c"># 查询任务与积分结算</span>{"\n"}
                GET /v1/tasks/<span className="n">{'{'}task_id{'}'}</span>
              </pre>
              <div className="u-marquee" style={{ marginTop: 14 }}>
                <div className="landing-marquee-track">
                  {[...landingChapters.developer.terms, ...landingChapters.developer.terms].map((term, index) => (
                    <span key={`${term}-${index}`} data-landing-marquee-term={index < landingChapters.developer.terms.length ? '' : undefined}>{term}</span>
                  ))}
                </div>
              </div>
              <div className="landing-marquee-static flex-wrap" style={{ display: 'none', gap: '8px 22px', marginTop: 14 }}>
                {landingChapters.developer.terms.map((term) => (
                  <span key={term} className="u-meta">{term}</span>
                ))}
              </div>
            </div>
          </div>
        </section>

        <section data-reveal>
          <div className="u-cta">
            <h2>{landingChapters.closing.title}</h2>
            <div className="u-hero__actions">
              <button className="u-btn u-btn--primary u-btn--lg" type="button" onClick={goCreate}>开始生成 <ArrowRight size={18} aria-hidden="true" /></button>
              <button className="u-btn u-btn--lg" type="button" onClick={() => openDocsEntry('home')}>阅读 API 文档</button>
            </div>
          </div>
        </section>

        <footer className="u-landing__foot">
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <span className="u-landing__brand" style={{ fontSize: 14 }}><i style={{ width: 26, height: 26, fontSize: 12 }}>M</i>{siteBrand.name}</span>
          </div>
          <div className="u-landing__foot-links">
            <button type="button" onClick={() => app.navigate('public-gallery')}>公开广场</button>
            <button type="button" onClick={() => openDocsEntry('footer')}>API 文档</button>
            <span>服务协议</span>
            <span>隐私条款</span>
          </div>
          <span className="u-meta">© 2026 {siteBrand.name}</span>
        </footer>
      </div>
    </main>
  )
}

export default LandingPage
