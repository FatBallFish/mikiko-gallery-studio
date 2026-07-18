import { useRef } from 'react'
import { cn } from '../../../shared/classnames'
import { BrandMark, siteBrand } from '../brand'
import { useApp } from '../components'
import {
  ArrowRight,
  Edit,
  ExternalLink,
  Image,
  KeyRound,
  Route,
  Sparkles,
  Wallet,
} from '../ui/icons'
import { useLandingMotion } from '../ui/useLandingMotion'
import { landingActionInk, landingAssetUrl, landingChapters } from './landingContent'

const container = 'mx-auto w-full max-w-[1440px] px-6 sm:px-8 lg:px-12 xl:px-16'
const sectionSpace = 'py-32 md:py-48'

const capabilityIcons = {
  generate: Sparkles,
  edit: Edit,
  reference: Image,
  estimate: Wallet,
} as const

export function LandingPage() {
  const app = useApp()
  const pageRef = useRef<HTMLElement>(null)
  useLandingMotion(pageRef)
  const heroGalleryAsset = landingAssetUrl(import.meta.env.BASE_URL, '/landing/hero-gallery.webp')
  const workspaceAsset = landingAssetUrl(import.meta.env.BASE_URL, '/landing/workspace.webp')

  const goCreate = () => app.navigate(app.isAuthenticated ? 'genpic' : 'login', { returnTo: 'genpic' })

  return (
    <main ref={pageRef} className="w-full max-w-full overflow-x-hidden bg-[var(--bg)] font-vault-body text-[var(--fg)]">
      <style>{`
        @keyframes landing-marquee { to { transform: translate3d(-50%, 0, 0); } }
        @media (prefers-reduced-motion: reduce) {
          .landing-marquee-track { display: none !important; }
          .landing-marquee-static { display: flex !important; }
        }
      `}</style>

      <header className="sticky top-0 z-[100] border-b border-[var(--border-subtle)] bg-[color-mix(in_oklch,var(--bg)_88%,transparent)] backdrop-blur-xl">
        <div className={cn(container, 'flex min-h-[72px] items-center justify-between gap-4')}>
          <button
            type="button"
            className="shrink-0 border-0 bg-transparent p-0 text-[var(--accent)] transition-transform duration-300 hover:scale-[1.03] focus-visible:rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] active:scale-[0.98]"
            onClick={() => app.navigate('landing')}
            aria-label={`${siteBrand.name} 首页`}
          >
            <BrandMark withText />
          </button>

          <nav className="hidden items-center gap-8 md:flex" aria-label="落地页导航">
            <a className="text-sm font-semibold text-[var(--muted)] transition-colors hover:text-[var(--fg)] focus-visible:rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]" href="#capabilities">能力</a>
            <a className="text-sm font-semibold text-[var(--muted)] transition-colors hover:text-[var(--fg)] focus-visible:rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]" href="#workflow">流程</a>
            <a className="text-sm font-semibold text-[var(--muted)] transition-colors hover:text-[var(--fg)] focus-visible:rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]" href="#developers">开发者</a>
          </nav>

          <button
            type="button"
            className="min-h-11 rounded-xl border border-[var(--border)] bg-[var(--surface)] px-4 text-sm font-bold text-[var(--fg)] transition-all duration-200 hover:-translate-y-px hover:border-[var(--border-strong)] hover:bg-[var(--elevated)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] active:translate-y-0 active:scale-[0.98]"
            onClick={() => app.navigate('login')}
          >
            {app.isAuthenticated ? '返回首页' : '登录'}
          </button>
        </div>
      </header>

      <section className="relative min-h-[600px] border-b border-[var(--border-subtle)] py-10 md:min-h-[760px] md:py-24">
        <div className={cn(container, 'relative flex min-h-[520px] flex-col justify-center md:min-h-[568px]')}>
          <div className="relative z-20 max-w-[1120px]">
            <h1 className="m-0 w-full max-w-6xl font-vault-display text-[1.5rem] font-bold leading-[1.08] tracking-[0] min-[360px]:text-[1.75rem] min-[420px]:text-[2.25rem] md:text-[3.5rem] lg:text-[4.5rem] xl:text-[4.75rem]">
              {landingChapters.hero.title.map((line) => (
                <span key={line} data-landing-title-line className="block whitespace-nowrap">{line}</span>
              ))}
            </h1>
            <p className="mt-8 max-w-[620px] text-base leading-8 text-[var(--muted)] md:text-lg">
              {landingChapters.hero.summary}
            </p>
            <div className="mt-10 flex flex-col gap-3 min-[420px]:flex-row">
              <button
                type="button"
                className="inline-flex min-h-14 items-center justify-center gap-2 rounded-xl border border-[var(--accent)] bg-[var(--accent)] px-7 text-base font-bold text-[#111218] shadow-[0_18px_50px_-24px_rgba(var(--accent-rgb),0.8)] transition-all duration-200 hover:-translate-y-1 hover:shadow-[0_24px_60px_-22px_rgba(var(--accent-rgb),0.9)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] focus-visible:ring-offset-4 focus-visible:ring-offset-[var(--bg)] active:translate-y-0 active:scale-[0.98]"
                style={{ color: landingActionInk }}
                onClick={goCreate}
              >
                {landingChapters.hero.actions[0].label}
                <ArrowRight size={18} aria-hidden="true" />
              </button>
              <button
                type="button"
                className="inline-flex min-h-14 items-center justify-center gap-2 rounded-xl border border-[#f6f2eb] bg-[#f6f2eb] px-7 text-base font-bold text-[#111218] transition-all duration-200 hover:-translate-y-1 hover:bg-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#f6f2eb] focus-visible:ring-offset-4 focus-visible:ring-offset-[var(--bg)] active:translate-y-0 active:scale-[0.98]"
                style={{ color: landingActionInk }}
                onClick={() => app.navigate('docs')}
              >
                {landingChapters.hero.actions[1].label}
                <ExternalLink size={17} aria-hidden="true" />
              </button>
            </div>
          </div>

          <button
            type="button"
            className="group absolute -bottom-12 -right-[32%] z-10 aspect-[16/9] w-[112%] overflow-hidden rounded-2xl border border-[var(--border-strong)] bg-[var(--surface)] text-left opacity-60 shadow-[0_36px_100px_-40px_rgba(0,0,0,0.8)] transition-all duration-700 hover:-translate-y-2 hover:border-[color-mix(in_oklch,var(--accent)_45%,var(--border))] hover:opacity-100 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] active:scale-[0.99] md:-bottom-28 md:-right-[14%] md:w-[64%] md:rotate-[-2deg] md:opacity-100 md:hover:rotate-0"
            onClick={goCreate}
            aria-label="打开图片生成页面"
          >
            <img
              data-landing-image
              className="size-full object-cover object-left transition-transform duration-700 ease-out group-hover:scale-105 group-focus-visible:scale-105"
              src={heroGalleryAsset}
              alt="Mikiko 图片详情中的真实生成结果"
              width={1280}
              height={720}
              loading="eager"
              decoding="async"
              fetchPriority="high"
            />
            <span data-landing-image-overlay className="pointer-events-none absolute inset-0 bg-black opacity-0" />
            <span className="absolute bottom-4 left-4 inline-flex items-center gap-2 rounded-lg border border-white/15 bg-black/70 px-3 py-2 text-xs font-semibold text-white backdrop-blur-md md:bottom-6 md:left-6 md:text-sm">
              从灵感发现进入生成与资产管理
              <ArrowRight size={15} aria-hidden="true" />
            </span>
          </button>
        </div>
      </section>

      <section id="capabilities" className={cn(sectionSpace, 'relative')}>
        <div className={container}>
          <div className="mb-16 grid gap-8 md:grid-cols-[minmax(0,1fr)_minmax(280px,440px)] md:items-end">
            <h2 className="m-0 max-w-[900px] font-vault-display text-4xl font-bold leading-[1.05] tracking-[0] md:text-6xl">
              从意图到结果，能力各有边界
            </h2>
            <p className="m-0 text-base leading-7 text-[var(--muted)]">
              平台读取当前模型能力开放参数，在提交前完成输入校验，让生成路径和预计消耗都更清楚。
            </p>
          </div>

          {/* Desktop occupancy: 12 columns x 2 rows = 24 cells; 7x2 + 5x1 + 3x1 + 2x1 = 24. */}
          <div className="grid grid-flow-dense grid-cols-1 gap-3 md:grid-cols-12 md:grid-rows-2 md:auto-rows-[240px]">
            {landingChapters.capabilities.map((capability) => {
              const Icon = capabilityIcons[capability.id]
              return (
                <button
                  key={capability.id}
                  type="button"
                  className={cn(
                    'group relative min-h-[220px] overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-6 text-left transition-all duration-500 hover:-translate-y-1 hover:border-[color-mix(in_oklch,var(--accent)_48%,var(--border))] hover:shadow-[0_28px_70px_-42px_rgba(var(--accent-rgb),0.8)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] active:translate-y-0 active:scale-[0.99] md:min-h-0',
                    capability.columns === 7 && 'md:col-span-7',
                    capability.columns === 5 && 'md:col-span-5',
                    capability.columns === 3 && 'md:col-span-3',
                    capability.columns === 2 && 'md:col-span-2',
                    capability.rows === 2 && 'md:row-span-2',
                  )}
                  onClick={goCreate}
                >
                  {capability.image ? (
                    <img
                      data-landing-image
                      src={landingAssetUrl(import.meta.env.BASE_URL, capability.image)}
                      alt=""
                      width={capability.image === '/landing/workspace.webp' ? 1291 : 1280}
                      height={capability.image === '/landing/workspace.webp' ? 808 : 720}
                      loading="lazy"
                      decoding="async"
                      className="absolute inset-0 size-full object-cover opacity-30 transition-all duration-700 ease-out group-hover:scale-105 group-hover:opacity-45 group-focus-visible:scale-105 group-focus-visible:opacity-45"
                    />
                  ) : null}
                  {capability.image ? <span data-landing-image-overlay className="pointer-events-none absolute inset-0 bg-black opacity-0" /> : null}
                  <span className="absolute inset-0 bg-[color-mix(in_oklch,var(--surface-solid)_76%,transparent)]" />
                  <span className="relative z-10 flex h-full flex-col">
                    <span className="mb-auto inline-grid size-11 place-items-center rounded-xl border border-[var(--border)] bg-[var(--bg)] text-[var(--accent)] transition-transform duration-300 group-hover:scale-105 group-focus-visible:scale-105">
                      <Icon size={20} strokeWidth={1.5} aria-hidden="true" />
                    </span>
                    <span className="mt-10 block font-vault-display text-2xl font-bold leading-tight text-[var(--fg)] md:text-3xl">
                      {capability.title}
                    </span>
                    <span className="mt-3 block max-w-[54ch] text-sm leading-6 text-[var(--muted)]">
                      {capability.detail}
                    </span>
                  </span>
                </button>
              )
            })}
          </div>
        </div>
      </section>

      <section id="workflow" className={cn(sectionSpace, 'border-y border-[var(--border-subtle)] bg-[var(--canvas)]')}>
        <div className={container}>
          <h2 className="m-0 max-w-[1100px] font-vault-display text-4xl font-bold leading-[1.08] tracking-[0] md:text-6xl">
            构想
            <span className="mx-3 inline-block h-[0.72em] w-20 overflow-hidden rounded-full border border-[var(--border-strong)] align-middle transition-transform duration-700 hover:scale-105 md:w-32" aria-hidden="true">
              <img
                src={workspaceAsset}
                alt=""
                width={1291}
                height={808}
                loading="lazy"
                decoding="async"
                className="size-full object-cover"
              />
            </span>
            经过一条可见链路，成为资产
          </h2>

          <p data-landing-reveal className="mt-16 max-w-[1100px] text-2xl font-medium leading-[1.6] text-[var(--fg)] md:text-4xl md:leading-[1.5]">
            {[...landingChapters.workflow.statement].map((character, index) => (
              <span key={`${character}-${index}`} data-landing-word>{character}</span>
            ))}
          </p>

          <div className="mt-20 grid gap-px overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--border)] sm:grid-cols-2 lg:grid-cols-4">
            {landingChapters.workflow.steps.map((step, index) => (
              <div key={step} className="min-h-36 bg-[var(--surface-solid)] p-6">
                <span className="font-vault-mono text-xs text-[var(--dim)]">0{index + 1}</span>
                <p className="mt-10 text-lg font-bold text-[var(--fg)]">{step}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className={sectionSpace} aria-labelledby="modes-title">
        <div className={container}>
          <div className="mb-14 flex flex-col justify-between gap-6 md:flex-row md:items-end">
            <h2 id="modes-title" className="m-0 max-w-[760px] font-vault-display text-4xl font-bold leading-[1.08] tracking-[0] md:text-6xl">
              选择输入方式，不改变结果去向
            </h2>
            <p className="max-w-[420px] text-base leading-7 text-[var(--muted)]">每种任务都沿用能力校验、预估、状态追踪和资产保存。</p>
          </div>

          <div className="flex min-h-[620px] flex-col gap-3 lg:min-h-[560px] lg:flex-row">
            {landingChapters.modes.map((mode) => (
              <button
                key={mode.id}
                type="button"
                className="group relative min-h-[190px] flex-1 overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--surface)] text-left transition-[flex,transform,border-color] duration-700 ease-out hover:flex-[1.75] hover:-translate-y-1 hover:border-[color-mix(in_oklch,var(--accent)_48%,var(--border))] focus-visible:flex-[1.75] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] active:translate-y-0 active:scale-[0.99] lg:min-h-0"
                onClick={goCreate}
              >
                <img
                  data-landing-image
                  src={landingAssetUrl(import.meta.env.BASE_URL, mode.image)}
                  alt=""
                  width={mode.image === '/landing/workspace.webp' ? 1291 : 1280}
                  height={mode.image === '/landing/workspace.webp' ? 808 : 720}
                  loading="lazy"
                  decoding="async"
                  className="absolute inset-0 size-full object-cover opacity-45 transition-all duration-700 ease-out group-hover:scale-105 group-hover:opacity-65 group-focus-visible:scale-105 group-focus-visible:opacity-65"
                />
                <span data-landing-image-overlay className="pointer-events-none absolute inset-0 bg-black opacity-0" />
                <span className="absolute inset-0 bg-[linear-gradient(to_top,rgba(5,6,10,0.96),rgba(5,6,10,0.12))]" />
                <span className="absolute inset-x-0 bottom-0 z-10 block p-6 text-white md:p-8">
                  <span className="block font-vault-display text-2xl font-bold md:text-3xl">{mode.title}</span>
                  <span className="mt-3 block max-w-[48ch] text-sm leading-6 text-white/70 opacity-100 transition-opacity duration-500 lg:opacity-0 lg:group-hover:opacity-100 lg:group-focus-visible:opacity-100">{mode.detail}</span>
                </span>
              </button>
            ))}
          </div>
        </div>
      </section>

      <section id="developers" className={cn(sectionSpace, 'overflow-hidden border-y border-[var(--border-subtle)] bg-[var(--canvas)]')}>
        <div className={container}>
          <div className="grid gap-16 lg:grid-cols-[minmax(0,1fr)_minmax(420px,0.82fr)] lg:items-center">
            <div>
              <div className="mb-8 inline-grid size-12 place-items-center rounded-xl border border-[var(--border)] bg-[var(--surface)] text-[var(--accent)]">
                <Route size={22} strokeWidth={1.5} aria-hidden="true" />
              </div>
              <h2 className="m-0 max-w-[820px] font-vault-display text-4xl font-bold leading-[1.08] tracking-[0] md:text-6xl">{landingChapters.developer.title}</h2>
              <p className="mt-8 max-w-[720px] text-base leading-8 text-[var(--muted)] md:text-lg">{landingChapters.developer.detail}</p>
              <button
                type="button"
                className="mt-10 inline-flex min-h-12 items-center gap-2 rounded-xl border border-[var(--border)] bg-[var(--surface)] px-5 text-sm font-bold text-[var(--fg)] transition-all duration-200 hover:-translate-y-1 hover:border-[var(--border-strong)] hover:bg-[var(--elevated)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] active:translate-y-0 active:scale-[0.98]"
                onClick={() => app.navigate('docs')}
              >
                查看接入方式
                <ExternalLink size={16} aria-hidden="true" />
              </button>
            </div>

            <div className="overflow-hidden rounded-2xl border border-[var(--border)] bg-[#080a10] text-[#f6f2eb] shadow-[0_32px_80px_-48px_rgba(0,0,0,0.9)]">
              <div className="flex items-center justify-between border-b border-white/10 px-5 py-4">
                <span className="font-vault-mono text-xs text-white/55">images/generations</span>
                <span className="inline-flex items-center gap-2 text-xs text-white/60"><KeyRound size={14} aria-hidden="true" />AK / SK</span>
              </div>
              <pre className="m-0 overflow-x-auto p-6 font-vault-mono text-[13px] leading-7 text-white/80"><code>{`POST /v1/images/generations

{
  "model": "gpt-image-2",
  "prompt": "cinematic product scene",
  "size": "auto",
  "n": 1
}`}</code></pre>
            </div>
          </div>
        </div>

        <div className="mt-24 overflow-hidden border-y border-[var(--border-subtle)] py-5" aria-label="平台接入能力">
          <div className="landing-marquee-track flex w-max animate-[landing-marquee_28s_linear_infinite] items-center will-change-transform">
            {[0, 1].map((copy) => (
              <div key={copy} className="flex shrink-0 items-center" aria-hidden={copy === 1 ? 'true' : undefined}>
                {landingChapters.developer.terms.map((term) => (
                  <span key={`${copy}-${term}`} className="flex items-center whitespace-nowrap px-7 font-vault-display text-xl font-semibold text-[var(--muted)] md:px-12 md:text-2xl">
                    {term}<span className="ml-14 size-1.5 rounded-full bg-[var(--accent)] md:ml-24" />
                  </span>
                ))}
              </div>
            ))}
          </div>
          <div className="landing-marquee-static hidden flex-wrap justify-center gap-x-8 gap-y-3 px-6">
            {landingChapters.developer.terms.map((term) => (
              <span
                key={term}
                data-landing-marquee-term
                className="whitespace-nowrap font-vault-display text-base font-semibold text-[var(--muted)]"
              >
                {term}
              </span>
            ))}
          </div>
        </div>
      </section>

      <section className={sectionSpace}>
        <div className={container}>
          <div className="border-y border-[var(--border-strong)] py-20 text-center md:py-28">
            <h2 className="mx-auto m-0 max-w-[1000px] font-vault-display text-4xl font-bold leading-[1.06] tracking-[0] md:text-7xl">{landingChapters.closing.title}</h2>
            <p className="mx-auto mt-7 max-w-[620px] text-base leading-7 text-[var(--muted)] md:text-lg">{landingChapters.closing.detail}</p>
            <div className="mt-10 flex flex-col justify-center gap-3 min-[420px]:flex-row">
              <button
                type="button"
                className="inline-flex min-h-14 items-center justify-center gap-2 rounded-xl border border-[var(--accent)] bg-[var(--accent)] px-7 text-base font-bold text-[#111218] transition-all duration-200 hover:-translate-y-1 hover:shadow-[0_20px_55px_-28px_rgba(var(--accent-rgb),0.9)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] focus-visible:ring-offset-4 focus-visible:ring-offset-[var(--bg)] active:translate-y-0 active:scale-[0.98]"
                style={{ color: landingActionInk }}
                onClick={goCreate}
              >
                开始生成
                <ArrowRight size={18} aria-hidden="true" />
              </button>
              <button
                type="button"
                className="inline-flex min-h-14 items-center justify-center gap-2 rounded-xl border border-[var(--border-strong)] bg-[var(--surface)] px-7 text-base font-bold text-[var(--fg)] transition-all duration-200 hover:-translate-y-1 hover:bg-[var(--elevated)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] active:translate-y-0 active:scale-[0.98]"
                onClick={() => app.navigate('docs')}
              >
                阅读 API 文档
                <ExternalLink size={17} aria-hidden="true" />
              </button>
            </div>
          </div>
        </div>
      </section>

      <footer className="border-t border-[var(--border-subtle)] py-12 text-sm text-[var(--muted)]">
        <div className={cn(container, 'flex flex-col justify-between gap-8 md:flex-row md:items-end')}>
          <div>
            <div className="text-[var(--accent)]"><BrandMark withText /></div>
            <p className="mt-5 max-w-[440px] leading-6">连接图片生成、任务状态、积分计费与历史资产的一体化平台。</p>
          </div>
          <div className="flex flex-wrap items-center gap-x-7 gap-y-3">
            <button type="button" className="border-0 bg-transparent p-0 text-inherit transition-colors hover:text-[var(--fg)] focus-visible:rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]" onClick={() => app.navigate('docs')}>API 文档</button>
            <button type="button" className="border-0 bg-transparent p-0 text-inherit transition-colors hover:text-[var(--fg)] focus-visible:rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]" onClick={() => app.navigate('public-gallery')}>公开画廊</button>
            <span>© 2026 {siteBrand.name}</span>
          </div>
        </div>
      </footer>
    </main>
  )
}

export default LandingPage
