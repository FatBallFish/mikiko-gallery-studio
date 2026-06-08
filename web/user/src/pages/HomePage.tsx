import { useMemo, useState } from 'react'
import type { Balance, Capability, ImageResult } from '../../../shared/api-types'
import { cn } from '../../../shared/classnames'
import { openApi } from '../../../shared/open-api'
import { userApi } from '../../../shared/user-api'
import heroImage from '../../../../docs/template/PicGallery/mpdhezm8-image.png'
import { Modal, PublicImageDetail, copyText, useApp } from '../components'
import { publicEngagementScore } from '../publicEngagementModel'
import { userButton, userShell } from '../ui/classes'
import { useApiResource } from '../useApiResource'
import { homeAccountReadinessView, homeGalleryCardView, homeModelReadinessView, homePublicGalleryAccess } from './homeGalleryModel'

type FilterMode = 'latest' | 'hot'

type MasonryCard = {
  id: string
  title: string
  meta: string
  imageUrl?: string
  createdAt: string
  hotScore: number
  image: ImageResult
  onClick: () => void
}

const homeClasses = {
  content: userShell.content,
  readiness: 'mb-[22px] grid grid-cols-[repeat(auto-fit,minmax(150px,1fr))] items-stretch gap-3.5 rounded-2xl border border-[var(--border)] bg-[linear-gradient(135deg,rgba(191,161,106,.14),rgba(255,255,255,.04))] p-[18px]',
  readinessIntro: 'grid min-w-0 content-center gap-1.5 [grid-column:span_2]',
  readinessTitle: 'm-0 text-lg font-black',
  readinessDetail: 'm-0 text-[13px] text-[var(--muted)]',
  readinessMetric: 'min-w-0 rounded-[10px] border border-white/[.08] bg-[rgba(8,10,16,.38)] p-3.5',
  readinessMetricWarning: 'border-[rgba(191,161,106,.55)]',
  readinessLabel: 'mb-2 block text-xs font-extrabold text-[var(--muted)]',
  readinessValue: 'block font-mono text-base font-black text-[var(--fg)] [overflow-wrap:anywhere]',
  readinessValueWarning: 'text-[var(--accent)]',
  readinessCta: 'grid min-w-[132px] content-center gap-2',
  carousel: 'relative mb-10 flex min-h-80 cursor-pointer items-center justify-center overflow-hidden rounded-[20px] border border-[var(--border)] bg-[var(--surface)] [aspect-ratio:21/9] max-[760px]:[aspect-ratio:4/3]',
  heroImage: 'size-full object-cover opacity-80',
  carouselOverlay: 'absolute inset-0 flex flex-col items-start justify-end bg-[linear-gradient(to_top,var(--bg)_0%,rgba(8,10,16,.58)_36%,transparent_70%)] p-10 max-[760px]:p-6',
  heroKicker: 'm-0 mb-2 font-mono text-xs uppercase tracking-[.1em] text-[var(--accent)]',
  heroTitle: 'm-0 font-[var(--font-display)] text-[clamp(2.4rem,5vw,4rem)] leading-[.95] text-white',
  heroText: 'my-3.5 mb-5 max-w-[60ch] text-sm text-[var(--muted)]',
  actionRow: 'flex flex-wrap gap-3',
  sectionHead: 'mb-6 flex items-end justify-between gap-4 max-[760px]:items-start max-[760px]:flex-col',
  sectionTitle: 'm-0 font-[var(--font-display)] text-[28px]',
  sectionDetail: 'mt-1.5 block text-[13px] text-[var(--muted)]',
  filterGroup: 'flex flex-wrap justify-end gap-3',
  masonry: 'columns-[280px_3] gap-6 max-[760px]:columns-1',
  masonryItem: 'mb-6 inline-block w-full cursor-pointer break-inside-avoid overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--surface)] text-left text-[var(--fg)]',
  masonryImage: 'h-auto w-full opacity-90 transition duration-300 hover:scale-[1.02] hover:opacity-100',
  cardBody: 'p-4',
  cardTitle: 'mb-1 block text-[13px] font-bold',
  cardMeta: 'font-mono text-[11px] uppercase text-[var(--muted)]',
  placeholder: 'grid min-h-[220px] place-items-center bg-white/[.04] text-[var(--muted)]',
  smallNote: 'm-0 mt-2.5 text-[13px] text-[var(--muted)]',
  button: cn(userButton.base, 'min-h-0 rounded-[20px] px-4 py-1.5 text-[13px] font-semibold'),
  buttonActive: cn(userButton.primary, 'font-extrabold'),
  buttonDisabled: 'cursor-not-allowed opacity-55',
}

export function HomePage() {
  const app = useApp()
  const [filter, setFilter] = useState<FilterMode>('hot')
  const [selected, setSelected] = useState<ImageResult | null>(null)
  const capability = useApiResource(() => userApi.getCapabilities(), [app.session?.token])
  const gallery = useApiResource(() => openApi.listPublicGallery(1, 8, { sort: filter, accessToken: app.session?.token }), [filter, app.session?.token])

  const cards = useMemo(() => {
    return (gallery.data?.items ?? []).map((image) => {
      const card = homeGalleryCardView(image)
      return {
        id: image.id,
        title: card.title,
        meta: card.meta,
        imageUrl: image.url ? userApi.imageAssetUrl(image.url, app.session?.token) : undefined,
        createdAt: image.created_at ?? '',
        hotScore: publicEngagementScore(image),
        image,
        onClick: () => void openFeaturedDetail(image),
      }
    })
  }, [app.session?.token, gallery.data])

  async function openFeaturedDetail(image: ImageResult) {
    const access = homePublicGalleryAccess(app.session?.token, image.id)
    if (access.action === 'login') {
      app.notify('info', '请先登录后再查看完整作品')
      app.navigate('login', { returnTo: access.returnTo })
      return
    }
    try {
      const detail = await openApi.getPublicGalleryImage(access.imageId, { accessToken: app.session?.token })
      setSelected(detail)
    } catch {
      app.notify('error', '作品详情读取失败，请稍后重试')
    }
  }

  async function toggleReaction(image: ImageResult, kind: 'like' | 'favorite') {
    if (!app.session?.token) {
      app.notify('info', kind === 'like' ? '请先登录后再点赞' : '请先登录后再收藏')
      app.navigate('login', { returnTo: 'home' })
      return
    }
    const active = kind === 'like' ? !image.liked_by_viewer : !image.favorited_by_viewer
    try {
      const updated = kind === 'like'
        ? await userApi.likePublicImage(image.id, active)
        : await userApi.favoritePublicImage(image.id, active)
      setSelected((current) => current?.id === image.id ? { ...current, ...updated, publish_status: updated.visibility_status } : current)
      await gallery.reload()
    } catch {
      app.notify('error', '操作失败，请稍后重试')
    }
  }

  function imageUrl(image: ImageResult) {
    const url = image.url || image.download_url
    return url ? userApi.imageAssetUrl(url, null) : undefined
  }

  function downloadImage(image: ImageResult) {
    const url = image.url || image.download_url
    if (!url) return
    const link = document.createElement('a')
    link.href = userApi.imageAssetUrl(url, null)
    link.download = downloadFilename(image)
    link.rel = 'noopener noreferrer'
    document.body.appendChild(link)
    link.click()
    link.remove()
  }

  function downloadFilename(image: ImageResult) {
    const source = image.download_url ?? image.url ?? ''
    const clean = source.split('?')[0]
    const ext = clean.match(/\.(png|jpe?g|webp|gif)$/i)?.[0] ?? '.png'
    return `${image.id || 'image'}${ext}`
  }

  async function copyPrompt(prompt: string) {
    await copyText(prompt)
    app.notify('success', 'Prompt 已复制')
  }

  return (
    <div className={homeClasses.content}>
      <AccountReadinessStrip
        balance={app.balance}
        capability={capability.data}
        loadingCapability={capability.loading}
        onGenerate={() => app.navigate('genpic')}
        onRecharge={() => app.navigate('checkout')}
        onOpenGallery={() => app.navigate('public-gallery')}
      />

      <section
        className={homeClasses.carousel}
        role="button"
        tabIndex={0}
        aria-label="套用 Cinematic Luxury Watch 主题"
        onClick={() => app.navigate('genpic')}
        onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') app.navigate('genpic')
        }}
      >
        <img src={heroImage} alt="Carousel Banner" className={homeClasses.heroImage} />
        <div className={homeClasses.carouselOverlay}>
          <p className={homeClasses.heroKicker}>FEATURED SHOWCASE</p>
          <h2 className={homeClasses.heroTitle}>Cinematic Luxury Watch</h2>
          <p className={homeClasses.heroText}>探索如何通过精确的提示词与全能 PRO 模型，生成令人惊叹的高奢产品视觉资产。</p>
          <div className={homeClasses.actionRow}>
            <HomeButton isActive disabled={!hasAvailableModels(capability.data)} onClick={(event) => navigateFromButton(event, () => app.navigate('genpic'))}>开始生成</HomeButton>
            <HomeButton onClick={(event) => navigateFromButton(event, () => app.navigate('public-gallery'))}>查看公开图库</HomeButton>
            <HomeButton onClick={(event) => navigateFromButton(event, () => app.navigate('api-keys'))}>API 开放平台</HomeButton>
            <HomeButton onClick={(event) => navigateFromButton(event, () => app.navigate('profile'))}>{app.profile?.avatar_initials ?? '账户'}</HomeButton>
          </div>
        </div>
      </section>

      <section aria-labelledby="inspiration-title">
        <div className={homeClasses.sectionHead}>
          <div>
            <h3 id="inspiration-title" className={homeClasses.sectionTitle}>灵感瀑布流</h3>
            <span className={homeClasses.sectionDetail}>
              {gallery.loading ? '正在刷新公开图库...' : `精选 ${gallery.data?.items.length ?? 0} 张公开图片，可直接寻找提示词灵感`}
            </span>
          </div>
          <div className={homeClasses.filterGroup} aria-label="灵感筛选">
            <FilterButton active={filter === 'latest'} onClick={() => setFilter('latest')}>最新</FilterButton>
            <FilterButton active={filter === 'hot'} onClick={() => setFilter('hot')}>最热</FilterButton>
            <HomeButton onClick={() => void gallery.reload()}>刷新</HomeButton>
          </div>
        </div>

        <div className={homeClasses.masonry}>
          {cards.map((card) => (
            <button type="button" className={homeClasses.masonryItem} key={card.id} onClick={card.onClick}>
              {card.imageUrl ? <img src={card.imageUrl} alt={card.title} className={homeClasses.masonryImage} /> : <ImagePlaceholder />}
              <div className={homeClasses.cardBody}>
                <strong className={homeClasses.cardTitle}>{card.title}</strong>
                <span className={homeClasses.cardMeta}>{card.meta}</span>
              </div>
            </button>
          ))}
        </div>
      </section>
      {selected ? (
        <Modal title="公开图片详情" onClose={() => setSelected(null)}>
          <PublicImageDetail
            image={selected}
            imageUrl={imageUrl(selected)}
            onLike={(image) => void toggleReaction(image as ImageResult, 'like')}
            onFavorite={(image) => void toggleReaction(image as ImageResult, 'favorite')}
            onDownload={(image) => downloadImage(image as ImageResult)}
            onCopyPrompt={(prompt) => void copyPrompt(prompt)}
          />
        </Modal>
      ) : null}
    </div>
  )
}

function AccountReadinessStrip({ balance, capability, loadingCapability, onGenerate, onRecharge, onOpenGallery }: {
  balance: Balance | null
  capability: Capability | null
  loadingCapability: boolean
  onGenerate: () => void
  onRecharge: () => void
  onOpenGallery: () => void
}) {
  const account = homeAccountReadinessView(balance)
  const model = homeModelReadinessView(capability, loadingCapability)
  return (
    <section className={homeClasses.readiness} aria-label="账户与生图状态">
      <div className={homeClasses.readinessIntro}>
        <h2 className={homeClasses.readinessTitle}>开始第一张图</h2>
        <p className={homeClasses.readinessDetail}>
          {model.ready ? '账户额度和生图能力已就绪，可以直接进入工作台。' : '当前没有可用生图模型，先浏览公开广场获取提示词灵感。'}
        </p>
      </div>
      <ReadinessMetric label="可用积分" value={account.availableValue} />
      <ReadinessMetric
        label="体验额度"
        value={account.trialValue}
        detail={account.trialDetail}
        warning={account.trialWarning}
      />
      <ReadinessMetric label="模型状态" value={model.value} detail={model.detail} warning={model.warning} />
      <div className={homeClasses.readinessCta}>
        <HomeButton isActive disabled={!model.ready} onClick={onGenerate}>开始生成</HomeButton>
        <HomeButton onClick={account.secondaryAction === 'gallery' ? onOpenGallery : onRecharge}>
          {account.secondaryAction === 'gallery' ? '查看广场' : '充值积分'}
        </HomeButton>
      </div>
    </section>
  )
}

function ReadinessMetric({ label, value, detail, warning }: { label: string; value: string; detail?: string; warning?: boolean }) {
  return (
    <div className={cn(homeClasses.readinessMetric, warning && homeClasses.readinessMetricWarning)}>
      <span className={homeClasses.readinessLabel}>{label}</span>
      <strong className={cn(homeClasses.readinessValue, warning && homeClasses.readinessValueWarning)}>{value}</strong>
      {detail ? <p className={homeClasses.smallNote}>{detail}</p> : null}
    </div>
  )
}

function hasAvailableModels(capability: Capability | null) {
  return Boolean(capability?.model_groups?.length)
}

function navigateFromButton(event: React.MouseEvent<HTMLButtonElement>, navigate: () => void) {
  event.stopPropagation()
  navigate()
}

function HomeButton({ children, isActive = false, disabled = false, onClick }: { children: React.ReactNode; isActive?: boolean; disabled?: boolean; onClick: React.MouseEventHandler<HTMLButtonElement> }) {
  return (
    <button type="button" className={cn(homeClasses.button, isActive && homeClasses.buttonActive, disabled && homeClasses.buttonDisabled)} disabled={disabled} onClick={onClick}>
      {children}
    </button>
  )
}

function FilterButton({ children, active, onClick }: { children: React.ReactNode; active: boolean; onClick: () => void }) {
  return (
    <button type="button" aria-pressed={active} className={cn(homeClasses.button, active && homeClasses.buttonActive)} onClick={onClick}>
      {children}
    </button>
  )
}

function ImagePlaceholder() {
  return (
    <div className={homeClasses.placeholder}>
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1" aria-hidden="true">
        <rect x="3" y="3" width="18" height="18" rx="2" />
        <circle cx="8.5" cy="8.5" r="1.5" />
        <path d="M21 15l-5-5L5 21" />
      </svg>
    </div>
  )
}
